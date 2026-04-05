package epubproc

import (
	"archive/zip"
	"context"
	"encoding/xml"
	"fmt"
	"io"
	"os"
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
	// get file info for better error context
	fileInfo, fileErr := os.Stat(epubPath)

	r, err := zip.OpenReader(epubPath)
	if err != nil {
		if fileErr == nil {
			return nil, fmt.Errorf("failed to open epub '%s' (size: %d bytes): %w", epubPath, fileInfo.Size(), err)
		}
		return nil, fmt.Errorf("failed to open epub '%s': %w", epubPath, err)
	}
	defer func() {
		if err := r.Close(); err != nil {
			log.Warn().Err(err).
				Str("epub", epubPath).
				Msg("failed to close epub reader")
		}
	}()

	fileToChapter := make(map[string]string, 10)

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

	// process all other files
	for _, f := range r.File {
		if f.FileInfo().IsDir() {
			continue
		}

		// secondary chapter processing
		if strings.Contains(strings.ToLower(f.Name), "content.opf") {
			processContentOpf(f, fileToChapter)
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

		rc, err := f.Open()
		if err != nil {
			log.Warn().Str("file", f.Name).
				Str("epub", epubPath).
				Msg("failed to open file in epub")
			continue
		}

		var fileMatches []Match
		switch getFileType(f.Name) {
		case "text":
			fileMatches = scanTextFile(rc, pattern, f.Name, contextLines)
		case "html":
			fileMatches = scanHTMLFile(ctx, rc, pattern, f.Name, contextLines)
		}

		// Close the file immediately after processing
		if err := rc.Close(); err != nil {
			log.Warn().Err(err).
				Str("file", f.Name).
				Msg("failed to close file in epub")
		}

		matches = append(matches, fileMatches...)
	}

	for i := range matches {
		match := matches[i]

		// get the base file name
		paths := strings.Split(match.FileName, "/")
		baseName := paths[len(paths)-1]

		if fn, ok := fileToChapter[baseName]; ok {
			match.Metadata = &MatchMetadata{
				Chapter: &fn,
			}
			matches[i] = match
		}
	}

	return matches, nil
}

func processXmlFile(f *zip.File, handler func(xmlBytes []byte)) {
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

	xmlBytes, err := io.ReadAll(rc)
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
	processXmlFile(f, func(xmlBytes []byte) {
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
			if strings.Contains(baseName, "#") {
				hashes := strings.Split(baseName, "#")
				altKey := hashes[0]
				fileToChapter[altKey] = np.Text
			}
		}
	})

	return fileToChapter
}

func processContentOpf(f *zip.File, fileToChapter map[string]string) {
	processXmlFile(f, func(xmlBytes []byte) {
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
	pooledSc := scannerPool.Get().(*pooledScanner)
	defer scannerPool.Put(pooledSc)
	pooledSc.reset(r)
	scanner := pooledSc.scanner

	// use sliding window approach for memory efficiency
	lines := make([]string, 0, 512)    // pre-allocate for ~512 lines (reduces reallocations)
	matchedLines := make([]int, 0, 16) // pre-allocate for expected matched lines

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

		if err := scanner.Err(); err != nil {
			log.Error().Err(err).Str("file", fileName).Msg("error scanning text file")
			return nil
		}
		return matches
	}

	// compile list of lines and identify matching lines
	for i := 0; scanner.Scan(); i++ {
		line := scanner.Text()
		lines = append(lines, line)

		if pattern.MatchString(line) {
			matchedLines = append(matchedLines, i)
		}
	}

	if err := scanner.Err(); err != nil {
		log.Error().Err(err).Str("file", fileName).Msg("error scanning text file")
		return nil
	}

	return createContextMatches(matchedLines, lines, fileName, contextLines)
}

// scanHTMLFile extracts text content from HTML and searches for pattern matches.
func scanHTMLFile(ctx context.Context, r io.Reader, pattern *regexp.Regexp, fileName string, contextLines int) []Match {
	tokenizer := html.NewTokenizer(r)
	textLines := make([]string, 0, 256) // pre-allocate for ~256 lines (typical HTML file)
	var currentLine strings.Builder
	currentLine.Grow(512) // pre-allocate for typical line length

	// isBlockLevelTag checks if a tag is a block-level element that should create a line break
	isBlockLevelTag := func(tagName string) bool {
		switch tagName {
		case "p", "div", "br", "h1", "h2", "h3", "h4", "h5", "h6", "li", "blockquote", "hr", "pre", "tr", "table":
			return true
		default:
			return false
		}
	}

	// flushLine processes the accumulated text in currentLine, normalizes it, and appends it to textLines unless empty
	flushLine := func() {
		// normalize whitespace by splitting on fields and rejoining with single spaces
		// this correctly handles text from multiple tags and removes extra whitespace
		line := strings.Join(strings.Fields(currentLine.String()), " ")
		if line != "" {
			textLines = append(textLines, line)
		}
		currentLine.Reset()
	}

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
			if tokenizer.Err() != io.EOF {
				log.Error().Err(tokenizer.Err()).Str("file", fileName).Msg("error tokenizing html")
			}
			break
		}

		switch tt {
		case html.TextToken:
			// add a space before the text to ensure separation between words from adjacent tags
			// the final whitespace normalization will handle any extra spaces
			currentLine.WriteString(" ")
			currentLine.WriteString(string(tokenizer.Text()))

		case html.StartTagToken, html.EndTagToken, html.SelfClosingTagToken:
			tagName, _ := tokenizer.TagName()
			if isBlockLevelTag(string(tagName)) {
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

// getFileType determines the file type for content scanning based on file extension.
func getFileType(name string) string {
	ext := strings.ToLower(filepath.Ext(name))
	switch ext {
	case ".txt":
		return "text"
	case ".html", ".xhtml", ".xml":
		return "html"
	default:
		return ""
	}
}

// shouldSkipFile determines whether a file should be excluded from content scanning.
func shouldSkipFile(fileName string) bool {
	// Normalize the file name to lowercase for comparison
	lowerName := strings.ToLower(fileName)
	baseName := strings.ToLower(filepath.Base(fileName))

	// skip epub metadata files
	if fileName == "mimetype" || fileName == "META-INF/container.xml" {
		return true
	}

	// Skip standard epub navigation and metadata files
	skipFiles := []string{
		"cover.xhtml", "toc.xhtml", "titlepage.xhtml", "copyright.xhtml",
		"imprint.xhtml", "dedication.xhtml", "dedication-1.xhtml",
		"license.xhtml", "license-1.xhtml", "colophon.xhtml",
		"about.xhtml", "about-1.xhtml", "acknowledgments.xhtml",
		"appendix.xhtml", "afterword.xhtml", "notes.xhtml",
		"bibliography.xhtml", "index.xhtml", "epilogue.xhtml",
		"glossary.xhtml", "extra.xhtml", "ads.xhtml", "trailer.xhtml",
	}

	if slices.Contains(skipFiles, baseName) {
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

// matchesMetadataFilters checks if the given metadata matches the specified filters.
func matchesMetadataFilters(metadata Metadata, filters *SearchRequestFilters) bool {
	// handle AuthorEquals filter
	if filters.AuthorEquals != "" {
		found := false
		for _, author := range metadata.Authors {
			if strings.EqualFold(author, filters.AuthorEquals) {
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
		if !strings.EqualFold(metadata.Series, filters.SeriesEquals) {
			return false
		}
	}

	// handle TitleEquals filter
	if filters.TitleEquals != "" {
		if !strings.EqualFold(metadata.Title, filters.TitleEquals) {
			return false
		}
	}

	return true
}
