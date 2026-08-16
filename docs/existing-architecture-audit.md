# Existing architecture audit (pre-Go master)

The pre-migration application was Node.js + React + Ink. The central `src/App.jsx` was responsible for terminal layout, command handling, session state, streaming state, storage switching, subagent orchestration and many modal interactions, making UI behavior and runtime behavior tightly coupled.

Key source areas audited before migration:

| Area | Legacy files / behavior |
|---|---|
| CLI | `src/index.js` |
| Main TUI/controller | `src/App.jsx` |
| Agent definitions | `src/agents.js` |
| Provider streaming | `src/provider.js` |
| Config/providers/secrets | `src/config.js` |
| SQLite session persistence | `src/db.js` |
| Context/token compaction | `src/context.js` |
| Tools | `src/tools.js`, worker files |
| Skills | `src/skills.js` |
| Markdown/reasoning/activity rendering | `Markdown.jsx`, `Thinking.jsx`, `ActivityPanel.jsx` |

Source inspection also found current-master behavior not fully represented in the initial migration checklist: `/effort` with `supercode`, and `/restore`. Both are included in the Go command surface.

The old metadata also described Node build/install instructions and package repository metadata that no longer matches the Go project. Version 2.0 replaces those instructions with Go module/build/release metadata.
