# TUI Status Header & Log Display

Manages the visual status display for the ralph-tui terminal interface, showing iteration progress, step checkboxes, and step separator formatting.

- **Last Updated:** 2026-04-10
- **Authors:**
  - River Bailey

## Overview

- `StatusHeader` is a pointer-mutable struct that Glyph reads on each render cycle — callers update state by mutating fields directly
- Displays the current iteration/issue on one line; shows `Iteration N/M` in bounded mode and `Iteration N` (no total) when total is 0 (unbounded mode)
- Displays step progress as a dynamic grid of rows (4 checkboxes per row), sized at startup to fit the largest phase
- Each step shows one of four states: `[ ]` pending, `[▸]` active, `[✓]` done, `[✗]` failed
- Switches between phases (initialize, iteration, finalize) by calling `SetPhaseSteps` with the new phase's step names
- `StepSeparator` and `RetryStepSeparator` produce formatted separator lines written to the log pipe between steps

Key files:
- `ralph-tui/internal/ui/header.go` — StatusHeader struct, SetIteration, SetPhaseSteps, SetStepState
- `ralph-tui/internal/ui/header_test.go` — Unit tests for header state management
- `ralph-tui/internal/ui/log.go` — StepSeparator, RetryStepSeparator
- `ralph-tui/internal/ui/log_test.go` — Unit tests for separator formatting

## Architecture

```
  Glyph TUI (reads by pointer each render cycle)
       │
       ▼
  ┌─────────────────────────────────────────────┐
  │              StatusHeader                    │
  │                                              │
  │  IterationLine: "Iteration 1/3 — Issue #42" │  ← bounded (total > 0)
  │  IterationLine: "Iteration 1 — Issue #42"   │  ← unbounded (total == 0)
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
| `ralph-tui/internal/ui/log.go` | Step separator line formatting |
| `ralph-tui/internal/ui/log_test.go` | Tests for separator string output |

## Core Types

```go
// StepState represents the display state of a single workflow step.
type StepState int

const (
    StepPending StepState = iota  // [ ]
    StepActive                     // [▸]
    StepDone                       // [✓]
    StepFailed                     // [✗]
)

// HeaderCols is the number of checkbox columns per row; constant to fit 80-column terminals.
const HeaderCols = 4

// StatusHeader manages pointer-mutable string state for the TUI.
// Glyph reads exported fields via pointer on each render cycle.
type StatusHeader struct {
    IterationLine string               // e.g. "Iteration 1/3 — Issue #42: Add widget support" (bounded)
                                       //   or "Iteration 1 — Issue #42: Add widget support" (unbounded, total==0)
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

`SetIteration` updates the iteration line. When `total > 0` the line shows `N/M`; when `total == 0` (unbounded mode, run until done) the total is omitted. `SetStepState` updates individual step checkboxes (0-indexed within the current phase's step list):

```go
func (h *StatusHeader) SetIteration(current, total int, issueID, issueTitle string) {
    if total > 0 {
        h.IterationLine = fmt.Sprintf("Iteration %d/%d — Issue #%s: %s", current, total, issueID, issueTitle)
    } else {
        h.IterationLine = fmt.Sprintf("Iteration %d — Issue #%s: %s", current, issueID, issueTitle)
    }
}

func (h *StatusHeader) SetStepState(idx int, state StepState) {
    if idx < 0 || idx >= len(h.stepNames) { return }  // bounds guard
    r, c := idx/HeaderCols, idx%HeaderCols
    h.Rows[r][c] = checkboxLabel(state, h.stepNames[idx])
}
```

### Checkbox Label Formatting

```go
func checkboxLabel(state StepState, name string) string {
    switch state {
    case StepActive: return fmt.Sprintf("[▸] %s", name)
    case StepDone:   return fmt.Sprintf("[✓] %s", name)
    case StepFailed: return fmt.Sprintf("[✗] %s", name)
    default:         return fmt.Sprintf("[ ] %s", name)
    }
}
```

### Step Separators

Visual separator lines injected into the log pipe between steps:

```go
func StepSeparator(stepName string) string {
    return fmt.Sprintf("── %s ─────────────", stepName)
}

func RetryStepSeparator(stepName string) string {
    return fmt.Sprintf("── %s (retry) ─────────────", stepName)
}
```

These are passed to `Runner.WriteToLog()` by the orchestration loop.

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

// Full layout: iteration line → checkbox rows → log panel → shortcut bar.
children := []any{
    glyph.Text(&header.IterationLine),
    // ...rowWidgets...
    glyph.Log(runner.LogReader()).Grow(1).MaxLines(500).BindVimNav(),
    glyph.Text(keyHandler.ShortcutLinePtr()),
}

app.SetView(glyph.VBox.Border(glyph.BorderRounded).Title("Ralph")(children...))
```

- `glyph.Log(...).Grow(1)` — the log panel expands to fill all remaining vertical space
- `.MaxLines(500)` — caps the in-memory line buffer
- `.BindVimNav()` — enables `↑`/`k` and `↓`/`j` scroll keys inside the log panel
- The shortcut bar is bound via `ShortcutLinePtr()` so mode changes update it in place without additional wiring

## Testing

- `ralph-tui/internal/ui/header_test.go` — Tests for NewStatusHeader (row count computation, negative input), SetIteration (bounded and unbounded), SetPhaseSteps (short/long phases, phase transition clearing, overflow panic, input immutability), SetStepState (state updates, failed steps, out-of-bounds no-op)
- `ralph-tui/internal/ui/log_test.go` — Tests for StepSeparator and RetryStepSeparator output

## Additional Information

- [Architecture Overview](../architecture.md) — System-level view showing how the header fits into the TUI
- [Workflow Orchestration](workflow-orchestration.md) — How step state transitions are triggered during orchestration
- [Keyboard Input & Error Recovery](keyboard-input.md) — How the shortcut bar text changes with keyboard modes
- [Subprocess Execution & Streaming](subprocess-execution.md) — How WriteToLog injects separator lines into the log pipe
- [Step Definitions & Prompt Building](step-definitions.md) — Where step names displayed in the header originate
- [API Design](../coding-standards/api-design.md) — Coding standards for bounds guards on array indexers (used by SetStepState)
