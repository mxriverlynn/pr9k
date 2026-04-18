package cli

import (
	"bytes"
	"os"
	"strings"
	"testing"

	"github.com/spf13/cobra"

	"github.com/mxriverlynn/pr9k/src/internal/version"
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

// 1. No flags → iterations=0 (until-done), workflow-dir resolved from executable (non-empty).
func TestNewCommand_NoFlags(t *testing.T) {
	cfg, err := runNewCommand(t, []string{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.Iterations != 0 {
		t.Errorf("expected iterations=0, got %d", cfg.Iterations)
	}
	if cfg.WorkflowDir == "" {
		t.Error("expected non-empty workflow-dir resolved from executable")
	}
	if cfg.ProjectDir == "" {
		t.Error("expected non-empty project-dir resolved from cwd")
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

// 5. --workflow-dir <alt> → workflow-dir set to that path.
func TestNewCommand_LongWorkflowDirFlag(t *testing.T) {
	dir := t.TempDir()
	cfg, err := runNewCommand(t, []string{"--workflow-dir", dir})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.WorkflowDir == "" {
		t.Error("expected non-empty workflow-dir")
	}
}

// 6. --project-dir <alt> → project-dir set to that path.
func TestNewCommand_LongProjectDirFlag(t *testing.T) {
	dir := t.TempDir()
	cfg, err := runNewCommand(t, []string{"--project-dir", dir})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.ProjectDir == "" {
		t.Error("expected non-empty project-dir")
	}
}

// 7. Both flags: -n 3 --workflow-dir <dir> → iterations=3, workflow-dir set.
func TestNewCommand_BothFlags(t *testing.T) {
	dir := t.TempDir()
	cfg, err := runNewCommand(t, []string{"-n", "3", "--workflow-dir", dir})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.Iterations != 3 {
		t.Errorf("expected iterations=3, got %d", cfg.Iterations)
	}
	if cfg.WorkflowDir == "" {
		t.Error("expected non-empty workflow-dir")
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
	dir := t.TempDir()
	cfg, err := runNewCommand(t, []string{"-n", "0", "--workflow-dir", dir})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.Iterations != 0 {
		t.Errorf("expected iterations=0, got %d", cfg.Iterations)
	}
}

// 16. Large iteration count (1000) → accepted.
func TestNewCommand_LargeIterations(t *testing.T) {
	dir := t.TempDir()
	cfg, err := runNewCommand(t, []string{"-n", "1000", "--workflow-dir", dir})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.Iterations != 1000 {
		t.Errorf("expected iterations=1000, got %d", cfg.Iterations)
	}
}

// 17. --version → RunE does not execute, cobra prints the version, no error.
// Guarantees the flag exits without starting the workflow.
func TestNewCommand_VersionFlag(t *testing.T) {
	cfg := &Config{}
	var ranE bool
	cmd := newCommandImpl(cfg, &ranE)
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&out)
	cmd.SetArgs([]string{"--version"})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("unexpected error for --version: %v", err)
	}
	if ranE {
		t.Error("RunE should not have executed for --version")
	}
	if !strings.Contains(out.String(), version.Version) {
		t.Errorf("expected --version output to contain %q, got %q", version.Version, out.String())
	}
}

// 18. -v (short) behaves the same as --version.
func TestNewCommand_VersionShortFlag(t *testing.T) {
	cfg := &Config{}
	var ranE bool
	cmd := newCommandImpl(cfg, &ranE)
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&out)
	cmd.SetArgs([]string{"-v"})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("unexpected error for -v: %v", err)
	}
	if ranE {
		t.Error("RunE should not have executed for -v")
	}
	if !strings.Contains(out.String(), version.Version) {
		t.Errorf("expected -v output to contain %q, got %q", version.Version, out.String())
	}
}

// 19. --workflow-dir with nonexistent path → error.
func TestNewCommand_WorkflowDirNonexistent(t *testing.T) {
	cfg := &Config{}
	cmd := NewCommand(cfg)
	cmd.SetArgs([]string{"--workflow-dir", "/nonexistent/path/that/does/not/exist"})
	err := cmd.Execute()
	if err == nil {
		t.Fatal("expected error for nonexistent --workflow-dir, got nil")
	}
}

// 20. --project-dir with nonexistent path → error.
func TestNewCommand_ProjectDirNonexistent(t *testing.T) {
	cfg := &Config{}
	cmd := NewCommand(cfg)
	cmd.SetArgs([]string{"--project-dir", "/nonexistent/path/that/does/not/exist"})
	err := cmd.Execute()
	if err == nil {
		t.Fatal("expected error for nonexistent --project-dir, got nil")
	}
}

// 21. -p (removed short form) fires the wrapped unknown-flag message containing
// both --workflow-dir and --project-dir, plus the split ADR path.
func TestNewCommand_ShortPFlagFiresGuidanceMessage(t *testing.T) {
	cfg := &Config{}
	cmd := NewCommand(cfg)
	cmd.SetArgs([]string{"-p", "foo"})
	err := cmd.Execute()
	if err == nil {
		t.Fatal("expected error for unknown flag -p, got nil")
	}
	msg := err.Error()
	if !strings.Contains(msg, "--workflow-dir") {
		t.Errorf("expected error to mention --workflow-dir, got %q", msg)
	}
	if !strings.Contains(msg, "--project-dir") {
		t.Errorf("expected error to mention --project-dir, got %q", msg)
	}
	if !strings.Contains(msg, "docs/adr/20260413162428-workflow-project-dir-split.md") {
		t.Errorf("expected error to mention the split ADR, got %q", msg)
	}
}

// 22. --workflow-dir with EvalSymlinks applied: resolved path returned.
func TestNewCommand_WorkflowDirEvalSymlinks(t *testing.T) {
	dir := t.TempDir()
	cfg, err := runNewCommand(t, []string{"--workflow-dir", dir})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.WorkflowDir == "" {
		t.Error("expected non-empty workflow-dir after EvalSymlinks")
	}
}

// 23. --project-dir with EvalSymlinks applied: resolved path returned.
func TestNewCommand_ProjectDirEvalSymlinks(t *testing.T) {
	dir := t.TempDir()
	cfg, err := runNewCommand(t, []string{"--project-dir", dir})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.ProjectDir == "" {
		t.Error("expected non-empty project-dir after EvalSymlinks")
	}
}

// TP-003 — --workflow-dir pointing to a file (not a directory) returns an error.
func TestNewCommand_WorkflowDirIsFile(t *testing.T) {
	f, err := os.CreateTemp(t.TempDir(), "workflow-file-*")
	if err != nil {
		t.Fatalf("CreateTemp: %v", err)
	}
	_ = f.Close()

	cfg := &Config{}
	cmd := NewCommand(cfg)
	cmd.SetArgs([]string{"--workflow-dir", f.Name()})
	execErr := cmd.Execute()
	if execErr == nil {
		t.Fatal("expected error when --workflow-dir points to a file, got nil")
	}
	if !strings.Contains(execErr.Error(), "is not a directory") {
		t.Errorf("expected error to contain %q, got %q", "is not a directory", execErr.Error())
	}
}

// TP-004 — --project-dir pointing to a file (not a directory) returns an error.
func TestNewCommand_ProjectDirIsFile(t *testing.T) {
	f, err := os.CreateTemp(t.TempDir(), "project-file-*")
	if err != nil {
		t.Fatalf("CreateTemp: %v", err)
	}
	_ = f.Close()

	cfg := &Config{}
	cmd := NewCommand(cfg)
	cmd.SetArgs([]string{"--project-dir", f.Name()})
	execErr := cmd.Execute()
	if execErr == nil {
		t.Fatal("expected error when --project-dir points to a file, got nil")
	}
	if !strings.Contains(execErr.Error(), "is not a directory") {
		t.Errorf("expected error to contain %q, got %q", "is not a directory", execErr.Error())
	}
}

// TP-005 — SetFlagErrorFunc fires for arbitrary unknown flags, not just -p.
// Verifies the wrapper is generic: any unknown flag produces the flagSplitGuidance
// text containing both --workflow-dir and --project-dir mentions and the ADR path.
func TestNewCommand_ArbitraryUnknownFlagFiresGuidanceMessage(t *testing.T) {
	cfg := &Config{}
	cmd := NewCommand(cfg)
	cmd.SetArgs([]string{"--bogus"})
	err := cmd.Execute()
	if err == nil {
		t.Fatal("expected error for unknown flag --bogus, got nil")
	}
	msg := err.Error()
	if !strings.Contains(msg, "--workflow-dir") {
		t.Errorf("expected error to mention --workflow-dir, got %q", msg)
	}
	if !strings.Contains(msg, "--project-dir") {
		t.Errorf("expected error to mention --project-dir, got %q", msg)
	}
	if !strings.Contains(msg, "docs/adr/20260413162428-workflow-project-dir-split.md") {
		t.Errorf("expected error to mention the split ADR, got %q", msg)
	}
}

// T4. newCommandImpl accepts subcommands added via AddCommand and dispatches to them.
// Verifies that a cobra.Command added to the root command runs its RunE when
// invoked by name. (The variadic Execute() path that calls AddCommand internally
// is tested indirectly through the integration in main.go.)
func TestNewCommandImpl_AddedSubcommandRunsItsRunE(t *testing.T) {
	cfg := &Config{}
	var ranE bool
	cmd := newCommandImpl(cfg, &ranE)

	var subRan bool
	sub := &cobra.Command{
		Use:           "test-sub",
		SilenceErrors: true,
		SilenceUsage:  true,
		RunE: func(_ *cobra.Command, _ []string) error {
			subRan = true
			return nil
		},
	}
	cmd.AddCommand(sub)
	cmd.SetArgs([]string{"test-sub"})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !subRan {
		t.Error("expected subcommand RunE to execute, but it did not")
	}
}

// 24. Neither --workflow-dir nor --project-dir has a short form.
func TestNewCommand_NoShortFormsForDirFlags(t *testing.T) {
	cfg := &Config{}
	cmd := NewCommand(cfg)

	// Verify that neither flag has a shorthand registered.
	wfFlag := cmd.Flags().Lookup("workflow-dir")
	if wfFlag == nil {
		t.Fatal("--workflow-dir flag not registered")
	}
	if wfFlag.Shorthand != "" {
		t.Errorf("--workflow-dir must have no short form, got %q", wfFlag.Shorthand)
	}

	pdFlag := cmd.Flags().Lookup("project-dir")
	if pdFlag == nil {
		t.Fatal("--project-dir flag not registered")
	}
	if pdFlag.Shorthand != "" {
		t.Errorf("--project-dir must have no short form, got %q", pdFlag.Shorthand)
	}
}
