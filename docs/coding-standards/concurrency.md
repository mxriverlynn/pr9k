# Concurrency

## Snapshot-then-unlock for mutex-guarded state

When holding a mutex to read a pointer or value, snapshot it into a local variable before unlocking. This prevents TOCTOU races where the guarded value could change between unlock and use.

```go
// Good — snapshot under lock, use after unlock
r.processMu.Lock()
proc := r.currentProc
done := r.procDone
r.processMu.Unlock()

if proc == nil {
    return // no-op
}
proc.Signal(syscall.SIGTERM)

// Bad — unlock, then use r.currentProc (could be nil or changed)
r.processMu.Unlock()
r.currentProc.Signal(syscall.SIGTERM)
```

## Drain goroutines with WaitGroup before cmd.Wait()

When spawning goroutines to forward subprocess stdout and stderr, use a `sync.WaitGroup` to wait for both goroutines to finish draining their pipes before calling `cmd.Wait()`. Calling `cmd.Wait()` first closes the pipes and can cause goroutines to miss trailing output.

```go
var wg sync.WaitGroup
wg.Add(2)
go func() { defer wg.Done(); forward(stdoutPipe) }()
go func() { defer wg.Done(); forward(stderrPipe) }()
wg.Wait()
cmd.Wait()
```

## Protect all shared io.Writer writes with sync.Mutex

When multiple goroutines write to a shared `io.Writer`, serialize every write under a mutex. Interleaved writes produce garbled output. The `Logger` is the canonical example: scanner goroutines call `log.Log` concurrently, and every write is serialized by the logger's internal mutex:

```go
func (l *Logger) Log(stepName string, line string) error {
    l.mu.Lock()
    defer l.mu.Unlock()
    // ...
    _, err := fmt.Fprintln(l.writer, prefix+line)
    return err
}
```

## Use a sendLine callback for real-time subprocess streaming

To stream subprocess output to a Bubble Tea TUI in real time, install a `sendLine` callback via `SetSender`. Scanner goroutines call the callback for each line; the callback writes to a buffered channel; a drain goroutine coalesces lines into `LogLinesMsg` batches and sends them to the program:

```
subprocess stdout/stderr → scanner goroutines → sendLine callback → buffered channel → drain goroutine → program.Send(LogLinesMsg)
```

## Channel-based action dispatch for UI events

Use a buffered channel to dispatch user actions from key-handler goroutines to the orchestration loop. This decouples key handling from orchestration and avoids blocking the key-event callback.

```go
type StepAction int
const (
    ActionRetry StepAction = iota
    ActionContinue
    ActionQuit
)
actions := make(chan StepAction, 1)
```

## Non-blocking send for signal-safe channel writes

Signal handlers and any code that must not block must use a non-blocking select when writing to a channel. A direct send blocks if the channel is full; this causes a deadlock when the handler fires while the orchestration goroutine is not listening.

```go
// Good — never blocks; drops the send if the channel is already full
select {
case h.Actions <- ActionQuit:
default:
}

// Bad — blocks if channel is full (deadlock risk from signal handler)
h.Actions <- ActionQuit
```

## Unexported field + mutex-protected getter for cross-goroutine reads

When a field is written by one goroutine and read by another, make it unexported and expose it only through a mutex-protected getter. Exported fields bypass the mutex and invite data races in test code and callers.

```go
type KeyHandler struct {
    mu           sync.Mutex
    shortcutLine string
}

// ShortcutLine is safe to call from any goroutine.
func (h *KeyHandler) ShortcutLine() string {
    h.mu.Lock()
    defer h.mu.Unlock()
    return h.shortcutLine
}
```

## Non-blocking drain before each loop iteration

When an orchestration loop receives control signals through a channel, drain the channel with a non-blocking select at the top of each iteration. This picks up any signal (e.g., `ActionQuit` injected by `ForceQuit`) that arrived while the previous step was running, before the next step starts.

```go
for _, step := range steps {
    // Drain any pending quit injected while previous step was running.
    select {
    case action := <-h.Actions:
        if action == ActionQuit {
            return ActionQuit
        }
    default:
    }
    // ... run step ...
}
```

## Signal path and completion path must converge cleanly

When a Bubble Tea TUI app has two paths that cause the program to stop — a signal path (SIGINT/SIGTERM) and a normal completion path — both must trigger clean shutdown via `program.Quit()` or `program.Kill()`. Missing one path leaves the program running or leaves the terminal in a bad state.

```go
// Signal path — triggered by SIGINT/SIGTERM
go func() {
    select {
    case <-sigChan:
        keyHandler.ForceQuit()
        select {
        case <-workflowDone:
        case <-time.After(2 * time.Second):
            program.Kill() // force if workflow doesn't unwind
        }
    case <-workflowDone:
    }
}()

// Normal completion path — workflow goroutine calls program.Quit() itself
go func() {
    defer close(workflowDone)
    _ = workflow.Run(...)
    program.Quit() // signals Bubble Tea to stop its event loop
}()
```

The workflow goroutine calls `program.Quit()` on normal completion. The signal path calls `ForceQuit()` (which unwinds orchestration) and waits for the workflow goroutine with a 2-second grace period before `program.Kill()`.

## Wait for background goroutines after program.Run() returns

After `program.Run()` returns (Bubble Tea's blocking event loop), use a `select` with a timeout to wait for the workflow goroutine to finish cleanup. The program may stop before the workflow goroutine flushes logs or closes channels.

```go
_, runErr := program.Run()
// ...

// program.Run() may return before the workflow goroutine closes workflowDone.
select {
case <-workflowDone:
case <-time.After(2 * time.Second):
}
```

Choose a timeout long enough for cleanup (flushing logs, deregistering signals) but short enough that a hung goroutine does not stall the process indefinitely.

## Unexported field + mutex-protected getter for shortcut bar text

The shortcut bar string is written by mode transitions (on the Update goroutine) and read by `View()` (also on the Update goroutine via `ShortcutLine()`). Keep it unexported and expose it only through a mutex-protected getter so that signal handlers and test goroutines can also read it safely without races:

```go
type KeyHandler struct {
    mu           sync.Mutex
    shortcutLine string
}

// ShortcutLine is safe to call from any goroutine.
func (h *KeyHandler) ShortcutLine() string {
    h.mu.Lock()
    defer h.mu.Unlock()
    return h.shortcutLine
}
```

In the Bubble Tea architecture, `View()` calls `ShortcutLine()` directly via the mutex-protected getter. All mode mutations happen on the Update goroutine, which serializes writes naturally; the mutex guards reads from other goroutines (signal handlers, test code).

## Prime the channel before entering a blocking receive

When a goroutine transitions to a mode where it blocks on a channel receive (`<-ch`), ensure the channel is either buffered with a pending send already in it, or that a concurrent sender has been started before the blocking call. Entering a blocking receive with an empty channel and no ready sender is a deadlock.

The error-recovery path in `runStepWithErrorHandling` demonstrates the correct pattern: when a step fails, `Orchestrate` sets `ModeError` and then blocks on `<-h.Actions`. The channel is buffered (capacity 10) and the user's keypress (`c`, `r`, or `q`) is queued by the key handler goroutine before or during the blocking receive.

```go
// Good — channel is buffered; a pending send can't be lost if it arrives
// before the blocking receive. The keypress goroutine sends via the
// KeyHandler; the buffered channel absorbs it whether the send happens
// before or after the receive starts.
h.SetMode(ModeError)
action := <-h.Actions  // blocks until user presses c / r / q

// Bad — unbuffered channel; a send that arrives before the receive is lost
actions := make(chan StepAction) // unbuffered — race between sender and receiver
<-actions
```

When adding any new blocking receive to orchestration code:
1. Verify the channel is buffered (capacity ≥ 1) or that a goroutine is already blocked on the send.
2. Document which goroutine is responsible for sending to unblock the receive.
3. Update tests to inject the required signal (see [Testing — Inject an additional signal for each new blocking receive](testing.md)).

## Additional Information

- [Architecture Overview](../architecture.md) — System-level architecture showing how concurrency patterns fit together
- [Subprocess Execution & Streaming](../features/subprocess-execution.md) — sendLine snapshot-then-unlock, WaitGroup drain, and snapshot-then-unlock in Terminate
- [TUI Display](../features/tui-display.md) — Dual-path shutdown, post-event-loop drain, and mutex-protected ShortcutLine access
- [Keyboard Input & Error Recovery](../features/keyboard-input.md) — Channel-based action dispatch, non-blocking sends in ForceQuit, and mutex-protected ShortcutLine getter
- [Signal Handling & Shutdown](../features/signal-handling.md) — Non-blocking send for signal-safe ForceQuit
- [Workflow Orchestration](../features/workflow-orchestration.md) — Non-blocking drain before each orchestration step
- [File Logging](../features/file-logging.md) — Mutex-protected concurrent writes from scanner goroutines
- [API Design](api-design.md) — Complementary standards for unexported fields with protected getters
- [Error Handling](error-handling.md) — Complementary standards for goroutine write error tracking
- [Testing](testing.md) — Standards for test doubles with shared state needing mutexes; injecting signals for blocking receives
- [Keyboard Input & Error Recovery](../features/keyboard-input.md) — Error-mode blocking receive as the canonical channel-priming example
