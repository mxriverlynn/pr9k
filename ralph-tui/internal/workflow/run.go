package workflow

import (
	_ "embed"
	"fmt"
	"strings"

	"github.com/mxriverlynn/pr9k/ralph-tui/internal/steps"
	"github.com/mxriverlynn/pr9k/ralph-tui/internal/ui"
)

//go:embed ralph-art.txt
var bannerArt string

// StepExecutor is the interface for running workflow steps and capturing command output.
// *Runner satisfies this interface.
type StepExecutor interface {
	ui.StepRunner
	CaptureOutput(command []string) (string, error)
	Close() error
}

// RunHeader is the interface for updating the TUI status header during workflow execution.
// *ui.StatusHeader satisfies this interface.
type RunHeader interface {
	SetPhaseSteps(label string, names []string)
	SetStepState(idx int, state ui.StepState)
}

// RunConfig holds all parameters needed by Run.
type RunConfig struct {
	ProjectDir string
	Iterations int
	Config     *steps.WorkflowConfig
}

// Run is the main orchestration goroutine. It displays the startup banner,
// executes the three-phase workflow (pre-loop, loop, post-loop) driven by the
// config, writes the completion summary, and closes the executor.
//
// Pre-loop steps run once before any iteration. Loop steps run for each
// iteration; a step with exitLoopIfEmpty that captures an empty string breaks
// the iteration loop early. Post-loop always runs regardless of how the loop
// exits. Variables captured by command steps with outputVariable are stored in
// the pool and substituted just-in-time into subsequent steps.
func Run(executor StepExecutor, header RunHeader, keyHandler *ui.KeyHandler, cfg RunConfig) {
	// 1. Display embedded banner.
	for _, line := range strings.Split(bannerArt, "\n") {
		executor.WriteToLog(line)
	}

	pool := NewVariablePool()
	loopVarNames := LoopVariableNames(cfg.Config)

	// 2. Phase 1: pre-loop.
	header.SetPhaseSteps("Pre-loop", phaseStepNames(cfg.Config.PreLoop))
	if quit := runPhase(executor, header, keyHandler, cfg.Config.PreLoop, cfg.ProjectDir, pool); quit {
		_ = executor.Close()
		return
	}

	// 3. Phase 2: loop.
	loopNames := phaseStepNames(cfg.Config.Loop)
	exitLoop := false
	iterationsRun := 0
	for i := 1; i <= cfg.Iterations && !exitLoop; i++ {
		pool.Clear(loopVarNames)
		header.SetPhaseSteps(fmt.Sprintf("Iteration %d/%d", i, cfg.Iterations), loopNames)

		for j, step := range cfg.Config.Loop {
			// Pre-step quit drain: honour a pending quit (e.g. from an OS signal)
			// before starting the next step.
			select {
			case action := <-keyHandler.Actions:
				if action == ui.ActionQuit {
					_ = executor.Close()
					return
				}
			default:
			}

			shouldExitLoop, quit := executeStep(executor, header, keyHandler, j, step, cfg.ProjectDir, pool)
			if quit {
				_ = executor.Close()
				return
			}
			if shouldExitLoop {
				exitLoop = true
				break
			}
		}
		if !exitLoop {
			iterationsRun++
		}
	}

	// 4. Phase 3: post-loop (always runs).
	header.SetPhaseSteps("Post-loop", phaseStepNames(cfg.Config.PostLoop))
	if quit := runPhase(executor, header, keyHandler, cfg.Config.PostLoop, cfg.ProjectDir, pool); quit {
		_ = executor.Close()
		return
	}

	// 5. Completion summary.
	executor.WriteToLog(fmt.Sprintf("Ralph completed after %d iteration(s).", iterationsRun))

	// 6. Close executor (sends EOF to log pipe).
	_ = executor.Close()
}

// runPhase executes all steps in a slice sequentially. It returns true if the
// user quit, false if all steps completed (possibly with errors the user continued
// past). It also drains the quit channel before each step.
func runPhase(executor StepExecutor, header RunHeader, keyHandler *ui.KeyHandler, phase []steps.Step, projectDir string, pool *VariablePool) (quit bool) {
	for i, step := range phase {
		select {
		case action := <-keyHandler.Actions:
			if action == ui.ActionQuit {
				return true
			}
		default:
		}

		_, q := executeStep(executor, header, keyHandler, i, step, projectDir, pool)
		if q {
			return true
		}
	}
	return false
}

// executeStep resolves and executes a single step. It returns (exitLoop, quit).
// exitLoop is true when the step has exitLoopIfEmpty set and captured an empty
// string. quit is true when the user chose to quit during error recovery.
//
// For command steps with outputVariable, stdout is captured silently and stored
// in the pool. All other steps stream output to the TUI via RunStep.
func executeStep(executor StepExecutor, header RunHeader, keyHandler *ui.KeyHandler, idx int, step steps.Step, projectDir string, pool *VariablePool) (exitLoop bool, quit bool) {
	vars := pool.All()
	var cmd []string
	if step.IsClaudeStep() {
		prompt, err := steps.BuildPrompt(projectDir, step, vars)
		if err != nil {
			executor.WriteToLog(fmt.Sprintf("workflow: step %q: could not build prompt: %v", step.Name, err))
			return false, false
		}
		cmd = []string{"claude", "--permission-mode", step.DefaultPermissionMode(), "--model", step.DefaultModel(), "-p", prompt}
	} else {
		cmd = ResolveCommand(projectDir, step.Command, vars)
	}

	for {
		header.SetStepState(idx, ui.StepActive)

		var err error
		var captured string

		if step.OutputVariable != "" {
			captured, err = executor.CaptureOutput(cmd)
		} else {
			err = executor.RunStep(step.Name, cmd)
		}

		if err == nil {
			header.SetStepState(idx, ui.StepDone)
			if step.OutputVariable != "" {
				pool.Set(step.OutputVariable, captured)
				if step.ExitLoopIfEmpty && strings.TrimSpace(captured) == "" {
					return true, false
				}
			}
			return false, false
		}

		// Step failed — enter error recovery.
		action := ui.HandleStepError(executor, header, keyHandler, idx)
		switch action {
		case ui.ActionQuit:
			return false, true
		case ui.ActionRetry:
			executor.WriteToLog(ui.RetryStepSeparator(step.Name))
			// loop back to retry
		case ui.ActionContinue:
			return false, false
		}
	}
}

// phaseStepNames extracts the Name field from each step in a phase.
func phaseStepNames(phase []steps.Step) []string {
	names := make([]string, len(phase))
	for i, s := range phase {
		names[i] = s.Name
	}
	return names
}
