package epubproc

import (
	"archive/zip"
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strings"

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

	var matches []Match

	for _, f := range r.File {
		if f.FileInfo().IsDir() {
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

	return matches, nil
}

// scanTextFile scans a plain text file for pattern matches.
func scanTextFile(r io.Reader, pattern *regexp.Regexp, fileName string, contextLines int) []Match {
	pooledSc := scannerPool.Get().(*pooledScanner)
	defer scannerPool.Put(pooledSc)
	pooledSc.reset(r)
	scanner := pooledSc.scanner

	// ise sliding window approach for memory efficiency
	lines := make([]string, 0, 512) // pre-allocate for ~512 lines (reduces reallocations)
	matches := make([]Match, 0, 16) // pre-allocate for expected matches
	lineNum := 0

	// for files without context, we can process line by line
	if contextLines == 0 {
		for scanner.Scan() {
			line := scanner.Text()
			if pattern.MatchString(line) {
				matches = append(matches, Match{
					Line:     strings.TrimSpace(line),
					FileName: fileName,
				})
			}
		}

		if err := scanner.Err(); err != nil {
			log.Error().Err(err).Str("file", fileName).Msg("error scanning text file")
			return nil
		}
		return matches
	}

	// for files with context, we need to track matched lines
	matchedLines := make(map[int]bool)

	// first pass: identify matching lines and build context
	for scanner.Scan() {
		line := scanner.Text()
		lines = append(lines, line)

		if pattern.MatchString(line) {
			// mark this line and surrounding context for inclusion
			start := max(lineNum-contextLines, 0)
			end := min(lineNum+contextLines+1, len(lines))
			for i := start; i < end; i++ {
				matchedLines[i] = true
			}
		}
		lineNum++
	}

	if err := scanner.Err(); err != nil {
		log.Error().Err(err).Str("file", fileName).Msg("error scanning text file")
		return nil
	}

	// second pass: build matches with context
	for i, line := range lines {
		if pattern.MatchString(line) {
			start := max(i-contextLines, 0)
			end := min(i+contextLines+1, len(lines))
			fullMatch := strings.Join(lines[start:end], "\n")
			matches = append(matches, Match{
				Line:     strings.TrimSpace(fullMatch),
				FileName: fileName,
			})
		}
	}

	return matches
}

// scanHTMLFile extracts text content from HTML and searches for pattern matches.
func scanHTMLFile(ctx context.Context, r io.Reader, pattern *regexp.Regexp, fileName string, contextLines int) []Match {
	pooledTok := tokenizerPool.Get().(*pooledTokenizer)
	defer tokenizerPool.Put(pooledTok)
	pooledTok.reset(r)
	z := pooledTok.tokenizer
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

		tt := z.Next()
		if tt == html.ErrorToken {
			// io.EOF is expected at the end of the file.
			if z.Err() != io.EOF {
				log.Error().Err(z.Err()).Str("file", fileName).Msg("error tokenizing html")
			}
			break
		}

		switch tt {
		case html.TextToken:
			// add a space before the text to ensure separation between words from adjacent tags
			// the final whitespace normalization will handle any extra spaces
			currentLine.WriteString(" ")
			currentLine.WriteString(string(z.Text()))

		case html.StartTagToken, html.EndTagToken, html.SelfClosingTagToken:
			tagName, _ := z.TagName()
			if isBlockLevelTag(string(tagName)) {
				flushLine()
			}
		}
	}

	// flush remaining text after the last tag
	flushLine()

	var matches []Match
	for i, line := range textLines {
		if pattern.MatchString(line) {
			start := max(i-contextLines, 0)
			end := min(i+contextLines+1, len(textLines))
			fullMatch := strings.Join(textLines[start:end], "\n")
			matches = append(matches, Match{
				Line:     strings.TrimSpace(fullMatch),
				FileName: fileName,
			})
		}
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
