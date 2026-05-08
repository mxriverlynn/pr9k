package main

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/mxriverlynn/pr9k/src/internal/cli"
	"github.com/mxriverlynn/pr9k/src/internal/workflow"
)

// TP-211-001: decideWorktreeAction returns FreshStart when no prior state and no
// concurrent run.
func TestDecideWorktreeAction_NoState_FreshStart(t *testing.T) {
	t.Parallel()
	got := decideWorktreeAction(nil, resumeResultNoActiveRun, true, false)
	if got != worktreeActionFreshStart {
		t.Errorf("want worktreeActionFreshStart, got %v", got)
	}
}

// TP-211-002: decideWorktreeAction returns Resume when prior state exists,
// worktrees enabled, and validation returned NoActiveRun (dead process).
func TestDecideWorktreeAction_PriorState_Enabled_Resume(t *testing.T) {
	t.Parallel()
	state := &ActiveRunState{
		WorktreeStamp: "pr9k-2026-05-08-120000.000",
		WorktreePath:  "/tmp/wt",
	}
	got := decideWorktreeAction(state, resumeResultNoActiveRun, true, false)
	if got != worktreeActionResume {
		t.Errorf("want worktreeActionResume, got %v", got)
	}
}

// TP-211-003: decideWorktreeAction returns Concurrent when validation says concurrent.
func TestDecideWorktreeAction_Concurrent(t *testing.T) {
	t.Parallel()
	got := decideWorktreeAction(nil, resumeResultConcurrent, true, false)
	if got != worktreeActionConcurrent {
		t.Errorf("want worktreeActionConcurrent, got %v", got)
	}
}

// TP-211-004: decideWorktreeAction returns FreshStart when --fresh flag is set,
// even when prior state exists and worktrees enabled.
func TestDecideWorktreeAction_Fresh_OverridesPriorState(t *testing.T) {
	t.Parallel()
	state := &ActiveRunState{WorktreeStamp: "pr9k-2026-05-08-120000.000"}
	got := decideWorktreeAction(state, resumeResultNoActiveRun, true, true /* fresh */)
	if got != worktreeActionFreshStart {
		t.Errorf("want worktreeActionFreshStart with --fresh, got %v", got)
	}
}

// TP-211-005: decideWorktreeAction returns FreshStart when worktree is missing.
func TestDecideWorktreeAction_WorktreeMissing_FreshStart(t *testing.T) {
	t.Parallel()
	state := &ActiveRunState{WorktreeStamp: "pr9k-2026-05-08-120000.000"}
	got := decideWorktreeAction(state, resumeResultWorktreeMissing, true, false)
	if got != worktreeActionFreshStart {
		t.Errorf("want worktreeActionFreshStart on worktree-missing, got %v", got)
	}
}

// TP-211-006: decideWorktreeAction returns FreshStart when branch is missing.
func TestDecideWorktreeAction_BranchMissing_FreshStart(t *testing.T) {
	t.Parallel()
	state := &ActiveRunState{WorktreeStamp: "pr9k-2026-05-08-120000.000"}
	got := decideWorktreeAction(state, resumeResultBranchMissing, true, false)
	if got != worktreeActionFreshStart {
		t.Errorf("want worktreeActionFreshStart on branch-missing, got %v", got)
	}
}

// TP-211-007: decideWorktreeAction returns FreshStart when prior state exists
// but worktrees.enabled is false.
func TestDecideWorktreeAction_PriorState_WorktreesDisabled_FreshStart(t *testing.T) {
	t.Parallel()
	state := &ActiveRunState{WorktreeStamp: "pr9k-2026-05-08-120000.000"}
	got := decideWorktreeAction(state, resumeResultNoActiveRun, false /* disabled */, false)
	if got != worktreeActionFreshStart {
		t.Errorf("want worktreeActionFreshStart when worktrees disabled, got %v", got)
	}
}

// TP-211-008: postRunCleanup on Completed with autoCleanup=true removes the
// worktree, branch, and state file.
// Ordering: worktree removed first so branch is no longer checked out, then
// branch deleted, then state file removed (matches git constraint and the
// existing worktree prune implementation).
func TestPostRunCleanup_Completed_AutoCleanup_RemovesAll(t *testing.T) {
	t.Parallel()

	primary := t.TempDir()
	initGitRepo(t, primary)

	stamp := "pr9k-2026-05-08-120000.000"
	wtPath := filepath.Join(t.TempDir(), stamp)
	if err := gitWorktreeAdd(primary, wtPath, stamp); err != nil {
		t.Fatalf("gitWorktreeAdd: %v", err)
	}

	stateFilePath := filepath.Join(primary, ".pr9k", "active-run.json")
	state := &ActiveRunState{
		SchemaVersion: activeRunSchemaVersion,
		WorktreeStamp: stamp,
		WorktreePath:  wtPath,
		PrimaryPath:   primary,
		Branch:        stamp,
	}
	if err := writeActiveRun(stateFilePath, *state); err != nil {
		t.Fatalf("writeActiveRun: %v", err)
	}

	var buf bytes.Buffer
	postRunCleanup(primary, stateFilePath, state, workflow.ExitReasonCompleted, true, &buf)

	// Worktree directory must be gone.
	if _, err := os.Stat(wtPath); !os.IsNotExist(err) {
		t.Errorf("worktree still exists after autoCleanup: %v", err)
	}
	// State file must be gone.
	if _, err := os.Stat(stateFilePath); !os.IsNotExist(err) {
		t.Errorf("state file still exists after autoCleanup: %v", err)
	}
	// Branch must be gone (gitWorktreeList should not contain it).
	entries, err := gitWorktreeList(primary)
	if err != nil {
		t.Fatalf("gitWorktreeList: %v", err)
	}
	wantBranch := "refs/heads/" + stamp
	for _, e := range entries {
		if e.Branch == wantBranch {
			t.Errorf("branch %s still exists after autoCleanup", stamp)
		}
	}
}

// TP-211-009: postRunCleanup on Completed without autoCleanup only removes the
// state file; worktree and branch remain.
func TestPostRunCleanup_Completed_NoAutoCleanup_KeepsWorktree(t *testing.T) {
	t.Parallel()

	primary := t.TempDir()
	initGitRepo(t, primary)

	stamp := "pr9k-2026-05-08-130000.000"
	wtPath := filepath.Join(t.TempDir(), stamp)
	if err := gitWorktreeAdd(primary, wtPath, stamp); err != nil {
		t.Fatalf("gitWorktreeAdd: %v", err)
	}

	stateFilePath := filepath.Join(primary, ".pr9k", "active-run.json")
	state := &ActiveRunState{
		SchemaVersion: activeRunSchemaVersion,
		WorktreeStamp: stamp,
		WorktreePath:  wtPath,
		PrimaryPath:   primary,
		Branch:        stamp,
	}
	if err := writeActiveRun(stateFilePath, *state); err != nil {
		t.Fatalf("writeActiveRun: %v", err)
	}

	var buf bytes.Buffer
	postRunCleanup(primary, stateFilePath, state, workflow.ExitReasonCompleted, false /* no autoCleanup */, &buf)

	// State file must be gone.
	if _, err := os.Stat(stateFilePath); !os.IsNotExist(err) {
		t.Errorf("state file still exists: %v", err)
	}
	// Worktree must still exist.
	if _, err := os.Stat(wtPath); err != nil {
		t.Errorf("worktree was removed unexpectedly: %v", err)
	}
}

// TP-211-010: postRunCleanup on UserQuit leaves the state file in place and
// does not touch the worktree.
func TestPostRunCleanup_UserQuit_LeavesStateFile(t *testing.T) {
	t.Parallel()

	primary := t.TempDir()
	initGitRepo(t, primary)

	stamp := "pr9k-2026-05-08-140000.000"
	wtPath := filepath.Join(t.TempDir(), stamp)
	if err := gitWorktreeAdd(primary, wtPath, stamp); err != nil {
		t.Fatalf("gitWorktreeAdd: %v", err)
	}

	stateFilePath := filepath.Join(primary, ".pr9k", "active-run.json")
	state := &ActiveRunState{
		SchemaVersion: activeRunSchemaVersion,
		WorktreeStamp: stamp,
		WorktreePath:  wtPath,
		PrimaryPath:   primary,
		Branch:        stamp,
	}
	if err := writeActiveRun(stateFilePath, *state); err != nil {
		t.Fatalf("writeActiveRun: %v", err)
	}

	var buf bytes.Buffer
	postRunCleanup(primary, stateFilePath, state, workflow.ExitReasonUserQuit, true, &buf)

	// State file must still exist.
	if _, err := os.Stat(stateFilePath); err != nil {
		t.Errorf("state file was removed on UserQuit: %v", err)
	}
	// Worktree must still exist.
	if _, err := os.Stat(wtPath); err != nil {
		t.Errorf("worktree was removed on UserQuit: %v", err)
	}
}

// TP-211-011: postRunCleanup on LoopBroken with autoCleanup=true behaves like
// Completed (removes worktree, branch, and state file).
func TestPostRunCleanup_LoopBroken_AutoCleanup_RemovesAll(t *testing.T) {
	t.Parallel()

	primary := t.TempDir()
	initGitRepo(t, primary)

	stamp := "pr9k-2026-05-08-150000.000"
	wtPath := filepath.Join(t.TempDir(), stamp)
	if err := gitWorktreeAdd(primary, wtPath, stamp); err != nil {
		t.Fatalf("gitWorktreeAdd: %v", err)
	}

	stateFilePath := filepath.Join(primary, ".pr9k", "active-run.json")
	state := &ActiveRunState{
		SchemaVersion: activeRunSchemaVersion,
		WorktreeStamp: stamp,
		WorktreePath:  wtPath,
		PrimaryPath:   primary,
		Branch:        stamp,
	}
	if err := writeActiveRun(stateFilePath, *state); err != nil {
		t.Fatalf("writeActiveRun: %v", err)
	}

	var buf bytes.Buffer
	postRunCleanup(primary, stateFilePath, state, workflow.ExitReasonLoopBroken, true, &buf)

	if _, err := os.Stat(wtPath); !os.IsNotExist(err) {
		t.Errorf("worktree still exists after autoCleanup on LoopBroken")
	}
	if _, err := os.Stat(stateFilePath); !os.IsNotExist(err) {
		t.Errorf("state file still exists after autoCleanup on LoopBroken")
	}
}

// TP-211-012: cli.Config.Fresh field exists and defaults to false.
func TestCliConfig_FreshField_DefaultFalse(t *testing.T) {
	t.Parallel()
	cfg := &cli.Config{}
	if cfg.Fresh {
		t.Error("cli.Config.Fresh must default to false")
	}
}

// TP-211-013: --fresh flag is wired into the root cobra command via cli.NewCommand.
func TestFreshFlag_WiredIntoRootCommand(t *testing.T) {
	t.Parallel()
	cfg := &cli.Config{}
	cmd := cli.NewCommand(cfg)
	// Verify --fresh is a registered flag on the root command.
	flag := cmd.Flags().Lookup("fresh")
	if flag == nil {
		t.Fatal("--fresh flag not registered on root command")
	}
	if flag.DefValue != "false" {
		t.Errorf("--fresh default must be false, got %q", flag.DefValue)
	}
}

// TP-211-014: steps.StepFile can parse a worktrees block from config.json.
func TestStepFile_WorktreesBlock_Parsed(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	content := `{
		"initialize": [],
		"iteration": [{"name":"s1","isClaude":false,"command":["echo","ok"]}],
		"finalize": [],
		"worktrees": {"enabled": true, "autoCleanup": true}
	}`
	if err := os.WriteFile(filepath.Join(dir, "config.json"), []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}

	sf, err := loadStepsForWorktrees(dir)
	if err != nil {
		t.Fatalf("loadStepsForWorktrees: %v", err)
	}
	if sf == nil {
		t.Fatal("loadStepsForWorktrees returned nil")
	}
	if !sf.Enabled {
		t.Error("Worktrees.Enabled must be true")
	}
	if !sf.AutoCleanup {
		t.Error("Worktrees.AutoCleanup must be true")
	}
}

// TP-211-015: loadStepsForWorktrees returns nil when no worktrees block present.
func TestStepFile_NoWorktreesBlock_ReturnsNil(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	content := `{
		"initialize": [],
		"iteration": [{"name":"s1","isClaude":false,"command":["echo","ok"]}],
		"finalize": []
	}`
	if err := os.WriteFile(filepath.Join(dir, "config.json"), []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}

	sf, err := loadStepsForWorktrees(dir)
	if err != nil {
		t.Fatalf("loadStepsForWorktrees: %v", err)
	}
	if sf != nil {
		t.Errorf("expected nil worktrees config when block absent, got %+v", sf)
	}
}

// TP-001: applyFreshFlag with fresh=true, dead PID, prior state on disk removes
// the worktree, branch, and state file; returns nil, nil; prints "Discarded prior run".
func TestApplyFreshFlag_Fresh_DeadPID_RemovesAll(t *testing.T) {
	t.Parallel()

	primary := t.TempDir()
	initGitRepo(t, primary)

	stamp := "pr9k-2026-05-08-170000.000"
	wtPath := filepath.Join(t.TempDir(), stamp)
	if err := gitWorktreeAdd(primary, wtPath, stamp); err != nil {
		t.Fatalf("gitWorktreeAdd: %v", err)
	}

	stateFilePath := filepath.Join(primary, ".pr9k", "active-run.json")
	state := &ActiveRunState{
		SchemaVersion: activeRunSchemaVersion,
		WorktreeStamp: stamp,
		WorktreePath:  wtPath,
		PrimaryPath:   primary,
		Branch:        stamp,
		PID:           1,
		Binary:        "/nonexistent/path/pr9k", // binary mismatch → dead
	}
	if err := writeActiveRun(stateFilePath, *state); err != nil {
		t.Fatalf("writeActiveRun: %v", err)
	}

	var buf bytes.Buffer
	got, err := applyFreshFlag(stateFilePath, primary, true, state, &buf)

	if err != nil {
		t.Fatalf("expected nil error for dead PID, got %v", err)
	}
	if got != nil {
		t.Errorf("expected nil return, got %+v", got)
	}
	if _, statErr := os.Stat(stateFilePath); !os.IsNotExist(statErr) {
		t.Errorf("state file still exists after applyFreshFlag: %v", statErr)
	}
	if _, statErr := os.Stat(wtPath); !os.IsNotExist(statErr) {
		t.Errorf("worktree still exists after applyFreshFlag: %v", statErr)
	}
	if !strings.Contains(buf.String(), "Discarded prior run") {
		t.Errorf("expected 'Discarded prior run' in stderr, got %q", buf.String())
	}
}

// TP-001b: applyFreshFlag with fresh=true and no prior state (nil) is a no-op.
func TestApplyFreshFlag_Fresh_NilPrior_Noop(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	stateFilePath := filepath.Join(dir, ".pr9k", "active-run.json")

	var buf bytes.Buffer
	got, err := applyFreshFlag(stateFilePath, dir, true, nil, &buf)

	if err != nil {
		t.Fatalf("expected nil error when prior is nil, got %v", err)
	}
	if got != nil {
		t.Errorf("expected nil return, got %+v", got)
	}
	if buf.Len() != 0 {
		t.Errorf("unexpected output for no-op case: %q", buf.String())
	}
}

// TP-001c: applyFreshFlag with fresh=false leaves prior state and file untouched.
func TestApplyFreshFlag_NotFresh_PriorUnchanged(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	stateFilePath := filepath.Join(dir, ".pr9k", "active-run.json")
	state := &ActiveRunState{
		SchemaVersion: activeRunSchemaVersion,
		WorktreeStamp: "pr9k-2026-05-08-190000.000",
	}
	if err := writeActiveRun(stateFilePath, *state); err != nil {
		t.Fatalf("writeActiveRun: %v", err)
	}

	var buf bytes.Buffer
	got, err := applyFreshFlag(stateFilePath, dir, false, state, &buf)

	if err != nil {
		t.Fatalf("expected nil error when fresh=false, got %v", err)
	}
	if got != state {
		t.Errorf("expected prior state pointer unchanged, got %+v", got)
	}
	if _, statErr := os.Stat(stateFilePath); statErr != nil {
		t.Errorf("state file was removed unexpectedly: %v", statErr)
	}
}

// TP-212-001: applyFreshFlag with --fresh and a live recorded process refuses
// with the spec-committed concurrent-run message containing the PID.
func TestApplyFreshFlag_Fresh_LiveProcess_RefusesWithPID(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	stateFilePath := filepath.Join(dir, ".pr9k", "active-run.json")

	exe, err := os.Executable()
	if err != nil {
		t.Fatalf("os.Executable: %v", err)
	}

	state := &ActiveRunState{
		SchemaVersion: activeRunSchemaVersion,
		WorktreeStamp: "pr9k-2026-05-08-210000.000",
		WorktreePath:  filepath.Join(dir, "wt"),
		PrimaryPath:   dir,
		Branch:        "pr9k-2026-05-08-210000.000",
		PID:           os.Getpid(),
		Binary:        exe,
	}
	if err := writeActiveRun(stateFilePath, *state); err != nil {
		t.Fatalf("writeActiveRun: %v", err)
	}

	var buf bytes.Buffer
	got, freshErr := applyFreshFlag(stateFilePath, dir, true, state, &buf)

	if freshErr == nil {
		t.Fatal("expected non-nil error for live process, got nil")
	}
	if !strings.Contains(freshErr.Error(), fmt.Sprintf("PID %d", os.Getpid())) {
		t.Errorf("error must include PID, got %q", freshErr.Error())
	}
	if !strings.Contains(freshErr.Error(), "another pr9k appears to be running") {
		t.Errorf("error must contain spec message, got %q", freshErr.Error())
	}
	if got != nil {
		t.Errorf("expected nil state on live-process refusal, got %+v", got)
	}
	// State file must still exist — we refused, did not clean up.
	if _, statErr := os.Stat(stateFilePath); statErr != nil {
		t.Errorf("state file was removed despite live process: %v", statErr)
	}
}

// TP-212-002: applyFreshFlag with --fresh and a dead-PID state file cleans up
// the worktree, branch, and state file, then prints "Discarded prior run".
func TestApplyFreshFlag_Fresh_DeadPID_WithWorktree_CleansUp(t *testing.T) {
	t.Parallel()

	primary := t.TempDir()
	initGitRepo(t, primary)

	stamp := "pr9k-2026-05-08-220000.000"
	wtPath := filepath.Join(t.TempDir(), stamp)
	if err := gitWorktreeAdd(primary, wtPath, stamp); err != nil {
		t.Fatalf("gitWorktreeAdd: %v", err)
	}

	stateFilePath := filepath.Join(primary, ".pr9k", "active-run.json")
	state := &ActiveRunState{
		SchemaVersion: activeRunSchemaVersion,
		WorktreeStamp: stamp,
		WorktreePath:  wtPath,
		PrimaryPath:   primary,
		Branch:        stamp,
		PID:           1,
		Binary:        "/nonexistent/path/pr9k",
	}
	if err := writeActiveRun(stateFilePath, *state); err != nil {
		t.Fatalf("writeActiveRun: %v", err)
	}

	var buf bytes.Buffer
	got, err := applyFreshFlag(stateFilePath, primary, true, state, &buf)

	if err != nil {
		t.Fatalf("expected nil error for dead PID, got %v", err)
	}
	if got != nil {
		t.Errorf("expected nil state return, got %+v", got)
	}
	if !strings.Contains(buf.String(), "Discarded prior run") {
		t.Errorf("expected 'Discarded prior run' in stderr, got %q", buf.String())
	}
	if _, statErr := os.Stat(stateFilePath); !os.IsNotExist(statErr) {
		t.Errorf("state file still exists after --fresh cleanup")
	}
	if _, statErr := os.Stat(wtPath); !os.IsNotExist(statErr) {
		t.Errorf("worktree still exists after --fresh cleanup")
	}
}

// TP-212-003: applyFreshFlag with --fresh and no state file (prior=nil) is a
// no-op: no error, no output, proceeds fresh.
func TestApplyFreshFlag_Fresh_NoStateFile_Noop(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	stateFilePath := filepath.Join(dir, ".pr9k", "active-run.json")

	var buf bytes.Buffer
	got, err := applyFreshFlag(stateFilePath, dir, true, nil, &buf)

	if err != nil {
		t.Fatalf("expected nil error when no state file, got %v", err)
	}
	if got != nil {
		t.Errorf("expected nil state return, got %+v", got)
	}
	if buf.Len() != 0 {
		t.Errorf("unexpected output for no-op case: %q", buf.String())
	}
}

// TP-212-004: applyFreshFlag with --fresh treats binary-path mismatch as dead
// (PID alive but binary path differs) and proceeds with cleanup.
func TestApplyFreshFlag_Fresh_BinaryMismatch_TreatsAsDead(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	stateFilePath := filepath.Join(dir, ".pr9k", "active-run.json")

	state := &ActiveRunState{
		SchemaVersion: activeRunSchemaVersion,
		WorktreeStamp: "pr9k-2026-05-08-230000.000",
		WorktreePath:  filepath.Join(dir, "wt"),
		PrimaryPath:   dir,
		Branch:        "pr9k-2026-05-08-230000.000",
		PID:           1,                        // alive PID (init/launchd)
		Binary:        "/nonexistent/path/pr9k", // wrong binary → treated as dead
	}
	if err := writeActiveRun(stateFilePath, *state); err != nil {
		t.Fatalf("writeActiveRun: %v", err)
	}

	var buf bytes.Buffer
	got, err := applyFreshFlag(stateFilePath, dir, true, state, &buf)

	if err != nil {
		t.Fatalf("expected nil error (binary mismatch treated as dead), got %v", err)
	}
	if got != nil {
		t.Errorf("expected nil state return, got %+v", got)
	}
	if !strings.Contains(buf.String(), "Discarded prior run") {
		t.Errorf("expected 'Discarded prior run' in stderr, got %q", buf.String())
	}
}

// TP-002: loadStepsForWorktrees returns a non-nil error when the workflowDir
// contains no config.json (LoadSteps fails).
func TestLoadStepsForWorktrees_NoConfigJSON_ReturnsError(t *testing.T) {
	t.Parallel()

	dir := t.TempDir() // empty — no config.json

	sf, err := loadStepsForWorktrees(dir)
	if err == nil {
		t.Fatal("expected non-nil error when config.json is absent, got nil")
	}
	if sf != nil {
		t.Errorf("expected nil *StepFile on error, got %+v", sf)
	}
}

// TP-211-016: postRunCleanup with autoCleanup prints stderr warnings for git
// failures rather than silently ignoring them.
func TestPostRunCleanup_GitFailure_PrintsWarning(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	// Use a non-existent primary path so git operations fail.
	stateFilePath := filepath.Join(dir, ".pr9k", "active-run.json")
	state := &ActiveRunState{
		SchemaVersion: activeRunSchemaVersion,
		WorktreeStamp: "pr9k-2026-05-08-160000.000",
		WorktreePath:  "/nonexistent/path",
		PrimaryPath:   "/nonexistent/primary",
		Branch:        "pr9k-2026-05-08-160000.000",
	}
	// Write a state file to confirm it gets removed even on git failure.
	if err := writeActiveRun(stateFilePath, *state); err != nil {
		t.Fatalf("writeActiveRun: %v", err)
	}

	var buf bytes.Buffer
	postRunCleanup("/nonexistent/primary", stateFilePath, state, workflow.ExitReasonCompleted, true, &buf)

	// Even on git failure, state file removal should be attempted (ENOENT-safe).
	// Warnings must appear for git failures.
	out := buf.String()
	if !strings.Contains(out, "warning") {
		t.Errorf("expected warning in output for git failures, got %q", out)
	}
}
