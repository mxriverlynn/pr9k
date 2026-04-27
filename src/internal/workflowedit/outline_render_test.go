package workflowedit

import (
	"fmt"
	"strings"
	"testing"

	"github.com/charmbracelet/lipgloss"
	"github.com/muesli/termenv"
	"github.com/mxriverlynn/pr9k/src/internal/ansi"
	"github.com/mxriverlynn/pr9k/src/internal/workflowmodel"
)

// TestOutline_RendersStepsList verifies that the outline renders all step names.
func TestOutline_RendersStepsList(t *testing.T) {
	m := newLoadedModel(sampleStep("alpha"), sampleStep("beta"), sampleStep("gamma"))
	view := stripView(m)
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
	m := newLoadedModelWithWidth(80, 24, sampleStep("first"), sampleStep("second"))
	m.focus = focusOutline
	m.outline.cursor = 3 // step "first" is at flat row 3
	view := stripView(m)
	// The focused row should be prefixed with "> " followed by the gripper.
	wantPrefix := "> " + GlyphGripper
	if !strings.Contains(view, wantPrefix) {
		t.Errorf("focused row should start with %q; view: %q", wantPrefix, view)
	}
	if !strings.Contains(view, "first") {
		t.Errorf("focused step name 'first' should be visible; view: %q", view)
	}
}

// TestOutline_UnnamedStep_ShowsPlaceholder verifies unnamed steps use HintNoName.
func TestOutline_UnnamedStep_ShowsPlaceholder(t *testing.T) {
	m := newLoadedModel(workflowmodel.Step{Kind: workflowmodel.StepKindShell})
	view := stripView(m)
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
	view := stripView(m)

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

	view := stripView(m)
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

	line := stripStr(m.ShortcutLine())
	if !strings.Contains(line, "Enter") {
		t.Errorf("shortcut line should contain 'Enter' on add row, got %q", line)
	}
	if !strings.Contains(line, "add") {
		t.Errorf("shortcut line should contain 'add' on add row, got %q", line)
	}
}

// TestOutline_BorderedPane verifies the outline pane renders with ╭ and ╰ chrome
// glyphs and that its top border references "Outline" (D18).
func TestOutline_BorderedPane(t *testing.T) {
	m := newLoadedModelWithWidth(80, 24, sampleStep("alpha"))
	view := stripView(m)
	// The outline pane top border should reference "Outline"; the outer frame
	// says "pr9k workflow builder" so this is outline-specific.
	if !strings.Contains(view, "Outline") {
		t.Errorf("outline pane top border should contain 'Outline', got: %q", view)
	}
	// At least two ╰ glyphs: one from the outer frame bottom border, one from the
	// outline pane bottom border.
	if strings.Count(view, "╰") < 2 {
		t.Errorf("view should have ≥2 ╰ glyphs (outer frame + outline pane), got %d in: %q",
			strings.Count(view, "╰"), view)
	}
}

// TestOutline_KindGlyphs verifies each step kind renders the correct glyph (D21).
func TestOutline_KindGlyphs(t *testing.T) {
	shell := sampleStep("shell-step")         // StepKindShell
	claude := sampleClaudeStep("claude-step") // StepKindClaude

	m := newLoadedModelWithWidth(80, 24, shell, claude)
	view := stripView(m)

	if !strings.Contains(view, GlyphKindShell) {
		t.Errorf("view should contain shell kind glyph %q, got: %q", GlyphKindShell, view)
	}
	if !strings.Contains(view, GlyphKindClaude) {
		t.Errorf("view should contain Claude kind glyph %q, got: %q", GlyphKindClaude, view)
	}
}

// TestOutline_FocusPrefix verifies the focused step row shows the "> " prefix
// followed by the gripper glyph (D22).
func TestOutline_FocusPrefix(t *testing.T) {
	m := newLoadedModelWithWidth(80, 24, sampleStep("alpha"))
	m.focus = focusOutline
	// Flat rows: 0=init header, 1=+Add init, 2=iter header, 3=alpha, 4=+Add iter,
	// 5=fin header, 6=+Add fin.
	m.outline.cursor = 3 // "alpha" step

	view := stripView(m)
	wantPrefix := "> " + GlyphGripper
	if !strings.Contains(view, wantPrefix) {
		t.Errorf("focused step should have prefix %q, got: %q", wantPrefix, view)
	}
}

// TestOutline_PhaseHeader verifies phase section headers render with section name
// and item count (D20, D22).
func TestOutline_PhaseHeader(t *testing.T) {
	step := sampleStep("my-step")
	step.Phase = workflowmodel.StepPhaseIteration

	m := newLoadedModelWithWidth(80, 24, step)
	view := stripView(m)

	for _, want := range []string{"initialize (0)", "iteration (1)", "finalize (0)"} {
		if !strings.Contains(view, want) {
			t.Errorf("view should contain phase header %q, got: %q", want, view)
		}
	}
}

// TestOutline_StepNameTruncated verifies that step names longer than the pane
// width are truncated using lipgloss.Width for CJK-safe measurement (D49).
func TestOutline_StepNameTruncated(t *testing.T) {
	longName := strings.Repeat("x", 80) // far wider than any outline pane
	step := sampleStep(longName)
	m := newLoadedModelWithWidth(80, 24, step)
	view := stripView(m)

	if strings.Contains(view, longName) {
		t.Errorf("long step name should be truncated; full name found in view")
	}
	// The beginning of the name should still be visible.
	if !strings.Contains(view, "xxx") {
		t.Errorf("truncated step name should still be partially visible, got: %q", view)
	}
}

// TestOutline_ScrollIndicator verifies the scroll indicator (▲/▼) appears when
// the outline content exceeds the pane height (D25 visual spec).
func TestOutline_ScrollIndicator(t *testing.T) {
	var steps []workflowmodel.Step
	for i := 0; i < 20; i++ {
		steps = append(steps, sampleStep(fmt.Sprintf("step-%d", i)))
	}
	m := newLoadedModelWithWidth(80, 24, steps...)
	view := stripView(m)

	if !strings.Contains(view, GlyphScrollDown) && !strings.Contains(view, GlyphScrollUp) {
		t.Errorf("scroll indicator should appear when content overflows; got: %q", view)
	}
}

// TestOutline_ReverseVideoOnReorderActive verifies that only the reorder-mode
// active step uses reverse-video styling; other rows do not (impl-D25).
func TestOutline_ReverseVideoOnReorderActive(t *testing.T) {
	// Force TrueColor so lipgloss produces ANSI escape codes in the test env.
	oldProfile := lipgloss.ColorProfile()
	lipgloss.SetColorProfile(termenv.TrueColor)
	t.Cleanup(func() { lipgloss.SetColorProfile(oldProfile) })

	m := newLoadedModelWithWidth(80, 24, sampleStep("alpha"), sampleStep("beta"))
	m.focus = focusOutline
	// Flat rows: 0=init header, 1=+Add init, 2=iter header,
	// 3=alpha, 4=beta, 5=+Add iter, 6=fin header, 7=+Add fin.
	m.outline.cursor = 3 // "alpha" step
	m.reorderMode = true

	raw := m.View()
	lines := strings.Split(raw, "\n")

	const revCode = "\x1b[7m"

	// GlyphGripper only appears in outline step rows, never in detail pane rows,
	// so requiring both GlyphGripper and the step name selects the outline row.
	var alphaLine, betaLine string
	for _, line := range lines {
		stripped := string(ansi.StripAll([]byte(line)))
		if strings.Contains(stripped, GlyphGripper) && strings.Contains(stripped, "alpha") && alphaLine == "" {
			alphaLine = line
		}
		if strings.Contains(stripped, GlyphGripper) && strings.Contains(stripped, "beta") && betaLine == "" {
			betaLine = line
		}
	}

	if alphaLine == "" {
		t.Fatal("could not find a line containing 'alpha' in view")
	}
	if betaLine == "" {
		t.Fatal("could not find a line containing 'beta' in view")
	}

	if !strings.Contains(alphaLine, revCode) {
		t.Errorf("reorder-active step 'alpha' should use reverse-video (\\x1b[7m); line: %q", alphaLine)
	}
	if strings.Contains(betaLine, revCode) {
		t.Errorf("non-active step 'beta' should not use reverse-video; line: %q", betaLine)
	}
}
