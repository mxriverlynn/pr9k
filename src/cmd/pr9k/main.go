package main

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/signal"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/mxriverlynn/pr9k/src/internal/cli"
	"github.com/mxriverlynn/pr9k/src/internal/logger"
	"github.com/mxriverlynn/pr9k/src/internal/preflight"
	"github.com/mxriverlynn/pr9k/src/internal/statusline"
	"github.com/mxriverlynn/pr9k/src/internal/steps"
	"github.com/mxriverlynn/pr9k/src/internal/ui"
	"github.com/mxriverlynn/pr9k/src/internal/validator"
	"github.com/mxriverlynn/pr9k/src/internal/version"
	"github.com/mxriverlynn/pr9k/src/internal/workflow"
)

// stepNames extracts the Name field from each step in a slice.
func stepNames(ss []steps.Step) []string {
	names := make([]string, len(ss))
	for i, s := range ss {
		names[i] = s.Name
	}
	return names
}

// hasClaudeSteps reports whether any step across all phases is a claude step.
// Used by startup to decide whether to run the claude-only preflight checks
// (profile dir, docker).
func hasClaudeSteps(sf steps.StepFile) bool {
	for _, group := range [][]steps.Step{sf.Initialize, sf.Iteration, sf.Finalize} {
		for _, s := range group {
			if s.IsClaude {
				return true
			}
		}
	}
	return false
}

// services bundles the logger, runner, and step file returned by startup.
type services struct {
	log      *logger.Logger
	runner   *workflow.Runner
	stepFile steps.StepFile
}

// startup performs the full pre-run sequence: load steps, run D13 config
// validation, run preflight checks. Errors from both are collected before
// any output is written, so all problems appear together. On success the
// services are fully initialised and warnings (if any) have been printed.
// profileDir must be resolved by the caller (preflight.ResolveProfileDir).
func startup(cfg *cli.Config, projectDir, profileDir string, prober preflight.Prober, stderr io.Writer) (*services, bool) {
	stepFile, err := steps.LoadSteps(cfg.WorkflowDir)
	if err != nil {
		_, _ = fmt.Fprintf(stderr, "error: %v\n", err)
		return nil, false
	}

	validationErrs := validator.Validate(cfg.WorkflowDir)
	preflightResult := preflight.Run(projectDir, profileDir, hasClaudeSteps(stepFile), prober)

	fatalCount := validator.FatalErrorCount(validationErrs)
	// fatal path: print all validation findings (fatal + non-fatal) so no finding is swallowed.
	if fatalCount > 0 || len(preflightResult.Errors) > 0 {
		for _, ve := range validationErrs {
			_, _ = fmt.Fprintln(stderr, ve.Error())
		}
		if fatalCount > 0 {
			_, _ = fmt.Fprintf(stderr, "%d validation error(s)\n", fatalCount)
		}
		for _, e := range preflightResult.Errors {
			_, _ = fmt.Fprintln(stderr, e.Error())
		}
		return nil, false
	}
	// Print non-fatal validation findings (warnings, info notices) after passing
	// the fatal-error gate so they appear alongside preflight warnings.
	for _, ve := range validationErrs {
		_, _ = fmt.Fprintln(stderr, ve.Error())
	}

	for _, w := range preflightResult.Warnings {
		_, _ = fmt.Fprintln(stderr, w)
	}

	log, err := logger.NewLogger(projectDir)
	if err != nil {
		_, _ = fmt.Fprintf(stderr, "error: %v\n", err)
		return nil, false
	}

	// Create the per-run artifact directory eagerly so per-step file opens
	// cannot race on directory creation (D14, Step 6).
	artifactDir := filepath.Join(projectDir, ".pr9k", "logs", log.RunStamp())
	if err := os.MkdirAll(artifactDir, 0o700); err != nil {
		_, _ = fmt.Fprintf(stderr, "error: %v\n", err)
		return nil, false
	}

	return &services{
		log:      log,
		runner:   workflow.NewRunner(log, projectDir),
		stepFile: stepFile,
	}, true
}

// formatUsageError formats a CLI parse error into the standard user-facing
// message that includes a help pointer.
func formatUsageError(err error) string {
	return fmt.Sprintf("error: %v\nRun 'pr9k --help' for usage.\n", err)
}

// buildVersionLabel composes the TUI footer version string.
func buildVersionLabel() string {
	return "pr9k v" + version.Version
}

func main() {
	// Step 1: parse CLI flags.
	cfg, err := cli.Execute(newSandboxCmd(), newWorkflowCmd(), newWorktreeCmd())
	if err != nil {
		if !errors.Is(err, errSilentExit) {
			fmt.Fprint(os.Stderr, formatUsageError(err))
		}
		os.Exit(1)
	}
	if cfg == nil {
		return
	}

	// Early step-file load to read the worktrees block before startup().
	// startup() loads the same file again for validation and step execution;
	// the duplicate read is intentional and acceptable (file is small).
	worktreesCfg, err := loadStepsForWorktrees(cfg.WorkflowDir)
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}
	worktreesEnabled := worktreesCfg != nil && worktreesCfg.Enabled

	// Step 2: read active-run.json from the primary project dir, regardless
	// of worktrees.enabled. This must happen before validateActiveRun so we
	// have the prior state available for the resume decision.
	primaryPath := cfg.ProjectDir
	activeRunStatePath := filepath.Join(primaryPath, ".pr9k", "active-run.json")
	priorState, _ := readActiveRun(activeRunStatePath)

	// Handle --fresh: discard stale state and force a clean start.
	var freshErr error
	priorState, freshErr = applyFreshFlag(activeRunStatePath, primaryPath, cfg.Fresh, priorState, os.Stderr)
	if freshErr != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", freshErr)
		os.Exit(1)
	}

	// Step 3: determine the action using the resume-validation function.
	decision := validateActiveRun(activeRunStatePath, primaryPath)
	action := decideWorktreeAction(priorState, decision, worktreesEnabled, cfg.Fresh)

	if action == worktreeActionConcurrent {
		fmt.Fprintf(os.Stderr, "error: another pr9k process is already running for this project\n")
		os.Exit(1)
	}

	// Warn when a state file exists but worktrees feature is disabled.
	if !worktreesEnabled && priorState != nil {
		fmt.Fprintf(os.Stderr, "warning: active-run.json found but worktrees.enabled is false; proceeding in-place\n")
	}

	// Step 4: resolve worktree path and stamp.
	var worktreePath, worktreeStamp, branch string
	isResume := action == worktreeActionResume

	if worktreesEnabled {
		if isResume {
			// Re-use the existing worktree from the prior run.
			worktreePath = priorState.WorktreePath
			worktreeStamp = priorState.WorktreeStamp
			branch = priorState.Branch
		} else {
			// Fresh start: mint a new stamp and create a linked worktree.
			worktreeStamp = logger.FormatStamp(time.Now())
			branch = worktreeStamp
			worktreePath = filepath.Join(filepath.Dir(primaryPath), worktreeStamp)
			if err := gitWorktreeAdd(primaryPath, worktreePath, branch); err != nil {
				fmt.Fprintf(os.Stderr, "error: %v\n", err)
				os.Exit(1)
			}
		}
	} else {
		worktreePath = primaryPath
	}

	// claimDone is set to true once writeActiveRun+verifyActiveRunClaim both succeed.
	// The defer and the explicit cleanup before startup failure both use removeUnclaimed
	// to remove the just-created worktree if we exit before the claim is recorded.
	var claimDone bool
	removeUnclaimed := func() {}
	if worktreesEnabled && !isResume {
		removeUnclaimed = func() {
			if err := gitWorktreeRemove(primaryPath, worktreePath); err != nil {
				fmt.Fprintf(os.Stderr, "warning: cleanup unclaimed worktree: %v\n", err)
			}
			if err := gitBranchDelete(primaryPath, branch); err != nil {
				fmt.Fprintf(os.Stderr, "warning: cleanup unclaimed branch: %v\n", err)
			}
		}
		defer func() {
			if !claimDone {
				removeUnclaimed()
			}
		}()
	}

	// Step 5: startup with worktree path as projectDir so all downstream
	// subsystems (sandbox bind-mount, logger, validator) operate against
	// the worktree transparently (T2 from spec).
	profileDir := preflight.ResolveProfileDir()
	svc, ok := startup(cfg, worktreePath, profileDir, preflight.RealProber{}, os.Stderr)
	if !ok {
		removeUnclaimed()
		os.Exit(1)
	}
	log := svc.log
	stepFile := svc.stepFile
	runner := svc.runner

	// Step 6: write the state file after preflight so a preflight failure
	// does not leave a state file pointing at an unusable worktree (D-1).
	var activeState *ActiveRunState
	if worktreesEnabled {
		binary, _ := os.Executable()
		activeState = &ActiveRunState{
			SchemaVersion: activeRunSchemaVersion,
			WorktreeStamp: worktreeStamp,
			WorktreePath:  worktreePath,
			PrimaryPath:   primaryPath,
			Branch:        branch,
			PID:           os.Getpid(),
			Binary:        binary,
		}
		if err := writeActiveRun(activeRunStatePath, *activeState); err != nil {
			fmt.Fprintf(os.Stderr, "error: %v\n", err)
			os.Exit(1)
		}
		if err := verifyActiveRunClaim(activeRunStatePath, *activeState); err != nil {
			fmt.Fprintf(os.Stderr, "error: %v\n", err)
			os.Exit(1)
		}
		claimDone = true
	}

	// Step 7: log header lines recording the worktree context.
	if worktreesEnabled {
		if isResume {
			_ = log.Log("worktrees", "RESUMED FROM "+priorState.WorktreePath)
		}
		_ = log.Log("worktrees", "worktree: "+worktreePath)
		_ = log.Log("worktrees", "branch: "+branch)
	}

	actions := make(chan ui.StepAction, 10)
	keyHandler := ui.NewKeyHandler(runner.Terminate, actions)

	// Build the statusline runner from config. New() returns a no-op runner
	// when StatusLine is nil or its command cannot be resolved, so all method
	// calls below are safe regardless of configuration.
	statusRunner := statusline.New(buildStatusLineConfig(stepFile.StatusLine), cfg.WorkflowDir, cfg.ProjectDir, log)
	keyHandler.SetStatusLineActive(statusRunner.Enabled())

	maxSteps := max(len(stepFile.Initialize), len(stepFile.Iteration), len(stepFile.Finalize))
	header := ui.NewStatusHeader(maxSteps)

	// Pre-populate the first visible phase state so the first rendered frame
	// shows real content, not empty slots.
	if len(stepFile.Initialize) > 0 {
		header.SetPhaseSteps(stepNames(stepFile.Initialize))
		header.SetStepState(0, ui.StepActive)
		header.IterationLine = "Initializing 1/" + strconv.Itoa(len(stepFile.Initialize)) + ": " + stepFile.Initialize[0].Name
	} else {
		header.SetPhaseSteps(stepNames(stepFile.Iteration))
		if len(stepFile.Iteration) > 0 {
			header.SetStepState(0, ui.StepActive)
		}
		if cfg.Iterations > 0 {
			header.IterationLine = "Iteration 1/" + strconv.Itoa(cfg.Iterations)
		} else {
			header.IterationLine = "Iteration 1"
		}
	}

	// Wire the D23 heartbeat reader into the StatusHeader so HeartbeatTickMsg
	// can query silence duration from the active claude pipeline.
	header.SetHeartbeatReader(runner)

	versionLabel := buildVersionLabel()
	model := ui.NewModel(header, keyHandler, versionLabel).
		WithStatusRunner(statusRunner).
		WithModeTrigger(statusRunner.Trigger)

	program := tea.NewProgram(model,
		tea.WithMouseCellMotion(),
		tea.WithAltScreen(),
		tea.WithoutSignalHandler(),
	)

	// Inject the Bubble Tea sender so the worker goroutine can notify the TUI
	// when the status-line cache is updated. The sender wraps the ui-package
	// message type so statusline does not import bubbletea.
	statusRunner.SetSender(newStatusLineSender(program.Send))

	// Inject the mode reader so the stdin payload reflects the current UI mode
	// at the moment the script is invoked.
	statusRunner.SetModeGetter(newModeGetter(keyHandler))

	// Start the status-line worker goroutine. The initial state push happens
	// inside workflow.Run (issue #116); the runner fires on its timer and on
	// mode-change triggers from the Model.
	statusCtx, statusCancel := context.WithCancel(context.Background())
	// Belt-and-suspenders: runWithShutdown calls runner.Shutdown() first,
	// which cancels the runner's internal context. This defer is a safety net
	// for early-return paths that bypass runWithShutdown.
	defer statusCancel()
	statusRunner.Start(statusCtx)

	proxy := ui.NewHeaderProxy(program.Send)

	// D23 heartbeat ticker: sends HeartbeatTickMsg once per second so the TUI
	// can render the "  ⋯ thinking (Ns)" suffix during silent claude turns.
	// The goroutine terminates with the process — no explicit shutdown needed.
	go func() {
		t := time.NewTicker(time.Second)
		defer t.Stop()
		for range t.C {
			program.Send(ui.HeartbeatTickMsg{})
		}
	}()

	// logWidth sizes the full-width phase banner underline to fill the log
	// panel. The panel sits inside a rounded border, so we subtract 2
	// columns for the left and right border glyphs.
	logWidth := ui.TerminalWidth() - 2
	if logWidth < 1 {
		logWidth = ui.DefaultTerminalWidth
	}

	var worktreeCountFn func() int
	if worktreesEnabled {
		capturedPrimaryPath := primaryPath
		capturedBranchRe := pr9kBranchRe
		worktreeCountFn = func() int {
			entries, err := gitWorktreeList(capturedPrimaryPath)
			if err != nil {
				return 0
			}
			count := 0
			for _, e := range entries {
				branchName := strings.TrimPrefix(e.Branch, "refs/heads/")
				if capturedBranchRe.MatchString(branchName) {
					count++
				}
			}
			return count
		}
	}

	var worktreeBasename, runWorktreePath, runBranch string
	if worktreesEnabled {
		worktreeBasename = filepath.Base(worktreePath)
		runWorktreePath = worktreePath
		runBranch = branch
	}

	runCfg := buildRunConfig(cfg, stepFile, statusRunner, logWidth, log.RunStamp(), worktreeBasename, isResume, runWorktreePath, runBranch, worktreeCountFn)

	// Buffered channel between forwardPipe and the drain goroutine. Lines are
	// written non-blockingly; drops are acceptable since the file logger still
	// captures every line.
	const senderBuffer = 4096
	lineCh := make(chan string, senderBuffer)

	// Drain goroutine: coalesces consecutive lines within a single scheduler
	// wakeup into one LogLinesMsg, reducing SetContent calls by ~100x under
	// burst load. Blocks on the first line of each batch, then non-blockingly
	// drains any lines already queued before forwarding.
	go func() {
		for {
			first, ok := <-lineCh
			if !ok {
				return
			}
			batch := []string{first}
		drain:
			for {
				select {
				case l, ok := <-lineCh:
					if !ok {
						program.Send(ui.LogLinesMsg{Lines: batch})
						return
					}
					batch = append(batch, l)
				default:
					break drain
				}
			}
			program.Send(ui.LogLinesMsg{Lines: batch})
		}
	}()

	runner.SetSender(func(line string) {
		select {
		case lineCh <- line:
		default:
			// buffer full — drop; file logger still has the line
		}
	})

	workflowDone := make(chan struct{})

	// Signal handling: wait for SIGINT/SIGTERM or for the workflow to finish.
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)
	signaled := make(chan struct{})
	go func() {
		<-sigChan
		close(signaled)
		keyHandler.ForceQuit()
		select {
		case <-workflowDone:
		case <-time.After(2 * time.Second):
		}
		program.Kill()
	}()

	// Workflow goroutine: run the full workflow, then tear down cleanly.
	go func() {
		defer close(workflowDone)
		// Step 9: capture exit reason for post-run cleanup.
		result := workflow.Run(runner, proxy, keyHandler, runCfg)
		_ = log.Close()
		close(lineCh)
		keyHandler.SetMode(ui.ModeDone)

		// Post-run: remove state file and optionally clean up worktree/branch.
		// Only relevant when worktrees are enabled.
		if worktreesEnabled && activeState != nil {
			autoCleanup := worktreesCfg != nil && worktreesCfg.AutoCleanup
			postRunCleanup(primaryPath, activeRunStatePath, activeState, result.ExitReason, autoCleanup, os.Stderr)
		}
	}()

	// Shut down the status-line runner before waiting for the workflow goroutine.
	// runWithShutdown: blocks until the worker drains (bounded 2 s deadline in
	// Shutdown, plus 4 s for workflowDone), ensuring no program.Send calls
	// happen after program.Run() has returned.
	// Note: the SIGINT path (program.Kill() → os.Exit(1)) does not reach here;
	// any in-flight script on that path is orphaned until the OS reaps the
	// process tree (matches claude-step behavior on forced exit).
	runErr := runWithShutdown(program, statusRunner, workflowDone)
	signal.Stop(sigChan)

	// program.Kill() (signal-path forced shutdown after 2s grace) causes Run
	// to return tea.ErrProgramKilled. That is a normal forced-exit path, not
	// a crash — don't print a scary error for it.
	if runErr != nil && !errors.Is(runErr, tea.ErrProgramKilled) {
		fmt.Fprintln(os.Stderr, "bubbletea:", runErr)
		os.Exit(1)
	}

	select {
	case <-signaled:
		os.Exit(1)
	default:
		os.Exit(0)
	}
}
