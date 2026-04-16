# Plan: Add `n` confirmation and post-workflow "Press q to quit"

## Context

Currently, pressing `n` in the TUI immediately kills the running subprocess with no confirmation. Also, when the workflow completes, `program.Quit()` is called immediately — the screen goes blank and the user can't review output. Two changes are needed:

1. **`n` key confirmation** — show "Skip current step? (y/n, esc to cancel)" before canceling
2. **Post-workflow done mode** — keep the TUI alive after workflow completion with a "q quit" footer, then go through the normal quit confirmation to exit

## Approach

Add two new modes to the keyboard state machine: `ModeNextConfirm` and `ModeDone`.

### Files to modify

- `ralph-tui/internal/ui/ui.go` — new modes, constants, shortcut line mapping
- `ralph-tui/internal/ui/keys.go` — new handlers, updated dispatch and `handleNormal`
- `ralph-tui/internal/ui/model.go` — `colorShortcutLine` for new prompt
- `ralph-tui/cmd/ralph-tui/main.go` — workflow goroutine, signal handler restructure
- `ralph-tui/internal/ui/keys_test.go` — new mode tests
- `ralph-tui/internal/ui/ui_test.go` — SetMode tests for new modes, updated concurrent test
- `ralph-tui/internal/ui/model_test.go` — update `TestNormalMode_N_ReturnsCancelCmd`

### Step 1: `ui.go` — Add modes and constants

Add `ModeNextConfirm` and `ModeDone` to the Mode enum (between `ModeQuitConfirm` and `ModeQuitting`):

```go
ModeQuitConfirm
ModeNextConfirm  // NEW
ModeDone         // NEW
ModeQuitting
```

Add new string constants:

```go
NextConfirmPrompt = "Skip current step? (y/n, esc to cancel)"
DoneShortcuts     = "q quit"
```

Add cases to `updateShortcutLineLocked`:

```go
case ModeNextConfirm:
    h.shortcutLine = NextConfirmPrompt
case ModeDone:
    h.shortcutLine = DoneShortcuts
```

### Step 2: `keys.go` — New handlers and updated dispatch

**Update `keysModel.Update` dispatch** — add two new cases:

```go
case ModeNextConfirm:
    return m.handleNextConfirm(key)
case ModeDone:
    return m.handleDone(key)
```

**Change `handleNormal` `n` case** — instead of calling `cancel()` directly, enter `ModeNextConfirm` (same pattern as `q` entering `ModeQuitConfirm`):

```go
case "n":
    cancel := m.handler.Cancel()
    if cancel == nil {
        return m, nil
    }
    m.handler.mu.Lock()
    m.handler.prevMode = m.handler.mode
    m.handler.mode = ModeNextConfirm
    m.handler.updateShortcutLineLocked()
    m.handler.mu.Unlock()
    return m, nil
```

Note: the `cancel` nil check gates entry to `ModeNextConfirm`. If cancel is nil (no subprocess running), pressing `n` is a no-op — same as before.

**Add `handleNextConfirm`**:
- `y`: re-acquire `cancel` via `m.handler.Cancel()`, restore prevMode, then offload `cancel()` via tea.Cmd (same goroutine-offload pattern as the old `handleNormal` `n` case). Re-acquiring `cancel` is safe because the subprocess is still running while we're in `ModeNextConfirm` — the workflow goroutine doesn't advance steps until the current step finishes or is terminated.
- `n`/`esc`: restore prevMode (same pattern as `handleQuitConfirm`)

```go
func (m keysModel) handleNextConfirm(key tea.KeyMsg) (keysModel, tea.Cmd) {
    switch key.String() {
    case "y":
        cancel := m.handler.Cancel()
        m.handler.mu.Lock()
        m.handler.mode = m.handler.prevMode
        m.handler.updateShortcutLineLocked()
        m.handler.mu.Unlock()
        if cancel == nil {
            return m, nil
        }
        return m, func() tea.Msg {
            cancel()
            return nil
        }
    case "n", "esc":
        m.handler.mu.Lock()
        m.handler.mode = m.handler.prevMode
        m.handler.updateShortcutLineLocked()
        m.handler.mu.Unlock()
    }
    return m, nil
}
```

**Add `handleDone`**:
- `q`: enter `ModeQuitConfirm` with `prevMode = ModeDone` (same pattern as `handleNormal` `q`)
- All other keys: no-op

```go
func (m keysModel) handleDone(key tea.KeyMsg) (keysModel, tea.Cmd) {
    switch key.String() {
    case "q":
        m.handler.mu.Lock()
        m.handler.prevMode = m.handler.mode
        m.handler.mode = ModeQuitConfirm
        m.handler.updateShortcutLineLocked()
        m.handler.mu.Unlock()
    }
    return m, nil
}
```

### Step 3: `keys.go` — Make quit confirmation exit the TUI

**Update `handleQuitConfirm` `y` case** — return `tea.QuitMsg{}` instead of `nil` from the cmd function:

```go
return m, func() tea.Msg {
    m.handler.ForceQuit()
    return tea.QuitMsg{}  // was: return nil
}
```

This is needed because in `ModeDone`, there's no workflow goroutine to call `program.Quit()`. The existing `Model.Update` already handles `tea.QuitMsg` at model.go:148-149 and returns `tea.Quit`. For the mid-workflow quit case, `tea.QuitMsg` causes `program.Run()` to return before the workflow goroutine finishes cleanup — the `workflowDone` wait added to `main.go` in Step 5 ensures logs are flushed and channels are closed before `os.Exit`.

### Step 4: `model.go` — Update `colorShortcutLine`

Add a case for `NextConfirmPrompt` (render all-white, same as `QuittingLine`):

```go
if s == NextConfirmPrompt {
    return white.Render(s)
}
```

Place this after the `QuitConfirmPrompt` block and before the `QuittingLine` check. `DoneShortcuts` ("q quit") naturally works with the existing default two-tone rendering (key white, description gray) — no special case needed.

### Step 5: `main.go` — Workflow goroutine and signal handler restructure

This step requires careful changes because the TUI stays alive after the workflow finishes, and signals must remain handled during `ModeDone` to avoid corrupting the terminal.

**Problem**: The current signal handler goroutine exits via `case <-workflowDone:` when the workflow completes. The workflow goroutine calls `signal.Stop(sigChan)` before calling `program.Quit()`. If we simply replace `program.Quit()` with `SetMode(ModeDone)`, SIGINT/SIGTERM during `ModeDone` would hit the OS default handler (immediate kill), leaving the terminal in alt-screen mode because Bubble Tea cleanup never runs.

**Solution**: Restructure the signal handler goroutine to remain active after workflow completion, and move `signal.Stop(sigChan)` out of the workflow goroutine.

**Workflow goroutine** — change from:

```go
go func() {
    defer close(workflowDone)
    _ = workflow.Run(runner, proxy, keyHandler, runCfg)
    signal.Stop(sigChan)
    _ = log.Close()
    close(lineCh)
    program.Quit()
}()
```

to:

```go
go func() {
    defer close(workflowDone)
    _ = workflow.Run(runner, proxy, keyHandler, runCfg)
    _ = log.Close()
    close(lineCh)
    keyHandler.SetMode(ui.ModeDone)
}()
```

Remove `signal.Stop(sigChan)` — signals stay registered until the process exits. Remove `program.Quit()` — the TUI stays alive in `ModeDone`.

**Signal handler goroutine** — change from:

```go
go func() {
    select {
    case <-sigChan:
        close(signaled)
        keyHandler.ForceQuit()
        select {
        case <-workflowDone:
        case <-time.After(2 * time.Second):
            program.Kill()
        }
    case <-workflowDone:
    }
}()
```

to:

```go
go func() {
    <-sigChan
    close(signaled)
    keyHandler.ForceQuit()
    select {
    case <-workflowDone:
    case <-time.After(2 * time.Second):
    }
    program.Kill()
}()
```

The signal handler now:
1. Blocks on `<-sigChan` unconditionally (no `case <-workflowDone:` escape hatch)
2. On signal: calls `ForceQuit()`, waits up to 2s for workflow to finish
3. Always calls `program.Kill()` at the end

If the signal arrives during `ModeDone` (workflow already finished):
- `ForceQuit()` sets `ModeQuitting`, calls `runner.Terminate()` (no-op — no subprocess), sends `ActionQuit` (non-blocking, nobody reads)
- `<-workflowDone` resolves immediately (already closed)
- `program.Kill()` force-stops the TUI, restoring terminal state

If no signal ever arrives: the goroutine stays blocked on `<-sigChan` forever. This is fine — the process exits when `program.Run()` returns (via user `q` → `y` → `tea.QuitMsg`), and the goroutine is cleaned up by process exit. The goroutine is lightweight (blocked on channel) and does not leak resources.

**After `program.Run()` returns** — add `signal.Stop(sigChan)` and a wait for `workflowDone` before exit codes:

```go
_, runErr := program.Run()
signal.Stop(sigChan)

// Wait for the workflow goroutine to finish cleanup (log flush, channel
// close). Needed because handleQuitConfirm's tea.QuitMsg can cause
// program.Run() to return before the workflow goroutine finishes — the
// mid-workflow quit path sends tea.QuitMsg immediately after ForceQuit,
// racing with the goroutine's log.Close() and close(lineCh).
select {
case <-workflowDone:
case <-time.After(4 * time.Second):
}

// ... existing error handling and exit codes ...
```

`signal.Stop(sigChan)` deregisters signal notifications after the TUI has exited cleanly. The `workflowDone` wait ensures the workflow goroutine's cleanup (log flush, channel close) completes before `os.Exit`. The 4-second timeout exceeds the 3-second `terminateGracePeriod` in `runner.Terminate()` plus buffer for `log.Close()` and `close(lineCh)` — this prevents `os.Exit` from firing while SIGTERM→SIGKILL is still in progress during a mid-workflow quit. Both align with the concurrency coding standard ("Signal path and completion path must converge cleanly" and "Wait for background goroutines after program.Run() returns").

### Step 6: Update tests

**`keys_test.go`**:
- `TestHandleNormal_N_NilCancel_NoCmd`: no change needed — nil cancel still returns early before entering `ModeNextConfirm`
- Add `TestHandleNormal_N_EntersNextConfirm`: press `n` with non-nil cancel, verify `ModeNextConfirm` and `NextConfirmPrompt` shortcut line, and nil cmd return
- Add `TestHandleNormal_N_SavesPrevModeAsNormal`: verify prevMode is `ModeNormal` after `n` transitions to `ModeNextConfirm`
- Add `TestHandleNextConfirm_Y_RestoresModeAndReturnsCmd`: enter `ModeNextConfirm` from `ModeNormal` via `n`, press `y`, verify mode restored to `ModeNormal`, non-nil cmd returned, executing cmd calls cancel
- Add `TestHandleNextConfirm_N_RevertsMode`: enter `ModeNextConfirm`, press `n`, verify mode reverted and `NormalShortcuts` restored
- Add `TestHandleNextConfirm_Esc_RevertsMode`: same with esc key
- Add `TestHandleNextConfirm_UnrecognizedKey_NoOp`: press `x`, verify no mode change
- Add `TestHandleDone_Q_EntersQuitConfirm`: set `ModeDone`, press `q`, verify `ModeQuitConfirm` with `prevMode=ModeDone`
- Add `TestHandleDone_Q_SavesPrevModeAsDone`: verify prevMode after transition
- Add `TestHandleDone_OtherKeys_NoOp`: press `n`, `c`, `r`, `x` — all no-op
- Update `TestHandleQuitConfirm_Y_ReturnsNonNilCmd`: verify cmd returns `tea.QuitMsg{}`
- Add `TestHandleQuitConfirm_Esc_FromDone_RevertsToDone`: enter quit-confirm from `ModeDone`, press esc, verify mode reverts to `ModeDone` and `DoneShortcuts` restored

**`ui_test.go`**:
- Add `TestSetMode_NextConfirm_UpdatesShortcutLine`: verify `NextConfirmPrompt`
- Add `TestSetMode_Done_UpdatesShortcutLine`: verify `DoneShortcuts`
- Add `TestForceQuit_SetsModeQuitting_FromNextConfirm`: set `ModeNextConfirm`, call `ForceQuit()`, verify `ModeQuitting` and `QuittingLine` (matches existing `_FromNormal` and `_FromError` pattern — covers SIGINT during the skip confirmation prompt)
- Add `TestForceQuit_SetsModeQuitting_FromDone`: set `ModeDone`, call `ForceQuit()`, verify `ModeQuitting` and `QuittingLine` (covers SIGINT during the post-workflow done screen)
- Update `TestShortcutLine_ConcurrentRead_NoRace`: add `ModeNextConfirm` and `ModeDone` to the mode cycle
- Update `TestForceQuit_ConcurrentAccess_NoRace`: add `ModeNextConfirm` and `ModeDone` to the SetMode cycle

**`model_test.go`**:
- Rewrite `TestNormalMode_N_ReturnsCancelCmd` as `TestNormalMode_N_ThenY_CallsCancel`: press `n` (enters `ModeNextConfirm`, nil cmd), then press `y` (returns non-nil cmd that calls cancel). Two-step test.
- Add `TestColorShortcutLine_NextConfirmPrompt_PreservesText`: verify plain text matches `NextConfirmPrompt`
- Add `TestColorShortcutLine_DoneShortcuts_PreservesText`: verify plain text matches `DoneShortcuts`

### Step 7: Update concurrency coding standard

Update `docs/coding-standards/concurrency.md`:

1. **"Signal path and completion path must converge cleanly"** — the code example shows `program.Quit()` in the workflow goroutine. Update the example to show `SetMode(ModeDone)` and the restructured signal handler, so the standard matches the new implementation.

2. **"Wait for background goroutines after program.Run() returns"** — the code example is missing the `signal.Stop(sigChan)` call that now lives after `program.Run()`. Update the example to show `signal.Stop(sigChan)` followed by the `workflowDone` select, so the standard documents the full post-Run shutdown sequence.

### Step 8: Update feature documentation

- `docs/features/keyboard-input.md` — update "Four-mode keyboard state machine" to six modes; add `ModeNextConfirm` (n confirmation) and `ModeDone` (post-workflow) to the state diagram and mode descriptions
- `docs/features/signal-handling.md` — update the signal handler description to reflect the restructured goroutine (unconditional `<-sigChan` block, no `workflowDone` escape hatch, always `program.Kill()` at the end)
- `docs/features/tui-display.md` — add `NextConfirmPrompt` and `DoneShortcuts` to the shortcut footer section

## Verification

1. `cd ralph-tui && go test -race ./...` — all tests pass
2. `make lint` — no lint issues
3. `make build` — builds successfully
4. Manual: run against a project, press `n` during a step — see confirmation prompt, `y` skips, `esc` cancels
5. Manual: let workflow complete — TUI stays up with "q quit" footer, press `q` then `y` to exit
6. Manual: let workflow complete — in `ModeDone`, press Ctrl+C — TUI exits cleanly, terminal restored
7. Manual: during a running step, press `q` then `y` — TUI exits after subprocess terminates

## Review summary

- Iterations completed: 3 + agent validation
- Assumptions challenged: 11 primary + secondary across iterations; 6 verified by evidence-based investigator; 8 examined by adversarial validator
- Consolidations: 0
- Changes from iterations:
  - **Critical**: Added `workflowDone` wait after `program.Run()` — without it, `os.Exit` races the workflow goroutine's log flush and channel close during mid-workflow quit
  - **Critical**: Set post-Run timeout to 4s (not 2s) — must exceed the 3s `terminateGracePeriod` in `runner.Terminate()` to prevent `os.Exit` during SIGTERM→SIGKILL
  - **Test gap**: Added `TestForceQuit_SetsModeQuitting_FromNextConfirm` and `_FromDone` — covers SIGINT arriving during new modes (flagged by adversarial validator)
  - Updated Step 3 text to accurately describe mid-workflow quit timing
  - Expanded Step 7 to include "Wait for background goroutines" standard update
  - Added Step 8 for feature documentation updates
- Inherited from original plan: signal handler restructured to remain active during `ModeDone`
