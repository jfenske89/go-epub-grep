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

// scannerPool reuses pooledScanner instances to reduce GC pressure during text file scanning. This pool significantly
// improves performance when processing large numbers of text files by avoiding repeated scanner and buffer allocations.
var scannerPool = sync.Pool{
	New: func() any {
		return newPooledScanner(strings.NewReader(""))
	},
}
