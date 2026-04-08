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
//	Row1[0..3]     →  HBox(Text(&h.Row1[0]), ..., Text(&h.Row1[3]))   // steps 1-4
//	Row2[0..3]     →  HBox(Text(&h.Row2[0]), ..., Text(&h.Row2[3]))   // steps 5-8
type StatusHeader struct {
	IterationLine string    // e.g. "Iteration 1/3 — Issue #42: Add widget support"
	Row1          [4]string // checkbox labels for steps 0-3
	Row2          [4]string // checkbox labels for steps 4-7
	stepNames     [8]string
	finalizeNames []string
}

// NewStatusHeader creates a StatusHeader with all 8 iteration step names, each
// initialised to pending state.
func NewStatusHeader(stepNames [8]string) *StatusHeader {
	h := &StatusHeader{stepNames: stepNames}
	for i, name := range stepNames {
		h.writeLabel(i, StepPending, name)
	}
	return h
}

// SetIteration updates the iteration line string.
// issueID is the bare number (e.g. "42"); issueTitle is the issue's full title.
func (h *StatusHeader) SetIteration(current, total int, issueID, issueTitle string) {
	h.IterationLine = fmt.Sprintf("Iteration %d/%d — Issue #%s: %s", current, total, issueID, issueTitle)
}

// SetStepState updates the checkbox label for iteration step idx (0-based, 0-7).
func (h *StatusHeader) SetStepState(idx int, state StepState) {
	if idx < 0 || idx >= 8 {
		return
	}
	h.writeLabel(idx, state, h.stepNames[idx])
}

// SetFinalization switches the header to finalization mode, showing
// "Finalizing current/total" and replacing the step rows with finalization
// step names (all initialised to pending). Supports up to 8 finalization steps
// across two rows; extra slots are set to "".
func (h *StatusHeader) SetFinalization(current, total int, steps []string) {
	h.IterationLine = fmt.Sprintf("Finalizing %d/%d", current, total)
	h.finalizeNames = steps
	for i := range 4 {
		if i < len(steps) {
			h.Row1[i] = checkboxLabel(StepPending, steps[i])
		} else {
			h.Row1[i] = ""
		}
	}
	for i := range 4 {
		if idx := i + 4; idx < len(steps) {
			h.Row2[i] = checkboxLabel(StepPending, steps[idx])
		} else {
			h.Row2[i] = ""
		}
	}
}

// SetFinalizeStepState updates the state of finalization step idx (0-based).
// Must be called after SetFinalization.
func (h *StatusHeader) SetFinalizeStepState(idx int, state StepState) {
	if h.finalizeNames == nil || idx < 0 || idx >= len(h.finalizeNames) {
		return
	}
	label := checkboxLabel(state, h.finalizeNames[idx])
	if idx < 4 {
		h.Row1[idx] = label
	} else {
		h.Row2[idx-4] = label
	}
}

func (h *StatusHeader) writeLabel(idx int, state StepState, name string) {
	label := checkboxLabel(state, name)
	if idx < 4 {
		h.Row1[idx] = label
	} else {
		h.Row2[idx-4] = label
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
