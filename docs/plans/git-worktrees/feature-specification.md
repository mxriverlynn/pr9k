# Feature Specification: useWorktrees

When `useWorktrees: true` is set at the top of the workflow config, pr9k runs the entire workflow inside a freshly created git worktree so the user's primary checkout stays usable for parallel development while pr9k mutates code on a separate branch.

## Background (for readers new to git worktrees)

A git worktree is a second on-disk working directory backed by the same repository. The repository's commits, branches, remotes, and config are shared across all worktrees, but each worktree has its own checked-out branch, its own working files, and its own staging area. Two worktrees cannot have the same branch checked out at the same time, so pr9k always creates a fresh branch when it makes a worktree — that is the only way to give the worktree a branch the primary doesn't already have. Creating a worktree is fast (no re-clone), and removing one leaves the rest of the repository untouched. This is the mechanism that lets pr9k operate on the next feature in one directory while the user keeps running the app from another.

A **run-stamp** is the millisecond-precision identifier pr9k already uses to name each run's log directory (e.g., `ralph-2026-05-07-143022.123`). This same stamp is reused throughout the worktree feature: the worktree directory, the run's branch name, and the log directory all share it, so a user inspecting `git worktree list` or `git branch --list 'pr9k/*'` can correlate worktrees back to runs. The stamp's source of truth is documented in [`docs/code-packages/logger.md`](../../code-packages/logger.md).

## Outcome

- pr9k's commits, file edits, and branch changes happen in a separate working directory; the user's primary checkout's working tree is unchanged on disk and stays on its original branch ([D1](artifacts/decision-log.md#d1-worktree-location)).
- The user can run, build, and test the application from the primary checkout while pr9k iterates.
- All commits pr9k produces during the run land on a single, traceable branch tied to that run, and that branch is pushed to the remote so the work is durable even if the worktree is later removed ([D2](artifacts/decision-log.md#d2-branch-naming-for-the-run)).
- Run-time observability — log files, the iteration log, per-step transcripts — is preserved after the run completes so the user can review what pr9k did. The worktree path and the run's branch name are written to both the run's log file (header) and the run's iteration log (first record) so they are greppable from structured output, not only from the TUI ([D3](artifacts/decision-log.md#d3-log-durability)).
- The default behavior of pr9k is unchanged: workflows that do not set `useWorktrees: true` continue to operate in-place exactly as before ([D8](artifacts/decision-log.md#d8-default-and-config-shape)).

### Invariants

- pr9k must not write to, change branches in, or otherwise mutate the primary checkout's working tree while the run is in progress. This is a hard constraint, enforced behaviorally by the worktree redirect ([T1](artifacts/feature-technical-notes.md#t1-sequencing-constraint)).

## Actors and Triggers

- **Actor:** the pr9k user — a developer who wants to keep using their primary checkout (running the app, testing other features, pairing on a separate branch) while pr9k iterates on the next feature.
- **Trigger:** the user sets `useWorktrees: true` at the top level of `workflow/config.json` and runs `pr9k` (with or without `-n`, `--project-dir`, or `--workflow-dir`). The toggle is read from the workflow config; there is no CLI flag for it. When `--project-dir <path>` is also passed, the resolved `--project-dir` is treated as "the primary checkout," and the worktree is created relative to that path.
- **Preconditions:**
  - The target repository is a git repository (already required by pr9k today).
  - `git` is installed and on PATH (already required).
  - The primary checkout has a resolvable HEAD (any branch, any commit, including a detached HEAD).
  - The primary checkout's working tree may have uncommitted changes — worktrees are independent of the primary's working state, so this is allowed.
  - The remote permits pushing to branch names beginning with `pr9k/`. Branch-protection rules that reject this pattern will cause the run's first push to fail ([D7](artifacts/decision-log.md#d7-first-push-sets-upstream)).
  - Only one pr9k instance should run against a given primary checkout at a time. Concurrent runs may try to implement the same issue (because issue selection is not serialized) and may collide on `git worktree add` if their run-stamps collide ([D10](artifacts/decision-log.md#d10-concurrent-runs)).
  - The primary's parent directory is writable, since the worktree is created as a sibling of the primary ([D1](artifacts/decision-log.md#d1-worktree-location)).

## Primary Flow

1. The user adds `useWorktrees: true` to the top of `workflow/config.json` and runs `pr9k` from the primary checkout (or with `--project-dir` pointing at one).
2. pr9k parses CLI flags and resolves the workflow bundle from the primary checkout, then loads `config.json` and reads the `useWorktrees` setting ([T1](artifacts/feature-technical-notes.md#t1-sequencing-constraint)).
3. Because `useWorktrees` is true, pr9k creates a new git worktree at `<primary-parent>/<primary-basename>-pr9k-<run-stamp>/` (a sibling of the primary checkout), checked out on a brand-new branch named `pr9k/<run-stamp>` that branches off the primary's current HEAD ([D1](artifacts/decision-log.md#d1-worktree-location), [D2](artifacts/decision-log.md#d2-branch-naming-for-the-run), [T2](artifacts/feature-technical-notes.md#t2-branch-uniqueness)).
4. pr9k internally redirects its "project directory" — the working directory that anchors all subprocess execution, container bind-mounts, log paths, and template variable substitution — from the primary checkout path to the worktree path.
5. pr9k completes startup against the worktree path: the run's log file is opened inside the worktree, the run's iteration log is opened inside the worktree, and preflight checks run against the worktree.
6. The run's log file header records the worktree path, the primary path, and the branch name. The first record written to the run's iteration log records the same fields. These are durable, structured, post-run-greppable artifacts ([D3](artifacts/decision-log.md#d3-log-durability), [D11](artifacts/decision-log.md#d11-worktree-startup-feedback)).
7. The TUI renders, showing the run as it normally would. The TUI's iteration line is prefixed with the worktree's basename so the user can confirm at a glance which checkout the run is operating on ([D11](artifacts/decision-log.md#d11-worktree-startup-feedback)).
8. The workflow loop runs as it always does: each iteration finds the next "ralph"-labeled issue, the feature-work step edits files, the test steps add tests, the issue is summarized and closed, and the run pushes to the remote. All file edits, all `git` operations, and all `gh` operations target the worktree's branch ([T3](artifacts/feature-technical-notes.md#t3-first-push-upstream)).
9. After all iterations, finalization runs (code review, fix review items, update docs, deferred work, lessons learned, final push) — also against the worktree's branch.
10. On graceful completion, pr9k writes a closing record to the run's log file and iteration log (with the worktree path, branch, and how to remove the worktree). It also surfaces, in the TUI's final summary line, the worktree path, the branch name, and the count of all existing `pr9k/*` worktrees on disk so the user knows their backlog ([D11](artifacts/decision-log.md#d11-worktree-startup-feedback)).
11. The worktree is left on disk by default ([D4](artifacts/decision-log.md#d4-cleanup-policy)). The user can inspect the worktree, merge or rebase the run's branch, or run `git worktree remove <path>` to clean up when finished.

## Alternate Flows and States

### The user quits during the run (`q` then `y`, or SIGINT/SIGTERM)

- **Entry condition:** the user requests a graceful quit while the workflow is mid-run.
- **Sequence:** pr9k's existing graceful-shutdown path runs (subprocess termination, log flush). The worktree is left on disk; pr9k writes a closing log record (worktree path + branch + removal command) to both the run's log file and iteration log.
- **Exit:** the worktree, its uncommitted files, any commits already made on the run's branch, and the log directory all remain on disk for the user to review or discard. The user removes the worktree with `git worktree remove --force <path>` when done.

### The pr9k process is killed (SIGKILL, panic, power loss)

- **Entry condition:** pr9k terminates without running its shutdown path.
- **Sequence:** the worktree is left on disk in whatever state it was in. No closing log record is written.
- **Exit:** the next pr9k run that has `useWorktrees: true` and starts in the same primary checkout detects the stale worktree at startup and warns the user. The user decides whether to keep or remove it ([D5](artifacts/decision-log.md#d5-stale-worktree-handling)).

### `useWorktrees` is set but worktree creation fails

- **Entry condition:** `git worktree add` returns a non-zero exit (e.g., the run-stamp path collides, disk is full, permissions are wrong, a prior process locked the worktree).
- **Sequence:** pr9k surfaces the git error to the user with the exact `git worktree add` command and stderr output. pr9k does not fall back to in-place execution — that would silently mutate the primary checkout, which is exactly what the feature exists to prevent.
- **Exit:** pr9k exits with a non-zero status. No partial state is left behind.

### A push step fails (any push within the run)

- **Entry condition:** any `git push` step returns a non-zero exit. The most likely causes are: a fresh branch with no upstream tracking ref (the script must establish one on first push), branch-protection rejection, network failure, or auth failure.
- **Sequence:** pr9k enters its existing error-recovery mode. The TUI presents the standard `c` (continue), `r` (retry), `q` (quit) prompt. The full stderr from the push is written to the iteration log and the run's log file. The run does not silently advance to the next step ([D7](artifacts/decision-log.md#d7-first-push-sets-upstream), [T3](artifacts/feature-technical-notes.md#t3-first-push-upstream)).
- **Exit:** the user resolves the underlying problem (sets the upstream, adjusts branch protection, fixes auth) and chooses retry, or quits. If the first push fails and is not resolved, every subsequent push in the same run will also fail because no upstream tracking ref was ever established — the run can no longer make code durable on the remote.

### Stale `pr9k/...` worktrees from prior crashed runs are detected at startup

- **Entry condition:** at startup, `git worktree list --porcelain` shows one or more existing worktrees whose path indicates pr9k ownership (the worktree directory name matches the `<primary-basename>-pr9k-*` pattern), regardless of branch state.
- **Sequence:** pr9k counts the stale entries. When the count is small (≤ 5), pr9k logs each stale worktree path on its own line in the TUI and the log file. When the count is larger, pr9k prints a single summary line in the TUI ("N stale pr9k worktrees found — run `git worktree list` to review") and writes the individual paths only to the log file. pr9k does not auto-remove any of them. pr9k proceeds with creating its own fresh worktree ([D5](artifacts/decision-log.md#d5-stale-worktree-handling), [D11](artifacts/decision-log.md#d11-worktree-startup-feedback)).
- **Exit:** the new run runs normally; the user is responsible for cleaning up stale worktrees themselves with `git worktree remove`.

### The user passes `--project-dir <path>` while `useWorktrees` is true

- **Entry condition:** explicit project-dir override combined with `useWorktrees: true`.
- **Sequence:** pr9k resolves `--project-dir` to the "primary" path, then creates the worktree as a sibling of that path. The worktree path becomes the working directory for the run.
- **Exit:** identical to the standard primary flow — the user's `--project-dir` value is treated as "the primary checkout."

## Edge Cases and Failure Modes

| Condition | Required Behavior |
|-----------|-------------------|
| `useWorktrees: false` or absent in config | The default in-place behavior is used; nothing about the run changes from today. ([D8](artifacts/decision-log.md#d8-default-and-config-shape)) |
| `useWorktrees: true` but the target is not a git repository | pr9k surfaces the git error from `git worktree add` and exits non-zero. |
| The primary checkout has uncommitted changes | The new worktree is created normally; the primary's uncommitted changes are not visible inside the worktree. (Worktrees do not share working-tree state.) |
| The primary is on a detached HEAD | The new worktree is created off that HEAD as a fresh branch named `pr9k/<run-stamp>`. ([D2](artifacts/decision-log.md#d2-branch-naming-for-the-run)) |
| A branch named `pr9k/<run-stamp>` already exists in the repository | Run-stamp is millisecond-precision and unique per run. If a collision occurs anyway (clock skew, manual creation, or two concurrent pr9k instances), pr9k surfaces the git error and exits — it does not force-reset an existing branch. |
| The worktree directory `<primary-parent>/<primary-basename>-pr9k-<run-stamp>/` already exists | The run-stamp is unique. If a collision occurs, pr9k surfaces the git error and exits without modifying the existing path. |
| The first `git push` of the run's branch happens (no upstream tracking ref yet) | The push step establishes a tracking ref so the push succeeds and subsequent pushes work without intervention; the change to the push step also surfaces non-zero exits. ([D7](artifacts/decision-log.md#d7-first-push-sets-upstream), [T3](artifacts/feature-technical-notes.md#t3-first-push-upstream)) |
| The remote rejects the push (branch protection, auth, network) | pr9k enters its standard error-recovery mode (c/r/q prompt) with the verbatim git error written to the iteration log. ([D7](artifacts/decision-log.md#d7-first-push-sets-upstream)) |
| The user manually deletes the `pr9k/<run-stamp>` branch but leaves the worktree directory on disk | The next run still detects and warns about the stale worktree because stale detection filters by directory name, not branch name. ([D5](artifacts/decision-log.md#d5-stale-worktree-handling)) |
| Disk fills mid-run because accumulated worktrees from prior runs have consumed space | pr9k has no special handling; the run fails with a filesystem error from the affected subprocess and enters error-recovery mode. The user must clean up stale worktrees with `git worktree remove` to recover space, then retry. |
| The repository contains git submodules | The worktree is created without initialized submodule contents. The user is responsible for initializing submodules manually inside the worktree if their workflow requires them. The limitation is documented in the user-facing how-to guide; pr9k does not warn at startup. ([D6](artifacts/decision-log.md#d6-submodule-policy)) |
| The user invokes `pr9k sandbox shell` or `pr9k workflow` while `useWorktrees` is true | These subcommands ignore `useWorktrees` and operate against the user's current working directory as they do today. The toggle only affects the workflow runner. ([D9](artifacts/decision-log.md#d9-subcommand-scope)) |
| Logs and artifacts on completion | Log file, iteration log, and per-step transcripts persist inside the worktree's `.pr9k/logs/` even after the run ends, until the user removes the worktree. ([D3](artifacts/decision-log.md#d3-log-durability)) |
| The user manually deletes the worktree directory mid-run | The run fails with a filesystem error from the next subprocess that touches the worktree. If the error reaches pr9k's standard error path, the c/r/q prompt is presented; if the error is fatal at the process level, the TUI exits without recovery. This scenario is unsupported. |

## User Interactions

- **Affordances:**
  - One config-file toggle, no CLI flag: `useWorktrees: true` at the top of `config.json`.
  - The TUI's iteration line is prefixed with the worktree's basename during the run, so the user can confirm at a glance which checkout the run is operating on.
  - The TUI's final summary line includes the worktree path, the run's branch name, and the count of all `pr9k/*` worktrees on disk.
- **Feedback:**
  - At startup, if `useWorktrees: true`, pr9k prints a single line indicating the worktree was created and where.
  - At startup, if stale `pr9k/...` worktrees were detected, the count is shown. Up to 5 stale paths are listed inline; above that, a summary line tells the user to run `git worktree list`. Individual paths are always written to the run's log file regardless of count.
  - At end-of-run, the worktree path, branch, and worktree-count appear in the TUI's final summary line and are written to the run's log file and iteration log.
- **Error states:**
  - Worktree-creation failure surfaces the underlying `git` error verbatim — the user sees enough information to debug.
  - Push failure surfaces the verbatim git error and enters pr9k's standard c/r/q error-recovery mode; the run does not silently continue.

## Coordinations

| Coordinating System | Direction | Interaction | Ordering / Consistency Requirement |
|---------------------|-----------|-------------|-----------------------------------|
| Git (local) | outbound | `git worktree add`, `git worktree list`, `git worktree remove` | Worktree must be created after the workflow bundle resolves but before the runtime directory is created and the workflow runner is constructed ([T1](artifacts/feature-technical-notes.md#t1-sequencing-constraint)) |
| Git (remote) — first push of the run's branch | outbound | `git push` with upstream-set semantics | First push must establish a remote tracking ref for `pr9k/<run-stamp>`; subsequent pushes use the established upstream. On non-zero exit, pr9k enters error-recovery mode; the run does not auto-continue ([T3](artifacts/feature-technical-notes.md#t3-first-push-upstream), [D7](artifacts/decision-log.md#d7-first-push-sets-upstream)) |
| GitHub (via `gh` CLI) | outbound | Issue listing, closing, summary posting, image fetches | All `gh` operations resolve the repository from the worktree's git state, which is the same repository as the primary's. No `gh` configuration changes are needed. |
| Docker sandbox | outbound | The container that runs `claude` for each step bind-mounts the project directory at `/home/agent/workspace`. | The bind-mount source is the worktree path while `useWorktrees` is true; the in-container path is unchanged. The worktree's location as a sibling of the primary keeps the bind-mount path inside the user's home directory hierarchy, which is the default-shared filesystem on macOS Docker Desktop. |
| `pr9k workflow` (workflow-builder TUI) and `pr9k sandbox shell` | none | These subcommands resolve their own working directory independently and ignore `useWorktrees`. ([D9](artifacts/decision-log.md#d9-subcommand-scope)) | — |

## Out of Scope

- Per-iteration worktrees (one worktree per ralph issue). The default workflow does not switch branches per issue, so a per-iteration model has no use today. ([D2](artifacts/decision-log.md#d2-branch-naming-for-the-run))
- Automated cleanup of stale worktrees at startup. pr9k warns and lets the user decide. ([D5](artifacts/decision-log.md#d5-stale-worktree-handling))
- Auto-initialization of submodules in the new worktree. The first iteration documents the limitation in the how-to guide; auto-init can be added later if a user reports it as a blocker. ([D6](artifacts/decision-log.md#d6-submodule-policy))
- Configurable worktree location. The first iteration ships a single fixed location (sibling of the primary); a config option to relocate worktrees can be added later if a user reports a real conflict. ([D1](artifacts/decision-log.md#d1-worktree-location))
- Bulk cleanup of stale worktrees automated by pr9k. `git worktree remove` already handles individual removals; see Deferred.
- Adding a `primaryDir` field to the statusline payload. ([D12](artifacts/decision-log.md#d12-statusline-payload-shape))
- Behavior change for `pr9k sandbox shell` and `pr9k workflow`. ([D9](artifacts/decision-log.md#d9-subcommand-scope))
- Concurrent-run serialization (a `.pr9k/pr9k.lock` file or similar). ([D10](artifacts/decision-log.md#d10-concurrent-runs))

## Deferred (YAGNI)

### Configurable cleanup-on-success policy
- **Why deferred:** simpler-version test — the strictly simpler version is "always keep the worktree on disk after the run." The user can remove it manually with `git worktree remove`. No user has asked for auto-removal.
- **Reopen when:** a user reports that worktrees accumulate to the point of disk-space or workflow-friction concerns, OR an automation use case (e.g., CI) emerges where leaving the worktree on disk is wrong.
- **Source:** investigation open question Q3 (cleanup policy).

### `logDir` split (logs in primary, work in worktree)
- **Why deferred:** simpler-version test — keeping the worktree on disk by default already satisfies the evidence (logs preserved for the user to review). Splitting `logDir` would be a multi-file change for no additional behavior.
- **Reopen when:** the cleanup-on-success policy is reopened (above) AND auto-remove becomes the default. At that point logs would vanish without a `logDir` split.
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

### `pr9k worktree prune` subcommand
- **Why deferred:** simpler-version test — the existing `git worktree remove` and `git worktree prune` commands already do the job; pr9k surfaces a warning so the user knows where to look.
- **Reopen when:** users report that running `git worktree` commands by hand is confusing or error-prone, OR stale worktrees accumulate fast enough that bulk cleanup becomes a real friction.
- **Source:** investigation open question Q5.

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

- **Outcome delivered:** users can run pr9k against the next feature without disrupting their primary checkout, with all run artifacts (commits, logs, transcripts) preserved on disk and traceable to a single per-run branch.
- **Primary actors:** the pr9k user; downstream consumers are git, GitHub (via `gh`), and the existing Docker sandbox.
- **Decisions settled by evidence:** 12 — see [artifacts/decision-log.md](artifacts/decision-log.md).
- **Decisions settled by user input:** 0 (auto-mode interview; the user can redirect any decision via review).
- **Sub-agents consulted:** junior-developer, devops-engineer — see [artifacts/team-findings.md](artifacts/team-findings.md).
- **Key adjustments from review:** worktree relocated from inside `.git/` to a sibling of the primary (Docker bind-mount safety on macOS); push failures explicitly enter error-recovery mode; stale-detection filter switched to path-prefix only; submodule startup warning removed; observability extended to log file and iteration log; concurrent-run and branch-protection preconditions added.
- **Remaining open items:** 2 (OI-1, OI-2 — neither blocks implementation).
- **Technical notes:** 3 — see [artifacts/feature-technical-notes.md](artifacts/feature-technical-notes.md).
