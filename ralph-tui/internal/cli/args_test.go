package cli

import (
	"strings"
	"testing"
)

func TestParseArgs_ValidIterations(t *testing.T) {
	cfg, err := ParseArgs([]string{"3", "-project-dir", "/tmp/foo"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.Iterations != 3 {
		t.Errorf("expected iterations=3, got %d", cfg.Iterations)
	}
}

func TestParseArgs_MissingIterations(t *testing.T) {
	_, err := ParseArgs([]string{})
	if err == nil {
		t.Fatal("expected error for missing iterations, got nil")
	}
}

func TestParseArgs_ZeroIterations(t *testing.T) {
	_, err := ParseArgs([]string{"0"})
	if err == nil {
		t.Fatal("expected error for iterations=0, got nil")
	}
}

func TestParseArgs_ProjectDirOverride(t *testing.T) {
	cfg, err := ParseArgs([]string{"3", "-project-dir", "/tmp/foo"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.ProjectDir != "/tmp/foo" {
		t.Errorf("expected project-dir=/tmp/foo, got %q", cfg.ProjectDir)
	}
}

func TestParseArgs_DefaultProjectDir(t *testing.T) {
	cfg, err := ParseArgs([]string{"1"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// The default is resolved from os.Executable(); just verify it's non-empty.
	if cfg.ProjectDir == "" {
		t.Error("expected non-empty default project dir")
	}
}

// T1: non-integer iterations string
func TestParseArgs_NonIntegerIterations(t *testing.T) {
	_, err := ParseArgs([]string{"abc"})
	if err == nil {
		t.Fatal("expected error for non-integer iterations, got nil")
	}
	if !strings.Contains(err.Error(), "iterations must be an integer") {
		t.Errorf("expected error to contain %q, got %q", "iterations must be an integer", err.Error())
	}
}

// T2: negative iterations (via "--" separator to bypass flag parsing of "-1")
func TestParseArgs_NegativeIterationsViaFlag(t *testing.T) {
	_, err := ParseArgs([]string{"-1"})
	if err == nil {
		t.Fatal("expected error for -1 (interpreted as unknown flag), got nil")
	}
}

func TestParseArgs_NegativeIterationsViaSeparator(t *testing.T) {
	_, err := ParseArgs([]string{"--", "-1"})
	if err == nil {
		t.Fatal("expected error for iterations=-1, got nil")
	}
}

// T4: flag before positional argument
func TestParseArgs_FlagBeforePositional(t *testing.T) {
	cfg, err := ParseArgs([]string{"-project-dir", "/tmp/foo", "5"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.Iterations != 5 {
		t.Errorf("expected iterations=5, got %d", cfg.Iterations)
	}
	if cfg.ProjectDir != "/tmp/foo" {
		t.Errorf("expected project-dir=/tmp/foo, got %q", cfg.ProjectDir)
	}
}

// T5: unknown flags return an error
func TestParseArgs_UnknownFlag(t *testing.T) {
	_, err := ParseArgs([]string{"-unknown-flag", "3"})
	if err == nil {
		t.Fatal("expected error for unknown flag, got nil")
	}
}

// T6: large iteration count is accepted
func TestParseArgs_LargeIterations(t *testing.T) {
	cfg, err := ParseArgs([]string{"1000", "-project-dir", "/tmp"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.Iterations != 1000 {
		t.Errorf("expected iterations=1000, got %d", cfg.Iterations)
	}
}

func TestParseArgs_DefaultStepsFile(t *testing.T) {
	cfg, err := ParseArgs([]string{"5"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.StepsFile != "ralph-steps.json" {
		t.Errorf("expected StepsFile=%q, got %q", "ralph-steps.json", cfg.StepsFile)
	}
}

func TestParseArgs_ExplicitStepsFlag(t *testing.T) {
	cfg, err := ParseArgs([]string{"--steps", "custom.json", "5"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.StepsFile != "custom.json" {
		t.Errorf("expected StepsFile=%q, got %q", "custom.json", cfg.StepsFile)
	}
}

func TestParseArgs_StepsFlagAfterPositional(t *testing.T) {
	cfg, err := ParseArgs([]string{"5", "-steps", "custom.json"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.StepsFile != "custom.json" {
		t.Errorf("expected StepsFile=%q, got %q", "custom.json", cfg.StepsFile)
	}
	if cfg.Iterations != 5 {
		t.Errorf("expected iterations=5, got %d", cfg.Iterations)
	}
}

func TestParseArgs_StepsFlagWithProjectDir(t *testing.T) {
	cfg, err := ParseArgs([]string{"-project-dir", "/tmp", "-steps", "my-steps.json", "3"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.ProjectDir != "/tmp" {
		t.Errorf("expected ProjectDir=%q, got %q", "/tmp", cfg.ProjectDir)
	}
	if cfg.StepsFile != "my-steps.json" {
		t.Errorf("expected StepsFile=%q, got %q", "my-steps.json", cfg.StepsFile)
	}
}
