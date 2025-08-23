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

// createTestEPUBWithMetadata creates an ePUB with specific metadata for testing
func createTestEPUBWithMetadata(dir, filename string, metadata TestEPUBMetadata) (string, error) {
	epubPath := filepath.Join(dir, filename)

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

	// create OPF content with specified metadata
	opfFile, err := writer.Create("OEBPS/content.opf")
	if err != nil {
		return "", err
	}

	opfContent := fmt.Sprintf(`<?xml version="1.0" encoding="UTF-8"?>
<package xmlns="http://www.idpf.org/2007/opf" version="2.0" unique-identifier="uuid_id">
  <metadata xmlns:dc="http://purl.org/dc/elements/1.1/" xmlns:opf="http://www.idpf.org/2007/opf">
    <dc:title>%s</dc:title>
    %s
    %s
    <dc:language>en</dc:language>
    %s
    %s
    %s
  </metadata>
  <manifest>
    <item href="chapter1.html" id="chapter1" media-type="application/xhtml+xml"/>
  </manifest>
  <spine>
    <itemref idref="chapter1"/>
  </spine>
</package>`,
		metadata.Title,
		createAuthorsXML(metadata.Authors),
		createGenresXML(metadata.Genres),
		createDateXML(metadata.Date),
		createIdentifiersXML(metadata.Identifiers),
		createMetaTagsXML(metadata.MetaTags))

	opfFile.Write([]byte(opfContent))

	// add basic chapter content
	chapterFile, err := writer.Create("OEBPS/chapter1.html")
	if err != nil {
		return "", err
	}
	fmt.Fprintf(chapterFile, `<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE html>
<html xmlns="http://www.w3.org/1999/xhtml">
<head><title>Chapter 1</title></head>
<body><h1>Chapter 1</h1><p>Test content</p></body>
</html>`)

	return epubPath, nil
}

// TestEPUBMetadata represents metadata for creating test ePUBs
type TestEPUBMetadata struct {
	Title       string
	Authors     []string
	Genres      []string
	Date        string
	Identifiers map[string]string // scheme -> value
	MetaTags    map[string]string // name -> content
}

func createAuthorsXML(authors []string) string {
	if len(authors) == 0 {
		return ""
	}

	var result strings.Builder
	for _, author := range authors {
		fmt.Fprintf(&result, "<dc:creator>%s</dc:creator>\n    ", author)
	}
	return strings.TrimSpace(result.String())
}

func createGenresXML(genres []string) string {
	if len(genres) == 0 {
		return ""
	}

	var result strings.Builder
	for _, genre := range genres {
		fmt.Fprintf(&result, "<dc:subject>%s</dc:subject>\n    ", genre)
	}
	return strings.TrimSpace(result.String())
}

func createDateXML(date string) string {
	if date == "" {
		return ""
	}
	return fmt.Sprintf("<dc:date>%s</dc:date>", date)
}

func createIdentifiersXML(identifiers map[string]string) string {
	if len(identifiers) == 0 {
		return ""
	}

	var result strings.Builder
	for scheme, value := range identifiers {
		fmt.Fprintf(&result, `<dc:identifier id="id_%s" opf:scheme="%s">%s</dc:identifier>`, scheme, scheme, value)
		result.WriteString("\n    ")
	}
	return strings.TrimSpace(result.String())
}

func createMetaTagsXML(metaTags map[string]string) string {
	if len(metaTags) == 0 {
		return ""
	}

	var result strings.Builder
	for name, content := range metaTags {
		fmt.Fprintf(&result, `<meta name="%s" content="%s"/>`, name, content)
		result.WriteString("\n    ")
	}
	return strings.TrimSpace(result.String())
}

// TestNewMetadataExtractor tests the constructor function
func TestNewMetadataExtractor(t *testing.T) {
	// test with specific thread count
	t.Run("SpecificThreadCount", func(t *testing.T) {
		extractor := NewMetadataExtractor(4)
		impl := extractor.(*metadataExtractorImpl)
		if impl.maxThreads != 4 {
			t.Errorf("Expected maxThreads to be 4, got %d", impl.maxThreads)
		}
	})

	// test with zero thread count (should default to CPU count)
	t.Run("DefaultThreadCount", func(t *testing.T) {
		extractor := NewMetadataExtractor(0)
		impl := extractor.(*metadataExtractorImpl)
		if impl.maxThreads <= 0 {
			t.Errorf("Expected maxThreads to be positive, got %d", impl.maxThreads)
		}
	})

	// test with negative thread count (should default to CPU count)
	t.Run("NegativeThreadCount", func(t *testing.T) {
		extractor := NewMetadataExtractor(-1)
		impl := extractor.(*metadataExtractorImpl)
		if impl.maxThreads <= 0 {
			t.Errorf("Expected maxThreads to be positive, got %d", impl.maxThreads)
		}
	})
}

// TestProcessFile tests single file metadata extraction
func TestProcessFile(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "metadata_test_*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tempDir)

	extractor := NewMetadataExtractor(2)
	ctx := context.Background()

	// test basic metadata extraction
	t.Run("BasicMetadata", func(t *testing.T) {
		testMetadata := TestEPUBMetadata{
			Title:   "Test Book Title",
			Authors: []string{"Author One", "Author Two"},
			Genres:  []string{"Fiction", "Science Fiction"},
			Date:    "2023-05-15",
			Identifiers: map[string]string{
				"isbn": "978-1234567890",
				"asin": "B07ABCDEFG",
			},
		}

		epubPath, err := createTestEPUBWithMetadata(tempDir, "basic.epub", testMetadata)
		if err != nil {
			t.Fatalf("Failed to create test ePUB: %v", err)
		}

		metadata, err := extractor.ProcessFile(ctx, epubPath)
		if err != nil {
			t.Fatalf("ProcessFile failed: %v", err)
		}

		// verify extracted metadata
		if metadata.Title != testMetadata.Title {
			t.Errorf("Expected title '%s', got '%s'", testMetadata.Title, metadata.Title)
		}

		if len(metadata.Authors) != len(testMetadata.Authors) {
			t.Errorf("Expected %d authors, got %d", len(testMetadata.Authors), len(metadata.Authors))
		}

		for i, expectedAuthor := range testMetadata.Authors {
			if i < len(metadata.Authors) && metadata.Authors[i] != expectedAuthor {
				t.Errorf("Expected author '%s', got '%s'", expectedAuthor, metadata.Authors[i])
			}
		}

		if len(metadata.Genres) != len(testMetadata.Genres) {
			t.Errorf("Expected %d genres, got %d", len(testMetadata.Genres), len(metadata.Genres))
		} else if metadata.YearReleased != 2023 {
			t.Errorf("Expected year 2023, got %d", metadata.YearReleased)
		} else if metadata.Identifiers["isbn"] != testMetadata.Identifiers["isbn"] {
			t.Errorf("Expected ISBN '%s', got '%s'", testMetadata.Identifiers["isbn"], metadata.Identifiers["isbn"])
		}
	})

	// Test series metadata extraction
	t.Run("SeriesMetadata", func(t *testing.T) {
		testMetadata := TestEPUBMetadata{
			Title:   "Series Book",
			Authors: []string{"Series Author"},
			MetaTags: map[string]string{
				"calibre:series":       "Test Series",
				"calibre:series_index": "2.5",
			},
		}

		epubPath, err := createTestEPUBWithMetadata(tempDir, "series.epub", testMetadata)
		if err != nil {
			t.Fatalf("Failed to create test ePUB: %v", err)
		}

		metadata, err := extractor.ProcessFile(ctx, epubPath)
		if err != nil {
			t.Fatalf("ProcessFile failed: %v", err)
		} else if metadata.Series != "Test Series" {
			t.Errorf("Expected series 'Test Series', got '%s'", metadata.Series)
		} else if metadata.SeriesPosition != 2.5 {
			t.Errorf("Expected series position 2.5, got %f", metadata.SeriesPosition)
		}
	})

	// Test date parsing variations
	t.Run("DateParsing", func(t *testing.T) {
		testCases := []struct {
			name         string
			date         string
			expectedYear int
		}{
			{"RFC3339 Date", "2023-05-15T10:30:00Z", 2023},
			{"Simple Date", "2023-05-15", 2023},
			{"Year Only", "2023", 2023},

			// should only extract first 4 characters
			{"Long Year", "20231", 2023},
		}

		for _, tc := range testCases {
			t.Run(tc.name, func(t *testing.T) {
				testMetadata := TestEPUBMetadata{
					Title:   "Date Test",
					Authors: []string{"Test Author"},
					Date:    tc.date,
				}

				epubPath, err := createTestEPUBWithMetadata(tempDir, fmt.Sprintf("date_%s.epub", strings.ReplaceAll(tc.name, " ", "_")), testMetadata)
				if err != nil {
					t.Fatalf("Failed to create test ePUB: %v", err)
				}

				metadata, err := extractor.ProcessFile(ctx, epubPath)
				if err != nil {
					t.Fatalf("ProcessFile failed: %v", err)
				}

				if metadata.YearReleased != tc.expectedYear {
					t.Errorf("Expected year %d, got %d", tc.expectedYear, metadata.YearReleased)
				}
			})
		}
	})
}

// TestProcessFileErrors tests error handling in ProcessFile
func TestProcessFileErrors(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "metadata_error_test_*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tempDir)

	extractor := NewMetadataExtractor(1)
	ctx := context.Background()

	// test with non-existent file
	t.Run("NonExistentFile", func(t *testing.T) {
		_, err := extractor.ProcessFile(ctx, "/non/existent/file.epub")
		if err == nil {
			t.Error("Expected error for non-existent file")
		} else if !strings.Contains(err.Error(), "failed to open epub") {
			t.Errorf("Expected 'failed to open epub' error, got: %v", err)
		}
	})

	// test with invalid ZIP file
	t.Run("InvalidZipFile", func(t *testing.T) {
		invalidPath := filepath.Join(tempDir, "invalid.epub")
		file, err := os.Create(invalidPath)
		if err != nil {
			t.Fatalf("Failed to create invalid file: %v", err)
		}
		file.WriteString("This is not a valid ZIP file")
		file.Close()

		if _, err = extractor.ProcessFile(ctx, invalidPath); err == nil {
			t.Error("Expected error for invalid ZIP file")
		}
	})

	// Test with missing container.xml (should fallback to finding .opf)
	t.Run("MissingContainerXML", func(t *testing.T) {
		epubPath := filepath.Join(tempDir, "no_container.epub")

		zipFile, err := os.Create(epubPath)
		if err != nil {
			t.Fatalf("Failed to create ZIP: %v", err)
		}
		defer zipFile.Close()

		writer := zip.NewWriter(zipFile)
		defer writer.Close()

		// add mimetype
		mimetypeFile, err := writer.Create("mimetype")
		if err != nil {
			t.Fatalf("Failed to create mimetype: %v", err)
		}
		mimetypeFile.Write([]byte("application/epub+zip"))

		// add OPF directly without container.xml
		opfFile, err := writer.Create("content.opf")
		if err != nil {
			t.Fatalf("Failed to create OPF: %v", err)
		}
		opfFile.Write([]byte(`<?xml version="1.0"?>
<package xmlns="http://www.idpf.org/2007/opf" version="2.0">
  <metadata xmlns:dc="http://purl.org/dc/elements/1.1/">
    <dc:title>Fallback Test</dc:title>
    <dc:creator>Test Author</dc:creator>
  </metadata>
  <manifest>
    <item href="chapter1.html" id="chapter1" media-type="application/xhtml+xml"/>
  </manifest>
  <spine>
    <itemref idref="chapter1"/>
  </spine>
</package>`))

		writer.Close()
		zipFile.Close()

		metadata, err := extractor.ProcessFile(ctx, epubPath)
		if err != nil {
			t.Fatalf("ProcessFile should succeed with fallback: %v", err)
		} else if metadata.Title != "Fallback Test" {
			t.Errorf("Expected title 'Fallback Test', got '%s'", metadata.Title)
		}
	})

	// test with no OPF file found
	t.Run("NoOPFFile", func(t *testing.T) {
		epubPath := filepath.Join(tempDir, "no_opf.epub")

		zipFile, err := os.Create(epubPath)
		if err != nil {
			t.Fatalf("Failed to create ZIP: %v", err)
		}
		defer zipFile.Close()

		writer := zip.NewWriter(zipFile)

		// add mimetype only, no OPF
		mimetypeFile, err := writer.Create("mimetype")
		if err != nil {
			t.Fatalf("Failed to create mimetype: %v", err)
		}
		mimetypeFile.Write([]byte("application/epub+zip"))

		writer.Close()
		zipFile.Close()

		_, err = extractor.ProcessFile(ctx, epubPath)
		if err == nil {
			t.Error("Expected error for missing OPF file")
		} else if !strings.Contains(err.Error(), "META-INF/container.xml not found and no .opf file") {
			t.Errorf("Expected container/OPF error, got: %v", err)
		}
	})
}

// TestProcessDirectory tests batch directory processing
func TestProcessDirectory(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "metadata_dir_test_*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tempDir)

	// create multiple test files
	testBooks := []TestEPUBMetadata{
		{
			Title:   "Book One",
			Authors: []string{"Author A"},
			Genres:  []string{"Fiction"},
		},
		{
			Title:   "Book Two",
			Authors: []string{"Author B"},
			Genres:  []string{"Non-Fiction"},
		},
		{
			Title:   "Book Three",
			Authors: []string{"Author C"},
			Genres:  []string{"Mystery"},
		},
	}

	// Create files in main directory
	for i, book := range testBooks {
		_, err := createTestEPUBWithMetadata(tempDir, fmt.Sprintf("book%d.epub", i+1), book)
		if err != nil {
			t.Fatalf("Failed to create test ePUB %d: %v", i+1, err)
		}
	}

	// create subdirectory with another file
	subDir := filepath.Join(tempDir, "subdir")
	os.MkdirAll(subDir, 0o755)
	_, err = createTestEPUBWithMetadata(subDir, "subbook.epub", TestEPUBMetadata{
		Title:   "Sub Book",
		Authors: []string{"Sub Author"},
	})
	if err != nil {
		t.Fatalf("Failed to create subdirectory ePUB: %v", err)
	}

	// add a non-epub file (should be ignored)
	nonEpubPath := filepath.Join(tempDir, "not_an_epub.txt")
	os.WriteFile(nonEpubPath, []byte("This is not an ePUB"), 0o644)

	extractor := NewMetadataExtractor(2)
	ctx := context.Background()

	// test successful directory processing
	t.Run("SuccessfulProcessing", func(t *testing.T) {
		var results []struct {
			path     string
			metadata *Metadata
		}
		var mu sync.Mutex

		err := extractor.ProcessDirectory(ctx, tempDir, func(epubPath string, metadata *Metadata) error {
			mu.Lock()
			results = append(results, struct {
				path     string
				metadata *Metadata
			}{epubPath, metadata})
			mu.Unlock()
			return nil
		})
		if err != nil {
			t.Fatalf("ProcessDirectory failed: %v", err)
		}

		// should find 4 epub files (3 in main dir + 1 in subdir)
		if len(results) != 4 {
			t.Errorf("Expected 4 results, got %d", len(results))
		}

		// verify we got the expected titles
		titles := make(map[string]bool)
		for _, result := range results {
			titles[result.metadata.Title] = true
		}

		expectedTitles := []string{"Book One", "Book Two", "Book Three", "Sub Book"}
		for _, title := range expectedTitles {
			if !titles[title] {
				t.Errorf("Expected to find title '%s'", title)
			}
		}
	})

	// test handler error propagation
	t.Run("HandlerError", func(t *testing.T) {
		expectedError := fmt.Errorf("handler error")
		count := 0
		var countMutex sync.Mutex

		err := extractor.ProcessDirectory(ctx, tempDir, func(epubPath string, metadata *Metadata) error {
			countMutex.Lock()
			count++
			currentCount := count
			countMutex.Unlock()

			if currentCount == 2 {
				// return error on second file
				return expectedError
			}
			return nil
		})

		if err == nil {
			t.Error("Expected handler error to be propagated")
		} else if !strings.Contains(err.Error(), expectedError.Error()) {
			t.Errorf("Expected error to contain '%s', got: %v", expectedError.Error(), err)
		}
	})

	// test context cancellation
	t.Run("ContextCancellation", func(t *testing.T) {
		ctx, cancel := context.WithCancel(context.Background())

		err := extractor.ProcessDirectory(ctx, tempDir, func(epubPath string, metadata *Metadata) error {
			// cancel after first file
			cancel()
			return nil
		})

		if err == nil {
			t.Error("Expected context cancellation error")
		} else if err != context.Canceled && !strings.Contains(err.Error(), "context canceled") {
			t.Errorf("Expected context cancellation error, got: %v", err)
		}
	})

	// test with non-existent directory
	t.Run("NonExistentDirectory", func(t *testing.T) {
		err := extractor.ProcessDirectory(ctx, "/non/existent/path", func(epubPath string, metadata *Metadata) error {
			return nil
		})

		if err == nil {
			t.Error("Expected error for non-existent directory")
		}
	})
}

// TestIdentifierNormalization tests the normalizeIdentifierKey function
func TestIdentifierNormalization(t *testing.T) {
	testCases := []struct {
		input    string
		expected string
	}{
		{"ISBN", "isbn"},
		{"isbn-10", "isbn"},
		{"ISBN-13", "isbn"},
		{"ASIN", "asin"},
		{"DOI", "doi"},
		{"GOODREADS", "goodreads"},
		{"AMAZON", "amazon"},
		{"", ""},
		{"unknown_scheme", "unknown_scheme"},
	}

	for _, tc := range testCases {
		t.Run(fmt.Sprintf("Normalize_%s", tc.input), func(t *testing.T) {
			result := normalizeIdentifierKey(tc.input)
			if result != tc.expected {
				t.Errorf("normalizeIdentifierKey(%q) = %q, expected %q", tc.input, result, tc.expected)
			}
		})
	}
}

// TestIdentifierDetection tests the detectIdentifierType function
func TestIdentifierDetection(t *testing.T) {
	testCases := []struct {
		value    string
		expected string
	}{
		{"978-1234567890", "isbn"},
		{"1234567890", "isbn"},
		{"B07ABCDEFG", "asin"},
		{"10.1000/123456", "doi"},
		{"http://dx.doi.org/10.1000/123456", "uri"},
		{"unknown123", ""},
		{"", ""},
	}

	for _, tc := range testCases {
		t.Run(fmt.Sprintf("Detect_%s", tc.value), func(t *testing.T) {
			result := detectIdentifierType(tc.value)
			if result != tc.expected {
				t.Errorf("detectIdentifierType(%q) = %q, expected %q", tc.value, result, tc.expected)
			}
		})
	}
}

// TestISBNValidation tests ISBN validation functions
func TestISBNValidation(t *testing.T) {
	testCases := []struct {
		isbn     string
		expected bool
	}{
		{"1234567890", true},   // Valid ISBN-10 format
		{"12345678901", false}, // Too long for ISBN-10
		{"123456789", false},   // Too short for ISBN-10
		{"123456789X", true},   // Valid ISBN-10 with X
		{"abcdefghij", false},  // Non-numeric
		{"", false},            // Empty string returns false for ISBN10
	}

	for _, tc := range testCases {
		t.Run(fmt.Sprintf("ISBN10_%s", tc.isbn), func(t *testing.T) {
			result := isISBN10(tc.isbn)
			if result != tc.expected {
				t.Errorf("isISBN10(%q) = %v, expected %v", tc.isbn, result, tc.expected)
			}
		})
	}
}

// TestNumericValidation tests the isNumeric function
func TestNumericValidation(t *testing.T) {
	testCases := []struct {
		input    string
		expected bool
	}{
		{"123456", true},
		{"123.456", false}, // Dots not allowed
		{"123abc", false},  // Letters not allowed
		{"", true},         // Empty string returns true in isNumeric
		{"X", false},       // Single letter
	}

	for _, tc := range testCases {
		t.Run(fmt.Sprintf("Numeric_%s", tc.input), func(t *testing.T) {
			result := isNumeric(tc.input)
			if result != tc.expected {
				t.Errorf("isNumeric(%q) = %v, expected %v", tc.input, result, tc.expected)
			}
		})
	}
}

// TestConcurrentProcessing tests that concurrent processing works correctly
func TestConcurrentProcessing(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "concurrent_test_*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tempDir)

	// create many files to test concurrency
	numBooks := 20
	for i := 0; i < numBooks; i++ {
		metadata := TestEPUBMetadata{
			Title:   fmt.Sprintf("Concurrent Book %d", i),
			Authors: []string{fmt.Sprintf("Author %d", i)},
		}
		_, err := createTestEPUBWithMetadata(tempDir, fmt.Sprintf("concurrent%d.epub", i), metadata)
		if err != nil {
			t.Fatalf("Failed to create test ePUB %d: %v", i, err)
		}
	}

	// Build an extractor instance that uses 4 threads
	extractor := NewMetadataExtractor(4)
	ctx := context.Background()

	var results []string
	var mu sync.Mutex

	start := time.Now()
	err = extractor.ProcessDirectory(ctx, tempDir, func(epubPath string, metadata *Metadata) error {
		mu.Lock()
		results = append(results, metadata.Title)
		mu.Unlock()
		return nil
	})
	duration := time.Since(start)

	if err != nil {
		t.Fatalf("Concurrent processing failed: %v", err)
	} else if len(results) != numBooks {
		t.Errorf("Expected %d results, got %d", numBooks, len(results))
	}

	t.Logf("Processed %d books in %v", numBooks, duration)
}
