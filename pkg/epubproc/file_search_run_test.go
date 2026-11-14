package epubproc

import (
	"context"
	"errors"
	"regexp"
	"testing"
)

// fakeExtractor returns fixed metadata and counts how often it is asked.
type fakeExtractor struct {
	metadata *Metadata
	err      error
	calls    int
}

func (f *fakeExtractor) ProcessFile(context.Context, string) (*Metadata, error) {
	f.calls++
	return f.metadata, f.err
}

func (f *fakeExtractor) ProcessDirectory(context.Context, string, MetadataHandler) error {
	return errors.New("not used")
}

// fakeGrep returns fixed matches and counts how often a book is scanned.
type fakeGrep struct {
	matches []Match
	err     error
	calls   int
}

func (f *fakeGrep) grep(context.Context, string, *regexp.Regexp, int) ([]Match, error) {
	f.calls++
	return f.matches, f.err
}

// TestSearchBookOrder verifies when the metadata is read and when the text is scanned. With a metadata filter the
// text of a book that does not pass the filter must never be scanned.
func TestSearchBookOrder(t *testing.T) {
	authorFilter := &SearchRequestFilters{AuthorEquals: "Jane Doe"}
	janeDoe := &Metadata{Title: "Book", Authors: []string{"Jane Doe"}}
	someoneElse := &Metadata{Title: "Other", Authors: []string{"Someone Else"}}
	oneMatch := []Match{{Line: "needle", FileName: "chapter.html"}}
	errScan := errors.New("scan failed")

	tests := []struct {
		name         string
		filters      *SearchRequestFilters
		noExtractor  bool
		metadata     *Metadata
		extractErr   error
		matches      []Match
		grepErr      error
		wantResult   bool
		wantTitle    string
		wantGrep     int
		wantExtract  int
		wantErrIsErr error
	}{
		{
			name:        "filter removes the book before the text is scanned",
			filters:     authorFilter,
			metadata:    someoneElse,
			matches:     oneMatch,
			wantGrep:    0,
			wantExtract: 1,
		},
		{
			name:        "filter passes: metadata is read once and reused",
			filters:     authorFilter,
			metadata:    janeDoe,
			matches:     oneMatch,
			wantResult:  true,
			wantTitle:   "Book",
			wantGrep:    1,
			wantExtract: 1,
		},
		{
			name:        "filter passes but the text has no match",
			filters:     authorFilter,
			metadata:    janeDoe,
			wantGrep:    1,
			wantExtract: 1,
		},
		{
			name:        "filter and unreadable metadata: the book is skipped without a scan",
			filters:     authorFilter,
			extractErr:  errors.New("no opf"),
			matches:     oneMatch,
			wantGrep:    0,
			wantExtract: 1,
		},
		{
			name:        "no filter and no match: metadata is not read",
			metadata:    janeDoe,
			wantGrep:    1,
			wantExtract: 0,
		},
		{
			name:        "no filter and a match: metadata is read after the scan",
			metadata:    janeDoe,
			matches:     oneMatch,
			wantResult:  true,
			wantTitle:   "Book",
			wantGrep:    1,
			wantExtract: 1,
		},
		{
			name:        "no filter and unreadable metadata: the book is still returned",
			extractErr:  errors.New("no opf"),
			matches:     oneMatch,
			wantResult:  true,
			wantGrep:    1,
			wantExtract: 1,
		},
		{
			name:        "FilesIn alone is not a metadata filter",
			filters:     &SearchRequestFilters{FilesIn: []string{"book.epub"}},
			metadata:    janeDoe,
			wantGrep:    1,
			wantExtract: 0,
		},
		{
			name:        "metadata is off: filters are not applied",
			filters:     authorFilter,
			noExtractor: true,
			matches:     oneMatch,
			wantResult:  true,
			wantGrep:    1,
			wantExtract: 0,
		},
		{
			name:         "scan error is returned",
			grepErr:      errScan,
			wantGrep:     1,
			wantErrIsErr: errScan,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			extractor := &fakeExtractor{metadata: test.metadata, err: test.extractErr}
			scanner := &fakeGrep{matches: test.matches, err: test.grepErr}

			matchers, err := newFilterMatchers(test.filters)
			if err != nil {
				t.Fatalf("newFilterMatchers failed: %v", err)
			}

			run := &searchRun{
				pattern:  regexp.MustCompile("needle"),
				filters:  test.filters,
				matchers: matchers,
				grep:     scanner.grep,
			}
			if !test.noExtractor {
				run.extractor = extractor
			}

			result, err := run.searchBook(context.Background(), "book.epub")

			if test.wantErrIsErr != nil {
				if !errors.Is(err, test.wantErrIsErr) {
					t.Fatalf("expected error %v, got %v", test.wantErrIsErr, err)
				}
			} else if err != nil {
				t.Fatalf("searchBook failed: %v", err)
			}

			if (result != nil) != test.wantResult {
				t.Fatalf("expected result = %t, got %+v", test.wantResult, result)
			}
			if result != nil && result.Metadata.Title != test.wantTitle {
				t.Errorf("expected title %q, got %q", test.wantTitle, result.Metadata.Title)
			}
			if scanner.calls != test.wantGrep {
				t.Errorf("expected %d scans, got %d", test.wantGrep, scanner.calls)
			}
			if extractor.calls != test.wantExtract {
				t.Errorf("expected %d metadata reads, got %d", test.wantExtract, extractor.calls)
			}
		})
	}
}
