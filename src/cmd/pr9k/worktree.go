package main

import (
	"bytes"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/spf13/cobra"
)

// pr9kBranchRe matches branch names that start exactly with "pr9k-" followed by
// at least one non-whitespace character (e.g. "pr9k-20260101-abc"). It uses the
// branch name from porcelain — not the directory basename — as the authoritative
// match key, so a worktree whose directory is named "my-pr9k-bench" but whose
// branch is "my-pr9k-bench" is not removed.
var pr9kBranchRe = regexp.MustCompile(`^pr9k-\S+$`)

// worktreePruneDeps holds injected state for runWorktreePrune so tests can
// drive every branch without a real CWD or state file.
type worktreePruneDeps struct {
	stdout             io.Writer
	stderr             io.Writer
	projectDir         string
	activeWorktreePath string // from active-run state file; empty → skip check
	cwdWorktreePath    string // worktree that contains CWD; empty → skip check
}

// newWorktreeCmd returns the `worktree` cobra parent command.
func newWorktreeCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:           "worktree",
		Short:         "Manage pr9k-managed git worktrees",
		SilenceErrors: true,
		SilenceUsage:  true,
		RunE: func(cmd *cobra.Command, args []string) error {
			return cmd.Help()
		},
	}
	cmd.AddCommand(newWorktreePruneCmd())
	return cmd
}

// newWorktreePruneCmd returns the `worktree prune` cobra command.
func newWorktreePruneCmd() *cobra.Command {
	var projectDir string
	var dryRun bool

	cmd := &cobra.Command{
		Use:           "prune",
		Short:         "Remove stale pr9k-* worktrees (skips active and CWD-containing)",
		SilenceErrors: true,
		SilenceUsage:  true,
		RunE: func(cmd *cobra.Command, args []string) error {
			resolved, err := resolveWorktreeProjectDir(projectDir)
			if err != nil {
				return err
			}

			activeState := readActiveWorktreePath(resolved)
			cwdWT := findCWDWorktree(resolved)

			deps := &worktreePruneDeps{
				stdout:             os.Stdout,
				stderr:             os.Stderr,
				projectDir:         resolved,
				activeWorktreePath: activeState,
				cwdWorktreePath:    cwdWT,
			}
			return runWorktreePrune(deps, dryRun)
		},
	}
	cmd.Flags().StringVar(&projectDir, "project-dir", "", "path to the primary git checkout (default: CWD)")
	cmd.Flags().BoolVar(&dryRun, "dry-run", false, "print what would be removed without removing anything")
	return cmd
}

// resolveWorktreeProjectDir resolves the --project-dir flag value (or CWD if
// empty) via EvalSymlinks, matching the pattern in cli/args.go.
func resolveWorktreeProjectDir(flag string) (string, error) {
	if flag == "" {
		cwd, err := os.Getwd()
		if err != nil {
			return "", fmt.Errorf("worktree: getwd: %w", err)
		}
		flag = cwd
	}
	resolved, err := filepath.EvalSymlinks(flag)
	if err != nil {
		return "", fmt.Errorf("worktree: --project-dir %q: %w", flag, err)
	}
	return resolved, nil
}

// readActiveWorktreePath reads the active-run state file and returns the
// WorktreePath field. Returns "" when there is no state file or on error.
func readActiveWorktreePath(primaryPath string) string {
	statePath := filepath.Join(primaryPath, ".pr9k", "active-run.json")
	state, err := readActiveRun(statePath)
	if err != nil || state == nil {
		return ""
	}
	if resolved, err := filepath.EvalSymlinks(state.WorktreePath); err == nil {
		return resolved
	}
	return state.WorktreePath
}

// findCWDWorktree returns the worktree path (from git's perspective) that
// contains the current working directory, or "" if CWD is not inside any
// known worktree.
func findCWDWorktree(primaryPath string) string {
	cwd, err := os.Getwd()
	if err != nil {
		return ""
	}
	entries, err := gitWorktreeList(primaryPath)
	if err != nil {
		return ""
	}
	// For each worktree, check if cwd is at or under its path.
	for _, e := range entries {
		wtPath := e.Path
		// Canonicalize both for symlink-safe comparison.
		resolvedWT, err := filepath.EvalSymlinks(wtPath)
		if err != nil {
			resolvedWT = wtPath
		}
		resolvedCWD, err := filepath.EvalSymlinks(cwd)
		if err != nil {
			resolvedCWD = cwd
		}
		if resolvedCWD == resolvedWT || strings.HasPrefix(resolvedCWD, resolvedWT+string(filepath.Separator)) {
			return resolvedWT
		}
	}
	return ""
}

// gitUncommittedCount returns the number of lines in `git status --porcelain`
// for the worktree at path — a proxy for uncommitted changes. Returns 0 on error.
func gitUncommittedCount(path string) int {
	var stdout bytes.Buffer
	cmd := exec.Command("git", "-C", path, "status", "--porcelain")
	cmd.Stdout = &stdout
	if err := cmd.Run(); err != nil {
		return 0
	}
	lines := strings.Split(strings.TrimRight(stdout.String(), "\n"), "\n")
	count := 0
	for _, l := range lines {
		if l != "" {
			count++
		}
	}
	return count
}

// runWorktreePrune implements the prune logic. When dryRun is true, it prints
// what would be removed without making any changes.
func runWorktreePrune(deps *worktreePruneDeps, dryRun bool) error {
	entries, err := gitWorktreeList(deps.projectDir)
	if err != nil {
		return fmt.Errorf("worktree prune: %w", err)
	}

	canon := func(p string) string {
		if resolved, err := filepath.EvalSymlinks(p); err == nil {
			return resolved
		}
		return p
	}

	activeCanon := ""
	if deps.activeWorktreePath != "" {
		activeCanon = canon(deps.activeWorktreePath)
	}
	cwdCanon := ""
	if deps.cwdWorktreePath != "" {
		cwdCanon = canon(deps.cwdWorktreePath)
	}

	pruned := 0
	for _, e := range entries {
		// Only match by branch name, not directory basename.
		branchName := strings.TrimPrefix(e.Branch, "refs/heads/")
		if !pr9kBranchRe.MatchString(branchName) {
			continue
		}

		wtCanon := canon(e.Path)

		// Skip active state-file worktree.
		if activeCanon != "" && wtCanon == activeCanon {
			_, _ = fmt.Fprintf(deps.stdout, "skip (active): %s\n", e.Path)
			continue
		}
		// Skip CWD-containing worktree.
		if cwdCanon != "" && wtCanon == cwdCanon {
			_, _ = fmt.Fprintf(deps.stdout, "skip (cwd): %s\n", e.Path)
			continue
		}

		count := gitUncommittedCount(e.Path)
		if dryRun {
			_, _ = fmt.Fprintf(deps.stdout, "[dry-run] would remove: %s  branch=%s  uncommitted=%d\n",
				e.Path, branchName, count)
			continue
		}

		_, _ = fmt.Fprintf(deps.stdout, "removing: %s  branch=%s  uncommitted=%d\n",
			e.Path, branchName, count)

		if err := gitWorktreeRemove(deps.projectDir, e.Path); err != nil {
			_, _ = fmt.Fprintf(deps.stderr, "worktree prune: remove %s: %v\n", e.Path, err)
			continue
		}
		if err := gitBranchDelete(deps.projectDir, branchName); err != nil {
			_, _ = fmt.Fprintf(deps.stderr, "worktree prune: branch delete %s: %v\n", branchName, err)
		}
		pruned++
	}

	if dryRun {
		_, _ = fmt.Fprintf(deps.stdout, "dry-run complete.\n")
	} else {
		_, _ = fmt.Fprintf(deps.stdout, "pruned %d worktree(s).\n", pruned)
	}
	return nil
}
