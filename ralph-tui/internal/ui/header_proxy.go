package ui

import tea "github.com/charmbracelet/bubbletea"

// headerProxy implements workflow.RunHeader and ui.StepHeader by sending
// header messages into the Bubble Tea program via program.Send. The
// orchestration goroutine never touches the StatusHeader directly — it calls
// proxy methods, which program.Send into the Update loop where headerModel
// applies the mutations safely on the Update goroutine.
type headerProxy struct {
	send func(tea.Msg)
}

// NewHeaderProxy creates a headerProxy backed by program.Send. Pass
// program.Send after calling tea.NewProgram.
func NewHeaderProxy(send func(tea.Msg)) *headerProxy {
	return &headerProxy{send: send}
}

func (p *headerProxy) SetStepState(idx int, state StepState) {
	p.send(headerStepStateMsg{idx: idx, state: state})
}

func (p *headerProxy) RenderIterationLine(iter, max int, issue string) {
	p.send(headerIterationLineMsg{iter: iter, max: max, issue: issue})
}

func (p *headerProxy) RenderInitializeLine(stepNum, stepCount int, stepName string) {
	p.send(headerInitializeLineMsg{stepNum: stepNum, stepCount: stepCount, stepName: stepName})
}

func (p *headerProxy) RenderFinalizeLine(stepNum, stepCount int, stepName string) {
	p.send(headerFinalizeLineMsg{stepNum: stepNum, stepCount: stepCount, stepName: stepName})
}

func (p *headerProxy) SetPhaseSteps(names []string) {
	p.send(headerPhaseStepsMsg{names: names})
}
