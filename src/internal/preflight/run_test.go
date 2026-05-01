package preflight

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// allGreenProber is a Prober that reports everything healthy.
var allGreenProber = fakeProber{
	binaryAvailable: true,
	daemonErr:       nil,
	imagePresent:    true,
}

// missingBinaryProber reports docker binary absent.
var missingBinaryProber = fakeProber{binaryAvailable: false}

func TestRun_ProfileDirMissing(t *testing.T) {
	result := Run(t.TempDir(), "/tmp/ralph-run-test-nonexistent-xyzzy", true, allGreenProber)

	if len(result.Errors) == 0 {
		t.Fatal("expected errors for missing profile dir, got none")
	}
	found := false
	for _, e := range result.Errors {
		if strings.Contains(e.Error(), "claude profile directory not found") {
			found = true
		}
	}
	if !found {
		t.Errorf("expected 'Claude profile directory not found' in errors, got: %v", result.Errors)
	}
	if len(result.Warnings) != 0 {
		t.Errorf("expected no warnings, got: %v", result.Warnings)
	}
}

func TestRun_ProfileDirIsFile(t *testing.T) {
	f, err := os.CreateTemp(t.TempDir(), "profile-file-*")
	if err != nil {
		t.Fatal(err)
	}
	if err := f.Close(); err != nil {
		t.Fatal(err)
	}

	result := Run(t.TempDir(), f.Name(), true, allGreenProber)

	if len(result.Errors) == 0 {
		t.Fatal("expected errors for file-as-profile-dir, got none")
	}
	found := false
	for _, e := range result.Errors {
		if strings.Contains(e.Error(), "claude profile path is not a directory") {
			found = true
		}
	}
	if !found {
		t.Errorf("expected 'not a directory' in errors, got: %v", result.Errors)
	}
	if len(result.Warnings) != 0 {
		t.Errorf("expected no warnings, got: %v", result.Warnings)
	}
}

func TestRun_DockerBinaryMissing(t *testing.T) {
	dir := t.TempDir()
	result := Run(t.TempDir(), dir, true, missingBinaryProber)

	if len(result.Errors) == 0 {
		t.Fatal("expected errors for missing docker binary, got none")
	}
	found := false
	for _, e := range result.Errors {
		if strings.Contains(e.Error(), "docker is not installed") {
			found = true
		}
	}
	if !found {
		t.Errorf("expected 'Docker is not installed' in errors, got: %v", result.Errors)
	}
}

func TestRun_DockerDaemonUnreachable(t *testing.T) {
	dir := t.TempDir()
	p := fakeProber{
		binaryAvailable: true,
		daemonErr:       errors.New("connection refused"),
	}

	result := Run(t.TempDir(), dir, true, p)

	if len(result.Errors) == 0 {
		t.Fatal("expected errors for unreachable daemon, got none")
	}
	found := false
	for _, e := range result.Errors {
		if strings.Contains(e.Error(), "daemon isn't running") {
			found = true
		}
	}
	if !found {
		t.Errorf("expected daemon error in errors, got: %v", result.Errors)
	}
}

func TestRun_ImageNotPresent(t *testing.T) {
	dir := t.TempDir()
	p := fakeProber{
		binaryAvailable: true,
		daemonErr:       nil,
		imagePresent:    false,
	}

	result := Run(t.TempDir(), dir, true, p)

	if len(result.Errors) == 0 {
		t.Fatal("expected errors for missing image, got none")
	}
	found := false
	for _, e := range result.Errors {
		if strings.Contains(e.Error(), "sandbox create") {
			found = true
		}
	}
	if !found {
		t.Errorf("expected 'sandbox create' in errors, got: %v", result.Errors)
	}
}

func TestRun_AllGreen(t *testing.T) {
	dir := t.TempDir()

	result := Run(t.TempDir(), dir, true, allGreenProber)

	if len(result.Errors) != 0 {
		t.Errorf("expected no errors, got: %v", result.Errors)
	}
	if len(result.Warnings) != 0 {
		t.Errorf("expected no warnings, got: %v", result.Warnings)
	}
}

func TestRun_CollectsAllErrors_ProfileAndDocker(t *testing.T) {
	// Profile dir missing AND docker binary missing → 2 errors.
	result := Run(t.TempDir(), "/tmp/ralph-run-test-nonexistent-xyzzy2", true, missingBinaryProber)

	if len(result.Errors) < 2 {
		t.Errorf("expected at least 2 errors (profile + docker), got %d: %v", len(result.Errors), result.Errors)
	}

	hasProfile := false
	hasDocker := false
	for _, e := range result.Errors {
		if strings.Contains(e.Error(), "claude profile directory not found") {
			hasProfile = true
		}
		if strings.Contains(e.Error(), "docker is not installed") {
			hasDocker = true
		}
	}
	if !hasProfile {
		t.Errorf("missing profile error in: %v", result.Errors)
	}
	if !hasDocker {
		t.Errorf("missing docker error in: %v", result.Errors)
	}
}

// TestRun_NoClaudeSteps_SkipsProfileAndDockerChecks verifies that when
// hasClaudeSteps is false, neither CheckProfileDir nor CheckDocker runs:
// a missing profile dir and a missing docker binary both produce zero errors,
// so a non-claude workflow can run on a fresh host with neither prerequisite.
func TestRun_NoClaudeSteps_SkipsProfileAndDockerChecks(t *testing.T) {
	result := Run(t.TempDir(), "/tmp/ralph-no-such-profile-xyzzy", false, missingBinaryProber)

	if len(result.Errors) != 0 {
		t.Errorf("expected no errors when hasClaudeSteps=false, got: %v", result.Errors)
	}
	if len(result.Warnings) != 0 {
		t.Errorf("expected no warnings, got: %v", result.Warnings)
	}
}

// TestRun_Pr9kDir_CreatedOnFirstRun verifies that Run creates .pr9k/ inside
// projectDir if it does not already exist.
func TestRun_Pr9kDir_CreatedOnFirstRun(t *testing.T) {
	projectDir := t.TempDir()
	profileDir := t.TempDir()

	_ = Run(projectDir, profileDir, true, allGreenProber)

	pr9kDir := filepath.Join(projectDir, ".pr9k")
	info, err := os.Stat(pr9kDir)
	if err != nil {
		t.Fatalf(".pr9k was not created: %v", err)
	}
	if !info.IsDir() {
		t.Errorf(".pr9k is not a directory")
	}
}

// TestRun_Pr9kDir_IdempotentOnRepeatRun verifies that calling Run twice does
// not return an error if .pr9k already exists.
func TestRun_Pr9kDir_IdempotentOnRepeatRun(t *testing.T) {
	projectDir := t.TempDir()
	profileDir := t.TempDir()

	r1 := Run(projectDir, profileDir, true, allGreenProber)
	r2 := Run(projectDir, profileDir, true, allGreenProber)

	for _, result := range []Result{r1, r2} {
		for _, e := range result.Errors {
			if strings.Contains(e.Error(), ".pr9k") {
				t.Errorf("unexpected .pr9k error on repeat run: %v", e)
			}
		}
	}
}

// TestRun_Pr9kDir_FileClashSurfacesError verifies that when a regular file
// already exists at .pr9k (e.g. a git-tracked placeholder), Run surfaces an
// error with the expected prefix and .pr9k in the message.
func TestRun_Pr9kDir_FileClashSurfacesError(t *testing.T) {
	projectDir := t.TempDir()
	profileDir := t.TempDir()

	pr9kPath := filepath.Join(projectDir, ".pr9k")
	if err := os.WriteFile(pr9kPath, []byte("placeholder"), 0644); err != nil {
		t.Fatal(err)
	}

	result := Run(projectDir, profileDir, true, allGreenProber)

	var pr9kErr error
	for _, e := range result.Errors {
		if strings.Contains(e.Error(), ".pr9k") {
			pr9kErr = e
			break
		}
	}
	if pr9kErr == nil {
		t.Fatalf("expected a .pr9k error when it exists as a regular file; got: %v", result.Errors)
	}
	if !strings.Contains(pr9kErr.Error(), "preflight: could not create .pr9k in") {
		t.Errorf("error message has wrong format: %q", pr9kErr.Error())
	}
}

// TestRun_Pr9kDir_ReadOnlyProjectDirSurfacesError verifies that a projectDir
// that cannot be written to surfaces a preflight error for .pr9k creation.
func TestRun_Pr9kDir_ReadOnlyProjectDirSurfacesError(t *testing.T) {
	if os.Getuid() == 0 {
		t.Skip("requires non-root: root bypasses permission checks")
	}

	parent := t.TempDir()
	projectDir := filepath.Join(parent, "readonly-project")
	if err := os.Mkdir(projectDir, 0o555); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chmod(projectDir, 0o755) })

	profileDir := t.TempDir()
	result := Run(projectDir, profileDir, true, allGreenProber)

	found := false
	for _, e := range result.Errors {
		if strings.Contains(e.Error(), ".pr9k") {
			found = true
		}
	}
	if !found {
		t.Errorf("expected a .pr9k error for read-only projectDir; got: %v", result.Errors)
	}
}

// TestRun_CollectsAllErrors_Pr9kProfileDocker documents the collect-all
// design: a .pr9k creation failure does not short-circuit the profile or
// docker checks — all three errors surface before Run returns.
func TestRun_CollectsAllErrors_Pr9kProfileDocker(t *testing.T) {
	if os.Getuid() == 0 {
		t.Skip("requires non-root: root bypasses permission checks")
	}

	parent := t.TempDir()
	projectDir := filepath.Join(parent, "readonly-project")
	if err := os.Mkdir(projectDir, 0o555); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chmod(projectDir, 0o755) })

	result := Run(projectDir, "/tmp/ralph-nonexistent-profile-xyzzy11", true, missingBinaryProber)

	hasPr9k := false
	hasProfile := false
	hasDocker := false
	for _, e := range result.Errors {
		if strings.Contains(e.Error(), ".pr9k") {
			hasPr9k = true
		}
		if strings.Contains(e.Error(), "claude profile directory") {
			hasProfile = true
		}
		if strings.Contains(e.Error(), "docker is not installed") {
			hasDocker = true
		}
	}
	if !hasPr9k {
		t.Errorf("expected .pr9k error in: %v", result.Errors)
	}
	if !hasProfile {
		t.Errorf("expected profile-dir error in: %v", result.Errors)
	}
	if !hasDocker {
		t.Errorf("expected docker error in: %v", result.Errors)
	}
}
