# Using Worktrees

The worktrees feature isolates each pr9k run inside a dedicated git linked
worktree on its own branch. The primary checkout stays untouched during the
run; changes land in a sibling directory that is created, used, and optionally
cleaned up automatically.

> **The bundled "Ralph" workflow ships with worktrees enabled and `autoCleanup`
> on.** If you are running the default workflow you do not need to do anything
> to opt in — every run already creates a `pr9k-*` worktree and cleans it up on
> a clean exit. The rest of this page applies whether you are using the
> bundled workflow as-is or authoring a custom one. For the actual config block
> as shipped, see [`workflow/config.json`](../../workflow/config.json).

## Prerequisites

- **git ≥ 2.17** — `git worktree list --porcelain` requires git 2.17 or later
  (A1). Run `git --version` to confirm.
- **No branch-protection rule that blocks force-delete** — autoCleanup uses
  `git branch -D` to remove the run branch after completion.

## Enabling the feature

Add a `worktrees` block to your `config.json`:

```json
{
  "worktrees": {
    "enabled": true,
    "autoCleanup": true
  },
  "initialize": [],
  "iteration": [],
  "finalize": []
}
```

Both fields default to `false` when the block is absent. `autoCleanup: true`
requires `enabled: true`; the validator rejects the combination
`enabled: false, autoCleanup: true` with a fatal error.

### Field reference

| Field | Type | Description |
|-------|------|-------------|
| `enabled` | bool | Create a linked worktree on each fresh start and use it as the run's project directory |
| `autoCleanup` | bool | After `Completed` or `LoopBroken` exits, remove the worktree and delete the branch automatically |

## What changes when worktrees is enabled

When a fresh run starts with `enabled: true`, pr9k:

1. Mints a timestamp-based stamp: `pr9k-YYYY-MM-DD-HHMMSS.mmm`
2. Creates a linked worktree at `<parent-of-primaryPath>/<stamp>` on a new
   branch of the same name.
3. Sets the run's **project directory** to the worktree path. All downstream
   subsystems — the Docker sandbox bind-mount, the logger, the validator, and
   every step subprocess — operate against the worktree, not the primary checkout.
4. Writes `<primaryPath>/.pr9k/active-run.json` recording the worktree path,
   branch, stamp, PID, and binary path for later resume or prune operations.

The primary checkout is never modified during the run.

## Auto-resume on next invocation

When pr9k starts and finds a valid `active-run.json` whose recorded process is
no longer alive, it automatically **resumes** the prior run — re-using the
existing worktree and branch instead of creating new ones. The TUI iteration
line shows `(resumed)` to indicate this.

To skip the resume and force a clean start, pass `--fresh`. See
[Managing Worktrees](managing-worktrees.md) for details.

## Auto-cleanup ordering

When `autoCleanup: true` and the run exits with reason `Completed` or
`LoopBroken`, pr9k performs cleanup in this order (D-3):

1. `git worktree remove --force <worktreePath>` — detach the worktree
2. `git branch -D <branch>` — delete the run branch
3. Remove `active-run.json`

The worktree must be removed before the branch can be deleted — git refuses to
delete a branch that is currently checked out in a linked worktree.

When the run exits with reason `UserQuit` (user confirmed `q` + `y`), nothing
is cleaned up; the state is preserved for auto-resume on the next invocation.

## Accepted limitations

### R3 — `iteration.jsonl` accumulation across resumed runs

`<projectDir>/.pr9k/iteration.jsonl` accumulates step records for the entire
run. When worktrees is enabled and a run is resumed, the worktree's
`iteration.jsonl` already contains records from the prior partial run. The
resumed run appends new records to that same file rather than starting fresh.

As a result, tools that parse `iteration.jsonl` will see records from both the
original and resumed segments interleaved by wall-clock time order. The
`invocation_stamp` field in each record identifies which pr9k invocation wrote
it, making the two segments distinguishable:

```json
{"schema_version":1,"issue_id":"42","invocation_stamp":"pr9k-2026-05-08-100000.000", ...}
{"schema_version":1,"issue_id":"42","invocation_stamp":"pr9k-2026-05-08-143000.000", ...}
```

This is intentional behavior; there is no per-resume reset of the log.

## Verifying the worktree

The log panel shows two header lines when worktrees is active:

```
[worktrees] worktree: /path/to/sibling/pr9k-2026-05-08-120000.000
[worktrees] branch: pr9k-2026-05-08-120000.000
```

On a resume, a third line appears first:

```
[worktrees] RESUMED FROM /path/to/sibling/pr9k-2026-05-08-120000.000
```

## See also

- [Managing Worktrees](managing-worktrees.md) — `pr9k worktree prune`,
  `--dry-run`, and `--fresh` recovery scenarios
- [Worktrees runtime behavior](../features/worktrees.md) — state-file schema,
  lifecycle, and resume decision tree
- [Building Custom Workflows](building-custom-workflows.md) — how `worktrees`
  fits among the other top-level `config.json` blocks
- [Bundled `workflow/config.json`](../../workflow/config.json) — the shipped
  default that turns this feature on
