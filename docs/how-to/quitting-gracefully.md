# Quitting Gracefully

Ralph-tui always shuts down through the same path — whether you press `q`, hit `Ctrl+C`, or `kill` the process — so the running subprocess gets a chance to clean up before the TUI tears down. This guide explains the three quit entry points, what you see during shutdown, and what the exit code tells you.

## The three ways to quit

| Entry point | Where it fires | What it does |
|-------------|----------------|--------------|
| `q` in Normal or Error mode, then `y` | `KeyHandler.handleQuitConfirm` | Flips footer to `Quitting...`, calls `ForceQuit` |
| `Ctrl+C` (SIGINT) or `kill` (SIGTERM) | Signal handler goroutine in `main.go` | Calls `ForceQuit`, stops the TUI, exits 1 |
| Workflow completes normally → any key | `ModeDone` in `handleDone` | Sends `ActionQuit`, `Run` returns |

All three paths go through `KeyHandler.ForceQuit()` (except the normal-completion one, which exits through `ModeDone`). That means subprocess termination and `ActionQuit` injection are **unified** — you get the same shutdown semantics regardless of which button you hit.

## The `q` path step by step

Pressing `q` in Normal or Error mode does **not** immediately quit. It opens a confirmation prompt so a stray keypress doesn't abort a long-running workflow.

```
[You press q]
       │
       ▼
┌────────────────────────────┐
│ Mode: QuitConfirm          │
│ Footer: Quit ralph?        │
│   (y/n, esc to cancel)     │
└──────┬──────────┬──────────┘
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

Pressing `n` or `Esc` while the footer shows `Quit ralph? (y/n, esc to cancel)` restores your previous mode — Normal if you came from a running step, Error if you came from a failure pause — and puts the footer back to its previous shortcuts. No subprocess is touched, no action is injected.

This is non-destructive: if you were paused in error mode deciding whether to retry, cancelling a quit drops you right back into the same `c / r / q` decision with the same failed step still marked `[✗]`.

### Confirming the quit

Pressing `y` does two things in order:

1. **Flips the mode to `ModeQuitting`** — the footer immediately repaints to `Quitting...`. This is visible feedback that ralph-tui accepted your confirmation and is now shutting down. Without this transition, you'd hit `y` and then sit there wondering whether the keypress registered.

2. **Calls `ForceQuit()`** — which:
   - Calls the cancel function (`Runner.Terminate`), sending SIGTERM to the currently running subprocess. If the subprocess doesn't exit within 3 seconds, it's sent SIGKILL.
   - Injects `ActionQuit` into the `Actions` channel via non-blocking send. If the channel is full, the send is dropped (there's already a quit waiting — no need to queue another).

The workflow goroutine picks up `ActionQuit` at its next drain point (before each step in `Orchestrate`, or inside the error-mode `<-h.Actions` receive), returns from `Run`, and the TUI tears down.

## The signal path (Ctrl+C / SIGTERM)

The OS signal handler in `main.go` listens for SIGINT and SIGTERM on a buffered channel. When a signal arrives, the handler goroutine:

1. Closes a `signaled` one-shot channel (used later for exit-code selection)
2. Calls `keyHandler.ForceQuit()` — same call as the `y` path, so the subprocess gets terminated and `ActionQuit` gets injected
3. Calls `app.Stop()` to tear down the Glyph TUI
4. Waits up to 2 seconds for the workflow goroutine to finish (so the log file is flushed)
5. Calls `os.Exit(1)` directly

Because the signal handler calls `ForceQuit`, **the signal path and the `q`→`y` path produce identical behavior from the workflow's perspective.** The only difference is the exit code: SIGINT/SIGTERM exits 1, a normal `q`→`y` shutdown exits with whatever status the workflow last had — which is usually 0.

The signal handler also runs whether or not the TUI is currently in Normal, Error, QuitConfirm, or Done mode — signals bypass the mode dispatcher entirely.

## The normal-completion path

When `Run` finishes all iterations and finalize steps, it switches to `ModeDone` and blocks on `<-keyHandler.Actions` for one final keypress:

```
Footer: done — press any key to exit
```

Pressing any key triggers `handleDone`, which sends `ActionQuit` to unblock `Run`. `Run` then closes the executor and returns, the workflow goroutine calls `signal.Stop` / `log.Close` / `app.Stop`, and `main` exits 0.

This path does **not** go through `ForceQuit` because there's no subprocess to terminate and no shutdown to unwind — the workflow already finished.

## Exit codes

| Shutdown path | Exit code |
|---------------|-----------|
| Normal completion → any key in `ModeDone` | `0` |
| `q` → `y` | `0` (the workflow returned normally from `Run`) |
| SIGINT / SIGTERM | `1` |
| `buildStep` error at startup (no valid config) | `1` |
| Validator errors before the TUI starts | `1` |
| Glyph returned an error from `app.Run` | `1` |

If you're scripting ralph-tui, only `0` means "ran to completion". Any non-zero means "something interrupted us or broke before we started". The main goroutine uses the `signaled` channel as a defensive second check in case `app.Run` returns before the signal handler exits — but in practice, the signal handler exits the process directly.

## What you see during the `Quitting...` window

Between pressing `y` and the process actually exiting, you're briefly in `ModeQuitting`:

- Footer shows `Quitting...`
- Checkbox grid still shows whatever state it was in
- Log panel still shows the last run output
- No keypresses are handled (no handler is registered for `ModeQuitting`)

The window is usually a fraction of a second — just long enough for the subprocess to receive its SIGTERM and the orchestration goroutine to drain its pending channel operations. For a long-running subprocess that doesn't honor SIGTERM, the window is up to 3 seconds (the SIGKILL fallback in `Runner.Terminate`).

## Not every keypress quits

Some interactions look like they might quit but don't:

- **`q` in `ModeDone`** — `ModeDone` treats *any* key as `ActionQuit`, so `q` works, but it's not special. You could press `space` or `\n` and get the same result.
- **`n` in Normal mode** — `n` means "skip the current step", not "quit". It sends SIGTERM to the subprocess and advances to the next step. See [Recovering from Step Failures](recovering-from-step-failures.md) for how skips interact with the workflow.
- **`Esc` in Normal or Error mode** — Escape only cancels a quit confirmation. Outside of `ModeQuitConfirm`, it's ignored.

## Related documentation

- [Reading the TUI](reading-the-tui.md) — What the footer looks like in each mode (`Quit ralph?`, `Quitting...`, etc.)
- [Recovering from Step Failures](recovering-from-step-failures.md) — The `q` entry point from error mode
- [Keyboard Input & Error Recovery](../features/keyboard-input.md) — `ModeQuitConfirm`, `ModeQuitting`, `ForceQuit`
- [Signal Handling & Shutdown](../features/signal-handling.md) — SIGINT/SIGTERM handler, exit code selection, cleanup ordering
- [Subprocess Execution & Streaming](../features/subprocess-execution.md) — `Runner.Terminate` (SIGTERM then SIGKILL after 3s)
