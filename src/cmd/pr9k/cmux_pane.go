package main

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"github.com/mxriverlynn/pr9k/src/internal/logger"
	"github.com/spf13/cobra"
)

// newCmuxPaneCmd returns the cobra sub-command for internal per-pane processes
// launched by cmux mode. Operators must not run this directly; it is invoked
// via SurfaceSpawn by RunPhase1.
func newCmuxPaneCmd() *cobra.Command {
	var role string
	cmd := &cobra.Command{
		Use:   "cmux-pane",
		Short: "Internal — launched by cmux mode, not for direct operator use",
		RunE: func(cmd *cobra.Command, args []string) error {
			switch role {
			case "orchestrator", "header", "log", "footer":
			default:
				return fmt.Errorf("cmux-pane: invalid --role %q; must be one of orchestrator, header, log, footer", role)
			}

			socketPath := os.Getenv("PR9K_CMUX_SOCKET")
			if socketPath == "" {
				return errors.New("cmux-pane: PR9K_CMUX_SOCKET is not set; this command is launched internally by pr9k cmux mode")
			}

			ctx := cmd.Context()
			switch role {
			case "orchestrator":
				projectDir := os.Getenv("PR9K_PROJECT_DIR")
				return runCmuxOrchestrator(ctx, socketPath, projectDir)
			case "header":
				return runCmuxHeaderPane(ctx, socketPath)
			case "log":
				return runCmuxLogPane(ctx, socketPath)
			case "footer":
				return runCmuxFooterPane(ctx, socketPath)
			}
			return nil
		},
	}
	cmd.Flags().StringVar(&role, "role", "", "pane role: orchestrator, header, log, or footer (required)")
	_ = cmd.MarkFlagRequired("role")
	return cmd
}

// runCmuxOrchestrator is the entry point for the orchestrator pane process.
// It creates the per-run logger and artifact directory mirroring the standard
// mode's startup() shape (D-7). projectDir is the target repository directory
// passed via PR9K_PROJECT_DIR by RunPhase1.
func runCmuxOrchestrator(_ context.Context, _ string, projectDir string) error {
	if projectDir == "" {
		return errors.New("cmux-pane: PR9K_PROJECT_DIR is not set; this command is launched internally by pr9k cmux mode")
	}

	log, err := logger.NewLogger(projectDir)
	if err != nil {
		return fmt.Errorf("cmux-pane: orchestrator: create logger: %w", err)
	}
	defer func() { _ = log.Close() }()

	// Create the per-run artifact directory eagerly, mirroring startup() in main.go.
	artifactDir := filepath.Join(projectDir, ".pr9k", "logs", log.RunStamp())
	if err := os.MkdirAll(artifactDir, 0o700); err != nil {
		return fmt.Errorf("cmux-pane: orchestrator: create artifact dir: %w", err)
	}

	// Real orchestration (readiness handshake, workflow.Run) ships in later work units.
	return nil
}

// runCmuxHeaderPane is the entry point for the header pane process.
// Real implementation ships in a later work unit.
func runCmuxHeaderPane(_ context.Context, _ string) error {
	return nil
}

// runCmuxLogPane is the entry point for the log pane process.
// Real implementation ships in a later work unit.
func runCmuxLogPane(_ context.Context, _ string) error {
	return nil
}

// runCmuxFooterPane is the entry point for the footer pane process.
// Real implementation ships in a later work unit.
func runCmuxFooterPane(_ context.Context, _ string) error {
	return nil
}
