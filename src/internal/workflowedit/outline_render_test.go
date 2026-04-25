package workflowedit

import (
	"strings"
	"testing"

	"github.com/mxriverlynn/pr9k/src/internal/workflowmodel"
)

// TestOutline_RendersStepsList verifies that the outline renders all step names.
func TestOutline_RendersStepsList(t *testing.T) {
	m := newLoadedModel(sampleStep("alpha"), sampleStep("beta"), sampleStep("gamma"))
	view := m.View()
	for _, name := range []string{"alpha", "beta", "gamma"} {
		if !strings.Contains(view, name) {
			t.Errorf("view should contain step name %q", name)
		}
	}
}

// TestOutline_CollapsibleSections verifies section glyphs are available.
func TestOutline_CollapsibleSections(t *testing.T) {
	// The outline section constants must exist and be distinct.
	if GlyphSectionOpen == GlyphSectionClose {
		t.Error("GlyphSectionOpen and GlyphSectionClose should be different")
	}
	if GlyphSectionOpen == "" || GlyphSectionClose == "" {
		t.Error("section glyphs should be non-empty")
	}
}

// TestOutline_FocusedRowHighlight verifies the focused row is distinguished from others.
// In the flat-row layout for two iteration steps, "first" is at row 3
// (0=init header, 1=+Add init, 2=iter header, 3=first, 4=second, …).
func TestOutline_FocusedRowHighlight(t *testing.T) {
	m := newLoadedModel(sampleStep("first"), sampleStep("second"))
	m.focus = focusOutline
	m.outline.cursor = 3 // step "first" is at flat row 3
	view := m.View()
	// The focused row should be prefixed with ">" or the gripper.
	if !strings.Contains(view, "> first") && !strings.Contains(view, GlyphGripper+" first") {
		t.Errorf("focused row should be highlighted; view: %q", view)
	}
}

// TestOutline_UnnamedStep_ShowsPlaceholder verifies unnamed steps use HintNoName.
func TestOutline_UnnamedStep_ShowsPlaceholder(t *testing.T) {
	m := newLoadedModel(workflowmodel.Step{Kind: workflowmodel.StepKindShell})
	view := m.View()
	if !strings.Contains(view, HintNoName) {
		t.Errorf("unnamed step should show %q, got %q", HintNoName, view)
	}
}

// TestOutline_RendersPhaseSections verifies phase section headers with item counts appear.
func TestOutline_RendersPhaseSections(t *testing.T) {
	initStep := sampleStep("init-step")
	initStep.Phase = workflowmodel.StepPhaseInitialize
	iterStep := sampleStep("iter-step")
	iterStep.Phase = workflowmodel.StepPhaseIteration
	finalStep := sampleStep("final-step")
	finalStep.Phase = workflowmodel.StepPhaseFinalize

	m := newLoadedModel(initStep, iterStep, finalStep)
	view := m.View()

	for _, header := range []string{"initialize", "iteration", "finalize"} {
		if !strings.Contains(view, header) {
			t.Errorf("view should contain phase section header %q; got: %q", header, view)
		}
	}
}

// TestOutline_RendersTopLevelSections verifies env/containerEnv/statusLine headers
// appear when items exist.
func TestOutline_RendersTopLevelSections(t *testing.T) {
	m := newLoadedModel()
	m.doc.Env = []string{"GH_TOKEN"}
	m.doc.ContainerEnv = map[string]string{"GOPATH": "/tmp/go"}
	m.doc.StatusLine = &workflowmodel.StatusLineBlock{Command: "echo"}

	view := m.View()
	for _, header := range []string{"env", "containerEnv", "statusLine"} {
		if !strings.Contains(view, header) {
			t.Errorf("view should contain top-level section %q; got: %q", header, view)
		}
	}
}

// TestOutline_AddRowFocused_ShortcutFooterShowsAdd verifies the footer shows "Enter add"
// when the cursor is on a "+ Add" row.
// Flat rows for newLoadedModel(sampleStep("step1")) — no env/containerEnv/statusLine:
//
//	0: initialize header, 1: +Add init,
//	2: iteration header,  3: step1, 4: +Add iter,
//	5: finalize header,   6: +Add final
func TestOutline_AddRowFocused_ShortcutFooterShowsAdd(t *testing.T) {
	m := newLoadedModel(sampleStep("step1"))
	m.focus = focusOutline
	m.outline.cursor = 4 // + Add step (iteration)

	line := m.ShortcutLine()
	if !strings.Contains(line, "Enter") {
		t.Errorf("shortcut line should contain 'Enter' on add row, got %q", line)
	}
	if !strings.Contains(line, "add") {
		t.Errorf("shortcut line should contain 'add' on add row, got %q", line)
	}
}
