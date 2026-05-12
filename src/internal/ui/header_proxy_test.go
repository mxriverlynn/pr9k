package ui

import (
	"testing"

	tea "github.com/charmbracelet/bubbletea"
)

// capturingSend returns a send func that records the last message it received,
// along with a pointer to that recorded value for assertions.
func capturingSend(t *testing.T) (func(tea.Msg), *tea.Msg) {
	t.Helper()
	var captured tea.Msg
	return func(msg tea.Msg) { captured = msg }, &captured
}

// --- TP-004: headerProxy methods ---

func TestHeaderProxy_SetStepState_SendsCorrectMsg(t *testing.T) {
	send, got := capturingSend(t)
	p := NewHeaderProxy(send)

	p.SetStepState(3, StepDone)

	msg, ok := (*got).(headerStepStateMsg)
	if !ok {
		t.Fatalf("expected headerStepStateMsg, got %T", *got)
	}
	if msg.idx != 3 {
		t.Errorf("idx: want 3, got %d", msg.idx)
	}
	if msg.state != StepDone {
		t.Errorf("state: want StepDone, got %v", msg.state)
	}
}

func TestHeaderProxy_RenderIterationLine_SendsCorrectMsg(t *testing.T) {
	send, got := capturingSend(t)
	p := NewHeaderProxy(send)

	p.RenderIterationLine(2, 5, "42")

	msg, ok := (*got).(headerIterationLineMsg)
	if !ok {
		t.Fatalf("expected headerIterationLineMsg, got %T", *got)
	}
	if msg.iter != 2 {
		t.Errorf("iter: want 2, got %d", msg.iter)
	}
	if msg.max != 5 {
		t.Errorf("max: want 5, got %d", msg.max)
	}
	if msg.issue != "42" {
		t.Errorf("issue: want %q, got %q", "42", msg.issue)
	}
}

func TestHeaderProxy_RenderInitializeLine_SendsCorrectMsg(t *testing.T) {
	send, got := capturingSend(t)
	p := NewHeaderProxy(send)

	p.RenderInitializeLine(1, 3, "Bootstrap")

	msg, ok := (*got).(headerInitializeLineMsg)
	if !ok {
		t.Fatalf("expected headerInitializeLineMsg, got %T", *got)
	}
	if msg.stepNum != 1 {
		t.Errorf("stepNum: want 1, got %d", msg.stepNum)
	}
	if msg.stepCount != 3 {
		t.Errorf("stepCount: want 3, got %d", msg.stepCount)
	}
	if msg.stepName != "Bootstrap" {
		t.Errorf("stepName: want %q, got %q", "Bootstrap", msg.stepName)
	}
}

func TestHeaderProxy_RenderFinalizeLine_SendsCorrectMsg(t *testing.T) {
	send, got := capturingSend(t)
	p := NewHeaderProxy(send)

	p.RenderFinalizeLine(2, 4, "Cleanup")

	msg, ok := (*got).(headerFinalizeLineMsg)
	if !ok {
		t.Fatalf("expected headerFinalizeLineMsg, got %T", *got)
	}
	if msg.stepNum != 2 {
		t.Errorf("stepNum: want 2, got %d", msg.stepNum)
	}
	if msg.stepCount != 4 {
		t.Errorf("stepCount: want 4, got %d", msg.stepCount)
	}
	if msg.stepName != "Cleanup" {
		t.Errorf("stepName: want %q, got %q", "Cleanup", msg.stepName)
	}
}

func TestHeaderProxy_SetPhaseSteps_SendsCorrectMsg(t *testing.T) {
	send, got := capturingSend(t)
	p := NewHeaderProxy(send)

	names := []string{"Alpha", "Beta", "Gamma"}
	p.SetPhaseSteps(names)

	msg, ok := (*got).(headerPhaseStepsMsg)
	if !ok {
		t.Fatalf("expected headerPhaseStepsMsg, got %T", *got)
	}
	if len(msg.names) != len(names) {
		t.Fatalf("names len: want %d, got %d", len(names), len(msg.names))
	}
	for i, want := range names {
		if msg.names[i] != want {
			t.Errorf("names[%d]: want %q, got %q", i, want, msg.names[i])
		}
	}
}
