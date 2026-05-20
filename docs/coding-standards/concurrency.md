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

## Read stdout and stderr concurrently

When a subprocess produces output on both stdout and stderr, drain them in separate goroutines — never sequentially. A sequential drain (`io.ReadAll(stdout)` then `io.ReadAll(stderr)`) deadlocks if the subprocess writes more than the OS pipe buffer (typically 64 KB) to the unread pipe before the first pipe reaches EOF.

```go
// Bad — sequential drain deadlocks when stderr fills its pipe before stdout EOF
stdoutBytes, _ := io.ReadAll(cmd.StdoutPipe())
stderrBytes, _ := io.ReadAll(cmd.StderrPipe()) // blocked if stdout pipe is full

// Good — concurrent drain with WaitGroup
var (
    stdoutBytes []byte
    stderrBytes []byte
    wg          sync.WaitGroup
)
wg.Add(2)
go func() { defer wg.Done(); stdoutBytes, _ = io.ReadAll(stdout) }()
go func() { defer wg.Done(); stderrBytes, _ = io.ReadAll(stderr) }()
wg.Wait()
cmd.Wait()
```

This applies equally when one stream is being forwarded to a logger and the other is being captured:

```go
var wg sync.WaitGroup
wg.Add(2)
go func() { defer wg.Done(); captured, _ = io.ReadAll(stdout) }()
go func() { defer wg.Done(); forwardToLog(stderr) }()
wg.Wait()
```

A code comment claiming "stderr is read after stdout" is a latent bug report — if you see that comment, the fix is to move to the concurrent pattern.

## Document or synchronize setters on types with goroutines

When a type starts goroutines (via `Start()`) and also has setter methods (`SetSender`, `SetModeGetter`), the setters are inherently racy if they can be called after `Start()` returns. Either:

1. **Document** a clear precondition: "callers must invoke all setters before calling `Start()`." Add this to the godoc of both the setters and `Start()`.
2. **Synchronize** with a dedicated mutex (`setterMu sync.RWMutex`): setters take a write lock; the goroutine that reads the value takes a read lock. This eliminates the ordering requirement at the cost of a small lock.

Choose synchronization when callers are likely to be wired from different goroutines or after `Start()` is already running. Choose documentation when the precondition is enforced by a clear initialization sequence (e.g., the dependency is injected before the program event loop starts).

```go
// Good — synchronized setters; no ordering requirement on callers
type Runner struct {
    setterMu   sync.RWMutex
    sender     func(interface{})
    modeGetter func() string
}

func (r *Runner) SetSender(fn func(interface{})) {
    r.setterMu.Lock()
    defer r.setterMu.Unlock()
    r.sender = fn
}

func (r *Runner) execScript() {
    r.setterMu.RLock()
    sender := r.sender
    modeGetter := r.modeGetter
    r.setterMu.RUnlock()
    // use snapshot; mutex not held during slow operations
    _ = sender
    _ = modeGetter
}
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

## Mutually exclusive state flags: first-flag-wins

When multiple concurrent conditions can each transition a step or operation to a terminal state (e.g., user cancellation vs. timeout), model them as mutually exclusive boolean flags and enforce that only the first condition to fire wins.

The risk: if both flags can be set, the downstream code that reads them produces contradictory results — for example, a step logged as `status="done"` with `notes="timed out after 30s"`.

```go
// Good — first flag to fire wins; second flag is a no-op
r.processMu.Lock()
if !r.timeoutFired && !r.terminated {
    r.terminated = true
}
r.processMu.Unlock()

// In the timeout goroutine:
r.processMu.Lock()
if !r.terminated && !r.timeoutFired {
    r.timeoutFired = true
}
r.processMu.Unlock()
```

When resetting flags (e.g., before a retry), reset both flags together under the same mutex lock to prevent a window where one is cleared and the other is stale.

Checklist:
1. Identify all conditions that can terminate the same operation.
2. Gate each flag write with "are all other flags currently false?"
3. Reset all flags atomically before any retry.
4. Verify that every downstream read is unambiguous given the mutual exclusion guarantee.

## Record state before resetting for retry

When a step fails (timeout, error) and the loop retries it, emit any audit record (e.g., `IterationRecord`, log entry) for the failed attempt *before* resetting the failure flags. Resetting first and then recording produces a record that lacks the failure context, defeating the audit trail.

```go
// Good — record the timed-out attempt, then reset for retry
if executor.WasTimedOut() {
    appendIterationRecord(rec) // record the failure with notes set
}
// Now reset so the retry doesn't inherit stale state
executor.ResetTimeout()

// Bad — reset first, then attempt to record (flags are gone)
executor.ResetTimeout()
if executor.WasTimedOut() { // always false now
    appendIterationRecord(rec) // never reached
}
```

Apply whenever a retry loop needs an audit trail of failed attempts.

## Deep-copy reference-type fields before passing structs to goroutines

When passing a struct to a goroutine (e.g., as a validator input), a plain value copy is shallow — slice and map fields still share their underlying backing storage with the original. If the UI goroutine appends to a slice or writes a map key while the validator goroutine iterates, you have a data race.

The fix: explicitly copy every reference-type field in the copy function.

```go
// Bad — shallow copy; doc.Env and doc.ContainerEnv share backing storage
// with the original; validator goroutine and UI goroutine race on them.
func shallowCopyDoc(doc WorkflowDoc) WorkflowDoc {
    return doc
}

// Good — explicit deep copy; goroutine has its own independent slice and map.
func deepCopyDoc(doc WorkflowDoc) WorkflowDoc {
    cp := doc
    if len(doc.Env) > 0 {
        cp.Env = make([]string, len(doc.Env))
        copy(cp.Env, doc.Env)
    }
    if len(doc.ContainerEnv) > 0 {
        cp.ContainerEnv = make(map[string]string, len(doc.ContainerEnv))
        for k, v := range doc.ContainerEnv {
            cp.ContainerEnv[k] = v
        }
    }
    // Pointer fields: copy the pointed-to value if the goroutine might write through it.
    if doc.StatusLine != nil {
        sl := *doc.StatusLine
        cp.StatusLine = &sl
    }
    return cp
}
```

Checklist when writing a copy function for cross-goroutine use:
1. Slice fields → `make` a new slice and `copy` the elements.
2. Map fields → `make` a new map and range-copy every entry.
3. Pointer fields the goroutine may write through → copy the pointed-to value and take the address of the copy.

The `go test -race` flag catches these races at test time — always run it before marking a cross-goroutine data path safe.

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

## Two-goroutine signal handler: cleanup then forced exit

When a long-running goroutine (e.g., a workspace teardown that may block on RPCs) must be interruptible, use a two-goroutine signal handler:

1. **Cleanup goroutine** — receives the first signal, sets the shuttingDown flag, closes an internal started gate, then invokes `teardownOnce.Do(teardownFn)`. It may block during teardown; that is acceptable.
2. **Watchdog goroutine** — waits on the started gate (ensuring cleanup has consumed the first signal), then waits for a second signal and calls `exitFn(1)` immediately — without waiting for the cleanup goroutine to finish.

The started gate channel prevents a race where both goroutines consume signals from the same channel: the watchdog only begins listening after the cleanup goroutine has confirmed it received the first signal.

Pass a `done` channel that the caller closes when the monitored operation returns normally. Both goroutines select on `done` so they exit cleanly when no signal was received, rather than leaking blocked goroutines if the caller returns without terminating the process.

```go
func runCmuxSignalHandler(
    sigCh <-chan os.Signal,
    done <-chan struct{},
    teardownOnce *sync.Once,
    teardownFn func(),
    setShuttingDown func(),
    exitFn func(int),
) {
    started := make(chan struct{})

    // Cleanup: first signal → set flag → close gate → run teardown (may block).
    go func() {
        select {
        case <-sigCh:
            setShuttingDown()
            close(started)
            teardownOnce.Do(teardownFn)
        case <-done:
        }
    }()

    // Watchdog: wait for cleanup to start, then force exit on second signal.
    go func() {
        select {
        case <-started:
        case <-done:
            return
        }
        select {
        case <-sigCh:
            exitFn(1)
        case <-done:
        }
    }()
}
```

The caller closes `done` after `signal.Stop(sigCh)` so no new signals can arrive after `done` is closed. Inject `exitFn` rather than calling `os.Exit` directly so the handler is testable without spawning a subprocess. Inject `setShuttingDown` as a separate argument so tests can verify flag transitions independently from `teardownFn` execution.

## sync.Once teardown with named-return error injection

When a function can exit via multiple code paths (normal completion, context cancellation, partial-setup failure) and teardown must run exactly once, combine `sync.Once` with a named return value:

- Wrap teardown in a `sync.Once` closure so any code path can call it without double-execution.
- Use a named return value (`returnErr`) in the function signature.
- In the deferred func, inject the teardown error into `returnErr` when the primary return would otherwise be nil. This prevents silently swallowing teardown failures on the success path.

```go
func RunPhase1(ctx context.Context, ...) (returnErr error) {
    var (
        teardownOnce sync.Once
        teardownErr  error
    )
    runTeardown := func() {
        teardownOnce.Do(func() {
            if err := client.WorkspaceClose(context.Background(), name); err != nil {
                teardownErr = err
            }
        })
    }
    defer func() {
        runTeardown()
        if returnErr == nil && teardownErr != nil {
            returnErr = fmt.Errorf("workspace close failed: %w", teardownErr)
        }
    }()

    // ... setup steps that may return early on error ...

    // Normal path: blocks until dismissal or cancellation.
    select {
    case <-ctx.Done():
        return ctx.Err()
    case evt := <-obs.Ch:
        return nil // teardown still runs via defer
    }
}
```

The `context.Background()` in the teardown closure is intentional: the parent `ctx` is already cancelled on the signal-driven path, so teardown needs its own fresh context for cleanup RPCs.

## Check ctx.Err() after timer.C fires in a select

When a goroutine selects on both `ctx.Done()` and `timer.C`, the Go runtime chooses randomly when both are ready simultaneously. Add an explicit `ctx.Err() != nil` check immediately after the timer fires before doing any work:

```go
for {
    t := time.NewTimer(pollInterval)
    select {
    case <-ctx.Done():
        t.Stop()
        return
    case <-t.C:
    }

    // Guard against ctx cancellation that arrived exactly as the timer fired.
    if ctx.Err() != nil {
        return
    }

    // ... do poll work ...
}
```

Without the guard, a cancellation that races with a timer expiry can cause one extra poll iteration to run after the goroutine was supposed to stop — potentially issuing an RPC against a closed connection or reporting a spurious error.

## Write goroutine closes conn on exit to unblock the reader

In a three-goroutine connection model (read + write + watcher), the write goroutine must close `nc` when it exits so the paired read goroutine's blocked `io.ReadFull` unblocks promptly. Without this, a write failure leaves the reader waiting until the watcher eventually cancels the context — adding unnecessary latency to connection teardown.

```go
// Write goroutine: serializes outbound messages; closes nc on exit.
go func() {
    defer c.wg.Done()
    defer c.removeConn(co)
    c.writeLoop(nc, co)
    // Close nc so readLoop's io.ReadFull unblocks promptly on write failure,
    // rather than waiting for ctx cancellation to reach the watcher goroutine.
    _ = nc.Close()
}()

// Watcher goroutine: closes nc when the Channel context is done (normal shutdown).
go func() {
    defer c.wg.Done()
    <-c.ctx.Done()
    _ = nc.Close()
}()
```

`net.Conn.Close()` is idempotent, so the watcher's `nc.Close()` on context cancellation and the write goroutine's `nc.Close()` on write failure do not interfere.

## Stop() must join every goroutine it starts before returning

Any `Stop()`, `Close()`, or `Shutdown()` method that claims to fully stop a component must join — via channel drain or WaitGroup — every goroutine started by that component. A partial stop (one that initiates shutdown without joining) makes callers unsafe: code that runs after `Stop()` returns may race with goroutines still doing network I/O or accessing shared state.

When `Stop()` closes a socket to interrupt an in-flight I/O goroutine, it must drain the goroutine's done channel before returning:

```go
case <-c.done:
    // Stop() called while I/O goroutine is in-flight.
    // disconnect() closes the socket so the goroutine exits promptly;
    // <-ioDone joins it so Stop() truly completes before returning.
    timer.Stop()
    disconnect()
    call.reply <- rpcResult{err: fmt.Errorf("cmuxctl: %s: client stopped", call.method)}
    <-ioDone
    return
```

The same obligation applies to background goroutines started during construction: if `New()` starts a goroutine, `Stop()` must join it even for goroutines that were never given work.

## Don't spawn goroutines to close resources that Close() already handles

Never spawn an anonymous goroutine whose sole purpose is to close a resource (listener, connection, file) that an explicit `Close()` method already closes. Such goroutines:

- Are not tracked by any WaitGroup, so they can execute after `Close()` has already returned and cleaned up.
- Create a double-close race — both the anonymous goroutine and `Close()` call `Close()` on the same resource.
- Add noise without adding any lifecycle guarantee the caller can rely on.

```go
// Bad — anonymous goroutine races with the explicit ln.Close() in Close().
func (c *Channel) Serve(socketPath string) error {
    ln, err := net.Listen("unix", socketPath)
    // ...
    go func() { <-c.ctx.Done(); ln.Close() }() // redundant and racy
    // ...
}

func (c *Channel) Close() {
    c.cancel()
    ln.Close() // already closed here; anonymous goroutine races this
    c.wg.Wait()
}

// Good — context cancellation unblocks Accept; the explicit Close() is sufficient.
func (c *Channel) Close() {
    c.cancel()
    _ = c.ln.Close() // unblocks Accept in the accept goroutine
    c.wg.Wait()
}
```

If context cancellation alone does not unblock a blocking call (e.g., `Accept`), close the resource explicitly in `Close()` — not via an anonymous goroutine.

## Prevent busy-spin with a Ready() channel on input interfaces

When a goroutine polls for input in a loop, never use a `default:` branch in the select — it spins at 100% CPU when no input is available.

Instead, add a `Ready() <-chan struct{}` method to the input interface. The goroutine selects on `Ready()` before calling `Next()`:

```go
type FooterKeySource interface {
    // Next returns the next available key. Returns ("", false) when empty —
    // callers must wait for Ready() to fire before calling Next.
    Next() (string, bool)
    // Ready returns a channel that fires when at least one key is available.
    // A nil return blocks forever (used by the noop production implementation).
    Ready() <-chan struct{}
}

// Production goroutine — blocks until input or context cancellation.
go func() {
    defer wg.Done()
    for {
        select {
        case <-ctx.Done():
            return
        case <-src.Ready():
            machine.Step() // calls src.Next() inside
        }
    }
}()
```

The two canonical implementations:

```go
// Noop (production, non-interactive pane): nil channel blocks select forever.
type noopFooterKeySource struct{}
func (noopFooterKeySource) Next() (string, bool)   { return "", false }
func (noopFooterKeySource) Ready() <-chan struct{}  { return nil } // nil blocks forever in select

// Fake (test double): capacity-1 level-trigger channel.
// Press signals it when the queue transitions from empty to non-empty.
// Next re-signals it if keys remain after popping.
type FakeFooterKeySource struct {
    mu    sync.Mutex
    keys  []string
    keyCh chan struct{} // capacity 1; level-triggered
}

func NewFakeFooterKeySource() *FakeFooterKeySource {
    return &FakeFooterKeySource{keyCh: make(chan struct{}, 1)}
}

func (f *FakeFooterKeySource) Press(key string) {
    f.mu.Lock()
    f.keys = append(f.keys, key)
    f.mu.Unlock()
    select { case f.keyCh <- struct{}{}: default: } // signal without blocking
}

func (f *FakeFooterKeySource) Next() (string, bool) {
    f.mu.Lock()
    if len(f.keys) == 0 { f.mu.Unlock(); return "", false }
    k := f.keys[0]; f.keys = f.keys[1:]; remaining := len(f.keys)
    f.mu.Unlock()
    if remaining > 0 { select { case f.keyCh <- struct{}{}: default: } } // re-signal for next key
    return k, true
}

func (f *FakeFooterKeySource) Ready() <-chan struct{} { return f.keyCh }
```

Apply any time a goroutine needs to poll an interface for availability. A `default:` branch in a select on a polling loop is always a busy-spin — replace it with a `Ready()` signal channel.

## Pass mutable view state as a parameter, not a shared field

When two goroutines both access a struct field — one goroutine writes it, the other reads it during rendering — the race can sometimes be eliminated entirely by removing the field and passing the value as a function parameter.

This approach is preferable to adding a mutex when the value is produced and consumed in the same call chain: one goroutine computes the value and immediately passes it to `Render(value)` without storing it anywhere.

```go
// Bad — two goroutines race on renderer.shortcutLine:
// keystroke goroutine writes it via SetShortcutLine;
// main select loop reads it via Render.
type cmuxFooterRenderer struct {
    shortcutLine string // data race: written by goroutine A, read by goroutine B
}

func (r *cmuxFooterRenderer) SetShortcutLine(line string) { r.shortcutLine = line }
func (r *cmuxFooterRenderer) Render(sl string) string     { return r.shortcutLine + " " + sl }

// goroutine A:
r.renderer.SetShortcutLine(line)
fmt.Fprint(out, r.renderer.Render(sl))

// Good — shortcutLine is never stored; passed directly as a parameter:
type cmuxFooterRenderer struct{} // no shared mutable state

func (r *cmuxFooterRenderer) Render(sl, shortcutLine string) string {
    return shortcutLine + " " + sl
}

// Both goroutines compute and pass shortcutLine locally; no race possible.
fmt.Fprint(out, r.renderer.Render(sl, line))
```

Checklist for applying this pattern:
1. Confirm the field is written immediately before it is read (same goroutine, same call).
2. Confirm no other goroutine reads the field between writes.
3. Delete the field; add a parameter to the function that was reading it.
4. Run `go test -race` to verify the race is gone.

When the value needs to persist across calls from different goroutines, use a mutex instead (see "Protect all shared io.Writer writes" above).

## Two-layer cleanup: explicit graceful path + deferred safety net

When a component performs cleanup at the end of its lifecycle (e.g., clearing a sidebar status, closing a connection), register the cleanup in two layers:

1. **Explicit call on the graceful path** — after the main operation completes normally, call cleanup directly so it runs in the correct context before any downstream teardown.
2. **Outer `defer` as safety net** — at the construction site (the outer scope that owns the component), register a `defer` that calls the same cleanup function. This ensures cleanup runs even on abort and panic paths that never reach the graceful call.

```go
// Outer scope: construction + safety-net defer.
sidebar := newCmuxSidebar(client, ws, log)
defer func() { _ = sidebar.ClearAll(context.Background()) }()
// D-7: safety-net clear for panic/abort paths; inner ClearAll in
// runCmuxWorkflowAdapted covers the graceful path.

exitCode := runCmuxWorkflowAdapted(ctx, ch, log, projectDir, workflowDir, sf, sidebar)
```

```go
// Inner function: explicit graceful-path cleanup before returning.
result := workflow.Run(runner, wrappedHeader, keyHandler, runCfg)
keyHandler.SetMode(ui.ModeDone)
_ = sidebar.ClearAll(ctx) // graceful-path sidebar cleanup (D-6)
```

The two calls are intentional and not redundant: the inner call happens while the function context is still valid (the `ctx` derived from the main context, with its deadline and cancellation). The outer deferred call uses `context.Background()` because at that point the primary context is already cancelled. On the graceful path the cleanup runs twice — which is acceptable when the cleanup is idempotent.

**Checklist for applying this pattern:**
1. Ensure the cleanup function is idempotent (safe to call twice).
2. Label the outer `defer` with a comment that names the abort/panic path it covers and the inner call it complements.
3. Label the inner explicit call with a comment that names the design decision it satisfies.
4. If the cleanup returns an error, log or capture it rather than silently discarding it on the graceful path.

Apply any time a component writes observable external state (UI display, file locks, database records) that must be cleared on normal exit AND abnormal exit.

## Additional Information

- [Architecture Overview](../architecture.md) — System-level architecture showing how concurrency patterns fit together
- [Subprocess Execution & Streaming](../features/subprocess-execution.md) — sendLine and Terminate use mutex snapshot; WaitGroup pipe drain
- [TUI Display](../features/tui-display.md) — Dual-path shutdown, post-event-loop drain, and mutex-protected ShortcutLine access; tea.Cmd wrappers for Terminate and ForceQuit
- [Keyboard Input & Error Recovery](../features/keyboard-input.md) — Channel-based action dispatch, non-blocking sends in ForceQuit, and mutex-protected ShortcutLine getter; keysModel.Update as the canonical tea.Cmd blocking-wrap example
- [Signal Handling & Shutdown](../features/signal-handling.md) — Non-blocking send for signal-safe ForceQuit
- [Workflow Orchestration](../features/workflow-orchestration.md) — Non-blocking drain before each orchestration step
- [File Logging](../code-packages/logger.md) — Mutex-protected concurrent writes from scanner goroutines
- [API Design](api-design.md) — Complementary standards for unexported fields with protected getters
- [Error Handling](error-handling.md) — Complementary standards for goroutine write error tracking
- [Testing](testing.md) — Standards for test doubles with shared state needing mutexes; injecting signals for blocking receives
- [Keyboard Input & Error Recovery](../features/keyboard-input.md) — Error-mode blocking receive as the canonical channel-priming example
- [Workflow Orchestration](../features/workflow-orchestration.md) — `terminated` / `timeoutFired` mutual exclusion and reset-then-record ordering in `stepDispatcher` (issue #130)
- [Workflow Builder](../code-packages/workflowedit.md) — `deepCopyDoc` as the canonical reference-type deep-copy example; race between validator goroutine and UI goroutine on `doc.Env` / `doc.ContainerEnv` (workflow-builder-pt-2 review issue #1)
- `src/cmd/pr9k/cmux_signal.go` — `runCmuxSignalHandler` as the canonical two-goroutine watchdog+cleanup example (issue #223)
- `src/internal/cmuxctl/runphase1.go` — `RunPhase1` as the canonical `sync.Once` + named-return teardown error injection example (issue #221/224)
- `src/internal/cmuxctl/dismissal.go` — `DismissalObserver.run` as the canonical ctx.Err() guard after timer.C fire example (issue #222)
- `src/internal/interactionchannel/channel.go` — `startConn` as the canonical write-goroutine-closes-conn pattern; absence of anonymous listener-close goroutine as the canonical redundant-goroutine avoidance example (cmux-p2 review F1/F2)
- `src/internal/cmuxctl/real.go` — `run` method `<-ioDone` drain in the `<-c.done` case as the canonical Stop()-must-join-all-goroutines example (cmux-p2 review F3)
- `src/cmd/pr9k/cmux_pane.go` — `noopFooterKeySource` as the canonical nil-Ready() implementation; `runCmuxFooterMachineWith` as the canonical `<-src.Ready()` goroutine pattern (cmux-p3 review)
- `src/internal/interactionchannel/fake.go` — `FakeFooterKeySource` as the canonical level-trigger Ready() test double (cmux-p3 review)
- `src/cmd/pr9k/cmux_footer_wiring.go` — `footerPaneSink.SetMode` as the canonical pass-as-parameter (no shared field) race elimination (cmux-p3 review)
- `src/cmd/pr9k/cmux_pane.go` line 199–201 — `defer sidebar.ClearAll(context.Background())` as the canonical outer safety-net defer; `cmux_workflow.go` line 199 — explicit graceful-path `sidebar.ClearAll(ctx)` as the inner counterpart (Phase 4 W-6, D-6/D-7)
