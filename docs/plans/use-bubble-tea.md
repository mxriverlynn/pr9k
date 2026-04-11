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
│  runner.SetSender(func(line string) {         │
│      program.Send(ui.LogLineMsg{Line: line})  │
│  })                                           │
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
│    logLineMsg     → log (append + scroll) │
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

**Change:** Remove the pipe. Add `Runner.sendLine func(string)` and a setter `Runner.SetSender(send func(string))`. The setter takes a string-typed sender (not `func(tea.Msg)`) so the `workflow` package does **not** need to import `github.com/charmbracelet/bubbletea`. The Bubble Tea adapter lives in `main.go`:

```go
runner.SetSender(func(line string) {
    program.Send(ui.LogLineMsg{Line: line})
})
```

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

`Runner.Close()` (`workflow.go:207-209`) currently closes the pipe writer to send EOF to the UI-side reader. After the migration there is no pipe, so **`Runner.Close()` becomes a no-op returning `nil`**. It stays on the struct because `workflow.StepExecutor` (`internal/workflow/run.go:19`) requires it — `workflow.Run` calls `executor.Close()` at each exit point. The `TestClose_IsIdempotent` test continues to pass because `nil` is idempotent. Removing `Close` from the interface entirely is *out of scope* for this migration — that cleanup can happen in a follow-up PR.

**Sender must be non-nil at `RunStep`/`WriteToLog` call time.** To prevent a silent-drop failure mode where a test forgets to call `SetSender` and then asserts on an empty capture list, `NewRunner` initializes `sendLine` to a **sentinel that panics** with `"workflow.Runner: sendLine not set — call SetSender before running steps"`. Every production path (`main.go`) and every test helper (`newCapturingRunner` in `workflow_test.go`) must call `SetSender` before any `RunStep`. This makes the missing-wire bug loud. The nil check is not used.

**Why:** `program.Send` is the canonical Bubble Tea bridge from an external goroutine into the Update loop. It's non-blocking in current Bubble Tea versions and handles its own concurrency. No `io.Pipe` EOF dance, no separate reader goroutine on the UI side.

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

**Mitigation (required, not optional):** Insert a small forwarding goroutine between `forwardPipe` and `program.Send` that batches or drops under pressure. Two viable shapes:

1. **Unbounded queue goroutine (simpler):** `Runner` owns a buffered `chan string` of e.g. 4096 entries; `forwardPipe` writes to the channel non-blockingly (drop on full with a rate-limited `[log panel] N lines dropped due to render backpressure` synthetic line); a single dedicated goroutine drains the channel and calls `program.Send`. Back-pressure is isolated to that goroutine only.
2. **Coalesce-on-send goroutine:** Same channel-based drain, but the drain goroutine batches consecutive lines within a ~10ms window into a single `logLinesMsg` struct carrying `[]string`, reducing round-trip count by 10-100x during bursts.

Shape 1 is the minimum viable fix and what this plan commits to. Shape 2 is an optimization worth considering if the implementation finds the drop rate non-trivial on real hardware.

The `logLineMsg` type becomes `type logLineMsg struct{ Line string }` for shape 1, or `type logLinesMsg struct{ Lines []string }` for shape 2. `logModel.Update` handles both identically (append-and-scroll), differing only in the inner loop.

**Consequences for file logging:** The file logger write must happen on the subprocess-reader goroutine (as today), *not* via the drain goroutine. If the drain drops, the file log still has the line — this preserves debugging state even when the TUI panel falls behind.

**Consequences for `TestWriteToLog_AfterCloseNoPanic`:** The test must be updated to call `WriteToLog` AFTER the drain goroutine has been stopped, to actually exercise the "late write" path. A vacuous pass (no sender wired) is not acceptable.

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
    case logLineMsg:
        m.lines = append(m.lines, msg.line)
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
    case logLineMsg: /* or logLinesMsg */
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

**Why:** The state machine *data* (Mode, Actions channel, ForceQuit) is TUI-library-independent and already well-tested in `ui_test.go`. The *dispatch* is what binds to the TUI library, and that's the only thing that needs to change. Deleting `ShortcutLinePtr` also removes the race-detector workaround documented at `ui.go:68-73` — Bubble Tea doesn't need pointer binding, so there's no concurrent reader of a `*string`.

The test file `internal/ui/ui_test.go` loses its `Handle`-based cases and grows new cases that drive the `keysModel` directly via `tea.KeyMsg`. The `SetMode`, `ShortcutLine`, and `ForceQuit` tests stay exactly as they are.

**Specific test deletions in `ui_test.go`:**

- All `Handle`-based dispatch tests: `TestNormalMode_N_SendsCancelSignal`, `TestNormalMode_Q_ShowsQuitConfirmation`, `TestNormalMode_OtherKeys_Ignored`, `TestQuitConfirm_Y_*`, `TestQuitConfirm_N_*`, `TestQuitConfirm_Escape_*`, `TestQuitConfirm_OtherKey_*`, `TestErrorMode_C_*`, `TestErrorMode_R_*`, `TestErrorMode_Q_*`, `TestErrorMode_OtherKeys_*`, `TestKeyboardDispatch_NormalVsError`, `TestNewKeyHandler_NilCancel_NKey_*` — all migrate into `internal/ui/keys_test.go` (or the new `model_test.go`) driving `keysModel.Update(tea.KeyMsg{...})`.
- All `ShortcutLinePtr` tests (~4): `TestShortcutLinePtr_ReturnsNonNilPointer`, `TestShortcutLinePtr_DereferencesToCurrentValue`, `TestShortcutLinePtr_StableAddress`, `TestShortcutLinePtr_AgreesWithShortcutLine` — **deleted**, not migrated. No pointer-binding surface exists in Bubble Tea.
- `TestShortcutLine_ConcurrentRead_NoRace` (`ui_test.go:428-451`) — **deleted**. This test exists specifically to validate Option Q's mutex-protected read path under Glyph's render goroutine. Bubble Tea reads `ShortcutLine` only from the single Update goroutine, so the concurrency scenario no longer exists.

**Tests that stay unchanged:** `TestSetMode_*`, `TestForceQuit_*` (all six), `TestNewKeyHandler_InitialState`. These exercise the library-independent data shape.

### 5. Dynamic title: both OS window title and in-TUI border title, same source

**OS title:** `tea.SetWindowTitle(title)` is a command emitted from `Update` whenever a `headerMsg` changes the `IterationLine`. `Update` returns `tea.Batch(existingCmd, tea.SetWindowTitle(newTitle))`.

**In-TUI border title:** `lipgloss.Border` has no title slot, so we hand-build the top border row. In `Model.View()`:

```go
func (m Model) renderTopBorder(title string) string {
    // Target: "╭── ralph-tui — Iteration 2/5 — Issue #42 ─ … ─╮"
    const tl, tr, h = "╭", "╮", "─"
    innerWidth := m.width - 2 // subtract corner glyphs
    leadDashes := 2
    titleSegment := " " + title + " "
    // lipgloss.Width handles multi-byte runes and any embedded ANSI
    titleWidth := lipgloss.Width(titleSegment)
    fillCount := innerWidth - leadDashes - titleWidth
    if fillCount < 0 {
        fillCount = 0
        // truncate title if width too small (fallback — paint dashes only)
    }
    return lipgloss.NewStyle().Foreground(LightGray).Render(
        tl + strings.Repeat(h, leadDashes) + titleSegment + strings.Repeat(h, fillCount) + tr,
    )
}
```

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
runner.SetSender(func(line string) { program.Send(ui.LogLineMsg{Line: line}) })

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

`SetSender` accepts a `func(string)` here so the tests don't need to import Bubble Tea. In `main.go`, a thin adapter wraps `program.Send` via a buffered-channel drain goroutine (per section 1's back-pressure mitigation):

```go
const senderBuffer = 4096
lineCh := make(chan string, senderBuffer)
go func() {
    for line := range lineCh {
        program.Send(ui.LogLineMsg{Line: line})
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

**Estimated scope:** ~25 test functions in `workflow_test.go` touch `collectLines`/`LogReader`; all get the same mechanical substitution.

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
runner.SetSender(func(line string) { program.Send(ui.LogLineMsg{Line: line}) })
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
   - `docs/how-to/reading-the-tui.md` — mention mouse wheel scrolling in the region tour.
   - `docs/how-to/quitting-gracefully.md` — update any `ShortcutLinePtr` references; verify the exit-code text still matches the new signal path.
   - `docs/project-discovery.md` — update framework/dependency listings if it names Glyph explicitly.

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
| `ralph-tui/internal/ui/messages.go` | **new** — `logLineMsg`, `headerStepStateMsg`, `headerIterationLineMsg`, `headerPhaseStepsMsg`, `setModeMsg` types |
| `ralph-tui/internal/ui/model_test.go` | **new** — tests for root `Update` routing, key dispatch per mode, log append + auto-scroll, viewport mouse events, title assembly |
| `ralph-tui/internal/workflow/workflow.go` | Remove `io.Pipe`, `LogReader`, `PipeWriter` mutex; add `sendLine`, `SetSender`; `forwardPipe` and `WriteToLog` call `sendLine` |
| `ralph-tui/internal/workflow/workflow_test.go` | Update `Runner` construction to pass a fake sender closure |
| `ralph-tui/internal/workflow/run.go` | Unchanged — the orchestration loop is TUI-library-independent |
| `ralph-tui/internal/workflow/run_test.go` | Unchanged |
| `docs/features/tui-display.md` | Drop pointer-binding language; describe ring buffer + viewport; note dynamic title |
| `docs/architecture.md` | Update TUI rendering path block diagram |
| `docs/how-to/reading-the-tui.md` | Mention mouse wheel scrolling |
| `ralph-tui/internal/ui/orchestrate.go` | Unchanged — still talks to `ui.StepHeader` interface, which `headerProxy` now satisfies |
| `ralph-tui/internal/ui/orchestrate_test.go` | Unchanged |
| `ralph-tui/internal/ui/terminal.go` | Unchanged — `ui.TerminalWidth()` is still the initial-width source (see key design decision 7) |

Files that do **not** change: `internal/workflow/run.go`, `internal/ui/orchestrate.go`, `internal/vars`, `internal/steps`, `internal/cli`, `internal/validator`, `internal/version`, `internal/logger`, everything in `scripts/` and `prompts/`. The [Narrow Reading ADR](../adr/20260410170952-narrow-reading-principle.md) is respected: no Ralph-specific knowledge moves into the Bubble Tea Model.

## Verification

1. **`make ci` passes.** The full pipeline: `test, lint, format, vet, vulncheck, mod-tidy, build`. Race detector stays on (`go test -race ./...`).
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

Per [`docs/coding-standards/versioning.md`](../coding-standards/versioning.md), ralph-tui's "public API" under semver is the CLI flags, `ralph-steps.json` schema, `{{VAR}}` language, and `--version` output. **The TUI rendering path is explicitly NOT part of the public API.** So this migration does not strictly require a version bump under the letter of the standard.

However, the change is large enough in spirit — it swaps a whole TUI library and changes keyboard/mouse input behavior — that a **MINOR bump** (currently `0.1.0` → `0.2.0` under the `0.y.z` rules) is appropriate for release-note visibility. Bump `internal/version.Version` as part of the same PR.

## Open questions & risks

1. **Bubble Tea non-tty fallback behavior.** Glyph today exits cleanly when stdout is not a tty (because the raw-mode call fails gracefully). Bubble Tea's behavior with `tea.WithAltScreen()` and a non-tty stdout is a known edge case — we may need to detect and skip alt-screen when `!term.IsTerminal(int(os.Stdout.Fd()))`. Verify during implementation; not a blocker. (Adversarial validator noted Bubble Tea opens `/dev/tty` via `openInputTTY` when stdin isn't a terminal — behavior with `2>&1 | cat` was not validated.)
2. **Version pinning.** The plan doesn't pin specific bubbletea/lipgloss/bubbles versions, but it does rely on specific behaviors confirmed against `bubbletea@v1.3.10`, `lipgloss@v1.1.0`, and `bubbles@v1.0.0`. Commit 1 pins these exact versions and the PR description records them. If the implementation needs to move to a newer version, re-verify the `program.Send` blocking semantics (section 1), `viewport.DefaultKeyMap` bindings (section 2), border rendering (section 5), and `ErrProgramKilled` behavior (section 6).
3. **Phase banner width on resize.** Historical phase banners written into the log at width W remain at width W after a resize. Accepted limitation. If users complain, the fix is to store phase banner info as typed `logLineMsg` kinds and re-render them on `tea.WindowSizeMsg`.
4. **`teatest` adoption.** Bubble Tea has a `teatest` package for higher-level TUI integration tests. This plan does not use it — model-level unit tests driving `Update` with synthetic messages are sufficient for the four-mode state machine and log append logic. Revisit if the `View()` output becomes complex enough to justify snapshot tests.
5. **`headerProxy` coupling risk.** The proxy is a thin forwarder, but it does introduce a layer of indirection between `workflow.Run` and the real header state. If a test wants to assert on header state after `workflow.Run` returns, it now needs to drive the `headerModel.Update` directly or hold the `tea.Program` instance. The existing `run_test.go` uses a fake header already, so this is not a regression — it's a continuation of the same testing pattern.
6. **Back-pressure drop rate.** Section 1 drops `logLineMsg` under backpressure when the buffered send-drain channel is full (4096 entries). Under the worst observed burst (claude CLI output during a code review step) the drop rate should be zero in practice, but if it's not, a rate-limited `[log panel] N lines dropped` synthetic line would be easy to add as a follow-up. Not a blocker.
7. **Mouse events over non-mouse terminals.** `tea.WithMouseCellMotion()` enables mouse byte emission. Terminals without mouse support (or `TERM=dumb`) may render the raw escape sequences as text. If this shows up in the wild, gate the option on a TTY-capability check.
8. **Viewport custom KeyMap carries forward.** Section 2 strips `f`/`b`/`u`/`d`/`space` from the viewport KeyMap as a forward-compatibility guard. This is a deliberate choice — it prevents future silent key collisions — but it also means users who type-muscle-memory `space` for page down won't get it. Acceptable because `PgDn` still works and the shortcut line never advertised `space`.

## Iteration Summary

This plan was refined through three iterations plus adversarial validation on 2026-04-11. All decisions below are evidence-based; file references are provided for every claim.

### Iterations completed: 3 + adversarial validation

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

The plan is now load-bearing against all known failure modes of Bubble Tea's actual implementation at version 1.3.10 / bubbles 1.0.0 / lipgloss 1.1.0. Three risks remain open for verification during implementation (enumerated in "Open questions & risks"): non-TTY fallback behavior, back-pressure drop rate on real hardware, and mouse events on non-mouse terminals. None are blockers.

## Related docs

- [ADR: Use Bubble Tea for the TUI Framework](../adr/20260411070907-bubble-tea-tui-framework.md)
- [ADR: Cobra CLI Framework](../adr/20260409135303-cobra-cli-framework.md) — same ecosystem-stability reasoning
- [ADR: Narrow Reading Principle](../adr/20260410170952-narrow-reading-principle.md) — scope guard for the migration
- [TUI Display Feature](../features/tui-display.md)
- [Keyboard Input Feature](../features/keyboard-input.md)
- [Reading the TUI](../how-to/reading-the-tui.md)
- [Quitting Gracefully](../how-to/quitting-gracefully.md)
