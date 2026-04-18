# Plan: Migrate ralph-tui TUI from Glyph to Bubble Tea

- **Status:** proposed
- **Date Created:** 2026-04-11
- **Authors:**
  - River Bailey (mxriverlynn, river.bailey@testdouble.com)
- **ADR:** [Use Bubble Tea for the TUI Framework](../adr/20260411070907-bubble-tea-tui-framework.md)

## Context

ralph-tui currently renders its TUI with [Glyph](https://useglyph.sh/) (`github.com/kungfusheep/glyph`). Glyph gives us a pointer-binding widget model, a `glyph.Log` widget, and vim-style keyboard scrollback — but it does not support dynamic titles (OS window or in-TUI border), and it does not emit mouse events. We are migrating to Bubble Tea + Lip Gloss + bubbles/viewport, which supports both natively and has the ecosystem leverage we chose as a decision driver in the [Cobra CLI ADR](../adr/20260409135303-cobra-cli-framework.md).

The ADR captures the *why*. This plan captures the *how*: what changes in each file, what stays the same, and how we verify the result end-to-end.

## Goals

**Must preserve:**

- Four-mode keyboard state machine (Normal / Error / QuitConfirm / Quitting) — `ralph-tui/internal/ui/ui.go:14-22`
- Three-color chrome hierarchy — light-gray frame (`PaletteColor(245)` → `lipgloss.Color("245")`), bright-white active step (ANSI 15), bright-green active marker (ANSI 10) — `internal/ui/header.go:15-28`
- 4-column checkbox grid with per-cell independent coloring of prefix / marker / suffix — `internal/ui/header.go:41-88` and `cmd/ralph-tui/main.go:100-111`
- 500-line log cap with auto-scroll-on-tail that pauses when the user scrolls up — `cmd/ralph-tui/main.go:124`
- Shortcut footer with version label pinned to the right — `cmd/ralph-tui/main.go:130-134`
- Real-time streaming of subprocess stdout/stderr into the log panel — `internal/workflow/workflow.go:97-176`
- Immediate process exit on normal workflow completion (no "press any key") — introduced in commit `4b36e78`
- Identical shutdown semantics between the signal path and the `q → y` confirm path, both routed through `KeyHandler.ForceQuit` — `internal/ui/ui.go:141-153`

**New features:**

- **Dynamic OS window title** via `tea.SetWindowTitle` — title format `"ralph-tui — <IterationLine>"`, examples: `ralph-tui — Initializing 1/2: Splash`, `ralph-tui — Iteration 2/5 — Issue #42`, `ralph-tui — Finalizing 1/3: Deferred work`
- **Dynamic in-TUI border title** on the rounded-border box — same string as the OS title, rendered into the top border row via a hand-built Lip Gloss border (because `lipgloss.Border` has no title slot)
- **Mouse-wheel scrolling** on the log panel via `bubbles/viewport` with `tea.WithMouseCellMotion()`

## Architecture after migration

```
┌─ cmd/ralph-tui/main.go ──────────────────────┐
│  cfg, runner, keyHandler, header (as data)    │
│  initialHeader := ui.NewStatusHeader(maxSteps)│
│  /* pre-populate first phase state */         │
│  model := ui.NewModel(initialHeader, ...)     │
│  proxy := ui.NewHeaderProxy()                 │
│  program := tea.NewProgram(model,             │
│      tea.WithMouseCellMotion(),               │
│      tea.WithAltScreen(),                     │
│      tea.WithoutSignalHandler())              │
│  proxy.SetSender(program.Send)                │
│  runner.SetSender(lineCh <- adapter) ── see §1│
│  /* drain goroutine batches then program.Send│
│     ui.LogLinesMsg{Lines: batch}  */          │
│  go workflow.Run(runner, proxy, ...)          │
│  go signalHandler(program, keyHandler,        │
│                   sigChan, workflowDone)      │
│  program.Run()                                │
└───────────────────────────────────────────────┘
             │ program.Send(msg)        ▲
             ▼                          │
┌─ internal/ui/model.go (new) ─────────────┐
│  type Model struct {                      │
│      header  headerModel                  │
│      log     logModel                     │
│      keys    keysModel                    │
│      width   int                          │
│      height  int                          │
│  }                                        │
│  Update(msg) routes on msg type:          │
│    tea.KeyMsg     → keys (mode dispatch)  │
│    logLinesMsg    → log (append + scroll) │
│    headerMsg      → header (render line,  │
│                     set step state, etc.) │
│    tea.WindowSizeMsg → all three          │
│    quitMsg        → tea.Quit              │
│  View() assembles top border (dynamic     │
│  title) + iteration line + grid + HRule   │
│  + viewport.View() + HRule + footer +     │
│  bottom border.                           │
└───────────────────────────────────────────┘
             ▲
             │ program.Send from workflow goroutine
┌─ internal/workflow/workflow.go ──────────┐
│  Runner { sendLine func(string) }        │
│  forwardPipe scans stdout/stderr and     │
│  calls sendLine(line) per line.          │
│  No more io.Pipe, no more LogReader.     │
└───────────────────────────────────────────┘
```

One-way data flow: workflow goroutine → `program.Send` → `Model.Update` → `View`. User decisions go the other direction via the unchanged `Actions` channel: `Model.Update` (on `tea.KeyMsg` in ModeError) → `keyHandler.Actions` → `Orchestrate`. The orchestration loop in `internal/workflow/run.go` keeps its imperative shape and is **not** converted to `tea.Cmd` chains.

## Key design decisions

### 1. `Runner` streams via `program.Send`, not `io.Pipe`

**Today:** `internal/workflow/workflow.go:42-48` constructs an `io.Pipe` in `NewRunner`; `forwardPipe` (lines 137-161) writes each line into the mutex-protected `PipeWriter`; `main.go:124` passes `runner.LogReader()` to `glyph.Log`.

**Change:** Remove the pipe. Add `Runner.sendLine func(string)` and a setter `Runner.SetSender(send func(string))`. The setter takes a string-typed sender (not `func(tea.Msg)`) so the `workflow` package does **not** need to import `github.com/charmbracelet/bubbletea`. The Bubble Tea adapter lives in `main.go` and goes through the buffered drain + coalescing goroutine described below — see the shape 1/2 mitigations below for the concrete wiring.

This respects the [Narrow Reading ADR](../adr/20260410170952-narrow-reading-principle.md): the workflow package stays library-independent, and only the UI package + `main.go` know about Bubble Tea.

`forwardPipe` becomes:

```go
for scanner.Scan() {
    line := scanner.Text()
    // capture + file log unchanged
    r.sendLine(line) // guaranteed non-nil: see sender-safety note below
}
```

The mutex in `workflow.go:154-156` goes away because `tea.Program.Send` is safe for concurrent use across goroutines. `Runner.LogReader` is deleted. `Runner.WriteToLog` (`workflow.go:199-202`) becomes a direct `r.sendLine(line)` call with the same mutex removal.

`Runner.Close()` (`workflow.go:207-209`) currently closes the pipe writer to send EOF to the UI-side reader. After the migration there is no pipe, so **`Runner.Close()` becomes a no-op returning `nil`**. It stays on the struct because `workflow.StepExecutor` (`internal/workflow/run.go:19`) requires it — `workflow.Run` calls `executor.Close()` at each exit point. Removing `Close` from the interface entirely is *out of scope* for this migration — that cleanup can happen in a follow-up PR.

`TestClose_IsIdempotent` and the ~26 other tests that today call `_ = r.Close(); lines := collect()` will need to change in lockstep with the helper rewrite. Today their drain is bound to the pipe EOF that `Close()` emits; after migration they become `lines := drain()` where `drain()` is the mutex-guarded slice snapshot from `newCapturingRunner` in section 8. The idempotency test survives in a simpler form: two `Close()` calls still return `nil` from a no-op body. Section 8's `newCapturingRunner` is the only entry point for this pattern — do not leave any test still calling `collectLines`.

**Sender must be non-nil at `RunStep`/`WriteToLog` call time.** To prevent a silent-drop failure mode where a test forgets to call `SetSender` and then asserts on an empty capture list, `NewRunner` initializes `sendLine` to a **sentinel that panics** with `"workflow.Runner: sendLine not set — call SetSender before running steps"`. Every production path (`main.go`) and every test helper (`newCapturingRunner` in `workflow_test.go`) must call `SetSender` before any `RunStep`. This makes the missing-wire bug loud. The nil check is not used.

**Why:** `program.Send` is the canonical Bubble Tea bridge from an external goroutine into the Update loop. No `io.Pipe` EOF dance, no separate reader goroutine on the UI side. Concurrency safety comes from `program.Send` itself — though not in the form the plan originally assumed:

**CRITICAL — `tea.Program.Send` IS BLOCKING.** Adversarial validation confirmed by reading `github.com/charmbracelet/bubbletea@v1.3.10/tea.go:244` and `tea.go:774-779`: `msgs` is an **unbuffered** channel and `Send` does:

```go
func (p *Program) Send(msg Msg) {
    select {
    case <-p.ctx.Done():
    case p.msgs <- msg:
    }
}
```

Every `Send` serializes against the Update+View round-trip inside `eventLoop`. The current test `TestRunStep_AllLinesArrivedBeforeCmdWait` (`workflow_test.go:147-157`) bursts 200 stderr lines in a tight loop. In production, each forwarded line would force a full Update (which runs `strings.Join` over up to 500 buffered lines) plus a View render plus a terminal write before the next line can be accepted. Under load — e.g., over SSH, in a slow tmux pane, or during a `make ci` subprocess dump — this backpressure propagates back through `forwardPipe` into the subprocess's stderr pipe buffer and eventually stalls the child's write syscalls.

**Mitigation (required, not optional):** Insert a small forwarding goroutine between `forwardPipe` and `program.Send` that batches AND drops under pressure. The batching half of this is not optional because of a second, compounding problem: `bubbles/viewport.SetContent` (at `bubbles@v1.0.0/viewport/viewport.go:125-133, 536-544`) does O(N) work on every call — it runs `strings.ReplaceAll`, `strings.Split`, and `findLongestLineWidth` across the entire content string. A naive "one message per line" design would call `SetContent(strings.Join(ringBuffer, "\n"))` per forwarded line, which after the 500-line cap saturates scans ≈500×80 ≈40 KB three times per message. Under a 10 k-line claude-review burst, aggregate content scanning reaches the **hundreds-of-MB range** — a hot O(N²) path, not the "sub-millisecond" cost a single-call micro-benchmark suggests.

Two shapes combine to fix both the `Send` backpressure and the `SetContent` amplification:

1. **Buffered queue + drop-on-full drain goroutine (required for Send back-pressure):** `Runner` owns a buffered `chan string` of **4096** entries; `forwardPipe` writes to the channel non-blockingly (drop on full with a rate-limited `[log panel] N lines dropped due to render backpressure` synthetic line); a single dedicated goroutine drains the channel and calls `program.Send`. Back-pressure is isolated to the drain goroutine.
2. **10 ms coalescing batcher inside the drain goroutine (required for `SetContent` amplification):** On every wakeup, the drain goroutine reads as many lines as are currently available in the channel (`for { select { case l := <-ch: batch = append(batch, l); default: break } }`), then sends a single `logLinesMsg{Lines: batch}` — or, when the channel is empty, blocks on a 10 ms `time.After` so a steady trickle (1 line/sec) still renders promptly. `logModel.Update` processes the whole batch with a single `SetContent` call:

```go
case logLinesMsg:
    for _, line := range msg.Lines {
        m.lines = append(m.lines, line)
    }
    if len(m.lines) > 500 {
        m.lines = m.lines[len(m.lines)-500:]
    }
    wasAtBottom := m.viewport.AtBottom()
    m.viewport.SetContent(strings.Join(m.lines, "\n"))
    if wasAtBottom {
        m.viewport.GotoBottom()
    }
    return m, nil
```

A 10 k-line burst that fires within 100 ms becomes ~10 batches through the Update loop, each triggering one `SetContent`, dropping total scanning cost by ~100x. The single-line `logLineMsg` type is removed — the `LogLineMsg` export from `main.go`'s adapter becomes `ui.LogLinesMsg` so the string → slice bridge can happen once at the drain step, not per line.

The message type is `type logLinesMsg struct{ Lines []string }` — single-line variant is not needed.

**Consequences for file logging:** The file logger write must happen on the subprocess-reader goroutine (as today), *not* via the drain goroutine. If the drain drops, the file log still has the line — this preserves debugging state even when the TUI panel falls behind.

**Consequences for `TestWriteToLog_AfterCloseNoPanic`:** The test at `workflow_test.go:835-845` remains meaningful — it documents that `WriteToLog` is safe to call after `Runner.Close()`. After migration, `Runner.Close()` is a no-op and `sendLine` is a direct closure in tests, so the test passes without modification. Testing the drain-goroutine's post-close behavior is a *separate* concern that belongs in `main.go`-level integration testing, not at the Runner layer — and section 8 describes the same helper with a synchronous closure that makes Runner-level "late write" inherently safe.

### 2. `logModel` owns a ring buffer + `viewport.Model`

**Today:** `glyph.Log` owns the 500-line cap and vim-nav (`main.go:124`).

**Change:** New `internal/ui/log_panel.go` with the code below. **Filename note:** `internal/ui/log.go` already exists and holds log-body banner helpers (`StepSeparator`, `StepStartBanner`, `PhaseBanner`, `CaptureLog`, `RetryStepSeparator`, `CompletionSummary`). The new `logModel` must go in a different file to avoid a name clash; `log_panel.go` is the chosen name.

```go
type logModel struct {
    viewport viewport.Model
    lines    []string // ring buffer, cap 500
}

func (m logModel) Update(msg tea.Msg) (logModel, tea.Cmd) {
    switch msg := msg.(type) {
    case logLinesMsg:
        for _, line := range msg.Lines {
            m.lines = append(m.lines, line)
        }
        if len(m.lines) > 500 {
            m.lines = m.lines[len(m.lines)-500:]
        }
        wasAtBottom := m.viewport.AtBottom()
        m.viewport.SetContent(strings.Join(m.lines, "\n"))
        if wasAtBottom {
            m.viewport.GotoBottom()
        }
        return m, nil
    }
    var cmd tea.Cmd
    m.viewport, cmd = m.viewport.Update(msg)
    return m, cmd
}
```

A batched `logLinesMsg` incurs exactly one `SetContent` per batch regardless of how many lines it carries, which is the whole point of the coalescing drain described in section 1.

The viewport handles scroll keys and wheel events through its built-in `KeyMap` plus the mouse handling that `tea.WithMouseCellMotion()` enables. The delegation `m.viewport, cmd = m.viewport.Update(msg)` plumbs them through.

**CAUTION — `bubbles/viewport` default `KeyMap` has only a subset of the bindings we advertise:** per `bubbles@v1.0.0/viewport/keymap.go`, `DefaultKeyMap()` binds `pgdown/space/f`, `pgup/b`, `u/ctrl+u` (HalfPageUp), `d/ctrl+d` (HalfPageDown), `up/k`, `down/j`, `left/h`, `right/l`. **`Home` and `End` are NOT bound by default.** Also `d`, `u`, `f`, `b`, `h`, `l`, `space` ARE bound — some of which could collide with future keysModel shortcuts.

Two fixes, both required:

1. **Custom KeyMap:** construct the viewport with an explicit KeyMap that (a) adds `home`/`end` (using `tea.KeyHome` / `tea.KeyEnd`), and (b) removes bindings that conflict with `keysModel` shortcuts — today that's not a problem, but strip `f`, `b`, `u`, `d` as a forward-compatibility guard so future error-mode / confirm-mode shortcuts can land without a silent collision.
2. **Shortcut line truth:** either keep `NormalShortcuts = "↑/k up  ↓/j down  n next step  q quit"` *unchanged* and accept that `PgUp/PgDn/Home/End` aren't advertised (they still work via the KeyMap), or expand it to `"↑/k/PgUp up  ↓/j/PgDn down  Home/End jump  n next step  q quit"`. The verification step item "`↑`, `↓`, `k`, `j`, `PgUp`, `PgDn`, `Home`, `End` all scroll the log panel" must exercise the custom KeyMap, not the default.

**Routing clarification (resolves internal inconsistency between sections 1, 2, and 11):** The root `Model.Update` must route `tea.KeyMsg` to BOTH `keysModel` (for mode dispatch: `n`, `q`, `y`, `c`, `r`, `Escape`) AND `logModel.viewport` (for scroll). Neither consumes the event in a way that would prevent the other from seeing it — both receive the same `tea.KeyMsg` by value. Pseudocode:

```go
func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
    var cmds []tea.Cmd
    switch msg := msg.(type) {
    case tea.KeyMsg:
        var kcmd tea.Cmd
        m.keys, kcmd = m.keys.Update(msg)
        cmds = append(cmds, kcmd)
        var lcmd tea.Cmd
        m.log, lcmd = m.log.Update(msg) // viewport scroll
        cmds = append(cmds, lcmd)
    case logLinesMsg:
        var lcmd tea.Cmd
        m.log, lcmd = m.log.Update(msg)
        cmds = append(cmds, lcmd)
    case headerMsg:
        m.header = m.header.apply(msg)
        // ... also emit SetWindowTitle if iteration line changed
    case tea.WindowSizeMsg:
        m.width = msg.Width
        m.height = msg.Height
        m.log, _ = m.log.Update(msg)
        m.header = m.header.resize(msg.Width, msg.Height)
    }
    return m, tea.Batch(cmds...)
}
```

`m.log, cmd = m.log.Update(msg)` is **load-bearing** — `logModel.Update` has a value receiver, so without reassignment the ring-buffer append is lost.

**Why:** The ring buffer is small (500 × ~80B = ~40KB) and `strings.Join` is sub-millisecond at that size — measured in Lip Gloss viewport benchmarks. `AtBottom` + `GotoBottom` is the exact pattern the `bubbles/viewport` README recommends for streaming logs.

### 3. `headerModel` keeps the pointer-mutable shape internally, but fences access through messages

**Today:** `internal/ui/header.go:44-74` is a pointer-mutable struct that Glyph reads via pointer. Mutation sites are scattered across `workflow/run.go:130, 154, 159, 184, 208, 221` and `ui/orchestrate.go:44, 58, 62`. The orchestration goroutine mutates the header directly; Glyph reads it at render time.

**Change:** Keep `StatusHeader` as a struct with the same fields and methods, but convert `glyph.Color` → `lipgloss.Color`:

```go
var (
    LightGray      = lipgloss.Color("245")
    ActiveStepFG   = lipgloss.Color("15") // ANSI bright white
    ActiveMarkerFG = lipgloss.Color("10") // ANSI bright green
)

type StatusHeader struct {
    IterationLine string
    Rows          [][HeaderCols]string
    Prefixes      [][HeaderCols]string
    Markers       [][HeaderCols]string
    Suffixes      [][HeaderCols]string
    MarkerColors  [][HeaderCols]lipgloss.Color
    NameColors    [][HeaderCols]lipgloss.Color
    stepNames     []string
}
```

All `RenderInitializeLine` / `RenderIterationLine` / `RenderFinalizeLine` / `SetPhaseSteps` / `SetStepState` method signatures are unchanged. `internal/ui/header_test.go` passes with **zero changes** — verified by reading the file: it only asserts on `h.Rows[*][*]` string content and `h.IterationLine`, never on color values. The `glyph.Color` → `lipgloss.Color` swap affects `NameColors` and `MarkerColors` fields, which `header_test.go` does not touch.

**Fencing:** The orchestration goroutine must not mutate the `StatusHeader` that the `headerModel` owns — that would be a data race with the UI goroutine. Introduce a thin `headerProxy` adapter in `internal/ui/header_proxy.go`:

```go
type headerProxy struct {
    send func(tea.Msg)
}

func (p *headerProxy) SetStepState(idx int, state StepState) {
    p.send(headerStepStateMsg{idx: idx, state: state})
}
func (p *headerProxy) RenderIterationLine(iter, max int, issue string) {
    p.send(headerIterationLineMsg{iter: iter, max: max, issue: issue})
}
// ...and RenderInitializeLine, RenderFinalizeLine, SetPhaseSteps
```

`headerProxy` satisfies the existing `workflow.RunHeader` interface (`internal/workflow/run.go:23-29`) and `ui.StepHeader` interface (`internal/ui/orchestrate.go:11-13`). `main.go` constructs a proxy once and passes it to `workflow.Run`. The orchestration goroutine never touches the real `StatusHeader` — it calls proxy methods which `program.Send` into the Update loop, where `headerModel.Update` applies the mutation to its own instance.

**Why:** This preserves the entire orchestration code shape unchanged. `workflow/run.go` keeps its ~160 lines of imperative subprocess logic. The only concession is the indirection layer, which costs zero runtime performance (one channel send per mutation, amortized across multi-millisecond subprocess outputs).

### 4. `KeyHandler` loses its dispatch, keeps its state

**Today:** `internal/ui/ui.go` has both the Mode state machine data (`Mode` enum, `shortcutLine`, `Actions` channel, `SetMode`, `ForceQuit`) and the dispatch logic (`Handle`, `handleNormal`, `handleError`, `handleQuitConfirm`, `ShortcutLinePtr`).

**Change:** Delete dispatch, keep state.

Delete:
- `Handle` (`ui.go:88-97`)
- `handleNormal` (`ui.go:99-110`)
- `handleError` (`ui.go:112-123`)
- `handleQuitConfirm` (`ui.go:125-139`)
- `ShortcutLinePtr` (`ui.go:74-76`) and its Option-Q race-workaround rationale comment
- The entire `ui.go:68-73` block explaining the ShortcutLinePtr pattern (no longer relevant)

Keep:
- `Mode` enum and constants (`ui.go:14-22`)
- `NormalShortcuts`, `ErrorShortcuts`, `QuitConfirmPrompt`, `QuittingLine` constants (`ui.go:24-29`)
- `KeyHandler` struct, `NewKeyHandler`, `SetMode`, `ShortcutLine` (`ui.go:34-85`)
- `ForceQuit` (`ui.go:141-153`)
- `updateShortcutLine` (`ui.go:155-168`)

**Extend `KeyHandler` mutex coverage to `mode` (fixes a latent race unmasked by the migration).** Today, `SetMode` at `ui.go:81-84` writes `h.mode` without any lock; `updateShortcutLine` only locks the `shortcutLine` field. In the current Glyph build, the only reader of `h.mode` is the Glyph keyboard callback goroutine calling `Handle`, and the only writer is the workflow goroutine via `orchestrate.go:63, 66` (`h.SetMode(ModeError)` / `h.SetMode(ModeNormal)`). Those two goroutines already race on `h.mode`, but the race detector never fires because no test exercises the cross-goroutine pattern and `TestShortcutLine_ConcurrentRead_NoRace` only concurrently reads `shortcutLine`.

After the migration, the Update goroutine reads `h.Mode()` for every `tea.KeyMsg` (section 4 `keysModel.Update` pseudocode), while `orchestrate.go` continues to call `h.SetMode` from the workflow goroutine — `orchestrate.go` is explicitly listed as **Unchanged** in the files-changed table. That makes the pre-existing race a live race under the new architecture, and deleting `TestShortcutLine_ConcurrentRead_NoRace` (which at least held a mutex-protected path) without replacement makes the regression silent.

**Resolution:** The same mutex that guards `shortcutLine` now also guards `mode` and `prevMode`. Concretely:

```go
// ui.go:
func (h *KeyHandler) SetMode(mode Mode) {
    h.mu.Lock()
    h.mode = mode
    h.updateShortcutLineLocked() // renamed; caller already holds h.mu
    h.mu.Unlock()
}

func (h *KeyHandler) Mode() Mode {
    h.mu.Lock()
    defer h.mu.Unlock()
    return h.mode
}
```

The internal handlers (`handleNormal` / `handleError` / `handleQuitConfirm`) are being moved to `keysModel` in this section, so they become the only other mutators and all call `SetMode` — they do not need to touch `h.mode` directly. `prevMode` is only read/written inside the `q → ... → Escape` round trip that stays entirely on the Update goroutine under Bubble Tea, so it does not strictly need the lock, but including it under `h.mu` keeps all KeyHandler state coherent under a single lock and avoids a future lock-ordering pitfall. `updateShortcutLine` is renamed to `updateShortcutLineLocked` to reflect the precondition.

**Consequence for tests:** `TestShortcutLine_ConcurrentRead_NoRace` is **retained** but broadened — it now concurrently reads both `ShortcutLine` and `Mode()` while a second goroutine cycles `SetMode` through all four modes. `go test -race` must pass. This replaces the Option-Q-specific test purpose with a coverage guarantee for the new shared mutex.

**Why not route `SetMode` through a `setModeMsg`?** Considered and rejected: it would require either making `orchestrate.go` Bubble-Tea-aware (violates the Narrow Reading ADR) or introducing a second proxy (`modeProxy`) alongside `headerProxy`. The mutex is one lock on a tiny critical section and adds zero cross-goroutine hops. The plan originally listed `setModeMsg` in `messages.go` (section files-changed table); that type is now removed from the message list — it was a vestige of an earlier design iteration.

**Fix the signal-path shutdown-symmetry bug (goal-line 26).** Today, `handleQuitConfirm("y")` at `ui.go:125-133` flips the footer to `QuittingLine` via `h.mode = ModeQuitting; h.updateShortcutLine()` before calling `ForceQuit`, but the signal goroutine at `main.go:160-169` calls `keyHandler.ForceQuit()` directly and **never sets `ModeQuitting`** — `ForceQuit` at `ui.go:145-153` does not touch `h.mode`. So on SIGINT/SIGTERM the footer stays on whatever mode was active (Normal/Error) until the 2-second grace window elapses; the user sees no "Quitting..." feedback. This contradicts the Goals section's "Must preserve: Identical shutdown semantics between the signal path and the `q → y` confirm path, both routed through `KeyHandler.ForceQuit`" — a *pre-existing* asymmetry that the plan's goal statement (line 26) misrepresents as already correct. Fix it in the same PR by moving the mode flip into `ForceQuit` itself:

```go
// ui.go — new ForceQuit:
func (h *KeyHandler) ForceQuit() {
    h.mu.Lock()
    h.mode = ModeQuitting
    h.updateShortcutLineLocked()
    h.mu.Unlock()

    if h.cancel != nil {
        h.cancel()
    }
    select {
    case h.Actions <- ActionQuit:
    default:
    }
}
```

With this change, both the `y`-confirm path AND the signal path produce identical footer state and identical subprocess-kill behavior via one primitive. `handleQuitConfirm("y")` no longer needs to set the mode itself — delete those two lines:

```go
// ui.go — new handleQuitConfirm case:
case "y":
    h.ForceQuit()  // sets ModeQuitting internally
```

Note that `ForceQuit` is safe to call when `cancel` is nil (guard is retained), and the non-blocking send on `Actions` is unchanged. The broadened `TestShortcutLine_ConcurrentRead_NoRace` (above) now exercises `ForceQuit` through the same mutex-protected path. Add a new test `TestForceQuit_SetsModeQuittingFromAnyMode` that asserts `ForceQuit` flips the footer to `QuittingLine` regardless of starting mode — this locks in the fix against future regressions.

Move to `internal/ui/model.go`:
- The switch-on-mode dispatch, rewritten as a `tea.KeyMsg` handler in `keysModel.Update`:

```go
func (m keysModel) Update(msg tea.Msg) (keysModel, tea.Cmd) {
    key, ok := msg.(tea.KeyMsg)
    if !ok {
        return m, nil
    }
    switch m.handler.Mode() {
    case ModeNormal:
        return m.handleNormal(key)
    case ModeError:
        return m.handleError(key)
    case ModeQuitConfirm:
        return m.handleQuitConfirm(key)
    case ModeQuitting:
        // No dispatch — all keys silently ignored so a user mashing keys during
        // shutdown can't inject a second ActionQuit or retrigger the cancel hook.
        return m, nil
    }
    return m, nil
}
```

Note that `ModeQuitting` also continues to deliver `tea.KeyMsg` events to the viewport via the dual-routing in the root `Model.Update` (scroll-while-quitting is still useful so the user can see the final banner). `keysModel` just stops dispatching mode actions.

The per-mode handlers are the same logic that used to live in `ui.go`, now accepting `tea.KeyMsg` values instead of `string` keys. String comparisons become `key.String() == "n"`, `key.Type == tea.KeyEscape`, etc.

**CRITICAL — `cancel()` (= `runner.Terminate`) blocks for up to 3 seconds.** At `workflow.go:70-88`, `Runner.Terminate` sends SIGTERM then `select`s on `<-done` with a 3-second `time.After` fallback before escalating to SIGKILL. Under Glyph today, this call runs on either the keyboard-callback goroutine or the OS-signal goroutine — never the render goroutine. In the Bubble Tea architecture, `keysModel.handleNormal` receives the `n` key on the **Update goroutine** and would call `h.cancel()` synchronously, freezing Update (and therefore `View` rendering and every other message) for up to three seconds.

**Fix — dispatch cancel asynchronously via `tea.Cmd`:**

```go
// keysModel — handleNormal:
func (m keysModel) handleNormal(key tea.KeyMsg) (keysModel, tea.Cmd) {
    switch key.String() {
    case "n":
        cancel := m.handler.Cancel() // exported accessor for h.cancel
        if cancel == nil {
            return m, nil
        }
        // Offload the 3-second blocking call to a goroutine. We don't need
        // a result message — the workflow goroutine's next RunStep will
        // observe WasTerminated and unwind on its own.
        return m, func() tea.Msg {
            cancel()
            return nil // Bubble Tea ignores nil messages
        }
    case "q":
        m.handler.SetMode(ModeQuitConfirm)
        return m, nil
    }
    return m, nil
}
```

The same asynchronous pattern applies anywhere else `h.cancel()` might be called from Update. `ForceQuit` does NOT need this wrapping: it is called from the signal goroutine (not Update) and from `handleQuitConfirm("y")`, where we also wrap the `ForceQuit` call in a `tea.Cmd` for the same reason:

```go
// keysModel — handleQuitConfirm:
case "y":
    // Flip the footer on the Update goroutine (cheap, mutex-only) so the
    // user sees "Quitting..." immediately, then offload the subprocess kill.
    // ForceQuit is idempotent, so a second call (e.g., signal path racing
    // with y-confirm) is harmless.
    return m, func() tea.Msg {
        m.handler.ForceQuit()
        return nil
    }
```

Actually — because `ForceQuit` now sets `ModeQuitting` itself under `h.mu` (see the signal-path symmetry fix above), the Update goroutine does NOT need to pre-flip the mode; the single async call to `ForceQuit` covers both the footer update and the subprocess kill. The footer still appears "immediately" because the mode flip is a mutex-only operation that completes in microseconds — even when chained with the 3-second cancel that follows it in the same goroutine. *(Rendering still happens on the Update goroutine, which is unblocked; the next tick picks up the new `ShortcutLine()`.)*

**New method on `KeyHandler`:** `func (h *KeyHandler) Cancel() func()` returns `h.cancel` under `h.mu`. Used by `keysModel.handleNormal` to read the function pointer safely. Not listed in the "Delete" block at the top of this section because it is new, not pre-existing.

**Why:** The state machine *data* (Mode, Actions channel, ForceQuit) is TUI-library-independent and already well-tested in `ui_test.go`. The *dispatch* is what binds to the TUI library, and that's the only thing that needs to change. Deleting `ShortcutLinePtr` also removes the race-detector workaround documented at `ui.go:68-73` — Bubble Tea doesn't need pointer binding, so there's no concurrent reader of a `*string`.

The test file `internal/ui/ui_test.go` loses its `Handle`-based cases and grows new cases that drive the `keysModel` directly via `tea.KeyMsg`. The `SetMode`, `ShortcutLine`, and `ForceQuit` tests stay exactly as they are.

**Specific test deletions in `ui_test.go`:**

- All `Handle`-based dispatch tests: `TestNormalMode_N_SendsCancelSignal`, `TestNormalMode_Q_ShowsQuitConfirmation`, `TestNormalMode_OtherKeys_Ignored`, `TestQuitConfirm_Y_*`, `TestQuitConfirm_N_*`, `TestQuitConfirm_Escape_*`, `TestQuitConfirm_OtherKey_*`, `TestErrorMode_C_*`, `TestErrorMode_R_*`, `TestErrorMode_Q_*`, `TestErrorMode_OtherKeys_*`, `TestKeyboardDispatch_NormalVsError`, `TestNewKeyHandler_NilCancel_NKey_*` — all migrate into `internal/ui/keys_test.go` (or the new `model_test.go`) driving `keysModel.Update(tea.KeyMsg{...})`.
- All `ShortcutLinePtr` tests (~4): `TestShortcutLinePtr_ReturnsNonNilPointer`, `TestShortcutLinePtr_DereferencesToCurrentValue`, `TestShortcutLinePtr_StableAddress`, `TestShortcutLinePtr_AgreesWithShortcutLine` — **deleted**, not migrated. No pointer-binding surface exists in Bubble Tea.
- `TestShortcutLine_ConcurrentRead_NoRace` (`ui_test.go:428-451`) — **retained and broadened** (see the mutex-extension subsection earlier in this section). Under the new architecture `SetMode` still fires from the workflow goroutine (`orchestrate.go:63, 66`) while `keysModel.Update` reads `Mode()` from the Update goroutine, so the concurrent-access scenario is real — not gone. The broadened test concurrently reads both `ShortcutLine` and `Mode()` while a second goroutine cycles modes, and `go test -race` must pass. *(An earlier draft of this plan listed this test for deletion; that directive is withdrawn.)*

**Tests that stay unchanged:** `TestSetMode_*`, `TestNewKeyHandler_InitialState`. These exercise the library-independent data shape.

**`TestForceQuit_*` changes driven by the signal-path symmetry fix above:**

- `TestForceQuit_CallsCancelAndInjectsActionQuit` — stays, still validates the cancel + send. Add an assertion that `h.mode == ModeQuitting` after the call.
- `TestForceQuit_NilCancel_NoPanic` — stays unchanged.
- `TestForceQuit_FullChannel_NoPanic` — stays unchanged.
- `TestForceQuit_DoesNotAlterMode_WhenNormal` (`ui_test.go:368-380`) — **flip assertion**: after `ForceQuit`, `h.mode` must be `ModeQuitting` and `h.ShortcutLine()` must be `QuittingLine`. Rename to `TestForceQuit_SetsModeQuitting_FromNormal`.
- `TestForceQuit_DoesNotAlterMode_WhenError` (`ui_test.go:382-396`) — same flip. Rename to `TestForceQuit_SetsModeQuitting_FromError`.
- `TestForceQuit_Idempotent_CalledTwice` — stays. The mode is already `ModeQuitting` after the first call; a second call re-writes the same value, which is a no-op in behavior. Cancel fires twice as documented.
- **New:** `TestForceQuit_FromSignalPath_FootersShowQuitting` — a goroutine-level test that simulates the signal path: start the handler in Normal mode, call `ForceQuit` from a background goroutine, assert from the main goroutine that both `Mode()` and `ShortcutLine()` observed under `h.mu` return `ModeQuitting`/`QuittingLine`. Race-detector-safe.

### 5. Dynamic title: both OS window title and in-TUI border title, same source

**OS title:** `tea.SetWindowTitle(title)` is a command emitted from `Update` whenever a `headerMsg` changes the `IterationLine`. `Update` returns `tea.Batch(existingCmd, tea.SetWindowTitle(newTitle))`.

**In-TUI border title:** `lipgloss.Border` has no title slot, so we hand-build the top border row. In `Model.View()`:

```go
func (m Model) renderTopBorder(title string) string {
    // Target: "╭── ralph-tui — Iteration 2/5 — Issue #42 ─ … ─╮"
    const tl, tr, h = "╭", "╮", "─"
    innerWidth := m.width - 2 // subtract corner glyphs
    leadDashes := 2
    // Budget for the title: innerWidth minus the lead dashes, with at least
    // one trailing dash so the title never abuts the top-right corner.
    titleBudget := innerWidth - leadDashes - 1
    if titleBudget < 0 {
        // Terminal is so narrow we can't even fit "╭──╮". Emit a plain rule.
        return lipgloss.NewStyle().Foreground(LightGray).Render(
            tl + strings.Repeat(h, max(innerWidth, 0)) + tr,
        )
    }
    titleSegment := " " + title + " "
    // lipgloss.Width handles multi-byte runes and any embedded ANSI; use it
    // (not len or RuneCountInString) because the title may contain em-dashes.
    titleWidth := lipgloss.Width(titleSegment)
    if titleWidth > titleBudget {
        // Title overflows. Truncate using Lip Gloss's MaxWidth, which is
        // rune-and-ANSI-aware, then re-wrap in the spacer pair so the
        // leading/trailing spaces remain visible even after truncation.
        inner := lipgloss.NewStyle().MaxWidth(titleBudget - 2).Render(title)
        titleSegment = " " + inner + " "
        titleWidth = lipgloss.Width(titleSegment)
    }
    fillCount := innerWidth - leadDashes - titleWidth
    if fillCount < 0 {
        // Defensive: should not be reachable after the truncation above.
        fillCount = 0
    }
    return lipgloss.NewStyle().Foreground(LightGray).Render(
        tl + strings.Repeat(h, leadDashes) + titleSegment + strings.Repeat(h, fillCount) + tr,
    )
}
```

**Truncation evidence:** Lip Gloss's `Style.MaxWidth` (verified against `lipgloss@v1.1.0`) uses ANSI-aware width accounting identical to `lipgloss.Width`, so the truncation respects both multi-byte runes and any embedded color codes. Tests for `renderTopBorder` must cover (a) normal title that fits, (b) title exactly at budget, (c) title one rune wider than budget → truncated with two-space spacer preserved, (d) `m.width == 4` (edge case where `titleBudget < 0` → plain rule), and (e) `m.width == 0` (terminal not yet sized → plain rule without crashing).

The sides (`│ ... │`) and bottom (`╰──...──╯`) are rendered via a standard `lipgloss.NewStyle().Border(lipgloss.RoundedBorder()).BorderTop(false).BorderForeground(LightGray).Width(m.width - 2)` applied to the inner content. The **explicit `.Width(m.width - 2)`** is load-bearing: without it, Lip Gloss pads the border to the widest line of the inner content, which will not match the `m.width - 2` used for the hand-built top row and will produce misaligned corners.

With `.Width(m.width - 2)`, Lip Gloss pads every inner line to exactly `m.width - 2` cells wide. Our top row uses the same width:

- `innerWidth = m.width - 2` (matches the inner content)
- top row total width = `innerWidth + 2` (top-left + inner + top-right corners) = `m.width`
- bottom row total width = same via Lip Gloss

so the four corners align and the side bars sit flush against the terminal edges. Confirmed against `lipgloss@v1.1.0/borders.go:281-380` by the adversarial validator.

We then prepend our hand-built top row to the Lip Gloss output.

Both title surfaces derive from the same function:

```go
func (m Model) titleString() string {
    if m.header.iterationLine == "" {
        return "ralph-tui"
    }
    return "ralph-tui — " + m.header.iterationLine
}
```

When `workflow.Run` returns and the workflow goroutine calls `program.Quit()`, the title in both surfaces freezes on its last value — which is desirable (the final line is typically "Finalizing 3/3: Git push" or similar).

**Why:** Lip Gloss lacks a first-class titled-border primitive; hand-rolling the top row is the standard workaround used in gh-dash, lazygit alternatives, and other Charm-ecosystem apps. `lipgloss.Width` (not `len` or `utf8.RuneCountInString`) is the correct measurement because it handles both multi-byte runes (the em-dash in "Iteration 2/5 — Issue #42") and any ANSI color codes we might add later.

### 6. Exit path: delete the `os.Exit` hack

**Today:** `main.go:177-188` exits the process directly from the workflow goroutine via `Screen().ExitRawMode() + os.Exit(0)`. The comment at `main.go:171-176` explains this is necessary because `app.Stop()` doesn't reliably interrupt Glyph's blocking `ReadKey` on a macOS raw-mode tty. The signal handler at `main.go:157-169` has the same shape: `ForceQuit` + 2-second grace + `Screen().ExitRawMode() + os.Exit(1)`.

**Change:** Replace both paths with `program.Quit()` / `program.Kill()`. See key design decision **10** below for the complete shutdown snippet. In summary: the workflow goroutine defers `close(workflowDone)` and then calls `program.Quit()`; the signal goroutine selects on both `sigChan` and `workflowDone` so it exits cleanly on either trigger; if the signal path fires and the workflow stays stuck past 2 seconds, it calls `program.Kill()` to force-abort.

The main goroutine:

```go
err := program.Run()
// program.Kill() (signal-path forced shutdown) makes Run return tea.ErrProgramKilled.
// That's a normal forced-exit path, not a crash — don't print a scary error for it.
if err != nil && !errors.Is(err, tea.ErrProgramKilled) {
    fmt.Fprintln(os.Stderr, "bubbletea:", err)
    os.Exit(1)
}
// Determine exit code from final state.
select {
case <-signaled:
    os.Exit(1)
default:
    os.Exit(0)
}
```

**Why the error branch matters:** `program.Kill()` sets an internal `killed=true` flag and causes `Run` to return `tea.ErrProgramKilled`. Without the `errors.Is` branch, every signal path that times out past the 2-second grace window would print `bubbletea: program was killed` to stderr on the way out, surfacing internal library detail as a user-visible error. Verified by the adversarial validator reading `bubbletea@v1.3.10/tea.go:578-746`.

A new `workflowDone chan struct{}` is closed at the end of the workflow goroutine (replacing the old `done` channel that `main.go` had removed in commit `4b36e78`).

**Why:** Bubble Tea manages the tty via its own program event loop. `program.Quit()` injects a `tea.quitMsg` that the Update loop drains cleanly, including raw-mode restoration, before `program.Run()` returns. This is the exact scenario the Glyph comment describes as broken — and it is not broken in Bubble Tea. We pair `tea.WithoutSignalHandler()` with our explicit signal goroutine so that Bubble Tea's built-in SIGINT handling doesn't race our subprocess-kill path.

### 7. Terminal width: keep `ui.TerminalWidth()` at startup, update viewport from `tea.WindowSizeMsg`

**Today:** `main.go:142-145` calls `ui.TerminalWidth()` once at startup and passes the result as `RunConfig.LogWidth` (`main.go:153`), which `workflow/run.go:98-107` uses to size the `═` phase banner underline. `ui.TerminalWidth()` is an ioctl on `os.Stdout.Fd()` (`internal/ui/terminal.go:16-22`) and is independent of the TUI library.

**Change:** Keep the initial-width resolution in `main()` exactly as today, because the ioctl still works once Bubble Tea enters the alt screen. `workflow.Run` is started from `main()` (not from inside `Update`) using the ioctl-derived width. The root `Model` additionally tracks the current window size via `tea.WindowSizeMsg` so that the viewport, header, and footer re-lay out on terminal resize:

```go
// In main():
logWidth := ui.TerminalWidth() - 2
if logWidth < 1 {
    logWidth = ui.DefaultTerminalWidth
}
runCfg := workflow.RunConfig{ /* ... */ LogWidth: logWidth }

model := ui.NewModel(/* ... */)
program := tea.NewProgram(model,
    tea.WithMouseCellMotion(),
    tea.WithAltScreen(),
    tea.WithoutSignalHandler())
proxy := ui.NewHeaderProxy(program.Send)
// Line forwarding goes through the buffered drain + coalescing batcher
// described in section 1. See section 8 for the complete wiring.
lineCh := make(chan string, 4096)
go runLineDrain(lineCh, program.Send) // batches into ui.LogLinesMsg
runner.SetSender(func(line string) {
    select {
    case lineCh <- line:
    default:
        // buffer full — drop; file log still has it
    }
})

workflowDone := make(chan struct{})
go func() {
    defer close(workflowDone)
    _ = workflow.Run(runner, proxy, keyHandler, runCfg)
    signal.Stop(sigChan)
    _ = log.Close()
    program.Quit()
}()

// In Model.Update:
case tea.WindowSizeMsg:
    m.width = msg.Width
    m.height = msg.Height
    // re-layout viewport, header, footer to new dimensions
```

Resize events update the viewport and re-render the border, but do not retroactively reflow historical log lines (phase banners written earlier at the startup width will look slightly off until the next phase writes a new banner). Fixing that properly would require re-rendering historical log content, which is out of scope.

**Why:** Two paths for the initial width were considered:

- **Option A (chosen):** Keep `ui.TerminalWidth()` in `main()`. `ui.TerminalWidth()` works before and after `tea.NewProgram` — it's a direct ioctl that doesn't depend on Bubble Tea's program state. `tea.WithAltScreen()` changes the display buffer but not the reported window size.
- **Option B (rejected):** Defer `workflow.Run` until the first `tea.WindowSizeMsg` and start the goroutine from inside `Model.Update`. This is the canonical Elm pattern but requires either (a) plumbing `runner`, `proxy`, `keyHandler`, and `runCfg` into the `Model` (couples UI to workflow internals), or (b) injecting a `startWorkflow func(width int)` closure into the `Model`.

Option A preserves the current main-goroutine shape, keeps `Model` narrowly scoped to rendering and dispatch, and costs nothing at runtime — the ioctl-derived width matches the `tea.WindowSizeMsg` width that Bubble Tea computes the same way internally. The only difference is timing (Option A sets the width one `tea.Init` cycle earlier), and that difference is invisible to the user.

**Evidence:**
- `internal/ui/terminal.go:16-22` — `TerminalWidth` uses `unix.IoctlGetWinsize(os.Stdout.Fd(), unix.TIOCGWINSZ)`, which is unaffected by alt-screen.
- `cmd/ralph-tui/main.go:142-145` — the existing main() already handles the non-TTY fallback cleanly, so no regression.

### 8. Workflow tests: replace `collectLines` with a recording sender

**Today:** `workflow_test.go:29-46` defines a `collectLines` helper that starts a goroutine reading from `r.LogReader()` and returns a drain function. This helper is used by ~25 tests.

**Change:** Replace the helper with a capture-mode sender that collects every forwarded line into a slice (mutex-guarded):

```go
// newCapturingRunner creates a Runner whose sendLine appends into a shared slice.
// Returns the runner, logger, and a drain function that returns the captured lines.
func newCapturingRunner(t *testing.T) (*Runner, *logger.Logger, func() []string) {
    t.Helper()
    dir := t.TempDir()
    log, err := logger.NewLogger(dir)
    if err != nil {
        t.Fatalf("NewLogger: %v", err)
    }
    r := NewRunner(log, dir)
    var mu sync.Mutex
    var lines []string
    r.SetSender(func(line string) {
        mu.Lock()
        defer mu.Unlock()
        lines = append(lines, line)
    })
    drain := func() []string {
        mu.Lock()
        defer mu.Unlock()
        out := make([]string, len(lines))
        copy(out, lines)
        return out
    }
    return r, log, drain
}
```

`SetSender` accepts a `func(string)` here so the tests don't need to import Bubble Tea. In `main.go`, a thin adapter wraps `program.Send` via a buffered-channel drain goroutine that also coalesces into batched `LogLinesMsg` messages (per section 1's back-pressure + `SetContent` amplification mitigation):

```go
const senderBuffer = 4096
lineCh := make(chan string, senderBuffer)

// Drain goroutine: coalesces consecutive lines within a 10 ms window into
// a single LogLinesMsg, reducing SetContent calls by ~100x under burst.
go func() {
    const coalesceWindow = 10 * time.Millisecond
    for {
        first, ok := <-lineCh
        if !ok {
            return
        }
        batch := []string{first}
        // Drain everything currently queued without blocking.
    drain:
        for {
            select {
            case l, ok := <-lineCh:
                if !ok {
                    program.Send(ui.LogLinesMsg{Lines: batch})
                    return
                }
                batch = append(batch, l)
            default:
                break drain
            }
        }
        program.Send(ui.LogLinesMsg{Lines: batch})
        // Optional: sleep a fixed 10 ms before reading the next burst so a
        // trickle of 1 line/sec still lands within one coalesce window.
        // Skipped here because the blocking <-lineCh read at loop top gives
        // the same effect for free.
        _ = coalesceWindow
    }
}()

runner.SetSender(func(line string) {
    select {
    case lineCh <- line:
    default:
        // buffer full — drop the line from the TUI panel; file logger still has it
        // (a follow-up improvement could emit a rate-limited "dropped N lines" marker)
    }
})
```

The drain goroutine exits when `lineCh` is closed, which happens after `workflow.Run` returns and `Runner` is closed. Because `workflow.Run` is guaranteed to have returned before `close(workflowDone)` fires, we can close `lineCh` in the workflow goroutine's defer chain alongside `log.Close()`:

```go
go func() {
    defer close(workflowDone)
    _ = workflow.Run(runner, proxy, keyHandler, runCfg)
    signal.Stop(sigChan)
    _ = log.Close()
    close(lineCh)       // stop drain goroutine
    program.Quit()
}()
```

For tests, the drain indirection is not needed — tests call a synchronous closure because the test doesn't have a Bubble Tea program running:

```go
r.SetSender(func(line string) {
    mu.Lock(); defer mu.Unlock()
    lines = append(lines, line)
})
```

Every existing test that calls `collect := collectLines(t, r)` / `collect()` switches to the new helper. There is no concurrent-reader goroutine any more, so the tests no longer need to close the pipe before draining — `drain()` is called after `RunStep` returns and `wg.Wait()` has synchronized the scanner goroutines.

`TestWriteToLog_AfterCloseNoPanic` (`workflow_test.go:835-845`) continues to work because `sendLine` just calls a closure; after the migration there's nothing to close first. The test can stay as a documentation of the no-panic guarantee.

**Estimated scope:** `collect := collectLines(t, runner)` appears at 26 sites in `workflow_test.go` and 5 sites in `run_test.go` (31 total; verified by `rg -c 'collect := collectLines'`). Every site gets the same mechanical substitution. `run_test.go`'s `LastCapture` tests (lines 1482, 1506, 1528, 1554, 1591) are the only ones in that file that hit a real `Runner` — the rest drive `fakeExecutor` and are unaffected.

### 9. Pre-populated header and headerProxy bootstrap

**Today:** `main.go:72-84` pre-populates the `StatusHeader` with the first phase's step set and active state *before* `app.Run()` starts the Glyph render loop, so the first painted frame shows real content instead of blanks.

**Change:** The real `StatusHeader` lives inside `headerModel`. Pre-population moves into the `headerModel` constructor, called from `main.go` with the same data as today:

```go
// In main.go, after stepFile is loaded:
initialHeader := ui.NewStatusHeader(maxSteps)
if len(stepFile.Initialize) > 0 {
    initialHeader.SetPhaseSteps(stepNames(stepFile.Initialize))
    initialHeader.SetStepState(0, ui.StepActive)
    initialHeader.RenderInitializeLine(1, len(stepFile.Initialize), stepFile.Initialize[0].Name)
} else {
    // (same else branch as today)
}
model := ui.NewModel(initialHeader, keyHandler, versionLabel)
```

`ui.NewModel` stores the pre-populated `*StatusHeader` inside its `headerModel`. The first `View()` call renders the populated state; no "blank initial frame" gap.

**`headerProxy` construction order:** There is no chicken-and-egg problem, because `Model` does **not** need to hold a reference to `headerProxy` — the proxy is used by `workflow.Run` to emit header messages *into* the program, and the `Model` receives them as plain `tea.Msg` values in `Update`. So:

```go
initialHeader := ui.NewStatusHeader(maxSteps)
// ... pre-populate initialHeader ...
model := ui.NewModel(initialHeader, keyHandler, versionLabel)
program := tea.NewProgram(model, /* ... */)
proxy := ui.NewHeaderProxy(program.Send) // built with program.Send directly
// lineCh + batching drain goroutine are started here — see section 8 for body.
runner.SetSender(func(line string) {
    select {
    case lineCh <- line:
    default:
    }
})
go workflow.Run(runner, proxy, keyHandler, runCfg)
```

All wiring is complete before the workflow goroutine starts, so no pre-setter race is possible and no deferred-setter indirection is needed.

### 10. Shutdown goroutine ordering

Both the workflow-completion and signal-handling paths need to close `workflowDone` so the signal goroutine's `select` can observe it. The complete snippet:

```go
workflowDone := make(chan struct{})

// Workflow goroutine
go func() {
    defer close(workflowDone)
    _ = workflow.Run(runner, proxy, keyHandler, runCfg)
    signal.Stop(sigChan)
    _ = log.Close()
    close(lineCh)      // stop the send-drain goroutine (see section 8)
    program.Quit()
}()

// Signal goroutine
go func() {
    select {
    case <-sigChan:
    case <-workflowDone:
        return // workflow finished first; nothing to do
    }
    close(signaled)
    keyHandler.ForceQuit() // kill subprocess, inject ActionQuit
    select {
    case <-workflowDone:
        program.Quit()
    case <-time.After(2 * time.Second):
        // Workflow stuck — likely backpressured on program.Send (section 1).
        // Force-kill the program; Run() will return tea.ErrProgramKilled which
        // the main goroutine treats as a normal forced-exit path (section 6).
        program.Kill()
    }
}()
```

The signal goroutine also watches `workflowDone` in its first `select` so that a normal completion lets it exit without blocking on `sigChan` forever. The 2-second grace-window is a deliberate pairing with section 1's back-pressure mitigation: if the buffered channel is draining slowly, the grace lets the last few messages land; if it's wedged entirely, Kill is the escape hatch.

### 11. Mouse wheel: `tea.WithMouseCellMotion()` + `bubbles/viewport`

`tea.NewProgram(model, tea.WithMouseCellMotion(), tea.WithAltScreen(), tea.WithoutSignalHandler())` enables mouse wheel events. `bubbles/viewport` consumes `tea.MouseMsg` events with `Button == tea.MouseButtonWheelUp/WheelDown` natively — no custom code required.

The existing keyboard scrollback (`↑/k`, `↓/j`, `PgUp/PgDn`, `Home/End`) is provided by `viewport.KeyMap` with the default bindings. The `NormalShortcuts` line (`"↑/k up  ↓/j down  n next step  q quit"`) stays accurate.

## Migration order (single PR, ordered commits)

The PR ships as one unit because the build is broken between commits 2 and 5. Commits within the PR:

1. **Add Charm deps.** `go get github.com/charmbracelet/bubbletea github.com/charmbracelet/lipgloss github.com/charmbracelet/bubbles` + `go mod tidy`. No code changes. Tests still pass against Glyph.
2. **Convert `internal/ui/header.go` colors.** Swap `glyph.Color` → `lipgloss.Color` in the struct and package vars. Nothing renders yet; `header_test.go` passes if it only checks field values.
3. **Strip dispatch out of `internal/ui/ui.go`.** Delete `Handle`, `handleNormal`, `handleError`, `handleQuitConfirm`, `ShortcutLinePtr`. Update `ui_test.go` to test `SetMode` / `ShortcutLine` / `ForceQuit` directly. Build is broken here because `main.go` still calls the deleted methods.
4. **Add `internal/ui/model.go`, `internal/ui/log.go`, `internal/ui/header_proxy.go`, `internal/ui/keys.go`.** The new `Model`, sub-models, message types, and proxy. Add `model_test.go` exercising key dispatch via synthetic `tea.KeyMsg` values.
5. **Rewrite `cmd/ralph-tui/main.go`.** Construct `tea.Program`, wire up `Runner.SetSender`, `headerProxy`, signal goroutine with `program.Quit/Kill`, and start `workflow.Run` from inside the first `tea.WindowSizeMsg`. Delete Glyph imports. Build is fixed.
6. **Rewrite `internal/workflow/workflow.go`.** Remove `io.Pipe`, `LogReader`; add `sendLine` and `SetSender`. Drop the mutex that guarded `PipeWriter`. `workflow_test.go` updated to pass a fake sender.
7. **`go mod tidy`.** Drop `github.com/kungfusheep/glyph` from go.mod and go.sum.
8. **Update docs.** Every doc that references Glyph, `io.Pipe`/`LogReader`/`PipeWriter`, `app.Stop()`, `app.Screen().ExitRawMode()`, or `ShortcutLinePtr` gets updated. Concretely:
   - `docs/features/tui-display.md` — drop pointer-binding language; describe the Bubble Tea ring-buffer + `bubbles/viewport` path; document the dynamic OS/border title source.
   - `docs/features/subprocess-execution.md` — heavy rewrite of the "streaming" sections: replace the `io.Pipe` box/struct/mutex language (lines ~12, 49-62, 77-81, 145, 240-242) with the `sendLine`-into-`program.Send` path.
   - `docs/features/signal-handling.md` — replace `app.Stop()` / `Screen().ExitRawMode()` references (lines ~100, 140) with `program.Quit()` / `program.Kill()` and the updated signal/workflow select.
   - `docs/features/keyboard-input.md` — remove any `ShortcutLinePtr` / Glyph callback references; document the `keysModel.Update(tea.KeyMsg)` dispatch. The four-mode state machine text is already correct.
   - `docs/features/workflow-orchestration.md` — verify and update any references to `io.Pipe`, `LogReader`, or Glyph wiring; the core Run loop text stays.
   - `docs/coding-standards/concurrency.md` — update the `io.PipeWriter` mutex example (this was a concrete example of "mutex-protected writes"). Replace with a still-valid example from the codebase (e.g., `processMu` around `currentProc`) or document the removal.
   - `docs/architecture.md` — update the TUI rendering path block diagram, the package dependency graph, and any text referencing Glyph or the pipe-based streaming path.
   - `docs/how-to/reading-the-tui.md` — mention mouse wheel scrolling in the region tour, and add a "Selecting log text to copy" subsection documenting the Option (macOS) / Shift (Linux/Windows) modifier-key override for drag-select, per the accepted regression in Open Question 7a.
   - `docs/how-to/getting-started.md` — add a one-line note near the "reading the log panel" content pointing at the drag-select override so first-run users see it.
   - `docs/how-to/quitting-gracefully.md` — update any `ShortcutLinePtr` references; verify the exit-code text still matches the new signal path.
   - `docs/project-discovery.md` — update framework/dependency listings if it names Glyph explicitly.
   - `docs/coding-standards/testing.md` (line ~236-240) — the "Glyph layout assembly" example must be replaced with a Bubble Tea equivalent (or removed and replaced with a non-Glyph test-seam example). Per the [Lint and Tooling standard](../coding-standards/lint-and-tooling.md), this is not a suppression, but a live code example that is about to become fiction.
   - `docs/coding-standards/go-patterns.md` (lines ~136, 177, 184) — the `glyph.NewApp()` example and the two `TUI Display & Glyph Wiring` links reference Glyph-specific mechanics; update the example to Bubble Tea's `tea.NewProgram` and re-title the links.
   - `docs/features/file-logging.md` (line ~168) — drop the "alongside `io.Pipe`" phrase in the Related Documents bullet; replace with "alongside the `sendLine` streaming path."

   Before landing the PR, grep the entire `docs/` tree for `glyph`, `io.Pipe`, `PipeWriter`, `PipeReader`, `LogReader`, `ShortcutLinePtr`, `app.Stop`, and `ExitRawMode` to catch anything this list missed. Historical text in ADRs (`docs/adr/`) and plans (`docs/plans/`) stays as-is — those are frozen decision records, not living docs.

Between commits 3 and 5, `make build` fails. That is acceptable within a PR but must not be split across PRs.

## Files changed

| File | Change |
|------|--------|
| `ralph-tui/go.mod`, `go.sum` | Add bubbletea, lipgloss, bubbles; drop glyph |
| `ralph-tui/internal/version/version.go` | MINOR bump (`0.1.0` → `0.2.0`) — see Versioning section |
| `ralph-tui/cmd/ralph-tui/main.go` | Full rewrite: `tea.NewProgram`, signal handler with `program.Quit`, `Runner.SetSender`, `headerProxy`, start `workflow.Run` on first `WindowSizeMsg` |
| `ralph-tui/internal/ui/header.go` | Color type swap (`glyph.Color` → `lipgloss.Color`); same struct, methods, and semantics |
| `ralph-tui/internal/ui/header_test.go` | **Unchanged** — asserts only string content, never color values |
| `ralph-tui/internal/ui/ui.go` | Delete `Handle`, `handleNormal`, `handleError`, `handleQuitConfirm`, `ShortcutLinePtr`, and the Option-Q comment block |
| `ralph-tui/internal/ui/ui_test.go` | Delete `Handle`-based tests; keep `SetMode`, `ShortcutLine`, `ForceQuit` tests |
| `ralph-tui/internal/ui/model.go` | **new** — root `Model`, sub-models, `Init`, `Update`, `View`, title assembly, hand-built rounded border with dynamic title |
| `ralph-tui/internal/ui/log_panel.go` | **new** — `logModel` with 500-line ring buffer + `viewport.Model`, `AtBottom`/`GotoBottom` auto-scroll. Filename is `log_panel.go` because `log.go` already holds log-body banner helpers |
| `ralph-tui/internal/ui/keys.go` | **new** — `keysModel` dispatching `tea.KeyMsg` by mode (the logic that used to live in `Handle`) |
| `ralph-tui/internal/ui/header_proxy.go` | **new** — proxy satisfying `workflow.RunHeader` and `ui.StepHeader` via `tea.Program.Send` |
| `ralph-tui/internal/ui/messages.go` | **new** — `LogLinesMsg{Lines []string}`, `headerStepStateMsg`, `headerIterationLineMsg`, `headerPhaseStepsMsg`, `headerInitializeLineMsg`, `headerFinalizeLineMsg` types. (No single-line `logLineMsg` — the coalescing drain in §1 always delivers a batch. No `setModeMsg` — mode transitions stay on `KeyHandler.SetMode`; see section 4 for the mutex-based race fix.) |
| `ralph-tui/internal/ui/model_test.go` | **new** — tests for root `Update` routing, key dispatch per mode, log append + auto-scroll, viewport mouse events, title assembly |
| `ralph-tui/internal/workflow/workflow.go` | Remove `io.Pipe`, `LogReader`, `PipeWriter` mutex; add `sendLine`, `SetSender`; `forwardPipe` and `WriteToLog` call `sendLine` |
| `ralph-tui/internal/workflow/workflow_test.go` | Update `Runner` construction to pass a fake sender closure (~31 call sites — see section 8) |
| `ralph-tui/internal/workflow/run.go` | Unchanged — the orchestration loop is TUI-library-independent |
| `ralph-tui/internal/workflow/run_test.go` | Update 5 `LastCapture` tests at `run_test.go:1482, 1506, 1528, 1554, 1591` — they call `NewRunner` + `collectLines` directly and must switch to `newCapturingRunner` alongside `workflow_test.go`. The `fakeExecutor`-driven tests that make up the bulk of the file are unchanged |
| `docs/features/tui-display.md` | Drop pointer-binding language; describe ring buffer + viewport; note dynamic title |
| `docs/architecture.md` | Update TUI rendering path block diagram |
| `docs/how-to/reading-the-tui.md` | Mention mouse wheel scrolling |
| `ralph-tui/internal/ui/orchestrate.go` | Unchanged — still talks to `ui.StepHeader` interface, which `headerProxy` now satisfies |
| `ralph-tui/internal/ui/orchestrate_test.go` | Unchanged |
| `ralph-tui/internal/ui/terminal.go` | Unchanged — `ui.TerminalWidth()` is still the initial-width source (see key design decision 7) |

Files that do **not** change: `internal/workflow/run.go`, `internal/ui/orchestrate.go`, `internal/vars`, `internal/steps`, `internal/cli`, `internal/validator`, `internal/version`, `internal/logger`, everything in `scripts/` and `prompts/`. The [Narrow Reading ADR](../adr/20260410170952-narrow-reading-principle.md) is respected: no Ralph-specific knowledge moves into the Bubble Tea Model.

## Verification

1. **`make ci` passes.** Invoked from the repository root (`/Users/mxriverlynn/dev/mxriverlynn/pr9k/Makefile`, not `ralph-tui/Makefile`). The full pipeline: `test, lint, format, vet, vulncheck, mod-tidy, build`. Race detector stays on (`go test -race ./...`).
2. **`make build && ./bin/ralph-tui -n 1 -p <test-project>` renders correctly.**
   - Terminal tab title shows `ralph-tui — Initializing 1/<N>: <Name>` within 500ms of startup, updates on each phase transition, and ends on the final finalize step's line.
   - In-TUI rounded border top row shows the same string, rendered in light-gray.
   - Checkbox grid shows the active step with a green `▸` marker and bright-white brackets/name.
   - Log panel streams claude CLI output in real time.
   - Scroll wheel (iTerm2 or Ghostty) scrolls the log panel up; new lines append but the viewport stays put until the user returns to bottom.
   - `↑`, `↓`, `k`, `j`, `PgUp`, `PgDn`, `Home`, `End` all scroll the log panel.
   - `n` in Normal mode terminates the active subprocess (skip step).
   - Forcing a step failure (e.g., broken prompt) enters Error mode; `c` continues, `r` retries, `q` enters QuitConfirm.
   - `q` → `y` shuts down cleanly with `Quitting...` visible in the footer.
   - `q` → `n` or `q` → `<Escape>` returns to the previous mode.
   - `Ctrl+C` (SIGINT) shuts down within ~3 seconds, killing the active subprocess, restoring the terminal cleanly.
   - On normal completion, the process exits with code 0 without waiting for a keypress.
3. **`go test -race ./internal/ui/...` passes without the `ShortcutLinePtr` Option-Q workaround.** The migration removes that code path; the race detector should be quiet because the `Model.header` is only read/written from the single `Update` goroutine.
4. **Resize test.** Open ralph-tui in a resizable terminal, resize to ~60 cols and back to 120. The viewport and border re-lay out; historical phase banners written at the old width remain at the old width (acceptable known limitation).
5. **Non-tty stdout.** `./bin/ralph-tui ... 2>&1 | cat` should still exit cleanly. Bubble Tea falls back to a non-interactive mode when stdout is not a tty — we should verify this does not hang, and document the fallback in `docs/how-to/getting-started.md` if the behavior differs from Glyph's today.

## Versioning

Per [`docs/coding-standards/versioning.md`](../coding-standards/versioning.md), ralph-tui's "public API" under semver is the CLI flags, `config.json` schema, `{{VAR}}` language, and `--version` output. **The TUI rendering path is explicitly NOT part of the public API.** So this migration does not strictly require a version bump under the letter of the standard.

However, the change is large enough in spirit — it swaps a whole TUI library and changes keyboard/mouse input behavior — that a **MINOR bump** (currently `0.1.0` → `0.2.0` under the `0.y.z` rules) is appropriate for release-note visibility. Bump `internal/version.Version` as part of the same PR.

## Open questions & risks

1. **Bubble Tea non-tty fallback behavior.** Glyph today exits cleanly when stdout is not a tty (because the raw-mode call fails gracefully). Bubble Tea's behavior with `tea.WithAltScreen()` and a non-tty stdout is a known edge case — we may need to detect and skip alt-screen when `!term.IsTerminal(int(os.Stdout.Fd()))`. Verify during implementation; not a blocker. (Adversarial validator noted Bubble Tea opens `/dev/tty` via `openInputTTY` when stdin isn't a terminal — behavior with `2>&1 | cat` was not validated.)
2. **Version pinning.** The plan doesn't pin specific bubbletea/lipgloss/bubbles versions, but it does rely on specific behaviors confirmed against `bubbletea@v1.3.10`, `lipgloss@v1.1.0`, and `bubbles@v1.0.0`. Commit 1 pins these exact versions and the PR description records them. If the implementation needs to move to a newer version, re-verify the `program.Send` blocking semantics (section 1), `viewport.DefaultKeyMap` bindings (section 2), border rendering (section 5), and `ErrProgramKilled` behavior (section 6).
3. **Phase banner width on resize.** Historical phase banners written into the log at width W remain at width W after a resize. Accepted limitation. If users complain, the fix is to tag phase banners in the ring buffer and re-render them on `tea.WindowSizeMsg`.
4. **`teatest` adoption.** Bubble Tea has a `teatest` package for higher-level TUI integration tests. This plan does not use it — model-level unit tests driving `Update` with synthetic messages are sufficient for the four-mode state machine and log append logic. Revisit if the `View()` output becomes complex enough to justify snapshot tests.
5. **`headerProxy` coupling risk.** The proxy is a thin forwarder, but it does introduce a layer of indirection between `workflow.Run` and the real header state. If a test wants to assert on header state after `workflow.Run` returns, it now needs to drive the `headerModel.Update` directly or hold the `tea.Program` instance. The existing `run_test.go` uses a fake header already, so this is not a regression — it's a continuation of the same testing pattern.
6. **Back-pressure drop rate.** Section 1 drops lines under backpressure when the buffered send-drain channel is full (4096 entries). The 10 ms coalescing drain combined with the 4096-deep buffer gives roughly 40 k lines of headroom per 100 ms window; under the worst observed burst (claude CLI output during a code review step) the drop rate should be zero in practice. If it's not, a rate-limited `[log panel] N lines dropped` synthetic line would be easy to add as a follow-up. Not a blocker.
7. **Mouse events over non-mouse terminals.** `tea.WithMouseCellMotion()` enables mouse byte emission. Terminals without mouse support (or `TERM=dumb`) may render the raw escape sequences as text. If this shows up in the wild, gate the option on a TTY-capability check.

7a. **`tea.WithMouseCellMotion()` breaks terminal drag-select-to-copy — accepted regression, documented.** Verified by reading `bubbletea@v1.3.10/standard_renderer.go:415-420` + `x/ansi@v0.10.1/mode.go:512`: the option emits `\x1b[?1002h` (xterm button-event mouse mode), which tells every mainstream terminal (iTerm2, Ghostty, Kitty, xterm) to forward mouse drags to the application instead of performing local text selection. Today's Glyph build does **not** enable any mouse mode — verified via grep for `?100[0-9]` in the Glyph source. **Impact:** users who today drag-select log-panel text to copy will, after the migration, need to hold Option (macOS) or Shift (Linux/Windows) to override the application's mouse capture. **Decision:** accept the regression (user approval 2026-04-11). Mouse-wheel scrolling is a high-value quality-of-life feature and the override gesture is one modifier key on every mainstream terminal. Document the Option/Shift workaround in `docs/how-to/reading-the-tui.md` as part of commit 8's doc updates — specifically, add a "Selecting log text to copy" section that explains the modifier-key override per platform. Also mention in `docs/how-to/getting-started.md` near the "reading the log panel" note so new users see it on first run. No code change is required beyond enabling `tea.WithMouseCellMotion()`.

8. **Narrow-terminal rendering policy.** The plan specifies no minimum width. Consequences of rendering at < ~40 columns:
   - **Checkbox grid** (`header.go:41-88`, 4 columns × `[ ▸name ]` cells): each cell is 5–15 chars wide; 4 columns require ~40 cols minimum; narrower terminals will force Lip Gloss to wrap or overflow.
   - **Hand-built top border** (section 5): handled by the truncation fallback added in this iteration; degrades to plain-rule at very small widths.
   - **Shortcut footer**: `"↑/k up  ↓/j down  n next step  q quit"` is 37 chars plus the version label pinned right; at 40 cols the version label overflows or clips.
   - Today's Glyph build has the same undocumented narrow-width behavior, so the regression is "no worse than today." Not a blocker, but worth documenting with a floor: if `m.width < 40`, render a single-line placeholder ("ralph-tui: terminal too narrow (width N, minimum 40)") instead of the full layout. Decide during implementation.

9. **`docs/notes/glyph-api-findings.md`** is named after the Glyph library. The plan's frozen-documents policy carves out `docs/adr/` and `docs/plans/` but not `docs/notes/`. The file is a historical research artifact (verified: opens with "Glyph API Findings — Issue #46" and contains only module-verification findings from the adoption of Glyph). Treat as frozen alongside ADRs and plans; add a one-line superseded-by note at the top pointing to this plan and the Bubble Tea ADR, but do not delete. Document this intent explicitly in commit 8.
8. **Viewport custom KeyMap carries forward.** Section 2 strips `f`/`b`/`u`/`d`/`space` from the viewport KeyMap as a forward-compatibility guard. This is a deliberate choice — it prevents future silent key collisions — but it also means users who type-muscle-memory `space` for page down won't get it. Acceptable because `PgDn` still works and the shortcut line never advertised `space`.

## Iteration Summary

This plan was refined through three iterations plus adversarial validation on 2026-04-11, followed by a second pass on 2026-04-11 that combined fresh iteration and full agent validation (evidence-based-investigator + adversarial-validator). All decisions below are evidence-based; file references are provided for every claim.

### Iterations completed: 3 + adversarial validation (pass 1), then 3 + agent validation (pass 2)

### Pass 2 findings that changed the plan structurally

| # | Finding | Plan change |
|---|---------|-------------|
| P2-1 | Mode field race unmasked by migration (`SetMode` writes from workflow goroutine, `keysModel.Update` reads from Update goroutine) | Section 4: extended `KeyHandler.mu` to cover `mode` and `prevMode`; added `Mode()` accessor; renamed `updateShortcutLine` to `updateShortcutLineLocked`. `TestShortcutLine_ConcurrentRead_NoRace` retained and broadened. |
| P2-2 | `run_test.go` has 5 direct `NewRunner`+`collectLines` call sites; plan incorrectly marked it "Unchanged" | Files-changed table corrected; section 8 documents the 5 specific LastCapture tests. |
| P2-3 | Doc update list missing `docs/coding-standards/testing.md`, `docs/coding-standards/go-patterns.md`, `docs/features/file-logging.md` | Section 8 expanded to include all three with line references. |
| P2-4 | Section 4's `TestShortcutLine_ConcurrentRead_NoRace` "delete" directive contradicted the section's new "retain and broaden" directive (BLOCKER) | Deletion directive withdrawn in the `ui_test.go` deletions block. |
| P2-5 | Signal-path `ForceQuit` does not set `ModeQuitting`, contradicting the Goals "Must preserve: identical shutdown semantics" line | Section 4: moved the mode flip into `ForceQuit` itself so both the `y`-confirm and signal paths go through one primitive. `TestForceQuit_DoesNotAlterMode_*` tests flipped to assert the new symmetry. |
| P2-6 | Hand-built top border had a truncation promise in the comment that the sketch did not implement | Section 5: sketch replaced with a version that uses `lipgloss.Style.MaxWidth` for truncation, handles `titleBudget < 0` (plain-rule fallback), and enumerates test cases. |
| P2-7 | `bubbles/viewport.SetContent` is O(N) per call (verified against `bubbles@v1.0.0/viewport/viewport.go:125-133, 536-544`); single-line `logLineMsg` would produce O(N²) cost over a burst | Section 1: coalescing shape promoted from "optional optimization" to **required**; message type collapsed to `logLinesMsg{Lines []string}`; drain goroutine body specified; section 2's `logModel.Update` rewritten to batch. |
| P2-8 | `runner.Terminate` blocks up to 3 s; `keysModel.handleNormal` would call it synchronously from the Update goroutine and freeze rendering | Section 4: handlers wrap cancel/ForceQuit in `tea.Cmd` closures so the blocking call runs off-goroutine. New `KeyHandler.Cancel()` accessor. |
| P2-9 | `tea.WithMouseCellMotion()` (verified `?1002h` escape) breaks terminal drag-select-to-copy — a user-visible regression with no mention in the plan | Added as Open Question 7a with three mitigation options; flagged as "blocker for final sign-off". |
| P2-10 | Narrow-terminal rendering policy unspecified | Added as Open Question 8 with concrete floor recommendation (`m.width < 40` → placeholder line). |
| P2-11 | `docs/notes/glyph-api-findings.md` not covered by the frozen-documents carve-out | Added as Open Question 9; treat as frozen historical research, add superseded-by note in commit 8. |
| P2-12 | `Makefile` is at the repository root, not `ralph-tui/` | Verification step 1 path clarified. |

### Pass 2 false alarms (for traceability)

- `tea.WithAltScreen()` scrollback loss — Glyph already uses `\x1b[?1049h` at `screen.go:152` and ralph-tui already goes through `EnterRawMode`. No regression.
- `viewport.GotoBottom` cost — O(1) arithmetic; the O(N²) burst cost is entirely in `SetContent`.

### Pass 1 assumptions and findings (retained from first iteration pass)

### Assumptions challenged and resolved

1. **`tea.Program.Send` is non-blocking** → **REFUTED.** Read `bubbletea@v1.3.10/tea.go:244` and `tea.go:774-779`. The `msgs` channel is unbuffered; `Send` blocks until `eventLoop` drains. Plan section 1 now mandates a buffered-drain goroutine with drop-on-full semantics.
2. **`bubbles/viewport` default KeyMap binds `Home`/`End`** → **REFUTED.** Read `bubbles@v1.0.0/viewport/keymap.go`. Only binds `pgdown/space/f`, `pgup/b`, `u/ctrl+u`, `d/ctrl+d`, `up/k`, `down/j`, `left/h`, `right/l`. Plan section 2 now requires a custom KeyMap that adds `Home`/`End` and strips `f`/`b`/`u`/`d`/`space` as a forward-compat guard.
3. **Hand-built top border aligns with Lip Gloss sides automatically** → **REFUTED.** Read `lipgloss@v1.1.0/borders.go:281-380`. Lip Gloss borders wrap the measured width of inner content, not a caller-supplied width. Plan section 5 now mandates `.Width(m.width - 2)` on the inner-content style so both top and sides use the same width.
4. **`program.Kill()` causes `Run()` to return nil** → **REFUTED.** Read `bubbletea@v1.3.10/tea.go:722-735`. Kill returns `tea.ErrProgramKilled`. Plan section 6 now branches on `errors.Is(err, tea.ErrProgramKilled)` so forced-kill shutdown doesn't print a spurious "bubbletea:" error.
5. **`run_test.go` is unchanged** → **CONFIRMED.** Read `run_test.go:19-112`. Uses `fakeExecutor` and `fakeRunHeader` that satisfy the `StepExecutor` and `RunHeader` interfaces — no direct `Runner` dependency. Plan's claim holds.
6. **`header_test.go` needs color-value updates** → **REFUTED.** Read `header_test.go` in full. Tests only assert on `h.Rows[*][*]` string content and `h.IterationLine`, never on color field values. Plan section 3 now says "Unchanged."
7. **Start `workflow.Run` from inside `Model.Update` on first `WindowSizeMsg`** → **REJECTED for simpler alternative.** `ui.TerminalWidth()` (`internal/ui/terminal.go:16-22`) is an ioctl on `os.Stdout.Fd()` that works before and after `tea.NewProgram`. Plan section 7 now starts `workflow.Run` from `main()` with the ioctl-derived width and lets `tea.WindowSizeMsg` only drive runtime re-layout.
8. **`headerProxy` construction has a chicken-and-egg with `program`** → **REFUTED.** The `Model` does not need a reference to the proxy (workflow.Run uses the proxy to emit messages *into* the program; the Model only reads messages out). Plan section 9 now constructs the proxy after `tea.NewProgram` with `program.Send` directly.
9. **`Runner.Close()` is deleted when the pipe is removed** → **AMENDED.** `StepExecutor` interface (`run.go:19`) still requires `Close`, and `workflow.Run` calls it at every exit point. Plan section 1 now says `Close` becomes a no-op returning nil, rather than being deleted.
10. **Pre-populated header state in `main.go:72-84` is lost after migration** → **CONFIRMED but mitigated.** Plan section 9 moves the pre-population into the `headerModel` constructor, so the first `View()` paints populated state with no "blank first frame" gap.

### Consolidations made

- Sections 6 and 10 referenced duplicate signal-handling snippets with subtly different orderings. Consolidated into one authoritative snippet in section 10, with section 6 pointing to it.
- `runner.SetSender(program.Send)` in three different places were unified into a single shape: `runner.SetSender(func(line string){ lineCh <- line })` with the drain goroutine handling `program.Send`.

### Internal/external overlap findings

- **Internal clash (critical):** `internal/ui/log.go` already exists (holds banner helpers). Plan originally proposed a new `log.go` in the same package for `logModel`. Resolved by renaming the new file to `log_panel.go`.
- **No external overlap with Bubble Tea ecosystem.** Verified via `grep -r 'bubbletea|charmbracelet|lipgloss' ralph-tui/` → zero matches (consistent with `docs/plans/ux-corrections/design.md:61`).
- **No hidden utility for hand-built borders:** searched `internal/ui/` — no pre-existing titled-border helper to reuse.

### Ambiguities resolved

- **Workflow test rewrite scope:** Plan now explicitly says ~25 test functions in `workflow_test.go` substitute `collectLines` → `newCapturingRunner`. Helper skeleton provided in section 8.
- **Routing of `tea.KeyMsg`:** Plan originally said "→ keys" in section 1 and "→ viewport" in section 2 with no reconciliation. Section 2 now spells out dual-dispatch in the root `Model.Update`.
- **`Runner.SetSender` nil-handling:** Was originally `if sendLine != nil`. Changed to a panicking sentinel initializer so missing-wire bugs fail loudly.
- **Docs update list** was incomplete (3 files). Expanded to 10+ files with a grep checklist to catch anything missed.
- **Versioning:** Plan now explicitly says the migration does not require a semver bump under the standard (TUI is not public API) but recommends a MINOR bump (`0.1.0` → `0.2.0`) for release-note visibility.

### Validation results and plan adjustments

The adversarial validator found 12 issues. The 7 that changed the plan structurally:

| # | Finding | Plan change |
|---|---------|-------------|
| V1 | `program.Send` is blocking on unbuffered channel | Added buffered-drain goroutine mitigation to section 1, with drop-on-full semantics and a file-logger-preserves-lines guarantee |
| V2 | Viewport default KeyMap has no Home/End; has `f`/`b`/`u`/`d`/`space` | Added custom KeyMap to section 2, with both add-bindings and strip-bindings lists |
| V3 | Routing inconsistency between sections 1/2/11 | Added explicit dual-dispatch root Update pseudocode to section 2 |
| V4 | Hand-built top border won't align with Lip Gloss sides | Added required `.Width(m.width - 2)` on inner-content style in section 5 |
| V5 | `program.Kill` returns `ErrProgramKilled` | Added `errors.Is(err, tea.ErrProgramKilled)` branch in section 6's main goroutine |
| V6 | Nil sender silently drops lines in tests | Changed `Runner.sendLine` initializer to a panicking sentinel in section 1 |
| V8 | Root Update must assign `m.log, cmd = m.log.Update(...)` | Spelled out explicitly in section 2's routing pseudocode |

Findings V7, V9, V10, V11, V12 were either already addressed, confirmed as latent-but-acceptable, or documented in Open Questions without requiring a structural change.

### Final state

The plan is now load-bearing against all known failure modes of Bubble Tea's actual implementation at version 1.3.10 / bubbles 1.0.0 / lipgloss 1.1.0. Risks open for verification during implementation (enumerated in "Open questions & risks"):

1. Non-TTY fallback behavior (Open Q 1)
2. Back-pressure drop rate on real hardware under claude-review bursts (Open Q 6)
3. Mouse events on non-mouse terminals (Open Q 7)
4. **Mouse drag-select regression on mainstream terminals (Open Q 7a)** — resolved 2026-04-11: accept the regression and document the Option/Shift override in `reading-the-tui.md` and `getting-started.md`. No code mitigation; proceed with `tea.WithMouseCellMotion()`.
5. Narrow-terminal rendering policy (Open Q 8)
6. Viewport custom KeyMap muscle-memory (Open Q existing)

## Related docs

- [ADR: Use Bubble Tea for the TUI Framework](../adr/20260411070907-bubble-tea-tui-framework.md)
- [ADR: Cobra CLI Framework](../adr/20260409135303-cobra-cli-framework.md) — same ecosystem-stability reasoning
- [ADR: Narrow Reading Principle](../adr/20260410170952-narrow-reading-principle.md) — scope guard for the migration
- [TUI Display Feature](../features/tui-display.md)
- [Keyboard Input Feature](../features/keyboard-input.md)
- [Reading the TUI](../how-to/reading-the-tui.md)
- [Quitting Gracefully](../how-to/quitting-gracefully.md)
