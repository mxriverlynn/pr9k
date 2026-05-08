# Worktrees

## Purpose

The worktrees feature isolates each pr9k run inside a dedicated git linked
worktree on its own branch. This keeps the primary checkout clean, enables
automatic resume after an interrupted run, and allows `pr9k worktree prune` to
reclaim abandoned worktrees by branch-name pattern.

## Config schema

The feature is controlled by a top-level `worktrees` block in `config.json`:

```json
{
  "worktrees": {
    "enabled": true,
    "autoCleanup": true
  }
}
```

| Field | Type | Default | Description |
|-------|------|---------|-------------|
| `enabled` | bool | `false` | Create and use a linked worktree for each run |
| `autoCleanup` | bool | `false` | Remove worktree and branch on `Completed`/`LoopBroken` exit |

Omitting the `worktrees` block entirely is equivalent to `{"enabled": false,
"autoCleanup": false}`. The validator rejects `autoCleanup: true` without
`enabled: true` with a fatal error.

## Worktree path and branch naming

When a fresh start occurs, pr9k mints a timestamp-based stamp:

```
pr9k-YYYY-MM-DD-HHMMSS.mmm
```

Example: `pr9k-2026-05-08-120000.000`

The stamp serves as both the branch name and the worktree directory basename:

- **Branch**: `pr9k-2026-05-08-120000.000`
- **Worktree path**: `<parent-of-primaryPath>/pr9k-2026-05-08-120000.000`

The worktree is created as a sibling of the primary checkout directory (one
level up), not inside it.

## State file

When `enabled: true`, pr9k writes a state file after preflight succeeds:

**Location:** `<primaryPath>/.pr9k/active-run.json`

**Schema (version 1):**

```json
{
  "schemaVersion": 1,
  "worktreeStamp": "pr9k-2026-05-08-120000.000",
  "worktreePath":  "/repos/pr9k-2026-05-08-120000.000",
  "primaryPath":   "/repos/myproject",
  "branch":        "pr9k-2026-05-08-120000.000",
  "pid":           12345,
  "binary":        "/usr/local/bin/pr9k"
}
```

| Field | Description |
|-------|-------------|
| `schemaVersion` | Always `1`; increment on incompatible changes |
| `worktreeStamp` | Timestamp stamp, used as a collision-guard key for state-file removal |
| `worktreePath` | Absolute path to the linked worktree |
| `primaryPath` | Absolute path to the primary checkout |
| `branch` | Git branch name (equals `worktreeStamp`) |
| `pid` | PID of the pr9k process that wrote this file |
| `binary` | Absolute path to the pr9k binary (used for liveness disambiguation) |

The state file is written **after** preflight passes (D-1), so a preflight
failure does not leave an orphaned state file pointing at an unusable worktree.

After writing, pr9k performs a **post-write readback** (`verifyActiveRunClaim`):
it reads the file back and confirms the on-disk `pid` and `worktreeStamp` match
what was just written. A mismatch means a concurrent process won the
read → validate → write race and claimed the run; pr9k exits with an error
rather than starting two processes against the same worktree.

If startup fails between worktree creation and claim write, a `removeUnclaimed`
cleanup removes the just-created worktree and branch so no orphan is left behind.

## Resume decision tree

At startup, pr9k reads `active-run.json` and evaluates the following rules in
order:

| Priority | Condition | Action |
|----------|-----------|--------|
| 1 | `--fresh` flag set | FreshStart (discard prior state) |
| 2 | Validation returns `Concurrent` (live process, same `primaryPath`) | Refuse with error |
| 3 | Validation returns `WorktreeMissing` or `BranchMissing` | FreshStart |
| 4 | No active run AND prior state exists AND `worktrees.enabled` | Resume |
| 5 | Otherwise | FreshStart |

**Validation result meanings:**

- `NoActiveRun` — no state file, or the recorded process is not alive
- `Concurrent` — the recorded process is alive and matches the `primaryPath`
- `WorktreeMissing` — the worktree directory is gone
- `BranchMissing` — the git branch no longer exists

On **Resume**, pr9k re-uses `worktreePath` and `branch` from the state file
and passes `worktreePath` as the project directory to all downstream subsystems.
The TUI iteration header shows `(resumed)` to signal this.

On **FreshStart**, pr9k creates a new worktree with a freshly minted stamp.

## Project directory substitution

Regardless of whether the run is fresh or resumed, when `enabled: true` the
**project directory** passed to all subsystems is the worktree path — not the
primary checkout path (T2). This means:

- The Docker sandbox bind-mounts the worktree as `/home/agent/workspace`
- The logger writes its artifact directory under the worktree's `.pr9k/logs/`
- The `{{PROJECT_DIR}}` variable resolves to the worktree path in prompts
- Shell steps (`cmd.Dir`) execute from the worktree

The primary checkout directory (`--project-dir` value) is used only for:
- Reading/writing `active-run.json`
- Creating new worktrees (`git worktree add`)

## autoCleanup ordering

Cleanup order is fixed regardless of `autoCleanup` (D-3):

1. `git worktree remove --force <worktreePath>`
2. `git branch -D <branch>`
3. Remove `active-run.json`

Step 1 must precede step 2: git refuses to delete a branch that is currently
checked out in a linked worktree. Errors from git operations are printed as
warnings but do not stop subsequent cleanup steps.

## ExitReason and cleanup mapping

| `ExitReason` | `autoCleanup: true` | `autoCleanup: false` |
|--------------|---------------------|----------------------|
| `completed` | Remove worktree + branch + state file | Remove state file only |
| `loop_broken` | Remove worktree + branch + state file | Remove state file only |
| `user_quit` | Leave everything in place (state preserved for resume) | Leave everything in place |

`loop_broken` triggers the same cleanup as `completed` because the iteration
loop exited cleanly via `breakLoopIfEmpty`; finalization ran.

## TUI surfaces

When `enabled: true`, the log panel shows a header block:

```
[worktrees] worktree: /repos/pr9k-2026-05-08-120000.000
[worktrees] branch: pr9k-2026-05-08-120000.000
```

On a resume, a preceding line appears:

```
[worktrees] RESUMED FROM /repos/pr9k-2026-05-08-120000.000
```

The iteration header line in the TUI shows the worktree directory basename as a
prefix (e.g., `pr9k-2026-05-08-120000.000 / iteration 1 of 5`) and appends
`(resumed)` on resumed runs.

## --fresh flag

`--fresh` forces a FreshStart regardless of existing state. When the prior
process is dead, `--fresh` removes the stale worktree and branch **only if**
`prior.PrimaryPath` canonicalizes to the same path as the current
`--project-dir`. This guards against accidental git-object destruction after
a primary checkout has been renamed or its symlink retarget. The state file
itself is always removed regardless.

See [Managing Worktrees — --fresh flag](../how-to/managing-worktrees.md#--fresh-flag)
for usage and recovery scenarios.

## Out of scope

- Sharing a single worktree across multiple simultaneous pr9k processes.
- Creating worktrees inside the primary checkout directory (always a sibling).
- Worktrees for non-git project directories.

## See also

- [Using Worktrees](../how-to/using-worktrees.md) — enabling the feature,
  prerequisites, accepted limitations (R3)
- [Managing Worktrees](../how-to/managing-worktrees.md) — `pr9k worktree prune`
  and `--fresh` recovery
- [worktree prune](worktree-prune.md) — prune subcommand details
