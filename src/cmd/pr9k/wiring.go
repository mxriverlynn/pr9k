package main

import (
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/mxriverlynn/pr9k/src/internal/cli"
	"github.com/mxriverlynn/pr9k/src/internal/statusline"
	"github.com/mxriverlynn/pr9k/src/internal/steps"
	"github.com/mxriverlynn/pr9k/src/internal/ui"
	"github.com/mxriverlynn/pr9k/src/internal/workflow"
)

// modeString converts a ui.Mode to the lowercase string used in the
// status-line stdin JSON payload's "mode" field. Each value matches the
// documented contract that user-supplied status-line scripts depend on.
func modeString(m ui.Mode) string {
	switch m {
	case ui.ModeNormal:
		return "normal"
	case ui.ModeError:
		return "error"
	case ui.ModeQuitConfirm:
		return "quitconfirm"
	case ui.ModeNextConfirm:
		return "nextconfirm"
	case ui.ModeDone:
		return "done"
	case ui.ModeSelect:
		return "select"
	case ui.ModeQuitting:
		return "quitting"
	case ui.ModeHelp:
		return "help"
	default:
		return "unknown"
	}
}

// newModeGetter returns a closure that reads the current mode from kh at
// call time. Injected into the status-line runner so every stdin payload
// reflects the live UI mode rather than a startup snapshot.
func newModeGetter(kh *ui.KeyHandler) func() string {
	return func() string {
		return modeString(kh.Mode())
	}
}

// newStatusLineSender returns a func(interface{}) that discards its argument
// and forwards a ui.StatusLineUpdatedMsg to the Bubble Tea program. This
// keeps the statusline package free of any bubbletea dependency.
func newStatusLineSender(send func(tea.Msg)) func(interface{}) {
	return func(_ interface{}) {
		send(ui.StatusLineUpdatedMsg{})
	}
}

// buildStatusLineConfig constructs a statusline.Config from the parsed
// steps.StatusLineConfig. Returns nil when slc is nil (no statusLine block
// in config.json), which causes statusline.New to return a no-op runner.
func buildStatusLineConfig(slc *steps.StatusLineConfig) *statusline.Config {
	if slc == nil {
		return nil
	}
	return &statusline.Config{
		Command:                slc.Command,
		RefreshIntervalSeconds: slc.RefreshIntervalSeconds,
	}
}

// buildRunConfig constructs the workflow.RunConfig from the resolved inputs.
// statusRunner must be the same runner passed to Start so that push/trigger
// calls reach the active worker goroutine.
func buildRunConfig(cfg *cli.Config, stepFile steps.StepFile, statusRunner workflow.StatusRunner, logWidth int, runStamp string) workflow.RunConfig {
	return workflow.RunConfig{
		WorkflowDir:     cfg.WorkflowDir,
		Iterations:      cfg.Iterations,
		Env:             stepFile.Env,
		ContainerEnv:    stepFile.ContainerEnv,
		InitializeSteps: stepFile.Initialize,
		Steps:           stepFile.Iteration,
		FinalizeSteps:   stepFile.Finalize,
		LogWidth:        logWidth,
		RunStamp:        runStamp,
		Runner:          statusRunner,
	}
}

// teaProgram is the subset of *tea.Program used by runWithShutdown. Defined
// as an interface so the ordering can be tested without a real Bubble Tea
// program.
type teaProgram interface {
	Run() (tea.Model, error)
}

// shutdownable is the subset of *statusline.Runner used by runWithShutdown.
type shutdownable interface {
	Shutdown()
}

// workflowDoneTimeout is the maximum time runWithShutdown waits for the
// workflow goroutine to finish after Shutdown returns.
const workflowDoneTimeout = 4 * time.Second

// runWithShutdown runs prog, then shuts down the status-line runner before
// waiting for the workflow goroutine. This ordering ensures no program.Send
// calls happen after the Bubble Tea program has stopped.
func runWithShutdown(prog teaProgram, runner shutdownable, workflowDone <-chan struct{}) error {
	_, err := prog.Run()
	runner.Shutdown()
	select {
	case <-workflowDone:
	case <-time.After(workflowDoneTimeout):
	}
	return err
}
