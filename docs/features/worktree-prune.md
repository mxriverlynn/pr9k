# worktree prune

## Purpose

`pr9k worktree prune` removes stale `pr9k-*` linked worktrees from the primary
checkout, freeing disk space and git refs left behind by interrupted or completed
runs. The bundled "Ralph" workflow ships with `worktrees.autoCleanup: true`, so
a normal end-to-end run cleans up its own worktree on exit; prune exists for
the cases that auto-cleanup cannot reach (SIGKILL, OOM, power loss, `q` → `y`
quits, or workflows that opt out of `autoCleanup`).

## Related

- [Worktrees runtime behavior](worktrees.md) — state file, resume decision tree, and `autoCleanup` ordering
- [Using Worktrees](../how-to/using-worktrees.md) — enabling the feature and lifecycle from a user's perspective
- [Managing Worktrees](../how-to/managing-worktrees.md) — `prune` invocation patterns and `--fresh` recovery

## Subcommand shape

```
pr9k worktree prune [--dry-run] [--project-dir <path>]
```

## Behavior

1. Runs `git worktree list --porcelain` against the resolved primary checkout.
2. Filters entries whose **branch name** (not directory basename) matches
   `^pr9k-\S+$`.
3. Skips two protected worktrees:
   - The **active-run worktree**: path read from
     `<primaryPath>/.pr9k/active-run.json` → `worktreePath`.
   - The **CWD-containing worktree**: resolved by walking up from `os.Getwd()`
     and cross-checking against the porcelain list.
4. For each candidate, prints the uncommitted-files count
   (`git -C <path> status --porcelain | wc -l`) before acting.
5. Removes the worktree (`git worktree remove --force`) and deletes the branch
   (`git branch -D`).

## --dry-run

Prints the same candidate set (with uncommitted counts) without performing any
removals or branch deletions.

## --project-dir

Resolves via `filepath.EvalSymlinks`, matching the pattern used by the main
`--project-dir` flag. Defaults to CWD when omitted.

## Filter key

The branch name from `git worktree list --porcelain` is the authoritative match
key — not the directory basename. A worktree whose directory is named
`my-pr9k-bench` but whose branch is `my-pr9k-bench` is not removed, because
`my-pr9k-bench` does not match `^pr9k-\S+$`.

## Out of scope

- Pruning worktrees whose branch name does not start with `pr9k-`.
- Removing the primary worktree.
- Any interaction with remote branches.
