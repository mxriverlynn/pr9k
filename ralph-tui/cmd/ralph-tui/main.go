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
	for _, key := range []string{"n", "q", "y", "c", "r", "<Escape>"} {
		k := key
		app.Handle(k, func() { keyHandler.Handle(k) })
	}

	// Build checkpoint row widgets — one HBox per header row, HeaderCols
	// cells each, with each cell composed of three adjacent Text widgets
	// so the marker glyph (▸/✓/✗/-) can be colored independently of the
	// brackets and step name. The per-cell color fields are bound by
	// pointer so state transitions (e.g. pending → active) repaint the
	// cell on the next render cycle without rebuilding the widget tree.
	rowWidgets := make([]any, len(header.Rows))
	for r := range header.Rows {
		cols := make([]any, ui.HeaderCols)
		for c := range cols {
			cols[c] = glyph.HBox(
				glyph.Text(&header.Prefixes[r][c]).FG(&header.NameColors[r][c]),
				glyph.Text(&header.Markers[r][c]).FG(&header.MarkerColors[r][c]),
				glyph.Text(&header.Suffixes[r][c]).FG(&header.NameColors[r][c]),
			)
		}
		rowWidgets[r] = glyph.HBox(cols...)
	}

	// Assemble the full VBox layout tree. The iteration status line sits
	// at the top of the header with an HRule under it; the checkbox grid
	// follows, then another HRule, the log panel, a final HRule, and the
	// shortcut footer. The chrome (iteration line, HRules, footer text,
	// and the outer rounded border) renders in LightGray so the active
	// step's green marker and white brackets/name pop against it.
	children := make([]any, 0, 6+len(rowWidgets))
	children = append(children, glyph.Text(&header.IterationLine).FG(ui.LightGray))
	children = append(children, glyph.HRule().FG(ui.LightGray))
	children = append(children, rowWidgets...)
	children = append(children, glyph.HRule().FG(ui.LightGray))
	children = append(children, glyph.Log(runner.LogReader()).Grow(1).MaxLines(500).BindVimNav())
	children = append(children, glyph.HRule().FG(ui.LightGray))
	// Footer: shortcut bar on the left, app version pinned to the bottom-right.
	// glyph.Space() is a flex spacer inside an HBox, pushing the version text
	// against the right border of the VBox.
	versionLabel := "ralph-tui v" + version.Version
	children = append(children, glyph.HBox(
		glyph.Text(keyHandler.ShortcutLinePtr()).FG(ui.LightGray),
		glyph.Space(),
		glyph.Text(&versionLabel).FG(ui.LightGray),
	))

	app.SetView(glyph.VBox.Border(glyph.BorderRounded).BorderFG(ui.LightGray).Title("Ralph")(children...))

	// logWidth sizes the full-width phase banner underline to fill the log
	// panel. The panel sits inside a rounded VBox border, so we subtract 2
	// columns for the left and right border glyphs. A non-TTY stdout falls
	// back to ui.DefaultTerminalWidth.
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

	// Set up OS signal handling for clean shutdown.
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)
	signaled := make(chan struct{})
	go func() {
		<-sigChan
		close(signaled)
		keyHandler.ForceQuit()
		// Give the workflow goroutine a moment to unwind and exit cleanly,
		// then force-exit if it's still stuck.
		time.Sleep(2 * time.Second)
		_ = app.Screen().ExitRawMode()
		os.Exit(1)
	}()

	// Run the workflow in the background; tear down the TUI and exit when
	// it completes. We exit directly from this goroutine instead of going
	// through app.Stop() because app.Stop() closes stdin from another
	// goroutine, which does not reliably interrupt glyph's in-progress
	// blocking read on a macOS raw-mode tty — the process would hang until
	// the user pressed another key.
	go func() {
		_ = workflow.Run(runner, header, keyHandler, runCfg)
		signal.Stop(sigChan)
		_ = log.Close()
		_ = app.Screen().ExitRawMode()
		select {
		case <-signaled:
			os.Exit(1)
		default:
			os.Exit(0)
		}
	}()

	if err := app.Run(); err != nil {
		fmt.Fprintln(os.Stderr, "glyph:", err)
		_ = app.Screen().ExitRawMode()
		os.Exit(1)
	}
}
