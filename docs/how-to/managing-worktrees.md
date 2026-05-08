# Managing Worktrees

This guide covers `pr9k worktree prune`, the `--dry-run` flag, and recovery
scenarios using `--fresh`.

## pr9k worktree prune

Over time, interrupted or completed runs may leave `pr9k-*` linked worktrees on
disk if `autoCleanup` was not set or a run was killed before cleanup ran.
`pr9k worktree prune` removes those stale worktrees.

```
pr9k worktree prune [--dry-run] [--project-dir <path>]
```

### What it removes

Prune removes linked worktrees whose **branch name** matches `^pr9k-\S+$`. The
branch name from `git worktree list --porcelain` is the authoritative key — not
the directory basename.

Two worktrees are always protected:

- **Active-run worktree** — the path recorded in
  `<primaryPath>/.pr9k/active-run.json`. Never removed even if the process is
  not running.
- **CWD-containing worktree** — the worktree that contains your current working
  directory. Never removed so you are not pruned out from under yourself.

For each candidate, prune reports the uncommitted-file count before acting (from
`git status --porcelain`).

### --dry-run

Preview what would be removed without making any changes:

```
pr9k worktree prune --dry-run
```

Output example:

```
[dry-run] would remove: /repos/myproject/pr9k-2026-05-01-090000.000  branch=pr9k-2026-05-01-090000.000  uncommitted=0
[dry-run] would remove: /repos/myproject/pr9k-2026-05-03-141500.123  branch=pr9k-2026-05-03-141500.123  uncommitted=3
dry-run complete.
```

### --project-dir

Defaults to the current working directory. Use this flag when running prune
from a different directory than the primary checkout:

```
pr9k worktree prune --project-dir /path/to/myproject
```

The flag resolves symlinks the same way the main `--project-dir` flag does.

### Removal order

For each stale worktree, prune:
1. `git worktree remove --force <path>` — removes the worktree directory
2. `git branch -D <branch>` — deletes the run branch

Errors from either step are printed as warnings; prune continues to the next
candidate rather than stopping.

## --fresh flag

The `--fresh` flag discards any prior active-run state and forces a clean start,
even if a valid `active-run.json` exists.

```
pr9k --fresh [--project-dir <path>] [--workflow-dir <path>] [-n <iterations>]
```

### Recovery scenario: stale state file after a crash

If pr9k was killed (SIGKILL, OOM, power loss) and left an `active-run.json`
pointing at an abandoned worktree, the next normal invocation would try to
resume that worktree. Use `--fresh` to discard it:

```
pr9k --fresh
```

When the recorded process is no longer alive, `--fresh`:
1. Removes the stale worktree (`git worktree remove --force`) — **only if** the
   path recorded in `active-run.json` as `primaryPath` resolves to the same
   directory as the current `--project-dir` (symlink-canonicalized). This guards
   against destroying git objects when the primary checkout has been renamed or
   its symlink target has changed.
2. Deletes the stale branch (`git branch -D`) — same path-check guard as step 1.
3. Removes `active-run.json` unconditionally.
4. Prints `Discarded prior run` to stderr.
5. Starts a clean fresh run.

Warnings are printed for any git cleanup failure; the fresh start proceeds
regardless.

### Recovery scenario: concurrent run detected

If `--fresh` is invoked while a pr9k process from a prior invocation is still
alive (same `active-run.json`, matching PID and binary), pr9k refuses:

```
error: another pr9k appears to be running for this primary checkout (PID 12345)
```

Kill or stop that process before retrying `--fresh`.

### --fresh with no state file

If there is no `active-run.json`, `--fresh` is a no-op — it simply starts a
normal fresh run without any warning or error.

## Checking for stale worktrees without pruning

To see all current linked worktrees for a repo without modifying anything:

```
git -C /path/to/myproject worktree list
```

Entries whose branch name starts with `pr9k-` are pr9k-managed worktrees. The
`active-run.json` file identifies which one is in use (if any).

## See also

- [Using Worktrees](using-worktrees.md) — enabling the feature, config schema,
  and accepted limitations
- [Worktrees runtime behavior](../features/worktrees.md) — state-file schema,
  lifecycle, and resume decision tree
- [worktree-prune feature doc](../features/worktree-prune.md) — detailed prune
  behavior and filter-key semantics
