package workflowedit

import (
	"fmt"
	"strings"
	"testing"

	"github.com/charmbracelet/lipgloss"
	"github.com/muesli/termenv"
	"github.com/mxriverlynn/pr9k/src/internal/workflowmodel"
)

// TestDetail_BracketGrammar verifies that every field kind uses the [ value ]
// bracket grammar (D26). Checks both opening "[ " and closing " ]" are present.
func TestDetail_BracketGrammar(t *testing.T) {
	step := sampleClaudeStep("my-step")
	m := newLoadedModelWithWidth(100, 30, step)
	m.focus = focusDetail
	m.outline.cursor = 3 // step row in flat outline (0=init hdr, 1=+Add, 2=iter hdr, 3=step)
	view := stripView(m)
	if !strings.Contains(view, "[ ") {
		t.Errorf("detail pane should use bracket grammar '[ value ]'; missing '[ ', view:\n%s", view)
	}
	if !strings.Contains(view, " ]") {
		t.Errorf("detail pane should use bracket grammar '[ value ]'; missing ' ]', view:\n%s", view)
	}
}

// TestDetail_DropdownIndicator verifies that ▾ appears on choice fields (D27/D28).
// The Kind field is always a choice field so it should always show ▾.
func TestDetail_DropdownIndicator(t *testing.T) {
	step := sampleClaudeStep("choice-step")
	m := newLoadedModelWithWidth(100, 30, step)
	m.focus = focusDetail
	m.outline.cursor = 3
	view := stripView(m)
	if !strings.Contains(view, "▾") {
		t.Errorf("choice fields should show ▾ indicator, view:\n%s", view)
	}
}

// TestDetail_LabelTruncation (D50) verifies that long labels are truncated
// rather than causing horizontal overflow in a narrow pane.
func TestDetail_LabelTruncation(t *testing.T) {
	step := workflowmodel.Step{
		Kind:    workflowmodel.StepKindShell,
		Name:    "s",
		Command: []string{"echo"},
		Env: []workflowmodel.EnvEntry{
			{Key: "VERY_LONG_CONTAINERENV_VARIABLE_NAME_EXCEEDING_COLUMN", Value: "v", IsLiteral: true},
		},
	}
	// Narrow pane: outline gets 20 (min), detail gets 30
	m := newLoadedModelWithWidth(50, 30, step)
	m.focus = focusDetail
	m.outline.cursor = 3
	// Must not panic and the view must not overflow beyond the terminal width.
	view := m.View()
	for _, line := range strings.Split(view, "\n") {
		if lipgloss.Width(line) > m.width {
			t.Errorf("line too wide (%d cols), truncation broken: %q", lipgloss.Width(line), line)
		}
	}
	_ = view
}

// TestDetail_DropdownOverflowFlip (D51) verifies that when an open dropdown
// would extend past the pane bottom, the options are still visible in the pane.
//
// Geometry with height=19:
//
//	panelH = 19-8 = 11; detailH = 11-2 = 9; contentH = 9-2 = 7
//	Claude fields (with PromptFile action row): Name(row 0), Kind(1), Model(2),
//	PromptFile(3)+action(4), CaptureAs(5), CaptureMode(6), TimeoutSecs(7),
//	OnTimeout(8), ResumePrevious(9), BreakLoopIfEmpty(row 10), SkipIfCaptureEmpty(11)
//	Total base rows = 12.
//	BreakLoopIfEmpty field index = 9; base row = 10.
//	offset = computeScrollOffset(10, 12, 7) → 10-3=7 clamped to 5 → focusedY = 5
//	5+1+2 = 8 > 7 → dropdown flips ABOVE the field.
func TestDetail_DropdownOverflowFlip(t *testing.T) {
	step := sampleClaudeStep("flip-step")
	// height=19 → contentH=7 for the detail pane.
	m := newLoadedModelWithWidth(100, 19, step)
	m.focus = focusDetail
	m.outline.cursor = 3
	// BreakLoopIfEmpty is field index 9 (0-based, after ResumePrevious).
	m.detail.cursor = 9
	m.detail.dropdownOpen = true
	m.detail.choiceOptions = []string{"false", "true"}
	m.detail.choiceIdx = 0
	view := stripView(m)
	// Options must appear regardless of flip direction.
	if !strings.Contains(view, "false") {
		t.Errorf("dropdown option 'false' should be visible even when flipped, view:\n%s", view)
	}
	if !strings.Contains(view, "true") {
		t.Errorf("dropdown option 'true' should be visible even when flipped, view:\n%s", view)
	}
}

// TestDetail_MultiLineActionRow (D32) verifies that fieldKindMultiLine fields
// show the "↩ Ctrl+E to edit" action row.
func TestDetail_MultiLineActionRow(t *testing.T) {
	step := sampleClaudeStep("multi-step")
	m := newLoadedModelWithWidth(100, 30, step)
	m.focus = focusDetail
	m.outline.cursor = 3
	// PromptFile is at cursor 3 (fieldKindMultiLine).
	// Fields: Name(0), Kind(1), Model(2), PromptFile(3)
	m.detail.cursor = 3
	view := stripView(m)
	if !strings.Contains(view, "↩ Ctrl+E to edit") {
		t.Errorf("multi-line field should show '↩ Ctrl+E to edit' action row, view:\n%s", view)
	}
}

// TestDetail_SecretMaskRendered verifies that secret-mask fields show
// •••••••• instead of the actual value (D27/D28).
func TestDetail_SecretMaskRendered(t *testing.T) {
	step := workflowmodel.Step{
		Kind:    workflowmodel.StepKindShell,
		Name:    "s",
		Command: []string{"echo"},
		Env:     []workflowmodel.EnvEntry{{Key: "MY_API_TOKEN", Value: "secretval", IsLiteral: true}},
	}
	m := newLoadedModelWithWidth(100, 30, step)
	m.focus = focusDetail
	m.outline.cursor = 3
	view := stripView(m)
	if !strings.Contains(view, GlyphMasked) {
		t.Errorf("secret field should show masked glyph %q, view:\n%s", GlyphMasked, view)
	}
	if strings.Contains(view, "secretval") {
		t.Errorf("secret value 'secretval' should not appear when masked, view:\n%s", view)
	}
}

// TestDetail_ScrollIndicator (D33) verifies that scroll indicators (▲ or ▼)
// appear when the field list overflows the pane height.
func TestDetail_ScrollIndicator(t *testing.T) {
	// 12 env vars → many fields; small pane height to force overflow.
	envs := make([]workflowmodel.EnvEntry, 12)
	for i := range envs {
		envs[i] = workflowmodel.EnvEntry{Key: fmt.Sprintf("VAR_%d", i), IsLiteral: false}
	}
	step := workflowmodel.Step{
		Kind:    workflowmodel.StepKindShell,
		Name:    "s",
		Command: []string{"echo"},
		Env:     envs,
	}
	// Height=20 → panelH=12 (12 content rows), but there will be 20+ field rows.
	m := newLoadedModelWithWidth(100, 20, step)
	m.focus = focusDetail
	m.outline.cursor = 3
	// Move cursor to a field that is NOT at the very beginning to force scroll.
	m.detail.cursor = 15
	view := stripView(m)
	if !strings.Contains(view, "▲") && !strings.Contains(view, "▼") {
		t.Errorf("should show ▲ or ▼ scroll indicator when fields overflow, view:\n%s", view)
	}
}

// TestDetail_ReverseVideoOnDropdownHighlight verifies that reverse-video is
// applied only to the highlighted dropdown item, not to the field row or other
// options (D25/D47).
func TestDetail_ReverseVideoOnDropdownHighlight(t *testing.T) {
	step := sampleClaudeStep("rv-step")
	m := newLoadedModelWithWidth(100, 30, step)
	m.focus = focusDetail
	m.outline.cursor = 3
	// Kind field (index 1) is a choice field; open its dropdown.
	m.detail.cursor = 1
	m.detail.dropdownOpen = true
	m.detail.choiceOptions = []string{"claude", "shell"}
	m.detail.choiceIdx = 0 // highlight "claude"

	// Force TrueColor so lipgloss produces ANSI escape codes in the test env.
	oldProfile := lipgloss.ColorProfile()
	lipgloss.SetColorProfile(termenv.TrueColor)
	t.Cleanup(func() { lipgloss.SetColorProfile(oldProfile) })

	raw := m.View()
	lines := strings.Split(raw, "\n")

	// Dropdown option lines contain the option text WITHOUT bracket grammar ("[ ... ]").
	// The field line contains ": [ claude ▾ ]" — distinguish by absence of ": [".
	isDropdownOptionLine := func(stripped, opt string) bool {
		return strings.Contains(stripped, opt) && !strings.Contains(stripped, ": [")
	}

	var highlightLine, plainShellLine string
	for _, line := range lines {
		stripped := stripStr(line)
		if isDropdownOptionLine(stripped, "claude") && highlightLine == "" {
			highlightLine = line
		}
		if isDropdownOptionLine(stripped, "shell") && plainShellLine == "" {
			plainShellLine = line
		}
	}

	if highlightLine == "" {
		t.Fatal("no dropdown option line with 'claude' found in view")
	}
	if plainShellLine == "" {
		t.Fatal("no dropdown option line with 'shell' found in view")
	}

	// The highlighted "claude" option must have reverse-video.
	hasRev := strings.Contains(highlightLine, "\x1b[7m") ||
		strings.Contains(highlightLine, "\x1b[0;7m") ||
		strings.Contains(highlightLine, "\x1b[1;7m")
	if !hasRev {
		t.Errorf("highlighted dropdown item 'claude' should have reverse-video, line: %q", highlightLine)
	}

	// The non-highlighted "shell" option must NOT have reverse-video.
	if strings.Contains(plainShellLine, "\x1b[7m") ||
		strings.Contains(plainShellLine, "\x1b[0;7m") ||
		strings.Contains(plainShellLine, "\x1b[1;7m") {
		t.Errorf("non-highlighted option 'shell' should NOT have reverse-video, line: %q", plainShellLine)
	}
}
