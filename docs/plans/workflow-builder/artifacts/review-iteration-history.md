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

## R2: User-initiated menu-bar redesign

- **Mode:** team (lightweight compared to R1 — the redesign was user-directed, so fewer adversarial specialists were engaged)
- **Spec-aware mode:** engaged
- **Specialists engaged:**
  - `user-experience-designer` — user explicitly requested UX help for the menu-bar design; delivered a 15-question structured recommendation covering initial state, menu rendering, activation, shortcuts, dropdowns, File > New walkthrough, File > Open walkthrough, path picker design, unsaved-changes interception, session-header coexistence, help modal, footer impact, what-goes-away, what-migrates, and first-time-user experience.
  - `junior-developer` — generalist stress-test; delivered plain-terms reframing, decisions audit, spec-sections audit, conflicts-with-standards check, hidden-assumptions list, and open questions for the team.
- **Findings raised:** 9 findings (F83-F91) — see [review-findings.md](review-findings.md).
- **Changed in plan:**
  - Primary Flow steps 1-2: empty-editor launch state with menu bar; `--workflow-dir` triggers auto-open via File > Open code path.
  - Primary Flow step 3: rewritten as File > New flow (unsaved-check → choice → integrity → path → load).
  - Primary Flow step 4: rewritten as File > Open flow (unsaved-check → path → load with all load-time banners/recovery).
  - Primary Flow step 5: edit view now has four surfaces (menu bar, session header, outline + detail pane, shortcut footer); session lifecycle redefined around file load events.
  - Primary Flow step 9: Save trigger is File > Save / Ctrl+S.
  - Primary Flow step 10: Quit trigger is File > Quit / Ctrl+Q.
  - Alternate Flows: "Scaffold-from-empty" renamed "File > New — empty scaffold"; "Copy-default-to-local" renamed "File > New — copy from default"; "Target switching within a session" rewritten as "File > Open — target switching within a running builder"; "Read-only target" entry condition updated; "External-workflow session" entry condition updated; "Unsaved-changes quit" renamed and extended to "Unsaved-changes interception (Quit, File > New, File > Open)" with auto-resume semantics; "Parse-error recovery" exit points updated (return to empty-editor hint state instead of removed landing page).
  - User Interactions — Affordances: old "Landing page" bullet replaced with four bullets (menu bar, empty-editor state, File > New choice dialog, path picker).
  - Documentation Obligations: workflow-builder.md description updated to include menu bar, File menu flows, and keyboard map.
  - Summary and Review History: R2 counts and changes recorded.
- **Changed in decision log:**
  - Superseded (status marker added, history preserved, pointers to D64): D2, D8, D31, D50.
  - Updated (trigger point moved from landing-page selection to File-menu load events): D3, D4, D22, D30, D57.
  - Added: D64 (menu-bar model), D65 (rendering), D66 (activation), D67 (shortcuts), D68 (initial state), D69 (File > New flow), D70 (File > Open flow), D71 (path picker), D72 (unsaved-changes interception auto-resume).
- **Changed in tech-notes:** None. All menu-bar mechanics are discoverable from conventional TUI patterns (F10 activation, Ctrl-letter shortcuts, text-input tab-completion).

### Stability assessment and next-step recommendation

R2 closed the user's redesign request completely. The UX designer's 15-question structured recommendation, paired with the junior-developer's decisions audit, produced a coherent replacement for the landing-page model with no unresolved open questions. The one pre-existing open item (OI-1 validator `safePromptPath` hardening) is unaffected by R2 and remains non-blocking.

After R2, the spec has absorbed all substantive feedback across two review rounds. Remaining work is pure implementation detail (validator API shape for in-memory validation, single-line input widget choice, pagination caps, specific Bubble Tea wire-up for the menu bar dropdown). These belong in the implementation plan produced by `han:plan-implementation`.

**Next step recommended:** proceed to `han:plan-implementation`.

## R3: User-initiated Quit-flow simplification

- **Mode:** direct user simplification (no specialists engaged)
- **Spec-aware mode:** engaged
- **Specialists engaged:** none. After R2's menu-bar redesign, the project-manager flagged that R2 preserved D54's two-step Discard confirmation even though the user's R2 description of the Quit dialog had been simpler. The user confirmed the intended simplification and added a new requirement (saved-Quit also confirms). No adversarial or domain-specialist review was needed — the change is a narrow, user-directed simplification of the Quit flow.
- **Findings raised:** 2 findings (F92, F93) — see [review-findings.md](review-findings.md).
- **Changed in plan:**
  - Primary Flow step 10: rewritten to document two dialog shapes — three-option (Save/Cancel/Discard, single-step) for unsaved state, and two-option (Yes/No) for saved state.
  - Alternate Flows — "Unsaved-changes interception" updated: removed the two-step Discard confirmation language.
  - Alternate Flows — new entry "Quit confirmation (no unsaved changes)": documents the always-confirm-on-Quit behavior with the simpler two-option dialog.
  - Review History and Summary: R3 recorded.
- **Changed in decision log:**
  - D54 superseded: two-step Discard confirmation removed. History preserved with pointer to D7's R3 simplification.
  - D7 updated: unsaved-Quit is single-step; saved-Quit uses the new D73 confirmation.
  - D72 updated: New/Open interception no longer references the two-step Discard.
  - D73 added: Quit always confirms, with a two-option `(Yes / No)` dialog when no unsaved changes exist.
- **Changed in tech-notes:** None.

### Stability assessment and next-step recommendation

R3 is a narrow simplification that does not surface any new design questions. The spec has now absorbed three rounds of review (R1 adversarial, R2 user redesign of target selection, R3 user simplification of Quit) and remains internally consistent with one non-blocking open item (OI-1 validator `safePromptPath` hardening).

**Next step recommended:** proceed to `han:plan-implementation`.
