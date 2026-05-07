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
- **Linked technical notes:** —
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
- **Dependent decisions:** D4 (cleanup policy must keep the worktree because logs are inside it), D11 (startup feedback uses the same structured records).
- **Referenced in spec:** Outcome, Edge Cases (logs and artifacts on completion).

### D4: Cleanup policy

- **Question:** What happens to the worktree on graceful run completion? On user-requested quit? On crash?
- **Decision:**
  - On graceful completion: pr9k leaves the worktree on disk and writes a closing record (worktree path + branch + removal command) to the run's log file and iteration log. The TUI's final summary line surfaces the same information plus the count of all `pr9k/*` worktrees on disk.
  - On user-requested quit (`q`/`y`, SIGINT, SIGTERM): same as graceful completion.
  - On crash (SIGKILL, panic, power loss): the worktree is left on disk. The next run detects and warns about it ([D5](#d5-stale-worktree-handling)).
- **Rationale:** The simpler-version answer to the cleanup question. Keeping the worktree preserves logs and partial work for review. The user has full control via standard `git worktree` commands. No new flag, no new state to manage, no risk of pr9k accidentally deleting work.
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

- **Question:** When a new run starts and finds existing pr9k-owned worktrees from prior crashed runs, what does pr9k do?
- **Decision:** pr9k discovers existing worktrees, filters for entries whose directory name matches the `<primary-basename>-pr9k-*` pattern (path-prefix filter only — branch state is not used for filtering), and warns the user. The warning behavior scales with count: ≤ 5 stale entries are listed inline in the TUI; above that, a single summary line is shown in the TUI and the individual paths are written only to the log file. pr9k does not auto-remove. The new run proceeds and creates its own worktree.
- **Rationale:** Auto-removal is destructive. A stale worktree may contain commits the user hasn't yet pushed (e.g., the prior crash happened before the final push) or files the user wants to recover. Warning is non-destructive, gives the user information, and matches the unix philosophy of "don't take destructive action without consent." `git worktree prune` alone is not enough because it only removes entries whose directory was deleted out-of-band — the typical crash leaves the directory intact, so prune is a no-op (validation finding V12). Path-prefix-only filtering closes the blind spot team finding F-03 surfaced: if the user manually deletes the `pr9k/<stamp>` branch but leaves the worktree directory, a branch-prefix filter would silently miss it. Aggregation above 5 entries (F-05) prevents alert fatigue at scale.
- **Evidence:** Validation finding V12; team findings F-03, F-05; `../investigation/01-worktree-mechanics.md` section 4 (cleanup paths and what `prune` covers).
- **Rejected alternatives:**
  - Auto-prune at startup — rejected because it destroys work without consent.
  - Filter by path AND branch prefix — rejected because deleted branches with surviving directories evade the filter (F-03).
  - Print one warning line per stale entry regardless of count — rejected because at scale (50+ entries from accumulated runs) it pushes the rest of the TUI off-screen and trains users to ignore startup warnings (F-05).
  - Require an explicit `--prune-worktrees` flag — rejected as unnecessary scope; the user can run `git worktree remove --force <path>` directly.
  - A new `pr9k worktree prune` subcommand — deferred to YAGNI.
- **Linked technical notes:** —
- **Driven by findings:** F-03, F-05
- **Dependent decisions:** —
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

### D7: First-push sets upstream and surfaces failures

- **Question:** The push step in the default workflow runs `git push` (no upstream flag) and currently traps all exit codes to zero. A brand-new branch in a fresh worktree has no tracking ref, so `git push` fails — but the trap hides the failure. What does the spec commit to?
- **Decision:** The push step in the default workflow uses upstream-setting semantics on first push so a brand-new run-branch is published to the remote and acquires a tracking ref. The push step also surfaces non-zero exit codes; on failure, pr9k enters its existing error-recovery mode (the c/r/q prompt) with the verbatim git stderr written to the iteration log and the run's log file.
- **Rationale:** Without upstream-setting, the first push of `pr9k/<run-stamp>` fails. Without surfacing the failure, pr9k closes the issue, prints "done," and the user is told everything succeeded — but no code is on the remote (validation finding V14 / E14; team finding F-02). This is a pre-existing bug that becomes a much louder problem in the worktree path because every `useWorktrees` run starts on a new branch with no upstream, and once the first push fails every subsequent push in the same run fails for the same reason. Routing failures into the existing error-recovery mode (rather than aborting silently or aborting hard) gives the user the standard c/r/q affordance and matches how every other failure in the workflow behaves.
- **Evidence:** `../investigation/03-behavioral-boundaries.md` (silent push failure); validation finding V14; team finding F-02; team finding F-07 (branch-protection rejection is a real reason a push can fail).
- **Rejected alternatives:**
  - Set the upstream at worktree-creation time (push an empty commit before the workflow runs) — rejected because it pushes empty commits before the workflow has done any work, wastes bandwidth, and clutters the remote.
  - Leave the script unchanged and document the limitation — rejected because the silent-failure mode means the user can't tell when the workflow "succeeded" but no code landed.
  - Abort the entire run on push failure (no recovery) — rejected because retry is often correct (transient network errors).
  - Skip the failed iteration and continue to the next — rejected because it implies the run is making progress when it isn't (F-02).
- **Linked technical notes:** [T3](feature-technical-notes.md#t3-first-push-upstream)
- **Driven by findings:** F-02, F-07
- **Dependent decisions:** —
- **Referenced in spec:** Edge Cases (first push, remote rejection), Coordinations (Git remote), User Interactions (Error states), Open Items (OI-1).

### D8: Default and config shape

- **Question:** How is `useWorktrees` exposed in `config.json`? Is there a CLI override?
- **Decision:** A single boolean field `useWorktrees` at the top level of `config.json`. Default is `false` (the current in-place behavior). No CLI flag override.
- **Rationale:** The user explicitly asked for a config-file toggle ("set a `useWorktrees` option at the top level of my workflow config.json"). A CLI flag duplicates the surface area and creates a precedence question (CLI vs. config). No user has asked for a CLI override. Default-false preserves the current behavior for everyone who hasn't opted in.
- **Evidence:** User's stated request in conversation; ADR `20260410170952-narrow-reading-principle.md` (workflow content lives in config.json, not Go code).
- **Rejected alternatives:**
  - A struct (e.g., `worktree: { enabled: true, location: ..., keep: true }`) — rejected on YAGNI grounds; defer the struct shape until a second config knob is needed.
  - A CLI flag `--worktrees` — rejected as duplicate surface area; defer until a real use case (e.g., one-off runs that override the config) appears.
- **Linked technical notes:** —
- **Driven by findings:** —
- **Dependent decisions:** —
- **Referenced in spec:** Outcome, Trigger, Edge Cases (default behavior unchanged).

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

### D10: Concurrent runs

- **Question:** What happens if two pr9k instances run against the same primary checkout at the same time?
- **Decision:** No serialization is provided. Concurrent runs are listed as a precondition violation: only one pr9k instance should run against a primary checkout at a time. If the user runs two anyway, the failure surface is well-defined: both instances may select the same issue from `gh issue list` (because issue selection is not atomic), and if their run-stamps collide the second `git worktree add` fails with the standard branch-already-exists error.
- **Rationale:** No user has reported an accidental concurrent run. A lock file (`.pr9k/pr9k.lock`) is the obvious mechanism but adds startup/shutdown complexity for a failure mode that hasn't yet occurred. The simpler version is to declare the precondition and let the failure modes surface clearly.
- **Evidence:** Team finding F-04.
- **Rejected alternatives:**
  - Lock file at startup (`.pr9k/pr9k.lock`) — rejected on YAGNI grounds; deferred with a reopening trigger.
  - Force-skip already-implemented issues — rejected because it requires understanding what "already implemented" means across two concurrent runs, which is itself a coordination problem.
- **Linked technical notes:** —
- **Driven by findings:** F-04
- **Dependent decisions:** —
- **Referenced in spec:** Preconditions, Edge Cases (branch already exists), Out of Scope, Deferred.

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
- **Decision:** Not in the first iteration. The existing `projectDir` field is repurposed to carry the worktree path while `useWorktrees` is true.
- **Rationale:** No user has asked for both paths. Adding a field is additive and backwards-compatible (existing scripts don't read it), but adding it preemptively is YAGNI. The single-path design is simpler and matches the existing semantics ("the directory where the workflow operates").
- **Evidence:** Validation finding V11; statusline payload documented in [`docs/code-packages/statusline.md`](../../../code-packages/statusline.md).
- **Rejected alternatives:**
  - Add `primaryDir` now — rejected on YAGNI grounds; deferred with a reopening trigger.
- **Linked technical notes:** —
- **Driven by findings:** —
- **Dependent decisions:** —
- **Referenced in spec:** Out of Scope, Deferred (YAGNI).
