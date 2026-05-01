package sandbox

import (
	"testing"
)

// TestBuiltinEnvAllowlist_ExactSnapshot verifies that BuiltinEnvAllowlist
// contains exactly the expected variable names in order (TP-003).
// The allowlist controls which host secrets enter the container, so a silent
// addition or removal should be caught by tests and reviewed.
func TestBuiltinEnvAllowlist_ExactSnapshot(t *testing.T) {
	want := []string{
		"ANTHROPIC_BASE_URL",
		"HTTPS_PROXY",
		"HTTP_PROXY",
		"NO_PROXY",
	}

	if len(BuiltinEnvAllowlist) != len(want) {
		t.Fatalf("BuiltinEnvAllowlist has %d entries, want %d: got %v",
			len(BuiltinEnvAllowlist), len(want), BuiltinEnvAllowlist)
	}

	for i, name := range want {
		if BuiltinEnvAllowlist[i] != name {
			t.Errorf("BuiltinEnvAllowlist[%d] = %q, want %q", i, BuiltinEnvAllowlist[i], name)
		}
	}
}

// TestImageConstants_ExactValues verifies that the image and path constants
// have their expected values (TP-004).
// Provides documentation value and catches accidental edits; the golden argv
// test (TestBuildRunArgs_GoldenArgv) covers these indirectly.
func TestImageConstants_ExactValues(t *testing.T) {
	if ImageTag != "docker/sandbox-templates:claude-code" {
		t.Errorf("ImageTag = %q, want %q", ImageTag, "docker/sandbox-templates:claude-code")
	}
	if ContainerRepoPath != "/home/agent/workspace" {
		t.Errorf("ContainerRepoPath = %q, want %q", ContainerRepoPath, "/home/agent/workspace")
	}
	if ContainerProfilePath != "/home/agent/.claude" {
		t.Errorf("ContainerProfilePath = %q, want %q", ContainerProfilePath, "/home/agent/.claude")
	}
}
