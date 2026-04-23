# Review Findings: Workflow Builder

<!--
Review findings raised during iterative-plan-review sessions against
`feature-specification.md`. Each F# entry records the agent that raised
it, the category, the finding itself, the resolution (or pending state),
and the cross-references to the round it was raised in, the decisions it
affected, any tech-notes it added or edited, and the spec sections it
changed.

Numbering continues from the `team-findings.md` entries produced by the
original plan-a-feature run (F1-F47). Round 1 of this review adds
F48-F82. Round 2 (user-initiated menu-bar redesign) adds F83-F91.
Round 3 (user-initiated Quit-flow simplification) adds F92-F93.
-->

## F48: T1 symlink write-through mechanism is physically impossible

- **Agent:** adversarial-validator
- **Category:** mechanics leaking into spec / falsified mechanic
- **Finding:** T1 claimed that renaming a temp file over a symlink path "replaces the inode the symlink resolves to, preserving the link relationship." Verified on macOS APFS with a live `mv` test: POSIX `rename(2)` operates on the destination directory entry, not on the symlink's target. The rename destroys the symlink itself — the link relationship is gone. This means T1's indicative approach, as written, breaks D17's "save through the symlink" commitment for every workflow where `config.json` is a symlink.
- **Evidence considered:** POSIX `rename(2)` semantics. Live test on this codebase's platform (Darwin 25.3.0). D17 behavioral commitment.
- **Resolution:** Rewrote T1's symlink-handling language. Implementation must resolve the symlink via path resolution at save time, compute the real target's directory, place the temp file there (still same-filesystem), and rename over the resolved real path — not over the symlink entry. The symlink entry is untouched; its target inode is atomically replaced. Noted that symlink resolution happens at save, not at load, to close a TOCTOU window.
- **Resolved by:** evidence
- **Raised in round:** R1
- **Affected decisions:** D17 (Evidence field updated)
- **Affected tech-notes:** T1 (symlink section rewritten; F48 added to Driven-by)
- **Changed in spec:** —

## F49: T1 TOCTOU guard not load-bearing in the original wording

- **Agent:** adversarial-validator (implied from F48) / edge-case-explorer round 1 (F38)
- **Category:** mechanics leaking into spec
- **Finding:** T1's original text did not explicitly require `O_EXCL` exclusive creation of the temp file and did not name the same-directory invariant as load-bearing for security — the symlink-race attack described in F38 depends on both being guaranteed.
- **Evidence considered:** F38 adversarial-security-analyst round 1.
- **Resolution:** T1 now explicitly requires exclusive temp-file creation with a pid/timestamp-suffixed name and same-directory placement.
- **Resolved by:** evidence
- **Raised in round:** R1
- **Affected decisions:** —
- **Affected tech-notes:** T1 (F49 added to Driven-by)
- **Changed in spec:** —

## F50: T3 approach-2 contradicts the closing invariant

- **Agent:** adversarial-validator
- **Category:** mechanics leaking into spec (internal contradiction)
- **Finding:** T3's final sentence stated "writes happen only under the T1 atomic save path." Approach 2 explicitly writes to a scratch directory during validation — a disk write outside the atomic save path. The two statements cannot both be correct; an implementer reading the closing sentence would conclude approach 2 is prohibited, even though T3 listed it as an acceptable alternative.
- **Evidence considered:** T3 text. T1 closing contract.
- **Resolution:** T3 closing sentence narrowed — the invariant is now "the real target directory is never written during validation," which is satisfied by both approaches. Approach 2 writes only to a scratch location disjoint from the real target.
- **Resolved by:** evidence
- **Raised in round:** R1
- **Affected decisions:** —
- **Affected tech-notes:** T3 (closing sentence rewritten)
- **Changed in spec:** —

## F51: D15 copy scope omits statusLine script

- **Agent:** adversarial-validator
- **Category:** behavioral gap
- **Finding:** D15 specified copying "configuration file, every prompt file referenced by any step, every script file referenced by any step" — but the `statusLine.command` field is a top-level block, not part of any step. The bundled default workflow ships with a `statusLine` block pointing at `scripts/statusline`. A user selecting "copy default to local" would have their copy produce a workflow whose config references a script that was not copied, failing validation on first load.
- **Evidence considered:** Default bundle layout at `workflow/` (statusLine present). Validator `statusLine` block at `src/internal/validator/validator.go:259-273`.
- **Resolution:** D15 decision expanded to include the statusLine script in the copy scope. Rejected-alternatives list extended with "copy without the statusLine script." Spec step 3 and Alternate Flows — Copy-default-to-local updated to match.
- **Resolved by:** evidence
- **Raised in round:** R1
- **Affected decisions:** D15
- **Affected tech-notes:** —
- **Changed in spec:** Primary Flow step 3; Alternate Flows — Copy-default-to-local

## F52: D26 cursor rationale imports Ralph-specific knowledge

- **Agent:** adversarial-validator
- **Category:** narrow-reading-ADR violation / unsupported claim
- **Finding:** D26 rationale said "Iteration is the most-edited phase (the Ralph loop operates entirely in iteration)" — a behavioral claim about user habits with no codebase evidence, and specifically a Ralph-workflow assumption that contradicts D11's commitment to no Ralph-specific knowledge in the builder.
- **Evidence considered:** Narrow-reading ADR `docs/adr/20260410170952-narrow-reading-principle.md`. D11 itself. Validator category 3 phase-size rule (iteration must have at least one step).
- **Resolution:** D26 rationale rewritten to ground in the schema invariant (iteration is the only always-non-empty phase, per the validator's category 3 rule) rather than an unfalsifiable user-habit claim. Rejected-alternatives list extended to include the prior rationale. Spec step 6 adjusted to the new three-phase-fallthrough behavior.
- **Resolved by:** evidence
- **Raised in round:** R1
- **Affected decisions:** D26
- **Affected tech-notes:** —
- **Changed in spec:** Primary Flow step 6

## F53: D33 rejects `$` under direct exec where it has no shell meaning

- **Agent:** adversarial-validator
- **Category:** fragile / overcautious
- **Finding:** D33 invokes the editor via direct exec (not `sh -c`), but still rejects values containing `$`. Under direct exec, `$` is a literal character with no shell meaning — rejecting it prevents legitimate user values like `VISUAL='$HOME/bin/myvim'` (single-quoted in the shell profile so the variable never expands before reaching the builder).
- **Evidence considered:** D33 decision text. Shell metacharacter semantics under direct-exec. Go `exec.Command` documentation.
- **Resolution:** D33 rationale updated to explain the distinction; `$` removed from the rejection list. Rejected-alternatives list extended.
- **Resolved by:** evidence
- **Raised in round:** R1
- **Affected decisions:** D33
- **Affected tech-notes:** —
- **Changed in spec:** —

## F54: Gripper glyph `⠿` requires Braille-capable terminal fonts

- **Agent:** adversarial-validator
- **Category:** fragile / preconditions violation
- **Finding:** D34 chose `⠿` (U+283F, Braille Pattern Dots-123456) as the gripper glyph. The Braille Patterns Unicode block (U+2800-U+28FF) is not guaranteed in default terminal fonts on Windows (Consolas, Cascadia Code pre-certain-versions), macOS (San Francisco, Monaco), or minimal container environments. The spec's preconditions inherit the main pr9k TUI's capability bar, which has never required Braille font support. Users on affected terminals would see `?` or `□` replacement glyphs, defeating the signifier.
- **Evidence considered:** Unicode block U+2800-U+28FF font-coverage history. Existing pr9k TUI glyphs at `src/internal/ui/header.go:245-249` (all in the BMP with broad coverage).
- **Resolution:** D34 switched to `⋮⋮` (U+22EE twice — vertical ellipsis), which is in the Mathematical Operators block and universally present in monospace fonts. Rejected-alternatives list extended. Spec step 7 and User Interactions updated.
- **Resolved by:** evidence
- **Raised in round:** R1
- **Affected decisions:** D34
- **Affected tech-notes:** —
- **Changed in spec:** Primary Flow step 7; User Interactions — Affordances

## F55: `Alt+↑/↓` is silently broken under tmux — the exact environment the spec recommends for long sessions

- **Agent:** user-experience-designer (round 2, UX-004)
- **Category:** behavioral gap
- **Finding:** D34 specified `Alt+↑`/`Alt+↓` as the sole keyboard reorder binding. `Alt` as a modifier is intercepted by tmux's default `escape-time` (converts `Alt+key` to `Esc key` two-event sequence), mosh, and some SSH clients. The spec's own edge-case table recommends tmux or `nohup` for SIGHUP mitigation — the user is guided into the environment where the shortcut fails.
- **Evidence considered:** UX-004 reasoning. Existing pr9k TUI source has no `alt+up` / `alt+down` handler to confirm — net-new pattern.
- **Resolution:** D34 added a non-modifier fallback: pressing `r` on a focused step enters reorder mode, during which `↑`/`↓` move the step, `Enter` commits, `Escape` cancels. The fallback binding is included in the shortcut footer and help modal. Documentation Obligations extended with a tmux/SSH-Alt caveat note for the how-to guide.
- **Resolved by:** evidence
- **Raised in round:** R1
- **Affected decisions:** D34
- **Affected tech-notes:** —
- **Changed in spec:** Primary Flow step 7; User Interactions — Affordances; Documentation Obligations

## F56: T2 describes the wrong Bubble Tea mechanism for terminal handoff

- **Agent:** adversarial-validator
- **Category:** mechanics leaking into spec / wrong-API
- **Finding:** T2 said "Bubble Tea exposes a terminal-release / restart pattern through its message types; the implementation plan will pick the specific message and wire it." The actual mechanism in bubbletea v1.3.10 is `tea.ExecProcess(*exec.Cmd, ExecCallback) tea.Cmd` — a `tea.Cmd` returned from `Update`, not a `tea.Msg` sent into it. An implementer searching for a message type to send would not find the right abstraction.
- **Evidence considered:** bubbletea v1.3.10 source. Zero existing usages of `tea.ExecProcess` in the pr9k codebase.
- **Resolution:** T2's "Technical detail" section rewritten to name `tea.ExecProcess` directly and describe its `ReleaseTerminal` / `RestoreTerminal` lifecycle-hook semantics.
- **Resolved by:** evidence
- **Raised in round:** R1
- **Affected decisions:** —
- **Affected tech-notes:** T2 (F56 added to Driven-by)
- **Changed in spec:** —

## F57: D37 wording risks a drive-by version bump that violates the coding standard

- **Agent:** adversarial-validator / junior-developer
- **Category:** cross-standard conflict
- **Finding:** D37 said "the feature PR must also include the version bump commit." `docs/coding-standards/versioning.md` says: "A version bump is its own commit, not a drive-by edit in a feature PR." The single-file `version.go` exception does not apply because this feature introduces substantial Go code outside `version.go`. A reader could interpret "include" as "bundle into a feature commit," which is prohibited.
- **Evidence considered:** `docs/coding-standards/versioning.md` (0.y.z rules AND the commit-separation rule).
- **Resolution:** D37 decision and rationale updated to state explicitly that the bump is its own commit (either landed ahead of the feature PR or included as an independent commit within it). Spec Versioning section rewritten to match.
- **Resolved by:** evidence
- **Raised in round:** R1
- **Affected decisions:** D37
- **Affected tech-notes:** —
- **Changed in spec:** Versioning

## F58: D41 mtime+size collision detection has a silent-failure window on one-second-resolution filesystems

- **Agent:** adversarial-validator
- **Category:** fragile / honest-expectation-setting
- **Finding:** D41 used mtime and size as the collision signal. On filesystems with one-second mtime resolution (HFS+, FAT32, many network filesystems), two saves within the same second that produce identical-size files are indistinguishable from no external change — the collision dialog silently does not fire. D41 described the mechanism as "best-effort" but did not name this specific failure case for user-expectation purposes.
- **Evidence considered:** Filesystem mtime resolution documentation. APFS (nanosecond) vs. HFS+ (one-second).
- **Resolution:** D41 decision text extended to name the same-second same-size false-negative case as a known limitation. Edge Cases table adds a row documenting it. The Out-of-Scope section's "no locking" note already covers this at the behavioral level.
- **Resolved by:** evidence
- **Raised in round:** R1
- **Affected decisions:** D41
- **Affected tech-notes:** —
- **Changed in spec:** Edge Cases table

## F59: D41 omits mtime-snapshot refresh after save

- **Agent:** edge-case-explorer (round 2, NF-8)
- **Category:** behavioral gap
- **Finding:** D41 said "on load, snapshot the mtime and size" and "at save, re-stat and compare" — but did not say the builder refreshes its snapshot after a successful save. Without the refresh, every save after the first in a session would see its own prior write as a change-since-snapshot and spuriously fire the conflict dialog.
- **Evidence considered:** D41 decision text. Logical consequence of snapshot + save sequence.
- **Resolution:** D41 decision extended: "After every successful save, the builder refreshes this snapshot." Edge Cases table adds a row covering rapid successive saves.
- **Resolved by:** evidence
- **Raised in round:** R1
- **Affected decisions:** D41
- **Affected tech-notes:** —
- **Changed in spec:** Primary Flow step 9; Edge Cases table

## F60: Reorder viewport does not track the moving step

- **Agent:** edge-case-explorer (round 2, NF-6)
- **Category:** behavioral gap
- **Finding:** D29 committed the outline to "keyboard navigation auto-scrolls to keep the focused item visible." `Alt+↑/↓` (and the reorder-mode fallback) is a reorder operation, not a navigation operation. The spec was silent on whether the viewport tracks a moving step — if it does not, a user reordering a step below the viewport edge loses visual contact with the item they are moving.
- **Evidence considered:** D29 text.
- **Resolution:** D34 extended to explicitly state that the reorder viewport auto-scrolls to keep the moving step visible. Edge Cases table adds a row.
- **Resolved by:** evidence
- **Raised in round:** R1
- **Affected decisions:** D34
- **Affected tech-notes:** —
- **Changed in spec:** Primary Flow step 7; Edge Cases table

## F61: Choice-list keyboard contract is unspecified

- **Agent:** user-experience-designer (round 2, UX-008)
- **Category:** behavioral gap
- **Finding:** D12 committed to rendering constrained fields as choice lists with the `▾` glyph, but never specified the keyboard interaction model. Without spec-level commitment, implementations would diverge on what opens the dropdown, how it is navigated, and whether Escape dismisses or commits.
- **Evidence considered:** D12 text. UX-008 reasoning.
- **Resolution:** New D45 commits to the keyboard contract: `Enter`/`Space` to open, `↑`/`↓` to navigate, `Enter` to confirm, `Escape` to dismiss-restoring-prior-value, character-typing for typeahead.
- **Resolved by:** evidence (standard dropdown conventions)
- **Raised in round:** R1
- **Affected decisions:** D45 (new)
- **Affected tech-notes:** —
- **Changed in spec:** Primary Flow step 7; User Interactions — Affordances

## F62: Add-step affordance has no named signifier or keyboard binding

- **Agent:** user-experience-designer (round 2, UX-001)
- **Category:** behavioral gap / task-blocker
- **Finding:** The spec committed to an "add-step" affordance "at the phase level" but named no form (button? footer key? outline entry?) and no keyboard binding. A workflow author navigating to an empty phase has no in-UI signal for how to trigger add-step. Keyboard-only users would be stuck. The same gap applied to `env` and `containerEnv` list sections, which have no "at the phase level" guidance at all.
- **Evidence considered:** D15 (the user's core task includes adding steps). UX-001.
- **Resolution:** New D46 commits to a visible `+ Add <item-type>` row at the end of every list-typed section; `a` as a secondary single-key shortcut from section headers. Every list section (phases, env, containerEnv) gets the same affordance.
- **Resolved by:** evidence
- **Raised in round:** R1
- **Affected decisions:** D46 (new)
- **Affected tech-notes:** —
- **Changed in spec:** Primary Flow steps 5 and 7; User Interactions — Affordances

## F63: Secret-reveal toggle has no keyboard binding

- **Agent:** user-experience-designer (round 2, UX-010)
- **Category:** equitable-use violation
- **Finding:** D20 committed to "a reveal affordance the user can toggle for the specific field" but named no signifier and no keyboard binding. A mouse-only user could toggle; a keyboard-only user could not. Equitable use requires both paths.
- **Evidence considered:** D20 text. Universal Design Principle 1.
- **Resolution:** New D47 commits to `r` as the toggle key on a focused secret field, `[press r to reveal]` as the persistent label, and automatic re-mask on focus loss.
- **Resolved by:** evidence
- **Raised in round:** R1
- **Affected decisions:** D47 (new)
- **Affected tech-notes:** —
- **Changed in spec:** Primary Flow step 7; User Interactions — Feedback

## F64: Dialog conventions are inconsistent across the eight dialog surfaces

- **Agent:** user-experience-designer (round 2, UX-011)
- **Category:** consistency violation
- **Finding:** The spec defined at least eight dialogs/notices with inconsistent option vocabularies ("cancel" vs. "leave" vs. "close"), inconsistent Escape handling (specified in one dialog, unspecified in others), and no shared default-option convention. Users learning from one dialog would be surprised by another.
- **Evidence considered:** Full spec dialog survey. Nielsen H4.
- **Resolution:** New D48 establishes a single dialog convention (Escape = Cancel, safe option is keyboard default, fixed lexicon, consistent spatial order, resize re-layout). Applied to every dialog by reference in the new Dialog Conventions spec section.
- **Resolved by:** evidence
- **Raised in round:** R1
- **Affected decisions:** D48 (new)
- **Affected tech-notes:** —
- **Changed in spec:** new Dialog Conventions section; Alternate Flows (all dialogs now reference D48)

## F65: Session header can carry up to eight concurrent signals with no priority

- **Agent:** user-experience-designer (round 2, UX-003)
- **Category:** on-screen-hierarchy violation
- **Finding:** The spec listed target path + unsaved-changes indicator + read-only indicator + external-workflow banner + shared-install banner + symlink banner + unknown-field warning banner + validator findings summary as concurrent session-header elements. On an 80-column terminal with a symlinked external workflow that has unknown fields, all of them would crowd for one line with no priority.
- **Evidence considered:** Spec session-header content across all D-entries.
- **Resolution:** New D49 commits to showing at most one warning banner at a time by priority (read-only > external-workflow > symlink > shared-install > unknown-field), with a `[N more warnings]` affordance for the rest. Persistent elements (target path, unsaved-changes, findings summary) still always render.
- **Resolved by:** evidence
- **Raised in round:** R1
- **Affected decisions:** D49 (new)
- **Affected tech-notes:** —
- **Changed in spec:** Primary Flow step 5; User Interactions — Affordances

## F66: Landing-page option labels assume prior knowledge

- **Agent:** user-experience-designer (round 2, UX-006)
- **Category:** match-real-world violation
- **Finding:** Four landing-page option labels ("Edit the default target in place," "Edit the local project's workflow," etc.) use "target" and "workflow" as jargon with no inline gloss. A first-time user cannot determine which option applies to their situation.
- **Evidence considered:** Spec step 3 text. Nielsen H2.
- **Resolution:** New D50 commits to each option rendering with a one-line subtitle showing the resolved absolute path. Users recognize the path even without knowing the terminology.
- **Resolved by:** evidence
- **Raised in round:** R1
- **Affected decisions:** D50 (new)
- **Affected tech-notes:** —
- **Changed in spec:** Primary Flow step 3

## F67: Section summary content unspecified

- **Agent:** user-experience-designer (round 2, UX-007)
- **Category:** behavioral gap
- **Finding:** D28 committed to a "section summary" with "counts, top-level field values" in the detail pane when a section header is focused, but never enumerated what those are for each of the six section types. Implementations would diverge.
- **Evidence considered:** D28 text.
- **Resolution:** New D51 enumerates summary content per section type (env / containerEnv key lists with cap; statusLine type/command/refresh; phase step lists with kind annotations; empty-section state).
- **Resolved by:** evidence
- **Raised in round:** R1
- **Affected decisions:** D51 (new)
- **Affected tech-notes:** —
- **Changed in spec:** Primary Flow step 5

## F68: Detail-pane scrollability unspecified

- **Agent:** user-experience-designer (round 2, UX-012)
- **Category:** visibility violation
- **Finding:** D29 specified outline scrollability but said nothing about the detail pane. A Claude step has eleven fields; on a short terminal they overflow the pane. Without a scroll commitment, fields below the fold are invisible.
- **Evidence considered:** D29 text. Claude-step field count.
- **Resolution:** New D52 commits to independent detail-pane scrollability with a scroll-position indicator, plus mouse-wheel routing.
- **Resolved by:** evidence
- **Raised in round:** R1
- **Affected decisions:** D52 (new)
- **Affected tech-notes:** —
- **Changed in spec:** Primary Flow step 5

## F69: Post-save success state unspecified

- **Agent:** user-experience-designer (round 2, UX-005)
- **Category:** visibility violation
- **Finding:** The spec said "if there are no findings, the save proceeds silently." Silent success is the absence of feedback, not a success state. Users cannot confirm their write landed.
- **Evidence considered:** Nielsen H1.
- **Resolution:** New D53 commits to three feedback elements on successful save: unsaved-changes indicator clears, transient `Saved at HH:MM:SS` in the session header for ~3 seconds, acknowledgment dialog dismissed.
- **Resolved by:** evidence
- **Raised in round:** R1
- **Affected decisions:** D53 (new)
- **Affected tech-notes:** —
- **Changed in spec:** Primary Flow step 9; User Interactions — Feedback

## F70: Discard in unsaved-quit dialog has no confirmation guard

- **Agent:** user-experience-designer (round 2, UX-009)
- **Category:** tolerance-for-error violation
- **Finding:** The three-way unsaved-quit dialog (save/discard/cancel) had no secondary confirmation on Discard, and D7 committed to no-undo-history. A user who accidentally navigated to Discard and pressed Enter would lose their entire session's work irrevocably.
- **Evidence considered:** D7. UX-009.
- **Resolution:** New D54 commits to Cancel as keyboard default, spatial order save-cancel-discard, and a mandatory second-step confirmation on Discard (`(y/N)` with N as default).
- **Resolved by:** evidence
- **Raised in round:** R1
- **Affected decisions:** D54 (new)
- **Affected tech-notes:** —
- **Changed in spec:** Primary Flow step 10; Alternate Flows — Unsaved-changes quit

## F71: Discard default-option visual priority unspecified

- **Agent:** user-experience-designer (round 2, UX-009)
- **Category:** dark-pattern risk
- **Finding:** The external-workflow confirmation dialog's primary/default option was not specified. If an implementation defaulted to "Confirm" for GUI-habit reasons, the security warning would be undermined (user taps Enter and bypasses it).
- **Evidence considered:** UX-009.
- **Resolution:** Covered by D48 (safe option is always keyboard default across all dialogs) and D54 (specific to the unsaved-quit dialog).
- **Resolved by:** evidence
- **Raised in round:** R1
- **Affected decisions:** D48, D54
- **Affected tech-notes:** —
- **Changed in spec:** Dialog Conventions section

## F72: Focus destination after manual findings-panel dismiss unspecified

- **Agent:** user-experience-designer (round 2, UX-013)
- **Category:** focus-order violation
- **Finding:** D35 committed to manual dismiss but never specified where focus lands. Keyboard-only users in the fix-and-check cycle lose their editing position on dismiss.
- **Evidence considered:** WCAG 2.2 SC 2.4.3.
- **Resolution:** New D55 commits to focus returning to the field or outline item focused immediately before the panel was opened.
- **Resolved by:** evidence
- **Raised in round:** R1
- **Affected decisions:** D55 (new)
- **Affected tech-notes:** —
- **Changed in spec:** Primary Flow step 9

## F73: "Clear error" in Edge Cases table is not a behavioral commitment

- **Agent:** user-experience-designer (round 2, UX-002)
- **Category:** behavioral gap / Nielsen H9 violation
- **Finding:** Several edge-case rows used the phrase "clear error" as the full behavioral specification. An implementation could satisfy this with a bare OS error string (`errno: EACCES`) that gives the user no recovery guidance.
- **Evidence considered:** Spec edge-case table. Existing well-formed error dialogs in Alternate Flows (editor-spawn failure, parse-error recovery).
- **Resolution:** New D56 establishes a four-element error template (what happened / why / in-memory state commitment / available action). Applied to every edge-case-table row via the new Dialog Conventions section preamble.
- **Resolved by:** evidence
- **Raised in round:** R1
- **Affected decisions:** D56 (new)
- **Affected tech-notes:** —
- **Changed in spec:** new Dialog Conventions section; Edge Cases table preamble

## F74: "Session" used throughout the spec without a definition

- **Agent:** user-experience-designer (round 2) / junior-developer (round 2)
- **Category:** undefined term
- **Finding:** "Session" appeared in phrases "per-session warning suppression," "session header," "session log," etc., but the spec never stated what starts or ends a session. This left target-switching semantics ambiguous — does a per-session-acknowledged warning survive if the user switches targets? Does the unsaved-changes indicator?
- **Evidence considered:** Full spec "session" usage survey.
- **Resolution:** New D57 defines a session as starting on reaching edit view and ending on builder exit or return-to-landing. Return-to-landing ends the session; session-scoped state is discarded.
- **Resolved by:** evidence
- **Raised in round:** R1
- **Affected decisions:** D57 (new)
- **Affected tech-notes:** —
- **Changed in spec:** Primary Flow step 5 (session lifecycle paragraph); new Alternate Flow — Target switching

## F75: Returning to landing page silently discards unsaved edits

- **Agent:** edge-case-explorer (round 2, NF-17)
- **Category:** crash-or-data-loss / behavioral gap
- **Finding:** The spec permitted return-to-landing from several alternate flows but described no confirmation guard. An accidental return would silently destroy all session edits with no undo (D7 explicitly has no undo history).
- **Evidence considered:** D7, spec alternate flows permitting return to landing.
- **Resolution:** D57 specifies that return-to-landing with unsaved changes triggers the unsaved-changes dialog first; only Save or confirmed Discard proceeds.
- **Resolved by:** evidence
- **Raised in round:** R1
- **Affected decisions:** D57
- **Affected tech-notes:** —
- **Changed in spec:** new Alternate Flow — Target switching

## F76: Concurrent read-only + external-workflow banners unspecified

- **Agent:** edge-case-explorer (round 2, NF-18)
- **Category:** behavioral gap
- **Finding:** Both read-only and external-workflow conditions can be true simultaneously; the spec did not specify banner priority.
- **Evidence considered:** Spec banner commitments.
- **Resolution:** Subsumed by D49 (banner priority model).
- **Resolved by:** evidence
- **Raised in round:** R1
- **Affected decisions:** D49
- **Affected tech-notes:** —
- **Changed in spec:** Primary Flow step 5

## F77: Model suggestion list has no maintenance contract

- **Agent:** adversarial-validator (V11)
- **Category:** fragile / honest-expectation-setting
- **Finding:** D12 committed to a "suggestion list of known-good values" for `model`, but named no source and no update cadence. A hardcoded list in Go will be stale within months of shipping — Anthropic regularly retires and introduces model identifiers.
- **Evidence considered:** D12. Absence of any `model` list in the codebase.
- **Resolution:** New D58 commits to honest expectation-setting: the list is a hardcoded snapshot with no update contract, explicitly documented as potentially stale in the how-to guide; the field accepts any typed value regardless of match.
- **Resolved by:** evidence
- **Raised in round:** R1
- **Affected decisions:** D58 (new)
- **Affected tech-notes:** —
- **Changed in spec:** Primary Flow step 7; Documentation Obligations (how-to guide note)

## F78: T1's coding-standards entry obligation not in Documentation Obligations list

- **Agent:** junior-developer (round 2)
- **Category:** documentation gap
- **Finding:** T1 recommended codifying the save-durability pattern in coding standards, but the Documentation Obligations list cited only an ADR. An ADR serves history; a coding-standards entry serves day-to-day contributor guidance. Shipping one without the other leaves the rule discoverable from history only, not from the standards index.
- **Evidence considered:** T1 closing text. Existing `docs/coding-standards/` files.
- **Resolution:** New D59 extends Documentation Obligations with a coding-standards entry. Spec Documentation Obligations section updated.
- **Resolved by:** evidence
- **Raised in round:** R1
- **Affected decisions:** D59 (new)
- **Affected tech-notes:** —
- **Changed in spec:** Documentation Obligations

## F79: Companion-file atomicity at first save unspecified

- **Agent:** junior-developer (round 2)
- **Category:** behavioral gap / silent-corruption risk
- **Finding:** T1 specified atomic rename for the configuration file. The scaffold-from-empty flow writes newly created prompt/script files at the same save. The spec was silent on whether those companion writes were atomic, creating the same tear-on-crash risk T1 was introduced to prevent.
- **Evidence considered:** T1 text. Scaffold-from-empty alternate flow.
- **Resolution:** New D60 extends the T1 pattern to every file the save writes, with companion-file-first, configuration-last ordering so a partial save leaves no dangling references.
- **Resolved by:** evidence
- **Raised in round:** R1
- **Affected decisions:** D60 (new)
- **Affected tech-notes:** T1 (referenced)
- **Changed in spec:** Alternate Flows — Scaffold-from-empty

## F80: Default bundle with missing referenced files — copy-from-default behavior unspecified

- **Agent:** junior-developer (round 2)
- **Category:** behavioral gap
- **Finding:** D15 defined the copy scope but said nothing about a default bundle whose config references a file the bundle does not contain. Under the existing spec, the builder would silently skip the missing file and land the user in edit view with an immediate fatal finding — confusing because the user asked for the default.
- **Evidence considered:** D15 text. Validator fatal-on-missing-referenced-file behavior.
- **Resolution:** New D61 commits to a pre-copy reference-integrity check with a copy-anyway / cancel choice on mismatch.
- **Resolved by:** evidence
- **Raised in round:** R1
- **Affected decisions:** D61 (new)
- **Affected tech-notes:** —
- **Changed in spec:** Alternate Flows — Copy-default-to-local

## F81: Numeric field non-numeric input behavior unspecified

- **Agent:** edge-case-explorer (round 2, NF-1)
- **Category:** behavioral gap
- **Finding:** D42 specified paste-time sanitization but said nothing about typed non-numeric characters on numeric fields. Different implementation interpretations (silent drop, inline error, accept-and-reject-at-save) are all consistent with the spec.
- **Evidence considered:** D42.
- **Resolution:** New D62 commits to silent-drop for typed non-numeric characters, paste-sanitize on multi-character input.
- **Resolved by:** evidence
- **Raised in round:** R1
- **Affected decisions:** D62 (new)
- **Affected tech-notes:** —
- **Changed in spec:** Primary Flow step 7

## F82: No-op save behavior unspecified

- **Agent:** edge-case-explorer (round 2, NF-20)
- **Category:** silent-misbehavior risk
- **Finding:** An always-rewrite save would change the file mtime on every invocation, producing spurious conflict dialogs for concurrent observers (other builder sessions or file watchers). The spec did not specify no-op detection.
- **Evidence considered:** D41 conflict signal. Filesystem mtime semantics.
- **Resolution:** New D63 commits to no-op save detection by comparing in-memory serialization against on-disk bytes; no write, no validation, no mtime change, `No changes to save` feedback.
- **Resolved by:** evidence
- **Raised in round:** R1
- **Affected decisions:** D63 (new)
- **Affected tech-notes:** —
- **Changed in spec:** Primary Flow step 9; Edge Cases table

## Evidence-based investigator round 2: line-citation drift corrections

- **Agent:** evidence-based-investigator
- **Category:** drift (minor) and broken (one)
- **Findings consolidated here:** E1-E9 from the investigator's round-2 report.
  - E1: D17 `args.go:100` → `101` (one-line drift; code present)
  - E2: D13 `main.go:174` → `175` (one-line drift; code present)
  - E3: D6 range description sound but undersold (informational)
  - E4: T3 `validator.go:159` → `154` (five-line drift; function present)
  - **E5: D24 `ui.go:81` → `keys.go:81` (BROKEN — wrong file).** The `?` / `statusLineActive` gate that D24's rationale calls out as a defect lives in `keys.go`, not `ui.go`.
  - E6: D1, D10 `sandbox.go:80` slight drift (function at line 80 on reread — confirmed correct)
  - E7: D1, D41-b `sandbox_create.go:19` points at struct type not injection constructor (line 44) — informational
  - E8: D1 `sandbox_login.go:16` blank-line (struct at 17) — minor drift
  - E9: D42 `sandbox.go:69` regex var vs. `stripANSI` function at line 73 — minor drift
  - E26: D26 "most-edited phase" unverifiable behavioral claim (subsumed by F52)
- **Resolution:** Applied in-place citation corrections for E1, E2, E5, E9 directly in the decision log. E3, E4, E7, E8 are minor descriptive drifts left as-is (reader can still find the right code). E5 is the only broken citation; D24's Evidence field now correctly names `keys.go:81`.
- **Resolved by:** evidence
- **Raised in round:** R1
- **Affected decisions:** D13, D17, D24, D42 (Evidence fields)
- **Affected tech-notes:** T3 (Evidence field)
- **Changed in spec:** —

## Meta-finding: Jr-F1 re-raised in round 2 — rejected again

- **Agent:** junior-developer (round 2, implicit in Finding 8)
- **Category:** spec-content rule (mechanics in T-notes)
- **Finding:** Round 2 of the junior-developer lens re-raised the observation that T1 and T2 name specific mechanics (`rename`, `fsync`, `tea.ExecProcess`), similar to the round-1 pushback (first-review F17).
- **Evidence considered:** plan-a-feature skill's operating principles for T-notes (load-bearing mechanics that are not discoverable from code belong in T-notes).
- **Resolution:** **Rejected again.** T-notes are specifically designed to carry this detail for plan-implementation, and the mechanics here meet both criteria (load-bearing and not discoverable from pr9k code). Round 1 of this review softened the prescriptive tone of T1/T2 and corrected their substantive errors (F48, F56); no further softening is warranted.
- **Resolved by:** evidence (review skill's operating principles)
- **Raised in round:** R1
- **Affected decisions:** —
- **Affected tech-notes:** —
- **Changed in spec:** —

## F83: User-initiated redesign — menu bar replaces landing page

- **Agent:** user (direct redesign) / user-experience-designer (round 2 confirmation)
- **Category:** scope change
- **Finding:** The user realized in round 2 that the four-option landing-page model they had originally specified was awkward: it existed only at startup, while the operations it surfaced (switch workflows, create a new one, save the current one, quit) are mid-session operations. The user requested a replacement model: a persistent menu bar at the top of the builder with a File menu containing New, Open, Save, and Quit.
- **Evidence considered:** User's verbatim redesign request. UX designer R2 structured recommendation covering 15 sub-questions of the new model. Junior-developer R2 decisions audit.
- **Resolution:** Landing page removed. Persistent menu bar added. Four landing-page decisions (D2, D8, D31, D50) marked superseded in place (history preserved, with pointers to D64). Six decisions whose trigger moves from landing-page selection to File-menu load events updated (D3, D4, D22, D30, D57). Nine new decisions added (D64 menu-bar model, D65 rendering, D66 activation, D67 shortcuts, D68 initial state, D69 New flow, D70 Open flow, D71 path picker, D72 unsaved-changes auto-resume). Spec Primary Flow steps 1-4 rewritten; step 5 updated for the menu-bar chrome row; Alternate Flows renamed and rewritten (Scaffold-from-empty → File > New empty scaffold; Copy-default → File > New copy; Target switching → File > Open switching); Unsaved-changes interception consolidated across Quit/New/Open with auto-resume.
- **Resolved by:** user input (design direction) + evidence (UX designer detailed recommendations for each sub-question)
- **Raised in round:** R2
- **Affected decisions:** D2 (superseded), D3 (updated), D4 (updated), D8 (superseded), D15 (dependency only), D17 (no change — behavior preserved, trigger moved), D22 (updated), D30 (updated), D31 (superseded), D32 (no change), D50 (superseded), D57 (updated), D61 (dependency only), D64 through D72 (new)
- **Affected tech-notes:** —
- **Changed in spec:** Primary Flow steps 1-5, 9, 10; Alternate Flows (multiple); User Interactions — Affordances; Documentation Obligations; Summary; Review History.

## F84: Menu bar rendering and placement

- **Agent:** user-experience-designer (R2)
- **Category:** behavioral gap (resolved by new decision)
- **Finding:** The user's request named a "header menu bar" but did not specify where it lives relative to the existing session header, whether the session header content moves into the menu bar, or how the layout accommodates the new row on narrow terminals.
- **Resolution:** New D65 commits to a dedicated menu bar row at the very top, separated from the session header by a single horizontal rule. The session header (target path, unsaved indicator, banners, findings summary) stays on its existing row. The menu bar is left-aligned with `File` as the only v1 item and room reserved for future menus (Edit, Help, etc.) without a layout redesign. Two rows of permanent chrome paid once, traded for removing an entire landing-page screen.
- **Resolved by:** evidence
- **Raised in round:** R2
- **Affected decisions:** D65 (new)
- **Affected tech-notes:** —
- **Changed in spec:** Primary Flow step 5; User Interactions — Affordances

## F85: Menu activation model

- **Agent:** user-experience-designer (R2)
- **Category:** behavioral gap (resolved by new decision)
- **Finding:** The user's request did not name an activation key. Keyboard-only users need a discoverable entry point; mouse users need a clickable surface; focus-in-a-text-field cases need precedence rules.
- **Resolution:** New D66 commits to `F10` (universal POSIX menu-bar activation), `Alt+F` (GUI convention, known-fragile under tmux), and mouse click — all three always available. `F10` steals focus from any focused text field; the field's partial input is preserved. `Alt+F` is a convenience alias given its tmux fragility (same approach as D34's `Alt+↑/↓` fallback).
- **Resolved by:** evidence (F10 POSIX precedent; Alt+F GUI precedent; D34 Alt-fragility precedent)
- **Raised in round:** R2
- **Affected decisions:** D66 (new)
- **Affected tech-notes:** —
- **Changed in spec:** Primary Flow steps 2, 5; User Interactions — Affordances

## F86: Menu item keyboard shortcuts and XON/XOFF caveat

- **Agent:** user-experience-designer (R2)
- **Category:** behavioral gap / cross-standard conflict
- **Finding:** The user's request did not specify keyboard shortcuts for individual menu items. Ctrl+S collides with terminal XON/XOFF flow control on many Linux setups — silently freezing output until Ctrl+Q is pressed.
- **Resolution:** New D67 commits to Ctrl+N / Ctrl+O / Ctrl+S / Ctrl+Q (the cross-platform standard vocabulary), intercepted at application level so they work regardless of focused text field. The XON/XOFF collision is documented explicitly — the how-to guide names `stty -ixon` as the mitigation, and File > Save via the menu bar is always a reachable alternative to the shortcut.
- **Resolved by:** evidence (standard shortcut vocabulary; XON/XOFF semantics)
- **Raised in round:** R2
- **Affected decisions:** D67 (new); D38 (Documentation Obligations now includes the `stty -ixon` caveat note)
- **Affected tech-notes:** —
- **Changed in spec:** Primary Flow step 2; Documentation Obligations

## F87: Initial-launch state

- **Agent:** user-experience-designer (R2); junior-developer (R2)
- **Category:** behavioral gap
- **Finding:** With the landing page removed, what does the user see first? Auto-load the D3-resolved default (convenient but silently decides what the user is editing), or empty editor with a hint (explicit but adds a step)? The user's request was silent on this.
- **Resolution:** New D68 commits to the empty-editor state with a centered hint naming File > New and File > Open (with their shortcuts). The one exception: if the user explicitly passed `--workflow-dir`, the builder auto-opens that file via the File > Open code path — treating the flag as an explicit expression of intent.
- **Resolved by:** evidence (Nielsen H3 user control; VS Code / Neovim precedent for no-file startup)
- **Raised in round:** R2
- **Affected decisions:** D68 (new); D3 (updated to scope the default-target resolution to two specific use cases: --workflow-dir auto-open and path-picker pre-fill).
- **Affected tech-notes:** —
- **Changed in spec:** Primary Flow step 2; User Interactions — Affordances

## F88: File > New flow

- **Agent:** user-experience-designer (R2)
- **Category:** behavioral gap (resolved by new decision)
- **Finding:** The user's description of File > New — "prompt for copy-of-default vs empty, then ask where to put it" — was a two-step flow, but didn't cover: (a) what to do when the current session has unsaved changes; (b) whether the copy-from-default integrity check (D61) still applies; (c) whether anything is written to disk before the first File > Save; (d) whether the destination path defaulting to `<projectDir>/.pr9k/workflow/` handles edge cases where `<projectDir>` isn't known.
- **Resolution:** New D69 formalizes the five-step flow: unsaved-changes interception (D72), choice dialog, pre-copy integrity check (D61 when applicable), path picker, load into edit view. Nothing is written to disk until first File > Save; on first save, every file in the bundle lands atomically per D60.
- **Resolved by:** evidence
- **Raised in round:** R2
- **Affected decisions:** D69 (new)
- **Affected tech-notes:** —
- **Changed in spec:** Primary Flow step 3; Alternate Flows — File > New (both variants)

## F89: File > Open flow

- **Agent:** user-experience-designer (R2)
- **Category:** behavioral gap (resolved by new decision)
- **Finding:** The user's description of File > Open — "let the user choose a config.json from wherever they want" — didn't cover: (a) unsaved-changes interception behavior; (b) what happens when the chosen path is a directory or doesn't exist; (c) whether existing load-time behaviors (D4 read-only, D17 symlink, D22 external-workflow, D43 load-time integrity, D36 parse-error recovery) still fire. Junior-developer flagged that "open a config.json" semantically implies opening a bundle (the config plus its companion files rooted at the config's parent directory).
- **Resolution:** New D70 formalizes the three-step flow: unsaved-changes interception, path picker targeting a file, load into edit view with all existing load-time behaviors intact. The path picker shows inline notes when the typed path is a directory or doesn't exist.
- **Resolved by:** evidence
- **Raised in round:** R2
- **Affected decisions:** D70 (new)
- **Affected tech-notes:** —
- **Changed in spec:** Primary Flow step 4; Alternate Flows — File > Open target switching

## F90: Path picker design

- **Agent:** user-experience-designer (R2); junior-developer (R2)
- **Category:** behavioral gap (resolved by new decision)
- **Finding:** The user didn't specify the form of the path picker. The existing TUI stack has no file-browser widget. Options: single text input, embedded file tree, hybrid. Junior-developer flagged that the target persona (workflow author) is shell-proficient.
- **Resolution:** New D71 commits to a single labeled text input with filesystem tab-completion. Matches shell idioms familiar to the target persona; minimal implementation surface; extensible later if needed. Pre-filled with sensible defaults so Enter is often enough.
- **Resolved by:** evidence (persona shell-proficiency from D38; existing TUI dependency stack)
- **Raised in round:** R2
- **Affected decisions:** D71 (new)
- **Affected tech-notes:** —
- **Changed in spec:** Alternate Flows — File > New, File > Open; User Interactions — Affordances

## F91: Unsaved-changes interception and auto-resume for New / Open

- **Agent:** user-experience-designer (R2); junior-developer (R2)
- **Category:** behavioral gap (resolved by new decision)
- **Finding:** The user's request mentioned the three-way unsaved-changes dialog for Quit but was silent on File > New and File > Open. Junior-developer flagged this as the single biggest gap — those two menu items switch workflows mid-session, which is exactly when unsaved changes matter most. They also flagged that the user's description omitted D54's two-step Discard confirmation, potentially suggesting simplification — but couldn't determine whether that was intentional or elided.
- **Resolution:** New D72 commits to the same D54 three-way dialog intercepting File > New and File > Open with auto-resume semantics: Save-success or confirmed-Discard continues the pending action; Save with fatal findings cancels the pending action and opens the findings panel (matching D40's Quit-with-fatals handling); Cancel leaves the user in the current session. D54's two-step Discard confirmation is preserved — the safety feature was added in round 1 for explicit UX reasons (irreversible, no undo) and nothing the user said in R2 indicates removal.
- **Resolved by:** evidence (D40 precedent; D54 safety rationale preserved)
- **Raised in round:** R2
- **Affected decisions:** D72 (new)
- **Affected tech-notes:** —
- **Changed in spec:** Primary Flow steps 3, 4, 10; Alternate Flows — Unsaved-changes interception

## F92: Two-step Discard confirmation removed at user direction

- **Agent:** user (direct simplification)
- **Category:** scope change (simplification)
- **Finding:** After R2, the project-manager flagged to the user that R2's spec preserved D54's two-step Discard confirmation even though the user's R2 redesign request had described the unsaved-Quit dialog as simpler. The user confirmed the intended simplification: unsaved-Quit is single-step (Save / Discard / Cancel, no second `(y/N)` confirmation on Discard).
- **Evidence considered:** User's R3 verbatim response: "yes, simplify the way you quit. unsaved workflow prompts you with those options." The Cancel-default plus rightmost-Discard spatial ordering retains meaningful protection against single-keystroke accidents from the dialog's initial state — the second confirmation was judged disproportionate.
- **Resolution:** D54 marked superseded (history preserved with pointer to D7's R3 simplification). D7 decision text updated to specify single-step Discard. D72 (New/Open interception) updated to match — no two-step Discard anywhere. Spec Primary Flow step 10 and the Unsaved-changes-interception alternate flow rewritten.
- **Resolved by:** user input
- **Raised in round:** R3
- **Affected decisions:** D54 (superseded), D7 (updated), D72 (updated)
- **Affected tech-notes:** —
- **Changed in spec:** Primary Flow step 10; Alternate Flows — Unsaved-changes interception

## F93: Quit always confirms, even with no unsaved changes

- **Agent:** user (direct specification)
- **Category:** behavioral gap (new requirement)
- **Finding:** The user's R3 simplification request named two distinct Quit shapes: unsaved-Quit uses the three-option dialog; **saved-Quit shows a simple confirmation** ("already saved workflow confirms you want to quit"). The spec before R3 was silent on the saved-Quit case — it documented only the unsaved path, implicitly allowing a no-confirm exit when the workflow was clean. The user's specification adds a confirmation on every Quit, regardless of unsaved state.
- **Evidence considered:** User's R3 verbatim: "already saved workflow confirms you want to quit." Existing pr9k TUI has a `ModeQuitConfirm` pattern (`src/internal/ui/ui.go`) that always confirms — matches the user's mental model. Accidental `Ctrl+Q` misfires would otherwise exit the builder silently.
- **Resolution:** New D73 commits to always-confirm-on-Quit with a two-option `(Yes / No)` dialog when there are no unsaved changes. `No` is keyboard default, Escape equivalent to No, `y` or arrow+Enter to confirm exit. D7 updated to reference D73 for the saved-Quit path. Spec Primary Flow step 10 rewritten to document both shapes. New Alternate Flow "Quit confirmation (no unsaved changes)" added.
- **Resolved by:** user input
- **Raised in round:** R3
- **Affected decisions:** D73 (new), D7 (updated)
- **Affected tech-notes:** —
- **Changed in spec:** Primary Flow step 10; Alternate Flows — Quit confirmation (no unsaved changes)
