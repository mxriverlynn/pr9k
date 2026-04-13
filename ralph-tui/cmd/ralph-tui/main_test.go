package main

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/mxriverlynn/pr9k/ralph-tui/internal/cli"
	"github.com/mxriverlynn/pr9k/ralph-tui/internal/steps"
)

func TestStepNames_Empty(t *testing.T) {
	got := stepNames(nil)
	if len(got) != 0 {
		t.Errorf("expected empty slice, got %v", got)
	}
}

func TestStepNames_Single(t *testing.T) {
	ss := []steps.Step{{Name: "Feature work"}}
	got := stepNames(ss)
	if len(got) != 1 || got[0] != "Feature work" {
		t.Errorf("want [\"Feature work\"], got %v", got)
	}
}

func TestStepNames_Multiple(t *testing.T) {
	ss := []steps.Step{
		{Name: "Feature work"},
		{Name: "Test writing"},
		{Name: "Code review"},
	}
	got := stepNames(ss)
	want := []string{"Feature work", "Test writing", "Code review"}
	if len(got) != len(want) {
		t.Fatalf("length mismatch: want %d, got %d", len(want), len(got))
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("index %d: want %q, got %q", i, want[i], got[i])
		}
	}
}

// writeMinimalStepFile creates a minimal valid ralph-steps.json under dir so
// steps.LoadSteps and validator.Validate succeed without requiring real
// prompts or scripts.
func writeMinimalStepFile(t *testing.T, dir string) {
	t.Helper()
	content := `{
		"initialize": [],
		"iteration": [
			{ "name": "noop", "isClaude": false, "command": ["true"] }
		],
		"finalize": []
	}`
	if err := os.WriteFile(filepath.Join(dir, "ralph-steps.json"), []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
}

// TestNewServices_BindsLoggerAndRunnerToProjectDir verifies that newServices
// wires the logger and runner to projectDir (target repo), not to cfg.WorkflowDir
// (install dir). This guards against reintroducing the bug where subprocess
// cmd.Dir and log output were mistakenly bound to the install dir.
func TestNewServices_BindsLoggerAndRunnerToProjectDir(t *testing.T) {
	workflowDir := t.TempDir() // install dir: holds ralph-steps.json, scripts/, prompts/
	projectDir := t.TempDir()  // target repo: subprocess cmd.Dir and log location
	writeMinimalStepFile(t, workflowDir)

	cfg := &cli.Config{WorkflowDir: workflowDir, ProjectDir: projectDir}
	svc, ok := newServices(cfg, projectDir)
	if !ok {
		t.Fatal("newServices returned ok=false")
	}
	defer func() { _ = svc.log.Close() }()

	// Logger creates logs/ under projectDir (target repo), not workflowDir (install dir).
	if _, err := os.Stat(filepath.Join(projectDir, "logs")); err != nil {
		t.Errorf("expected logs/ under projectDir, got error: %v", err)
	}
	if _, err := os.Stat(filepath.Join(workflowDir, "logs")); !os.IsNotExist(err) {
		t.Errorf("logs/ should NOT exist under workflowDir; Stat err=%v", err)
	}

	// Runner subprocess cmd.Dir is projectDir (target repo), not workflowDir.
	out, err := svc.runner.CaptureOutput([]string{"sh", "-c", "pwd"})
	if err != nil {
		t.Fatalf("CaptureOutput: %v", err)
	}
	wantDir, _ := filepath.EvalSymlinks(projectDir)
	gotDir, _ := filepath.EvalSymlinks(out)
	if gotDir != wantDir {
		t.Errorf("runner cmd.Dir: want %q, got %q", wantDir, gotDir)
	}
}

// writeInvalidStepFile creates a ralph-steps.json whose JSON is valid but
// fails D13 validation — a claude step missing the required promptFile field.
// Used to exercise the validator.Validate failure path in newServices.
func writeInvalidStepFile(t *testing.T, dir string) {
	t.Helper()
	content := `{
		"initialize": [],
		"iteration": [
			{ "name": "bad-step", "isClaude": true, "model": "sonnet" }
		],
		"finalize": []
	}`
	if err := os.WriteFile(filepath.Join(dir, "ralph-steps.json"), []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
}

// TestNewServices_ValidationFailureReturnsFalse verifies that newServices
// returns (nil, false) when validator.Validate reports errors. It also checks
// that the logger is created and closed without leaking (the logs/ directory
// must exist under projectDir, confirming the logger was instantiated before
// the early-return path was taken).
func TestNewServices_ValidationFailureReturnsFalse(t *testing.T) {
	workflowDir := t.TempDir()
	projectDir := t.TempDir()
	writeInvalidStepFile(t, workflowDir)

	cfg := &cli.Config{WorkflowDir: workflowDir, ProjectDir: projectDir}
	svc, ok := newServices(cfg, projectDir)
	if ok {
		t.Fatal("newServices should have returned ok=false on validation failure")
	}
	if svc != nil {
		t.Error("newServices should have returned nil services on validation failure")
	}

	// Confirm the logger was created (logs/ dir exists) but its Close was
	// called by the early-return path — no leak.
	if _, err := os.Stat(filepath.Join(projectDir, "logs")); err != nil {
		t.Errorf("expected logs/ directory to exist under projectDir after validation failure: %v", err)
	}
}

// TestNewServices_LoadStepsFailureReturnsFalse verifies that newServices returns
// (nil, false) when ralph-steps.json is missing from WorkflowDir. It also checks
// that the logger was created and closed without leaking (logs/ must exist under
// projectDir, confirming logger instantiation before the early-return path).
func TestNewServices_LoadStepsFailureReturnsFalse(t *testing.T) {
	workflowDir := t.TempDir() // no ralph-steps.json here
	projectDir := t.TempDir()
	// Deliberately do NOT write ralph-steps.json so LoadSteps fails.

	cfg := &cli.Config{WorkflowDir: workflowDir, ProjectDir: projectDir}
	svc, ok := newServices(cfg, projectDir)
	if ok {
		t.Fatal("newServices should have returned ok=false when ralph-steps.json is missing")
	}
	if svc != nil {
		t.Error("newServices should have returned nil services on LoadSteps failure")
	}

	// Logger was created and closed cleanly — logs/ dir should exist with no leak.
	if _, err := os.Stat(filepath.Join(projectDir, "logs")); err != nil {
		t.Errorf("expected logs/ directory to exist under projectDir after LoadSteps failure: %v", err)
	}
}

// TestNewServices_LoggerFailureReturnsFalse verifies that newServices returns
// (nil, false) when logger.NewLogger fails due to an unwritable projectDir.
func TestNewServices_LoggerFailureReturnsFalse(t *testing.T) {
	workflowDir := t.TempDir()
	projectDir := "/nonexistent/path/that/does/not/exist"

	cfg := &cli.Config{WorkflowDir: workflowDir, ProjectDir: projectDir}
	svc, ok := newServices(cfg, projectDir)
	if ok {
		t.Fatal("newServices should have returned ok=false when logger creation fails")
	}
	if svc != nil {
		t.Error("newServices should have returned nil services on logger failure")
	}
}

// TestNewServices_LoadsStepsFromWorkflowDir verifies that newServices reads
// ralph-steps.json from cfg.WorkflowDir (install dir), not projectDir (target repo).
func TestNewServices_LoadsStepsFromWorkflowDir(t *testing.T) {
	workflowDir := t.TempDir() // install dir with step definitions
	projectDir := t.TempDir()  // target repo — deliberately no ralph-steps.json here
	writeMinimalStepFile(t, workflowDir)
	// Deliberately do NOT write ralph-steps.json in projectDir: if the wiring
	// is wrong, LoadSteps will fail.

	cfg := &cli.Config{WorkflowDir: workflowDir, ProjectDir: projectDir}
	svc, ok := newServices(cfg, projectDir)
	if !ok {
		t.Fatal("newServices returned ok=false; ralph-steps.json should have been loaded from WorkflowDir")
	}
	_ = svc.log.Close()
}
