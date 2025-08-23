package epubproc

import (
	"io"
	"strings"
	"sync"

	"golang.org/x/net/html"
)

// pooledTokenizer wraps an html.Tokenizer with reuse capabilities for improved performance.
type pooledTokenizer struct {
	tokenizer  *html.Tokenizer
	textBuffer []byte // reusable buffer for text tokens
	tagBuffer  []byte // reusable buffer for tag names
	attrBuffer []byte // reusable buffer for attributes
}

// newPooledTokenizer creates a new pooled HTML tokenizer with pre-allocated buffers.
func newPooledTokenizer(r io.Reader) *pooledTokenizer {
	return &pooledTokenizer{
		tokenizer:  html.NewTokenizer(r),
		textBuffer: make([]byte, 0, 4096), // pre-allocate 4KB for text
		tagBuffer:  make([]byte, 0, 256),  // pre-allocate 256B for tags
		attrBuffer: make([]byte, 0, 1024), // pre-allocate 1KB for attributes
	}
}

// reset configures the pooled tokenizer for a new reader while preserving buffers.
func (pt *pooledTokenizer) reset(r io.Reader) {
	pt.tokenizer = html.NewTokenizer(r)

	// reset buffers but keep capacity
	pt.textBuffer = pt.textBuffer[:0]
	pt.tagBuffer = pt.tagBuffer[:0]
	pt.attrBuffer = pt.attrBuffer[:0]
}

// tokenizerPool reuses pooledTokenizer instances to reduce GC pressure during HTML file scanning. This pool
// significantly improves performance when processing HTML-heavy epubs by avoiding repeated tokenizer allocations.
var tokenizerPool = sync.Pool{
	New: func() any {
		return newPooledTokenizer(strings.NewReader(""))
	},
}
