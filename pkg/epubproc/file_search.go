package epubproc

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"path/filepath"
	"regexp"
	"runtime"
	"slices"
	"strings"

	"github.com/rs/zerolog/log"
	"github.com/sourcegraph/conc/pool"
)

// ResultHandler defines a handler function for epub results.
type ResultHandler func(result *SearchResult) error

// FileSearch defines the interface for searching within epub files.
type FileSearch interface {
	// Search performs a search across multiple epub files, streaming results via a handler function.
	Search(ctx context.Context, request *SearchRequest, handler ResultHandler) error
}

type fileSearchImpl struct {
	// epubDir is a directory containing epub files to search
	epubDir string

	// maxThreads fines the maximum number of worker goroutines to use
	maxThreads int

	// extractMetadata controls whether to extract metadata for search results
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
func (s *fileSearchImpl) Search(ctx context.Context, request *SearchRequest, handler ResultHandler) error {
	var pattern string
	if request.Query.IsRegex {
		if request.Query.Regex == nil {
			return fmt.Errorf("regex configuration is required when IsRegex is true")
		}

		pattern = request.Query.Regex.Pattern
	} else {
		if request.Query.Text == nil {
			return fmt.Errorf("text configuration is required when IsRegex is false")
		}

		pattern = regexp.QuoteMeta(request.Query.Text.Value)
		if request.Query.Text.IgnoreCase {
			pattern = "(?i)" + pattern
		}
	}

	patternRegex, err := patternCache.get(pattern)
	if err != nil {
		return fmt.Errorf("invalid pattern '%s': %w", pattern, err)
	}

	p := pool.New().WithContext(ctx).WithCancelOnError()
	paths := make(chan string)

	// producer goroutine to find all .epub files
	p.Go(func(ctx context.Context) error {
		defer close(paths)
		return filepath.WalkDir(s.epubDir, func(path string, d fs.DirEntry, err error) error {
			if err != nil {
				// an error during walk is fatal
				return err
			}

			if !d.IsDir() && strings.HasSuffix(strings.ToLower(d.Name()), ".epub") {
				// apply FilesIn filter if provided
				if request.Filters != nil && len(request.Filters.FilesIn) > 0 {
					if !slices.Contains(request.Filters.FilesIn, path) {
						// skip files not in the FilesIn list
						return nil
					}
				}

				select {
				case paths <- path:
				case <-ctx.Done():
					return ctx.Err()
				}
			}

			return nil
		})
	})

	var metaExtractor MetadataExtractor
	if s.extractMetadata {
		metaExtractor = NewMetadataExtractor(s.maxThreads)
	}

	// worker goroutines to process files
	for i := 0; i < s.maxThreads; i++ {
		p.Go(func(ctx context.Context) error {
			for path := range paths {
				select {
				case <-ctx.Done():
					err := ctx.Err()
					if errors.Is(err, context.Canceled) {
						// skip returning an error on cancel
						return nil
					}
					return err
				default:
				}

				matches, err := grepInEpub(ctx, path, patternRegex, request.Context)
				if err != nil && errors.Is(err, context.Canceled) {
					break
				} else if err != nil {
					log.Err(err).Str("path", path).Msg("error searching in epub")
					continue
				}

				if len(matches) > 0 {
					var metadata Metadata
					if s.extractMetadata {
						extractedMetadata, err := metaExtractor.ProcessFile(ctx, path)
						if err != nil {
							log.Err(err).Str("path", path).Msg("error extracting metadata")
							continue
						}
						metadata = *extractedMetadata
					}

					// apply metadata-based filters if provided and metadata is extracted
					if request.Filters != nil && s.extractMetadata {
						if !matchesMetadataFilters(metadata, request.Filters) {
							continue
						}
					}

					// send this result to the handler
					result := &SearchResult{
						Path:     path,
						Metadata: metadata,
						Matches:  matches,
					}
					if err := handler(result); err != nil {
						return err
					}
				}
			}
			return nil
		})
	}

	return p.Wait()
}
