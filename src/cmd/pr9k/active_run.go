package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/mxriverlynn/pr9k/src/internal/atomicwrite"
)

// ActiveRunState is the on-disk schema for the active-run state file.
// It is stored at <primaryPath>/.pr9k/active-run.json.
type ActiveRunState struct {
	SchemaVersion int    `json:"schemaVersion"`
	WorktreeStamp string `json:"worktreeStamp"`
	WorktreePath  string `json:"worktreePath"`
	PrimaryPath   string `json:"primaryPath"`
	Branch        string `json:"branch"`
	PID           int    `json:"pid"`
	Binary        string `json:"binary"`
}

const activeRunSchemaVersion = 1

// resumeResult describes the outcome of validateActiveRun.
type resumeResult int

const (
	resumeResultNoActiveRun     resumeResult = iota // no file or stale process → safe to start fresh
	resumeResultConcurrent                          // live process with same primaryPath → refuse
	resumeResultWorktreeMissing                     // process dead or gone → safe to start fresh
	resumeResultBranchMissing                       // branch gone → safe to start fresh
)

// readActiveRun reads the active-run state file at path.
// Returns (nil, nil) on ENOENT.
// On JSON parse failure → renames to .corrupted-<timestamp> and returns (nil, nil).
// On unknown schemaVersion → renames to .incompatible-<timestamp> and returns (nil, nil).
func readActiveRun(path string) (*ActiveRunState, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil, nil
		}
		return nil, fmt.Errorf("active-run: read %s: %w", path, err)
	}

	var state ActiveRunState
	if err := json.Unmarshal(data, &state); err != nil {
		renamed := fmt.Sprintf("%s.corrupted-%d", path, time.Now().UnixNano())
		if renameErr := os.Rename(path, renamed); renameErr != nil {
			fmt.Fprintf(os.Stderr, "pr9k: active-run: rename corrupted state file %s: %v\n", path, renameErr)
		} else {
			fmt.Fprintf(os.Stderr, "active-run: corrupted state file renamed to %s\n", filepath.Base(renamed))
		}
		return nil, nil
	}

	if state.SchemaVersion != activeRunSchemaVersion {
		renamed := fmt.Sprintf("%s.incompatible-%d", path, time.Now().UnixNano())
		if renameErr := os.Rename(path, renamed); renameErr != nil {
			fmt.Fprintf(os.Stderr, "pr9k: active-run: rename incompatible state file %s: %v\n", path, renameErr)
		} else {
			fmt.Fprintf(os.Stderr, "active-run: incompatible schemaVersion %d renamed to %s\n",
				state.SchemaVersion, filepath.Base(renamed))
		}
		return nil, nil
	}

	return &state, nil
}

// writeActiveRun writes state to path using atomicwrite, creating parent dirs.
func writeActiveRun(path string, state ActiveRunState) error {
	data, err := json.Marshal(state)
	if err != nil {
		return fmt.Errorf("active-run: marshal: %w", err)
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return fmt.Errorf("active-run: mkdir %s: %w", filepath.Dir(path), err)
	}
	if err := atomicwrite.Write(path, data, 0o600); err != nil {
		return fmt.Errorf("active-run: write %s: %w", path, err)
	}
	return nil
}

// verifyActiveRunClaim reads the state file back and confirms PID and
// WorktreeStamp match claimed. A mismatch means a concurrent writer won the
// read→validate→write race; the caller should abort with a clear message.
func verifyActiveRunClaim(path string, claimed ActiveRunState) error {
	onDisk, err := readActiveRun(path)
	if err != nil {
		return fmt.Errorf("active-run: claim verify read: %w", err)
	}
	if onDisk == nil {
		return fmt.Errorf("active-run: claim verify: state file missing after write")
	}
	if onDisk.PID != claimed.PID || onDisk.WorktreeStamp != claimed.WorktreeStamp {
		return fmt.Errorf("active-run: concurrent process claimed the run (on-disk pid=%d stamp=%s, ours pid=%d stamp=%s)",
			onDisk.PID, onDisk.WorktreeStamp, claimed.PID, claimed.WorktreeStamp)
	}
	return nil
}

// removeActiveRun deletes the state file only if the on-disk WorktreeStamp
// matches the stamp in caller. ENOENT is benign (D-6). Stamp mismatch → no-op (D-7).
func removeActiveRun(path string, caller *ActiveRunState) error {
	onDisk, err := readActiveRun(path)
	if err != nil {
		return err
	}
	if onDisk == nil {
		// ENOENT (or renamed away as corrupted/incompatible) — benign
		return nil
	}
	if onDisk.WorktreeStamp != caller.WorktreeStamp {
		fmt.Fprintf(os.Stderr, "active-run: stamp mismatch; skipping remove (on-disk=%s, caller=%s)\n",
			onDisk.WorktreeStamp, caller.WorktreeStamp)
		return nil
	}
	if err := os.Remove(path); err != nil && !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("active-run: remove %s: %w", path, err)
	}
	return nil
}

// validateActiveRun evaluates whether a concurrent run is active.
// Evaluation order (D-8):
//  1. Read state file — ENOENT / unreadable → resumeResultNoActiveRun
//  2. Process liveness (isProcessAlive) — dead → resumeResultNoActiveRun
//  3. primaryPath equality (EvalSymlinks-canonicalized) — mismatch → resumeResultNoActiveRun
//  4. Worktree existence — missing → resumeResultWorktreeMissing
//  5. Branch existence (via gitWorktreeList) — missing → resumeResultBranchMissing
//  6. All checks pass → resumeResultConcurrent
func validateActiveRun(path, currentPrimaryPath string) resumeResult {
	state, err := readActiveRun(path)
	if err != nil || state == nil {
		return resumeResultNoActiveRun
	}

	if !isProcessAlive(state.PID, state.Binary) {
		return resumeResultNoActiveRun
	}

	canon := func(p string) string {
		if resolved, err := filepath.EvalSymlinks(p); err == nil {
			return resolved
		}
		return p
	}
	if canon(state.PrimaryPath) != canon(currentPrimaryPath) {
		return resumeResultNoActiveRun
	}

	if _, err := os.Stat(state.WorktreePath); err != nil {
		return resumeResultWorktreeMissing
	}

	entries, err := gitWorktreeList(state.PrimaryPath)
	if err != nil {
		return resumeResultWorktreeMissing
	}
	wantBranch := "refs/heads/" + state.Branch
	for _, e := range entries {
		if e.Branch == wantBranch {
			return resumeResultConcurrent
		}
	}
	return resumeResultBranchMissing
}
