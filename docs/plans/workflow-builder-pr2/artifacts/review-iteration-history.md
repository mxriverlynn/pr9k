# Review Iteration History: Workflow Builder PR-2 Implementation Plan

Companion to `../feature-implementation-plan.md` and [review-findings.md](review-findings.md).

## R1 — Team review (2026-04-24)

- **Mode:** team
- **Spec-aware mode:** not engaged (the file is `feature-implementation-plan.md`, not a feature specification).
- **Specialists engaged:** `junior-developer`, `evidence-based-investigator`, `adversarial-validator`, `concurrency-analyst`, `adversarial-security-analyst`, `test-engineer`.
- **Findings raised:** F-PR2-1 through F-PR2-58 (58 total) — see [review-findings.md](review-findings.md).
- **Findings resolved by edit applied to plan:** 49.
- **Findings resolved by verifies clean (no change needed):** 5 (F-PR2-44, F-PR2-45, F-PR2-46, F-PR2-47, F-PR2-48).
- **Findings deferred / non-blocking:** 1 (F-PR2-38 — D-PR2-3 versioning rationale wording).
- **Findings resolved by repo artifact commit:** 1 (F-PR2-41 — round-1 specialist outputs and PM facilitation summaries committed under `artifacts/`).
- **Findings cross-referenced / consolidated:** 2 (F-PR2-39 → F-PR2-23; F-PR2-47 → F-PR2-5).
- **Decisions added:** D-PR2-24 (Ctrl+E handler applies safePromptPath containment AND regular-file check) — see [implementation-decision-log.md](implementation-decision-log.md).
- **Decisions edited:** D-PR2-23 (Go-version annotation added per F-PR2-34); D-PR2-1, D-PR2-2, D-PR2-4, D-PR2-5, D-PR2-13, D-PR2-18, D-PR2-19, D-PR2-22 (clarifications and corrections — full list in review-findings.md).
- **Changed in plan:**
  - Source Specification — note added on OI-4 framing inaccuracy (PR-1 did not deliver empty-editor state); R1 specialist outputs and PM resolutions committed under `artifacts/` and linked from Source Specification (F-PR2-41, F-PR2-43).
  - Implementation Approach > Architecture and Integration Points (Wiring sequence at startup) — step 3 introduces `*sync.WaitGroup` in `workflowDeps`; step 8 expanded to bounded drain; goroutine ownership matrix added; concurrency invariants added; Update routing tier (0) made explicit with quitMsg pre-dispatch (F-PR2-3, F-PR2-4, F-PR2-5, F-PR2-11, F-PR2-18, F-PR2-21, F-PR2-55).
  - Decomposition and Sequencing — table reshaped: WU-PR2-2 absorbs IsDirty fix; WU-PR2-3 absorbs constructor refactor + fakeFS mutex/counters + deepCopyDoc replace + lint hygiene; WU-PR2-4 absorbs Ctrl+E containment + workflowDir wiring + fmtEditorOpened fix + expanded T2 matrix; WU-PR2-5 specifies bounded drain; WU-PR2-7 specifies quitMsg pre-dispatch and counter-on-prefix-mutation; WU-PR2-8 specifies full keyboard contracts and CrashTempDiscard containment; WU-PR2-9 split into 9a (outline) and 9b (detail-pane input); WU-PR2-10 adds load-time security signals and split editor-event helpers; WU-PR2-11 specifies DI-1/DI-2 assertions explicitly; WU-PR2-12 specifies the 5-step Update sequence (F-PR2-1, F-PR2-2, F-PR2-6, F-PR2-7, F-PR2-8, F-PR2-9, F-PR2-10, F-PR2-12, F-PR2-13, F-PR2-14, F-PR2-19, F-PR2-20, F-PR2-24, F-PR2-25, F-PR2-27, F-PR2-28, F-PR2-31, F-PR2-32, F-PR2-35, F-PR2-36, F-PR2-49, F-PR2-51, F-PR2-52, F-PR2-53).
  - Testing Strategy — test count budget added; test package scope clarified; test approach by mode specified (Quit-cmd type-switch); rewrite-vs-reactivate distinction added; T2 matrix expansion enumerated; DI-1/DI-2 assertions enumerated; fakeFS/fakeEditorRunner method enumeration added; pendingAction 4-test set added; D-PR2-19 swallow-bug test added; goroutine drain test added (F-PR2-23, F-PR2-24, F-PR2-25, F-PR2-26, F-PR2-27, F-PR2-28, F-PR2-29, F-PR2-30, F-PR2-31, F-PR2-33, F-PR2-39, F-PR2-51, F-PR2-52, F-PR2-54, F-PR2-57, F-PR2-58).
  - Definition of Done — 28-mode "with post-gap-close behavior" wording added; rename-guard Hidden:false + tea.NewProgram pins added; test-engineer review elevated to merge-blocker (F-PR2-33, F-PR2-39).
  - Operational Readiness > Rollback — sentence added explaining why intermediate-WU rollback is unnecessary (workflowedit unreachable when subcommand hidden) (F-PR2-37).
  - Open Items — OI-PR2-3 added documenting the spec D33 deviation (F-PR2-17).
  - Review History section added.
- **Stability assessment:** Plan is structurally sound after R1 edits. The 49 plan edits applied address all blocker-class findings (sequencing breaks, WaitGroup architecture, quitMsg routing, Ctrl+E containment, WU-PR2-9 split). The remaining 5 verifies-clean findings and 1 deferred finding require no further iteration.
- **Next-step recommendation:** **Stop iterating.** A second round would have less than 80% probability of producing a meaningful structural improvement — the structural issues that R1 surfaced are all resolved, and a R2 would primarily produce wording polish. The plan is ready for implementation.

<!-- End of iteration history. New rounds append below as needed. -->
