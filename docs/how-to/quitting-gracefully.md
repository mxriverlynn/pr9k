# Quitting Gracefully

pr9k always shuts down through the same path — whether you press `q`, hit `Ctrl+C`, or `kill` the process — so the running subprocess gets a chance to clean up before the TUI tears down. This guide explains the three quit entry points, what you see during shutdown, and what the exit code tells you.

## The three ways to quit

| Entry point | Where it fires | What it does |
|-------------|----------------|--------------|
| `q` in Normal or Error mode, then `y` | `KeyHandler.handleQuitConfirm` | Flips footer to `Quitting...` (white), calls `ForceQuit` |
| `Ctrl+C` (SIGINT) or `kill` (SIGTERM) | Signal handler goroutine in `main.go` | Calls `ForceQuit`, waits up to 2s, then `program.Kill()` |
| Workflow completes normally | The workflow goroutine in `main.go` | `Run` returns on its own; enters `ModeDone` (`q quit` footer) |

The two interactive paths go through `KeyHandler.ForceQuit()` to unify subprocess termination and `ActionQuit` injection — you get the same shutdown semantics whether you press `y` or hit Ctrl+C. The normal-completion path doesn't need `ForceQuit` because there's nothing to cancel.

## The `q` path step by step

Pressing `q` in Normal or Error mode does **not** immediately quit. It opens a confirmation prompt so a stray keypress doesn't abort a long-running workflow.

```
[You press q]
       │
       ▼
┌─────────────────────────────────────┐
│ Mode: QuitConfirm                   │
│ Footer: Quit Power-Ralph.9000?      │
│   (y/n, esc to cancel)              │
└──────┬──────────┬───────────────────┘
       │          │
    y  │          │  n or Esc
       ▼          ▼
┌──────────┐  ┌──────────────┐
│ Quitting │  │ prev mode    │
│ ForceQuit│  │ (Normal or   │
│          │  │  Error)      │
└──────────┘  └──────────────┘
```

### Cancelling the quit

Pressing `n` or `Esc` while the footer shows `Quit Power-Ralph.9000? (y/n, esc to cancel)` restores your previous mode — Normal if you came from a running step, Error if you came from a failure pause — and puts the footer back to its previous shortcuts. No subprocess is touched, no action is injected.

This is non-destructive: if you were paused in error mode deciding whether to retry, cancelling a quit drops you right back into the same `c / r / q` decision with the same failed step still marked `[✗]`.

### Confirming the quit

Pressing `y` does two things in order:

1. **Flips the mode to `ModeQuitting`** — the footer immediately repaints to `Quitting...`. This is visible feedback that pr9k accepted your confirmation and is now shutting down. Without this transition, you'd hit `y` and then sit there wondering whether the keypress registered.

2. **Calls `ForceQuit()`** — which:
   - Calls the cancel function (`Runner.Terminate`), sending SIGTERM to the currently running subprocess. If the subprocess doesn't exit within 3 seconds, it's sent SIGKILL.
   - Injects `ActionQuit` into the `Actions` channel via non-blocking send. If the channel is full, the send is dropped (there's already a quit waiting — no need to queue another).

The workflow goroutine picks up `ActionQuit` at its next drain point (before each step in `Orchestrate`, or inside the error-mode `<-h.Actions` receive), returns from `Run`, and the TUI tears down.

## The signal path (Ctrl+C / SIGTERM)

The OS signal handler in `main.go` listens for SIGINT and SIGTERM on a buffered channel. When a signal arrives, the handler goroutine:

1. Closes a `signaled` one-shot channel (checked after `program.Run()` returns to select the exit code)
2. Calls `keyHandler.ForceQuit()` — same call as the `y` path, so the subprocess gets terminated and `ActionQuit` gets injected
3. Waits up to 2 seconds for the workflow goroutine to unwind on its own
4. If the workflow goroutine hasn't finished, calls `program.Kill()` to force the Bubble Tea program to stop
5. After `program.Run()` returns in main, the exit code is selected: `os.Exit(1)` because `signaled` is closed

Because the signal handler calls `ForceQuit`, **the signal path and the `q`→`y` path produce identical behavior from the workflow's perspective.** The only difference is the exit code: SIGINT/SIGTERM exits 1, a normal `q`→`y` shutdown exits 0.

The signal handler also runs whether or not the TUI is currently in Normal, Error, QuitConfirm, NextConfirm, Done, or Quitting mode — signals bypass the mode dispatcher entirely.

## The normal-completion path

When `Run` finishes all iterations and finalize steps, it writes the completion summary to the log body and returns. The workflow goroutine in `main.go` then flushes the log, closes the drain channel, and calls `keyHandler.SetMode(ui.ModeDone)`. The TUI stays alive with a `q quit` footer so you can review the final output. Press `q` then `y` to exit — this sends `tea.QuitMsg`, which causes `program.Run()` to return. After `program.Run()` returns, `main` calls `signal.Stop`, waits for the workflow goroutine to finish cleanup, selects the exit code, and calls `os.Exit(0)`.

## Exit codes

| Shutdown path | Exit code |
|---------------|-----------|
| Normal completion | `0` |
| `q` → `y` | `0` (the workflow returned normally from `Run`) |
| SIGINT / SIGTERM | `1` |
| `buildStep` error at startup (no valid config) | `1` |
| Validator errors before the TUI starts | `1` |
| Bubble Tea `program.Run()` returned an unexpected error | `1` |

If you're scripting pr9k, only `0` means "ran to completion". Any non-zero means "something interrupted us or broke before we started". The workflow goroutine checks the `signaled` channel before deciding between `os.Exit(0)` and `os.Exit(1)` so a signal that arrives while the workflow is already finishing still produces the correct exit code.

## What you see during the `Quitting...` window

Between pressing `y` and the process actually exiting, you're briefly in `ModeQuitting`:

- Footer shows `Quitting...`
- Checkbox grid still shows whatever state it was in
- Log panel still shows the last run output
- No keypresses are handled (no handler is registered for `ModeQuitting`)

The window is usually a fraction of a second — just long enough for the subprocess to receive its SIGTERM and the orchestration goroutine to drain its pending channel operations. For a long-running subprocess that doesn't honor SIGTERM, the window is up to 3 seconds (the SIGKILL fallback in `Runner.Terminate`).

## Not every keypress quits

Some interactions look like they might quit but don't:

- **`n` in Normal mode** — `n` means "skip the current step", not "quit". It enters a `ModeNextConfirm` prompt (`Skip current step? y/n, esc to cancel`). Pressing `y` confirms the skip and sends SIGTERM to the subprocess; pressing `n` or `Esc` cancels. See [Recovering from Step Failures](recovering-from-step-failures.md) for how skips interact with the workflow.
- **`Esc` in Normal or Error mode** — Escape only cancels a quit confirmation. Outside of `ModeQuitConfirm`, it's ignored.

## Related documentation

- [Reading the TUI](reading-the-tui.md) — What the footer looks like in each mode (`Quit Power-Ralph.9000?`, `Quitting...`, etc.)
- [Recovering from Step Failures](recovering-from-step-failures.md) — The `q` entry point from error mode
- [Keyboard Input & Error Recovery](../features/keyboard-input.md) — `ModeQuitConfirm`, `ModeQuitting`, `ForceQuit`
- [Signal Handling & Shutdown](../features/signal-handling.md) — SIGINT/SIGTERM handler, exit code selection, cleanup ordering
- [Subprocess Execution & Streaming](../features/subprocess-execution.md) — `Runner.Terminate` (SIGTERM then SIGKILL after 3s)
