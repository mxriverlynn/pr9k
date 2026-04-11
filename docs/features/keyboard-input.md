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
- `ralph-tui/internal/ui/ui.go` — KeyHandler struct, mode dispatch, ForceQuit
- `ralph-tui/internal/ui/ui_test.go` — Unit tests for all modes and transitions

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
| `ralph-tui/internal/ui/ui.go` | KeyHandler struct, mode dispatch, ForceQuit, ShortcutLine, ShortcutLinePtr |
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
    ModeQuitting    // confirmed quit; footer shows "Quitting..." during shutdown
)

type KeyHandler struct {
    mode         Mode
    prevMode     Mode           // restored when quit confirm is cancelled
    cancel       func()         // terminates the current subprocess
    Actions      chan StepAction // communicates decisions to orchestration
    mu           sync.Mutex     // protects mode, prevMode, and shortcutLine
    shortcutLine string         // protected by mu; use ShortcutLine() or ShortcutLinePtr() to access
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

`Handle` routes keypresses to the appropriate mode handler:

```go
func (h *KeyHandler) Handle(key string) {
    h.mu.Lock()
    mode := h.mode
    h.mu.Unlock()

    switch mode {
    case ModeNormal:      h.handleNormal(key)
    case ModeError:       h.handleError(key)
    case ModeQuitConfirm: h.handleQuitConfirm(key)
    }
    // ModeQuitting is a terminal state — no handler; keypresses are ignored
    // while the shutdown unwinds.
}
```

`main.go` registers these keys with Glyph: `"n"`, `"q"`, `"y"`, `"c"`, `"r"`, and `"<Escape>"`. Each registered key forwards to `keyHandler.Handle(key)`.

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

- `y` — flips the mode to `ModeQuitting` (so the footer immediately shows `Quitting...` as visible feedback) and calls `ForceQuit()`, which terminates the active subprocess and injects `ActionQuit` into the Actions channel
- `n` — restores `prevMode` (returns to whichever mode initiated the quit)
- `<Escape>` — same as `n`: restores `prevMode` and cancels the quit without firing `ForceQuit` or sending any action
- All other keys are ignored

The flip to `ModeQuitting` happens **before** `ForceQuit` is called so the footer paints `Quitting...` on the very next render cycle, before the orchestration goroutine starts unwinding. This is the only mode that exists purely for user feedback — no keypresses are processed from `ModeQuitting` because the state machine will either terminate the process (signal path) or the workflow goroutine will close the executor and return from `Run` (normal path).

### Quitting Mode

Entered by the QuitConfirm `y` path or by `ForceQuit()` directly (which is called by the OS signal handler from any mode, including Normal and Error). The footer shows `QuittingLine` (`"Quitting..."`). No keypress handler is registered for this mode; any keypresses received while `mode == ModeQuitting` fall through `Handle`'s switch and are ignored. The mode persists until the workflow goroutine unwinds and tears the TUI down.

### Normal Completion (no mode transition)

When the workflow finishes all iterations and finalize steps successfully, `Run` writes the completion summary line to the log body and returns on its own. There is no dedicated "done" mode — the workflow goroutine in `main.go` calls `app.Screen().ExitRawMode()` and `os.Exit(0)` directly. Exiting from the workflow goroutine rather than through `app.Run` avoids a macOS raw-tty quirk where closing stdin from another goroutine doesn't reliably unblock Glyph's in-progress `ReadKey`.

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

Two accessors expose the shortcut bar text for different callers:

**`ShortcutLine()`** is a mutex-protected getter, safe to call from any goroutine (e.g., the orchestration goroutine, the signal handler):

```go
func (h *KeyHandler) ShortcutLine() string {
    h.mu.Lock()
    defer h.mu.Unlock()
    return h.shortcutLine
}
```

**`ShortcutLinePtr()`** returns a `*string` pointing to the underlying field for Glyph's `Text(&...)` pointer-binding API:

```go
func (h *KeyHandler) ShortcutLinePtr() *string {
    return &h.shortcutLine
}
```

`ShortcutLinePtr()` is intended exclusively for Glyph's single-threaded event loop, which reads the pointer synchronously between write windows. It bypasses the mutex and must not be called from concurrent goroutines.

The shortcut line is updated internally by `updateShortcutLineLocked()` whenever the mode changes.

> **Why Option Q?** Option P (exporting `ShortcutLine` as a field, dropping the mutex) was attempted first but `go test -race` detected a genuine race between the `Orchestrate` goroutine writing via `SetMode` and the test goroutine reading the field concurrently. Option Q retains the private field and mutex for `ShortcutLine()`, and adds `ShortcutLinePtr()` for Glyph's pointer-binding path.

## Testing

- `ralph-tui/internal/ui/ui_test.go` — Tests for all key handlers in each mode, mode transitions, quit confirm with cancel (`n` and `<Escape>` from both Normal and Error), `y` flipping to `ModeQuitting` with `QuittingLine` footer, `SetMode(ModeQuitting)` updating the shortcut bar, ForceQuit (cancel fires, ActionQuit sent, idempotent, nil-cancel-no-panic, full-channel-no-panic, `TestForceQuit_SetsModeQuitting_FromNormal`, `TestForceQuit_SetsModeQuitting_FromError`), ShortcutLine thread safety, ShortcutLinePtr (non-nil return, value tracking, stable address, agreement with ShortcutLine)

## Additional Information

- [Architecture Overview](../architecture.md) — Keyboard & mode state machine diagram
- [Workflow Orchestration](workflow-orchestration.md) — How Actions channel drives the orchestration loop
- [Signal Handling & Shutdown](signal-handling.md) — How ForceQuit is triggered by OS signals
- [Subprocess Execution & Streaming](subprocess-execution.md) — How Terminate stops the active subprocess
- [TUI Status Header](tui-display.md) — How the shortcut bar is displayed alongside the status header
- [Concurrency](../coding-standards/concurrency.md) — Coding standards for non-blocking sends, channel dispatch, and mutex-protected getters
