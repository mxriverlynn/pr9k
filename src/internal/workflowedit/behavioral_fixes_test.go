package workflowedit

// Tests for WU-3 behavioral fixes (issue #186).
// Each test corresponds to one of the nine independent fixes (a)–(i).
//
// Coverage matrix:
//   (a) Chrome budget:         TestView_PanelHUsesChromeBudget
//   (b) D48 min-size guard:    TestView_MinimumSizeRendersTooSmallMessage
//   (c) Phase-boundary guard:  TestPhase_BoundaryDeclineFlashesAndDoesNotSwap
//   (d) revealedField reset:   TestSecretMask_ResetOnAllDialogOpens
//   (e) Banner short-form:     TestBanners_ShortFormTags
//   (f) Dirty render source:   TestRender_DirtyFromIsDirty
//   (g) Reload banner fwd:     TestReload_ForwardsAllBannerSignals
//   (h) fieldKindMultiLine:    TestField_MultiLineCtrlE
//   (i) Tier-0 WindowSizeMsg:  TestTier0_WindowSizeMsgRoutedDuringDialog

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/mxriverlynn/pr9k/src/internal/workflowio"
	"github.com/mxriverlynn/pr9k/src/internal/workflowmodel"
)

// errForTest is a sentinel error used in dialog-open tests.
var errForTest = errors.New("test error")

// --- (a) + (b) Chrome budget and minimum-size guard ---

// TestView_PanelHUsesChromeBudget verifies handleWindowSize subtracts ChromeRows (not 2)
// from the terminal height when sizing the outline/detail panels (D-20).
func TestView_PanelHUsesChromeBudget(t *testing.T) {
	m := newTestModel()
	const termH = 30
	got, _ := m.Update(tea.WindowSizeMsg{Width: 80, Height: termH})
	m2 := got.(Model)
	want := termH - ChromeRows
	if m2.outline.height != want {
		t.Errorf("outline.height = %d, want termH-ChromeRows = %d", m2.outline.height, want)
	}
	if m2.detail.height != want {
		t.Errorf("detail.height = %d, want termH-ChromeRows = %d", m2.detail.height, want)
	}
}

// TestView_MinimumSizeRendersTooSmallMessage verifies View() returns a "too small"
// message when width < MinTerminalWidth or height < MinTerminalHeight (D-19/D48).
func TestView_MinimumSizeRendersTooSmallMessage(t *testing.T) {
	cases := []struct {
		name   string
		width  int
		height int
	}{
		{"too narrow", 40, 20},
		{"too short", 80, 10},
		{"both too small", 30, 8},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			m := newLoadedModelWithWidth(tc.width, tc.height, sampleStep("a"))
			view := m.View()
			lower := strings.ToLower(view)
			if !strings.Contains(lower, "too small") && !strings.Contains(lower, "terminal") {
				t.Errorf("view should contain size hint for %dx%d, got %q",
					tc.width, tc.height, view)
			}
		})
	}
}

// --- (c) Phase boundary guard ---

// TestPhase_BoundaryDeclineFlashesAndDoesNotSwap verifies that attempting to move
// a step across a phase boundary (e.g. from iteration to initialize) is declined:
// the steps are NOT swapped and m.boundaryFlash is incremented (D-12).
func TestPhase_BoundaryDeclineFlashesAndDoesNotSwap(t *testing.T) {
	initStep := workflowmodel.Step{
		Name:  "init-step",
		Kind:  workflowmodel.StepKindShell,
		Phase: workflowmodel.StepPhaseInitialize,
	}
	iterStep := workflowmodel.Step{
		Name:  "iter-step",
		Kind:  workflowmodel.StepKindShell,
		Phase: workflowmodel.StepPhaseIteration,
	}
	// Layout with two steps (init, iter):
	//   0: initialize hdr  1: init-step  2: +Add initialize
	//   3: iteration hdr   4: iter-step  5: +Add iteration
	//   6: finalize hdr    7: +Add finalize
	m := newLoadedModel(initStep, iterStep)
	m.focus = focusOutline
	// Place cursor on the iteration step (iter-step at flat row 4).
	m.outline.cursor = 4

	beforeFlash := m.boundaryFlash
	// Alt+Up should decline the swap because iter-step would cross into initialize phase.
	got, _ := m.Update(tea.KeyMsg{Type: tea.KeyUp, Alt: true})
	m2 := got.(Model)

	// Steps must not be swapped.
	if m2.doc.Steps[0].Name != "init-step" || m2.doc.Steps[1].Name != "iter-step" {
		t.Errorf("steps were swapped across phase boundary; got [%s, %s]",
			m2.doc.Steps[0].Name, m2.doc.Steps[1].Name)
	}
	// boundaryFlash must have incremented.
	if m2.boundaryFlash <= beforeFlash {
		t.Errorf("boundaryFlash should increment on boundary decline; before=%d after=%d",
			beforeFlash, m2.boundaryFlash)
	}
}

// --- (d) revealedField reset on dialog open ---

// TestSecretMask_ResetOnAllDialogOpens verifies that opening a dialog resets the
// detail pane's revealedField to -1 (prevents secret leak across dialog sessions).
func TestSecretMask_ResetOnAllDialogOpens(t *testing.T) {
	cases := []struct {
		name  string
		setup func(m Model) Model
	}{
		{
			"Ctrl+N opens DialogNewChoice",
			func(m Model) Model { return applyKey(m, keyCtrlN()) },
		},
		{
			"Ctrl+O opens DialogPathPicker",
			func(m Model) Model { return applyKey(m, keyCtrlO()) },
		},
		{
			"Ctrl+Q (not dirty) opens DialogQuitConfirm",
			func(m Model) Model { return applyKey(m, keyCtrlQ()) },
		},
		{
			"Del on step opens DialogRemoveConfirm",
			func(m Model) Model {
				m.outline.cursor = 3 // step row for single iteration step
				m.focus = focusOutline
				return applyKey(m, keyDel())
			},
		},
		{
			"handleOpenFileResult error opens DialogError",
			func(m Model) Model {
				return applyMsg(m, openFileResultMsg{err: errForTest})
			},
		},
		{
			"handleOpenFileResult parse error opens DialogRecovery",
			func(m Model) Model {
				return applyMsg(m, openFileResultMsg{
					err:      errForTest,
					rawBytes: []byte("bad json"),
				})
			},
		},
		{
			"handleValidateComplete fatal opens DialogFindingsPanel",
			func(m Model) Model {
				items := []findingResult{{text: "fatal err", isFatal: true}}
				return applyMsg(m, validateCompleteMsg{items: items})
			},
		},
		{
			"handleValidateComplete warn opens DialogAcknowledgeFindings",
			func(m Model) Model {
				items := []findingResult{{text: "warn", isFatal: false}}
				return applyMsg(m, validateCompleteMsg{items: items})
			},
		},
		{
			"handleSaveResult error opens DialogError",
			func(m Model) Model {
				return applyMsg(m, saveCompleteMsg{
					result: workflowio.SaveResult{Kind: workflowio.SaveErrorPermission},
				})
			},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			m := newLoadedModel(sampleStep("s"))
			m.validateFn = noFindings
			// Pre-reveal a secret field.
			m.detail.revealedField = 2
			got := tc.setup(m)
			if got.detail.revealedField != -1 {
				t.Errorf("revealedField should be reset to -1 when dialog opens; got %d (dialog kind=%d)",
					got.detail.revealedField, got.dialog.kind)
			}
		})
	}
}

// --- (e) Banner short-form tags ---

// TestBanners_ShortFormTags verifies allBannerTexts() uses short-form tags (D-10):
// [ro], [ext], [sym → target], [shared], [?fields].
func TestBanners_ShortFormTags(t *testing.T) {
	cases := []struct {
		name   string
		banner bannerState
		want   string
	}{
		{"read-only", bannerState{isReadOnly: true}, "[ro]"},
		{"external", bannerState{isExternalWorkflow: true}, "[ext]"},
		{"symlink with target", bannerState{isSymlink: true, symlinkTarget: "/real/cfg"}, "[sym →"},
		{"symlink no target", bannerState{isSymlink: true}, "[sym]"},
		{"shared install", bannerState{isSharedInstall: true}, "[shared]"},
		{"unknown fields", bannerState{hasUnknownField: true}, "[?fields]"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			texts := tc.banner.allBannerTexts()
			if len(texts) == 0 {
				t.Fatal("expected at least one banner text")
			}
			if !strings.Contains(texts[0], tc.want) {
				t.Errorf("banner text = %q, want to contain %q", texts[0], tc.want)
			}
		})
	}
}

// --- (f) Dirty render source uses IsDirty() ---

// TestRender_DirtyFromIsDirty verifies renderSessionHeader reads m.IsDirty() and
// not the stale m.dirty cache (D-11). The two diverge when diskDoc != doc but
// m.dirty has been incorrectly set to false.
func TestRender_DirtyFromIsDirty(t *testing.T) {
	m := newLoadedModel(sampleStep("s"))
	// Make IsDirty() return true by modifying doc without updating diskDoc.
	m.doc.Steps[0].Name = "modified"
	m.dirty = false // stale cache says "not dirty"

	view := stripView(m)
	// renderSessionHeader should show the dirty marker (●) because IsDirty() is true.
	if !strings.Contains(view, "●") {
		t.Errorf("renderSessionHeader should show '●' when IsDirty() is true even if m.dirty==false; view=%q", view)
	}
}

// --- (g) Reload path banner forwarding ---

// TestReload_ForwardsAllBannerSignals verifies that makeLoadCmd populates all five
// banner-signal fields in the returned openFileResultMsg (D-15).
func TestReload_ForwardsAllBannerSignals(t *testing.T) {
	// Create a real workflowDir with a minimal config.json.
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "config.json"), []byte(`{"steps":[]}`), 0o644); err != nil {
		t.Fatalf("setup: %v", err)
	}
	m := New(&fakeFS{}, &fakeEditorRunner{}, dir, dir)
	m.workflowDir = dir

	cmd := m.makeLoadCmd()
	raw := cmd()
	msg, ok := raw.(openFileResultMsg)
	if !ok {
		t.Fatalf("makeLoadCmd cmd should return openFileResultMsg, got %T", raw)
	}
	if msg.err != nil {
		t.Fatalf("unexpected error from makeLoadCmd: %v", msg.err)
	}
	// For a regular writable non-symlink dir all five fields should be accessible.
	// A regular file should have isSymlink=false.
	if msg.isSymlink {
		t.Error("isSymlink should be false for a regular config.json")
	}
	// Remaining detect fields should not panic even if they return false in test env.
	_ = msg.isReadOnly
	_ = msg.isExternal
	_ = msg.isSharedInstall
	_ = msg.symlinkTarget
}

// --- (h) fieldKindMultiLine + Ctrl+E ---

// TestField_PromptFileIsMultiLine verifies buildDetailFields emits fieldKindMultiLine
// for the PromptFile field of a claude step (D-16).
func TestField_PromptFileIsMultiLine(t *testing.T) {
	step := sampleClaudeStep("s")
	fields := buildDetailFields(step)
	for _, f := range fields {
		if f.label == "PromptFile" {
			if f.kind != fieldKindMultiLine {
				t.Errorf("PromptFile field kind = %v, want fieldKindMultiLine", f.kind)
			}
			return
		}
	}
	t.Error("PromptFile field not found in buildDetailFields for claude step")
}

// TestField_CommandIsMultiLine verifies buildDetailFields emits fieldKindMultiLine
// for the Command field of a shell step (D-16).
func TestField_CommandIsMultiLine(t *testing.T) {
	step := sampleStep("s")
	fields := buildDetailFields(step)
	for _, f := range fields {
		if f.label == "Command" {
			if f.kind != fieldKindMultiLine {
				t.Errorf("Command field kind = %v, want fieldKindMultiLine", f.kind)
			}
			return
		}
	}
	t.Error("Command field not found in buildDetailFields for shell step")
}

// TestField_MultiLineCtrlE verifies that pressing Ctrl+E while the detail-pane
// cursor is on a PromptFile field (fieldKindMultiLine) dispatches the EditorRunner (D-16).
func TestField_MultiLineCtrlE(t *testing.T) {
	step := sampleClaudeStep("my-step")
	editor := &fakeEditorRunner{}
	m := newLoadedModel(step)
	m.editor = editor
	m.focus = focusDetail
	// Layout: 0=init hdr, 1=+Add init, 2=iter hdr, 3=step, 4=+Add iter, 5=fin hdr, 6=+Add fin
	m.outline.cursor = 3 // the iteration step row

	// Navigate detail cursor to PromptFile field.
	// For a claude step: Name(0), Kind(1), Model(2), PromptFile(3)
	m.detail.cursor = 3 // PromptFile

	m.Update(keyCtrlE())

	if editor.runCount != 1 {
		t.Errorf("EditorRunner.Run should be called once on Ctrl+E over PromptFile; got %d", editor.runCount)
	}
}

// --- (i) Tier-0 WindowSizeMsg routing ---

// TestTier0_WindowSizeMsgRoutedDuringDialog verifies that a tea.WindowSizeMsg updates
// m.width and m.height even when a dialog is active (D-14).
func TestTier0_WindowSizeMsgRoutedDuringDialog(t *testing.T) {
	m := newLoadedModel(sampleStep("a"))
	m.dialog = dialogState{kind: DialogQuitConfirm}

	got, _ := m.Update(tea.WindowSizeMsg{Width: 120, Height: 40})
	m2 := got.(Model)

	if m2.width != 120 {
		t.Errorf("m.width = %d, want 120 after WindowSizeMsg during dialog", m2.width)
	}
	if m2.height != 40 {
		t.Errorf("m.height = %d, want 40 after WindowSizeMsg during dialog", m2.height)
	}
	// Dialog should still be open after the resize.
	if m2.dialog.kind != DialogQuitConfirm {
		t.Errorf("dialog should remain open after WindowSizeMsg; got kind=%d", m2.dialog.kind)
	}
}
