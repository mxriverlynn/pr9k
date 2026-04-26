package workflowedit

import (
	"strings"
	"testing"
	"time"

	"github.com/mxriverlynn/pr9k/src/internal/workflowmodel"
)

// TestModel_LoadPipeline_SymlinkBannerFires — openFileResultMsg with isSymlink=true sets banner state.
func TestModel_LoadPipeline_SymlinkBannerFires(t *testing.T) {
	m := newTestModel()
	msg := openFileResultMsg{
		doc:           workflowmodel.WorkflowDoc{},
		diskDoc:       workflowmodel.WorkflowDoc{},
		workflowDir:   "/testworkflow",
		isSymlink:     true,
		symlinkTarget: "/real/config.json",
	}
	got := applyMsg(m, msg)
	if !got.banners.isSymlink {
		t.Error("banners.isSymlink should be true after load with isSymlink=true")
	}
	if got.banners.symlinkTarget != "/real/config.json" {
		t.Errorf("banners.symlinkTarget wrong, got %q", got.banners.symlinkTarget)
	}
	view := stripView(got)
	if !strings.Contains(view, "symlink") {
		t.Errorf("view should show symlink banner; view: %q", view)
	}
}

// TestModel_LoadPipeline_ExternalWorkflowBanner — openFileResultMsg with isExternal=true sets banner state.
func TestModel_LoadPipeline_ExternalWorkflowBanner(t *testing.T) {
	m := newTestModel()
	msg := openFileResultMsg{
		doc:         workflowmodel.WorkflowDoc{},
		diskDoc:     workflowmodel.WorkflowDoc{},
		workflowDir: "/testworkflow",
		isExternal:  true,
	}
	got := applyMsg(m, msg)
	if !got.banners.isExternalWorkflow {
		t.Error("banners.isExternalWorkflow should be true after load with isExternal=true")
	}
	view := stripView(got)
	if !strings.Contains(view, "external") {
		t.Errorf("view should show external-workflow banner; view: %q", view)
	}
}

// TestModel_BannerPriority_ReadOnlyWinsOverSymlink — read-only banner takes priority,
// with [1 more warnings] affordance for the symlink banner.
func TestModel_BannerPriority_ReadOnlyWinsOverSymlink(t *testing.T) {
	m := newLoadedModel(sampleStep("a"))
	m.banners = bannerState{
		isReadOnly:    true,
		isSymlink:     true,
		symlinkTarget: "/some/target",
	}
	view := stripView(m)
	if !strings.Contains(view, "read-only") {
		t.Errorf("view should show read-only banner (highest priority); view: %q", view)
	}
	if !strings.Contains(view, "[1 more") {
		t.Errorf("view should show [1 more warnings] affordance; view: %q", view)
	}
}

// TestModel_AckSet_PopulatedOnAcknowledge — Enter on DialogAcknowledgeFindings writes keys
// into ackSet; subsequent validate with same warnings does not re-show dialog.
func TestModel_AckSet_PopulatedOnAcknowledge(t *testing.T) {
	m := newLoadedModel(sampleStep("a"))
	findings := []findingResult{{text: "warn: captureAs has no consumer", isFatal: false}}
	m.findingsPanel = buildFindingsPanel(findings, nil, findingsPanel{})
	m.dialog = dialogState{kind: DialogAcknowledgeFindings}

	// Press Enter to acknowledge.
	got := applyKey(m, keyEnter())

	// ackSet should be populated.
	if !got.findingsPanel.ackSet["warn: captureAs has no consumer"] {
		t.Error("ackSet should contain acknowledged finding key after Enter")
	}

	// Simulate another validation returning the same warnings.
	// All findings are already acknowledged → should NOT re-show dialog.
	got2 := applyMsg(got, validateCompleteMsg{items: findings})
	if got2.dialog.kind == DialogAcknowledgeFindings {
		t.Error("should not re-show DialogAcknowledgeFindings for already-acknowledged warnings")
	}
}

// TestModel_PreCopyIntegrity_BrokenDefault_ShowsCopyAnywayDialog — pressing 'c' (Copy)
// in DialogNewChoice when workflowDir has no valid bundle shows DialogCopyBrokenRef.
func TestModel_PreCopyIntegrity_BrokenDefault_ShowsCopyAnywayDialog(t *testing.T) {
	// workflowDir="/testworkflow" does not exist on disk, so CopyFromDefault fails.
	m := newTestModel()
	m.dialog = dialogState{kind: DialogNewChoice}
	got := applyKey(m, keyRune('c'))
	if got.dialog.kind != DialogCopyBrokenRef {
		t.Fatalf("want DialogCopyBrokenRef, got dialog=%d", got.dialog.kind)
	}
}

// TestSessionEvent_NoContainerEnvValuesLogged — R7 regression: containerEnv secret values
// must never appear in any session-event log line.
func TestSessionEvent_NoContainerEnvValuesLogged(t *testing.T) {
	var buf strings.Builder
	m := newTestModelWithLog(&buf)
	m.doc.ContainerEnv = map[string]string{"MY_API_KEY": "super-secret-value-12345"}
	m.loaded = true
	m.dirty = false

	// Trigger a load event that fires security signals.
	msg := openFileResultMsg{
		doc:           m.doc,
		diskDoc:       m.doc,
		workflowDir:   "/testworkflow",
		isSymlink:     true,
		symlinkTarget: "/real/config.json",
	}
	applyMsg(m, msg)

	log := buf.String()
	if strings.Contains(log, "super-secret-value-12345") {
		t.Error("R7: log must not contain containerEnv secret values")
	}
}

// TestSessionEvent_SymlinkDetected_LoggedOnLoad — symlink_detected event emitted when
// openFileResultMsg has isSymlink=true.
func TestSessionEvent_SymlinkDetected_LoggedOnLoad(t *testing.T) {
	var buf strings.Builder
	m := newTestModelWithLog(&buf)
	msg := openFileResultMsg{
		doc:           workflowmodel.WorkflowDoc{},
		diskDoc:       workflowmodel.WorkflowDoc{},
		workflowDir:   "/testworkflow",
		isSymlink:     true,
		symlinkTarget: "/real/config.json",
	}
	applyMsg(m, msg)
	log := buf.String()
	if !strings.Contains(log, "symlink_detected") {
		t.Errorf("expected symlink_detected event in log, got: %q", log)
	}
}

// TestSessionEvent_SharedInstallDetected_LoggedOnLoad — shared_install_detected event
// emitted when openFileResultMsg has isSharedInstall=true.
func TestSessionEvent_SharedInstallDetected_LoggedOnLoad(t *testing.T) {
	var buf strings.Builder
	m := newTestModelWithLog(&buf)
	msg := openFileResultMsg{
		doc:             workflowmodel.WorkflowDoc{},
		diskDoc:         workflowmodel.WorkflowDoc{},
		workflowDir:     "/testworkflow",
		isSharedInstall: true,
	}
	applyMsg(m, msg)
	log := buf.String()
	if !strings.Contains(log, "shared_install_detected") {
		t.Errorf("expected shared_install_detected event in log, got: %q", log)
	}
}

// TestFmtEditorEvents_TwoDistinctLogLines — fmtEditorInvoked and fmtEditorExited
// produce distinct, non-overlapping event strings.
func TestFmtEditorEvents_TwoDistinctLogLines(t *testing.T) {
	inv := fmtEditorInvoked("vim")
	exit := fmtEditorExited("vim", 0, time.Second)
	if inv == exit {
		t.Error("fmtEditorInvoked and fmtEditorExited must produce distinct strings")
	}
	if !strings.Contains(inv, "invoked") {
		t.Errorf("fmtEditorInvoked should contain 'invoked', got %q", inv)
	}
	if !strings.Contains(exit, "exit") {
		t.Errorf("fmtEditorExited should contain 'exit', got %q", exit)
	}
}

// TestFmtEditorOpened_RemovedFromCodebase confirms the two split helpers exist and
// produce events that don't cross-contaminate (guard against merging them back).
func TestFmtEditorOpened_RemovedFromCodebase(t *testing.T) {
	inv := fmtEditorInvoked("nvim")
	exit := fmtEditorExited("nvim", 1, 3*time.Second)
	if strings.Contains(inv, "exit") {
		t.Errorf("editor_invoked event must not mention 'exit', got %q", inv)
	}
	if strings.Contains(exit, "invoked") {
		t.Errorf("editor_exit event must not mention 'invoked', got %q", exit)
	}
}
