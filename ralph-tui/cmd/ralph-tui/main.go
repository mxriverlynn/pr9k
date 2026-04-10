package main

import (
	"fmt"
	"os"
	"os/signal"
	"strconv"
	"syscall"
	"time"

	"github.com/kungfusheep/glyph"
	"github.com/mxriverlynn/pr9k/ralph-tui/internal/cli"
	"github.com/mxriverlynn/pr9k/ralph-tui/internal/logger"
	"github.com/mxriverlynn/pr9k/ralph-tui/internal/steps"
	"github.com/mxriverlynn/pr9k/ralph-tui/internal/ui"
	"github.com/mxriverlynn/pr9k/ralph-tui/internal/validator"
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

func main() {
	cfg, err := cli.Execute()
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\nRun 'ralph-tui --help' for usage.\n", err)
		os.Exit(1)
	}
	if cfg == nil {
		os.Exit(0)
	}

	log, err := logger.NewLogger(cfg.ProjectDir)
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}

	stepFile, err := steps.LoadSteps(cfg.ProjectDir)
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		_ = log.Close()
		os.Exit(1)
	}

	if validationErrs := validator.Validate(cfg.ProjectDir); len(validationErrs) > 0 {
		for _, ve := range validationErrs {
			fmt.Fprintln(os.Stderr, ve.Error())
		}
		fmt.Fprintf(os.Stderr, "%d validation error(s)\n", len(validationErrs))
		_ = log.Close()
		os.Exit(1)
	}

	runner := workflow.NewRunner(log, cfg.ProjectDir)

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

	app := glyph.NewApp()

	// Wire keyboard dispatch: Glyph owns the tty and forwards each keypress to keyHandler.
	for _, key := range []string{"n", "q", "y", "c", "r"} {
		k := key
		app.Handle(k, func() { keyHandler.Handle(k) })
	}

	// Build checkpoint row widgets — one HBox per header row, HeaderCols Text widgets each.
	rowWidgets := make([]any, len(header.Rows))
	for r := range header.Rows {
		cols := make([]any, ui.HeaderCols)
		for c := range cols {
			cols[c] = glyph.Text(&header.Rows[r][c])
		}
		rowWidgets[r] = glyph.HBox(cols...)
	}

	// Assemble the full VBox layout tree.
	children := make([]any, 0, 2+len(rowWidgets)+2)
	children = append(children, glyph.Text(&header.IterationLine))
	children = append(children, rowWidgets...)
	children = append(children, glyph.Log(runner.LogReader()).Grow(1).MaxLines(500).BindVimNav())
	children = append(children, glyph.Text(keyHandler.ShortcutLinePtr()))

	app.SetView(glyph.VBox.Border(glyph.BorderRounded).Title("Ralph")(children...))

	runCfg := workflow.RunConfig{
		ProjectDir:      cfg.ProjectDir,
		Iterations:      cfg.Iterations,
		InitializeSteps: stepFile.Initialize,
		Steps:           stepFile.Iteration,
		FinalizeSteps:   stepFile.Finalize,
	}

	done := make(chan struct{})

	// Set up OS signal handling for clean shutdown.
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)
	signaled := make(chan struct{})
	go func() {
		<-sigChan
		close(signaled)
		keyHandler.ForceQuit()
		app.Stop()
		select {
		case <-done:
		case <-time.After(2 * time.Second):
		}
		os.Exit(1)
	}()

	// Run the workflow in the background; stop the TUI when it completes.
	go func() {
		defer close(done)
		_ = workflow.Run(runner, header, keyHandler, runCfg)
		signal.Stop(sigChan)
		_ = log.Close()
		app.Stop()
	}()

	if err := app.Run(); err != nil {
		fmt.Fprintln(os.Stderr, "glyph:", err)
		os.Exit(1)
	}

	// Wait for the workflow goroutine if app.Run returned before it finished.
	select {
	case <-done:
	case <-time.After(2 * time.Second):
	}

	select {
	case <-signaled:
		os.Exit(1)
	default:
		os.Exit(0)
	}
}
