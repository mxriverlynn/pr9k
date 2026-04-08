# Signal Handling & Shutdown

Handles OS signals (SIGINT/SIGTERM) to trigger clean shutdown of the ralph-tui workflow, terminating the active subprocess and exiting with the appropriate status code.

- **Last Updated:** 2026-04-08 12:00
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
  ┌──────────────────────────┐
  │  <-sigChan               │
  │  close(signaled)         │  ← marks that a signal was received
  │  keyHandler.ForceQuit()  │  ← terminates subprocess + injects ActionQuit
  └──────────────────────────┘
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
                │
                ▼
          main goroutine:
          ┌─────────────────────┐
          │  <-done             │
          │  signal.Stop(sigChan)│
          │  log.Close()        │
          │                     │
          │  select {           │
          │  case <-signaled:   │
          │    os.Exit(1)       │  ← signal-initiated
          │  default:           │
          │    os.Exit(0)       │  ← normal completion
          │  }                  │
          └─────────────────────┘
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
}()
```

The `signaled` channel is a one-shot flag — once closed, it stays closed for the exit code check.

### ForceQuit Integration

`KeyHandler.ForceQuit()` does two things:
1. Calls the `cancel` function (which is `Runner.Terminate()`) to stop the active subprocess
2. Non-blocking sends `ActionQuit` to the `Actions` channel so the orchestration loop exits cleanly

The non-blocking send (`select`/`default`) ensures `ForceQuit` never blocks, which is critical since it runs in a signal handler goroutine.

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

### Exit Code Selection

After the workflow goroutine completes, the main goroutine checks whether a signal was received:

```go
<-done
signal.Stop(sigChan)
_ = log.Close()

select {
case <-signaled:
    os.Exit(1)  // signal-initiated shutdown
default:
    os.Exit(0)  // normal completion
}
```

`signal.Stop` is called before exit to deregister the signal handler and avoid processing stale signals.

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
