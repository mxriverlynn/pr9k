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

// TestNewServices_BindsLoggerAndRunnerToWorkingDir verifies that newServices
// wires the logger and runner to workingDir, not to cfg.ProjectDir. This
// guards against reintroducing the bug where subprocess cmd.Dir and log
// output were mistakenly bound to the install dir.
func TestNewServices_BindsLoggerAndRunnerToWorkingDir(t *testing.T) {
	installDir := t.TempDir()
	workingDir := t.TempDir()
	writeMinimalStepFile(t, installDir)

	cfg := &cli.Config{ProjectDir: installDir}
	svc, ok := newServices(cfg, workingDir)
	if !ok {
		t.Fatal("newServices returned ok=false")
	}
	defer func() { _ = svc.log.Close() }()

	// Logger creates logs/ under workingDir, not installDir.
	if _, err := os.Stat(filepath.Join(workingDir, "logs")); err != nil {
		t.Errorf("expected logs/ under workingDir, got error: %v", err)
	}
	if _, err := os.Stat(filepath.Join(installDir, "logs")); !os.IsNotExist(err) {
		t.Errorf("logs/ should NOT exist under installDir; Stat err=%v", err)
	}

	// Runner subprocess cmd.Dir is workingDir, not installDir.
	out, err := svc.runner.CaptureOutput([]string{"sh", "-c", "pwd"})
	if err != nil {
		t.Fatalf("CaptureOutput: %v", err)
	}
	wantDir, _ := filepath.EvalSymlinks(workingDir)
	gotDir, _ := filepath.EvalSymlinks(out)
	if gotDir != wantDir {
		t.Errorf("runner cmd.Dir: want %q, got %q", wantDir, gotDir)
	}
}

// TestNewServices_LoadsStepsFromProjectDir verifies that newServices reads
// ralph-steps.json from cfg.ProjectDir (install dir), not workingDir.
func TestNewServices_LoadsStepsFromProjectDir(t *testing.T) {
	installDir := t.TempDir()
	workingDir := t.TempDir()
	writeMinimalStepFile(t, installDir)
	// Deliberately do NOT write ralph-steps.json in workingDir: if the wiring
	// is wrong, LoadSteps will fail.

	cfg := &cli.Config{ProjectDir: installDir}
	svc, ok := newServices(cfg, workingDir)
	if !ok {
		t.Fatal("newServices returned ok=false; ralph-steps.json should have been loaded from ProjectDir")
	}
	_ = svc.log.Close()
}
