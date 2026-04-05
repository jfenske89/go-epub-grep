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

// TestGrepInEpubChapterMetadata tests chapter name enrichment via toc.ncx parsing.
func TestGrepInEpubChapterMetadata(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "grep_chapter_test_*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tempDir)

	const validTocNcx = `<?xml version="1.0" encoding="UTF-8"?>
<ncx>
  <navMap>
    <navPoint id="nav1" playOrder="1">
      <navLabel><text>Chapter One</text></navLabel>
      <content src="chapter1.html"/>
    </navPoint>
    <navPoint id="nav2" playOrder="2">
      <navLabel><text>Chapter Two</text></navLabel>
      <content src="chapter2.html"/>
    </navPoint>
  </navMap>
</ncx>`

	// ChapterMetadataFromToc verifies that matches in files listed in toc.ncx have
	// Metadata.Chapter populated, and that files absent from toc.ncx have nil Metadata.
	t.Run("ChapterMetadataFromToc", func(t *testing.T) {
		epubPath := filepath.Join(tempDir, "with_toc.epub")
		files := map[string]string{
			"toc.ncx":       validTocNcx,
			"chapter1.html": "<p>Hello target world</p>",
			"chapter2.html": "<p>Another target here</p>",
			"unknown.html":  "<p>target without chapter</p>",
		}
		if err := createTestZIPWithFiles(epubPath, files); err != nil {
			t.Fatalf("Failed to create test ePUB: %v", err)
		}

		pattern, _ := regexp.Compile("target")
		matches, err := grepInEpub(context.Background(), epubPath, pattern, 0)
		if err != nil {
			t.Fatalf("grepInEpub failed: %v", err)
		}
		if len(matches) != 3 {
			t.Fatalf("Expected 3 matches, got %d", len(matches))
		}

		byFile := make(map[string]Match)
		for _, m := range matches {
			byFile[m.FileName] = m
		}

		chapterCases := []struct {
			file    string
			chapter string
		}{
			{"chapter1.html", "Chapter One"},
			{"chapter2.html", "Chapter Two"},
		}
		for _, c := range chapterCases {
			m, ok := byFile[c.file]
			if !ok {
				t.Errorf("Expected match in %s", c.file)
				continue
			}
			if m.Metadata == nil || m.Metadata.Chapter == nil {
				t.Errorf("%s: expected chapter metadata, got nil", c.file)
			} else if *m.Metadata.Chapter != c.chapter {
				t.Errorf("%s: expected chapter %q, got %q", c.file, c.chapter, *m.Metadata.Chapter)
			}
		}

		m, ok := byFile["unknown.html"]
		if !ok {
			t.Fatal("Expected match in unknown.html")
		}
		if m.Metadata != nil {
			t.Errorf("unknown.html: expected nil metadata, got %+v", m.Metadata)
		}
	})

	// NoChapterMetadataWithoutToc verifies that matches have nil Metadata when no toc.ncx
	// is present in the epub.
	t.Run("NoChapterMetadataWithoutToc", func(t *testing.T) {
		epubPath := filepath.Join(tempDir, "no_toc.epub")
		files := map[string]string{
			"chapter1.html": "<p>target content here</p>",
		}
		if err := createTestZIPWithFiles(epubPath, files); err != nil {
			t.Fatalf("Failed to create test ePUB: %v", err)
		}

		pattern, _ := regexp.Compile("target")
		matches, err := grepInEpub(context.Background(), epubPath, pattern, 0)
		if err != nil {
			t.Fatalf("grepInEpub failed: %v", err)
		}
		if len(matches) != 1 {
			t.Fatalf("Expected 1 match, got %d", len(matches))
		}
		if matches[0].Metadata != nil {
			t.Errorf("Expected nil metadata, got %+v", matches[0].Metadata)
		}
	})

	// TocNcxNotSearched verifies that the toc.ncx file itself is not included in match
	// results, even when it contains the search term.
	t.Run("TocNcxNotSearched", func(t *testing.T) {
		epubPath := filepath.Join(tempDir, "toc_not_searched.epub")
		tocWithTerm := `<?xml version="1.0" encoding="UTF-8"?>
<ncx>
  <navMap>
    <navPoint id="nav1" playOrder="1">
      <navLabel><text>target chapter</text></navLabel>
      <content src="chapter1.html"/>
    </navPoint>
  </navMap>
</ncx>`
		files := map[string]string{
			"toc.ncx":       tocWithTerm,
			"chapter1.html": "<p>content without match</p>",
		}
		if err := createTestZIPWithFiles(epubPath, files); err != nil {
			t.Fatalf("Failed to create test ePUB: %v", err)
		}

		pattern, _ := regexp.Compile("target")
		matches, err := grepInEpub(context.Background(), epubPath, pattern, 0)
		if err != nil {
			t.Fatalf("grepInEpub failed: %v", err)
		}
		if len(matches) != 0 {
			t.Errorf("Expected 0 matches (toc.ncx should not be searched), got %d", len(matches))
			for _, m := range matches {
				t.Logf("  unexpected match in: %s", m.FileName)
			}
		}
	})

	// AnchorInTocSrcResolved verifies that a toc.ncx nav point with an anchor fragment in
	// its src (e.g. "chapter1.html#section1") still resolves chapter metadata for a file
	// named "chapter1.html" (without the anchor).
	t.Run("AnchorInTocSrcResolved", func(t *testing.T) {
		epubPath := filepath.Join(tempDir, "anchor_toc.epub")
		anchorToc := `<?xml version="1.0" encoding="UTF-8"?>
<ncx>
  <navMap>
    <navPoint id="nav1" playOrder="1">
      <navLabel><text>Chapter One</text></navLabel>
      <content src="chapter1.html#section1"/>
    </navPoint>
  </navMap>
</ncx>`
		files := map[string]string{
			"toc.ncx":       anchorToc,
			"chapter1.html": "<p>Hello target world</p>",
		}
		if err := createTestZIPWithFiles(epubPath, files); err != nil {
			t.Fatalf("Failed to create test ePUB: %v", err)
		}

		pattern, _ := regexp.Compile("target")
		matches, err := grepInEpub(context.Background(), epubPath, pattern, 0)
		if err != nil {
			t.Fatalf("grepInEpub failed: %v", err)
		}
		if len(matches) != 1 {
			t.Fatalf("Expected 1 match, got %d", len(matches))
		}
		if matches[0].Metadata == nil || matches[0].Metadata.Chapter == nil {
			t.Fatal("Expected chapter metadata via anchor-stripped fallback key, got nil")
		}
		if *matches[0].Metadata.Chapter != "Chapter One" {
			t.Errorf("Expected chapter %q, got %q", "Chapter One", *matches[0].Metadata.Chapter)
		}
	})

	// MalformedTocNcxHandled verifies that invalid XML in toc.ncx does not cause an error,
	// and that matches from content files are still returned (with nil Metadata).
	t.Run("MalformedTocNcxHandled", func(t *testing.T) {
		epubPath := filepath.Join(tempDir, "malformed_toc.epub")
		files := map[string]string{
			"toc.ncx":       "this is not valid XML <<<",
			"chapter1.html": "<p>target content here</p>",
		}
		if err := createTestZIPWithFiles(epubPath, files); err != nil {
			t.Fatalf("Failed to create test ePUB: %v", err)
		}

		pattern, _ := regexp.Compile("target")
		matches, err := grepInEpub(context.Background(), epubPath, pattern, 0)
		if err != nil {
			t.Fatalf("grepInEpub should not return an error on malformed toc.ncx, got: %v", err)
		}
		if len(matches) != 1 {
			t.Fatalf("Expected 1 match from content file, got %d", len(matches))
		}
		if matches[0].Metadata != nil {
			t.Errorf("Expected nil metadata when toc.ncx is malformed, got %+v", matches[0].Metadata)
		}
	})

	// TocNcxWithEpubNamespace verifies that a toc.ncx using the standard epub NCX namespace
	// (xmlns="http://www.daisy.org/z3986/2005/ncx/") is parsed correctly. Go's xml.Unmarshal
	// matches namespaced elements against bare local-name struct tags, so real epub files work.
	t.Run("TocNcxWithEpubNamespace", func(t *testing.T) {
		epubPath := filepath.Join(tempDir, "namespace_toc.epub")
		namespacedToc := `<?xml version="1.0" encoding="UTF-8"?>
<ncx xmlns="http://www.daisy.org/z3986/2005/ncx/" version="2005-1">
  <navMap>
    <navPoint id="nav1" playOrder="1">
      <navLabel><text>Chapter One</text></navLabel>
      <content src="chapter1.html"/>
    </navPoint>
  </navMap>
</ncx>`
		files := map[string]string{
			"toc.ncx":       namespacedToc,
			"chapter1.html": "<p>target content</p>",
		}
		if err := createTestZIPWithFiles(epubPath, files); err != nil {
			t.Fatalf("Failed to create test ePUB: %v", err)
		}

		pattern, _ := regexp.Compile("target")
		matches, err := grepInEpub(context.Background(), epubPath, pattern, 0)
		if err != nil {
			t.Fatalf("grepInEpub failed: %v", err)
		}
		if len(matches) != 1 {
			t.Fatalf("Expected 1 match, got %d", len(matches))
		}
		if matches[0].Metadata == nil || matches[0].Metadata.Chapter == nil {
			t.Fatal("Expected chapter metadata from namespaced toc.ncx, got nil")
		}
		if *matches[0].Metadata.Chapter != "Chapter One" {
			t.Errorf("Expected chapter %q, got %q", "Chapter One", *matches[0].Metadata.Chapter)
		}
	})
}

// TestGrepInEpubContentOpfFallback tests the secondary chapter fallback via content.opf.
func TestGrepInEpubContentOpfFallback(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "grep_opf_test_*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tempDir)

	// ContentOpfProvidesChapterFallback verifies that when no toc.ncx is present,
	// manifest item IDs from content.opf are used as chapter names.
	t.Run("ContentOpfProvidesChapterFallback", func(t *testing.T) {
		epubPath := filepath.Join(tempDir, "opf_fallback.epub")
		contentOpf := `<?xml version="1.0" encoding="UTF-8"?>
<package>
  <manifest>
    <item id="chapter-one" href="chapter1.html" media-type="application/xhtml+xml"/>
    <item id="chapter-two" href="chapter2.html" media-type="application/xhtml+xml"/>
  </manifest>
</package>`
		files := map[string]string{
			"content.opf":   contentOpf,
			"chapter1.html": "<p>Hello target world</p>",
			"chapter2.html": "<p>Another target here</p>",
			"unknown.html":  "<p>target without manifest entry</p>",
		}
		if err := createTestZIPWithFiles(epubPath, files); err != nil {
			t.Fatalf("Failed to create test ePUB: %v", err)
		}

		pattern, _ := regexp.Compile("target")
		matches, err := grepInEpub(context.Background(), epubPath, pattern, 0)
		if err != nil {
			t.Fatalf("grepInEpub failed: %v", err)
		}
		if len(matches) != 3 {
			t.Fatalf("Expected 3 matches, got %d", len(matches))
		}

		byFile := make(map[string]Match)
		for _, m := range matches {
			byFile[m.FileName] = m
		}

		opfCases := []struct {
			file    string
			chapter string
		}{
			{"chapter1.html", "chapter-one"},
			{"chapter2.html", "chapter-two"},
		}
		for _, c := range opfCases {
			m, ok := byFile[c.file]
			if !ok {
				t.Errorf("Expected match in %s", c.file)
				continue
			}
			if m.Metadata == nil || m.Metadata.Chapter == nil {
				t.Errorf("%s: expected chapter metadata from content.opf, got nil", c.file)
			} else if *m.Metadata.Chapter != c.chapter {
				t.Errorf("%s: expected chapter %q, got %q", c.file, c.chapter, *m.Metadata.Chapter)
			}
		}

		m, ok := byFile["unknown.html"]
		if !ok {
			t.Fatal("Expected match in unknown.html")
		}
		if m.Metadata != nil {
			t.Errorf("unknown.html: expected nil metadata (not in manifest), got %+v", m.Metadata)
		}
	})

	// TocNcxTakesPriorityOverContentOpf verifies that when both toc.ncx and content.opf
	// have entries for the same file, the toc.ncx chapter name is used.
	t.Run("TocNcxTakesPriorityOverContentOpf", func(t *testing.T) {
		epubPath := filepath.Join(tempDir, "opf_priority.epub")
		tocNcx := `<?xml version="1.0" encoding="UTF-8"?>
<ncx>
  <navMap>
    <navPoint id="nav1" playOrder="1">
      <navLabel><text>Chapter One From Toc</text></navLabel>
      <content src="chapter1.html"/>
    </navPoint>
  </navMap>
</ncx>`
		contentOpf := `<?xml version="1.0" encoding="UTF-8"?>
<package>
  <manifest>
    <item id="chapter-one-from-opf" href="chapter1.html" media-type="application/xhtml+xml"/>
  </manifest>
</package>`
		files := map[string]string{
			"toc.ncx":       tocNcx,
			"content.opf":   contentOpf,
			"chapter1.html": "<p>target content</p>",
		}
		if err := createTestZIPWithFiles(epubPath, files); err != nil {
			t.Fatalf("Failed to create test ePUB: %v", err)
		}

		pattern, _ := regexp.Compile("target")
		matches, err := grepInEpub(context.Background(), epubPath, pattern, 0)
		if err != nil {
			t.Fatalf("grepInEpub failed: %v", err)
		}
		if len(matches) != 1 {
			t.Fatalf("Expected 1 match, got %d", len(matches))
		}
		if matches[0].Metadata == nil || matches[0].Metadata.Chapter == nil {
			t.Fatal("Expected chapter metadata, got nil")
		}
		if *matches[0].Metadata.Chapter != "Chapter One From Toc" {
			t.Errorf("Expected toc.ncx chapter name %q, got %q", "Chapter One From Toc", *matches[0].Metadata.Chapter)
		}
	})

	// ContentOpfNotSearched verifies that the content.opf file itself is not included in
	// match results, even when it contains the search term.
	t.Run("ContentOpfNotSearched", func(t *testing.T) {
		epubPath := filepath.Join(tempDir, "opf_not_searched.epub")
		contentOpf := `<?xml version="1.0" encoding="UTF-8"?>
<package>
  <manifest>
    <item id="target-chapter" href="chapter1.html" media-type="application/xhtml+xml"/>
  </manifest>
</package>`
		files := map[string]string{
			"content.opf":   contentOpf,
			"chapter1.html": "<p>content without match</p>",
		}
		if err := createTestZIPWithFiles(epubPath, files); err != nil {
			t.Fatalf("Failed to create test ePUB: %v", err)
		}

		pattern, _ := regexp.Compile("target")
		matches, err := grepInEpub(context.Background(), epubPath, pattern, 0)
		if err != nil {
			t.Fatalf("grepInEpub failed: %v", err)
		}
		if len(matches) != 0 {
			t.Errorf("Expected 0 matches (content.opf should not be searched), got %d", len(matches))
			for _, m := range matches {
				t.Logf("  unexpected match in: %s", m.FileName)
			}
		}
	})

	// ContentOpfDirectoryHrefResolved verifies that manifest hrefs with a directory
	// component (e.g. "Text/chapter1.html") are resolved to their basename for lookup,
	// matching the same basename extraction applied to match file names.
	t.Run("ContentOpfDirectoryHrefResolved", func(t *testing.T) {
		epubPath := filepath.Join(tempDir, "opf_dir_href.epub")
		contentOpf := `<?xml version="1.0" encoding="UTF-8"?>
<package>
  <manifest>
    <item id="chapter-one" href="Text/chapter1.html" media-type="application/xhtml+xml"/>
  </manifest>
</package>`
		files := map[string]string{
			"content.opf":        contentOpf,
			"Text/chapter1.html": "<p>target content</p>",
		}
		if err := createTestZIPWithFiles(epubPath, files); err != nil {
			t.Fatalf("Failed to create test ePUB: %v", err)
		}

		pattern, _ := regexp.Compile("target")
		matches, err := grepInEpub(context.Background(), epubPath, pattern, 0)
		if err != nil {
			t.Fatalf("grepInEpub failed: %v", err)
		}
		if len(matches) != 1 {
			t.Fatalf("Expected 1 match, got %d", len(matches))
		}
		if matches[0].Metadata == nil || matches[0].Metadata.Chapter == nil {
			t.Fatal("Expected chapter metadata from directory-prefixed manifest href, got nil")
		}
		if *matches[0].Metadata.Chapter != "chapter-one" {
			t.Errorf("Expected chapter %q, got %q", "chapter-one", *matches[0].Metadata.Chapter)
		}
	})

	// MalformedContentOpfHandled verifies that invalid XML in content.opf does not cause
	// an error, and that matches from content files are still returned (with nil Metadata).
	t.Run("MalformedContentOpfHandled", func(t *testing.T) {
		epubPath := filepath.Join(tempDir, "malformed_opf.epub")
		files := map[string]string{
			"content.opf":   "this is not valid XML <<<",
			"chapter1.html": "<p>target content here</p>",
		}
		if err := createTestZIPWithFiles(epubPath, files); err != nil {
			t.Fatalf("Failed to create test ePUB: %v", err)
		}

		pattern, _ := regexp.Compile("target")
		matches, err := grepInEpub(context.Background(), epubPath, pattern, 0)
		if err != nil {
			t.Fatalf("grepInEpub should not return an error on malformed content.opf, got: %v", err)
		}
		if len(matches) != 1 {
			t.Fatalf("Expected 1 match from content file, got %d", len(matches))
		}
		if matches[0].Metadata != nil {
			t.Errorf("Expected nil metadata when content.opf is malformed, got %+v", matches[0].Metadata)
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
