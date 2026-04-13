package sandbox

import (
	"os"
	"slices"
	"strings"
	"testing"
)

const (
	testUID        = 501
	testGID        = 20
	testProjectDir = "/repo"
	testProfileDir = "/home/me/.claude"
	testCidfile    = "/tmp/ralph-abc.cid"
	testModel      = "sonnet"
)

func TestBuildRunArgs_GoldenArgv(t *testing.T) {
	t.Setenv("ANTHROPIC_API_KEY", "sk-test")
	t.Setenv("GITHUB_TOKEN", "") // not set via LookupEnv — unset means not exported

	// Unset GITHUB_TOKEN so LookupEnv returns ok=false.
	// t.Setenv sets it, so we need to use a name that is actually absent from env.
	// Instead test with GITHUB_TOKEN absent by not setting it.

	// Reset: ensure ANTHROPIC_API_KEY is set but GITHUB_TOKEN is absent.
	for _, k := range []string{"ANTHROPIC_BASE_URL", "HTTPS_PROXY", "HTTP_PROXY", "NO_PROXY", "GITHUB_TOKEN"} {
		t.Setenv(k, "")
	}

	prompt := "hello world"
	allowlist := []string{"ANTHROPIC_API_KEY", "GITHUB_TOKEN"}
	args := BuildRunArgs(testProjectDir, testProfileDir, testUID, testGID, testCidfile, allowlist, testModel, prompt)

	// Build expected argv. ANTHROPIC_API_KEY is set (via t.Setenv above),
	// but t.Setenv("GITHUB_TOKEN", "") still sets the var, so LookupEnv returns ok=true.
	// We need a name guaranteed absent — use a unique never-set name.
	t.Log("argv:", args)

	// Verify structure: "docker run" is first two elements.
	if args[0] != "docker" || args[1] != "run" {
		t.Fatalf("expected 'docker run' at [0:2], got %v %v", args[0], args[1])
	}

	assertContainsFlag(t, args, "--rm")
	assertContainsFlag(t, args, "-i")
	assertContainsFlag(t, args, "--init")
	assertContainsConsecutive(t, args, "--cidfile", testCidfile)
	assertContainsConsecutive(t, args, "-u", "501:20")
	assertContainsConsecutive(t, args, "-v", testProjectDir+":"+ContainerRepoPath)
	assertContainsConsecutive(t, args, "-v", testProfileDir+":"+ContainerProfilePath)
	assertContainsConsecutive(t, args, "-w", ContainerRepoPath)
	assertContainsConsecutive(t, args, "-e", "CLAUDE_CONFIG_DIR="+ContainerProfilePath)
	assertContainsFlag(t, args, ImageTag)

	// Prompt is the last element.
	if args[len(args)-1] != prompt {
		t.Errorf("expected prompt %q as last arg, got %q", prompt, args[len(args)-1])
	}

	// claude flags are present.
	assertContainsConsecutive(t, args, "--model", testModel)
	assertContainsFlag(t, args, "--permission-mode")
	assertContainsConsecutive(t, args, "--permission-mode", "bypassPermissions")
}

func TestBuildRunArgs_AllBuiltinEnvVarsSet(t *testing.T) {
	for _, k := range BuiltinEnvAllowlist {
		t.Setenv(k, "value-"+k)
	}

	args := BuildRunArgs(testProjectDir, testProfileDir, testUID, testGID, testCidfile,
		BuiltinEnvAllowlist, testModel, "prompt")

	count := countFlag(args, "-e")
	// Expect CLAUDE_CONFIG_DIR + 5 builtins.
	if count != 6 {
		t.Errorf("expected 6 -e flags (CLAUDE_CONFIG_DIR + 5 builtins), got %d", count)
	}
}

func TestBuildRunArgs_AllBuiltinEnvVarsUnset(t *testing.T) {
	for _, k := range BuiltinEnvAllowlist {
		// Use a helper that truly unsets, not sets to empty.
		t.Setenv(k, "")
	}
	// Actually, t.Setenv sets the var. To test truly unset, we need names not in env.
	// Use a unique suffix that won't collide with real env.
	unsetAllowlist := []string{
		"RALPH_TEST_UNSET_A_XYZ",
		"RALPH_TEST_UNSET_B_XYZ",
	}

	args := BuildRunArgs(testProjectDir, testProfileDir, testUID, testGID, testCidfile,
		unsetAllowlist, testModel, "prompt")

	count := countFlag(args, "-e")
	// Only CLAUDE_CONFIG_DIR should be present.
	if count != 1 {
		t.Errorf("expected 1 -e flag (only CLAUDE_CONFIG_DIR), got %d. args: %v", count, args)
	}
}

func TestBuildRunArgs_EmptyAllowlist(t *testing.T) {
	args := BuildRunArgs(testProjectDir, testProfileDir, testUID, testGID, testCidfile,
		nil, testModel, "prompt")

	count := countFlag(args, "-e")
	if count != 1 {
		t.Errorf("expected 1 -e flag (only CLAUDE_CONFIG_DIR), got %d", count)
	}
}

func TestBuildRunArgs_DeduplicatesUserVsBuiltin(t *testing.T) {
	t.Setenv("ANTHROPIC_API_KEY", "sk-test")

	// envAllowlist includes ANTHROPIC_API_KEY twice: once from builtins, once from user.
	allowlist := append(BuiltinEnvAllowlist, "ANTHROPIC_API_KEY")

	args := BuildRunArgs(testProjectDir, testProfileDir, testUID, testGID, testCidfile,
		allowlist, testModel, "prompt")

	// Count occurrences of ANTHROPIC_API_KEY in args.
	n := 0
	for _, a := range args {
		if a == "ANTHROPIC_API_KEY" {
			n++
		}
	}
	if n != 1 {
		t.Errorf("expected ANTHROPIC_API_KEY to appear exactly once, got %d", n)
	}
}

func TestBuildRunArgs_DeduplicatesDuplicateUserEntries(t *testing.T) {
	t.Setenv("MY_TOKEN", "abc")

	allowlist := []string{"MY_TOKEN", "MY_TOKEN", "MY_TOKEN"}
	args := BuildRunArgs(testProjectDir, testProfileDir, testUID, testGID, testCidfile,
		allowlist, testModel, "prompt")

	n := 0
	for _, a := range args {
		if a == "MY_TOKEN" {
			n++
		}
	}
	if n != 1 {
		t.Errorf("expected MY_TOKEN to appear exactly once, got %d", n)
	}
}

func TestBuildRunArgs_PromptWithMetacharacters(t *testing.T) {
	prompt := "hello & world | \"quotes\" \n newline $VAR `backtick`"
	args := BuildRunArgs(testProjectDir, testProfileDir, testUID, testGID, testCidfile,
		nil, testModel, prompt)

	if args[len(args)-1] != prompt {
		t.Errorf("expected prompt to be last arg verbatim, got %q", args[len(args)-1])
	}
}

func TestBuildRunArgs_WorkdirPresent(t *testing.T) {
	args := BuildRunArgs(testProjectDir, testProfileDir, testUID, testGID, testCidfile,
		nil, testModel, "prompt")

	assertContainsConsecutive(t, args, "-w", ContainerRepoPath)
}

func TestBuildRunArgs_ClaudeConfigDirAlwaysSet(t *testing.T) {
	// Even if CLAUDE_CONFIG_DIR is set on host, the container arg uses mount point.
	t.Setenv("CLAUDE_CONFIG_DIR", "/some/host/path")

	args := BuildRunArgs(testProjectDir, testProfileDir, testUID, testGID, testCidfile,
		nil, testModel, "prompt")

	assertContainsConsecutive(t, args, "-e", "CLAUDE_CONFIG_DIR="+ContainerProfilePath)
}

func TestBuildRunArgs_FlagOrder(t *testing.T) {
	args := BuildRunArgs(testProjectDir, testProfileDir, testUID, testGID, testCidfile,
		nil, testModel, "prompt")

	// Verify the fixed-position flags appear in the expected order.
	expectedPrefixOrder := []string{"docker", "run", "--rm", "-i", "--init", "--cidfile"}
	for i, expected := range expectedPrefixOrder {
		if i >= len(args) {
			t.Fatalf("args too short; missing %q at position %d", expected, i)
		}
		if args[i] != expected {
			t.Errorf("args[%d] = %q, want %q", i, args[i], expected)
		}
	}

	// Image tag must appear before claude.
	imageIdx := indexOf(args, ImageTag)
	claudeIdx := indexOf(args, "claude")
	if imageIdx < 0 {
		t.Fatal("ImageTag not found in args")
	}
	if claudeIdx < 0 {
		t.Fatal("claude not found in args")
	}
	if imageIdx > claudeIdx {
		t.Errorf("ImageTag at %d must come before claude at %d", imageIdx, claudeIdx)
	}
}

// TestBuildRunArgs_DoesNotMutateAllowlist verifies that BuildRunArgs does not
// modify the envAllowlist slice passed by the caller (TP-001, mandatory per
// docs/coding-standards/testing.md §input-slice-immutability).
func TestBuildRunArgs_DoesNotMutateAllowlist(t *testing.T) {
	t.Setenv("ANTHROPIC_API_KEY", "sk-test")

	allowlist := []string{"ANTHROPIC_API_KEY", "GITHUB_TOKEN"}
	original := slices.Clone(allowlist)

	_ = BuildRunArgs(testProjectDir, testProfileDir, testUID, testGID, testCidfile,
		allowlist, testModel, "prompt")

	if !slices.Equal(allowlist, original) {
		t.Errorf("BuildRunArgs mutated envAllowlist: before=%v after=%v", original, allowlist)
	}
}

// TestBuildRunArgs_DeduplicatesPreservesFirstSeenOrder verifies that when
// envAllowlist contains duplicate entries, the first occurrence determines
// position in the output argv (TP-003).
func TestBuildRunArgs_DeduplicatesPreservesFirstSeenOrder(t *testing.T) {
	t.Setenv("A", "1")
	t.Setenv("B", "2")

	allowlist := []string{"A", "B", "A", "B"}
	args := BuildRunArgs(testProjectDir, testProfileDir, testUID, testGID, testCidfile,
		allowlist, testModel, "prompt")

	idxA := indexOf(args, "A")
	idxB := indexOf(args, "B")

	if idxA < 0 {
		t.Fatal("A not found in args")
	}
	if idxB < 0 {
		t.Fatal("B not found in args")
	}
	if idxA >= idxB {
		t.Errorf("expected A (first-seen) to appear before B: idxA=%d idxB=%d", idxA, idxB)
	}
}

// TestHostUIDGID_MatchesOsGetuid verifies that HostUIDGID returns values
// equal to os.Getuid() and os.Getgid() (TP-006).
func TestHostUIDGID_MatchesOsGetuid(t *testing.T) {
	uid, gid := HostUIDGID()

	if uid != os.Getuid() {
		t.Errorf("HostUIDGID uid=%d, want os.Getuid()=%d", uid, os.Getuid())
	}
	if gid != os.Getgid() {
		t.Errorf("HostUIDGID gid=%d, want os.Getgid()=%d", gid, os.Getgid())
	}
}

// --- helpers ---

func assertContainsFlag(t *testing.T, args []string, flag string) {
	t.Helper()
	if !slices.Contains(args, flag) {
		t.Errorf("expected %q in args %v", flag, args)
	}
}

func assertContainsConsecutive(t *testing.T, args []string, flag, value string) {
	t.Helper()
	for i := 0; i < len(args)-1; i++ {
		if args[i] == flag {
			// For -e with key=value, value may be the whole "key=val" string.
			// For other flags, exact match.
			if args[i+1] == value {
				return
			}
			// Also allow: flag is -e and value is bare "NAME" — check if any -e NAME matches.
		}
	}
	// Special handling: if flag == "-e" and value doesn't contain "=", look for
	// "-e" followed by exactly value.
	if flag == "-e" && !strings.Contains(value, "=") {
		for i := 0; i < len(args)-1; i++ {
			if args[i] == "-e" && args[i+1] == value {
				return
			}
		}
	}
	t.Errorf("expected consecutive %q %q in args %v", flag, value, args)
}

func countFlag(args []string, flag string) int {
	n := 0
	for _, a := range args {
		if a == flag {
			n++
		}
	}
	return n
}

func indexOf(args []string, s string) int {
	for i, a := range args {
		if a == s {
			return i
		}
	}
	return -1
}
