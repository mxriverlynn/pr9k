# Feature Specification: useWorktrees

When `useWorktrees: true` is set at the top of the workflow config, pr9k runs the entire workflow inside a freshly created git worktree so the user's primary checkout stays usable for parallel development while pr9k mutates code on a separate branch.

## Background (for readers new to git worktrees)

A git worktree is a second on-disk working directory backed by the same repository. The repository's commits, branches, remotes, and config are shared across all worktrees, but each worktree has its own checked-out branch, its own working files, and its own staging area. Two worktrees cannot have the same branch checked out at the same time. Creating a worktree is fast (no re-clone), and removing one leaves the rest of the repository untouched. This is the mechanism that lets pr9k operate on the next feature in one directory while the user keeps running the app from another.

## Outcome

- pr9k's commits, file edits, and branch changes happen in a separate working directory; the user's primary checkout's working tree is unchanged on disk and stays on its original branch ([D1](artifacts/decision-log.md#d1-worktree-location)).
- The user can run, build, and test the application from the primary checkout while pr9k iterates.
- All commits pr9k produces during the run land on a single, traceable branch tied to that run, and that branch is pushed to the remote so the work is durable even if the worktree is later removed ([D2](artifacts/decision-log.md#d2-branch-naming-for-the-run)).
- Run-time observability — log files, the iteration log, per-step transcripts — is preserved after the run completes so the user can review what pr9k did ([D3](artifacts/decision-log.md#d3-log-durability)).
- The default behavior of pr9k is unchanged: workflows that do not set `useWorktrees: true` continue to operate in-place exactly as before ([D8](artifacts/decision-log.md#d8-default-and-config-shape)).

## Actors and Triggers

- **Actor:** the pr9k user — a developer who wants to keep using their primary checkout (running the app, testing other features, pairing on a separate branch) while pr9k iterates on the next feature.
- **Trigger:** the user sets `useWorktrees: true` at the top level of `workflow/config.json` and runs `pr9k` (with or without `-n`, `--project-dir`, or `--workflow-dir`). The toggle is read from the workflow config; there is no CLI flag for it.
- **Preconditions:**
  - The target repository is a git repository (already required by pr9k today).
  - `git` is installed and on PATH (already required).
  - The primary checkout has a resolvable HEAD (any branch, any commit, including a detached HEAD).
  - The primary checkout's working tree may have uncommitted changes — worktrees are independent of the primary's working state, so this is allowed.
  - For repositories that use submodules, the user is responsible for understanding the limitation ([D6](artifacts/decision-log.md#d6-submodule-policy)).

## Primary Flow

1. The user adds `useWorktrees: true` to the top of `workflow/config.json` and runs `pr9k` from the primary checkout.
2. pr9k parses CLI flags and resolves the workflow bundle from the primary checkout, then loads `config.json` and reads the `useWorktrees` setting ([T1](artifacts/feature-technical-notes.md#t1-sequencing-constraint)).
3. Because `useWorktrees` is true, pr9k creates a new git worktree at `<primary>/.git/pr9k-worktrees/<run-stamp>/`, checked out on a brand-new branch named `pr9k/<run-stamp>` that branches off the primary's current HEAD ([D1](artifacts/decision-log.md#d1-worktree-location), [D2](artifacts/decision-log.md#d2-branch-naming-for-the-run), [T2](artifacts/feature-technical-notes.md#t2-branch-uniqueness)).
4. pr9k internally redirects its "project directory" — the working directory that anchors all subprocess execution, container bind-mounts, log paths, and template variable substitution — from the primary checkout path to the worktree path. The pr9k process itself does not change its own working directory.
5. pr9k continues startup: it creates the worktree's `.pr9k/` runtime directory, opens its log file inside the worktree, and runs preflight checks against the worktree path.
6. The TUI renders, showing the run as it normally would. The status header indicates the run is operating in a worktree (the user is shown the worktree path so they can find it on disk).
7. The workflow loop runs as it always does: each iteration finds the next "ralph"-labeled issue, the feature-work step edits files, the test steps add tests, the issue is summarized and closed, and the run pushes to the remote. All file edits, all `git` operations, and all `gh` operations target the worktree's branch ([T3](artifacts/feature-technical-notes.md#t3-first-push-upstream)).
8. After all iterations, finalization runs (code review, fix review items, update docs, deferred work, lessons learned, final push) — also against the worktree's branch.
9. On graceful completion, pr9k prints the worktree path and the branch name to the TUI's final summary so the user can find the work, then exits. The worktree is left on disk by default ([D4](artifacts/decision-log.md#d4-cleanup-policy)).
10. The user can inspect the worktree, merge or rebase the run's branch, run `git worktree remove <path>` to clean up when finished, or invoke a future cleanup helper.

## Alternate Flows and States

### The user quits during the run (`q` then `y`, or SIGINT/SIGTERM)

- **Entry condition:** the user requests a graceful quit while the workflow is mid-run.
- **Sequence:** pr9k's existing graceful-shutdown path runs (subprocess termination, log flush). The worktree is left on disk; pr9k prints the worktree path and the branch name in the final summary so the user can recover or discard the partial work.
- **Exit:** the worktree, its uncommitted files, any commits already made on the run's branch, and the log directory all remain on disk for the user to review or discard. The user removes the worktree with `git worktree remove --force <path>` when done.

### The pr9k process is killed (SIGKILL, panic, power loss)

- **Entry condition:** pr9k terminates without running its shutdown path.
- **Sequence:** the worktree is left on disk in whatever state it was in. No cleanup runs.
- **Exit:** the next pr9k run that has `useWorktrees: true` and starts in the same primary checkout detects the stale worktree and warns the user. The user decides whether to keep or remove it ([D5](artifacts/decision-log.md#d5-stale-worktree-handling)).

### `useWorktrees` is set but worktree creation fails

- **Entry condition:** `git worktree add` returns a non-zero exit (e.g., the run-stamp path collides, disk is full, permissions are wrong, a prior process locked the worktree).
- **Sequence:** pr9k surfaces the git error to the user with the exact `git worktree add` command and stderr output; pr9k does not fall back to in-place execution.
- **Exit:** pr9k exits with a non-zero status. No partial worktree state is left behind.

### A stale `pr9k/<stamp>` worktree from a prior crashed run is detected at startup

- **Entry condition:** at the moment the new worktree is being created, `git worktree list` shows one or more existing worktrees under `<primary>/.git/pr9k-worktrees/`.
- **Sequence:** pr9k logs a warning to the TUI and to the log file naming each stale worktree path. pr9k does not auto-remove them. pr9k proceeds with creating its own fresh worktree.
- **Exit:** the new run runs normally; the user is responsible for cleaning up stale worktrees themselves ([D5](artifacts/decision-log.md#d5-stale-worktree-handling)).

### The user passes `--project-dir <path>` while `useWorktrees` is true

- **Entry condition:** explicit project-dir override combined with `useWorktrees: true`.
- **Sequence:** pr9k resolves `--project-dir` to the "primary" path, then creates the worktree under that primary's `.git/pr9k-worktrees/`. The worktree path becomes the working directory for the run.
- **Exit:** identical to the standard primary flow — the user's `--project-dir` value is treated as "the primary checkout."

## Edge Cases and Failure Modes

| Condition | Required Behavior |
|-----------|-------------------|
| `useWorktrees: false` or absent in config | The default in-place behavior is used; nothing about the run changes from today. ([D8](artifacts/decision-log.md#d8-default-and-config-shape)) |
| `useWorktrees: true` but the target is not a git repository | pr9k surfaces the git error from `git worktree add` and exits non-zero. |
| The primary checkout has uncommitted changes | The new worktree is created normally; the primary's uncommitted changes are not visible inside the worktree. (Worktrees do not share working-tree state.) |
| The primary is on a detached HEAD | The new worktree is created off that HEAD as a fresh branch named `pr9k/<run-stamp>`. ([D2](artifacts/decision-log.md#d2-branch-naming-for-the-run)) |
| A branch named `pr9k/<run-stamp>` already exists in the repository | Run-stamp is millisecond-precision and unique per run, so this should not happen in practice. If it does (clock skew, manual creation), pr9k surfaces the git error and exits — it does not force-reset an existing branch. |
| The worktree path `<primary>/.git/pr9k-worktrees/<run-stamp>/` already exists | The run-stamp is unique. If a collision occurs anyway, pr9k surfaces the git error and exits without modifying the existing path. |
| The first `git push` of the run's branch happens (no upstream tracking ref yet) | The push step sets the upstream so the push succeeds and subsequent pushes work without the flag. ([D7](artifacts/decision-log.md#d7-first-push-sets-upstream), [T3](artifacts/feature-technical-notes.md#t3-first-push-upstream)) |
| The repository contains git submodules | The worktree is created without initialized submodule contents. The user is shown a warning at startup that submodule-using projects may need to initialize submodules manually inside the worktree. ([D6](artifacts/decision-log.md#d6-submodule-policy)) |
| The user invokes `pr9k sandbox shell` or `pr9k workflow` while `useWorktrees` is true | These subcommands ignore `useWorktrees` and operate against the user's current working directory as they do today. The toggle only affects the workflow runner. ([D9](artifacts/decision-log.md#d9-subcommand-scope)) |
| Logs and artifacts on completion | Log file, iteration log, and per-step transcripts persist inside the worktree's `.pr9k/logs/` even after the run ends, until the user removes the worktree. ([D3](artifacts/decision-log.md#d3-log-durability)) |
| The user manually deletes the worktree directory mid-run | The current run fails with a file-system error from the next subprocess; the run is not designed to recover. The user is expected not to do this. |

## User Interactions

- **Affordances:**
  - One config-file toggle, no CLI flag: `useWorktrees: true` at the top of `config.json`.
  - The TUI's status header surfaces the worktree path during the run.
  - The TUI's final summary surfaces the worktree path, the run's branch name, and the command to remove the worktree (`git worktree remove <path>`).
- **Feedback:**
  - At startup, if `useWorktrees: true`, the TUI prints a single line indicating the worktree was created and where.
  - At startup, if stale `pr9k/...` worktrees were detected, a warning lists each stale path.
  - At end-of-run, the worktree path and branch name are printed to the TUI and to the log file.
- **Error states:**
  - Failed worktree creation surfaces the underlying `git` error verbatim — the user sees enough information to debug (no swallowed stderr).
  - Failed first-push surfaces the underlying error verbatim and pr9k does not silently continue.

## Coordinations

| Coordinating System | Direction | Interaction | Ordering / Consistency Requirement |
|---------------------|-----------|-------------|-----------------------------------|
| Git (local) | outbound | `git worktree add`, `git worktree list`, `git worktree remove` | Worktree must be created after the workflow bundle resolves but before the runtime directory is created and the workflow runner is constructed ([T1](artifacts/feature-technical-notes.md#t1-sequencing-constraint)) |
| Git (remote) — first push of the run's branch | outbound | `git push` with upstream-set semantics | First push must establish a remote tracking ref for `pr9k/<run-stamp>`; subsequent pushes use the established upstream ([T3](artifacts/feature-technical-notes.md#t3-first-push-upstream)) |
| GitHub (via `gh` CLI) | outbound | Issue listing, closing, summary posting, image fetches | All `gh` operations resolve the repository from the worktree's git state, which is the same repository as the primary's. No `gh` configuration changes are needed. |
| Docker sandbox | outbound | The container that runs `claude` for each step bind-mounts the project directory at `/home/agent/workspace`. | The bind-mount source is the worktree path while `useWorktrees` is true; the in-container path is unchanged. |
| User's primary checkout | none (deliberately) | pr9k must not write to, change branches in, or otherwise mutate the primary checkout's working tree while the run is in progress. | The pr9k process never calls `cd` into the worktree; the redirect is internal to pr9k's project-directory plumbing. |
| `pr9k workflow` (workflow-builder TUI) and `pr9k sandbox shell` | none | These subcommands resolve their own working directory independently and ignore `useWorktrees`. ([D9](artifacts/decision-log.md#d9-subcommand-scope)) | — |

## Out of Scope

- Per-iteration worktrees (one worktree per ralph issue). The default workflow does not switch branches per issue, so a per-iteration model has no use today; it would also require parallelizing the iteration loop, which is a much larger change. ([D2](artifacts/decision-log.md#d2-branch-naming-for-the-run))
- Automated cleanup of stale worktrees at startup. pr9k warns and lets the user decide. ([D5](artifacts/decision-log.md#d5-stale-worktree-handling))
- Auto-initialization of submodules in the new worktree. The first iteration documents the limitation; auto-init can be added later if a user reports it as a blocker. ([D6](artifacts/decision-log.md#d6-submodule-policy))
- Configurable worktree location. The first iteration ships a single fixed location (`.git/pr9k-worktrees/<run-stamp>/`); a config option to relocate worktrees can be added later if a user reports a real conflict. ([D1](artifacts/decision-log.md#d1-worktree-location))
- A new `pr9k worktree prune` subcommand to bulk-clean stale entries. Useful but not required for the MVP; deferred. ([D5](artifacts/decision-log.md#d5-stale-worktree-handling))
- Adding a `primaryDir` field to the statusline payload. No user has asked for it, and the worktree path is already visible. Add later if statusline scripts need it. ([D11](artifacts/decision-log.md#d11-statusline-payload-shape))
- Behavior change for `pr9k sandbox shell` and `pr9k workflow`. ([D9](artifacts/decision-log.md#d9-subcommand-scope))

## Deferred (YAGNI)

### Configurable cleanup-on-success policy
- **Why deferred:** simpler-version test — the strictly simpler version is "always keep the worktree on disk after the run." The user can remove it manually with `git worktree remove`. No user has asked for auto-removal.
- **Reopen when:** a user reports that worktrees accumulate to the point of disk-space or workflow-friction concerns, OR an automation use case (e.g., CI) emerges where leaving the worktree on disk is wrong.
- **Source:** investigation open question Q3 (cleanup policy).

### `logDir` split (logs in primary, work in worktree)
- **Why deferred:** simpler-version test — keeping the worktree on disk by default already satisfies the evidence (logs preserved for the user to review). Splitting `logDir` would be a four-file change for no additional behavior.
- **Reopen when:** the cleanup-on-success policy is reopened (above) AND auto-remove becomes the default. At that point logs would vanish without a `logDir` split.
- **Source:** investigation open question Q1, validation finding V7.

### Submodule auto-initialization
- **Why deferred:** evidence test — pr9k itself has no submodules, the default workflow does not exercise them, and no user has reported a submodule-using project as a target.
- **Reopen when:** a user reports a submodule-using project failing inside a worktree.
- **Source:** validation finding V10.

### Statusline payload `primaryDir` field
- **Why deferred:** evidence test — no user has reported that their statusline script needs both paths. The worktree path appears in the existing `projectDir` field.
- **Reopen when:** a statusline-script author reports the missing context as a real problem.
- **Source:** validation finding V11.

### `pr9k worktree prune` subcommand
- **Why deferred:** simpler-version test — the existing `git worktree remove` and `git worktree prune` commands already do the job; pr9k just needs to surface a warning so the user knows where to look.
- **Reopen when:** users report that running `git worktree` commands by hand is confusing or error-prone, OR stale worktrees accumulate fast enough that bulk cleanup becomes a real friction.
- **Source:** investigation open question Q5.

## Open Items

- **OI-1:** First-push upstream-setting needs to land in the `git_push` script (which today silently swallows all push failures via `trap 'exit 0' EXIT`). The fix is in scope for this feature ([D7](artifacts/decision-log.md#d7-first-push-sets-upstream)), but the script is shared with the in-place workflow path. Does the in-place workflow want the same trap-removal? Recommend yes; defer to implementation review.
  - **Resolves when:** the implementation plan confirms whether the trap removal is bundled with this feature or split out. If split out, this feature is blocked until the script-level fix lands.
  - **Blocks implementation:** Yes — the worktree path cannot push reliably until the script either gains `-u origin HEAD` or stops swallowing push errors.

## Summary

- **Outcome delivered:** users can run pr9k against the next feature without disrupting their primary checkout, with all run artifacts (commits, logs, transcripts) preserved on disk and traceable to a single per-run branch.
- **Primary actors:** the pr9k user; downstream consumers are git, GitHub (via `gh`), and the existing Docker sandbox.
- **Decisions settled by evidence:** 11 — see [artifacts/decision-log.md](artifacts/decision-log.md).
- **Decisions settled by user input:** 0 (auto-mode interview; the user can redirect any decision via review).
- **Sub-agents consulted:** TBD after Step 6 — see [artifacts/team-findings.md](artifacts/team-findings.md).
- **Key adjustments from review:** TBD after Step 7 — see [artifacts/team-findings.md](artifacts/team-findings.md).
- **Remaining open items:** 1 (OI-1, blocking).
- **Technical notes:** 3 — see [artifacts/feature-technical-notes.md](artifacts/feature-technical-notes.md).
