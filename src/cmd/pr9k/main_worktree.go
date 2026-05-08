package main

import (
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"

	"github.com/mxriverlynn/pr9k/src/internal/steps"
	"github.com/mxriverlynn/pr9k/src/internal/workflow"
)

// worktreeAction describes the action to take for the worktree at run start.
type worktreeAction int

const (
	worktreeActionFreshStart worktreeAction = iota // mint new stamp, create worktree (or use projectDir)
	worktreeActionResume                           // use existing worktree from state file
	worktreeActionConcurrent                       // another run is active — refuse
)

// decideWorktreeAction returns the worktree action based on the prior active-run
// state, the validation result, configuration, and the --fresh flag.
//
// Decision rules (evaluated in order):
//  1. --fresh flag → always FreshStart (discards any prior state).
//  2. Concurrent result → refuse.
//  3. WorktreeMissing or BranchMissing → FreshStart (prior worktree is gone).
//  4. NoActiveRun + prior state exists + worktrees enabled → Resume.
//  5. Otherwise → FreshStart.
func decideWorktreeAction(priorState *ActiveRunState, decision resumeResult, worktreesEnabled, fresh bool) worktreeAction {
	if fresh {
		return worktreeActionFreshStart
	}
	switch decision {
	case resumeResultConcurrent:
		return worktreeActionConcurrent
	case resumeResultWorktreeMissing, resumeResultBranchMissing:
		return worktreeActionFreshStart
	default: // resumeResultNoActiveRun
		if priorState != nil && worktreesEnabled {
			return worktreeActionResume
		}
		return worktreeActionFreshStart
	}
}

// postRunCleanup handles state file removal and optional worktree/branch cleanup
// after the workflow goroutine exits.
//
// On Completed or LoopBroken:
//   - If autoCleanup: remove worktree (--force), delete branch, remove state file.
//   - If !autoCleanup: remove state file only.
//
// On UserQuit: leave everything in place for resume on next invocation.
//
// Cleanup order (D-3): worktree removal → branch deletion → state file removal.
// Worktree must be removed before branch deletion because git prevents deleting
// a branch that is currently checked out in a worktree.
// Errors from git operations are printed as warnings but do not stop subsequent steps.
func postRunCleanup(primaryPath, stateFilePath string, state *ActiveRunState, exitReason workflow.ExitReason, autoCleanup bool, stderr io.Writer) {
	if state == nil {
		return
	}
	switch exitReason {
	case workflow.ExitReasonCompleted, workflow.ExitReasonLoopBroken:
		if autoCleanup {
			if state.WorktreePath != "" {
				if err := gitWorktreeRemove(primaryPath, state.WorktreePath); err != nil {
					fmt.Fprintf(stderr, "warning: worktree cleanup: %v\n", err)
				}
			}
			if state.Branch != "" {
				if err := gitBranchDelete(primaryPath, state.Branch); err != nil {
					fmt.Fprintf(stderr, "warning: branch cleanup: %v\n", err)
				}
			}
		}
		if err := removeActiveRun(stateFilePath, state); err != nil {
			fmt.Fprintf(stderr, "warning: state file removal: %v\n", err)
		}
	case workflow.ExitReasonUserQuit:
		// leave state file and worktree in place for auto-resume
	}
}

// applyFreshFlag handles the --fresh flag before the resume-validation step.
//
// If fresh is false, returns (prior, nil) unchanged.
// If fresh is true and prior is nil (no state file), returns (nil, nil) — no-op.
// If fresh is true and the recorded process is alive, returns an error with the
// spec-committed concurrent-run message: "another pr9k appears to be running
// for this primary checkout (PID N)".
// If fresh is true and the process is dead, removes the state file unconditionally,
// and removes the worktree and branch only when prior.PrimaryPath canonicalizes to
// the same path as currentPrimaryPath (skipping git ops protects against accidental
// cleanup after a primary-checkout rename or symlink retarget).
// Emits "Discarded prior run" to stderr and returns (nil, nil).
func applyFreshFlag(stateFilePath, currentPrimaryPath string, fresh bool, prior *ActiveRunState, stderr io.Writer) (*ActiveRunState, error) {
	if !fresh {
		return prior, nil
	}
	if prior == nil {
		return nil, nil
	}
	if isProcessAlive(prior.PID, prior.Binary) {
		return nil, fmt.Errorf("another pr9k appears to be running for this primary checkout (PID %d)", prior.PID)
	}

	// Canonicalize both paths to handle symlinks (e.g. /var vs /private/var on macOS).
	canon := func(p string) string {
		if resolved, err := filepath.EvalSymlinks(p); err == nil {
			return resolved
		}
		return p
	}
	if canon(prior.PrimaryPath) == canon(currentPrimaryPath) {
		// Same primary checkout — safe to destroy git objects.
		if err := gitWorktreeRemove(prior.PrimaryPath, prior.WorktreePath); err != nil {
			fmt.Fprintf(stderr, "warning: --fresh: worktree cleanup: %v\n", err)
		}
		if err := gitBranchDelete(prior.PrimaryPath, prior.Branch); err != nil {
			fmt.Fprintf(stderr, "warning: --fresh: branch cleanup: %v\n", err)
		}
	}

	if err := os.Remove(stateFilePath); err != nil && !errors.Is(err, os.ErrNotExist) {
		fmt.Fprintf(stderr, "warning: --fresh: could not remove state file: %v\n", err)
	}
	fmt.Fprintln(stderr, "Discarded prior run")
	return nil, nil
}

// loadStepsForWorktrees loads the step file from workflowDir and returns the
// WorktreesConfig block, or nil if the block is absent.
// This is the minimal early load needed to determine the worktree action before
// calling startup() (which also loads the step file for validation and execution).
func loadStepsForWorktrees(workflowDir string) (*steps.WorktreesConfig, error) {
	sf, err := steps.LoadSteps(workflowDir)
	if err != nil {
		return nil, err
	}
	return sf.Worktrees, nil
}
