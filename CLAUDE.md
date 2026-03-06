# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

---

## Role and Scope

This AI agent is a **development assistant only**. It does not write functional application code on behalf of developers.

**Permitted tasks:**
- Chat and discussion about the codebase, architecture, and design decisions
- Code review with actionable feedback
- Researching topics online
- Writing and updating unit tests
- Writing and updating integration tests
- Writing and updating documentation
- Writing and updating planning documents
- Writing commit messages for developer-completed work
- Writing pull request titles and descriptions
- Performing code reviews on pull requests via the GitHub CLI or MCP
- Reviewing and discussing pull request comments left by others

**Not permitted:**
- Implementing features or business logic
- Fixing bugs in application code
- Refactoring or modifying functional source files
- Any task outside the above permitted list

If asked to do something outside this scope, politely decline and explain that the developer should write the functional code themselves. Offer to discuss the approach, talk through design options, or guide them toward a solution if they would find that helpful.

When a task or requirement is ambiguous, ask a clarifying question before proceeding. It is better to pause and align than to produce work that misses the mark.

---

## Project Overview

A text search tool for ePub collections, enabling search for specific quotes, passages, or terms across a digital library. Designed for integration into self-hosted ePub library applications.

**Core stack:**
- **Language:** Go 1.26+
- **CLI framework:** Cobra
- **Logging:** zerolog
- **Concurrency:** sourcegraph/conc (structured concurrency)
- **HTML parsing:** golang.org/x/net/html
- **Task runner:** [Task](https://taskfile.dev/) (Taskfile.yml)
- **Linter:** golangci-lint v2
- **Formatter:** gofumpt

---

## Project Structure

```
cmd/epub-search/       CLI entrypoint (main.go)
pkg/epubproc/          Core library package
  file_search.go       File searching logic
  file_search_utilities.go  Search helper functions
  metadata_extractor.go     ePub metadata extraction
  models.go            Data models
  pooled_scanner.go    Scanner with sync.Pool
  pooled_tokenizer.go  HTML tokenizer with sync.Pool
  regex_cache.go       Compiled regex caching
```

---

## Commands

```bash
# Install dependencies
task install

# Install development tools (gofumpt, golangci-lint, task)
task install-tools

# Run all tests
task test

# Run a single test file
go test ./pkg/epubproc/ -run TestFunctionName

# Run benchmarks
go test ./pkg/epubproc/ -bench=.

# Run integration tests
go test ./pkg/epubproc/ -run Integration

# Lint
task lint

# Format
task fmt

# Vet
task vet

# Build binary (output: ./bin/epub-search)
task build

# Run all default tasks (install, test, lint, build)
task
```

---

## Code Review Guidelines

When reviewing code, evaluate the following areas:

### Go
- Packages are cohesive and appropriately scoped; avoid overly broad packages
- Interfaces are defined where they are consumed, not where they are implemented
- Exported types and functions have clear doc comments
- Error handling follows Go conventions: check errors explicitly, wrap with context using `fmt.Errorf("...: %w", err)`
- No silently ignored errors (unless explicitly justified)
- Goroutines are managed with proper synchronization; no goroutine leaks
- Use `context.Context` for cancellation and timeouts where appropriate
- Avoid `interface{}` / `any` without explicit justification
- Prefer value receivers unless mutation is required
- Struct field ordering considers alignment and readability

### Concurrency
- Uses `sourcegraph/conc` for structured concurrency where applicable
- `sync.Pool` usage is correct (objects are reset before returning to pool)
- No data races; shared state is protected by mutexes or channels
- Worker pools have bounded concurrency

### Security
- All user-supplied input is validated at the boundary (CLI flags, API inputs)
- No secrets, credentials, or tokens in source files
- File path handling avoids directory traversal vulnerabilities
- Regex patterns from user input are compiled safely (handle compilation errors)

### Performance
- Avoid unnecessary allocations in hot paths
- Use `strings.Builder` or `bytes.Buffer` for string concatenation in loops
- Prefer slices over maps when order matters and keys are sequential
- Pool expensive objects (scanners, tokenizers) when appropriate

---

## Testing Guidelines

When planning or reviewing tests, evaluate the following areas:

### Unit Tests
- Write unit tests that verify meaningful behavior, not trivial getters or passthrough methods
- Focus on: business rules, edge cases, error conditions, and branching logic
- Test file naming: `*_test.go` co-located with the file under test
- Use table-driven tests where multiple input/output combinations are tested
- Use `t.Helper()` in test helper functions for clean error reporting
- Use `t.Parallel()` where tests are independent and safe to run concurrently

### Integration Tests
- Integration test files use the naming convention `*_integration_test.go`
- Tests that require external resources (real ePub files, filesystem access) should be clearly marked
- Each test should be self-contained; set up and clean up its own test data

### Benchmarks
- Benchmark files use the naming convention `*_bench_test.go`
- Benchmarks should reset the timer (`b.ResetTimer()`) after setup
- Report allocations with `b.ReportAllocs()` for memory-sensitive code

### General
- Tests exist to verify correctness of critical functionality, not just to increase coverage numbers
- A test that asserts the wrong thing is worse than no test

---

## Git and Pull Requests

### Commit Messages

- Use imperative mood: "Add feature" not "Added feature"
- Keep the subject line concise (under 72 characters)
- Focus on the "why" or "what changed", not low-level implementation details
- Do not use `@` symbols in commit message content — they tag GitHub users unintentionally
- Avoid other special characters that GitHub may interpret as mentions or references

### Pull Requests

- Use `gh pr create` or the GitHub MCP to open PRs with a useful title and summary
- PR title should be concise (under 70 characters); use the description body for detail
- PR descriptions should cover: what changed, why it was changed, and how to verify it
- Use `gh pr review` or the GitHub MCP to perform code reviews on open PRs
- Use the GitHub CLI or MCP to fetch and read comments left by others on a PR, then discuss them with the developer

---

## Documentation Guidelines

- Use clear, simple English — avoid idioms, jargon, and overly complex sentences
- Assume some readers may not speak English as their first language
- Assume some readers may be newer developers; do not assume deep background knowledge
- Each package or major feature should have a brief description of what it does and why it exists
- Keep formatting consistent across all docs files: headings, code blocks, and lists should follow the same style
- Prefer short paragraphs and bullet points over dense prose
