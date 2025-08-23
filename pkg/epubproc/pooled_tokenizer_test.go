package epubproc

import (
	"strings"
	"testing"
)

// TestPooledTokenizerCreation verifies that pooled tokenizers are created correctly.
func TestPooledTokenizerCreation(t *testing.T) {
	reader := strings.NewReader("<p>test</p>")
	pt := newPooledTokenizer(reader)

	if pt == nil {
		t.Fatal("Expected pooled tokenizer, got nil")
	}

	if pt.tokenizer == nil {
		t.Fatal("Expected tokenizer to be initialized")
	}

	// verify buffers are allocated
	if pt.textBuffer == nil {
		t.Fatal("Expected text buffer to be allocated")
	}

	if pt.tagBuffer == nil {
		t.Fatal("Expected tag buffer to be allocated")
	}

	if pt.attrBuffer == nil {
		t.Fatal("Expected attr buffer to be allocated")
	}

	// verify buffer capacities
	if cap(pt.textBuffer) < 4096 {
		t.Errorf("Expected text buffer capacity >= 4096, got %d", cap(pt.textBuffer))
	}

	if cap(pt.tagBuffer) < 256 {
		t.Errorf("Expected tag buffer capacity >= 256, got %d", cap(pt.tagBuffer))
	}

	if cap(pt.attrBuffer) < 1024 {
		t.Errorf("Expected attr buffer capacity >= 1024, got %d", cap(pt.attrBuffer))
	}
}

// TestPooledTokenizerReset verifies that tokenizers can be reset with new readers.
func TestPooledTokenizerReset(t *testing.T) {
	// create initial tokenizer
	pt := newPooledTokenizer(strings.NewReader("<div>initial</div>"))

	// store initial capacities
	textCap := cap(pt.textBuffer)
	tagCap := cap(pt.tagBuffer)
	attrCap := cap(pt.attrBuffer)

	// add some data to buffers
	pt.textBuffer = append(pt.textBuffer, []byte("test")...)
	pt.tagBuffer = append(pt.tagBuffer, []byte("div")...)
	pt.attrBuffer = append(pt.attrBuffer, []byte("class")...)

	// reset with new reader
	pt.reset(strings.NewReader("<p>new</p>"))

	// buffers should be reset but capacity preserved
	if len(pt.textBuffer) != 0 {
		t.Errorf("Text buffer not reset: length = %d", len(pt.textBuffer))
	} else if cap(pt.textBuffer) != textCap {
		t.Errorf("Text buffer capacity changed: expected %d, got %d", textCap, cap(pt.textBuffer))
	}

	if len(pt.tagBuffer) != 0 {
		t.Errorf("Tag buffer not reset: length = %d", len(pt.tagBuffer))
	} else if cap(pt.tagBuffer) != tagCap {
		t.Errorf("Tag buffer capacity changed: expected %d, got %d", tagCap, cap(pt.tagBuffer))
	}

	if len(pt.attrBuffer) != 0 {
		t.Errorf("Attr buffer not reset: length = %d", len(pt.attrBuffer))
	} else if cap(pt.attrBuffer) != attrCap {
		t.Errorf("Attr buffer capacity changed: expected %d, got %d", attrCap, cap(pt.attrBuffer))
	}
}

// TestTokenizerPoolReuse verifies that the tokenizer pool correctly reuses instances.
func TestTokenizerPoolReuse(t *testing.T) {
	// get a tokenizer from the pool
	tokenizer1 := tokenizerPool.Get()
	if tokenizer1 == nil {
		t.Fatal("Pool returned nil tokenizer")
	}

	// return it to the pool
	tokenizerPool.Put(tokenizer1)

	// get another tokenizer
	tokenizer2 := tokenizerPool.Get()
	if tokenizer2 == nil {
		t.Fatal("Pool returned nil tokenizer on second get")
	}
	tokenizerPool.Put(tokenizer2)

	// verify pool doesn't panic with concurrent access
	done := make(chan bool)
	for range 10 {
		go func() {
			tok := tokenizerPool.Get()
			tokenizerPool.Put(tok)
			done <- true
		}()
	}

	for range 10 {
		<-done
	}
}
