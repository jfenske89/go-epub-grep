package epubproc

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

// searchAll runs a search and returns every result sorted by path, because the order of results is not fixed.
func searchAll(t *testing.T, fs FileSearch, request *SearchRequest) []*SearchResult {
	t.Helper()

	var mu sync.Mutex
	var results []*SearchResult

	err := fs.Search(context.Background(), request, func(result *SearchResult) error {
		mu.Lock()
		defer mu.Unlock()
		results = append(results, result)
		return nil
	})
	if err != nil {
		t.Fatalf("Search failed: %v", err)
	}

	slices.SortFunc(results, func(a, b *SearchResult) int {
		return strings.Compare(a.Path, b.Path)
	})
	return results
}

// mustCreateEPUB creates a test ePUB or stops the test.
func mustCreateEPUB(t *testing.T, dir, filename, content string) string {
	t.Helper()

	path, err := createTestEPUB(dir, filename, content)
	if err != nil {
		t.Fatalf("Failed to create test ePUB %s: %v", filename, err)
	}
	return path
}

func textRequest(value string, ignoreCase bool) *SearchRequest {
	return &SearchRequest{
		Query: SearchRequestQuery{
			Text: &SearchRequestText{Value: value, IgnoreCase: ignoreCase},
		},
	}
}

// TestFileSearchCaseSensitivity verifies that IgnoreCase decides whether a different case matches.
func TestFileSearchCaseSensitivity(t *testing.T) {
	dir := t.TempDir()
	mustCreateEPUB(t, dir, "book.epub", "<p>Holmes</p>")
	fs := NewFileSearch(dir, 2, false)

	if got := searchAll(t, fs, textRequest("holmes", true)); len(got) != 1 {
		t.Errorf("Expected 1 result for case-insensitive search, got %d", len(got))
	}
	if got := searchAll(t, fs, textRequest("holmes", false)); len(got) != 0 {
		t.Errorf("Expected 0 results for case-sensitive search with different case, got %d", len(got))
	}
	if got := searchAll(t, fs, textRequest("Holmes", false)); len(got) != 1 {
		t.Errorf("Expected 1 result for case-sensitive search with same case, got %d", len(got))
	}
}

// TestFileSearchTextIsLiteral verifies that regex metacharacters in a text query are matched as plain text.
func TestFileSearchTextIsLiteral(t *testing.T) {
	dir := t.TempDir()
	mustCreateEPUB(t, dir, "dot.epub", "<p>the code a.b here</p>")
	mustCreateEPUB(t, dir, "other.epub", "<p>the code axb here</p>")

	results := searchAll(t, NewFileSearch(dir, 2, false), textRequest("a.b", false))

	if len(results) != 1 || filepath.Base(results[0].Path) != "dot.epub" {
		t.Fatalf("Expected only dot.epub to match 'a.b', got %d results: %+v", len(results), results)
	}
}

// TestFileSearchInlineTags verifies that a phrase split by inline markup can still be found.
func TestFileSearchInlineTags(t *testing.T) {
	dir := t.TempDir()
	mustCreateEPUB(t, dir, "book.epub", "<p><i>word</i>, then <span>T</span>he end</p>")
	fs := NewFileSearch(dir, 1, false)

	for _, query := range []string{"word, then", "The end"} {
		if got := searchAll(t, fs, textRequest(query, false)); len(got) != 1 {
			t.Errorf("Expected 1 result for %q, got %d", query, len(got))
		}
	}
}

// TestFileSearchFilesIn verifies that FilesIn limits the search to the listed paths.
func TestFileSearchFilesIn(t *testing.T) {
	dir := t.TempDir()
	first := mustCreateEPUB(t, dir, "first.epub", "<p>Holmes</p>")
	mustCreateEPUB(t, dir, "second.epub", "<p>Holmes</p>")
	fs := NewFileSearch(dir, 2, false)

	request := textRequest("Holmes", false)
	request.Filters = &SearchRequestFilters{FilesIn: []string{filepath.Join(dir, "missing.epub")}}
	if got := searchAll(t, fs, request); len(got) != 0 {
		t.Errorf("Expected 0 results for a FilesIn path that does not exist, got %d", len(got))
	}

	request.Filters = &SearchRequestFilters{FilesIn: []string{first}}
	got := searchAll(t, fs, request)
	if len(got) != 1 || got[0].Path != first {
		t.Errorf("Expected only %s, got %+v", first, got)
	}
}

// TestFileSearchMetadataFilter verifies that a metadata filter keeps only matching books.
func TestFileSearchMetadataFilter(t *testing.T) {
	dir := t.TempDir()
	mustCreateEPUB(t, dir, "book.epub", "<p>Holmes</p>")
	fs := NewFileSearch(dir, 1, true)

	request := textRequest("Holmes", false)
	request.Filters = &SearchRequestFilters{AuthorEquals: "Nobody Atall"}
	if got := searchAll(t, fs, request); len(got) != 0 {
		t.Errorf("Expected 0 results for a non-matching author, got %d", len(got))
	}

	request.Filters = &SearchRequestFilters{AuthorEquals: "Test Author"}
	got := searchAll(t, fs, request)
	if len(got) != 1 {
		t.Fatalf("Expected 1 result for the matching author, got %d", len(got))
	}
	if got[0].Metadata.Title != "Test Book" {
		t.Errorf("Expected metadata title %q, got %q", "Test Book", got[0].Metadata.Title)
	}
}

// TestFileSearchCorruptEpub verifies that one unreadable book does not stop the search.
func TestFileSearchCorruptEpub(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "bad.epub"), []byte("this is not a zip file"), 0o600); err != nil {
		t.Fatalf("Failed to write corrupt ePUB: %v", err)
	}
	good := mustCreateEPUB(t, dir, "good.epub", "<p>Holmes</p>")

	got := searchAll(t, NewFileSearch(dir, 2, false), textRequest("Holmes", false))

	if len(got) != 1 || got[0].Path != good {
		t.Errorf("Expected only %s, got %+v", good, got)
	}
}

// TestFileSearchFindsUppercaseAndNested verifies the extension check ignores case and the walk enters subdirectories.
func TestFileSearchFindsUppercaseAndNested(t *testing.T) {
	dir := t.TempDir()
	nested := filepath.Join(dir, "shelf", "fiction")
	if err := os.MkdirAll(nested, 0o755); err != nil {
		t.Fatalf("Failed to create nested dir: %v", err)
	}
	upper := mustCreateEPUB(t, nested, "BOOK.EPUB", "<p>Holmes</p>")
	mustCreateEPUB(t, dir, "notes.txt", "Holmes") // not an epub, must be ignored

	got := searchAll(t, NewFileSearch(dir, 2, false), textRequest("Holmes", false))

	if len(got) != 1 || got[0].Path != upper {
		t.Errorf("Expected only %s, got %+v", upper, got)
	}
}

// TestFileSearchNoLeakBetweenBooks verifies that parallel workers return each book's own matches only.
func TestFileSearchNoLeakBetweenBooks(t *testing.T) {
	dir := t.TempDir()
	const books = 12
	for i := range books {
		mustCreateEPUB(t, dir, fmt.Sprintf("book%02d.epub", i), fmt.Sprintf("<p>unique-token-%02d here</p>", i))
	}

	request := &SearchRequest{
		Query: SearchRequestQuery{
			Regex: &SearchRequestRegex{Pattern: `unique-token-\d\d`},
		},
	}
	got := searchAll(t, NewFileSearch(dir, 4, false), request)

	if len(got) != books {
		t.Fatalf("Expected %d results, got %d", books, len(got))
	}
	for i, result := range got {
		want := fmt.Sprintf("unique-token-%02d", i)
		if filepath.Base(result.Path) != fmt.Sprintf("book%02d.epub", i) {
			t.Errorf("Result %d has unexpected path %s", i, result.Path)
		}
		if len(result.Matches) != 1 || !strings.Contains(result.Matches[0].Line, want) {
			t.Errorf("Result %s: expected one match containing %q, got %+v", result.Path, want, result.Matches)
		}
	}
}

// TestFileSearchMissingMetadata verifies that a book with matches but unreadable metadata is still returned,
// unless a metadata filter needs that metadata.
func TestFileSearchMissingMetadata(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "no-opf.epub")
	if err := createTestZIPWithFiles(path, map[string]string{"chapter.html": "<p>Holmes</p>"}); err != nil {
		t.Fatalf("Failed to create ePUB without OPF: %v", err)
	}
	fs := NewFileSearch(dir, 1, true)

	got := searchAll(t, fs, textRequest("Holmes", false))
	if len(got) != 1 || got[0].Path != path {
		t.Fatalf("Expected the book to be returned without metadata, got %+v", got)
	}
	if got[0].Metadata.Title != "" || len(got[0].Matches) != 1 {
		t.Errorf("Expected empty metadata and 1 match, got %+v", got[0])
	}

	request := textRequest("Holmes", false)
	request.Filters = &SearchRequestFilters{AuthorEquals: "Test Author"}
	if got := searchAll(t, fs, request); len(got) != 0 {
		t.Errorf("Expected the book to be dropped by an author filter, got %+v", got)
	}
}

// makeUnreadableDir creates a subdirectory containing an ePUB and removes read access to it.
func makeUnreadableDir(t *testing.T, root, name string) {
	t.Helper()

	if os.Geteuid() == 0 {
		t.Skip("permissions are not enforced for root")
	}

	dir := filepath.Join(root, name)
	if err := os.Mkdir(dir, 0o755); err != nil {
		t.Fatalf("Failed to create dir: %v", err)
	}
	mustCreateEPUB(t, dir, "hidden.epub", "<p>Holmes</p>")

	if err := os.Chmod(dir, 0o000); err != nil {
		t.Fatalf("Failed to remove permissions: %v", err)
	}
	// t.TempDir cleanup cannot delete a directory it cannot read
	t.Cleanup(func() { _ = os.Chmod(dir, 0o755) })
}

// TestFileSearchUnreadableSubdirectory verifies that a subdirectory without read access is skipped.
func TestFileSearchUnreadableSubdirectory(t *testing.T) {
	dir := t.TempDir()
	good := mustCreateEPUB(t, dir, "good.epub", "<p>Holmes</p>")
	makeUnreadableDir(t, dir, "locked")

	got := searchAll(t, NewFileSearch(dir, 2, false), textRequest("Holmes", false))

	if len(got) != 1 || got[0].Path != good {
		t.Errorf("Expected only %s, got %+v", good, got)
	}
}

// TestProcessDirectoryUnreadableSubdirectory verifies that metadata extraction also skips an unreadable subdirectory.
func TestProcessDirectoryUnreadableSubdirectory(t *testing.T) {
	dir := t.TempDir()
	good := mustCreateEPUB(t, dir, "good.epub", "<p>Holmes</p>")
	makeUnreadableDir(t, dir, "locked")

	var mu sync.Mutex
	var paths []string
	err := NewMetadataExtractor(2).ProcessDirectory(context.Background(), dir, func(epubPath string, _ *Metadata) error {
		mu.Lock()
		defer mu.Unlock()
		paths = append(paths, epubPath)
		return nil
	})
	if err != nil {
		t.Fatalf("ProcessDirectory failed: %v", err)
	}

	if len(paths) != 1 || paths[0] != good {
		t.Errorf("Expected only %s, got %v", good, paths)
	}
}

// TestFileSearchFilesInPathSpellings verifies that FilesIn matches the same file however its path is written.
func TestFileSearchFilesInPathSpellings(t *testing.T) {
	dir := t.TempDir()
	target := mustCreateEPUB(t, dir, "first.epub", "<p>Holmes</p>")
	mustCreateEPUB(t, dir, "second.epub", "<p>Holmes</p>")
	fs := NewFileSearch(dir, 2, false)

	spellings := map[string]string{
		"dot segment":    dir + "/./first.epub",
		"parent segment": filepath.Join(dir, "other", "..") + "/../" + filepath.Base(dir) + "/first.epub",
		"double slash":   dir + "//first.epub",
	}

	// a relative path is only possible when the directory is below the working directory
	if wd, err := os.Getwd(); err == nil {
		if rel, err := filepath.Rel(wd, target); err == nil {
			spellings["relative"] = rel
		}
	}

	for name, spelling := range spellings {
		t.Run(name, func(t *testing.T) {
			request := textRequest("Holmes", false)
			request.Filters = &SearchRequestFilters{FilesIn: []string{spelling}}

			got := searchAll(t, fs, request)

			if len(got) != 1 || got[0].Path != target {
				t.Errorf("Expected only %s for FilesIn %q, got %+v", target, spelling, got)
			}
		})
	}
}

// TestFileSearchContextEnd verifies that Search reports why it stopped when the context ends, for a deadline as
// well as for a cancel, and that no result is handled after that.
func TestFileSearchContextEnd(t *testing.T) {
	dir := t.TempDir()
	for i := range 5 {
		mustCreateEPUB(t, dir, fmt.Sprintf("book%d.epub", i), "<p>Holmes</p>")
	}

	expired, cancelExpired := context.WithDeadline(context.Background(), time.Now().Add(-time.Second))
	defer cancelExpired()

	cancelled, cancel := context.WithCancel(context.Background())
	cancel()

	tests := []struct {
		name string
		ctx  context.Context
		want error
	}{
		{"expired deadline", expired, context.DeadlineExceeded},
		{"cancelled", cancelled, context.Canceled},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			var handlerCalls atomic.Int32

			err := NewFileSearch(dir, 2, false).Search(test.ctx, textRequest("Holmes", false), func(*SearchResult) error {
				handlerCalls.Add(1)
				return nil
			})

			if !errors.Is(err, test.want) {
				t.Errorf("Expected %v, got: %v", test.want, err)
			}
			if handlerCalls.Load() != 0 {
				t.Errorf("Expected no handler calls after the context ended, got %d", handlerCalls.Load())
			}
		})
	}
}

// TestFileSearchHandlerErrorIsNotMasked verifies that the handler error is returned as it is, without a context error.
func TestFileSearchHandlerErrorIsNotMasked(t *testing.T) {
	dir := t.TempDir()
	for i := range 5 {
		mustCreateEPUB(t, dir, fmt.Sprintf("book%d.epub", i), "<p>Holmes</p>")
	}
	errHandler := errors.New("handler failed")

	err := NewFileSearch(dir, 1, false).Search(context.Background(), textRequest("Holmes", false), func(*SearchResult) error {
		return errHandler
	})

	if !errors.Is(err, errHandler) || errors.Is(err, context.Canceled) {
		t.Errorf("Expected only the handler error, got: %v", err)
	}
}

// TestFileSearchRegexIgnoreCase verifies the IgnoreCase option for regex queries, and that an invalid pattern is
// reported as the caller wrote it.
func TestFileSearchRegexIgnoreCase(t *testing.T) {
	dir := t.TempDir()
	mustCreateEPUB(t, dir, "book.epub", "<p>Holmes and WATSON</p>")
	fs := NewFileSearch(dir, 1, false)

	regexRequest := func(pattern string, ignoreCase bool) *SearchRequest {
		return &SearchRequest{Query: SearchRequestQuery{Regex: &SearchRequestRegex{Pattern: pattern, IgnoreCase: ignoreCase}}}
	}

	if got := searchAll(t, fs, regexRequest("holmes|watson", true)); len(got) != 1 {
		t.Errorf("Expected 1 result with IgnoreCase, got %d", len(got))
	}
	if got := searchAll(t, fs, regexRequest("holmes|watson", false)); len(got) != 0 {
		t.Errorf("Expected 0 results without IgnoreCase, got %d", len(got))
	}

	err := fs.Search(context.Background(), regexRequest("[invalid", true), func(*SearchResult) error { return nil })
	if err == nil || !strings.Contains(err.Error(), `invalid pattern "[invalid"`) {
		t.Errorf("Expected an error that shows the pattern as written, got: %v", err)
	}
}
