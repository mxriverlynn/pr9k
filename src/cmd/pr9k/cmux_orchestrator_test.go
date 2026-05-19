package main

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestRunCmuxOrchestrator_CreatesLogDirectory verifies that runCmuxOrchestrator
// creates the per-run log directory under <projectDir>/.pr9k/logs/<run-stamp>/
// exactly as standard mode's startup() does (D-7).
func TestRunCmuxOrchestrator_CreatesLogDirectory(t *testing.T) {
	projectDir := t.TempDir()

	ctx, cancel := context.WithCancel(context.Background())
	cancel() // cancel immediately so the orchestrator returns quickly

	err := runCmuxOrchestrator(ctx, "/tmp/unused.sock", projectDir)
	// A cancelled context is acceptable; the logger must still have been created.
	if err != nil && !strings.Contains(err.Error(), "context") {
		t.Fatalf("runCmuxOrchestrator returned unexpected error: %v", err)
	}

	logsDir := filepath.Join(projectDir, ".pr9k", "logs")
	entries, statErr := os.ReadDir(logsDir)
	if statErr != nil {
		t.Fatalf(".pr9k/logs/ not created: %v", statErr)
	}
	if len(entries) == 0 {
		t.Fatal(".pr9k/logs/ is empty; expected at least one run-stamp directory")
	}

	// The run-stamp directory (e.g. "ralph-2026-05-18-123456.789") must exist
	// inside logs/.
	runStampDir := filepath.Join(logsDir, entries[0].Name())
	if fi, err := os.Stat(runStampDir); err != nil || !fi.IsDir() {
		t.Errorf("run-stamp subdirectory %q not created or not a directory: %v", runStampDir, err)
	}
}

// TestRunCmuxOrchestrator_LogFileExists verifies that the per-run .log file is
// created under <projectDir>/.pr9k/logs/ (the file that NewLogger creates).
func TestRunCmuxOrchestrator_LogFileExists(t *testing.T) {
	projectDir := t.TempDir()

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	_ = runCmuxOrchestrator(ctx, "/tmp/unused.sock", projectDir)

	logsDir := filepath.Join(projectDir, ".pr9k", "logs")
	entries, err := os.ReadDir(logsDir)
	if err != nil {
		t.Fatalf(".pr9k/logs/ not created: %v", err)
	}

	var logFile string
	for _, e := range entries {
		if strings.HasSuffix(e.Name(), ".log") {
			logFile = e.Name()
			break
		}
	}
	if logFile == "" {
		t.Errorf(".pr9k/logs/ contains no .log file; entries: %v", entries)
	}
}

// TestRunCmuxOrchestrator_EmptyProjectDir_ReturnsError verifies that
// runCmuxOrchestrator returns a non-nil error mentioning PR9K_PROJECT_DIR when
// projectDir is empty (cmux_pane.go:59-61).
func TestRunCmuxOrchestrator_EmptyProjectDir_ReturnsError(t *testing.T) {
	ctx := context.Background()
	err := runCmuxOrchestrator(ctx, "/tmp/unused.sock", "")
	if err == nil {
		t.Fatal("expected error for empty projectDir, got nil")
	}
	if !strings.Contains(err.Error(), "PR9K_PROJECT_DIR") {
		t.Errorf("error %q should mention PR9K_PROJECT_DIR", err.Error())
	}
}

// TestCmuxPaneCmd_OrchestratorRole_RequiresProjectDir verifies that the
// cmux-pane cobra command dispatches the orchestrator role correctly when
// PR9K_PROJECT_DIR is set to a valid temp directory.
func TestCmuxPaneCmd_OrchestratorRole_RequiresProjectDir(t *testing.T) {
	projectDir := t.TempDir()
	socketPath := shortSockPath(t)
	t.Setenv("PR9K_CMUX_SOCKET", socketPath)
	t.Setenv("PR9K_PROJECT_DIR", projectDir)

	cmd := newCmuxPaneCmd()
	suppressCobra(cmd)
	cmd.SetArgs([]string{"--role=orchestrator"})
	// Use a pre-cancelled context so the orchestrator returns immediately without
	// waiting for the full 10-second AwaitReady handshake timeout.
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	err := cmd.ExecuteContext(ctx)
	// We expect it to return nil or a context/handshake error — not a "project dir not set" error.
	if err != nil && strings.Contains(err.Error(), "PR9K_PROJECT_DIR") {
		t.Errorf("cmux-pane --role=orchestrator should not complain about PR9K_PROJECT_DIR when it is set; got: %v", err)
	}
}
