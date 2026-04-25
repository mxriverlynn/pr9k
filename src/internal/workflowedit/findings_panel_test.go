package workflowedit

import (
	"testing"

	"github.com/mxriverlynn/pr9k/src/internal/workflowmodel"
)

// TestFindingsPanel_EscCloses — Esc key closes the findings dialog
func TestFindingsPanel_EscCloses(t *testing.T) {
	m := newLoadedModel(sampleStep("s1"))
	m.prevFocus = focusOutline
	m.dialog = dialogState{kind: DialogFindingsPanel}
	m.findingsPanel = buildFindingsPanel(
		[]findingResult{{text: "err1", isFatal: true}},
		m.doc.Steps,
		findingsPanel{ackSet: make(map[string]bool)},
	)

	got := applyKey(m, keyEsc())
	if got.dialog.kind != DialogNone {
		t.Fatalf("want DialogNone after Esc, got %d", got.dialog.kind)
	}
	if got.focus != focusOutline {
		t.Errorf("want focus restored to focusOutline, got %d", got.focus)
	}
}

// TestFindingsPanel_IndependentlyScrollable — Down key scrolls findings, not outline
func TestFindingsPanel_IndependentlyScrollable(t *testing.T) {
	findings := []findingResult{
		{text: "err1", isFatal: true},
		{text: "err2", isFatal: true},
		{text: "err3", isFatal: true},
	}
	m := newLoadedModel(sampleStep("s1"), sampleStep("s2"))
	m.dialog = dialogState{kind: DialogFindingsPanel}
	m.findingsPanel = buildFindingsPanel(findings, m.doc.Steps, findingsPanel{ackSet: make(map[string]bool)})

	// DialogFindingsPanel is active; Down should route to findings panel, not outline
	got := applyKey(m, keyDown())

	// Dialog should stay open (Down doesn't close it)
	if got.dialog.kind != DialogFindingsPanel {
		t.Errorf("dialog should remain open after Down, got %d", got.dialog.kind)
	}
	// Outline cursor should NOT move (findings panel absorbed the key)
	if got.outline.cursor != 0 {
		t.Errorf("outline cursor should not change while findings dialog is active, got %d", got.outline.cursor)
	}
}

// TestFindingsPanel_ScrollStatePreservedAcrossRebuild — viewport offset preserved when rebuilt
func TestFindingsPanel_ScrollStatePreservedAcrossRebuild(t *testing.T) {
	findings1 := []findingResult{
		{text: "err1", isFatal: true},
		{text: "err2", isFatal: true},
	}
	steps := []workflowmodel.Step{}

	fp1 := buildFindingsPanel(findings1, steps, findingsPanel{ackSet: make(map[string]bool)})
	fp1.vp.Height = 1  // small height forces scroll range
	fp1.vp.YOffset = 1 // simulate scrolled-down state

	findings2 := []findingResult{
		{text: "err1", isFatal: true},
		{text: "err2", isFatal: true},
		{text: "err3", isFatal: true}, // superset
	}
	fp2 := buildFindingsPanel(findings2, steps, fp1)

	// The viewport offset should be preserved (not reset to 0 on rebuild)
	if fp2.vp.YOffset < 1 {
		t.Errorf("scroll offset should be preserved across rebuild: want >=1, got %d", fp2.vp.YOffset)
	}
}

// TestFindingsPanel_AcknowledgmentPreserved_WhenNewFindingsSupersetOfAck
func TestFindingsPanel_AcknowledgmentPreserved_WhenNewFindingsSupersetOfAck(t *testing.T) {
	findings1 := []findingResult{
		{text: "err1", isFatal: true},
		{text: "err2", isFatal: true},
	}
	steps := []workflowmodel.Step{}

	fp1 := buildFindingsPanel(findings1, steps, findingsPanel{ackSet: make(map[string]bool)})
	fp1.ackSet["err1"] = true // acknowledge first finding

	findings2 := []findingResult{
		{text: "err1", isFatal: true},
		{text: "err2", isFatal: true},
		{text: "err3", isFatal: true}, // new finding
	}
	fp2 := buildFindingsPanel(findings2, steps, fp1)

	if !fp2.ackSet["err1"] {
		t.Error("acknowledged finding 'err1' should remain acknowledged after rebuild")
	}
	if fp2.ackSet["err3"] {
		t.Error("new finding 'err3' should not be pre-acknowledged")
	}
	if fp2.ackSet["err2"] {
		t.Error("unacknowledged finding 'err2' should not become acknowledged after rebuild")
	}
}

// TestFindingsPanel_EnterJumpsToReferencedField — Enter closes dialog and moves cursor to step
func TestFindingsPanel_EnterJumpsToReferencedField(t *testing.T) {
	findings := []findingResult{
		{text: "error on step s2", isFatal: true, stepName: "s2"},
	}
	steps := []workflowmodel.Step{sampleStep("s1"), sampleStep("s2"), sampleStep("s3")}

	m := newLoadedModel(steps...)
	m.prevFocus = focusOutline
	m.dialog = dialogState{kind: DialogFindingsPanel}
	m.findingsPanel = buildFindingsPanel(findings, m.doc.Steps, findingsPanel{ackSet: make(map[string]bool)})

	got := applyKey(m, keyEnter())

	if got.dialog.kind != DialogNone {
		t.Errorf("dialog should close after Enter, got %d", got.dialog.kind)
	}
	// Outline cursor should jump to step "s2" (index 1)
	if got.outline.cursor != 1 {
		t.Errorf("want outline.cursor=1 (step 's2'), got %d", got.outline.cursor)
	}
}
