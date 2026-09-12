<div align="center">

# ⚡ Uinxed-Agent 2.0

**A next-generation, native Go AI coding agent crafted for your terminal.**

Fast Bubble Tea v2 TUI • Autonomous Multi-Agent Delegation • Streaming Reasoning & Tool Calls • Git Diff Review • Pure Go & Zero Dependencies

[![CI](https://github.com/ViudiraTech/Uinxed-Agent/actions/workflows/ci.yml/badge.svg)](https://github.com/ViudiraTech/Uinxed-Agent/actions/workflows/ci.yml)
[![Go Version](https://img.shields.io/badge/Go-1.25%2B-00ADD8?style=flat-square&logo=go&logoColor=white)](https://go.dev/)
[![TUI Framework](https://img.shields.io/badge/TUI-Bubble%20Tea%20v2-ff69b4?style=flat-square)](https://github.com/charmbracelet/bubbletea)
[![Zero CGO](https://img.shields.io/badge/CGO-Disabled%20(Pure%20Go)-success?style=flat-square)](https://github.com/modernc/sqlite)
[![License](https://img.shields.io/badge/License-Apache%202.0-blue.svg?style=flat-square)](LICENSE)
[![Platform](https://img.shields.io/badge/Platform-Linux%20%7C%20macOS%20%7C%20Windows-lightgrey?style=flat-square)](https://github.com/ViudiraTech/Uinxed-Agent/releases)

<p align="center">
  <a href="README.md"><b>English</b></a> •
  <a href="README.zh.md"><b>简体中文</b></a>
</p>

<p align="center">
  <a href="#-overview">Overview</a> •
  <a href="#-terminal-preview">Terminal Preview</a> •
  <a href="#-key-features">Features</a> •
  <a href="#-quick-start">Quick Start</a> •
  <a href="#-keybindings--controls">Controls</a> •
  <a href="#-multi-agent-system">Multi-Agent</a> •
  <a href="#-architecture">Architecture</a> •
  <a href="#-themes">Themes</a>
</p>

</div>

---

## 🌟 Overview

**Uinxed-Agent 2.0** is a complete, ground-up rewrite in **pure Go**. It discards the heavy Node.js + React + Ink runtime in favor of a native, single-binary architecture built for performance, stability, and terminal ergonomics.

> **💡 Zero Runtime Dependencies**: No Node.js, npm, Python, or external `libsqlite3` libraries are needed. Download or build a single binary and start pair-programming with AI immediately.

### Why 2.0?

| Capability | Legacy (v1.x) | Uinxed-Agent 2.0 |
|---|---|---|
| **Runtime** | Node.js + npm + React Ink | **Native Go 1.25+** (Single static binary) |
| **Startup Time** | ~1.5s – 3.0s | **< 2ms** cold launch |
| **Memory (RSS)** | ~180MB – 350MB | **< 30MB** base footprint |
| **TUI Engine** | React Ink DOM emulation | **Bubble Tea v2 + Lip Gloss v2 + Glamour v2** |
| **Storage** | Fragile JSON files | **Embedded Pure-Go SQLite WAL** (`modernc.org/sqlite`) |
| **Mouse Support** | None | **Native Click, Wheel Routing & Text Selection** |
| **Tool Execution** | Blocking child processes | **Concurrent Scheduler + Context Cancellation** |
| **Distribution** | `npm install -g` + hundreds of dependencies | **Single independent executable** |

---

## 🖥️ Terminal Preview

```text
┌─ Uinxed-Agent 2.0 ────────────────────────────────────────────────────────────────────┐
│                                                                                       │
│  ❯ user                                                                               │
│    Implement an atomic file writer in Go with tests and verify with go test.          │
│                                                                                       │
│  ⏺ assistant                                                                          │
│    ✻ Thinking (1.4s) · Ctrl+T to toggle ────────────────────────────────────────────  │
│                                                                                       │
│    I will create the file writer with atomic temp-file rename semantics, then run     │
│    the test suite to verify behavior.                                                 │
│                                                                                       │
│    ⏺ Running bash "go test -v ./internal/tools"                                       │
│      ⎿ === RUN   TestAtomicFileWriter                                                 │
│        --- PASS: TestAtomicFileWriter (0.02s)                                         │
│        PASS                                                                           │
│                                                                                       │
│    All tests passed! The implementation guarantees atomic file replacement.           │
│                                                                                       │
├───────────────────────────────────────────────────────────────────────────────────────┤
│ ❯ Ask anything…   ( / commands · @ files · ? shortcuts )                              │
├───────────────────────────────────────────────────────────────────────────────────────┤
│ 󰘧 step-3.7-flash · 📁 ~/workspace · 💾 db · ⚡ supercode · 🤖 build · 󰄴 Ready          │
└───────────────────────────────────────────────────────────────────────────────────────┘
```

---

## ✨ Key Features

<div align="center">

| 🚀 **Pure Go & Single Binary** | ⚡ **Real-Time Streaming** | 🤖 **Autonomous Multi-Agent** |
|---|---|---|
| Zero dependencies. Instant sub-millisecond cold start. Pure-Go SQLite WAL storage. | Live SSE streaming for content and deep reasoning (`thinking_content`) with collapsible view. | Primary agents (`build`, `coding`, `plan`) + isolated parallel subagents (`explorer`, `coding`, `general`). |

| 🛠️ **Full Developer Toolchain** | 🖱️ **Modern Bubble Tea v2 TUI** | 🔒 **Local-First Security** |
|---|---|---|
| Sandboxed shell, atomic file patches, AST grep/glob, web search & scrape, interactive Todos. | Responsive layout, semantic mouse support (click/wheel), `Ctrl+P` command palette, fuzzy autocomplete. | Project-root boundary sandbox, AES-256-GCM encrypted API keys, credential redaction in logs. |

</div>

---

## ⚡ Quick Start

### 1. Installation

#### Option A: Install with Go (Recommended)

```bash
go install github.com/ViudiraTech/Uinxed-Agent/cmd/ux-agent@latest
ux-agent
```

#### Option B: Build from Source

```bash
# Clone repository
git clone https://github.com/ViudiraTech/Uinxed-Agent.git
cd Uinxed-Agent

# Download dependencies & compile
go mod download
go build -trimpath -o ux-agent ./cmd/ux-agent

# Launch
./ux-agent
```

*On Windows (PowerShell):*
```powershell
go build -trimpath -o ux-agent.exe .\cmd\ux-agent
.\ux-agent.exe
```

---

### 2. Configure Providers

Uinxed-Agent works with any **OpenAI-compatible** endpoint (DeepSeek, StepFun, OpenAI, Claude proxies, Ollama, vLLM, etc.).

#### Method A: Built-in Interactive Wizard (Fastest)
Launch `ux-agent` and run:
```text
/connect
```
The wizard guides you through naming, base URL, API key, and model selection.

#### Method B: In-App Slash Commands
```text
/provider      # Open provider selector
/model         # Select model for current session
/key           # Enter and AES-256-GCM encrypt your API key
```

#### Method C: CLI Startup Flags
```bash
# Start directly with provider credentials
./ux-agent --provider deepseek --key "sk-..." --model deepseek-v4-flash

# Override base URL for custom gateway / local LLM
./ux-agent --provider custom --base http://localhost:11434/v1 --model qwen2.5-coder
```

---

## ⌨️ Keybindings & Controls

### Global Shortcuts

| Shortcut | Scope | Action |
|---|---|---|
| `Ctrl+P` | Global | Open **Command Palette** (search all commands & actions) |
| `Ctrl+T` | Global | Expand / collapse **Reasoning (Thinking)** blocks |
| `Ctrl+O` | Global | Toggle **Task Todos** checklist overlay |
| `Ctrl+E` | Global | Expand / collapse **Tool Execution Details** |
| `Tab` | Composer | Accept `@` or `/` autocomplete; cycle primary agents if prompt is empty |
| `PgUp` / `PgDn` | Chat / Overlay | Scroll conversation history or active overlay |
| `Esc` | Global | Close overlay; **interrupt active agent generation** |
| `Ctrl+C` | Global | Cancel turn; exit application when idle |
| `Mouse Click` | UI | Focus regions, switch sessions, toggle tools, click status pills |
| `Mouse Wheel` | Viewports | Smoothly scroll whichever region is directly under the pointer |

---

### Context Injection & Mentions (`@`)

Type `@` in the prompt composer to trigger fuzzy suggestions:

- `@path/to/file` — Ephemerally attach file contents into model context (bounded reads).
- `@explorer <task>` — Launch an isolated read-only subagent to investigate code.
- `@coding <task>` — Launch an autonomous subagent for focused implementation.
- `@general <task>` — Launch a versatile subagent for multi-step tasks.
- `@skill:<name>` — Discover and activate specialized skill prompts.

---

### Slash Commands (`/`)

Type `/` in the prompt or press `Ctrl+P` to access commands:

| Command | Category | Description |
|---|---|---|
| `/connect` | Provider | Launch interactive provider connection wizard |
| `/provider` | Provider | Switch active provider |
| `/model` | Provider | Switch model for active session |
| `/key` | Provider | Set or update encrypted API key |
| `/thinking` | Reasoning | Toggle reasoning mode on/off |
| `/effort` | Reasoning | Set reasoning effort (`low`, `medium`, `high`, `xhigh`, `max`, `supercode`) |
| `/agent` | Agent | Switch primary agent (`build`, `coding`, `plan`) |
| `/diff` | Git | Open interactive visual diff viewer with per-file navigation |
| `/todos` | Tasks | View task progress and checklist items |
| `/context` | Context | Inspect token budget and context window utilization |
| `/compact` | Context | Trigger intelligent LLM-driven context compaction |
| `/sessions` | Session | Browse and switch between saved conversation sessions |
| `/new` | Session | Create a new isolated session |
| `/theme` | Interface | Switch active color palette (`tokyonight`, `nord`, `catppuccin`, etc.) |
| `/mouse` | Interface | Toggle mouse capture (`/mouse on`, `/mouse off`) |
| `/help` | System | View shortcuts and command reference |

---

## 🤖 Multi-Agent System

Uinxed-Agent features a hierarchical multi-agent architecture designed for autonomous, verified software development:

```text
                     ┌──────────────────┐
                     │   Primary Agent  │
                     │  (build/coding)  │
                     └────────┬─────────┘
                              │
               delegate (parallel & isolated)
         ┌────────────────────┼────────────────────┐
         ▼                    ▼                    ▼
  ┌──────────────┐     ┌──────────────┐     ┌──────────────┐
  │   explorer   │     │    coding    │     │   general    │
  │  (read-only) │     │ (expert dev) │     │  (multi-step)│
  └──────────────┘     └──────────────┘     └──────────────┘
```

### 1. Primary Agents (Interactive)
- **`build`** *(Default)*: Full tool access, interactive development, shell commands, and code editing.
- **`coding`**: Rigorous software engineer following the red-green-refactor loop: *Understand → Plan → Implement → Verify → Review*.
- **`plan`**: Read-only strategic analysis. Formulates implementation plans without mutating workspace files.

### 2. Subagents (Delegated)
- **`explorer`**: High-speed, read-only code exploration using grep, glob, and AST inspection.
- **`coding`**: Sandboxed expert coder handling independent modules or complex sub-features.
- **`general`**: Versatile worker for running benchmarks, compiling dependencies, or multi-step tasks.

### ⚡ Supercode Mode
Activate `supercode` effort via `/effort supercode` to enable high-concurrency multi-subagent orchestration. Complex prompts are automatically decomposed into parallel subagent workflows that explore, implement, and verify simultaneously.

---

## 🏗️ Architecture

Uinxed-Agent maintains a clean, decoupled boundary between presentation, orchestration, and execution:

```text
┌─────────────────────────────────────────────────────────────┐
│                       Bubble Tea v2 TUI                     │
│   Conversation View · Viewports · Overlays · Mouse Router   │
└──────────────────────────────┬──────────────────────────────┘
                               │ Typed UI Events (SSE deltas, tool events)
┌──────────────────────────────▼──────────────────────────────┐
│                    Application Controller                   │
│   Event Coalescing · Session Management · File Indexer      │
└──────────────┬───────────────────────────────┬──────────────┘
               │                               │
               │ Storage Ops                   ▼
               │                    ┌─────────────────────────┐
               │                    │      Session Store      │
               │                    │  SQLite WAL / JSONStore │
               ▼                    └─────────────────────────┘
┌─────────────────────────────────────────────────────────────┐
│                        Agent Runtime                        │
│   Turn Loop · Token Budgeting · Delegation · Compaction     │
└──────────────┬───────────────────────────────┬──────────────┘
               │                               │
               ▼                               ▼
  ┌─────────────────────────┐     ┌─────────────────────────┐
  │    Provider Gateway     │     │     Tool Registry       │
  │  OpenAI / Claude SSE    │     │ Shell, Files, Search,   │
  │  Connection Pooling     │     │ Web, Skills, Todos      │
  └─────────────────────────┘     └─────────────────────────┘
```

- **Zero Coupling**: `internal/agent` and `internal/tools` have zero dependencies on Bubble Tea or terminal UI libraries.
- **Resilient Tool Loop**: Automatic detection and healing of offset tool call streams (`index: 2, 3...`) and orphan responses.
- **Non-Blocking SSE Streaming**: Token streaming bypasses UI queue bottlenecks; high-frequency cumulative stdout chunks are smoothly coalesced to prevent terminal tearing.

---

## 🎨 Themes

Switch themes anytime with `/theme <name>`:

| Theme | Style & Aesthetic |
|---|---|
| **`tokyonight`** | Deep midnight blue with neon cyan & magenta accents |
| **`catppuccin`** | Warm, soothing pastel palette (Mocha variant) |
| **`nord`** | Arctic, elegant ice-blue & cool slate tones |
| **`gruvbox`** | Earthy, retro warm grooved colors |
| **`dracula`** | Classic high-contrast dark vampire theme |
| **`solarized`** | Precision calibrated low-contrast palette |
| **`monokai`** | Iconic vibrant code editor colors |
| **`uinxed`** | Native branded terminal aesthetic |

*Automatic terminal adaptability:*
- Set `NO_COLOR=1` or `TERM=dumb` for automatic color stripping.
- Unpatched terminals automatically fall back to clean ASCII glyphs (no Nerd Font required).

---

## 📊 Performance & Reliability

- **Cold Startup**: ~1ms process launch (`ux-agent --version`).
- **Memory Footprint**: Under 30MB base RSS during active sessions.
- **Zero Lockup**: Tool streams are bounded, cancellable via `Esc`, and isolated from parent session state.
- **SQLite WAL**: Ultra-fast ACID writes with zero CGO compilation dependencies.

Run the benchmark suite on your machine:
```bash
./scripts/benchmark.sh
```

---

## 🛠️ Development & Contributing

We welcome contributions! Ensure tests and formatting pass before submitting PRs:

```bash
# Format code
make fmt

# Run all unit tests
make test

# Run race detector
make race

# Full verification gate (fmt + test + race + vet + build)
make check
```

---

## 📄 License

Uinxed-Agent is licensed under the [Apache License 2.0](LICENSE).

<div align="center">
  <sub>Built with ❤️ by the ViudiraTech team.</sub>
</div>
