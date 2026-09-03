<div align="center">

# Uinxed-Agent 2.0

**A native Go AI coding agent for the terminal.**
Fast TUI, multi-agent delegation, streaming tool execution, persistent sessions, diff review, mouse control and keyboard-first workflows — packaged as a single Go binary.

[![CI](https://github.com/ViudiraTech/Uinxed-Agent/actions/workflows/ci.yml/badge.svg)](https://github.com/ViudiraTech/Uinxed-Agent/actions/workflows/ci.yml)
![Go](https://img.shields.io/badge/Go-1.25%2B-00ADD8?logo=go&logoColor=white)
![License](https://img.shields.io/badge/license-Apache--2.0-blue)
![TUI](https://img.shields.io/badge/UI-Bubble%20Tea%20v2-ff69b4)

</div>

Uinxed-Agent 2.0 is a ground-up rewrite of the original Node.js + React + Ink implementation. The new version keeps the coding-agent workflow while moving the runtime, TUI, storage, provider layer and tool system to Go.

The result is a smaller deployment surface, native terminal interaction, explicit concurrency and cancellation, and a UI architecture that is no longer coupled to the agent runtime.

> No Node.js, npm or `node_modules` are required at runtime.

## Why 2.0

| Area | Uinxed-Agent 2.0 |
|---|---|
| Runtime | Native Go 1.25+ |
| TUI | Bubble Tea v2 + Bubbles v2 + Lip Gloss v2 |
| Markdown | Glamour v2 |
| Storage | Pure-Go SQLite (`modernc.org/sqlite`) + legacy config compatibility |
| Providers | OpenAI-compatible Chat Completions / Responses streaming |
| Agents | Primary agents + isolated delegated subagents |
| Input | Keyboard, multiline prompt, mouse click/wheel, command palette, `@` completion |
| Distribution | Single binary; no Node runtime |

## Highlights

- **Responsive terminal UI** with large, medium and small layouts.
- **Keyboard + mouse interaction** with semantic hit regions, wheel routing and optional mouse capture.
- **True live SSE conversations**: content/reasoning deltas reach the TUI immediately; only cumulative tool stdout snapshots are coalesced to avoid redundant redraws.
- **Provider reasoning display** with collapsible reasoning content and effort controls.
- **Multi-agent runtime** with `build`, `coding`, `plan`, plus isolated `explorer`, `coding` and `general` subagents.
- **Concurrent delegation** through a bounded tool scheduler and cancellable child sessions.
- **Real coding tools** for shell execution, file reads/writes/edits, grep, glob, URL fetch, web search, skills and Todo management.
- **Safe file writes** with project-root validation, symlink escape protection and atomic replace semantics.
- **Persistent sessions** backed by SQLite WAL with legacy storage migration and verification.
- **Diff review**, session navigation, provider/model/agent pickers, Todo panel and command palette.
- **Structured logs** with secret redaction.
- **Cross-platform CI** for Linux, macOS and Windows.

## Quick start

### Requirements

- Go **1.25+**
- Git
- `rg` / ripgrep is optional but recommended; Uinxed-Agent has Go fallbacks where possible.

### Build from source

```bash
git clone https://github.com/ViudiraTech/Uinxed-Agent.git
cd Uinxed-Agent

go mod download
go build -trimpath -o ux-agent ./cmd/ux-agent
./ux-agent
```

On Windows:

```powershell
go build -trimpath -o ux-agent.exe ./cmd/ux-agent
.\ux-agent.exe
```

### Install with Go

After the repository is published, you can also install the command into `$(go env GOPATH)/bin`:

```bash
go install github.com/ViudiraTech/Uinxed-Agent/cmd/ux-agent@latest
ux-agent
```

## First run

Uinxed-Agent ships provider definitions, **not provider credentials**. Configure your own key locally from the TUI or CLI.

Inside the TUI:

```text
/provider
/key
/model
```

Or at startup:

```bash
./ux-agent --provider <provider-id> --key "$API_KEY" --model <model-id>
```

For an OpenAI-compatible endpoint you can also override the base URL:

```bash
./ux-agent --provider <provider-id> --base https://example.com/v1 --key "$API_KEY"
```

## TUI controls

| Input | Action |
|---|---|
| `Ctrl+P` | Open command palette |
| `Tab` | Accept completion, otherwise cycle primary agent |
| `Ctrl+T` | Expand / collapse provider reasoning |
| `Ctrl+O` | Toggle Todos |
| `Ctrl+E` | Expand / collapse tool details |
| `PgUp` / `PgDn` | Scroll conversation or active overlay |
| `Esc` | Close overlay; while generating, cancel current operation |
| `Ctrl+C` | Cancel current operation; when idle, quit |
| Mouse click | Activate visible controls / selectable rows |
| Mouse wheel | Scroll the region under the pointer |
| `@explorer task` | Run an isolated explorer subagent |
| `@coding task` | Run an isolated coding subagent |
| `@general task` | Run an isolated general subagent |
| `@path/to/file` | Attach bounded file context to the turn |
| `@skill:name` | Discover a skill from autocomplete |

Mouse capture is enabled by default and can be changed at runtime:

```text
/mouse on
/mouse off
```

Use `--no-mouse` if you want the terminal to retain native text-selection behavior.

## Commands

The Go rewrite preserves the existing command surface, including commands that existed only in the previous source implementation:

```text
/help       /connect    /provider   /key        /model
/thinking   /effort     /agent      /quota      /context
/compact    /todos      /cd         /pwd        /new
/sessions   /rename     /parent     /delete     /storage
/migrate    /diff       /skills     /mouse      /theme
/clear      /restore    /exit
```

Reasoning effort accepts:

```text
low  medium  high  xhigh  max  supercode
```

`supercode` keeps Uinxed-Agent's orchestration mode while mapping provider reasoning effort to the strongest supported setting.

## Architecture

```text
┌───────────────────────────────┐
│          Bubble Tea TUI       │
│ input · view · overlays       │
└──────────────┬────────────────┘
               │ typed events
┌──────────────▼────────────────┐
│      Application Controller   │
└───────┬───────────┬───────────┘
        │           │
        │           └──────────────► Session Store
        │                              SQLite / config
        ▼
┌───────────────────────────────┐
│          Agent Runtime        │
│ sessions · cancellation       │
│ delegation · compaction       │
└───────┬───────────┬───────────┘
        │           │
        ▼           ▼
   Providers      Tools / Skills
   streaming      scheduler
```

The agent runtime intentionally does **not** import Bubble Tea. Providers do not know about the TUI, and tools do not mutate UI state directly.

More detail:

- [`docs/architecture.md`](docs/architecture.md)
- [`docs/existing-architecture-audit.md`](docs/existing-architecture-audit.md)
- [`docs/opencode-research.md`](docs/opencode-research.md)

## Storage and migration

Default configuration directory:

```text
~/.config/ux-agent/
├── config.json
└── ux-agent.db
```

Uinxed-Agent detects legacy `config.json` sessions and the previous SQLite session schema before migration. Migration is backup-first: data is imported, read back for verification, and only then is the active storage mode switched.

SQLite uses WAL mode, a busy timeout and foreign-key enforcement. The Go implementation uses `modernc.org/sqlite`, so a system `libsqlite3-dev` package is not required for the normal build.

See:

- [`docs/migration.md`](docs/migration.md)
- [`docs/migration-parity.md`](docs/migration-parity.md)

## Security notes

- No API keys are meant to be committed to the repository.
- Provider keys entered by the user are stored in the local configuration using AES-256-GCM encryption.
- Normal logs and UI output redact credential-like values.
- `.env`, local databases, WAL files, logs and local config files are ignored by Git.
- File tools enforce a project-root boundary and reject symlink escapes.

If you discover a security issue, avoid posting credentials or exploit details in a public issue.

## Performance

The rendering path is designed to avoid doing expensive work for every provider token:

- model content and reasoning deltas are forwarded to the UI immediately;
- only cumulative high-frequency tool-output snapshots are coalesced;
- partial assistant Markdown uses a lightweight streaming renderer, then switches to full Markdown after completion;
- completed Markdown is cached by message/version/width/theme;
- the conversation view materializes only the visible range plus overscan;
- file search prefers `rg` when available and falls back to Go;
- HTTP transports use connection pooling and keep-alive;
- shell commands and agent runs are context-cancellable.

Run the benchmark harness with:

```bash
./scripts/benchmark.sh
```

Methodology and validation notes are documented in:

- [`docs/performance.md`](docs/performance.md)
- [`docs/validation.md`](docs/validation.md)

## Development

Useful targets:

```bash
make fmt
make test
make vet
make race
make build
```

Or run the full local check:

```bash
make check
```

Equivalent commands:

```bash
gofmt -w $(find . -name '*.go')
go test ./...
go test -race ./...
go vet ./...
go build -trimpath -o ux-agent ./cmd/ux-agent
```

The GitHub Actions CI matrix covers Linux, macOS and Windows. Tagged releases run their own validation gate before building release archives and SHA-256 checksums.

## Repository layout

```text
cmd/ux-agent/          CLI entry point
internal/agent/        agent runtime, delegation, scheduling
internal/app/          application controller
internal/config/       configuration and local secret storage
internal/context/      context/token budgeting and compaction
internal/domain/       shared domain types
internal/indexer/      project file index and watcher
internal/markdown/     cached Markdown rendering
internal/provider/     provider APIs and streaming
internal/skills/       skill discovery/loading
internal/storage/      SQLite + compatibility storage
internal/tools/        coding tools
internal/tui/          Bubble Tea UI, focus, mouse, overlays
scripts/               benchmark / maintenance scripts
docs/                  architecture, migration and validation notes
```

## Contributing

Issues and pull requests are welcome. For code changes, please run at least:

```bash
gofmt -w $(find . -name '*.go')
go test ./...
go vet ./...
```

For concurrency-sensitive changes, also run:

```bash
go test -race ./...
```

## License

Licensed under the [Apache License 2.0](LICENSE).

---

<div align="center">

**Uinxed-Agent · ViudiraTech**

</div>
