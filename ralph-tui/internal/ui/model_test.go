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
