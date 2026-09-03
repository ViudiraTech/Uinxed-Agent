# Architecture

## Boundaries

Uinxed-Agent 2.0 is split into domain layers rather than translating the old `App.jsx` component tree into Go.

```text
cmd/ux-agent
    │
    ▼
internal/tui                 Bubble Tea only here
    │
    ▼
internal/app                 orchestration/controller
    │
    ├────────► internal/agent
    │              │
    │              ├────► internal/provider
    │              ├────► internal/tools
    │              ├────► internal/context
    │              ├────► internal/skills
    │              └────► internal/storage
    │
    ├────────► internal/indexer
    └────────► internal/git
```

Rules enforced by package dependencies:

1. Agent runtime does not import Bubble Tea, Bubbles or Lip Gloss.
2. Providers only know provider requests/events and domain messages.
3. Tools return typed results and optional output callbacks; they never mutate TUI models.
4. Persistence has no knowledge of the active screen or focus.
5. TUI invokes the controller and consumes typed runtime events.

## Runtime event model

`internal/domain/events.go` defines stream delta, reasoning delta, tool lifecycle, Agent lifecycle, Todo, session, usage, compaction and error events. Runtime event channels are bounded. `internal/app/stream.go` forwards model content/reasoning deltas immediately so provider SSE remains visibly live. Only cumulative `ToolOutput` snapshots are coalesced on the configurable render interval (minimum 8 ms).

This protects input latency when a fast model emits many tiny SSE chunks.

## Provider layer

`internal/provider/OpenAICompatible` uses one reusable `http.Client`/`Transport` per provider instance with keep-alive, connection pooling, request cancellation, retry/backoff for transient 429/5xx failures, response limits and tolerant SSE parsing.

Supported wire formats:

- OpenAI-compatible `/chat/completions`
- OpenAI Responses API `/responses`

Both normalize content, provider-exposed reasoning, tool calls, usage and finish state into one event stream. Unknown/malformed SSE frames are skipped instead of crashing the TUI.

## Agent execution

A session can have at most one active turn. Every turn has a `context.Context` and cancel function. Runtime shutdown marks the runtime closed, cancels active turns and waits for their goroutines before storage/terminal teardown, preventing orphan work from surviving application exit. The runtime builds a system prompt from the selected Agent, installed skills, model and effort state, then streams provider events.

Tool calls are assembled by streaming tool-call index/ID and run through `ToolScheduler`. Independent calls can execute concurrently under category limits. Write/shell/delegate/state categories have separate concurrency controls so the application does not spawn unbounded goroutines.

`delegate` creates a child `Session` with `ParentID`, its own messages/Todos/tool history and a child Agent ID. A direct `@explorer`, `@coding` or `@general` prompt uses the same isolated child-session mechanism.

## File references

`@file` autocomplete is backed by a cached repository index. `rg --files` is preferred; fallback uses `WalkDir`. Heavy directories such as `.git`, `node_modules`, `vendor`, `build` and `dist` are excluded.

Because fsnotify is non-recursive, the index registers the de-duplicated parent directory set discovered during the initial scan. A debounced filesystem event triggers a rebuild and new directories are added on that rebuild.

A referenced file is read through the regular sandboxed `read_file` tool. The current turn receives bounded ephemeral `<referenced_file>` context; file contents are not appended to the visible/persisted user message.

## TUI

Layouts:

- **Large (`>=120`)**: conversation plus optional session/sidebar information.
- **Medium (`80–119`)**: conversation-first, secondary surfaces are overlays.
- **Small (`<80`)**: single column with reduced status information.

`FocusManager` controls Prompt/Chat/Sidebar/Overlay focus. Mouse hit regions are produced while rendering, and pointer-wheel routing uses those regions. Overlays include unified pickers, Diff, Todos, connect wizard and confirmations.

The conversation renderer keeps lightweight block estimates for all messages but only renders the visible region plus overscan. Completed Markdown uses an LRU-style cache keyed by message ID, content version, width and theme.

## Persistence

The default store is pure-Go SQLite (`modernc.org/sqlite`). The schema normalizes sessions, messages, Todos and tool activities. SQLite is configured with WAL, a busy timeout, foreign keys and indexes.

`HybridStore` can switch between SQLite and legacy-compatible `config.json` storage. Legacy migration is backup-first and verify-before-switch.

## Process and file safety

- File paths are normalized against the session working directory and symlink escape is rejected.
- Writes use temp-file + sync + rename and preserve existing permission bits where possible.
- `edit_file` refuses ambiguous multi-match replacement.
- Binary reads/edits are rejected.
- Shell commands stream stdout/stderr, inherit cancellation and terminate their process group/tree on cancellation.
- API keys are redacted in normal output and encrypted at rest.
- Untrusted model/tool/session text is stripped of ANSI CSI/OSC, OSC-52 clipboard operations, terminal controls and bidi spoofing controls before display.
- Structured JSON logging records agent/tool/compaction lifecycle and duration without logging raw tool arguments or secrets.
