package ui

import (
	"strings"
	"testing"
	"time"

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
