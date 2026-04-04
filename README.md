# go-epub-search

<p align="center">
  <img src="./logo.png" alt="Go ePUB grep" width="100" height="auto">
</p>

A tool to search the content of ePub files. Also includes a Go library for advanced integrations.

## Features

- Regular expression support
- Metadata support: title, authors, series, identifiers (ISBN, ASIN, DOI)
- High-performance multi-threaded processing for large collections
- Optionally filter results by author, title, series, or specific files
- Configurable context lines around matches for better readability
- Structured JSON output suitable for API integration and web applications

## Installation

### Using `go install`

```bash
go install github.com/jfenske89/go-epub-grep/cmd/epub-search@latest
```

This installs the `epub-search` binary to your `$GOPATH/bin` directory. Make sure that directory is in your `$PATH`.

### Using Docker

See the [Docker](#docker) section below.

## CLI Usage

### Basic Search

```bash
# Simple text search
epub-search search \
  -d /path/to/epubs \
  -p "search term"

# Case-insensitive search with context
epub-search search \
  -d /path/to/epubs \
  -p "Holmes" \
  --ignore-case \
  --context 2
```

### Regular Expression Search

```bash
# Regex pattern matching
epub-search search \
  -d /path/to/epubs \
  -p "Holmes|Watson" \
  --regex

# Complex pattern with case sensitivity
epub-search search \
  -d /path/to/epubs \
  -p "\b[A-Z][a-z]+\s+Holmes\b" \
  --regex \
  --context 1
```

### Metadata-Based Filtering

```bash
# Search with metadata extraction
epub-search search \
  -d /path/to/epubs \
  -p "mystery" \
  --extract-metadata

# Filter by specific author
epub-search search \
  -d /path/to/epubs \
  -p "detective" \
  --extract-metadata \
  --author "Arthur Conan Doyle"

# Filter by title and series
epub-search search \
  -d /path/to/epubs \
  -p "London" \
  --extract-metadata \
  --title "A Study in Scarlet"
```

### Performance Options

```bash
# Multi-threaded processing
epub-search search \
  -d /path/to/epubs \
  -p "text" \
  --threads 8

# Search specific files only
epub-search search \
  -d /path/to/epubs \
  -p "pattern" \
  --files-in "book1.epub,book2.epub"
```

### Command-Line Options

| Flag                 | Short | Description                                    | Required |
| -------------------- | ----- | ---------------------------------------------- | -------- |
| `--directory`        | `-d`  | Directory containing ePUB files                | ✓        |
| `--pattern`          | `-p`  | Search pattern (text or regex)                 | ✓        |
| `--regex`            |       | Treat pattern as regular expression            |          |
| `--ignore-case`      | `-i`  | Case-insensitive search (text mode only)       |          |
| `--context`          | `-c`  | Number of context lines around matches         |          |
| `--threads`          | `-t`  | Maximum worker threads (default: CPU cores)    |          |
| `--extract-metadata` |       | Extract and include metadata in results        |          |
| `--author`           |       | Filter by author (requires --extract-metadata) |          |
| `--series`           |       | Filter by series (requires --extract-metadata) |          |
| `--title`            |       | Filter by title (requires --extract-metadata)  |          |
| `--files-in`         |       | Filter to specific ePUB files                  |          |
| `--pretty`           |       | Pretty-print JSON output                       |          |

## Output Format

All commands output structured JSON. Example:

```json
{
  "results": [
    {
      "path": "example-library/Romeo and Juliet - Shakespeare, William.epub",
      "metadata": {
        "title": "Romeo and Juliet",
        "authors": ["William Shakespeare"],
        "genres": ["Vendetta -- Drama", "Youth -- Drama", "Tragedies (Drama)"],
        "series": "",
        "seriesPosition": 0,
        "yearReleased": 1998,
        "identifiers": {
          "uri": "http://www.gutenberg.org/1513"
        }
      },
      "matches": [
        {
          "line": "A pair of star-cross'd lovers take their life;",
          "fileName": "OEBPS/717674059043090192_1513-h-2.htm.xhtml"
        }
      ]
    }
  ],
  "summary": {
    "totalFiles": 1,
    "totalMatches": 132
  }
}
```

## Docker

### Building and Running with Docker

```bash
# Build the Docker image
docker build -t ghcr.io/jfenske89/go-epub-grep:latest .

# Run a search
docker run --rm -it -v $(pwd)/public-domain:/books \
      ghcr.io/jfenske89/go-epub-grep:latest \
       search -d /books \
      -p 'Holmes' \
      --ignore-case \
      --context 2 \
      --pretty
```

## Development

### Building and Testing

```bash
# Install development dependencies
task install-tools

# Run tests
task test

# Run benchmarks
task bench

# Lint code
task lint

# Format code
task fmt

# Build binary
task build
```

### Testing

The project includes end-to-end tests:

```bash
# Run the test suite with sample ePUBs
./test_cli.sh

# Run with custom ePUB directory
./test_cli.sh --directory /path/to/test/epubs

# Verbose output for debugging
./test_cli.sh --verbose
```

## Contributing

This project follows Go best practices and emphasizes:

- Clean, performant code
- Comprehensive documentation
- Concurrent-safe operations
- Memory-efficient processing
- Extensive test coverage
