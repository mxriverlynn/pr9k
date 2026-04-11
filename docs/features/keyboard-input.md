# Keyboard Input & Error Recovery

A four-mode state machine that routes keypresses and communicates user decisions to the orchestration goroutine via a channel.

- **Last Updated:** 2026-04-11
- **Authors:**
  - River Bailey

## Overview

- `KeyHandler` operates in four modes: Normal, Error, QuitConfirm, and Quitting — each with its own keypress bindings and shortcut bar text
- User decisions are sent to the orchestration goroutine via a buffered `Actions` channel carrying `StepAction` values (Retry, Continue, Quit)
- In Normal mode, `n` terminates the current subprocess (skip step) and `q` enters quit confirmation
- In Error mode (entered when a step fails), `c` continues past the failure, `r` retries the step, and `q` enters quit confirmation
- In QuitConfirm mode, `y` flips to the `Quitting` mode (footer shows `Quitting...`) and calls `ForceQuit`; `n` or `<Escape>` cancel and restore the previous mode
- In Quitting mode the footer shows `Quitting...` as visible confirmation that the user's quit was accepted while the orchestration goroutine unwinds
- `ForceQuit()` is a signal-safe method that terminates the subprocess and injects `ActionQuit` via non-blocking send — it is called both by the OS signal handler (SIGINT/SIGTERM) and by the QuitConfirm `y` path, so both paths produce identical shutdown behavior
- When the workflow finishes normally, `Run` returns on its own (no "press any key to exit" state); the workflow goroutine in `main.go` restores the terminal and exits the process directly

Key files:
- `ralph-tui/internal/ui/ui.go` — KeyHandler struct, mode state, ForceQuit, ShortcutLine
- `ralph-tui/internal/ui/keys.go` — keysModel Bubble Tea sub-model, Update dispatch to mode handlers
- `ralph-tui/internal/ui/ui_test.go` — Unit tests for KeyHandler modes and transitions
- `ralph-tui/internal/ui/keys_test.go` — Unit tests for keysModel.Update routing

## Architecture

```
                    Keyboard Input
                         │
                         ▼
                  ┌──────────────┐
                  │  KeyHandler  │
                  └──────┬───────┘
                         │
             ┌───────────┴───────────┐
             │                       │
             ▼                       ▼
      ┌────────────┐           ┌────────────┐
      │ ModeNormal │           │ ModeError  │
      │            │           │            │
      │ n → cancel │           │ c → cont.  │
      │   (skip)   │           │ r → retry  │
      │ q ───┐     │           │ q ───┐     │
      └──────┼─────┘           └──────┼─────┘
             │                        │
             └──────────┬─────────────┘
                        ▼
            ┌───────────────────────┐
            │   ModeQuitConfirm     │
            │                       │
            │  y → ModeQuitting +   │
            │      ForceQuit        │
            │  n, <Escape> → prev   │
            └───────────┬───────────┘
                        │
                        │ y
                        ▼
                 ┌──────────────┐
                 │ ModeQuitting │
                 │              │
                 │ footer shows │
                 │ "Quitting..."│
                 │ (terminal)   │
                 └──────┬───────┘
                        │
                        │ ForceQuit →
                        ▼
                 ┌──────────────┐
                 │   Actions    │  buffered channel (cap 10)
                 │   channel    │
                 └──────┬───────┘
                        │
                        ▼
                 Orchestrate()
                 (workflow goroutine)

  OS Signal (SIGINT/SIGTERM):
    → signal handler goroutine
    → keyHandler.ForceQuit()
    → cancel subprocess + inject ActionQuit
    (unified with the QuitConfirm 'y' path)

  Normal completion:
    → Run returns on its own after writing the
      completion summary to the log body
    → workflow goroutine restores the terminal
      and os.Exit(0)s directly
```

## Key Files

| File | Purpose |
|------|---------|
| `ralph-tui/internal/ui/ui.go` | KeyHandler struct, mode state, ForceQuit, ShortcutLine |
| `ralph-tui/internal/ui/keys.go` | keysModel Bubble Tea sub-model; Update dispatches tea.KeyMsg to mode handlers |
| `ralph-tui/internal/ui/ui_test.go` | Tests for KeyHandler modes, transitions, and ForceQuit |
| `ralph-tui/internal/ui/keys_test.go` | Tests for keysModel.Update routing (normal, error, quit-confirm, quitting) |

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
    ModeQuitting    // confirmed quit; footer shows "Quitting..." during shutdown
)

type KeyHandler struct {
    mode         Mode
    prevMode     Mode           // restored when quit confirm is cancelled
    cancel       func()         // terminates the current subprocess
    Actions      chan StepAction // communicates decisions to orchestration
    mu           sync.Mutex     // protects mode, prevMode, and shortcutLine
    shortcutLine string         // protected by mu; use ShortcutLine() to access
}
```

## Constants

| Constant | Value | Description |
|----------|-------|-------------|
| `NormalShortcuts` | `"↑/k up  ↓/j down  n next step  q quit"` | Shortcut bar in normal mode |
| `ErrorShortcuts` | `"c continue  r retry  q quit"` | Shortcut bar in error mode |
| `QuitConfirmPrompt` | `"Quit ralph? (y/n, esc to cancel)"` | Shortcut bar in quit confirm mode |
| `QuittingLine` | `"Quitting..."` | Shortcut bar in quitting mode (visible while shutdown unwinds) |

## Implementation Details

### Mode Dispatch

`keysModel.Update` (in `keys.go`) receives `tea.KeyMsg` events from the Bubble Tea event loop and routes them to the appropriate mode handler:

```go
func (m keysModel) Update(msg tea.Msg) (keysModel, tea.Cmd) {
    key, ok := msg.(tea.KeyMsg)
    if !ok {
        return m, nil
    }
    switch m.handler.Mode() {
    case ModeNormal:      return m.handleNormal(key)
    case ModeError:       return m.handleError(key)
    case ModeQuitConfirm: return m.handleQuitConfirm(key)
    case ModeQuitting:
        // All keys silently ignored so a user mashing keys during shutdown
        // can't inject a second ActionQuit or retrigger the cancel hook.
        return m, nil
    }
    return m, nil
}
```

The Bubble Tea program delivers all keypresses as `tea.KeyMsg` to `Model.Update`, which routes them to `keysModel.Update`. No separate key registration is required.

> **Implementation note:** Mode transitions in `handleNormal`, `handleError`, and `handleQuitConfirm` access `KeyHandler`'s unexported fields directly (`handler.mu`, `handler.mode`, `handler.prevMode`, `handler.updateShortcutLineLocked()`). Both types live in the same `ui` package, so this is valid Go — `keysModel` is an intentional package-internal collaborator of `KeyHandler`, not a general caller.

### Normal Mode

- `n` — calls the `cancel` function to terminate the current subprocess (step skip)
- `q` — saves the current mode as `prevMode` and switches to `ModeQuitConfirm` (direct field write under `handler.mu`)
- All other keys are ignored

### Error Mode

Entered by `Orchestrate` when a step fails (via `h.SetMode(ModeError)`):

- `c` — sends `ActionContinue` to the `Actions` channel (step stays marked failed, advance to next)
- `r` — sends `ActionRetry` to the `Actions` channel (re-run the failed step)
- `q` — saves current mode and switches to `ModeQuitConfirm`

### Quit Confirm Mode

- `y` — flips the mode to `ModeQuitting` (so the footer immediately shows `Quitting...` as visible feedback) and calls `ForceQuit()`, which terminates the active subprocess and injects `ActionQuit` into the Actions channel
- `n` — restores `prevMode` (returns to whichever mode initiated the quit)
- `<Escape>` — same as `n`: restores `prevMode` and cancels the quit without firing `ForceQuit` or sending any action
- All other keys are ignored

The flip to `ModeQuitting` happens **before** `ForceQuit` is called so the footer paints `Quitting...` on the very next render cycle, before the orchestration goroutine starts unwinding. This is the only mode that exists purely for user feedback — no keypresses are processed from `ModeQuitting` because the state machine will either terminate the process (signal path) or the workflow goroutine will close the executor and return from `Run` (normal path).

### Quitting Mode

Entered by the QuitConfirm `y` path or by `ForceQuit()` directly (which is called by the OS signal handler from any mode, including Normal and Error). The footer shows `QuittingLine` (`"Quitting..."`). No keypress handler is registered for this mode; any keypresses received while `mode == ModeQuitting` fall through `Handle`'s switch and are ignored. The mode persists until the workflow goroutine unwinds and tears the TUI down.

### Normal Completion (no mode transition)

When the workflow finishes all iterations and finalize steps successfully, `Run` writes the completion summary line to the log body and returns on its own. There is no dedicated "done" mode — the workflow goroutine in `main.go` calls `program.Quit()` after `workflow.Run` returns, which causes `program.Run()` to return cleanly in `main`.

### ForceQuit

`ForceQuit` is safe to call from a signal handler goroutine. It terminates the subprocess and injects `ActionQuit` using a non-blocking send. It is called from two places — the OS signal handler (SIGINT/SIGTERM) and the QuitConfirm `y` path — so both quit paths produce identical shutdown semantics (subprocess terminated, ActionQuit injected, orchestration unwinds):

```go
func (h *KeyHandler) ForceQuit() {
    h.mu.Lock()
    h.mode = ModeQuitting
    h.updateShortcutLineLocked()
    h.mu.Unlock()

    if h.cancel != nil {
        h.cancel()
    }
    select {
    case h.Actions <- ActionQuit:
    default: // non-blocking: don't hang if channel is full
    }
}
```

### Mode Accessor

**`Mode()`** is a mutex-protected getter that returns the current dispatch mode, safe to call from any goroutine:

```go
func (h *KeyHandler) Mode() Mode {
    h.mu.Lock()
    defer h.mu.Unlock()
    return h.mode
}
```

Used by tests to assert mode transitions without accessing private fields, and may be used by any goroutine that needs to inspect handler state (e.g., to gate UI rendering decisions).

### ShortcutLine Thread Safety

**`ShortcutLine()`** is a mutex-protected getter, safe to call from any goroutine (e.g., the signal handler, test goroutines, and `Model.View()` on the Bubble Tea Update goroutine):

```go
func (h *KeyHandler) ShortcutLine() string {
    h.mu.Lock()
    defer h.mu.Unlock()
    return h.shortcutLine
}
```

The shortcut line is updated internally by `updateShortcutLineLocked()` whenever the mode changes. `Model.View()` calls `ShortcutLine()` directly to read the current text for the footer.

### Dual-Routing of tea.KeyMsg

When a `tea.KeyMsg` arrives, `Model.Update` routes it to **both** the key handler and the viewport:

```go
case tea.KeyMsg:
    m.keys, kcmd = m.keys.Update(msg)   // mode dispatch (n, q, c, r, y, Escape)
    m.log, lcmd = m.log.Update(msg)     // viewport scroll (↑/k, ↓/j)
```

This means scroll keys (`↑`/`k`/`↓`/`j`) work during Normal mode — the viewport consumes them while the key handler ignores them. In Error and QuitConfirm modes, the key handler consumes the action keys but scroll keys still pass through to the viewport.

## Testing

- `ralph-tui/internal/ui/ui_test.go` — Tests for all key handlers in each mode, mode transitions, quit confirm with cancel (`n` and `<Escape>` from both Normal and Error), `y` flipping to `ModeQuitting` with `QuittingLine` footer, `SetMode(ModeQuitting)` updating the shortcut bar, ForceQuit (cancel fires, ActionQuit sent, idempotent, nil-cancel-no-panic, full-channel-no-panic, `TestForceQuit_SetsModeQuitting_FromNormal`, `TestForceQuit_SetsModeQuitting_FromError`), ShortcutLine thread safety

## Additional Information

- [Architecture Overview](../architecture.md) — Keyboard & mode state machine diagram
- [Workflow Orchestration](workflow-orchestration.md) — How Actions channel drives the orchestration loop
- [Signal Handling & Shutdown](signal-handling.md) — How ForceQuit is triggered by OS signals
- [Subprocess Execution & Streaming](subprocess-execution.md) — How Terminate stops the active subprocess
- [TUI Status Header](tui-display.md) — How the shortcut bar is displayed alongside the status header
- [Concurrency](../coding-standards/concurrency.md) — Coding standards for non-blocking sends, channel dispatch, and mutex-protected getters
