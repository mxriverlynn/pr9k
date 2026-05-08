# Implementation Iteration History — useWorktrees

This file records the round-by-round record of the implementation-planning loop for the `useWorktrees` feature: which specialists were engaged, what they raised, and how each finding was resolved.

- **Source spec:** [../feature-specification.md](../feature-specification.md)
- **Decision log:** [implementation-decision-log.md](implementation-decision-log.md)
- **Discovery notes:** [.discovery-notes.md](.discovery-notes.md)

## Round R1 (2026-05-08) — parallel specialist review

### Specialists engaged (4)

- `behavioral-analyst` — runtime data flow, state-file lifecycle, RunResult.ExitReason plumbing, invocation_stamp injection layer, cleanup ordering crash-resilience, iteration-log scope expansion, resume-validation set ordering.
- `concurrency-analyst` — TOCTOU race on state-file create, PID + binary-path identity check ordering, cleanup-vs-statusline-runner shutdown, signal-handler timing, AppendIterationRecord POSIX atomicity, prune-vs-active-run race.
- `devops-engineer` — `git_push` script change, `pr9k worktree prune` subcommand placement, observability YAGNI boundary, iteration-log growth bound, autoCleanup ergonomics, branch-protection prerequisites, schema-version state file, documentation pack, soft-lock UX, version bump category.
- `junior-developer` — generalist reframing and twelve plain-language Open Questions covering logger stamp surface, three-file coordinated change tracking, state-file MkdirAll placement, binary-path lookup mechanism, worktree-stamp threading, autoCleanup execution location, prune subcommand `--project-dir` resolution, doc pack granularity, lessons-learned truncation reliability, post_issue_summary contamination acceptance, prune git shell-out pattern, and startup-sequence split.

### Claim ledger (R1)

| ID | Specialist(s) | Claim | State | Spec-maturity | Resolution route |
|----|---|---|---|---|---|
| L1 | behavioral, devops | `workflow/scripts/git_push` traps all exits to zero; D7 + D18 are unimplemented prerequisites that block the entire auto-resume guarantee | Evidenced (git_push:1-8, config.json:28-29) | plan-level | Settled by evidence — implementation must include the 3-line bash rewrite + step reorder + review_verdict path fix in the same commit set |
| L2 | behavioral, junior-dev, devops | `RunResult.ExitReason` is absent today; `main.go:298` discards the result; `breakLoopIfEmpty` falls through to natural completion | Evidenced (run.go:614-617, main.go:298) | plan-level | Settled by evidence — D16 commits to the field; implementation plan adds it and main.go consumes it |
| L3 | behavioral, devops, junior-dev | Three-file coordinated schema change required (`validator.go` vFile, `workflowmodel/scaffold.go` rawConfig, `workflowio/marshal.go` outConfig) or any user adding the `worktrees` block sees a fatal validator error | Evidenced (validator.go:111-119, scaffold.go:72-80) | plan-level | Settled by evidence — must be a single atomic deliverable; round-trip test asserts block survives parse → save |
| L4 | behavioral | Resume-validation set must check process liveness (T4 condition 4) **before** any stale-rename action on conditions 1–3, or a live process's state file gets renamed and a second worktree is created | Evidenced (T4 lifecycle, spec Primary Flow step 3) | plan-level | Settled by evidence — implementation rule documented in the plan |
| L5 | behavioral | All eleven `newIterationRecord` call sites flow through one constructor; adding `invocation_stamp` at that single layer covers all records (no bypass paths exist) | Evidenced (run.go:417, 437, 656, 698; iterationlog.go:32) | plan-level | Settled by evidence — confirms T5 §4 single-layer-injection commitment |
| L6 | behavioral | `projectDir` redirect is a single-field substitution in `Runner.projectDir`; every consumer flows through `executor.ProjectDir()`; workflow-bundle resolution correctly runs against the primary | Evidenced (workflow.go:NewRunner, workflow.go:ProjectDir, args.go:resolveWorkflowDir) | plan-level | Settled by evidence — no additional plumbing required |
| L7 | behavioral | The autoCleanup ordering (branch → worktree → state file) is crash-resilient: both SIGKILL windows land on a documented orphan handler in Primary Flow step 3 | Evidenced (spec D13, Edge Cases) | plan-level | Settled by evidence — confirmatory finding, no work needed |
| L8 | concurrency | ENOENT on state-file removal must be handled as benign (the spec commits to it but no existing code path enforces it) | Evidenced (spec TOCTOU row) | plan-level | Settled by evidence — `os.IsNotExist(err) → nil` guard in the removal path |
| L9 | concurrency | TOCTOU between `syscall.Kill(pid, 0)` and the binary-path read: process can exit between the two calls; spec is silent on this interleave | Evidenced (T4 process-identity check; no spec branch covers it) | plan-level | Settled by evidence — implementation rule: any error reading the binary path falls through to "treat as dead → resume" |
| L10 | behavioral | TOCTOU stamp-equality on state-file removal: in the rare two-instance race, the losing instance may remove the winner's state file by accident | Evidenced (spec TOCTOU row) | **spec-level** (silent on whether removal compares stamps) | Settled by evidence — implementation rule: read the state file's `worktreeStamp` before `os.Remove`; if it does not match this invocation's worktreeStamp, log and skip removal. The change preserves the spec's "no silent destruction" invariant for state belonging to another invocation. (Documented in plan; not a behavioral change.) |
| L11 | concurrency | Heartbeat ticker / statusline runner do not write to the worktree path during cleanup; existing shutdown sequence is sufficient | Evidenced (main.go:219, statusline shutdown) | plan-level | Settled by evidence — confirmatory |
| L12 | concurrency | The state-file write before `signal.Notify` is fine: pre-handler SIGINT is equivalent to SIGKILL for state-file purposes; covered by the spec's hard-kill recovery row | Evidenced (main.go:281-302, spec Edge Cases) | plan-level | Settled by evidence — confirmatory |
| L13 | concurrency | `AppendIterationRecord` POSIX atomicity holds for all current and `invocation_stamp`-augmented records; cross-instance log races cannot occur (each worktree has its own `iteration.jsonl`) | Evidenced (iterationlog.go:44-46, spec D17) | plan-level | Settled by evidence — confirmatory |
| L14 | concurrency | `pr9k worktree prune` reads the state file safely under atomic-write semantics; the only race vector is the documented logical TOCTOU on prune-vs-concurrent-start, which is an accepted soft-lock limitation | Evidenced (atomicwrite.Write rename, spec D14) | plan-level | Settled by evidence — confirmatory |
| L15 | devops | `pr9k worktree prune` belongs in `src/cmd/pr9k/worktree.go` as an inline subcommand handler (~100 lines), no new package | YAGNI candidate (against new package) | plan-level | Settled by evidence — codebase pattern (`sandbox_create.go` etc.) |
| L16 | devops | Three observability temptations to resist: per-record `worktree_path` field, "stale-worktree count" metric event, statusline `worktreeStamp`/`primaryDir` field | YAGNI candidate | plan-level | Settled by evidence — implement only the four spec-committed surfaces |
| L17 | devops | Iteration-log size cap / rotation is YAGNI; lessons-learned truncation + worktree removal is the bound | YAGNI candidate | plan-level | Settled by evidence — defer with reopening trigger "user reports the file growing past a noticeable size" |
| L18 | devops | Cleanup-recovery-on-next-startup is YAGNI; orphan handlers in Primary Flow step 3 + D5 stale detection cover all crash windows | YAGNI candidate | plan-level | Settled by evidence — defer with reopening trigger "user reports a partial-cleanup recovery gap" |
| L19 | devops | Branch-prefix config option + GitHub branch-protection preflight check are both YAGNI | YAGNI candidate | plan-level | Settled by evidence — documented prerequisite in `using-worktrees.md`; defer config until reported |
| L20 | devops | CI schema-change detector / state-file fixture is YAGNI; unit tests on the rename-and-warn path are the right granularity | YAGNI candidate | plan-level | Settled by evidence — defer with reopening trigger "real downgrade scenario causes data loss" |
| L21 | devops, junior-dev | Documentation pack required: `docs/how-to/using-worktrees.md`, `docs/how-to/managing-worktrees.md`, `docs/features/worktree-mode.md` (or extend workflow-orchestration), `docs/code-packages/workflow.md` updates, CLAUDE.md index entries | Evidenced (documentation.md standard) | plan-level | Settled by evidence — minimum required set; ship in same PR |
| L22 | devops | Soft-lock UX: emit only the spec-committed "PID N" message; richer UX is YAGNI | YAGNI candidate | plan-level | Settled by evidence — minimal message |
| L23 | devops | Version bump should be MINOR (0.8.1 → 0.9.0): new config block + new subcommand + new flag + user-visible push behavior change combined justify a minor under 0.y.z | Evidenced (versioning.md) | plan-level | Settled by evidence — MINOR bump committed in plan |
| L24 | junior-dev | Logger stamp surface: export `logger.FormatStamp(t time.Time) string` so the worktree-stamp can be generated in `main()` before `startup()` runs | Evidenced (logger.go internal format string already exists) | plan-level | Settled by evidence — small additive surface |
| L25 | junior-dev | State-file lifecycle placement: state-file read + worktree decision + state-file write live in `main()`, not in `startup()`. `startup()` keeps its single-call shape; `main()` plumbs the worktree path as the new `projectDir` | Evidenced (main.go shape, narrow-reading ADR) | plan-level | Settled by evidence — codebase pattern |
| L26 | junior-dev | Worktree-stamp threading into log header: `main()` writes the header line(s) directly after `startup()` returns, before launching the workflow goroutine. No `services` struct change needed | Evidenced (main.go:96-104 shape) | plan-level | Settled by evidence |
| L27 | junior-dev | Binary-path lookup mechanism: use `os.Executable()` for the current binary; for the live-PID lookup use `/proc/<pid>/exe` on Linux (no CGo) and `ps -p <pid> -o comm=` on macOS (no CGo); compare via `EvalSymlinks`. Any error → "treat as dead → resume" (also resolves L9) | Evidenced (no CGo precedent, go-patterns.md symlink-resolution rule) | plan-level | Settled by evidence |
| L28 | junior-dev | autoCleanup execution location: `workflow.Run` returns `ExitReason` only; `main()` consumes it and drives state-file removal + autoCleanup branch/worktree removal. Honors narrow-reading ADR | Evidenced (ADR-20260410170952) | plan-level | Settled by evidence |
| L29 | junior-dev | `pr9k worktree prune` declares its own local `--project-dir` flag (no PersistentFlags on root); resolves with the same `EvalSymlinks` pattern as `cli/args.go:resolveProjectDir` | Evidenced (sandbox subcommand pattern) | plan-level | Settled by evidence |
| L30 | junior-dev | `pr9k worktree prune` uses `exec.Command` directly for `git worktree list --porcelain` parsing — not `Runner.RunStep` (which is TUI-coupled). Test the porcelain parser and the path-prefix filter as units | Evidenced (sandbox/realDockerRun pattern) | plan-level | Settled by evidence |
| L31 | behavioral, devops | OI-2 fix: `workflow/scripts/review_verdict` line 19 changes to `REVIEW_FILE=".pr9k/artifacts/code-review.md"` (one-line change) | Evidenced (review_verdict:19) | plan-level | Settled by evidence — same commit as L1 |
| L32 | junior-dev | `lessons-learned` truncation reliability: pre-existing accepted limitation; document the kill-before-finalization growth path in `using-worktrees.md` so users know the bound is conditional | Evidenced (spec accepts this) | plan-level | Settled by evidence — documentation-only |
| L33 | junior-dev | `post_issue_summary` cross-issue contamination on resumed runs: pre-existing user-accepted limitation per spec D17; the per-iteration `issue_id` filter is a candidate follow-up that is **not** in this plan | Evidenced (spec D17) | plan-level | Settled by evidence — defer to post-feature follow-up; no work in this plan |

### Open Questions raised (R1)

All Open Questions raised in R1 were resolved by codebase evidence and project precedent during this round (no escalation needed). The resolutions are documented inline in L1–L33 above. None remain open.

### Spec-maturity tags

- T#-contradictions: 0 (no specialist proposed a mechanic conflicting with T1–T5).
- spec-level findings: **1** (L10 — TOCTOU stamp-equality on state-file removal). Resolved by an implementation rule in the plan that preserves spec invariants without changing committed behavior.
- plan-level findings: 32.

### Spec-maturity gate

Gate did **not** trip:
- T#-contradiction trigger requires ≥ 2 contradictions raised by ≥ 2 distinct specialists; 0 raised.
- Spec-level trigger requires ≥ 5 spec-level findings raised by ≥ 3 distinct specialists; 1 raised by 1 specialist.

No PM facilitation pass invoked.

### Deterministic next-step recommendation (R1)

**Go to synthesis.** All Open Questions resolved by codebase evidence and project precedent in a single pass. No specialist named another specialist as a needed handoff for a finding that is not already settled. The remaining work is PM synthesis of the plan, decision log, and YAGNI ledger.

(`junior-developer` flagged `test-engineer` and `software-architect` as "could weigh in" for specific questions — Q1 logger stamp surface, Q4 binary-path lookup, Q6 autoCleanup execution location, Q7 cobra flag, Q11 prune parsing — but each of those questions resolved cleanly via codebase precedent, so no follow-up specialist round adds evidence the plan does not already carry.)

### Decisions produced

R1 produced all 17 decisions in [implementation-decision-log.md](implementation-decision-log.md):

- **Full decisions:** D-1 (state-file lifecycle placement), D-2 (RunResult.ExitReason enum), D-3 (autoCleanup execution location), D-4 (worktree-stamp generator surface), D-5 (binary-path identity check mechanism), D-6 (ENOENT on state-file removal benign), D-7 (state-file removal compares worktreeStamp), D-8 (resume-validation set evaluation order), D-9 (invocation_stamp injection layer), D-10 (`pr9k worktree prune` placement and shape), D-11 (`git_push` rewrite shape), D-12 (three-file coordinated schema change), D-13 (documentation pack), D-14 (version bump category — MINOR), D-15 (schema-version mismatch — rename-and-warn).
- **Trivial decisions:** D-16 (concurrent-run "PID N" message), D-17 (iteration-log size cap — none).

Ledger-to-decision mapping: L1, L31 → D-11 · L2 → D-2 · L3 → D-12 · L4 → D-8 · L5 → D-9 · L6, L7, L11–L14 → confirmatory, no decisions · L8 → D-6 · L9, L27 → D-5 · L10 → D-7 · L15, L29, L30 → D-10 · L16–L20, L22 → Deferred (YAGNI) section of plan · L21 → D-13 · L22 → D-16 · L23 → D-14 · L24 → D-4 · L25, L26, L28 → D-1 / D-3 · L32, L33 → documentation / accepted limitations.

### Changed in plan

R1 wrote (and is the only contributor to) every section of [../feature-implementation-plan.md](../feature-implementation-plan.md):

- "Source Specification" — inherited spec D1–D19, T1–T5, OI-1, OI-2.
- "Outcome" — restated in implementation terms with inline links to D-1, D-2, D-4, D-7, D-9, D-10, D-11, D-12, D-13, D-14.
- "Context" — driving constraint, stakeholders, future-state concerns (iteration-log growth bound, PID-reuse soft-lock, push behavior change), out-of-scope (Windows, configurable location, per-step bookmarks, post_issue_summary filter).
- "Team Composition and Participation" — five-row table; explicit "stood down" for `adversarial-security-analyst`.
- "Implementation Approach" — Architecture and Integration Points (touch points with file:line citations), Data Model and Persistence ([D-9](implementation-decision-log.md#d-9-invocation_stamp-injection-layer), [D-12](implementation-decision-log.md#d-12-three-file-coordinated-worktrees-block-schema-change), [D-15](implementation-decision-log.md#d-15-schema-version-mismatch-handling--rename-and-warn-no-migration-framework)), Runtime Behavior (D-1, D-5, D-6, D-7, D-8), External Interfaces (D-10, D-11, D-16).
- "Decomposition and Sequencing" — 13 work units, each with verification and inline D# links.
- "RAID Log" — R1–R4 risks, A1–A3 assumptions, Dep1.
- "Testing Strategy" — observable behaviors, test doubles posture, edge cases sourced from spec Edge Cases table.
- "Security Posture" — explicit stand-down for `adversarial-security-analyst`.
- "Operational Readiness" — four spec-committed observability surfaces; `worktrees.enabled` is the rollout flag; rollback by removing the block.
- "Definition of Done" — 13 testable items, each linked to its decision.
- "Specialist Handoffs for Implementation" — `test-engineer`, `evidence-based-investigator`, `adversarial-validator` dispatch points.
- "Deferred (YAGNI)" — 10 items from L15–L20, L22, plus the larger versions rejected under D-5, D-10, D-11 simpler-version replacements.
- "Open Items" — 0.
- "Summary" — 17 decisions / 30+ rejected alternatives / 0 open items / Ship as planned.
