# Review Iteration History: Workflow Builder

<!--
One R# entry per review round. Cross-references the F# entries in
`review-findings.md` and the plan sections changed.
-->

## R1: First review round, team mode, spec-aware

- **Mode:** team
- **Spec-aware mode:** engaged (the plan file is `feature-specification.md` and obeys the behavioral-spec rules from `han:plan-a-feature`)
- **Specialists engaged:**
  - `user-experience-designer` — per user's explicit emphasis on usability completeness and accuracy
  - `evidence-based-investigator` — verified codebase claims in the spec and decision log against the actual source tree
  - `adversarial-validator` — attacked the plan's mechanical commitments, claims about POSIX semantics, and assumptions about user behavior
  - `junior-developer` — generalist stress-test; surfaced hidden assumptions and cross-standard conflicts
  - `edge-case-explorer` — second-pass usability edge-case discovery, emphasis on fields, dialogs, reorder, save flow
- **Findings raised:** 35 new findings, F48-F82 — see [review-findings.md](review-findings.md). All resolved by evidence. Zero findings required new user judgment beyond what the first review round (F1-F47) had already settled via Q-A through Q-I.
- **Changed in plan:** Primary Flow steps 3, 5, 6, 7, 8, 9, 10. New Dialog Conventions section. New Alternate Flow: Target switching within a session. Alternate Flows updated: Scaffold-from-empty, Copy-default-to-local. Edge Cases table extended by 9 rows. User Interactions rewritten to match new commitments. Documentation Obligations extended. Versioning rewritten. Open Items populated. Summary updated. New Review History section appended at the bottom.
- **Changed in decision-log:** 19 new decisions (D45-D63). 6 existing decisions updated (D13, D15, D17, D24, D26, D33, D34, D37, D41, D42).
- **Changed in tech-notes:** T1 symlink-handling rewritten (F48); TOCTOU guard made explicit (F49). T2 mechanism corrected to `tea.ExecProcess` (F56). T3 closing-sentence self-contradiction resolved (F50). Line citation in T3 corrected (E4).

### Stability assessment and next-step recommendation

After round 1, the spec has absorbed every substantive round-2 finding. The remaining "open" work falls into two buckets:

1. **Genuine open items** — one, OI-1 (validator `safePromptPath` hardening), which was raised in the first review but not closed and which is non-blocking given the builder's symlink banner.
2. **Pure implementation detail** — the final validator API shape for in-memory validation (T3 picks one of two approaches), the widget choice for single-line input, final pagination caps on long workflows, and similar. These belong in the implementation plan produced by `han:plan-implementation`, not in the spec.

No round 2 of this iterative review is warranted. The skill's 80% rule — only continue if the next round is at least 80% likely to produce a meaningful structural change — is not met: another round would likely find only cosmetic refinements, because the biggest structural corrections (symlink rename semantics, terminal handoff API, dialog conventions, interaction mechanics coverage) have all been made.

**Next step recommended:** proceed to `han:plan-implementation` to turn this behavioral specification into a concrete implementation plan.
