# CLAUDE.md

Agent-facing file. Terse on purpose. Human docs: `README.md`, `docs/`.

## Role

Collaborator. Discuss, plan, review, change code, tests, config, docs when developer asks.

- Smallest change that solves task. No unrelated refactor. No unrequested features.
- Read code and existing tests first. Match style of file.
- Before saying done: run `task test`, `task lint`, `task vet`. No `task` (`command -v task`): run Go commands directly. Report failures as they are.
- Touch `go.mod`, `go.sum`, `Taskfile.yml`, lint config, `.claude/` only when task needs it. Say so.
- Go rules: load skill `epub-grep-go-conventions` (`.claude/skills/epub-grep-go-conventions/SKILL.md`) before writing, reviewing, planning Go. Existing code and this file win over skill.
- Ambiguous task: ask before proceeding.

## Git rules

- Stage, commit, push only when developer asks. One ask covers that one action only. Earlier approval never carries over.
- Never force push, never `--no-verify`, never rewrite history unless developer explicitly asks.
- No pull requests unless asked.
- Read-only git (`status`, `diff`, `log`, `show`) always fine.
- `.claude/settings.json` enforces these with permission prompts. Never edit it to get around them.

## Agents (`.claude/agents/`)

- `go-review-sec`: read-only security and robustness review
- `go-review-perf`: read-only concurrency and performance review, benchmark-backed
- `go-review-general`: read-only conventions and idiom review
- `go-review-tests`: read-only test quality and coverage review
- `go-test-writer`: writes and fixes tests. Edits only `*_test.go`, `testdata/`, `test_cli.sh`.

Review agents never edit files. Report is advice. Fix findings only when developer asks.

## Project

ePub text search tool. Finds quotes, passages, terms across ePub library. Built for embedding in self-hosted library apps.

Stack: Go 1.27+, Cobra, zerolog, sourcegraph/conc, golang.org/x/net/html, Task, golangci-lint v2, gofumpt.

```
cmd/epub-search/main.go     CLI entrypoint
pkg/epubproc/               core library
  file_search.go            search orchestration, workers
  file_search_utilities.go  scanning, HTML tokenizing, filters
  metadata_extractor.go     ePub metadata extraction
  limits.go                 size limits for untrusted input
  models.go                 data models
  pooled_scanner.go         scanner with sync.Pool
```

## Commands

```bash
task              # default: deps, test, fmt, lint, build, vuln
task test         # all tests
task bench        # benchmarks
task fmt          # gofumpt + prettier
task vet
task lint         # golangci-lint v2
task vuln         # govulncheck
task modernize    # modernize fixes, re-test
task build        # ./bin/epub-search
task deps         # tidy modules
task deps:update  # upgrade deps, test
go test ./pkg/epubproc/ -run TestName     # one test
go test ./pkg/epubproc/ -run Integration  # integration tests
```

## Tests

- Naming: `*_test.go` beside code, `*_integration_test.go`, `*_bench_test.go`.
- Details and rules: skill.

## Docs

- Code comments explain why, not what. Keep type and function godoc. No field or line comments that restate names.
- Rules below apply to `README.md` and `docs/` only: simple English, short paragraphs, bullets, consistent headings, no idioms. Readers may be new developers or non-native English speakers.
