# Keyboard Input & Error Recovery

A three-mode state machine that routes keypresses and communicates user decisions to the orchestration goroutine via a channel.

- **Last Updated:** 2026-04-08 12:00
- **Authors:**
  - River Bailey

## Overview

- `KeyHandler` operates in three modes: Normal, Error, and QuitConfirm — each with its own keypress bindings and shortcut bar text
- User decisions are sent to the orchestration goroutine via a buffered `Actions` channel carrying `StepAction` values (Retry, Continue, Quit)
- In Normal mode, `n` terminates the current subprocess (skip step) and `q` enters quit confirmation
- In Error mode (entered when a step fails), `c` continues past the failure, `r` retries the step, and `q` enters quit confirmation
- `ForceQuit()` is a signal-safe method that terminates the subprocess and injects `ActionQuit` via non-blocking send

Key files:
- `ralph-tui/internal/ui/ui.go` — KeyHandler struct, mode dispatch, ForceQuit
- `ralph-tui/internal/ui/ui_test.go` — Unit tests for all modes and transitions

## Architecture

```
                    Keyboard Input
                         │
                         ▼
                  ┌──────────────┐
                  │  KeyHandler  │
                  │              │
          ┌───────┤  mode: Mode  ├───────┐
          │       │              │       │
          ▼       └──────────────┘       ▼
   ┌────────────┐                 ┌────────────────┐
   │ ModeNormal │──── q ────────▶│ModeQuitConfirm │
   │            │                 │                │
   │ n → cancel │                 │ y → ActionQuit │
   │   (skip)   │   ┌── n ──────│ n → prevMode   │
   └────────────┘   │            └────────────────┘
                    │                    ▲
   ┌────────────┐   │                    │
   │ ModeError  │───┼──── q ────────────┘
   │            │   │
   │ c → Action │   │
   │   Continue │   │
   │ r → Action │   │
   │   Retry    │   │
   └────────────┘   │
         ▲          │
         │          │
   step failure     │
   (set by          │
   Orchestrate)     │
                    │
                    ▼
            ┌──────────────┐
            │   Actions    │  buffered channel (cap 10)
            │   channel    │
            └──────┬───────┘
                   │
                   ▼
            Orchestrate()
            (workflow goroutine)
```

## Key Files

| File | Purpose |
|------|---------|
| `ralph-tui/internal/ui/ui.go` | KeyHandler struct, mode dispatch, ForceQuit, ShortcutLine |
| `ralph-tui/internal/ui/ui_test.go` | Tests for all modes, transitions, and ForceQuit |

## Core Types

```go
type StepAction int
const (
    ActionRetry    StepAction = iota
    ActionContinue
    ActionQuit
)

type Mode int
const (
    ModeNormal      Mode = iota
    ModeError
    ModeQuitConfirm
)

type KeyHandler struct {
    mode         Mode
    prevMode     Mode           // restored when quit confirm is cancelled
    cancel       func()         // terminates the current subprocess
    Actions      chan StepAction // communicates decisions to orchestration
    mu           sync.Mutex     // protects shortcutLine
    shortcutLine string         // current shortcut bar text
}
```

## Constants

| Constant | Value | Description |
|----------|-------|-------------|
| `NormalShortcuts` | `"↑/k up  ↓/j down  n next step  q quit"` | Shortcut bar in normal mode |
| `ErrorShortcuts` | `"c continue  r retry  q quit"` | Shortcut bar in error mode |
| `QuitConfirmPrompt` | `"Quit ralph? (y/n)"` | Shortcut bar in quit confirm mode |

## Implementation Details

### Mode Dispatch

`Handle` routes keypresses to the appropriate mode handler:

```go
func (h *KeyHandler) Handle(key string) {
    switch h.mode {
    case ModeNormal:      h.handleNormal(key)
    case ModeError:       h.handleError(key)
    case ModeQuitConfirm: h.handleQuitConfirm(key)
    }
}
```

### Normal Mode

- `n` — calls the `cancel` function to terminate the current subprocess (step skip)
- `q` — saves the current mode as `prevMode` and switches to `ModeQuitConfirm`
- All other keys are ignored

### Error Mode

Entered by `Orchestrate` when a step fails (via `h.SetMode(ModeError)`):

- `c` — sends `ActionContinue` to the `Actions` channel (step stays marked failed, advance to next)
- `r` — sends `ActionRetry` to the `Actions` channel (re-run the failed step)
- `q` — saves current mode and switches to `ModeQuitConfirm`

### Quit Confirm Mode

- `y` — sends `ActionQuit` to the `Actions` channel
- `n` — restores `prevMode` (returns to whichever mode initiated the quit)
- All other keys are ignored

### ForceQuit

`ForceQuit` is safe to call from a signal handler goroutine. It terminates the subprocess and injects `ActionQuit` using a non-blocking send:

```go
func (h *KeyHandler) ForceQuit() {
    if h.cancel != nil {
        h.cancel()
    }
    select {
    case h.Actions <- ActionQuit:
    default: // non-blocking: don't hang if channel is full
    }
}
```

### ShortcutLine Thread Safety

`ShortcutLine()` is a mutex-protected getter for the current shortcut bar text, safe to call from any goroutine (e.g., Glyph's render loop):

```go
func (h *KeyHandler) ShortcutLine() string {
    h.mu.Lock()
    defer h.mu.Unlock()
    return h.shortcutLine
}
```

The shortcut line is updated internally by `updateShortcutLine()` whenever the mode changes.

## Testing

- `ralph-tui/internal/ui/ui_test.go` — Tests for all key handlers in each mode, mode transitions, quit confirm with cancel, ForceQuit, ShortcutLine thread safety

## Related Documentation

- [Architecture Overview](../architecture.md) — Keyboard & mode state machine diagram
- [Workflow Orchestration](workflow-orchestration.md) — How Actions channel drives the orchestration loop
- [Signal Handling & Shutdown](signal-handling.md) — How ForceQuit is triggered by OS signals
- [TUI Status Header](tui-display.md) — How the shortcut bar is displayed alongside the status header
