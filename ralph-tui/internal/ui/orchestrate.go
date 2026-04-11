package ui

// StepRunner is the workflow execution interface required by Orchestrate.
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

// Orchestrate runs steps in sequence. On step failure (non-zero exit, not
// user-initiated), it enters error mode and blocks on h.Actions until the
// user decides to continue, retry, or quit.
// Returns ActionQuit if the user quit; ActionContinue when all steps complete.
func Orchestrate(steps []ResolvedStep, runner StepRunner, header StepHeader, h *KeyHandler) StepAction {
	for i, step := range steps {
		// Check for a pending quit (e.g. injected by an OS signal) before starting each step.
		select {
		case action := <-h.Actions:
			if action == ActionQuit {
				return ActionQuit
			}
		default:
		}

		// Write the "Starting step: <name>" banner to the log body so every
		// started step has a visible heading, followed by a blank line that
		// separates the heading from the step's streamed output.
		heading, underline := StepStartBanner(step.Name)
		runner.WriteToLog(heading)
		runner.WriteToLog(underline)
		runner.WriteToLog("")

		header.SetStepState(i, StepActive)
		action := runStepWithErrorHandling(i, step, runner, header, h)
		if action == ActionQuit {
			return ActionQuit
		}
	}
	return ActionContinue
}

func runStepWithErrorHandling(idx int, step ResolvedStep, runner StepRunner, header StepHeader, h *KeyHandler) StepAction {
	for {
		err := runner.RunStep(step.Name, step.Command)

		if err == nil || runner.WasTerminated() {
			header.SetStepState(idx, StepDone)
			return ActionContinue
		}

		header.SetStepState(idx, StepFailed)
		h.SetMode(ModeError)

		action := <-h.Actions
		h.SetMode(ModeNormal)

		switch action {
		case ActionContinue:
			// Step stays [✗]; advance to next step.
			return ActionContinue
		case ActionRetry:
			runner.WriteToLog(RetryStepSeparator(step.Name))
			// Loop back to re-run the step.
		case ActionQuit:
			return ActionQuit
		}
	}
}
