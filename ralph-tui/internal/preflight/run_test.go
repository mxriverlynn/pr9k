package preflight

import (
	"errors"
	"io/fs"
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
	result := Run("/tmp/ralph-run-test-nonexistent-xyzzy", allGreenProber)

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
		t.Errorf("expected no warnings when profile dir is missing (CheckCredentials should be gated), got: %v", result.Warnings)
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

	result := Run(f.Name(), allGreenProber)

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
		t.Errorf("expected no warnings when profile path is a file (CheckCredentials should be gated), got: %v", result.Warnings)
	}
}

func TestRun_DockerBinaryMissing(t *testing.T) {
	t.Setenv("ANTHROPIC_API_KEY", "sk-test-iso")
	dir := t.TempDir()
	result := Run(dir, missingBinaryProber)

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
	t.Setenv("ANTHROPIC_API_KEY", "sk-test-iso")
	dir := t.TempDir()
	p := fakeProber{
		binaryAvailable: true,
		daemonErr:       errors.New("connection refused"),
	}

	result := Run(dir, p)

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
	t.Setenv("ANTHROPIC_API_KEY", "sk-test-iso")
	dir := t.TempDir()
	p := fakeProber{
		binaryAvailable: true,
		daemonErr:       nil,
		imagePresent:    false,
	}

	result := Run(dir, p)

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

func TestRun_ZeroByteCredentials_WarningNotFatal(t *testing.T) {
	t.Setenv("ANTHROPIC_API_KEY", "")
	dir := t.TempDir()
	path := filepath.Join(dir, ".credentials.json")
	if err := os.WriteFile(path, []byte{}, 0600); err != nil {
		t.Fatal(err)
	}

	result := Run(dir, allGreenProber)

	if len(result.Errors) != 0 {
		t.Errorf("expected no errors for zero-byte credentials, got: %v", result.Errors)
	}
	if len(result.Warnings) == 0 {
		t.Fatal("expected a warning for zero-byte credentials, got none")
	}
	if !strings.Contains(result.Warnings[0], "will likely fail authentication") {
		t.Errorf("unexpected warning text: %q", result.Warnings[0])
	}
}

// TP-002: Run collects a CheckCredentials error (permission-denied) into Result.Errors.
func TestRun_CredentialsPermissionError_CollectedAsError(t *testing.T) {
	if os.Getuid() == 0 {
		t.Skip("requires non-root: root bypasses permission checks")
	}
	t.Setenv("ANTHROPIC_API_KEY", "")

	parent := t.TempDir()
	dir := filepath.Join(parent, "profile")
	if err := os.Mkdir(dir, 0700); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(dir, ".credentials.json")
	if err := os.WriteFile(path, []byte(`{"token":"abc"}`), 0600); err != nil {
		t.Fatal(err)
	}
	// Revoke execute permission on dir so os.Stat(dir/.credentials.json) fails.
	if err := os.Chmod(dir, 0000); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_ = os.Chmod(dir, 0700)
	})

	result := Run(dir, allGreenProber)

	if len(result.Errors) == 0 {
		t.Fatal("expected at least 1 error for permission-denied credentials, got none")
	}
	if len(result.Warnings) != 0 {
		t.Errorf("expected no warnings, got: %v", result.Warnings)
	}
	found := false
	for _, e := range result.Errors {
		if errors.Is(e, fs.ErrPermission) {
			found = true
		}
	}
	if !found {
		t.Errorf("expected a permission error in result.Errors, got: %v", result.Errors)
	}
}

func TestRun_AllGreen(t *testing.T) {
	t.Setenv("ANTHROPIC_API_KEY", "")
	dir := t.TempDir()
	path := filepath.Join(dir, ".credentials.json")
	if err := os.WriteFile(path, []byte(`{"token":"abc"}`), 0600); err != nil {
		t.Fatal(err)
	}

	result := Run(dir, allGreenProber)

	if len(result.Errors) != 0 {
		t.Errorf("expected no errors, got: %v", result.Errors)
	}
	if len(result.Warnings) != 0 {
		t.Errorf("expected no warnings, got: %v", result.Warnings)
	}
}

func TestRun_MissingCredentials_EmitsWarning(t *testing.T) {
	t.Setenv("ANTHROPIC_API_KEY", "")
	dir := t.TempDir()

	result := Run(dir, allGreenProber)

	if len(result.Errors) != 0 {
		t.Errorf("expected no errors, got: %v", result.Errors)
	}
	if len(result.Warnings) != 1 {
		t.Fatalf("expected exactly 1 warning for missing credentials, got %d: %v", len(result.Warnings), result.Warnings)
	}
	for _, want := range []string{"does not exist", "sandbox login", "ANTHROPIC_API_KEY"} {
		if !strings.Contains(result.Warnings[0], want) {
			t.Errorf("warning %q does not contain %q", result.Warnings[0], want)
		}
	}
}

func TestRun_MissingProfileDir_NoCredentialsWarning(t *testing.T) {
	t.Setenv("ANTHROPIC_API_KEY", "")

	result := Run("/tmp/ralph-run-test-nonexistent-credgate-xyzzy", allGreenProber)

	hasProfileErr := false
	for _, e := range result.Errors {
		if strings.Contains(e.Error(), "claude profile directory not found") {
			hasProfileErr = true
		}
	}
	if !hasProfileErr {
		t.Errorf("expected profile-not-found error, got: %v", result.Errors)
	}
	if len(result.Warnings) != 0 {
		t.Errorf("expected no credentials warning when profile dir is missing (gating), got: %v", result.Warnings)
	}
}

func TestRun_CollectsAllErrors_ProfileAndDocker(t *testing.T) {
	// Profile dir missing AND docker binary missing → 2 errors.
	result := Run("/tmp/ralph-run-test-nonexistent-xyzzy2", missingBinaryProber)

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
