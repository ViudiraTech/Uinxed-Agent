# OpenCode TUI research notes

Research snapshot: 2026-08-16.

The current official OpenCode project is `anomalyco/opencode` on the `dev` branch. The older `opencode-ai/opencode` Go repository is archived and is not the reference implementation used for this rewrite.

Sources reviewed:

- current OpenCode repository and README (`anomalyco/opencode`)
- current official TUI documentation (`packages/web/src/content/docs/tui.mdx`)
- current TUI package extraction/design specification (`specs/tui-package.md`)
- current command-palette/session/model/agent behavior surfaced by the TUI implementation and docs

## UX patterns adopted

The Go rewrite borrows interaction principles rather than source code or technology:

- conversation-first screen with restrained borders and high information density;
- command palette as a first-class navigation surface (`Ctrl+P`);
- session/model/provider/agent selection through focused pickers instead of permanent dashboards;
- slash commands remain discoverable from the prompt;
- `@file` performs fuzzy repository search and adds actual file contents to model context;
- tool executions are compact rows that can be expanded for details;
- supporting information becomes sidebars/overlays rather than permanently consuming the conversation area;
- terminal configuration controls scroll behavior and other TUI preferences.

The official TUI documentation explicitly describes fuzzy `@` file references whose content is added to the conversation and a configurable command list/scroll experience. Those semantics were retained while extending the picker to Uinxed agents and skills.

## Patterns deliberately not copied

OpenCode's current implementation is TypeScript and uses its own OpenTUI/Solid/client-server stack. Uinxed-Agent does not copy that implementation or change its runtime to match it.

Instead:

- Bubble Tea v2 owns the terminal event loop;
- `internal/tui` is the only layer that imports Bubble Tea/Bubbles/Lip Gloss;
- `internal/app` is an application controller;
- `internal/agent`, providers, tools, storage and context are terminal-framework independent;
- runtime-to-UI updates use typed, bounded events.

This is also consistent with OpenCode's current architecture direction: its TUI package design separates presentation/client responsibilities from backend domain operations through an SDK boundary. Uinxed uses a Go controller/runtime boundary for the same maintainability goal without copying OpenCode's code.

## Uinxed-specific differences

Uinxed keeps capabilities that are part of its existing identity rather than forcing OpenCode parity:

- build/coding/plan primary agents;
- explorer/coding/general isolated subagents;
- `delegate` with concurrent child execution and result return;
- legacy `low..max` reasoning effort plus `supercode`;
- explicit Todo tool state;
- legacy config/SQLite migration and provider compatibility;
- `/storage`, `/restore`, `/quota` and existing command behavior.
