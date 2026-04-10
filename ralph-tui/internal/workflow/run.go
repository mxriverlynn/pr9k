package workflow

import (
	"fmt"
	"path/filepath"
	"strings"

	"github.com/mxriverlynn/pr9k/ralph-tui/internal/steps"
	"github.com/mxriverlynn/pr9k/ralph-tui/internal/ui"
	"github.com/mxriverlynn/pr9k/ralph-tui/internal/vars"
)

// StepExecutor is the interface for running workflow steps and capturing command output.
// *Runner satisfies this interface.
type StepExecutor interface {
	ui.StepRunner
	LastCapture() string
	Close() error
}

// RunHeader is the interface for updating the TUI status header during workflow execution.
// *ui.StatusHeader satisfies this interface.
type RunHeader interface {
	SetIteration(current, total int, issueID, issueTitle string)
	SetPhaseSteps(names []string)
	SetStepState(idx int, state ui.StepState)
}

// RunConfig holds all parameters needed by Run.
type RunConfig struct {
	ProjectDir      string
	Iterations      int
	InitializeSteps []steps.Step
	Steps           []steps.Step
	FinalizeSteps   []steps.Step
}

// noopHeader satisfies ui.StepHeader with no-op methods. Used for phases (e.g.
// initialize) that do not update the TUI step-checkbox display.
type noopHeader struct{}

func (noopHeader) SetStepState(int, ui.StepState) {}

// trackingOffsetIterHeader adapts RunHeader to ui.StepHeader for a single
// iteration step at absolute index idx. It also records the last StepState
// set so Run can check whether the step ended as StepDone before consulting
// BreakLoopIfEmpty.
type trackingOffsetIterHeader struct {
	h         RunHeader
	idx       int
	lastState ui.StepState
}

func (a *trackingOffsetIterHeader) SetStepState(_ int, state ui.StepState) {
	a.lastState = state
	a.h.SetStepState(a.idx, state)
}

// RunResult holds the outcome of a completed Run call.
type RunResult struct {
	// IterationsRun is the index of the last iteration that began (1-based).
	// It includes the iteration that triggered a breakLoopIfEmpty exit.
	// Zero if the iteration loop never started.
	IterationsRun int
}

// Run is the main orchestration goroutine. It drives three config-defined phases
// — initialize, iteration loop, finalize — via VarTable-based substitution, and
// closes the executor when done.
func Run(executor StepExecutor, header RunHeader, keyHandler *ui.KeyHandler, cfg RunConfig) RunResult {
	vt := vars.New(cfg.ProjectDir, cfg.Iterations)

	// 1. Initialize phase: run each step in order, binding captureAs results
	// into the persistent variable table so they are available in all phases.
	vt.SetPhase(vars.Initialize)
	for j, s := range cfg.InitializeSteps {
		vt.SetStep(j+1, len(cfg.InitializeSteps), s.Name)
		resolved, err := buildStep(cfg.ProjectDir, s, vt, vars.Initialize)
		if err != nil {
			executor.WriteToLog(fmt.Sprintf("Error preparing initialize step: %v", err))
			continue
		}
		action := ui.Orchestrate([]ui.ResolvedStep{resolved}, executor, noopHeader{}, keyHandler)
		if action == ui.ActionQuit {
			_ = executor.Close()
			return RunResult{}
		}
		if s.CaptureAs != "" {
			vt.Bind(vars.Initialize, s.CaptureAs, executor.LastCapture())
		}
	}

	// 2. Iteration loop: repeat until the configured limit or until a step with
	// BreakLoopIfEmpty produces empty stdout capture on successful completion.
	iterationsRun := 0
	for i := 1; cfg.Iterations == 0 || i <= cfg.Iterations; i++ {
		iterationsRun = i
		vt.ResetIteration()
		vt.SetIteration(i)
		vt.SetPhase(vars.Iteration)

		header.SetIteration(i, cfg.Iterations, "", "")
		iterStepNames := make([]string, len(cfg.Steps))
		for j, s := range cfg.Steps {
			iterStepNames[j] = s.Name
		}
		header.SetPhaseSteps(iterStepNames)

		executor.WriteToLog(ui.StepSeparator(fmt.Sprintf("Iteration %d", i)))

		breakOuter := false
		for j, s := range cfg.Steps {
			vt.SetStep(j+1, len(cfg.Steps), s.Name)
			resolved, err := buildStep(cfg.ProjectDir, s, vt, vars.Iteration)
			if err != nil {
				executor.WriteToLog(fmt.Sprintf("Error preparing steps: %v", err))
				breakOuter = true
				break
			}
			th := &trackingOffsetIterHeader{h: header, idx: j}
			action := ui.Orchestrate([]ui.ResolvedStep{resolved}, executor, th, keyHandler)
			if action == ui.ActionQuit {
				_ = executor.Close()
				return RunResult{IterationsRun: iterationsRun}
			}
			captured := executor.LastCapture()
			if s.CaptureAs != "" {
				vt.Bind(vars.Iteration, s.CaptureAs, captured)
			}
			// BreakLoopIfEmpty fires only on successful completion (StepDone).
			// If the step failed (non-zero exit), the check is skipped so that
			// normal error-mode handling takes effect instead.
			if s.BreakLoopIfEmpty && th.lastState == ui.StepDone && captured == "" {
				breakOuter = true
				break
			}
		}
		if breakOuter {
			break
		}
	}

	// 3. Finalization phase: runs even after an early loop exit.
	finalizeNames := make([]string, len(cfg.FinalizeSteps))
	for i, s := range cfg.FinalizeSteps {
		finalizeNames[i] = s.Name
	}
	header.SetPhaseSteps(finalizeNames)

	vt.SetPhase(vars.Finalize)
	for j, s := range cfg.FinalizeSteps {
		vt.SetStep(j+1, len(cfg.FinalizeSteps), s.Name)
		resolved, err := buildStep(cfg.ProjectDir, s, vt, vars.Finalize)
		if err != nil {
			executor.WriteToLog(fmt.Sprintf("Error preparing finalize step: %v", err))
			continue
		}
		action := ui.Orchestrate([]ui.ResolvedStep{resolved}, executor, &trackingOffsetIterHeader{h: header, idx: j}, keyHandler)
		if action == ui.ActionQuit {
			_ = executor.Close()
			return RunResult{IterationsRun: iterationsRun}
		}
	}

	// 4. Close executor (sends EOF to log pipe).
	_ = executor.Close()
	return RunResult{IterationsRun: iterationsRun}
}

// buildStep resolves a single step into a runnable ResolvedStep using vt for
// {{VAR}} substitution in the given phase.
func buildStep(projectDir string, s steps.Step, vt *vars.VarTable, phase vars.Phase) (ui.ResolvedStep, error) {
	if s.IsClaude {
		prompt, err := steps.BuildPrompt(projectDir, s, vt, phase)
		if err != nil {
			return ui.ResolvedStep{}, fmt.Errorf("step %q: %w", s.Name, err)
		}
		return ui.ResolvedStep{
			Name:    s.Name,
			Command: []string{"claude", "--permission-mode", "acceptEdits", "--model", s.Model, "-p", prompt},
		}, nil
	}
	return ui.ResolvedStep{
		Name:    s.Name,
		Command: ResolveCommand(projectDir, s.Command, vt, phase),
	}, nil
}

// ResolveCommand substitutes {{VAR}} tokens in each command element using vt
// and resolves relative script paths against projectDir.
//
// For each element:
//   - All {{VAR_NAME}} tokens are replaced using the substitution engine.
//   - The first element (the executable) is resolved relative to projectDir if
//     it is a relative path containing a path separator (i.e. not a bare
//     command like "git").
func ResolveCommand(projectDir string, command []string, vt *vars.VarTable, phase vars.Phase) []string {
	if len(command) == 0 {
		return command
	}

	result := make([]string, len(command))
	for i, arg := range command {
		substituted, _ := vars.Substitute(arg, vt, phase)
		result[i] = substituted
	}

	// Resolve the executable if it looks like a relative script path.
	exe := result[0]
	if !filepath.IsAbs(exe) && strings.ContainsRune(exe, '/') {
		result[0] = filepath.Join(projectDir, exe)
	}

	return result
}
