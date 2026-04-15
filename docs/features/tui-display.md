# TUI Status Header & Log Display

Manages the visual status display for the ralph-tui terminal interface, showing iteration progress, step checkboxes, log panel rhythm, and the full-width phase banners / per-step headings written into the log body.

- **Last Updated:** 2026-04-15
- **Authors:**
  - River Bailey

## Overview

- `StatusHeader` is a struct that holds the checkbox grid state and iteration line; mutations are applied on the Bubble Tea Update goroutine via `headerProxy` message-passing
- The iteration line is embedded into the top-border title (not rendered as a separate inner row); it shows `Iteration N/M` in bounded mode and `Iteration N` (no total) when total is 0 (unbounded mode)
- When no stream-json event arrives for ≥15s during an active claude step, the title is suffixed with `  ⋯ thinking (Ns)` — a passive heartbeat indicator (D23) that replaces the "feels alive" contribution of token-level streaming without requiring `--include-partial-messages`. The suffix is pure view state: it updates in-place each second and is never appended to the log ring buffer
- Displays step progress as a dynamic grid of rows (4 checkboxes per row), sized at startup to fit the largest phase
- Each step shows one of five states: `[ ]` pending, `[▸]` active, `[✓]` done, `[✗]` failed, `[-]` skipped; the active step marker (`▸`) is rendered in green; all other chrome is light gray
- Switches between phases (initialize, iteration, finalize) by sending `headerPhaseStepsMsg` through `headerProxy`
- The log body is structured with phase banners, iteration separators, per-step "Starting step" banners, variable capture logs, and a final completion summary — all spaced with blank lines (helpers in `log.go`)
- Terminal width for full-width phase banner underlines is detected via `ui.TerminalWidth()` (ioctl TIOCGWINSZ) with an 80-column fallback
- The completion summary line is written to the log body (not the header) as the last non-blank line before `Run` returns
- `View()` hand-builds the entire rounded frame row-by-row (no `lipgloss.Border` wrapper) so the two horizontal rules can use `├─┤` T-junction glyphs that visually connect to the `│` side borders
- The top-border title renders two-tone: the app name (from the `AppTitle` constant, `Power-Ralph.9000`) in green and the iteration detail after ` — ` in white; log body content renders in white; the version label renders in white; shortcut-bar keys render in white with gray descriptions

Key files:
- `ralph-tui/internal/ui/model.go` — Root Bubble Tea `Model`; `Update` routes messages to sub-models; `View` assembles the full TUI output
- `ralph-tui/internal/ui/header.go` — StatusHeader struct, RenderInitializeLine, RenderIterationLine, RenderFinalizeLine, SetPhaseSteps, SetStepState
- `ralph-tui/internal/ui/header_proxy.go` — `HeaderProxy` — sends header mutations as messages via `program.Send`
- `ralph-tui/internal/ui/header_test.go` — Unit tests for header state management
- `ralph-tui/internal/ui/log.go` — Log-body helpers: StepSeparator, RetryStepSeparator, StepStartBanner, PhaseBanner, CaptureLog, CompletionSummary
- `ralph-tui/internal/ui/log_test.go` — Unit tests for log-body helper formatting
- `ralph-tui/internal/ui/terminal.go` — TerminalWidth() and DefaultTerminalWidth for sizing full-width banners
- `ralph-tui/internal/ui/messages.go` — Message types including `HeartbeatReader` interface and `HeartbeatTickMsg` (D23)

## Architecture

```
  Orchestration goroutine
  (calls headerProxy methods)
         │
         │  program.Send(headerMsg)
         ▼
  ┌──────────────────────────────────────────────────┐
  │            Bubble Tea Update goroutine            │
  │                                                   │
  │  Model.Update(msg) dispatches:                    │
  │  ├─ headerStepStateMsg   → headerModel.apply()   │
  │  ├─ headerPhaseStepsMsg  → headerModel.apply()   │
  │  ├─ headerIterationLineMsg → apply + SetWindowTitle│
  │  ├─ LogLinesMsg          → logModel.Update()     │
  │  ├─ tea.KeyMsg           → keysModel.Update()    │
  │  ├─ HeartbeatTickMsg     → update heartbeatSuffix│
  │  └─ tea.WindowSizeMsg    → resize viewport        │
  │                                                   │
  │  Model.View() assembles (hand-built frame,       │
  │  row-by-row, no lipgloss.Border wrapper):         │
  │  ┌──────────────────────────────────────────────┐│
  │  │╭── Power-Ralph.9000 — Iteration 2/5 — … ──╮ ││  ← dynamic title in top border (green/white)
  │  │ [▸] Feature work   [ ] Test planning      │ ││  ← checkbox grid (per-cell color)
  │  │ [ ] Test writing   [ ] Code review        │ ││
  │  │├───────────────────────────────────────── ┤ ││  ← HRule (T-junctions)
  │  │ [log panel — bubbles/viewport]            │ ││  ← scrollable log viewport (white text)
  │  │├───────────────────────────────────────── ┤ ││  ← HRule (T-junctions)
  │  │ ↑/k up  ↓/j down  n next  q quit          │ ││  ← shortcut footer (ShortcutLine)
  │  │                     ralph-tui v0.2.1      │ ││  ← version label (right-aligned, white)
  │  │╰─────────────────────────────────────────╯  ││  ← bottom border
  │  └──────────────────────────────────────────────┘│
  └──────────────────────────────────────────────────┘
```

## Key Files

| File | Purpose |
|------|---------|
| `ralph-tui/internal/ui/model.go` | Root Bubble Tea Model: Update message routing, View assembly, renderTopBorder |
| `ralph-tui/internal/ui/header.go` | StatusHeader struct and state mutation methods |
| `ralph-tui/internal/ui/header_proxy.go` | HeaderProxy — sends header mutations via program.Send |
| `ralph-tui/internal/ui/header_test.go` | Tests for iteration/finalization state transitions |
| `ralph-tui/internal/ui/log.go` | Log-body helpers: step/phase banners, capture log, completion summary |
| `ralph-tui/internal/ui/log_test.go` | Tests for log-body helper output |
| `ralph-tui/internal/ui/log_panel.go` | logModel: viewport wrapper, 2000-entry ring buffer (`logRingBufferCap`), auto-scroll, logContentStyle |
| `ralph-tui/internal/ui/log_panel_test.go` | Tests for logModel ring buffer, auto-scroll, Home/End key handling |
| `ralph-tui/internal/ui/terminal.go` | `TerminalWidth()` via ioctl + `DefaultTerminalWidth` fallback |

## Core Types

```go
// StepState represents the display state of a single workflow step.
type StepState int

const (
    StepPending StepState = iota  // [ ]
    StepActive                     // [▸]
    StepDone                       // [✓]
    StepFailed                     // [✗]
    StepSkipped                    // [-]
)

// HeaderCols is the number of checkbox columns per row; constant to fit 80-column terminals.
const HeaderCols = 4

// StatusHeader manages the checkbox grid state and iteration line.
// Each cell stores its content split across parallel Prefixes/Markers/Suffixes/Colors
// fields so the marker glyph can be colored independently from the brackets and name.
type StatusHeader struct {
    IterationLine string               // e.g. "Iteration 2/5 — Issue #42"

    Rows          [][HeaderCols]string // legacy single-string labels ("[X] name") — test assertions only

    // Split-cell fields: the checkbox grid is rendered from these.
    Prefixes     [][HeaderCols]string
    Markers      [][HeaderCols]string
    Suffixes     [][HeaderCols]string
    MarkerColors [][HeaderCols]lipgloss.Color
    NameColors   [][HeaderCols]lipgloss.Color

    stepNames []string // current phase's step name list
}
```

## Implementation Details

### Startup Sizing

`NewStatusHeader` takes the maximum step count across all phases and sizes the grid to fit. The row count is computed via ceiling division so all steps fit without overflow:

```go
func NewStatusHeader(maxStepsAcrossPhases int) *StatusHeader {
    rowCount := max((maxStepsAcrossPhases+HeaderCols-1)/HeaderCols, 1) // ceil division, min 1
    return &StatusHeader{
        Rows:         make([][HeaderCols]string, rowCount),
        Prefixes:     make([][HeaderCols]string, rowCount),
        Markers:      make([][HeaderCols]string, rowCount),
        Suffixes:     make([][HeaderCols]string, rowCount),
        MarkerColors: make([][HeaderCols]lipgloss.Color, rowCount),
        NameColors:   make([][HeaderCols]lipgloss.Color, rowCount),
    }
}
```

### Phase Switching

`SetPhaseSteps` replaces the current step name list and re-renders all checkbox slots. Call this at the start of each phase to swap the header to the new phase's step set. Trailing slots beyond the current phase's step count are cleared. Panics if `len(names)` exceeds the grid capacity — this is a programming error, not a user-reachable path:

```go
func (h *StatusHeader) SetPhaseSteps(names []string) {
    // panics if len(names) > len(h.Rows)*HeaderCols
    h.stepNames = append(h.stepNames[:0], names...)  // copy — does not alias input
    // fills each slot with pending state; clears trailing slots
}
```

### Iteration Line and Step State

Three phase-specific render methods update `IterationLine`. `SetStepState` updates individual step checkboxes (0-indexed within the current phase's step list):

```go
// RenderInitializeLine: "Initializing 1/2: Splash"
func (h *StatusHeader) RenderInitializeLine(stepNum, stepCount int, stepName string)

// RenderIterationLine: "Iteration 2/5 — Issue #42" (bounded, with issue)
//                      "Iteration 3" (unbounded, no issue)
func (h *StatusHeader) RenderIterationLine(iter, maxIter int, issueID string)

// RenderFinalizeLine: "Finalizing 1/3: Deferred work"
func (h *StatusHeader) RenderFinalizeLine(stepNum, stepCount int, stepName string)

func (h *StatusHeader) SetStepState(idx int, state StepState) {
    if idx < 0 || idx >= len(h.stepNames) { return }  // bounds guard
    r, c := idx/HeaderCols, idx%HeaderCols
    h.writeCell(r, c, state, h.stepNames[idx])
}
```

### headerProxy Message Passing

The orchestration goroutine never mutates `StatusHeader` directly. Instead, it calls methods on `HeaderProxy`, which wraps each mutation as a typed message and sends it via `program.Send`. The Bubble Tea `Update` goroutine receives the message and calls `headerModel.apply()`, which performs the actual mutation on the `StatusHeader`. This eliminates data races that previously existed when goroutines wrote to header fields directly.

```go
// HeaderProxy funnels all StatusHeader mutations through program.Send.
type HeaderProxy struct {
    send func(tea.Msg)
}

func NewHeaderProxy(send func(tea.Msg)) *HeaderProxy {
    return &HeaderProxy{send: send}
}

func (p *HeaderProxy) SetStepState(idx int, state StepState) {
    p.send(headerStepStateMsg{idx: idx, state: state})
}
// ... RenderIterationLine, SetPhaseSteps, etc. similarly
```

### Heartbeat Indicator (D23)

During active claude steps, stream-json can be silent for 30+ seconds while claude thinks or waits on a slow tool. The heartbeat indicator shows `  ⋯ thinking (Ns)` in the top-border title after 15 seconds of silence, updating in place each second until the next event arrives.

**Architecture:**
- `ui.HeartbeatReader` interface (in `messages.go`) exposes `HeartbeatSilence() (time.Duration, bool)` — implemented by `workflow.Runner`
- `Runner.HeartbeatSilence()` reads `activePipeline.LastEventAt()` under `processMu`. When no events have arrived yet (zero `LastEventAt`), it counts silence from `activePipelineStartedAt` instead
- `Runner.activePipeline` is set when a claude step starts (in `RunSandboxedStep`) and cleared in a LIFO defer before `pipeline.Close()`
- `Model.WithHeartbeat(h HeartbeatReader)` installs the reader at construction (called from `main.go`); tests pass nil to keep `Init()` returning nil and avoid starting a ticker
- `Model.Init()` starts a 1-second `tea.Tick` when a heartbeat reader is set; each tick emits `HeartbeatTickMsg`
- `Model.Update()` handles `HeartbeatTickMsg`: reads silence duration, updates `heartbeatSuffix`, reschedules the ticker
- `Model.titleString()` appends `heartbeatSuffix` to the iteration line: `"Power-Ralph.9000 — Iteration 2/5 — Issue #42  ⋯ thinking (17s)"`

The suffix is pure view state — it is never sent to the log ring buffer and does not affect the log body.

```go
// ui.HeartbeatReader — implemented by workflow.Runner
type HeartbeatReader interface {
    HeartbeatSilence() (time.Duration, bool)
}

// workflow.Runner — satisfies HeartbeatReader
func (r *Runner) HeartbeatSilence() (time.Duration, bool) {
    r.processMu.Lock()
    pipeline := r.activePipeline
    startedAt := r.activePipelineStartedAt
    r.processMu.Unlock()

    if pipeline == nil {
        return 0, false
    }
    t := pipeline.LastEventAt()
    if t.IsZero() {
        return time.Since(startedAt), true
    }
    return time.Since(t), true
}
```

### View Assembly

`Model.View()` hand-builds the complete TUI output row-by-row each render cycle, using a small `wrapLine` helper that wraps each content line in `│ … │` side borders with right-padding to `innerWidth` (and `MaxWidth` truncation for overflow):

1. **Top border with title** — `renderTopBorder(titleString())` builds a hand-crafted `╭── Power-Ralph.9000 — <iterationLine> ─ … ─╮`. The title is two-tone colored via `colorTitle()`: the `AppTitle` constant (`Power-Ralph.9000`) renders in green (color 10) and the iteration detail after the ` — ` separator renders in white (color 15). `lipgloss.Width` provides rune-aware truncation when the title overflows.
2. **Checkbox grid** — before rendering, a first pass computes `contentMaxWidth` by calling `lipgloss.Width` on every `Rows[r][c]` value across all rows and columns. A terminal-derived `termCellWidth` is also computed as `(innerWidth - separatorWidth) / HeaderCols`, and `cellWidth = max(termCellWidth, contentMaxWidth)` takes whichever is larger — this makes the grid fill the terminal edge-to-edge on wide terminals and fall back to content-width on narrow ones. A second pass renders each cell as three adjacent Lip Gloss spans: `Prefix` + `Marker` + `Suffix`, each with its own `lipgloss.Color` from `NameColors`/`MarkerColors`. After the three spans are written, trailing spaces are appended until the cell reaches `cellWidth`, so all four columns occupy the same visual width and the step list is distributed evenly across the header. Active steps appear in white/green; all other states in light gray. Each row is wrapped in sidebars via `wrapLine`.
3. **HRule** — `gray.Render("├" + strings.Repeat("─", innerWidth) + "┤")` uses T-junction glyphs so the rule visually connects to the `│` side borders at both ends.
4. **Log viewport** — `m.log.View()` from the `bubbles/viewport` sub-model is split on `\n` and each resulting line is wrapped individually via `wrapLine`. The viewport content is set through `logContentStyle` (`White` foreground) so log body text pops against the gray chrome.
5. **HRule** — same T-junction form as step 3.
6. **Footer** — shortcut bar (left, from `m.keys.handler.ShortcutLine()`, passed through `colorShortcutLine` for per-key coloring) + spacer + version label (right, from `m.versionLabel`, rendered white), wrapped via `wrapLine`.
7. **Bottom border** — `gray.Render("╰" + strings.Repeat("─", innerWidth) + "╯")`.

There is **no** `lipgloss.Border` wrapper around the inner block. The previous approach (wrapping inner content in a `Border(RoundedBorder()).BorderTop(false)` style) was scrapped because it forced plain `─` on the hrule rows and left visual gaps at the side-border intersections; hand-building each row is what lets the hrules use the `├─┤` T-junction glyphs.

### Shortcut Footer Coloring

`colorShortcutLine()` applies the footer's per-key color scheme. For the key-mapping lines (`NormalShortcuts`, `ErrorShortcuts`), the string is split on `"  "` (double-space) into groups; within each group the first whitespace-separated token is the mapped key (rendered `White`) and the rest is its description (rendered `LightGray`). The `"  "` separators between groups remain `LightGray`.

For status-message lines the whole string renders in `White`, with one exception: when the line is `QuitConfirmPrompt`, `colorShortcutLine` splits the string on the `AppTitle` substring and renders that substring in `Green` to match the top-border title's brand color, with the surrounding prompt text in `White`.

### Checkbox Label Formatting

Each cell is split into three adjacent spans using the split-cell fields. `cellStyle` returns the marker glyph and per-cell colors for a given step state:

```go
func cellStyle(state StepState) (marker string, nameColor, markerColor lipgloss.Color) {
    switch state {
    case StepActive:  return "▸", ActiveStepFG, ActiveMarkerFG  // white name, green marker
    case StepDone:    return "✓", LightGray, LightGray
    case StepFailed:  return "✗", LightGray, LightGray
    case StepSkipped: return "-", LightGray, LightGray
    default:          return " ", LightGray, LightGray           // pending
    }
}
```

### Log-Body Helpers

`ralph-tui/internal/ui/log.go` owns every helper that writes structured chrome into the log body. Every helper returns a plain string (or a tuple of strings) — the workflow loop calls `executor.WriteToLog()` with the result.

```go
// Iteration separator: "── Iteration 1 ─────────────"
func StepSeparator(stepName string) string

// Retry separator: "── <step name> (retry) ─────────────"
func RetryStepSeparator(stepName string) string

// StepStartBanner: returns the two-line "Starting step: <name>" heading and
// a "─"-character underline whose rune count matches the heading width.
func StepStartBanner(stepName string) (heading, underline string)

// PhaseBanner: returns the phase name plus a full-width "═"-character
// underline `width` runes wide. Widths <= 0 are clamped to 1.
func PhaseBanner(phaseName string, width int) (heading, underline string)

// CaptureLog: returns a single-line `Captured VAR = "value"` log entry.
func CaptureLog(varName, value string) string

// CompletionSummary: the final body line written before Run returns.
// Format: "Ralph completed after N iteration(s) and M finalizing tasks."
func CompletionSummary(iterationsRun, finalizeCount int) string
```

### Log Body Rhythm

`workflow.Run` interleaves helper output with subprocess streaming to produce a readable run log. A local `emitBlank` closure writes a single blank separator line before every content block (no-op on the very first content so the log does not begin with a blank line). The rhythm for a run with one init step, two iterations of two steps each, and two finalize steps — assuming the second iteration step in each iteration has `captureAs: "ISSUE_ID"` — is:

```
Initializing
════════════════════════════════════════

Starting step: Get GitHub user
──────────────────────────────

[init step output]

════════════════════════════════════════
Iterations
════════════════════════════════════════

── Iteration 1 ─────────────

Starting step: Get next issue
─────────────────────────────

[step output]

Captured ISSUE_ID = "42"

Starting step: Feature work
───────────────────────────

[step output]

── Iteration 2 ─────────────

Starting step: Get next issue
─────────────────────────────

[step output]

Captured ISSUE_ID = "43"

Starting step: Feature work
───────────────────────────

[step output]

Finalizing
════════════════════════════════════════

Starting step: Deferred work
────────────────────────────

[final step output]

Starting step: Final git push
─────────────────────────────

[final step output]

Ralph completed after 2 iteration(s) and 2 finalizing tasks.
```

Key properties:
- Phase banners are full-width (`═` repeated `cfg.LogWidth` runes); the underline fills the log panel so phase transitions stand out
- Iteration separators (`── Iteration N ─────────────`) are a fixed-width `StepSeparator`
- Per-step banners (`Starting step: <name>` + `─` underline of matching width) are written by `Orchestrate` for every started step — initialize, iteration, and finalize phases all share this via `StepStartBanner`
- Capture logs appear immediately after the step whose `captureAs` produced them, offset by a blank line for visual separation from raw subprocess output
- The completion summary is the last non-blank line — readable as a terminal state even if the panel is scrolled to the bottom

### Terminal Width Detection

`ui.TerminalWidth()` wraps `unix.IoctlGetWinsize(stdout, TIOCGWINSZ)` to return the current terminal column count. Non-TTY or ioctl failure returns `ui.DefaultTerminalWidth` (80). `main.go` computes `LogWidth` once at startup from `ui.TerminalWidth() - 2` (subtracting the rounded-border glyphs) and passes it through `workflow.RunConfig.LogWidth`. `workflow.Run` re-derives the effective width each call, falling back to `DefaultTerminalWidth` when `LogWidth <= 0` so test doubles without a TTY still produce deterministic banners.

## First-Frame Pre-Population

Before `program.Run()` is called, `main()` pre-populates the header so the first rendered frame shows real content instead of empty slots. A `stepNames()` helper (in `main.go`) extracts `Step.Name` from a step slice:

```go
func stepNames(ss []steps.Step) []string {
    names := make([]string, len(ss))
    for i, s := range ss {
        names[i] = s.Name
    }
    return names
}
```

The pre-population block runs immediately after `NewStatusHeader`:

```go
if len(stepFile.Initialize) > 0 {
    header.SetPhaseSteps(stepNames(stepFile.Initialize))
    header.SetStepState(0, ui.StepActive)
    header.IterationLine = "Initializing 1/" + strconv.Itoa(len(stepFile.Initialize)) + ": " + stepFile.Initialize[0].Name
} else {
    header.SetPhaseSteps(stepNames(stepFile.Iteration))
    header.SetStepState(0, ui.StepActive)
    if cfg.Iterations > 0 {
        header.IterationLine = "Iteration 1/" + strconv.Itoa(cfg.Iterations)
    } else {
        header.IterationLine = "Iteration 1"
    }
}
```

- If an initialize phase exists, the header is set to that phase with the first step marked active and `IterationLine` showing `Initializing 1/N: <step name>`
- Otherwise the header starts on the iteration phase with `Iteration 1/M` (bounded) or `Iteration 1` (unbounded)
- The workflow goroutine then sends header messages via `headerProxy` as it progresses, which the Update goroutine applies to the `StatusHeader`

## Testing

- `ralph-tui/internal/ui/header_test.go` — Tests for NewStatusHeader (row count computation, negative input), RenderInitializeLine/RenderIterationLine/RenderFinalizeLine (bounded and unbounded modes, with/without issueID, substitute template correctness), SetPhaseSteps (short/long phases, phase transition clearing, overflow panic, input immutability), SetStepState (state updates, failed steps, skipped steps, out-of-bounds no-op, grid arithmetic for multi-row layouts)
- `ralph-tui/internal/ui/log_test.go` — Tests for every log-body helper: StepSeparator / RetryStepSeparator formatting, StepStartBanner (ASCII/empty/Unicode rune-count assertions), PhaseBanner (width matching, clamp on non-positive width, `═` fill), CaptureLog (simple/empty/multi-line-escaped/embedded-quotes), CompletionSummary (format exactness)
- `ralph-tui/internal/ui/model_test.go` — Smoke test for `View()` (non-empty output, contains version label, contains step name), panic-safety test (zero-dimension WindowSizeMsg), header message routing, title assembly, renderTopBorder edge cases, viewport clamping, checkbox grid even-spacing (`TestView_CheckboxGrid_EqualCellWidth` plus TP-001 through TP-005: multi-row global max, empty trailing cells, equal-width no-op padding, truncation interaction, single-step minimum), `colorShortcutLine` plain-text preservation for `NormalShortcuts` and `ErrorShortcuts`, `QuitConfirmPrompt` AppTitle embedding, `QuittingLine` pass-through, D23 heartbeat indicator tests (no-tick without reader, tick cmd returned, suffix at ≥15s, no suffix when inactive, no suffix below threshold, suffix cleared on transition)
- `ralph-tui/internal/ui/header_proxy_test.go` — Tests for each `HeaderProxy` method (correct message type and fields)

## Additional Information

- [Architecture Overview](../architecture.md) — System-level view showing how the header fits into the TUI
- [Workflow Orchestration](workflow-orchestration.md) — How step state transitions are triggered during orchestration
- [Keyboard Input & Error Recovery](keyboard-input.md) — How the shortcut bar text changes with keyboard modes
- [Subprocess Execution & Streaming](subprocess-execution.md) — How WriteToLog injects separator lines into the log stream
- [Step Definitions & Prompt Building](step-definitions.md) — Where step names displayed in the header originate
- [API Design](../coding-standards/api-design.md) — Coding standards for bounds guards on array indexers (used by SetStepState)
