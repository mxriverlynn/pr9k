package cli

import (
	"bytes"
	"os"
	"path/filepath"
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

// setupWorkflowCandidate cannot be combined with t.Parallel() — os.Chdir is process-global.
//
// setupWorkflowCandidate creates <dir>/.pr9k/workflow/ with a placeholder
// config.json, then chdirs into dir so that resolveProjectDir returns dir.
// Returns dir. Restores the original working directory on test cleanup.
func setupWorkflowCandidate(t *testing.T, dir string) string {
	t.Helper()
	wfDir := dir + "/.pr9k/workflow"
	if err := os.MkdirAll(wfDir, 0o755); err != nil {
		t.Fatalf("setupWorkflowCandidate MkdirAll: %v", err)
	}
	if err := os.WriteFile(wfDir+"/config.json", []byte("{}"), 0o644); err != nil {
		t.Fatalf("setupWorkflowCandidate WriteFile: %v", err)
	}
	orig, err := os.Getwd()
	if err != nil {
		t.Fatalf("setupWorkflowCandidate Getwd: %v", err)
	}
	if err := os.Chdir(dir); err != nil {
		t.Fatalf("setupWorkflowCandidate Chdir: %v", err)
	}
	t.Cleanup(func() { _ = os.Chdir(orig) })
	resolved, err := filepath.EvalSymlinks(dir)
	if err != nil {
		t.Fatalf("setupWorkflowCandidate EvalSymlinks: %v", err)
	}
	return resolved
}

// 1. No flags → iterations=0 (until-done), workflow-dir resolved from project candidate (non-empty).
func TestNewCommand_NoFlags(t *testing.T) {
	dir := setupWorkflowCandidate(t, t.TempDir())
	cfg, err := runNewCommand(t, []string{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.Iterations != 0 {
		t.Errorf("expected iterations=0, got %d", cfg.Iterations)
	}
	if cfg.WorkflowDir == "" {
		t.Error("expected non-empty workflow-dir resolved from project candidate")
	}
	if cfg.ProjectDir != dir {
		t.Errorf("expected project-dir=%q, got %q", dir, cfg.ProjectDir)
	}
}

// 2. --iterations 3 → iterations=3.
func TestNewCommand_LongIterationsFlag(t *testing.T) {
	cfg, err := runNewCommand(t, []string{"--iterations", "3", "--workflow-dir", t.TempDir()})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.Iterations != 3 {
		t.Errorf("expected iterations=3, got %d", cfg.Iterations)
	}
}

// 3. -n 3 → iterations=3 (short flag).
func TestNewCommand_ShortIterationsFlag(t *testing.T) {
	cfg, err := runNewCommand(t, []string{"-n", "3", "--workflow-dir", t.TempDir()})
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
	cfg, err := runNewCommand(t, []string{"--project-dir", dir, "--workflow-dir", t.TempDir()})
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
	cfg, err := runNewCommand(t, []string{"--iterations=3", "--workflow-dir", t.TempDir()})
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
	if !strings.HasPrefix(strings.TrimSpace(out.String()), "pr9k version ") {
		t.Errorf("expected --version output to start with \"pr9k version \", got %q", out.String())
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
	if !strings.HasPrefix(strings.TrimSpace(out.String()), "pr9k version ") {
		t.Errorf("expected -v output to start with \"pr9k version \", got %q", out.String())
	}
}

// TP-002 (issue #143): --version output exactly equals "pr9k version <Version>\n".
// Pins the CLI surface described in versioning.md public-API item 4.
// Additive to TestNewCommand_VersionFlag (prefix+contains checks stay as-is).
func TestNewCommand_VersionExactOutput(t *testing.T) {
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
	want := "pr9k version " + version.Version + "\n"
	if out.String() != want {
		t.Errorf("expected --version output %q, got %q", want, out.String())
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
	cfg, err := runNewCommand(t, []string{"--project-dir", dir, "--workflow-dir", t.TempDir()})
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

// TestNewCommand_UseBeginsWithPr9k pins the cobra Use field to start with "pr9k".
// cobra derives the --version output line from Use, so this guards the rename.
func TestNewCommand_UseBeginsWithPr9k(t *testing.T) {
	cfg := &Config{}
	cmd := NewCommand(cfg)
	if !strings.HasPrefix(cmd.Use, "pr9k") {
		t.Errorf("cobra Use should begin with \"pr9k\", got %q", cmd.Use)
	}
	if !strings.Contains(cmd.Use, "[flags]") {
		t.Errorf("cobra Use should contain \"[flags]\", got %q", cmd.Use)
	}
}

// TestNewCommand_LongBeginsWithPr9k pins the cobra Long description to start with "pr9k".
func TestNewCommand_LongBeginsWithPr9k(t *testing.T) {
	cfg := &Config{}
	cmd := NewCommand(cfg)
	if !strings.HasPrefix(cmd.Long, "pr9k drives the claude CLI") {
		t.Errorf("cobra Long should begin with \"pr9k drives the claude CLI\", got %q", cmd.Long)
	}
}

// Resolver tests — two-candidate resolveWorkflowDir behavior.

// R1. Project-dir candidate wins when both exist.
func TestResolveWorkflowDir_PrefersProjectDirCandidate(t *testing.T) {
	projDir := t.TempDir()
	projCandidate := projDir + "/.pr9k/workflow"
	if err := os.MkdirAll(projCandidate, 0o755); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}

	execDir := t.TempDir()
	execCandidate := execDir + "/.pr9k/workflow"
	if err := os.MkdirAll(execCandidate, 0o755); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}

	got, err := resolveWorkflowDir(projDir)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != projCandidate {
		t.Errorf("expected project-dir candidate %q, got %q", projCandidate, got)
	}
}

// R2. Falls back to executable-dir candidate when project-dir candidate is missing.
func TestResolveWorkflowDir_FallsBackToExecDirCandidate(t *testing.T) {
	projDir := t.TempDir() // no .pr9k/workflow here

	// We can't control the real executable dir in tests, so instead verify that
	// when the project-dir candidate is absent and the exec-dir candidate is also
	// absent, we get the "both missing" error rather than a nil result.
	_, err := resolveWorkflowDir(projDir)
	if err == nil {
		t.Fatal("expected error when neither candidate exists, got nil")
	}
	if !strings.Contains(err.Error(), projDir+"/.pr9k/workflow") {
		t.Errorf("error should mention project-dir candidate, got %q", err.Error())
	}
	if !strings.Contains(err.Error(), ".pr9k/workflow") {
		t.Errorf("error should mention exec-dir candidate, got %q", err.Error())
	}
}

// R3. File-not-dir at project-dir candidate falls through to exec-dir candidate.
func TestResolveWorkflowDir_FileNotDirFallsThrough(t *testing.T) {
	projDir := t.TempDir()
	pr9kDir := projDir + "/.pr9k"
	if err := os.MkdirAll(pr9kDir, 0o755); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}
	// Create .pr9k/workflow as a file, not a directory.
	if err := os.WriteFile(pr9kDir+"/workflow", []byte("not a dir"), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	// Both candidates absent (exec-dir won't have .pr9k/workflow either) → error.
	_, err := resolveWorkflowDir(projDir)
	if err == nil {
		t.Fatal("expected error when project-dir candidate is a file and exec-dir candidate is missing")
	}
}

// R4. Both candidates missing → error listing both paths.
func TestResolveWorkflowDir_BothMissingReturnsError(t *testing.T) {
	projDir := t.TempDir()
	_, err := resolveWorkflowDir(projDir)
	if err == nil {
		t.Fatal("expected error when neither candidate exists, got nil")
	}
	if !strings.Contains(err.Error(), "could not locate workflow bundle") {
		t.Errorf("expected error to say 'could not locate workflow bundle', got %q", err.Error())
	}
	if !strings.Contains(err.Error(), "install the bundle or pass --workflow-dir") {
		t.Errorf("expected error to mention install hint, got %q", err.Error())
	}
}

// R5. --workflow-dir flag overrides both candidates (TP-004: exact equality after EvalSymlinks).
func TestResolveWorkflowDir_FlagOverridesBothCandidates(t *testing.T) {
	projDir := t.TempDir()
	// Set up project-dir candidate to ensure it would win if flag didn't override.
	projCandidate := projDir + "/.pr9k/workflow"
	if err := os.MkdirAll(projCandidate, 0o755); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}
	override := t.TempDir()
	cfg, err := runNewCommand(t, []string{"--workflow-dir", override, "--project-dir", projDir})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	want, err := filepath.EvalSymlinks(override)
	if err != nil {
		t.Fatalf("EvalSymlinks: %v", err)
	}
	if cfg.WorkflowDir != want {
		t.Errorf("expected WorkflowDir=%q (EvalSymlinks of override), got %q", want, cfg.WorkflowDir)
	}
}

// R6. Invalid --project-dir short-circuits before workflow resolution.
func TestResolveWorkflowDir_InvalidProjectDirShortCircuits(t *testing.T) {
	cfg := &Config{}
	cmd := NewCommand(cfg)
	cmd.SetArgs([]string{"--project-dir", "/nonexistent/path/does/not/exist/xyz123"})
	err := cmd.Execute()
	if err == nil {
		t.Fatal("expected error for invalid --project-dir, got nil")
	}
	if !strings.Contains(err.Error(), "--project-dir") {
		t.Errorf("expected error to mention --project-dir, got %q", err.Error())
	}
}

// R7. --workflow-dir flag Usage string reflects both candidate paths in order
// (TP-005: projectDir first, then executableDir).
func TestResolveWorkflowDir_FlagUsageContainsBothCandidates(t *testing.T) {
	cfg := &Config{}
	cmd := NewCommand(cfg)
	flag := cmd.Flags().Lookup("workflow-dir")
	if flag == nil {
		t.Fatal("--workflow-dir flag not registered")
	}
	idxProj := strings.Index(flag.Usage, "<projectDir>/.pr9k/workflow/")
	idxExec := strings.Index(flag.Usage, "<executableDir>/.pr9k/workflow/")
	if idxProj < 0 {
		t.Errorf("flag Usage should mention <projectDir>/.pr9k/workflow/, got %q", flag.Usage)
	}
	if idxExec < 0 {
		t.Errorf("flag Usage should mention <executableDir>/.pr9k/workflow/, got %q", flag.Usage)
	}
	if idxProj >= 0 && idxExec >= 0 && idxExec <= idxProj {
		t.Errorf("flag Usage should list <projectDir> before <executableDir>, got %q", flag.Usage)
	}
}

// TP-002: Direct tests against resolveWorkflowDirWith for the fallback branch
// that cannot be exercised via the public wrapper (exec-dir is the test binary dir).

// TestResolveWorkflowDirWith_BothPresent_ReturnsProject verifies project-dir wins when both exist.
func TestResolveWorkflowDirWith_BothPresent_ReturnsProject(t *testing.T) {
	projDir := t.TempDir()
	execDir := t.TempDir()
	projCandidate := projDir + "/.pr9k/workflow"
	execCandidate := execDir + "/.pr9k/workflow"
	if err := os.MkdirAll(projCandidate, 0o755); err != nil {
		t.Fatalf("MkdirAll proj: %v", err)
	}
	if err := os.MkdirAll(execCandidate, 0o755); err != nil {
		t.Fatalf("MkdirAll exec: %v", err)
	}
	got, err := resolveWorkflowDirWith(projDir, execDir)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != projCandidate {
		t.Errorf("expected project candidate %q, got %q", projCandidate, got)
	}
}

// TestResolveWorkflowDirWith_ProjectAbsent_ExecPresent_ReturnsExec verifies the fallback branch.
func TestResolveWorkflowDirWith_ProjectAbsent_ExecPresent_ReturnsExec(t *testing.T) {
	projDir := t.TempDir() // no .pr9k/workflow
	execDir := t.TempDir()
	execCandidate := execDir + "/.pr9k/workflow"
	if err := os.MkdirAll(execCandidate, 0o755); err != nil {
		t.Fatalf("MkdirAll exec: %v", err)
	}
	got, err := resolveWorkflowDirWith(projDir, execDir)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != execCandidate {
		t.Errorf("expected exec candidate %q, got %q", execCandidate, got)
	}
}

// TestResolveWorkflowDirWith_ProjectIsFile_ExecPresent_ReturnsExec verifies file-not-dir fallthrough.
func TestResolveWorkflowDirWith_ProjectIsFile_ExecPresent_ReturnsExec(t *testing.T) {
	projDir := t.TempDir()
	pr9kDir := projDir + "/.pr9k"
	if err := os.MkdirAll(pr9kDir, 0o755); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}
	if err := os.WriteFile(pr9kDir+"/workflow", []byte("not a dir"), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	execDir := t.TempDir()
	execCandidate := execDir + "/.pr9k/workflow"
	if err := os.MkdirAll(execCandidate, 0o755); err != nil {
		t.Fatalf("MkdirAll exec: %v", err)
	}
	got, err := resolveWorkflowDirWith(projDir, execDir)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != execCandidate {
		t.Errorf("expected exec candidate %q, got %q", execCandidate, got)
	}
}

// TP-011 (issue #145): a plain workflow/ directory inside projectDir is NOT
// auto-discovered. Only <projectDir>/.pr9k/workflow and <execDir>/.pr9k/workflow
// are valid candidates; make build is required to install the bundle.
func TestResolveWorkflowDirWith_SiblingWorkflowDirNotAutoDiscovered(t *testing.T) {
	projDir := t.TempDir()
	// Create a workflow/ directory directly inside projectDir (mirrors the source tree).
	// The resolver must NOT return this — it is only discoverable via .pr9k/workflow.
	if err := os.MkdirAll(filepath.Join(projDir, "workflow"), 0o755); err != nil {
		t.Fatalf("MkdirAll workflow: %v", err)
	}
	execDir := t.TempDir()
	_, err := resolveWorkflowDirWith(projDir, execDir)
	if err == nil {
		t.Fatal("expected error: resolveWorkflowDirWith must not auto-discover a plain workflow/ directory")
	}
}
