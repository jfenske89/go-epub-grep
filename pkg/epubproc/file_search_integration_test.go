package epubproc

import (
	"archive/zip"
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
)

// createTestEPUB creates a minimal test file with specified content
func createTestEPUB(dir, filename, content string) (string, error) {
	epubPath := filepath.Join(dir, filename)

	// create a zip file
	zipFile, err := os.Create(epubPath)
	if err != nil {
		return "", err
	}
	defer zipFile.Close()

	writer := zip.NewWriter(zipFile)
	defer writer.Close()

	// add mimetype
	mimetypeFile, err := writer.Create("mimetype")
	if err != nil {
		return "", err
	}
	mimetypeFile.Write([]byte("application/epub+zip"))

	// add META-INF/container.xml
	containerFile, err := writer.Create("META-INF/container.xml")
	if err != nil {
		return "", err
	}
	containerFile.Write([]byte(`<?xml version="1.0"?>
<container version="1.0" xmlns="urn:oasis:names:tc:opendocument:xmlns:container">
  <rootfiles>
    <rootfile full-path="OEBPS/content.opf" media-type="application/oebps-package+xml"/>
  </rootfiles>
</container>`))

	// add content.opf
	opfFile, err := writer.Create("OEBPS/content.opf")
	if err != nil {
		return "", err
	}
	opfFile.Write([]byte(`<?xml version="1.0" encoding="UTF-8"?>
<package xmlns="http://www.idpf.org/2007/opf" version="2.0" unique-identifier="uuid_id">
  <metadata xmlns:dc="http://purl.org/dc/elements/1.1/">
    <dc:title>Test Book</dc:title>
    <dc:creator>Test Author</dc:creator>
    <dc:language>en</dc:language>
    <dc:identifier id="uuid_id">test-123</dc:identifier>
  </metadata>
  <manifest>
    <item href="chapter1.html" id="chapter1" media-type="application/xhtml+xml"/>
  </manifest>
  <spine>
    <itemref idref="chapter1"/>
  </spine>
</package>`))

	// add chapter content
	chapterFile, err := writer.Create("OEBPS/chapter1.html")
	if err != nil {
		return "", err
	}
	fmt.Fprintf(chapterFile, `<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE html>
<html xmlns="http://www.w3.org/1999/xhtml">
<head><title>Chapter 1</title></head>
<body>
<h1>Chapter 1</h1>
%s
</body>
</html>`, content)

	return epubPath, nil
}

// TestFileSearchIntegration tests the main Search method with real epub files
func TestFileSearchIntegration(t *testing.T) {
	// create temporary directory for test files
	tempDir, err := os.MkdirTemp("", "epub_test_*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tempDir)

	// create test files
	epub1, err := createTestEPUB(tempDir, "book1.epub", "<p>This contains the target word Holmes.</p><p>Another line without match.</p>")
	if err != nil {
		t.Fatalf("Failed to create test ePUB: %v", err)
	}

	_, err = createTestEPUB(tempDir, "book2.epub", "<p>Watson appeared in this book.</p><p>No matches here.</p>")
	if err != nil {
		t.Fatalf("Failed to create test ePUB: %v", err)
	}

	// test basic text search
	t.Run("BasicTextSearch", func(t *testing.T) {
		fs := NewFileSearch(tempDir, 2, false)

		request := &SearchRequest{
			Query: SearchRequestQuery{
				Text: &SearchRequestText{
					Value:      "Holmes",
					IgnoreCase: false,
				},
			},
			Context: 0,
		}

		var results []*SearchResult
		var mu sync.Mutex
		ctx := context.Background()

		err := fs.Search(ctx, request, func(result *SearchResult) error {
			mu.Lock()
			results = append(results, result)
			mu.Unlock()
			return nil
		})
		if err != nil {
			t.Fatalf("Search failed: %v", err)
		}

		// should find one match in book1
		if len(results) != 1 {
			t.Errorf("Expected 1 result, got %d", len(results))
		}

		if len(results) > 0 {
			if !strings.Contains(results[0].Path, "book1.epub") {
				t.Errorf("Expected match in book1.epub, got %s", results[0].Path)
			}
			if len(results[0].Matches) != 1 {
				t.Errorf("Expected 1 match, got %d", len(results[0].Matches))
			}
		}
	})

	// test regex search
	t.Run("RegexSearch", func(t *testing.T) {
		fs := NewFileSearch(tempDir, 2, false)

		request := &SearchRequest{
			Query: SearchRequestQuery{
				Regex: &SearchRequestRegex{
					Pattern: "Holmes|Watson",
				},
			},
			Context: 0,
		}

		var results []*SearchResult
		var mu sync.Mutex
		ctx := context.Background()

		err := fs.Search(ctx, request, func(result *SearchResult) error {
			mu.Lock()
			results = append(results, result)
			mu.Unlock()
			return nil
		})
		if err != nil {
			t.Fatalf("Search failed: %v", err)
		}

		// should find matches in both books
		if len(results) != 2 {
			t.Errorf("Expected 2 results, got %d", len(results))
		}
	})

	// test case-insensitive search
	t.Run("CaseInsensitiveSearch", func(t *testing.T) {
		fs := NewFileSearch(tempDir, 2, false)

		request := &SearchRequest{
			Query: SearchRequestQuery{
				Text: &SearchRequestText{
					Value:      "holmes",
					IgnoreCase: true,
				},
			},
			Context: 0,
		}

		var results []*SearchResult
		var mu sync.Mutex
		ctx := context.Background()

		err := fs.Search(ctx, request, func(result *SearchResult) error {
			mu.Lock()
			results = append(results, result)
			mu.Unlock()
			return nil
		})
		if err != nil {
			t.Fatalf("Search failed: %v", err)
		}

		// should find match despite case difference
		if len(results) != 1 {
			t.Errorf("Expected 1 result, got %d", len(results))
		}
	})

	// test files-in filter
	t.Run("FilesInFilter", func(t *testing.T) {
		fs := NewFileSearch(tempDir, 2, false)

		request := &SearchRequest{
			Query: SearchRequestQuery{
				Regex: &SearchRequestRegex{
					Pattern: "Holmes|Watson",
				},
			},
			Context: 0,
			Filters: &SearchRequestFilters{
				// only search book1
				FilesIn: []string{epub1},
			},
		}

		var results []*SearchResult
		var mu sync.Mutex
		ctx := context.Background()

		if err := fs.Search(ctx, request, func(result *SearchResult) error {
			mu.Lock()
			results = append(results, result)
			mu.Unlock()
			return nil
		}); err != nil {
			t.Fatalf("Search failed: %v", err)
		}

		// should only find match in book1
		if len(results) != 1 {
			t.Errorf("Expected 1 result, got %d", len(results))
		}

		if len(results) > 0 && !strings.Contains(results[0].Path, "book1.epub") {
			t.Errorf("Expected match in book1.epub, got %s", results[0].Path)
		}
	})

	// test word-boundary metadata filter matching (FilterMatchWord)
	t.Run("FilterMatchWord", func(t *testing.T) {
		fs := NewFileSearch(tempDir, 2, true)

		request := &SearchRequest{
			Query: SearchRequestQuery{
				Text: &SearchRequestText{Value: "Holmes"},
			},
			Context: 0,
			Filters: &SearchRequestFilters{
				// createTestEPUB hardcodes dc:creator to "Test Author"
				AuthorEquals: "Author",
				MatchMode:    FilterMatchWord,
			},
		}

		var results []*SearchResult
		var mu sync.Mutex
		ctx := context.Background()

		if err := fs.Search(ctx, request, func(result *SearchResult) error {
			mu.Lock()
			results = append(results, result)
			mu.Unlock()
			return nil
		}); err != nil {
			t.Fatalf("Search failed: %v", err)
		}

		// whole word "Author" should match "Test Author"
		if len(results) != 1 {
			t.Errorf("Expected 1 result, got %d", len(results))
		}
	})

	// test that a partial-word filter value does not match under FilterMatchWord
	t.Run("FilterMatchWordPartialNoMatch", func(t *testing.T) {
		fs := NewFileSearch(tempDir, 2, true)

		request := &SearchRequest{
			Query: SearchRequestQuery{
				Text: &SearchRequestText{Value: "Holmes"},
			},
			Context: 0,
			Filters: &SearchRequestFilters{
				AuthorEquals: "Auth",
				MatchMode:    FilterMatchWord,
			},
		}

		var results []*SearchResult
		var mu sync.Mutex
		ctx := context.Background()

		if err := fs.Search(ctx, request, func(result *SearchResult) error {
			mu.Lock()
			results = append(results, result)
			mu.Unlock()
			return nil
		}); err != nil {
			t.Fatalf("Search failed: %v", err)
		}

		// "Auth" is not a whole word within "Test Author"
		if len(results) != 0 {
			t.Errorf("Expected 0 results, got %d", len(results))
		}
	})

	// cancelling from inside the handler must stop the search before any other file is handled
	t.Run("ContextCancellation", func(t *testing.T) {
		dir := t.TempDir()
		for i := range 5 {
			if _, err := createTestEPUB(dir, fmt.Sprintf("book%d.epub", i), "<p>Holmes and Watson</p>"); err != nil {
				t.Fatalf("Failed to create test ePUB: %v", err)
			}
		}

		// one worker makes the handler call count deterministic
		fs := NewFileSearch(dir, 1, false)
		request := &SearchRequest{
			Query: SearchRequestQuery{
				Regex: &SearchRequestRegex{Pattern: "Holmes|Watson"},
			},
		}

		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()

		var handlerCalls int
		err := fs.Search(ctx, request, func(result *SearchResult) error {
			handlerCalls++
			cancel()
			return nil
		})

		if !errors.Is(err, context.Canceled) {
			t.Errorf("Expected context.Canceled, got: %v", err)
		}
		if handlerCalls != 1 {
			t.Errorf("Expected handler to run once before cancel took effect, got %d calls", handlerCalls)
		}
	})
}

// TestFileSearchErrorCases tests error handling in the Search method
func TestFileSearchErrorCases(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "epub_error_test_*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tempDir)

	fs := NewFileSearch(tempDir, 2, false)
	ctx := context.Background()

	// test missing text configuration
	t.Run("MissingTextConfig", func(t *testing.T) {
		request := &SearchRequest{
			Query: SearchRequestQuery{
				// missing text config
				Text: nil,
			},
		}

		err := fs.Search(ctx, request, func(result *SearchResult) error {
			return nil
		})

		if err == nil || !strings.Contains(err.Error(), "text configuration is required") {
			t.Errorf("Expected text config error, got: %v", err)
		}
	})

	// test invalid regex pattern
	t.Run("InvalidRegexPattern", func(t *testing.T) {
		request := &SearchRequest{
			Query: SearchRequestQuery{
				Regex: &SearchRequestRegex{
					// invalid regex
					Pattern: "[invalid",
				},
			},
		}

		err := fs.Search(ctx, request, func(result *SearchResult) error {
			return nil
		})

		if err == nil || !strings.Contains(err.Error(), "invalid pattern") {
			t.Errorf("Expected invalid pattern error, got: %v", err)
		}
	})

	// a handler error must be returned to the caller and stop further work
	t.Run("HandlerError", func(t *testing.T) {
		dir := t.TempDir()
		for i := range 5 {
			if _, err := createTestEPUB(dir, fmt.Sprintf("book%d.epub", i), "<p>Find this text</p>"); err != nil {
				t.Fatalf("Failed to create test ePUB: %v", err)
			}
		}

		request := &SearchRequest{
			Query: SearchRequestQuery{
				Text: &SearchRequestText{Value: "Find"},
			},
		}

		errHandler := errors.New("handler error")

		// one worker makes the handler call count deterministic
		var handlerCalls int
		err := NewFileSearch(dir, 1, false).Search(ctx, request, func(result *SearchResult) error {
			handlerCalls++
			return errHandler
		})

		if !errors.Is(err, errHandler) {
			t.Errorf("Expected handler error to be propagated, got: %v", err)
		}
		if handlerCalls != 1 {
			t.Errorf("Expected handler to stop after its first error, got %d calls", handlerCalls)
		}
	})

	// a negative context would otherwise cause a slice-bounds panic when building context windows
	t.Run("NegativeContext", func(t *testing.T) {
		dir := t.TempDir()
		if _, err := createTestEPUB(dir, "book.epub", "<p>Find this text</p>"); err != nil {
			t.Fatalf("Failed to create test ePUB: %v", err)
		}

		request := &SearchRequest{
			Query: SearchRequestQuery{
				Text: &SearchRequestText{Value: "Find"},
			},
			Context: -1,
		}

		err := NewFileSearch(dir, 1, false).Search(ctx, request, func(result *SearchResult) error {
			t.Error("handler must not run for an invalid request")
			return nil
		})

		if err == nil || !strings.Contains(err.Error(), "context must not be negative") {
			t.Errorf("Expected negative context error, got: %v", err)
		}
	})

	// test non-existent directory
	t.Run("NonExistentDirectory", func(t *testing.T) {
		fs := NewFileSearch("/non/existent/path", 2, false)

		request := &SearchRequest{
			Query: SearchRequestQuery{
				Text: &SearchRequestText{
					Value: "test",
				},
			},
		}

		err := fs.Search(ctx, request, func(result *SearchResult) error {
			return nil
		})

		// should handle the error gracefully (filepath.WalkDir will return an error)
		if err == nil {
			t.Error("Expected error for non-existent directory")
		}
	})
}
