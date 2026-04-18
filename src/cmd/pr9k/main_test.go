package main

import (
	"bytes"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/spf13/cobra"

	"github.com/mxriverlynn/pr9k/src/internal/cli"
	"github.com/mxriverlynn/pr9k/src/internal/steps"
	"github.com/mxriverlynn/pr9k/src/internal/version"
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

// writeMinimalStepFile creates a minimal valid config.json under dir so
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
	if err := os.WriteFile(filepath.Join(dir, "config.json"), []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
}

// writeInvalidStepFile creates a config.json whose JSON is valid but
// fails D13 validation — a claude step missing the required promptFile field.
// Used to exercise the validator.Validate failure path in startup.
func writeInvalidStepFile(t *testing.T, dir string) {
	t.Helper()
	content := `{
		"initialize": [],
		"iteration": [
			{ "name": "bad-step", "isClaude": true, "model": "sonnet" }
		],
		"finalize": []
	}`
	if err := os.WriteFile(filepath.Join(dir, "config.json"), []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
}

// TestStartupPreflight_RunsBeforeOrchestrator verifies that startup() fails
// fast when the sandbox image is missing, returning (nil, false) and printing
// the preflight error to stderr. Because startup returns false, the
// runner/orchestrator is never started (the caller exits before reaching it).
func TestStartupPreflight_RunsBeforeOrchestrator(t *testing.T) {
	workflowDir := t.TempDir()
	projectDir := t.TempDir()
	profileDir := t.TempDir() // real dir so profile check passes
	writeMinimalStepFile(t, workflowDir)

	prober := &fakeProber{
		binaryAvailable: true,
		daemonErr:       nil,
		imagePresent:    false, // image missing → preflight error
	}

	cfg := &cli.Config{WorkflowDir: workflowDir, ProjectDir: projectDir}
	var buf bytes.Buffer
	svc, ok := startup(cfg, projectDir, profileDir, prober, &buf)
	if ok {
		t.Fatal("startup() returned ok=true; want false when sandbox image is missing")
	}
	if svc != nil {
		t.Error("startup() returned non-nil services on preflight failure")
	}

	got := buf.String()
	const wantSubstr = "preflight: claude sandbox image is missing. Run: pr9k sandbox create"
	if !strings.Contains(got, wantSubstr) {
		t.Errorf("stderr %q does not contain %q", got, wantSubstr)
	}
}

// TestStartupPreflight_SkippedForSandboxCreate verifies that when the
// `sandbox create` subcommand is dispatched, the root RunE does NOT fire.
// Because startup() is only called from the root RunE path, its non-execution
// proves preflight is skipped for sandbox subcommands.
func TestStartupPreflight_SkippedForSandboxCreate(t *testing.T) {
	cfg := &cli.Config{}
	rootCmd := cli.NewCommand(cfg)
	parent := &cobra.Command{Use: "sandbox"}
	parent.AddCommand(&cobra.Command{
		Use:  "create",
		RunE: func(_ *cobra.Command, _ []string) error { return nil },
	})
	rootCmd.AddCommand(parent)
	rootCmd.SetArgs([]string{"sandbox", "create"})
	if err := rootCmd.Execute(); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// Root RunE populates WorkflowDir when it fires. If it remains empty,
	// root RunE did not run — startup() (and therefore preflight) was not called.
	if cfg.WorkflowDir != "" {
		t.Errorf("root RunE fired for `sandbox create`; WorkflowDir = %q, startup() would have been reached", cfg.WorkflowDir)
	}
}

// TestStartupPreflight_SkippedForSandboxLogin verifies the same non-dispatch
// behavior for the `sandbox login` subcommand.
func TestStartupPreflight_SkippedForSandboxLogin(t *testing.T) {
	cfg := &cli.Config{}
	rootCmd := cli.NewCommand(cfg)
	parent := &cobra.Command{Use: "sandbox"}
	parent.AddCommand(&cobra.Command{
		Use:  "login",
		RunE: func(_ *cobra.Command, _ []string) error { return nil },
	})
	rootCmd.AddCommand(parent)
	rootCmd.SetArgs([]string{"sandbox", "login"})
	if err := rootCmd.Execute(); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.WorkflowDir != "" {
		t.Errorf("root RunE fired for `sandbox login`; WorkflowDir = %q, startup() would have been reached", cfg.WorkflowDir)
	}
}

// TestStartup_HappyPath verifies the startup() happy path: valid step file,
// passing prober, and a zero-byte .credentials.json that triggers a warning.
// It asserts all returned services are wired correctly and that the warning
// text appears in the output buffer.
func TestStartup_HappyPath(t *testing.T) {
	workflowDir := t.TempDir()
	projectDir := t.TempDir()
	profileDir := t.TempDir()
	writeMinimalStepFile(t, workflowDir)

	// Write a zero-byte .credentials.json to trigger the credentials warning.
	credPath := filepath.Join(profileDir, ".credentials.json")
	if err := os.WriteFile(credPath, []byte{}, 0o600); err != nil {
		t.Fatal(err)
	}

	prober := &fakeProber{
		binaryAvailable: true,
		daemonErr:       nil,
		imagePresent:    true,
	}

	cfg := &cli.Config{WorkflowDir: workflowDir, ProjectDir: projectDir}
	var buf bytes.Buffer
	svc, ok := startup(cfg, projectDir, profileDir, prober, &buf)
	if !ok {
		t.Fatalf("startup() returned ok=false; want true on happy path. stderr=%q", buf.String())
	}
	if svc == nil {
		t.Fatal("startup() returned nil services on happy path")
	}
	defer func() { _ = svc.log.Close() }()

	if svc.log == nil {
		t.Error("svc.log is nil")
	}
	if svc.runner == nil {
		t.Error("svc.runner is nil")
	}
	if len(svc.stepFile.Iteration) == 0 {
		t.Error("svc.stepFile.Iteration is empty; expected at least one step")
	}

	// Warning text for zero-byte credentials must appear in output.
	got := buf.String()
	if !strings.Contains(got, "Warning:") {
		t.Errorf("expected credentials warning in output, got: %q", got)
	}
	if !strings.Contains(got, "is empty") {
		t.Errorf("expected 'is empty' in credentials warning, got: %q", got)
	}

	// .pr9k/logs/ directory must be created under projectDir (not workflowDir).
	if _, err := os.Stat(filepath.Join(projectDir, ".pr9k", "logs")); err != nil {
		t.Errorf("expected .pr9k/logs/ under projectDir, got error: %v", err)
	}
	if _, err := os.Stat(filepath.Join(workflowDir, ".pr9k", "logs")); !os.IsNotExist(err) {
		t.Errorf(".pr9k/logs/ should NOT exist under workflowDir; Stat err=%v", err)
	}

	// TP-001: per-run artifact dir must exist under .pr9k/logs/<runStamp>/.
	runStampDir := filepath.Join(projectDir, ".pr9k", "logs", svc.log.RunStamp())
	if info, err := os.Stat(runStampDir); err != nil || !info.IsDir() {
		t.Errorf("expected per-run artifact dir at %q, err=%v", runStampDir, err)
	}

	// TP-002: legacy logs/ must NOT be created under projectDir.
	if _, err := os.Stat(filepath.Join(projectDir, "logs")); !os.IsNotExist(err) {
		t.Errorf("legacy logs/ should NOT exist under projectDir; Stat err=%v", err)
	}
}

// TestStartup_LoadStepsFailure verifies that startup() returns (nil, false) and
// prints an error when config.json is missing — before creating the logger
// or running validation/preflight.
func TestStartup_LoadStepsFailure(t *testing.T) {
	workflowDir := t.TempDir() // no config.json
	projectDir := t.TempDir()
	profileDir := t.TempDir()

	prober := &fakeProber{
		binaryAvailable: true,
		daemonErr:       nil,
		imagePresent:    true,
	}

	cfg := &cli.Config{WorkflowDir: workflowDir, ProjectDir: projectDir}
	var buf bytes.Buffer
	svc, ok := startup(cfg, projectDir, profileDir, prober, &buf)
	if ok {
		t.Fatal("startup() returned ok=true; want false when config.json is missing")
	}
	if svc != nil {
		t.Error("startup() returned non-nil services on LoadSteps failure")
	}

	got := buf.String()
	if !strings.Contains(got, "error:") {
		t.Errorf("expected 'error:' in output, got: %q", got)
	}

	// Logger must NOT have been created — .pr9k/logs/ must not exist under projectDir.
	if _, err := os.Stat(filepath.Join(projectDir, ".pr9k", "logs")); !os.IsNotExist(err) {
		t.Errorf(".pr9k/logs/ should NOT exist under projectDir when LoadSteps fails before logger creation; Stat err=%v", err)
	}
}

// TestStartup_LoggerFailure verifies that startup() returns (nil, false) when
// logger creation fails due to an unwritable projectDir, after validation and
// preflight have passed.
func TestStartup_LoggerFailure(t *testing.T) {
	workflowDir := t.TempDir()
	profileDir := t.TempDir()
	writeMinimalStepFile(t, workflowDir)

	prober := &fakeProber{
		binaryAvailable: true,
		daemonErr:       nil,
		imagePresent:    true,
	}

	cfg := &cli.Config{WorkflowDir: workflowDir, ProjectDir: "/nonexistent/unwritable/path"}
	var buf bytes.Buffer
	svc, ok := startup(cfg, "/nonexistent/unwritable/path", profileDir, prober, &buf)
	if ok {
		t.Fatal("startup() returned ok=true; want false when logger creation fails")
	}
	if svc != nil {
		t.Error("startup() returned non-nil services on logger failure")
	}

	got := buf.String()
	if got == "" {
		t.Errorf("expected non-empty error output from startup(), got empty string")
	}
}

// TestStartup_ValidationOnlyErrors verifies that startup() returns (nil, false)
// when the step file fails D13 validation while preflight passes cleanly. The
// output must contain validation error markers but no "preflight:" lines.
func TestStartup_ValidationOnlyErrors(t *testing.T) {
	workflowDir := t.TempDir()
	projectDir := t.TempDir()
	profileDir := t.TempDir()
	writeInvalidStepFile(t, workflowDir)

	prober := &fakeProber{
		binaryAvailable: true,
		daemonErr:       nil,
		imagePresent:    true,
	}

	cfg := &cli.Config{WorkflowDir: workflowDir, ProjectDir: projectDir}
	var buf bytes.Buffer
	svc, ok := startup(cfg, projectDir, profileDir, prober, &buf)
	if ok {
		t.Fatal("startup() returned ok=true; want false on validation failure")
	}
	if svc != nil {
		t.Error("startup() returned non-nil services on validation failure")
	}

	got := buf.String()
	if !strings.Contains(got, "config error:") {
		t.Errorf("expected 'config error:' in output, got: %q", got)
	}
	if !strings.Contains(got, "validation error(s)") {
		t.Errorf("expected 'validation error(s)' in output, got: %q", got)
	}
	if strings.Contains(got, "preflight:") {
		t.Errorf("clean preflight should produce no 'preflight:' output, got: %q", got)
	}
}

// TestStartupPreflight_CollectsAllErrors verifies that startup() collects D13
// config errors AND preflight errors before printing, so all problems appear
// together rather than short-circuiting after the first failure.
func TestStartupPreflight_CollectsAllErrors(t *testing.T) {
	workflowDir := t.TempDir()
	projectDir := t.TempDir()
	// Profile dir that does not exist → preflight profile error.
	profileDir := filepath.Join(t.TempDir(), "nonexistent-profile")
	writeInvalidStepFile(t, workflowDir) // produces a D13 validation error

	// Docker binary unavailable → preflight docker error.
	prober := &fakeProber{binaryAvailable: false}

	cfg := &cli.Config{WorkflowDir: workflowDir, ProjectDir: projectDir}
	var buf bytes.Buffer
	svc, ok := startup(cfg, projectDir, profileDir, prober, &buf)
	if ok {
		t.Fatal("startup() returned ok=true; want false when D13 and preflight errors exist")
	}
	if svc != nil {
		t.Error("startup() returned non-nil services on combined error")
	}

	got := buf.String()
	checks := []string{
		"config error:",                      // D13 error line
		"validation error(s)",                // D13 count line
		"preflight: claude profile",          // preflight profile error
		"preflight: docker is not installed", // preflight docker error
	}
	for _, want := range checks {
		if !strings.Contains(got, want) {
			t.Errorf("stderr %q missing expected substring %q", got, want)
		}
	}
}

// TestStartup_PreflightOnlyErrors verifies that when only preflight errors exist
// (validation passes cleanly), startup() returns (nil, false) and the output
// contains the preflight error but does NOT contain a "validation error(s)" count
// line. This is the symmetric counterpart to TestStartup_ValidationOnlyErrors.
func TestStartup_PreflightOnlyErrors(t *testing.T) {
	workflowDir := t.TempDir()
	projectDir := t.TempDir()
	writeMinimalStepFile(t, workflowDir) // valid config — no D13 errors

	// Docker binary unavailable → preflight error; profile dir exists → no profile error.
	prober := &fakeProber{binaryAvailable: false}
	profileDir := t.TempDir() // exists → profile check passes

	cfg := &cli.Config{WorkflowDir: workflowDir, ProjectDir: projectDir}
	var buf bytes.Buffer
	svc, ok := startup(cfg, projectDir, profileDir, prober, &buf)
	if ok {
		t.Fatal("startup() returned ok=true; want false when preflight fails")
	}
	if svc != nil {
		t.Error("startup() returned non-nil services on preflight failure")
	}

	got := buf.String()
	if !strings.Contains(got, "preflight:") {
		t.Errorf("expected 'preflight:' in output, got: %q", got)
	}
	// Clean validation must not produce a "validation error(s)" count line.
	if strings.Contains(got, "validation error(s)") {
		t.Errorf("clean validation should produce no 'validation error(s)' line, got: %q", got)
	}
}

// writeWarningOnlyStepFile creates a config.json that triggers only a
// warning-level validation finding (GITHUB_TOKEN in containerEnv), no fatal errors.
func writeWarningOnlyStepFile(t *testing.T, dir string) {
	t.Helper()
	content := `{
		"containerEnv": {"GITHUB_TOKEN": "hardcoded-value"},
		"initialize": [],
		"iteration": [
			{ "name": "noop", "isClaude": false, "command": ["true"] }
		],
		"finalize": []
	}`
	if err := os.WriteFile(filepath.Join(dir, "config.json"), []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
}

// writeWarningAndFatalStepFile creates a config.json with both a fatal
// containerEnv error (CLAUDE_CONFIG_DIR is reserved) and a non-fatal warning
// (GITHUB_TOKEN looks like a secret).
func writeWarningAndFatalStepFile(t *testing.T, dir string) {
	t.Helper()
	content := `{
		"containerEnv": {
			"CLAUDE_CONFIG_DIR": "bad",
			"GITHUB_TOKEN": "hardcoded-value"
		},
		"initialize": [],
		"iteration": [
			{ "name": "noop", "isClaude": false, "command": ["true"] }
		],
		"finalize": []
	}`
	if err := os.WriteFile(filepath.Join(dir, "config.json"), []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
}

// TestStartup_WarningOnlyValidation_ProceedsAndPrintsWarning (TP-003) verifies
// that startup() returns ok=true and non-nil services when validation produces
// only non-fatal warnings (no fatal errors, no preflight errors), and that the
// warning text appears in stderr output.
func TestStartup_WarningOnlyValidation_ProceedsAndPrintsWarning(t *testing.T) {
	workflowDir := t.TempDir()
	projectDir := t.TempDir()
	profileDir := t.TempDir()
	writeWarningOnlyStepFile(t, workflowDir)

	prober := &fakeProber{
		binaryAvailable: true,
		daemonErr:       nil,
		imagePresent:    true,
	}

	cfg := &cli.Config{WorkflowDir: workflowDir, ProjectDir: projectDir}
	var buf bytes.Buffer
	svc, ok := startup(cfg, projectDir, profileDir, prober, &buf)
	if !ok {
		t.Fatalf("startup() returned ok=false with warning-only config; want true. stderr=%q", buf.String())
	}
	if svc == nil {
		t.Fatal("startup() returned nil services with warning-only config")
	}
	defer func() { _ = svc.log.Close() }()

	got := buf.String()
	if !strings.Contains(got, "config warning:") {
		t.Errorf("expected 'config warning:' in output, got: %q", got)
	}
}

// TestStartup_WarningOnlyValidation_NoValidationErrorCountLine (TP-004) verifies
// that when validation produces only non-fatal warnings, the "N validation error(s)"
// count line is NOT printed — it must only appear when fatalCount > 0.
func TestStartup_WarningOnlyValidation_NoValidationErrorCountLine(t *testing.T) {
	workflowDir := t.TempDir()
	projectDir := t.TempDir()
	profileDir := t.TempDir()
	writeWarningOnlyStepFile(t, workflowDir)

	prober := &fakeProber{
		binaryAvailable: true,
		daemonErr:       nil,
		imagePresent:    true,
	}

	cfg := &cli.Config{WorkflowDir: workflowDir, ProjectDir: projectDir}
	var buf bytes.Buffer
	_, ok := startup(cfg, projectDir, profileDir, prober, &buf)
	if !ok {
		t.Fatalf("startup() returned ok=false with warning-only config. stderr=%q", buf.String())
	}

	got := buf.String()
	if strings.Contains(got, "validation error(s)") {
		t.Errorf("warning-only config should not produce 'validation error(s)' line, got: %q", got)
	}
}

// TestStartup_FatalAndWarningValidation_CountsOnlyFatals (TP-014) verifies that
// when a step file combines a fatal error (reserved CLAUDE_CONFIG_DIR key) with a
// non-fatal warning (GITHUB_TOKEN), the output includes BOTH messages but the
// "N validation error(s)" count line reports only the fatal count (1, not 2).
// Intent: regression guard for the inline-mix print behavior in main.go:60-65.
func TestStartup_FatalAndWarningValidation_CountsOnlyFatals(t *testing.T) {
	workflowDir := t.TempDir()
	projectDir := t.TempDir()
	profileDir := t.TempDir()
	writeWarningAndFatalStepFile(t, workflowDir)

	prober := &fakeProber{
		binaryAvailable: true,
		daemonErr:       nil,
		imagePresent:    true,
	}

	cfg := &cli.Config{WorkflowDir: workflowDir, ProjectDir: projectDir}
	var buf bytes.Buffer
	svc, ok := startup(cfg, projectDir, profileDir, prober, &buf)
	if ok {
		t.Fatal("startup() returned ok=true; want false with fatal validation error")
	}
	if svc != nil {
		t.Error("startup() returned non-nil services on validation failure")
	}

	got := buf.String()
	if !strings.Contains(got, "CLAUDE_CONFIG_DIR") {
		t.Errorf("expected CLAUDE_CONFIG_DIR in output, got: %q", got)
	}
	if !strings.Contains(got, "GITHUB_TOKEN") {
		t.Errorf("expected GITHUB_TOKEN in output, got: %q", got)
	}
	// Fatal count is 1 (CLAUDE_CONFIG_DIR), warning is non-fatal — count must be 1 not 2.
	if !strings.Contains(got, "1 validation error(s)") {
		t.Errorf("expected '1 validation error(s)' in output, got: %q", got)
	}
	if strings.Contains(got, "2 validation error(s)") {
		t.Errorf("count line must not report 2 (non-fatal warning counts as error), got: %q", got)
	}
}

// TestFormatUsageError_ContainsPr9kHelpPointer pins the help-pointer string
// shown to users on any CLI parse failure.
func TestFormatUsageError_ContainsPr9kHelpPointer(t *testing.T) {
	msg := formatUsageError(errors.New("unknown flag: --bad"))
	if !strings.Contains(msg, "error:") {
		t.Errorf("formatUsageError output missing \"error:\": %q", msg)
	}
	if !strings.Contains(msg, "Run 'pr9k --help'") {
		t.Errorf("formatUsageError output missing \"Run 'pr9k --help'\": %q", msg)
	}
}

// TestBuildVersionLabel pins the TUI footer label composition to use "pr9k v".
func TestBuildVersionLabel(t *testing.T) {
	label := buildVersionLabel()
	if !strings.HasPrefix(label, "pr9k v") {
		t.Errorf("buildVersionLabel() should start with \"pr9k v\", got %q", label)
	}
	want := "pr9k v" + version.Version
	if label != want {
		t.Errorf("buildVersionLabel() = %q, want %q", label, want)
	}
}

// TestStartupPreflight_MissingImageErrorReferencesPr9k is a focused sibling of
// TestStartupPreflight_RunsBeforeOrchestrator that names the invariant it pins:
// the missing-image error message must name "pr9k sandbox create".
func TestStartupPreflight_MissingImageErrorReferencesPr9k(t *testing.T) {
	workflowDir := t.TempDir()
	projectDir := t.TempDir()
	profileDir := t.TempDir()
	writeMinimalStepFile(t, workflowDir)

	prober := &fakeProber{
		binaryAvailable: true,
		daemonErr:       nil,
		imagePresent:    false,
	}

	cfg := &cli.Config{WorkflowDir: workflowDir, ProjectDir: projectDir}
	var buf bytes.Buffer
	_, ok := startup(cfg, projectDir, profileDir, prober, &buf)
	if ok {
		t.Fatal("startup() returned ok=true; want false when sandbox image is missing")
	}
	if !strings.Contains(buf.String(), "pr9k sandbox create") {
		t.Errorf("missing-image error does not mention \"pr9k sandbox create\": %q", buf.String())
	}
}
