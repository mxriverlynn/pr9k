package main

import (
	"bufio"
	"fmt"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/mxriverlynn/pr9k/ralph-tui/internal/cli"
	"github.com/mxriverlynn/pr9k/ralph-tui/internal/logger"
	"github.com/mxriverlynn/pr9k/ralph-tui/internal/steps"
	"github.com/mxriverlynn/pr9k/ralph-tui/internal/ui"
	"github.com/mxriverlynn/pr9k/ralph-tui/internal/workflow"
)

func main() {
	cfg, err := cli.Execute()
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
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

	// Drain the log pipe to stdout until EOF.
	go func() {
		scanner := bufio.NewScanner(runner.LogReader())
		buf := make([]byte, 256*1024)
		scanner.Buffer(buf, 256*1024)
		for scanner.Scan() {
			fmt.Println(scanner.Text())
		}
	}()

	runCfg := workflow.RunConfig{
		ProjectDir:    cfg.ProjectDir,
		Iterations:    cfg.Iterations,
		Steps:         stepFile.Iteration,
		FinalizeSteps: stepFile.Finalize,
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
		select {
		case <-done:
		case <-time.After(2 * time.Second):
		}
		os.Exit(1)
	}()

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
