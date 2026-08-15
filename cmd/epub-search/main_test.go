package main

import (
	"context"
	"strings"
	"testing"

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
			err := runSearch(context.Background(), test.flags)
			if err == nil || !strings.Contains(err.Error(), test.wantErr) {
				t.Errorf("expected error containing %q, got: %v", test.wantErr, err)
			}
		})
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
