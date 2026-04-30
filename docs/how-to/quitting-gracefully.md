# Quitting Gracefully

← [Back to How-To Guides](README.md)

pr9k always shuts down through the same path — whether you press `q`, hit `Ctrl+C`, or `kill` the process — so the running subprocess gets a chance to clean up before the TUI tears down. This guide explains the three quit entry points, what you see during shutdown, and what the exit code tells you.

**Prerequisites**: a run that's currently on screen, or a CI script that needs to know what exit code to expect. The keyboard map is in [Reading the TUI](reading-the-tui.md), and pressing `?` while pr9k is running shows the live keyboard reference.

> Heads-up before you press anything: pressing `n` does **not** quit. `n` is "skip the current step." The only quit shortcut is `q` followed by `y`, or `Ctrl+C`. Escape only cancels a quit confirmation — it does nothing in Normal mode.

## The three ways to quit

| Entry point | What it does |
|-------------|--------------|
| `q` in Normal or Error mode, then `y` | Footer flips to `Quitting...`, the running subprocess is terminated, the workflow returns from its loop |
| `Ctrl+C` (SIGINT) or `kill` (SIGTERM) | Same termination path as `q` → `y`, with up to 2 seconds to unwind cleanly before pr9k force-stops the TUI |
| Workflow completes normally | The workflow returns on its own, the TUI enters Done mode (`q quit` footer) — press `q` then `y` to exit |

The two interactive paths use the same termination logic — you get the same shutdown semantics whether you press `y` or hit Ctrl+C. The normal-completion path doesn't need to terminate anything because the workflow already finished.

## The `q` path step by step

Pressing `q` in Normal or Error mode does **not** immediately quit. It opens a confirmation prompt so a stray keypress doesn't abort a long-running workflow.

```
[You press q]
       │
       ▼
┌─────────────────────────────────────┐
│ Footer: Quit Power-Ralph.9000?      │
│   (y/n, esc to cancel)              │
└──────┬──────────┬───────────────────┘
       │          │
    y  │          │  n or Esc
       ▼          ▼
┌──────────┐  ┌──────────────┐
│ Quitting │  │ prev mode    │
│ shutdown │  │ (Normal or   │
│          │  │  Error)      │
└──────────┘  └──────────────┘
```

### Cancelling the quit

Pressing `n` or `Esc` while the footer shows `Quit Power-Ralph.9000? (y/n, esc to cancel)` restores your previous mode — Normal if you came from a running step, Error if you came from a failure pause — and puts the footer back to its previous shortcuts. No subprocess is touched, no action is injected.

This is non-destructive: if you were paused in error mode deciding whether to retry, cancelling a quit drops you right back into the same `c / r / q` decision with the same failed step still marked `[✗]`.

### Confirming the quit

Pressing `y` does two things in order:

1. **Footer immediately repaints to `Quitting...`** — visible feedback that pr9k accepted your confirmation and is now shutting down. Without this transition, you'd hit `y` and then sit there wondering whether the keypress registered.

2. **Termination begins** — the currently running subprocess receives SIGTERM. If the subprocess doesn't exit within 3 seconds, it's sent SIGKILL. A quit signal is also queued for the workflow loop.

The workflow loop picks up the quit signal at its next checkpoint (before each step, or while paused in error mode), returns from its main loop, and the TUI tears down.

## The signal path (Ctrl+C / SIGTERM)

When pr9k receives SIGINT (Ctrl+C) or SIGTERM, the signal handler:

1. Records that a signal arrived (so the exit code reflects it)
2. Triggers the same termination path as `q` → `y` — the subprocess gets SIGTERM, the workflow loop gets a quit signal
3. Waits up to 2 seconds for the workflow to unwind on its own
4. If the workflow hasn't finished, force-stops the TUI

**The signal path and the `q` → `y` path produce identical behaviour from the workflow's perspective.** The only difference is the exit code: SIGINT/SIGTERM exits 1, a normal `q` → `y` shutdown exits 0. Signals are honoured regardless of which mode the TUI is in.

## The normal-completion path

When all iterations and finalize steps finish, pr9k writes the completion summary to the log body and the TUI enters Done mode with a `q quit` footer. The process does **not** auto-exit — press `q` then `y` to exit so you have time to review the final output.

## Exit codes

| Shutdown path | Exit code |
|---------------|-----------|
| Normal completion | `0` |
| `q` → `y` | `0` |
| SIGINT / SIGTERM | `1` |
| Step preparation error at startup (no valid config) | `1` |
| Validator errors before the TUI starts | `1` |
| Unexpected TUI error | `1` |

If you're scripting pr9k, only `0` means "ran to completion". Any non-zero means "something interrupted us or broke before we started". A signal that arrives while the workflow is already finishing still produces exit code 1, so a wrapper script can rely on the distinction.

## What you see during the `Quitting...` window

Between pressing `y` and the process actually exiting:

- Footer shows `Quitting...`
- Checkbox grid still shows whatever state it was in
- Log panel still shows the last run output
- Keypresses are not handled

The window is usually a fraction of a second — just long enough for the subprocess to receive SIGTERM and the orchestrator to drain its pending work. For a long-running subprocess that doesn't honour SIGTERM, the window is up to 3 seconds before SIGKILL is sent.

## Not every keypress quits

Some interactions look like they might quit but don't:

- **`n` in Normal mode** — `n` means "skip the current step", not "quit". It enters a `Skip current step? y/n, esc to cancel` prompt. Pressing `y` confirms the skip and sends SIGTERM to the subprocess; pressing `n` or `Esc` cancels. See [Recovering from Step Failures](recovering-from-step-failures.md) for how skips interact with the workflow.
- **`Esc` in Normal or Error mode** — Escape only cancels a quit confirmation. Outside of the quit-confirm prompt, it's ignored.

## Related documentation

- ← [Back to How-To Guides](README.md)
- [Reading the TUI](reading-the-tui.md) — what the footer looks like in each mode (`Quit Power-Ralph.9000?`, `Quitting...`, etc.); press `?` while running for the live keyboard map
- [Recovering from Step Failures](recovering-from-step-failures.md) — the `q` entry point from error mode
- [Debugging a Run](debugging-a-run.md) — what's left in the persisted log file after you quit
- [Keyboard Input & Error Recovery](../features/keyboard-input.md) — keyboard mode state machine (contributor reference)
- [Signal Handling & Shutdown](../features/signal-handling.md) — SIGINT/SIGTERM handler, exit code selection, cleanup ordering
- [Subprocess Execution & Streaming](../features/subprocess-execution.md) — subprocess SIGTERM-then-SIGKILL termination
