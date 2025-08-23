package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"runtime"
	"strings"
	"time"

	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"
	"github.com/spf13/cobra"

	"github.com/jfenske89/go-epub-grep/pkg/epubproc"
)

// searchFlags holds command-line flags for the search command
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
	filesIn         []string
	pretty          bool
	logLevel        string
}

// searchOutput represents search output in JSON format
type searchOutput struct {
	Results []searchResult `json:"results"`
	Summary summaryInfo    `json:"summary"`
}

// searchResult represents a search result with metadata and matches
type searchResult struct {
	Path     string             `json:"path"`
	Metadata *epubproc.Metadata `json:"metadata,omitempty"`
	Matches  []epubproc.Match   `json:"matches"`
}

// summaryInfo provides search result summary
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

// createRootCmd creates the root command with flags
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

// createSearchCmd creates the search command with flags
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

// setupSearchFlags configures flags for the search command
func setupSearchFlags(cmd *cobra.Command, flags *searchFlags) {
	// required flags
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
	cmd.Flags().StringVar(&flags.authorEquals, "author", "", "Filter by author (requires --extract-metadata)")
	cmd.Flags().StringVar(&flags.seriesEquals, "series", "", "Filter by series (requires --extract-metadata)")
	cmd.Flags().StringVar(&flags.titleEquals, "title", "", "Filter by title (requires --extract-metadata)")
	cmd.Flags().StringSliceVar(&flags.filesIn, "files-in", nil, "Filter to specific ePUB files")

	// output options
	cmd.Flags().BoolVar(&flags.pretty, "pretty", false, "Pretty-print JSON output")

	// logging options
	cmd.Flags().StringVar(&flags.logLevel, "log-level", "warn", "Set logging level (disabled, error, warn, info, debug, trace)")

	// required flags
	if err := cmd.MarkFlagRequired("directory"); err != nil {
		log.Err(err).Msg("failed to mark directory flag as required")
	} else if err := cmd.MarkFlagRequired("pattern"); err != nil {
		log.Err(err).Msg("failed to mark pattern flag as required")
	}
}

// runSearch executes the search command with the provided flags
func runSearch(ctx context.Context, flags *searchFlags) error {
	// configure logging
	configureLogging(flags.logLevel)

	// validate that metadata extraction is enabled when using metadata filters
	if (flags.authorEquals != "" || flags.seriesEquals != "" || flags.titleEquals != "") && !flags.extractMetadata {
		return fmt.Errorf("metadata filters (--author, --series, --title) require --extract-metadata")
	}

	// validate directory exists
	if _, err := os.Stat(flags.epubDir); os.IsNotExist(err) {
		return fmt.Errorf("directory does not exist: %s", flags.epubDir)
	}

	// build search request
	request := buildSearchRequest(flags)

	// create a file search instance
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

	if err := fileSearch.Search(ctx, request, func(result *epubproc.SearchResult) error {
		searchRes := searchResult{
			Path:    result.Path,
			Matches: result.Matches,
		}

		if flags.extractMetadata {
			searchRes.Metadata = &result.Metadata
		}

		results = append(results, searchRes)
		totalMatches += len(result.Matches)
		return nil
	}); err != nil {
		return fmt.Errorf("search failed: %w", err)
	}

	log.Debug().
		Int("files_with_matches", len(results)).
		Int("total_matches", totalMatches).
		Str("duation", time.Since(startedAt).String()).
		Msg("ePUB search completed")

	// process results and write output
	output := searchOutput{
		Results: results,
		Summary: summaryInfo{
			TotalFiles:   len(results),
			TotalMatches: totalMatches,
		},
	}
	return outputJSON(output, flags.pretty)
}

// outputJSON marshals and outputs the search results as JSON
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

// buildSearchRequest constructs a SearchRequest from command-line flags
func buildSearchRequest(flags *searchFlags) *epubproc.SearchRequest {
	request := &epubproc.SearchRequest{
		Context: flags.context,
	}

	// configure search query as regex or plain text
	if flags.isRegex {
		request.Query = epubproc.SearchRequestQuery{
			IsRegex: true,
			Regex: &epubproc.SearchRequestRegex{
				Pattern: flags.pattern,
			},
		}
	} else {
		request.Query = epubproc.SearchRequestQuery{
			IsRegex: false,
			Text: &epubproc.SearchRequestText{
				Value:      flags.pattern,
				IgnoreCase: flags.ignoreCase,
			},
		}
	}

	// configure filters
	if flags.authorEquals != "" || flags.seriesEquals != "" || flags.titleEquals != "" || len(flags.filesIn) > 0 {
		request.Filters = &epubproc.SearchRequestFilters{
			AuthorEquals: flags.authorEquals,
			SeriesEquals: flags.seriesEquals,
			TitleEquals:  flags.titleEquals,
			FilesIn:      flags.filesIn,
		}
	}

	return request
}

// configureLogging sets up zerolog based on the specified level
func configureLogging(level string) {
	level = strings.ToLower(level)

	if level == "disabled" {
		// disable logging
		zerolog.SetGlobalLevel(zerolog.Disabled)
		return
	}

	// use a standard error console writer to keep the command output processable
	log.Logger = log.Output(zerolog.ConsoleWriter{
		Out:        os.Stderr,
		TimeFormat: "15:04:05",
	})

	// set log level
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
