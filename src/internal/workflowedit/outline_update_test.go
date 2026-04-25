package workflowedit

import (
	"strings"
	"testing"

	"github.com/mxriverlynn/pr9k/src/internal/workflowmodel"
)

// Flat-row layout for newLoadedModel(sampleStep("step1")) with no top-level sections:
//   0: initialize header   1: +Add initialize
//   2: iteration header    3: step1   4: +Add iteration
//   5: finalize header     6: +Add finalize

// Flat-row layout for newLoadedModel(sampleStep("step1"), sampleStep("step2")):
//   0: initialize header   1: +Add initialize
//   2: iteration header    3: step1   4: step2   5: +Add iteration
//   6: finalize header     7: +Add finalize

// TestOutline_AddRowEnter_CreatesEmptyItem verifies Enter on a "+ Add step" row
// appends a new empty step in the focused phase.
func TestOutline_AddRowEnter_CreatesEmptyItem(t *testing.T) {
	m := newLoadedModel(sampleStep("step1"))
	m.focus = focusOutline
	m.outline.cursor = 4 // + Add step (iteration)

	m = applyKey(m, keyEnter())

	if len(m.doc.Steps) != 2 {
		t.Errorf("expected 2 steps after add, got %d", len(m.doc.Steps))
	}
	// The new step should be in iteration phase.
	found := false
	for _, s := range m.doc.Steps {
		if s.Name == "" && s.Phase == workflowmodel.StepPhaseIteration {
			found = true
		}
	}
	if !found {
		t.Error("expected new empty iteration step after pressing Enter on add row")
	}
	if !m.dirty {
		t.Error("model should be dirty after adding a step")
	}
}

// TestOutline_SectionCollapseToggle_HidesItems verifies the collapse toggle key
// hides section items and the expand toggle restores them.
func TestOutline_SectionCollapseToggle_HidesItems(t *testing.T) {
	m := newLoadedModel(sampleStep("step1"), sampleStep("step2"))
	m.focus = focusOutline
	m.outline.cursor = 2 // iteration section header

	// Collapse.
	m = applyKey(m, keyRune(' '))
	view := m.View()
	if strings.Contains(view, "step1") || strings.Contains(view, "step2") {
		t.Error("step names should be hidden when section is collapsed")
	}
	if !strings.Contains(view, "iteration") {
		t.Error("iteration section header should still appear when collapsed")
	}

	// Expand.
	m = applyKey(m, keyRune(' '))
	view = m.View()
	if !strings.Contains(view, "step1") || !strings.Contains(view, "step2") {
		t.Error("step names should appear again when section is expanded")
	}
}

// TestOutline_SectionCollapse_MovesCursorToHeader verifies that collapsing a section
// while the cursor is inside it moves the cursor to the section header.
// With step1 and step2 (both iteration), step1 is at flat-row 3.
func TestOutline_SectionCollapse_MovesCursorToHeader(t *testing.T) {
	m := newLoadedModel(sampleStep("step1"), sampleStep("step2"))
	m.focus = focusOutline
	m.outline.cursor = 3 // cursor on "step1" inside iteration section

	// Collapse the section from inside.
	m = applyKey(m, keyRune(' '))

	// After collapse the iteration header is at row 2 (same layout except items hidden).
	if m.outline.cursor != 2 {
		t.Errorf("cursor should jump to iteration header (2) after collapse, got %d", m.outline.cursor)
	}
}

// TestOutline_AOnSectionHeader_TriggersAdd verifies that pressing 'a' on a phase
// section header adds a new empty step in that phase.
func TestOutline_AOnSectionHeader_TriggersAdd(t *testing.T) {
	m := newLoadedModel(sampleStep("step1"))
	m.focus = focusOutline
	m.outline.cursor = 0 // initialize section header

	m = applyKey(m, keyRune('a'))

	initCount := 0
	for _, s := range m.doc.Steps {
		if s.Phase == workflowmodel.StepPhaseInitialize {
			initCount++
		}
	}
	if initCount != 1 {
		t.Errorf("expected 1 initialize step after 'a' on initialize header, got %d", initCount)
	}
	if !m.dirty {
		t.Error("model should be dirty after adding a step")
	}
}
