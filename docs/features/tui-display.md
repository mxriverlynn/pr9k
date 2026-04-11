# TUI Status Header & Log Display

Manages the visual status display for the ralph-tui terminal interface, showing iteration progress, step checkboxes, log panel rhythm, and the full-width phase banners / per-step headings written into the log body.

- **Last Updated:** 2026-04-10
- **Authors:**
  - River Bailey

## Overview

- `StatusHeader` is a pointer-mutable struct that Glyph reads on each render cycle — callers update state by mutating fields directly
- Displays the current iteration/issue on one line; shows `Iteration N/M` in bounded mode and `Iteration N` (no total) when total is 0 (unbounded mode)
- Displays step progress as a dynamic grid of rows (4 checkboxes per row), sized at startup to fit the largest phase
- Each step shows one of five states: `[ ]` pending, `[▸]` active, `[✓]` done, `[✗]` failed, `[-]` skipped
- Switches between phases (initialize, iteration, finalize) by calling `SetPhaseSteps` with the new phase's step names
- The log body is structured with phase banners, iteration separators, per-step "Starting step" banners, variable capture logs, and a final completion summary — all spaced with blank lines (helpers in `log.go`)
- Terminal width for full-width phase banner underlines is detected via `ui.TerminalWidth()` (ioctl TIOCGWINSZ) with an 80-column fallback
- The completion summary line is written to the log body (not the header) as the last non-blank line before `ModeDone`

Key files:
- `ralph-tui/internal/ui/header.go` — StatusHeader struct, RenderInitializeLine, RenderIterationLine, RenderFinalizeLine, SetPhaseSteps, SetStepState
- `ralph-tui/internal/ui/header_test.go` — Unit tests for header state management
- `ralph-tui/internal/ui/log.go` — Log-body helpers: StepSeparator, RetryStepSeparator, StepStartBanner, PhaseBanner, CaptureLog, CompletionSummary
- `ralph-tui/internal/ui/log_test.go` — Unit tests for log-body helper formatting
- `ralph-tui/internal/ui/terminal.go` — TerminalWidth() and DefaultTerminalWidth for sizing full-width banners

## Architecture

```
  Glyph TUI (reads by pointer each render cycle)
       │
       ▼
  ┌─────────────────────────────────────────────┐
  │              StatusHeader                    │
  │                                              │
  │  IterationLine: "Iteration 1/3 — Issue #42" │  ← bounded (maxIter > 0)
  │  IterationLine: "Iteration 1 — Issue #42"   │  ← unbounded (maxIter == 0)
  │                                              │
  │  Rows[0]: [▸] Feature work  [✓] Test planning│
  │           [ ] Test writing   [ ] Code review  │
  │                                              │
  │  Rows[1]: [ ] Review fixes   [ ] Close issue  │
  │           [ ] Update docs    [ ] Git push     │
  └─────────────────────────────────────────────┘

  After SetPhaseSteps called with finalize step names:

  ┌─────────────────────────────────────────────┐
  │  IterationLine: (set by caller)             │
  │                                              │
  │  Rows[0]: [▸] Deferred work  [ ] Lessons learned│
  │           [ ] Final git push                 │
  │                                              │
  │  Rows[1]: (empty — trailing slots cleared)  │
  └─────────────────────────────────────────────┘
```

## Key Files

| File | Purpose |
|------|---------|
| `ralph-tui/internal/ui/header.go` | StatusHeader struct and state mutation methods |
| `ralph-tui/internal/ui/header_test.go` | Tests for iteration/finalization state transitions |
| `ralph-tui/internal/ui/log.go` | Log-body helpers: step/phase banners, capture log, completion summary |
| `ralph-tui/internal/ui/log_test.go` | Tests for log-body helper output |
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

// StatusHeader manages pointer-mutable string state for the TUI.
// Glyph reads exported fields via pointer on each render cycle.
type StatusHeader struct {
    IterationLine string               // e.g. "Iteration 2/5 — Issue #42", "Initializing 1/2: Splash", "Finalizing 1/3: Deferred work"
    Rows          [][HeaderCols]string // row count computed at startup; each row has HeaderCols slots
    stepNames     []string             // current phase's step name list
}
```

## Implementation Details

### Startup Sizing

`NewStatusHeader` takes the maximum step count across all phases and sizes the `Rows` grid to fit. The row count is computed via ceiling division so all steps fit without overflow:

```go
func NewStatusHeader(maxStepsAcrossPhases int) *StatusHeader {
    rowCount := (maxStepsAcrossPhases + HeaderCols - 1) / HeaderCols // ceil division
    if rowCount < 1 {
        rowCount = 1
    }
    return &StatusHeader{
        Rows: make([][HeaderCols]string, rowCount),
    }
}
```

### Phase Switching

`SetPhaseSteps` replaces the current step name list and re-renders all checkbox slots. Call this at the start of each phase to swap the header to the new phase's step set. Trailing slots beyond the current phase's step count are cleared to empty string. Panics if `len(names)` exceeds the grid capacity — this is a programming error (caller passed wrong max to constructor), not a user-reachable path:

```go
func (h *StatusHeader) SetPhaseSteps(names []string) {
    // panics if len(names) > len(h.Rows)*HeaderCols
    h.stepNames = append(h.stepNames[:0], names...)  // copy — does not alias input
    // fills Rows[r][c] with pending checkboxes; clears trailing slots
}
```

### Iteration Line and Step State

Three phase-specific render methods update `IterationLine`. A local `substitute` helper replaces `{{KEY}}` tokens for the initialize and finalize formats; `RenderIterationLine` uses a `strings.Builder` because its output is conditional (bounded/unbounded, with/without issueID). `SetStepState` updates individual step checkboxes (0-indexed within the current phase's step list):

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
    h.Rows[r][c] = checkboxLabel(state, h.stepNames[idx])
}
```

The header has no "completion" render method — after the finalize phase finishes, `IterationLine` retains the final `"Finalizing N/M: <step name>"` value. The completion summary (`"Ralph completed after N iteration(s) and M finalizing tasks."`) is written to the **log body** via `executor.WriteToLog(ui.CompletionSummary(...))` as the last non-blank line before `ModeDone` blocks on the final keypress.

### Checkbox Label Formatting

```go
func checkboxLabel(state StepState, name string) string {
    switch state {
    case StepActive:  return fmt.Sprintf("[▸] %s", name)
    case StepDone:    return fmt.Sprintf("[✓] %s", name)
    case StepFailed:  return fmt.Sprintf("[✗] %s", name)
    case StepSkipped: return fmt.Sprintf("[-] %s", name)
    default:          return fmt.Sprintf("[ ] %s", name)
    }
}
```

### Log-Body Helpers

`ralph-tui/internal/ui/log.go` owns every helper that writes structured chrome into the log pipe. Every helper returns a plain string (or a tuple of strings) — the workflow loop calls `executor.WriteToLog()` with the result so writes go through the same `io.Pipe` path as subprocess output.

```go
// Iteration separator: "── Iteration 1 ─────────────"
// Written once per iteration at the top of the iteration body.
func StepSeparator(stepName string) string {
    return fmt.Sprintf("── %s ─────────────", stepName)
}

// Retry separator: "── <step name> (retry) ─────────────"
// Written by Orchestrate.runStepWithErrorHandling before a retry.
func RetryStepSeparator(stepName string) string {
    return fmt.Sprintf("── %s (retry) ─────────────", stepName)
}

// StepStartBanner: returns the two-line "Starting step: <name>" heading and
// a "─"-character underline whose rune count matches the heading width.
// Orchestrate writes this (plus a trailing blank line) before every step.
func StepStartBanner(stepName string) (heading, underline string)

// PhaseBanner: returns the phase name plus a full-width "═"-character
// underline `width` runes wide. Widths <= 0 are clamped to 1. Run writes
// this on entering each phase (Initializing / Iterations / Finalizing).
func PhaseBanner(phaseName string, width int) (heading, underline string)

// CaptureLog: returns a single-line `Captured VAR = "value"` log entry,
// %q-quoted so multi-line / whitespace-heavy captures stay on one log
// line. Run writes this after any step with CaptureAs set.
func CaptureLog(varName, value string) string

// CompletionSummary: the final body line written before ModeDone.
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

Before `app.Run()` is called, `main()` pre-populates the header so the first rendered frame shows real content instead of empty slots. A `stepNames()` helper (in `main.go`) extracts `Step.Name` from a step slice:

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
- The workflow goroutine then calls `SetPhaseSteps` / `SetStepState` / `RenderIterationLine` (and `RenderInitializeLine`/`RenderFinalizeLine` for those phases) as it progresses, overwriting this initial state

## Glyph Layout Assembly

`main.go` assembles the full `StatusHeader` into Glyph's widget tree. Each `Rows[r][c]` cell is bound via `glyph.Text(&header.Rows[r][c])`, so Glyph reads the current string on every render cycle without any explicit refresh calls:

```go
// One HBox per row, one Text widget per column slot.
rowWidgets := make([]any, len(header.Rows))
for r := range header.Rows {
    cols := make([]any, ui.HeaderCols)
    for c := range cols {
        cols[c] = glyph.Text(&header.Rows[r][c])
    }
    rowWidgets[r] = glyph.HBox(cols...)
}

// Full layout: iteration line → checkbox rows → HRule → log panel → HRule → shortcut bar.
children := []any{
    glyph.Text(&header.IterationLine),
    // ...rowWidgets...
    glyph.HRule(),
    glyph.Log(runner.LogReader()).Grow(1).MaxLines(500).BindVimNav(),
    glyph.HRule(),
    glyph.Text(keyHandler.ShortcutLinePtr()),
}

app.SetView(glyph.VBox.Border(glyph.BorderRounded).Title("Ralph")(children...))
```

- `glyph.Log(...).Grow(1)` — the log panel expands to fill all remaining vertical space
- `.MaxLines(500)` — caps the in-memory line buffer
- `.BindVimNav()` — enables `↑`/`k` and `↓`/`j` scroll keys inside the log panel
- `glyph.HRule()` widgets draw horizontal divider lines below the status header and above the shortcut footer so the three regions are visually separated
- The shortcut bar is bound via `ShortcutLinePtr()` so mode changes update it in place without additional wiring

## Testing

- `ralph-tui/internal/ui/header_test.go` — Tests for NewStatusHeader (row count computation, negative input), RenderInitializeLine/RenderIterationLine/RenderFinalizeLine (bounded and unbounded modes, with/without issueID, substitute template correctness), SetPhaseSteps (short/long phases, phase transition clearing, overflow panic, input immutability), SetStepState (state updates, failed steps, skipped steps, out-of-bounds no-op, grid arithmetic for multi-row layouts)
- `ralph-tui/internal/ui/log_test.go` — Tests for every log-body helper: StepSeparator / RetryStepSeparator formatting, StepStartBanner (ASCII/empty/Unicode rune-count assertions), PhaseBanner (width matching, clamp on non-positive width, `═` fill), CaptureLog (simple/empty/multi-line-escaped/embedded-quotes), CompletionSummary (format exactness)

## Additional Information

- [Architecture Overview](../architecture.md) — System-level view showing how the header fits into the TUI
- [Workflow Orchestration](workflow-orchestration.md) — How step state transitions are triggered during orchestration
- [Keyboard Input & Error Recovery](keyboard-input.md) — How the shortcut bar text changes with keyboard modes
- [Subprocess Execution & Streaming](subprocess-execution.md) — How WriteToLog injects separator lines into the log pipe
- [Step Definitions & Prompt Building](step-definitions.md) — Where step names displayed in the header originate
- [API Design](../coding-standards/api-design.md) — Coding standards for bounds guards on array indexers (used by SetStepState)
