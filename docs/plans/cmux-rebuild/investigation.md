# cmux + pr9k Integration — Investigation

This document captures two rounds of investigation into how pr9k could integrate with [cmux](https://cmux.com/). It was produced before any feature specification work; the feature spec is sequenced separately in this same folder.

The investigation answered two distinct questions:

1. **Can pr9k host cmux *inside* its own TUI?** (Embedding direction.)
2. **Can pr9k *drive* cmux to recreate the pr9k TUI layout as cmux panes?** (Inverted direction.)

Round 1 ruled out (1). Round 2 sketched (2) and identified it as the only viable shape for a "pr9k + cmux" product.

---

## Round 1 — Can pr9k embed cmux?

### What cmux actually is

cmux is a **native GUI application** (built on the Ghostty terminal renderer) that draws pixels into its own OS window. It started as a macOS app; there are community Linux and Windows ports, but the shape is the same: a windowed app, not a library or a terminal program.

It exposes a Unix socket API at `/tmp/cmux.sock` (production) or `/tmp/cmux-debug.sock` (debug), overridable via `CMUX_SOCKET_PATH`. The protocol is **newline-terminated JSON-RPC 2.0**:

- Request: `{"id":"<request-id>","method":"<method>","params":{...}}\n`
- Response: `{"id":"<request-id>","ok":true,"result":{...}}`

### cmux API surface (verbatim from cmux docs)

| Category | Methods |
|----------|---------|
| **Workspaces** | `workspace.list`, `workspace.create`, `workspace.select`, `workspace.current`, `workspace.close` |
| **Surfaces** | `surface.split`, `surface.list`, `surface.focus` |
| **Input** | `surface.send_text`, `surface.send_key` |
| **Notifications** | `notification.create`, `notification.list`, `notification.clear` |
| **Sidebar** | `set_status`, `clear_status`, `list_status`, `set_progress`, `clear_progress`, `log`, `clear_log`, `list_log`, `sidebar_state` |
| **System** | `system.ping`, `system.capabilities`, `system.identify` |

### cmux auth model

Three access modes:
- **Off** — socket disabled.
- **cmux processes only** (default) — only child processes of cmux can connect.
- **allowAll** — any local process allowed.

No credential-based auth; ancestry-based on Unix socket permissions.

### What the cmux API does *not* offer

- **No way to read the contents of a pane** — no screen scrape, no pane stdout subscription.
- **No PTY stream** that an external program can consume.
- **No screen-capture / framebuffer / pixel access.**
- **No headless or embeddable rendering mode.**

cmux *is* the renderer. The API is control-plane only — it lets you create panes, focus them, send keys into them, and post sidebar/notification metadata. It does not let you observe rendered output or run cmux without its window.

### pr9k's main content area today

pr9k's body — the area between the checkbox header and the footer — is a `bubbles/viewport` over a **2000-line ring buffer** of **plain text with ANSI color codes only**.

- Rendered in `src/internal/ui/model.go` `View()` (lines 415–494) and `src/internal/ui/log_panel.go`.
- Lines arrive via `os.Pipe` from subprocesses, get word-wrapped by `ansi.Wrap`, and are appended as strings.
- The viewport renders with `logContentStyle` (foreground `White`) on top of the buffered content.

### pr9k has no PTY

Grepping the codebase for `pty`, `creack/pty`, `forkpty`, `ConPTY`, `term` returns nothing. Subprocess I/O is handled exclusively via `os.Pipe()` (stdout/stderr file descriptors). Consequences:

- Subprocesses run in non-interactive mode; they cannot use cursor-movement, alt-screen, or interactive prompts.
- Output is line-buffered text; no raw escape sequences for cursor/screen manipulation.
- pr9k's own TUI uses Bubble Tea's alt-screen mode (`tea.WithAltScreen()`), but that's separate from subprocess terminal allocation.

### How subprocess output reaches the TUI

1. `workflow.Runner.runCommand()` (`src/internal/workflow/workflow.go` ~line 432) calls `exec.Command()` and creates `stdout`/`stderr` pipes.
2. The runner reads lines via a scanner and forwards each to a `sendLine` callback.
3. In `main.go` (~line 273), `runner.SetSender()` deposits each line into a buffered channel `lineCh`.
4. A drain goroutine batches consecutive lines and sends `LogLinesMsg` to the Bubble Tea program.
5. `Model.Update()` (~line 232) receives `LogLinesMsg` and forwards to `logModel.Update()`, which appends to the ring buffer and re-wraps.

For Claude steps, output is additionally parsed via `claudestream.Pipeline.Observe()` (NDJSON parser/renderer/aggregator) before reaching the TUI.

### The one existing "embedded interactive content" pattern

The workflow builder's external-editor invocation uses Bubble Tea's `tea.ExecProcess` (`src/cmd/pr9k/workflow.go` lines 34–59). It:

- **Suspends the TUI** (restores cooked terminal).
- Launches the editor in the user's native terminal.
- **Resumes the TUI** when the process exits.
- Dispatches an `ExecCallback` message with the exit error.

There is **no pattern in pr9k for embedding a live interactive subprocess inside the viewport.**

### Two independent blockers (either one fatal)

1. **cmux does not expose its rendered output.** Even if pr9k were a full terminal emulator, there is no API to subscribe to a cmux surface's screen contents. cmux assumes *it* is the visible surface.
2. **pr9k is not a terminal emulator.** Even if cmux did stream pane bytes, pr9k's viewport can only render colored text. To host a live cmux pane you would need to embed something like `vt10x` or a stripped-down Ghostty inside the viewport — a substantial rewrite of `internal/ui/log_panel.go` plus PTY plumbing.

### Round 1 verdict

Embedding cmux windows inside pr9k's TUI is **not possible** without rewriting cmux, pr9k, or both.

---

## Round 2 — Can pr9k drive cmux to render the pr9k layout?

This flips the architecture: pr9k becomes a **controller plus a small set of display processes**, and cmux becomes the chrome.

### The core mental model

A cmux surface is a terminal pane running a process. cmux does not let you "paint" a surface from outside — `surface.send_text` and `surface.send_key` send to the program's stdin, not to its screen. So to make a surface show "the pr9k checkbox grid," **something has to actually be running in that surface that draws the grid**.

```
pr9k --cmux  (the CLI you type)
  │
  ├── 1) Talks to cmux socket → creates workspace + 3 panes
  ├── 2) Spawns a display process inside each pane
  ├── 3) Runs the workflow orchestrator (the existing Run loop)
  └── 4) Pipes orchestrator events to the displays over a local IPC socket
```

Three panes, three small Bubble Tea programs, one orchestrator. cmux owns the borders, splits, focus, and sidebar; pr9k owns the content of each pane and the workflow logic.

### What lives where

**Pane 1 (top, small height): header display.** Standalone subcommand — `pr9k display header` — connects to a local socket and renders only the checkbox grid + iteration line. On every step state change, the orchestrator pushes a JSON message; the header redraws.

**Pane 2 (main, large): log display.** `pr9k display log` — receives streaming log lines from the orchestrator and runs the existing viewport / ring-buffer / word-wrap logic. Most of the current `log_panel.go` lifts and shifts.

**Pane 3 (bottom, small): footer display.** `pr9k display footer` — status line + shortcut bar. Note: cmux has a **native sidebar** (`set_status`, `set_progress`, `log`) and that is arguably the more idiomatic home for status info; the footer could shrink to just the shortcut bar.

**The orchestrator.** Everything in `internal/workflow`, `internal/steps`, `internal/claudestream`, `internal/sandbox` as it stands today — unchanged. The only difference is that instead of pushing `LogLinesMsg` and header-state messages into a Bubble Tea program in the same process, it publishes those same events over a Unix socket to whatever displays are listening. The orchestrator could live in a fourth hidden pane, or as a detached process spawned by the CLI invocation. Hidden-pane is cleanest because cmux owns its lifecycle.

### New components required

1. **A cmux client (`internal/cmuxctl`).** Small Go package speaking JSON-RPC 2.0 over `/tmp/cmux.sock`: `workspace.create`, `surface.split`, `surface.focus`, `set_status`, `set_progress`, `log`, `notification.create`, `workspace.close`. Rough estimate: 200–400 lines.

2. **An IPC layer (`internal/displayipc`).** Server in the orchestrator, client in each display. Pub/sub over a Unix socket with newline-delimited JSON. Event types: `HeaderState`, `LogLines`, `FooterState`, `StatusLine`, `Shutdown`. Rough estimate: 300 lines.

3. **Three display subcommands.** `pr9k display header|log|footer`. Each is a tiny Bubble Tea program: connect to socket, receive events, render. Existing rendering code from `internal/ui/log_panel.go` and the header/footer halves of `internal/ui/model.go` would be extracted into per-display packages.

4. **A new top-level entrypoint** — `pr9k --cmux` or `pr9k cmux run`. Pings cmux, creates workspace, splits surfaces, launches displays, starts orchestrator, waits, tears down on exit.

### Reuse from today's code

The workflow engine, claude streaming parser, Docker sandbox, validator, variable substitution — all unchanged. The render-side code in `internal/ui/log_panel.go` (viewport + ring buffer + ANSI-aware wrap) moves into the log-display subcommand mostly verbatim.

### The hard parts

#### Input routing

Today's pr9k has an 8-mode keyboard state machine (Normal / Error / QuitConfirm / NextConfirm / Done / Select / Quitting / Help). When the user presses `q`, the model knows whether to quit, confirm, or cancel a selection. In a three-pane layout, **the user's keystrokes go to whichever cmux pane has focus**. Two options:

- **The footer pane is the "control pane"** — it owns the keyboard, and other panes are read-only. Add a visible hint ("focus the bottom pane for shortcuts"). Simpler.
- **Every display handles its own keys and ships intents** (`QuitRequested`, `ContinueRequested`, etc.) back to the orchestrator via the IPC socket. More honest but more work — effectively re-implementing the state machine as a distributed protocol.

Recommend starting with the first.

#### Selection and copy

Today pr9k has a custom selection layer because alt-screen + mouse-cell-motion mode breaks the terminal's native drag-to-select. In cmux, **each pane has its own native selection** (Ghostty handles selection itself). The custom selection layer **gets deleted**. You can't drag-select across panes, but you also don't have to maintain that code. Net win.

#### Error / confirmation modals

Today these are overlays in the same Bubble Tea program. In cmux you would render them inside one of the panes (probably the footer or the log), or use `notification.create` for non-blocking ones. Workable but needs design.

#### External editor

Today `tea.ExecProcess` suspends pr9k entirely. In a cmux world, you just open a new cmux surface for the editor — `surface.split`, run `$EDITOR`, wait for exit, `surface.close`. Cleaner than the current flow.

#### Lifecycle

Quitting pr9k has to also close the cmux workspace and kill the display processes. Need a `defer workspace.close()` plus signal handling so Ctrl+C in the orchestrator tears everything down.

#### Initial sizing

Each display still gets a `WindowSizeMsg` from its own pane (cmux is a real terminal as far as the display process is concerned), so all the existing word-wrap and viewport-resize logic still works. No change needed.

#### Synchronization between panes

Because the three displays are separate processes, an "iteration ticks over" moment will hit the header and the footer at slightly different times — a few milliseconds. Cosmetically invisible in practice, but it is a real change from today's atomic redraws.

### What is lost

- **Platform portability.** Today's pr9k runs in any terminal. The cmux mode would only work where cmux runs (mac primarily; community Linux/Windows ports). Opt-in alongside the existing TUI, not a replacement.
- **Single-process simplicity.** Four processes instead of one. Logs and crash diagnostics spread out.
- **Cross-pane mouse selection.** Already noted.

### What is gained

- **Drag-resizable panes** — cmux feature, free.
- **Native cmux sidebar integration** — status, progress, log entries showing up in cmux's persistent UI. Notifications when a run finishes or fails. This is what cmux was built for.
- **A natural place to spawn more surfaces** — e.g., a side pane showing the iteration JSONL; or a browser pane for the resulting PR (cmux has Playwright integration built in).
- **Cleaner external-editor flow** — no TUI suspension.

### Effort estimate

Not a flag-flip. Realistically: a cmux client package, an IPC layer, three display subcommands extracted from `internal/ui`, a new entrypoint, signal/lifecycle wiring, plus design work on input routing. Rough estimate: **two to three weeks of focused work** for one person, ending with `pr9k --cmux` as an opt-in alternative to the existing TUI.

### A smaller starter step

Keep pr9k's existing TUI exactly as it is, but have it call `set_status`, `set_progress`, `log`, and `notification.create` on the cmux socket when running inside cmux. Roughly 300 lines of code. Gets sidebar integration without touching rendering — proves the cmux relationship works before committing to the three-pane rewrite.

---

## Sources

- [cmux — The terminal built for multitasking](https://cmux.com/)
- [cmux API Reference](https://cmux.com/docs/api)
- [One Open Source Project a Day (No. 54): cmux](https://dev.to/wonderlab/one-open-source-project-a-day-no-54-cmux-a-native-multiplexer-for-the-ai-agent-era-1gij)
- [cmux vs tmux — Agent Terminal vs Terminal Multiplexer (2026)](https://soloterm.com/cmux-vs-tmux)
