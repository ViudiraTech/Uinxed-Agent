# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Project

Uinxed-Agent 2.0 — a native Go terminal AI coding agent. Single static binary, no CGO, no Node/Python runtime. Module: `github.com/ViudiraTech/Uinxed-Agent`. Requires **Go 1.25+** (Bubble Tea v2 / modernc SQLite).

## Commands

```bash
make              # go build -trimpath -ldflags ... -o ux-agent ./cmd/ux-agent
make test         # go test ./...
make race         # go test -race ./...
make vet          # go vet ./...
make fmt          # gofmt -w on all *.go (excluding .gocache)
make check        # fmt + test + race + vet + build — run before submitting
make bench        # scripts/benchmark.sh (build, startup, package benchmarks)
make help         # list targets
```

Single test / package:

```bash
go test ./internal/tui -run TestConversationHidesSystemAndVirtualizes -v
go test -race ./internal/agent
```

Run the binary: `./ux-agent` (or `go run ./cmd/ux-agent`). Useful flags: `--provider`, `--key`, `--base`, `--model`, `--theme`, `--session`, `--config-dir`, `--debug`, `--no-mouse`, `--no-banner`, `--reset`, `--version`.

CI (`.github/workflows/ci.yml`) runs on ubuntu/macos/windows: gofmt check, `go test ./...`, `go vet ./...`, build, and `go test -race ./...` on Linux only. Tests that need git or symlink privileges `t.Skip` when unavailable.

## Architecture

Layered, with dependency direction enforced by convention (see `docs/architecture.md`):

```
cmd/ux-agent → internal/tui → internal/app → internal/agent → {provider, tools, context, skills, storage}
```

**Hard boundary rules — do not break these:**
1. `internal/agent` and `internal/tools` must not import Bubble Tea / Bubbles / Lip Gloss. They are headless and unit-testable.
2. `internal/provider` knows only provider requests/events and domain messages.
3. Tools return typed `tools.Result` plus output callbacks; they never touch TUI models.
4. `internal/storage` has no knowledge of screen or focus.

### Runtime event pipeline

`internal/domain/events.go` defines the one-way event stream (stream/reasoning deltas, tool lifecycle, agent lifecycle, todo, usage, compaction, error, approval notifications, plan changed). `internal/app/stream.go` `CoalesceEvents` batches them on a render interval (floor 8 ms): content/reasoning deltas append, cumulative tool-output snapshots replace, everything else is forwarded after flushing pending batches so lifecycle boundaries never overtake their text.

**Approval answers never travel through events.** `CoalesceEvents` reorders and batches, so it cannot carry a response; the UI replies with a direct call to `Controller.ResolveApproval`. This is a deliberate contract, not an oversight.

### Agent runtime (`internal/agent`)

One active turn per session, each with its own `context.Context`. `Runtime.StartTurn` → `runTurn` → `loop` streams provider events, assembles tool calls from streaming index/ID deltas (`toolAccumulator`), and dispatches through `tools.Scheduler`. `Runtime.Close` cancels active turns and waits before storage/terminal teardown.

`delegate` spawns a child `Session` with `ParentID`, its own messages/Todos/tool history, and a child agent ID. A direct `@explorer` / `@coding` / `@general` prompt goes through the same mechanism.

Special tools are implemented in the runtime, not the registry: `todo_write`/`todo_update`, `plan_write`, `switch_mode`, `exit_plan`, `delegate`, plus context compaction.

- `plan_write` is gated by the **session's working mode**, not the active agent. Leaving `plan` mode must go through `exit_plan` (which opens the plan-approval dialog); `switch_mode` out of `plan` is refused. Subagents may never switch modes.
- Provider SSE is deliberately tolerant: unknown/malformed frames are skipped, never fatal.

### Approval policy (`internal/approval`)

A pure policy layer — no TUI, no runtime, no I/O. Modes: `plan` (read-only, no escape hatch) → `read-only` → `auto-edit` (default) → `full-auto`, cycled with Shift+Tab. The decision matrix is keyed on `tools.Category`, resolved through the tool registry so a tool cannot be registered with one category and evaluated against another. Keep the blocking/broker mechanics in `internal/agent` (`approval_broker.go`), not here.

### Tool registry / scheduler (`internal/tools`)

**Adding a tool** = implement the `Tool` interface (`Name`, `Description`, `Schema`, `Category`, `Execute`) and add it to `DefaultRegistry()`. The `Category()` method is the single source of truth for both scheduler concurrency and the approval matrix.

Scheduler categories and default concurrency: read 8, write 1, shell 2, network 6, delegate 4, state 4.

File safety invariants live here and must be preserved: paths are normalized against the session working directory with symlink escape rejected; writes are temp-file + sync + rename preserving permission bits; `edit_file` refuses ambiguous multi-match replacement; binary reads/edits are rejected; shell commands stream output and kill their process group on cancellation (see `proc_unix.go` / `proc_windows.go`).

### TUI (`internal/tui`)

The only package that imports Bubble Tea. A single full-width column at every size — no side panel, no width thresholds; overlays narrow below 40 columns and the diff reviewer splits at 90. `FocusManager` owns Prompt/Chat/Overlay focus. The working line, the todo block and the welcome card all live in `renderBase`'s row arithmetic: reserve the row before rendering or the transcript shifts. **Mouse hit regions are produced during rendering** (`regions.go`) and consumed by `findRegion`; the topmost region wins. The conversation renderer virtualizes (renders visible region + overscan) and caches completed Markdown by message ID, content version, width and theme.

Slash commands are declared in `commandDefs` in `commands.go` — add there and handle in `executeCommand`. Themes live in `theme.go`; the valid set is also listed in `config.Themes()`, and the default one is led by `themeNames`. `claude` is the default and carries Claude Code's brand tokens; the others are untouched. Ctrl+O expands tool details (the tool cards advertise it) and Ctrl+E aliases it; the todos overlay is `/todos`.

### Provider, config, storage

- Providers are OpenAI-compatible. Two wire formats supported: `/chat/completions` and the Responses API `/responses`. One reusable `http.Client`/`Transport` per provider instance, with keep-alive, retry/backoff on transient 429/5xx, and tolerant SSE parsing. Both formats normalize to the same event stream.
- Config: `~/.config/ux-agent/config.json` (`config.DefaultDir()`). API keys are AES-256-GCM encrypted at rest (`secrets.go`) and redacted via `RedactSecret` before logging. A malformed config is backed up to `*.invalid` and the app boots on defaults.
- Logs/state: `~/.local/state/ux-agent` (`$XDG_STATE_HOME` respected).
- Storage: pure-Go SQLite WAL by default (`modernc.org/sqlite`); `HybridStore` can switch to legacy `config.json` storage. Legacy migration is backup-first and verify-before-switch.
- Skills: `SKILL.md` with YAML frontmatter, discovered from project roots (`.ux-agent/skills`, `.opencode/skills`, `.claude/skills`, `.agents/skills`) and global equivalents under `~/.config/...`. Skill names must match `^[a-z0-9]+(?:-[a-z0-9]+)*$`.

## Conventions

- **Package comments explain why, not what.** Several files (`internal/approval/approval.go`, `internal/app/stream.go`, `internal/tools/registry.go`) open with prose justifying a design decision. Match that register when adding non-obvious code.
- **Untrusted text is sanitized before display.** Model/tool/session output is stripped of ANSI CSI/OSC, OSC-52 clipboard ops, terminal controls, and bidi spoofing controls (`internal/terminal/sanitize.go`). Route new display paths through it.
- **Logging is structured** (`log/slog`) and records agent/tool/compaction lifecycle and duration without raw tool arguments or secrets.
- Tests live next to the code as `*_test.go`, table-driven where practical. TUI tests build a `Model` directly with a temp `config.Store` and assert on `ansi.Strip`-ed render output rather than driving a real terminal.
- The legacy Node.js/React implementation must not return — see the `.gitignore` block for `node_modules/`, `dist-js/`, etc.
- `docs/` holds architecture, migration/parity, performance and validation notes. `docs/validation.md` distinguishes checks that actually ran from CI-only gates — keep that distinction honest when updating it.

## Available skills in this repo

`.ux-agent/skills/{laobandahua,dabaihua}/SKILL.md` are project-local skills loaded by the agent at runtime, not Claude Code skills.
