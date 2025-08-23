package epubproc

import (
	"context"
	"fmt"
	"regexp"
	"strings"
	"testing"
)

// TestScanTextFileWithPool verifies that the scanner pool implementation
// correctly processes text files and finds pattern matches.
func TestScanTextFileWithPool(t *testing.T) {
	// prepare test data
	testText := "This is line 1\nThis contains pattern\nThis is line 3\nAnother pattern here\nFinal line"
	reader := strings.NewReader(testText)

	// compile a test pattern
	pattern, err := regexp.Compile("pattern")
	if err != nil {
		t.Fatalf("Failed to compile pattern: %v", err)
	}

	// test without context
	matches := scanTextFile(reader, pattern, "test.txt", 0)

	// verify we found the expected matches
	expectedMatches := 2
	if len(matches) != expectedMatches {
		t.Errorf("Expected %d matches, got %d", expectedMatches, len(matches))
	}

	// verify match content
	if !strings.Contains(matches[0].Line, "contains pattern") {
		t.Errorf("First match should contain 'contains pattern', got: %s", matches[0].Line)
	}

	if !strings.Contains(matches[1].Line, "Another pattern here") {
		t.Errorf("Second match should contain 'Another pattern here', got: %s", matches[1].Line)
	}

	// verify filename is set correctly
	for _, match := range matches {
		if match.FileName != "test.txt" {
			t.Errorf("Expected filename 'test.txt', got: %s", match.FileName)
		}
	}
}

// TestScanTextFileWithContext verifies that context lines work correctly with the pool.
func TestScanTextFileWithContext(t *testing.T) {
	testText := "Before line\nThis contains target\nAfter line"
	reader := strings.NewReader(testText)

	pattern, err := regexp.Compile("target")
	if err != nil {
		t.Fatalf("Failed to compile pattern: %v", err)
	}

	// test with 1 line of context
	matches := scanTextFile(reader, pattern, "test.txt", 1)

	if len(matches) != 1 {
		t.Fatalf("Expected 1 match, got %d", len(matches))
	}

	// should include before and after lines
	expectedLines := []string{"Before line", "This contains target", "After line"}
	fullMatch := matches[0].Line

	for _, expectedLine := range expectedLines {
		if !strings.Contains(fullMatch, expectedLine) {
			t.Errorf("Match should contain '%s', got: %s", expectedLine, fullMatch)
		}
	}
}

// TestScanHTMLFileWithPool verifies that the HTML tokenizer pool implementation
// correctly processes HTML files and finds pattern matches.
func TestScanHTMLFileWithPool(t *testing.T) {
	// test HTML with various tags
	testHTML := `<html><body>
		<p>This is paragraph one with target word.</p>
		<div>This is a div with another target.</div>
		<h1>Header without match</h1>
		<span>Inline text with target</span>
	</body></html>`

	reader := strings.NewReader(testHTML)
	pattern, err := regexp.Compile("target")
	if err != nil {
		t.Fatalf("Failed to compile pattern: %v", err)
	}

	// test without context
	ctx := context.Background()
	matches := scanHTMLFile(ctx, reader, pattern, "test.html", 0)

	// should find 3 matches (paragraph, div, and span)
	expectedMatches := 3
	if len(matches) != expectedMatches {
		t.Errorf("Expected %d matches, got %d", expectedMatches, len(matches))
		for i, match := range matches {
			t.Logf("Match %d: %s", i, match.Line)
		}
	}

	// verify filename is set correctly
	for _, match := range matches {
		if match.FileName != "test.html" {
			t.Errorf("Expected filename 'test.html', got: %s", match.FileName)
		}
	}
}

// TestScanHTMLFileWithContext verifies that context lines work correctly with HTML parsing.
func TestScanHTMLFileWithContext(t *testing.T) {
	testHTML := `<html><body>
		<p>Before paragraph</p>
		<p>This contains the target word</p>
		<p>After paragraph</p>
	</body></html>`

	reader := strings.NewReader(testHTML)
	pattern, err := regexp.Compile("target")
	if err != nil {
		t.Fatalf("Failed to compile pattern: %v", err)
	}

	// test with 1 line of context
	ctx := context.Background()
	matches := scanHTMLFile(ctx, reader, pattern, "test.html", 1)

	if len(matches) != 1 {
		t.Fatalf("Expected 1 match, got %d", len(matches))
	}

	// should include before and after paragraphs
	fullMatch := matches[0].Line
	if !strings.Contains(fullMatch, "Before paragraph") {
		t.Errorf("Match should contain 'Before paragraph', got: %s", fullMatch)
	} else if !strings.Contains(fullMatch, "target word") {
		t.Errorf("Match should contain 'target word', got: %s", fullMatch)
	} else if !strings.Contains(fullMatch, "After paragraph") {
		t.Errorf("Match should contain 'After paragraph', got: %s", fullMatch)
	}
}

// TestGetFileType verifies file type detection.
func TestGetFileType(t *testing.T) {
	tests := []struct {
		filename string
		expected string
	}{
		{"test.txt", "text"},
		{"page.html", "html"},
		{"content.xhtml", "html"},
		{"metadata.xml", "html"},
		{"image.png", ""},
		{"", ""},
		{"test", ""},
	}

	for _, test := range tests {
		result := getFileType(test.filename)
		if result != test.expected {
			t.Errorf("getFileType(%s): expected %s, got %s", test.filename, test.expected, result)
		}
	}
}

// TestShouldSkipFile verifies file skipping logic.
func TestShouldSkipFile(t *testing.T) {
	tests := []struct {
		filename string
		expected bool
	}{
		{"mimetype", true},
		{"META-INF/container.xml", true},
		{"cover.xhtml", true},
		{"toc.xhtml", true},
		{"sample_chapter.html", true},
		{"ads.xhtml", true},
		{"content/chapter1.xhtml", false},
		{"text/page1.txt", false},
		{"", false},
	}

	for _, test := range tests {
		result := shouldSkipFile(test.filename)
		if result != test.expected {
			t.Errorf("shouldSkipFile(%s): expected %t, got %t", test.filename, test.expected, result)
		}
	}
}

// TestMatchesMetadataFilters verifies metadata filtering logic.
func TestMatchesMetadataFilters(t *testing.T) {
	metadata := Metadata{
		Title:   "Test Book",
		Authors: []string{"John Doe", "Jane Smith"},
		Series:  "Test Series",
	}

	tests := []struct {
		name     string
		filters  *SearchRequestFilters
		expected bool
	}{
		{
			name:     "No filters",
			filters:  &SearchRequestFilters{},
			expected: true,
		},
		{
			name: "Author match",
			filters: &SearchRequestFilters{
				AuthorEquals: "John Doe",
			},
			expected: true,
		},
		{
			name: "Author no match",
			filters: &SearchRequestFilters{
				AuthorEquals: "Unknown Author",
			},
			expected: false,
		},
		{
			name: "Title match",
			filters: &SearchRequestFilters{
				TitleEquals: "Test Book",
			},
			expected: true,
		},
		{
			name: "Title no match",
			filters: &SearchRequestFilters{
				TitleEquals: "Different Book",
			},
			expected: false,
		},
		{
			name: "Series match",
			filters: &SearchRequestFilters{
				SeriesEquals: "Test Series",
			},
			expected: true,
		},
		{
			name: "Multiple filters match",
			filters: &SearchRequestFilters{
				AuthorEquals: "Jane Smith",
				TitleEquals:  "Test Book",
			},
			expected: true,
		},
		{
			name: "Multiple filters partial match",
			filters: &SearchRequestFilters{
				AuthorEquals: "John Doe",
				TitleEquals:  "Wrong Book",
			},
			expected: false,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			result := matchesMetadataFilters(metadata, test.filters)
			if result != test.expected {
				t.Errorf("Expected %t, got %t", test.expected, result)
			}
		})
	}
}

// TestScanTextFileErrors tests error handling in scanTextFile
func TestScanTextFileErrors(t *testing.T) {
	// test with invalid reader that causes scanner errors
	t.Run("ScannerError", func(t *testing.T) {
		// create a reader that will cause an error
		errorReader := &errorReader{}
		pattern, _ := regexp.Compile("test")

		matches := scanTextFile(errorReader, pattern, "test.txt", 0)

		// should return nil on scanner error
		if matches != nil {
			t.Errorf("Expected nil matches on scanner error, got %v", matches)
		}
	})

	// test context scanner error
	t.Run("ScannerErrorWithContext", func(t *testing.T) {
		errorReader := &errorReader{}
		pattern, _ := regexp.Compile("test")

		matches := scanTextFile(errorReader, pattern, "test.txt", 1)

		// should return nil on scanner error
		if matches != nil {
			t.Errorf("Expected nil matches on scanner error, got %v", matches)
		}
	})
}

// TestScanHTMLFileErrors tests error handling in scanHTMLFile
func TestScanHTMLFileErrors(t *testing.T) {
	// test with context cancellation during HTML parsing
	t.Run("ContextCancellation", func(t *testing.T) {
		// create HTML that will take some time to process
		largeHTML := "<html><body>"
		for i := 0; i < 200; i++ {
			largeHTML += "<p>Some content here</p>"
		}
		largeHTML += "</body></html>"

		reader := strings.NewReader(largeHTML)
		pattern, _ := regexp.Compile("content")

		// create context that cancels immediately
		ctx, cancel := context.WithCancel(context.Background())
		cancel()

		matches := scanHTMLFile(ctx, reader, pattern, "test.html", 0)

		// should return nil when context is cancelled
		if matches != nil {
			t.Errorf("Expected nil matches on cancelled context, got %v", matches)
		}
	})

	// test with malformed HTML that causes tokenizer errors
	t.Run("MalformedHTML", func(t *testing.T) {
		// HTML with unclosed tags and other issues
		malformedHTML := "<html><body><p>Unclosed paragraph<div><span>Nested"
		reader := strings.NewReader(malformedHTML)
		pattern, _ := regexp.Compile("paragraph")

		matches := scanHTMLFile(context.Background(), reader, pattern, "test.html", 0)

		// should handle malformed HTML gracefully and still find matches
		if len(matches) == 0 {
			t.Error("Expected to handle malformed HTML and find matches")
		}
	})
}

// errorReader is a helper that always returns an error when Read is called
type errorReader struct{}

func (er *errorReader) Read(p []byte) (n int, err error) {
	return 0, fmt.Errorf("simulated read error")
}
