package main

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// waitConditionFP polls cond up to timeout, sleeping interval between checks.
func waitConditionFP(timeout, interval time.Duration, cond func() bool) bool {
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if cond() {
			return true
		}
		time.Sleep(interval)
	}
	return cond()
}

// writeFooterPaneConfig creates a minimal config.json with a statusLine block
// pointing to scriptPath under workflowDir.
func writeFooterPaneConfig(t *testing.T, workflowDir, scriptPath string) {
	t.Helper()
	cfgJSON := fmt.Sprintf(`{"iteration":[],"finalize":[],"statusLine":{"command":%q,"refreshIntervalSeconds":0}}`, scriptPath)
	if err := os.WriteFile(filepath.Join(workflowDir, "config.json"), []byte(cfgJSON), 0o644); err != nil {
		t.Fatal(err)
	}
}

// TestRunCmuxFooterPane_NoStatusLine_ReturnsNil verifies that runCmuxFooterPane
// returns nil immediately when there is no statusLine config in the workflow.
func TestRunCmuxFooterPane_NoStatusLine_ReturnsNil(t *testing.T) {
	projectDir := t.TempDir()
	t.Setenv("PR9K_PROJECT_DIR", projectDir)

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	if err := runCmuxFooterPane(ctx, "/tmp/unused.sock"); err != nil {
		t.Fatalf("expected nil error, got: %v", err)
	}
}

// TestRunCmuxFooterPane_NoProjectDir_ReturnsNil verifies graceful degradation
// when PR9K_PROJECT_DIR is not set.
func TestRunCmuxFooterPane_NoProjectDir_ReturnsNil(t *testing.T) {
	t.Setenv("PR9K_PROJECT_DIR", "")

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	if err := runCmuxFooterPane(ctx, "/tmp/unused.sock"); err != nil {
		t.Fatalf("expected nil error, got: %v", err)
	}
}

// TestRunCmuxFooterPane_StatusLineScript_Runs verifies that runCmuxFooterPane
// constructs and starts a statusline.Runner from the config.json, and that the
// configured script is executed (acceptance criterion AC-3).
func TestRunCmuxFooterPane_StatusLineScript_Runs(t *testing.T) {
	projectDir := t.TempDir()
	workflowDir := filepath.Join(projectDir, ".pr9k", "workflow")
	if err := os.MkdirAll(workflowDir, 0o755); err != nil {
		t.Fatal(err)
	}

	markerFile := filepath.Join(t.TempDir(), "ran")
	scriptPath := filepath.Join(workflowDir, "status.sh")
	script := fmt.Sprintf("#!/bin/sh\ntouch %s\necho footer-status-ok\n", markerFile)
	if err := os.WriteFile(scriptPath, []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	writeFooterPaneConfig(t, workflowDir, scriptPath)

	t.Setenv("PR9K_PROJECT_DIR", projectDir)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	done := make(chan error, 1)
	go func() {
		done <- runCmuxFooterPane(ctx, "/tmp/unused.sock")
	}()

	// Wait for the script to run (marker file appears).
	if !waitConditionFP(3*time.Second, 20*time.Millisecond, func() bool {
		_, err := os.Stat(markerFile)
		return err == nil
	}) {
		t.Error("statusLine script was not executed by runCmuxFooterPane")
	}

	cancel()
	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("runCmuxFooterPane returned unexpected error: %v", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("runCmuxFooterPane did not return after context cancel")
	}
}

// TestRunCmuxFooterPane_StatusLineScript_OutputReachesSender verifies that the
// script's stdout reaches the footer pane's output writer (acceptance criterion AC-4).
func TestRunCmuxFooterPane_StatusLineScript_OutputReachesSender(t *testing.T) {
	projectDir := t.TempDir()
	workflowDir := filepath.Join(projectDir, ".pr9k", "workflow")
	if err := os.MkdirAll(workflowDir, 0o755); err != nil {
		t.Fatal(err)
	}

	scriptPath := filepath.Join(workflowDir, "status.sh")
	const expectedOutput = "footer-render-output-xyz"
	script := fmt.Sprintf("#!/bin/sh\necho %s\n", expectedOutput)
	if err := os.WriteFile(scriptPath, []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	writeFooterPaneConfig(t, workflowDir, scriptPath)

	t.Setenv("PR9K_PROJECT_DIR", projectDir)

	var out bytes.Buffer
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	done := make(chan error, 1)
	go func() {
		done <- runCmuxFooterPaneWith(ctx, "/tmp/unused.sock", &out)
	}()

	// Wait for the output to appear in the writer.
	if !waitConditionFP(3*time.Second, 20*time.Millisecond, func() bool {
		return strings.Contains(out.String(), expectedOutput)
	}) {
		t.Errorf("expected %q in footer output; got: %q", expectedOutput, out.String())
	}

	cancel()
	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("runCmuxFooterPaneWith returned unexpected error: %v", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("runCmuxFooterPaneWith did not return after context cancel")
	}
}

// TestRunCmuxFooterPane_BlocksUntilContextCancel verifies that runCmuxFooterPane
// blocks until the context is cancelled when a statusLine runner is active.
func TestRunCmuxFooterPane_BlocksUntilContextCancel(t *testing.T) {
	projectDir := t.TempDir()
	workflowDir := filepath.Join(projectDir, ".pr9k", "workflow")
	if err := os.MkdirAll(workflowDir, 0o755); err != nil {
		t.Fatal(err)
	}

	scriptPath := filepath.Join(workflowDir, "status.sh")
	if err := os.WriteFile(scriptPath, []byte("#!/bin/sh\necho ok\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	writeFooterPaneConfig(t, workflowDir, scriptPath)

	t.Setenv("PR9K_PROJECT_DIR", projectDir)

	ctx, cancel := context.WithCancel(context.Background())

	done := make(chan error, 1)
	go func() {
		done <- runCmuxFooterPane(ctx, "/tmp/unused.sock")
	}()

	// The function must NOT return before the context is cancelled.
	select {
	case <-done:
		t.Fatal("runCmuxFooterPane returned before context was cancelled")
	case <-time.After(200 * time.Millisecond):
		// Good — function is still blocking.
	}

	cancel()
	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("expected nil after cancel, got: %v", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("runCmuxFooterPane did not return within 2s after cancel")
	}
}
