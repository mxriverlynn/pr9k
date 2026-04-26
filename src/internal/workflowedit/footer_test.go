package workflowedit

import (
	"strings"
	"testing"
)

// TestShortcutLine_DelegatesToFocusedWidget verifies ShortcutLine asks focused widget.
func TestShortcutLine_DelegatesToFocusedWidget(t *testing.T) {
	m := newLoadedModel(sampleStep("a"))

	m.focus = focusOutline
	outlineLine := m.ShortcutLine()

	m.focus = focusDetail
	detailLine := m.ShortcutLine()

	if outlineLine == detailLine {
		t.Error("outline and detail should produce different ShortcutLine output")
	}
}

// TestShortcutLine_OutlineFocus — outline's ShortcutLine is called.
func TestShortcutLine_OutlineFocus(t *testing.T) {
	m := newLoadedModel(sampleStep("a"))
	m.focus = focusOutline
	line := m.ShortcutLine()
	if !strings.Contains(line, "navigate") {
		t.Errorf("outline ShortcutLine should contain navigate hint, got %q", line)
	}
}

// TestShortcutLine_DetailFocus — detail's ShortcutLine is called.
func TestShortcutLine_DetailFocus(t *testing.T) {
	m := newLoadedModel(sampleStep("a"))
	m.focus = focusDetail
	line := m.ShortcutLine()
	if !strings.Contains(line, "Tab") {
		t.Errorf("detail ShortcutLine should contain Tab hint, got %q", line)
	}
}

// TestShortcutLine_HelpHintSuppressedDuringDialog — ? help not shown for non-findings dialogs.
func TestShortcutLine_HelpHintSuppressedDuringDialog(t *testing.T) {
	m := newLoadedModel(sampleStep("a"))
	// QuitConfirm is a non-findings dialog.
	m.dialog = dialogState{kind: DialogQuitConfirm}
	line := m.ShortcutLine()
	if strings.Contains(line, "help") {
		t.Errorf("? help hint should be suppressed during QuitConfirm dialog, got %q", line)
	}
}

// TestShortcutLine_HelpHintShownDuringFindingsPanel — ? help IS shown over FindingsPanel.
func TestShortcutLine_HelpHintShownDuringFindingsPanel(t *testing.T) {
	m := newLoadedModel(sampleStep("a"))
	m.dialog = dialogState{kind: DialogFindingsPanel}
	line := m.ShortcutLine()
	if !strings.Contains(line, "help") {
		t.Errorf("? help hint should be visible during FindingsPanel, got %q", line)
	}
}

// TestShortcutLine_ReorderMode_ShowsReorderHints verifies reorder hints via outline delegate.
func TestShortcutLine_ReorderMode_ShowsReorderHints(t *testing.T) {
	m := newLoadedModel(sampleStep("a"), sampleStep("b"))
	m.focus = focusOutline
	m.reorderMode = true
	line := m.ShortcutLine()
	if !strings.Contains(line, "commit") {
		t.Errorf("reorder mode should show commit hint, got %q", line)
	}
}
