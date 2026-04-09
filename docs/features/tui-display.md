# TUI Status Header & Log Display

Manages the visual status display for the ralph-tui terminal interface, showing iteration progress, step checkboxes, and step separator formatting.

- **Last Updated:** 2026-04-09
- **Authors:**
  - River Bailey

## Overview

- `StatusHeader` is a pointer-mutable struct that Glyph reads on each render cycle — callers update state by mutating fields directly
- Displays the current iteration/issue on one line and step progress as two rows of checkboxes, split dynamically based on step count
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
  │  IterationLine: "Iteration 1/3 — Issue #42" │
  │                                              │
  │  Row1: [▸] Feature work   [✓] Test planning  │
  │        [ ] Test writing   [ ] Code review    │
  │                                              │
  │  Row2: [ ] Review fixes   [ ] Close issue    │
  │        [ ] Update docs    [ ] Git push       │
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
    IterationLine string   // e.g., "Iteration 1/3 — Issue #42: Add widget support"
    Row1          []string // checkbox labels for first half of steps
    Row2          []string // checkbox labels for second half of steps
    stepNames     []string
    finalizeNames []string
}
```

## Implementation Details

### Iteration Mode

`NewStatusHeader` takes a slice of step names (any length) and initializes all checkboxes to pending. It makes a defensive copy of the input slice. Row1 receives the first `ceil(n/2)` steps; Row2 receives the rest:

```go
func NewStatusHeader(stepNames []string) *StatusHeader {
    names := make([]string, len(stepNames))
    copy(names, stepNames)
    rowSize := (len(names) + 1) / 2
    h := &StatusHeader{
        stepNames: names,
        Row1:      make([]string, rowSize),
        Row2:      make([]string, len(names)-rowSize),
    }
    for i, name := range names {
        h.writeLabel(i, StepPending, name)
    }
    return h
}
```

`SetIteration` updates the iteration line. `SetStepState` updates individual step checkboxes (0-indexed, bounds-guarded by `len(stepNames)`):

```go
func (h *StatusHeader) SetIteration(current, total int, issueID, issueTitle string) {
    h.IterationLine = fmt.Sprintf("Iteration %d/%d — Issue #%s: %s", current, total, issueID, issueTitle)
}

func (h *StatusHeader) SetStepState(idx int, state StepState) {
    if idx < 0 || idx >= len(h.stepNames) { return }  // bounds guard
    h.writeLabel(idx, state, h.stepNames[idx])
}
```

### Finalization Mode

`SetFinalization` switches the header to finalization mode, replacing iteration step names with finalization step names. Supports any number of finalization steps; Row1 and Row2 are resized dynamically using the same `ceil(n/2)` split:

```go
func (h *StatusHeader) SetFinalization(current, total int, steps []string) {
    h.IterationLine = fmt.Sprintf("Finalizing %d/%d", current, total)
    names := make([]string, len(steps))
    copy(names, steps)
    h.finalizeNames = names
    rowSize := (len(steps) + 1) / 2
    h.Row1 = make([]string, rowSize)
    h.Row2 = make([]string, len(steps)-rowSize)
    // Fill Row1 and Row2 with pending checkboxes
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

- `ralph-tui/internal/ui/header_test.go` — Tests for NewStatusHeader, SetIteration, SetStepState, SetFinalization, SetFinalizeStepState, bounds guards
- `ralph-tui/internal/ui/log_test.go` — Tests for StepSeparator and RetryStepSeparator output

## Additional Information

- [Architecture Overview](../architecture.md) — System-level view showing how the header fits into the TUI
- [Workflow Orchestration](workflow-orchestration.md) — How step state transitions are triggered during orchestration
- [Keyboard Input & Error Recovery](keyboard-input.md) — How the shortcut bar text changes with keyboard modes
- [Subprocess Execution & Streaming](subprocess-execution.md) — How WriteToLog injects separator lines into the log pipe
- [Step Definitions & Prompt Building](step-definitions.md) — Where step names displayed in the header originate
- [API Design](../coding-standards/api-design.md) — Coding standards for bounds guards on array indexers (used by SetStepState)
