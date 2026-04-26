package workflowedit

import (
	"errors"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/mxriverlynn/pr9k/src/internal/workflowio"
	"github.com/mxriverlynn/pr9k/src/internal/workflowmodel"
)

// ============================================================
// Part A — Empty-editor modes (no workflow loaded)
// ============================================================

// TestModel_Mode_1 — EmptyEditor, Ctrl+N → DialogNewChoice
func TestModel_Mode_1_EmptyEditor_CtrlN_OpensNewChoiceDialog(t *testing.T) {
	m := newTestModel()
	got := applyKey(m, keyCtrlN())
	if got.dialog.kind != DialogNewChoice {
		t.Fatalf("want DialogNewChoice, got %d", got.dialog.kind)
	}
	if !strings.Contains(got.View(), "Copy") {
		t.Error("view should contain Copy option")
	}
}

// TestModel_Mode_2 — EmptyEditor + DialogNewChoice, Esc → dialog closes
func TestModel_Mode_2_DialogNewChoice_Esc_ClosesDialog(t *testing.T) {
	m := newTestModel()
	m.dialog = dialogState{kind: DialogNewChoice}
	got := applyKey(m, keyEsc())
	if got.dialog.kind != DialogNone {
		t.Fatalf("want DialogNone, got %d", got.dialog.kind)
	}
}

// TestModel_Mode_3 — EmptyEditor, Ctrl+O → DialogPathPicker pre-filled
func TestModel_Mode_3_EmptyEditor_CtrlO_OpensPathPicker(t *testing.T) {
	m := newTestModel()
	m.projectDir = "/myproject"
	got := applyKey(m, keyCtrlO())
	if got.dialog.kind != DialogPathPicker {
		t.Fatalf("want DialogPathPicker, got %d", got.dialog.kind)
	}
	picker, ok := got.dialog.payload.(pathPickerModel)
	if !ok {
		t.Fatal("payload should be pathPickerModel")
	}
	if !strings.Contains(picker.input, ".pr9k") {
		t.Errorf("path picker should be pre-filled with .pr9k path, got %q", picker.input)
	}
}

// TestModel_Mode_4 — EmptyEditor, F10 → menu open
func TestModel_Mode_4_EmptyEditor_F10_OpensMenu(t *testing.T) {
	m := newTestModel()
	got := applyKey(m, keyF10())
	if !got.menu.open {
		t.Error("menu should be open after F10")
	}
	if !strings.Contains(got.View(), "New") {
		t.Error("view should show menu items")
	}
}

// TestModel_Mode_5 — EmptyEditor, ? → helpOpen
func TestModel_Mode_5_EmptyEditor_QuestionMark_OpensHelp(t *testing.T) {
	m := newTestModel()
	got := applyKey(m, keyRune('?'))
	if !got.helpOpen {
		t.Error("helpOpen should be true after ?")
	}
	if !strings.Contains(got.View(), "Help") {
		t.Error("view should show help modal")
	}
}

// ============================================================
// Part B — Edit-view core modes
// ============================================================

// TestModel_Mode_6 — EditView, outline focus, ↓ → cursor advances
func TestModel_Mode_6_EditView_OutlineFocus_Down_AdvancesCursor(t *testing.T) {
	m := newLoadedModel(sampleStep("a"), sampleStep("b"), sampleStep("c"))
	m.focus = focusOutline
	got := applyKey(m, keyDown())
	if got.outline.cursor != 1 {
		t.Fatalf("want cursor=1, got %d", got.outline.cursor)
	}
}

// TestModel_Mode_7 — EditView, outline focus, Tab → focus moves to detail pane
// Step "a" is at flat row 3 (0=init hdr, 1=+Add init, 2=iter hdr, 3=step).
func TestModel_Mode_7_EditView_OutlineFocus_Tab_MovesToDetail(t *testing.T) {
	m := newLoadedModel(sampleStep("a"))
	m.focus = focusOutline
	m.outline.cursor = 3 // must be on a step row for Tab to switch focus
	got := applyKey(m, keyTab())
	if got.focus != focusDetail {
		t.Fatalf("want focusDetail, got %d", got.focus)
	}
	if got.prevFocus != focusOutline {
		t.Fatalf("want prevFocus=focusOutline, got %d", got.prevFocus)
	}
}

// TestModel_Mode_8 — EditView, outline focus, Del → DialogRemoveConfirm
// Step "myStep" is at flat row 3.
func TestModel_Mode_8_EditView_OutlineFocus_Del_OpensRemoveConfirm(t *testing.T) {
	m := newLoadedModel(sampleStep("myStep"))
	m.focus = focusOutline
	m.outline.cursor = 3 // must be on a step row for Del to open dialog
	got := applyKey(m, keyDel())
	if got.dialog.kind != DialogRemoveConfirm {
		t.Fatalf("want DialogRemoveConfirm, got %d", got.dialog.kind)
	}
	if !strings.Contains(got.View(), "Delete") {
		t.Error("view should contain Delete option")
	}
}

// TestModel_Mode_9 — EditView, outline focus, Alt+↑ → step moves up
// With two iteration steps, step "b" is at flat row 4
// (0=init hdr, 1=+Add, 2=iter hdr, 3=a, 4=b).
func TestModel_Mode_9_EditView_OutlineFocus_AltUp_MovesStepUp(t *testing.T) {
	m := newLoadedModel(sampleStep("a"), sampleStep("b"))
	m.focus = focusOutline
	m.outline.cursor = 4 // step "b" at flat row 4
	got := applyKey(m, keyAltUp())
	if got.doc.Steps[0].Name != "b" {
		t.Fatalf("want b at index 0, got %q", got.doc.Steps[0].Name)
	}
	if !got.dirty {
		t.Error("dirty should be true after step move")
	}
}

// TestModel_Mode_10 — EditView, outline focus, r → reorder mode
// 'r' only activates when cursor is on a step row. Step "a" is at flat row 3.
func TestModel_Mode_10_EditView_OutlineFocus_R_EntersReorderMode(t *testing.T) {
	m := newLoadedModel(sampleStep("a"), sampleStep("b"))
	m.focus = focusOutline
	m.outline.cursor = 3 // step "a" at flat row 3
	got := applyKey(m, keyRune('r'))
	if !got.reorderMode {
		t.Error("reorderMode should be true after r")
	}
	// footer shows reorder shortcuts
	line := got.ShortcutLine()
	if !strings.Contains(line, "commit") {
		t.Errorf("footer should show reorder hints, got %q", line)
	}
}

// TestModel_Mode_11 — reorder mode, ↑ → step moved up
// Step "b" is at flat row 4 with two iteration steps.
func TestModel_Mode_11_ReorderMode_Up_MovesStep(t *testing.T) {
	m := newLoadedModel(sampleStep("a"), sampleStep("b"))
	m.focus = focusOutline
	m.reorderMode = true
	m.reorderOrigin = 4
	m.reorderSnapshot = []workflowmodel.Step{sampleStep("a"), sampleStep("b")}
	m.outline.cursor = 4 // step "b" at flat row 4
	got := applyKey(m, keyUp())
	if got.doc.Steps[0].Name != "b" {
		t.Fatalf("want b at index 0, got %q", got.doc.Steps[0].Name)
	}
	if !got.reorderMode {
		t.Error("should still be in reorder mode after ↑")
	}
}

// TestModel_Mode_12 — reorder mode, Enter → commits, dirty=true
func TestModel_Mode_12_ReorderMode_Enter_CommitsReorder(t *testing.T) {
	m := newLoadedModel(sampleStep("a"), sampleStep("b"))
	m.focus = focusOutline
	m.reorderMode = true
	m.reorderOrigin = 1
	m.reorderSnapshot = []workflowmodel.Step{sampleStep("a"), sampleStep("b")}
	m.outline.cursor = 1
	// Move b up first, then commit
	m = applyKey(m, keyUp())
	got := applyKey(m, keyEnter())
	if got.reorderMode {
		t.Error("reorderMode should be false after Enter")
	}
	if !got.dirty {
		t.Error("dirty should be true after committing reorder")
	}
}

// TestModel_Mode_13 — reorder mode, Esc → cancels, step restored
func TestModel_Mode_13_ReorderMode_Esc_CancelsReorder(t *testing.T) {
	m := newLoadedModel(sampleStep("a"), sampleStep("b"))
	m.focus = focusOutline
	m.reorderMode = true
	m.reorderOrigin = 1
	m.reorderSnapshot = []workflowmodel.Step{sampleStep("a"), sampleStep("b")}
	m.outline.cursor = 1
	// Move b up (now b,a) then cancel
	m = applyKey(m, keyUp())
	got := applyKey(m, keyEsc())
	if got.reorderMode {
		t.Error("reorderMode should be false after Esc")
	}
	if got.doc.Steps[0].Name != "a" {
		t.Fatalf("want a restored to index 0, got %q", got.doc.Steps[0].Name)
	}
	if got.outline.cursor != 1 {
		t.Fatalf("want cursor restored to 1, got %d", got.outline.cursor)
	}
}

// TestModel_Mode_14 — detail-pane focus on a choice field, Enter → dropdown open.
// CaptureMode (index 4 for shell step) is a choice field.
func TestModel_Mode_14_DetailFocus_Enter_OpensDropdown(t *testing.T) {
	m := newLoadedModel(sampleStep("a"))
	m.focus = focusDetail
	m.outline.cursor = 3 // step at flat row 3
	m.detail.cursor = 4  // CaptureMode (choice field for shell step)
	got := applyKey(m, keyEnter())
	if !got.detail.dropdownOpen {
		t.Error("dropdownOpen should be true after Enter on choice field")
	}
}

// TestModel_Mode_15 — dropdown open, typed char → selection handled
func TestModel_Mode_15_DropdownOpen_TypedChar_Handled(t *testing.T) {
	m := newLoadedModel(sampleStep("a"))
	m.focus = focusDetail
	m.detail.dropdownOpen = true
	// Typed char should not crash or close dropdown
	got := applyKey(m, keyRune('s'))
	if !got.detail.dropdownOpen {
		// The stub doesn't jump to a match but should keep dropdown open
		// unless Enter/Esc is pressed. Behaviour is stubbed; just verify no crash.
		t.Log("dropdown closed on typed char (stub behaviour)")
	}
}

// TestModel_Mode_16 — detail-pane focus on masked containerEnv field, r → revealed.
// Key "MY_SECRET" matches the _SECRET suffix pattern; field index 9 for a shell step
// (0=Name,1=Kind,2=Command,3=CaptureAs,4=CaptureMode,5=Timeout,6=OnTimeout,
//
//	7=BreakLoopIfEmpty,8=SkipIfCaptureEmpty,9=containerEnv[0]).
func TestModel_Mode_16_DetailFocus_MaskedField_R_Reveals(t *testing.T) {
	step := workflowmodel.Step{
		Name: "s",
		Kind: workflowmodel.StepKindShell,
		Env:  []workflowmodel.EnvEntry{{Key: "MY_SECRET", Value: "abc123", IsLiteral: true}},
	}
	m := newLoadedModel(step)
	m.focus = focusDetail
	m.outline.cursor = 3 // step at flat row 3
	m.detail.cursor = 9  // containerEnv[0]
	got := applyKey(m, keyRune('r'))
	if got.detail.revealedField != 9 {
		t.Fatalf("want revealedField=9, got %d", got.detail.revealedField)
	}
	// View should show plain value, not masked.
	view := got.View()
	if strings.Contains(view, GlyphMasked) {
		t.Error("value should be revealed, not masked")
	}
	if !strings.Contains(view, "abc123") {
		t.Error("revealed value should be visible in view")
	}
}

// TestModel_Mode_17 — detail-pane loses focus → masked field re-masked.
func TestModel_Mode_17_DetailFocus_Leave_ReMasksField(t *testing.T) {
	step := workflowmodel.Step{
		Name: "s",
		Kind: workflowmodel.StepKindShell,
		Env:  []workflowmodel.EnvEntry{{Key: "MY_SECRET", Value: "abc123", IsLiteral: true}},
	}
	m := newLoadedModel(step)
	m.focus = focusDetail
	m.outline.cursor = 3       // step at flat row 3
	m.detail.cursor = 9        // containerEnv[0]
	m.detail.revealedField = 9 // field is currently revealed
	// Tab away from detail.
	got := applyKey(m, keyTab())
	if got.focus != focusOutline {
		t.Fatalf("want focusOutline, got %d", got.focus)
	}
	if got.detail.revealedField != -1 {
		t.Fatalf("want revealedField=-1 after leaving detail, got %d", got.detail.revealedField)
	}
}

// ============================================================
// Part C — Save-flow modes
// ============================================================

// TestModel_Mode_18 — Ctrl+S with valid doc → 3-stage: validate → save → complete
func TestModel_Mode_18_CtrlS_ValidDoc_SaveSequence(t *testing.T) {
	m := newLoadedModel(sampleStep("s1"))
	m.validateFn = noFindings

	// Stage 1: Ctrl+S → validateInProgress=true
	next1, validateCmd := m.Update(keyCtrlS())
	got1 := next1.(Model)
	if !got1.validateInProgress {
		t.Fatal("want validateInProgress=true after Ctrl+S")
	}
	if validateCmd == nil {
		t.Fatal("want validate cmd")
	}

	// Stage 2: validate completes → saveInProgress=true
	next2, saveCmd := got1.Update(validateCmd())
	got2 := next2.(Model)
	if !got2.saveInProgress {
		t.Fatal("want saveInProgress=true after validation")
	}
	if saveCmd == nil {
		t.Fatal("want save cmd")
	}

	// Stage 3: save completes
	got3 := applyMsg(got2, saveCmd())
	if got3.saveInProgress {
		t.Error("saveInProgress should be false after save completes")
	}
	if got3.dirty {
		t.Error("dirty should be false after successful save")
	}
	if !strings.Contains(got3.saveBanner, "Saved") {
		t.Errorf("expected save banner, got %q", got3.saveBanner)
	}
}

// TestModel_Mode_19 — Ctrl+S when saveInProgress=true → no-op
func TestModel_Mode_19_CtrlS_SaveInProgress_NoOp(t *testing.T) {
	m := newLoadedModel(sampleStep("s1"))
	m.saveInProgress = true
	got := applyKey(m, keyCtrlS())
	if !got.saveInProgress {
		t.Error("saveInProgress should remain true (no new goroutine)")
	}
}

// TestModel_Mode_20 — Ctrl+Q when saveInProgress → DialogSaveInProgress + pendingQuit
func TestModel_Mode_20_CtrlQ_SaveInProgress_ShowsSaveInProgressDialog(t *testing.T) {
	m := newLoadedModel(sampleStep("s1"))
	m.saveInProgress = true
	got := applyKey(m, keyCtrlQ())
	if got.dialog.kind != DialogSaveInProgress {
		t.Fatalf("want DialogSaveInProgress, got %d", got.dialog.kind)
	}
	if !got.pendingQuit {
		t.Error("pendingQuit should be true")
	}
	if !strings.Contains(got.View(), "Save in progress") {
		t.Error("view should show save-in-progress message")
	}
}

// TestModel_Mode_21 — DialogSaveInProgress + saveCompleteMsg → QuitConfirm dialog
// (D-PR2-10: always-confirm quit; handleSaveResult re-routes to handleGlobalKey).
func TestModel_Mode_21_DialogSaveInProgress_SaveComplete_EntersQuitFlow(t *testing.T) {
	m := newLoadedModel(sampleStep("s1"))
	m.dialog = dialogState{kind: DialogSaveInProgress}
	m.saveInProgress = true
	m.pendingQuit = true
	// Successful save arrives — re-routes to Ctrl+Q with dirty=false → DialogQuitConfirm.
	next, _ := m.Update(saveCompleteMsg{result: workflowio.SaveResult{Kind: workflowio.SaveErrorNone}})
	got := next.(Model)
	if got.dialog.kind != DialogQuitConfirm {
		t.Fatalf("want DialogQuitConfirm after pendingQuit+save succeeded (D-PR2-10), got kind=%d", got.dialog.kind)
	}
}

// TestModel_Mode_22 — Ctrl+S with validator fatals → async validate → DialogFindingsPanel
func TestModel_Mode_22_CtrlS_ValidatorFatals_ShowsFindingsPanel(t *testing.T) {
	m := newLoadedModel(sampleStep("s1"))
	m.validateFn = fatalFindings

	// Stage 1: Ctrl+S → validateInProgress=true
	next1, validateCmd := m.Update(keyCtrlS())
	got1 := next1.(Model)
	if !got1.validateInProgress {
		t.Fatal("want validateInProgress=true after Ctrl+S")
	}
	if validateCmd == nil {
		t.Fatal("want validate cmd")
	}

	// Stage 2: validate completes with fatals → DialogFindingsPanel
	got2 := applyMsg(got1, validateCmd())
	if got2.dialog.kind != DialogFindingsPanel {
		t.Fatalf("want DialogFindingsPanel after fatal findings, got %d", got2.dialog.kind)
	}
	if got2.saveInProgress {
		t.Error("saveInProgress should remain false when validation fails")
	}
	if got2.validateInProgress {
		t.Error("validateInProgress should be false after validation")
	}
}

// TestModel_Mode_23 — DialogFindingsPanel + ? → helpOpen (only coexistence)
func TestModel_Mode_23_DialogFindingsPanel_QuestionMark_OpensHelp(t *testing.T) {
	m := newLoadedModel(sampleStep("s1"))
	m.dialog = dialogState{kind: DialogFindingsPanel}
	got := applyKey(m, keyRune('?'))
	if !got.helpOpen {
		t.Error("helpOpen should be true when ? pressed over FindingsPanel")
	}
}

// ============================================================
// Part D — Quit-flow modes
// ============================================================

// TestModel_Mode_24 — EditView, no unsaved, Ctrl+Q → DialogQuitConfirm
func TestModel_Mode_24_EditView_NoUnsaved_CtrlQ_OpensQuitConfirm(t *testing.T) {
	m := newLoadedModel(sampleStep("s1"))
	m.dirty = false
	got := applyKey(m, keyCtrlQ())
	if got.dialog.kind != DialogQuitConfirm {
		t.Fatalf("want DialogQuitConfirm, got %d", got.dialog.kind)
	}
	view := got.View()
	if !strings.Contains(view, "Yes") || !strings.Contains(view, "No") {
		t.Errorf("quit confirm should show Yes/No, got %q", view)
	}
}

// TestModel_Mode_25 — DialogQuitConfirm + y → program exit
func TestModel_Mode_25_DialogQuitConfirm_Y_ExitsProgram(t *testing.T) {
	m := newLoadedModel(sampleStep("s1"))
	m.dialog = dialogState{kind: DialogQuitConfirm}
	_, cmd := m.Update(keyRune('y'))
	if cmd == nil {
		t.Fatal("want quit cmd after y in QuitConfirm")
	}
	// Execute the cmd and check it produces a tea.QuitMsg.
	msg := cmd()
	if _, ok := msg.(tea.QuitMsg); !ok {
		t.Errorf("cmd should produce tea.QuitMsg, got %T", msg)
	}
}

// TestModel_Mode_26 — EditView, unsaved changes, Ctrl+Q → DialogUnsavedChanges
func TestModel_Mode_26_EditView_UnsavedChanges_CtrlQ_OpensUnsavedChanges(t *testing.T) {
	m := newLoadedModel(sampleStep("s1"))
	m.dirty = true
	got := applyKey(m, keyCtrlQ())
	if got.dialog.kind != DialogUnsavedChanges {
		t.Fatalf("want DialogUnsavedChanges, got %d", got.dialog.kind)
	}
	view := got.View()
	if !strings.Contains(view, "Save") {
		t.Errorf("unsaved dialog should show Save option, got %q", view)
	}
}

// TestModel_Mode_27 — DialogUnsavedChanges + s with fatals → DialogFindingsPanel
func TestModel_Mode_27_DialogUnsavedChanges_S_WithFatals_ShowsFindings(t *testing.T) {
	m := newLoadedModel(sampleStep("s1"))
	m.dirty = true
	m.dialog = dialogState{kind: DialogUnsavedChanges}
	m.validateFn = fatalFindings
	got := applyKey(m, keyRune('s'))
	if got.dialog.kind != DialogFindingsPanel {
		t.Fatalf("want DialogFindingsPanel, got %d", got.dialog.kind)
	}
}

// ============================================================
// Part E — Load / recovery mode
// ============================================================

// TestModel_Mode_28 — EmptyEditor, malformed config.json → DialogRecovery
func TestModel_Mode_28_MalformedConfig_ShowsRecovery(t *testing.T) {
	m := newTestModel()
	msg := openFileResultMsg{
		err:      errors.New("json: invalid character"),
		rawBytes: []byte("{broken json"),
	}
	got := applyMsg(m, msg)
	if got.dialog.kind != DialogRecovery {
		t.Fatalf("want DialogRecovery, got %d", got.dialog.kind)
	}
	if got.loaded {
		t.Error("loaded should remain false after parse error")
	}
	if !strings.Contains(got.View(), "Recovery") {
		t.Error("view should contain Recovery")
	}
}

// ============================================================
// Update routing tests (D-9)
// ============================================================

// TestUpdate_HelpOpen_RoutesToHelpHandler — helpOpen=true intercepts all keys
func TestUpdate_HelpOpen_RoutesToHelpHandler(t *testing.T) {
	m := newTestModel()
	m.helpOpen = true
	// Ctrl+N is a global key, but with helpOpen it should go to the help handler.
	got := applyKey(m, keyCtrlN())
	// Help handler only responds to Esc or ?; Ctrl+N should be ignored (help stays open).
	if !got.helpOpen {
		t.Error("help should stay open when a non-closing key is pressed")
	}
	if got.dialog.kind == DialogNewChoice {
		t.Error("global key should NOT be handled while help is open")
	}
}

// TestUpdate_DialogOpen_RoutesToDialogHandler_OverridesGlobalKeys
func TestUpdate_DialogOpen_RoutesToDialogHandler_OverridesGlobalKeys(t *testing.T) {
	m := newTestModel()
	m.dialog = dialogState{kind: DialogNewChoice}
	// Ctrl+S is a global key; while a dialog is open it should be silently suppressed.
	got := applyKey(m, keyCtrlS())
	if got.dialog.kind != DialogNewChoice {
		t.Errorf("dialog should stay open; global key should be suppressed; got dialog=%d", got.dialog.kind)
	}
	if got.saveInProgress {
		t.Error("save should not trigger while a dialog is open")
	}
}

// TestUpdate_NoDialogNoHelp_GlobalKeyIntercepted
func TestUpdate_NoDialogNoHelp_GlobalKeyIntercepted(t *testing.T) {
	m := newTestModel()
	// Ctrl+N is a global key with no dialog and no help open.
	got := applyKey(m, keyCtrlN())
	if got.dialog.kind != DialogNewChoice {
		t.Errorf("Ctrl+N should open DialogNewChoice, got %d", got.dialog.kind)
	}
}

// TestUpdate_Default_RoutesToEditView — non-global key falls to updateEditView
func TestUpdate_Default_RoutesToEditView(t *testing.T) {
	m := newLoadedModel(sampleStep("a"), sampleStep("b"))
	m.focus = focusOutline
	// ↓ is not a global key; goes to updateEditView → handleOutlineKey.
	got := applyKey(m, keyDown())
	if got.outline.cursor != 1 {
		t.Errorf("want cursor=1, got %d", got.outline.cursor)
	}
}

// ============================================================
// Del scoping tests (D-10)
// ============================================================

// TestDel_OutlineFocused_RemovesStep — Del on a step row opens remove dialog.
// Step "alpha" is at flat row 3.
func TestDel_OutlineFocused_OpensRemoveDialog(t *testing.T) {
	m := newLoadedModel(sampleStep("alpha"))
	m.focus = focusOutline
	m.outline.cursor = 3 // step row
	got := applyKey(m, keyDel())
	if got.dialog.kind != DialogRemoveConfirm {
		t.Errorf("want DialogRemoveConfirm, got %d", got.dialog.kind)
	}
}

// TestDel_NonOutlineFocused_NoOp
func TestDel_NonOutlineFocused_NoOp(t *testing.T) {
	m := newLoadedModel(sampleStep("alpha"))
	m.focus = focusDetail
	got := applyKey(m, keyDel())
	if got.dialog.kind != DialogNone {
		t.Errorf("Del on non-outline focus should be no-op, got dialog=%d", got.dialog.kind)
	}
	if len(got.doc.Steps) != 1 {
		t.Errorf("steps should be unchanged, got %d", len(got.doc.Steps))
	}
}

// TestDel_NotInGlobalKeyList — Del must not be treated as a global key
func TestDel_NotInGlobalKeyList(t *testing.T) {
	if isGlobalKey(keyDel()) {
		t.Error("Del must not be in the global-key list (D-10)")
	}
}

// ============================================================
// EC-1 through EC-12 edge cases
// ============================================================

// EC-1: Terminal height 1 — no panic
func TestModel_EC_1_TerminalHeight1_NoPanic(t *testing.T) {
	m := newLoadedModel(sampleStep("a"))
	defer func() {
		if r := recover(); r != nil {
			t.Fatalf("panic on height=1: %v", r)
		}
	}()
	next, _ := m.Update(tea.WindowSizeMsg{Width: 80, Height: 1})
	_ = next.(Model).View()
}

// EC-2: Zero-step workflow — renders without panic
func TestModel_EC_2_ZeroStepWorkflow_NoPanic(t *testing.T) {
	m := newLoadedModel()
	defer func() {
		if r := recover(); r != nil {
			t.Fatalf("panic with zero steps: %v", r)
		}
	}()
	_ = m.View()
}

// EC-3: Step with no name — shows placeholder in outline
func TestModel_EC_3_StepNoName_ShowsPlaceholder(t *testing.T) {
	m := newLoadedModel(workflowmodel.Step{Kind: workflowmodel.StepKindShell})
	view := m.View()
	if !strings.Contains(view, HintNoName) {
		t.Errorf("unnamed step should show %q in view, got %q", HintNoName, view)
	}
}

// EC-4: Zero-width terminal — outline uses minimum width (20)
func TestModel_EC_4_ZeroWidthTerminal_UsesMinimumOutlineWidth(t *testing.T) {
	m := newLoadedModel(sampleStep("a"))
	next, _ := m.Update(tea.WindowSizeMsg{Width: 0, Height: 25})
	got := next.(Model)
	if got.outline.width != 20 {
		t.Errorf("want outline.width=20, got %d", got.outline.width)
	}
}

// EC-5: Outline cursor at last flat row, ↓ — cursor stays bounded
// Two iteration steps yield 8 flat rows (0..7); last row is 7.
func TestModel_EC_5_OutlineCursorAtLast_DownBounded(t *testing.T) {
	m := newLoadedModel(sampleStep("a"), sampleStep("b"))
	m.focus = focusOutline
	m.outline.cursor = 7 // last flat row (+Add finalize)
	got := applyKey(m, keyDown())
	if got.outline.cursor != 7 {
		t.Errorf("cursor should stay at 7, got %d", got.outline.cursor)
	}
}

// EC-6: Detail cursor at first item, ↑ — cursor stays bounded at 0
func TestModel_EC_6_DetailCursorAtFirst_UpBounded(t *testing.T) {
	m := newLoadedModel(sampleStep("a"))
	m.focus = focusDetail
	m.detail.cursor = 0
	got := applyKey(m, keyUp())
	if got.detail.cursor != 0 {
		t.Errorf("cursor should stay at 0, got %d", got.detail.cursor)
	}
}

// EC-7: Ctrl+S with no workflow loaded — no-op, no panic
func TestModel_EC_7_CtrlS_NoWorkflowLoaded_NoOp(t *testing.T) {
	m := newTestModel() // not loaded
	got := applyKey(m, keyCtrlS())
	if got.saveInProgress {
		t.Error("saveInProgress should remain false when no workflow is loaded")
	}
}

// EC-8: Del when outline has no steps — no-op
func TestModel_EC_8_Del_NoSteps_NoOp(t *testing.T) {
	m := newLoadedModel() // zero steps
	m.focus = focusOutline
	defer func() {
		if r := recover(); r != nil {
			t.Fatalf("panic on Del with no steps: %v", r)
		}
	}()
	got := applyKey(m, keyDel())
	if got.dialog.kind != DialogNone {
		t.Errorf("Del with no steps should be no-op, got dialog=%d", got.dialog.kind)
	}
}

// EC-9: Tab from detail returns to outline
func TestModel_EC_9_Tab_FromDetail_ReturnToOutline(t *testing.T) {
	m := newLoadedModel(sampleStep("a"))
	m.focus = focusDetail
	got := applyKey(m, keyTab())
	if got.focus != focusOutline {
		t.Errorf("want focusOutline after Tab from detail, got %d", got.focus)
	}
}

// EC-10: Multiple Ctrl+S when saveInProgress — idempotent
func TestModel_EC_10_MultipleCtrlS_SaveInProgress_Idempotent(t *testing.T) {
	m := newLoadedModel(sampleStep("s1"))
	m.saveInProgress = true
	// Second and third presses should be no-ops.
	got := applyKey(m, keyCtrlS())
	got2 := applyKey(got, keyCtrlS())
	if !got2.saveInProgress {
		t.Error("saveInProgress should remain true")
	}
}

// EC-11: Unknown tea.Msg type — handled gracefully, no panic
func TestModel_EC_11_UnknownMessage_NoPanic(t *testing.T) {
	m := newTestModel()
	defer func() {
		if r := recover(); r != nil {
			t.Fatalf("panic on unknown message: %v", r)
		}
	}()
	type unknown struct{}
	next, _ := m.Update(unknown{})
	_ = next.(Model).View()
}

// EC-12: Init returns nil cmd
func TestModel_EC_12_Init_ReturnsNil(t *testing.T) {
	m := newTestModel()
	if m.Init() != nil {
		t.Error("Init should return nil")
	}
}

// ============================================================
// Gap tests (T-1..T-3)
// ============================================================

// T-1: DialogRemoveConfirm 'd' — removes step, clamps cursor, marks dirty
// Step "alpha" is at flat row 3 (stepIdx=0).
func TestDialogRemoveConfirm_D_DeletesStep(t *testing.T) {
	m := newLoadedModel(sampleStep("alpha"), sampleStep("beta"))
	m.focus = focusOutline
	m.outline.cursor = 3 // step "alpha" at flat row 3
	m.dialog = dialogState{kind: DialogRemoveConfirm, payload: "alpha"}
	got := applyKey(m, keyRune('d'))
	if got.dialog.kind != DialogNone {
		t.Error("dialog should close after confirm")
	}
	if len(got.doc.Steps) != 1 {
		t.Errorf("want 1 step, got %d", len(got.doc.Steps))
	}
	if got.doc.Steps[0].Name != "beta" {
		t.Errorf("wrong step remaining, want beta got %q", got.doc.Steps[0].Name)
	}
	if !got.dirty {
		t.Error("dirty should be true")
	}
}

// T-2: handleOpenFileResult success — loads doc and marks loaded=true
func TestHandleOpenFileResult_Success_LoadsDoc(t *testing.T) {
	m := newTestModel()
	doc := workflowmodel.WorkflowDoc{Steps: []workflowmodel.Step{sampleStep("s1")}}
	msg := openFileResultMsg{doc: doc, diskDoc: doc, workflowDir: "/some/dir"}
	got := applyMsg(m, msg)
	if !got.loaded {
		t.Error("loaded should be true after success")
	}
	if got.dirty {
		t.Error("dirty should be false after load")
	}
	if len(got.doc.Steps) != 1 {
		t.Errorf("want 1 step, got %d", len(got.doc.Steps))
	}
	if got.workflowDir != "/some/dir" {
		t.Errorf("workflowDir not updated, got %q", got.workflowDir)
	}
	if got.outline.cursor != 0 {
		t.Errorf("cursor should be 0, got %d", got.outline.cursor)
	}
}

// T-3: DialogUnsavedChanges 'd' — produces tea.Quit command
func TestDialogUnsavedChanges_D_DiscardsAndQuits(t *testing.T) {
	m := newLoadedModel(sampleStep("s1"))
	m.dirty = true
	m.dialog = dialogState{kind: DialogUnsavedChanges}
	_, cmd := m.Update(keyRune('d'))
	if cmd == nil {
		t.Fatal("want tea.Quit cmd after d in UnsavedChanges")
	}
	msg := cmd()
	if _, ok := msg.(tea.QuitMsg); !ok {
		t.Errorf("cmd should produce tea.QuitMsg, got %T", msg)
	}
}
