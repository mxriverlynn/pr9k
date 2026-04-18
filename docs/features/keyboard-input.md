# Keyboard Input & Error Recovery

A seven-mode state machine that routes keypresses and communicates user decisions to the orchestration goroutine via a channel.

- **Last Updated:** 2026-04-17
- **Authors:**
  - River Bailey

## Overview

- `KeyHandler` operates in eight modes: Normal, Error, QuitConfirm, NextConfirm, Done, Select, Quitting, and Help — each with its own keypress bindings and shortcut bar text
- User decisions are sent to the orchestration goroutine via a buffered `Actions` channel carrying `StepAction` values (Retry, Continue, Quit)
- In Normal mode, `n` enters the skip confirmation prompt (NextConfirm) and `q` enters quit confirmation
- In NextConfirm mode (entered when the user presses `n` during a running step), `y` terminates the current subprocess (skip step), `n` or `<Escape>` cancel and restore the previous mode
- In Error mode (entered when a step fails), `c` continues past the failure, `r` retries the step, and `q` enters quit confirmation
- In QuitConfirm mode, `y` flips to the `Quitting` mode (footer shows `Quitting...`), calls `ForceQuit`, and returns `tea.QuitMsg` to exit the TUI; `n` or `<Escape>` cancel and restore the previous mode
- In Done mode (entered when the workflow completes), the TUI stays alive so the user can review output; `q` enters quit confirmation; `v` enters `ModeSelect`
- In Select mode (entered when `v` is pressed from Normal or Done, or via a left mouse click on the log viewport), the cursor is shown as a reverse-video cell in the log panel; `Esc` clears the selection and returns to the prior mode; all cursor movement keys (hjkl/arrows, 0/$, J/K, PgUp/PgDn) are implemented; left-drag extends; release commits and shows `SelectCommittedShortcuts`; `y` or `Enter` copies the selected text to the clipboard (with OSC 52 fallback) and exits ModeSelect
- In Quitting mode the footer shows `Quitting...` as visible confirmation that the user's quit was accepted while the orchestration goroutine unwinds
- In Help mode (entered via `?` from Normal mode, only when `StatusLineActive()` is true), the footer shows `"esc  close"`; `<Escape>` restores the previous mode and `q` enters quit confirmation without overwriting the saved previous mode
- `ForceQuit()` is a signal-safe method that terminates the subprocess and injects `ActionQuit` via non-blocking send — it is called both by the OS signal handler (SIGINT/SIGTERM) and by the QuitConfirm `y` path, so both paths produce identical shutdown behavior

Key files:
- `src/internal/ui/ui.go` — KeyHandler struct, mode state, ForceQuit, ShortcutLine
- `src/internal/ui/keys.go` — keysModel Bubble Tea sub-model, Update dispatch to mode handlers
- `src/internal/ui/ui_test.go` — Unit tests for KeyHandler modes and transitions
- `src/internal/ui/keys_test.go` — Unit tests for keysModel.Update routing

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
  │  y/Enter → copy to clipboard, exit Select   │
  │  Cursor movement and clipboard copy impl.   │
  └─────────────────────────────────────────────┘

  ModeHelp (entered via ? from ModeNormal when StatusLineActive() == true):
    → footer shows HelpModeShortcuts ("esc  close")
    → modal body shows per-mode shortcut grid (HelpModalNormal etc.)
    → Esc → restores prevMode
    → q → ModeQuitConfirm (prevMode unchanged, so Esc-from-quit restores idle mode)
    → all other keys are no-ops
    → ? is a no-op in all other modes even when StatusLineActive

  StatusLineActive:
    Set via SetStatusLineActive(true) from main.go when a statusLine command is
    configured. When false (default), ? is a no-op in every mode and ModeHelp
    is unreachable from the public API.
```

## Key Files

| File | Purpose |
|------|---------|
| `src/internal/ui/ui.go` | KeyHandler struct, mode state, ForceQuit, ShortcutLine |
| `src/internal/ui/keys.go` | keysModel Bubble Tea sub-model; Update dispatches tea.KeyMsg to mode handlers |
| `src/internal/ui/clipboard.go` | `copyToClipboard` (clipboard write + OSC 52 fallback), `copySelectedText` (async tea.Cmd wrapper with feedback log line) |
| `src/internal/ui/ui_test.go` | Tests for KeyHandler modes, transitions, and ForceQuit |
| `src/internal/ui/keys_test.go` | Tests for keysModel.Update routing (normal, error, quit-confirm, quitting) |

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
    ModeNextConfirm    // entered after n; shows "Skip current step?" prompt
    ModeDone           // entered after workflow completes; shows "v select  q quit" footer
    ModeSelect         // entered by v from Normal/Done; shows selection cursor overlay
    ModeQuitting       // confirmed quit; footer shows "Quitting..." during shutdown
    ModeHelp           // entered via ? from Normal (when StatusLineActive); esc/q exit
)

type KeyHandler struct {
    mode               Mode
    prevMode           Mode           // restored when quit confirm is cancelled
    cancel             func()         // terminates the current subprocess
    Actions            chan StepAction // communicates decisions to orchestration
    mu                 sync.Mutex     // protects mode, prevMode, and shortcutLine
    shortcutLine       string         // protected by mu; use ShortcutLine() to access
    statusLineActive   bool           // protected by mu; gates the ? → ModeHelp transition
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
| `SelectShortcuts` | `"hjkl/↑↓←→ extend  0/$ line  ⇧↑↓ line-ext  y copy  esc cancel  q quit"` | Shortcut bar in select mode (while selection is in progress) |
| `SelectCommittedShortcuts` | `"y copy  esc cancel  drag for new selection"` | Shortcut bar shown immediately after a mouse drag release; cleared on next key or mouse event |
| `QuittingLine` | `"Quitting..."` | Shortcut bar in quitting mode (visible while shutdown unwinds) |
| `HelpModeShortcuts` | `"esc  close"` | Shortcut bar while the Help modal is open |
| `HelpModalNormal` | *(multiline grid)* | Two-column shortcut grid shown in the Help modal for Normal mode |
| `HelpModalSelect` | *(multiline grid)* | Two-column shortcut grid shown in the Help modal for Select mode |
| `HelpModalError` | *(multiline grid)* | Two-column shortcut grid shown in the Help modal for Error mode |
| `HelpModalDone` | *(multiline grid)* | Two-column shortcut grid shown in the Help modal for Done mode |

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
    case ModeSelect:      return m.handleSelect(key)
    case ModeHelp:        return m.handleHelp(key)
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
- `v` — saves the current mode as `prevMode` and switches to `ModeSelect` (log emptiness guard and selection initialization happen in `model.go`)
- `?` — if `StatusLineActive()` is true, saves the current mode as `prevMode` and switches to `ModeHelp`; if false, no-op
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

### Help Mode

Entered by pressing `?` from `ModeNormal`, but only when `StatusLineActive()` returns true (i.e., a `statusLine` command is configured in `ralph-steps.json`). When `StatusLineActive()` is false, `?` is a no-op in every mode and `ModeHelp` is unreachable. The footer shows `HelpModeShortcuts` (`"esc  close"`).

The modal body is drawn by `Model.View()` using one of the four `HelpModal*` constants, each a two-column grid of shortcuts targeting ~67 columns of interior space:

| Constant | Modal content |
|----------|---------------|
| `HelpModalNormal` | Normal-mode keys: scroll, select, skip, quit, and `?` |
| `HelpModalSelect` | Select-mode keys: cursor movement, copy, cancel, quit |
| `HelpModalError` | Error-mode keys: continue, retry, quit |
| `HelpModalDone` | Done-mode keys: scroll, select, quit |

The modal section shown is chosen based on the `prevMode` stored when `?` was pressed (i.e., the mode the user was in before entering Help).

Key bindings in Help mode:
- `<Escape>` — restores `prevMode` (the idle mode active before `?` was pressed)
- `q` — transitions to `ModeQuitConfirm` without overwriting `prevMode`; pressing `<Escape>` from `ModeQuitConfirm` restores the original idle mode, not `ModeHelp`
- All other keys are no-ops

`?` is only handled in `handleNormal`; all other mode handlers (`handleError`, `handleDone`, `handleSelect`, etc.) do not process `?`, so Help is only reachable from `ModeNormal`.

### Select Mode

Entered by pressing `v` from `ModeNormal` or `ModeDone`, or by left-clicking/dragging in the log viewport from any non-modal mode. The footer shows `SelectShortcuts` while a selection is in progress; after a drag release the footer switches to `SelectCommittedShortcuts` (`"y copy  esc cancel  drag for new selection"`) until the next key or mouse event.

**Entry conditions (keyboard `v`):** `v` is blocked in `ModeError` (orchestration goroutine is blocked on `KeyHandler.Actions`), `ModeQuitConfirm`, `ModeNextConfirm`, and `ModeQuitting`. `v` with an empty log buffer (`len(m.log.lines) == 0`) is also a no-op — mode stays unchanged.

**Entry conditions (mouse press):** a left-click on log viewport content from `ModeNormal` or `ModeDone` transitions to `ModeSelect`. If the click lands in empty viewport padding below the content (resolveVisualPos returns false), the mode stays unchanged — no zero-value selection is created. Left-press in `ModeError`, `ModeQuitConfirm`, `ModeNextConfirm`, and `ModeQuitting` is a no-op.

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
- `y` / `Enter` — copies the selected text to the clipboard and exits ModeSelect (restores prevMode). If the selection is empty, this is a silent no-op that still exits ModeSelect. The clipboard write runs asynchronously inside a `tea.Cmd` so it does not block the Bubble Tea Update goroutine. On success, a `[copied N chars]` feedback line is appended to the log. On failure (no clipboard daemon), the OSC 52 escape sequence is written to stderr so clipboard-capable terminals (iTerm2, Kitty, Windows Terminal) can deliver the payload over SSH. If stderr is not a terminal, `[copy failed: install xclip/xsel or run in a terminal that supports OSC 52]` is appended to the log.
- All other keys are no-ops.

**Mouse gestures in `ModeSelect`:**
- **Left-drag** — extends the cursor endpoint as the pointer moves. Fires on every `MouseActionMotion` event while `sel.active=true`. Auto-scrolls one line per motion event when the pointer is above or below the visible window.
- **Left-release** — commits the active drag: `active=false`, `committed=true`. The footer immediately shows `SelectCommittedShortcuts`. The next key or mouse event clears the flag and restores `SelectShortcuts`.
- **Shift-click** — when a committed selection exists, moves only the cursor to the clicked cell; the anchor stays fixed.
- **Bare click** — re-anchors the selection at the click cell (clears previous selection and sets `active=true`).
- **Wheel** — always forwards to the viewport for scrolling, regardless of mode. Does not affect selection state.

**Virtual column preservation:** vertical movement remembers `virtualCol` — the column the cursor was at before moving to a shorter line. When the cursor moves back to a longer line, it restores to `virtualCol` (clamped to the new row's width). Horizontal movement updates `virtualCol`.

**Auto-scroll (keyboard):** after every cursor movement, `autoscrollToCursor()` adjusts `viewport.YOffset` so the cursor row is always visible. Moving above the top scrolls the viewport up; moving below the bottom scrolls down.

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

### SelectJustReleased

```go
func (h *KeyHandler) SelectJustReleased() bool
```

A mutex-protected getter that reports whether a mouse drag selection was just committed (the mouse was released, producing a `SelectCommittedShortcuts` footer). The flag is set inside `updateShortcutLineLocked` when the mode is `ModeSelect` and a release event committed the drag, and is cleared on the next key or mouse event via `clearJustReleasedLocked()`. Used by tests to assert the committed-shortcut path without reaching through unexported fields.

### PrevMode

```go
func (h *KeyHandler) PrevMode() Mode
```

A mutex-protected getter that returns the mode that will be restored when `<Escape>` is pressed from `ModeQuitConfirm`. Set by the `q` key handler (and the `?` key handler for the Help path); not set by `SetMode`. Used by tests to assert the saved idle mode without reaching through unexported fields.

### StatusLineActive / SetStatusLineActive

```go
func (h *KeyHandler) StatusLineActive() bool
func (h *KeyHandler) SetStatusLineActive(active bool)
```

Mutex-protected getter/setter for whether a `statusLine` command is configured. `SetStatusLineActive(true)` must be called from `main.go` after constructing the `statusline.Runner` if the config contains a `statusLine` block. When false (the default), `?` is a no-op in `ModeNormal` and `ModeHelp` is unreachable. When true, `?` in `ModeNormal` transitions to `ModeHelp` and the Help modal is shown.

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

- `src/internal/ui/ui_test.go` — Tests for all key handlers in each mode, mode transitions, quit confirm with cancel (`n` and `<Escape>` from Normal, Error, and Done), `y` flipping to `ModeQuitting` with `QuittingLine` footer and returning `tea.QuitMsg`, `SetMode` for all eight modes, ForceQuit (cancel fires, ActionQuit sent, idempotent, nil-cancel-no-panic, full-channel-no-panic, `TestForceQuit_SetsModeQuitting_FromNormal`, `TestForceQuit_SetsModeQuitting_FromError`, `TestForceQuit_SetsModeQuitting_FromNextConfirm`, `TestForceQuit_SetsModeQuitting_FromDone`, `TestForceQuit_FromModeHelp_TransitionsToQuitting`), ShortcutLine thread safety with all eight modes; `TestKeyHandler_StatusLineActive_DefaultAndRoundTrip`, `TestKeyHandler_StatusLineActive_ConcurrentAccess`, `TestHelpModeShortcuts_ConstantContents`, `TestHelpModalConstants_MaxLineWidthWithinBudget`, `TestUpdateShortcutLineLocked_AllModesHaveExplicitCase`, `TestSetMode_ModeHelp_DoesNotSavePrevMode`
- `src/internal/ui/select_mode_test.go` — 16 integration tests for `ModeSelect`: `v` enters select from Normal/Done (parameterized), `v` ignored in Error/QuitConfirm/NextConfirm/Quitting, `v` no-op with empty log, cursor starts at last visible row col 0, `Esc` returns to prevMode and clears selection immediately, `Esc` clears immediately (not next update), prevObservedMode double-guard idempotency, `LogLinesMsg` in select does not clear selection, external `SetMode` clears selection on next Update, unknown key no-op, `home`/`end` not forwarded; `j` in ModeSelect does not scroll viewport (routing guard), `SelectShortcuts` shown in footer, `v select` in Normal/Done shortcuts but not Error, `v` from Done restores Done on Esc
- `src/internal/ui/keys_select_movement_test.go` — 16 tests covering all cursor movement acceptance criteria: h/j/k/l single-cell move, arrow keys move cursor, anchor fixed during movement, 0/Home → line start, $/End → line end, K/J/Shift+↑↓ extend by row, PgUp/PgDn by viewport.Height-1, virtual column preserved across shorter lines, cursor clamps to line end on narrow rows, viewport autoscrolls to cursor, q from ModeSelect enters QuitConfirm with pre-Select prevMode, q clears selection before entering QuitConfirm, Esc from QuitConfirm restores idle mode, SelectedText updated after cursor moves
- `src/internal/ui/clipboard_copy_test.go` — 9 tests for the clipboard copy action: y/Enter copies and exits ModeSelect, OSC 52 fallback when clipboard daemon unavailable, `[copied N chars]` feedback on success, `[copy failed: ...]` on failure, empty selection is a silent no-op, clipboard payload uses raw coordinates (no wrap-induced newlines), `github.com/atotto/clipboard` is a direct dep, copyFn and stderrWriter test seam isolation, resetClipboardFns restores defaults
- `src/internal/ui/clipboard_additional_test.go` — 21 additional tests across 7 categories: `copyToClipboard` unit tests (empty string, multi-byte UTF-8 OSC 52 encoding, no stderr leak on success, large payload), `copySelectedText` helper isolated (empty returns nil cmd, success produces LogLinesMsg, failure produces error LogLinesMsg), model.go routing (multi-line selection payload, double-y second is no-op, Enter from Done restores Done, single-char feedback), `handleSelect` separation of concerns (y does not call copyFn directly, Enter restores prevMode, shortcut footer updates on exit), test seam safety (resetClipboardFns, stderrWriter restore, no-parallel audit), LogLinesMsg integration (copied/failed lines appear in viewport, byte-vs-rune count documented), go mod tidy produces no diff
- `src/internal/ui/mouse_selection_test.go` — 16 integration tests covering all mouse selection acceptance criteria: left-drag selects with live reverse-video feedback, release commits and shows `SelectCommittedShortcuts`, dragging past top/bottom edge auto-scrolls one line per motion, bare click re-anchors, shift-click extends committed cursor, left-press in Error/QuitConfirm/NextConfirm/Quitting is a no-op, wheel scrolls in every mode, mid-drag resize force-commits without losing raw coords, `y` copies committed selection
- `src/internal/ui/mouse_selection_extra_test.go` — 24 additional tests across 7 categories: `resolveVisualPos` edge cases (negative row, row past end, col clamped to row width, negative col clamped), `HandleMouse` unit tests (empty visualLines, motion/release guards, shift-press without committed selection, negative row clamping), model.go routing (press above/below viewport, stray motion/release), `selectJustReleased` lifecycle (cleared by wheel, by second press, not set by keyboard `v`), auto-scroll clamping (multi-event accumulation, top/bottom boundary stops), shift-click edge cases (same cell no-op, before anchor, during active drag ignored), `SelectCommittedShortcuts` constant and `updateShortcutLineLocked` path
- `src/internal/ui/keys_test.go` (ModeHelp additions) — `?` enters ModeHelp when StatusLineActive, `?` no-op when StatusLineActive false, `?` no-op in all non-Normal modes even when StatusLineActive (`TestHandleNonNormal_QuestionMark_NoOp_EvenWhenStatusLineActive`), esc from ModeHelp restores prevMode, q from ModeHelp enters QuitConfirm, esc from QuitConfirm-via-Help restores idle mode (not ModeHelp), shortcut line set to `HelpModeShortcuts` on entry, unrecognized key is no-op; `TestShortcutModalParity` and `TestShortcutModalParity_NoUnapprovedModalExtras` (bidirectional token parity between footer and modal constants); `TestHandleHelp_Esc_RestoresPrevModeShortcutLine`, `TestHandleHelp_Q_ShortcutLineShowsQuitConfirmPrompt`, `TestHandleHelp_StatusLineFlipsInactive_DoesNotEject`, `TestHandleHelp_IgnoresNormalModeKeys`, `TestKeyHandler_StatusLineActive_ConcurrentAccess`, `TestForceQuit_FromModeHelp_TransitionsToQuitting`
- `src/internal/ui/model_test.go` (ModeHelp additions) — `TestModel_ModeTrigger_Help_FiresOnEachEdge` (4 sub-tests: Normal→Help, Help→Normal, Help→QuitConfirm, QuitConfirm→Normal-via-Help-path); `TestModel_ModeTrigger_NoFireOn_QuestionMarkNoOp` (? no-op does not fire the mode-change trigger)

## Additional Information

- [Architecture Overview](../architecture.md) — Keyboard & mode state machine diagram
- [Workflow Orchestration](workflow-orchestration.md) — How Actions channel drives the orchestration loop
- [Signal Handling & Shutdown](signal-handling.md) — How ForceQuit is triggered by OS signals
- [Subprocess Execution & Streaming](subprocess-execution.md) — How Terminate stops the active subprocess
- [TUI Status Header](tui-display.md) — How the shortcut bar is displayed alongside the status header
- [Concurrency](../coding-standards/concurrency.md) — Coding standards for non-blocking sends, channel dispatch, and mutex-protected getters
