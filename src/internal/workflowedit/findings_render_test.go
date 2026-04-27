package workflowedit

import (
	"strings"
	"testing"

	"github.com/charmbracelet/lipgloss"
	"github.com/muesli/termenv"
)

// TestFindingsPanel_RenderInDetailPane verifies that actual finding entries appear
// in the view when DialogFindingsPanel is active (D38).
func TestFindingsPanel_RenderInDetailPane(t *testing.T) {
	m := newLoadedModel(sampleStep("s1"))
	m.dialog = dialogState{kind: DialogFindingsPanel}
	m.findingsPanel = buildFindingsPanel(
		[]findingResult{{text: "step model required", isFatal: true}},
		m.doc.Steps,
		findingsPanel{ackSet: make(map[string]bool)},
	)
	view := stripView(m)
	if !strings.Contains(view, "step model required") {
		t.Errorf("findings panel should show actual finding text; view:\n%s", view)
	}
}

// TestFindingsPanel_AcknowledgedGlyph verifies that [WARN ✓] appears for warnings
// that have been acknowledged in the current session (D39).
func TestFindingsPanel_AcknowledgedGlyph(t *testing.T) {
	m := newLoadedModel(sampleStep("s1"))
	m.dialog = dialogState{kind: DialogFindingsPanel}
	m.findingsPanel = buildFindingsPanel(
		[]findingResult{{text: "captureAs no consumer", isFatal: false}},
		m.doc.Steps,
		findingsPanel{ackSet: make(map[string]bool)},
	)
	// Acknowledge the warning.
	m.findingsPanel.ackSet["captureAs no consumer"] = true
	view := stripView(m)
	if !strings.Contains(view, "[WARN ✓]") {
		t.Errorf("acknowledged warning should show [WARN ✓]; view:\n%s", view)
	}
}

// TestFindingsPanel_DimUnderHelp verifies that Color("8") is applied to findings
// panel content when the help modal is open over it (dim-under-help coexistence).
// The test calls renderFindingsPanel() directly because the help modal overlay
// completely covers the findings panel in the assembled View(), making the dimmed
// text invisible at the frame level.
func TestFindingsPanel_DimUnderHelp(t *testing.T) {
	m := newLoadedModel(sampleStep("s1"))
	m.dialog = dialogState{kind: DialogFindingsPanel}
	m.findingsPanel = buildFindingsPanel(
		[]findingResult{{text: "step-model-required", isFatal: true}},
		m.doc.Steps,
		findingsPanel{ackSet: make(map[string]bool)},
	)
	m.helpOpen = true

	// Force TrueColor so lipgloss emits ANSI escape codes in the test environment.
	oldProfile := lipgloss.ColorProfile()
	lipgloss.SetColorProfile(termenv.TrueColor)
	t.Cleanup(func() { lipgloss.SetColorProfile(oldProfile) })

	// renderFindingsPanel returns the panel before the help overlay is spliced.
	raw := m.renderFindingsPanel()
	found := false
	for _, line := range strings.Split(raw, "\n") {
		if strings.Contains(stripStr(line), "step-model-required") {
			// Color("8") renders as ANSI bright-black: [90m (16-color) or 38;5;8 (256-color).
			if strings.Contains(line, "[90m") || strings.Contains(line, "38;5;8") {
				found = true
				break
			}
		}
	}
	if !found {
		t.Errorf("finding text should be dimmed (Color 8) when help modal is open; panel:\n%s", raw)
	}
}

// TestFindingsPanel_FullColorWithoutHelp verifies that findings are NOT dimmed
// when the help modal is closed.
func TestFindingsPanel_FullColorWithoutHelp(t *testing.T) {
	m := newLoadedModel(sampleStep("s1"))
	m.dialog = dialogState{kind: DialogFindingsPanel}
	m.findingsPanel = buildFindingsPanel(
		[]findingResult{{text: "step-model-required", isFatal: true}},
		m.doc.Steps,
		findingsPanel{ackSet: make(map[string]bool)},
	)
	m.helpOpen = false

	// Force TrueColor so lipgloss would emit ANSI codes (if any were present).
	oldProfile := lipgloss.ColorProfile()
	lipgloss.SetColorProfile(termenv.TrueColor)
	t.Cleanup(func() { lipgloss.SetColorProfile(oldProfile) })

	raw := m.renderFindingsPanel()
	for _, line := range strings.Split(raw, "\n") {
		if strings.Contains(stripStr(line), "step-model-required") {
			// When help is closed, the dim Color("8") code ([90m or 38;5;8) must not appear.
			if strings.Contains(line, "[90m") || strings.Contains(line, "38;5;8") {
				t.Errorf("finding text should NOT be dimmed when help modal is closed; line: %q", line)
			}
		}
	}
}
