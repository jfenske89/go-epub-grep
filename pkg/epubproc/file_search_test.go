package epubproc

import (
	"testing"
)

// TestNewFileSearch verifies that FileSearch instances are created correctly.
func TestNewFileSearch(t *testing.T) {
	epubDir := "/test/path"

	// test with default thread count
	fs := NewFileSearch(epubDir, 0, false)
	if fs == nil {
		t.Fatal("Expected FileSearch instance, got nil")
	}

	// test with specific thread count
	fs2 := NewFileSearch(epubDir, 4, true)
	if fs2 == nil {
		t.Fatal("Expected FileSearch instance, got nil")
	}
}

// TestFileSearchImpl verifies that the fileSearchImpl struct is properly initialized.
func TestFileSearchImpl(t *testing.T) {
	epubDir := "/test/epub/dir"
	maxThreads := 8
	extractMetadata := true

	fs := NewFileSearch(epubDir, maxThreads, extractMetadata).(*fileSearchImpl)

	if fs.epubDir != epubDir {
		t.Errorf("Expected epubDir '%s', got '%s'", epubDir, fs.epubDir)
	}

	if fs.maxThreads != maxThreads {
		t.Errorf("Expected maxThreads %d, got %d", maxThreads, fs.maxThreads)
	}

	if fs.extractMetadata != extractMetadata {
		t.Errorf("Expected extractMetadata %t, got %t", extractMetadata, fs.extractMetadata)
	}
}

// TestFileSearchDefaultThreads verifies that default thread count is set correctly.
func TestFileSearchDefaultThreads(t *testing.T) {
	fs := NewFileSearch("/test", -1, false).(*fileSearchImpl)

	// should default to runtime.NumCPU()
	if fs.maxThreads <= 0 {
		t.Errorf("Expected positive thread count, got %d", fs.maxThreads)
	}
}
