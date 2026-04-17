package workflow

import (
	"fmt"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/mxriverlynn/pr9k/ralph-tui/internal/claudestream"
	"github.com/mxriverlynn/pr9k/ralph-tui/internal/preflight"
	"github.com/mxriverlynn/pr9k/ralph-tui/internal/sandbox"
	"github.com/mxriverlynn/pr9k/ralph-tui/internal/statusline"
	"github.com/mxriverlynn/pr9k/ralph-tui/internal/steps"
	"github.com/mxriverlynn/pr9k/ralph-tui/internal/ui"
	"github.com/mxriverlynn/pr9k/ralph-tui/internal/vars"
	"github.com/mxriverlynn/pr9k/ralph-tui/internal/version"
)

// StatusRunner is the interface for driving status-line refreshes from the
// workflow goroutine. *statusline.Runner satisfies this interface.
// A nil StatusRunner is safe: all push/trigger calls check for nil first.
type StatusRunner interface {
	PushState(statusline.State)
	Trigger()
}

// buildState snapshots the current workflow state into a statusline.State.
// It reads all built-in variables from vt using the given phase's resolution
// rules and copies non-built-in captures as a defensive map. sessionID and ver
// are forwarded verbatim.
func buildState(vt *vars.VarTable, phase vars.Phase, sessionID, ver string) statusline.State {
	getInt := func(name string) int {
		v, _ := vt.GetInPhase(phase, name)
		n, _ := strconv.Atoi(v)
		return n
	}
	getString := func(name string) string {
		v, _ := vt.GetInPhase(phase, name)
		return v
	}
	phaseStr := "initialize"
	switch phase {
	case vars.Iteration:
		phaseStr = "iteration"
	case vars.Finalize:
		phaseStr = "finalize"
	}
	return statusline.State{
		SessionID:     sessionID,
		Version:       ver,
		Phase:         phaseStr,
		Iteration:     getInt("ITER"),
		MaxIterations: getInt("MAX_ITER"),
		StepNum:       getInt("STEP_NUM"),
		StepCount:     getInt("STEP_COUNT"),
		StepName:      getString("STEP_NAME"),
		WorkflowDir:   getString("WORKFLOW_DIR"),
		ProjectDir:    getString("PROJECT_DIR"),
		Captures:      vt.AllCaptures(phase),
	}
}

// StepExecutor is the interface for running workflow steps and capturing command output.
// *Runner satisfies this interface.
type StepExecutor interface {
	ui.StepRunner
	LastCapture() string
	LastStats() claudestream.StepStats
	ProjectDir() string
	RunSandboxedStep(stepName string, command []string, opts SandboxOptions) error
	// WriteRunSummary writes line to both the TUI and the file logger. Used for
	// the run-level cumulative summary (D13 2c) so it is visible in the TUI and
	// persisted to disk, unlike WriteToLog which only sends to the TUI.
	WriteRunSummary(line string)
}

// RunStats accumulates StepStats across all claude step invocations in a run
// (D21). It lives in Run's stack frame and is never accessed from another
// goroutine (D25 — no mutex required).
type runStats struct {
	invocations int
	retries     int
	total       claudestream.StepStats
}

func (rs *runStats) add(s claudestream.StepStats, isRetry bool) {
	rs.invocations++
	if isRetry {
		rs.retries++
	}
	rs.total.InputTokens += s.InputTokens
	rs.total.OutputTokens += s.OutputTokens
	rs.total.CacheCreationTokens += s.CacheCreationTokens
	rs.total.CacheReadTokens += s.CacheReadTokens
	rs.total.NumTurns += s.NumTurns
	rs.total.TotalCostUSD += s.TotalCostUSD
	rs.total.DurationMS += s.DurationMS
}

// stepDispatcher wraps StepExecutor and implements ui.StepRunner so that
// Orchestrate can call runner.RunStep uniformly. For a step that is marked
// IsClaude, RunStep transparently delegates to the wrapped executor's
// RunSandboxedStep instead. Non-claude steps pass through to RunStep unchanged.
//
// A new stepDispatcher is created for each step so that current always reflects
// the step that is about to be executed. stats holds a pointer to Run's local
// runStats so every invocation (including retry-loop intermediates) is folded in
// immediately after RunSandboxedStep returns (D21). Per-step construction also
// intentionally resets prevFailed between steps — retries only count
// re-executions of the same step, not cross-step continue sequences (M3).
type stepDispatcher struct {
	exec    StepExecutor
	current ui.ResolvedStep
	stats   *runStats
	// prevFailed tracks whether the last RunSandboxedStep call ended in error
	// so we know whether the next call is a retry (for runStats.retries).
	prevFailed bool
}

func (d *stepDispatcher) RunStep(name string, command []string) error {
	if d.current.IsClaude {
		err := d.exec.RunSandboxedStep(name, command, SandboxOptions{
			CidfilePath:  d.current.CidfilePath,
			ArtifactPath: d.current.ArtifactPath,
			CaptureMode:  d.current.CaptureMode,
		})
		// Fold stats regardless of outcome — D21: the spend was real.
		if d.stats != nil {
			d.stats.add(d.exec.LastStats(), d.prevFailed)
		}
		d.prevFailed = err != nil
		return err
	}
	d.prevFailed = false
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
	WorkflowDir string
	Iterations  int
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
	// RunStamp is the per-run identifier used to name the artifact directory
	// (e.g. "ralph-2026-04-14-173022.123"). Populated from Logger.RunStamp() in
	// main.go. When empty, JSONL artifact paths are not populated for claude
	// steps (persistence is skipped).
	RunStamp string
	// Runner is the optional status-line runner. When nil, all PushState and
	// Trigger calls are skipped.
	Runner StatusRunner
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

	// Seed the runner with an initial State immediately after VarTable
	// construction so the timer goroutine never fires against a zero-value State.
	if cfg.Runner != nil {
		cfg.Runner.PushState(buildState(vt, vars.Initialize, cfg.RunStamp, version.Version))
	}

	// push snapshots the current VarTable state and fires a Trigger so the
	// status-line script re-runs after each meaningful mutation. Called after
	// every vt.SetPhase / vt.SetIteration / vt.ResetIteration / vt.SetStep /
	// vt.Bind call.
	push := func(phase vars.Phase) {
		if cfg.Runner == nil {
			return
		}
		cfg.Runner.PushState(buildState(vt, phase, cfg.RunStamp, version.Version))
		cfg.Runner.Trigger()
	}

	// rs accumulates StepStats across all claude step invocations in the run
	// (D21, D25). Owned exclusively by this goroutine — no mutex required.
	// Emitted as the run-level cumulative summary after the finalize phase (D13 2c).
	rs := &runStats{}

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

	// artifactPath builds the per-step .jsonl artifact path for claude steps
	// (D14). Returns "" when cfg.RunStamp is empty (persistence disabled) or
	// when the step is not a claude step.
	artifactPath := func(resolved *ui.ResolvedStep, phasePrefix string, stepIdx int) string {
		if !resolved.IsClaude || cfg.RunStamp == "" {
			return ""
		}
		filename := fmt.Sprintf("%s%02d-%s.jsonl", phasePrefix, stepIdx, claudestream.Slug(resolved.Name))
		return filepath.Join(executor.ProjectDir(), "logs", cfg.RunStamp, filename)
	}

	// 1. Initialize phase: run each step in order, binding captureAs results
	// into the persistent variable table so they are available in all phases.
	vt.SetPhase(vars.Initialize)
	push(vars.Initialize)
	if len(cfg.InitializeSteps) > 0 {
		writePhaseBanner("Initializing")
	}
	for j, s := range cfg.InitializeSteps {
		vt.SetStep(j+1, len(cfg.InitializeSteps), s.Name)
		push(vars.Initialize)
		resolved, err := buildStep(cfg.WorkflowDir, s, vt, vars.Initialize, cfg.Env, executor)
		if err != nil {
			executor.WriteToLog(fmt.Sprintf("Error preparing initialize step: %v", err))
			continue
		}
		if resolved.IsClaude {
			resolved.ArtifactPath = artifactPath(&resolved, "initialize-", j+1)
			resolved.CaptureMode = ui.CaptureResult
		}
		header.RenderInitializeLine(j+1, len(cfg.InitializeSteps), s.Name)
		emitBlank()
		action := ui.Orchestrate([]ui.ResolvedStep{resolved}, &stepDispatcher{exec: executor, current: resolved, stats: rs}, noopHeader{}, keyHandler)
		if action == ui.ActionQuit {
			return RunResult{}
		}
		if s.CaptureAs != "" {
			captured := executor.LastCapture()
			vt.Bind(vars.Initialize, s.CaptureAs, captured)
			push(vars.Initialize)
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
		push(vars.Iteration)
		vt.SetIteration(i)
		push(vars.Iteration)
		vt.SetPhase(vars.Iteration)
		push(vars.Iteration)

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
			push(vars.Iteration)
			resolved, err := buildStep(cfg.WorkflowDir, s, vt, vars.Iteration, cfg.Env, executor)
			if err != nil {
				executor.WriteToLog(fmt.Sprintf("Error preparing steps: %v", err))
				breakOuter = true
				break
			}
			if resolved.IsClaude {
				resolved.ArtifactPath = artifactPath(&resolved, fmt.Sprintf("iter%02d-", i), j+1)
				resolved.CaptureMode = ui.CaptureResult
			}
			emitBlank()
			th := &trackingOffsetIterHeader{h: header, idx: j}
			action := ui.Orchestrate([]ui.ResolvedStep{resolved}, &stepDispatcher{exec: executor, current: resolved, stats: rs}, th, keyHandler)
			if action == ui.ActionQuit {
				return RunResult{IterationsRun: iterationsRun}
			}
			captured := executor.LastCapture()
			if s.CaptureAs != "" {
				vt.Bind(vars.Iteration, s.CaptureAs, captured)
				push(vars.Iteration)
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
	push(vars.Finalize)
	if len(cfg.FinalizeSteps) > 0 {
		writePhaseBanner("Finalizing")
	}
	for j, s := range cfg.FinalizeSteps {
		vt.SetStep(j+1, len(cfg.FinalizeSteps), s.Name)
		push(vars.Finalize)
		resolved, err := buildStep(cfg.WorkflowDir, s, vt, vars.Finalize, cfg.Env, executor)
		if err != nil {
			executor.WriteToLog(fmt.Sprintf("Error preparing finalize step: %v", err))
			continue
		}
		if resolved.IsClaude {
			resolved.ArtifactPath = artifactPath(&resolved, "finalize-", j+1)
			resolved.CaptureMode = ui.CaptureResult
		}
		header.RenderFinalizeLine(j+1, len(cfg.FinalizeSteps), s.Name)
		emitBlank()
		action := ui.Orchestrate([]ui.ResolvedStep{resolved}, &stepDispatcher{exec: executor, current: resolved, stats: rs}, &trackingOffsetIterHeader{h: header, idx: j}, keyHandler)
		if action == ui.ActionQuit {
			return RunResult{IterationsRun: iterationsRun}
		}
	}

	// D13 2c: emit the run-level cumulative summary after all phases complete.
	// Written to both the TUI and the file logger via WriteRunSummary so the
	// total claude spend is persisted to disk (unlike WriteToLog which is TUI-only).
	var runRenderer claudestream.Renderer
	for _, line := range runRenderer.FinalizeRun(rs.invocations, rs.retries, rs.total) {
		emitBlank()
		executor.WriteRunSummary(line)
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
