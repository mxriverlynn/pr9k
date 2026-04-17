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
actions := make(chan StepAction, 10)
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

When a Bubble Tea TUI app has two paths that cause the program to stop — a signal path (SIGINT/SIGTERM) and a normal completion path — both must trigger clean shutdown via the TUI's quit mechanisms. Missing one path leaves the program running or leaves the terminal in a bad state.

```go
// Signal path — triggered by SIGINT/SIGTERM; remains active during ModeDone
go func() {
    <-sigChan
    close(signaled)
    keyHandler.ForceQuit()
    select {
    case <-workflowDone:
    case <-time.After(2 * time.Second):
    }
    program.Kill() // always kill — safe even if workflow already finished
}()

// Normal completion path — workflow goroutine enters ModeDone; user quits via q→y
go func() {
    defer close(workflowDone)
    _ = workflow.Run(...)
    _ = log.Close()
    close(lineCh)
    keyHandler.SetMode(ui.ModeDone) // TUI stays alive for user review
}()
```

The workflow goroutine enters `ModeDone` on normal completion — the TUI stays alive so the user can review output and quit via `q` → `y` (which sends `tea.QuitMsg`). The signal handler goroutine blocks unconditionally on `<-sigChan` (no `case <-workflowDone` escape hatch) so it remains active during `ModeDone` — a SIGINT during the done screen still triggers `ForceQuit` + `program.Kill()`, restoring the terminal cleanly.

## Wait for background goroutines after program.Run() returns

After `program.Run()` returns (Bubble Tea's blocking event loop), deregister signal notifications and use a `select` with a timeout to wait for the workflow goroutine to finish cleanup. The program may stop before the workflow goroutine flushes logs or closes channels — particularly in the mid-workflow quit path, where `handleQuitConfirm`'s `tea.QuitMsg` causes `program.Run()` to return immediately after `ForceQuit`, racing the goroutine's `log.Close()` and `close(lineCh)`.

```go
_, runErr := program.Run()
signal.Stop(sigChan) // deregister after TUI exits cleanly

// Wait for the workflow goroutine to finish cleanup (log flush, channel close).
select {
case <-workflowDone:
case <-time.After(4 * time.Second):
}
```

The 4-second timeout exceeds the 3-second `terminateGracePeriod` in `runner.Terminate()` plus buffer for `log.Close()` and `close(lineCh)` — this prevents `os.Exit` from firing while SIGTERM→SIGKILL is still in progress during a mid-workflow quit.

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

## Wrap blocking operations in tea.Cmd closures

In Bubble Tea, the `Update` goroutine is the single-threaded event loop. Never block it with long-running calls (file I/O, subprocess waits, channel blocks, or external process invocations). Wrap any blocking operation in a `tea.Cmd` closure so it runs in a separate goroutine and sends a message back when done.

```go
// Bad — Terminate() blocks up to 3 seconds; freezes the event loop
func (m keysModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
    m.handler.ForceQuit() // sets mode + cancel
    m.handler.Terminate() // BLOCKS up to 3s — freezes all rendering
    return m, nil
}

// Good — blocking call runs in a goroutine; Update returns immediately
func (m keysModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
    m.handler.ForceQuit()
    return m, func() tea.Msg {
        m.handler.Terminate() // runs off the Update goroutine
        return tea.Quit()
    }
}
```

**External process calls require the same discipline.** `clipboard.WriteAll` shells out to `xclip`, `xsel`, or `pbcopy`. Calling it synchronously inside `Update()` freezes the TUI for the duration of the daemon round-trip (or indefinitely if the daemon is absent). The fix is identical — move the call into the returned cmd closure:

```go
// Bad — clipboard write blocks Update(); slow daemon freezes the TUI
func copySelectedText(text string) tea.Cmd {
    err := copyToClipboard(text) // shells out to xclip/pbcopy — may block
    return func() tea.Msg { return makeLogLinesMsg(err) }
}

// Good — blocking call is inside the closure, not before it
func copySelectedText(text string) tea.Cmd {
    return func() tea.Msg {
        err := copyToClipboard(text) // runs in a separate goroutine
        return makeLogLinesMsg(err)
    }
}
```

The same rule applies to `cancel()` context cancellations that trigger blocking waits, and to any channel send that might block. If it can take more than a few microseconds, it belongs in a cmd closure.

## Additional Information

- [Architecture Overview](../architecture.md) — System-level architecture showing how concurrency patterns fit together
- [Subprocess Execution & Streaming](../features/subprocess-execution.md) — sendLine and Terminate use mutex snapshot; WaitGroup pipe drain
- [TUI Display](../features/tui-display.md) — Dual-path shutdown, post-event-loop drain, and mutex-protected ShortcutLine access; tea.Cmd wrappers for Terminate and ForceQuit
- [Keyboard Input & Error Recovery](../features/keyboard-input.md) — Channel-based action dispatch, non-blocking sends in ForceQuit, and mutex-protected ShortcutLine getter; keysModel.Update as the canonical tea.Cmd blocking-wrap example
- [Signal Handling & Shutdown](../features/signal-handling.md) — Non-blocking send for signal-safe ForceQuit
- [Workflow Orchestration](../features/workflow-orchestration.md) — Non-blocking drain before each orchestration step
- [File Logging](../features/file-logging.md) — Mutex-protected concurrent writes from scanner goroutines
- [API Design](api-design.md) — Complementary standards for unexported fields with protected getters
- [Error Handling](error-handling.md) — Complementary standards for goroutine write error tracking
- [Testing](testing.md) — Standards for test doubles with shared state needing mutexes; injecting signals for blocking receives
- [Keyboard Input & Error Recovery](../features/keyboard-input.md) — Error-mode blocking receive as the canonical channel-priming example
