# Validation record

This file separates checks that were actually executed from checks that are defined for CI but could not run in the current build environment.

## Executed in the implementation environment

The delivered working tree was formatted with `gofmt`; the final `gofmt -l` scan returned no files. Both GitHub Actions workflow files were parsed as YAML successfully, and the source tree was scanned for `TODO`, `FIXME`, placeholder/mock/fake core implementations and `panic(` without findings.

The implementation environment provides Go 1.23.2, while Bubble Tea v2.0.8 and modernc SQLite v1.56.0 require Go 1.25. An actual final command was attempted:

```text
$ go test ./...
go: downloading go1.25.0 (linux/amd64)
go: download go1.25.0: golang.org/toolchain@v0.0.1-go1.25.0.linux-amd64: Get "https://proxy.golang.org/golang.org/toolchain/@v/v0.0.1-go1.25.0.linux-amd64.zip": dial tcp: lookup proxy.golang.org ...: read: connection refused
```

The failure is an environment DNS/network restriction while downloading the required Go toolchain; it is not a test failure produced by project code. A second attempt through the available controlled download channel also could not retrieve the Go 1.25 archive.

To continue catching internal integration defects without weakening the production dependency versions, an **ephemeral compile-check copy outside the delivered project** replaced third-party modules with minimal compile-only API shims. Those shims are not included in this project and are not production code. The final source was synchronized into that copy after the last code changes.

The following were actually run successfully:

```text
# Compile every package and every _test.go file.
go test -run '^$' ./...

# Execute unit tests whose assertions exercise Uinxed-owned logic.
go test \
  ./internal/config ./internal/context ./internal/tools ./internal/git \
  ./internal/agent ./internal/app ./internal/provider ./internal/tui \
  ./internal/indexer ./internal/markdown ./internal/terminal

# Static analysis.
go vet ./...

# Race detector on the concurrency-bearing core.
go test -race \
  ./internal/agent ./internal/app ./internal/provider ./internal/tools \
  ./internal/tui ./internal/config ./internal/context ./internal/terminal

# Cross-compilation/type checking from Linux without trying to execute
# foreign test binaries.
GOOS=windows GOARCH=amd64 go test -run '^$' -exec=/bin/true ./...
GOOS=darwin  GOARCH=amd64 go test -run '^$' -exec=/bin/true ./...
```

All listed successful commands above completed successfully. A broader `go test -race ./...` attempt additionally reached the three SQLite round-trip tests, but those tests cannot execute against the compile-only shim because it deliberately does not register a fake `sqlite` driver; all non-SQLite-driver packages passed race. A compile-check binary was also built with injected version metadata to verify that the release-time `main.version`, `main.commit` and `main.buildDate` ldflags are wired to `--version`.

Regression coverage includes:

- provider stream lifecycle and cancellation;
- Responses API `call_id` handling;
- provider key/decryption error propagation;
- provider setup failure still producing a terminal Agent lifecycle event;
- runtime shutdown cancelling and waiting for active turns;
- per-session cancel-and-wait before destructive deletion;
- bounded terminal lifecycle delivery so shutdown cannot deadlock on a stopped UI consumer;
- start-after-close rejection;
- file-reference context remaining ephemeral;
- legacy config/secret compatibility;
- config-mode startup fallback when SQLite is unavailable;
- SQLite import/migration code paths compiled; SQL schema and LIKE-escape contract separately executed with SQLite 3.46.1; real modernc driver round-trip remains a CI-only check in this environment;
- path/symlink escape protection and atomic file operations;
- terminal ANSI/OSC-52/bidi-control sanitization;
- immediate content/reasoning streaming plus tool-output coalescing/backpressure;
- Markdown cache and repository file-index behavior.

The production SQLite schema string was also extracted from `internal/storage/sqlite.go` and executed using the environment's SQLite 3.46.1 implementation. Creation of `sessions`, `messages`, `todos`, `tool_activities` and `schema_migrations` succeeded, and the exact `LIKE ... ESCAPE '\'` search contract was exercised with literal `%`, `_` and backslash characters. This validates SQL syntax/escaping only; it is not presented as a substitute for the real `modernc.org/sqlite` driver tests.

## Real dependency release gate

The repository intentionally keeps the real current production dependencies in `go.mod`; the compile-only shims never enter the delivered tree. On Go 1.25 with normal module access, ordinary CI runs:

```text
go mod download
go mod verify
gofmt check
go test ./...
go test -race ./...      # Linux
go vet ./...
go build ./cmd/ux-agent
```

CI covers Linux, macOS and Windows. The tag-triggered Release workflow repeats its own independent validate gate (`go mod verify`, formatting, tests, vet and Linux race detector) **before** the release build matrix is allowed to run. Tagged artifacts are then built with `CGO_ENABLED=0` for Linux amd64/arm64, macOS amd64/arm64 and Windows amd64, with version/commit/build-date metadata injected and SHA-256 checksums generated.

No benchmark numbers are reported until `scripts/benchmark.sh` is run on a concrete machine using the real dependency graph. See `docs/performance.md`.
