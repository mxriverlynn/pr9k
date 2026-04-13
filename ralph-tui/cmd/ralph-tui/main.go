package main

import (
	"errors"
	"fmt"
	"os"
	"os/signal"
	"strconv"
	"syscall"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/mxriverlynn/pr9k/ralph-tui/internal/cli"
	"github.com/mxriverlynn/pr9k/ralph-tui/internal/logger"
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

// services bundles the dependencies wired from cfg and the captured working
// directory. Split out so tests can verify each constructor receives the
// correct dir (logger/runner bound to workingDir; steps/validator bound to
// cfg.ProjectDir).
type services struct {
	log      *logger.Logger
	runner   *workflow.Runner
	stepFile steps.StepFile
}

// newServices wires the logger, runner, and step file. workingDir is the
// shell CWD captured at startup and governs subprocess cmd.Dir and log file
// location. cfg.ProjectDir is the install dir (where ralph-steps.json,
// scripts/, prompts/ live). On validation failure, errors are written to
// stderr and ok=false is returned.
func newServices(cfg *cli.Config, workingDir string) (s *services, ok bool) {
	log, err := logger.NewLogger(workingDir)
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		return nil, false
	}

	stepFile, err := steps.LoadSteps(cfg.ProjectDir)
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		_ = log.Close()
		return nil, false
	}

	if validationErrs := validator.Validate(cfg.ProjectDir); len(validationErrs) > 0 {
		for _, ve := range validationErrs {
			fmt.Fprintln(os.Stderr, ve.Error())
		}
		fmt.Fprintf(os.Stderr, "%d validation error(s)\n", len(validationErrs))
		_ = log.Close()
		return nil, false
	}

	return &services{
		log:      log,
		runner:   workflow.NewRunner(log, workingDir),
		stepFile: stepFile,
	}, true
}

func main() {
	workingDir, err := os.Getwd()
	if err != nil {
		fmt.Fprintf(os.Stderr, "main: could not resolve working dir: %v\n", err)
		os.Exit(1)
	}

	cfg, err := cli.Execute()
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\nRun 'ralph-tui --help' for usage.\n", err)
		os.Exit(1)
	}
	if cfg == nil {
		os.Exit(0)
	}

	svc, ok := newServices(cfg, workingDir)
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
		header.SetStepState(0, ui.StepActive)
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
		ProjectDir:      cfg.ProjectDir,
		Iterations:      cfg.Iterations,
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
