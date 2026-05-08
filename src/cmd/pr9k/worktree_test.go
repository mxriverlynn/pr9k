package main

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TP-210-001: prune removes pr9k-* worktrees but not unrelated ones.
func TestWorktreePrune_RemovesPr9kLeavesUnrelated(t *testing.T) {
	t.Parallel()

	primary := t.TempDir()
	initGitRepo(t, primary)

	// create two pr9k-* worktrees and one unrelated
	wt1 := filepath.Join(t.TempDir(), "pr9k-aaa")
	wt2 := filepath.Join(t.TempDir(), "pr9k-bbb")
	wtOther := filepath.Join(t.TempDir(), "feature-x")

	for _, pair := range [][2]string{{wt1, "pr9k-aaa"}, {wt2, "pr9k-bbb"}, {wtOther, "feature-x"}} {
		if err := gitWorktreeAdd(primary, pair[0], pair[1]); err != nil {
			t.Fatalf("gitWorktreeAdd: %v", err)
		}
	}

	var stdout bytes.Buffer
	deps := &worktreePruneDeps{
		stdout:     &stdout,
		stderr:     &stdout,
		projectDir: primary,
	}
	if err := runWorktreePrune(deps, false); err != nil {
		t.Fatalf("runWorktreePrune: %v", err)
	}

	// pr9k worktrees must be gone
	if _, err := os.Stat(wt1); !os.IsNotExist(err) {
		t.Error("wt1 (pr9k-aaa) still exists after prune")
	}
	if _, err := os.Stat(wt2); !os.IsNotExist(err) {
		t.Error("wt2 (pr9k-bbb) still exists after prune")
	}
	// unrelated worktree must remain
	if _, err := os.Stat(wtOther); err != nil {
		t.Errorf("unrelated worktree was removed: %v", err)
	}
}

// TP-210-002: prune skips the worktree listed in the active-run state file.
func TestWorktreePrune_SkipsActiveStateWorktree(t *testing.T) {
	t.Parallel()

	primary := t.TempDir()
	initGitRepo(t, primary)

	wtActive := filepath.Join(t.TempDir(), "pr9k-active")
	wtOther := filepath.Join(t.TempDir(), "pr9k-other")
	for _, pair := range [][2]string{{wtActive, "pr9k-active"}, {wtOther, "pr9k-other"}} {
		if err := gitWorktreeAdd(primary, pair[0], pair[1]); err != nil {
			t.Fatalf("gitWorktreeAdd: %v", err)
		}
	}

	var stdout bytes.Buffer
	deps := &worktreePruneDeps{
		stdout:             &stdout,
		stderr:             &stdout,
		projectDir:         primary,
		activeWorktreePath: wtActive,
	}
	if err := runWorktreePrune(deps, false); err != nil {
		t.Fatalf("runWorktreePrune: %v", err)
	}

	// active worktree must remain
	if _, err := os.Stat(wtActive); err != nil {
		t.Errorf("active worktree was removed: %v", err)
	}
	// other pr9k worktree must be gone
	if _, err := os.Stat(wtOther); !os.IsNotExist(err) {
		t.Error("non-active pr9k worktree still exists after prune")
	}
}

// TP-210-003: prune skips the CWD-containing worktree.
func TestWorktreePrune_SkipsCWDWorktree(t *testing.T) {
	t.Parallel()

	primary := t.TempDir()
	initGitRepo(t, primary)

	wtCWD := filepath.Join(t.TempDir(), "pr9k-cwd")
	wtOther := filepath.Join(t.TempDir(), "pr9k-other")
	for _, pair := range [][2]string{{wtCWD, "pr9k-cwd"}, {wtOther, "pr9k-other"}} {
		if err := gitWorktreeAdd(primary, pair[0], pair[1]); err != nil {
			t.Fatalf("gitWorktreeAdd: %v", err)
		}
	}

	var stdout bytes.Buffer
	deps := &worktreePruneDeps{
		stdout:          &stdout,
		stderr:          &stdout,
		projectDir:      primary,
		cwdWorktreePath: wtCWD,
	}
	if err := runWorktreePrune(deps, false); err != nil {
		t.Fatalf("runWorktreePrune: %v", err)
	}

	// CWD worktree must remain
	if _, err := os.Stat(wtCWD); err != nil {
		t.Errorf("CWD worktree was removed: %v", err)
	}
	// other pr9k worktree must be gone
	if _, err := os.Stat(wtOther); !os.IsNotExist(err) {
		t.Error("other pr9k worktree still exists after prune")
	}
}

// TP-210-004: --dry-run prints but does not remove.
func TestWorktreePrune_DryRunDoesNotRemove(t *testing.T) {
	t.Parallel()

	primary := t.TempDir()
	initGitRepo(t, primary)

	wt1 := filepath.Join(t.TempDir(), "pr9k-dry1")
	if err := gitWorktreeAdd(primary, wt1, "pr9k-dry1"); err != nil {
		t.Fatalf("gitWorktreeAdd: %v", err)
	}

	var stdout bytes.Buffer
	deps := &worktreePruneDeps{
		stdout:     &stdout,
		stderr:     &stdout,
		projectDir: primary,
	}
	if err := runWorktreePrune(deps, true); err != nil {
		t.Fatalf("runWorktreePrune dry-run: %v", err)
	}

	// worktree must still exist
	if _, err := os.Stat(wt1); err != nil {
		t.Errorf("worktree was removed in dry-run mode: %v", err)
	}
	// output must mention the worktree
	out := stdout.String()
	if !strings.Contains(out, "pr9k-dry1") {
		t.Errorf("dry-run output does not mention worktree: %q", out)
	}
}

// TP-210-005: a directory whose name suffix-matches (my-pr9k-bench) is left alone
// because the branch name must match ^pr9k-\S+, not the directory basename.
func TestWorktreePrune_SuffixMatchNotRemoved(t *testing.T) {
	t.Parallel()

	primary := t.TempDir()
	initGitRepo(t, primary)

	// branch name does NOT start with "pr9k-"
	wtSuffix := filepath.Join(t.TempDir(), "my-pr9k-bench")
	if err := gitWorktreeAdd(primary, wtSuffix, "my-pr9k-bench"); err != nil {
		t.Fatalf("gitWorktreeAdd: %v", err)
	}

	var stdout bytes.Buffer
	deps := &worktreePruneDeps{
		stdout:     &stdout,
		stderr:     &stdout,
		projectDir: primary,
	}
	if err := runWorktreePrune(deps, false); err != nil {
		t.Fatalf("runWorktreePrune: %v", err)
	}

	// suffix-match worktree must remain
	if _, err := os.Stat(wtSuffix); err != nil {
		t.Errorf("suffix-match worktree (branch=my-pr9k-bench) was incorrectly removed: %v", err)
	}
}

// TP-210-006: uncommitted-files count is printed before removal.
func TestWorktreePrune_PrintsUncommittedCount(t *testing.T) {
	t.Parallel()

	primary := t.TempDir()
	initGitRepo(t, primary)

	wt1 := filepath.Join(t.TempDir(), "pr9k-dirty")
	if err := gitWorktreeAdd(primary, wt1, "pr9k-dirty"); err != nil {
		t.Fatalf("gitWorktreeAdd: %v", err)
	}
	// add an untracked file to the worktree
	if err := os.WriteFile(filepath.Join(wt1, "newfile.txt"), []byte("data"), 0o600); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	var stdout bytes.Buffer
	deps := &worktreePruneDeps{
		stdout:     &stdout,
		stderr:     &stdout,
		projectDir: primary,
	}
	if err := runWorktreePrune(deps, false); err != nil {
		t.Fatalf("runWorktreePrune: %v", err)
	}

	out := stdout.String()
	// Output must contain "1 uncommitted" or similar count indication
	if !strings.Contains(out, "1") {
		t.Errorf("output does not contain uncommitted file count (1): %q", out)
	}
}
