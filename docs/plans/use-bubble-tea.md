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
┌─ cmd/ralph-tui/main.go ──────────────────┐
│  cfg, runner, keyHandler, header (as data)│
│  model := ui.NewModel(header, keyHandler) │
│  program := tea.NewProgram(model,         │
│      tea.WithMouseCellMotion(),           │
│      tea.WithAltScreen(),                 │
│      tea.WithoutSignalHandler())          │
│  runner.SetSender(program.Send)           │
│  go workflow.Run(runner, proxy, ...)      │
│  go signalHandler(program, keyHandler)    │
│  program.Run()                            │
└───────────────────────────────────────────┘
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

**Change:** Remove the pipe. Add `Runner.sendLine func(string)` and a setter `Runner.SetSender(send func(tea.Msg))` that wraps it. `forwardPipe` becomes:

```go
for scanner.Scan() {
    line := scanner.Text()
    // capture + file log unchanged
    if r.sendLine != nil {
        r.sendLine(line)
    }
}
```

The mutex in `workflow.go:154-156` goes away because `tea.Program.Send` is safe for concurrent use across goroutines. `Runner.LogReader` is deleted. `Runner.WriteToLog` (`workflow.go:199-202`) becomes a direct `r.sendLine(line)` call with the same mutex removal.

**Why:** `program.Send` is the canonical Bubble Tea bridge from an external goroutine into the Update loop. It's non-blocking in current Bubble Tea versions and handles its own concurrency. No `io.Pipe` EOF dance, no separate reader goroutine on the UI side.

### 2. `logModel` owns a ring buffer + `viewport.Model`

**Today:** `glyph.Log` owns the 500-line cap and vim-nav (`main.go:124`).

**Change:** New `internal/ui/log.go` with:

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

The viewport handles `tea.KeyMsg{↑, ↓, k, j, PgUp, PgDn, Home, End}` and `tea.MouseMsg{Wheel...}` via its built-in `KeyMap` and mouse handling. The delegation `m.viewport, cmd = m.viewport.Update(msg)` plumbs them through.

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

All `RenderInitializeLine` / `RenderIterationLine` / `RenderFinalizeLine` / `SetPhaseSteps` / `SetStepState` method signatures are unchanged. `internal/ui/header_test.go` (if it exists) should pass with zero changes except the color value equality checks.

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
    // ModeQuitting: no dispatch, keys ignored
    }
    return m, nil
}
```

The per-mode handlers are the same logic that used to live in `ui.go`, now accepting `tea.KeyMsg` values instead of `string` keys. String comparisons become `key.String() == "n"`, `key.Type == tea.KeyEscape`, etc.

**Why:** The state machine *data* (Mode, Actions channel, ForceQuit) is TUI-library-independent and already well-tested in `ui_test.go`. The *dispatch* is what binds to the TUI library, and that's the only thing that needs to change. Deleting `ShortcutLinePtr` also removes the race-detector workaround documented at `ui.go:68-73` — Bubble Tea doesn't need pointer binding, so there's no concurrent reader of a `*string`.

The test file `internal/ui/ui_test.go` loses its `Handle`-based cases and grows new cases that drive the `keysModel` directly via `tea.KeyMsg`. The `SetMode`, `ShortcutLine`, and `ForceQuit` tests stay exactly as they are.

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

The sides (`│ ... │`) and bottom (`╰──...──╯`) are rendered via a standard `lipgloss.NewStyle().Border(lipgloss.RoundedBorder()).BorderTop(false).BorderForeground(LightGray)` applied to the inner content, then we prepend our hand-built top row.

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

**Change:** Replace both paths with `program.Quit()`:

```go
// Workflow goroutine
go func() {
    _ = workflow.Run(runner, proxy, keyHandler, runCfg)
    signal.Stop(sigChan)
    _ = log.Close()
    program.Quit()
}()

// Signal goroutine
go func() {
    <-sigChan
    close(signaled)
    keyHandler.ForceQuit() // kill subprocess, inject ActionQuit
    // Give the workflow goroutine ~2s to unwind on its own via ActionQuit.
    // If it's stuck, Kill the program directly — Bubble Tea restores the tty.
    select {
    case <-workflowDone:
        program.Quit()
    case <-time.After(2 * time.Second):
        program.Kill()
    }
}()
```

The main goroutine:

```go
if err := program.Run(); err != nil {
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

A new `workflowDone chan struct{}` is closed at the end of the workflow goroutine (replacing the old `done` channel that `main.go` had removed in commit `4b36e78`).

**Why:** Bubble Tea manages the tty via its own program event loop. `program.Quit()` injects a `tea.quitMsg` that the Update loop drains cleanly, including raw-mode restoration, before `program.Run()` returns. This is the exact scenario the Glyph comment describes as broken — and it is not broken in Bubble Tea. We pair `tea.WithoutSignalHandler()` with our explicit signal goroutine so that Bubble Tea's built-in SIGINT handling doesn't race our subprocess-kill path.

### 7. Terminal width: `tea.WindowSizeMsg` replaces `ui.TerminalWidth()` at startup

**Today:** `main.go:142-145` calls `ui.TerminalWidth()` once at startup and passes the result as `RunConfig.LogWidth` (`main.go:153`), which `workflow/run.go:98-107` uses to size the `═` phase banner underline.

**Change:** The root `Model` starts with an unknown width; the first `tea.WindowSizeMsg` sets it. Because `workflow.Run` starts in a background goroutine and the phase banner needs the log width immediately, we seed `RunConfig.LogWidth` from the initial `WindowSizeMsg` by deferring the `go workflow.Run(...)` call until after the first `WindowSizeMsg` arrives:

```go
// In Model.Update:
case tea.WindowSizeMsg:
    m.width = msg.Width
    m.height = msg.Height
    if !m.workflowStarted {
        m.workflowStarted = true
        go workflow.Run(runner, proxy, keyHandler, RunConfig{
            // ...
            LogWidth: msg.Width - 2, // subtract border glyphs
        })
    }
    // re-layout viewport, header, footer to new dimensions
```

Resize events after the first one update the viewport and re-render the border, but do not retroactively reflow historical log lines (phase banners written earlier will look slightly off until the next phase rewrites them). Fixing that properly would require re-wrapping historical log content, which is out of scope.

**Why:** This is the canonical Bubble Tea idiom. Starting `workflow.Run` from `Update` instead of from `main()` is the only subtlety — it ensures the first phase banner has a real width.

### 8. Mouse wheel: `tea.WithMouseCellMotion()` + `bubbles/viewport`

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
8. **Update docs.** `docs/features/tui-display.md` (drop pointer-binding language, document the ring-buffer/viewport path); `docs/architecture.md` (update the TUI rendering path section); `docs/how-to/reading-the-tui.md` (mention mouse scroll in the region tour). No changes to `docs/features/keyboard-input.md` or `docs/how-to/quitting-gracefully.md` beyond removing any lingering `ShortcutLinePtr` references — the four-mode state machine text is already correct.

Between commits 3 and 5, `make build` fails. That is acceptable within a PR but must not be split across PRs.

## Files changed

| File | Change |
|------|--------|
| `ralph-tui/go.mod`, `go.sum` | Add bubbletea, lipgloss, bubbles; drop glyph |
| `ralph-tui/cmd/ralph-tui/main.go` | Full rewrite: `tea.NewProgram`, signal handler with `program.Quit`, `Runner.SetSender`, `headerProxy`, start `workflow.Run` on first `WindowSizeMsg` |
| `ralph-tui/internal/ui/header.go` | Color type swap (`glyph.Color` → `lipgloss.Color`); same struct, methods, and semantics |
| `ralph-tui/internal/ui/header_test.go` | Update color assertions if any compare values |
| `ralph-tui/internal/ui/ui.go` | Delete `Handle`, `handleNormal`, `handleError`, `handleQuitConfirm`, `ShortcutLinePtr`, and the Option-Q comment block |
| `ralph-tui/internal/ui/ui_test.go` | Delete `Handle`-based tests; keep `SetMode`, `ShortcutLine`, `ForceQuit` tests |
| `ralph-tui/internal/ui/model.go` | **new** — root `Model`, sub-models, `Init`, `Update`, `View`, title assembly, hand-built rounded border with dynamic title |
| `ralph-tui/internal/ui/log.go` | **new** — `logModel` with 500-line ring buffer + `viewport.Model`, `AtBottom`/`GotoBottom` auto-scroll |
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

## Open questions & risks

1. **Bubble Tea non-tty fallback behavior.** Glyph today exits cleanly when stdout is not a tty (because the raw-mode call fails gracefully). Bubble Tea's behavior with `tea.WithAltScreen()` and a non-tty stdout is a known edge case — we may need to detect and skip alt-screen when `!term.IsTerminal(int(os.Stdout.Fd()))`. Verify during implementation; not a blocker.
2. **Version pinning.** The plan doesn't pin specific bubbletea/lipgloss/bubbles versions because they'll drift before the PR lands. Resolve via `go get ...@latest` at commit time; record the exact versions in the PR description.
3. **Phase banner width on resize.** Historical phase banners written into the log at width W remain at width W after a resize. Accepted limitation. If users complain, the fix is to store phase banner info as typed `logLineMsg` kinds and re-render them on `tea.WindowSizeMsg`.
4. **`teatest` adoption.** Bubble Tea has a `teatest` package for higher-level TUI integration tests. This plan does not use it — model-level unit tests driving `Update` with synthetic messages are sufficient for the four-mode state machine and log append logic. Revisit if the `View()` output becomes complex enough to justify snapshot tests.
5. **`headerProxy` coupling risk.** The proxy is a thin forwarder, but it does introduce a layer of indirection between `workflow.Run` and the real header state. If a test wants to assert on header state after `workflow.Run` returns, it now needs to drive the `headerModel.Update` directly or hold the `tea.Program` instance. The existing `run_test.go` uses a fake header already, so this is not a regression — it's a continuation of the same testing pattern.

## Related docs

- [ADR: Use Bubble Tea for the TUI Framework](../adr/20260411070907-bubble-tea-tui-framework.md)
- [ADR: Cobra CLI Framework](../adr/20260409135303-cobra-cli-framework.md) — same ecosystem-stability reasoning
- [ADR: Narrow Reading Principle](../adr/20260410170952-narrow-reading-principle.md) — scope guard for the migration
- [TUI Display Feature](../features/tui-display.md)
- [Keyboard Input Feature](../features/keyboard-input.md)
- [Reading the TUI](../how-to/reading-the-tui.md)
- [Quitting Gracefully](../how-to/quitting-gracefully.md)
