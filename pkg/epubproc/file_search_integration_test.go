package epubproc

import (
	"archive/zip"
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"
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
				IsRegex: false,
				Text: &SearchRequestText{
					Value:      "Holmes",
					IgnoreCase: false,
				},
			},
			Context: 0,
		}

		var results []*SearchResult
		ctx := context.Background()

		err := fs.Search(ctx, request, func(result *SearchResult) error {
			results = append(results, result)
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
				IsRegex: true,
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
				IsRegex: false,
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
				IsRegex: true,
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

	// test context with cancellation
	t.Run("ContextCancellation", func(t *testing.T) {
		fs := NewFileSearch(tempDir, 1, false)

		request := &SearchRequest{
			Query: SearchRequestQuery{
				IsRegex: true,
				Regex: &SearchRequestRegex{
					Pattern: "Holmes|Watson",
				},
			},
			Context: 0,
		}

		// create context that cancels quickly
		ctx, cancel := context.WithTimeout(context.Background(), 1*time.Nanosecond)
		defer cancel()

		var results []*SearchResult
		var mu sync.Mutex
		err := fs.Search(ctx, request, func(result *SearchResult) error {
			mu.Lock()
			results = append(results, result)
			mu.Unlock()
			return nil
		})

		// should return context error or complete quickly
		if err != nil && err != context.DeadlineExceeded && err != context.Canceled && !strings.Contains(err.Error(), "context") {
			t.Errorf("Expected context cancellation error or nil, got: %v", err)
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

	// test missing regex configuration
	t.Run("MissingRegexConfig", func(t *testing.T) {
		request := &SearchRequest{
			Query: SearchRequestQuery{
				IsRegex: true,

				// missing regex config
				Regex: nil,
			},
		}

		err := fs.Search(ctx, request, func(result *SearchResult) error {
			return nil
		})

		if err == nil || !strings.Contains(err.Error(), "regex configuration is required") {
			t.Errorf("Expected regex config error, got: %v", err)
		}
	})

	// test missing text configuration
	t.Run("MissingTextConfig", func(t *testing.T) {
		request := &SearchRequest{
			Query: SearchRequestQuery{
				IsRegex: false,

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
				IsRegex: true,
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

	// test handler error propagation
	t.Run("HandlerError", func(t *testing.T) {
		// create a test file
		createTestEPUB(tempDir, "error_test.epub", "<p>Find this text</p>")

		request := &SearchRequest{
			Query: SearchRequestQuery{
				IsRegex: false,
				Text: &SearchRequestText{
					Value: "Find",
				},
			},
		}

		expectedError := fmt.Errorf("handler error")
		err := fs.Search(ctx, request, func(result *SearchResult) error {
			// simulate handler error
			return expectedError
		})

		if err == nil || err.Error() != expectedError.Error() {
			t.Errorf("Expected handler error to be propagated, got: %v", err)
		}
	})

	// test non-existent directory
	t.Run("NonExistentDirectory", func(t *testing.T) {
		fs := NewFileSearch("/non/existent/path", 2, false)

		request := &SearchRequest{
			Query: SearchRequestQuery{
				IsRegex: false,
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
