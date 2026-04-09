package ui

import "fmt"

// StepState represents the display state of a single workflow step.
type StepState int

const (
	StepPending StepState = iota
	StepActive
	StepDone
	StepFailed
)

// StatusHeader manages the pointer-mutable string state for the TUI status header.
// Glyph reads the exported fields via pointer on each render cycle — callers
// update state by mutating struct fields directly (e.g. SetIteration, SetStepState).
//
// Layout:
//
//	IterationLine  →  Text(&h.IterationLine)
//	Row1           →  HBox(Text(&h.Row1[0]), ...)   // first half of steps
//	Row2           →  HBox(Text(&h.Row2[0]), ...)   // second half of steps
type StatusHeader struct {
	IterationLine string   // e.g. "Iteration 1/3 — Issue #42: Add widget support"
	Row1          []string // checkbox labels for first half of steps
	Row2          []string // checkbox labels for second half of steps
	stepNames     []string
	finalizeNames []string
}

// NewStatusHeader creates a StatusHeader with the given iteration step names, each
// initialised to pending state. Row1 receives the first ceil(n/2) steps; Row2 the rest.
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

// SetIteration updates the iteration line string.
// issueID is the bare number (e.g. "42"); issueTitle is the issue's full title.
func (h *StatusHeader) SetIteration(current, total int, issueID, issueTitle string) {
	h.IterationLine = fmt.Sprintf("Iteration %d/%d — Issue #%s: %s", current, total, issueID, issueTitle)
}

// SetStepState updates the checkbox label for iteration step idx (0-based).
func (h *StatusHeader) SetStepState(idx int, state StepState) {
	if idx < 0 || idx >= len(h.stepNames) {
		return
	}
	h.writeLabel(idx, state, h.stepNames[idx])
}

// SetFinalization switches the header to finalization mode, showing
// "Finalizing current/total" and replacing the step rows with finalization
// step names (all initialised to pending). Row1 receives the first ceil(n/2)
// steps; Row2 the rest.
func (h *StatusHeader) SetFinalization(current, total int, steps []string) {
	h.IterationLine = fmt.Sprintf("Finalizing %d/%d", current, total)
	names := make([]string, len(steps))
	copy(names, steps)
	h.finalizeNames = names
	rowSize := (len(steps) + 1) / 2
	h.Row1 = make([]string, rowSize)
	h.Row2 = make([]string, len(steps)-rowSize)
	for i := range rowSize {
		h.Row1[i] = checkboxLabel(StepPending, steps[i])
	}
	for i := range len(steps) - rowSize {
		h.Row2[i] = checkboxLabel(StepPending, steps[rowSize+i])
	}
}

// SetFinalizeStepState updates the state of finalization step idx (0-based).
// Must be called after SetFinalization.
func (h *StatusHeader) SetFinalizeStepState(idx int, state StepState) {
	if h.finalizeNames == nil || idx < 0 || idx >= len(h.finalizeNames) {
		return
	}
	rowSize := (len(h.finalizeNames) + 1) / 2
	label := checkboxLabel(state, h.finalizeNames[idx])
	if idx < rowSize {
		h.Row1[idx] = label
	} else {
		h.Row2[idx-rowSize] = label
	}
}

func (h *StatusHeader) writeLabel(idx int, state StepState, name string) {
	label := checkboxLabel(state, name)
	rowSize := (len(h.stepNames) + 1) / 2
	if idx < rowSize {
		h.Row1[idx] = label
	} else {
		h.Row2[idx-rowSize] = label
	}
}

func checkboxLabel(state StepState, name string) string {
	switch state {
	case StepActive:
		return fmt.Sprintf("[▸] %s", name)
	case StepDone:
		return fmt.Sprintf("[✓] %s", name)
	case StepFailed:
		return fmt.Sprintf("[✗] %s", name)
	default:
		return fmt.Sprintf("[ ] %s", name)
	}
}
