package workflowedit

import (
	"bytes"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/mxriverlynn/pr9k/src/internal/workflowio"
	"github.com/mxriverlynn/pr9k/src/internal/workflowmodel"
)

// TestSessionLog_ResizeEvent — resize event logged with width/height.
func TestSessionLog_ResizeEvent(t *testing.T) {
	var buf bytes.Buffer
	m := newTestModelWithLog(&buf)
	applyMsg(m, tea.WindowSizeMsg{Width: 120, Height: 40})
	log := buf.String()
	if !strings.Contains(log, "resize") {
		t.Errorf("expected resize event in log, got: %q", log)
	}
	if !strings.Contains(log, "120") {
		t.Errorf("expected width=120 in resize event, got: %q", log)
	}
	if !strings.Contains(log, "40") {
		t.Errorf("expected height=40 in resize event, got: %q", log)
	}
}

// TestSessionLog_DialogOpenEvent — dialog_open logged when a dialog opens.
func TestSessionLog_DialogOpenEvent(t *testing.T) {
	var buf bytes.Buffer
	m := newTestModelWithLog(&buf)
	m.loaded = true
	// Ctrl+Q on an unmodified loaded model opens DialogQuitConfirm.
	applyKey(m, keyCtrlQ())
	log := buf.String()
	if !strings.Contains(log, "dialog_open") {
		t.Errorf("expected dialog_open event in log, got: %q", log)
	}
}

// TestSessionLog_DialogCloseEvent — dialog_close logged when dialog is dismissed with Esc.
func TestSessionLog_DialogCloseEvent(t *testing.T) {
	var buf bytes.Buffer
	m := newTestModelWithLog(&buf)
	m.loaded = true
	// Open QuitConfirm dialog.
	m = applyKey(m, keyCtrlQ())
	// Reset buffer so we only capture the close event.
	buf.Reset()
	// Dismiss with Esc.
	applyKey(m, keyEsc())
	log := buf.String()
	if !strings.Contains(log, "dialog_close") {
		t.Errorf("expected dialog_close event in log after Esc, got: %q", log)
	}
}

// TestSessionLog_SaveBannerSetEvent — save_banner_set logged after successful save.
func TestSessionLog_SaveBannerSetEvent(t *testing.T) {
	var buf bytes.Buffer
	m := newTestModelWithLog(&buf)
	m.saveInProgress = true
	applyMsg(m, saveCompleteMsg{result: workflowio.SaveResult{Kind: workflowio.SaveErrorNone}})
	log := buf.String()
	if !strings.Contains(log, "save_banner_set") {
		t.Errorf("expected save_banner_set in log after successful save, got: %q", log)
	}
}

// TestSessionLog_SaveBannerClearedEvent — save_banner_cleared logged when the timer fires.
func TestSessionLog_SaveBannerClearedEvent(t *testing.T) {
	var buf bytes.Buffer
	m := newTestModelWithLog(&buf)
	m.saveBanner = "Saved at 12:00:00"
	m.bannerGen = 3
	// Deliver a matching clearSaveBannerMsg.
	applyMsg(m, clearSaveBannerMsg{gen: 3})
	log := buf.String()
	if !strings.Contains(log, "save_banner_cleared") {
		t.Errorf("expected save_banner_cleared in log when clearSaveBannerMsg matches gen, got: %q", log)
	}
}

// TestSessionLog_TerminalTooSmallEvent — terminal_too_small logged on resize below minimum.
func TestSessionLog_TerminalTooSmallEvent(t *testing.T) {
	var buf bytes.Buffer
	m := newTestModelWithLog(&buf)
	// Resize below the minimum (60×16).
	applyMsg(m, tea.WindowSizeMsg{Width: 30, Height: 10})
	log := buf.String()
	if !strings.Contains(log, "terminal_too_small") {
		t.Errorf("expected terminal_too_small in log for 30×10 terminal, got: %q", log)
	}
}

// TestSessionLog_FocusChangedEvent — focus_changed logged on Tab from outline to detail.
func TestSessionLog_FocusChangedEvent(t *testing.T) {
	var buf bytes.Buffer
	m := newTestModelWithLog(&buf)
	m.doc.Steps = []workflowmodel.Step{sampleStep("a")}
	m.diskDoc.Steps = m.doc.Steps
	m.loaded = true
	m.focus = focusOutline
	// Move cursor to the step row (not a header or add row).
	rows := buildOutlineRows(m.doc, m.outline.collapsed)
	for i, r := range rows {
		if r.kind == rowKindStep {
			m.outline.cursor = i
			break
		}
	}
	applyKey(m, keyTab())
	log := buf.String()
	if !strings.Contains(log, "focus_changed") {
		t.Errorf("expected focus_changed in log on Tab from outline, got: %q", log)
	}
}

// TestSessionLog_ValidateStartedEvent — validate_started logged when Ctrl+S fires validation.
func TestSessionLog_ValidateStartedEvent(t *testing.T) {
	var buf bytes.Buffer
	m := newTestModelWithLog(&buf)
	m.loaded = true
	m.validateFn = noFindings
	applyKey(m, keyCtrlS())
	log := buf.String()
	if !strings.Contains(log, "validate_started") {
		t.Errorf("expected validate_started in log on Ctrl+S, got: %q", log)
	}
}

// TestSessionLog_ValidateCompleteEvent — validate_complete logged when validation finishes.
func TestSessionLog_ValidateCompleteEvent(t *testing.T) {
	var buf bytes.Buffer
	m := newTestModelWithLog(&buf)
	m.validateInProgress = true
	applyMsg(m, validateCompleteMsg{items: nil})
	log := buf.String()
	if !strings.Contains(log, "validate_complete") {
		t.Errorf("expected validate_complete in log on validateCompleteMsg, got: %q", log)
	}
}

// TestSessionLog_PhaseBoundaryDeclineEvent — phase_boundary_decline logged when Alt+Up
// is blocked by a phase boundary.
func TestSessionLog_PhaseBoundaryDeclineEvent(t *testing.T) {
	var buf bytes.Buffer
	m := newTestModelWithLog(&buf)
	// Two steps in different phases: init then iteration.
	step1 := workflowmodel.Step{
		Name:    "init-step",
		Kind:    workflowmodel.StepKindShell,
		Command: []string{"echo"},
		Phase:   workflowmodel.StepPhaseInitialize,
	}
	step2 := workflowmodel.Step{
		Name:    "iter-step",
		Kind:    workflowmodel.StepKindShell,
		Command: []string{"echo"},
		Phase:   workflowmodel.StepPhaseIteration,
	}
	m.doc.Steps = []workflowmodel.Step{step1, step2}
	m.diskDoc.Steps = m.doc.Steps
	m.loaded = true
	m.focus = focusOutline
	// Position cursor on the iteration step (stepIdx=1) so Alt+Up crosses a boundary.
	rows := buildOutlineRows(m.doc, m.outline.collapsed)
	for i, r := range rows {
		if r.kind == rowKindStep && r.stepIdx == 1 {
			m.outline.cursor = i
			break
		}
	}
	applyKey(m, keyAltUp())
	log := buf.String()
	if !strings.Contains(log, "phase_boundary_decline") {
		t.Errorf("expected phase_boundary_decline in log on cross-phase Alt+Up, got: %q", log)
	}
}

// TestSessionLog_SecretRevealedEvent — secret_revealed logged when 'r' reveals a masked field.
func TestSessionLog_SecretRevealedEvent(t *testing.T) {
	var buf bytes.Buffer
	m := newTestModelWithLog(&buf)
	// Step with a sensitive env entry so fieldKindSecretMask field exists.
	secretStep := workflowmodel.Step{
		Name:    "envstep",
		Kind:    workflowmodel.StepKindShell,
		Command: []string{"echo"},
		Env: []workflowmodel.EnvEntry{
			{Key: "MY_API_SECRET", Value: "secretval", IsLiteral: true},
		},
	}
	m.doc.Steps = []workflowmodel.Step{secretStep}
	m.diskDoc.Steps = m.doc.Steps
	m.loaded = true
	m.focus = focusDetail
	// Position outline cursor on the step row.
	rows := buildOutlineRows(m.doc, m.outline.collapsed)
	for i, r := range rows {
		if r.kind == rowKindStep && r.stepIdx == 0 {
			m.outline.cursor = i
			break
		}
	}
	// Find the secretMask field index.
	fields := buildDetailFields(secretStep)
	secretIdx := -1
	for j, f := range fields {
		if f.kind == fieldKindSecretMask {
			secretIdx = j
			break
		}
	}
	if secretIdx < 0 {
		t.Fatal("test setup: no secretMask field found in step")
	}
	m.detail.cursor = secretIdx
	applyKey(m, keyRune('r'))
	log := buf.String()
	if !strings.Contains(log, "secret_revealed") {
		t.Errorf("expected secret_revealed in log on 'r' key, got: %q", log)
	}
}

// TestSessionLog_SecretRemaskedEvent — secret_remasked logged when Tab re-masks a revealed field.
func TestSessionLog_SecretRemaskedEvent(t *testing.T) {
	var buf bytes.Buffer
	m := newTestModelWithLog(&buf)
	m.doc.Steps = []workflowmodel.Step{sampleStep("a")}
	m.diskDoc.Steps = m.doc.Steps
	m.loaded = true
	m.focus = focusDetail
	// Simulate a revealed field (revealedField != -1).
	m.detail.revealedField = 0
	applyKey(m, keyTab())
	log := buf.String()
	if !strings.Contains(log, "secret_remasked") {
		t.Errorf("expected secret_remasked in log on Tab from detail with revealed field, got: %q", log)
	}
}
