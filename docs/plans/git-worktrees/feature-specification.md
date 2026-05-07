# Feature Specification: useWorktrees (with auto-resume)

When the workflow config's top-level `worktrees.enabled` is `true`, pr9k runs the entire workflow inside a freshly created git worktree so the user's primary checkout stays usable for parallel development while pr9k mutates code on a separate branch. If pr9k is killed mid-run for any reason, the next pr9k invocation **automatically resumes** into the same worktree on the same branch, preserving local commits and continuing the iteration loop without manual intervention. A companion `worktrees.autoCleanup` option and a new `pr9k worktree prune` subcommand make removal of finished worktrees a one-step or zero-step operation.

## Background (for readers new to git worktrees)

A git worktree is a second on-disk working directory backed by the same repository. The repository's commits, branches, remotes, and config are shared across all worktrees, but each worktree has its own checked-out branch, its own working files, and its own staging area. Two worktrees cannot have the same branch checked out at the same time, so pr9k always creates a fresh branch when it makes a worktree — that is the only way to give the worktree a branch the primary doesn't already have. Creating a worktree is fast (no re-clone), and removing one leaves the rest of the repository untouched. This is the mechanism that lets pr9k operate on the next feature in one directory while the user keeps running the app from another.

The feature uses **two stamps**, both millisecond-precision wall-clock identifiers in the format `ralph-YYYY-MM-DD-HHMMSS.mmm`:

- A **worktree-stamp** is baked into the worktree path and the run's branch name when the worktree is first created (e.g., `pr9k-<worktree-stamp>` is the branch). It does not change across resumes — every pr9k invocation that resumes into the same worktree uses the same worktree-stamp.
- An **invocation-stamp** is generated fresh at the start of every pr9k invocation. It identifies the run's log file, per-step artifact directory, and per-invocation iteration log inside the worktree. This is how a user reading logs after the fact can tell which records belong to which pr9k invocation.

Both stamps come from the same source — pr9k's existing wall-clock generator documented in [`docs/code-packages/logger.md`](../../../code-packages/logger.md) — but they are produced at different lifecycle moments.

**Cross-run resume is automatic.** When pr9k starts, it checks for an active-run state file at `<primary>/.pr9k/active-run.json`. If the file exists and points to a valid worktree, pr9k enters that worktree and continues. If the file is absent, pr9k creates a fresh worktree on a fresh branch. The state file is removed at the end of a successful or naturally-exited run; it is left on disk on user-requested quit (so the next start resumes) and on hard-kill paths. This makes resume **deterministic** — either the marker is there (resume) or it isn't (fresh) — with no fuzzy matching ([D15](artifacts/decision-log.md#d15-cross-run-resume-via-active-run-state-file), [T4](artifacts/feature-technical-notes.md#t4-active-run-state-file-schema)).

## Outcome

- pr9k's commits, file edits, and branch changes happen in a separate working directory; the user's primary checkout's working tree is unchanged on disk and stays on its original branch ([D1](artifacts/decision-log.md#d1-worktree-location)).
- The user can run, build, and test the application from the primary checkout while pr9k iterates.
- All commits pr9k produces during the run land on a single, traceable branch tied to that run, and that branch is pushed to the remote so the work is durable even if the worktree is later removed ([D2](artifacts/decision-log.md#d2-branch-naming-for-the-run)).
- **If pr9k is killed mid-run, the next invocation auto-resumes into the same worktree, on the same branch, with all local commits intact.** No manual lookup, no `--continue` flag, no user choice required ([D15](artifacts/decision-log.md#d15-cross-run-resume-via-active-run-state-file)).
- Run-time observability is preserved: each pr9k invocation writes its own log file and its own per-invocation iteration log inside the worktree, so the boundaries between invocations are clear and existing consumers like `post_issue_summary` and `lessons-learned` operate on a single invocation's records ([D3](artifacts/decision-log.md#d3-log-durability), [D17](artifacts/decision-log.md#d17-per-invocation-iteration-log-files)).
- The default behavior of pr9k is unchanged: workflows that do not set `worktrees.enabled: true` continue to operate in-place exactly as before ([D8](artifacts/decision-log.md#d8-default-and-config-shape)).
- When `worktrees.autoCleanup: true` is set, the worktree, its branch, and the active-run state file are removed automatically on graceful completion. When `false` (the default), the worktree is left on disk for inspection; only the state file is removed ([D13](artifacts/decision-log.md#d13-autocleanup-behavior)).
- A new `pr9k worktree prune` subcommand removes all pr9k-owned worktrees and their branches in one step. A new `--fresh` flag on the workflow runner explicitly abandons an in-flight run before starting a new one ([D14](artifacts/decision-log.md#d14-pr9k-worktree-prune-subcommand), [D19](artifacts/decision-log.md#d19-fresh-cli-flag)).

### Invariants

- pr9k must not write to, change branches in, or otherwise mutate the primary checkout's working tree while the run is in progress. This is a hard constraint, enforced behaviorally by the worktree redirect ([T1](artifacts/feature-technical-notes.md#t1-sequencing-constraint)).
- When the active-run state file exists and points to a valid worktree on the same primary, pr9k MUST resume into it rather than create a new worktree. This is the determinism guarantee ([D15](artifacts/decision-log.md#d15-cross-run-resume-via-active-run-state-file)).
- Local commits on a worktree's branch MUST never be silently discarded by pr9k. Removal happens only on graceful completion with `autoCleanup: true`, or on explicit user action (`pr9k worktree prune`, `--fresh`).

## Actors and Triggers

- **Actor:** the pr9k user — a developer who wants to keep using their primary checkout (running the app, testing other features, pairing on a separate branch) while pr9k iterates on the next feature.
- **Trigger:** the user adds a `worktrees` block to `workflow/config.json` with `enabled: true` (and optionally `autoCleanup: true`) and runs `pr9k`. The toggle is read from the workflow config; there is no CLI flag override for the on/off state. When `--project-dir <path>` is also passed, the resolved `--project-dir` is treated as "the primary checkout," and the worktree (and the active-run state file) are anchored to that path.
- **Resume trigger:** there is no separate trigger for resume — running `pr9k` while an active-run state file exists IS the resume trigger. The user does nothing special.
- **Auxiliary triggers:** `pr9k worktree prune` (manual bulk cleanup of pr9k-owned worktrees) and `pr9k --fresh` (explicit abandon of an in-flight run before starting fresh).
- **Preconditions:**
  - The target repository is a git repository (already required by pr9k today).
  - `git` is installed and on PATH (already required).
  - The primary checkout has a resolvable HEAD (any branch, any commit, including a detached HEAD).
  - The primary checkout's working tree may have uncommitted changes — worktrees are independent of the primary's working state, so this is allowed.
  - The remote permits pushing to branch names beginning with `pr9k-`. Branch-protection rules that reject this pattern will cause the run's first push to fail ([D7](artifacts/decision-log.md#d7-first-push-sets-upstream-and-step-reorder)).
  - Only one pr9k instance should run against a given primary checkout at a time. The active-run state file's process-identity check (PID alive AND binary path matches) is the soft enforcement; concurrent invocations of the same binary will see "another pr9k appears to be running" and exit ([D10](artifacts/decision-log.md#d10-concurrent-runs)).
  - The primary's parent directory is writable, since the worktree is created as a sibling of the primary ([D1](artifacts/decision-log.md#d1-worktree-location)).

## Primary Flow

Two paths through the flow share most steps. The first divergence is at step 3 (resume vs fresh). The second divergence is at the cleanup step (autoCleanup or not).

1. The user adds a `worktrees` block with `enabled: true` to `workflow/config.json` and runs `pr9k` (with or without `-n`, `--project-dir`, `--workflow-dir`, or `--fresh`).
2. pr9k parses CLI flags and resolves the workflow bundle from the primary checkout, then loads `config.json` and reads the `worktrees` block ([T1](artifacts/feature-technical-notes.md#t1-sequencing-constraint)).
3. pr9k checks for an active-run state file at `<primary>/.pr9k/active-run.json`:
   - **If `--fresh` was passed**, pr9k removes the state file (and the worktree it pointed to, if `worktrees.enabled` is true) before continuing. The branch in the prior state file (if any) **is also deleted** so the user has a fully clean slate. Then proceed as fresh ([D19](artifacts/decision-log.md#d19-fresh-cli-flag)).
   - **If the file exists, points to an existing worktree on this primary, and the recorded process is no longer running** (PID + binary-path liveness check), pr9k auto-resumes: it does not create a new worktree, does not generate a new worktree-stamp, and does not write a new state file. It generates a fresh invocation-stamp for log/artifact paths and proceeds with the existing worktree path and branch ([D15](artifacts/decision-log.md#d15-cross-run-resume-via-active-run-state-file), [D16](artifacts/decision-log.md#d16-runresult-exitreason)).
   - **If the file exists but the recorded process is alive AND its binary matches pr9k's**, pr9k exits non-zero with "another pr9k appears to be running for this primary checkout (PID N)." The user can override with `--fresh` if they know the prior process is no longer cooperating ([D10](artifacts/decision-log.md#d10-concurrent-runs)).
   - **If the file exists but the worktree path is gone** (manually deleted), pr9k logs a warning, removes the orphaned state file (and the orphaned branch if it still exists), then proceeds as fresh.
   - **If the file is corrupt** (atomicwrite makes this rare, but manual edits could cause it), pr9k renames it to `active-run.json.corrupt-<timestamp>` with a loud warning and proceeds as fresh.
   - **If the file is absent**, pr9k proceeds as fresh.
4. **Fresh path only:** pr9k creates a new git worktree at `<primary-parent>/<primary-basename>-pr9k-<worktree-stamp>/` (a sibling of the primary), checked out on a brand-new branch named `pr9k-<worktree-stamp>` that branches off the primary's current HEAD ([D1](artifacts/decision-log.md#d1-worktree-location), [D2](artifacts/decision-log.md#d2-branch-naming-for-the-run), [T2](artifacts/feature-technical-notes.md#t2-branch-uniqueness)).
5. pr9k internally redirects its "project directory" — the working directory that anchors all subprocess execution, container bind-mounts, log paths, and template variable substitution — from the primary checkout path to the worktree path ([T1](artifacts/feature-technical-notes.md#t1-sequencing-constraint)).
6. pr9k completes startup against the worktree path: preflight runs, the run's log file is opened, and a per-invocation iteration log is opened at `<worktree>/.pr9k/iteration-<invocation-stamp>.jsonl` ([D17](artifacts/decision-log.md#d17-per-invocation-iteration-log-files)).
7. **Fresh path only:** pr9k writes the active-run state file at `<primary>/.pr9k/active-run.json` with worktree path, branch name, worktree-stamp, PID, binary path, started-at, schema-version, and pr9k version ([T4](artifacts/feature-technical-notes.md#t4-active-run-state-file-schema)).
8. The run's log file header records the worktree path, the primary path, the branch name, the worktree-stamp, and the invocation-stamp. The first record written to the iteration log captures the same as a structured event. On a resumed run, the log header explicitly notes "RESUMED FROM `<worktree-stamp>`" so the post-run reader knows what kind of run this was ([D3](artifacts/decision-log.md#d3-log-durability), [D11](artifacts/decision-log.md#d11-worktree-startup-feedback)).
9. The TUI renders, showing the run as it normally would. The TUI's iteration line is prefixed with the worktree's basename during the run; on a resumed run the prefix also includes "(resumed)" so the user can confirm at a glance ([D11](artifacts/decision-log.md#d11-worktree-startup-feedback)).
10. The workflow loop runs as it always does: each iteration finds the next "ralph"-labeled issue, the feature-work step edits files, the test steps add tests, and the run pushes to the remote *before* closing the issue ([D18](artifacts/decision-log.md#d18-push-before-close-step-ordering)). All file edits, all `git` operations, and all `gh` operations target the worktree's branch ([T3](artifacts/feature-technical-notes.md#t3-first-push-upstream)). On a resumed run, `get_next_issue` naturally re-delivers the in-progress issue (because pr9k did not close it before the kill), and the iteration runs again from feature-work — but on a branch that already carries any prior commits.
11. After all iterations, finalization runs (code review, fix review items, update docs, deferred work, lessons learned, final push) — also against the worktree's branch.
12. On graceful completion, pr9k writes a closing record to the run's log file and the per-invocation iteration log. Then:
    - **`autoCleanup: true`** → pr9k removes the worktree (`git worktree remove --force`), deletes the run's branch (`git branch -D pr9k-<worktree-stamp>`), and removes the active-run state file ([D13](artifacts/decision-log.md#d13-autocleanup-behavior)).
    - **`autoCleanup: false`** (default) → pr9k removes the active-run state file but leaves the worktree and branch on disk. The TUI's final summary line surfaces the worktree path, the branch name, and the count of all `pr9k-*` worktrees on disk so the user knows their backlog ([D11](artifacts/decision-log.md#d11-worktree-startup-feedback)).
    - In both cases, removal of the state file happens only when the run completed naturally (`Completed`) or exited via `breakLoopIfEmpty` (`LoopBroken`). On user-requested quit (`UserQuit`), the state file is **kept** so the next invocation auto-resumes ([D16](artifacts/decision-log.md#d16-runresult-exitreason)).
13. With the state file gone (or kept, in the UserQuit case), the next pr9k invocation either starts fresh or resumes accordingly.

## Alternate Flows and States

### A prior run was killed mid-iteration and the user restarts pr9k

- **Entry condition:** a prior pr9k run with `worktrees.enabled: true` was killed for any reason (SIGKILL, panic, OOM, network drop, the user fixing a pr9k bug). The active-run state file at `<primary>/.pr9k/active-run.json` exists and points to a worktree that still exists. The recorded PID either does not exist, or exists but doesn't belong to a pr9k binary.
- **Sequence:**
  1. pr9k reads the state file at startup (Primary Flow step 3).
  2. The PID + binary-path liveness check determines the prior process is gone.
  3. pr9k enters the existing worktree. No new worktree is created. The worktree-stamp from the state file is reused for log header context. A fresh invocation-stamp is generated for log/artifact paths.
  4. The TUI iteration line shows the worktree basename and "(resumed)" suffix.
  5. The workflow loop runs normally. `get_next_issue` re-delivers the prior run's in-flight issue (still open because pr9k didn't get to the close step), and the iteration runs from feature-work onward on a branch that already carries prior commits.
  6. The run completes (or is killed again, in which case the state file is still present for the next attempt).
- **Exit:** indistinguishable from a normal completion from the user's perspective. The original goal — pr9k continues the work — is met without manual intervention ([D15](artifacts/decision-log.md#d15-cross-run-resume-via-active-run-state-file)).

### The user quits during the run (`q` then `y`, or SIGINT/SIGTERM)

- **Entry condition:** the user requests a graceful quit while the workflow is mid-run.
- **Sequence:** pr9k's existing graceful-shutdown path runs (subprocess termination, log flush). pr9k writes a closing log record to both the run's log file and the per-invocation iteration log. Because the exit reason is `UserQuit` (not `Completed` or `LoopBroken`), the active-run state file is **kept** ([D16](artifacts/decision-log.md#d16-runresult-exitreason)). If `worktrees.autoCleanup` is true, cleanup of worktree+branch is also skipped — `autoCleanup` only fires on `Completed` or `LoopBroken`.
- **Exit:** the worktree, its uncommitted files, any commits already made on the run's branch, the per-invocation iteration log, and the active-run state file all remain on disk. The next pr9k invocation auto-resumes. The user can also run `pr9k --fresh` to discard the in-flight work, or `pr9k worktree prune` after that.

### The pr9k process is killed (SIGKILL, panic, power loss)

- **Entry condition:** pr9k terminates without running its shutdown path.
- **Sequence:** the worktree is left on disk. The active-run state file remains (it was atomic-written at startup). No closing log record is written.
- **Exit:** the next pr9k run with `worktrees.enabled: true` reads the state file, runs the liveness check (the recorded PID is gone), and auto-resumes. If `--fresh` is passed, it discards the in-flight state instead.

### `worktrees.enabled: true` but worktree creation fails

- **Entry condition:** `git worktree add` returns a non-zero exit (e.g., the run-stamp path collides, disk is full, permissions are wrong, a prior process locked the worktree).
- **Sequence:** pr9k surfaces the git error to the user with the exact `git worktree add` command and stderr output. pr9k does not write the state file (because the state we'd record is invalid). pr9k does not fall back to in-place execution.
- **Exit:** pr9k exits with a non-zero status. No partial state is left behind.

### A push step fails (any push within the run)

- **Entry condition:** any `git push` step returns a non-zero exit. The most likely causes are: a fresh branch with no upstream tracking ref (the script must establish one on first push), branch-protection rejection, network failure, or auth failure.
- **Sequence:** pr9k enters its existing error-recovery mode. The TUI presents the standard `c` (continue), `r` (retry), `q` (quit) prompt. The full stderr from the push is written to the iteration log and the run's log file. The run does not silently advance to the next step ([D7](artifacts/decision-log.md#d7-first-push-sets-upstream-and-step-reorder), [T3](artifacts/feature-technical-notes.md#t3-first-push-upstream)). Crucially, because `git push` runs **before** `Close issue` in the default workflow ([D18](artifacts/decision-log.md#d18-push-before-close-step-ordering)), a push failure leaves the issue open — the safety property that makes auto-resume correct.
- **Exit:** the user resolves the underlying problem (sets the upstream, adjusts branch protection, fixes auth) and chooses retry, or quits. If the user quits, the state file is kept (UserQuit path); the next invocation resumes.

### Stale `pr9k-*` worktrees from prior runs are detected at startup

- **Entry condition:** at startup, pr9k discovers one or more existing pr9k-owned worktrees (directory name matches `<primary-basename>-pr9k-*`). The currently-active worktree referenced by `active-run.json` (if any) is excluded from this set.
- **Sequence:** pr9k counts the stale entries (the active one is never counted). When the count is small (≤ 5), pr9k logs each stale worktree path on its own line in the TUI and the log file. When the count is larger, pr9k prints a single summary line in the TUI ("N stale pr9k worktrees found — run `git worktree list` or `pr9k worktree prune` to review") and writes the individual paths only to the log file. pr9k does not auto-remove any of them ([D5](artifacts/decision-log.md#d5-stale-worktree-handling)).
- **Exit:** the new run runs normally; the user is responsible for cleaning up stale worktrees themselves with `pr9k worktree prune` or `git worktree remove`.

### The user runs `pr9k worktree prune`

- **Entry condition:** the user invokes `pr9k worktree prune` (or `pr9k worktree prune --dry-run`) from a primary checkout. This is independent of whether the workflow config sets `worktrees.enabled` — the subcommand is always available.
- **Sequence:**
  1. pr9k discovers existing worktrees and filters for pr9k-owned entries (same path-prefix filter D5 uses).
  2. **The currently-active worktree** (if `active-run.json` exists) is excluded from removal — pruning would orphan a running pr9k.
  3. **In `--dry-run` mode**, pr9k prints the list of worktrees that would be removed and the branches that would be deleted, then exits.
  4. **In normal mode**, pr9k removes each pr9k-owned worktree (`git worktree remove --force <path>`) and deletes its `pr9k-<stamp>` branch. The list of removed paths is printed to stdout.
  5. If a worktree cannot be removed (e.g., a file lock, permissions error), pr9k surfaces the git error verbatim and continues with the rest. The exit code is non-zero if any removal failed ([D14](artifacts/decision-log.md#d14-pr9k-worktree-prune-subcommand)).
- **Exit:** all pr9k-owned worktrees and branches **except the active one** are removed.

### The user runs `pr9k --fresh`

- **Entry condition:** the user invokes the workflow runner with `--fresh`. They want to abandon the in-flight run and start over.
- **Sequence:**
  1. pr9k reads `active-run.json`. If absent, `--fresh` is a no-op for cleanup purposes; pr9k proceeds as a fresh run.
  2. If present, pr9k removes the worktree (`git worktree remove --force`), deletes the branch (`git branch -D pr9k-<worktree-stamp>`), and removes the state file. A clear "Discarded prior run from <worktree-stamp>" line goes to the TUI and log.
  3. pr9k then proceeds with a fresh start — generates a new worktree-stamp, creates a new worktree on a new branch, writes a new state file ([D19](artifacts/decision-log.md#d19-fresh-cli-flag)).
- **Exit:** the run proceeds as fresh, with the prior in-flight state fully discarded.

### The user passes `--project-dir <path>` while `worktrees.enabled` is true

- **Entry condition:** explicit project-dir override combined with `worktrees.enabled: true`.
- **Sequence:** pr9k resolves `--project-dir` to the "primary" path, then looks for `<primary>/.pr9k/active-run.json` under that path, then either resumes or creates a new worktree as a sibling of that path.
- **Exit:** identical to the standard primary flow — the user's `--project-dir` value is treated as "the primary checkout" for all worktree and state-file purposes.

## Edge Cases and Failure Modes

| Condition | Required Behavior |
|-----------|-------------------|
| `worktrees` block is absent or `worktrees.enabled` is `false` | The default in-place behavior is used; the active-run state file is not consulted or written. ([D8](artifacts/decision-log.md#d8-default-and-config-shape)) |
| `worktrees.enabled: true` but the target is not a git repository | pr9k surfaces the git error from `git worktree add` and exits non-zero. |
| `worktrees.autoCleanup: true` and the run completes naturally (Completed or LoopBroken) | The worktree is removed, the branch is deleted, and the state file is removed before pr9k exits. The TUI's final summary shows what was removed. ([D13](artifacts/decision-log.md#d13-autocleanup-behavior), [D16](artifacts/decision-log.md#d16-runresult-exitreason)) |
| `worktrees.autoCleanup: true` and the user quits (UserQuit) | autoCleanup is suppressed. The state file is also kept so the next invocation resumes. ([D13](artifacts/decision-log.md#d13-autocleanup-behavior), [D16](artifacts/decision-log.md#d16-runresult-exitreason)) |
| `worktrees.autoCleanup: true` and the run is hard-killed (SIGKILL/panic) | Cleanup needs the shutdown path to run, so nothing is cleaned up in this case. The next run resumes via the state file. ([D13](artifacts/decision-log.md#d13-autocleanup-behavior)) |
| `worktrees.autoCleanup` is set without `worktrees.enabled` | Validator rejects the config at startup with a clear message. ([D13](artifacts/decision-log.md#d13-autocleanup-behavior)) |
| Active-run state file present but worktree directory is gone | pr9k logs a warning, removes the orphaned state file (and the branch if it still exists), and proceeds as fresh. ([D15](artifacts/decision-log.md#d15-cross-run-resume-via-active-run-state-file)) |
| Active-run state file present and worktree present, but recorded PID is alive AND its binary matches pr9k | pr9k exits non-zero with "another pr9k appears to be running for this primary checkout (PID N). Use `pr9k --fresh` to override." ([D10](artifacts/decision-log.md#d10-concurrent-runs)) |
| Active-run state file present and worktree present, but recorded PID is alive yet its binary does NOT match pr9k | Treat as dead and resume. PID reuse is the assumed cause. ([D10](artifacts/decision-log.md#d10-concurrent-runs), [T4](artifacts/feature-technical-notes.md#t4-active-run-state-file-schema)) |
| Active-run state file is corrupt JSON | pr9k renames it to `active-run.json.corrupt-<timestamp>` with a loud warning and proceeds as fresh. ([T4](artifacts/feature-technical-notes.md#t4-active-run-state-file-schema)) |
| Active-run state file's schema-version is unknown to the running pr9k | pr9k logs a warning and attempts to resume anyway, reading whatever fields it recognizes. If structural fields are missing (worktree path, branch), pr9k treats the file as corrupt and follows that path. Refusing to resume is rejected because the user might have intentionally downgraded to fix a regression. ([T4](artifacts/feature-technical-notes.md#t4-active-run-state-file-schema)) |
| The primary checkout has uncommitted changes | The new (or resumed) worktree is unaffected; the primary's uncommitted changes are not visible inside the worktree. |
| The primary is on a detached HEAD | Fresh start: a new worktree is created off that HEAD on a fresh branch named `pr9k-<worktree-stamp>`. Resume: the prior worktree's branch is unaffected by the primary's HEAD state. ([D2](artifacts/decision-log.md#d2-branch-naming-for-the-run)) |
| A branch named `pr9k-<worktree-stamp>` already exists | Run-stamp is millisecond-precision and unique per worktree-creation event. If a collision occurs anyway (clock skew, manual creation, prior run not cleaned up), pr9k surfaces the git error and exits — it does not force-reset an existing branch. |
| The first `git push` of a fresh worktree's branch happens (no upstream tracking ref yet) | The push step establishes a tracking ref so the push succeeds and subsequent pushes work without intervention; the change to the push step also surfaces non-zero exits. Push runs **before** `Close issue` in the default workflow so a failure leaves the issue open. ([D7](artifacts/decision-log.md#d7-first-push-sets-upstream-and-step-reorder), [D18](artifacts/decision-log.md#d18-push-before-close-step-ordering), [T3](artifacts/feature-technical-notes.md#t3-first-push-upstream)) |
| The remote rejects the push (branch protection, auth, network) | pr9k enters its standard error-recovery mode (c/r/q prompt) with the verbatim git error written to the iteration log. ([D7](artifacts/decision-log.md#d7-first-push-sets-upstream-and-step-reorder)) |
| The user manually deletes the `pr9k-<worktree-stamp>` branch but leaves the worktree directory | The next run still detects and warns about the stale worktree because stale detection filters by directory name, not branch name. ([D5](artifacts/decision-log.md#d5-stale-worktree-handling)) |
| Disk fills mid-run | pr9k has no special handling; the run fails with a filesystem error from the affected subprocess and enters error-recovery mode. The state file persists — the user can clean up stale worktrees with `pr9k worktree prune`, then re-run pr9k to resume from the same state. |
| The repository contains git submodules | The worktree is created without initialized submodule contents. The user is responsible for initializing submodules manually inside the worktree if their workflow requires them. ([D6](artifacts/decision-log.md#d6-submodule-policy)) |
| The user invokes `pr9k sandbox shell` or `pr9k workflow` while `worktrees.enabled` is true | These subcommands ignore the `worktrees` block and operate against the user's current working directory as they do today. ([D9](artifacts/decision-log.md#d9-subcommand-scope)) |
| The user runs `pr9k worktree prune` while a pr9k run is active (state file exists, PID alive) | The active worktree is excluded from removal. All other pr9k-owned worktrees are removed normally. ([D14](artifacts/decision-log.md#d14-pr9k-worktree-prune-subcommand)) |
| `pr9k worktree prune` invoked from a path that is not a git repository | pr9k surfaces the underlying git error and exits non-zero. ([D14](artifacts/decision-log.md#d14-pr9k-worktree-prune-subcommand)) |
| `pr9k worktree prune` invoked when no pr9k-owned worktrees exist (or all are excluded) | pr9k prints "No pr9k-owned worktrees to remove" and exits zero. ([D14](artifacts/decision-log.md#d14-pr9k-worktree-prune-subcommand)) |
| Per-invocation iteration log accumulation | Each pr9k invocation writes a new `iteration-<invocation-stamp>.jsonl` file inside the worktree's `.pr9k/`. Files persist until autoCleanup or `pr9k worktree prune` removes the worktree. ([D17](artifacts/decision-log.md#d17-per-invocation-iteration-log-files)) |
| The user manually deletes the worktree directory mid-run | The run fails with a filesystem error from the next subprocess. If the error reaches pr9k's standard error path, the c/r/q prompt is presented. The state file remains; on the next run the orphaned-worktree handler removes it and proceeds as fresh. |

## User Interactions

- **Affordances:**
  - A config-file block, no CLI flag for the on/off state: `worktrees: { enabled: true, autoCleanup: false }` at the top of `config.json`.
  - A `--fresh` CLI flag on the workflow runner for explicit abandon-and-restart.
  - A `pr9k worktree prune` subcommand (with optional `--dry-run`) for one-step bulk removal of pr9k-owned worktrees and branches.
  - The TUI's iteration line is prefixed with the worktree's basename during the run; on a resumed run the prefix also includes "(resumed)".
  - The TUI's final summary line includes the worktree path, the run's branch name, and the count of all `pr9k-*` worktrees on disk.
- **Feedback:**
  - At startup on a fresh run, pr9k prints a single line indicating the worktree was created and where.
  - At startup on a resumed run, pr9k prints "RESUMED FROM `<worktree-stamp>`" along with the worktree path.
  - At startup, if stale `pr9k-*` worktrees were detected (excluding the active one), the count is shown. Up to 5 stale paths are listed inline; above that, a summary line tells the user to run `git worktree list` or `pr9k worktree prune`. Individual paths are always written to the run's log file regardless of count.
  - At end-of-run, the worktree path, branch, and worktree-count appear in the TUI's final summary line and are written to the run's log file and per-invocation iteration log. If `autoCleanup` ran, the summary also reports that the worktree was removed.
  - `pr9k worktree prune` prints one line per removed worktree and a final count.
  - `pr9k --fresh` prints "Discarded prior run from `<worktree-stamp>`" before continuing.
- **Error states:**
  - Worktree-creation failure surfaces the underlying `git` error verbatim.
  - Push failure surfaces the verbatim git error and enters pr9k's standard c/r/q error-recovery mode; the run does not silently continue.
  - "Another pr9k appears to be running" includes the PID and the message "Use `pr9k --fresh` to override if you know the prior process is gone."
  - State-file corruption is reported with the path of the renamed file and a clear "starting fresh" line.

## Coordinations

| Coordinating System | Direction | Interaction | Ordering / Consistency Requirement |
|---------------------|-----------|-------------|-----------------------------------|
| Git (local) | outbound | `git worktree add`, `git worktree list`, `git worktree remove`, `git branch -D` | Worktree must be created (or resume verified) after the workflow bundle resolves but before the runner is constructed. State-file write must happen after `preflight.Run` creates `.pr9k/`. ([T1](artifacts/feature-technical-notes.md#t1-sequencing-constraint), [T4](artifacts/feature-technical-notes.md#t4-active-run-state-file-schema)) |
| Git (remote) — pushes during the run | outbound | `git push` with upstream-set semantics; runs before `Close issue` in the default workflow | First push must establish a remote tracking ref for `pr9k-<worktree-stamp>`; subsequent pushes use the established upstream. On non-zero exit, pr9k enters error-recovery mode. ([T3](artifacts/feature-technical-notes.md#t3-first-push-upstream), [D7](artifacts/decision-log.md#d7-first-push-sets-upstream-and-step-reorder), [D18](artifacts/decision-log.md#d18-push-before-close-step-ordering)) |
| GitHub (via `gh` CLI) | outbound | Issue listing (`get_next_issue` re-delivers in-flight issues naturally), closing (now after push), summary posting, image fetches | All `gh` operations resolve the repository from the worktree's git state. No `gh` configuration changes are needed. |
| Docker sandbox | outbound | Container bind-mounts the project directory at `/home/agent/workspace`. | The bind-mount source is the worktree path while `worktrees.enabled` is true. The sibling-of-primary location keeps the path inside the user's home hierarchy (default-shared on macOS Docker Desktop). |
| `pr9k workflow` (workflow-builder TUI) and `pr9k sandbox shell` | none | These subcommands ignore the `worktrees` block. ([D9](artifacts/decision-log.md#d9-subcommand-scope)) | — |
| `pr9k worktree prune` subcommand | invoked manually by user | Lists pr9k-owned worktrees and removes them with `git worktree remove --force`; deletes their branches; excludes the active worktree referenced in `active-run.json`. | Independent of `worktrees.enabled`; safe to run from any primary checkout. ([D14](artifacts/decision-log.md#d14-pr9k-worktree-prune-subcommand)) |
| Active-run state file at `<primary>/.pr9k/active-run.json` | filesystem | Atomic-written at run start (fresh path only); read at every startup; removed on graceful end (Completed/LoopBroken) but kept on UserQuit and on hard kill. | Write happens once per fresh run, after preflight. Read happens once per pr9k startup, before subsystem construction. ([T4](artifacts/feature-technical-notes.md#t4-active-run-state-file-schema)) |

## Out of Scope

- Per-iteration worktrees (one worktree per ralph issue). The default workflow does not switch branches per issue, so a per-iteration model has no use today. ([D2](artifacts/decision-log.md#d2-branch-naming-for-the-run))
- Automated cleanup of stale worktrees at startup. pr9k warns and lets the user decide. ([D5](artifacts/decision-log.md#d5-stale-worktree-handling))
- Auto-initialization of submodules in the new worktree. ([D6](artifacts/decision-log.md#d6-submodule-policy))
- Configurable worktree location. The first iteration ships a single fixed location (sibling of the primary). ([D1](artifacts/decision-log.md#d1-worktree-location))
- Adding a `primaryDir` field to the statusline payload. ([D12](artifacts/decision-log.md#d12-statusline-payload-shape))
- Per-step bookmarks for finer-grained resume. The iteration-loop-level resume via `get_next_issue` idempotency is sufficient for current needs. ([D15](artifacts/decision-log.md#d15-cross-run-resume-via-active-run-state-file))
- Cross-platform support beyond POSIX. The PID liveness check uses POSIX semantics (consistent with existing `workflowio` crash-temp detection). Windows support is not in scope.
- Behavior change for `pr9k sandbox shell` and `pr9k workflow`. ([D9](artifacts/decision-log.md#d9-subcommand-scope))
- A persistent run lock file separate from the active-run state file. The state file's PID + binary-path check fills the soft-lock role. ([D10](artifacts/decision-log.md#d10-concurrent-runs))

## Deferred (YAGNI)

### `logDir` split (logs in primary, work in worktree)
- **Why deferred:** simpler-version test — keeping the worktree on disk by default already satisfies the evidence (logs preserved for the user to review). When `autoCleanup: true` is set, logs intentionally vanish with the worktree; that is the user's chosen tradeoff.
- **Reopen when:** users with `autoCleanup: true` report that they want post-run logs preserved despite the cleanup, AND no simpler workaround (e.g., per-run log archival) suffices.
- **Source:** investigation open question Q1, validation finding V7.

### Submodule auto-initialization
- **Why deferred:** evidence test — pr9k itself has no submodules, no user has reported a submodule-using project as a target.
- **Reopen when:** a user reports a submodule-using project failing inside a worktree.
- **Source:** validation finding V10; team finding F-13.

### Submodule startup warning
- **Why deferred:** evidence test — emitting a startup warning for a class of user that has not yet been reported is observability machinery for a failure mode that has never occurred.
- **Reopen when:** a user reports a submodule-using project failing without being able to find the limitation in the docs.
- **Source:** team finding F-13.

### Statusline payload `primaryDir` field
- **Why deferred:** evidence test — no user has reported that their statusline script needs both paths.
- **Reopen when:** a statusline-script author reports the missing context as a real problem.
- **Source:** validation finding V11.

### Concurrent-run lock file
- **Why deferred:** evidence test — the active-run state file's PID + binary-path check is already a soft lock. A dedicated lock file would add complexity for no behavioral gain.
- **Reopen when:** a user reports a real concurrent-run incident that the soft check failed to prevent.
- **Source:** team finding F-04.

### Mid-run disk-full proactive warning
- **Why deferred:** evidence test — no user has reported a mid-run disk-full incident.
- **Reopen when:** a user reports being unable to diagnose a mid-run failure as disk-full.
- **Source:** team finding F-06.

### Cross-platform (Windows) support
- **Why deferred:** evidence test — no user has requested Windows support and the existing pr9k codebase is POSIX-assuming throughout.
- **Reopen when:** a user requests Windows support, at which point a process-liveness abstraction would be needed.
- **Source:** validation finding V10.

### Schema-version migration tooling for the active-run state file
- **Why deferred:** simpler-version test — when the schema changes incompatibly, pr9k attempts a best-effort resume and falls back to "treat as corrupt" if structural fields are missing. A migration framework is overkill for a single small JSON file.
- **Reopen when:** a real schema-version mismatch causes data loss in practice.
- **Source:** validation finding V11.

## Open Items

- **OI-1:** The push-step change required by [D7](artifacts/decision-log.md#d7-first-push-sets-upstream-and-step-reorder) is shared with the in-place workflow path. The behavioral question — should push failures in the in-place workflow also enter error-recovery mode? — has not been settled by the user. Recommended answer: yes (failed pushes are dangerous regardless of mode). Non-blocking.
  - **Resolves when:** the user accepts or amends the recommendation.
  - **Blocks implementation:** No.

- **OI-2:** The default workflow's finalize phase writes its code-review output to a path that the `review_verdict` script reads from CWD. If the two paths disagree, `review_verdict` silently skips. Pre-existing inconsistency, more visible under `worktrees.enabled` because the working directory is no longer the user's familiar primary checkout. The fix is to confirm both ends of the path agree.
  - **Resolves when:** implementation review confirms the finalize phase writes to the path the verdict reads.
  - **Blocks implementation:** No.

## Summary

- **Outcome delivered:** users can run pr9k against the next feature without disrupting their primary checkout, can be killed and restarted at any moment with no loss of in-flight work, and have ergonomic cleanup affordances. All run artifacts (commits, logs, transcripts) are preserved on disk by default or removed automatically when the user opts into `autoCleanup`. Resume is automatic, deterministic, and free of manual intervention.
- **Primary actors:** the pr9k user; downstream consumers are git, GitHub (via `gh`), and the existing Docker sandbox.
- **Decisions settled by evidence:** 19 (D1–D19) — see [artifacts/decision-log.md](artifacts/decision-log.md).
- **Decisions settled by user input:** 2 (D8 reshape and D13–D14–D15 redirects came from user feedback) — see [artifacts/decision-log.md](artifacts/decision-log.md).
- **Sub-agents consulted:** junior-developer, devops-engineer, behavioral-analyst, three evidence-based-investigator passes, two adversarial-validator passes — see [artifacts/team-findings.md](artifacts/team-findings.md).
- **Key adjustments from review:** worktree relocated from inside `.git/` to a sibling of the primary; push failures explicitly enter error-recovery mode; stale-detection filter switched to path-prefix only and excludes the active worktree; submodule startup warning removed; observability extended to log file and per-invocation iteration log; concurrent-run guard via PID + binary-path check; default workflow's `Close issue` and `Git push` steps reordered (push before close) to eliminate silent-skip windows on resume; cross-run resume via active-run state file replacing the original "no resume" stance.
- **Remaining open items:** 2 (OI-1, OI-2 — neither blocks implementation).
- **Technical notes:** 4 (T1–T4) — see [artifacts/feature-technical-notes.md](artifacts/feature-technical-notes.md).
