package epubproc

import (
	"bufio"
	"io"
	"strings"
	"sync"
)

// pooledScanner wraps a bufio.Scanner with buffer reuse capabilities for improved performance.
type pooledScanner struct {
	scanner *bufio.Scanner
	buffer  []byte
}

// newPooledScanner creates a new pooled scanner with a reusable buffer.
func newPooledScanner(r io.Reader) *pooledScanner {
	ps := &pooledScanner{
		scanner: bufio.NewScanner(r),
		buffer:  make([]byte, 0, 16*1024), // pre-allocate 16KB buffer (larger for better performance)
	}

	// use the pre-allocated buffer for token storage to reduce allocations
	// with an increased max token size of 256KB to handle larger lines in epub files
	ps.scanner.Buffer(ps.buffer, 256*1024)
	return ps
}

// reset configures the pooled scanner for a new reader while reusing the buffer.
func (ps *pooledScanner) reset(r io.Reader) {
	ps.scanner = bufio.NewScanner(r)

	// reuse the buffer - this avoids allocations for most text files
	ps.scanner.Buffer(ps.buffer[:0], 256*1024)
}

// release drops the reference to the last reader and returns the scanner to the pool. Without this, an idle
// pooled scanner would keep the last zip entry (and its buffers) alive until the next reset.
func (ps *pooledScanner) release() {
	ps.scanner = nil
	scannerPool.Put(ps)
}

// scannerPool reuses pooledScanner instances to reduce GC pressure during text file scanning. Each reset still
// allocates a fresh bufio.Scanner, but starts it on the pooledScanner's initial buffer, which avoids repeated
// buffer growth allocations for typical text files. A scan that needs a larger buffer allocates its own, and that
// larger buffer is not kept for reuse.
var scannerPool = sync.Pool{
	New: func() any {
		return newPooledScanner(strings.NewReader(""))
	},
}
