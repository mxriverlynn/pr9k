package workflow

import (
	_ "embed"
	"fmt"
	"path/filepath"
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
	SetIteration(current, total int, issueID, issueTitle string)
	SetStepState(idx int, state ui.StepState)
	SetFinalization(current, total int, steps []string)
	SetFinalizeStepState(idx int, state ui.StepState)
}

// RunConfig holds all parameters needed by Run.
type RunConfig struct {
	ProjectDir    string
	Iterations    int
	Steps         []steps.Step
	FinalizeSteps []steps.Step
}

// iterHeader adapts RunHeader to ui.StepHeader for iteration steps.
type iterHeader struct{ h RunHeader }

func (a *iterHeader) SetStepState(idx int, state ui.StepState) { a.h.SetStepState(idx, state) }

// finalHeader adapts RunHeader to ui.StepHeader for finalization steps.
type finalHeader struct{ h RunHeader }

func (a *finalHeader) SetStepState(idx int, state ui.StepState) {
	a.h.SetFinalizeStepState(idx, state)
}

// Run is the main orchestration goroutine. It displays the startup banner,
// fetches the GitHub username, runs N workflow iterations, executes the
// finalization phase, writes the completion summary, and closes the executor.
func Run(executor StepExecutor, header RunHeader, keyHandler *ui.KeyHandler, cfg RunConfig) {
	// 1. Display embedded banner.
	for _, line := range strings.Split(bannerArt, "\n") {
		executor.WriteToLog(line)
	}

	// 2. Get GitHub username.
	userScript := filepath.Join(cfg.ProjectDir, "scripts", "get_gh_user")
	username, err := executor.CaptureOutput([]string{userScript})
	if err != nil {
		executor.WriteToLog(fmt.Sprintf("Warning: could not get GitHub user: %v", err))
	}

	// 3. Iteration loop.
	iterationsRun := 0
	for i := 1; i <= cfg.Iterations; i++ {
		issueScript := filepath.Join(cfg.ProjectDir, "scripts", "get_next_issue")
		issueID, _ := executor.CaptureOutput([]string{issueScript, username})

		if issueID == "" {
			executor.WriteToLog(fmt.Sprintf("Iteration %d/%d — No issue found. Exiting loop.", i, cfg.Iterations))
			break
		}

		sha, shaErr := executor.CaptureOutput([]string{"git", "rev-parse", "HEAD"})
		if shaErr != nil {
			executor.WriteToLog(fmt.Sprintf("Warning: could not get HEAD SHA: %v", shaErr))
		}

		header.SetIteration(i, cfg.Iterations, issueID, "")
		for j := range cfg.Steps {
			header.SetStepState(j, ui.StepPending)
		}

		executor.WriteToLog(ui.StepSeparator(fmt.Sprintf("Iteration %d/%d — Issue #%s", i, cfg.Iterations, issueID)))

		resolvedSteps, err := buildIterationSteps(cfg.ProjectDir, cfg.Steps, issueID, sha)
		if err != nil {
			executor.WriteToLog(fmt.Sprintf("Error preparing steps: %v", err))
			break
		}

		action := ui.Orchestrate(resolvedSteps, executor, &iterHeader{header}, keyHandler)
		if action == ui.ActionQuit {
			_ = executor.Close()
			return
		}

		iterationsRun++
	}

	// 4. Finalization phase (runs even after early loop exit).
	finalizeNames := make([]string, len(cfg.FinalizeSteps))
	for i, s := range cfg.FinalizeSteps {
		finalizeNames[i] = s.Name
	}
	header.SetFinalization(1, len(cfg.FinalizeSteps), finalizeNames)

	finalResolvedSteps, err := buildFinalizeSteps(cfg.ProjectDir, cfg.FinalizeSteps)
	if err == nil {
		action := ui.Orchestrate(finalResolvedSteps, executor, &finalHeader{header}, keyHandler)
		if action == ui.ActionQuit {
			_ = executor.Close()
			return
		}
	}

	// 5. Completion summary.
	executor.WriteToLog(fmt.Sprintf("Ralph completed after %d iteration(s) and %d finalizing tasks.",
		iterationsRun, len(cfg.FinalizeSteps)))

	// 6. Close executor (sends EOF to log pipe).
	_ = executor.Close()
}

func buildIterationSteps(projectDir string, stepsConfig []steps.Step, issueID, sha string) ([]ui.ResolvedStep, error) {
	vars := map[string]string{
		"ISSUE_ID":    issueID,
		"ISSUENUMBER": issueID,
		"STARTINGSHA": sha,
	}
	result := make([]ui.ResolvedStep, len(stepsConfig))
	for i, s := range stepsConfig {
		if s.IsClaudeStep() {
			prompt, err := steps.BuildPrompt(projectDir, s, vars)
			if err != nil {
				return nil, fmt.Errorf("step %q: %w", s.Name, err)
			}
			result[i] = ui.ResolvedStep{
				Name:    s.Name,
				Command: []string{"claude", "--permission-mode", "acceptEdits", "--model", s.Model, "-p", prompt},
			}
		} else {
			result[i] = ui.ResolvedStep{
				Name:    s.Name,
				Command: ResolveCommand(projectDir, s.Command, vars),
			}
		}
	}
	return result, nil
}

func buildFinalizeSteps(projectDir string, stepsConfig []steps.Step) ([]ui.ResolvedStep, error) {
	result := make([]ui.ResolvedStep, len(stepsConfig))
	for i, s := range stepsConfig {
		if s.IsClaudeStep() {
			prompt, err := steps.BuildPrompt(projectDir, s, nil)
			if err != nil {
				return nil, fmt.Errorf("finalize step %q: %w", s.Name, err)
			}
			result[i] = ui.ResolvedStep{
				Name:    s.Name,
				Command: []string{"claude", "--permission-mode", "acceptEdits", "--model", s.Model, "-p", prompt},
			}
		} else {
			result[i] = ui.ResolvedStep{
				Name:    s.Name,
				Command: ResolveCommand(projectDir, s.Command, nil),
			}
		}
	}
	return result, nil
}
