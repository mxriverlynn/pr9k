# Feature Specification: useWorktrees

When the workflow config's top-level `worktrees.enabled` is `true`, pr9k runs the entire workflow inside a freshly created git worktree so the user's primary checkout stays usable for parallel development while pr9k mutates code on a separate branch. A companion `worktrees.autoCleanup` option and a new `pr9k worktree prune` subcommand make removal of finished worktrees a one-step or zero-step operation rather than a manual chore.

## Background (for readers new to git worktrees)

A git worktree is a second on-disk working directory backed by the same repository. The repository's commits, branches, remotes, and config are shared across all worktrees, but each worktree has its own checked-out branch, its own working files, and its own staging area. Two worktrees cannot have the same branch checked out at the same time, so pr9k always creates a fresh branch when it makes a worktree — that is the only way to give the worktree a branch the primary doesn't already have. Creating a worktree is fast (no re-clone), and removing one leaves the rest of the repository untouched. This is the mechanism that lets pr9k operate on the next feature in one directory while the user keeps running the app from another.

A **run-stamp** is a millisecond-precision identifier generated once per pr9k invocation from the wall clock at startup. The format is `ralph-YYYY-MM-DD-HHMMSS.mmm` (e.g., `ralph-2026-05-07-143022.123`). The same stamp identifies the run's log file, the run's artifact directory, and — when worktrees are enabled — the worktree directory and the worktree branch. There is no persistence and no coordination between runs: each pr9k invocation produces a brand-new stamp at startup. The stamp's source of truth is documented in [`docs/code-packages/logger.md`](../../../code-packages/logger.md).

**Runs do not coordinate with each other.** Each pr9k invocation operates independently — there is no run-level resume, no shared state on disk between invocations, and no awareness that a prior run was working on the same set of issues. If a prior run is killed mid-iteration, its partial work remains on its own worktree and branch, and a fresh run picks up the next open issue (which may be the same one) from scratch on a new worktree and branch. See [the dedicated alternate flow below](#a-prior-run-was-killed-mid-iteration-and-the-user-restarts-pr9k) for the consequences and reconciliation steps.

## Outcome

- pr9k's commits, file edits, and branch changes happen in a separate working directory; the user's primary checkout's working tree is unchanged on disk and stays on its original branch ([D1](artifacts/decision-log.md#d1-worktree-location)).
- The user can run, build, and test the application from the primary checkout while pr9k iterates.
- All commits pr9k produces during the run land on a single, traceable branch tied to that run, and that branch is pushed to the remote so the work is durable even if the worktree is later removed ([D2](artifacts/decision-log.md#d2-branch-naming-for-the-run)).
- Run-time observability — log files, the iteration log, per-step transcripts — is preserved after the run completes so the user can review what pr9k did. The worktree path and the run's branch name are written to both the run's log file (header) and the run's iteration log (first record) so they are greppable from structured output, not only from the TUI ([D3](artifacts/decision-log.md#d3-log-durability)).
- The default behavior of pr9k is unchanged: workflows that do not set `worktrees.enabled: true` continue to operate in-place exactly as before ([D8](artifacts/decision-log.md#d8-default-and-config-shape)).
- When `worktrees.autoCleanup: true` is set, the worktree and its branch are removed automatically on graceful completion or graceful quit. When it is `false` (the default), the worktree is left on disk for the user to inspect and clean up themselves or via `pr9k worktree prune` ([D13](artifacts/decision-log.md#d13-autocleanup-behavior)).
- A new `pr9k worktree prune` subcommand removes all pr9k-owned worktrees and their branches in one step, so cleanup is never a manual sequence of `git worktree remove` invocations ([D14](artifacts/decision-log.md#d14-pr9k-worktree-prune-subcommand)).

### Invariants

- pr9k must not write to, change branches in, or otherwise mutate the primary checkout's working tree while the run is in progress. This is a hard constraint, enforced behaviorally by the worktree redirect ([T1](artifacts/feature-technical-notes.md#t1-sequencing-constraint)).

## Actors and Triggers

- **Actor:** the pr9k user — a developer who wants to keep using their primary checkout (running the app, testing other features, pairing on a separate branch) while pr9k iterates on the next feature.
- **Trigger:** the user adds a `worktrees` block to `workflow/config.json` with `enabled: true` (and optionally `autoCleanup: true`) and runs `pr9k` (with or without `-n`, `--project-dir`, or `--workflow-dir`). The toggle is read from the workflow config; there is no CLI flag override. When `--project-dir <path>` is also passed, the resolved `--project-dir` is treated as "the primary checkout," and the worktree is created relative to that path. The `pr9k worktree prune` subcommand is a separate trigger — it is invoked manually by the user to clean up accumulated pr9k-owned worktrees, regardless of the `worktrees` block's contents.
- **Preconditions:**
  - The target repository is a git repository (already required by pr9k today).
  - `git` is installed and on PATH (already required).
  - The primary checkout has a resolvable HEAD (any branch, any commit, including a detached HEAD).
  - The primary checkout's working tree may have uncommitted changes — worktrees are independent of the primary's working state, so this is allowed.
  - The remote permits pushing to branch names beginning with `pr9k/`. Branch-protection rules that reject this pattern will cause the run's first push to fail ([D7](artifacts/decision-log.md#d7-first-push-sets-upstream)).
  - Only one pr9k instance should run against a given primary checkout at a time. Concurrent runs may try to implement the same issue (because issue selection is not serialized) and may collide on `git worktree add` if their run-stamps collide ([D10](artifacts/decision-log.md#d10-concurrent-runs)).
  - The primary's parent directory is writable, since the worktree is created as a sibling of the primary ([D1](artifacts/decision-log.md#d1-worktree-location)).

## Primary Flow

1. The user adds a `worktrees` block with `enabled: true` to `workflow/config.json` and runs `pr9k` from the primary checkout (or with `--project-dir` pointing at one).
2. pr9k parses CLI flags and resolves the workflow bundle from the primary checkout, then loads `config.json` and reads the `worktrees` block ([T1](artifacts/feature-technical-notes.md#t1-sequencing-constraint)).
3. Because `worktrees.enabled` is true, pr9k creates a new git worktree at `<primary-parent>/<primary-basename>-pr9k-<run-stamp>/` (a sibling of the primary checkout), checked out on a brand-new branch named `pr9k/<run-stamp>` that branches off the primary's current HEAD ([D1](artifacts/decision-log.md#d1-worktree-location), [D2](artifacts/decision-log.md#d2-branch-naming-for-the-run), [T2](artifacts/feature-technical-notes.md#t2-branch-uniqueness)).
4. pr9k internally redirects its "project directory" — the working directory that anchors all subprocess execution, container bind-mounts, log paths, and template variable substitution — from the primary checkout path to the worktree path ([T1](artifacts/feature-technical-notes.md#t1-sequencing-constraint)).
5. pr9k completes startup against the worktree path: the run's log file is opened inside the worktree, the run's iteration log is opened inside the worktree, and preflight checks run against the worktree.
6. The run's log file header records the worktree path, the primary path, and the branch name. The first record written to the run's iteration log records the same fields. These are durable, structured, post-run-greppable artifacts ([D3](artifacts/decision-log.md#d3-log-durability), [D11](artifacts/decision-log.md#d11-worktree-startup-feedback)).
7. The TUI renders, showing the run as it normally would. The TUI's iteration line is prefixed with the worktree's basename so the user can confirm at a glance which checkout the run is operating on ([D11](artifacts/decision-log.md#d11-worktree-startup-feedback)).
8. The workflow loop runs as it always does: each iteration finds the next "ralph"-labeled issue, the feature-work step edits files, the test steps add tests, the issue is summarized and closed, and the run pushes to the remote. All file edits, all `git` operations, and all `gh` operations target the worktree's branch ([T3](artifacts/feature-technical-notes.md#t3-first-push-upstream)).
9. After all iterations, finalization runs (code review, fix review items, update docs, deferred work, lessons learned, final push) — also against the worktree's branch.
10. On graceful completion, pr9k writes a closing record to the run's log file and iteration log (with the worktree path, branch, and how to remove the worktree). It also surfaces, in the TUI's final summary line, the worktree path, the branch name, and the count of all existing `pr9k/*` worktrees on disk so the user knows their backlog ([D11](artifacts/decision-log.md#d11-worktree-startup-feedback)).
11. **If `worktrees.autoCleanup: true` was set**, pr9k now removes the worktree (`git worktree remove --force`) and deletes the run's branch (`git branch -D pr9k/<run-stamp>`) before exiting. The closing log record is written *before* removal so the structured log still records what happened. Because the worktree directory is being deleted, anything that lived only inside it (the log file, iteration log, per-step transcripts) goes with it; users who need post-run logs should leave `autoCleanup: false` or run a workflow without it ([D13](artifacts/decision-log.md#d13-autocleanup-behavior)).
12. **If `worktrees.autoCleanup` is false (default)**, the worktree is left on disk. The user can inspect the worktree, merge or rebase the run's branch, run `pr9k worktree prune` to clean up everything pr9k owns in one step ([D14](artifacts/decision-log.md#d14-pr9k-worktree-prune-subcommand)), or run `git worktree remove <path>` to remove just this one.

## Alternate Flows and States

### The user quits during the run (`q` then `y`, or SIGINT/SIGTERM)

- **Entry condition:** the user requests a graceful quit while the workflow is mid-run.
- **Sequence:** pr9k's existing graceful-shutdown path runs (subprocess termination, log flush). pr9k writes a closing log record (worktree path + branch + removal command) to both the run's log file and iteration log. If `worktrees.autoCleanup: true` was set, pr9k then removes the worktree and deletes the run's branch as part of shutdown; if `false`, both are left on disk.
- **Exit:** with autoCleanup, the worktree directory and the `pr9k/<run-stamp>` branch are gone (so are any uncommitted intermediate files and any logs that lived only in the worktree). Without autoCleanup, the worktree, its uncommitted files, any commits already made on the run's branch, and the log directory all remain on disk for the user to review or discard. The user removes them later with `pr9k worktree prune` or `git worktree remove --force <path>`.

### The pr9k process is killed (SIGKILL, panic, power loss)

- **Entry condition:** pr9k terminates without running its shutdown path.
- **Sequence:** the worktree is left on disk in whatever state it was in, regardless of the `autoCleanup` setting (cleanup needs the shutdown path to run). No closing log record is written.
- **Exit:** the next pr9k run that has `worktrees.enabled: true` and starts in the same primary checkout detects the stale worktree at startup and warns the user. The user decides whether to keep or remove it ([D5](artifacts/decision-log.md#d5-stale-worktree-handling)). The user can also run `pr9k worktree prune` at any time to remove all stale pr9k-owned worktrees in one step ([D14](artifacts/decision-log.md#d14-pr9k-worktree-prune-subcommand)).

### A prior run was killed mid-iteration and the user restarts pr9k

- **Entry condition:** a prior pr9k run was terminated (gracefully, via crash, or via SIGKILL) before completing all of its iterations. One or more "ralph"-labeled GitHub issues that the prior run was working on are still open. The user runs pr9k again against the same primary checkout.
- **Sequence:**
  1. The new run detects any stale pr9k-owned worktrees from prior runs and warns according to D5 (path-prefix-based detection, count-aware aggregation). It does **not** consult or continue any prior run's state.
  2. The new run creates its own fresh worktree at `<primary-parent>/<primary-basename>-pr9k-<new-run-stamp>/` on a fresh branch `pr9k/<new-run-stamp>`. This is a different worktree on a different branch from any prior run.
  3. The new run calls `get_next_issue` and picks up the lowest-numbered open ralph issue. If the prior run was working on issue #N and #N is still open, the new run will pick up #N — **starting from scratch**, with no awareness of any partial work the prior run did.
  4. The new run implements the issue from the primary checkout's HEAD as it currently exists, in its own worktree, on its own branch. None of the prior run's edits, commits, or step transcripts are visible to the new run.
- **Exit:** the user is left with up to two diverging branches (`pr9k/<old-stamp>` and `pr9k/<new-stamp>`), each with its own version of the work. To reconcile, the user inspects both worktrees, decides which version to keep, and either:
  - Picks the new run's branch and removes the old worktree (`pr9k worktree prune` or `git worktree remove --force <old-path>`), or
  - Picks the old run's branch (e.g., if it had progressed further), discards the new run's work, and resumes from there manually.
- **Important consequence — silent skip risk:** if the prior run *completed* an iteration (the close-issue step ran) but its push step failed and was never recovered, the issue is closed on GitHub but no code is on the remote. The new run will skip that issue (because it is closed) and the work survives only inside the prior run's stale worktree. The push-failure error-recovery behavior committed to in [D7](artifacts/decision-log.md#d7-first-push-sets-upstream) is what protects against this happening silently — push failures enter the c/r/q prompt rather than being swallowed, so the user has the chance to retry or quit before the issue gets closed without code being durable on the remote ([D15](artifacts/decision-log.md#d15-no-cross-run-resume)).
- **`resumePrevious` does not bridge across runs:** the existing `resumePrevious` Claude-session resume mechanism is intra-run only — it cannot resume a step from a previous pr9k invocation. Cross-run resume is explicitly out of scope.

### The user runs `pr9k worktree prune`

- **Entry condition:** the user invokes `pr9k worktree prune` (or `pr9k worktree prune --dry-run`) from a primary checkout. This is independent of whether the workflow config sets `worktrees.enabled` — the subcommand is always available.
- **Sequence:**
  1. pr9k discovers existing worktrees and filters for pr9k-owned entries (directory name matches the `<primary-basename>-pr9k-*` pattern), the same filter D5 uses at startup.
  2. **In `--dry-run` mode**, pr9k prints the list of worktrees that would be removed and the branches that would be deleted, then exits. Nothing is changed on disk.
  3. **In normal mode**, pr9k removes each pr9k-owned worktree (`git worktree remove --force <path>`, since pr9k worktrees regularly contain uncommitted intermediate files like `progress.txt`) and deletes its `pr9k/<stamp>` branch. The list of removed paths is printed to stdout.
  4. If a worktree cannot be removed (e.g., a file lock, permissions error), pr9k surfaces the git error verbatim and continues with the rest. The exit code is non-zero if any removal failed ([D14](artifacts/decision-log.md#d14-pr9k-worktree-prune-subcommand)).
- **Exit:** all pr9k-owned worktrees and branches are removed. The primary checkout and any non-pr9k worktrees are untouched.

### `worktrees.enabled` is set but worktree creation fails

- **Entry condition:** `git worktree add` returns a non-zero exit (e.g., the run-stamp path collides, disk is full, permissions are wrong, a prior process locked the worktree).
- **Sequence:** pr9k surfaces the git error to the user with the exact `git worktree add` command and stderr output. pr9k does not fall back to in-place execution — that would silently mutate the primary checkout, which is exactly what the feature exists to prevent.
- **Exit:** pr9k exits with a non-zero status. No partial state is left behind.

### A push step fails (any push within the run)

- **Entry condition:** any `git push` step returns a non-zero exit. The most likely causes are: a fresh branch with no upstream tracking ref (the script must establish one on first push), branch-protection rejection, network failure, or auth failure.
- **Sequence:** pr9k enters its existing error-recovery mode. The TUI presents the standard `c` (continue), `r` (retry), `q` (quit) prompt. The full stderr from the push is written to the iteration log and the run's log file. The run does not silently advance to the next step ([D7](artifacts/decision-log.md#d7-first-push-sets-upstream), [T3](artifacts/feature-technical-notes.md#t3-first-push-upstream)).
- **Exit:** the user resolves the underlying problem (sets the upstream, adjusts branch protection, fixes auth) and chooses retry, or quits. If the first push fails and is not resolved, every subsequent push in the same run will also fail because no upstream tracking ref was ever established — the run can no longer make code durable on the remote.

### Stale `pr9k/...` worktrees from prior crashed runs are detected at startup

- **Entry condition:** at startup, pr9k discovers one or more existing worktrees whose path indicates pr9k ownership (the worktree directory name matches the `<primary-basename>-pr9k-*` pattern), regardless of branch state.
- **Sequence:** pr9k counts the stale entries. When the count is small (≤ 5), pr9k logs each stale worktree path on its own line in the TUI and the log file. When the count is larger, pr9k prints a single summary line in the TUI ("N stale pr9k worktrees found — run `git worktree list` to review") and writes the individual paths only to the log file. pr9k does not auto-remove any of them. pr9k proceeds with creating its own fresh worktree ([D5](artifacts/decision-log.md#d5-stale-worktree-handling), [D11](artifacts/decision-log.md#d11-worktree-startup-feedback)).
- **Exit:** the new run runs normally; the user is responsible for cleaning up stale worktrees themselves with `git worktree remove`.

### The user passes `--project-dir <path>` while `worktrees.enabled` is true

- **Entry condition:** explicit project-dir override combined with `worktrees.enabled: true`.
- **Sequence:** pr9k resolves `--project-dir` to the "primary" path, then creates the worktree as a sibling of that path. The worktree path becomes the working directory for the run.
- **Exit:** identical to the standard primary flow — the user's `--project-dir` value is treated as "the primary checkout."

## Edge Cases and Failure Modes

| Condition | Required Behavior |
|-----------|-------------------|
| `worktrees` block is absent or `worktrees.enabled` is `false` | The default in-place behavior is used; nothing about the run changes from today. ([D8](artifacts/decision-log.md#d8-default-and-config-shape)) |
| `worktrees.enabled: true` but the target is not a git repository | pr9k surfaces the git error from `git worktree add` and exits non-zero. |
| `worktrees.autoCleanup: true` and the run completes gracefully | The worktree is removed and its branch is deleted before pr9k exits. The TUI's final summary shows the path that was removed. ([D13](artifacts/decision-log.md#d13-autocleanup-behavior)) |
| `worktrees.autoCleanup: true` and the run is killed (SIGKILL/panic) | Cleanup needs the shutdown path to run, so the worktree is left on disk in this case. The next run detects it as stale or `pr9k worktree prune` removes it. ([D13](artifacts/decision-log.md#d13-autocleanup-behavior)) |
| `worktrees.autoCleanup` is set without `worktrees.enabled` | Validator rejects the config at startup with a clear message — `autoCleanup` is meaningless without `enabled`. ([D13](artifacts/decision-log.md#d13-autocleanup-behavior)) |
| `pr9k worktree prune` is invoked from a path that is not a git repository | pr9k surfaces the underlying git error and exits non-zero. ([D14](artifacts/decision-log.md#d14-pr9k-worktree-prune-subcommand)) |
| `pr9k worktree prune` is invoked when no pr9k-owned worktrees exist | pr9k prints "No pr9k-owned worktrees found" and exits zero. ([D14](artifacts/decision-log.md#d14-pr9k-worktree-prune-subcommand)) |
| `pr9k worktree prune --dry-run` is invoked | pr9k lists what would be removed and exits zero without changing anything. ([D14](artifacts/decision-log.md#d14-pr9k-worktree-prune-subcommand)) |
| The primary checkout has uncommitted changes | The new worktree is created normally; the primary's uncommitted changes are not visible inside the worktree. (Worktrees do not share working-tree state.) |
| The primary is on a detached HEAD | The new worktree is created off that HEAD as a fresh branch named `pr9k/<run-stamp>`. ([D2](artifacts/decision-log.md#d2-branch-naming-for-the-run)) |
| A branch named `pr9k/<run-stamp>` already exists in the repository | Run-stamp is millisecond-precision and unique per run. If a collision occurs anyway (clock skew, manual creation, or two concurrent pr9k instances), pr9k surfaces the git error and exits — it does not force-reset an existing branch. |
| The worktree directory `<primary-parent>/<primary-basename>-pr9k-<run-stamp>/` already exists | The run-stamp is unique. If a collision occurs, pr9k surfaces the git error and exits without modifying the existing path. |
| The first `git push` of the run's branch happens (no upstream tracking ref yet) | The push step establishes a tracking ref so the push succeeds and subsequent pushes work without intervention; the change to the push step also surfaces non-zero exits. ([D7](artifacts/decision-log.md#d7-first-push-sets-upstream), [T3](artifacts/feature-technical-notes.md#t3-first-push-upstream)) |
| The remote rejects the push (branch protection, auth, network) | pr9k enters its standard error-recovery mode (c/r/q prompt) with the verbatim git error written to the iteration log. ([D7](artifacts/decision-log.md#d7-first-push-sets-upstream)) |
| The user manually deletes the `pr9k/<run-stamp>` branch but leaves the worktree directory on disk | The next run still detects and warns about the stale worktree because stale detection filters by directory name, not branch name. ([D5](artifacts/decision-log.md#d5-stale-worktree-handling)) |
| Disk fills mid-run because accumulated worktrees from prior runs have consumed space | pr9k has no special handling; the run fails with a filesystem error from the affected subprocess and enters error-recovery mode. The user must clean up stale worktrees with `git worktree remove` to recover space, then retry. |
| The repository contains git submodules | The worktree is created without initialized submodule contents. The user is responsible for initializing submodules manually inside the worktree if their workflow requires them. The limitation is documented in the user-facing how-to guide; pr9k does not warn at startup. ([D6](artifacts/decision-log.md#d6-submodule-policy)) |
| The user invokes `pr9k sandbox shell` or `pr9k workflow` while `worktrees.enabled` is true | These subcommands ignore the `worktrees` block and operate against the user's current working directory as they do today. The toggle only affects the workflow runner. ([D9](artifacts/decision-log.md#d9-subcommand-scope)) |
| The user runs pr9k a second time while one or more issues from a prior killed run are still open | The new run does not continue prior work. It picks up the next open issue from scratch in a fresh worktree on a fresh branch. The user is responsible for reconciling the divergent branches. See the dedicated alternate flow above. ([D15](artifacts/decision-log.md#d15-no-cross-run-resume)) |
| Logs and artifacts on completion | Log file, iteration log, and per-step transcripts persist inside the worktree's `.pr9k/logs/` even after the run ends, until the user removes the worktree. ([D3](artifacts/decision-log.md#d3-log-durability)) |
| The user manually deletes the worktree directory mid-run | The run fails with a filesystem error from the next subprocess that touches the worktree. If the error reaches pr9k's standard error path, the c/r/q prompt is presented; if the error is fatal at the process level, the TUI exits without recovery. This scenario is unsupported. |

## User Interactions

- **Affordances:**
  - A config-file block, no CLI flag: `worktrees: { enabled: true, autoCleanup: false }` at the top of `config.json`.
  - A new `pr9k worktree prune` subcommand (with optional `--dry-run`) for one-step bulk removal of all pr9k-owned worktrees and branches.
  - The TUI's iteration line is prefixed with the worktree's basename during the run, so the user can confirm at a glance which checkout the run is operating on.
  - The TUI's final summary line includes the worktree path, the run's branch name, and the count of all `pr9k/*` worktrees on disk.
- **Feedback:**
  - At startup, if `worktrees.enabled: true`, pr9k prints a single line indicating the worktree was created and where.
  - At startup, if stale `pr9k/...` worktrees were detected, the count is shown. Up to 5 stale paths are listed inline; above that, a summary line tells the user to run `git worktree list` or `pr9k worktree prune`. Individual paths are always written to the run's log file regardless of count.
  - At end-of-run, the worktree path, branch, and worktree-count appear in the TUI's final summary line and are written to the run's log file and iteration log. If `autoCleanup` ran, the summary also reports that the worktree was removed.
  - `pr9k worktree prune` prints one line per removed worktree and a final count.
- **Error states:**
  - Worktree-creation failure surfaces the underlying `git` error verbatim — the user sees enough information to debug.
  - Push failure surfaces the verbatim git error and enters pr9k's standard c/r/q error-recovery mode; the run does not silently continue.
  - `pr9k worktree prune` failures (per-worktree) surface the underlying error and the subcommand exits non-zero, but it always tries every candidate so a single failure does not block the rest.

## Coordinations

| Coordinating System | Direction | Interaction | Ordering / Consistency Requirement |
|---------------------|-----------|-------------|-----------------------------------|
| Git (local) | outbound | `git worktree add`, `git worktree list`, `git worktree remove`, `git branch -D` | Worktree must be created after the workflow bundle resolves but before the runtime directory is created and the workflow runner is constructed ([T1](artifacts/feature-technical-notes.md#t1-sequencing-constraint)). Cleanup (autoCleanup or `pr9k worktree prune`) removes worktrees with `--force` because they routinely contain uncommitted intermediate workflow files. |
| Git (remote) — first push of the run's branch | outbound | `git push` with upstream-set semantics | First push must establish a remote tracking ref for `pr9k/<run-stamp>`; subsequent pushes use the established upstream. On non-zero exit, pr9k enters error-recovery mode; the run does not auto-continue ([T3](artifacts/feature-technical-notes.md#t3-first-push-upstream), [D7](artifacts/decision-log.md#d7-first-push-sets-upstream)) |
| GitHub (via `gh` CLI) | outbound | Issue listing, closing, summary posting, image fetches | All `gh` operations resolve the repository from the worktree's git state, which is the same repository as the primary's. No `gh` configuration changes are needed. |
| Docker sandbox | outbound | The container that runs `claude` for each step bind-mounts the project directory at `/home/agent/workspace`. | The bind-mount source is the worktree path while `worktrees.enabled` is true; the in-container path is unchanged. The worktree's location as a sibling of the primary keeps the bind-mount path inside the user's home directory hierarchy, which is the default-shared filesystem on macOS Docker Desktop. |
| `pr9k workflow` (workflow-builder TUI) and `pr9k sandbox shell` | none | These subcommands resolve their own working directory independently and ignore the `worktrees` block. ([D9](artifacts/decision-log.md#d9-subcommand-scope)) | — |
| `pr9k worktree prune` subcommand | invoked manually by user | Lists pr9k-owned worktrees (path-prefix filter, same as D5) and removes them with `git worktree remove --force`; deletes their `pr9k/*` branches. | Independent of `worktrees.enabled`; safe to run from any primary checkout. ([D14](artifacts/decision-log.md#d14-pr9k-worktree-prune-subcommand)) |

## Out of Scope

- Per-iteration worktrees (one worktree per ralph issue). The default workflow does not switch branches per issue, so a per-iteration model has no use today. ([D2](artifacts/decision-log.md#d2-branch-naming-for-the-run))
- Automated cleanup of stale worktrees at startup. pr9k warns and lets the user decide. ([D5](artifacts/decision-log.md#d5-stale-worktree-handling))
- Auto-initialization of submodules in the new worktree. The first iteration documents the limitation in the how-to guide; auto-init can be added later if a user reports it as a blocker. ([D6](artifacts/decision-log.md#d6-submodule-policy))
- Configurable worktree location. The first iteration ships a single fixed location (sibling of the primary); a config option to relocate worktrees can be added later if a user reports a real conflict. ([D1](artifacts/decision-log.md#d1-worktree-location))
- Adding a `primaryDir` field to the statusline payload. ([D12](artifacts/decision-log.md#d12-statusline-payload-shape))
- Cross-run resume — pr9k does not persist run state between invocations and does not attempt to continue a prior killed run's iteration. The user reconciles divergent branches manually. ([D15](artifacts/decision-log.md#d15-no-cross-run-resume))
- Behavior change for `pr9k sandbox shell` and `pr9k workflow`. ([D9](artifacts/decision-log.md#d9-subcommand-scope))
- Concurrent-run serialization (a `.pr9k/pr9k.lock` file or similar). ([D10](artifacts/decision-log.md#d10-concurrent-runs))

## Deferred (YAGNI)

### `logDir` split (logs in primary, work in worktree)
- **Why deferred:** simpler-version test — keeping the worktree on disk by default already satisfies the evidence (logs preserved for the user to review). When `autoCleanup: true` is set, logs intentionally vanish with the worktree; that is the user's chosen tradeoff. Splitting `logDir` would be a multi-file change for no behavioral gain in the default case and would defeat the point of `autoCleanup` for users who chose it.
- **Reopen when:** users with `autoCleanup: true` report that they want post-run logs preserved despite the cleanup, AND no simpler workaround (e.g., per-run log archival) suffices.
- **Source:** investigation open question Q1, validation finding V7.

### Submodule auto-initialization
- **Why deferred:** evidence test — pr9k itself has no submodules, the default workflow does not exercise them, and no user has reported a submodule-using project as a target.
- **Reopen when:** a user reports a submodule-using project failing inside a worktree.
- **Source:** validation finding V10; team finding F-13.

### Submodule startup warning
- **Why deferred:** evidence test — emitting a startup warning for a class of user that has not yet been reported is observability machinery for a failure mode that has never occurred. The limitation is documented in the user-facing how-to guide instead.
- **Reopen when:** a user reports a submodule-using project failing without being able to find the limitation in the docs.
- **Source:** team finding F-13.

### Statusline payload `primaryDir` field
- **Why deferred:** evidence test — no user has reported that their statusline script needs both paths. The worktree path appears in the existing `projectDir` field.
- **Reopen when:** a statusline-script author reports the missing context as a real problem.
- **Source:** validation finding V11.

### Concurrent-run lock
- **Why deferred:** evidence test — no user has reported an accidental concurrent run. The failure mode (issue duplication, branch collision) surfaces with a clear git error if it happens.
- **Reopen when:** a user reports a real concurrent-run incident OR pr9k starts to be invoked from automation (e.g., scheduled jobs) where overlap is plausible.
- **Source:** team finding F-04.

### Mid-run disk-full proactive warning
- **Why deferred:** evidence test — no user has reported a mid-run disk-full incident, and the existing behavior (filesystem error → error-recovery mode) is acceptable.
- **Reopen when:** a user reports being unable to diagnose a mid-run failure as disk-full.
- **Source:** team finding F-06.

## Open Items

- **OI-1:** The push-step change required by [D7](artifacts/decision-log.md#d7-first-push-sets-upstream) is shared with the in-place workflow path. The behavioral question this raises — should push failures in the in-place workflow also enter pr9k's error-recovery mode (c/r/q), consistent with the behavior being added for `useWorktrees` runs? — has not been settled. The recommended answer is yes: failed pushes are dangerous regardless of mode, and surfacing them is uniformly correct.
  - **Resolves when:** the user accepts or amends the recommendation.
  - **Blocks implementation:** No. The push behavior change is in scope for this feature regardless; the open question is only whether the change applies to in-place runs as well, which is a small extension.

- **OI-2:** The default workflow's finalize phase writes its code-review output to a path that the `review_verdict` script reads from the working directory. The investigation noted (validation Remaining Risk #3) that if the finalize-phase output is written to a subdirectory, the verdict step silently skips. This is a pre-existing inconsistency that becomes more visible under `useWorktrees` because the working directory is no longer the user's familiar primary checkout. The fix is to confirm both ends of the path agree before shipping the worktree feature.
  - **Resolves when:** implementation review confirms the finalize phase writes to the path the verdict reads, in both in-place and worktree modes.
  - **Blocks implementation:** No, but ships a known silent-skip risk if not confirmed.

## Summary

- **Outcome delivered:** users can run pr9k against the next feature without disrupting their primary checkout, with all run artifacts (commits, logs, transcripts) preserved on disk by default or removed automatically when the user opts into `autoCleanup`. A `pr9k worktree prune` subcommand makes manual bulk cleanup a single command.
- **Primary actors:** the pr9k user; downstream consumers are git, GitHub (via `gh`), and the existing Docker sandbox.
- **Decisions settled by evidence:** 14 (D1–D14) — see [artifacts/decision-log.md](artifacts/decision-log.md).
- **Decisions settled by user input:** 1 (D8 reshaped + D13, D14, D15 added per user redirect on cleanup ergonomics) — see [artifacts/decision-log.md](artifacts/decision-log.md).
- **Sub-agents consulted:** junior-developer, devops-engineer — see [artifacts/team-findings.md](artifacts/team-findings.md).
- **Key adjustments from review:** worktree relocated from inside `.git/` to a sibling of the primary (Docker bind-mount safety on macOS); push failures explicitly enter error-recovery mode; stale-detection filter switched to path-prefix only; submodule startup warning removed; observability extended to log file and iteration log; concurrent-run and branch-protection preconditions added.
- **Remaining open items:** 2 (OI-1, OI-2 — neither blocks implementation).
- **Technical notes:** 3 — see [artifacts/feature-technical-notes.md](artifacts/feature-technical-notes.md).
