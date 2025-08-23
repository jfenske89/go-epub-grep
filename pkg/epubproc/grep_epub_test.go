package epubproc

import (
	"archive/zip"
	"context"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
	"time"
)

// createTestZIPWithFiles creates a test ZIP file with specified files and content
func createTestZIPWithFiles(path string, files map[string]string) error {
	zipFile, err := os.Create(path)
	if err != nil {
		return err
	}
	defer zipFile.Close()

	writer := zip.NewWriter(zipFile)
	defer writer.Close()

	for filename, content := range files {
		file, err := writer.Create(filename)
		if err != nil {
			return err
		}
		file.Write([]byte(content))
	}

	return nil
}

// TestGrepInEpub tests the grepInEpub function with various scenarios
func TestGrepInEpub(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "grep_epub_test_*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tempDir)

	// test with mixed file types
	t.Run("MixedFileTypes", func(t *testing.T) {
		epubPath := filepath.Join(tempDir, "mixed.epub")
		files := map[string]string{
			"chapter1.txt":   "This is plain text with target word Holmes.",
			"chapter2.html":  "<p>This is HTML with target word Watson.</p>",
			"chapter3.xhtml": "<p>Another target Holmes in XHTML.</p>",

			// these assets should be skipped
			"image.png": "binary data",
			"style.css": ".target { color: red; }",
		}

		if err := createTestZIPWithFiles(epubPath, files); err != nil {
			t.Fatalf("Failed to create test ePUB: %v", err)
		}

		pattern, _ := regexp.Compile("target")
		matches, err := grepInEpub(context.Background(), epubPath, pattern, 0)
		if err != nil {
			t.Fatalf("grepInEpub failed: %v", err)
		}

		// should find matches in text and HTML files, but not CSS or binary files
		expectedFiles := []string{"chapter1.txt", "chapter2.html", "chapter3.xhtml"}
		if len(matches) != len(expectedFiles) {
			t.Errorf("Expected %d matches, got %d", len(expectedFiles), len(matches))
		}

		foundFiles := make(map[string]bool)
		for _, match := range matches {
			foundFiles[match.FileName] = true
		}

		for _, expectedFile := range expectedFiles {
			if !foundFiles[expectedFile] {
				t.Errorf("Expected match in %s, but not found", expectedFile)
			}
		}
	})

	// test with context lines
	t.Run("ContextLines", func(t *testing.T) {
		epubPath := filepath.Join(tempDir, "context.epub")
		files := map[string]string{
			"content.txt": "Line before\nTarget line here\nLine after",
		}

		if err := createTestZIPWithFiles(epubPath, files); err != nil {
			t.Fatalf("Failed to create test ePUB: %v", err)
		}

		pattern, _ := regexp.Compile("Target")
		matches, err := grepInEpub(context.Background(), epubPath, pattern, 1)
		if err != nil {
			t.Fatalf("grepInEpub failed: %v", err)
		}

		if len(matches) != 1 {
			t.Fatalf("Expected 1 match, got %d", len(matches))
		}

		// should include context lines
		expectedLines := []string{"Line before", "Target line here", "Line after"}
		for _, line := range expectedLines {
			if !strings.Contains(matches[0].Line, line) {
				t.Errorf("Expected context to include '%s', got: %s", line, matches[0].Line)
			}
		}
	})

	// test file skipping logic
	t.Run("FileSkipping", func(t *testing.T) {
		epubPath := filepath.Join(tempDir, "skip.epub")
		files := map[string]string{
			"content.html":           "<p>This should be found: target</p>",
			"mimetype":               "application/epub+zip",
			"META-INF/container.xml": "<?xml version='1.0'?><container>target</container>",
			"cover.xhtml":            "<p>This should be skipped: target</p>",
			"toc.xhtml":              "<p>This should be skipped: target</p>",
			"ads.xhtml":              "<p>This should be skipped: target</p>",
			"sample_chapter.html":    "<p>This should be skipped: target</p>",
		}

		if err := createTestZIPWithFiles(epubPath, files); err != nil {
			t.Fatalf("Failed to create test ePUB: %v", err)
		}

		pattern, _ := regexp.Compile("target")
		matches, err := grepInEpub(context.Background(), epubPath, pattern, 0)
		if err != nil {
			t.Fatalf("grepInEpub failed: %v", err)
		}

		// should only find match in content.html
		if len(matches) != 1 {
			t.Errorf("Expected 1 match (only in content.html), got %d", len(matches))
		}

		if len(matches) > 0 && matches[0].FileName != "content.html" {
			t.Errorf("Expected match in content.html, got %s", matches[0].FileName)
		}
	})

	// test context cancellation
	t.Run("ContextCancellation", func(t *testing.T) {
		epubPath := filepath.Join(tempDir, "cancel.epub")
		files := map[string]string{
			"content.txt": "target content here",
		}

		if err := createTestZIPWithFiles(epubPath, files); err != nil {
			t.Fatalf("Failed to create test ePUB: %v", err)
		}

		pattern, _ := regexp.Compile("target")

		// create a context that's already cancelled
		ctx, cancel := context.WithCancel(context.Background())
		cancel()

		_, err := grepInEpub(ctx, epubPath, pattern, 0)

		if err != context.Canceled {
			t.Errorf("Expected context.Canceled error, got: %v", err)
		}
	})

	// test with directories in ZIP
	t.Run("DirectoriesInZip", func(t *testing.T) {
		epubPath := filepath.Join(tempDir, "dirs.epub")

		files := map[string]string{
			"chapters/content.txt": "target content",
		}

		if err := createTestZIPWithFiles(epubPath, files); err != nil {
			t.Fatalf("Failed to create test ZIP: %v", err)
		}

		pattern, _ := regexp.Compile("target")
		matches, err := grepInEpub(context.Background(), epubPath, pattern, 0)
		if err != nil {
			t.Fatalf("grepInEpub failed: %v", err)
		}

		// should find match in the file
		if len(matches) != 1 {
			t.Errorf("Expected 1 match, got %d", len(matches))
		}
	})
}

// TestGrepInEpubErrors tests error handling in grepInEpub
func TestGrepInEpubErrors(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "grep_error_test_*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tempDir)

	// test with non-existent file
	t.Run("NonExistentFile", func(t *testing.T) {
		pattern, _ := regexp.Compile("test")
		_, err := grepInEpub(context.Background(), "/non/existent/file.epub", pattern, 0)

		if err == nil {
			t.Error("Expected error for non-existent file")
		}

		if !strings.Contains(err.Error(), "failed to open epub") {
			t.Errorf("Expected 'failed to open epub' error, got: %v", err)
		}
	})

	// test with invalid ZIP file
	t.Run("InvalidZipFile", func(t *testing.T) {
		invalidZipPath := filepath.Join(tempDir, "invalid.epub")

		// create a file that's not a valid ZIP
		file, err := os.Create(invalidZipPath)
		if err != nil {
			t.Fatalf("Failed to create invalid ZIP: %v", err)
		}
		file.WriteString("This is not a valid ZIP file")
		file.Close()

		pattern, _ := regexp.Compile("test")
		_, err = grepInEpub(context.Background(), invalidZipPath, pattern, 0)
		if err == nil {
			t.Error("Expected error for invalid ZIP file")
		}
	})

	// test with corrupted ZIP entry (simulated by timeout during processing)
	t.Run("ProcessingTimeout", func(t *testing.T) {
		epubPath := filepath.Join(tempDir, "timeout.epub")
		files := map[string]string{
			"content.txt": "target content here",
		}

		if err := createTestZIPWithFiles(epubPath, files); err != nil {
			t.Fatalf("Failed to create test ePUB: %v", err)
		}

		pattern, _ := regexp.Compile("target")

		// create context with very short timeout
		ctx, cancel := context.WithTimeout(context.Background(), 1*time.Microsecond)
		defer cancel()

		// wait to ensure timeout
		time.Sleep(10 * time.Microsecond)

		// should get context timeout or cancellation
		_, err := grepInEpub(ctx, epubPath, pattern, 0)
		if err == nil {
			t.Error("Expected timeout error")
		} else if err != context.DeadlineExceeded && err != context.Canceled {
			t.Errorf("Expected timeout or cancellation error, got: %v", err)
		}
	})
}

// TestGrepInEpubEdgeCases tests edge cases and boundary conditions
func TestGrepInEpubEdgeCases(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "grep_edge_test_*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tempDir)

	// test with empty file
	t.Run("EmptyEpub", func(t *testing.T) {
		epubPath := filepath.Join(tempDir, "empty.epub")
		files := map[string]string{}

		if err := createTestZIPWithFiles(epubPath, files); err != nil {
			t.Fatalf("Failed to create empty ePUB: %v", err)
		}

		pattern, _ := regexp.Compile("target")
		matches, err := grepInEpub(context.Background(), epubPath, pattern, 0)
		if err != nil {
			t.Fatalf("grepInEpub failed: %v", err)
		} else if len(matches) != 0 {
			t.Errorf("Expected 0 matches in empty ePUB, got %d", len(matches))
		}
	})

	// Test with empty files
	t.Run("EmptyFiles", func(t *testing.T) {
		epubPath := filepath.Join(tempDir, "empty_files.epub")
		files := map[string]string{
			"empty.txt":  "",
			"empty.html": "",
		}

		if err := createTestZIPWithFiles(epubPath, files); err != nil {
			t.Fatalf("Failed to create ePUB with empty files: %v", err)
		}

		pattern, _ := regexp.Compile("target")
		matches, err := grepInEpub(context.Background(), epubPath, pattern, 0)
		if err != nil {
			t.Fatalf("grepInEpub failed: %v", err)
		} else if len(matches) != 0 {
			t.Errorf("Expected 0 matches in empty files, got %d", len(matches))
		}
	})

	// test with large context value
	t.Run("LargeContext", func(t *testing.T) {
		epubPath := filepath.Join(tempDir, "large_context.epub")

		// create content with many lines
		lines := make([]string, 100)
		for i := range lines {
			if i == 50 {
				lines[i] = "This line contains the target"
			} else {
				lines[i] = "Regular line " + string(rune('A'+i%26))
			}
		}
		content := strings.Join(lines, "\n")

		files := map[string]string{
			"content.txt": content,
		}

		if err := createTestZIPWithFiles(epubPath, files); err != nil {
			t.Fatalf("Failed to create ePUB: %v", err)
		}

		pattern, _ := regexp.Compile("target")
		matches, err := grepInEpub(context.Background(), epubPath, pattern, 20)
		if err != nil {
			t.Fatalf("grepInEpub failed: %v", err)
		} else if len(matches) != 1 {
			t.Fatalf("Expected 1 match, got %d", len(matches))
		}

		// should include 20 lines before and after for the configured context
		matchLines := strings.Split(matches[0].Line, "\n")
		if len(matchLines) < 30 {
			t.Errorf("Expected large context, got %d lines", len(matchLines))
		}
	})
}
