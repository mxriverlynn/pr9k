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
func TestOutline_FocusedRowHighlight(t *testing.T) {
	m := newLoadedModel(sampleStep("first"), sampleStep("second"))
	m.focus = focusOutline
	m.outline.cursor = 0
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
