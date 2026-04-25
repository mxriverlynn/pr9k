package workflowedit

import (
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"testing"
)

// --- DialogFileConflict update tests ---

// TestModel_DialogFileConflict_Overwrite_SetsBypassFlag verifies that pressing
// 'o' sets forceSave=true and closes the dialog (D-41).
func TestModel_DialogFileConflict_Overwrite_SetsBypassFlag(t *testing.T) {
	m := newLoadedModel(sampleStep("s1"))
	m.dialog = dialogState{kind: DialogFileConflict}
	m.prevFocus = focusOutline

	got := applyKey(m, keyRune('o'))

	if !got.forceSave {
		t.Error("want forceSave=true after overwrite")
	}
	if got.dialog.kind != DialogNone {
		t.Errorf("want dialog closed, got kind=%d", got.dialog.kind)
	}
}

// TestModel_DialogFileConflict_Reload_ReloadsAndClearsState verifies that
// pressing 'r' returns a load cmd and clears the dialog.
func TestModel_DialogFileConflict_Reload_ReloadsAndClearsState(t *testing.T) {
	m := newLoadedModel(sampleStep("s1"))
	m.dialog = dialogState{kind: DialogFileConflict}

	next, cmd := m.Update(keyRune('r'))
	got := next.(Model)

	if got.dialog.kind != DialogNone {
		t.Errorf("want dialog closed after reload, got kind=%d", got.dialog.kind)
	}
	if cmd == nil {
		t.Error("want non-nil cmd for reload")
	}
}

// TestModel_DialogFileConflict_Reload_ParseError_ShowsRecovery verifies that
// when a reload triggered from FileConflict encounters a parse error, the model
// shows DialogRecovery (F-PR2-20). We simulate the load result directly.
func TestModel_DialogFileConflict_Reload_ParseError_ShowsRecovery(t *testing.T) {
	m := newLoadedModel(sampleStep("s1"))
	// Simulate openFileResultMsg with parse error (rawBytes set).
	msg := openFileResultMsg{
		err:      errParseError,
		rawBytes: []byte("broken json content"),
	}
	got := applyMsg(m, msg)

	if got.dialog.kind != DialogRecovery {
		t.Errorf("want DialogRecovery after parse error, got kind=%d", got.dialog.kind)
	}
	raw, _ := got.dialog.payload.(string)
	if !strings.Contains(raw, "broken json") {
		t.Errorf("recovery payload should contain raw bytes, got %q", raw)
	}
}

// TestModel_DialogFileConflict_Cancel_PreservesState verifies that pressing 'c'
// closes the dialog without modifying the in-memory document.
func TestModel_DialogFileConflict_Cancel_PreservesState(t *testing.T) {
	m := newLoadedModel(sampleStep("s1"), sampleStep("s2"))
	m.dialog = dialogState{kind: DialogFileConflict}
	stepsBefore := len(m.doc.Steps)

	got := applyKey(m, keyRune('c'))

	if got.dialog.kind != DialogNone {
		t.Errorf("want dialog closed after cancel, got kind=%d", got.dialog.kind)
	}
	if len(got.doc.Steps) != stepsBefore {
		t.Errorf("want %d steps preserved, got %d", stepsBefore, len(got.doc.Steps))
	}
}

// --- DialogFirstSaveConfirm update tests ---

// TestModel_DialogFirstSaveConfirm_Yes_TriggersSave verifies that pressing 'y'
// marks firstSaveConfirmed and returns a save cmd (D17/D22).
func TestModel_DialogFirstSaveConfirm_Yes_TriggersSave(t *testing.T) {
	m := newLoadedModel(sampleStep("s1"))
	m.dialog = dialogState{kind: DialogFirstSaveConfirm}

	next, cmd := m.Update(keyRune('y'))
	got := next.(Model)

	if !got.firstSaveConfirmed {
		t.Error("want firstSaveConfirmed=true after yes")
	}
	if got.dialog.kind != DialogNone {
		t.Errorf("want dialog closed, got kind=%d", got.dialog.kind)
	}
	if cmd == nil {
		t.Error("want non-nil save cmd")
	}
	if !got.saveInProgress {
		t.Error("want saveInProgress=true")
	}
}

// TestModel_DialogFirstSaveConfirm_No_CancelsSave verifies that pressing 'n'
// closes the dialog without triggering a save.
func TestModel_DialogFirstSaveConfirm_No_CancelsSave(t *testing.T) {
	m := newLoadedModel(sampleStep("s1"))
	m.dialog = dialogState{kind: DialogFirstSaveConfirm}

	next, cmd := m.Update(keyRune('n'))
	got := next.(Model)

	if got.dialog.kind != DialogNone {
		t.Errorf("want dialog closed after no, got kind=%d", got.dialog.kind)
	}
	if got.firstSaveConfirmed {
		t.Error("want firstSaveConfirmed=false (save not confirmed)")
	}
	if cmd != nil {
		t.Error("want nil cmd (no save triggered)")
	}
}

// --- DialogCrashTempNotice update tests ---

// TestModel_DialogCrashTempNotice_Discard_RemovesTempFile_AssertsContainmentBeforeRemove
// verifies that pressing 'd' removes a temp file that is inside workflowDir
// (containment check passes) and is a regular file.
func TestModel_DialogCrashTempNotice_Discard_RemovesTempFile_AssertsContainmentBeforeRemove(t *testing.T) {
	// Create a real temp dir as workflowDir with a file inside.
	dir := t.TempDir()
	tmpFile := filepath.Join(dir, "config.json.12345-678.tmp")
	if err := os.WriteFile(tmpFile, []byte("temp"), 0o600); err != nil {
		t.Fatalf("setup: write temp file: %v", err)
	}

	m := newTestModel()
	m.workflowDir = dir
	m.dialog = dialogState{kind: DialogCrashTempNotice, payload: tmpFile}

	got := applyKey(m, keyRune('d'))

	// File should be removed.
	if _, err := os.Lstat(tmpFile); !os.IsNotExist(err) {
		t.Errorf("want temp file removed, but it still exists (err=%v)", err)
	}
	if got.dialog.kind != DialogNone {
		t.Errorf("want dialog closed after discard, got kind=%d", got.dialog.kind)
	}
}

// TestDialog_CrashTempDiscard_RejectsPathEscapingWorkflowDir verifies that
// pressing 'd' with a path outside workflowDir opens DialogError and does NOT
// remove the file (F-PR2-7).
func TestDialog_CrashTempDiscard_RejectsPathEscapingWorkflowDir(t *testing.T) {
	// workflowDir is one temp dir; the "crash temp" file is in a different dir.
	workflowDir := t.TempDir()
	otherDir := t.TempDir()
	externalFile := filepath.Join(otherDir, "config.json.99999-000.tmp")
	if err := os.WriteFile(externalFile, []byte("external"), 0o600); err != nil {
		t.Fatalf("setup: %v", err)
	}

	m := newTestModel()
	m.workflowDir = workflowDir
	m.dialog = dialogState{kind: DialogCrashTempNotice, payload: externalFile}

	got := applyKey(m, keyRune('d'))

	// File must NOT be removed.
	if _, err := os.Lstat(externalFile); err != nil {
		t.Errorf("external file should NOT be removed, but got error: %v", err)
	}
	// Must open DialogError.
	if got.dialog.kind != DialogError {
		t.Errorf("want DialogError for path escaping workflowDir, got kind=%d", got.dialog.kind)
	}
}

// TestDialog_CrashTempDiscard_RejectsFIFO verifies that pressing 'd' with a
// FIFO path opens DialogError and does NOT remove the FIFO (F-PR2-7).
func TestDialog_CrashTempDiscard_RejectsFIFO(t *testing.T) {
	dir := t.TempDir()
	fifoPath := filepath.Join(dir, "config.json.77777-000.tmp")
	if err := syscall.Mkfifo(fifoPath, 0o600); err != nil {
		t.Skipf("cannot create FIFO: %v", err)
	}
	defer os.Remove(fifoPath)

	m := newTestModel()
	m.workflowDir = dir
	m.dialog = dialogState{kind: DialogCrashTempNotice, payload: fifoPath}

	got := applyKey(m, keyRune('d'))

	// FIFO must NOT be removed.
	if _, err := os.Lstat(fifoPath); err != nil {
		t.Errorf("FIFO should NOT be removed, but got error: %v", err)
	}
	if got.dialog.kind != DialogError {
		t.Errorf("want DialogError for FIFO, got kind=%d", got.dialog.kind)
	}
}

// --- DialogRecovery update tests ---

// TestModel_DialogRecovery_OpenInEditor_AttemptsReload verifies that pressing
// 'o' invokes the editor and, when the editor exits, a reload is attempted.
func TestModel_DialogRecovery_OpenInEditor_AttemptsReload(t *testing.T) {
	// Use an invokeCallback editor so the cmd returned by Run will immediately
	// fire the reload (which will fail since /testworkflow doesn't exist).
	editor := &fakeEditorRunner{invokeCallback: true}
	m := newTestModel()
	m.editor = editor
	m.dialog = dialogState{kind: DialogRecovery, payload: "broken content"}

	next, editorCmd := m.Update(keyRune('o'))
	m1 := next.(Model)
	_ = m1

	if editor.runCount != 1 {
		t.Fatalf("want editor invoked once, got %d", editor.runCount)
	}
	if !strings.HasSuffix(editor.lastPath, "config.json") {
		t.Errorf("want editor invoked on config.json, got %q", editor.lastPath)
	}
	if editorCmd == nil {
		t.Fatal("want non-nil editor cmd")
	}

	// Execute the editor cmd (simulates editor exit with nil error).
	// This triggers the reload attempt. /testworkflow doesn't exist, so we
	// expect an error result (openFileResultMsg with err set, no rawBytes).
	reloadMsg := editorCmd()
	m2 := applyMsg(m1, reloadMsg)

	// Reload failed (workflowDir doesn't exist) → DialogError or DialogRecovery.
	// Either indicates reload was attempted.
	if m2.dialog.kind != DialogError && m2.dialog.kind != DialogRecovery {
		t.Errorf("after failed reload, want DialogError or DialogRecovery, got kind=%d", m2.dialog.kind)
	}
}

// TestModel_DialogRecovery_Reload_ReturnsToEditView verifies that pressing 'r'
// dispatches a load cmd; when load succeeds, the model transitions to edit view.
func TestModel_DialogRecovery_Reload_ReturnsToEditView(t *testing.T) {
	// Create a real workflowDir with a valid config.json.
	dir := t.TempDir()
	cfg := []byte(`{"steps":[{"name":"s1","type":"shell","command":["echo"]}]}`)
	if err := os.WriteFile(filepath.Join(dir, "config.json"), cfg, 0o600); err != nil {
		t.Fatalf("setup: %v", err)
	}

	m := newTestModel()
	m.workflowDir = dir
	m.dialog = dialogState{kind: DialogRecovery, payload: "broken content"}

	next, loadCmd := m.Update(keyRune('r'))
	m1 := next.(Model)
	_ = m1

	if loadCmd == nil {
		t.Fatal("want non-nil load cmd after 'r'")
	}

	// Execute the load cmd.
	loadMsg := loadCmd()
	m2 := applyMsg(m1, loadMsg)

	if !m2.loaded {
		t.Error("want loaded=true after successful reload")
	}
	if m2.dialog.kind != DialogNone {
		t.Errorf("want dialog closed after successful reload, got kind=%d", m2.dialog.kind)
	}
}

// TestModel_DialogRecovery_Discard_ReturnsToEmptyEditor verifies that pressing
// 'd' closes the dialog and returns to the empty-editor hint state.
func TestModel_DialogRecovery_Discard_ReturnsToEmptyEditor(t *testing.T) {
	m := newTestModel()
	m.loaded = true // simulate partial load state
	m.dialog = dialogState{kind: DialogRecovery, payload: "broken content"}

	got := applyKey(m, keyRune('d'))

	if got.dialog.kind != DialogNone {
		t.Errorf("want dialog closed after discard, got kind=%d", got.dialog.kind)
	}
	if got.loaded {
		t.Error("want loaded=false after discard (returns to empty-editor hint)")
	}
	if strings.Contains(got.View(), "broken") {
		t.Error("view should not contain recovery content after discard")
	}
}

// errParseError is a sentinel used in test messages to signal a parse-error
// scenario. The actual routing is determined by rawBytes != nil, not the error
// value, so any non-nil error suffices here.
var errParseError = os.ErrInvalid
