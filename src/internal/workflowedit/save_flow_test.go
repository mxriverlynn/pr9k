package workflowedit

import (
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/mxriverlynn/pr9k/src/internal/workflowio"
	"github.com/mxriverlynn/pr9k/src/internal/workflowmodel"
)

// TestSave_ThreeStageStateMachine — idle → validating → saving → idle transitions
func TestSave_ThreeStageStateMachine(t *testing.T) {
	m := newLoadedModel(sampleStep("s1"))
	m.validateFn = noFindings

	// Stage 1: Ctrl+S → validateInProgress=true
	next1, validateCmd := m.Update(keyCtrlS())
	m1 := next1.(Model)
	if !m1.validateInProgress {
		t.Fatal("want validateInProgress=true after Ctrl+S")
	}
	if m1.saveInProgress {
		t.Error("saveInProgress should be false during validation")
	}
	if validateCmd == nil {
		t.Fatal("validate cmd should not be nil")
	}

	// Stage 2: validate completes → saveInProgress=true
	validateMsg := validateCmd()
	next2, saveCmd := m1.Update(validateMsg)
	m2 := next2.(Model)
	if m2.validateInProgress {
		t.Error("validateInProgress should be false after validate completes")
	}
	if !m2.saveInProgress {
		t.Fatal("want saveInProgress=true after zero-findings validate")
	}
	if saveCmd == nil {
		t.Fatal("save cmd should not be nil")
	}

	// Stage 3: save completes → idle
	saveMsg := saveCmd()
	m3 := applyMsg(m2, saveMsg)
	if m3.saveInProgress {
		t.Error("saveInProgress should be false after save completes")
	}
	if m3.validateInProgress {
		t.Error("validateInProgress should be false after save completes")
	}
	if m3.dirty {
		t.Error("dirty should be false after successful save")
	}
}

// TestSave_DoubleSaveSuppressed — saveInProgress or validateInProgress silences Ctrl+S
func TestSave_DoubleSaveSuppressed(t *testing.T) {
	t.Run("saveInProgress", func(t *testing.T) {
		m := newLoadedModel(sampleStep("s1"))
		m.saveInProgress = true
		next, cmd := m.Update(keyCtrlS())
		got := next.(Model)
		if !got.saveInProgress {
			t.Error("saveInProgress should remain true (no new goroutine)")
		}
		if cmd != nil {
			t.Error("should not dispatch a cmd when saveInProgress")
		}
	})
	t.Run("validateInProgress", func(t *testing.T) {
		m := newLoadedModel(sampleStep("s1"))
		m.validateInProgress = true
		next, cmd := m.Update(keyCtrlS())
		got := next.(Model)
		if !got.validateInProgress {
			t.Error("validateInProgress should remain true (no new goroutine)")
		}
		if cmd != nil {
			t.Error("should not dispatch a cmd when validateInProgress")
		}
	})
}

// TestSave_SaveSnapshot_NilOnSessionTransition — File > Open resets snapshot to nil (F-98)
func TestSave_SaveSnapshot_NilOnSessionTransition(t *testing.T) {
	m := newLoadedModel(sampleStep("s1"))
	m.saveSnapshot = &workflowio.SaveSnapshot{ModTime: time.Now(), Size: 100}

	// Deliver openFileResultMsg (simulates File > Open)
	msg := openFileResultMsg{
		doc:     workflowmodel.WorkflowDoc{Steps: []workflowmodel.Step{sampleStep("new")}},
		diskDoc: workflowmodel.WorkflowDoc{Steps: []workflowmodel.Step{sampleStep("new")}},
	}
	got := applyMsg(m, msg)
	if got.saveSnapshot != nil {
		t.Error("saveSnapshot should be nil after session transition (F-98)")
	}
}

// TestSave_SaveSnapshot_UpdatedAfterSuccess — saveSnapshot set from result after successful save
func TestSave_SaveSnapshot_UpdatedAfterSuccess(t *testing.T) {
	m := newLoadedModel(sampleStep("s1"))
	ts := time.Now()
	snapshot := &workflowio.SaveSnapshot{ModTime: ts, Size: 42}

	msg := saveCompleteMsg{result: workflowio.SaveResult{
		Kind:     workflowio.SaveErrorNone,
		Snapshot: snapshot,
	}}
	got := applyMsg(m, msg)
	if got.saveSnapshot == nil {
		t.Fatal("saveSnapshot should be set after successful save")
	}
	if !got.saveSnapshot.ModTime.Equal(ts) {
		t.Error("saveSnapshot.ModTime should equal the result snapshot's ModTime")
	}
	if got.saveSnapshot.Size != 42 {
		t.Errorf("saveSnapshot.Size should be 42, got %d", got.saveSnapshot.Size)
	}
}

// TestSave_ConflictDetected_OpensDialog — mtime mismatch at save time → DialogFileConflict
func TestSave_ConflictDetected_OpensDialog(t *testing.T) {
	t1 := time.Now()
	t2 := t1.Add(time.Second) // disk has different mtime

	fs := &fakeFS{info: fakeFileInfo{modTime: t2}}
	m := New(fs, &fakeEditorRunner{}, "/proj", "/wf")
	m.doc.Steps = []workflowmodel.Step{sampleStep("s1")}
	m.diskDoc.Steps = []workflowmodel.Step{sampleStep("s1")}
	m.loaded = true
	m.saveSnapshot = &workflowio.SaveSnapshot{ModTime: t1} // model expects t1, disk has t2

	got := applyKey(m, keyCtrlS())
	if got.dialog.kind != DialogFileConflict {
		t.Fatalf("want DialogFileConflict when mtime mismatches, got %d", got.dialog.kind)
	}
	if got.validateInProgress {
		t.Error("validateInProgress should be false when conflict detected")
	}
}

// TestSave_ZeroFindings_ProceedsToSave — zero findings after async validate → save starts
func TestSave_ZeroFindings_ProceedsToSave(t *testing.T) {
	m := newLoadedModel(sampleStep("s1"))
	m.validateFn = noFindings

	next1, validateCmd := m.Update(keyCtrlS())
	m1 := next1.(Model)
	if validateCmd == nil {
		t.Fatal("want validate cmd after Ctrl+S")
	}

	next2, saveCmd := m1.Update(validateCmd())
	m2 := next2.(Model)
	if !m2.saveInProgress {
		t.Fatal("want saveInProgress=true after zero-findings validate")
	}
	if saveCmd == nil {
		t.Fatal("want save cmd after zero findings")
	}
}

// TestSave_FatalFindings_OpensFindingsPanelAndBlocksSave — fatal findings block save
func TestSave_FatalFindings_OpensFindingsPanelAndBlocksSave(t *testing.T) {
	m := newLoadedModel(sampleStep("s1"))
	m.validateFn = fatalFindings

	next1, validateCmd := m.Update(keyCtrlS())
	m1 := next1.(Model)
	if validateCmd == nil {
		t.Fatal("want validate cmd")
	}

	got := applyMsg(m1, validateCmd())
	if got.dialog.kind != DialogFindingsPanel {
		t.Fatalf("want DialogFindingsPanel after fatal findings, got %d", got.dialog.kind)
	}
	if got.saveInProgress {
		t.Error("saveInProgress should be false when validation has fatals")
	}
	if got.validateInProgress {
		t.Error("validateInProgress should be false after validate completes")
	}
}

// TestSave_WarnOnly_OpensAcknowledgmentDialog — warn-only findings open acknowledgment dialog
func TestSave_WarnOnly_OpensAcknowledgmentDialog(t *testing.T) {
	m := newLoadedModel(sampleStep("s1"))
	m.validateFn = warnFindings

	next1, validateCmd := m.Update(keyCtrlS())
	m1 := next1.(Model)
	if validateCmd == nil {
		t.Fatal("want validate cmd")
	}

	got := applyMsg(m1, validateCmd())
	if got.dialog.kind != DialogAcknowledgeFindings {
		t.Fatalf("want DialogAcknowledgeFindings for warn-only findings, got %d", got.dialog.kind)
	}
	if got.saveInProgress {
		t.Error("saveInProgress should be false during acknowledgment")
	}
}

// TestSave_ValidatorDeepCopyNotSharedWithDoc — mutations after Ctrl+S don't affect validation input
func TestSave_ValidatorDeepCopyNotSharedWithDoc(t *testing.T) {
	m := newLoadedModel(sampleStep("original"))
	var capturedName string
	m.validateFn = func(doc workflowmodel.WorkflowDoc, _ string, _ map[string][]byte) []findingResult {
		if len(doc.Steps) > 0 {
			capturedName = doc.Steps[0].Name
		}
		return nil
	}

	// Ctrl+S captures deep copy
	next, validateCmd := m.Update(keyCtrlS())
	got := next.(Model)
	// Mutate the model's doc BEFORE the cmd runs
	if len(got.doc.Steps) > 0 {
		got.doc.Steps[0].Name = "mutated"
	}

	// Execute the validate cmd — should use the original "original" name
	validateCmd()
	if capturedName != "original" {
		t.Errorf("validate cmd should use deep copy; want name 'original', got %q", capturedName)
	}
}

// TestCtrlQDuringSave_SetsPendingQuitAndOpensDialog — Ctrl+Q during save sets pendingQuit (F-97)
func TestCtrlQDuringSave_SetsPendingQuitAndOpensDialog(t *testing.T) {
	m := newLoadedModel(sampleStep("s1"))
	m.saveInProgress = true

	got := applyKey(m, keyCtrlQ())
	if got.dialog.kind != DialogSaveInProgress {
		t.Fatalf("want DialogSaveInProgress when Ctrl+Q during save, got %d", got.dialog.kind)
	}
	if !got.pendingQuit {
		t.Error("pendingQuit should be true")
	}
}

// TestSaveComplete_WithPendingQuit_ReentersQuitFlow — save+pendingQuit → quit flow (F-97)
func TestSaveComplete_WithPendingQuit_ReentersQuitFlow(t *testing.T) {
	m := newLoadedModel(sampleStep("s1"))
	m.dialog = dialogState{kind: DialogSaveInProgress}
	m.saveInProgress = true
	m.pendingQuit = true

	snapshot := &workflowio.SaveSnapshot{ModTime: time.Now(), Size: 10}
	_, cmd := m.Update(saveCompleteMsg{result: workflowio.SaveResult{
		Kind:     workflowio.SaveErrorNone,
		Snapshot: snapshot,
	}})
	if cmd == nil {
		t.Fatal("want tea.Quit cmd when pendingQuit=true and save succeeded")
	}
}

// TestSaveComplete_Success_ClearsUnsavedIndicatorAndFiresBanner
func TestSaveComplete_Success_ClearsUnsavedIndicatorAndFiresBanner(t *testing.T) {
	m := newLoadedModel(sampleStep("s1"))
	m.dirty = true
	m.saveInProgress = true

	snapshot := &workflowio.SaveSnapshot{ModTime: time.Now(), Size: 10}
	got := applyMsg(m, saveCompleteMsg{result: workflowio.SaveResult{
		Kind:     workflowio.SaveErrorNone,
		Snapshot: snapshot,
	}})
	if got.dirty {
		t.Error("dirty should be false after successful save")
	}
	if !strings.Contains(got.saveBanner, "Saved") {
		t.Errorf("want save banner with 'Saved', got %q", got.saveBanner)
	}
	if got.saveInProgress {
		t.Error("saveInProgress should be false after save completes")
	}
}

// TestDialogAcknowledgeFindings_Enter_ProceedsToSave — Enter in acknowledgment dialog starts save
func TestDialogAcknowledgeFindings_Enter_ProceedsToSave(t *testing.T) {
	m := newLoadedModel(sampleStep("s1"))
	m.validateFn = warnFindings

	// Open the acknowledgment dialog via validate → warn findings
	next1, validateCmd := m.Update(keyCtrlS())
	m1 := next1.(Model)
	got1 := applyMsg(m1, validateCmd())
	if got1.dialog.kind != DialogAcknowledgeFindings {
		t.Fatalf("precondition: want DialogAcknowledgeFindings, got %d", got1.dialog.kind)
	}

	// Send Enter to confirm acknowledgment
	next2, saveCmd := got1.Update(keyEnter())
	got2 := next2.(Model)

	if got2.dialog.kind != DialogNone {
		t.Errorf("want dialog closed after Enter, got kind %d", got2.dialog.kind)
	}
	if !got2.saveInProgress {
		t.Error("want saveInProgress=true after acknowledgment confirmed")
	}
	if saveCmd == nil {
		t.Error("want non-nil save cmd after acknowledgment confirmed")
	}
}

// TestDialogAcknowledgeFindings_Esc_CancelsSave — Esc in acknowledgment dialog cancels save
func TestDialogAcknowledgeFindings_Esc_CancelsSave(t *testing.T) {
	m := newLoadedModel(sampleStep("s1"))
	m.validateFn = warnFindings
	prevFocus := m.focus

	// Open the acknowledgment dialog
	next1, validateCmd := m.Update(keyCtrlS())
	m1 := next1.(Model)
	got1 := applyMsg(m1, validateCmd())
	if got1.dialog.kind != DialogAcknowledgeFindings {
		t.Fatalf("precondition: want DialogAcknowledgeFindings, got %d", got1.dialog.kind)
	}

	// Send Esc to cancel
	got2 := applyKey(got1, keyEsc())

	if got2.dialog.kind != DialogNone {
		t.Errorf("want dialog closed after Esc, got kind %d", got2.dialog.kind)
	}
	if got2.saveInProgress {
		t.Error("saveInProgress should be false after cancel")
	}
	if got2.focus != prevFocus {
		t.Errorf("focus should be restored to %d after cancel, got %d", prevFocus, got2.focus)
	}
}

// TestCtrlN_ResetsSnapshot — Ctrl+N resets saveSnapshot to nil (F-98)
func TestCtrlN_ResetsSnapshot(t *testing.T) {
	m := newLoadedModel(sampleStep("s1"))
	m.saveSnapshot = &workflowio.SaveSnapshot{ModTime: time.Now(), Size: 50}

	got := applyKey(m, keyCtrlN())

	if got.saveSnapshot != nil {
		t.Error("saveSnapshot should be nil after Ctrl+N session transition (F-98)")
	}
	if got.dialog.kind != DialogNewChoice {
		t.Errorf("want DialogNewChoice after Ctrl+N, got %d", got.dialog.kind)
	}
}

// TestSaveComplete_Error_OpensKindSpecificDialog — one subtest per SaveErrorKind
func TestSaveComplete_Error_OpensKindSpecificDialog(t *testing.T) {
	cases := []struct {
		kind     workflowio.SaveErrorKind
		wantText string
	}{
		{workflowio.SaveErrorPermission, "permission"},
		{workflowio.SaveErrorDiskFull, "disk"},
		{workflowio.SaveErrorEXDEV, "device"},
		{workflowio.SaveErrorSymlinkEscape, "symlink"},
		{workflowio.SaveErrorTargetNotRegularFile, "regular"},
		{workflowio.SaveErrorParse, "marshal"},
		{workflowio.SaveErrorOther, "save error"},
	}
	for _, tc := range cases {
		t.Run(fmt.Sprintf("Kind_%d", tc.kind), func(t *testing.T) {
			m := newLoadedModel(sampleStep("s1"))
			m.saveInProgress = true

			result := workflowio.SaveResult{Kind: tc.kind, Err: errors.New("test error")}
			got := applyMsg(m, saveCompleteMsg{result: result})

			if got.dialog.kind != DialogError {
				t.Fatalf("want DialogError for kind %d, got %d", tc.kind, got.dialog.kind)
			}
			payload, _ := got.dialog.payload.(string)
			if !strings.Contains(strings.ToLower(payload), tc.wantText) {
				t.Errorf("want %q in error message, got %q", tc.wantText, payload)
			}
			if got.saveInProgress {
				t.Error("saveInProgress should be false after error")
			}
		})
	}
}
