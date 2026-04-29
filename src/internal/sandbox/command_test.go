package sandbox

import (
	"fmt"
	"maps"
	"os"
	"reflect"
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
	args := BuildRunArgs(testProjectDir, testProfileDir, testUID, testGID, testCidfile, allowlist, nil, "", testModel, prompt)

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
	assertContainsConsecutive(t, args, "--mount", fmt.Sprintf("type=bind,source=%s,target=%s", testProjectDir, ContainerRepoPath))
	assertContainsConsecutive(t, args, "--mount", fmt.Sprintf("type=bind,source=%s,target=%s", testProfileDir, ContainerProfilePath))
	assertContainsConsecutive(t, args, "-w", ContainerRepoPath)
	assertContainsConsecutive(t, args, "-e", "CLAUDE_CONFIG_DIR="+ContainerProfilePath)
	assertContainsFlag(t, args, ImageTag)

	// claude flags are present.
	assertContainsConsecutive(t, args, "--model", testModel)
	assertContainsFlag(t, args, "--permission-mode")
	assertContainsConsecutive(t, args, "--permission-mode", "bypassPermissions")
	assertContainsConsecutive(t, args, "-p", prompt)
	assertContainsConsecutive(t, args, "--output-format", "stream-json")
	assertContainsFlag(t, args, "--verbose")
}

func TestBuildRunArgs_AllBuiltinEnvVarsSet(t *testing.T) {
	for _, k := range BuiltinEnvAllowlist {
		t.Setenv(k, "value-"+k)
	}

	args := BuildRunArgs(testProjectDir, testProfileDir, testUID, testGID, testCidfile,
		BuiltinEnvAllowlist, nil, "", testModel, "prompt")

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
		unsetAllowlist, nil, "", testModel, "prompt")

	count := countFlag(args, "-e")
	// Only CLAUDE_CONFIG_DIR should be present.
	if count != 1 {
		t.Errorf("expected 1 -e flag (only CLAUDE_CONFIG_DIR), got %d. args: %v", count, args)
	}
}

func TestBuildRunArgs_EmptyAllowlist(t *testing.T) {
	args := BuildRunArgs(testProjectDir, testProfileDir, testUID, testGID, testCidfile,
		nil, nil, "", testModel, "prompt")

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
		allowlist, nil, "", testModel, "prompt")

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
		allowlist, nil, "", testModel, "prompt")

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
		nil, nil, "", testModel, prompt)

	assertContainsConsecutive(t, args, "-p", prompt)
}

func TestBuildRunArgs_WorkdirPresent(t *testing.T) {
	args := BuildRunArgs(testProjectDir, testProfileDir, testUID, testGID, testCidfile,
		nil, nil, "", testModel, "prompt")

	assertContainsConsecutive(t, args, "-w", ContainerRepoPath)
}

func TestBuildRunArgs_ClaudeConfigDirAlwaysSet(t *testing.T) {
	// Even if CLAUDE_CONFIG_DIR is set on host, the container arg uses mount point.
	t.Setenv("CLAUDE_CONFIG_DIR", "/some/host/path")

	args := BuildRunArgs(testProjectDir, testProfileDir, testUID, testGID, testCidfile,
		nil, nil, "", testModel, "prompt")

	assertContainsConsecutive(t, args, "-e", "CLAUDE_CONFIG_DIR="+ContainerProfilePath)
}

func TestBuildRunArgs_FlagOrder(t *testing.T) {
	args := BuildRunArgs(testProjectDir, testProfileDir, testUID, testGID, testCidfile,
		nil, nil, "", testModel, "prompt")

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

// TestBuildRunArgs_EnvVarsEmittedAsBareNames verifies that env vars in the
// allowlist are emitted as bare names ("-e MY_VAR"), not as key=value pairs
// ("-e MY_VAR=secret") (TP-002).
// Security-relevant: bare names cause the container runtime to inherit the
// value from the host environment at startup, keeping secrets out of argv
// (and therefore out of /proc/*/cmdline).
func TestBuildRunArgs_EnvVarsEmittedAsBareNames(t *testing.T) {
	t.Setenv("MY_VAR", "secret")

	args := BuildRunArgs(testProjectDir, testProfileDir, testUID, testGID, testCidfile,
		[]string{"MY_VAR"}, nil, "", testModel, "prompt")

	// Find the -e flag that corresponds to MY_VAR.
	for i := 0; i < len(args)-1; i++ {
		if args[i] == "-e" && args[i+1] == "MY_VAR" {
			return // found: bare name, not key=value
		}
	}
	// Check if it was emitted as key=value (the wrong form).
	for i := 0; i < len(args)-1; i++ {
		if args[i] == "-e" && strings.HasPrefix(args[i+1], "MY_VAR=") {
			t.Errorf("env var emitted as key=value %q; want bare name %q", args[i+1], "MY_VAR")
			return
		}
	}
	t.Errorf("MY_VAR not found in args following -e: %v", args)
}

// TestBuildRunArgs_MixedSetUnsetEnvVarsInterleaved verifies that when the
// allowlist contains a mix of set and unset vars in interleaved order, unset
// vars are silently skipped and the set vars appear in first-seen order.
func TestBuildRunArgs_MixedSetUnsetEnvVarsInterleaved(t *testing.T) {
	t.Setenv("RALPH_TEST_SET_A", "val-a")
	t.Setenv("RALPH_TEST_SET_B", "val-b")
	// RALPH_TEST_UNSET_XYZ is guaranteed absent from the environment.

	allowlist := []string{"RALPH_TEST_SET_A", "RALPH_TEST_UNSET_XYZ", "RALPH_TEST_SET_B"}
	args := BuildRunArgs(testProjectDir, testProfileDir, testUID, testGID, testCidfile,
		allowlist, nil, "", testModel, "prompt")

	// CLAUDE_CONFIG_DIR + 2 set vars = 3 -e flags total.
	count := countFlag(args, "-e")
	if count != 3 {
		t.Errorf("expected 3 -e flags (CLAUDE_CONFIG_DIR + 2 set vars), got %d. args: %v", count, args)
	}

	// Unset var must not appear.
	for _, a := range args {
		if a == "RALPH_TEST_UNSET_XYZ" {
			t.Errorf("unset var RALPH_TEST_UNSET_XYZ must not appear in args: %v", args)
		}
	}

	// Set vars appear in first-seen order: A before B.
	idxA := indexOf(args, "RALPH_TEST_SET_A")
	idxB := indexOf(args, "RALPH_TEST_SET_B")
	if idxA < 0 {
		t.Fatal("RALPH_TEST_SET_A not found in args")
	}
	if idxB < 0 {
		t.Fatal("RALPH_TEST_SET_B not found in args")
	}
	if idxA >= idxB {
		t.Errorf("expected RALPH_TEST_SET_A before RALPH_TEST_SET_B: idxA=%d idxB=%d", idxA, idxB)
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
		allowlist, nil, "", testModel, "prompt")

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
		allowlist, nil, "", testModel, "prompt")

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

// TestBuildRunArgs_ContainerEnv_SortedKeyOrder verifies that containerEnv entries
// are rendered as -e KEY=VALUE pairs in sorted key order for deterministic argv.
func TestBuildRunArgs_ContainerEnv_SortedKeyOrder(t *testing.T) {
	containerEnv := map[string]string{
		"ZEBRA": "z",
		"ALPHA": "a",
		"BETA":  "b",
	}
	args := BuildRunArgs(testProjectDir, testProfileDir, testUID, testGID, testCidfile,
		nil, containerEnv, "", testModel, "prompt")

	// Collect all -e KEY=VALUE pairs for containerEnv keys.
	var pairs []string
	for i := 0; i < len(args)-1; i++ {
		if args[i] == "-e" && strings.Contains(args[i+1], "=") {
			// Exclude CLAUDE_CONFIG_DIR (always present).
			if !strings.HasPrefix(args[i+1], "CLAUDE_CONFIG_DIR=") {
				pairs = append(pairs, args[i+1])
			}
		}
	}
	want := []string{"ALPHA=a", "BETA=b", "ZEBRA=z"}
	if len(pairs) != len(want) {
		t.Fatalf("expected %d containerEnv pairs, got %d: %v", len(want), len(pairs), pairs)
	}
	for i, w := range want {
		if pairs[i] != w {
			t.Errorf("pairs[%d] = %q, want %q", i, pairs[i], w)
		}
	}
}

// TestBuildRunArgs_ContainerEnv_NilOrEmptyIsNoop verifies that a nil or empty
// containerEnv map adds no extra -e flags.
func TestBuildRunArgs_ContainerEnv_NilOrEmptyIsNoop(t *testing.T) {
	argsNil := BuildRunArgs(testProjectDir, testProfileDir, testUID, testGID, testCidfile,
		nil, nil, "", testModel, "prompt")
	argsEmpty := BuildRunArgs(testProjectDir, testProfileDir, testUID, testGID, testCidfile,
		nil, map[string]string{}, "", testModel, "prompt")

	if countFlag(argsNil, "-e") != 1 {
		t.Errorf("nil containerEnv: expected 1 -e flag (CLAUDE_CONFIG_DIR only), got %d", countFlag(argsNil, "-e"))
	}
	if countFlag(argsEmpty, "-e") != 1 {
		t.Errorf("empty containerEnv: expected 1 -e flag (CLAUDE_CONFIG_DIR only), got %d", countFlag(argsEmpty, "-e"))
	}
}

// TestBuildRunArgs_ContainerEnv_AppearsAfterAllowlist verifies that containerEnv
// entries are emitted AFTER the envAllowlist entries so Docker's last-wins
// semantics mean containerEnv takes precedence on a name collision.
func TestBuildRunArgs_ContainerEnv_AppearsAfterAllowlist(t *testing.T) {
	t.Setenv("MY_VAR", "host-value")

	containerEnv := map[string]string{"MY_VAR": "container-value"}
	args := BuildRunArgs(testProjectDir, testProfileDir, testUID, testGID, testCidfile,
		[]string{"MY_VAR"}, containerEnv, "", testModel, "prompt")

	// Find the index of the bare "-e MY_VAR" (from allowlist) and the
	// "-e MY_VAR=container-value" (from containerEnv).
	bareIdx := -1
	kvIdx := -1
	for i := 0; i < len(args)-1; i++ {
		if args[i] == "-e" {
			if args[i+1] == "MY_VAR" {
				bareIdx = i
			}
			if args[i+1] == "MY_VAR=container-value" {
				kvIdx = i
			}
		}
	}
	if bareIdx < 0 {
		t.Fatal("bare -e MY_VAR (allowlist) not found in args")
	}
	if kvIdx < 0 {
		t.Fatal("-e MY_VAR=container-value (containerEnv) not found in args")
	}
	if bareIdx >= kvIdx {
		t.Errorf("allowlist entry (idx %d) must appear before containerEnv entry (idx %d) for Docker last-wins", bareIdx, kvIdx)
	}
}

// TestBuildRunArgs_ContainerEnv_ValueWithEqualsPassesThrough verifies that a
// containerEnv value containing "=" (e.g. FOO=bar=baz) is passed through verbatim.
func TestBuildRunArgs_ContainerEnv_ValueWithEqualsPassesThrough(t *testing.T) {
	containerEnv := map[string]string{"FOO": "bar=baz"}
	args := BuildRunArgs(testProjectDir, testProfileDir, testUID, testGID, testCidfile,
		nil, containerEnv, "", testModel, "prompt")

	assertContainsConsecutive(t, args, "-e", "FOO=bar=baz")
}

func TestBuildInteractiveArgs_Shape(t *testing.T) {
	args := BuildInteractiveArgs(testProfileDir, testUID, testGID)

	wantFlags := []string{"-it", "--rm", "--init"}
	for _, flag := range wantFlags {
		if indexOf(args, flag) < 0 {
			t.Errorf("argv missing %q; got %v", flag, args)
		}
	}

	uidArg := fmt.Sprintf("%d:%d", testUID, testGID)
	uidIdx := indexOf(args, "-u")
	if uidIdx < 0 || uidIdx+1 >= len(args) || args[uidIdx+1] != uidArg {
		t.Errorf("expected -u %s; got %v", uidArg, args)
	}

	mountSpec := fmt.Sprintf("type=bind,source=%s,target=%s", testProfileDir, ContainerProfilePath)
	mountCount := countFlag(args, "--mount")
	if mountCount != 1 {
		t.Errorf("expected exactly 1 --mount (profile only, no project); got %d", mountCount)
	}
	if indexOf(args, mountSpec) < 0 {
		t.Errorf("argv missing profile mount %q; got %v", mountSpec, args)
	}

	envSpec := "CLAUDE_CONFIG_DIR=" + ContainerProfilePath
	if indexOf(args, envSpec) < 0 {
		t.Errorf("argv missing %q; got %v", envSpec, args)
	}

	// Must end with ImageTag then "claude" with nothing after (no -p prompt trailer).
	if args[len(args)-2] != ImageTag || args[len(args)-1] != "claude" {
		t.Errorf("argv must end with [ImageTag, 'claude']; got tail %v", args[len(args)-3:])
	}

	// Must NOT contain -p, -w, --cidfile, --permission-mode (create-only plumbing).
	forbidden := []string{"-p", "-w", "--cidfile", "--permission-mode"}
	for _, f := range forbidden {
		if indexOf(args, f) >= 0 {
			t.Errorf("argv must NOT contain %q; got %v", f, args)
		}
	}

	// Must NOT contain a project-dir mount (login is auth-only).
	if strings.Contains(strings.Join(args, " "), "source="+testProjectDir) {
		t.Errorf("argv must NOT mount project dir %q; got %v", testProjectDir, args)
	}
}

// TestBuildInteractiveArgs_ForwardsTERMWhenSet asserts that BuildInteractiveArgs
// adds `-e TERM` (name only, value inherited by docker) when the host has TERM
// set. Without TERM forwarded, the container's pty defaults to a bare "xterm"
// and the inner claude REPL can silently drop bracketed-paste sequences sent by
// macOS Terminal.app, making Cmd+V appear to do nothing.
func TestBuildInteractiveArgs_ForwardsTERMWhenSet(t *testing.T) {
	t.Setenv("TERM", "xterm-256color")

	args := BuildInteractiveArgs(testProfileDir, testUID, testGID)

	found := false
	for i := 0; i < len(args)-1; i++ {
		if args[i] == "-e" && args[i+1] == "TERM" {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("argv missing consecutive `-e TERM` pair; got %v", args)
	}
}

// TestBuildRunArgs_ContainerEnv_NilAllowlistOrdering (TP-013) verifies that when
// the allowlist is nil but containerEnv is non-empty, the CLAUDE_CONFIG_DIR entry
// still appears before the containerEnv entries in the resulting argv.
func TestBuildRunArgs_ContainerEnv_NilAllowlistOrdering(t *testing.T) {
	containerEnv := map[string]string{"FOO": "x"}
	args := BuildRunArgs(testProjectDir, testProfileDir, testUID, testGID, testCidfile,
		nil, containerEnv, "", testModel, "prompt")

	claudeConfigIdx := -1
	fooIdx := -1
	for i := 0; i < len(args)-1; i++ {
		if args[i] == "-e" && strings.HasPrefix(args[i+1], "CLAUDE_CONFIG_DIR=") {
			claudeConfigIdx = i
		}
		if args[i] == "-e" && args[i+1] == "FOO=x" {
			fooIdx = i
		}
	}
	if claudeConfigIdx < 0 {
		t.Fatal("CLAUDE_CONFIG_DIR not found in args")
	}
	if fooIdx < 0 {
		t.Fatal("-e FOO=x not found in args")
	}
	if claudeConfigIdx >= fooIdx {
		t.Errorf("CLAUDE_CONFIG_DIR (idx %d) must appear before containerEnv FOO=x (idx %d)", claudeConfigIdx, fooIdx)
	}
}

// TestBuildRunArgs_DoesNotMutateContainerEnv (TP-015) verifies that BuildRunArgs
// does not modify the containerEnv map passed by the caller, matching the
// input-immutability standard from docs/coding-standards/testing.md.
func TestBuildRunArgs_DoesNotMutateContainerEnv(t *testing.T) {
	containerEnv := map[string]string{"ALPHA": "a", "BETA": "b"}
	original := maps.Clone(containerEnv)

	_ = BuildRunArgs(testProjectDir, testProfileDir, testUID, testGID, testCidfile,
		nil, containerEnv, "", testModel, "prompt")

	if !reflect.DeepEqual(containerEnv, original) {
		t.Errorf("BuildRunArgs mutated containerEnv: before=%v after=%v", original, containerEnv)
	}
}

// TestBuildRunArgs_ResumeSessionID_NonEmpty verifies that a non-empty
// resumeSessionID appends "--resume <id>" to the claude argv immediately
// before "-p <prompt>".
func TestBuildRunArgs_ResumeSessionID_NonEmpty(t *testing.T) {
	const sid = "abc123-session"
	args := BuildRunArgs(testProjectDir, testProfileDir, testUID, testGID, testCidfile,
		nil, nil, sid, testModel, "prompt")

	// --resume <sid> must appear before -p.
	resumeIdx := -1
	pIdx := -1
	for i := 0; i < len(args)-1; i++ {
		if args[i] == "--resume" && args[i+1] == sid {
			resumeIdx = i
		}
		if args[i] == "-p" {
			pIdx = i
		}
	}
	if resumeIdx < 0 {
		t.Fatalf("--resume %q not found in args %v", sid, args)
	}
	if pIdx < 0 {
		t.Fatal("-p not found in args")
	}
	if resumeIdx >= pIdx {
		t.Errorf("--resume (idx %d) must appear before -p (idx %d)", resumeIdx, pIdx)
	}
}

// TestBuildRunArgs_ResumeSessionID_Empty verifies that an empty resumeSessionID
// leaves the argv unchanged — no --resume flag is added.
func TestBuildRunArgs_ResumeSessionID_Empty(t *testing.T) {
	args := BuildRunArgs(testProjectDir, testProfileDir, testUID, testGID, testCidfile,
		nil, nil, "", testModel, "prompt")

	for _, a := range args {
		if a == "--resume" {
			t.Errorf("--resume must not appear in args when resumeSessionID is empty: %v", args)
		}
	}
}

func TestBuildShellArgs_Shape(t *testing.T) {
	args := BuildShellArgs(testProjectDir, testProfileDir, testUID, testGID)

	wantFlags := []string{"-it", "--rm", "--init"}
	for _, flag := range wantFlags {
		if indexOf(args, flag) < 0 {
			t.Errorf("argv missing %q; got %v", flag, args)
		}
	}

	uidArg := fmt.Sprintf("%d:%d", testUID, testGID)
	uidIdx := indexOf(args, "-u")
	if uidIdx < 0 || uidIdx+1 >= len(args) || args[uidIdx+1] != uidArg {
		t.Errorf("expected -u %s; got %v", uidArg, args)
	}

	projectMount := fmt.Sprintf("type=bind,source=%s,target=%s", testProjectDir, ContainerRepoPath)
	profileMount := fmt.Sprintf("type=bind,source=%s,target=%s", testProfileDir, ContainerProfilePath)
	if indexOf(args, projectMount) < 0 {
		t.Errorf("argv missing project mount %q; got %v", projectMount, args)
	}
	if indexOf(args, profileMount) < 0 {
		t.Errorf("argv missing profile mount %q; got %v", profileMount, args)
	}
	if countFlag(args, "--mount") != 2 {
		t.Errorf("expected exactly 2 --mount entries (project + profile); got %d", countFlag(args, "--mount"))
	}

	wIdx := indexOf(args, "-w")
	if wIdx < 0 || wIdx+1 >= len(args) || args[wIdx+1] != ContainerRepoPath {
		t.Errorf("expected -w %s; got %v", ContainerRepoPath, args)
	}

	envSpec := "CLAUDE_CONFIG_DIR=" + ContainerProfilePath
	if indexOf(args, envSpec) < 0 {
		t.Errorf("argv missing %q; got %v", envSpec, args)
	}

	// argv must end with [ImageTag, "bash"].
	if args[len(args)-2] != ImageTag || args[len(args)-1] != "bash" {
		t.Errorf("argv must end with [ImageTag, 'bash']; got tail %v", args[len(args)-3:])
	}

	// Must NOT contain claude/workflow plumbing.
	forbidden := []string{"--cidfile", "--permission-mode", "-p", "claude", "--model"}
	for _, f := range forbidden {
		if indexOf(args, f) >= 0 {
			t.Errorf("argv must NOT contain %q; got %v", f, args)
		}
	}
}

// TestBuildShellArgs_ForwardsTERMWhenSet asserts BuildShellArgs adds
// `-e TERM` (name only) when the host has TERM set, matching the
// BuildInteractiveArgs behaviour.
func TestBuildShellArgs_ForwardsTERMWhenSet(t *testing.T) {
	t.Setenv("TERM", "xterm-256color")

	args := BuildShellArgs(testProjectDir, testProfileDir, testUID, testGID)

	found := false
	for i := 0; i < len(args)-1; i++ {
		if args[i] == "-e" && args[i+1] == "TERM" {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("argv missing consecutive `-e TERM` pair; got %v", args)
	}
}
