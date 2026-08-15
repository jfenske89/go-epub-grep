package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"runtime"
	"strings"
	"sync"
	"time"

	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"
	"github.com/spf13/cobra"

	"github.com/jfenske89/go-epub-grep/pkg/epubproc"
)

type searchFlags struct {
	epubDir         string
	pattern         string
	isRegex         bool
	ignoreCase      bool
	context         int
	maxThreads      int
	extractMetadata bool
	authorEquals    string
	seriesEquals    string
	titleEquals     string
	filterMatch     string
	filesIn         []string
	pretty          bool
	logLevel        string
}

type searchOutput struct {
	Results []searchResult `json:"results"`
	Summary summaryInfo    `json:"summary"`
}

type searchResult struct {
	Path     string             `json:"path"`
	Metadata *epubproc.Metadata `json:"metadata,omitempty"`
	Matches  []epubproc.Match   `json:"matches"`
}

type summaryInfo struct {
	TotalFiles   int `json:"totalFiles"`
	TotalMatches int `json:"totalMatches"`
}

func main() {
	rootCmd := createRootCmd(context.Background())
	if err := rootCmd.Execute(); err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
}

func createRootCmd(ctx context.Context) *cobra.Command {
	flags := &searchFlags{}

	rootCmd := &cobra.Command{
		Use:   "epub-search",
		Short: "CLI tool for searching ePUB files",
		Long: `High-performance CLI tool for searching text content within ePUB files.
Supports plain text and regex pattern matching with metadata extraction and filtering.`,
		Example: `  # Simple text search
  epub-search search -d /path/to/epubs -p "search term"

  # Regex search
  epub-search search -d /path/to/epubs -p "pattern.*" --regex

  # Search with metadata filtering
  epub-search search -d /path/to/epubs -p "text" --author "Author Name" --extract-metadata

  # Enable logging for debugging
  epub-search search -d /path/to/epubs -p "text" --log-level info`,
	}

	searchCmd := createSearchCmd(ctx, flags)
	rootCmd.AddCommand(searchCmd)

	return rootCmd
}

func createSearchCmd(ctx context.Context, flags *searchFlags) *cobra.Command {
	searchCmd := &cobra.Command{
		Use:   "search",
		Short: "Search for text patterns in ePUB files",
		Long: `Search for text patterns within ePUB files using plain text or regex matching.
Supports concurrent processing, metadata extraction, and filtering options.`,
		RunE: func(cmd *cobra.Command, args []string) error {
			return runSearch(ctx, flags)
		},
	}

	setupSearchFlags(searchCmd, flags)
	return searchCmd
}

func setupSearchFlags(cmd *cobra.Command, flags *searchFlags) {
	// register required flags
	cmd.Flags().StringVarP(&flags.epubDir, "directory", "d", "", "Directory containing ePUB files (required)")
	cmd.Flags().StringVarP(&flags.pattern, "pattern", "p", "", "Search pattern (required)")

	// search options
	cmd.Flags().BoolVar(&flags.isRegex, "regex", false, "Treat pattern as regular expression")
	cmd.Flags().BoolVarP(&flags.ignoreCase, "ignore-case", "i", false, "Case-insensitive search (text mode only)")
	cmd.Flags().IntVarP(&flags.context, "context", "c", 0, "Number of context lines around each match")

	// performance options
	cmd.Flags().IntVarP(&flags.maxThreads, "threads", "t", runtime.NumCPU(), "Maximum number of worker threads")
	cmd.Flags().BoolVar(&flags.extractMetadata, "extract-metadata", false, "Extract and include metadata in results")

	// filter options
	cmd.Flags().StringVar(&flags.authorEquals, "author", "",
		"Filter by author (requires --extract-metadata; see --filter-match for matching behavior)")
	cmd.Flags().StringVar(&flags.seriesEquals, "series", "",
		"Filter by series (requires --extract-metadata; see --filter-match for matching behavior)")
	cmd.Flags().StringVar(&flags.titleEquals, "title", "",
		"Filter by title (requires --extract-metadata; see --filter-match for matching behavior)")
	cmd.Flags().StringVar(&flags.filterMatch, "filter-match", string(epubproc.FilterMatchExact),
		"Metadata filter matching mode: exact, word (requires --author, --series, or --title)")
	cmd.Flags().StringSliceVar(&flags.filesIn, "files-in", nil, "Filter to specific ePUB files")

	// output options
	cmd.Flags().BoolVar(&flags.pretty, "pretty", false, "Pretty-print JSON output")

	// logging options
	cmd.Flags().StringVar(&flags.logLevel, "log-level", "warn", "Set logging level (disabled, error, warn, info, debug, trace)")

	// enforce required flags
	if err := cmd.MarkFlagRequired("directory"); err != nil {
		log.Err(err).Msg("failed to mark directory flag as required")
	}
	if err := cmd.MarkFlagRequired("pattern"); err != nil {
		log.Err(err).Msg("failed to mark pattern flag as required")
	}
}

func runSearch(ctx context.Context, flags *searchFlags) error {
	configureLogging(flags.logLevel)

	if (flags.authorEquals != "" || flags.seriesEquals != "" || flags.titleEquals != "") && !flags.extractMetadata {
		return fmt.Errorf("metadata filters (--author, --series, --title) require --extract-metadata")
	}

	matchMode := epubproc.FilterMatchMode(flags.filterMatch)
	if matchMode != epubproc.FilterMatchExact && matchMode != epubproc.FilterMatchWord {
		return fmt.Errorf("invalid --filter-match value %q: must be 'exact' or 'word'", flags.filterMatch)
	}

	if matchMode != epubproc.FilterMatchExact &&
		flags.authorEquals == "" && flags.seriesEquals == "" && flags.titleEquals == "" {
		return fmt.Errorf("--filter-match=%s requires at least one of --author, --series, --title", flags.filterMatch)
	}

	if _, err := os.Stat(flags.epubDir); os.IsNotExist(err) {
		return fmt.Errorf("directory does not exist: %s", flags.epubDir)
	}

	request := buildSearchRequest(flags)
	fileSearch := epubproc.NewFileSearch(flags.epubDir, flags.maxThreads, flags.extractMetadata)

	startedAt := time.Now()
	log.Debug().
		Str("directory", flags.epubDir).
		Str("pattern", flags.pattern).
		Bool("regex", flags.isRegex).
		Bool("extract_metadata", flags.extractMetadata).
		Int("max_threads", flags.maxThreads).
		Msg("starting ePUB search")

	// collect results with pre-allocated capacity for improved performance
	results := make([]searchResult, 0, 16)
	var totalMatches int
	var mu sync.Mutex

	if err := fileSearch.Search(ctx, request, func(result *epubproc.SearchResult) error {
		searchRes := searchResult{
			Path:    result.Path,
			Matches: result.Matches,
		}

		if flags.extractMetadata {
			searchRes.Metadata = &result.Metadata
		}

		mu.Lock()
		results = append(results, searchRes)
		totalMatches += len(result.Matches)
		mu.Unlock()

		return nil
	}); err != nil {
		return fmt.Errorf("search failed: %w", err)
	}

	log.Debug().
		Int("files_with_matches", len(results)).
		Int("total_matches", totalMatches).
		Str("duration", time.Since(startedAt).String()).
		Msg("ePUB search completed")

	output := searchOutput{
		Results: results,
		Summary: summaryInfo{
			TotalFiles:   len(results),
			TotalMatches: totalMatches,
		},
	}
	return outputJSON(output, flags.pretty)
}

func outputJSON(output searchOutput, pretty bool) error {
	var jsonData []byte
	var err error

	if pretty {
		jsonData, err = json.MarshalIndent(output, "", "  ")
	} else {
		jsonData, err = json.Marshal(output)
	}

	if err != nil {
		return fmt.Errorf("failed to marshal JSON output: %w", err)
	}

	fmt.Println(string(jsonData))
	return nil
}

func buildSearchRequest(flags *searchFlags) *epubproc.SearchRequest {
	request := &epubproc.SearchRequest{
		Context: flags.context,
	}

	if flags.isRegex {
		request.Query = epubproc.SearchRequestQuery{
			Regex: &epubproc.SearchRequestRegex{
				Pattern: flags.pattern,
			},
		}
	} else {
		request.Query = epubproc.SearchRequestQuery{
			Text: &epubproc.SearchRequestText{
				Value:      flags.pattern,
				IgnoreCase: flags.ignoreCase,
			},
		}
	}

	if flags.authorEquals != "" || flags.seriesEquals != "" || flags.titleEquals != "" || len(flags.filesIn) > 0 {
		request.Filters = &epubproc.SearchRequestFilters{
			AuthorEquals: flags.authorEquals,
			SeriesEquals: flags.seriesEquals,
			TitleEquals:  flags.titleEquals,
			FilesIn:      flags.filesIn,
			MatchMode:    epubproc.FilterMatchMode(flags.filterMatch),
		}
	}

	return request
}

func configureLogging(level string) {
	level = strings.ToLower(level)

	if level == "disabled" {
		zerolog.SetGlobalLevel(zerolog.Disabled)
		return
	}

	// use a standard error console writer to keep the command output processable
	log.Logger = log.Output(zerolog.ConsoleWriter{
		Out:        os.Stderr,
		TimeFormat: "15:04:05",
	})

	switch level {
	case "trace":
		zerolog.SetGlobalLevel(zerolog.TraceLevel)
	case "debug":
		zerolog.SetGlobalLevel(zerolog.DebugLevel)
	case "info":
		zerolog.SetGlobalLevel(zerolog.InfoLevel)
	case "warn", "warning":
		zerolog.SetGlobalLevel(zerolog.WarnLevel)
	case "error":
		zerolog.SetGlobalLevel(zerolog.ErrorLevel)
	default:
		zerolog.SetGlobalLevel(zerolog.WarnLevel)
		log.Warn().Str("log_level", level).Msg("unknown log level - falling back to WARN")
	}
}
