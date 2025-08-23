package epubproc

import (
	"context"
	"regexp"
	"strings"
	"testing"
)

// TestScanTextFileEdgeCases tests boundary conditions and edge cases for text scanning
func TestScanTextFileEdgeCases(t *testing.T) {
	// test with empty content
	t.Run("EmptyContent", func(t *testing.T) {
		reader := strings.NewReader("")
		pattern, _ := regexp.Compile("test")

		matches := scanTextFile(reader, pattern, "empty.txt", 0)

		if len(matches) != 0 {
			t.Errorf("Expected 0 matches for empty content, got %d", len(matches))
		}
	})

	// test with single character
	t.Run("SingleCharacter", func(t *testing.T) {
		reader := strings.NewReader("a")
		pattern, _ := regexp.Compile("a")

		matches := scanTextFile(reader, pattern, "single.txt", 0)

		if len(matches) != 1 {
			t.Errorf("Expected 1 match for single character, got %d", len(matches))
		}
	})

	// test with very long lines
	t.Run("VeryLongLine", func(t *testing.T) {
		// create a line longer than typical buffer sizes
		longLine := strings.Repeat("a", 100000) + "target" + strings.Repeat("b", 100000)
		reader := strings.NewReader(longLine)
		pattern, _ := regexp.Compile("target")

		matches := scanTextFile(reader, pattern, "long.txt", 0)

		if len(matches) != 1 {
			t.Errorf("Expected 1 match for very long line, got %d", len(matches))
		}
	})

	// test with many small lines
	t.Run("ManySmallLines", func(t *testing.T) {
		lines := make([]string, 10_000)
		for i := range lines {
			if i%100 == 0 {
				lines[i] = "line with target"
			} else {
				lines[i] = "regular line"
			}
		}
		content := strings.Join(lines, "\n")
		reader := strings.NewReader(content)
		pattern, _ := regexp.Compile("target")

		matches := scanTextFile(reader, pattern, "many.txt", 0)

		// every 100th line has "target"
		expectedMatches := 100
		if len(matches) != expectedMatches {
			t.Errorf("Expected %d matches for many lines, got %d", expectedMatches, len(matches))
		}
	})

	// test with Unicode content
	t.Run("UnicodeContent", func(t *testing.T) {
		content := "Hello 世界\nThis has émojis 🎯\nTesting αβγ characters"
		reader := strings.NewReader(content)
		pattern, _ := regexp.Compile("🎯")

		matches := scanTextFile(reader, pattern, "unicode.txt", 0)

		if len(matches) != 1 {
			t.Errorf("Expected 1 match for Unicode content, got %d", len(matches))
		}

		if len(matches) > 0 && !strings.Contains(matches[0].Line, "🎯") {
			t.Errorf("Expected match to contain emoji, got: %s", matches[0].Line)
		}
	})

	// test with zero context on single line
	t.Run("ZeroContextSingleLine", func(t *testing.T) {
		reader := strings.NewReader("only line with target")
		pattern, _ := regexp.Compile("target")

		matches := scanTextFile(reader, pattern, "single.txt", 0)

		if len(matches) != 1 {
			t.Errorf("Expected 1 match, got %d", len(matches))
		}

		if len(matches) > 0 && matches[0].Line != "only line with target" {
			t.Errorf("Expected exact line match, got: %s", matches[0].Line)
		}
	})

	// test with context larger than available lines
	t.Run("ContextLargerThanContent", func(t *testing.T) {
		content := "line1\ntarget line\nline3"
		reader := strings.NewReader(content)
		pattern, _ := regexp.Compile("target")

		// context larger than content
		matches := scanTextFile(reader, pattern, "small.txt", 10)

		if len(matches) != 1 {
			t.Errorf("Expected 1 match, got %d", len(matches))
		}

		// should include all available lines
		expectedLines := []string{"line1", "target line", "line3"}
		for _, expectedLine := range expectedLines {
			if !strings.Contains(matches[0].Line, expectedLine) {
				t.Errorf("Expected match to include '%s', got: %s", expectedLine, matches[0].Line)
			}
		}
	})
}

// TestScanHTMLFileEdgeCases tests boundary conditions and edge cases for HTML scanning
func TestScanHTMLFileEdgeCases(t *testing.T) {
	// test with empty HTML
	t.Run("EmptyHTML", func(t *testing.T) {
		reader := strings.NewReader("")
		pattern, _ := regexp.Compile("test")

		matches := scanHTMLFile(context.Background(), reader, pattern, "empty.html", 0)

		if len(matches) != 0 {
			t.Errorf("Expected 0 matches for empty HTML, got %d", len(matches))
		}
	})

	// test with HTML containing only tags (no text)
	t.Run("TagsOnlyHTML", func(t *testing.T) {
		html := "<html><head><title></title></head><body><div></div><p></p></body></html>"
		reader := strings.NewReader(html)
		pattern, _ := regexp.Compile("test")

		matches := scanHTMLFile(context.Background(), reader, pattern, "tags.html", 0)

		if len(matches) != 0 {
			t.Errorf("Expected 0 matches for tags-only HTML, got %d", len(matches))
		}
	})

	// test with deeply nested HTML
	t.Run("DeeplyNestedHTML", func(t *testing.T) {
		// create deeply nested structure
		html := "<html><body>"
		for i := 0; i < 50; i++ {
			html += "<div>"
		}

		html += "deeply nested target"
		for i := 0; i < 50; i++ {
			html += "</div>"
		}

		html += "</body></html>"

		reader := strings.NewReader(html)
		pattern, _ := regexp.Compile("target")

		matches := scanHTMLFile(context.Background(), reader, pattern, "nested.html", 0)
		if len(matches) != 1 {
			t.Errorf("Expected 1 match for deeply nested HTML, got %d", len(matches))
		}
	})

	// test with malformed HTML (unclosed tags, wrong nesting)
	t.Run("MalformedHTML", func(t *testing.T) {
		malformed := "<html><body><p>Paragraph <div>Wrong nesting <span>target content</p></div>"
		reader := strings.NewReader(malformed)
		pattern, _ := regexp.Compile("target")

		matches := scanHTMLFile(context.Background(), reader, pattern, "malformed.html", 0)

		// should still find the content despite malformed structure
		if len(matches) != 1 {
			t.Errorf("Expected 1 match despite malformed HTML, got %d", len(matches))
		}
	})

	// test with HTML entities and special characters
	t.Run("HTMLEntities", func(t *testing.T) {
		html := "<p>This has &amp; entities and &lt;script&gt; tags with target content</p>"
		reader := strings.NewReader(html)
		pattern, _ := regexp.Compile("target")

		matches := scanHTMLFile(context.Background(), reader, pattern, "entities.html", 0)

		if len(matches) != 1 {
			t.Errorf("Expected 1 match with HTML entities, got %d", len(matches))
		}

		// should find the word "target" in the text content
		if len(matches) > 0 && !strings.Contains(matches[0].Line, "target") {
			t.Errorf("Expected match to contain 'target', got: %s", matches[0].Line)
		}
	})

	// test with inline vs block element mixing
	t.Run("InlineBlockMixing", func(t *testing.T) {
		html := `<p>Start of paragraph <span>inline target</span> middle <strong>bold</strong> end.</p>
				 <div>Block level <em>emphasized target</em> content.</div>`
		reader := strings.NewReader(html)
		pattern, _ := regexp.Compile("target")

		matches := scanHTMLFile(context.Background(), reader, pattern, "mixed.html", 0)

		// should find 2 matches, one in each block-level element
		if len(matches) != 2 {
			t.Errorf("Expected 2 matches for inline/block mixing, got %d", len(matches))
		}
	})

	// test with whitespace normalization
	t.Run("WhitespaceNormalization", func(t *testing.T) {
		html := `<p>   Multiple    spaces   and
					  line breaks   with   target   word   </p>`
		reader := strings.NewReader(html)
		pattern, _ := regexp.Compile("target")

		matches := scanHTMLFile(context.Background(), reader, pattern, "whitespace.html", 0)

		if len(matches) != 1 {
			t.Errorf("Expected 1 match with whitespace normalization, got %d", len(matches))
		}

		// should normalize whitespace to single spaces
		if len(matches) > 0 {
			// double space
			if strings.Contains(matches[0].Line, "  ") {
				t.Errorf("Expected normalized whitespace, got: %s", matches[0].Line)
			}
		}
	})
}

// TestRegexPatternEdgeCases tests edge cases with different regex patterns
func TestRegexPatternEdgeCases(t *testing.T) {
	// test with empty pattern (matches everything)
	t.Run("EmptyPattern", func(t *testing.T) {
		content := "line1\nline2\nline3"
		reader := strings.NewReader(content)
		pattern, _ := regexp.Compile("")

		matches := scanTextFile(reader, pattern, "test.txt", 0)

		// empty pattern matches every line
		if len(matches) != 3 {
			t.Errorf("Expected 3 matches for empty pattern, got %d", len(matches))
		}
	})

	// test with pattern that matches word boundaries
	t.Run("WordBoundaryPattern", func(t *testing.T) {
		content := "target targeting targets"
		reader := strings.NewReader(content)
		pattern, _ := regexp.Compile(`\btarget\b`)

		matches := scanTextFile(reader, pattern, "test.txt", 0)

		// should match only the exact word "target", not "targeting" or "targets"
		if len(matches) != 1 {
			t.Errorf("Expected 1 match for word boundary pattern, got %d", len(matches))
		}
	})

	// test with Unicode-aware regex
	t.Run("UnicodePattern", func(t *testing.T) {
		content := "café naïve résumé"
		reader := strings.NewReader(content)
		pattern, _ := regexp.Compile(`\p{L}+é`)

		matches := scanTextFile(reader, pattern, "test.txt", 0)

		// should match words ending with é
		if len(matches) != 1 {
			t.Errorf("Expected 1 match for Unicode pattern, got %d", len(matches))
		}
	})

	// test with very complex regex
	t.Run("ComplexPattern", func(t *testing.T) {
		content := "phone: +1-555-123-4567\nemail: test@example.com\ndate: 2023-12-25"
		reader := strings.NewReader(content)

		// regex to match phone numbers
		pattern, _ := regexp.Compile(`\+\d{1,3}-\d{3}-\d{3}-\d{4}`)

		matches := scanTextFile(reader, pattern, "test.txt", 0)

		if len(matches) != 1 {
			t.Errorf("Expected 1 match for complex pattern, got %d", len(matches))
		}

		if len(matches) > 0 && !strings.Contains(matches[0].Line, "+1-555-123-4567") {
			t.Errorf("Expected phone number match, got: %s", matches[0].Line)
		}
	})
}

// TestPerformanceEdgeCases tests performance with extreme inputs
func TestPerformanceEdgeCases(t *testing.T) {
	// test with extremely long single line (stress test buffer handling)
	t.Run("ExtremelyLongLine", func(t *testing.T) {
		// create a line that's large but within scanner limits
		longLine := strings.Repeat("x", 100000) + "target" + strings.Repeat("y", 100000)
		reader := strings.NewReader(longLine)
		pattern, _ := regexp.Compile("target")

		matches := scanTextFile(reader, pattern, "huge.txt", 0)

		// very long lines may exceed scanner token limits, verify it doesn't crash
		if len(matches) > 1 {
			t.Errorf("Expected 0 or 1 match for extremely long line, got %d", len(matches))
		}
	})

	// test with many matches in single line
	t.Run("ManyMatchesInLine", func(t *testing.T) {
		// create line with many occurrences of pattern
		parts := make([]string, 1000)
		for i := range parts {
			parts[i] = "target"
		}
		content := strings.Join(parts, " ")
		reader := strings.NewReader(content)
		pattern, _ := regexp.Compile("target")

		matches := scanTextFile(reader, pattern, "many.txt", 0)

		// should find the line (which contains many matches of the pattern)
		if len(matches) != 1 {
			t.Errorf("Expected 1 match (line with many targets), got %d", len(matches))
		}
	})
}

// TestSpecialCharacterHandling tests handling of special characters and encoding
func TestSpecialCharacterHandling(t *testing.T) {
	// test with various Unicode categories
	t.Run("UnicodeCategories", func(t *testing.T) {
		// mix of different Unicode categories
		content := "Latin: hello, Cyrillic: привет, Arabic: مرحبا, Chinese: 你好, Emoji: 👋"
		reader := strings.NewReader(content)
		pattern, _ := regexp.Compile("👋")

		matches := scanTextFile(reader, pattern, "unicode.txt", 0)
		if len(matches) != 1 {
			t.Errorf("Expected 1 match for Unicode emoji, got %d", len(matches))
		}
	})

	// test with control characters and non-printable characters
	t.Run("ControlCharacters", func(t *testing.T) {
		// include some control characters
		content := "before\ttab\nafter\x00null\rtarget content"
		reader := strings.NewReader(content)
		pattern, _ := regexp.Compile("target")

		matches := scanTextFile(reader, pattern, "control.txt", 0)
		if len(matches) != 1 {
			t.Errorf("Expected 1 match with control characters, got %d", len(matches))
		}
	})

	// test with mixed line endings
	t.Run("MixedLineEndings", func(t *testing.T) {
		// mix of \n, \r\n, and \r line endings
		content := "line1\nline2 with target\r\nline3\rline4"
		reader := strings.NewReader(content)
		pattern, _ := regexp.Compile("target")

		matches := scanTextFile(reader, pattern, "mixed.txt", 0)
		if len(matches) != 1 {
			t.Errorf("Expected 1 match with mixed line endings, got %d", len(matches))
		}
	})
}

// TestContextBoundaryConditions tests edge cases around context line handling
func TestContextBoundaryConditions(t *testing.T) {
	// test first line match with context
	t.Run("FirstLineWithContext", func(t *testing.T) {
		content := "first line with target\nsecond line\nthird line"
		reader := strings.NewReader(content)
		pattern, _ := regexp.Compile("target")

		matches := scanTextFile(reader, pattern, "first.txt", 2)

		if len(matches) != 1 {
			t.Errorf("Expected 1 match, got %d", len(matches))
		}

		// should include the first line and 2 lines after (no lines before available)
		expectedLines := []string{"first line with target", "second line", "third line"}
		for _, expectedLine := range expectedLines {
			if !strings.Contains(matches[0].Line, expectedLine) {
				t.Errorf("Expected context to include '%s', got: %s", expectedLine, matches[0].Line)
			}
		}
	})

	// test last line match with context
	t.Run("LastLineWithContext", func(t *testing.T) {
		content := "first line\nsecond line\nlast line with target"
		reader := strings.NewReader(content)
		pattern, _ := regexp.Compile("target")

		matches := scanTextFile(reader, pattern, "last.txt", 2)

		if len(matches) != 1 {
			t.Errorf("Expected 1 match, got %d", len(matches))
		}

		// should include 2 lines before and the last line (no lines after available)
		expectedLines := []string{"first line", "second line", "last line with target"}
		for _, expectedLine := range expectedLines {
			if !strings.Contains(matches[0].Line, expectedLine) {
				t.Errorf("Expected context to include '%s', got: %s", expectedLine, matches[0].Line)
			}
		}
	})

	// test adjacent matches with overlapping context
	t.Run("AdjacentMatchesOverlapContext", func(t *testing.T) {
		content := "line1\ntarget1 here\nline3\ntarget2 here\nline5"
		reader := strings.NewReader(content)
		pattern, _ := regexp.Compile("target")

		matches := scanTextFile(reader, pattern, "adjacent.txt", 1)
		if len(matches) != 2 {
			t.Errorf("Expected 2 matches, got %d", len(matches))
		}

		// each match should have its own context
		for _, match := range matches {
			lineCount := len(strings.Split(match.Line, "\n"))

			// match + 1 before + 1 after
			if lineCount != 3 {
				t.Errorf("Expected 3 lines in context, got %d in: %s", lineCount, match.Line)
			}
		}
	})
}
