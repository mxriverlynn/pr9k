package workflowedit

import (
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/mxriverlynn/pr9k/src/internal/workflowmodel"
)

// TestDetailPane_ClaudeStep_ShowsModelAndPromptFile verifies claude-specific fields.
// Cursor must be on the step row (row 3 for a single iteration step).
func TestDetailPane_ClaudeStep_ShowsModelAndPromptFile(t *testing.T) {
	step := sampleClaudeStep("sonnet-step")
	m := newLoadedModel(step)
	m.focus = focusDetail
	m.outline.cursor = 3 // step at flat row 3 (0=init hdr, 1=+Add, 2=iter hdr, 3=step)
	view := stripView(m)
	if !strings.Contains(view, "claude-sonnet-4-6") {
		t.Errorf("detail pane should show model name, view: %q", view)
	}
	if !strings.Contains(view, "prompts/sonnet-step.md") {
		t.Errorf("detail pane should show prompt file, view: %q", view)
	}
}

// TestDetailPane_ShellStep_ShowsCommand verifies shell-specific fields.
func TestDetailPane_ShellStep_ShowsCommand(t *testing.T) {
	step := sampleStep("shell-step")
	m := newLoadedModel(step)
	m.focus = focusDetail
	m.outline.cursor = 3 // step at flat row 3
	view := stripView(m)
	if !strings.Contains(view, "echo") {
		t.Errorf("detail pane should show command, view: %q", view)
	}
}

// TestDetailPane_MaskedEnv_ShowsMaskedValue verifies containerEnv masking.
// Keys matching the secret-suffix pattern (_TOKEN, _SECRET, etc.) are masked.
func TestDetailPane_MaskedEnv_ShowsMaskedValue(t *testing.T) {
	step := workflowmodel.Step{
		Name:    "s",
		Kind:    workflowmodel.StepKindShell,
		Command: []string{"echo"},
		Env:     []workflowmodel.EnvEntry{{Key: "MY_API_SECRET", Value: "hunter2", IsLiteral: true}},
	}
	m := newLoadedModel(step)
	m.focus = focusDetail
	m.outline.cursor = 3 // step at flat row 3
	// revealedField = -1 (default) means the value should be masked.
	view := stripView(m)
	if strings.Contains(view, "hunter2") {
		t.Error("secret value should not appear in view when masked")
	}
	if !strings.Contains(view, GlyphMasked) {
		t.Errorf("view should contain masked glyph %q", GlyphMasked)
	}
}

// TestDetailPane_AutoScroll_CursorInView verifies scrolls counter updates on scroll.
func TestDetailPane_AutoScroll_ScrollsCounterUpdates(t *testing.T) {
	m := newLoadedModelWithWidth(100, 25, sampleStep("a"))
	// Send a mouse wheel in the detail area (X > outlineWidth(100)=40, so X=50).
	msg := tea.MouseMsg{X: 50, Button: tea.MouseButtonWheelDown}
	got := applyMsg(m, msg)
	if got.detail.scrolls < 1 {
		t.Error("detail.scrolls should increment on mouse wheel in detail column")
	}
}
