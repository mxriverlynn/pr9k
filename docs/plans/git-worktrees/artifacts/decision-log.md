# Decision Log: useWorktrees

This file records every decision settled while specifying the `useWorktrees` feature. Behavioral statements live in [../feature-specification.md](../feature-specification.md); this file captures the history, rationale, evidence, and rejected alternatives for each decision.

## Trivial decisions

(None at present — all decisions have at least one rejected alternative or were reshaped by review findings.)

## Full decisions

### D1: Worktree location

- **Question:** Where on disk should the worktree live?
- **Decision:** As a sibling of the primary checkout, at `<primary-parent>/<primary-basename>-pr9k-<run-stamp>/`. A single fixed location, not configurable in the first iteration.
- **Rationale:** A sibling location avoids two real risks the original `.git/`-internal location did not. First, paths inside `.git/` may not be bind-mountable by Docker Desktop on macOS under all common File Sharing configurations (validation Remaining Risk; team finding F-08). A sibling location lives under the user's home directory hierarchy, which is the default-shared filesystem on macOS Docker Desktop. Second, a sibling location creates no path-nesting confusion (a worktree under `.git/` ends up containing its own nested `.pr9k/` runtime directory, which is correct but disorienting). The cost of a sibling location is that it pollutes the parent directory; this is the conventional layout for `git worktree` and matches user expectation.
- **Evidence:** Validation Remaining Risk on Docker bind-mount; team finding F-08; `../investigation/01-worktree-mechanics.md` section 10 (location options tested).
- **Rejected alternatives:**
  - `<primary>/.git/pr9k-worktrees/<run-stamp>/` — original choice. Rejected after team finding F-08 surfaced an untested macOS Docker Desktop bind-mount risk for `.git/`-internal paths.
  - `<primary>/.pr9k/worktrees/<run-stamp>/` — rejected because `.pr9k/` itself is not currently in `.gitignore` (only specific subdirectories are), so a worktree directory there would appear as untracked in `git status` until an additional ignore entry was added; that is more invasive than placing the worktree outside the repo entirely.
  - User home `~/.pr9k/worktrees/<repo-hash>/<stamp>/` — rejected because it physically separates the worktree from the repo and surprises users who inspect worktrees with `git worktree list`.
  - Configurable location — rejected on YAGNI grounds; deferred until a user reports a real conflict.
- **Linked technical notes:** [T1](feature-technical-notes.md#t1-sequencing-constraint)
- **Driven by findings:** F-08
- **Dependent decisions:** D5 (stale worktree detection looks for entries whose directory name matches the `<primary-basename>-pr9k-*` pattern).
- **Referenced in spec:** Outcome, Preconditions, Primary Flow (step 3), Out of Scope.

### D2: Branch naming for the run

- **Question:** What branch should the worktree sit on?
- **Decision:** A brand-new branch named `pr9k/<run-stamp>` created off the primary's HEAD at worktree-creation time. The run-stamp is the millisecond-precision stamp pr9k already uses for its log directory (so the branch and the log dir are the same identifier).
- **Rationale:** Git enforces "one branch can be checked out in only one worktree at a time" (`../investigation/01-worktree-mechanics.md` E1), so the worktree must be on a different branch than whatever the primary has checked out. Creating a fresh branch off the primary's HEAD gives pr9k a clean starting point with no risk of conflict. The `pr9k/` prefix makes branches discoverable with `git branch --list 'pr9k/*'` and removable with the same pattern. The run-stamp is unique by construction.
- **Evidence:** `../investigation/01-worktree-mechanics.md` E1 (branch uniqueness), E2 (auto-create from HEAD); existing run-stamp convention in [`docs/code-packages/logger.md`](../../../code-packages/logger.md).
- **Rejected alternatives:**
  - Reuse the primary's current branch — rejected because git refuses; this is a hard constraint.
  - Detached HEAD — rejected because it makes pushing painful and the user can't easily check out the result later.
  - Per-iteration branches inside the worktree — rejected because the default workflow's feature-work prompt explicitly says "do not switch branches" (validation finding V9).
- **Linked technical notes:** [T2](feature-technical-notes.md#t2-branch-uniqueness)
- **Driven by findings:** —
- **Dependent decisions:** D5 (stale-worktree detection lists `pr9k/`-prefixed branches as a discovery aid), D7 (first push needs upstream because the branch is fresh).
- **Referenced in spec:** Outcome, Primary Flow (step 3), Edge Cases (detached HEAD, branch already exists), Out of Scope.

### D3: Log durability

- **Question:** Where do log files, the iteration log, and per-step transcripts live so they survive after the run, and how are they made discoverable from outside the TUI?
- **Decision:** Logs and artifacts live inside the worktree's `.pr9k/logs/` exactly as they do today in the in-place model. The worktree itself is left on disk after the run by default ([D4](#d4-cleanup-policy)), so the logs persist. The worktree path, the primary path, and the run's branch name are written to the run's log file (header) and to the run's iteration log (first record), so post-run users can grep structured artifacts to learn which worktree a run used without depending on TUI history.
- **Rationale:** The simplest version that satisfies the user-stated need ("monitor a parallel run, review logs after"). Splitting a separate `logDir` from `projectDir` would be a four-file change inside pr9k for no behavioral gain. Validation finding V7 explicitly rules out "ephemeral logs that vanish on cleanup" as the default; keeping the worktree on disk avoids that failure mode without the split. Team finding F-09 surfaced that TUI-only surfacing of the worktree path was not enough — a user away from the terminal during a long run cannot scroll back to read it; structured log records solve that.
- **Evidence:** Validation finding V7 in `../investigation/04-validation.md`; the four log/artifact construction sites in `../investigation/02-projectdir-flow.md` E10 that would otherwise need a `logDir` parameter; team finding F-09.
- **Rejected alternatives:**
  - Split `logDir` from `projectDir` so logs live in the primary checkout — rejected on simpler-version grounds; deferred to YAGNI with the trigger "auto-remove becomes the default."
  - TUI-only surfacing of the worktree path — rejected because users away from the terminal cannot recover the path. Replaced by structured log + iteration-log records.
- **Linked technical notes:** —
- **Driven by findings:** F-09
- **Dependent decisions:** D4 (default cleanup keeps the worktree because logs are inside it), D13 (autoCleanup opt-in trades log durability for cleanup ergonomics), D11 (startup feedback uses the same structured records).
- **Referenced in spec:** Outcome, Edge Cases (logs and artifacts on completion).

### D4: Cleanup policy (default behavior when `autoCleanup` is false)

- **Question:** When `worktrees.autoCleanup` is `false` (the default), what happens to the worktree on graceful run completion? On user-requested quit? On crash?
- **Decision:**
  - On graceful completion: pr9k leaves the worktree on disk and writes a closing record (worktree path + branch + removal command) to the run's log file and iteration log. The TUI's final summary line surfaces the same information plus the count of all `pr9k/*` worktrees on disk.
  - On user-requested quit (`q`/`y`, SIGINT, SIGTERM): same as graceful completion.
  - On crash (SIGKILL, panic, power loss): the worktree is left on disk. The next run detects and warns about it ([D5](#d5-stale-worktree-handling)).
  - When `worktrees.autoCleanup: true` is set, the graceful-completion and graceful-quit paths instead remove the worktree and delete the branch — see [D13](#d13-autocleanup-behavior). Crash paths still leave the worktree on disk regardless of the autoCleanup setting.
- **Rationale:** The default keeps the worktree because that preserves logs and partial work for review. The opt-in `autoCleanup` (D13) covers users who want the cleanup automated; both are first-class behaviors. The user has full control via the config or via the `pr9k worktree prune` subcommand (D14). No new state to manage, no risk of pr9k accidentally deleting work without consent.
- **Evidence:** Validation finding V7 (logs cannot be ephemeral); investigation open questions Q3 and Q4; team finding F-01 (need growth visibility in final summary).
- **Rejected alternatives:**
  - Auto-remove on success, keep on failure — rejected on YAGNI grounds; no user has asked for it. Deferred with the reopening trigger documented in the spec.
  - Auto-remove always — rejected because it destroys logs (V7).
  - Configurable per-run via `worktreeKeep` — rejected on YAGNI grounds; the simpler "always keep" satisfies all known evidence.
- **Linked technical notes:** —
- **Driven by findings:** F-01
- **Dependent decisions:** D3 (logs live in the worktree because we keep it), D11 (final-summary content includes worktree count).
- **Referenced in spec:** Outcome, Primary Flow (steps 10–11), Alternate Flows (graceful quit, SIGKILL), Deferred (YAGNI).

### D5: Stale worktree handling at startup

- **Question:** When a new run starts and finds existing pr9k-owned worktrees from prior crashed runs, what does pr9k do? How is the *active* worktree (referenced by the active-run state file) excluded from stale-detection?
- **Decision:** pr9k discovers existing worktrees, filters for entries whose directory name matches the `<primary-basename>-pr9k-*` pattern (path-prefix filter only — branch state is not used for filtering), **excludes the worktree referenced by `<primary>/.pr9k/active-run.json` (if present)**, and warns about the rest. The warning behavior scales with count: ≤ 5 stale entries are listed inline in the TUI; above that, a single summary line is shown in the TUI and the individual paths are written only to the log file. pr9k does not auto-remove. The new run proceeds (either by resuming into the active worktree or by creating a fresh one).
- **Rationale:** Auto-removal is destructive. A stale worktree may contain commits the user hasn't yet pushed (e.g., the prior crash happened before the final push) or files the user wants to recover. Warning is non-destructive, gives the user information, and matches the unix philosophy of "don't take destructive action without consent." `git worktree prune` alone is not enough because it only removes entries whose directory was deleted out-of-band — the typical crash leaves the directory intact, so prune is a no-op (validation finding V12). Path-prefix-only filtering closes the blind spot team finding F-03 surfaced: if the user manually deletes the `pr9k/<stamp>` branch but leaves the worktree directory, a branch-prefix filter would silently miss it. Aggregation above 5 entries (F-05) prevents alert fatigue at scale.
- **Evidence:** Validation finding V12; team findings F-03, F-05; `../investigation/01-worktree-mechanics.md` section 4 (cleanup paths and what `prune` covers).
- **Rejected alternatives:**
  - Auto-prune at startup — rejected because it destroys work without consent.
  - Filter by path AND branch prefix — rejected because deleted branches with surviving directories evade the filter (F-03).
  - Print one warning line per stale entry regardless of count — rejected because at scale (50+ entries from accumulated runs) it pushes the rest of the TUI off-screen and trains users to ignore startup warnings (F-05).
  - Require an explicit `--prune-worktrees` flag — rejected as unnecessary scope; the user can run `git worktree remove --force <path>` directly.
  - A new `pr9k worktree prune` subcommand — deferred to YAGNI.
- **Linked technical notes:** [T2](feature-technical-notes.md#t2-branch-uniqueness), [T4](feature-technical-notes.md#t4-active-run-state-file-schema)
- **Driven by findings:** F-03, F-05, F-08, V6 (resume validation)
- **Dependent decisions:** D15 (the active-run state file is the source of truth for which worktree is active and therefore excluded).
- **Referenced in spec:** Alternate Flows (stale worktree detected), Edge Cases (deleted branch, surviving directory), User Interactions (Feedback).

### D6: Submodule policy

- **Question:** What happens when `useWorktrees: true` is enabled in a repository that uses git submodules?
- **Decision:** pr9k does not auto-initialize submodules in the new worktree and does not warn at startup when `.gitmodules` is present. The limitation is documented in the user-facing how-to guide.
- **Rationale:** pr9k itself has no submodules and no current user has reported a submodule-using project as a target (validation finding V10). Auto-initializing submodules is a non-trivial dependency (submodule semantics, recursive init, network access) that adds risk without evidence of need. A startup warning was originally proposed (initial draft of D6) but team finding F-13 flagged it as observability machinery for a failure mode that has never occurred — the simpler answer is documentation that the user finds when something breaks.
- **Evidence:** Validation finding V10; team finding F-13; pr9k repository has no `.gitmodules`.
- **Rejected alternatives:**
  - Auto-run `git submodule update --init --recursive` after worktree creation — rejected on YAGNI grounds; deferred with a reopening trigger.
  - Refuse to run when `.gitmodules` exists — rejected because it would block users who have submodules but don't need them initialized for their workflow.
  - Print a startup warning when `.gitmodules` is present — rejected on YAGNI grounds (F-13); replaced with how-to documentation.
- **Linked technical notes:** —
- **Driven by findings:** F-13
- **Dependent decisions:** —
- **Referenced in spec:** Edge Cases (repository contains submodules), Deferred (YAGNI).

### D7: First-push sets upstream, surfaces failures, and step reorder

- **Question:** The push step in the default workflow runs `git push` (no upstream flag) and currently traps all exit codes to zero. A brand-new branch in a fresh worktree has no tracking ref, so `git push` fails — but the trap hides the failure. What does the spec commit to?
- **Decision:** Three coordinated changes:
  1. The push step uses upstream-setting semantics on first push so a brand-new run-branch is published to the remote and acquires a tracking ref.
  2. The push step surfaces non-zero exit codes; on failure, pr9k enters its existing error-recovery mode (the c/r/q prompt) with the verbatim git stderr written to the iteration log and the run's log file.
  3. **The `Close issue` step in the default workflow is reordered to run *after* `Git push` (D18).** This eliminates the silent-skip window where a kill between close and push would leave the issue closed with unpushed commits, and the resumed run would skip the issue entirely.
- **Rationale:** Without upstream-setting, the first push of `pr9k/<run-stamp>` fails. Without surfacing the failure, pr9k closes the issue, prints "done," and the user is told everything succeeded — but no code is on the remote (validation finding V14 / E14; team finding F-02). This is a pre-existing bug that becomes a much louder problem in the worktree path because every `useWorktrees` run starts on a new branch with no upstream, and once the first push fails every subsequent push in the same run fails for the same reason. Routing failures into the existing error-recovery mode (rather than aborting silently or aborting hard) gives the user the standard c/r/q affordance and matches how every other failure in the workflow behaves.
- **Evidence:** `../investigation/03-behavioral-boundaries.md` (silent push failure); validation finding V14; team finding F-02; team finding F-07 (branch-protection rejection is a real reason a push can fail).
- **Rejected alternatives:**
  - Set the upstream at worktree-creation time (push an empty commit before the workflow runs) — rejected because it pushes empty commits before the workflow has done any work, wastes bandwidth, and clutters the remote.
  - Leave the script unchanged and document the limitation — rejected because the silent-failure mode means the user can't tell when the workflow "succeeded" but no code landed.
  - Abort the entire run on push failure (no recovery) — rejected because retry is often correct (transient network errors).
  - Skip the failed iteration and continue to the next — rejected because it implies the run is making progress when it isn't (F-02).
- **Linked technical notes:** [T3](feature-technical-notes.md#t3-first-push-upstream)
- **Driven by findings:** F-02, F-07, V1 (resume validation — gating on D7)
- **Dependent decisions:** D15 (cross-run resume relies on push-before-close to avoid silent-skip windows), D18 (the step-order reorder).
- **Referenced in spec:** Edge Cases (first push, remote rejection), Coordinations (Git remote), User Interactions (Error states), Alternate Flows (push step fails), Open Items (OI-1).

### D8: Config shape — `worktrees` block

- **Question:** How is the worktree feature exposed in `config.json`? Is it a single field or a structured block? Is there a CLI override?
- **Decision:** A top-level `worktrees` block in `config.json` containing the boolean fields `enabled` (default `false`) and `autoCleanup` (default `false`). No CLI flag override. The block shape is:

  ```json
  {
    "worktrees": {
      "enabled": true,
      "autoCleanup": false
    }
  }
  ```

  Both fields are optional; an absent block is equivalent to `{ enabled: false, autoCleanup: false }`.
- **Rationale:** The original draft used a single boolean (`useWorktrees: true`) on YAGNI grounds, but the user redirected toward a block during review because cleanup ergonomics need a second knob (`autoCleanup`) and a block makes future additions (e.g., `location`, `branchPrefix`, `keepLogs`) compatible without re-shaping the config. Default-off preserves the current in-place behavior for users who don't opt in. A CLI flag is rejected for the same reasons as the original draft — it duplicates surface area and creates a precedence question, and no user has asked for one.
- **Evidence:** User redirect in conversation; ADR `20260410170952-narrow-reading-principle.md` (workflow content lives in config.json, not Go code).
- **Rejected alternatives:**
  - Single top-level boolean `useWorktrees: true` — rejected after the user redirected toward a block to accommodate `autoCleanup` and future fields without breaking the schema.
  - A flat pair of top-level fields (`useWorktrees: true`, `worktreeAutoCleanup: false`) — rejected because the two fields are conceptually one feature; a block makes that relationship clear and validation simpler.
  - A CLI flag `--worktrees` or `--worktree-auto-cleanup` — rejected as duplicate surface area.
- **Linked technical notes:** [T1](feature-technical-notes.md#t1-sequencing-constraint)
- **Driven by findings:** User redirect (post-review)
- **Dependent decisions:** D13 (autoCleanup behavior nests inside this block).
- **Referenced in spec:** Outcome, Trigger, Edge Cases (default behavior unchanged, autoCleanup-without-enabled validation), Primary Flow (step 2).

### D9: Subcommand scope

- **Question:** Do `pr9k sandbox shell` and `pr9k workflow` (the workflow-builder TUI) honor `useWorktrees`?
- **Decision:** No. Both subcommands resolve their own working directory independently and ignore the flag. Only the workflow runner (`pr9k` or `pr9k workflow run`) honors `useWorktrees`.
- **Rationale:** `sandbox shell` is an interactive shell session against the user's chosen project; running it inside an automation worktree would be confusing. The workflow-builder edits the workflow config itself (which lives in the primary checkout's workflow bundle, not the worktree); editing it inside a worktree would mean the edits live in a per-run branch and never reach the bundle. Both behaviors would surprise users.
- **Evidence:** `../investigation/02-projectdir-flow.md` E12, E23 (independent project-dir resolution for these subcommands).
- **Rejected alternatives:**
  - Make all subcommands worktree-aware — rejected because none of them benefit and several are actively confusing.
- **Linked technical notes:** —
- **Driven by findings:** —
- **Dependent decisions:** —
- **Referenced in spec:** Edge Cases (subcommand interaction), Coordinations.

### D10: Concurrent runs (soft lock via active-run state file)

- **Question:** What happens if two pr9k instances run against the same primary checkout at the same time?
- **Decision:** The active-run state file at `<primary>/.pr9k/active-run.json` (D15) doubles as a soft lock. On startup, if the state file exists and its recorded PID is alive AND the recorded binary path matches the running pr9k binary, the new invocation exits non-zero with "another pr9k appears to be running for this primary checkout (PID N). Use `pr9k --fresh` to override." If PID is alive but binary doesn't match, treat as PID-reuse-by-unrelated-process and resume normally. If PID is dead, resume normally.
- **Rationale:** The state file already needs to exist for cross-run resume (D15). Reusing it as a soft lock costs nothing extra. PID alone is unreliable due to OS PID reuse (validation V2); the binary-path check addresses the false-positive case where PID got recycled by an unrelated process. A formal lock file (e.g., `.pr9k/pr9k.lock` with `flock`) is rejected for the same YAGNI reason as before — no user has reported a concurrent-run incident, and the soft check covers the accidental-double-invoke case cleanly.
- **Evidence:** Team finding F-04; validation finding V2 (PID-reuse false positives).
- **Rejected alternatives:**
  - Dedicated `flock`-based lock file — rejected on YAGNI grounds; soft lock via state file suffices.
  - PID-only liveness check — rejected because PID reuse on macOS is real (V2). Binary-path match closes the false-positive window.
  - Force-skip already-implemented issues — rejected because it requires understanding what "already implemented" means across two concurrent runs, which is itself a coordination problem.
- **Linked technical notes:** [T4](feature-technical-notes.md#t4-active-run-state-file-schema)
- **Driven by findings:** F-04, V2 (resume validation)
- **Dependent decisions:** D15 (the state file mechanism this builds on).
- **Referenced in spec:** Preconditions, Edge Cases (concurrent-run rows), Out of Scope, Deferred.

### D11: Worktree startup feedback and TUI surfacing

- **Question:** How does the user observe that a run is operating in a worktree, both during the run and after it ends?
- **Decision:** The user observes the worktree from three places:
  1. **The run's log file (header line)** records the worktree path, primary path, and branch name when the run starts.
  2. **The run's iteration log (first record)** records the same fields as a structured event.
  3. **The TUI** prefixes its iteration line with the worktree's basename during the run, and the final summary line shows the worktree path, branch name, and the count of all `pr9k/*` worktrees on disk.
- **Rationale:** The original draft committed to a status-header indicator and a final-summary block but did not specify how those would be rendered against the existing TUI surfaces (team findings JD-002, JD-003). The existing TUI has a free-text iteration line that can carry a basename prefix without new infrastructure, and the existing final-summary line can be extended to include the new fields. The structured log records (F-09) are the durable post-run artifact that does not depend on the TUI being visible. Together these three surfaces give a user who is doing other work during the run a way to confirm what's happening (TUI prefix), a way to find the worktree after the run ends (final-summary line), and a way to recover the path after the TUI is gone (log file + iteration log grep).
- **Evidence:** Team findings JD-001 (run-stamp definition), JD-002 (status header has no slot), JD-003 (final summary infrastructure), F-09 (structured log fields needed), F-01 (worktree count gives growth visibility).
- **Rejected alternatives:**
  - A new TUI status-header field for the worktree path — rejected because it requires new chrome infrastructure that the spec was implying without naming.
  - TUI-only surfacing — rejected because users away from the terminal cannot recover the path after the TUI is dismissed.
  - Print every existing `pr9k/*` worktree path in the final summary — rejected as too noisy at scale; the count + a `git worktree list` pointer is enough.
- **Linked technical notes:** —
- **Driven by findings:** JD-001, JD-002, JD-003, F-09, F-01
- **Dependent decisions:** D3 (logs/observability fields), D5 (stale-worktree warning aggregation).
- **Referenced in spec:** Outcome, Primary Flow (steps 6, 7, 10), User Interactions, Edge Cases.

### D12: Statusline payload shape

- **Question:** Should the statusline JSON payload gain a `primaryDir` field so user-authored statusline scripts can show both paths?
- **Decision:** Not in the first iteration. The existing `projectDir` field is repurposed to carry the worktree path while `worktrees.enabled` is true.
- **Rationale:** No user has asked for both paths. Adding a field is additive and backwards-compatible (existing scripts don't read it), but adding it preemptively is YAGNI. The single-path design is simpler and matches the existing semantics ("the directory where the workflow operates").
- **Evidence:** Validation finding V11; statusline payload documented in [`docs/code-packages/statusline.md`](../../../code-packages/statusline.md).
- **Rejected alternatives:**
  - Add `primaryDir` now — rejected on YAGNI grounds; deferred with a reopening trigger.
- **Linked technical notes:** —
- **Driven by findings:** —
- **Dependent decisions:** —
- **Referenced in spec:** Out of Scope, Deferred (YAGNI).

### D13: autoCleanup behavior

- **Question:** Should pr9k offer an opt-in `autoCleanup` mode that removes the worktree, its branch, and the active-run state file on graceful completion, and what are its semantics around logs, crash paths, user-quit, and validation?
- **Decision:** Yes. `worktrees.autoCleanup: true` causes pr9k to remove the worktree (`git worktree remove --force`), delete the run's branch (`git branch -D pr9k-<worktree-stamp>`), AND remove the active-run state file at the end of the graceful-shutdown path — but only on **`Completed` or `LoopBroken`** exit reasons (D16). On **`UserQuit`** (the user pressed `q`/`y` or sent SIGINT/SIGTERM), autoCleanup is **suppressed** and the state file is **kept** so the next pr9k invocation auto-resumes. The closing log record is written *before* removal so the structured log captures what happened, but anything that lived only inside the worktree (the log file itself, per-invocation iteration log, per-step transcripts) is removed with it. Crash paths (SIGKILL, panic, power loss) skip cleanup entirely because they skip shutdown. Default is `false`. The validator rejects a config that sets `autoCleanup: true` without `enabled: true`.
- **Rationale:** The user asked for cleanup to be easy. Auto-cleanup on natural completion eliminates the manual `git worktree remove` step for users whose workflow tolerates ephemeral logs. Suppressing cleanup on `UserQuit` is what makes resume work — if the user quits expecting to come back to their work, autoCleanup must not erase it. Crash paths can't run shutdown code at all, which is the case `pr9k worktree prune` and the resume mechanism handle. Validating `autoCleanup` requires `enabled` because the option is meaningless otherwise.
- **Evidence:** User redirect in conversation; validation finding V7 (logs cannot be ephemeral by default); validation finding V3 (need ExitReason to suppress cleanup on UserQuit); the user's hard requirement for auto-resume after kill.
- **Rejected alternatives:**
  - `autoCleanup` defaults to `true` — rejected because the safe default is to keep work.
  - autoCleanup fires on every graceful exit including `UserQuit` — rejected because that erases the in-flight work that resume depends on.
  - Two separate fields (`removeOnSuccess`, `removeOnQuit`) — rejected on YAGNI grounds; the ExitReason model gives the right behavior automatically.
  - Always preserve logs by copying them out of the worktree before removal — rejected on simpler-version grounds; the `logDir` split remains in Deferred.
  - Cleanup on hard crash via post-run startup probe — rejected because the resume mechanism handles this case more cleanly (prior worktree+state become recoverable, not garbage).
- **Linked technical notes:** —
- **Driven by findings:** User redirect (post-review), V3 (resume validation), V7 (worktree investigation).
- **Dependent decisions:** D14 (prune subcommand handles the crash-path case), D15 (cross-run resume relies on `UserQuit` keeping the state file), D16 (ExitReason is the control input).
- **Referenced in spec:** Outcome, Trigger, Primary Flow (step 12), Alternate Flows (graceful quit), Edge Cases (autoCleanup-related rows).

### D14: `pr9k worktree prune` subcommand

- **Question:** Should pr9k ship a subcommand that removes all pr9k-owned worktrees and branches in one step, regardless of whether `worktrees.enabled` was set in the active config?
- **Decision:** Yes. A new `pr9k worktree prune` subcommand is added. It discovers pr9k-owned worktrees using the same path-prefix filter D5 uses (directory name matches `<primary-basename>-pr9k-*`), removes each with `git worktree remove --force` (because pr9k worktrees routinely contain uncommitted intermediate workflow files), and deletes each `pr9k/<stamp>` branch with `git branch -D`. A `--dry-run` flag lists what would be removed without changing anything. Per-worktree errors are surfaced verbatim and the subcommand continues with the rest; the exit code is non-zero if any removal failed. The subcommand is independent of the workflow config — it works whether or not `worktrees.enabled` is set, and even works on a primary checkout that never used the worktree feature (it simply finds no entries and exits zero).
- **Rationale:** The user explicitly asked for this subcommand to make cleanup not a manual process. Even with `autoCleanup: true`, hard crashes leave worktrees on disk (D13). Without a subcommand, the user must inspect `git worktree list`, identify pr9k-owned entries, run `git worktree remove --force` for each, and `git branch -D` for each branch — a multi-step manual chore. A single `pr9k worktree prune` collapses that into one command. `--dry-run` is the safe default-discovery mode for users who want to see what would be affected before committing. The path-prefix filter ensures pr9k never touches user-created worktrees or branches that aren't pr9k's.
- **Evidence:** User redirect in conversation ("include the `pr9k worktree prune` subcommand in the plan"); D5 already establishes the path-prefix-based filter.
- **Rejected alternatives:**
  - Interactive prompt before each removal — rejected because the user's stated goal is "easy, not manual"; interactive prompts re-introduce manual labor. `--dry-run` covers the "I want to see first" case.
  - Add `pr9k worktree list` as a separate subcommand — rejected on YAGNI grounds; `git worktree list` already exists and `pr9k worktree prune --dry-run` covers the pr9k-filtered listing case.
  - Make prune accept a path argument to remove a specific worktree — rejected on YAGNI grounds; `git worktree remove` already does this. `pr9k worktree prune` is the bulk operation.
  - Run prune automatically at startup — rejected because it conflicts with D5's "warn, don't auto-remove" stance; the user owns the cleanup decision.
- **Linked technical notes:** —
- **Driven by findings:** User redirect (post-review)
- **Dependent decisions:** —
- **Referenced in spec:** Outcome, Trigger, Alternate Flows (`pr9k worktree prune` invocation), Edge Cases (prune-related rows), Coordinations.

### D15: Cross-run resume via active-run state file

- **Question:** When pr9k is invoked on a primary checkout where a prior run was killed (mid-iteration, mid-step, anywhere), does the new run continue the prior run's work — automatically, deterministically, with no user intervention?
- **Decision:** Yes. pr9k writes an active-run state file at `<primary>/.pr9k/active-run.json` at the start of every run with `worktrees.enabled: true`. The file contains the worktree path, branch name, worktree-stamp, PID, binary path, started-at timestamp, schema version, and pr9k version (T4). On every pr9k startup, the file is read; if it exists and points to a valid worktree on the same primary, AND the recorded process is no longer running (PID dead, or PID alive but binary doesn't match — the PID-reuse case), pr9k auto-resumes by entering the existing worktree on its existing branch. No new worktree is created, no new worktree-stamp is generated; only a fresh invocation-stamp is generated for log/artifact paths. The state file is removed at the end of a `Completed` or `LoopBroken` run (D16). On `UserQuit` it is kept (so the next invocation auto-resumes). On hard kills it is kept by virtue of nothing running. The `--fresh` CLI flag (D19) lets the user explicitly discard the in-flight state.

  Resume is **iteration-loop-level**, not step-level: the resumed run's iteration loop calls `get_next_issue` as it normally would. Because the script returns the lowest-numbered open ralph issue and a prior run that died mid-iteration left its issue open (D7+D18 ensure push-before-close so closed-but-unpushed cannot happen), the in-flight issue is naturally re-delivered. The iteration runs again from feature-work onward on a branch that already carries any prior commits.

- **Rationale:** The user stated this as a hard requirement: "pr9k MUST pick up the existing work and worktree, to continue. There must not be any manual 'find a previous worktree' step." The original D15 ("no cross-run resume") was YAGNI-deferred on the assumption that no user had asked for it; the user has now explicitly asked.

  The architecture supports cross-run resume cheaply because `get_next_issue` is naturally idempotent (E1, E14 in `../investigation/05-resume-options.md`). No iteration-loop changes are needed; the state file is just a deterministic "is there an active run?" marker. `atomicwrite.Write` and the existing crash-temp PID-liveness pattern in `workflowio` (E8, E9) are the building blocks. Six options were evaluated in `../investigation/05-resume-options.md`; this is "refined Option A" (stamped worktree + state file) — the only option that satisfies all six hard requirements while preserving `autoCleanup: false` as a first-class user choice.

- **Evidence:** User redirect in conversation ("this is a massive problem, and must be solved before we can implement any of this"); `../investigation/05-resume-options.md` (full options analysis); validation findings V1, V3, V4 (gating constraints addressed by D7+D18, D16, and D17 respectively); E1, E14 (`get_next_issue` idempotency); E8 (atomicwrite); E9 (crash-temp pattern).

- **Rejected alternatives:**
  - **Option B (fixed worktree path, no state file)** — rejected because it cannot distinguish "completed and kept for inspection" from "killed mid-run" without a state file, forcing the user to abandon `autoCleanup: false`.
  - **Option C (separate clone instead of worktree)** — rejected because it offers no behavioral advantage over A and adds full-clone time/disk cost on every fresh start.
  - **Option D (per-step bookmarks)** — rejected because it requires invasive iteration-loop changes for behavior that `get_next_issue` already provides for free.
  - **Option E (in-place workflow)** — rejected because it fails the original "primary stays usable for parallel development" goal.
  - **Option F (fixed worktree + state file)** — strictly inferior to A; loses the ability to keep multiple historical worktrees on disk for users with `autoCleanup: false`.
  - **No cross-run resume (the original D15)** — rejected by user redirect.
  - **Refuse to resume on schema-version mismatch** — rejected because the user might intentionally downgrade pr9k to fix a regression; refusing would force them to abandon work. Best-effort resume with a warning is correct.
  - **A dedicated lock file separate from the state file** — rejected on YAGNI grounds; the state file's PID + binary-path check covers the soft-lock role (D10).

- **Linked technical notes:** [T4](feature-technical-notes.md#t4-active-run-state-file-schema)
- **Driven by findings:** User redirect (post-review), V1/V2/V3/V4/V5/V6 (resume validation).
- **Dependent decisions:** D5 (active worktree excluded from stale detection), D7+D18 (push-before-close eliminates the silent-skip window resume must not hit), D10 (state file doubles as soft lock), D13 (autoCleanup is gated on ExitReason, which D16 introduces), D16 (ExitReason controls when the state file is removed), D17 (per-invocation iteration logs avoid consumer breakage), D19 (--fresh as the explicit-abandon affordance), D14 (prune excludes the active worktree).
- **Referenced in spec:** Header, Background, Outcome, Invariants, Trigger, Preconditions, Primary Flow (step 3, 7, 12), Alternate Flows ("A prior run was killed..."), Edge Cases (state-file rows), Coordinations.

### D16: `RunResult.ExitReason`

- **Question:** Today `main.go:298` discards `RunResult` (`_ = workflow.Run(...)`). The state-file-removal hook needs to know whether a run completed naturally, the user quit, or `breakLoopIfEmpty` triggered an exit. How is this signal carried out of `Run`?
- **Decision:** `RunResult` gains an `ExitReason` field with at least three values: `Completed` (all configured iterations and finalize ran), `LoopBroken` (`breakLoopIfEmpty` was triggered), and `UserQuit` (the user pressed `q`/`y` or pr9k received SIGINT/SIGTERM during the run). `main.go` propagates the value to its post-Run cleanup logic. The state-file-removal hook fires on `Completed` and `LoopBroken`; it is suppressed on `UserQuit`. autoCleanup of the worktree+branch follows the same gate. SIGKILL/panic skip the post-Run cleanup entirely (no opportunity to run any code).
- **Rationale:** Without distinguishing exit reasons, the state-file-removal logic cannot be correctly placed (validation V3). Two values aren't enough — `Completed` and `LoopBroken` both mean "run is done," but distinguishing them is useful for log/audit purposes. `UserQuit` is the case that must NOT trigger removal because resume depends on the state file being kept across user-initiated quits.
- **Evidence:** Validation finding V3; `src/cmd/pr9k/main.go:298` (`_ = workflow.Run(...)` today); D15 dependency.
- **Rejected alternatives:**
  - Boolean `Completed bool` — insufficient; `breakLoopIfEmpty` and natural completion are both "done" but a boolean conflates them with `UserQuit`.
  - Detect quit reason at the call site by inspecting the TUI state — rejected because it duplicates state and is fragile.
- **Linked technical notes:** —
- **Driven by findings:** V3 (resume validation).
- **Dependent decisions:** D13 (autoCleanup is gated on this), D15 (state-file removal is gated on this).
- **Referenced in spec:** Primary Flow (step 12), Alternate Flows (graceful quit), Edge Cases (autoCleanup rows).

### D17: Per-invocation iteration log files

- **Question:** Today `iteration.jsonl` is a single file inside `.pr9k/`. Cross-run resume causes multiple pr9k invocations to share a worktree. If `iteration.jsonl` accumulates records across invocations, does anything break?
- **Decision:** Yes — `post_issue_summary` and `lessons-learned` both read all of `iteration.jsonl` and would mix records from different invocations. Switch to **per-invocation files** named `iteration-<invocation-stamp>.jsonl`. Each pr9k invocation writes to its own file inside `<worktree>/.pr9k/`. `post_issue_summary` and `lessons-learned` read the current invocation's file (the most recent one, by stamp). Past invocations' files persist alongside the worktree until autoCleanup or `pr9k worktree prune` removes the worktree.
- **Rationale:** Validation finding V4 surfaced that the existing consumers cannot tolerate mixed records. Per-invocation files are strictly simpler than introducing session-boundary records and updating consumers. They preserve forensic value (you can read a prior invocation's file directly to see what it did) and require no consumer changes beyond "use the current invocation's file" — which is naturally derived from `RunStamp()` (the invocation-stamp) at the time the script runs.
- **Evidence:** Validation finding V4; `workflow/scripts/post_issue_summary:16`, `workflow/prompts/lessons-learned.md:5,8` (existing consumers that read the whole file).
- **Rejected alternatives:**
  - Single shared file with session-boundary records — rejected because it requires updating both consumers AND the iteration-log schema.
  - Truncate `iteration.jsonl` at the start of every run — rejected because it loses prior-invocation history (defeats forensic value).
  - Keep the old file but write a parallel per-invocation file — rejected as redundant.
- **Linked technical notes:** —
- **Driven by findings:** V4 (resume validation).
- **Dependent decisions:** D15 (cross-run resume is the reason the change is needed).
- **Referenced in spec:** Outcome, Primary Flow (step 6), Edge Cases (per-invocation accumulation row).

### D18: Push-before-close step ordering

- **Question:** The default workflow today runs `Close issue` *before* `Git push`. A kill in that window closes the issue with no code on the remote. The next run skips the closed issue entirely (silent skip). Should the order be reversed?
- **Decision:** Yes. The default workflow's iteration sequence is reordered so `Git push` runs **before** `Close issue`. A push failure (which is now surfaced via D7's error-recovery routing) blocks the close from running. Combined with D15's resume mechanism, this ensures any prior run's in-flight issue is still open when a new pr9k starts, so `get_next_issue` re-delivers it correctly.
- **Rationale:** The close-then-push window is the silent-skip risk that V1 surfaced. The reorder eliminates the window for the default workflow at near-zero cost (just swapping two step entries in `workflow/config.json`). Custom workflows that follow the same pattern should adopt the reorder; the spec documents the requirement.
- **Evidence:** Validation finding V1; `workflow/config.json:28-29` (current ordering).
- **Rejected alternatives:**
  - Add a "did push succeed?" gate before `Close issue` — rejected because it duplicates D7's error-recovery routing and adds custom logic for one step.
  - Leave the order, document the silent-skip risk — rejected because the user's hard requirement for resume includes "no manual reconciliation," and silent skip violates that.
- **Linked technical notes:** —
- **Driven by findings:** V1 (resume validation).
- **Dependent decisions:** D7 (the push-surfacing change is the safety net), D15 (resume relies on this for correctness).
- **Referenced in spec:** Primary Flow (step 10), Edge Cases (first push), Coordinations (Git remote).

### D19: `--fresh` CLI flag

- **Question:** How does the user explicitly abandon an in-flight run when they don't want resume?
- **Decision:** A new `--fresh` boolean flag on the workflow runner. When passed, pr9k at startup removes the active-run state file (if any), removes the worktree it pointed to (if `worktrees.enabled` is true), and deletes the prior run's branch. Then pr9k proceeds as a normal fresh start. The flag is a no-op if no state file exists.
- **Rationale:** Resume is automatic by default — that's the user's stated requirement. But there must be an opt-out for the case where the user wants to discard partial work (e.g., the prior run was on a now-dead branch, or the user is intentionally restarting). A CLI flag is the right surface: it's a per-invocation override, doesn't pollute config, and has a clear name. The combined "remove state file + worktree + branch" semantics ensure no orphaned artifacts after `--fresh`.
- **Evidence:** Validation finding V9; user redirect on the resume requirement.
- **Rejected alternatives:**
  - `--no-resume` — rejected as semantically less clear; "fresh" implies positive action (start fresh) where "no-resume" is a double-negative.
  - A separate `pr9k abandon` subcommand — rejected because it's a one-shot operation tied to starting a run; a flag is the right shape.
  - `--fresh` removes only the state file, leaving the worktree — rejected because it would orphan the worktree (becoming stale on next D5 detection); cleaner to remove all three together.
- **Linked technical notes:** —
- **Driven by findings:** V9 (resume validation), user redirect.
- **Dependent decisions:** —
- **Referenced in spec:** Trigger, Alternate Flows (`pr9k --fresh`), User Interactions (Affordances).
