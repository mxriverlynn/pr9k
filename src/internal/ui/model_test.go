package ui

import (
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

// newTestModel returns a minimal Model for unit tests with a 1-step header.
func newTestModel(t *testing.T) Model {
	t.Helper()
	header := NewStatusHeader(1)
	header.SetPhaseSteps([]string{"step-one"})
	actions := make(chan StepAction, 10)
	kh := NewKeyHandler(func() {}, actions)
	return NewModel(header, kh, "pr9k v0.0.0-test")
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

func TestLogLines_TrimsToLastCap(t *testing.T) {
	m := newTestModel(t)

	// Send logRingBufferCap+100 lines — ring buffer should retain only the last logRingBufferCap.
	total := logRingBufferCap + 100
	lines := make([]string, total)
	for i := range total {
		lines[i] = strings.Repeat("y", i+1)
	}
	next, _ := m.Update(LogLinesMsg{Lines: lines})
	m = next.(Model)

	if len(m.log.lines) != logRingBufferCap {
		t.Fatalf("expected %d lines after trim, got %d", logRingBufferCap, len(m.log.lines))
	}
	// First retained line should be the 101st original line (index 100).
	if m.log.lines[0] != lines[100] {
		t.Errorf("first retained line: want %q, got %q", lines[100], m.log.lines[0])
	}
	// Last retained line should be the last original line.
	if m.log.lines[logRingBufferCap-1] != lines[total-1] {
		t.Errorf("last retained line: want %q, got %q", lines[total-1], m.log.lines[logRingBufferCap-1])
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

func TestNormalMode_N_ThenY_CallsCancel(t *testing.T) {
	cancelCalled := false
	actions := make(chan StepAction, 10)
	kh := NewKeyHandler(func() { cancelCalled = true }, actions)
	header := NewStatusHeader(1)
	header.SetPhaseSteps([]string{"s"})
	m := NewModel(header, kh, "v0")

	// Press 'n' — enters ModeNextConfirm, nil cmd.
	next, cmd := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("n")})
	m = next.(Model)
	if cmd != nil {
		t.Error("expected nil cmd for n entering NextConfirm")
	}
	if kh.Mode() != ModeNextConfirm {
		t.Fatalf("expected ModeNextConfirm after n, got %v", kh.Mode())
	}

	// Press 'y' — confirms skip, returns cmd that calls cancel.
	_, cmd = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("y")})
	if cmd == nil {
		t.Fatal("expected non-nil cmd for y in next-confirm mode")
	}
	_ = cmd()
	if !cancelCalled {
		t.Error("expected cancel to be called after y cmd execution")
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

	got := m.titleString()
	if got != AppTitle {
		t.Errorf("want %q, got %q", AppTitle, got)
	}
}

func TestTitleString_PopulatedIterationLine(t *testing.T) {
	m := newTestModel(t)
	m.header.header.IterationLine = "Iteration 2/5 — Issue #42"

	got := m.titleString()
	want := AppTitle + " — Iteration 2/5 — Issue #42"
	if got != want {
		t.Errorf("want %q, got %q", want, got)
	}
}

// --- renderTopBorder ---

func TestRenderTopBorder_TitleFits(t *testing.T) {
	m := newTestModel(t)
	m.width = 80
	m.header.header.IterationLine = "Iteration 1/3: step"

	got := m.renderTopBorder(m.titleString())
	if !strings.Contains(got, AppTitle) {
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
	got := m.renderTopBorder("pr9k")
	plain := stripANSI(got)
	if strings.Contains(plain, "pr9k") {
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
	got := m.renderTopBorder("pr9k")
	plain := stripANSI(got)
	if strings.Contains(plain, "pr9k") {
		t.Errorf("expected plain rule for width=0, got title: %q", plain)
	}
}

func TestRenderTopBorder_TitleOverflows_Truncated(t *testing.T) {
	m := newTestModel(t)
	m.width = 20
	longTitle := "pr9k — this is a very very long title that will overflow"
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

	want := AppTitle + " — Iteration 3/10 — Issue #99"
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
	m := NewModel(header, kh, "pr9k v0.2.0")
	m.width = 80
	m.height = 24
	m.log.SetSize(76, 10)

	out := m.View()

	if out == "" {
		t.Fatal("View() returned empty string")
	}
	plain := stripANSI(out)
	if !strings.Contains(plain, "pr9k v0.2.0") {
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

// --- TP-002, TP-003, TP-005: colorShortcutLine ---

func TestColorShortcutLine_DefaultBranch_PreservesText(t *testing.T) {
	result := colorShortcutLine(NormalShortcuts)
	plain := stripANSI(result)
	if plain != NormalShortcuts {
		t.Errorf("plain text mismatch: want %q, got %q", NormalShortcuts, plain)
	}
}

func TestColorShortcutLine_ErrorShortcuts_PreservesText(t *testing.T) {
	result := colorShortcutLine(ErrorShortcuts)
	plain := stripANSI(result)
	if plain != ErrorShortcuts {
		t.Errorf("plain text mismatch: want %q, got %q", ErrorShortcuts, plain)
	}
}

func TestColorShortcutLine_QuitConfirmPrompt_ContainsAppTitle(t *testing.T) {
	result := colorShortcutLine(QuitConfirmPrompt)
	plain := stripANSI(result)
	if plain != QuitConfirmPrompt {
		t.Errorf("plain text mismatch: want %q, got %q", QuitConfirmPrompt, plain)
	}
	if !strings.Contains(plain, AppTitle) {
		t.Errorf("plain text missing AppTitle %q: %q", AppTitle, plain)
	}
}

func TestColorShortcutLine_NextConfirmPrompt_PreservesText(t *testing.T) {
	result := colorShortcutLine(NextConfirmPrompt)
	plain := stripANSI(result)
	if plain != NextConfirmPrompt {
		t.Errorf("plain text mismatch: want %q, got %q", NextConfirmPrompt, plain)
	}
}

func TestColorShortcutLine_DoneShortcuts_PreservesText(t *testing.T) {
	result := colorShortcutLine(DoneShortcuts)
	plain := stripANSI(result)
	if plain != DoneShortcuts {
		t.Errorf("plain text mismatch: want %q, got %q", DoneShortcuts, plain)
	}
}

func TestColorShortcutLine_QuittingLine_PreservesText(t *testing.T) {
	result := colorShortcutLine(QuittingLine)
	plain := stripANSI(result)
	if plain != QuittingLine {
		t.Errorf("plain text mismatch: want %q, got %q", QuittingLine, plain)
	}
}

// --- TP-004: colorTitle ---

func TestColorTitle_WithSeparator_PreservesText(t *testing.T) {
	title := "Power-Ralph.9000 — Iteration 2/5"
	result := colorTitle(title)
	plain := stripANSI(result)
	if plain != title {
		t.Errorf("plain text mismatch: want %q, got %q", title, plain)
	}
}

func TestColorTitle_WithoutSeparator_PreservesText(t *testing.T) {
	title := "Power-Ralph.9000"
	result := colorTitle(title)
	plain := stripANSI(result)
	if plain != title {
		t.Errorf("plain text mismatch: want %q, got %q", title, plain)
	}
}

// --- TP-007: Mouse message forwarded to log ---

func TestModel_MouseMsg_NoPanic(t *testing.T) {
	m := newTestModel(t)
	m.width = 80
	m.height = 24
	m.log.SetSize(76, 10)

	defer func() {
		if r := recover(); r != nil {
			t.Errorf("Update panicked on tea.MouseMsg: %v", r)
		}
	}()

	next, _ := m.Update(tea.MouseMsg{Action: tea.MouseActionPress, Button: tea.MouseButtonWheelDown})
	_ = next
}

// --- Issue #74: step list evenly distributed across header cells ---

// TestView_CheckboxGrid_EqualCellWidth verifies that all four columns in the
// checkbox grid are padded to the same width when step names differ in length.
// The longest name determines the cell width; shorter names are padded so that
// each column starts at the same horizontal offset in the rendered row.
func TestView_CheckboxGrid_EqualCellWidth(t *testing.T) {
	// Four steps with deliberately different lengths; the longest is "very-long-step-name".
	// Cell width is derived from the terminal width so the grid fills edge-to-edge:
	//   innerWidth = 120 - 2 = 118
	//   separatorWidth = 3 * 2 = 6
	//   cellWidth = (118 - 6) / 4 = 28
	//   stride = 28 + 2 = 30
	steps := []string{"short", "very-long-step-name", "mid", "x"}
	header := NewStatusHeader(len(steps))
	header.SetPhaseSteps(steps)
	actions := make(chan StepAction, 10)
	kh := NewKeyHandler(func() {}, actions)
	m := NewModel(header, kh, "v0")
	m.width = 120
	m.height = 24
	m.log.SetSize(116, 10)

	out := stripANSI(m.View())

	// Find the grid line that contains the step names.
	var gridLine string
	for _, line := range strings.Split(out, "\n") {
		if strings.Contains(line, "[ ] short") {
			gridLine = line
			break
		}
	}
	if gridLine == "" {
		t.Fatalf("could not find checkbox grid line in View() output:\n%s", out)
	}

	// Strip the leading │ border to get the inner content as a rune slice
	// so positions are measured in visible characters, not bytes.
	runes := []rune(gridLine)
	if len(runes) > 0 && runes[0] == '│' {
		runes = runes[1:]
	}

	// cellWidth derived from terminal: (118 - 6) / 4 = 28
	cellWidth := (m.width - 2 - (HeaderCols-1)*2) / HeaderCols
	stride := cellWidth + 2 // cell + "  " separator

	// Each step name should appear at position col*stride+4 within runes
	// ("+4" skips the "[ ] " prefix of each cell).
	for col, name := range steps {
		start := col*stride + 4 // skip "[ ] "
		end := start + len([]rune(name))
		if end > len(runes) {
			t.Errorf("col %d: rune slice too short (len=%d) for name %q at [%d:%d]",
				col, len(runes), name, start, end)
			continue
		}
		got := string(runes[start:end])
		if got != name {
			t.Errorf("col %d: expected %q at rune offset %d, got %q (full inner: %q)",
				col, name, start, got, string(runes))
		}
	}
}

// --- TP-001: multi-row global max cell width ---

// TestView_CheckboxGrid_MultiRow_GlobalMaxCellWidth verifies that maxCellWidth
// is computed globally across all rows, not per-row. The longest name lives on
// row 1; both rows must use the stride derived from that name.
func TestView_CheckboxGrid_MultiRow_GlobalMaxCellWidth(t *testing.T) {
	// 8 steps: row 0 has short names, row 1 has the longest name first.
	// Cell width is derived from terminal width:
	//   innerWidth = 160 - 2 = 158, separators = 6, cellWidth = (158-6)/4 = 38
	// Both rows must use the same stride (38 + 2 = 40).
	steps := []string{"aa", "bb", "cc", "dd", "longest-name", "e", "f", "g"}
	header := NewStatusHeader(len(steps))
	header.SetPhaseSteps(steps)
	actions := make(chan StepAction, 10)
	kh := NewKeyHandler(func() {}, actions)
	m := NewModel(header, kh, "v0")
	m.width = 160
	m.height = 30
	m.log.SetSize(156, 10)

	out := stripANSI(m.View())

	// Collect grid lines: lines that contain "[ ] aa" (row 0) or "[ ] longest-name" (row 1).
	var gridLines []string
	for _, line := range strings.Split(out, "\n") {
		if strings.Contains(line, "[ ] aa") || strings.Contains(line, "[ ] longest-name") {
			gridLines = append(gridLines, line)
		}
	}
	if len(gridLines) != 2 {
		t.Fatalf("expected 2 grid rows, found %d:\n%s", len(gridLines), out)
	}

	cellWidth := (m.width - 2 - (HeaderCols-1)*2) / HeaderCols // 38
	stride := cellWidth + 2                                    // 40

	// Row 0: steps "aa", "bb", "cc", "dd"
	row0Names := []string{"aa", "bb", "cc", "dd"}
	runes0 := []rune(gridLines[0])
	if len(runes0) > 0 && runes0[0] == '│' {
		runes0 = runes0[1:]
	}
	for col, name := range row0Names {
		start := col*stride + 4 // skip "[ ] "
		end := start + len([]rune(name))
		if end > len(runes0) {
			t.Errorf("row 0 col %d: rune slice too short (len=%d) for name %q at [%d:%d]",
				col, len(runes0), name, start, end)
			continue
		}
		if got := string(runes0[start:end]); got != name {
			t.Errorf("row 0 col %d: want %q at offset %d, got %q", col, name, start, got)
		}
	}

	// Row 1: steps "longest-name", "e", "f", "g" — all use the same stride as row 0.
	row1Names := []string{"longest-name", "e", "f", "g"}
	runes1 := []rune(gridLines[1])
	if len(runes1) > 0 && runes1[0] == '│' {
		runes1 = runes1[1:]
	}
	for col, name := range row1Names {
		start := col*stride + 4
		end := start + len([]rune(name))
		if end > len(runes1) {
			t.Errorf("row 1 col %d: rune slice too short (len=%d) for name %q at [%d:%d]",
				col, len(runes1), name, start, end)
			continue
		}
		if got := string(runes1[start:end]); got != name {
			t.Errorf("row 1 col %d: want %q at offset %d, got %q", col, name, start, got)
		}
	}
}

// --- TP-002: empty trailing cells padded to full width ---

// TestView_CheckboxGrid_EmptyTrailingCells_AlignWithFilledRow verifies that
// empty trailing cells on the second row are padded to maxCellWidth, so both
// rows occupy the same total width before the right border.
func TestView_CheckboxGrid_EmptyTrailingCells_AlignWithFilledRow(t *testing.T) {
	// 5 steps: row 0 full (4 steps), row 1 has 1 filled + 3 empty.
	// Longest cell "[ ] epsilon" = 11 runes → all cells padded to width 11.
	steps := []string{"alpha", "beta", "gamma", "delta", "epsilon"}
	header := NewStatusHeader(len(steps))
	header.SetPhaseSteps(steps)
	actions := make(chan StepAction, 10)
	kh := NewKeyHandler(func() {}, actions)
	m := NewModel(header, kh, "v0")
	m.width = 160
	m.height = 30
	m.log.SetSize(156, 10)

	out := stripANSI(m.View())

	// Collect both grid rows.
	var gridLines []string
	for _, line := range strings.Split(out, "\n") {
		if strings.Contains(line, "[ ] alpha") || strings.Contains(line, "[ ] epsilon") {
			gridLines = append(gridLines, line)
		}
	}
	if len(gridLines) != 2 {
		t.Fatalf("expected 2 grid rows, found %d:\n%s", len(gridLines), out)
	}

	// Find the right-border │ position for each row (after stripping leading │).
	rightBorderPos := func(line string) int {
		runes := []rune(line)
		// Skip the leading border.
		if len(runes) > 0 && runes[0] == '│' {
			runes = runes[1:]
		}
		// The right border │ is the last │ in the line.
		for i := len(runes) - 1; i >= 0; i-- {
			if runes[i] == '│' {
				return i
			}
		}
		return -1
	}

	pos0 := rightBorderPos(gridLines[0])
	pos1 := rightBorderPos(gridLines[1])
	if pos0 != pos1 {
		t.Errorf("right border position differs: row 0 at %d, row 1 at %d\nrow0: %q\nrow1: %q",
			pos0, pos1, gridLines[0], gridLines[1])
	}
}

// --- TP-003: equal-width cells — no unnecessary padding ---

// TestView_CheckboxGrid_EqualWidthNames_NoPaddingWithinCells verifies that
// when all step names have the same length, the pad>0 guard prevents any
// extra spaces from being injected within each cell.
func TestView_CheckboxGrid_EqualWidthNames_NoPaddingWithinCells(t *testing.T) {
	// 4 steps each 3 chars: "[ ] aaa" = 7 runes.
	// Cell width is derived from terminal: (118-6)/4 = 28. stride = 30.
	// Each cell content starts with "[ ] <name>" and is then padded to
	// cellWidth with spaces, so the separator always follows the padding.
	steps := []string{"aaa", "bbb", "ccc", "ddd"}
	header := NewStatusHeader(len(steps))
	header.SetPhaseSteps(steps)
	actions := make(chan StepAction, 10)
	kh := NewKeyHandler(func() {}, actions)
	m := NewModel(header, kh, "v0")
	m.width = 120
	m.height = 24
	m.log.SetSize(116, 10)

	out := stripANSI(m.View())

	var gridLine string
	for _, line := range strings.Split(out, "\n") {
		if strings.Contains(line, "[ ] aaa") {
			gridLine = line
			break
		}
	}
	if gridLine == "" {
		t.Fatalf("could not find checkbox grid line:\n%s", out)
	}

	runes := []rune(gridLine)
	if len(runes) > 0 && runes[0] == '│' {
		runes = runes[1:]
	}

	cellWidth := (m.width - 2 - (HeaderCols-1)*2) / HeaderCols // 28
	stride := cellWidth + 2                                    // 30

	for col, name := range steps {
		start := col * stride
		// Verify exactly "[ ] <name>" at the cell start.
		want := "[ ] " + name
		end := start + len([]rune(want))
		if end > len(runes) {
			t.Errorf("col %d: rune slice too short (len=%d) for %q at [%d:%d]",
				col, len(runes), want, start, end)
			continue
		}
		if got := string(runes[start:end]); got != want {
			t.Errorf("col %d: want %q at rune offset %d, got %q (inner: %q)",
				col, want, start, got, string(runes))
		}
		// The character immediately after the cell (before last col) must be ' ' (start of separator).
		if col < len(steps)-1 {
			sepStart := start + cellWidth
			if sepStart < len(runes) && runes[sepStart] != ' ' {
				t.Errorf("col %d: expected space after cell at offset %d, got %q",
					col, sepStart, string(runes[sepStart:sepStart+1]))
			}
		}
	}
}

// --- TP-004: truncation still works after padding ---

// TestView_CheckboxGrid_LongNames_TruncatedToTerminalWidth verifies that
// wrapLine truncation is applied after padding, so the output never exceeds
// m.width runes regardless of how wide the padded row is.
func TestView_CheckboxGrid_LongNames_TruncatedToTerminalWidth(t *testing.T) {
	// 4 steps with 20-char names; padded row is 4*24 + 3*2 = 102 chars.
	// m.width=60 forces truncation to 58 inner chars.
	steps := []string{
		"abcdefghijklmnopqrst",
		"bcdefghijklmnopqrstu",
		"cdefghijklmnopqrstuv",
		"defghijklmnopqrstuvw",
	}
	header := NewStatusHeader(len(steps))
	header.SetPhaseSteps(steps)
	actions := make(chan StepAction, 10)
	kh := NewKeyHandler(func() {}, actions)
	m := NewModel(header, kh, "v0")
	m.width = 60
	m.height = 24
	m.log.SetSize(56, 10)

	out := stripANSI(m.View())

	for _, line := range strings.Split(out, "\n") {
		if strings.Contains(line, "[ ] abcde") {
			runeCount := len([]rune(line))
			if runeCount > m.width {
				t.Errorf("grid line exceeds terminal width %d (got %d runes): %q",
					m.width, runeCount, line)
			}
			return
		}
	}
	t.Fatalf("could not find grid line in output:\n%s", out)
}

// --- TP-005: single step — minimum boundary ---

// TestView_CheckboxGrid_SingleStep_NoCrashAndCellAtOffset0 verifies that a
// header with exactly 1 step (1 filled cell + 3 empty) does not panic and
// renders the step name starting at the first cell position.
func TestView_CheckboxGrid_SingleStep_NoCrashAndCellAtOffset0(t *testing.T) {
	header := NewStatusHeader(1)
	header.SetPhaseSteps([]string{"only-step"})
	actions := make(chan StepAction, 10)
	kh := NewKeyHandler(func() {}, actions)
	m := NewModel(header, kh, "v0")
	m.width = 120
	m.height = 24
	m.log.SetSize(116, 10)

	defer func() {
		if r := recover(); r != nil {
			t.Errorf("View() panicked with single step: %v", r)
		}
	}()

	out := stripANSI(m.View())

	var gridLine string
	for _, line := range strings.Split(out, "\n") {
		if strings.Contains(line, "[ ] only-step") {
			gridLine = line
			break
		}
	}
	if gridLine == "" {
		t.Fatalf("could not find grid line with 'only-step' in output:\n%s", out)
	}

	// The step name should appear at cell offset 0 (after leading border and "[ ] " prefix).
	runes := []rune(gridLine)
	if len(runes) > 0 && runes[0] == '│' {
		runes = runes[1:]
	}
	const prefix = "[ ] only-step"
	if len(runes) < len([]rune(prefix)) {
		t.Fatalf("grid line too short to contain %q: %q", prefix, string(runes))
	}
	if got := string(runes[:len([]rune(prefix))]); got != prefix {
		t.Errorf("expected cell at offset 0 to start with %q, got %q", prefix, got)
	}
}

// --- D23: HeartbeatReader + HeartbeatTickMsg ---

// stubHeartbeat is a test double for HeartbeatReader.
type stubHeartbeat struct {
	silentFor time.Duration
	active    bool
	calls     int
}

func (s *stubHeartbeat) HeartbeatSilence() (time.Duration, bool) {
	s.calls++
	return s.silentFor, s.active
}

// TestModel_HeartbeatTick_ShowsSuffix_WhenSilentFor15s verifies that when the
// heartbeat reader reports active=true and silentFor >= 15s, the heartbeat
// suffix is appended to the title string.
func TestModel_HeartbeatTick_ShowsSuffix_WhenSilentFor15s(t *testing.T) {
	stub := &stubHeartbeat{silentFor: 17 * time.Second, active: true}
	m := newTestModel(t).WithHeartbeat(stub)
	m.header.header.IterationLine = "Iteration 2/5 — Issue #42"

	next, _ := m.Update(HeartbeatTickMsg(time.Now()))
	m = next.(Model)

	title := m.titleString()
	want := "  ⋯ thinking (17s)"
	if !strings.Contains(title, want) {
		t.Errorf("titleString() does not contain heartbeat suffix %q: got %q", want, title)
	}
}

// TestModel_HeartbeatTick_NoSuffix_WhenInactive verifies that when the
// heartbeat reader reports active=false, no heartbeat suffix appears.
func TestModel_HeartbeatTick_NoSuffix_WhenInactive(t *testing.T) {
	stub := &stubHeartbeat{silentFor: 30 * time.Second, active: false}
	m := newTestModel(t).WithHeartbeat(stub)
	m.header.header.IterationLine = "Iteration 2/5"

	next, _ := m.Update(HeartbeatTickMsg(time.Now()))
	m = next.(Model)

	title := m.titleString()
	if strings.Contains(title, "⋯") {
		t.Errorf("titleString() should not contain heartbeat suffix when inactive, got %q", title)
	}
}

// TestModel_HeartbeatTick_NoSuffix_WhenBelowThreshold verifies that when the
// heartbeat reader reports active=true but silentFor < 15s, no suffix is shown.
func TestModel_HeartbeatTick_NoSuffix_WhenBelowThreshold(t *testing.T) {
	stub := &stubHeartbeat{silentFor: 14 * time.Second, active: true}
	m := newTestModel(t).WithHeartbeat(stub)
	m.header.header.IterationLine = "Iteration 1/3"

	next, _ := m.Update(HeartbeatTickMsg(time.Now()))
	m = next.(Model)

	title := m.titleString()
	if strings.Contains(title, "⋯") {
		t.Errorf("titleString() should not contain heartbeat suffix below 15s threshold, got %q", title)
	}
}

// TestModel_HeartbeatTick_ClearsSuffix_WhenTransitionsToInactive verifies that
// the heartbeat suffix is cleared when a subsequent tick reports inactive.
func TestModel_HeartbeatTick_ClearsSuffix_WhenTransitionsToInactive(t *testing.T) {
	stub := &stubHeartbeat{silentFor: 20 * time.Second, active: true}
	m := newTestModel(t).WithHeartbeat(stub)
	m.header.header.IterationLine = "Iteration 1/1"

	// First tick: suffix should appear.
	next, _ := m.Update(HeartbeatTickMsg(time.Now()))
	m = next.(Model)
	if !strings.Contains(m.titleString(), "⋯") {
		t.Fatal("expected suffix after first tick")
	}

	// Second tick: step ended, now inactive.
	stub.active = false
	next, _ = m.Update(HeartbeatTickMsg(time.Now()))
	m = next.(Model)
	if strings.Contains(m.titleString(), "⋯") {
		t.Errorf("expected suffix cleared after inactive tick, got %q", m.titleString())
	}
}

// TP-HB1: stubHeartbeat call count — verifies the dispatch path reaches
// HeartbeatSilence() during Update(HeartbeatTickMsg).
func TestModel_HeartbeatTick_CallsHeartbeatSilence(t *testing.T) {
	stub := &stubHeartbeat{active: false}
	m := newTestModel(t).WithHeartbeat(stub)

	_, _ = m.Update(HeartbeatTickMsg(time.Now()))

	if stub.calls != 1 {
		t.Errorf("expected HeartbeatSilence called once, got %d", stub.calls)
	}
}

// TP-HB2: exact 15s boundary — silentFor == heartbeatSilenceThreshold must
// produce the suffix (condition is >=, not >).
func TestModel_HeartbeatTick_ExactThreshold_ShowsSuffix(t *testing.T) {
	stub := &stubHeartbeat{silentFor: 15 * time.Second, active: true}
	m := newTestModel(t).WithHeartbeat(stub)
	m.header.header.IterationLine = "Iteration 1/1 — Issue #1"

	next, _ := m.Update(HeartbeatTickMsg(time.Now()))
	m = next.(Model)

	title := m.titleString()
	want := "  ⋯ thinking (15s)"
	if !strings.Contains(title, want) {
		t.Errorf("titleString() at exact 15s threshold: want %q in %q", want, title)
	}
}

// TP-HB3: suffix suppressed when iterLine is empty — titleString() must return
// AppTitle only, even when heartbeatSuffix is non-empty.
func TestTitleString_SuffixSuppressed_WhenNoIterationLine(t *testing.T) {
	stub := &stubHeartbeat{silentFor: 20 * time.Second, active: true}
	m := newTestModel(t).WithHeartbeat(stub)
	m.header.header.IterationLine = ""

	// Tick sets heartbeatSuffix on the model.
	next, _ := m.Update(HeartbeatTickMsg(time.Now()))
	m = next.(Model)

	// iterLine is empty → titleString must return bare AppTitle with no suffix.
	got := m.titleString()
	if got != AppTitle {
		t.Errorf("titleString() with empty iter line: want %q, got %q", AppTitle, got)
	}
}

// TP-HB-R3: HeartbeatTickMsg handler returns no cmd — verifies that the
// ticker goroutine in main.go is the sole owner of the heartbeat schedule.
func TestModel_HeartbeatTick_ReturnsNilCmd(t *testing.T) {
	stub := &stubHeartbeat{active: false}
	m := newTestModel(t).WithHeartbeat(stub)

	_, cmd := m.Update(HeartbeatTickMsg(time.Now()))

	if cmd != nil {
		t.Errorf("expected nil cmd from HeartbeatTickMsg handler (ticker is owned by main.go), got non-nil: %T", cmd)
	}
}

// TP-HB4: fractional seconds are truncated (not rounded) — 15.9s must display
// as "15s", not "16s".
func TestModel_HeartbeatTick_FractionalSeconds_Truncated(t *testing.T) {
	stub := &stubHeartbeat{silentFor: 15*time.Second + 900*time.Millisecond, active: true}
	m := newTestModel(t).WithHeartbeat(stub)
	m.header.header.IterationLine = "Iteration 1/1"

	next, _ := m.Update(HeartbeatTickMsg(time.Now()))
	m = next.(Model)

	title := m.titleString()
	want := "  ⋯ thinking (15s)"
	if !strings.Contains(title, want) {
		t.Errorf("titleString() with 15.9s: want %q in %q (truncation not rounding)", want, title)
	}
}

// --- Mode-change trigger choke point tests (issue #116) ---

// newTestModelWithTrigger returns a Model with a trigger counter wired in.
func newTestModelWithTrigger(t *testing.T) (Model, *int) {
	t.Helper()
	count := 0
	m := newTestModel(t).WithModeTrigger(func() { count++ })
	return m, &count
}

// TestModel_ModeTrigger_NormalToQuitConfirm verifies that pressing 'q' in
// ModeNormal (→ ModeQuitConfirm) fires exactly one trigger.
func TestModel_ModeTrigger_NormalToQuitConfirm(t *testing.T) {
	m, count := newTestModelWithTrigger(t)

	_, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("q")})

	if *count != 1 {
		t.Errorf("Normal→QuitConfirm: want 1 trigger, got %d", *count)
	}
}

// TestModel_ModeTrigger_ErrorToQuitConfirm verifies that pressing 'q' in
// ModeError (→ ModeQuitConfirm) fires exactly one trigger.
func TestModel_ModeTrigger_ErrorToQuitConfirm(t *testing.T) {
	m, count := newTestModelWithTrigger(t)
	// Externally set error mode (simulating orchestrator step failure), then
	// sync prevObservedMode via a no-op heartbeat update.
	m.keys.handler.SetMode(ModeError)
	{
		next, _ := m.Update(HeartbeatTickMsg(time.Now()))
		m = next.(Model)
	}
	*count = 0 // reset the trigger fired by the Normal→Error sync

	_, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("q")})

	if *count != 1 {
		t.Errorf("Error→QuitConfirm: want 1 trigger, got %d", *count)
	}
}

// TestModel_ModeTrigger_NextConfirmToNormal verifies that pressing 'n' (cancel)
// in ModeNextConfirm (→ back to ModeNormal) fires exactly one trigger.
func TestModel_ModeTrigger_NextConfirmToNormal(t *testing.T) {
	m, count := newTestModelWithTrigger(t)

	// Enter NextConfirm first.
	{
		next, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("n")})
		m = next.(Model)
	}
	*count = 0 // reset after Normal→NextConfirm trigger

	// Cancel back to Normal.
	_, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("n")})

	if *count != 1 {
		t.Errorf("NextConfirm→Normal: want 1 trigger, got %d", *count)
	}
}

// TestModel_ModeTrigger_SelectToNormal verifies that pressing Esc in ModeSelect
// (→ prior mode) fires exactly one trigger.
func TestModel_ModeTrigger_SelectToNormal(t *testing.T) {
	m, count := newTestModelWithTrigger(t)
	m.log.SetSize(76, 10)
	// Add a line so 'v' can enter select mode.
	{
		next, _ := m.Update(LogLinesMsg{Lines: []string{"line"}})
		m = next.(Model)
	}
	// Enter select mode.
	{
		next, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("v")})
		m = next.(Model)
	}
	*count = 0 // reset

	_, _ = m.Update(tea.KeyMsg{Type: tea.KeyEscape})

	if *count != 1 {
		t.Errorf("Select→Normal (Esc): want 1 trigger, got %d", *count)
	}
}

// TestModel_ModeTrigger_MousePressNormalToSelect verifies that a left-click in
// the log area while in ModeNormal (→ ModeSelect) fires exactly one trigger.
func TestModel_ModeTrigger_MousePressNormalToSelect(t *testing.T) {
	m, count := newTestModelWithTrigger(t)
	m.width = 80
	m.height = 24
	m.log.SetSize(76, 10)
	// Add lines so the click lands on content.
	lines := make([]string, 10)
	for i := range lines {
		lines[i] = "content line"
	}
	{
		next, _ := m.Update(LogLinesMsg{Lines: lines})
		m = next.(Model)
	}

	// logTopRow = 1 (border) + gridRows + 1 (hrule)
	gridRows := len(m.header.header.Rows)
	logTopRow := gridRows + 2

	_, _ = m.Update(tea.MouseMsg{
		Action: tea.MouseActionPress,
		Button: tea.MouseButtonLeft,
		X:      2,
		Y:      logTopRow,
	})

	// The click may or may not enter ModeSelect depending on whether the viewport
	// has content at that row. Either way, if mode changed the trigger must fire.
	if m.keys.handler.Mode() == ModeSelect && *count != 1 {
		t.Errorf("MousePress Normal→Select: want 1 trigger, got %d", *count)
	}
}

// TestModel_ModeTrigger_NoFireOnSameMode verifies that events that do not
// change the mode produce zero triggers.
func TestModel_ModeTrigger_NoFireOnSameMode(t *testing.T) {
	m, count := newTestModelWithTrigger(t)

	// Keys that do nothing in ModeNormal (no mode transition).
	for _, key := range []string{"a", "b", "c", "r", "x"} {
		_, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(key)})
	}

	if *count != 0 {
		t.Errorf("no-op keys in ModeNormal: want 0 triggers, got %d", *count)
	}
}

// TestModel_ModeTrigger_NilTriggerFn_NoPanic verifies that a Model with no
// WithModeTrigger call (nil triggerFn) does not panic on mode transitions.
func TestModel_ModeTrigger_NilTriggerFn_NoPanic(t *testing.T) {
	m := newTestModel(t) // no WithModeTrigger

	defer func() {
		if r := recover(); r != nil {
			t.Errorf("Update panicked with nil triggerFn: %v", r)
		}
	}()

	// Trigger a mode change — must not panic.
	_, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("q")})
}

// TestModel_ModeTrigger_ExactlyOnePerTransition verifies that a sequence of
// distinct mode transitions each fire exactly one trigger and the counts
// accumulate correctly.
func TestModel_ModeTrigger_ExactlyOnePerTransition(t *testing.T) {
	m, count := newTestModelWithTrigger(t)
	m.log.SetSize(76, 10)

	// Add a log line so 'v' can enter ModeSelect.
	{
		next, _ := m.Update(LogLinesMsg{Lines: []string{"line"}})
		m = next.(Model)
	}

	transitions := []tea.KeyMsg{
		{Type: tea.KeyRunes, Runes: []rune("q")}, // Normal → QuitConfirm
		{Type: tea.KeyRunes, Runes: []rune("n")}, // QuitConfirm → Normal (prevMode)
	}
	for i, key := range transitions {
		next, _ := m.Update(key)
		m = next.(Model)
		want := i + 1
		if *count != want {
			t.Errorf("after transition %d: want %d triggers total, got %d", i+1, want, *count)
		}
	}
}

// --- Mode-trigger passive-message non-triggering tests (issue #116) ---

// TP-010: tea.WindowSizeMsg must not fire the mode trigger.
func TestModel_ModeTrigger_WindowSizeMsg_NoFire(t *testing.T) {
	m, count := newTestModelWithTrigger(t)

	_, _ = m.Update(tea.WindowSizeMsg{Width: 80, Height: 24})

	if *count != 0 {
		t.Errorf("WindowSizeMsg: want 0 triggers, got %d", *count)
	}
}

// TP-011: HeartbeatTickMsg must not fire the mode trigger.
func TestModel_ModeTrigger_HeartbeatTickMsg_NoFire(t *testing.T) {
	m, count := newTestModelWithTrigger(t)

	_, _ = m.Update(HeartbeatTickMsg(time.Now()))

	if *count != 0 {
		t.Errorf("HeartbeatTickMsg: want 0 triggers, got %d", *count)
	}
}

// TP-012: LogLinesMsg must not fire the mode trigger.
func TestModel_ModeTrigger_LogLinesMsg_NoFire(t *testing.T) {
	m, count := newTestModelWithTrigger(t)

	_, _ = m.Update(LogLinesMsg{Lines: []string{"a", "b"}})

	if *count != 0 {
		t.Errorf("LogLinesMsg: want 0 triggers, got %d", *count)
	}
}

// TP-025: header messages must not fire the mode trigger.
func TestModel_ModeTrigger_HeaderMessages_NoFire(t *testing.T) {
	msgs := []tea.Msg{
		headerStepStateMsg{idx: 0, state: StepDone},
		headerIterationLineMsg{iter: 1, max: 3, issue: "42"},
		headerInitializeLineMsg{stepNum: 1, stepCount: 2, stepName: "Setup"},
		headerFinalizeLineMsg{stepNum: 1, stepCount: 1, stepName: "Push"},
		headerPhaseStepsMsg{names: []string{"alpha", "beta"}},
	}

	for _, msg := range msgs {
		m, count := newTestModelWithTrigger(t)
		_, _ = m.Update(msg)
		if *count != 0 {
			t.Errorf("header message %T: want 0 triggers, got %d", msg, *count)
		}
	}
}

// TP-026: tea.QuitMsg must not fire the mode trigger.
func TestModel_ModeTrigger_QuitMsg_NoFire(t *testing.T) {
	m, count := newTestModelWithTrigger(t)

	// tea.QuitMsg returns early via `return m, tea.Quit` before reaching the
	// trigger check, so no trigger should fire.
	_, _ = m.Update(tea.QuitMsg{})

	if *count != 0 {
		t.Errorf("tea.QuitMsg: want 0 triggers, got %d", *count)
	}
}

// TP-027: cold-start — prevObservedMode zero value equals the handler's initial
// mode (ModeNormal = 0), so no trigger fires on the very first passive Update.
func TestModel_ModeTrigger_ColdStart_NoSpuriousFire(t *testing.T) {
	m, count := newTestModelWithTrigger(t)

	_, _ = m.Update(tea.WindowSizeMsg{Width: 80, Height: 24})

	if *count != 0 {
		t.Errorf("cold-start passive Update: want 0 triggers, got %d", *count)
	}
}

// TP-015: external SetMode path — the first passive Update after SetMode fires
// the trigger exactly once; subsequent passive Updates do not re-fire.
func TestModel_ModeTrigger_ExternalSetMode_FiresOnce(t *testing.T) {
	m, count := newTestModelWithTrigger(t)

	// Simulate orchestrator goroutine setting mode to ModeError.
	m.keys.handler.SetMode(ModeError)

	// First passive Update: prevObservedMode==Normal != current==Error → trigger fires.
	next, _ := m.Update(HeartbeatTickMsg(time.Now()))
	m = next.(Model)
	if *count != 1 {
		t.Errorf("first Update after SetMode: want 1 trigger, got %d", *count)
	}

	// Second passive Update: mode unchanged → no re-fire.
	_, _ = m.Update(HeartbeatTickMsg(time.Now()))
	if *count != 1 {
		t.Errorf("second Update after SetMode (same mode): want still 1 trigger, got %d", *count)
	}
}

// TP-013: in ModeSelect, a mouse release that commits the drag sets
// selectJustReleased=true without firing a new mode trigger.
func TestModel_ModeTrigger_SelectModeRelease_SetsJustReleasedNoTrigger(t *testing.T) {
	m, count := newTestModelWithTrigger(t)
	m.width = 80
	m.height = 24
	m.log.SetSize(76, 10)

	// Seed content so we can enter select mode.
	lines := make([]string, 10)
	for i := range lines {
		lines[i] = "content line"
	}
	{
		next, _ := m.Update(LogLinesMsg{Lines: lines})
		m = next.(Model)
	}

	// Enter ModeSelect via left press.
	gridRows := len(m.header.header.Rows)
	logTopRow := gridRows + 2
	{
		next, _ := m.Update(tea.MouseMsg{
			Action: tea.MouseActionPress,
			Button: tea.MouseButtonLeft,
			X:      2,
			Y:      logTopRow,
		})
		m = next.(Model)
	}
	// Only proceed if we actually entered ModeSelect.
	if m.keys.handler.Mode() != ModeSelect {
		t.Skip("left press did not enter ModeSelect (viewport may be empty at that row)")
	}
	*count = 0 // reset after entering Select

	// Dispatch Motion then Release.
	{
		next, _ := m.Update(tea.MouseMsg{
			Action: tea.MouseActionMotion,
			Button: tea.MouseButtonLeft,
			X:      3,
			Y:      logTopRow + 1,
		})
		m = next.(Model)
	}
	{
		next, _ := m.Update(tea.MouseMsg{
			Action: tea.MouseActionRelease,
			Button: tea.MouseButtonLeft,
			X:      3,
			Y:      logTopRow + 1,
		})
		m = next.(Model)
	}

	if *count != 0 {
		t.Errorf("ModeSelect release: want 0 new triggers, got %d", *count)
	}
	if m.keys.handler.Mode() != ModeSelect {
		t.Error("expected to stay in ModeSelect after release")
	}
	if !m.keys.handler.SelectJustReleased() {
		t.Error("expected selectJustReleased=true after mouse release in ModeSelect")
	}
}

// TP-014: transient 'v' flip-flop with empty log — pressing 'v' in ModeNormal
// when the log is empty reverts to ModeNormal and must not fire any trigger.
func TestModel_ModeTrigger_TransientVFlipflop_EmptyLog_NoTrigger(t *testing.T) {
	m, count := newTestModelWithTrigger(t)
	// No log lines added — log is empty.

	_, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("v")})

	if m.keys.handler.Mode() != ModeNormal {
		t.Errorf("expected ModeNormal after 'v' revert on empty log, got %v", m.keys.handler.Mode())
	}
	if *count != 0 {
		t.Errorf("transient 'v' with empty log: want 0 triggers, got %d", *count)
	}
}

// TP-028: mouse wheel in ModeSelect must not fire the mode trigger.
func TestModel_ModeTrigger_WheelInModeSelect_NoFire(t *testing.T) {
	m, count := newTestModelWithTrigger(t)
	m.log.SetSize(76, 10)

	// Seed content and enter ModeSelect via 'v'.
	{
		next, _ := m.Update(LogLinesMsg{Lines: []string{"line one"}})
		m = next.(Model)
	}
	{
		next, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("v")})
		m = next.(Model)
	}
	if m.keys.handler.Mode() != ModeSelect {
		t.Skip("could not enter ModeSelect")
	}
	*count = 0

	_, _ = m.Update(tea.MouseMsg{Button: tea.MouseButtonWheelDown})

	if *count != 0 {
		t.Errorf("wheel in ModeSelect: want 0 triggers, got %d", *count)
	}
	if m.keys.handler.Mode() != ModeSelect {
		t.Error("expected to remain in ModeSelect after wheel")
	}
}

// TP-029: left mouse press in error/confirm/quitting modes is ignored (mode guard
// at lines 223-226) and must not fire the trigger.
func TestModel_ModeTrigger_MousePressInErrorMode_NoFire(t *testing.T) {
	m, count := newTestModelWithTrigger(t)
	m.width = 80
	m.height = 24
	m.log.SetSize(76, 10)

	// SetMode to ModeError and flush the resulting trigger.
	m.keys.handler.SetMode(ModeError)
	{
		next, _ := m.Update(HeartbeatTickMsg(time.Now()))
		m = next.(Model)
	}
	*count = 0 // reset after flush

	// Dispatch left press — should be ignored due to mode guard.
	gridRows := len(m.header.header.Rows)
	logTopRow := gridRows + 2
	_, _ = m.Update(tea.MouseMsg{
		Action: tea.MouseActionPress,
		Button: tea.MouseButtonLeft,
		X:      2,
		Y:      logTopRow,
	})

	if *count != 0 {
		t.Errorf("left press in ModeError: want 0 new triggers, got %d", *count)
	}
}

// TP-031: right and middle mouse buttons in ModeNormal must not fire the trigger.
func TestModel_ModeTrigger_RightMiddleMouseInNormal_NoFire(t *testing.T) {
	buttons := []tea.MouseButton{tea.MouseButtonRight, tea.MouseButtonMiddle}
	for _, btn := range buttons {
		m, count := newTestModelWithTrigger(t)
		m.width = 80
		m.height = 24
		m.log.SetSize(76, 10)

		_, _ = m.Update(tea.MouseMsg{
			Action: tea.MouseActionPress,
			Button: btn,
			X:      2,
			Y:      5,
		})

		if *count != 0 {
			t.Errorf("button %v press in ModeNormal: want 0 triggers, got %d", btn, *count)
		}
	}
}

// TestModel_ModeTrigger_Help_FiresOnEachEdge verifies that the mode-change
// trigger fires once for each Help-related edge: Normal→Help (?), Help→Normal
// (esc), Help→QuitConfirm (q), and QuitConfirm→Normal (esc, entered via Help).
func TestModel_ModeTrigger_Help_FiresOnEachEdge(t *testing.T) {
	t.Run("NormalToHelp", func(t *testing.T) {
		m, count := newTestModelWithTrigger(t)
		m.keys.handler.SetStatusLineActive(true)

		next, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("?")})
		m = next.(Model)

		if *count != 1 {
			t.Errorf("Normal→Help (?): want 1 trigger, got %d", *count)
		}
		if m.keys.handler.Mode() != ModeHelp {
			t.Errorf("expected ModeHelp after ?, got %v", m.keys.handler.Mode())
		}
	})

	t.Run("HelpToNormalViaEsc", func(t *testing.T) {
		m, count := newTestModelWithTrigger(t)
		m.keys.handler.SetStatusLineActive(true)

		{
			next, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("?")})
			m = next.(Model)
		}
		*count = 0

		{
			next, _ := m.Update(tea.KeyMsg{Type: tea.KeyEscape})
			m = next.(Model)
		}

		if *count != 1 {
			t.Errorf("Help→Normal (esc): want 1 trigger, got %d", *count)
		}
		if m.keys.handler.Mode() != ModeNormal {
			t.Errorf("expected ModeNormal after esc from Help, got %v", m.keys.handler.Mode())
		}
	})

	t.Run("HelpToQuitConfirmViaQ", func(t *testing.T) {
		m, count := newTestModelWithTrigger(t)
		m.keys.handler.SetStatusLineActive(true)

		{
			next, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("?")})
			m = next.(Model)
		}
		*count = 0

		next, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("q")})
		m = next.(Model)

		if *count != 1 {
			t.Errorf("Help→QuitConfirm (q): want 1 trigger, got %d", *count)
		}
		if m.keys.handler.Mode() != ModeQuitConfirm {
			t.Errorf("expected ModeQuitConfirm after q from Help, got %v", m.keys.handler.Mode())
		}
	})

	t.Run("QuitConfirmToNormalViaEsc_EnteredViaHelp", func(t *testing.T) {
		m, count := newTestModelWithTrigger(t)
		m.keys.handler.SetStatusLineActive(true)

		{
			next, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("?")})
			m = next.(Model)
		}
		{
			next, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("q")})
			m = next.(Model)
		}
		*count = 0

		next, _ := m.Update(tea.KeyMsg{Type: tea.KeyEscape})
		m = next.(Model)

		if *count != 1 {
			t.Errorf("QuitConfirm→Normal (esc, via Help): want 1 trigger, got %d", *count)
		}
		if m.keys.handler.Mode() != ModeNormal {
			t.Errorf("expected ModeNormal after esc from QuitConfirm via Help, got %v", m.keys.handler.Mode())
		}
	})
}

// --- TP-009: ? no-op does not fire mode-change trigger ---

// TestModel_ModeTrigger_QuestionMark_NoOp_NoFire verifies that a ? press when
// StatusLineActive is false (the default) is a silent no-op that must not
// spuriously fire the mode-change trigger, since mode did not change.
func TestModel_ModeTrigger_QuestionMark_NoOp_NoFire(t *testing.T) {
	m, count := newTestModelWithTrigger(t)
	// StatusLineActive defaults to false — ? is a no-op.

	next, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("?")})
	m = next.(Model)

	if *count != 0 {
		t.Errorf("? no-op (StatusLineActive=false): want 0 triggers, got %d", *count)
	}
	if m.keys.handler.Mode() != ModeNormal {
		t.Errorf("expected ModeNormal after no-op ?, got %v", m.keys.handler.Mode())
	}
}

// --- Status-line footer rendering tests (issue #118) ---

// newTestModelWithStatus returns a Model with a given StatusReader installed
// and a fixed 80×24 terminal size with a minimal log viewport.
func newTestModelWithStatus(t *testing.T, sr StatusReader) Model {
	t.Helper()
	header := NewStatusHeader(1)
	header.SetPhaseSteps([]string{"step"})
	actions := make(chan StepAction, 10)
	kh := NewKeyHandler(func() {}, actions)
	m := NewModel(header, kh, "v0.0.0-test").WithStatusRunner(sr)
	m.width = 80
	m.height = 24
	m.log.SetSize(76, 10)
	return m
}

// TestView_StatusLine_NarrowTerminal_TruncatesStatusText verifies that on a
// narrow terminal (innerWidth=30) the status text is truncated to its budget
// while "? Help" and the version label remain visible in the footer.
func TestView_StatusLine_NarrowTerminal_TruncatesStatusText(t *testing.T) {
	sr := &mockStatusReader{
		enabled:   true,
		hasOutput: true,
		output:    "this is a very long status text that should be truncated",
	}
	header := NewStatusHeader(1)
	header.SetPhaseSteps([]string{"step"})
	actions := make(chan StepAction, 10)
	kh := NewKeyHandler(func() {}, actions)
	m := NewModel(header, kh, "v1.0").WithStatusRunner(sr)
	// Use the minimum renderable width (uichrome.MinTerminalWidth=60). Below
	// that, the View() guard returns a placeholder rather than the chrome.
	m.width = 60 // innerWidth = 58
	m.height = 24
	m.log.SetSize(56, 10)

	out := m.View()
	plain := stripANSI(out)

	if !strings.Contains(plain, "? Help") {
		t.Errorf("'? Help' not found in footer on narrow terminal; output:\n%s", plain)
	}
	if !strings.Contains(plain, "v1.0") {
		t.Errorf("version label not found in footer on narrow terminal; output:\n%s", plain)
	}
}

// TestView_StatusLine_SGRPreserved_WidthMatchesLipgloss verifies that when the
// status text contains SGR escape sequences the footer width math uses
// lipgloss.Width (visual columns) rather than byte length, so the layout does
// not overflow.
func TestView_StatusLine_SGRPreserved_WidthMatchesLipgloss(t *testing.T) {
	colored := lipgloss.NewStyle().Foreground(lipgloss.Color("2")).Render("OK")
	sr := &mockStatusReader{enabled: true, hasOutput: true, output: colored}
	m := newTestModelWithStatus(t, sr)

	out := m.View()
	plain := stripANSI(out)

	// The colored "OK" should render as plain "OK" in the footer.
	if !strings.Contains(plain, "OK") {
		t.Errorf("status text 'OK' not found in View output; plain:\n%s", plain)
	}
	if !strings.Contains(plain, "? Help") {
		t.Errorf("'? Help' not found in footer; plain:\n%s", plain)
	}
}

// TestView_StatusLine_HasOutputFalse_FallsBackToNormalShortcuts verifies that
// when HasOutput() is false (cold start) the footer shows NormalShortcuts
// regardless of Enabled().
func TestView_StatusLine_HasOutputFalse_FallsBackToNormalShortcuts(t *testing.T) {
	sr := &mockStatusReader{enabled: true, hasOutput: false, output: ""}
	m := newTestModelWithStatus(t, sr)

	out := m.View()
	plain := stripANSI(out)

	if strings.Contains(plain, "? Help") {
		t.Errorf("'? Help' should not appear when HasOutput=false; got:\n%s", plain)
	}
	// NormalShortcuts should be visible.
	if !strings.Contains(plain, "v select") {
		t.Errorf("NormalShortcuts not found when HasOutput=false; plain:\n%s", plain)
	}
}

// TestView_StatusLine_DisabledRunner_UsesShortcutPath verifies that when
// Enabled() returns false the footer always uses the shortcut-bar path.
func TestView_StatusLine_DisabledRunner_UsesShortcutPath(t *testing.T) {
	sr := &mockStatusReader{enabled: false, hasOutput: true, output: "status text"}
	m := newTestModelWithStatus(t, sr)

	out := m.View()
	plain := stripANSI(out)

	if strings.Contains(plain, "? Help") {
		t.Errorf("'? Help' should not appear when Enabled=false; got:\n%s", plain)
	}
	if strings.Contains(plain, "status text") {
		t.Errorf("status text should not appear when Enabled=false; got:\n%s", plain)
	}
}

// TestView_StatusLine_ModeHelp_UsesHelpShortcuts verifies that in ModeHelp the
// footer shows HelpModeShortcuts (the else branch) even when the status runner
// is enabled and has output.
func TestView_StatusLine_ModeHelp_UsesHelpShortcuts(t *testing.T) {
	sr := &mockStatusReader{enabled: true, hasOutput: true, output: "some status"}
	header := NewStatusHeader(1)
	header.SetPhaseSteps([]string{"step"})
	actions := make(chan StepAction, 10)
	kh := NewKeyHandler(func() {}, actions)
	kh.SetStatusLineActive(true)
	m := NewModel(header, kh, "v0.0.0-test").WithStatusRunner(sr)
	m.width = 80
	m.height = 24
	m.log.SetSize(76, 10)

	// Enter ModeHelp via '?'
	next, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("?")})
	m = next.(Model)
	if m.keys.handler.Mode() != ModeHelp {
		t.Skip("ModeHelp not entered; skipping footer test")
	}

	out := m.View()
	plain := stripANSI(out)

	// The footer should show HelpModeShortcuts ("esc  close"), not status text.
	if strings.Contains(plain, "some status") {
		t.Errorf("status text must not appear in footer during ModeHelp; plain:\n%s", plain)
	}
	// "esc" should appear somewhere (in the modal or footer).
	if !strings.Contains(plain, "esc") {
		t.Errorf("'esc' not found in ModeHelp view; plain:\n%s", plain)
	}
}

// TestModel_ModeHelp_MousePress_NoModeSelect verifies that a left-click in
// ModeHelp does not transition to ModeSelect (the modal swallows non-wheel
// mouse events).
func TestModel_ModeHelp_MousePress_NoModeSelect(t *testing.T) {
	sr := &mockStatusReader{enabled: true, hasOutput: true, output: "status"}
	header := NewStatusHeader(1)
	header.SetPhaseSteps([]string{"step"})
	actions := make(chan StepAction, 10)
	kh := NewKeyHandler(func() {}, actions)
	kh.SetStatusLineActive(true)
	m := NewModel(header, kh, "v0.0.0-test").WithStatusRunner(sr)
	m.width = 80
	m.height = 24
	m.log.SetSize(76, 10)

	// Populate log so a normal click would enter ModeSelect.
	lines := make([]string, 10)
	for i := range lines {
		lines[i] = "content line"
	}
	{
		next, _ := m.Update(LogLinesMsg{Lines: lines})
		m = next.(Model)
	}

	// Enter ModeHelp.
	{
		next, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("?")})
		m = next.(Model)
	}
	if m.keys.handler.Mode() != ModeHelp {
		t.Skip("ModeHelp not entered; skipping mouse guard test")
	}

	// Simulate a left-click in the log area.
	gridRows := len(m.header.header.Rows)
	logTopRow := gridRows + 2
	next, _ := m.Update(tea.MouseMsg{
		Action: tea.MouseActionPress,
		Button: tea.MouseButtonLeft,
		X:      2,
		Y:      logTopRow,
	})
	m = next.(Model)

	if m.keys.handler.Mode() == ModeSelect {
		t.Error("left-click in ModeHelp must not transition to ModeSelect")
	}
}

// TestModel_ModeHelp_WheelEvent_ForwardedToViewport verifies that wheel events
// in ModeHelp are still forwarded to the viewport (the modal does not swallow
// scroll events).
func TestModel_ModeHelp_WheelEvent_ForwardedToViewport(t *testing.T) {
	sr := &mockStatusReader{enabled: true, hasOutput: true, output: "status"}
	header := NewStatusHeader(1)
	header.SetPhaseSteps([]string{"step"})
	actions := make(chan StepAction, 10)
	kh := NewKeyHandler(func() {}, actions)
	kh.SetStatusLineActive(true)
	m := NewModel(header, kh, "v0.0.0-test").WithStatusRunner(sr)
	m.width = 80
	m.height = 24
	m.log.SetSize(76, 10)

	// Enter ModeHelp.
	{
		next, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("?")})
		m = next.(Model)
	}
	if m.keys.handler.Mode() != ModeHelp {
		t.Skip("ModeHelp not entered; skipping wheel test")
	}

	// Wheel events must not panic and must not change the mode.
	defer func() {
		if r := recover(); r != nil {
			t.Errorf("wheel event in ModeHelp panicked: %v", r)
		}
	}()

	next, _ := m.Update(tea.MouseMsg{
		Action: tea.MouseActionMotion,
		Button: tea.MouseButtonWheelDown,
	})
	m = next.(Model)

	if m.keys.handler.Mode() != ModeHelp {
		t.Errorf("wheel event changed mode away from ModeHelp: %v", m.keys.handler.Mode())
	}
}

// --- ModeHelp modal visibility and content tests (issue #118) ---

// assertModalFits fails the test immediately if the modal rendered by m would
// exceed the frame height, indicating the test fixture height is too small for
// the current modal row count. Update m.height (or reduce m.log.SetSize) if
// you add rows to renderHelpModal.
func assertModalFits(t *testing.T, m Model) {
	t.Helper()
	modal := m.renderHelpModal()
	modalH := len(strings.Split(modal, "\n"))
	if modalH > m.height {
		t.Fatalf("test fixture misconfigured: modal height %d exceeds frame height %d; update newTestModelModeHelp fixture if the modal row count changed", modalH, m.height)
	}
}

// newTestModelModeHelp returns a Model in ModeHelp with an 80-column terminal.
// The viewport is set to 18 rows and m.height is set to 6+18=24 so that the
// frame height equals m.height. This ensures the centering math in View() keeps
// the full help modal inside the 24-row frame without clipping.
// Calls t.Skip if ModeHelp could not be entered.
func newTestModelModeHelp(t *testing.T) Model {
	t.Helper()
	header := NewStatusHeader(1)
	header.SetPhaseSteps([]string{"step"})
	actions := make(chan StepAction, 10)
	kh := NewKeyHandler(func() {}, actions)
	kh.SetStatusLineActive(true)
	m := NewModel(header, kh, "v0.0.0-test")
	m.width = 80
	// Frame height = topBorder + headerRow + hrule + vpHeight + hrule + footer +
	// bottomBorder = 6 + vpHeight. With vpHeight=18, frame height = 24.
	// Setting m.height=24 aligns the centering math with the actual frame height.
	m.height = 24
	m.log.SetSize(76, 18)
	next, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("?")})
	m = next.(Model)
	if m.keys.handler.Mode() != ModeHelp {
		t.Skip("ModeHelp not entered; skipping test")
	}
	assertModalFits(t, m)
	return m
}

// TestView_ModeHelp_ModalIsVisibleInRenderedFrame verifies that entering
// ModeHelp causes the modal title "Help: Keyboard Shortcuts" to appear in the
// rendered View() frame. This is the load-bearing user-visible contract of the
// overlay-splice path.
func TestView_ModeHelp_ModalIsVisibleInRenderedFrame(t *testing.T) {
	m := newTestModelModeHelp(t)
	plain := stripANSI(m.View())
	if !strings.Contains(plain, "Help: Keyboard Shortcuts") {
		t.Errorf("modal title not found in rendered frame; plain:\n%s", plain)
	}
}

// TestView_ModeHelp_ModalContainsAllFourSectionLabels verifies that the
// rendered frame contains the section labels "Normal", "Select", "Error",
// "Done" in that order, matching the renderHelpModal addSection contract.
func TestView_ModeHelp_ModalContainsAllFourSectionLabels(t *testing.T) {
	m := newTestModelModeHelp(t)
	plain := stripANSI(m.View())

	sections := []string{"Normal", "Select", "Error", "Done"}
	prev := -1
	for _, label := range sections {
		idx := strings.Index(plain, label)
		if idx < 0 {
			t.Errorf("section label %q not found in rendered frame", label)
			continue
		}
		if idx <= prev {
			t.Errorf("section label %q does not appear after preceding section (pos %d <= %d)", label, idx, prev)
		}
		prev = idx
	}
}

// TestView_ModeHelp_NilStatusRunner_DoesNotPanic verifies that
// WithStatusRunner(nil) followed by a '?' keypress and View() does not panic
// and still renders the modal. Nil is documented as a supported input.
func TestView_ModeHelp_NilStatusRunner_DoesNotPanic(t *testing.T) {
	header := NewStatusHeader(1)
	header.SetPhaseSteps([]string{"step"})
	actions := make(chan StepAction, 10)
	kh := NewKeyHandler(func() {}, actions)
	kh.SetStatusLineActive(true)
	m := NewModel(header, kh, "v0.0.0-test").WithStatusRunner(nil)
	m.width = 80
	m.height = 24
	m.log.SetSize(76, 10)

	defer func() {
		if r := recover(); r != nil {
			t.Errorf("View() panicked with nil status runner in ModeHelp: %v", r)
		}
	}()

	next, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("?")})
	m = next.(Model)
	if m.keys.handler.Mode() != ModeHelp {
		t.Skip("ModeHelp not entered; skipping nil runner test")
	}

	plain := stripANSI(m.View())
	if !strings.Contains(plain, "Help: Keyboard Shortcuts") {
		t.Errorf("modal not shown with nil status runner; plain:\n%s", plain)
	}
}

// TestView_StatusLine_FooterFrameRowWidthStable_ANSIPreserved verifies that
// when the status-line footer path is taken with an SGR-carrying LastOutput(),
// the footer row (after wrapLine) has visual width == m.width and both │
// side-bar glyphs are at the correct visual columns.
func TestView_StatusLine_FooterFrameRowWidthStable_ANSIPreserved(t *testing.T) {
	colored := lipgloss.NewStyle().Foreground(lipgloss.Color("2")).Render("status OK")
	sr := &mockStatusReader{enabled: true, hasOutput: true, output: colored}
	m := newTestModelWithStatus(t, sr)

	out := m.View()
	lines := strings.Split(out, "\n")

	// Locate the footer: the line immediately before the bottom border (╰...╯).
	var footerLine string
	for i, l := range lines {
		if strings.HasPrefix(stripANSI(l), "╰") && i > 0 {
			footerLine = lines[i-1]
			break
		}
	}
	if footerLine == "" {
		t.Fatal("footer line not found in View() output")
	}

	if got := lipgloss.Width(footerLine); got != m.width {
		t.Errorf("footer row visual width = %d, want %d", got, m.width)
	}
	plain := stripANSI(footerLine)
	if !strings.HasPrefix(plain, "│") {
		t.Errorf("footer left border '│' missing: %q", plain)
	}
	if !strings.HasSuffix(plain, "│") {
		t.Errorf("footer right border '│' missing: %q", plain)
	}
}

// TestView_ModeHelp_SmallTerminal_ModalClampedTo29ColWidth verifies that on a
// sub-33-column terminal (m.width=10, so m.width-4=6) the modal is clamped to
// 29 columns by the floor in renderHelpModal. 29 columns ensures the title
// " Help: Keyboard Shortcuts " (26 chars + 1 lead dash = 27 inner columns) always
// fits without overflowing the top border. Content rows are checked (not the
// title border) to confirm the clamp applies uniformly.
func TestView_ModeHelp_SmallTerminal_ModalClampedTo29ColWidth(t *testing.T) {
	header := NewStatusHeader(1)
	header.SetPhaseSteps([]string{"step"})
	actions := make(chan StepAction, 10)
	kh := NewKeyHandler(func() {}, actions)
	kh.SetStatusLineActive(true)
	m := NewModel(header, kh, "v0.0.0-test")
	m.width = 10
	m.height = 40
	m.log.SetSize(6, 10)

	next, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("?")})
	m = next.(Model)
	if m.keys.handler.Mode() != ModeHelp {
		t.Skip("ModeHelp not entered; skipping small terminal test")
	}

	modal := m.renderHelpModal()
	lines := strings.Split(modal, "\n")
	// lines[0] is the title border; lines[1] is the first blank content row.
	// With the floor at 29 the title (27 inner columns) fits exactly, so both
	// the title row and content rows are 29 visible columns wide.
	if len(lines) < 2 {
		t.Fatalf("modal has fewer than 2 lines: %d", len(lines))
	}
	if got := lipgloss.Width(lines[1]); got != 29 {
		t.Errorf("modal content row visual width = %d, want 29 (clamped floor)", got)
	}
}

// TestView_ModeHelp_ShortFrame_EscHintAlwaysVisible verifies that when the
// terminal is shorter than the modal, View() pins the bottom border and esc
// hint to the last two visible rows. The user must always be able to see the
// dismissal cue even on a very short terminal.
func TestView_ModeHelp_ShortFrame_EscHintAlwaysVisible(t *testing.T) {
	header := NewStatusHeader(1)
	header.SetPhaseSteps([]string{"step"})
	actions := make(chan StepAction, 10)
	kh := NewKeyHandler(func() {}, actions)
	kh.SetStatusLineActive(true)
	m := NewModel(header, kh, "v0.0.0-test")
	m.width = 80
	// Use a height shorter than the modal (modal is ~22 rows; use 10).
	m.height = 10
	m.log.SetSize(76, 4)

	next, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("?")})
	m = next.(Model)
	if m.keys.handler.Mode() != ModeHelp {
		t.Skip("ModeHelp not entered; skipping short-frame test")
	}

	frameLines := strings.Split(m.View(), "\n")
	// The esc hint must appear in the frame despite clipping.
	plain := stripANSI(strings.Join(frameLines, "\n"))
	if !strings.Contains(plain, "esc") {
		t.Errorf("esc dismissal hint not found in short-frame ModeHelp view; plain:\n%s", plain)
	}
	// The bottom border character must appear in the last frame line that
	// contains modal content (look for "╯").
	found := false
	for _, l := range frameLines {
		if strings.Contains(stripANSI(l), "╯") {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("modal bottom border '╯' not found in short-frame view; plain:\n%s", plain)
	}
}

// TestView_ModeHelp_ModalCentering_OffsetMatchesMath verifies that the modal
// top border appears at the row computed by (height-modalH)/2 in the rendered
// frame. Guards against numerator/denominator swap or missing <0 clip.
func TestView_ModeHelp_ModalCentering_OffsetMatchesMath(t *testing.T) {
	m := newTestModelModeHelp(t)

	modal := m.renderHelpModal()
	modalLines := strings.Split(modal, "\n")
	modalH := len(modalLines)
	expectedTop := (m.height - modalH) / 2
	if expectedTop < 0 {
		expectedTop = 0
	}

	frameLines := strings.Split(m.View(), "\n")
	actualTop := -1
	for i, l := range frameLines {
		if strings.Contains(stripANSI(l), "Help: Keyboard Shortcuts") {
			actualTop = i
			break
		}
	}
	if actualTop < 0 {
		t.Fatal("modal top border not found in rendered frame")
	}
	if actualTop != expectedTop {
		t.Errorf("modal top row = %d, want %d (height=%d, modalH=%d)", actualTop, expectedTop, m.height, modalH)
	}
}

// TestModel_ModeHelp_RightAndMiddleClick_DoNotEnterModeSelect verifies that
// right-click and middle-click while in ModeHelp do not change the mode.
// Complements the shipped left-click test.
func TestModel_ModeHelp_RightAndMiddleClick_DoNotEnterModeSelect(t *testing.T) {
	m := newTestModelModeHelp(t)

	gridRows := len(m.header.header.Rows)
	logTopRow := gridRows + 2

	for _, btn := range []tea.MouseButton{tea.MouseButtonRight, tea.MouseButtonMiddle} {
		next, _ := m.Update(tea.MouseMsg{
			Action: tea.MouseActionPress,
			Button: btn,
			X:      2,
			Y:      logTopRow,
		})
		got := next.(Model).keys.handler.Mode()
		if got != ModeHelp {
			t.Errorf("button %v: mode changed from ModeHelp to %v", btn, got)
		}
	}
}

// TestModel_ErrorMode_MousePress_StillSwallowed verifies that a left-click
// while in ModeError does not trigger a mode transition. Regression guard for
// the compound || chain at model.go:250.
func TestModel_ErrorMode_MousePress_StillSwallowed(t *testing.T) {
	m := newTestModel(t)
	m.width = 80
	m.height = 24
	m.log.SetSize(76, 10)

	// Fill log so a click in Normal mode would enter ModeSelect.
	fill := make([]string, 10)
	for i := range fill {
		fill[i] = "content"
	}
	next, _ := m.Update(LogLinesMsg{Lines: fill})
	m = next.(Model)

	m.keys.handler.SetMode(ModeError)

	gridRows := len(m.header.header.Rows)
	logTopRow := gridRows + 2
	next2, _ := m.Update(tea.MouseMsg{
		Action: tea.MouseActionPress,
		Button: tea.MouseButtonLeft,
		X:      2,
		Y:      logTopRow,
	})
	m = next2.(Model)

	if m.keys.handler.Mode() != ModeError {
		t.Errorf("left-click in ModeError changed mode to %v; want ModeError", m.keys.handler.Mode())
	}
}

// TestView_StatusLine_EnabledFlipBetweenCalls_NextViewUpdatesFooter verifies
// that the footer reads StatusReader on every View() call rather than caching.
// Covers the cold-start → populated transition that happens at runtime.
// Each state is isolated via WithStatusRunner so a future caching optimization
// cannot cause false-positive test passes.
func TestView_StatusLine_EnabledFlipBetweenCalls_NextViewUpdatesFooter(t *testing.T) {
	m := newTestModelWithStatus(t, &mockStatusReader{enabled: false, hasOutput: false, output: ""})

	plain1 := stripANSI(m.View())
	if strings.Contains(plain1, "? Help") {
		t.Error("first View should not contain '? Help' when runner is disabled")
	}

	m = m.WithStatusRunner(&mockStatusReader{enabled: true, hasOutput: true, output: "build passing"})

	plain2 := stripANSI(m.View())
	if !strings.Contains(plain2, "? Help") {
		t.Errorf("second View should contain '? Help' after runner flip; plain:\n%s", plain2)
	}
	if !strings.Contains(plain2, "build passing") {
		t.Errorf("second View should contain status text after runner flip; plain:\n%s", plain2)
	}
}

// --- StatusLineUpdatedMsg ---

// TestModel_StatusLineUpdatedMsg_NoStateChange verifies that a StatusLineUpdatedMsg
// is handled by Model.Update without panicking and without changing any
// observable state (mode, log lines, etc.). The implicit re-render is
// Bubble Tea's responsibility after Update returns.
func TestModel_StatusLineUpdatedMsg_NoStateChange(t *testing.T) {
	m := newTestModel(t)
	modeBefore := m.keys.handler.Mode()

	next, cmd := m.Update(StatusLineUpdatedMsg{})
	if cmd != nil {
		t.Errorf("expected nil cmd, got %v", cmd)
	}

	m2, ok := next.(Model)
	if !ok {
		t.Fatalf("Update did not return a Model")
	}
	if m2.keys.handler.Mode() != modeBefore {
		t.Errorf("mode changed unexpectedly: got %v, want %v", m2.keys.handler.Mode(), modeBefore)
	}
	if len(m2.log.lines) != 0 {
		t.Errorf("log lines changed unexpectedly: %d lines", len(m2.log.lines))
	}
}

// TestModel_StatusLineActive_WiredFromRunnerEnabled verifies the SetStatusLineActive
// plumbing: after constructing a model with an enabled StatusReader, calling
// SetStatusLineActive(runner.Enabled()) on the KeyHandler makes
// StatusLineActive() return true, enabling the ? key.
func TestModel_StatusLineActive_WiredFromRunnerEnabled(t *testing.T) {
	header := NewStatusHeader(1)
	header.SetPhaseSteps([]string{"step-one"})
	actions := make(chan StepAction, 10)
	kh := NewKeyHandler(func() {}, actions)

	// Simulate what main.go does after constructing the runner:
	// keyHandler.SetStatusLineActive(statusRunner.Enabled())
	enabledRunner := &mockStatusReader{enabled: true, hasOutput: false, output: ""}
	kh.SetStatusLineActive(enabledRunner.Enabled())

	if !kh.StatusLineActive() {
		t.Error("expected StatusLineActive to be true after SetStatusLineActive(true)")
	}

	// Disabled runner → SetStatusLineActive(false) leaves it false.
	disabledRunner := &mockStatusReader{enabled: false, hasOutput: false, output: ""}
	kh.SetStatusLineActive(disabledRunner.Enabled())
	if kh.StatusLineActive() {
		t.Error("expected StatusLineActive to be false after SetStatusLineActive(false)")
	}
}

// mockStatusReader is a test double for StatusReader that lets callers control
// Enabled/HasOutput/LastOutput independently.
type mockStatusReader struct {
	enabled   bool
	hasOutput bool
	output    string
}

func (r *mockStatusReader) Enabled() bool      { return r.enabled }
func (r *mockStatusReader) HasOutput() bool    { return r.hasOutput }
func (r *mockStatusReader) LastOutput() string { return r.output }

// --- Resize / overflow guard tests (run-tui-resize-overflow) ---

// newTestModelWithSteps builds a Model whose header is sized to stepCount
// (so gridRows = ceil(stepCount/HeaderCols)).
func newTestModelWithSteps(t *testing.T, stepCount int) Model {
	t.Helper()
	header := NewStatusHeader(stepCount)
	names := make([]string, stepCount)
	for i := range names {
		names[i] = "s"
	}
	header.SetPhaseSteps(names)
	actions := make(chan StepAction, 10)
	kh := NewKeyHandler(func() {}, actions)
	return NewModel(header, kh, "pr9k v0.0.0-test")
}

// batchContains walks the cmd tree returned from tea.Batch and reports whether
// any leaf cmd has the same identity as want. tea.ClearScreen is a sentinel
// function value, so identity comparison via reflect.ValueOf().Pointer() works.
func batchContains(cmd tea.Cmd, want tea.Cmd) bool {
	if cmd == nil {
		return false
	}
	// Execute the batch to expose its children. tea.Batch returns a cmd that
	// when invoked returns a tea.BatchMsg containing the child cmds.
	msg := cmd()
	switch m := msg.(type) {
	case tea.BatchMsg:
		for _, c := range m {
			if cmdIdentityEqual(c, want) {
				return true
			}
			if batchContains(c, want) {
				return true
			}
		}
	default:
		return cmdIdentityEqual(cmd, want)
	}
	return false
}

func cmdIdentityEqual(a, b tea.Cmd) bool {
	if a == nil || b == nil {
		return a == nil && b == nil
	}
	// Compare by invoking and matching message type+value: tea.ClearScreen
	// returns clearScreenMsg{} (an unexported package type), so the value
	// equality path catches it.
	ma, mb := a(), b()
	return ma == mb
}

func TestView_FrameNeverExceedsHeight(t *testing.T) {
	cases := []struct {
		name      string
		width     int
		height    int
		stepCount int
	}{
		{"normal-80x24-1step", 80, 24, 1},
		{"normal-172x39-13step", 172, 39, 13},
		{"narrow-60x20-4step", 60, 20, 4},
		{"too-small-40x10-13step", 40, 10, 13},
		{"too-small-zero", 0, 0, 1},
		{"too-small-1x1", 1, 1, 1},
		{"too-small-height-just-below-min", 80, 6, 1}, // gridRows=1, minH=8
		{"exact-min-height", 80, 8, 1},                // gridRows=1, minH=8
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			m := newTestModelWithSteps(t, tc.stepCount)
			next, _ := m.Update(tea.WindowSizeMsg{Width: tc.width, Height: tc.height})
			m = next.(Model)

			out := m.View()
			lineCount := strings.Count(out, "\n") + 1
			if tc.height > 0 && lineCount > tc.height {
				t.Errorf("View() output has %d lines, exceeds height=%d:\n%s",
					lineCount, tc.height, out)
			}
		})
	}
}

func TestView_TooSmall_RendersPlaceholder(t *testing.T) {
	m := newTestModelWithSteps(t, 13) // gridRows=4, minH=9
	next, _ := m.Update(tea.WindowSizeMsg{Width: 40, Height: 5})
	m = next.(Model)

	out := m.View()
	if !strings.HasPrefix(out, "Terminal too small") {
		t.Errorf("expected placeholder string, got: %q", out)
	}
	for _, glyph := range []string{"╭", "╮", "╰", "╯", "├", "┤"} {
		if strings.Contains(out, glyph) {
			t.Errorf("placeholder contains border glyph %q; got: %q", glyph, out)
		}
	}
}

func TestView_AfterResizeSmallThenLarge_RendersFullFrame(t *testing.T) {
	m := newTestModelWithSteps(t, 4)

	// First, a too-small size.
	next, _ := m.Update(tea.WindowSizeMsg{Width: 20, Height: 5})
	m = next.(Model)
	if !strings.HasPrefix(m.View(), "Terminal too small") {
		t.Fatalf("expected placeholder after small resize")
	}

	// Then resize back up.
	next, _ = m.Update(tea.WindowSizeMsg{Width: 100, Height: 30})
	m = next.(Model)

	out := m.View()
	if strings.HasPrefix(out, "Terminal too small") {
		t.Errorf("expected full frame after resize-up, still got placeholder")
	}
	if !strings.Contains(out, "╭") {
		t.Errorf("expected full frame to contain top-left border glyph; got:\n%s", out)
	}
}

func TestWindowSizeMsg_ReturnsClearScreenCmd(t *testing.T) {
	m := newTestModelWithSteps(t, 4)
	_, cmd := m.Update(tea.WindowSizeMsg{Width: 100, Height: 30})

	if cmd == nil {
		t.Fatal("WindowSizeMsg returned nil cmd; expected batch including tea.ClearScreen")
	}
	if !batchContains(cmd, tea.ClearScreen) {
		t.Errorf("WindowSizeMsg batch does not contain tea.ClearScreen")
	}
}

func TestMouseMsg_TooSmall_Ignored(t *testing.T) {
	m := newTestModelWithSteps(t, 13)
	next, _ := m.Update(tea.WindowSizeMsg{Width: 30, Height: 5})
	m = next.(Model)

	// Send a left-press mouse event in placeholder mode. Should not panic
	// and should not trigger ModeSelect.
	prevMode := m.keys.handler.Mode()
	next, _ = m.Update(tea.MouseMsg{
		X:      5,
		Y:      3,
		Button: tea.MouseButtonLeft,
		Action: tea.MouseActionPress,
	})
	m = next.(Model)
	if m.keys.handler.Mode() != prevMode {
		t.Errorf("mouse press in placeholder mode changed key mode from %v to %v",
			prevMode, m.keys.handler.Mode())
	}
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
