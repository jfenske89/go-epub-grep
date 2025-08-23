# CLI Integration Guide

This guide demonstrates how to integrate the `epub-search` CLI tool into existing projects.

## Overview

The `epub-search` CLI tool is designed for cross-platform integration through:
- **JSON Output**: Structured, parseable results in JSON format
- **Exit Codes**: Standard Unix exit codes for success/failure detection
- **Configurable Parameters**: Configurable search and filtering behavior
- **Error Handling**: Clear error messages and appropriate exit codes

## Basic Integration Pattern

### Command Structure

```bash
epub-search search [OPTIONS]
```

### Required Parameters

- `--directory` (`-d`): Path to directory containing ePUB files
- `--pattern` (`-p`): Search pattern (text or regex)

### Common Integration Workflow

1. **Execute Command**: Run `epub-search` with appropriate parameters
2. **Parse JSON Output**: Process the structured JSON response
3. **Handle Errors**: Check exit codes and parse error messages
4. **Present Results**: Transform and display results to users

## Integration Examples by Language

### .NET (C#)

**Core Libraries**: `System.Diagnostics`, `System.Text.Json`

Use `ProcessStartInfo` to execute the CLI and `JsonSerializer` to parse responses. Handle async execution with `Process.WaitForExitAsync()` and implement proper error handling with exit codes.

### Java (Spring Boot)  

**Core Libraries**: `ProcessBuilder`, `com.fasterxml.jackson.databind.ObjectMapper`

Build commands with `ProcessBuilder`, capture stdout/stderr with `BufferedReader`, and deserialize JSON responses with Jackson. Set appropriate timeouts with `Process.waitFor()`.

### Python (Django/Flask)

**Core Libraries**: `subprocess`, `json`, `dataclasses`

Execute CLI with `subprocess.run()`, use dataclasses for type-safe response models, and implement proper timeout/error handling. Parse JSON responses directly into Python objects.

### Node.js (Express)

**Core Libraries**: `child_process`, built-in `JSON`

Use `spawn()` from child_process for CLI execution, handle async data streams with event listeners, and implement timeout logic with `setTimeout()`. Parse JSON responses with native JSON methods.

## JSON Response Format

All CLI commands return a consistent JSON structure:

```json
{
  "results": [
    {
      "path": "example-library/Romeo and Juliet - Shakespeare, William.epub",
      "metadata": {
        "title": "Romeo and Juliet",
        "authors": ["William Shakespeare"],
        "genres": [
          "Vendetta -- Drama",
          "Youth -- Drama",
          "Verona (Italy) -- Drama",
          "Juliet (Fictitious character) -- Drama",
          "Romeo (Fictitious character) -- Drama",
          "Conflict of generations -- Drama",
          "Tragedies (Drama)"
        ],
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
        },
        {
          "line": "The fearful passage of their death-mark'd love,",
          "fileName": "OEBPS/717674059043090192_1513-h-2.htm.xhtml"
        }
      ]
    }
  ],
  "summary": {
    "totalFiles": 1,
    "totalMatches": 2
  }
}
```

### Field Descriptions

- **`results`**: Array of search results, one per ePUB file with matches
- **`path`**: Full path to the ePUB file (relative or absolute based on input)
- **`metadata`**: Book metadata (only present when `--extract-metadata` is used)
  - **`title`**: Book title extracted from ePUB metadata
  - **`authors`**: Array of author names
  - **`genres`**: Array of genre/subject classifications
  - **`series`**: Series name (empty string if not part of a series)
  - **`seriesPosition`**: Position within series (0 if not applicable)
  - **`yearReleased`**: Publication year extracted from metadata
  - **`identifiers`**: Map of identifier types to values (ISBN, ASIN, DOI, URI, etc.)
- **`matches`**: Array of text matches found in the book
- **`line`**: Text content (single line when context=0, multi-line with context)
- **`fileName`**: Internal file path within the ePUB archive (e.g., "OEBPS/chapter01.xhtml")
- **`summary`**: Search result statistics
  - **`totalFiles`**: Number of ePUB files that contained matches
  - **`totalMatches`**: Total number of individual text matches across all files

## Error Handling

### Exit Codes

- **0**: Success
- **1**: General error (invalid arguments, file not found, etc.)
- **2**: Search execution error

### Error Response Format

When an error occurs, the CLI writes to stderr and exits with a non-zero code. The error output includes both the error message and usage information:

```bash
$ epub-search search --directory /nonexistent --pattern "test"
Error: directory does not exist: /nonexistent
Usage:
  epub-search search [flags]

Flags:
      --author string      Filter by author (requires --extract-metadata)
  -c, --context int        Number of context lines around each match
  -d, --directory string   Directory containing ePUB files (required)
      --extract-metadata   Extract and include metadata in results
      --files-in strings   Filter to specific ePUB files
  -h, --help               help for search
  -i, --ignore-case        Case-insensitive search (text mode only)
  -p, --pattern string     Search pattern (required)
      --pretty             Pretty-print JSON output
      --regex              Treat pattern as regular expression
      --series string      Filter by series (requires --extract-metadata)
  -t, --threads int        Maximum number of worker threads (default 8)
      --title string       Filter by title (requires --extract-metadata)

$ echo $?
1
```

### Common Error Scenarios

1. **Missing Required Parameters**
   ```bash
   Error: required flag(s) "directory", "pattern" not set
   ```

2. **Invalid Directory**
   ```bash
   Error: directory does not exist: /path/to/directory
   ```

3. **Invalid Regex Pattern**
   ```bash
   Error: invalid pattern: error parsing regexp: missing closing ]: `[invalid`
   ```

4. **Metadata Filter Without Extraction**
   ```bash
   Error: metadata filters (--author, --series, --title) require --extract-metadata
   ```

## Performance Considerations

### Optimal Thread Configuration

```bash
# Use CPU core count for balanced performance
epub-search search -d /epubs -p "text" --threads $(nproc)

# For I/O heavy operations, consider more threads
epub-search search -d /epubs -p "text" --threads $(($(nproc) * 2))
```

### Large Collection Handling

For very large ePUB collections, consider:

1. **Batch Processing**: Process subsets of your collection
2. **Caching**: Cache search results for common queries
3. **Indexing**: Pre-process metadata for faster filtering
4. **Timeouts**: Set appropriate timeouts in your application

### Memory Usage

The CLI uses streaming output, so memory usage remains constant regardless of result set size. However, your application should handle large JSON responses appropriately.

## Deployment Considerations

### Binary Distribution

1. **Single Binary**: The CLI compiles to a single executable
2. **Cross-Platform**: Available for Linux, macOS, and Windows
3. **No Dependencies**: Self-contained with no external runtime requirements

### Container Integration

```dockerfile
# Dockerfile example
FROM alpine:latest
RUN apk add --no-cache ca-certificates
COPY epub-search /usr/local/bin/
VOLUME ["/epub-library"]
EXPOSE 8080
CMD ["your-app"]
```

### Security Considerations

1. **Input Validation**: Always validate and sanitize user inputs before passing to CLI
2. **Path Restrictions**: Ensure ePUB directory paths are within allowed boundaries
3. **Resource Limits**: Set appropriate timeouts and process limits
4. **User Permissions**: Run with minimal required permissions

## Testing Your Integration

### Unit Testing

```bash
# Test basic functionality
epub-search search --directory ./test-epubs --pattern "love" --pretty

# Test regex patterns
epub-search search --directory ./test-epubs --pattern "the.*?" --regex --context 1 --pretty

# Test metadata extraction
epub-search search --directory ./test-epubs --pattern "monster" --extract-metadata --ignore-case --pretty

# Test filtering with specific author
epub-search search --directory ./test-epubs --pattern "William.*Shakespeare" --regex --extract-metadata --author "William Shakespeare" --pretty

# Test performance with multiple threads
epub-search search --directory ./test-epubs --pattern "\\d{4}" --regex --threads 2 --pretty
```

### Real-World Testing Results

Based on testing with classic literature ePUBs, here are typical performance characteristics:

- **Romeo and Juliet** (132 instances of "love"): ~50ms search time
- **Frankenstein** (98 instances of "monster"): ~75ms search time  
- **Multi-file regex search** (4 files, year patterns): ~150ms search time
- **Metadata extraction** adds ~20-30ms overhead per file

### Expected Output Volumes

For reference, typical search results from classic literature:
- Single word searches: 50-200 matches per book
- Regex pattern searches: 10-100 matches per book
- Context lines significantly increase JSON output size (3x for context=2)

### Integration Testing

Create comprehensive tests that verify:
1. JSON parsing of various response sizes
2. Error handling for different failure scenarios
3. Timeout handling for long-running searches
4. Memory usage with large result sets
5. Concurrent request handling

This CLI integration approach allows any digital library system to add powerful ePUB search capabilities regardless of the underlying technology stack.