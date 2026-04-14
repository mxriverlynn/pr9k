package main

import (
	"errors"
	"fmt"
	"io"
	"os"
	"os/signal"
	"strconv"
	"syscall"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/mxriverlynn/pr9k/ralph-tui/internal/cli"
	"github.com/mxriverlynn/pr9k/ralph-tui/internal/logger"
	"github.com/mxriverlynn/pr9k/ralph-tui/internal/preflight"
	"github.com/mxriverlynn/pr9k/ralph-tui/internal/steps"
	"github.com/mxriverlynn/pr9k/ralph-tui/internal/ui"
	"github.com/mxriverlynn/pr9k/ralph-tui/internal/validator"
	"github.com/mxriverlynn/pr9k/ralph-tui/internal/version"
	"github.com/mxriverlynn/pr9k/ralph-tui/internal/workflow"
)

// stepNames extracts the Name field from each step in a slice.
func stepNames(ss []steps.Step) []string {
	names := make([]string, len(ss))
	for i, s := range ss {
		names[i] = s.Name
	}
	return names
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
	preflightResult := preflight.Run(profileDir, prober)

	if len(validationErrs) > 0 || len(preflightResult.Errors) > 0 {
		for _, ve := range validationErrs {
			_, _ = fmt.Fprintln(stderr, ve.Error())
		}
		if len(validationErrs) > 0 {
			_, _ = fmt.Fprintf(stderr, "%d validation error(s)\n", len(validationErrs))
		}
		for _, e := range preflightResult.Errors {
			_, _ = fmt.Fprintln(stderr, e.Error())
		}
		return nil, false
	}

	for _, w := range preflightResult.Warnings {
		_, _ = fmt.Fprintln(stderr, w)
	}

	log, err := logger.NewLogger(projectDir)
	if err != nil {
		_, _ = fmt.Fprintf(stderr, "error: %v\n", err)
		return nil, false
	}

	return &services{
		log:      log,
		runner:   workflow.NewRunner(log, projectDir),
		stepFile: stepFile,
	}, true
}

func main() {
	cfg, err := cli.Execute(newCreateSandboxCmd())
	if err != nil {
		if !errors.Is(err, errSilentExit) {
			fmt.Fprintf(os.Stderr, "error: %v\nRun 'ralph-tui --help' for usage.\n", err)
		}
		os.Exit(1)
	}
	if cfg == nil {
		os.Exit(0)
	}

	profileDir := preflight.ResolveProfileDir()
	svc, ok := startup(cfg, cfg.ProjectDir, profileDir, preflight.RealProber{}, os.Stderr)
	if !ok {
		os.Exit(1)
	}
	log := svc.log
	stepFile := svc.stepFile
	runner := svc.runner

	actions := make(chan ui.StepAction, 10)
	keyHandler := ui.NewKeyHandler(runner.Terminate, actions)

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

	versionLabel := "ralph-tui v" + version.Version
	model := ui.NewModel(header, keyHandler, versionLabel)

	program := tea.NewProgram(model,
		tea.WithMouseCellMotion(),
		tea.WithAltScreen(),
		tea.WithoutSignalHandler(),
	)

	proxy := ui.NewHeaderProxy(program.Send)

	// logWidth sizes the full-width phase banner underline to fill the log
	// panel. The panel sits inside a rounded border, so we subtract 2
	// columns for the left and right border glyphs.
	logWidth := ui.TerminalWidth() - 2
	if logWidth < 1 {
		logWidth = ui.DefaultTerminalWidth
	}

	runCfg := workflow.RunConfig{
		WorkflowDir:     cfg.WorkflowDir,
		Iterations:      cfg.Iterations,
		Env:             stepFile.Env,
		InitializeSteps: stepFile.Initialize,
		Steps:           stepFile.Iteration,
		FinalizeSteps:   stepFile.Finalize,
		LogWidth:        logWidth,
	}

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
		select {
		case <-sigChan:
			close(signaled)
			keyHandler.ForceQuit()
			// Give the workflow goroutine up to 2 seconds to exit cleanly.
			select {
			case <-workflowDone:
			case <-time.After(2 * time.Second):
				program.Kill()
			}
		case <-workflowDone:
		}
	}()

	// Workflow goroutine: run the full workflow, then tear down cleanly.
	go func() {
		defer close(workflowDone)
		_ = workflow.Run(runner, proxy, keyHandler, runCfg)
		signal.Stop(sigChan)
		_ = log.Close()
		close(lineCh)
		program.Quit()
	}()

	_, runErr := program.Run()
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
