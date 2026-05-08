package main

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// --- unit tests: porcelain parser ---

const samplePorcelain = `worktree /repo/main
HEAD aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa
branch refs/heads/main

worktree /repo/wt-detached
HEAD bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb
detached

worktree /repo/wt-locked
HEAD cccccccccccccccccccccccccccccccccccccccc
branch refs/heads/locked-branch
locked portable device

worktree /repo/wt-prunable
HEAD dddddddddddddddddddddddddddddddddddddddd
branch refs/heads/prunable-branch
prunable gitdir file points to non-existent location

`

func TestParseWorktreePorcelain_Basic(t *testing.T) {
	entries := parseWorktreePorcelain(samplePorcelain)
	if len(entries) != 4 {
		t.Fatalf("expected 4 entries, got %d", len(entries))
	}
}

func TestParseWorktreePorcelain_MainEntry(t *testing.T) {
	entries := parseWorktreePorcelain(samplePorcelain)
	e := entries[0]
	if e.Path != "/repo/main" {
		t.Errorf("Path: want /repo/main, got %q", e.Path)
	}
	if e.HEAD != "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa" {
		t.Errorf("HEAD: unexpected %q", e.HEAD)
	}
	if e.Branch != "refs/heads/main" {
		t.Errorf("Branch: want refs/heads/main, got %q", e.Branch)
	}
	if e.Detached {
		t.Error("Detached: want false")
	}
	if e.Locked {
		t.Error("Locked: want false")
	}
	if e.Prunable {
		t.Error("Prunable: want false")
	}
}

func TestParseWorktreePorcelain_DetachedEntry(t *testing.T) {
	entries := parseWorktreePorcelain(samplePorcelain)
	e := entries[1]
	if e.Path != "/repo/wt-detached" {
		t.Errorf("Path: want /repo/wt-detached, got %q", e.Path)
	}
	if !e.Detached {
		t.Error("Detached: want true")
	}
	if e.Branch != "" {
		t.Errorf("Branch: want empty, got %q", e.Branch)
	}
}

func TestParseWorktreePorcelain_LockedEntry(t *testing.T) {
	entries := parseWorktreePorcelain(samplePorcelain)
	e := entries[2]
	if !e.Locked {
		t.Error("Locked: want true")
	}
	if e.LockReason != "portable device" {
		t.Errorf("LockReason: want %q, got %q", "portable device", e.LockReason)
	}
}

func TestParseWorktreePorcelain_PrunableEntry(t *testing.T) {
	entries := parseWorktreePorcelain(samplePorcelain)
	e := entries[3]
	if !e.Prunable {
		t.Error("Prunable: want true")
	}
	if e.PrunableReason != "gitdir file points to non-existent location" {
		t.Errorf("PrunableReason: want %q, got %q",
			"gitdir file points to non-existent location", e.PrunableReason)
	}
}

func TestParseWorktreePorcelain_Empty(t *testing.T) {
	entries := parseWorktreePorcelain("")
	if len(entries) != 0 {
		t.Fatalf("expected 0 entries for empty input, got %d", len(entries))
	}
}

// --- integration tests ---

// initGitRepo creates a bare minimal git repo in dir (with an initial commit)
// so worktree operations work.
func initGitRepo(t *testing.T, dir string) {
	t.Helper()
	run := func(args ...string) {
		t.Helper()
		cmd := exec.Command(args[0], args[1:]...)
		cmd.Dir = dir
		// set git config for the repo
		cmd.Env = append(os.Environ(),
			"GIT_AUTHOR_NAME=test",
			"GIT_AUTHOR_EMAIL=test@test",
			"GIT_COMMITTER_NAME=test",
			"GIT_COMMITTER_EMAIL=test@test",
		)
		out, err := cmd.CombinedOutput()
		if err != nil {
			t.Fatalf("cmd %v: %v\n%s", args, err, out)
		}
	}
	run("git", "init", "-b", "main")
	run("git", "config", "user.email", "test@test")
	run("git", "config", "user.name", "test")
	// create a commit so HEAD is valid
	f := filepath.Join(dir, "README")
	if err := os.WriteFile(f, []byte("hello\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	run("git", "add", "README")
	run("git", "commit", "-m", "initial")
}

func TestGitWorktreeAddListRemove(t *testing.T) {
	primary := t.TempDir()
	initGitRepo(t, primary)

	wtPath := filepath.Join(t.TempDir(), "wt-feature")
	branch := "feature-branch"

	// add worktree
	if err := gitWorktreeAdd(primary, wtPath, branch); err != nil {
		t.Fatalf("gitWorktreeAdd: %v", err)
	}

	// verify worktree dir exists
	if _, err := os.Stat(wtPath); err != nil {
		t.Fatalf("worktree dir missing after add: %v", err)
	}

	// list includes the new worktree
	entries, err := gitWorktreeList(primary)
	if err != nil {
		t.Fatalf("gitWorktreeList: %v", err)
	}
	found := false
	for _, e := range entries {
		if e.Path == wtPath {
			found = true
			if e.Branch != "refs/heads/"+branch {
				t.Errorf("Branch: want refs/heads/%s, got %q", branch, e.Branch)
			}
		}
	}
	if !found {
		t.Errorf("worktree %q not found in list; entries: %+v", wtPath, entries)
	}

	// remove worktree
	if err := gitWorktreeRemove(primary, wtPath); err != nil {
		t.Fatalf("gitWorktreeRemove: %v", err)
	}

	// verify directory is gone
	if _, err := os.Stat(wtPath); !os.IsNotExist(err) {
		t.Errorf("worktree dir still exists after remove: %v", err)
	}
}

func TestGitWorktreeAdd_BranchAlreadyExists(t *testing.T) {
	primary := t.TempDir()
	initGitRepo(t, primary)

	wtPath := filepath.Join(t.TempDir(), "wt-dup")
	branch := "dup-branch"

	if err := gitWorktreeAdd(primary, wtPath, branch); err != nil {
		t.Fatalf("first gitWorktreeAdd: %v", err)
	}

	wtPath2 := filepath.Join(t.TempDir(), "wt-dup2")
	err := gitWorktreeAdd(primary, wtPath2, branch)
	if err == nil {
		t.Fatal("expected error on duplicate branch, got nil")
	}
	msg := err.Error()
	if !strings.HasPrefix(msg, "git: worktree add") {
		t.Errorf("error message should start with %q, got %q", "git: worktree add", msg)
	}
	// stderr from git should be non-empty and included in the message
	if len(msg) <= len("git: worktree add") {
		t.Errorf("error message appears to have no stderr content: %q", msg)
	}
}

func TestGitWorktreeRemove_ForceRemovesDirtyWorktree(t *testing.T) {
	primary := t.TempDir()
	initGitRepo(t, primary)

	wtPath := filepath.Join(t.TempDir(), "wt-dirty")
	branch := "dirty-branch"

	if err := gitWorktreeAdd(primary, wtPath, branch); err != nil {
		t.Fatalf("gitWorktreeAdd: %v", err)
	}

	// modify a tracked file to make the worktree dirty
	if err := os.WriteFile(filepath.Join(wtPath, "README"), []byte("dirty\n"), 0o600); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	if err := gitWorktreeRemove(primary, wtPath); err != nil {
		t.Fatalf("gitWorktreeRemove on dirty worktree: %v", err)
	}

	if _, err := os.Stat(wtPath); !os.IsNotExist(err) {
		t.Errorf("worktree dir still exists after force-remove: %v", err)
	}
}

func TestGitBranchDelete(t *testing.T) {
	primary := t.TempDir()
	initGitRepo(t, primary)

	wtPath := filepath.Join(t.TempDir(), "wt-del")
	branch := "delete-me"

	if err := gitWorktreeAdd(primary, wtPath, branch); err != nil {
		t.Fatalf("gitWorktreeAdd: %v", err)
	}

	// remove worktree first so branch is no longer checked out
	if err := gitWorktreeRemove(primary, wtPath); err != nil {
		t.Fatalf("gitWorktreeRemove: %v", err)
	}

	// now delete the branch
	if err := gitBranchDelete(primary, branch); err != nil {
		t.Fatalf("gitBranchDelete: %v", err)
	}

	// verify branch is gone
	cmd := exec.Command("git", "branch", "--list", branch)
	cmd.Dir = primary
	out, err := cmd.Output()
	if err != nil {
		t.Fatalf("git branch --list: %v", err)
	}
	if len(out) != 0 {
		t.Errorf("branch %q still exists after delete: %q", branch, out)
	}
}
