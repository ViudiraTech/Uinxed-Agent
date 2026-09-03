# Migration parity matrix

This matrix is based on the previous master implementation, not README claims alone. “Coverage” means an automated test exists in this tree; execution status is recorded separately in `docs/performance.md` / release CI and must not be fabricated.

| Legacy behavior | Previous area | Go implementation | Coverage |
|---|---|---|---|
| build / coding / plan | `src/agents.js` | `internal/agent/agents.go` | runtime tests + definitions |
| explorer / coding / general subagents | `src/agents.js`, `App.jsx` | `internal/agent/runtime.go` | runtime isolation paths |
| `delegate` | `tools.js`, `App.jsx` | runtime callback + ToolScheduler | runtime tool-loop tests |
| Parallel tool/subagent execution | `App.jsx` workers | `errgroup`, scheduler, contexts | race CI + runtime tests |
| Main/sub session isolation | `App.jsx` | `Session.ParentID`, child sessions | runtime tests |
| SSE content streaming | `provider.js` | `provider/openai.go` | Chat SSE tests |
| Responses API | `provider.js` | `provider/openai.go` | Responses SSE tests |
| Provider reasoning | `provider.js`, `Thinking.jsx` | normalized reasoning events/TUI | provider + TUI tests |
| `low..max` effort | `App.jsx` | config/runtime/provider | command/provider paths |
| `supercode` | current master source | runtime orchestration + provider max effort | config/runtime paths |
| Local gateway | `config.js` / `provider.js` | built-in provider | provider implementation |
| DeepSeek | `config.js` / `provider.js` | built-in provider | provider implementation |
| Custom OpenAI-compatible provider | `/connect` | connect wizard + persisted provider | command path |
| API key configuration | `/key` | encrypted config store | config tests |
| Model switching | `/model` | controller + model picker | command/TUI paths |
| Session new/switch/delete/resume | `App.jsx` | storage/controller/TUI | storage tests |
| SQLite sessions | `db.js` | normalized pure-Go SQLite | SQLite tests |
| Legacy config storage | `config.js` | `JSONStore` | migration/config tests |
| Legacy config → DB migration | startup `/storage` | backup/import/verify/switch | migration test |
| Legacy SQLite migration | `db.js` schema | v1 detector/transaction conversion | migration implementation |
| Context token estimate | `context.js` | `context/token.go` | context tests |
| Auto/manual compaction | `context.js`, `/compact` | runtime compactor | context/runtime paths |
| Skills discovery | `skills.js` | `internal/skills` | implementation |
| Agent Skills directories | `skills.js` | project/global roots | implementation |
| `bash` | `tools.js` | streaming cancellable shell | tool implementation |
| `read_file` | `tools.js` | bounded binary-safe reader | tool tests |
| `write_file` | `tools.js` | atomic write | tool tests |
| `edit_file` | `tools.js` | unique-match atomic edit | tool tests |
| `list_dir` | `tools.js` | bounded directory listing | tool implementation |
| `grep` / `glob` | `tools.js` | ripgrep first + Go fallback | tool implementation |
| `fetch_url` | `tools.js` | bounded HTTP fetch | tool implementation |
| `web_search` | `tools.js` | provider-independent network tool | tool implementation |
| `todo_write` / `todo_update` | tools/App | runtime callbacks + persisted Todo | runtime/storage paths |
| `use_skill` | tools | skills loader tool | implementation |
| `get_current_time` | removed | current date/time is injected into every system prompt from the host clock | no tool call required |
| `calc` | tools | safe expression parser | runtime tool test |
| Git diff | `/diff` | `internal/git` + responsive Diff UI | implementation |
| Markdown/code blocks | `Markdown.jsx` | Glamour + render cache | Markdown tests |
| Streaming Markdown | React stream state | immediate SSE deltas + lightweight partial renderer; full Markdown on completion | app/TUI tests |
| Command palette | slash UI | `Ctrl+P` fuzzy picker | picker tests |
| Slash completion | `App.jsx` | inline command suggestions | TUI implementation |
| `@agent` | `App.jsx` | direct child Agent turn | runtime/TUI paths |
| `@file` | new migration target | cached fuzzy index + ephemeral context | runtime/index tests |
| `@skill` | skills | unified autocomplete | TUI implementation |
| Terminal resize | Ink dynamic size | Bubble Tea window events | TUI implementation |
| Mouse click | absent/limited legacy | semantic render regions | region tests |
| Region-aware wheel | absent/limited legacy | pointer hit routing | TUI implementation |
| Hover enhancement | absent/limited legacy | all-motion hover for clickable tool/reasoning/picker regions | TUI implementation |
| Configurable mouse capture | new | `/mouse`, `--no-mouse` | config/TUI implementation |
| Ctrl+T | legacy | reasoning toggle | TUI implementation |
| Ctrl+O | legacy | Todo toggle | TUI implementation |
| Ctrl+E | current master footer/behavior | tool-details toggle | TUI implementation |
| Tab Agent switch | legacy | primary Agent cycle | TUI implementation |
| `/provider` | legacy | preserved | command layer |
| `/connect` | legacy | preserved | connect wizard |
| `/key` | legacy | preserved | command layer |
| `/model` | legacy | preserved | command layer |
| `/thinking` | legacy | preserved | command layer |
| `/effort` | source-only/current master | preserved | command layer |
| `/agent` | legacy | preserved | command layer |
| `/help` | legacy | preserved | command layer |
| `/quota` | gateway | preserved | provider/controller |
| `/context` | legacy | preserved | command/context |
| `/compact` | legacy | preserved | runtime |
| `/todos` | legacy | preserved | command/TUI |
| `/cd` / `/pwd` | legacy | preserved | controller/commands |
| `/new` / `/sessions` / `/delete` | legacy | preserved | storage/TUI |
| `/storage` / `/migrate` | legacy | preserved | HybridStore/migration |
| `/diff` | legacy | upgraded | Diff UI |
| `/skills` | legacy | preserved | commands/skills |
| `/clear` | legacy | preserved | controller |
| `/restore` | source-only/current master | preserved with confirmation | command/controller |
| `/exit` | legacy | preserved | TUI |
