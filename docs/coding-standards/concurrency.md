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

When multiple goroutines write to a shared `io.PipeWriter` (or any `io.Writer`), serialize every write under a mutex. Interleaved writes produce garbled output.

```go
r.mu.Lock()
fmt.Fprintln(r.logWriter, line)
r.mu.Unlock()
```

## Use io.Pipe for real-time subprocess streaming

To stream subprocess output to a UI component in real time, connect subprocess stdout/stderr through an `io.Pipe`. The Glyph `Log` component takes an `io.Reader`; the subprocess side writes to the `io.PipeWriter`.

```
subprocess stdout/stderr → goroutines → io.PipeWriter → io.PipeReader → Glyph Log
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

## Both shutdown paths must call terminal restore

When a TUI app has two paths that cause the event loop to stop — a signal path (SIGINT/SIGTERM) and a normal completion path — both must call the terminal restore function (`app.Stop()`). Missing it on either path leaves the terminal in raw mode after the process exits.

```go
// Signal path — triggered by SIGINT/SIGTERM
go func() {
    <-sigChan
    keyHandler.ForceQuit()
    app.Stop() // must be here
    // ...
}()

// Normal completion path — triggered when the workflow goroutine finishes
go func() {
    defer close(done)
    _ = workflow.Run(...)
    app.Stop() // must also be here
}()
```

If `app.Stop()` is only in the signal handler, normal exits corrupt the terminal. If it is only in the completion goroutine, signal exits corrupt the terminal.

## Drain background goroutines after the event loop exits

After a blocking event loop call (e.g., `app.Run()`) returns, use a `select` with a timeout to wait for background goroutines. The event loop may return before the workflow goroutine finishes — especially when `app.Stop()` is called from inside that goroutine.

```go
if err := app.Run(); err != nil {
    fmt.Fprintln(os.Stderr, "glyph:", err)
    os.Exit(1)
}

// app.Run() may return before the workflow goroutine closes done.
select {
case <-done:
case <-time.After(2 * time.Second):
}
```

Choose a timeout that is long enough to cover cleanup (flushing logs, deregistering signals) but short enough that a hung goroutine does not stall the process indefinitely. Two seconds is a reasonable default for in-process cleanup.

## Split pointer/mutex access for single-threaded event loop frameworks

When a UI framework reads a field via a pointer binding (e.g., Glyph's `Text(&field)`) and the same field is written by other goroutines, use two accessors with different safety contracts:

- **Pointer method** — returns `*string` for the framework's render loop. Safe because the event loop reads synchronously between write windows and the race detector does not flag this access pattern.
- **Mutex-protected method** — returns a copy for all other goroutines.

```go
type KeyHandler struct {
    mu           sync.Mutex
    shortcutLine string // protected by mu for concurrent callers
}

// ShortcutLine is safe to call from any goroutine.
func (h *KeyHandler) ShortcutLine() string {
    h.mu.Lock()
    defer h.mu.Unlock()
    return h.shortcutLine
}

// ShortcutLinePtr returns a pointer for Glyph's Text(&...) widget binding.
// Use only from code that runs inside the Glyph event loop — not from other goroutines.
func (h *KeyHandler) ShortcutLinePtr() *string {
    return &h.shortcutLine
}
```

Document which accessor is appropriate for which caller. This pattern should be attempted only after verifying that the exported-field approach (Option P) produces a real data race under `go test -race`.

## Additional Information

- [Architecture Overview](../architecture.md) — System-level architecture showing how concurrency patterns fit together
- [Subprocess Execution & Streaming](../features/subprocess-execution.md) — Mutex-protected io.Pipe writes, WaitGroup drain, and snapshot-then-unlock in Terminate
- [TUI Display & Glyph Wiring](../features/tui-display.md) — Dual-path shutdown, post-event-loop drain, and split pointer/mutex access for ShortcutLinePtr
- [Keyboard Input & Error Recovery](../features/keyboard-input.md) — Channel-based action dispatch, non-blocking sends in ForceQuit, and mutex-protected ShortcutLine getter
- [Signal Handling & Shutdown](../features/signal-handling.md) — Non-blocking send for signal-safe ForceQuit
- [Workflow Orchestration](../features/workflow-orchestration.md) — Non-blocking drain before each orchestration step
- [File Logging](../features/file-logging.md) — Mutex-protected concurrent writes from scanner goroutines
- [API Design](api-design.md) — Complementary standards for unexported fields with protected getters
- [Error Handling](error-handling.md) — Complementary standards for goroutine write error tracking
- [Testing](testing.md) — Standards for test doubles with shared state needing mutexes
