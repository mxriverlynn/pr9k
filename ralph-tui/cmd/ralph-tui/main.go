package main

import (
	"fmt"
	"os"
	"os/signal"
	"syscall"

	"github.com/mxriverlynn/pr9k/ralph-tui/internal/cli"
	"github.com/mxriverlynn/pr9k/ralph-tui/internal/logger"
	"github.com/mxriverlynn/pr9k/ralph-tui/internal/steps"
	"github.com/mxriverlynn/pr9k/ralph-tui/internal/ui"
	"github.com/mxriverlynn/pr9k/ralph-tui/internal/workflow"
)

func main() {
	cfg, err := cli.ParseArgs(os.Args[1:])
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}

	log, err := logger.NewLogger(cfg.ProjectDir)
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}

	workflowCfg, err := steps.LoadWorkflowConfig(cfg.ProjectDir, cfg.StepsFile)
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		_ = log.Close()
		os.Exit(1)
	}

	runner := workflow.NewRunner(log, cfg.ProjectDir)

	actions := make(chan ui.StepAction, 10)
	keyHandler := ui.NewKeyHandler(runner.Terminate, actions)

	// Initialize the header with loop step names for the initial display.
	loopNames := make([]string, len(workflowCfg.Loop))
	for i, s := range workflowCfg.Loop {
		loopNames[i] = s.Name
	}
	header := ui.NewStatusHeader(loopNames)

	// Set up OS signal handling for clean shutdown.
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)
	signaled := make(chan struct{})
	go func() {
		<-sigChan
		close(signaled)
		keyHandler.ForceQuit()
	}()

	runCfg := workflow.RunConfig{
		ProjectDir: cfg.ProjectDir,
		Iterations: cfg.Iterations,
		Config:     workflowCfg,
	}

	done := make(chan struct{})
	go func() {
		defer close(done)
		workflow.Run(runner, header, keyHandler, runCfg)
	}()

	<-done
	signal.Stop(sigChan)
	_ = log.Close()

	select {
	case <-signaled:
		os.Exit(1)
	default:
		os.Exit(0)
	}
}
