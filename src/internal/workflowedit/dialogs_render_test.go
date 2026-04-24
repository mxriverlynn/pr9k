package workflowedit

import (
	"strings"
	"testing"
)

// One test per DialogKind confirming the render output (D-8).

func TestDialog_NewChoice_ContainsCopyEmptyCancel(t *testing.T) {
	m := newTestModel()
	m.dialog = dialogState{kind: DialogNewChoice}
	view := m.View()
	if !strings.Contains(view, "Copy") || !strings.Contains(view, "Empty") || !strings.Contains(view, "Cancel") {
		t.Errorf("DialogNewChoice should show Copy/Empty/Cancel, got %q", view)
	}
}

func TestDialog_PathPicker_ContainsOpenLabel(t *testing.T) {
	m := newTestModel()
	m.dialog = dialogState{kind: DialogPathPicker, payload: "/some/path"}
	view := m.View()
	if !strings.Contains(view, "Open") {
		t.Errorf("DialogPathPicker should show Open, got %q", view)
	}
}

func TestDialog_UnsavedChanges_ContainsSaveCancelDiscard(t *testing.T) {
	m := newTestModel()
	m.dialog = dialogState{kind: DialogUnsavedChanges}
	view := m.View()
	for _, opt := range []string{"Save", "Cancel", "Discard"} {
		if !strings.Contains(view, opt) {
			t.Errorf("DialogUnsavedChanges should contain %q, got %q", opt, view)
		}
	}
}

func TestDialog_QuitConfirm_ContainsYesNo(t *testing.T) {
	m := newTestModel()
	m.dialog = dialogState{kind: DialogQuitConfirm}
	view := m.View()
	if !strings.Contains(view, "Yes") || !strings.Contains(view, "No") {
		t.Errorf("DialogQuitConfirm should show Yes/No, got %q", view)
	}
}

func TestDialog_FindingsPanel_ContainsFindings(t *testing.T) {
	m := newTestModel()
	m.dialog = dialogState{kind: DialogFindingsPanel}
	view := m.View()
	if !strings.Contains(view, "Findings") {
		t.Errorf("DialogFindingsPanel should mention Findings, got %q", view)
	}
}

func TestDialog_SaveInProgress_ContainsWaitMessage(t *testing.T) {
	m := newTestModel()
	m.dialog = dialogState{kind: DialogSaveInProgress}
	view := m.View()
	if !strings.Contains(view, "Save in progress") {
		t.Errorf("DialogSaveInProgress should show wait message, got %q", view)
	}
}

func TestDialog_RemoveConfirm_ContainsDeleteCancel(t *testing.T) {
	m := newTestModel()
	m.dialog = dialogState{kind: DialogRemoveConfirm, payload: "my-step"}
	view := m.View()
	if !strings.Contains(view, "Delete") || !strings.Contains(view, "Cancel") {
		t.Errorf("DialogRemoveConfirm should show Delete/Cancel, got %q", view)
	}
	if !strings.Contains(view, "my-step") {
		t.Errorf("DialogRemoveConfirm should include step name, got %q", view)
	}
}

func TestDialog_Recovery_ContainsRecoveryLabel(t *testing.T) {
	m := newTestModel()
	m.dialog = dialogState{kind: DialogRecovery, payload: "raw-bytes-here"}
	view := m.View()
	if !strings.Contains(view, "Recovery") {
		t.Errorf("DialogRecovery should contain Recovery label, got %q", view)
	}
}

func TestDialog_Error_ContainsErrorLabel(t *testing.T) {
	m := newTestModel()
	m.dialog = dialogState{kind: DialogError, payload: "something went wrong"}
	view := m.View()
	if !strings.Contains(view, "Error") {
		t.Errorf("DialogError should contain Error label, got %q", view)
	}
}

func TestDialog_FileConflict_ContainsOptions(t *testing.T) {
	m := newTestModel()
	m.dialog = dialogState{kind: DialogFileConflict}
	view := m.View()
	if !strings.Contains(view, "conflict") {
		t.Errorf("DialogFileConflict should mention conflict, got %q", view)
	}
}

func TestDialog_CrashTempNotice_ContainsDetectedLabel(t *testing.T) {
	m := newTestModel()
	m.dialog = dialogState{kind: DialogCrashTempNotice}
	view := m.View()
	if !strings.Contains(view, "Crash temp") {
		t.Errorf("DialogCrashTempNotice should mention crash temp, got %q", view)
	}
}

func TestDialog_FirstSaveConfirm_ContainsConfirmLabel(t *testing.T) {
	m := newTestModel()
	m.dialog = dialogState{kind: DialogFirstSaveConfirm}
	view := m.View()
	if !strings.Contains(view, "save") {
		t.Errorf("DialogFirstSaveConfirm should mention save, got %q", view)
	}
}

func TestDialog_ExternalEditorOpening_ContainsOpeningLabel(t *testing.T) {
	m := newTestModel()
	m.dialog = dialogState{kind: DialogExternalEditorOpening}
	view := m.View()
	if !strings.Contains(view, "editor") {
		t.Errorf("DialogExternalEditorOpening should mention editor, got %q", view)
	}
}

// TestDialog_None_NoDialogInView verifies DialogNone shows normal content.
func TestDialog_None_NoDialogInView(t *testing.T) {
	m := newTestModel()
	// No dialog — should show empty editor hint.
	view := m.View()
	if !strings.Contains(view, "No workflow") {
		t.Errorf("with no dialog and no workflow, should show empty hint, got %q", view)
	}
}
