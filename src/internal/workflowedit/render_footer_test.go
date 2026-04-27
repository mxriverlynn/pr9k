package workflowedit

import (
	"strings"
	"testing"

	"github.com/charmbracelet/lipgloss"
	"github.com/muesli/termenv"
)

// TestFooter_ValidatingTransient — "Validating…" shown when m.validateInProgress (D-17).
func TestFooter_ValidatingTransient(t *testing.T) {
	m := newLoadedModel(sampleStep("a"))
	m.validateInProgress = true
	line := m.ShortcutLine()
	if !strings.Contains(line, "Validating") {
		t.Errorf("footer should show Validating… when validateInProgress, got %q", line)
	}
}

// TestFooter_SavingTransient — "Saving…" shown when m.saveInProgress (D-17).
func TestFooter_SavingTransient(t *testing.T) {
	m := newLoadedModel(sampleStep("a"))
	m.saveInProgress = true
	line := m.ShortcutLine()
	if !strings.Contains(line, "Saving") {
		t.Errorf("footer should show Saving… when saveInProgress, got %q", line)
	}
}

// TestFooter_NormalShortcutsOtherwise — normal shortcuts when neither flag set.
func TestFooter_NormalShortcutsOtherwise(t *testing.T) {
	m := newLoadedModel(sampleStep("a"))
	m.focus = focusOutline
	line := stripStr(m.ShortcutLine())
	if strings.Contains(line, "Validating") || strings.Contains(line, "Saving") {
		t.Errorf("footer should not show transient text when flags clear, got %q", line)
	}
	if !strings.Contains(line, "navigate") {
		t.Errorf("footer should show outline navigate hint, got %q", line)
	}
}

// TestFooter_GreyedSaveShortcutWhenReadOnly — Save key dim in browse-only (D-18).
func TestFooter_GreyedSaveShortcutWhenReadOnly(t *testing.T) {
	m := newLoadedModel(sampleStep("a"))
	m.banners.isReadOnly = true

	// Stripped text must include "save" to give the user a hint.
	stripped := stripStr(m.ShortcutLine())
	if !strings.Contains(stripped, "save") {
		t.Errorf("footer should show 'save' shortcut hint in browse-only mode, got %q", stripped)
	}

	// Force TrueColor so lipgloss emits ANSI escape codes.
	oldProfile := lipgloss.ColorProfile()
	lipgloss.SetColorProfile(termenv.TrueColor)
	t.Cleanup(func() { lipgloss.SetColorProfile(oldProfile) })

	raw := m.ShortcutLine()
	// uichrome.Dim = lipgloss.Color("8") → ANSI bright-black: [90m or 38;5;8.
	if !strings.Contains(raw, "[90m") && !strings.Contains(raw, "38;5;8") {
		t.Errorf("save shortcut should use Dim color (Color 8) in browse-only mode; raw footer: %q", raw)
	}

	// Non-read-only: "save" should NOT appear in the footer (no explicit Ctrl+S hint).
	m2 := newLoadedModel(sampleStep("a"))
	m2.focus = focusOutline
	line2 := stripStr(m2.ShortcutLine())
	if strings.Contains(line2, "save") {
		t.Errorf("non-read-only footer should not include 'save' shortcut hint, got %q", line2)
	}
}

// TestRender_BrowseOnlyAllSignals — composite D-18: [ro] banner, suppressed dirty
// indicator, and Dim save shortcut in footer all coexist for a read-only workflow.
func TestRender_BrowseOnlyAllSignals(t *testing.T) {
	m := newLoadedModelWithWidth(80, 24, sampleStep("a"))
	// Force dirty so the indicator would normally appear.
	m.doc.Steps[0].Name = "modified"
	m.banners.isReadOnly = true

	view := stripView(m)
	lines := strings.Split(view, "\n")

	// Signal 1: [ro] banner visible somewhere in the session header.
	if !strings.Contains(view, "[ro]") {
		t.Errorf("browse-only view should show [ro] banner; view:\n%s", view)
	}

	// Signal 2: dirty indicator (●) must be suppressed for read-only.
	// Check the session-header line (line 2, index 2 after top-border and menu bar).
	sessionHeader := ""
	if len(lines) > 2 {
		sessionHeader = lines[2]
	}
	if strings.Contains(sessionHeader, "●") {
		t.Errorf("browse-only session header should suppress dirty indicator ●; line: %q", sessionHeader)
	}

	// Signal 3: footer (last non-border line, index height-2) shows "save" hint.
	footerLine := ""
	if len(lines) >= 2 {
		footerLine = lines[len(lines)-2]
	}
	if !strings.Contains(footerLine, "save") {
		t.Errorf("browse-only footer should show 'save' hint; footer line: %q", footerLine)
	}
}
