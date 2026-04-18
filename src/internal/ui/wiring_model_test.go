package ui

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/mxriverlynn/pr9k/src/internal/statusline"
)

// --- TP-007: chained WithStatusRunner + WithModeTrigger composition ---

// TestModel_WithStatusRunner_WithModeTrigger_ChainedComposition verifies that
// chaining WithStatusRunner and WithModeTrigger on a single NewModel call
// retains both effects. A value-receiver bug in either builder would silently
// drop the other's state.
func TestModel_WithStatusRunner_WithModeTrigger_ChainedComposition(t *testing.T) {
	var triggerCalled int
	triggerFn := func() { triggerCalled++ }
	mockSR := &mockStatusReader{enabled: true, hasOutput: true, output: "build: ok"}

	header := NewStatusHeader(1)
	header.SetPhaseSteps([]string{"step-one"})
	actions := make(chan StepAction, 10)
	kh := NewKeyHandler(func() {}, actions)

	m := NewModel(header, kh, "v0.0.0-test").
		WithStatusRunner(mockSR).
		WithModeTrigger(triggerFn)
	m.width = 80
	m.height = 24
	m.log.SetSize(76, 10)

	// In ModeNormal with an enabled+has-output runner, the footer shows the
	// status runner output. Assert WithStatusRunner retained its effect.
	viewBefore := stripANSI(m.View())
	if !strings.Contains(viewBefore, "build: ok") {
		t.Errorf("View() before mode change does not contain status runner output; plain:\n%s", viewBefore)
	}

	// 'q' in ModeNormal transitions to ModeQuitConfirm, firing triggerFn.
	// This asserts WithModeTrigger retained its effect.
	next, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'q'}})
	_ = next.(Model)

	if triggerCalled != 1 {
		t.Errorf("triggerFn called %d times after mode change, want 1", triggerCalled)
	}
}

// TestModel_WithModeTrigger_RetainsStatusRunner verifies the chained builder
// order does not matter: WithModeTrigger applied after WithStatusRunner keeps
// the status runner wired.
func TestModel_WithModeTrigger_RetainsStatusRunner(t *testing.T) {
	mockSR := &mockStatusReader{enabled: true, hasOutput: true, output: "ci: green"}

	header := NewStatusHeader(1)
	header.SetPhaseSteps([]string{"step"})
	actions := make(chan StepAction, 10)
	kh := NewKeyHandler(func() {}, actions)

	m := NewModel(header, kh, "v0.0.0-test").
		WithStatusRunner(mockSR).
		WithModeTrigger(func() {})
	m.width = 80
	m.height = 24
	m.log.SetSize(76, 10)

	view := stripANSI(m.View())
	if !strings.Contains(view, "ci: green") {
		t.Errorf("status runner output missing after WithModeTrigger; plain:\n%s", view)
	}
}

// --- TP-008: real *statusline.Runner in SetStatusLineActive wiring ---

// TestModel_StatusLineActive_WiredFromRealRunner_NilConfig verifies that a
// nil-config runner returns Enabled() == false, and that
// SetStatusLineActive(false) leaves StatusLineActive() false.
func TestModel_StatusLineActive_WiredFromRealRunner_NilConfig(t *testing.T) {
	runner := statusline.New(nil, t.TempDir(), t.TempDir(), nil)

	if runner.Enabled() {
		t.Error("nil-config runner should not be enabled")
	}

	actions := make(chan StepAction, 10)
	kh := NewKeyHandler(func() {}, actions)
	kh.SetStatusLineActive(runner.Enabled())

	if kh.StatusLineActive() {
		t.Error("StatusLineActive should be false after SetStatusLineActive(false) from nil-config runner")
	}
}

// TestModel_StatusLineActive_WiredFromRealRunner_EnabledCommand verifies that
// a runner built with a real executable returns Enabled() == true, and that
// SetStatusLineActive(true) makes StatusLineActive() true.
func TestModel_StatusLineActive_WiredFromRealRunner_EnabledCommand(t *testing.T) {
	tmp := t.TempDir()
	scriptPath := filepath.Join(tmp, "x.sh")
	if err := os.WriteFile(scriptPath, []byte("#!/bin/sh\necho ok\n"), 0o755); err != nil {
		t.Fatal(err)
	}

	cfg := &statusline.Config{Command: "./x.sh"}
	runner := statusline.New(cfg, tmp, tmp, nil)

	if !runner.Enabled() {
		t.Error("runner with real executable should be enabled")
	}

	actions := make(chan StepAction, 10)
	kh := NewKeyHandler(func() {}, actions)
	kh.SetStatusLineActive(runner.Enabled())

	if !kh.StatusLineActive() {
		t.Error("StatusLineActive should be true after SetStatusLineActive(true) from real-command runner")
	}
}

// --- TP-009: StatusLineUpdatedMsg leaves all observable state unchanged ---

// TestModel_StatusLineUpdatedMsg_NoStateChange_Extended extends the existing
// no-state-change test to cover additional fields: header, selection, and
// log-viewport scroll offset.
func TestModel_StatusLineUpdatedMsg_NoStateChange_Extended(t *testing.T) {
	m := newTestModel(t)
	m.width = 80
	m.height = 24
	m.log.SetSize(76, 10)

	// Snapshot relevant state before Update.
	selBefore := m.log.sel
	yOffBefore := m.log.viewport.YOffset
	iterLineBefore := m.header.header.IterationLine
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
		t.Errorf("mode changed: got %v, want %v", m2.keys.handler.Mode(), modeBefore)
	}
	if m2.log.sel != selBefore {
		t.Errorf("selection changed unexpectedly")
	}
	if m2.log.viewport.YOffset != yOffBefore {
		t.Errorf("viewport YOffset changed: got %d, want %d", m2.log.viewport.YOffset, yOffBefore)
	}
	if m2.header.header.IterationLine != iterLineBefore {
		t.Errorf("header IterationLine changed: got %q, want %q", m2.header.header.IterationLine, iterLineBefore)
	}
	if len(m2.log.lines) != 0 {
		t.Errorf("log lines changed unexpectedly: %d lines", len(m2.log.lines))
	}
}
