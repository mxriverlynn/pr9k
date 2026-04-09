package ui

// StepRunner is the workflow execution interface required by the workflow execution loop and HandleStepError.
type StepRunner interface {
	RunStep(name string, command []string) error
	WasTerminated() bool
	WriteToLog(line string)
}

// StepHeader updates the visual checkbox state for workflow steps.
type StepHeader interface {
	SetStepState(idx int, state StepState)
}

// ResolvedStep holds a step's name and its fully-resolved command argv.
type ResolvedStep struct {
	Name    string
	Command []string
}

// HandleStepError is called after a step fails. If the step was user-terminated,
// it marks the step done and returns ActionContinue immediately. Otherwise it
// enters error mode, blocks on h.Actions until the user decides, restores
// normal mode, and returns the chosen action (ActionContinue, ActionRetry, or
// ActionQuit). The caller is responsible for writing a retry separator and
// re-running the step when ActionRetry is returned.
func HandleStepError(runner StepRunner, header StepHeader, h *KeyHandler, stepIdx int) StepAction {
	if runner.WasTerminated() {
		header.SetStepState(stepIdx, StepDone)
		return ActionContinue
	}

	header.SetStepState(stepIdx, StepFailed)
	h.SetMode(ModeError)

	action := <-h.Actions
	h.SetMode(ModeNormal)

	return action
}
