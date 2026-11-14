package epubproc

import (
	"regexp"
	"strings"
	"testing"
)

// TestPooledScannerCreation verifies that pooled scanners are created correctly.
func TestPooledScannerCreation(t *testing.T) {
	reader := strings.NewReader("test content")
	ps := newPooledScanner(reader)

	if ps == nil {
		t.Fatal("Expected pooled scanner, got nil")
	} else if ps.scanner == nil {
		t.Fatal("Expected scanner to be initialized")
	} else if ps.buffer == nil {
		t.Fatal("Expected buffer to be allocated")
	}

	// verify buffer has expected capacity
	if cap(ps.buffer) < 16384 {
		t.Errorf("Expected buffer capacity >= 16384, got %d", cap(ps.buffer))
	}
}

// TestPooledScannerReset verifies that scanners can be reset with new readers.
func TestPooledScannerReset(t *testing.T) {
	// create initial scanner
	ps := newPooledScanner(strings.NewReader("initial"))
	initialBufferCap := cap(ps.buffer)

	// reset with new reader
	ps.reset(strings.NewReader("new content"))

	// buffer capacity should be preserved
	if cap(ps.buffer) != initialBufferCap {
		t.Errorf("Buffer capacity changed after reset: expected %d, got %d",
			initialBufferCap, cap(ps.buffer))
	}

	// scanner should work with new content
	if !ps.scanner.Scan() {
		t.Fatal("Failed to scan after reset")
	}

	if ps.scanner.Text() != "new content" {
		t.Errorf("Expected 'new content', got '%s'", ps.scanner.Text())
	}
}

// TestPooledScannerResetKeepsNoOldState verifies that a reset scanner shows nothing from its previous reader,
// including an error left by that reader.
func TestPooledScannerResetKeepsNoOldState(t *testing.T) {
	// a line longer than the 256 KB token limit leaves the scanner in an error state
	ps := newPooledScanner(strings.NewReader(strings.Repeat("x", 300*1024)))
	if ps.scanner.Scan() {
		t.Fatal("Expected the oversized line to stop the scan")
	}
	if ps.scanner.Err() == nil {
		t.Fatal("Expected an error for the oversized line")
	}

	ps.reset(strings.NewReader("alpha\nbeta"))

	var lines []string
	for ps.scanner.Scan() {
		lines = append(lines, ps.scanner.Text())
	}

	if err := ps.scanner.Err(); err != nil {
		t.Errorf("Expected no error after reset, got: %v", err)
	}
	if strings.Join(lines, ",") != "alpha,beta" {
		t.Errorf("Expected lines [alpha beta] from the new reader only, got %v", lines)
	}
}

// TestScanTextFileAfterFailedScan verifies that a scan which fails does not damage the next scan, whichever
// pooled scanner the pool hands out. The pool does not promise reuse, so the sequence is repeated.
func TestScanTextFileAfterFailedScan(t *testing.T) {
	pattern := regexp.MustCompile("beta")

	for range 20 {
		_ = scanTextFile(strings.NewReader(strings.Repeat("x", 300*1024)), pattern, "huge.txt", 0)

		matches := scanTextFile(strings.NewReader("alpha\nbeta\ngamma"), pattern, "small.txt", 0)

		if len(matches) != 1 || matches[0].Line != "beta" || matches[0].FileName != "small.txt" {
			t.Fatalf("Expected one match for 'beta' in small.txt, got %+v", matches)
		}
	}
}

// TestPooledScannerRelease verifies that a released scanner no longer holds the reader it scanned.
// No parallel test uses scannerPool, so reading ps after release is safe.
func TestPooledScannerRelease(t *testing.T) {
	ps := newPooledScanner(strings.NewReader("content"))

	ps.release()

	if ps.scanner != nil {
		t.Error("Expected release to drop the scanner and its reader")
	}
}
