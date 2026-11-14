---
name: epub-grep-go-conventions
description: Go conventions for go-epub-grep (cmd/epub-search, pkg/epubproc). Load before writing, reviewing, or planning Go code or tests here. Layout, errors, interfaces, concurrency, security, performance, testing, tooling, repo overrides.
---

# Go conventions: go-epub-grep

Precedence: existing code, `.golangci.yml`, `Taskfile.yml`, `CLAUDE.md` > this file > general Go practice. Never "fix" existing convention to match this file.

## Repo overrides

- Logging: `zerolog` global `log`. Not `slog`.
- Tests: stdlib `testing`, `t.Errorf`/`t.Fatalf`. No testify rewrite. testify only if file already imports it or developer asks.
- Concurrency: `sourcegraph/conc`. No hand-rolled `WaitGroup` pools.
- Existing interfaces not defined at consumer: leave alone in unrelated changes.

## Layout

- `cmd/epub-search/main.go` thin: flags, build request, call `pkg/epubproc`, print.
- Logic in `pkg/epubproc`. Split files by responsibility: scanning, filters, metadata.
- Planned, add when touching package (no restructure just for them):
  - `doc.go`: purpose, handlers run concurrently
  - `errors.go`: exported sentinels (`ErrMissingQuery`, `ErrOPFNotFound`)
  - `options.go`: functional options if constructors grow

## Errors

- Wrap with context: `fmt.Errorf("open epub %q: %w", path, err)`.
- Sentinels for conditions callers test. Callers use `errors.Is`/`errors.As`, never string compare.
- `errors.New` when no verb.
- No discarded error without short comment why safe.
- Validate input at boundary (CLI flags, exported args). Several fields: report all with `errors.Join`.
- Log-and-continue only when skipping one item is intended (one corrupt ePub). Comment it.

## Types and naming

- Accept interfaces, return concrete types. Constructor returns `*T`.
- Interface in consumer package. Small.
- Initialisms: `XML`, `OPF`, `ID`, `URL`, `HTML`.
- Typed constants for small value sets. No magic strings.
- No state: plain function, not type.
- No positional `bool` params: struct or functional options.
- Value receivers unless mutation needed. No `any` without reason.
- Struct field order: alignment and readability.
- Doc comment on every exported identifier. No field or line comment restating name.

## Concurrency

- Every goroutine has owner that waits. `conc` pool, bounded workers.
- `context.Context` first param. Check `ctx.Err()` in loops and long ops. Return `ctx.Err()` consistently; cancel and deadline handled same.
- Handlers run from several workers: document as concurrent. Caller makes them safe.
- Pooled object: reset before `Put`, drop held refs (readers, buffers).
- Bound untrusted reads: `io.LimitReader`, size caps. Zip entry can inflate hugely.
- Run `-race` before calling concurrency work done.

## Security

- Never extract ePub entries to disk. Read in memory with limits.
- Compile user regex once per request. Handle compile error.
- `filepath.Clean` paths. Never trust archive entry names.
- No secrets or tokens in source.

## Performance

- Avoid allocations in hot paths. `Write([]byte)` over `WriteString(string(b))`.
- `strings.Builder` / `bytes.Buffer` for loop concatenation.
- Allocate after early returns, not before.
- Constant slices/maps at package level, not rebuilt per call.
- Slices over maps when order matters. Pool expensive objects (scanners, tokenizers) only with measured benefit.
- Prefer `slices`, `maps`, `min`, `max`, `for range n`.

## Testing

- Behavior names. Table-driven for several cases.
- `t.TempDir()`, not `os.MkdirTemp` + cleanup. `t.Helper()` in helpers. `t.Parallel()` when independent.
- No `time.Sleep`, no tiny timeouts. Use already-cancelled context, channel, blocking fake.
- One correct outcome: assert exactly it. Test passing on success and failure proves nothing.
- Setup errors explicit (`t.Fatalf`/`require.NoError`). Never `_`.
- Concurrent handler results unordered: sort before compare.
- Files: `*_test.go`, `*_integration_test.go`, `*_bench_test.go`. Benchmarks: `b.ReportAllocs()`, `b.ResetTimer()` after setup.
- Test critical behavior, not getters. Wrong assertion worse than no test.

## Tooling

- Prefer `task test|lint|fmt|vet|vuln|build|bench`.
- No `task` (`command -v task`): `go test -race ./...`, `go vet ./...`, `golangci-lint run ./...`, `gofumpt -l .`, `govulncheck ./...`.
- `gofumpt` format. `golangci-lint` v2, `gosec` on.

## Comments

- Why, not what. Keep "ePub" spelling consistent in comments, help text, messages.
