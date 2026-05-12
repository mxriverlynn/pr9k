package ui

import tea "github.com/charmbracelet/bubbletea"

// HeaderProxy implements workflow.RunHeader and ui.StepHeader by sending
// header messages into the Bubble Tea program via program.Send. The
// orchestration goroutine never touches the StatusHeader directly — it calls
// proxy methods, which program.Send into the Update loop where headerModel
// applies the mutations safely on the Update goroutine.
type HeaderProxy struct {
	send func(tea.Msg)
}

// NewHeaderProxy creates a HeaderProxy backed by program.Send. Pass
// program.Send after calling tea.NewProgram.
func NewHeaderProxy(send func(tea.Msg)) *HeaderProxy {
	return &HeaderProxy{send: send}
}

func (p *HeaderProxy) SetStepState(idx int, state StepState) {
	p.send(headerStepStateMsg{idx: idx, state: state})
}

func (p *HeaderProxy) RenderIterationLine(iter, max int, issue string) {
	p.send(headerIterationLineMsg{iter: iter, max: max, issue: issue})
}

func (p *HeaderProxy) RenderInitializeLine(stepNum, stepCount int, stepName string) {
	p.send(headerInitializeLineMsg{stepNum: stepNum, stepCount: stepCount, stepName: stepName})
}

func (p *HeaderProxy) RenderFinalizeLine(stepNum, stepCount int, stepName string) {
	p.send(headerFinalizeLineMsg{stepNum: stepNum, stepCount: stepCount, stepName: stepName})
}

func (p *HeaderProxy) SetPhaseSteps(names []string) {
	p.send(headerPhaseStepsMsg{names: names})
}
