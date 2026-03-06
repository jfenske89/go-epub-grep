package epubproc

import (
	"testing"
)

func TestCreateContextMatches(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name         string
		matchedLines []int
		lines        []string
		fileName     string
		contextLines int
		wantCount    int
		wantLines    []string
	}{
		{
			name:         "single match with context",
			matchedLines: []int{2},
			lines:        []string{"line0", "line1", "MATCH", "line3", "line4"},
			fileName:     "test.txt",
			contextLines: 1,
			wantCount:    1,
			wantLines:    []string{"line1\nMATCH\nline3"},
		},
		{
			name:         "single match at start",
			matchedLines: []int{0},
			lines:        []string{"MATCH", "line1", "line2"},
			fileName:     "test.txt",
			contextLines: 2,
			wantCount:    1,
			wantLines:    []string{"MATCH\nline1\nline2"},
		},
		{
			name:         "single match at end",
			matchedLines: []int{2},
			lines:        []string{"line0", "line1", "MATCH"},
			fileName:     "test.txt",
			contextLines: 2,
			wantCount:    1,
			wantLines:    []string{"line0\nline1\nMATCH"},
		},
		{
			name:         "two matches far apart no overlap",
			matchedLines: []int{1, 8},
			lines:        []string{"line0", "MATCH1", "line2", "line3", "line4", "line5", "line6", "line7", "MATCH2", "line9"},
			fileName:     "test.txt",
			contextLines: 1,
			wantCount:    2,
			wantLines: []string{
				"line0\nMATCH1\nline2",
				"line7\nMATCH2\nline9",
			},
		},
		{
			name:         "two matches with overlapping context merged",
			matchedLines: []int{2, 4},
			lines:        []string{"line0", "line1", "MATCH1", "line3", "MATCH2", "line5", "line6"},
			fileName:     "test.txt",
			contextLines: 2,
			wantCount:    1,
			wantLines:    []string{"line0\nline1\nMATCH1\nline3\nMATCH2\nline5\nline6"},
		},
		{
			name:         "two matches with adjacent context merged",
			matchedLines: []int{1, 4},
			lines:        []string{"line0", "MATCH1", "line2", "line3", "MATCH2", "line5"},
			fileName:     "test.txt",
			contextLines: 1,
			wantCount:    1,
			wantLines:    []string{"line0\nMATCH1\nline2\nline3\nMATCH2\nline5"},
		},
		{
			name:         "three matches two merge one separate",
			matchedLines: []int{1, 3, 9},
			lines:        []string{"line0", "MATCH1", "line2", "MATCH2", "line4", "line5", "line6", "line7", "line8", "MATCH3", "line10"},
			fileName:     "test.txt",
			contextLines: 1,
			wantCount:    2,
			wantLines: []string{
				"line0\nMATCH1\nline2\nMATCH2\nline4",
				"line8\nMATCH3\nline10",
			},
		},
		{
			name:         "no matches",
			matchedLines: []int{},
			lines:        []string{"line0", "line1", "line2"},
			fileName:     "test.txt",
			contextLines: 1,
			wantCount:    0,
			wantLines:    nil,
		},
		{
			name:         "zero context lines",
			matchedLines: []int{1, 3},
			lines:        []string{"line0", "MATCH1", "line2", "MATCH2", "line4"},
			fileName:     "test.txt",
			contextLines: 0,
			wantCount:    2,
			wantLines:    []string{"MATCH1", "MATCH2"},
		},
		{
			name:         "context larger than file",
			matchedLines: []int{1},
			lines:        []string{"line0", "MATCH", "line2"},
			fileName:     "test.txt",
			contextLines: 100,
			wantCount:    1,
			wantLines:    []string{"line0\nMATCH\nline2"},
		},
		{
			name:         "fileName is propagated",
			matchedLines: []int{0},
			lines:        []string{"MATCH"},
			fileName:     "chapter1.xhtml",
			contextLines: 0,
			wantCount:    1,
			wantLines:    []string{"MATCH"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			matches := createContextMatches(tt.matchedLines, tt.lines, tt.fileName, tt.contextLines)

			if len(matches) != tt.wantCount {
				t.Fatalf("expected %d matches, got %d", tt.wantCount, len(matches))
			}

			for i, match := range matches {
				if match.FileName != tt.fileName {
					t.Errorf("match[%d]: expected fileName %q, got %q", i, tt.fileName, match.FileName)
				}

				if tt.wantLines != nil && match.Line != tt.wantLines[i] {
					t.Errorf("match[%d]: expected line %q, got %q", i, tt.wantLines[i], match.Line)
				}
			}
		})
	}
}
