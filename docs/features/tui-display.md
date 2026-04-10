# TUI Status Header & Log Display

Manages the visual status display for the ralph-tui terminal interface, showing iteration progress, step checkboxes, and step separator formatting.

- **Last Updated:** 2026-04-09
- **Authors:**
  - River Bailey

## Overview

- `StatusHeader` is a pointer-mutable struct that Glyph reads on each render cycle — callers update state by mutating fields directly
- Displays the current iteration/issue on one line; shows `Iteration N/M` in bounded mode and `Iteration N` (no total) when total is 0 (unbounded mode)
- Displays step progress as two rows of 4 checkboxes (8 steps total)
- Each step shows one of four states: `[ ]` pending, `[▸]` active, `[✓]` done, `[✗]` failed
- Switches between iteration mode and finalization mode with different step names
- `StepSeparator` and `RetryStepSeparator` produce formatted separator lines written to the log pipe between steps

Key files:
- `ralph-tui/internal/ui/header.go` — StatusHeader struct, SetIteration, SetStepState, SetFinalization
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
  │  Row1: [▸] Feature work  [✓] Test planning   │
  │        [ ] Test writing   [ ] Code review     │
  │                                              │
  │  Row2: [ ] Review fixes   [ ] Close issue     │
  │        [ ] Update docs    [ ] Git push        │
  └─────────────────────────────────────────────┘

  After iteration loop completes:

  ┌─────────────────────────────────────────────┐
  │  IterationLine: "Finalizing 1/3"            │
  │                                              │
  │  Row1: [▸] Deferred work  [ ] Lessons learned│
  │        [ ] Final git push                    │
  │                                              │
  │  Row2: (empty)                               │
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

// StatusHeader manages pointer-mutable string state for the TUI.
// Glyph reads exported fields via pointer on each render cycle.
type StatusHeader struct {
    IterationLine string    // e.g., "Iteration 1/3 — Issue #42: Add widget support" (bounded)
                            //    or "Iteration 1 — Issue #42: Add widget support" (unbounded, total==0)
    Row1          [4]string // checkbox labels for steps 0-3
    Row2          [4]string // checkbox labels for steps 4-7
    stepNames     [8]string
    finalizeNames []string
}
```

## Implementation Details

### Iteration Mode

`NewStatusHeader` takes an array of 8 step names and initializes all checkboxes to pending:

```go
func NewStatusHeader(stepNames [8]string) *StatusHeader {
    h := &StatusHeader{stepNames: stepNames}
    for i, name := range stepNames {
        h.writeLabel(i, StepPending, name)
    }
    return h
}
```

`SetIteration` updates the iteration line. When `total > 0` the line shows `N/M`; when `total == 0` (unbounded mode, run until done) the total is omitted. `SetStepState` updates individual step checkboxes (0-indexed, mapped to Row1[0-3] and Row2[0-3]):

```go
func (h *StatusHeader) SetIteration(current, total int, issueID, issueTitle string) {
    if total > 0 {
        h.IterationLine = fmt.Sprintf("Iteration %d/%d — Issue #%s: %s", current, total, issueID, issueTitle)
    } else {
        h.IterationLine = fmt.Sprintf("Iteration %d — Issue #%s: %s", current, issueID, issueTitle)
    }
}

func (h *StatusHeader) SetStepState(idx int, state StepState) {
    if idx < 0 || idx >= 8 { return }  // bounds guard
    h.writeLabel(idx, state, h.stepNames[idx])
}
```

### Finalization Mode

`SetFinalization` switches the header to finalization mode, replacing iteration step names with finalization step names. Supports up to 8 finalization steps across two rows; unused slots are set to empty string:

```go
func (h *StatusHeader) SetFinalization(current, total int, steps []string) {
    h.IterationLine = fmt.Sprintf("Finalizing %d/%d", current, total)
    h.finalizeNames = steps
    // Fill Row1[0-3] and Row2[0-3] with finalization step names or ""
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

## Testing

- `ralph-tui/internal/ui/header_test.go` — Tests for NewStatusHeader, SetIteration (bounded and unbounded), SetStepState, SetFinalization, SetFinalizeStepState, bounds guards
- `ralph-tui/internal/ui/log_test.go` — Tests for StepSeparator and RetryStepSeparator output

## Additional Information

- [Architecture Overview](../architecture.md) — System-level view showing how the header fits into the TUI
- [Workflow Orchestration](workflow-orchestration.md) — How step state transitions are triggered during orchestration
- [Keyboard Input & Error Recovery](keyboard-input.md) — How the shortcut bar text changes with keyboard modes
- [Subprocess Execution & Streaming](subprocess-execution.md) — How WriteToLog injects separator lines into the log pipe
- [Step Definitions & Prompt Building](step-definitions.md) — Where step names displayed in the header originate
- [API Design](../coding-standards/api-design.md) — Coding standards for bounds guards on array indexers (used by SetStepState)
