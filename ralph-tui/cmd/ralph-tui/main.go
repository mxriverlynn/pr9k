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

	stepFile, err := steps.LoadSteps(cfg.ProjectDir)
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		_ = log.Close()
		os.Exit(1)
	}

	runner := workflow.NewRunner(log, cfg.ProjectDir)

	actions := make(chan ui.StepAction, 10)
	keyHandler := ui.NewKeyHandler(runner.Terminate, actions)

	var stepNames [8]string
	for i, s := range stepFile.Iteration {
		if i >= 8 {
			break
		}
		stepNames[i] = s.Name
	}
	header := ui.NewStatusHeader(stepNames)

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
		ProjectDir:    cfg.ProjectDir,
		Iterations:    cfg.Iterations,
		Steps:         stepFile.Iteration,
		FinalizeSteps: stepFile.Finalize,
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
