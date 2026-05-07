# Team Findings: useWorktrees

This file records every finding raised by the review team for the `useWorktrees` feature, and how each was resolved. Behavioral outcomes live in [../feature-specification.md](../feature-specification.md); decisions the findings affected live in [decision-log.md](decision-log.md); load-bearing mechanics captured live in [feature-technical-notes.md](feature-technical-notes.md).

## Major findings

### F-01: Worktree accumulation is unbounded — no growth model, no operator escape valve

- **Agent:** devops-engineer
- **Finding:** D4 keeps worktrees on disk forever. After many runs the user accumulates many worktrees, each containing a full working tree of edited files. The spec did not surface a count, did not estimate per-worktree size, and did not guide proactive cleanup. The deferred YAGNI trigger ("user reports disk-space concerns") only fires after the user has already hit the problem.
- **Resolution:** Added the count of all `pr9k/*` worktrees on disk to the TUI's final summary line and to the closing log record (D11, D4). At scale this gives the user a leading indicator of accumulation without committing pr9k to any cleanup behavior. The deferred YAGNI item for configurable cleanup-on-success is unchanged.
- **Resolved by:** evidence
- **Affected decisions:** D4, D11
- **Affected tech-notes:** —
- **Changed in spec:** Outcome, Primary Flow (step 10), User Interactions (Affordances and Feedback)

### F-02: Push failure disposition is unspecified — "surfaces" without defining run continuation behavior

- **Agent:** devops-engineer
- **Finding:** "Pr9k does not silently continue" was ambiguous across abort-the-run, abort-the-iteration, and abort-the-finalize-phase. Additionally, after a failed first push, every subsequent push in the same run also fails (because no upstream is established) — the spec did not address this cascade.
- **Resolution:** Specified that any push step's non-zero exit enters pr9k's existing error-recovery mode (the c/r/q prompt), with verbatim git stderr written to the iteration log and run log file. Added an Alternate Flow ("A push step fails") that explicitly notes the cascade: a failed first push will cause every subsequent push in the same run to fail until the upstream is established.
- **Resolved by:** evidence
- **Affected decisions:** D7
- **Affected tech-notes:** T3
- **Changed in spec:** Alternate Flows (push step fails), Edge Cases (first push, remote rejection), Coordinations (Git remote), User Interactions (Error states)

### F-03: Stale-detection filter has a blind spot — deleted branch with surviving directory is not warned about

- **Agent:** devops-engineer
- **Finding:** The original D5 filtered by both path prefix AND branch prefix. If the user manually deleted the `pr9k/<stamp>` branch but left the worktree directory, the branch filter would skip the entry and the worktree would sit on disk silently.
- **Resolution:** Changed D5 to filter by directory-name pattern alone (`<primary-basename>-pr9k-*`), regardless of branch state. Added an Edge Cases row covering this scenario explicitly.
- **Resolved by:** evidence
- **Affected decisions:** D5
- **Affected tech-notes:** —
- **Changed in spec:** Alternate Flows (stale worktree detection), Edge Cases (deleted branch, surviving directory)

### F-04: Concurrent pr9k instances — issue duplication, branch collision unspecified

- **Agent:** devops-engineer
- **Finding:** The spec modeled exactly one pr9k per primary checkout. Two concurrent instances may select the same issue (no atomicity in `gh issue list`) and may collide on `git worktree add` if their run-stamps fall in the same millisecond.
- **Resolution:** Added a Precondition stating that only one pr9k instance should run against a primary checkout at a time. Added a new D10 ("Concurrent runs") capturing the decision to not serialize via a lock file in this iteration. Added the lock file to the Deferred section with a reopening trigger.
- **Resolved by:** evidence
- **Affected decisions:** D10 (new)
- **Affected tech-notes:** —
- **Changed in spec:** Preconditions, Out of Scope, Deferred (YAGNI)

### F-05: Stale-warning volume at scale

- **Agent:** devops-engineer
- **Finding:** The original D5 specified one warning line per stale entry with no cap. After 50 accumulated runs the startup warning would be 50 lines of noise, training users to ignore it.
- **Resolution:** Added an aggregation rule to D5: ≤ 5 entries listed inline; > 5 entries shown as a single summary line. Individual paths always go to the log file regardless of count.
- **Resolved by:** evidence
- **Affected decisions:** D5
- **Affected tech-notes:** —
- **Changed in spec:** Alternate Flows (stale worktree detection), User Interactions (Feedback)

### F-06: Disk-full mid-run scenario unaddressed

- **Agent:** devops-engineer
- **Finding:** The spec covered disk-full as a worktree-creation failure but not as a mid-run failure caused by accumulated worktrees consuming space.
- **Resolution:** Added an Edge Cases row for mid-run disk-full. Added a Deferred entry for proactive disk-full warning with a reopening trigger.
- **Resolved by:** evidence
- **Affected decisions:** —
- **Affected tech-notes:** —
- **Changed in spec:** Edge Cases (mid-run disk-full), Deferred (YAGNI)

### F-07: Branch-protection rejection path is unspecified

- **Agent:** devops-engineer
- **Finding:** When the remote rejects a push due to branch-protection rules on `pr9k/*`, the spec did not specify what the user sees or how to recover.
- **Resolution:** Added a Precondition stating the remote must permit pushes to `pr9k/*`. Added the rejection scenario to Edge Cases. Routed the failure through the same error-recovery path established by F-02.
- **Resolved by:** evidence
- **Affected decisions:** D7
- **Affected tech-notes:** —
- **Changed in spec:** Preconditions, Edge Cases (remote rejects push), Coordinations (Git remote)

### F-08: Docker bind-mount of a `.git/`-internal path is untested on macOS Docker Desktop — original D1 carries rollout risk

- **Agent:** devops-engineer
- **Finding:** The original D1 placed the worktree at `<primary>/.git/pr9k-worktrees/<stamp>/`. macOS Docker Desktop's File Sharing configuration may exclude `.git/`-internal paths under some configurations; if so, the bind-mount would silently mount empty and Claude would operate on an empty workspace.
- **Resolution:** Changed D1 to a sibling location (`<primary-parent>/<primary-basename>-pr9k-<run-stamp>/`). Sibling paths live under the user's home hierarchy, which is the default-shared filesystem on macOS Docker Desktop. This eliminates the bind-mount risk entirely at the cost of a more visible directory layout.
- **Resolved by:** evidence
- **Affected decisions:** D1, D5 (stale-detection pattern updated)
- **Affected tech-notes:** —
- **Changed in spec:** Outcome, Preconditions, Primary Flow (step 3), Edge Cases (worktree path collision), Coordinations (Docker sandbox), Out of Scope

### F-09: Worktree path is not written to a structured log field — observability gap

- **Agent:** devops-engineer
- **Finding:** The spec surfaced the worktree path only in the TUI (status header during run, final summary at end). For users away from the terminal during a long run, neither location is recoverable after the TUI is dismissed.
- **Resolution:** Added structured log records: the run's log file header and the iteration log's first record both capture the worktree path, primary path, and branch name. Closing record on graceful exit captures the same.
- **Resolved by:** evidence
- **Affected decisions:** D3, D11 (new)
- **Affected tech-notes:** —
- **Changed in spec:** Outcome, Primary Flow (step 6), Alternate Flows (graceful quit)

### F-11: Pre-existing `review_verdict` path inconsistency could silently skip the "Fix review items" step in worktree mode

- **Agent:** devops-engineer (referencing investigation Remaining Risk #3)
- **Finding:** The default workflow's finalize phase writes its code-review output to a path that the `review_verdict` script reads from CWD. If the two paths disagree, `review_verdict` silently skips. This is pre-existing but more visible under `useWorktrees` because the working directory is no longer the user's familiar primary.
- **Resolution:** Added OI-2 capturing the open question. Does not block implementation but flags it as a confirmation step.
- **Resolved by:** project-manager synthesis (deferred to implementation)
- **Affected decisions:** —
- **Affected tech-notes:** —
- **Changed in spec:** Open Items (OI-2)

### F-13: Submodule warning at startup is YAGNI — premature operational machinery

- **Agent:** devops-engineer
- **Finding:** The original D6 committed to a startup warning when `.gitmodules` was present. No user has reported a submodule-using project as a target, so the warning fires for every submodule-using user as observability for a failure mode that has never occurred.
- **Resolution:** Removed the startup warning from D6. The submodule limitation is documented in the user-facing how-to guide instead. Added the startup warning itself to Deferred with a reopening trigger.
- **Resolved by:** evidence (YAGNI rule applied)
- **Affected decisions:** D6
- **Affected tech-notes:** —
- **Changed in spec:** Edge Cases (repository contains submodules), Deferred (YAGNI)

### JD-001: `<run-stamp>` is used throughout the spec but never defined

- **Agent:** junior-developer
- **Finding:** The spec used `<run-stamp>` 10+ times without ever defining it for a reader who is new to git worktrees and the codebase.
- **Resolution:** Added a paragraph to the Background section defining `<run-stamp>` with a concrete example and a cross-reference to `docs/code-packages/logger.md`.
- **Resolved by:** evidence
- **Affected decisions:** D11
- **Affected tech-notes:** —
- **Changed in spec:** Background

### JD-002: TUI status-header indicator assumed infrastructure that does not exist

- **Agent:** junior-developer
- **Finding:** The spec asserted "the status header indicates the run is operating in a worktree" without naming a TUI surface that could carry that text.
- **Resolution:** Decision D11 (new) replaced the unspecified status-header indicator with a concrete behavior: the existing iteration line is prefixed with the worktree's basename. No new TUI infrastructure is required.
- **Resolved by:** evidence
- **Affected decisions:** D11 (new)
- **Affected tech-notes:** —
- **Changed in spec:** Primary Flow (step 7), User Interactions (Affordances)

### JD-003: TUI final-summary block assumed infrastructure that does not exist

- **Agent:** junior-developer
- **Finding:** The spec asserted that the final summary "prints the worktree path and the branch name" but the existing `CompletionSummary` mechanism has a fixed signature with no such parameters.
- **Resolution:** Decision D11 (new) replaced the implicit need for new infrastructure with a concrete behavior: the final summary line is extended to include worktree path, branch, and worktree count. Implementation can extend the existing summary mechanism rather than inventing a new one.
- **Resolved by:** evidence
- **Affected decisions:** D11 (new)
- **Affected tech-notes:** —
- **Changed in spec:** Primary Flow (step 10), User Interactions (Affordances)

### JD-004: OI-1 was simultaneously "in scope" and "blocking implementation"

- **Agent:** junior-developer
- **Finding:** The original OI-1 contradicted itself: the push fix was "in scope" but the open item said it "blocks implementation." This left the implementation PR's scope undefined.
- **Resolution:** Reframed OI-1 as a behavioral question (does the in-place workflow path also gain push-failure surfacing?) rather than as a sequencing/scope question. Marked OI-1 as non-blocking — the push behavior change is in scope for the worktree feature regardless of the in-place answer.
- **Resolved by:** evidence
- **Affected decisions:** D7
- **Affected tech-notes:** —
- **Changed in spec:** Open Items (OI-1)

### JD-005: OI-1 leaked implementation mechanics (`-u origin HEAD`, `trap 'exit 0' EXIT`)

- **Agent:** junior-developer (mechanics leaking into spec)
- **Finding:** The original OI-1 named a shell flag and the script's trap mechanism, prescribing implementation in what should be a behavioral document.
- **Resolution:** Removed mechanism names from OI-1. The spec now describes the behavioral commitment (establish a tracking ref, surface non-zero exits) and leaves the mechanism to the implementation plan; the load-bearing mechanic is captured in T3.
- **Resolved by:** evidence
- **Affected decisions:** D7
- **Affected tech-notes:** T3
- **Changed in spec:** Open Items (OI-1)

### JD-009: "The pr9k process itself does not change its own working directory" was an implementation note in a behavioral flow

- **Agent:** junior-developer (mechanics leaking into spec)
- **Finding:** Primary Flow step 4 included an implementation constraint that belongs in the technical notes, not the spec body.
- **Resolution:** Removed the sentence from Primary Flow step 4. The constraint is preserved in T1, where it belongs.
- **Resolved by:** evidence
- **Affected decisions:** —
- **Affected tech-notes:** T1
- **Changed in spec:** Primary Flow (step 4)

### JD-010: Primary Flow step 5 referenced `.pr9k/` directory creation as an explicit step — leaks logger initialization sequencing

- **Agent:** junior-developer (mechanics leaking into spec)
- **Finding:** "Creates the worktree's `.pr9k/` runtime directory" describes implementation sequencing rather than observable behavior.
- **Resolution:** Rewrote step 5 to describe the behavior — log file and iteration log are opened against the worktree path, preflight runs against the worktree path — without naming a `.pr9k/` mkdir step.
- **Resolved by:** evidence
- **Affected decisions:** —
- **Affected tech-notes:** —
- **Changed in spec:** Primary Flow (step 5)

### JD-013: D5's stale-detection algorithm cited `git worktree list --porcelain` directly — mechanics in a decision rationale

- **Agent:** junior-developer (mechanics leaking into spec)
- **Finding:** The decision body of D5 prescribed the discovery command at flag level, which is implementation detail.
- **Resolution:** Rewrote D5 to describe the behavior (discover existing worktrees, filter by path-name pattern). The exact command is left to the implementation plan; the load-bearing piece (filter is path-only, not path+branch) is captured behaviorally.
- **Resolved by:** evidence
- **Affected decisions:** D5
- **Affected tech-notes:** —
- **Changed in spec:** Alternate Flows (stale worktree detection)

## Minor edits

- F-12: T3 OI-1 framing tilted toward mechanics rather than behavior — devops-engineer — Open Items (OI-1)
- JD-006: Background section did not bridge "branches can't be shared" to "therefore pr9k creates a fresh branch" — junior-developer — Background
- JD-007: D12 was a trivial-decisions footnote and not surfaced in the spec body — junior-developer — Trigger (now describes `--project-dir` interaction inline)
- JD-008: "A future cleanup helper" speculative naming in Out of Scope — junior-developer — Out of Scope (subcommand name removed)
- JD-011: D11 cited a Go source file with a line number — junior-developer — D12 (Evidence updated to reference the package doc)
- JD-012: Coordinations table row for "User's primary checkout" was a constraint, not a coordination — junior-developer — Outcome (moved to a new "Invariants" subsection)
- F-10: Mid-run worktree deletion failure mode "not designed to recover" was vague — devops-engineer — Edge Cases (clarified that failure routes through the standard error path or terminates the TUI without recovery)

## User-driven redirects (post-review)

These items were not raised by review specialists; they were redirects from the user after the spec was first presented. Recorded here for traceability with the same F# convention.

### F-USER-01: Restructure config from single boolean to a `worktrees` block with `enabled` + `autoCleanup`

- **Agent:** user
- **Finding:** The original `useWorktrees: true` shape didn't accommodate the cleanup ergonomics the user actually wanted. The simpler-version reasoning that rejected a struct in the original D8 was overruled by the user's stated need to make cleanup easy.
- **Resolution:** Reshaped D8 to a top-level `worktrees` block containing `enabled` and `autoCleanup`. Added D13 covering `autoCleanup` semantics (graceful-only, crash-safe, validator rejects autoCleanup-without-enabled).
- **Resolved by:** user input
- **Affected decisions:** D8 (reshape), D13 (new), D4 (now references D13 for the autoCleanup case)
- **Affected tech-notes:** —
- **Changed in spec:** Header, Outcome, Trigger, Primary Flow (steps 1, 2, 3, 11, 12), Alternate Flows (graceful quit), Edge Cases (autoCleanup-related rows), User Interactions, Out of Scope, Deferred (consolidated `logDir` entry)

### F-USER-02: Promote `pr9k worktree prune` subcommand from Deferred to in-scope

- **Agent:** user
- **Finding:** Deferring the prune subcommand left cleanup as a manual chore (`git worktree list` → identify pr9k entries → `git worktree remove` per path → `git branch -D` per branch). The user explicitly wants cleanup to be a single command.
- **Resolution:** Added D14 covering the subcommand's behavior: path-prefix-filtered discovery, force-remove worktrees, delete `pr9k/*` branches, `--dry-run` flag, per-worktree error tolerance with non-zero exit on any failure. Added a new alternate flow ("The user runs `pr9k worktree prune`") and Edge Cases rows. Removed the corresponding Deferred entry.
- **Resolved by:** user input
- **Affected decisions:** D14 (new); removed from Deferred
- **Affected tech-notes:** —
- **Changed in spec:** Outcome, Trigger, Alternate Flows (new "user runs `pr9k worktree prune`" flow), Edge Cases (prune-related rows), User Interactions, Coordinations (new row), Deferred (removed prune entry), Out of Scope (removed bulk-cleanup entry)

### F-USER-03: Define `<run-stamp>` source and document cross-run-restart implications

- **Agent:** user
- **Finding:** The user asked how `<run-stamp>` is created and what happens when pr9k is killed mid-run and restarted on the same set of tickets. The spec answered the first only obliquely (via a `docs/code-packages/logger.md` cross-reference) and did not address the second at all.
- **Resolution:** Expanded the Background section to define `<run-stamp>` precisely (millisecond-precision wall clock from `time.Now()`, format `ralph-YYYY-MM-DD-HHMMSS.mmm`, generated once per pr9k invocation). Added a paragraph explicitly stating that runs do not coordinate. Added a dedicated alternate flow ("A prior run was killed mid-iteration and the user restarts pr9k") covering: stale-detection behavior, fresh worktree on fresh branch, the from-scratch implementation of the same issue, divergent-branch reconciliation steps, and the silent-skip risk that D7's push-failure error-recovery routing prevents. Added D15 ("No cross-run resume") capturing the boundary explicitly. Added the matching Out of Scope entry.
- **Resolved by:** user input + evidence (logger source, workflow.md docs)
- **Affected decisions:** D15 (new)
- **Affected tech-notes:** —
- **Changed in spec:** Background, Alternate Flows (new "A prior run was killed mid-iteration..." flow), Edge Cases (cross-run row), Out of Scope (cross-run resume entry)
