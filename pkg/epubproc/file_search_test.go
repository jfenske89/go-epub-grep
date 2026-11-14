package epubproc

import (
	"runtime"
	"testing"
)

// TestNewFileSearchDefaultThreads verifies that a thread count of zero or less falls back to the CPU count,
// and that a positive count is kept.
func TestNewFileSearchDefaultThreads(t *testing.T) {
	tests := []struct {
		name       string
		maxThreads int
		want       int
	}{
		{"zero", 0, runtime.NumCPU()},
		{"negative", -1, runtime.NumCPU()},
		{"positive", 3, 3},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			fs := NewFileSearch("/test", test.maxThreads, false).(*fileSearchImpl)

			if fs.maxThreads != test.want {
				t.Errorf("Expected %d threads, got %d", test.want, fs.maxThreads)
			}
		})
	}
}
