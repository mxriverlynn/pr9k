package main

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/spf13/cobra"

	"github.com/mxriverlynn/pr9k/src/internal/logger"
	"github.com/mxriverlynn/pr9k/src/internal/workflowedit"
	"github.com/mxriverlynn/pr9k/src/internal/workflowio"
)

// osGetwd is a var so tests can inject a failing implementation (D-44).
var osGetwd = os.Getwd

// newBuilderTeaProgram is a var so tests can inject a no-op program that exits
// immediately without requiring a real terminal (D-44 pattern).
var newBuilderTeaProgram func(tea.Model) teaProgram = func(m tea.Model) teaProgram {
	return tea.NewProgram(m,
		tea.WithAltScreen(),
		tea.WithoutSignalHandler(),
	)
}

// realEditorRunner resolves $VISUAL or $EDITOR and launches the editor via
// tea.ExecProcess. The editor value may contain flags (e.g. "code --wait");
// they are split on whitespace before exec (D-6).
type realEditorRunner struct{}

func (realEditorRunner) Run(filePath string, cb workflowedit.ExecCallback) tea.Cmd {
	editor := os.Getenv("VISUAL")
	if editor == "" {
		editor = os.Getenv("EDITOR")
	}
	if editor == "" {
		return func() tea.Msg {
			return cb(fmt.Errorf("workflow: no editor configured: set $VISUAL or $EDITOR"))
		}
	}
	parts := strings.Fields(editor)
	if len(parts) == 0 {
		return func() tea.Msg {
			return cb(fmt.Errorf("workflow: no editor configured: set $VISUAL or $EDITOR"))
		}
	}
	args := append(parts[1:], filePath)
	return tea.ExecProcess(exec.Command(parts[0], args...), func(err error) tea.Msg {
		return cb(err)
	})
}

// newWorkflowCmd returns the `workflow` cobra command with --workflow-dir and
// --project-dir flags. It does NOT expose --iterations or any other run-only flag.
func newWorkflowCmd() *cobra.Command {
	var workflowDir, projectDir string

	cmd := &cobra.Command{
		Use:           "workflow",
		Short:         "Open the interactive workflow builder",
		SilenceErrors: true,
		SilenceUsage:  true,
		RunE: func(cmd *cobra.Command, args []string) error {
			return runWorkflowBuilder(cmd, projectDir, workflowDir)
		},
	}
	cmd.Flags().StringVar(&workflowDir, "workflow-dir", "", "path to the workflow bundle directory (default: <projectDir>/.pr9k/workflow/, then <executableDir>/.pr9k/workflow/)")
	cmd.Flags().StringVar(&projectDir, "project-dir", "", "path to the target repository (default: current working directory)")
	return cmd
}

// runWorkflowBuilder is the RunE implementation for the workflow subcommand.
// It owns its own logger creation, directory resolution, and goroutine lifecycle.
// It does NOT call startup().
func runWorkflowBuilder(cmd *cobra.Command, projectDirFlag, workflowDirFlag string) error {
	ctx := cmd.Context()
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()

	// Resolve the log base directory with fallback (D-44).
	logBaseDir := resolveBuilderLogBaseDir(projectDirFlag)

	log, err := logger.NewLoggerWithPrefix(logBaseDir, "workflow")
	if err != nil {
		return fmt.Errorf("workflow: %w", err)
	}
	defer func() { _ = log.Close() }()

	// Signal handling: SIGINT/SIGTERM cancels the context, then waits up to
	// 10 s for the TUI to quit cleanly before falling back to os.Exit(130) (D-7).
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)
	go func() {
		defer signal.Stop(sigChan)
		select {
		case <-sigChan:
			cancel()
			time.AfterFunc(10*time.Second, func() {
				os.Exit(130)
			})
		case <-ctx.Done():
		}
	}()

	model := workflowedit.New(workflowio.RealSaveFS(), realEditorRunner{}, logBaseDir, workflowDirFlag).
		WithLog(log.Writer())
	prog := newBuilderTeaProgram(model)
	_, runErr := prog.Run()
	return runErr
}

// resolveBuilderLogBaseDir returns the base directory under which the builder
// writes its log. If a --project-dir flag was given, that is used. Otherwise
// the current working directory is tried, with a fallback to os.UserConfigDir()
// (D-44) if neither is available.
func resolveBuilderLogBaseDir(projectDirFlag string) string {
	if projectDirFlag != "" {
		return projectDirFlag
	}
	if cwd, err := osGetwd(); err == nil {
		return cwd
	}
	if configDir, err := os.UserConfigDir(); err == nil {
		return filepath.Join(configDir, ".pr9k")
	}
	return os.TempDir()
}
