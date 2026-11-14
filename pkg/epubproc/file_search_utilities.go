package epubproc

import (
	"archive/zip"
	"context"
	"encoding/xml"
	"errors"
	"fmt"
	"io"
	"path/filepath"
	"regexp"
	"slices"
	"strings"

	"github.com/kapmahc/epub"
	"github.com/rs/zerolog/log"
	"golang.org/x/net/html"
)

// grepInEpub searches for a compiled regex pattern within a single epub file.
func grepInEpub(ctx context.Context, epubPath string, pattern *regexp.Regexp, contextLines int) ([]Match, error) {
	r, err := zip.OpenReader(epubPath)
	if err != nil {
		return nil, fmt.Errorf("failed to open epub '%s': %w", epubPath, err)
	}
	defer func() {
		if err := r.Close(); err != nil {
			log.Warn().Err(err).
				Str("epub", epubPath).
				Msg("failed to close epub reader")
		}
	}()

	var fileToChapter map[string]string

	var matches []Match

	// 1st pass to process toc.ncx for priority chapter info
	for _, f := range r.File {
		if f.FileInfo().IsDir() {
			continue
		}

		if strings.Contains(strings.ToLower(f.Name), "toc.ncx") {
			fileToChapter = processTableOfContents(f)
			break
		}
	}

	if fileToChapter == nil {
		fileToChapter = make(map[string]string, 10)
	}

	// the combined size of the files that are scanned, checked against limits.total
	var scannedSize int64

	// process all other files
	for _, f := range r.File {
		if f.FileInfo().IsDir() {
			continue
		}

		// secondary chapter processing
		if strings.Contains(strings.ToLower(f.Name), "content.opf") {
			processContentOPF(f, fileToChapter)
			continue
		}

		// skip non-content files (metadata, navigation, promotional content)
		if shouldSkipFile(f.Name) {
			continue
		}

		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		default:
		}

		kind := getFileType(f.Name)
		if kind == fileTypeOther {
			// images, styles and fonts have no text to search
			continue
		}

		// a file or an epub over the limits is treated as an error for the whole book, because skipping only
		// part of it would return results that look complete
		if err := checkEntrySize(f, limits.content); err != nil {
			return nil, fmt.Errorf("epub '%s': %w", epubPath, err)
		}
		scannedSize += declaredSize(f)
		if scannedSize > limits.total {
			return nil, fmt.Errorf("epub '%s' has more than %d bytes of text: %w", epubPath, limits.total, errEntryTooLarge)
		}

		rc, err := f.Open()
		if err != nil {
			log.Warn().Err(err).
				Str("file", f.Name).
				Str("epub", epubPath).
				Msg("failed to open file in epub")
			continue
		}

		// the declared size was checked above, so this cap only matters if the zip reader ever stops enforcing it
		reader := newLimitedReader(rc, limits.content)

		var fileMatches []Match
		switch kind {
		case fileTypeText:
			fileMatches = scanTextFile(reader, pattern, f.Name, contextLines)
		case fileTypeHTML:
			fileMatches = scanHTMLFile(ctx, reader, pattern, f.Name, contextLines)
		}

		// close the file immediately after processing
		if err := rc.Close(); err != nil {
			log.Warn().Err(err).
				Str("file", f.Name).
				Msg("failed to close file in epub")
		}

		matches = append(matches, fileMatches...)
	}

	// scanHTMLFile stops quietly when the context ends, so a cancelled scan would look like "no matches"
	if err := ctx.Err(); err != nil {
		return nil, err
	}

	for i := range matches {
		// get the base file name
		paths := strings.Split(matches[i].FileName, "/")
		baseName := paths[len(paths)-1]

		if fn, ok := fileToChapter[baseName]; ok {
			matches[i].Metadata = &MatchMetadata{Chapter: &fn}
		}
	}

	return matches, nil
}

func processXMLFile(f *zip.File, handler func(xmlBytes []byte)) {
	if err := checkEntrySize(f, limits.xml); err != nil {
		log.Warn().Err(err).Str("file", f.Name).Msg("skipping file in epub that is too large")
		return
	}

	rc, err := f.Open()
	if err != nil {
		log.Warn().Err(err).
			Str("file", f.Name).
			Msg("failed to open file in epub")
		return
	}
	defer func() {
		if err := rc.Close(); err != nil {
			log.Warn().Err(err).
				Str("file", f.Name).
				Msg("failed to close file in epub")
		}
	}()

	xmlBytes, err := io.ReadAll(newLimitedReader(rc, limits.xml))
	if err != nil {
		log.Warn().Err(err).
			Str("file", f.Name).
			Msg("failed to read file in epub")
		return
	}

	handler(xmlBytes)
}

func processTableOfContents(f *zip.File) map[string]string {
	fileToChapter := make(map[string]string, 10)
	processXMLFile(f, func(xmlBytes []byte) {
		var ncx epub.Ncx
		if err := xml.Unmarshal(xmlBytes, &ncx); err != nil {
			log.Warn().Err(err).
				Str("file", f.Name).
				Msg("failed to unmarshal file in epub")
			return
		}

		for _, np := range ncx.Points {
			paths := strings.Split(np.Content.Src, "/")
			baseName := paths[len(paths)-1]
			fileToChapter[baseName] = np.Text

			// remove anchor tags as an alternative lookup
			// the first heading in a file wins, and an entry that names the file itself is never replaced
			if strings.Contains(baseName, "#") {
				hashes := strings.Split(baseName, "#")
				altKey := hashes[0]
				if _, ok := fileToChapter[altKey]; !ok {
					fileToChapter[altKey] = np.Text
				}
			}
		}
	})

	return fileToChapter
}

func processContentOPF(f *zip.File, fileToChapter map[string]string) {
	processXMLFile(f, func(xmlBytes []byte) {
		var opf epub.Opf
		if err := xml.Unmarshal(xmlBytes, &opf); err != nil {
			log.Warn().Err(err).
				Str("file", f.Name).
				Msg("failed to unmarshal file in epub")
			return
		}

		for _, manifest := range opf.Manifest {
			paths := strings.Split(manifest.Href, "/")
			baseName := paths[len(paths)-1]
			if _, ok := fileToChapter[baseName]; !ok {
				fileToChapter[baseName] = manifest.ID
			}
		}
	})
}

// scanTextFile scans a plain text file for pattern matches.
func scanTextFile(r io.Reader, pattern *regexp.Regexp, fileName string, contextLines int) []Match {
	// the pool's New function only creates *pooledScanner, so this assertion cannot fail
	pooledSc := scannerPool.Get().(*pooledScanner)
	defer pooledSc.release()
	pooledSc.reset(r)
	scanner := pooledSc.scanner

	// for files without context, we can process line by line
	if contextLines == 0 {
		matches := make([]Match, 0, 16) // pre-allocate for expected matches
		for scanner.Scan() {
			line := scanner.Text()
			if pattern.MatchString(line) {
				match := Match{
					Line:     strings.TrimSpace(line),
					FileName: fileName,
				}
				matches = append(matches, match)
			}
		}

		// a line over the scanner limit stops the scan, but the matches found before it are still valid
		if err := scanner.Err(); err != nil {
			log.Error().Err(err).Str("file", fileName).Msg("error scanning text file, returning matches found so far")
		}
		return matches
	}

	lines := make([]string, 0, 512)    // pre-allocate for ~512 lines (reduces reallocations)
	matchedLines := make([]int, 0, 16) // pre-allocate for expected matched lines

	// compile list of lines and identify matching lines
	for i := 0; scanner.Scan(); i++ {
		line := scanner.Text()
		lines = append(lines, line)

		if pattern.MatchString(line) {
			matchedLines = append(matchedLines, i)
		}
	}

	if err := scanner.Err(); err != nil {
		log.Error().Err(err).Str("file", fileName).Msg("error scanning text file, returning matches found so far")
	}

	return createContextMatches(matchedLines, lines, fileName, contextLines)
}

// scanHTMLFile extracts text content from HTML and searches for pattern matches.
func scanHTMLFile(ctx context.Context, r io.Reader, pattern *regexp.Regexp, fileName string, contextLines int) []Match {
	tokenizer := html.NewTokenizer(r)
	textLines := make([]string, 0, 256) // pre-allocate for ~256 lines (typical HTML file)
	var currentLine strings.Builder
	currentLine.Grow(512) // pre-allocate for typical line length

	flushLine := func() {
		// normalize whitespace by splitting on fields and rejoining with single spaces
		// this correctly handles text from multiple tags and removes extra whitespace
		line := strings.Join(strings.Fields(currentLine.String()), " ")
		if line != "" {
			textLines = append(textLines, line)
		}
		currentLine.Reset()
	}

	// text inside <script> and <style> is code, not book text, so it must not be searchable
	inCode := false

	tokenCount := 0
	for {
		// check context cancellation every 100 tokens for responsiveness
		if tokenCount%100 == 0 {
			select {
			case <-ctx.Done():
				return nil
			default:
			}
		}
		tokenCount++

		tt := tokenizer.Next()
		if tt == html.ErrorToken {
			// io.EOF is expected at the end of the file.
			if !errors.Is(tokenizer.Err(), io.EOF) {
				log.Error().Err(tokenizer.Err()).Str("file", fileName).Msg("error tokenizing html")
			}
			break
		}

		switch tt {
		case html.TextToken:
			if inCode {
				break
			}

			// do not add a space between text tokens: inline tags such as <i> or <span> can split one
			// word or phrase ("<span>T</span>he"), and a space would stop a search for it from matching.
			// the source text already has the spaces that belong between words, and block tags flush the line.
			currentLine.Write(tokenizer.Text())

		case html.StartTagToken, html.EndTagToken, html.SelfClosingTagToken:
			// the second value only says whether the tag has attributes, which does not matter here
			tagName, _ := tokenizer.TagName()
			name := string(tagName)

			if name == "script" || name == "style" {
				inCode = tt == html.StartTagToken
			}
			if isBlockLevelTag(name) {
				flushLine()
			}
		}
	}

	// flush remaining text after the last tag
	flushLine()

	var matchedLines []int
	for i, line := range textLines {
		if pattern.MatchString(line) {
			matchedLines = append(matchedLines, i)
		}
	}

	return createContextMatches(matchedLines, textLines, fileName, contextLines)
}

// createContextMatches compiles matches with context lines, merging overlapping context windows.
func createContextMatches(matchedLines []int, lines []string, fileName string, contextLines int) []Match {
	// without context, each match is independent
	if contextLines == 0 {
		matches := make([]Match, 0, len(matchedLines))
		for _, idx := range matchedLines {
			match := Match{
				Line:     strings.TrimSpace(lines[idx]),
				FileName: fileName,
			}
			matches = append(matches, match)
		}
		return matches
	}

	type window struct {
		start int
		end   int
	}

	var windows []window
	var windowIndex, previousEnd int

	// build context windows
	for i := range matchedLines {
		start := max(matchedLines[i]-contextLines, 0)
		end := min(matchedLines[i]+contextLines+1, len(lines))

		if len(windows) == 0 {
			// start the first window
			windows = append(windows, window{
				start: start,
				end:   end,
			})

			previousEnd = end
			continue
		}

		if start <= previousEnd {
			// extend the window
			windows[windowIndex].end = end
		} else {
			// start a new window
			windowIndex++
			windows = append(windows, window{
				start: start,
				end:   end,
			})
		}

		previousEnd = end
	}

	// compile matches
	matches := make([]Match, 0, len(windows))
	for i := range windows {
		start := windows[i].start
		end := windows[i].end
		fullMatch := strings.Join(lines[start:end], "\n")
		match := Match{
			Line:     strings.TrimSpace(fullMatch),
			FileName: fileName,
		}
		matches = append(matches, match)
	}
	return matches
}

// fileType says how a file inside an epub is scanned.
type fileType int

const (
	fileTypeOther fileType = iota
	fileTypeText
	fileTypeHTML
)

// getFileType determines the file type for content scanning based on file extension.
func getFileType(name string) fileType {
	ext := strings.ToLower(filepath.Ext(name))
	switch ext {
	case ".txt":
		return fileTypeText
	case ".html", ".xhtml", ".xml":
		return fileTypeHTML
	default:
		return fileTypeOther
	}
}

// isBlockLevelTag reports whether a tag ends the current line of text.
func isBlockLevelTag(tagName string) bool {
	switch tagName {
	case "p", "div", "br", "h1", "h2", "h3", "h4", "h5", "h6", "li", "blockquote", "hr", "pre", "tr", "table",
		"td", "th", "dt", "dd", "section", "figcaption":
		return true
	default:
		return false
	}
}

// skippedFileNames are standard epub navigation and metadata files that are not book text.
var skippedFileNames = []string{
	"cover.xhtml", "toc.xhtml", "titlepage.xhtml", "copyright.xhtml",
	"imprint.xhtml", "dedication.xhtml", "dedication-1.xhtml",
	"license.xhtml", "license-1.xhtml", "colophon.xhtml",
	"about.xhtml", "about-1.xhtml", "acknowledgments.xhtml",
	"appendix.xhtml", "afterword.xhtml", "notes.xhtml",
	"bibliography.xhtml", "index.xhtml", "epilogue.xhtml",
	"glossary.xhtml", "extra.xhtml", "ads.xhtml", "trailer.xhtml",
}

// shouldSkipFile determines whether a file should be excluded from content scanning.
func shouldSkipFile(fileName string) bool {
	// compare in lowercase so that "Mimetype" or "COVER.XHTML" are treated like the standard names
	lowerName := strings.ToLower(fileName)
	baseName := strings.ToLower(filepath.Base(fileName))

	// skip epub metadata files
	if lowerName == "mimetype" || lowerName == "meta-inf/container.xml" {
		return true
	}

	if slices.Contains(skippedFileNames, baseName) {
		return true
	}

	// skip files containing promotional or sample content
	promoKeywords := []string{"sample", "advert", "promo", "teaser"}
	for _, keyword := range promoKeywords {
		if strings.Contains(lowerName, keyword) {
			return true
		}
	}

	return false
}

// filterMatchers holds precompiled word-boundary regexes for a request's metadata filters,
// used when SearchRequestFilters.MatchMode is FilterMatchWord. Fields are nil for filters that
// are unset or when MatchMode is FilterMatchExact.
type filterMatchers struct {
	author *regexp.Regexp
	series *regexp.Regexp
	title  *regexp.Regexp
}

// newFilterMatchers compiles word-boundary regexes for the given filters, once per search
// request. It returns nil when filters is nil or MatchMode is not FilterMatchWord, since
// FilterMatchExact needs no compiled pattern.
func newFilterMatchers(filters *SearchRequestFilters) (*filterMatchers, error) {
	if filters == nil || filters.MatchMode != FilterMatchWord {
		return nil, nil
	}

	var matchers filterMatchers
	var err error

	if filters.AuthorEquals != "" {
		if matchers.author, err = compileWordBoundaryPattern(filters.AuthorEquals); err != nil {
			return nil, fmt.Errorf("invalid author filter: %w", err)
		}
	}
	if filters.SeriesEquals != "" {
		if matchers.series, err = compileWordBoundaryPattern(filters.SeriesEquals); err != nil {
			return nil, fmt.Errorf("invalid series filter: %w", err)
		}
	}
	if filters.TitleEquals != "" {
		if matchers.title, err = compileWordBoundaryPattern(filters.TitleEquals); err != nil {
			return nil, fmt.Errorf("invalid title filter: %w", err)
		}
	}

	return &matchers, nil
}

// compileWordBoundaryPattern builds a case-insensitive regex that matches value as a whole word
// or phrase, bounded by non-letter/non-digit characters (or string start/end) on both ends.
// Unicode letter/number classes are used instead of \b, since Go's regexp \b only recognizes
// ASCII word characters.
func compileWordBoundaryPattern(value string) (*regexp.Regexp, error) {
	pattern := `(?i)(^|[^\p{L}\p{N}])` + regexp.QuoteMeta(value) + `([^\p{L}\p{N}]|$)`
	return regexp.Compile(pattern)
}

// matchesMetadataFilters checks if the given metadata matches the specified filters. matchers
// holds precompiled word-boundary regexes for FilterMatchWord mode, or nil for FilterMatchExact.
func matchesMetadataFilters(metadata Metadata, filters *SearchRequestFilters, matchers *filterMatchers) bool {
	var authorPattern, seriesPattern, titlePattern *regexp.Regexp
	if matchers != nil {
		authorPattern, seriesPattern, titlePattern = matchers.author, matchers.series, matchers.title
	}

	// handle AuthorEquals filter
	if filters.AuthorEquals != "" {
		found := false
		for _, author := range metadata.Authors {
			if matchesFilterValue(author, filters.AuthorEquals, authorPattern) {
				found = true
				break
			}
		}
		if !found {
			return false
		}
	}

	// handle SeriesEquals filter
	if filters.SeriesEquals != "" {
		if !matchesFilterValue(metadata.Series, filters.SeriesEquals, seriesPattern) {
			return false
		}
	}

	// handle TitleEquals filter
	if filters.TitleEquals != "" {
		if !matchesFilterValue(metadata.Title, filters.TitleEquals, titlePattern) {
			return false
		}
	}

	return true
}

// matchesFilterValue compares a metadata field against a filter value: word-boundary containment
// via wordPattern when set (FilterMatchWord), or case-insensitive equality otherwise (FilterMatchExact).
func matchesFilterValue(field, filterValue string, wordPattern *regexp.Regexp) bool {
	if wordPattern != nil {
		return wordPattern.MatchString(field)
	}
	return strings.EqualFold(field, filterValue)
}
