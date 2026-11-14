#!/bin/bash

# Smoke test for the built CLI. Tests plain text search, regex search, and search with filters
# against public domain ePUBs and checks the results, not only the exit code.
# Needs jq, ./bin/epub-search (task build) and a ./public-domain directory (not committed).

set -euo pipefail  # Exit on error, undefined vars, pipe failures

# --- Configuration ---
readonly SCRIPT_NAME="$(basename "$0")"
readonly DEFAULT_EPUB_DIR="./public-domain"
readonly CLI_BINARY="./bin/epub-search"

# Test queries
readonly TEXT_QUERY="Holmes"
readonly REGEX_QUERY="Holmes|Watson"
readonly FILTER_AUTHOR="Arthur Conan Doyle"
readonly FILTER_TITLE="A Study in Scarlet"
readonly NO_MATCH_QUERY="zzqxj-no-such-text-9731"

# Set by test_metadata_extraction, read by test_filtered_search
UNFILTERED_FILES=0

# --- Functions ---

# Format duration in a human-friendly way with millisecond precision
format_duration() {
    local total_ms="$1"
    
    if [[ $total_ms -lt 1000 ]]; then
        echo "${total_ms}ms"
    elif [[ $total_ms -lt 60000 ]]; then
        local seconds=$((total_ms / 1000))
        local ms=$((total_ms % 1000))
        printf "%d.%03ds" "$seconds" "$ms"
    else
        local minutes=$((total_ms / 60000))
        local seconds=$(((total_ms % 60000) / 1000))
        local ms=$((total_ms % 1000))
        if [[ $ms -gt 0 ]]; then
            printf "%dm %d.%03ds" "$minutes" "$seconds" "$ms"
        else
            printf "%dm %ds" "$minutes" "$seconds"
        fi
    fi
}

# Get current time in milliseconds (works on macOS and Linux)
get_time_ms() {
    if [[ "$OSTYPE" == "darwin"* ]]; then
        # macOS - use perl for millisecond precision
        perl -MTime::HiRes=time -e 'printf "%.0f\n", time * 1000'
    else
        # Linux - use date with nanoseconds
        echo $(($(date +%s%N) / 1000000))
    fi
}

show_usage() {
    cat << EOF
Usage: $SCRIPT_NAME [OPTIONS]

End-to-end test for go-epub-grep CLI that performs plain text search, regex search, and filtered search.

OPTIONS:
    -d, --directory DIR     Directory containing ePUB files (default: $DEFAULT_EPUB_DIR)
    -b, --binary PATH       Path to epub-search binary (default: $CLI_BINARY)
    -v, --verbose           Show detailed JSON output from search commands
    -h, --help              Show this help message

EXAMPLES:
    $SCRIPT_NAME
    $SCRIPT_NAME --directory /path/to/epubs
    $SCRIPT_NAME --binary /usr/local/bin/epub-search
    $SCRIPT_NAME --verbose  # Show full JSON output

EOF
}

log_info() {
    echo "ℹ️  $*" >&2
}

log_success() {
    echo "✅ $*" >&2
}

log_error() {
    echo "❌ $*" >&2
}

log_step() {
    echo ""
    echo "🔄 [$1] $2"
}

log_separator() {
    echo ""
    echo "═══════════════════════════════════════════════════════════════════════════════"
    echo ""
}

check_dependencies() {
    local missing_deps=()
    
    for cmd in jq; do
        if ! command -v "$cmd" &> /dev/null; then
            missing_deps+=("$cmd")
        fi
    done
    
    if [[ ${#missing_deps[@]} -gt 0 ]]; then
        log_error "Missing required dependencies: ${missing_deps[*]}"
        log_error "Please install them and try again."
        exit 1
    fi
}

validate_setup() {
    log_step "SETUP" "Validating test environment"
    
    # Check if CLI binary exists
    if [[ ! -f "$CLI_BIN" ]]; then
        log_error "CLI binary not found: $CLI_BIN"
        log_error "Please build the project first: task build"
        exit 1
    fi
    
    # Check if CLI binary is executable
    if [[ ! -x "$CLI_BIN" ]]; then
        log_error "CLI binary is not executable: $CLI_BIN"
        exit 1
    fi
    
    # Check if ePUB directory exists
    if [[ ! -d "$EPUB_DIR" ]]; then
        log_error "ePUB directory not found: $EPUB_DIR"
        exit 1
    fi
    
    # Check if there are ePUB files
    local epub_count
    epub_count=$(find "$EPUB_DIR" -name "*.epub" | wc -l)
    
    if [[ "$epub_count" -eq 0 ]]; then
        log_error "No ePUB files found in directory: $EPUB_DIR"
        exit 1
    fi
    
    log_success "Environment validation passed"
    log_info "CLI Binary: $CLI_BIN"
    log_info "ePUB Directory: $EPUB_DIR"
    log_info "ePUB Files Found: $epub_count"
}

# Output of the last successful run_cli_command, used by the assert_* helpers.
LAST_OUTPUT=""

# Fail unless a jq filter is true for the last output. Extra args go to jq (for example --arg).
assert_jq() {
    local description="$1"
    local filter="$2"
    shift 2

    if ! jq -e "$@" "$filter" <<< "$LAST_OUTPUT" > /dev/null; then
        log_error "Assertion failed: $description"
        log_error "jq filter: $filter"
        jq -c '.summary' <<< "$LAST_OUTPUT" >&2 || true
        return 1
    fi
    log_success "Assertion passed: $description"
}

run_cli_command() {
    local description="$1"
    shift
    local cmd_args=("$@")
    
    log_info "Running: $CLI_BIN ${cmd_args[*]}"
    
    local output
    local exit_code=0
    local start_time
    local end_time
    
    # Capture start time
    start_time=$(get_time_ms)
    
    # Run command and capture output and exit code
    output=$("$CLI_BIN" "${cmd_args[@]}" 2>&1) || exit_code=$?
    
    # Capture end time and calculate duration
    end_time=$(get_time_ms)
    local duration_ms=$((end_time - start_time))
    local formatted_duration=$(format_duration "$duration_ms")
    
    if [[ $exit_code -ne 0 ]]; then
        log_error "$description failed (exit code: $exit_code)"
        echo "$output"
        return 1
    fi
    
    # Validate JSON output
    if ! echo "$output" | jq . > /dev/null 2>&1; then
        log_error "$description produced invalid JSON"
        echo "$output"
        return 1
    fi
    
    log_success "$description completed successfully (${formatted_duration})"
    LAST_OUTPUT="$output"
    
    # Show JSON output only in verbose mode
    if [[ "${VERBOSE:-false}" == "true" ]]; then
        echo "$output" | jq .
    else
        # Show just a summary of results
        local result_count
        result_count=$(echo "$output" | jq -r '.summary.totalFiles' 2>/dev/null || echo "0")
        local match_count
        match_count=$(echo "$output" | jq -r '.summary.totalMatches' 2>/dev/null || echo "0")
        log_info "Found $match_count matches in $result_count files"
    fi
    
    return 0
}

test_plain_text_search() {
    log_step "1/4" "Testing plain text search"
    log_info "Searching for: '$TEXT_QUERY'"
    
    run_cli_command "Plain text search" \
        search \
        --directory "$EPUB_DIR" \
        --pattern "$TEXT_QUERY" \
        --ignore-case \
        --context 1 \
        --pretty
    assert_jq "plain text search finds matches" '.summary.totalMatches > 0 and .summary.totalFiles > 0'
    assert_jq "every result has matches and a path" 'all(.results[]; (.matches | length) > 0 and .path != "")'
    assert_jq "match text contains the query" \
        'all(.results[].matches[]; .line | ascii_downcase | contains($q | ascii_downcase))' --arg q "$TEXT_QUERY"
    
    log_info "Searching for text that is not in any book"
    run_cli_command "No-match search" \
        search \
        --directory "$EPUB_DIR" \
        --pattern "$NO_MATCH_QUERY"
    assert_jq "nonsense query finds nothing" '.summary.totalMatches == 0 and .summary.totalFiles == 0'
    
    log_success "Plain text search test passed"
}

test_regex_search() {
    log_step "2/4" "Testing regex search"
    log_info "Searching for regex: '$REGEX_QUERY'"
    
    run_cli_command "Regex search" \
        search \
        --directory "$EPUB_DIR" \
        --pattern "$REGEX_QUERY" \
        --regex \
        --context 2 \
        --pretty
    assert_jq "regex search finds matches" '.summary.totalMatches > 0 and .summary.totalFiles > 0'
    assert_jq "every match contains one of the alternatives" \
        'all(.results[].matches[]; .line | test("Holmes|Watson"))'
    
    log_success "Regex search test passed"
}

test_metadata_extraction() {
    log_step "3/4" "Testing metadata extraction"
    log_info "Searching with metadata extraction enabled"
    
    run_cli_command "Metadata extraction" \
        search \
        --directory "$EPUB_DIR" \
        --pattern "$TEXT_QUERY" \
        --extract-metadata \
        --context 1 \
        --pretty
    assert_jq "metadata search finds matches" '.summary.totalMatches > 0'
    assert_jq "every result has a title and authors" \
        'all(.results[]; (.metadata.title // "") != "" and (.metadata.authors | length) > 0)'
    
    # remember the unfiltered file count so the filter tests can check that filtering narrows the result
    UNFILTERED_FILES=$(jq -r '.summary.totalFiles' <<< "$LAST_OUTPUT")
    
    log_success "Metadata extraction test passed"
}

test_filtered_search() {
    log_step "4/4" "Testing filtered search"
    log_info "Searching with author filter: '$FILTER_AUTHOR'"
    
    # Test author filter
    run_cli_command "Author filter search" \
        search \
        --directory "$EPUB_DIR" \
        --pattern "$TEXT_QUERY" \
        --extract-metadata \
        --author "$FILTER_AUTHOR" \
        --context 1 \
        --pretty
    assert_jq "author filter returns results" '.summary.totalFiles > 0'
    assert_jq "author filter returns only that author" \
        'all(.results[]; .metadata.authors | any(ascii_downcase == ($a | ascii_downcase)))' --arg a "$FILTER_AUTHOR"
    assert_jq "author filter returns fewer files than the unfiltered search" \
        '.summary.totalFiles < ($n | tonumber)' --arg n "$UNFILTERED_FILES"
    
    log_info "Testing title filter: '$FILTER_TITLE'"
    
    # Test title filter
    run_cli_command "Title filter search" \
        search \
        --directory "$EPUB_DIR" \
        --pattern "Sherlock" \
        --extract-metadata \
        --title "$FILTER_TITLE" \
        --context 1 \
        --pretty
    assert_jq "title filter returns results" '.summary.totalFiles > 0'
    assert_jq "title filter returns only that title" \
        'all(.results[]; .metadata.title | ascii_downcase == ($t | ascii_downcase))' --arg t "$FILTER_TITLE"
    
    # --files-in needs paths that equal the walked paths, so use an absolute directory for this check
    local abs_dir
    abs_dir=$(cd "$EPUB_DIR" && pwd)
    local specific_files=(
        "$abs_dir/A Study in Scarlet - Arthur Conan Doyle.epub"
        "$abs_dir/The Adventures of Sherlock Holmes - Arthur Conan Doyle.epub"
    )
    
    log_info "Testing files-in filter with specific files"
    
    run_cli_command "Files-in filter search" \
        search \
        --directory "$abs_dir" \
        --pattern "$TEXT_QUERY" \
        --files-in "${specific_files[0]},${specific_files[1]}" \
        --context 1 \
        --pretty
    assert_jq "files-in returns exactly the two requested files" \
        '.summary.totalFiles == 2 and ([.results[].path] | sort) == ($f | sort)' \
        --argjson f "$(printf '%s\n' "${specific_files[@]}" | jq -R . | jq -s .)"
    
    log_success "Filtered search test passed"
}

run_performance_test() {
    log_step "PERF" "Running performance test"
    log_info "Testing with multiple threads and larger context"
    
    local start_time
    start_time=$(get_time_ms)
    
    run_cli_command "Performance test" \
        search \
        --directory "$EPUB_DIR" \
        --pattern "the" \
        --ignore-case \
        --threads 8 \
        --context 3 \
        --extract-metadata \
        --pretty
    
    local end_time
    end_time=$(get_time_ms)
    local duration_ms=$((end_time - start_time))
    local formatted_duration=$(format_duration "$duration_ms")
    
    log_success "Performance test completed in ${formatted_duration}"
}

test_error_handling() {
    log_step "ERROR" "Testing error handling"
    
    # Test missing required flags
    log_info "Testing missing required flags..."
    if "$CLI_BIN" search >/dev/null 2>&1; then
        log_error "Expected error for missing required flags, but command succeeded"
        return 1
    fi
    log_success "Missing required flags properly rejected"
    
    # Test non-existent directory
    log_info "Testing non-existent directory..."
    if "$CLI_BIN" search --directory "/non/existent/path" --pattern "test" >/dev/null 2>&1; then
        log_error "Expected error for non-existent directory, but command succeeded"
        return 1
    fi
    log_success "Non-existent directory properly rejected"
    
    # Test invalid regex
    log_info "Testing invalid regex pattern..."
    if "$CLI_BIN" search --directory "$EPUB_DIR" --pattern "[invalid" --regex >/dev/null 2>&1; then
        log_error "Expected error for invalid regex, but command succeeded"
        return 1
    fi
    log_success "Invalid regex properly rejected"
    
    # Test negative context
    log_info "Testing negative context..."
    if "$CLI_BIN" search --directory "$EPUB_DIR" --pattern "test" --context -1 >/dev/null 2>&1; then
        log_error "Expected error for negative context, but command succeeded"
        return 1
    fi
    log_success "Negative context properly rejected"
    
    log_success "Error handling test passed"
}

show_test_summary() {
    local total_duration_ms="$1"
    local formatted_total=$(format_duration "$total_duration_ms")
    
    log_separator
    log_success "🎉 All CLI tests completed successfully!"
    echo ""
    echo "Tests performed:"
    echo "  ✅ Plain text search with case-insensitive matching"
    echo "  ✅ Regex pattern search"
    echo "  ✅ Metadata extraction"
    echo "  ✅ Author-based filtering"
    echo "  ✅ Title-based filtering"
    echo "  ✅ File-based filtering"
    echo "  ✅ Performance test with multiple threads"
    echo "  ✅ Error handling validation"
    echo ""
    log_info "Total test execution time: ${formatted_total}"
    log_info "The epub-search CLI tool is working correctly!"
    log_separator
}

# --- Main Script ---

main() {
    local EPUB_DIR="$DEFAULT_EPUB_DIR"
    local CLI_BIN="$CLI_BINARY"
    export VERBOSE="false"
    
    # Parse command line arguments
    while [[ $# -gt 0 ]]; do
        case $1 in
            -d|--directory)
                EPUB_DIR="$2"
                shift 2
                ;;
            -b|--binary)
                CLI_BIN="$2"
                shift 2
                ;;
            -v|--verbose)
                export VERBOSE="true"
                shift
                ;;
            -h|--help)
                show_usage
                exit 0
                ;;
            *)
                log_error "Unknown option: $1"
                show_usage
                exit 1
                ;;
        esac
    done
    
    # Check dependencies
    check_dependencies
    
    # Main test flow
    echo "🚀 Starting go-epub-grep CLI End-to-End Test"
    echo "📁 ePUB Directory: $EPUB_DIR"
    echo "🔧 CLI Binary: $CLI_BIN"
    
    # Track total execution time
    local test_start_time
    test_start_time=$(get_time_ms)
    
    validate_setup
    
    # Run all tests
    test_plain_text_search
    test_regex_search
    test_metadata_extraction
    test_filtered_search
    run_performance_test
    test_error_handling
    
    # Calculate total execution time
    local test_end_time
    test_end_time=$(get_time_ms)
    local total_duration_ms=$((test_end_time - test_start_time))
    
    show_test_summary "$total_duration_ms"
}

# Execute main function with all arguments
main "$@"