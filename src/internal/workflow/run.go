package workflow

import (
	"fmt"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/mxriverlynn/pr9k/src/internal/claudestream"
	"github.com/mxriverlynn/pr9k/src/internal/preflight"
	"github.com/mxriverlynn/pr9k/src/internal/sandbox"
	"github.com/mxriverlynn/pr9k/src/internal/statusline"
	"github.com/mxriverlynn/pr9k/src/internal/steps"
	"github.com/mxriverlynn/pr9k/src/internal/ui"
	"github.com/mxriverlynn/pr9k/src/internal/vars"
	"github.com/mxriverlynn/pr9k/src/internal/version"
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
// *Runner satisfies this interface. WasTimedOut and ClearTimeoutFlag are
// inherited from ui.StepRunner (embedded above).
type StepExecutor interface {
	ui.StepRunner
	LastCapture() string
	LastStats() claudestream.StepStats
	ProjectDir() string
	RunSandboxedStep(stepName string, command []string, opts SandboxOptions) error
	RunStepFull(stepName string, command []string, captureMode ui.CaptureMode, timeoutSeconds int) error
	// SessionBlacklisted reports whether id is in the session blacklist (timed-out
	// sessions). Used by evaluateResumeGates (G5). The zero-value implementation
	// (returning false for every id) is safe and correct for test doubles that
	// do not need to exercise G5.
	SessionBlacklisted(id string) bool
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
	// capturedStats holds the StepStats from the most recent RunSandboxedStep
	// call. Captured here (rather than calling LastStats again later) so that
	// Run can read per-step stats for IterationRecord without a second call.
	capturedStats claudestream.StepStats
	// onTimeoutRetry, when non-nil, is called at the start of RunStep whenever
	// the previous call timed out. Run uses this to emit an IterationRecord for
	// the timed-out attempt before the retry begins (WARN-001).
	onTimeoutRetry func()
}

func (d *stepDispatcher) RunStep(name string, command []string) error {
	// WARN-001: If the previous attempt timed out, emit a record for it before
	// the retry resets timeoutFired. WasTimedOut() is still true here because
	// RunStepFull/RunSandboxedStep reset the flag only at their entry.
	if d.exec.WasTimedOut() && d.onTimeoutRetry != nil {
		d.onTimeoutRetry()
	}
	if d.current.IsClaude {
		err := d.exec.RunSandboxedStep(name, command, SandboxOptions{
			CidfilePath:    d.current.CidfilePath,
			ArtifactPath:   d.current.ArtifactPath,
			CaptureMode:    d.current.CaptureMode,
			TimeoutSeconds: d.current.TimeoutSeconds,
		})
		// Capture stats once — both for D21 accounting and for IterationRecord.
		s := d.exec.LastStats()
		d.capturedStats = s
		if d.stats != nil {
			d.stats.add(s, d.prevFailed)
		}
		d.prevFailed = err != nil
		return err
	}
	d.prevFailed = false
	return d.exec.RunStepFull(name, command, d.current.CaptureMode, d.current.TimeoutSeconds)
}

func (d *stepDispatcher) WasTerminated() bool    { return d.exec.WasTerminated() }
func (d *stepDispatcher) WasTimedOut() bool      { return d.exec.WasTimedOut() }
func (d *stepDispatcher) ClearTimeoutFlag()      { d.exec.ClearTimeoutFlag() }
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
	// config.json (StepFile.Env). Combined with sandbox.BuiltinEnvAllowlist
	// when building docker run args for claude steps.
	Env []string
	// ContainerEnv is the per-workflow literal env map from the "containerEnv"
	// field of config.json. Each entry is injected as -e KEY=VALUE into
	// the Docker command. Emitted after Env allowlist entries so containerEnv
	// wins on collision (Docker last-wins).
	ContainerEnv    map[string]string
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

// stateTracker is a ui.StepHeader that records the last StepState set without
// any visible TUI output. Used by the initialize phase so AppendIterationRecord
// can determine step success or failure after Orchestrate returns.
type stateTracker struct {
	lastState ui.StepState
}

func (s *stateTracker) SetStepState(_ int, state ui.StepState) {
	s.lastState = state
}

// stepStatus converts a ui.StepState to the IterationRecord Status string.
// StepTimedOutContinuing maps to "failed" so the iteration log preserves the
// signal that a timeout fired; setTimeoutNote attaches the "timed out after
// Ns" note alongside.
func stepStatus(state ui.StepState) string {
	switch state {
	case ui.StepDone, ui.StepActive:
		return "done"
	case ui.StepFailed, ui.StepTimedOutContinuing:
		return "failed"
	case ui.StepSkipped:
		return "skipped"
	case ui.StepPending:
		return "unknown"
	default:
		return "done"
	}
}

// setTimeoutNote sets rec.Notes to the standard timeout message when a per-step
// timeout was the cause of the step ending. Two signals satisfy this: the
// executor's WasTimedOut flag (the direct signal), or a lastState of
// StepTimedOutContinuing (the soft-fail policy consumed the flag via
// ClearTimeoutFlag before this function runs, but the state is preserved).
// Either signal is sufficient; both are equivalent.
// SUGG-001: extracted from three near-identical inline blocks in Run.
func setTimeoutNote(rec *IterationRecord, executor StepExecutor, s steps.Step, lastState ui.StepState) {
	if s.TimeoutSeconds <= 0 {
		return
	}
	if executor.WasTimedOut() || lastState == ui.StepTimedOutContinuing {
		rec.Notes = fmt.Sprintf("timed out after %ds", s.TimeoutSeconds)
	}
}

// resumeInputTokenLimit is the G4 gate ceiling. Steps whose prior input token
// count meets or exceeds this value start a fresh session to prevent unbounded
// context growth.
const resumeInputTokenLimit = 200_000

// evaluateResumeGates checks the five gates that must pass before a step's
// --resume flag is issued. Returns the session ID to resume (non-empty = all
// gates passed) and the gate label that blocked (empty string if all passed).
//
// G1: previous step produced a non-empty session ID.
// G2: previous step ended as StepDone (not StepFailed, not timed-out).
// G3: implicitly satisfied by G2 — is_error=true causes StepFailed, which blocks G2, so a separate Aggregator.Err() check is unnecessary.
// G4: previous step's input token count is below resumeInputTokenLimit to prevent unbounded session growth.
// G5: session ID is NOT in the timeout blacklist (a timed-out session's state is unknown and must not be resumed).
func evaluateResumeGates(
	prevStats claudestream.StepStats,
	prevState ui.StepState,
	blacklisted func(string) bool,
) (sessionID string, gate string) {
	if prevStats.SessionID == "" {
		return "", "G1: previous step has no session ID"
	}
	if prevState != ui.StepDone {
		return "", "G2: previous step did not complete successfully"
	}
	if prevStats.InputTokens >= resumeInputTokenLimit {
		return "", fmt.Sprintf("G4: previous step context too large (%d input tokens >= 200,000)", prevStats.InputTokens)
	}
	if blacklisted(prevStats.SessionID) {
		return "", "G5: previous step session is blacklisted (was timed out)"
	}
	return prevStats.SessionID, ""
}

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
	// buildState is phase-pure: it reads phase as a parameter without consulting
	// any internal phase field on vt, so this call is safe before vt.SetPhase.
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
		return filepath.Join(executor.ProjectDir(), ".pr9k", "logs", cfg.RunStamp, filename)
	}

	// 1. Initialize phase: run each step in order, binding captureAs results
	// into the persistent variable table so they are available in all phases.
	vt.SetPhase(vars.Initialize)
	push(vars.Initialize)
	if len(cfg.InitializeSteps) > 0 {
		writePhaseBanner("Initializing")
	}
	// prevInitStats / prevInitState track the immediately preceding step for
	// resumePrevious gate evaluation (G1–G5). Zero values cause G1 to fail on
	// the first step, which safely falls through to a fresh session.
	var prevInitStats claudestream.StepStats
	var prevInitState ui.StepState
	for j, s := range cfg.InitializeSteps {
		vt.SetStep(j+1, len(cfg.InitializeSteps), s.Name)
		push(vars.Initialize)
		var resumeSID string
		if s.ResumePrevious {
			var gate string
			resumeSID, gate = evaluateResumeGates(prevInitStats, prevInitState, executor.SessionBlacklisted)
			if gate != "" {
				executor.WriteToLog(fmt.Sprintf("resume gate blocked (%s) -- starting fresh session for step %q", gate, s.Name))
			}
		}
		resolved, err := buildStep(cfg.WorkflowDir, s, vt, vars.Initialize, cfg.Env, cfg.ContainerEnv, executor, resumeSID)
		if err != nil {
			executor.WriteToLog(fmt.Sprintf("Error preparing initialize step: %v", err))
			prepRec := newIterationRecord("", 0, s, "failed")
			prepRec.Notes = err.Error()
			if logErr := AppendIterationRecord(executor.ProjectDir(), prepRec); logErr != nil {
				executor.WriteToLog(fmt.Sprintf("warning: %v", logErr))
			}
			continue
		}
		if resolved.IsClaude {
			resolved.ArtifactPath = artifactPath(&resolved, "initialize-", j+1)
			resolved.CaptureMode = ui.CaptureResult
		}
		header.RenderInitializeLine(j+1, len(cfg.InitializeSteps), s.Name)
		emitBlank()
		st := &stateTracker{}
		// WARN-001: capture step/iteration context for potential timeout-before-retry record.
		capturedS := s
		disp := &stepDispatcher{
			exec:    executor,
			current: resolved,
			stats:   rs,
			onTimeoutRetry: func() {
				timeoutRec := newIterationRecord("", 0, capturedS, "failed")
				timeoutRec.Notes = fmt.Sprintf("timed out after %ds", capturedS.TimeoutSeconds)
				if logErr := AppendIterationRecord(executor.ProjectDir(), timeoutRec); logErr != nil {
					executor.WriteToLog(fmt.Sprintf("warning: %v", logErr))
				}
			},
		}
		stepStart := time.Now()
		action := ui.Orchestrate([]ui.ResolvedStep{resolved}, disp, st, keyHandler)
		rec := newIterationRecord("", 0, s, stepStatus(st.lastState))
		rec.DurationS = time.Since(stepStart).Seconds()
		rec.InputTokens = disp.capturedStats.InputTokens
		rec.OutputTokens = disp.capturedStats.OutputTokens
		rec.SessionID = disp.capturedStats.SessionID
		setTimeoutNote(&rec, executor, s, st.lastState)
		if logErr := AppendIterationRecord(executor.ProjectDir(), rec); logErr != nil {
			executor.WriteToLog(fmt.Sprintf("warning: %v", logErr))
		}
		// Update prev-step tracking for resumePrevious gate evaluation in the
		// next initialize step (if any).
		prevInitStats = disp.capturedStats
		prevInitState = st.lastState
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

		// captureStates maps each captureAs variable name to the final StepState
		// of the step that produced it. Used by skipIfCaptureEmpty to verify the
		// source step succeeded (StepDone) before applying the skip.
		captureStates := make(map[string]ui.StepState)
		// prevIterStats / prevIterState track the immediately preceding step in
		// this iteration for resumePrevious gate evaluation. Reset each iteration
		// so resume cannot bridge across iteration boundaries.
		var prevIterStats claudestream.StepStats
		var prevIterState ui.StepState
		breakOuter := false
		for j, s := range cfg.Steps {
			vt.SetStep(j+1, len(cfg.Steps), s.Name)
			push(vars.Iteration)

			// skipIfCaptureEmpty: skip this step when the named capture is empty
			// AND the step that produced it completed successfully (StepDone).
			// If the source step failed, we fall through and run this step normally
			// so the failure is not silently swallowed.
			if s.SkipIfCaptureEmpty != "" {
				val, ok := vt.GetInPhase(vars.Iteration, s.SkipIfCaptureEmpty)
				if !ok {
					executor.WriteToLog(fmt.Sprintf("warning: skipIfCaptureEmpty %q not found in iteration scope; step will run", s.SkipIfCaptureEmpty))
				}
				if val == "" && captureStates[s.SkipIfCaptureEmpty] == ui.StepDone {
					header.SetStepState(j, ui.StepSkipped)
					executor.WriteToLog(fmt.Sprintf("Step skipped (capture %q is empty)", s.SkipIfCaptureEmpty))
					issueID, _ := vt.GetInPhase(vars.Iteration, "ISSUE_ID")
					skipRec := newIterationRecord(issueID, i, s, "skipped")
					skipRec.Notes = fmt.Sprintf("capture %q empty", s.SkipIfCaptureEmpty)
					if logErr := AppendIterationRecord(executor.ProjectDir(), skipRec); logErr != nil {
						executor.WriteToLog(fmt.Sprintf("warning: %v", logErr))
					}
					continue
				}
			}

			var resumeSID string
			if s.ResumePrevious {
				var gate string
				resumeSID, gate = evaluateResumeGates(prevIterStats, prevIterState, executor.SessionBlacklisted)
				if gate != "" {
					executor.WriteToLog(fmt.Sprintf("resume gate blocked (%s) -- starting fresh session for step %q", gate, s.Name))
				}
			}
			resolved, err := buildStep(cfg.WorkflowDir, s, vt, vars.Iteration, cfg.Env, cfg.ContainerEnv, executor, resumeSID)
			if err != nil {
				executor.WriteToLog(fmt.Sprintf("Error preparing steps: %v", err))
				issueID, _ := vt.GetInPhase(vars.Iteration, "ISSUE_ID")
				prepRec := newIterationRecord(issueID, i, s, "failed")
				prepRec.Notes = err.Error()
				if logErr := AppendIterationRecord(executor.ProjectDir(), prepRec); logErr != nil {
					executor.WriteToLog(fmt.Sprintf("warning: %v", logErr))
				}
				breakOuter = true
				break
			}
			if resolved.IsClaude {
				resolved.ArtifactPath = artifactPath(&resolved, fmt.Sprintf("iter%02d-", i), j+1)
				resolved.CaptureMode = ui.CaptureResult
			}
			emitBlank()
			th := &trackingOffsetIterHeader{h: header, idx: j}
			// WARN-001: snapshot issueID and step at dispatcher construction time for
			// the timeout-before-retry record. issueID may be empty if not yet bound.
			issueIDAtDispatch, _ := vt.GetInPhase(vars.Iteration, "ISSUE_ID")
			iterNum := i
			capturedS := s
			disp := &stepDispatcher{
				exec:    executor,
				current: resolved,
				stats:   rs,
				onTimeoutRetry: func() {
					timeoutRec := newIterationRecord(issueIDAtDispatch, iterNum, capturedS, "failed")
					timeoutRec.Notes = fmt.Sprintf("timed out after %ds", capturedS.TimeoutSeconds)
					if logErr := AppendIterationRecord(executor.ProjectDir(), timeoutRec); logErr != nil {
						executor.WriteToLog(fmt.Sprintf("warning: %v", logErr))
					}
				},
			}
			stepStart := time.Now()
			action := ui.Orchestrate([]ui.ResolvedStep{resolved}, disp, th, keyHandler)
			issueID, _ := vt.GetInPhase(vars.Iteration, "ISSUE_ID")
			rec := newIterationRecord(issueID, i, s, stepStatus(th.lastState))
			rec.DurationS = time.Since(stepStart).Seconds()
			rec.InputTokens = disp.capturedStats.InputTokens
			rec.OutputTokens = disp.capturedStats.OutputTokens
			rec.SessionID = disp.capturedStats.SessionID
			setTimeoutNote(&rec, executor, s, th.lastState)
			if logErr := AppendIterationRecord(executor.ProjectDir(), rec); logErr != nil {
				executor.WriteToLog(fmt.Sprintf("warning: %v", logErr))
			}
			// Update prev-step tracking for resumePrevious gate evaluation in the
			// next iteration step (if any).
			prevIterStats = disp.capturedStats
			prevIterState = th.lastState
			if action == ui.ActionQuit {
				return RunResult{IterationsRun: iterationsRun}
			}
			captured := executor.LastCapture()
			if s.CaptureAs != "" {
				vt.Bind(vars.Iteration, s.CaptureAs, captured)
				push(vars.Iteration)
				// Re-read issueID after the bind so that if this step captured
				// ISSUE_ID, the header shows the freshly bound value.
				updatedIssueID, _ := vt.GetInPhase(vars.Iteration, "ISSUE_ID")
				header.RenderIterationLine(i, cfg.Iterations, updatedIssueID)
				writeCaptureLog(s.CaptureAs, captured)
				// Record the final state so skipIfCaptureEmpty checks can verify
				// the source step completed successfully (StepDone) before skipping.
				captureStates[s.CaptureAs] = th.lastState
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
	// prevFinalStats / prevFinalState track the immediately preceding step for
	// resumePrevious gate evaluation. Zero values cause G1 to fail on the first
	// finalize step, safely falling through to a fresh session.
	var prevFinalStats claudestream.StepStats
	var prevFinalState ui.StepState
	// finalizeCaptureStates mirrors the iteration captureStates map: it tracks
	// the final StepState of the step that produced each finalize-scoped capture,
	// so skipIfCaptureEmpty can verify the source step succeeded before skipping.
	finalizeCaptureStates := make(map[string]ui.StepState)
	for j, s := range cfg.FinalizeSteps {
		vt.SetStep(j+1, len(cfg.FinalizeSteps), s.Name)
		push(vars.Finalize)

		// skipIfCaptureEmpty in finalize phase: skip this step when the named
		// capture is empty AND the step that produced it completed successfully.
		if s.SkipIfCaptureEmpty != "" {
			val, ok := vt.GetInPhase(vars.Finalize, s.SkipIfCaptureEmpty)
			if !ok {
				executor.WriteToLog(fmt.Sprintf("warning: skipIfCaptureEmpty %q not found in finalize scope; step will run", s.SkipIfCaptureEmpty))
			}
			if val == "" && finalizeCaptureStates[s.SkipIfCaptureEmpty] == ui.StepDone {
				header.SetStepState(j, ui.StepSkipped)
				executor.WriteToLog(fmt.Sprintf("Step skipped (capture %q is empty)", s.SkipIfCaptureEmpty))
				skipRec := newIterationRecord("", 0, s, "skipped")
				skipRec.Notes = fmt.Sprintf("capture %q empty", s.SkipIfCaptureEmpty)
				if logErr := AppendIterationRecord(executor.ProjectDir(), skipRec); logErr != nil {
					executor.WriteToLog(fmt.Sprintf("warning: %v", logErr))
				}
				continue
			}
		}

		var resumeSID string
		if s.ResumePrevious {
			var gate string
			resumeSID, gate = evaluateResumeGates(prevFinalStats, prevFinalState, executor.SessionBlacklisted)
			if gate != "" {
				executor.WriteToLog(fmt.Sprintf("resume gate blocked (%s) -- starting fresh session for step %q", gate, s.Name))
			}
		}
		resolved, err := buildStep(cfg.WorkflowDir, s, vt, vars.Finalize, cfg.Env, cfg.ContainerEnv, executor, resumeSID)
		if err != nil {
			executor.WriteToLog(fmt.Sprintf("Error preparing finalize step: %v", err))
			issueID, _ := vt.GetInPhase(vars.Finalize, "ISSUE_ID")
			prepRec := newIterationRecord(issueID, 0, s, "failed")
			prepRec.Notes = err.Error()
			if logErr := AppendIterationRecord(executor.ProjectDir(), prepRec); logErr != nil {
				executor.WriteToLog(fmt.Sprintf("warning: %v", logErr))
			}
			continue
		}
		if resolved.IsClaude {
			resolved.ArtifactPath = artifactPath(&resolved, "finalize-", j+1)
			resolved.CaptureMode = ui.CaptureResult
		}
		header.RenderFinalizeLine(j+1, len(cfg.FinalizeSteps), s.Name)
		emitBlank()
		th := &trackingOffsetIterHeader{h: header, idx: j}
		// WARN-001: snapshot issueID and step at dispatcher construction time.
		issueIDAtDispatch, _ := vt.GetInPhase(vars.Finalize, "ISSUE_ID")
		capturedS := s
		disp := &stepDispatcher{
			exec:    executor,
			current: resolved,
			stats:   rs,
			onTimeoutRetry: func() {
				timeoutRec := newIterationRecord(issueIDAtDispatch, 0, capturedS, "failed")
				timeoutRec.Notes = fmt.Sprintf("timed out after %ds", capturedS.TimeoutSeconds)
				if logErr := AppendIterationRecord(executor.ProjectDir(), timeoutRec); logErr != nil {
					executor.WriteToLog(fmt.Sprintf("warning: %v", logErr))
				}
			},
		}
		stepStart := time.Now()
		action := ui.Orchestrate([]ui.ResolvedStep{resolved}, disp, th, keyHandler)
		issueID, _ := vt.GetInPhase(vars.Finalize, "ISSUE_ID")
		rec := newIterationRecord(issueID, 0, s, stepStatus(th.lastState))
		rec.DurationS = time.Since(stepStart).Seconds()
		rec.InputTokens = disp.capturedStats.InputTokens
		rec.OutputTokens = disp.capturedStats.OutputTokens
		rec.SessionID = disp.capturedStats.SessionID
		setTimeoutNote(&rec, executor, s, th.lastState)
		if logErr := AppendIterationRecord(executor.ProjectDir(), rec); logErr != nil {
			executor.WriteToLog(fmt.Sprintf("warning: %v", logErr))
		}
		// Update prev-step tracking for resumePrevious gate evaluation in the
		// next finalize step (if any).
		prevFinalStats = disp.capturedStats
		prevFinalState = th.lastState
		if action == ui.ActionQuit {
			return RunResult{IterationsRun: iterationsRun}
		}
		captured := executor.LastCapture()
		if s.CaptureAs != "" {
			vt.Bind(vars.Finalize, s.CaptureAs, captured)
			push(vars.Finalize)
			writeCaptureLog(s.CaptureAs, captured)
			// Record the final state so skipIfCaptureEmpty checks can verify
			// the source step completed successfully (StepDone) before skipping.
			finalizeCaptureStates[s.CaptureAs] = th.lastState
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
// steps. containerEnv is passed as literal key=value pairs (StepFile.ContainerEnv).
// executor provides ProjectDir for the docker bind-mount. resumeSessionID, when
// non-empty, embeds --resume <id> into the docker argv for claude steps so the
// step resumes the previous step's claude session; pass "" for a fresh session.
func buildStep(workflowDir string, s steps.Step, vt *vars.VarTable, phase vars.Phase, env []string, containerEnv map[string]string, executor StepExecutor, resumeSessionID string) (ui.ResolvedStep, error) {
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
		argv := sandbox.BuildRunArgs(projectDir, profileDir, uid, gid, cidfile, envAllowlist, containerEnv, resumeSessionID, s.Model, s.Effort, prompt)
		return ui.ResolvedStep{
			Name:           s.Name,
			Command:        argv,
			IsClaude:       true,
			CidfilePath:    cidfile,
			TimeoutSeconds: s.TimeoutSeconds,
			OnTimeout:      s.OnTimeout,
		}, nil
	}
	var capMode ui.CaptureMode
	switch s.CaptureMode {
	case "", "lastLine":
		capMode = ui.CaptureLastLine
	case "fullStdout":
		capMode = ui.CaptureFullStdout
	default:
		return ui.ResolvedStep{}, fmt.Errorf("workflow: step %q: invalid captureMode %q", s.Name, s.CaptureMode)
	}
	return ui.ResolvedStep{
		Name:           s.Name,
		Command:        ResolveCommand(workflowDir, s.Command, vt, phase),
		CaptureMode:    capMode,
		TimeoutSeconds: s.TimeoutSeconds,
		OnTimeout:      s.OnTimeout,
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
