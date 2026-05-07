# Review Iteration History: useWorktrees feature specification (iterative-plan-review)

This file records each round of the iterative-plan-review run on [`feature-specification.md`](../feature-specification.md). Round-level decisions live here; per-finding details live in [`review-findings.md`](review-findings.md). Plan-a-feature stage iterations live in [`team-findings.md`](team-findings.md) and are not duplicated here.

## R1 — Initial team review (2026-05-07)

- **Mode:** team
- **Spec-aware mode:** engaged (filename is `feature-specification.md`; structural headings match)
- **Size:** medium (justification: 320-line spec, single feature surface, but already heavily reviewed during plan-a-feature; round cap = 2)
- **Specialists engaged (parallel, in a single message):**
  - `junior-developer` — generalist stress-test; full-spec read.
  - `adversarial-validator` — attack assumptions and resume safety; full-spec read.
  - `evidence-based-investigator` — verify load-bearing codebase claims; targeted code read across `src/cmd/pr9k/main.go`, `src/internal/{atomicwrite,preflight,workflow,workflowio,logger,statusline,sandbox,vars}/`, `workflow/{config.json,scripts/*,prompts/lessons-learned.md}`.
  - `edge-case-explorer` — focused mode (top-12 edge cases); state-file/`--fresh`/`autoCleanup`/`prune` interaction matrix.
- **Specialists explicitly excluded (spec-aware mode roster rule):** `structural-analyst`, `behavioral-analyst`, `concurrency-analyst`, `software-architect`, `system-architect`, `data-engineer` — all six are mechanic-level analysts that belong in `plan-implementation`, not in a behavioral-spec review. The user did not name any of them, so the exclusion held.
- **Findings raised:** F1–F16 (major); m1–m6 (minor) — see [`review-findings.md`](review-findings.md). Counts: 16 major, 6 minor, 0 YAGNI candidates that were raised but kept-with-evidence (F15 was raised and resolved by replacing the partial-parse path with corruption-handler treatment).
- **Major-finding categories:** internal contradiction (F1, F4, F14), unhandled edge case (F5, F7, F8, F9, F10, F11, F13), unstated assumption (F6), invariant gap (F12), YAGNI candidate (F15), mechanics leaking into spec (F3, F16), load-bearing gap (F2).
- **Cross-finding overlaps:** F2 / F4 / V4 / EC-2 (`--fresh` worktree-removal conditional); F3 / V1 / V5 / I4 (D17 consumer access); F5 / EC-6 (concurrent-run TOCTOU race); F1 / EBI-I1 (branch naming inconsistency). All collapsed into single F# entries with multi-agent attribution.
- **Conflict between agents:** none. Where two agents surfaced overlapping findings (above), the agreement strengthened the case rather than producing competing recommendations.
- **Changed in plan:** Background; Outcome (push prerequisite bullet, observability bullet); Primary Flow (steps 3, 6, 12); Alternate Flows (`pr9k --fresh`, `pr9k worktree prune`, `worktrees.enabled: true` but creation fails); Edge Cases (added 8 new rows: state-file-with-enabled-false, schema-version updated to corruption treatment, primaryPath mismatch, branch-deleted-mid-resume, TOCTOU race, `--fresh` + concurrent-run, prune-from-inside-worktree, prune + uncommitted files, structural-config-drift; updated 1 row: per-invocation iteration log accumulation; updated 1 row: autoCleanup + LoopBroken zero-iteration); User Interactions (Feedback, Error states); Coordinations (GitHub via `gh`, Git remote prerequisite, active-run state file, new iteration log alias row); Out of Scope (structural config edits between invocations); Deferred (schema-version, concurrent-run lock); Open Items (OI-1 reframe); Summary.
- **Changed in decision-log:** D2 (branch naming hyphen + rationale), D5 (rationale terminology fix), D7 (rationale branch name fix), D10 (`--fresh` no longer overrides), D11 (TUI summary terminology), D14 (resolution-from-inside-worktree, uncommitted-files warning, mechanics removal), D16 (rationale wording), D17 (alias rationale, lessons-learned prompt update), D19 (unconditional removal, concurrent-guard refusal).
- **Changed in tech-notes:** T2 (branch naming, branch-prefix load-bearing claim corrected); T4 (lifecycle ordering wording, resume validation set extended to include `primaryPath` and branch existence, schema-version simplified to corruption treatment, process-identity check spec-mode rewording); **T5 (new — stable iteration-log alias)**.
- **Stability assessment:** the spec gained 8 new Edge Cases rows, several Primary Flow validation steps, and a new tech-note. The changes are local to the resume / state-file lifecycle and the cleanup flows — no decision was reversed. The mechanics-leaking-into-spec ratio is improved (Primary Flow step 12 and the prune Alternate Flow no longer cite shell flags inside behavioural sentences). Internal cross-references checked: every new Edge Case row links to the appropriate D# / T# / Alternate Flow.
- **Deterministic stop rule:** R1 produced 16 major findings — well above the ≤2-major threshold. **Round 2 must run.**
- **Next-step recommendation:** run round 2 as a focused **delta verification** rather than a full restart. Cap the team at `adversarial-validator` (attack the round-1 fixes) + `edge-case-explorer` (sanity-check the 8 new Edge Cases rows for newly introduced gaps). Skip `junior-developer` and `evidence-based-investigator` because (a) the round-1 jr-dev findings were either resolved in-spec or routed to T5 / OI-1 / Deferred; (b) the round-1 evidence investigation was thorough and the changes were behavioural rather than code-citation-introducing.

## R2 — Delta verification (2026-05-07)

- **Mode:** team (focused / delta-verification scope)
- **Spec-aware mode:** engaged
- **Specialists engaged:**
  - `adversarial-validator` — attacked each of the round-1 fixes (F1–F16) for completeness, internal consistency, and unhandled edge cases.
  - `edge-case-explorer` — sanity-checked the 8 new R1 Edge Cases rows and the rewritten flows for newly introduced gaps.
- **Specialists explicitly skipped:**
  - `junior-developer` — R1 jr-dev findings were resolved in-spec or routed to T5/OI-1/Deferred; no new generalist surface to revisit.
  - `evidence-based-investigator` — R1 was thorough and the R1 changes were behavioural, not new code-citation-introducing.
- **Findings raised:** F17–F29 (13 major) — see [`review-findings.md`](review-findings.md). No new minor edits beyond the round-1 set.
- **Major-finding categories:** unstated assumption (F17, F20, F21, F22, F23, F24, F25, F28), internal contradiction (F18), unhandled edge case (F19, F26, F27, F29).
- **Severity highlights:** F19 (prune walk-up false-positive could remove the user's primary's siblings), F20 (macOS symlink ambiguity would cause spurious stale-detection on every re-invocation), F27 (prune from a stale worktree the user is `cd`'d into would delete their CWD), F29 (mid-cleanup crash without specified ordering could leak orphan branch + worktree).
- **Cross-finding overlaps:** F20 / EC item 7 (path canonicalisation); F21 / EC item 6 (untracked-files scope); F24 / EC item 2 (warning persistence). All collapsed into single F# entries with multi-agent attribution.
- **Conflict between agents:** none.
- **Changed in plan:** Primary Flow (step 12 cleanup ordering); Alternate Flows (`pr9k --fresh` step 1, `pr9k worktree prune` Primary resolution + step 2 + step 3); Edge Cases (TOCTOU row updated; schema-version row updated; branch-deleted-mid-resume row clarified; state-file-with-enabled-false row clarified; structural-config-drift row narrowed; prune + uncommitted-files row updated; prune-from-inside-worktree row rewritten; new legitimately-named-directory row added); User Interactions (Error states `kill -9` recovery, prune feedback wording); Out of Scope (structural config edits narrowed).
- **Changed in decision-log:** D14 (uncommitted-files scope expanded to include untracked).
- **Changed in tech-notes:** T4 (Path-comparison semantics subsection added; schema-version handling switched to `.incompatible-` suffix); T5 (Failure handling subsection added).
- **Stability assessment:** the round-2 fixes are concentrated on tightening spec language around the round-1 additions. No decision was reversed. The mechanics-leaking-into-spec ratio remains clean. Internal cross-references checked: every new/updated Edge Cases row links to the relevant D# / T#. No new T# was needed in round 2.
- **Deterministic stop rule:** R2 produced 13 major findings, but the medium size cap is 2 rounds. **Round 2 is the last round.** Remaining concerns surface as Open Items rather than being absorbed in another round.
- **Remaining open items going into final synthesis:** none beyond the existing OI-1 and OI-2 — every R1 and R2 major finding was resolved in-spec.
