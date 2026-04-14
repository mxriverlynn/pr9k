package workflow

import (
	"fmt"
	"path/filepath"
	"strings"

	"github.com/mxriverlynn/pr9k/ralph-tui/internal/preflight"
	"github.com/mxriverlynn/pr9k/ralph-tui/internal/sandbox"
	"github.com/mxriverlynn/pr9k/ralph-tui/internal/steps"
	"github.com/mxriverlynn/pr9k/ralph-tui/internal/ui"
	"github.com/mxriverlynn/pr9k/ralph-tui/internal/vars"
)

// StepExecutor is the interface for running workflow steps and capturing command output.
// *Runner satisfies this interface.
type StepExecutor interface {
	ui.StepRunner
	LastCapture() string
	ProjectDir() string
	RunSandboxedStep(stepName string, command []string, opts SandboxOptions) error
}

// stepDispatcher wraps StepExecutor and implements ui.StepRunner so that
// Orchestrate can call runner.RunStep uniformly. For a step that is marked
// IsClaude, RunStep transparently delegates to the wrapped executor's
// RunSandboxedStep instead. Non-claude steps pass through to RunStep unchanged.
//
// A new stepDispatcher is created for each step so that current always reflects
// the step that is about to be executed.
type stepDispatcher struct {
	exec    StepExecutor
	current ui.ResolvedStep
}

func (d *stepDispatcher) RunStep(name string, command []string) error {
	if d.current.IsClaude {
		return d.exec.RunSandboxedStep(name, command, SandboxOptions{CidfilePath: d.current.CidfilePath})
	}
	return d.exec.RunStep(name, command)
}

func (d *stepDispatcher) WasTerminated() bool    { return d.exec.WasTerminated() }
func (d *stepDispatcher) WriteToLog(line string) { d.exec.WriteToLog(line) }

// RunHeader is the interface for updating the TUI status header during workflow execution.
// *ui.StatusHeader satisfies this interface.
type RunHeader interface {
	RenderInitializeLine(stepNum, stepCount int, stepName string)
	RenderIterationLine(iter, maxIter int, issueID string)
	RenderFinalizeLine(stepNum, stepCount int, stepName string)
	SetPhaseSteps(names []string)
	SetStepState(idx int, state ui.StepState)
}

// RunConfig holds all parameters needed by Run.
type RunConfig struct {
	WorkflowDir     string
	Iterations      int
	// Env is the per-workflow env allowlist loaded from the "env" field of
	// ralph-steps.json (StepFile.Env). Combined with sandbox.BuiltinEnvAllowlist
	// when building docker run args for claude steps.
	Env             []string
	InitializeSteps []steps.Step
	Steps           []steps.Step
	FinalizeSteps   []steps.Step
	// LogWidth is the column width to use for full-width log separators
	// (e.g. phase banner underlines). A value of 0 or less falls back to
	// ui.DefaultTerminalWidth. Callers should pass the log panel's visible
	// width so banners fill the panel without wrapping.
	LogWidth int
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
// — initialize, iteration loop, finalize — via VarTable-based substitution.
func Run(executor StepExecutor, header RunHeader, keyHandler *ui.KeyHandler, cfg RunConfig) RunResult {
	vt := vars.New(cfg.WorkflowDir, executor.ProjectDir(), cfg.Iterations)

	logWidth := cfg.LogWidth
	if logWidth <= 0 {
		logWidth = ui.DefaultTerminalWidth
	}

	// emitBlank writes a single blank line to the log body if one is needed
	// to separate the next piece of content from the previous. It is called
	// before each iteration separator, each step's Orchestrate call, and
	// the completion summary. The first call in Run is a no-op so the log
	// does not begin with a leading blank line.
	needBlank := false
	emitBlank := func() {
		if needBlank {
			executor.WriteToLog("")
		}
		needBlank = true
	}

	// writePhaseBanner emits the full-width phase-entry banner: an emit-blank
	// separator (suppressed on the very first log line), the phase name, and
	// a full-width "═" underline. A trailing blank line is supplied by the
	// next content block's emitBlank call.
	writePhaseBanner := func(phaseName string) {
		emitBlank()
		heading, underline := ui.PhaseBanner(phaseName, logWidth)
		executor.WriteToLog(heading)
		executor.WriteToLog(underline)
	}

	// writeCaptureLog appends the "Captured VAR = value" line to the log
	// body directly after a step that defined captureAs, separated from the
	// preceding step output by a blank line for readability.
	writeCaptureLog := func(varName, value string) {
		emitBlank()
		executor.WriteToLog(ui.CaptureLog(varName, value))
	}

	// 1. Initialize phase: run each step in order, binding captureAs results
	// into the persistent variable table so they are available in all phases.
	vt.SetPhase(vars.Initialize)
	if len(cfg.InitializeSteps) > 0 {
		writePhaseBanner("Initializing")
	}
	for j, s := range cfg.InitializeSteps {
		vt.SetStep(j+1, len(cfg.InitializeSteps), s.Name)
		resolved, err := buildStep(cfg.WorkflowDir, s, vt, vars.Initialize, cfg.Env, executor)
		if err != nil {
			executor.WriteToLog(fmt.Sprintf("Error preparing initialize step: %v", err))
			continue
		}
		header.RenderInitializeLine(j+1, len(cfg.InitializeSteps), s.Name)
		emitBlank()
		action := ui.Orchestrate([]ui.ResolvedStep{resolved}, &stepDispatcher{exec: executor, current: resolved}, noopHeader{}, keyHandler)
		if action == ui.ActionQuit {
			return RunResult{}
		}
		if s.CaptureAs != "" {
			captured := executor.LastCapture()
			vt.Bind(vars.Initialize, s.CaptureAs, captured)
			writeCaptureLog(s.CaptureAs, captured)
		}
	}

	// 2. Iteration loop: repeat until the configured limit or until a step with
	// BreakLoopIfEmpty produces empty stdout capture on successful completion.
	writePhaseBanner("Iterations")
	iterationsRun := 0
	for i := 1; cfg.Iterations == 0 || i <= cfg.Iterations; i++ {
		iterationsRun = i
		vt.ResetIteration()
		vt.SetIteration(i)
		vt.SetPhase(vars.Iteration)

		header.RenderIterationLine(i, cfg.Iterations, "")
		iterStepNames := make([]string, len(cfg.Steps))
		for j, s := range cfg.Steps {
			iterStepNames[j] = s.Name
		}
		header.SetPhaseSteps(iterStepNames)

		emitBlank()
		executor.WriteToLog(ui.StepSeparator(fmt.Sprintf("Iteration %d", i)))

		breakOuter := false
		for j, s := range cfg.Steps {
			vt.SetStep(j+1, len(cfg.Steps), s.Name)
			resolved, err := buildStep(cfg.WorkflowDir, s, vt, vars.Iteration, cfg.Env, executor)
			if err != nil {
				executor.WriteToLog(fmt.Sprintf("Error preparing steps: %v", err))
				breakOuter = true
				break
			}
			emitBlank()
			th := &trackingOffsetIterHeader{h: header, idx: j}
			action := ui.Orchestrate([]ui.ResolvedStep{resolved}, &stepDispatcher{exec: executor, current: resolved}, th, keyHandler)
			if action == ui.ActionQuit {
				return RunResult{IterationsRun: iterationsRun}
			}
			captured := executor.LastCapture()
			if s.CaptureAs != "" {
				vt.Bind(vars.Iteration, s.CaptureAs, captured)
				issueID, _ := vt.GetInPhase(vars.Iteration, "ISSUE_ID")
				header.RenderIterationLine(i, cfg.Iterations, issueID)
				writeCaptureLog(s.CaptureAs, captured)
			}
			// BreakLoopIfEmpty fires only on successful completion (StepDone).
			// If the step failed (non-zero exit), the check is skipped so that
			// normal error-mode handling takes effect instead.
			if s.BreakLoopIfEmpty && th.lastState == ui.StepDone && captured == "" {
				for remaining := j + 1; remaining < len(cfg.Steps); remaining++ {
					header.SetStepState(remaining, ui.StepSkipped)
				}
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
	if len(cfg.FinalizeSteps) > 0 {
		writePhaseBanner("Finalizing")
	}
	for j, s := range cfg.FinalizeSteps {
		vt.SetStep(j+1, len(cfg.FinalizeSteps), s.Name)
		resolved, err := buildStep(cfg.WorkflowDir, s, vt, vars.Finalize, cfg.Env, executor)
		if err != nil {
			executor.WriteToLog(fmt.Sprintf("Error preparing finalize step: %v", err))
			continue
		}
		header.RenderFinalizeLine(j+1, len(cfg.FinalizeSteps), s.Name)
		emitBlank()
		action := ui.Orchestrate([]ui.ResolvedStep{resolved}, &stepDispatcher{exec: executor, current: resolved}, &trackingOffsetIterHeader{h: header, idx: j}, keyHandler)
		if action == ui.ActionQuit {
			return RunResult{IterationsRun: iterationsRun}
		}
	}

	// 4. Completion sequence: write summary as the last line of the main
	// body log and return — the caller tears down the TUI.
	emitBlank()
	executor.WriteToLog(ui.CompletionSummary(iterationsRun, len(cfg.FinalizeSteps)))

	return RunResult{IterationsRun: iterationsRun}
}

// buildStep resolves a single step into a runnable ResolvedStep using vt for
// {{VAR}} substitution in the given phase. env is the per-workflow env
// allowlist (StepFile.Env) appended to sandbox.BuiltinEnvAllowlist for claude
// steps. executor provides ProjectDir for the docker bind-mount.
func buildStep(workflowDir string, s steps.Step, vt *vars.VarTable, phase vars.Phase, env []string, executor StepExecutor) (ui.ResolvedStep, error) {
	if s.IsClaude {
		prompt, err := steps.BuildPrompt(workflowDir, s, vt, phase)
		if err != nil {
			return ui.ResolvedStep{}, fmt.Errorf("step %q: %w", s.Name, err)
		}
		uid, gid := sandbox.HostUIDGID()
		cidfile, err := sandbox.Path()
		if err != nil {
			return ui.ResolvedStep{}, fmt.Errorf("step %q: cidfile: %w", s.Name, err)
		}
		profileDir := preflight.ResolveProfileDir()
		projectDir := executor.ProjectDir()
		envAllowlist := append([]string{}, sandbox.BuiltinEnvAllowlist...)
		envAllowlist = append(envAllowlist, env...)
		argv := sandbox.BuildRunArgs(projectDir, profileDir, uid, gid, cidfile, envAllowlist, s.Model, prompt)
		return ui.ResolvedStep{
			Name:        s.Name,
			Command:     argv,
			IsClaude:    true,
			CidfilePath: cidfile,
		}, nil
	}
	return ui.ResolvedStep{
		Name:    s.Name,
		Command: ResolveCommand(workflowDir, s.Command, vt, phase),
	}, nil
}

// ResolveCommand substitutes {{VAR}} tokens in each command element using vt
// and resolves relative script paths against workflowDir.
//
// For each element:
//   - All {{VAR_NAME}} tokens are replaced using the substitution engine.
//   - The first element (the executable) is resolved relative to workflowDir if
//     it is a relative path containing a path separator (i.e. not a bare
//     command like "git").
func ResolveCommand(workflowDir string, command []string, vt *vars.VarTable, phase vars.Phase) []string {
	if len(command) == 0 {
		return command
	}

	result := make([]string, len(command))
	for i, arg := range command {
		// vars.Substitute currently always returns a nil error; the blank
		// identifier is intentional. If Substitute ever gains a strict mode that
		// returns errors for unresolved variables, this site must propagate them
		// rather than silently substituting the empty string.
		substituted, _ := vars.Substitute(arg, vt, phase)
		result[i] = substituted
	}

	// Resolve the executable if it looks like a relative script path.
	exe := result[0]
	if !filepath.IsAbs(exe) && strings.ContainsRune(exe, '/') {
		result[0] = filepath.Join(workflowDir, exe)
	}

	return result
}
