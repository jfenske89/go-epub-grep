package epubproc

import (
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

// TestScannerPoolReuse verifies that the scanner pool correctly reuses instances.
func TestScannerPoolReuse(t *testing.T) {
	// get a scanner from the pool
	scanner1 := scannerPool.Get()
	if scanner1 == nil {
		t.Fatal("Pool returned nil scanner")
	}

	// return it to the pool
	scannerPool.Put(scanner1)

	// get another scanner - should be the same instance if pool is working
	scanner2 := scannerPool.Get()
	if scanner2 == nil {
		t.Fatal("Pool returned nil scanner on second get")
	}
	scannerPool.Put(scanner2)

	// verify pool doesn't panic with concurrent access
	done := make(chan bool)
	for range 10 {
		go func() {
			s := scannerPool.Get()
			scannerPool.Put(s)
			done <- true
		}()
	}

	for range 10 {
		<-done
	}
}
