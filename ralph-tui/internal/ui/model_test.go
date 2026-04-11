package ui

import (
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
)

// newTestModel returns a minimal Model for unit tests with a 1-step header.
func newTestModel(t *testing.T) Model {
	t.Helper()
	header := NewStatusHeader(1)
	header.SetPhaseSteps([]string{"step-one"})
	actions := make(chan StepAction, 10)
	kh := NewKeyHandler(func() {}, actions)
	return NewModel(header, kh, "ralph-tui v0.0.0-test")
}

// --- LogLinesMsg batching ---

func TestLogLines_AppendInOrder(t *testing.T) {
	m := newTestModel(t)

	// Send 200 lines in one batch.
	lines := make([]string, 200)
	for i := range 200 {
		lines[i] = strings.Repeat("x", i+1)
	}
	next, _ := m.Update(LogLinesMsg{Lines: lines})
	m = next.(Model)

	got := m.log.lines
	if len(got) != 200 {
		t.Fatalf("expected 200 lines, got %d", len(got))
	}
	for i, want := range lines {
		if got[i] != want {
			t.Errorf("line %d: want %q, got %q", i, want, got[i])
		}
	}
}

func TestLogLines_TrimsToLast500(t *testing.T) {
	m := newTestModel(t)

	// Send 600 lines — ring buffer should retain only the last 500.
	lines := make([]string, 600)
	for i := range 600 {
		lines[i] = strings.Repeat("y", i+1)
	}
	next, _ := m.Update(LogLinesMsg{Lines: lines})
	m = next.(Model)

	if len(m.log.lines) != 500 {
		t.Fatalf("expected 500 lines after trim, got %d", len(m.log.lines))
	}
	// First retained line should be the 101st original line (index 100).
	if m.log.lines[0] != lines[100] {
		t.Errorf("first retained line: want %q, got %q", lines[100], m.log.lines[0])
	}
	// Last retained line should be the last original line (index 599).
	if m.log.lines[499] != lines[599] {
		t.Errorf("last retained line: want %q, got %q", lines[599], m.log.lines[499])
	}
}

// --- Auto-scroll behavior ---

func TestLogLines_AtBottom_StaysAtBottom(t *testing.T) {
	m := newTestModel(t)
	// Size the viewport so we can test scrolling.
	m.width = 80
	m.height = 24
	m.log.SetSize(76, 10)

	// Fill past viewport height and scroll to bottom.
	fill := make([]string, 20)
	for i := range fill {
		fill[i] = "line"
	}
	next, _ := m.Update(LogLinesMsg{Lines: fill})
	m = next.(Model)
	m.log.viewport.GotoBottom()

	// Send more lines while at bottom.
	more := []string{"extra1", "extra2"}
	next, _ = m.Update(LogLinesMsg{Lines: more})
	m = next.(Model)

	if !m.log.viewport.AtBottom() {
		t.Error("expected viewport to stay at bottom after new lines")
	}
}

func TestLogLines_ScrolledUp_DoesNotAutoScroll(t *testing.T) {
	m := newTestModel(t)
	m.log.SetSize(76, 5)

	// Fill the buffer.
	fill := make([]string, 20)
	for i := range fill {
		fill[i] = "line"
	}
	next, _ := m.Update(LogLinesMsg{Lines: fill})
	m = next.(Model)

	// Scroll to top to simulate the user scrolling up.
	m.log.viewport.GotoTop()
	positionBefore := m.log.viewport.YOffset

	// Send more lines.
	more := []string{"new1", "new2"}
	next, _ = m.Update(LogLinesMsg{Lines: more})
	m = next.(Model)

	positionAfter := m.log.viewport.YOffset
	if positionAfter != positionBefore {
		t.Errorf("expected viewport position unchanged when scrolled up: before=%d after=%d", positionBefore, positionAfter)
	}
}

// --- Normal-mode 'n' key routing ---

func TestNormalMode_N_ReturnsCancelCmd(t *testing.T) {
	cancelCalled := false
	actions := make(chan StepAction, 10)
	kh := NewKeyHandler(func() { cancelCalled = true }, actions)
	header := NewStatusHeader(1)
	header.SetPhaseSteps([]string{"s"})
	m := NewModel(header, kh, "v0")

	// Press 'n' in normal mode.
	_, cmd := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("n")})

	if cmd == nil {
		t.Fatal("expected non-nil cmd for n in normal mode")
	}
	// Execute the command — it should call cancel.
	_ = cmd()
	if !cancelCalled {
		t.Error("expected cancel to be called after cmd execution")
	}
}

// --- ModeQuitting silently eats all keys ---

func TestModeQuitting_AllKeys_NoActions_NoCancel(t *testing.T) {
	cancelCount := 0
	actions := make(chan StepAction, 10)
	kh := NewKeyHandler(func() { cancelCount++ }, actions)
	header := NewStatusHeader(1)
	header.SetPhaseSteps([]string{"s"})
	m := NewModel(header, kh, "v0")

	// Trigger quit.
	kh.ForceQuit()
	<-actions       // drain the ActionQuit
	cancelCount = 0 // reset

	for _, k := range []string{"q", "y", "n", "c", "r", "x"} {
		next, cmd := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(k)})
		m = next.(Model)
		if cmd != nil {
			_ = cmd() // execute — must not call cancel or send actions
		}
	}

	if len(actions) != 0 {
		t.Errorf("expected no new actions in ModeQuitting, got %d", len(actions))
	}
	if cancelCount != 0 {
		t.Errorf("expected cancel not called during ModeQuitting, got %d", cancelCount)
	}
}

// --- Title assembly ---

func TestTitleString_EmptyIterationLine(t *testing.T) {
	m := newTestModel(t)
	m.header.header.IterationLine = ""
	m.header.iterationLine = ""

	got := m.titleString()
	if got != "Power-Ralph.9000" {
		t.Errorf("want %q, got %q", "Power-Ralph.9000", got)
	}
}

func TestTitleString_PopulatedIterationLine(t *testing.T) {
	m := newTestModel(t)
	m.header.header.IterationLine = "Iteration 2/5 — Issue #42"
	m.header.iterationLine = m.header.header.IterationLine

	got := m.titleString()
	want := "Power-Ralph.9000 — Iteration 2/5 — Issue #42"
	if got != want {
		t.Errorf("want %q, got %q", want, got)
	}
}

// --- renderTopBorder ---

func TestRenderTopBorder_TitleFits(t *testing.T) {
	m := newTestModel(t)
	m.width = 80
	m.header.header.IterationLine = "Iteration 1/3: step"
	m.header.iterationLine = m.header.header.IterationLine

	got := m.renderTopBorder(m.titleString())
	if !strings.Contains(got, "Power-Ralph.9000") {
		t.Errorf("expected title in border, got: %q", got)
	}
	// Must start with ╭ and end with ╮ (after stripping ANSI).
	plain := stripANSI(got)
	if !strings.HasPrefix(plain, "╭") {
		t.Errorf("expected ╭ prefix, got: %q", plain)
	}
	if !strings.HasSuffix(plain, "╮") {
		t.Errorf("expected ╮ suffix, got: %q", plain)
	}
}

func TestRenderTopBorder_VeryNarrow_PlainRule(t *testing.T) {
	m := newTestModel(t)
	m.width = 4
	got := m.renderTopBorder("ralph-tui")
	plain := stripANSI(got)
	if strings.Contains(plain, "ralph-tui") {
		t.Errorf("expected plain rule for very narrow terminal, got title: %q", plain)
	}
	if !strings.HasPrefix(plain, "╭") {
		t.Errorf("expected plain rule starting with ╭, got: %q", plain)
	}
}

func TestRenderTopBorder_ZeroWidth_NocrashPlainRule(t *testing.T) {
	m := newTestModel(t)
	m.width = 0
	// Must not panic.
	got := m.renderTopBorder("ralph-tui")
	plain := stripANSI(got)
	if strings.Contains(plain, "ralph-tui") {
		t.Errorf("expected plain rule for width=0, got title: %q", plain)
	}
}

func TestRenderTopBorder_TitleOverflows_Truncated(t *testing.T) {
	m := newTestModel(t)
	m.width = 20
	longTitle := "ralph-tui — this is a very very long title that will overflow"
	got := m.renderTopBorder(longTitle)
	plain := stripANSI(got)
	// The rendered string should fit within m.width runes.
	// lipgloss.Width accounts for multi-byte chars correctly; use rune count here.
	if len([]rune(plain)) > m.width+3 { // some slack for ANSI reset sequences missed by strip
		t.Errorf("expected truncated border within width=%d, got len=%d: %q", m.width, len([]rune(plain)), plain)
	}
}

// --- tea.WindowSizeMsg ---

func TestWindowSizeMsg_SetsWidthAndHeight(t *testing.T) {
	m := newTestModel(t)

	next, _ := m.Update(tea.WindowSizeMsg{Width: 120, Height: 40})
	m = next.(Model)

	if m.width != 120 {
		t.Errorf("width: want 120, got %d", m.width)
	}
	if m.height != 40 {
		t.Errorf("height: want 40, got %d", m.height)
	}
}

// --- TP-013: Init() ---

func TestModel_Init_ReturnsNil(t *testing.T) {
	m := newTestModel(t)
	if m.Init() != nil {
		t.Error("expected Init() to return nil")
	}
}

// --- TP-011: tea.QuitMsg ---

func TestModel_Update_QuitMsg_ReturnsQuitCmd(t *testing.T) {
	m := newTestModel(t)

	_, cmd := m.Update(tea.QuitMsg{})
	if cmd == nil {
		t.Fatal("expected non-nil cmd for QuitMsg")
	}
	// tea.Quit returns tea.QuitMsg when executed — check it returns a tea.Msg.
	result := cmd()
	if _, ok := result.(tea.QuitMsg); !ok {
		t.Errorf("expected cmd() to return tea.QuitMsg, got %T", result)
	}
}

// --- TP-005: header message routing ---

func TestModel_Update_HeaderStepStateMsg_MutatesHeader(t *testing.T) {
	m := newTestModel(t)

	next, _ := m.Update(headerStepStateMsg{idx: 0, state: StepDone})
	m = next.(Model)

	if m.header.header.Rows[0][0] != "[✓] step-one" {
		t.Errorf("expected step-one marked done, got %q", m.header.header.Rows[0][0])
	}
}

func TestModel_Update_HeaderPhaseStepsMsg_MutatesHeader(t *testing.T) {
	m := newTestModel(t)

	next, _ := m.Update(headerPhaseStepsMsg{names: []string{"alpha", "beta"}})
	m = next.(Model)

	if m.header.header.Rows[0][0] != "[ ] alpha" {
		t.Errorf("expected '[ ] alpha', got %q", m.header.header.Rows[0][0])
	}
	if m.header.header.Rows[0][1] != "[ ] beta" {
		t.Errorf("expected '[ ] beta', got %q", m.header.header.Rows[0][1])
	}
}

func TestModel_Update_HeaderIterationLineMsg_UpdatesIterationLine(t *testing.T) {
	m := newTestModel(t)

	next, cmd := m.Update(headerIterationLineMsg{iter: 2, max: 5, issue: "42"})
	m = next.(Model)

	want := "Iteration 2/5 — Issue #42"
	if m.header.header.IterationLine != want {
		t.Errorf("IterationLine: want %q, got %q", want, m.header.header.IterationLine)
	}
	// A title cmd must be returned when the line changes.
	if cmd == nil {
		t.Error("expected non-nil cmd (SetWindowTitle) when iteration line changes")
	}
}

func TestModel_Update_HeaderIterationLineMsg_NoTitleCmd_WhenUnchanged(t *testing.T) {
	m := newTestModel(t)
	// Apply once to set the line.
	m.header = m.header.apply(headerIterationLineMsg{iter: 1, max: 3, issue: "7"})

	// Apply the same message again — line is unchanged, so no cmd expected.
	_, cmd := m.Update(headerIterationLineMsg{iter: 1, max: 3, issue: "7"})
	if cmd != nil {
		// The batch cmd may be non-nil but contain only nils; execute to check.
		result := cmd()
		if result != nil {
			t.Errorf("expected no meaningful cmd when iteration line unchanged, got: %T", result)
		}
	}
}

func TestModel_Update_HeaderInitializeLineMsg_UpdatesIterationLine(t *testing.T) {
	m := newTestModel(t)

	next, cmd := m.Update(headerInitializeLineMsg{stepNum: 1, stepCount: 2, stepName: "Setup"})
	m = next.(Model)

	want := "Initializing 1/2: Setup"
	if m.header.header.IterationLine != want {
		t.Errorf("IterationLine: want %q, got %q", want, m.header.header.IterationLine)
	}
	if cmd == nil {
		t.Error("expected non-nil cmd (SetWindowTitle) when initialize line changes")
	}
}

func TestModel_Update_HeaderFinalizeLineMsg_UpdatesIterationLine(t *testing.T) {
	m := newTestModel(t)

	next, cmd := m.Update(headerFinalizeLineMsg{stepNum: 3, stepCount: 3, stepName: "Push"})
	m = next.(Model)

	want := "Finalizing 3/3: Push"
	if m.header.header.IterationLine != want {
		t.Errorf("IterationLine: want %q, got %q", want, m.header.header.IterationLine)
	}
	if cmd == nil {
		t.Error("expected non-nil cmd (SetWindowTitle) when finalize line changes")
	}
}

// --- TP-006: viewport clamping to minimum 1 ---

func TestWindowSizeMsg_VerySmall_ViewportClampsToOne(t *testing.T) {
	m := newTestModel(t)

	// width=1, height=1 is extreme: vpHeight and vpWidth must both clamp to 1.
	next, _ := m.Update(tea.WindowSizeMsg{Width: 1, Height: 1})
	m = next.(Model)

	if m.log.viewport.Width < 1 {
		t.Errorf("vpWidth clamped below 1: got %d", m.log.viewport.Width)
	}
	if m.log.viewport.Height < 1 {
		t.Errorf("vpHeight clamped below 1: got %d", m.log.viewport.Height)
	}
}

func TestWindowSizeMsg_Width3_VpWidthClampsToOne(t *testing.T) {
	m := newTestModel(t)

	// width=3: inside border subtracts 2, leaving vpWidth=1.
	next, _ := m.Update(tea.WindowSizeMsg{Width: 3, Height: 40})
	m = next.(Model)

	if m.log.viewport.Width < 1 {
		t.Errorf("vpWidth clamped below 1 for width=3: got %d", m.log.viewport.Width)
	}
}

// --- TP-009: titleString via headerIterationLineMsg ---

func TestTitleString_AfterIterationLineMsg(t *testing.T) {
	m := newTestModel(t)

	next, _ := m.Update(headerIterationLineMsg{iter: 3, max: 10, issue: "99"})
	m = next.(Model)

	want := "Power-Ralph.9000 — Iteration 3/10 — Issue #99"
	if m.titleString() != want {
		t.Errorf("titleString: want %q, got %q", want, m.titleString())
	}
}

// --- WARN-004: Model.View() smoke test ---

func TestView_NonEmpty_ContainsVersionAndStepName(t *testing.T) {
	header := NewStatusHeader(1)
	header.SetPhaseSteps([]string{"Feature work"})
	actions := make(chan StepAction, 10)
	kh := NewKeyHandler(func() {}, actions)
	m := NewModel(header, kh, "ralph-tui v0.2.0")
	m.width = 80
	m.height = 24
	m.log.SetSize(76, 10)

	out := m.View()

	if out == "" {
		t.Fatal("View() returned empty string")
	}
	plain := stripANSI(out)
	if !strings.Contains(plain, "ralph-tui v0.2.0") {
		t.Errorf("View() output missing version label; got:\n%s", plain)
	}
	if !strings.Contains(plain, "Feature work") {
		t.Errorf("View() output missing step name; got:\n%s", plain)
	}
}

// --- WARN-005: Model.View() panic-safety with zero dimensions ---

func TestView_ZeroDimensions_NoPanic(t *testing.T) {
	m := newTestModel(t)

	// Must not panic with zero width/height.
	next, _ := m.Update(tea.WindowSizeMsg{Width: 0, Height: 0})
	m = next.(Model)

	defer func() {
		if r := recover(); r != nil {
			t.Errorf("View() panicked with zero dimensions: %v", r)
		}
	}()
	_ = m.View()
}

// stripANSI removes ANSI escape sequences from s for plain-text comparisons.
func stripANSI(s string) string {
	var out strings.Builder
	i := 0
	for i < len(s) {
		if s[i] == '\x1b' && i+1 < len(s) && s[i+1] == '[' {
			// Skip until 'm'
			for i < len(s) && s[i] != 'm' {
				i++
			}
			if i < len(s) {
				i++ // skip the 'm'
			}
			continue
		}
		out.WriteByte(s[i])
		i++
	}
	return out.String()
}
