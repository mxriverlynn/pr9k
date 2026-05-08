package main

import (
	"bytes"
	"fmt"
	"os/exec"
	"strings"
)

// WorktreeEntry holds the parsed fields from one record in
// `git worktree list --porcelain` output.
type WorktreeEntry struct {
	Path           string
	HEAD           string
	Branch         string // refs/heads/<name>; empty when detached
	Detached       bool
	Bare           bool
	Locked         bool
	LockReason     string
	Prunable       bool
	PrunableReason string
}

// parseWorktreePorcelain parses the output of `git worktree list --porcelain`
// into a slice of WorktreeEntry. Records are separated by blank lines.
// Unknown fields are silently ignored so that future git versions are
// forward-compatible.
func parseWorktreePorcelain(output string) []WorktreeEntry {
	var entries []WorktreeEntry
	var cur *WorktreeEntry

	for _, line := range strings.Split(output, "\n") {
		if line == "" {
			if cur != nil {
				entries = append(entries, *cur)
				cur = nil
			}
			continue
		}
		if strings.HasPrefix(line, "worktree ") {
			if cur != nil {
				entries = append(entries, *cur)
			}
			cur = &WorktreeEntry{Path: strings.TrimPrefix(line, "worktree ")}
			continue
		}
		if cur == nil {
			continue
		}
		switch {
		case strings.HasPrefix(line, "HEAD "):
			cur.HEAD = strings.TrimPrefix(line, "HEAD ")
		case strings.HasPrefix(line, "branch "):
			cur.Branch = strings.TrimPrefix(line, "branch ")
		case line == "detached":
			cur.Detached = true
		case line == "bare":
			cur.Bare = true
		case strings.HasPrefix(line, "locked"):
			cur.Locked = true
			if rest := strings.TrimPrefix(line, "locked"); rest != "" {
				cur.LockReason = strings.TrimPrefix(rest, " ")
			}
		case strings.HasPrefix(line, "prunable"):
			cur.Prunable = true
			if rest := strings.TrimPrefix(line, "prunable"); rest != "" {
				cur.PrunableReason = strings.TrimPrefix(rest, " ")
			}
		}
	}
	if cur != nil {
		entries = append(entries, *cur)
	}
	return entries
}

// gitWorktreeAdd creates a new linked worktree at path on a new branch.
// Equivalent to: git worktree add -b <branch> <path>
// dir is the primary checkout directory (used as cmd.Dir).
func gitWorktreeAdd(dir, path, branch string) error {
	var stderr bytes.Buffer
	cmd := exec.Command("git", "worktree", "add", "-b", branch, path)
	cmd.Dir = dir
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("git: worktree add -b %s %s: %w: %s", branch, path, err, strings.TrimSpace(stderr.String()))
	}
	return nil
}

// gitWorktreeList runs `git worktree list --porcelain` from dir and returns
// the parsed result.
func gitWorktreeList(dir string) ([]WorktreeEntry, error) {
	var stdout, stderr bytes.Buffer
	cmd := exec.Command("git", "worktree", "list", "--porcelain")
	cmd.Dir = dir
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		return nil, fmt.Errorf("git: worktree list: %w: %s", err, strings.TrimSpace(stderr.String()))
	}
	return parseWorktreePorcelain(stdout.String()), nil
}

// gitWorktreeRemove removes the linked worktree at path, using --force so
// untracked or modified files do not block removal.
// dir is the primary checkout directory (used as cmd.Dir).
func gitWorktreeRemove(dir, path string) error {
	var stderr bytes.Buffer
	cmd := exec.Command("git", "worktree", "remove", "--force", path)
	cmd.Dir = dir
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("git: worktree remove --force %s: %w: %s", path, err, strings.TrimSpace(stderr.String()))
	}
	return nil
}

// gitBranchDelete deletes the named branch from the repository at dir.
// Equivalent to: git branch -D <branch>
func gitBranchDelete(dir, branch string) error {
	var stderr bytes.Buffer
	cmd := exec.Command("git", "branch", "-D", branch)
	cmd.Dir = dir
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("git: branch -D %s: %w: %s", branch, err, strings.TrimSpace(stderr.String()))
	}
	return nil
}
