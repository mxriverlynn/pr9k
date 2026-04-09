package cli

import (
	"strings"
	"testing"
)

// helper: build a NewCommand with a fresh Config and set args on it.
func runNewCommand(t *testing.T, args []string) (*Config, error) {
	t.Helper()
	cfg := &Config{}
	cmd := NewCommand(cfg)
	cmd.SetArgs(args)
	err := cmd.Execute()
	if err != nil {
		return nil, err
	}
	return cfg, nil
}

// 1. No flags → iterations=0 (until-done), project-dir resolved from executable (non-empty).
func TestNewCommand_NoFlags(t *testing.T) {
	cfg, err := runNewCommand(t, []string{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.Iterations != 0 {
		t.Errorf("expected iterations=0, got %d", cfg.Iterations)
	}
	if cfg.ProjectDir == "" {
		t.Error("expected non-empty project-dir resolved from executable")
	}
}

// 2. --iterations 3 → iterations=3.
func TestNewCommand_LongIterationsFlag(t *testing.T) {
	cfg, err := runNewCommand(t, []string{"--iterations", "3"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.Iterations != 3 {
		t.Errorf("expected iterations=3, got %d", cfg.Iterations)
	}
}

// 3. -n 3 → iterations=3 (short flag).
func TestNewCommand_ShortIterationsFlag(t *testing.T) {
	cfg, err := runNewCommand(t, []string{"-n", "3"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.Iterations != 3 {
		t.Errorf("expected iterations=3, got %d", cfg.Iterations)
	}
}

// 4. --iterations -1 → error containing "--iterations must be a non-negative integer".
func TestNewCommand_NegativeIterations(t *testing.T) {
	cfg := &Config{}
	cmd := NewCommand(cfg)
	cmd.SetArgs([]string{"--iterations", "-1"})
	err := cmd.Execute()
	if err == nil {
		t.Fatal("expected error for --iterations -1, got nil")
	}
	if !strings.Contains(err.Error(), "--iterations must be a non-negative integer") {
		t.Errorf("expected error to contain %q, got %q", "--iterations must be a non-negative integer", err.Error())
	}
}

// 5. --project-dir /tmp/foo → project-dir=/tmp/foo.
func TestNewCommand_LongProjectDirFlag(t *testing.T) {
	cfg, err := runNewCommand(t, []string{"--project-dir", "/tmp/foo"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.ProjectDir != "/tmp/foo" {
		t.Errorf("expected project-dir=/tmp/foo, got %q", cfg.ProjectDir)
	}
}

// 6. -p /tmp/foo → project-dir=/tmp/foo (short flag).
func TestNewCommand_ShortProjectDirFlag(t *testing.T) {
	cfg, err := runNewCommand(t, []string{"-p", "/tmp/foo"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.ProjectDir != "/tmp/foo" {
		t.Errorf("expected project-dir=/tmp/foo, got %q", cfg.ProjectDir)
	}
}

// 7. Both flags: -n 3 -p /tmp/foo → iterations=3, project-dir=/tmp/foo.
func TestNewCommand_BothFlags(t *testing.T) {
	cfg, err := runNewCommand(t, []string{"-n", "3", "-p", "/tmp/foo"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.Iterations != 3 {
		t.Errorf("expected iterations=3, got %d", cfg.Iterations)
	}
	if cfg.ProjectDir != "/tmp/foo" {
		t.Errorf("expected project-dir=/tmp/foo, got %q", cfg.ProjectDir)
	}
}

// 8. Positional args rejected: ["somearg"] → error from cobra.NoArgs.
func TestNewCommand_PositionalArgRejected(t *testing.T) {
	cfg := &Config{}
	cmd := NewCommand(cfg)
	cmd.SetArgs([]string{"somearg"})
	err := cmd.Execute()
	if err == nil {
		t.Fatal("expected error for positional arg, got nil")
	}
}

// 9. Unknown flags: ["--unknown"] → error.
func TestNewCommand_UnknownFlag(t *testing.T) {
	cfg := &Config{}
	cmd := NewCommand(cfg)
	cmd.SetArgs([]string{"--unknown"})
	err := cmd.Execute()
	if err == nil {
		t.Fatal("expected error for unknown flag, got nil")
	}
}

// 10. --help → Execute() returns (nil, nil) — no workflow started.
func TestExecute_HelpReturnsNilNil(t *testing.T) {
	// We can't inject args into Execute(), so we test via NewCommand + cmd.SetArgs.
	// Execute() itself uses os.Args; test the --help guard behavior via the internal pattern.
	cfg := &Config{}
	var ranE bool
	cmd := newCommandImpl(cfg, &ranE)
	cmd.SetArgs([]string{"--help"})
	err := cmd.Execute()
	if err != nil {
		t.Fatalf("unexpected error for --help: %v", err)
	}
	if ranE {
		t.Error("RunE should not have executed for --help")
	}
}

// 11. --iterations=3 (equals syntax) → iterations=3.
func TestNewCommand_EqualsSyntax(t *testing.T) {
	cfg, err := runNewCommand(t, []string{"--iterations=3"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.Iterations != 3 {
		t.Errorf("expected iterations=3, got %d", cfg.Iterations)
	}
}

// 12. --iterations with no value → error from pflag.
func TestNewCommand_IterationsNoValue(t *testing.T) {
	cfg := &Config{}
	cmd := NewCommand(cfg)
	cmd.SetArgs([]string{"--iterations"})
	err := cmd.Execute()
	if err == nil {
		t.Fatal("expected error for --iterations with no value, got nil")
	}
}

// 13. -n with no value → error from pflag.
func TestNewCommand_ShortIterationsNoValue(t *testing.T) {
	cfg := &Config{}
	cmd := NewCommand(cfg)
	cmd.SetArgs([]string{"-n"})
	err := cmd.Execute()
	if err == nil {
		t.Fatal("expected error for -n with no value, got nil")
	}
}

// 14. -- extraarg → error from cobra.NoArgs (args after -- are still positional).
func TestNewCommand_ArgsAfterSeparatorRejected(t *testing.T) {
	cfg := &Config{}
	cmd := NewCommand(cfg)
	cmd.SetArgs([]string{"--", "extraarg"})
	err := cmd.Execute()
	if err == nil {
		t.Fatal("expected error for positional arg after --, got nil")
	}
}

// 15. -n 0 (explicit zero) → iterations=0, equivalent to omitting (until-done mode).
func TestNewCommand_ExplicitZeroIterations(t *testing.T) {
	cfg, err := runNewCommand(t, []string{"-n", "0", "-p", "/tmp/foo"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.Iterations != 0 {
		t.Errorf("expected iterations=0, got %d", cfg.Iterations)
	}
}

// 16. Large iteration count (1000) → accepted.
func TestNewCommand_LargeIterations(t *testing.T) {
	cfg, err := runNewCommand(t, []string{"-n", "1000", "-p", "/tmp/foo"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.Iterations != 1000 {
		t.Errorf("expected iterations=1000, got %d", cfg.Iterations)
	}
}
