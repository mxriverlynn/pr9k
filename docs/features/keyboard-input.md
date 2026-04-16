# Keyboard Input & Error Recovery

A seven-mode state machine that routes keypresses and communicates user decisions to the orchestration goroutine via a channel.

- **Last Updated:** 2026-04-16
- **Authors:**
  - River Bailey

## Overview

- `KeyHandler` operates in seven modes: Normal, Error, QuitConfirm, NextConfirm, Done, Select, and Quitting — each with its own keypress bindings and shortcut bar text
- User decisions are sent to the orchestration goroutine via a buffered `Actions` channel carrying `StepAction` values (Retry, Continue, Quit)
- In Normal mode, `n` enters the skip confirmation prompt (NextConfirm) and `q` enters quit confirmation
- In NextConfirm mode (entered when the user presses `n` during a running step), `y` terminates the current subprocess (skip step), `n` or `<Escape>` cancel and restore the previous mode
- In Error mode (entered when a step fails), `c` continues past the failure, `r` retries the step, and `q` enters quit confirmation
- In QuitConfirm mode, `y` flips to the `Quitting` mode (footer shows `Quitting...`), calls `ForceQuit`, and returns `tea.QuitMsg` to exit the TUI; `n` or `<Escape>` cancel and restore the previous mode
- In Done mode (entered when the workflow completes), the TUI stays alive so the user can review output; `q` enters quit confirmation; `v` enters `ModeSelect`
- In Select mode (entered when `v` is pressed from Normal or Done), the cursor is shown as a reverse-video cell in the log panel; `Esc` clears the selection and returns to the prior mode; cursor movement and copy land in later tickets (#105+)
- In Quitting mode the footer shows `Quitting...` as visible confirmation that the user's quit was accepted while the orchestration goroutine unwinds
- `ForceQuit()` is a signal-safe method that terminates the subprocess and injects `ActionQuit` via non-blocking send — it is called both by the OS signal handler (SIGINT/SIGTERM) and by the QuitConfirm `y` path, so both paths produce identical shutdown behavior

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
      │ n ───┐     │           │ c → cont.  │
      │ q ───┼─┐   │           │ r → retry  │
      └──────┼─┼───┘           │ q ───┐     │
             │ │               └──────┼─────┘
             │ └──────────┬───────────┘
             │            ▼
             │  ┌───────────────────────┐
             │  │   ModeQuitConfirm     │
             │  │                       │
             │  │  y → ModeQuitting +   │
             │  │      ForceQuit +      │
             │  │      tea.QuitMsg      │
             │  │  n, <Escape> → prev   │
             │  └───────────┬───────────┘
             │              │
             ▼              │ y
    ┌──────────────────┐    ▼
    │ ModeNextConfirm  │  ┌──────────────┐
    │                  │  │ ModeQuitting │
    │ "Skip current    │  │              │
    │  step? (y/n,     │  │ footer shows │
    │  esc to cancel)" │  │ "Quitting..."│
    │                  │  │ (terminal)   │
    │ y → cancel step  │  └──────┬───────┘
    │ n, esc → prev    │         │
    └──────────────────┘         │ ForceQuit →
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
    → workflow goroutine enters ModeDone
    → TUI stays alive; user reviews output
    → v → ModeSelect (selection cursor in log panel)
    → q → QuitConfirm → y → tea.QuitMsg exits TUI

                 ┌──────────────┐
                 │   ModeDone   │
                 │              │
                 │ v → ModeSelect
                 │ q → QuitConfirm
                 └──────────────┘    → y → tea.QuitMsg

  ┌─────────────────────────────────────────────┐
  │                ModeSelect                   │
  │                                             │
  │  Entered by v from ModeNormal or ModeDone   │
  │  Shows reverse-video cursor cell in log     │
  │  Esc → clears selection, returns prevMode   │
  │  q → ModeQuitConfirm                        │
  │  Cursor movement / copy land in #105+       │
  └─────────────────────────────────────────────┘
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
    ModeNextConfirm // entered after n; shows "Skip current step?" prompt
    ModeDone        // entered after workflow completes; shows "v select  q quit" footer
    ModeSelect      // entered by v from Normal/Done; shows selection cursor overlay
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
| `AppTitle` | `"Power-Ralph.9000"` | Canonical display name; single source of truth for the user-facing app name in titles and prompts |
| `NormalShortcuts` | `"↑/k up  ↓/j down  v select  n next step  q quit"` | Shortcut bar in normal mode |
| `ErrorShortcuts` | `"c continue  r retry  q quit"` | Shortcut bar in error mode |
| `QuitConfirmPrompt` | `"Quit " + AppTitle + "? (y/n, esc to cancel)"` | Shortcut bar in quit confirm mode |
| `NextConfirmPrompt` | `"Skip current step? (y/n, esc to cancel)"` | Shortcut bar in next-confirm mode |
| `DoneShortcuts` | `"↑/k up  ↓/j down  v select  q quit"` | Shortcut bar in done mode (post-workflow) |
| `SelectShortcuts` | `"hjkl/↑↓←→ extend  0/$ line  ⇧↑↓ line-ext  y copy  esc cancel  q quit"` | Shortcut bar in select mode |
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
    case ModeNextConfirm: return m.handleNextConfirm(key)
    case ModeDone:        return m.handleDone(key)
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

- `n` — if a cancel function is available (subprocess running), saves the current mode as `prevMode` and switches to `ModeNextConfirm`; if no cancel function (no subprocess), no-op
- `q` — saves the current mode as `prevMode` and switches to `ModeQuitConfirm` (direct field write under `handler.mu`)
- All other keys are ignored

### NextConfirm Mode

Entered when the user presses `n` during a running step. The footer shows `NextConfirmPrompt` (`"Skip current step? (y/n, esc to cancel)"`):

- `y` — re-acquires the cancel function, restores `prevMode`, and offloads `cancel()` via `tea.Cmd` (same goroutine-offload pattern as the old direct-cancel path). Re-acquiring cancel is safe because the subprocess is still running while in `ModeNextConfirm`
- `n` — restores `prevMode` (cancels the skip without terminating the subprocess)
- `<Escape>` — same as `n`
- All other keys are ignored

### Error Mode

Entered by `Orchestrate` when a step fails (via `h.SetMode(ModeError)`):

- `c` — sends `ActionContinue` to the `Actions` channel (step stays marked failed, advance to next)
- `r` — sends `ActionRetry` to the `Actions` channel (re-run the failed step)
- `q` — saves current mode and switches to `ModeQuitConfirm`

### Quit Confirm Mode

- `y` — calls `ForceQuit()` (which sets `ModeQuitting` and terminates the subprocess) and returns `tea.QuitMsg{}` so the TUI exits. Returning `tea.QuitMsg` is needed because in `ModeDone` there is no workflow goroutine to call `program.Quit()` — the QuitMsg causes `program.Run()` to return directly
- `n` — restores `prevMode` (returns to whichever mode initiated the quit)
- `<Escape>` — same as `n`: restores `prevMode` and cancels the quit without firing `ForceQuit` or sending any action
- All other keys are ignored

`ForceQuit` sets `ModeQuitting` internally so the footer paints `Quitting...` on the very next render cycle, before the orchestration goroutine starts unwinding.

### Quitting Mode

Entered by the QuitConfirm `y` path or by `ForceQuit()` directly (which is called by the OS signal handler from any mode, including Normal and Error). The footer shows `QuittingLine` (`"Quitting..."`). No keypress handler is registered for this mode; any keypresses received while `mode == ModeQuitting` fall through `Handle`'s switch and are ignored. The mode persists until the workflow goroutine unwinds and tears the TUI down.

### Done Mode

When the workflow finishes all iterations and finalize steps successfully, `Run` writes the completion summary line to the log body and returns. The workflow goroutine in `main.go` flushes logs, closes channels, and calls `keyHandler.SetMode(ModeDone)`. The TUI stays alive so the user can scroll through the output. The footer shows `DoneShortcuts` (`"↑/k up  ↓/j down  v select  q quit"`):

- `v` — enters `ModeSelect` (same behavior as in Normal mode; see Select Mode below)
- `q` — saves `ModeDone` as `prevMode` and enters `ModeQuitConfirm`
- All other keys are ignored

### Select Mode

Entered by pressing `v` from `ModeNormal` or `ModeDone`. The footer shows `SelectShortcuts`. The cursor renders as a single reverse-video cell at column 0 of the last visible visual row in the log panel.

**Entry conditions:** `v` is blocked in `ModeError` (orchestration goroutine is blocked on `KeyHandler.Actions`), `ModeQuitConfirm`, `ModeNextConfirm`, and `ModeQuitting`. `v` with an empty log buffer (`len(m.log.lines) == 0`) is also a no-op — mode stays unchanged.

**Entry sequence (in `Model.Update`):**
1. `keysModel.Update` sets `mode = ModeSelect` and saves `prevMode`.
2. `model.go` post-dispatch: if mode just became `ModeSelect` and log is non-empty, calls `m.log.SetSelection(m.log.initSelectionAtLastVisibleRow())`.
3. If log is empty, the mode is reverted to `prevMode`.

**In `ModeSelect`:**
- `Esc` — clears the selection and returns to `prevMode`. The selection is cleared immediately (no single-frame stale overlay) within the same `Update` call that processes the Esc key.
- `q` — clears the selection, saves `prevMode`, and enters `ModeQuitConfirm` (same pattern as Normal/Done `q`).
- `h` / `←` — move cursor left one display column; clamped to column 0.
- `l` / `→` — move cursor right one display column; clamped to last column of the current visual row.
- `j` / `↓` — move cursor down one visual row; virtual column (vim-style) is preserved and clamped to the new row's width.
- `k` / `↑` — move cursor up one visual row; same virtual-column behavior.
- `0` / `Home` — jump cursor to column 0 of the current visual row.
- `$` / `End` — jump cursor to the last display column of the current visual row.
- `J` / `Shift+↓` — extend selection by one visual row downward (alias for `MoveSelectionCursor(0, +1)`).
- `K` / `Shift+↑` — extend selection by one visual row upward (alias for `MoveSelectionCursor(0, -1)`).
- `PgDn` — move cursor down by `viewport.Height - 1` visual rows (page step).
- `PgUp` — move cursor up by `viewport.Height - 1` visual rows (page step).
- All other keys are no-ops.

**Virtual column preservation:** vertical movement remembers `virtualCol` — the column the cursor was at before moving to a shorter line. When the cursor moves back to a longer line, it restores to `virtualCol` (clamped to the new row's width). Horizontal movement updates `virtualCol`.

**Auto-scroll:** after every cursor movement, `autoscrollToCursor()` adjusts `viewport.YOffset` so the cursor row is always visible. Moving above the top scrolls the viewport up; moving below the bottom scrolls down.

**Key routing guard:** When `modeBeforeKey == ModeSelect`, `Model.Update` skips the `m.log.Update(msg)` forward for `tea.KeyMsg`. This prevents `j`/`k` and other scroll-bound keys from double-dispatching to the viewport while in select mode.

**External mode-change guard:** `Model.prevObservedMode` tracks the mode at the end of the previous `Update`. If `prevObservedMode == ModeSelect` and the current mode is not (detected at the start of each `Update`), `m.log.ClearSelection()` is called — this covers the orchestration goroutine calling `h.SetMode(ModeError)` externally while a selection is visible.

From `ModeQuitConfirm`, `y` triggers `ForceQuit` + `tea.QuitMsg` which causes `program.Run()` to return. `<Escape>` or `n` restores `ModeDone`.

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

When a `tea.KeyMsg` arrives, `Model.Update` routes it to the key handler and, unless in `ModeSelect`, also to the viewport:

```go
case tea.KeyMsg:
    modeBeforeKey := m.keys.handler.Mode()
    m.keys, kcmd = m.keys.Update(msg)   // mode dispatch (n, q, c, r, y, Escape, v)
    if modeBeforeKey != ModeSelect {
        m.log, lcmd = m.log.Update(msg) // viewport scroll (↑/k, ↓/j)
    }
```

This means scroll keys (`↑`/`k`/`↓`/`j`) work during Normal mode — the viewport consumes them while the key handler ignores them. In Error and QuitConfirm modes, the key handler consumes the action keys but scroll keys still pass through to the viewport.

**`ModeSelect` routing guard:** when the mode is `ModeSelect` at the time a key arrives, the `m.log.Update(msg)` forward is skipped entirely. This prevents `j`/`k` and other scroll-bound keys from double-dispatching to the viewport: in ModeSelect, `handleSelectKey` has sole authority over key dispatch and drives viewport positioning explicitly via `autoscrollToCursor`. The pre-dispatch mode is used (not the post-dispatch mode) so that an Esc key that *exits* ModeSelect also doesn't double-dispatch.

## Testing

- `ralph-tui/internal/ui/ui_test.go` — Tests for all key handlers in each mode, mode transitions, quit confirm with cancel (`n` and `<Escape>` from Normal, Error, and Done), `y` flipping to `ModeQuitting` with `QuittingLine` footer and returning `tea.QuitMsg`, `SetMode` for all seven modes, ForceQuit (cancel fires, ActionQuit sent, idempotent, nil-cancel-no-panic, full-channel-no-panic, `TestForceQuit_SetsModeQuitting_FromNormal`, `TestForceQuit_SetsModeQuitting_FromError`, `TestForceQuit_SetsModeQuitting_FromNextConfirm`, `TestForceQuit_SetsModeQuitting_FromDone`), ShortcutLine thread safety with all seven modes
- `ralph-tui/internal/ui/select_mode_test.go` — 16 integration tests for `ModeSelect`: `v` enters select from Normal/Done (parameterized), `v` ignored in Error/QuitConfirm/NextConfirm/Quitting, `v` no-op with empty log, cursor starts at last visible row col 0, `Esc` returns to prevMode and clears selection immediately, `Esc` clears immediately (not next update), prevObservedMode double-guard idempotency, `LogLinesMsg` in select does not clear selection, external `SetMode` clears selection on next Update, unknown key no-op, `home`/`end` not forwarded; `j` in ModeSelect does not scroll viewport (routing guard), `SelectShortcuts` shown in footer, `v select` in Normal/Done shortcuts but not Error, `v` from Done restores Done on Esc
- `ralph-tui/internal/ui/keys_select_movement_test.go` — 15 tests covering all cursor movement acceptance criteria: h/j/k/l single-cell move, anchor fixed during movement, 0/Home → line start, $/End → line end, K/J/Shift+↑↓ extend by row, PgUp/PgDn by viewport.Height-1, virtual column preserved across shorter lines, viewport autoscrolls to cursor, q from ModeSelect enters QuitConfirm with pre-Select prevMode, Esc from QuitConfirm restores idle mode

## Additional Information

- [Architecture Overview](../architecture.md) — Keyboard & mode state machine diagram
- [Workflow Orchestration](workflow-orchestration.md) — How Actions channel drives the orchestration loop
- [Signal Handling & Shutdown](signal-handling.md) — How ForceQuit is triggered by OS signals
- [Subprocess Execution & Streaming](subprocess-execution.md) — How Terminate stops the active subprocess
- [TUI Status Header](tui-display.md) — How the shortcut bar is displayed alongside the status header
- [Concurrency](../coding-standards/concurrency.md) — Coding standards for non-blocking sends, channel dispatch, and mutex-protected getters
