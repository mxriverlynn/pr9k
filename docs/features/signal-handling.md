# Signal Handling & Shutdown

Handles OS signals (SIGINT/SIGTERM) to trigger clean shutdown of the ralph-tui workflow, terminating the active subprocess and exiting with the appropriate status code.

- **Last Updated:** 2026-04-10
- **Authors:**
  - River Bailey

## Overview

- Listens for SIGINT and SIGTERM via `os/signal.Notify` on a buffered channel
- On signal receipt, calls `KeyHandler.ForceQuit()` which terminates the active subprocess and injects `ActionQuit` into the orchestration channel
- The orchestration loop picks up the quit action before the next step starts via a non-blocking channel drain
- The main goroutine tracks whether a signal was received to select the exit code: 0 for normal completion, 1 for signal-initiated shutdown

Key files:
- `ralph-tui/cmd/ralph-tui/main.go` — Signal setup, signal handler goroutine, exit code selection

## Architecture

```
  OS Signal
  (SIGINT / SIGTERM)
       │
       ▼
  ┌──────────┐
  │ sigChan  │  buffered channel (cap 1)
  │ (os.Signal)│
  └────┬─────┘
       │
       ▼
  signal handler goroutine:
  ┌──────────────────────────────────────────────┐
  │  <-sigChan                                   │
  │  close(signaled)         ← one-shot flag     │
  │  keyHandler.ForceQuit()  ← terminate sub +  │
  │                             inject ActionQuit│
  │  app.Stop()              ← stop the TUI      │
  │  wait on <-done or 2s timeout                │
  │  os.Exit(1)              ← direct exit       │
  └──────────────────────────────────────────────┘
       │
       ├───▶ Runner.Terminate()     → SIGTERM subprocess, SIGKILL after 3s
       │
       └───▶ Actions <- ActionQuit  → non-blocking send to orchestration
                │
                ▼
          Orchestrate() drains Actions
          before each step → sees ActionQuit
          → returns ActionQuit
          → Run() closes and returns

  workflow goroutine (normal completion path):
  ┌──────────────────────────┐
  │  workflow.Run(...)       │
  │  signal.Stop(sigChan)    │  ← deregister signal handler
  │  log.Close()             │  ← flush and close log file
  │  app.Stop()              │  ← stop the TUI
  │  close(done)             │
  └──────────────────────────┘

  main goroutine (after app.Run() returns):
  ┌─────────────────────────────────────────┐
  │  wait on <-done or 2s timeout           │
  │                                         │
  │  select {                               │
  │  case <-signaled:                       │
  │    os.Exit(1)   ← signal path (already │
  │                    exited above, but    │
  │                    defensive fallback)  │
  │  default:                               │
  │    os.Exit(0)   ← normal completion    │
  │  }                                      │
  └─────────────────────────────────────────┘
```

## Implementation Details

### Signal Registration

In `main.go`, signals are registered before the workflow goroutine starts:

```go
sigChan := make(chan os.Signal, 1)
signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)
signaled := make(chan struct{})
go func() {
    <-sigChan
    close(signaled)
    keyHandler.ForceQuit()
    app.Stop()
    select {
    case <-done:
    case <-time.After(2 * time.Second):
    }
    os.Exit(1)
}()
```

The `signaled` channel is a one-shot flag — once closed, it stays closed. The signal handler calls `app.Stop()` to tear down the Glyph TUI, waits up to 2 seconds for the workflow goroutine to finish (so the log is flushed), then calls `os.Exit(1)` directly.

### ForceQuit Integration

`KeyHandler.ForceQuit()` does two things:
1. Calls the `cancel` function (which is `Runner.Terminate()`) to stop the active subprocess
2. Non-blocking sends `ActionQuit` to the `Actions` channel so the orchestration loop exits cleanly

The non-blocking send (`select`/`default`) ensures `ForceQuit` never blocks, which is critical since it runs in a signal handler goroutine.

`ForceQuit` is called from **two** places and both produce identical shutdown semantics:
- The OS signal handler goroutine (this file)
- The QuitConfirm `y` path in `KeyHandler.handleQuitConfirm` (see [Keyboard Input](keyboard-input.md))

Unifying these paths means the signal-initiated and user-confirmed quit flows go through the same subprocess-termination + ActionQuit-injection sequence. A test harness that drives `y` from an error-mode step gets the same behavior as sending SIGINT at the terminal.

### Pre-Step Drain

Before each step, `Orchestrate()` performs a non-blocking drain of the `Actions` channel:

```go
select {
case action := <-h.Actions:
    if action == ActionQuit { return ActionQuit }
default:
}
```

This catches the `ActionQuit` injected by `ForceQuit` even if the signal arrives between steps (when no goroutine is blocking on the channel).

### Workflow Goroutine Cleanup

On normal workflow completion, `signal.Stop`, `log.Close`, and `app.Stop` all run inside the workflow goroutine before `done` is closed:

```go
go func() {
    defer close(done)
    _ = workflow.Run(runner, header, keyHandler, runCfg)
    signal.Stop(sigChan)  // deregister signal handler
    _ = log.Close()       // flush and close the log file
    app.Stop()            // stop the Glyph TUI
}()
```

Placing cleanup here ensures it always runs when the workflow finishes naturally, without relying on the main goroutine to sequence it.

### Exit Code Selection

After `app.Run()` returns, the main goroutine waits for the workflow goroutine and checks whether a signal was received:

```go
select {
case <-done:
case <-time.After(2 * time.Second):
}

select {
case <-signaled:
    os.Exit(1)  // signal-initiated shutdown (defensive; signal handler already exited)
default:
    os.Exit(0)  // normal completion
}
```

In the signal path, the signal handler goroutine already called `os.Exit(1)` directly. The exit code check in main is a defensive fallback for the case where `app.Run()` returns before the signal handler fires.

## Testing

Signal handling is tested indirectly through:
- `ralph-tui/internal/ui/ui_test.go` — Tests for `ForceQuit` behavior (cancel called, ActionQuit sent)
- `ralph-tui/internal/ui/orchestrate_test.go` — Tests for pre-step quit drain (ActionQuit injected before step starts)

## Additional Information

- [Architecture Overview](../architecture.md) — System-level signal flow diagram
- [Keyboard Input & Error Recovery](keyboard-input.md) — ForceQuit method and Actions channel
- [Subprocess Execution & Streaming](subprocess-execution.md) — Terminate method (SIGTERM/SIGKILL)
- [Workflow Orchestration](workflow-orchestration.md) — Pre-step drain in Orchestrate
- [Concurrency](../coding-standards/concurrency.md) — Coding standards for non-blocking sends (critical for signal-safe ForceQuit)
