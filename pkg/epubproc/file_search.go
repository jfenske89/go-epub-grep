package epubproc

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"
	"sync/atomic"

	"github.com/rs/zerolog/log"
	"github.com/sourcegraph/conc/pool"
)

// ResultHandler defines a handler function for epub results.
//
// Search calls the handler from up to maxThreads goroutines at the same time, in no fixed order, so the handler
// must be safe for concurrent use. If the handler returns an error, Search cancels the remaining work and returns
// that error.
type ResultHandler func(result *SearchResult) error

// FileSearch defines the interface for searching within epub files.
type FileSearch interface {
	// Search performs a search across multiple epub files, streaming results via a handler function.
	Search(ctx context.Context, request *SearchRequest, handler ResultHandler) error
}

type fileSearchImpl struct {
	epubDir         string
	maxThreads      int
	extractMetadata bool
}

// NewFileSearch creates a new FileSearch instance for the specified epub directory.
func NewFileSearch(epubDir string, maxThreads int, extractMetadata bool) FileSearch {
	if maxThreads <= 0 {
		// default to number of CPU cores if not specified
		maxThreads = runtime.NumCPU()
	}

	return &fileSearchImpl{
		epubDir:         epubDir,
		maxThreads:      maxThreads,
		extractMetadata: extractMetadata,
	}
}

// Search performs a full-text search across all epub files in the configured directory.
//
// It returns an error if request.Context is negative. If ctx ends before the search finishes, Search stops
// starting new files, calls the handler no more, and returns an error that wraps ctx.Err(), whether the context was
// cancelled or reached its deadline. A file that cannot be read is logged and skipped, so one damaged book does not
// stop the search.
func (s *fileSearchImpl) Search(ctx context.Context, request *SearchRequest, handler ResultHandler) error {
	if request.Context < 0 {
		return fmt.Errorf("context must not be negative: %d", request.Context)
	}

	patternRegex, err := compilePattern(request.Query)
	if err != nil {
		return err
	}

	matchers, err := newFilterMatchers(request.Filters)
	if err != nil {
		return err
	}

	var filesIn map[string]struct{}
	if request.Filters != nil && len(request.Filters.FilesIn) > 0 {
		filesIn = make(map[string]struct{}, len(request.Filters.FilesIn))
		for _, path := range request.Filters.FilesIn {
			filesIn[normalizePath(path)] = struct{}{}
		}
	}

	// stopped records that the pool context ended before all files were handled. The goroutines return nil in that
	// case, so a handler error stays the only error returned and a cancelled parent context can be told apart from
	// a finished search.
	var stopped atomic.Bool

	p := pool.New().WithContext(ctx).WithCancelOnError()
	paths := make(chan string)

	// producer goroutine to find all .epub files
	p.Go(func(poolCtx context.Context) error {
		defer close(paths)
		return filepath.WalkDir(s.epubDir, func(path string, d fs.DirEntry, err error) error {
			if err != nil {
				return handleWalkError(s.epubDir, path, d, err)
			}

			if d.IsDir() || !strings.HasSuffix(strings.ToLower(d.Name()), ".epub") {
				return nil
			}

			if filesIn != nil {
				if _, ok := filesIn[normalizePath(path)]; !ok {
					return nil
				}
			}

			select {
			case paths <- path:
				return nil
			case <-poolCtx.Done():
				stopped.Store(true)
				return filepath.SkipAll
			}
		})
	})

	run := &searchRun{
		pattern:      patternRegex,
		contextLines: request.Context,
		filters:      request.Filters,
		matchers:     matchers,
		grep:         grepInEpub,
	}
	if s.extractMetadata {
		run.extractor = NewMetadataExtractor(s.maxThreads)
	}

	// worker goroutines to process files
	for range s.maxThreads {
		p.Go(func(poolCtx context.Context) error {
			for path := range paths {
				if poolCtx.Err() != nil {
					stopped.Store(true)
					return nil
				}

				result, err := run.searchBook(poolCtx, path)
				if err != nil {
					if poolCtx.Err() != nil {
						stopped.Store(true)
						return nil
					}

					log.Err(err).Str("path", path).Msg("error searching in epub")
					continue
				}
				if result == nil {
					continue
				}

				// the handler must not run after the context has ended
				if poolCtx.Err() != nil {
					stopped.Store(true)
					return nil
				}

				if err := handler(result); err != nil {
					return err
				}
			}
			return nil
		})
	}

	err = p.Wait()
	if err == nil && stopped.Load() && ctx.Err() != nil {
		return fmt.Errorf("search stopped: %w", ctx.Err())
	}
	return err
}

// searchRun holds what is needed to search one book, and is shared by all workers of a search.
type searchRun struct {
	pattern      *regexp.Regexp
	contextLines int
	filters      *SearchRequestFilters
	matchers     *filterMatchers

	// extractor is nil when the search does not extract metadata.
	extractor MetadataExtractor

	// grep scans one book. It is a field so that a test can count the books that are scanned.
	grep func(ctx context.Context, path string, pattern *regexp.Regexp, contextLines int) ([]Match, error)
}

// searchBook searches one book. It returns a nil result, without an error, when the book has no match or is
// removed by a metadata filter.
//
// With a metadata filter, the metadata is read first, because it is much cheaper than scanning the text and most
// books do not pass the filter. Without a filter, the text is scanned first, so metadata is read only for books
// that matched. A book with matches is still a result when its metadata cannot be read, unless a metadata filter
// needs that metadata.
func (r *searchRun) searchBook(ctx context.Context, path string) (*SearchResult, error) {
	var metadata Metadata

	filterFirst := r.extractor != nil && hasMetadataFilters(r.filters)
	if filterFirst {
		extracted, err := r.extractor.ProcessFile(ctx, path)
		if err != nil {
			log.Warn().Err(err).Str("path", path).Msg("error extracting metadata, skipping the book because a metadata filter is set")
			return nil, nil
		}

		metadata = *extracted
		if !matchesMetadataFilters(metadata, r.filters, r.matchers) {
			return nil, nil
		}
	}

	matches, err := r.grep(ctx, path, r.pattern, r.contextLines)
	if err != nil {
		return nil, err
	}
	if len(matches) == 0 {
		return nil, nil
	}

	if r.extractor != nil && !filterFirst {
		extracted, err := r.extractor.ProcessFile(ctx, path)
		if err != nil {
			log.Warn().Err(err).Str("path", path).Msg("error extracting metadata, returning result without it")
		} else {
			metadata = *extracted
		}
	}

	return &SearchResult{Path: path, Metadata: metadata, Matches: matches}, nil
}

// hasMetadataFilters reports whether the request filters books by author, series or title.
func hasMetadataFilters(filters *SearchRequestFilters) bool {
	return filters != nil && (filters.AuthorEquals != "" || filters.SeriesEquals != "" || filters.TitleEquals != "")
}

// compilePattern builds the regular expression for a query. A text query is matched as plain text. The error shows
// the pattern as the caller wrote it, without the options added here.
func compilePattern(query SearchRequestQuery) (*regexp.Regexp, error) {
	var userValue, pattern string

	switch {
	case query.Regex != nil:
		userValue = query.Regex.Pattern
		pattern = userValue
		if query.Regex.IgnoreCase {
			pattern = "(?i)" + pattern
		}
	case query.Text != nil:
		userValue = query.Text.Value
		pattern = regexp.QuoteMeta(userValue)
		if query.Text.IgnoreCase {
			pattern = "(?i)" + pattern
		}
	default:
		return nil, errors.New("text configuration is required when Regex is empty")
	}

	compiled, err := regexp.Compile(pattern)
	if err != nil {
		return nil, fmt.Errorf("invalid pattern %q: %w", userValue, err)
	}
	return compiled, nil
}

// normalizePath makes two spellings of the same file path compare equal, such as "./lib/a.epub" and "lib/a.epub".
func normalizePath(path string) string {
	if abs, err := filepath.Abs(path); err == nil {
		return abs
	}
	return filepath.Clean(path)
}

// handleWalkError decides what a directory walk error means. An error on the root directory is fatal, because
// nothing can be searched. An error on anything below it, such as a subdirectory without read permission, is logged
// and skipped so that one bad path does not discard the results from the rest of the library.
func handleWalkError(root, path string, d fs.DirEntry, err error) error {
	if path == root || d == nil {
		return err
	}

	log.Warn().Err(err).Str("path", path).Msg("skipping path that cannot be read")
	if d.IsDir() {
		return fs.SkipDir
	}
	return nil
}
