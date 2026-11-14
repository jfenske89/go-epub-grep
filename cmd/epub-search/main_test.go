package main

import (
	"archive/zip"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/rs/zerolog"

	"github.com/jfenske89/go-epub-grep/pkg/epubproc"
)

// TestRunSearchFilterMatchValidation verifies --filter-match is validated before a search runs.
func TestRunSearchFilterMatchValidation(t *testing.T) {
	tests := []struct {
		name    string
		flags   *searchFlags
		wantErr string
	}{
		{
			name: "invalid filter-match value",
			flags: &searchFlags{
				epubDir:     ".",
				pattern:     "test",
				filterMatch: "fuzzy",
			},
			wantErr: "invalid --filter-match value",
		},
		{
			name: "filter-match word without a metadata filter",
			flags: &searchFlags{
				epubDir:     ".",
				pattern:     "test",
				filterMatch: "word",
			},
			wantErr: "requires at least one of --author, --series, --title",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			err := runSearch(context.Background(), test.flags, io.Discard)
			if err == nil || !strings.Contains(err.Error(), test.wantErr) {
				t.Errorf("expected error containing %q, got: %v", test.wantErr, err)
			}
		})
	}
}

// TestRunSearchDirectoryValidation verifies that the directory flag must name an existing directory.
func TestRunSearchDirectoryValidation(t *testing.T) {
	dir := t.TempDir()
	file := filepath.Join(dir, "book.epub")
	if err := os.WriteFile(file, []byte("not a directory"), 0o600); err != nil {
		t.Fatalf("Failed to write file: %v", err)
	}

	tests := []struct {
		name    string
		epubDir string
		wantErr string
		wantIs  error
	}{
		{"missing path", filepath.Join(dir, "missing"), "directory does not exist", fs.ErrNotExist},
		{"file instead of directory", file, "not a directory", nil},
		{"path through a file", filepath.Join(file, "child"), "cannot access directory", nil},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			flags := &searchFlags{
				epubDir:     test.epubDir,
				pattern:     "test",
				filterMatch: string(epubproc.FilterMatchExact),
			}

			err := runSearch(context.Background(), flags, io.Discard)
			if err == nil {
				t.Fatal("expected an error, got nil")
			}
			if !strings.Contains(err.Error(), test.wantErr) {
				t.Errorf("expected error containing %q, got: %v", test.wantErr, err)
			}
			if test.wantIs != nil && !errors.Is(err, test.wantIs) {
				t.Errorf("expected error to wrap %v, got: %v", test.wantIs, err)
			}
		})
	}
}

// TestRunSearchNegativeContext verifies a negative --context is rejected instead of reaching the search.
func TestRunSearchNegativeContext(t *testing.T) {
	flags := &searchFlags{
		epubDir:     t.TempDir(),
		pattern:     "test",
		context:     -1,
		filterMatch: string(epubproc.FilterMatchExact),
	}

	err := runSearch(context.Background(), flags, io.Discard)
	if err == nil || !strings.Contains(err.Error(), "invalid --context value") {
		t.Errorf("expected invalid --context error, got: %v", err)
	}
}

// TestBuildSearchRequestFilterMatch verifies the --filter-match flag is carried into the request.
func TestBuildSearchRequestFilterMatch(t *testing.T) {
	flags := &searchFlags{
		pattern:      "test",
		authorEquals: "Tolkien",
		filterMatch:  "word",
	}

	request := buildSearchRequest(flags)
	if request.Filters == nil {
		t.Fatal("expected filters to be set")
	}
	if request.Filters.MatchMode != epubproc.FilterMatchWord {
		t.Errorf("expected MatchMode %q, got %q", epubproc.FilterMatchWord, request.Filters.MatchMode)
	}
}

// writeTextEPUB writes a small epub-like zip that holds one plain text file, which is enough to search.
func writeTextEPUB(t *testing.T, dir, name, text string) string {
	t.Helper()

	path := filepath.Join(dir, name)
	file, err := os.Create(path)
	if err != nil {
		t.Fatalf("Failed to create %s: %v", path, err)
	}
	defer file.Close()

	writer := zip.NewWriter(file)
	entry, err := writer.Create("chapter.txt")
	if err != nil {
		t.Fatalf("Failed to create zip entry: %v", err)
	}
	if _, err := entry.Write([]byte(text)); err != nil {
		t.Fatalf("Failed to write zip entry: %v", err)
	}
	if err := writer.Close(); err != nil {
		t.Fatalf("Failed to close zip: %v", err)
	}

	return path
}

// runSearchForTest runs a search with defaults for the flags that a test does not care about.
func runSearchForTest(t *testing.T, flags *searchFlags) (searchOutput, string, error) {
	t.Helper()

	flags.filterMatch = string(epubproc.FilterMatchExact)
	if flags.maxThreads == 0 {
		flags.maxThreads = 2
	}

	var out bytes.Buffer
	err := runSearch(context.Background(), flags, &out)

	var output searchOutput
	if err == nil {
		if decodeErr := json.Unmarshal(out.Bytes(), &output); decodeErr != nil {
			t.Fatalf("Output is not valid JSON: %v\n%s", decodeErr, out.String())
		}
	}
	return output, out.String(), err
}

// TestRunSearchFlagCombinations verifies that the search flags change the result in the way their help text says.
func TestRunSearchFlagCombinations(t *testing.T) {
	dir := t.TempDir()
	writeTextEPUB(t, dir, "book.epub", "one\ntwo\nNeedle here\nfour\nfive\n")

	tests := []struct {
		name      string
		flags     searchFlags
		wantLines []string // each must be in the single match line
		notLines  []string // none may be in the match line
		wantFiles int
	}{
		{
			name:      "plain text is case-sensitive by default",
			flags:     searchFlags{pattern: "needle"},
			wantFiles: 0,
		},
		{
			name:      "ignore-case with plain text",
			flags:     searchFlags{pattern: "needle", ignoreCase: true},
			wantLines: []string{"Needle here"},
			wantFiles: 1,
		},
		{
			name:      "regex is case-sensitive by default",
			flags:     searchFlags{pattern: "needle|haystack", isRegex: true},
			wantFiles: 0,
		},
		{
			name:      "ignore-case with regex",
			flags:     searchFlags{pattern: "needle|haystack", isRegex: true, ignoreCase: true},
			wantLines: []string{"Needle here"},
			wantFiles: 1,
		},
		{
			name:      "no context returns only the matching line",
			flags:     searchFlags{pattern: "Needle"},
			wantLines: []string{"Needle here"},
			notLines:  []string{"two", "four"},
			wantFiles: 1,
		},
		{
			name:      "context 1 adds one line on each side",
			flags:     searchFlags{pattern: "Needle", context: 1},
			wantLines: []string{"two", "Needle here", "four"},
			notLines:  []string{"one", "five"},
			wantFiles: 1,
		},
		{
			name:      "context larger than the file returns the whole file",
			flags:     searchFlags{pattern: "Needle", context: 100},
			wantLines: []string{"one", "two", "Needle here", "four", "five"},
			wantFiles: 1,
		},
		{
			name:      "zero threads uses the default",
			flags:     searchFlags{pattern: "Needle", maxThreads: 0},
			wantLines: []string{"Needle here"},
			wantFiles: 1,
		},
		{
			name:      "negative threads uses the default",
			flags:     searchFlags{pattern: "Needle", maxThreads: -3},
			wantLines: []string{"Needle here"},
			wantFiles: 1,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			flags := test.flags
			flags.epubDir = dir

			output, _, err := runSearchForTest(t, &flags)
			if err != nil {
				t.Fatalf("runSearch failed: %v", err)
			}

			if output.Summary.TotalFiles != test.wantFiles {
				t.Fatalf("expected %d files, got %+v", test.wantFiles, output.Summary)
			}
			if test.wantFiles == 0 {
				return
			}

			if len(output.Results) != 1 || len(output.Results[0].Matches) != 1 {
				t.Fatalf("expected 1 result with 1 match, got %+v", output.Results)
			}
			line := output.Results[0].Matches[0].Line
			for _, want := range test.wantLines {
				if !strings.Contains(line, want) {
					t.Errorf("expected match %q to contain %q", line, want)
				}
			}
			for _, not := range test.notLines {
				if strings.Contains(line, not) {
					t.Errorf("expected match %q not to contain %q", line, not)
				}
			}
		})
	}
}

// TestRunSearchInvalidRegex verifies that the error shows the pattern the user typed.
func TestRunSearchInvalidRegex(t *testing.T) {
	flags := &searchFlags{epubDir: t.TempDir(), pattern: "[invalid", isRegex: true, ignoreCase: true}

	_, _, err := runSearchForTest(t, flags)

	if err == nil || !strings.Contains(err.Error(), `invalid pattern "[invalid"`) {
		t.Errorf("expected an error that shows the pattern as typed, got: %v", err)
	}
}

// TestRunSearchOutputOrderAndEscaping verifies that results are sorted by path on every run, and that HTML characters
// in book text are written as they are.
func TestRunSearchOutputOrderAndEscaping(t *testing.T) {
	dir := t.TempDir()
	names := []string{"e.epub", "b.epub", "d.epub", "a.epub", "c.epub", "f.epub"}
	for _, name := range names {
		writeTextEPUB(t, dir, name, "x < y & z needle\n")
	}

	for run := range 10 {
		flags := &searchFlags{epubDir: dir, pattern: "needle", maxThreads: 4}

		output, raw, err := runSearchForTest(t, flags)
		if err != nil {
			t.Fatalf("run %d: runSearch failed: %v", run, err)
		}

		var paths []string
		for _, result := range output.Results {
			paths = append(paths, filepath.Base(result.Path))
		}
		if want := []string{"a.epub", "b.epub", "c.epub", "d.epub", "e.epub", "f.epub"}; !slices.Equal(paths, want) {
			t.Fatalf("run %d: expected sorted paths %v, got %v", run, want, paths)
		}
		if output.Summary.TotalFiles != 6 || output.Summary.TotalMatches != 6 {
			t.Fatalf("run %d: unexpected summary %+v", run, output.Summary)
		}
		if !strings.Contains(raw, "x < y & z needle") || strings.Contains(raw, "\\u003c") {
			t.Fatalf("run %d: expected HTML characters to be written as they are, got: %s", run, raw)
		}
	}
}

// TestRunSearchCancelledContext verifies that a cancelled context ends the search with an error that wraps the cause.
func TestRunSearchCancelledContext(t *testing.T) {
	dir := t.TempDir()
	writeTextEPUB(t, dir, "book.epub", "needle\n")

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	flags := &searchFlags{epubDir: dir, pattern: "needle", maxThreads: 1, filterMatch: string(epubproc.FilterMatchExact)}
	err := runSearch(ctx, flags, io.Discard)

	if !errors.Is(err, context.Canceled) {
		t.Errorf("expected context.Canceled, got: %v", err)
	}
}

// TestOutputJSON verifies the compact and pretty formats.
func TestOutputJSON(t *testing.T) {
	output := searchOutput{
		Results: []searchResult{},
		Summary: summaryInfo{TotalFiles: 0, TotalMatches: 0},
	}

	var compact, pretty bytes.Buffer
	if err := outputJSON(&compact, output, false); err != nil {
		t.Fatalf("compact output failed: %v", err)
	}
	if err := outputJSON(&pretty, output, true); err != nil {
		t.Fatalf("pretty output failed: %v", err)
	}

	if want := `{"results":[],"summary":{"totalFiles":0,"totalMatches":0}}` + "\n"; compact.String() != want {
		t.Errorf("compact output = %q, want %q", compact.String(), want)
	}
	if !strings.Contains(pretty.String(), "\n  \"summary\": {\n") {
		t.Errorf("expected indented output, got %q", pretty.String())
	}
}

// TestConfigureLogging verifies the level names that --log-level accepts.
func TestConfigureLogging(t *testing.T) {
	original := zerolog.GlobalLevel()
	t.Cleanup(func() { zerolog.SetGlobalLevel(original) })

	tests := map[string]zerolog.Level{
		"trace":    zerolog.TraceLevel,
		"DEBUG":    zerolog.DebugLevel,
		"info":     zerolog.InfoLevel,
		"warn":     zerolog.WarnLevel,
		"warning":  zerolog.WarnLevel,
		"error":    zerolog.ErrorLevel,
		"disabled": zerolog.Disabled,
		"nonsense": zerolog.WarnLevel,
		"":         zerolog.WarnLevel,
	}

	for level, want := range tests {
		t.Run(level, func(t *testing.T) {
			configureLogging(level)

			if got := zerolog.GlobalLevel(); got != want {
				t.Errorf("configureLogging(%q) set level %v, want %v", level, got, want)
			}
		})
	}
}
