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
