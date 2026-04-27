# Team Findings: Workflow-Builder TUI Visual Design

Findings from the four review agents dispatched in Step 6 of the `plan-a-feature` skill: junior-developer (JD), user-experience-designer (UX), gap-analyzer (GAP), edge-case-explorer (EC). Findings are renumbered F1..F25 across all agents.

## Review composition

- `junior-developer` — generalist stress-test, cross-standards, scope and assumption surfacing
- `user-experience-designer` — usability, affordance clarity, focus and feedback patterns, accessibility
- `gap-analyzer` — verification that mockups and spec match the behavioral spec's commitments and the 28-mode coverage; full output at [`visual-gaps.md`](visual-gaps.md)
- `edge-case-explorer` — boundary cases for layout (narrow terminals, deeply nested overlays, content overflow)

## Findings

### F1: Reverse-video focus signal overloaded across three states

- **Raised by:** JD (F1), UX (F1), GAP (GAP-011)
- **Severity:** major
- **Finding:** Reverse-video is currently applied to (a) the focused outline row, (b) the focused choice-list / secret-mask input in the detail pane, and (c) the moving step in reorder mode. D22 explicitly *rejects* reverse-video for the outline focus and commits to white-text + `> ` prefix only — but every mockup applies reverse-video on top. This creates an ambiguous focus state when the outline cursor sits on one row while the detail pane is also "active," and reorder mode loses its uniqueness as the only reverse-video state.
- **Resolution:** Reserve reverse-video for two states only: (1) reorder mode (the moving step), and (2) the *highlighted item inside an open dropdown* (choice list, model suggestion list, menu dropdown). Outline focus uses `> ` + white text, no reverse-video. Detail-pane field focus brightens the bracket border to white and renders the cursor `▏`, no reverse-video on the field body. Mockups updated.
- **Affected decisions:** D22 rationale kept; mockups corrected to match D22.
- **Affected tech-notes:** —
- **Changed in spec:** Outline pane — visual structure; Detail pane — field rendering; Reorder mode — visual treatment.
- **Resolved by:** evidence (D22's already-committed rule)

### F2: Chrome row count internally inconsistent

- **Raised by:** JD (F2)
- **Severity:** moderate
- **Finding:** Spec says "9 rows of chrome plus the bottom border" implying 10. D9 says "8 + 1 = 9" but double-counts the menu-bar-to-session-header hrule. Counting from the empty-editor mockup: top border (1) + menu bar (2) + hrule (3) + session header (4) + hrule (5) + pane-area body (6..N-3) + hrule (N-2) + status footer (N-1) + bottom border (N) = 9 chrome rows. The formula `terminalHeight - 9` is correct; the prose is muddled.
- **Resolution:** Rewrite the chrome description in spec and D9 as an explicit enumerated list of 9 rows: top border, menu bar, hrule, session header, hrule, hrule above footer, status footer, bottom border. Wait — that's 8. Recount: there are **3 hrules** (below menu bar, below session header, above footer) plus 5 fixed-content rows (top border, menu bar, session header, status footer, bottom border) = **8 chrome rows** total. The pane-area absorbs `terminalHeight - 8` rows. Update spec and D9.
- **Affected decisions:** D9 rewritten with correct count.
- **Changed in spec:** Layout — the persistent frame.
- **Resolved by:** evidence (counted from existing mockups)

### F3: D6 cites a non-existent `┬`/`┴` precedent in run-mode TUI

- **Raised by:** JD (F3)
- **Severity:** moderate
- **Finding:** D6's "Evidence" claims the run-mode hrule construction provides a precedent for `┬`/`┴` junctions. It does not — the run-mode hrule is `├` + `─` repeat + `┤`, no mid-rule junctions exist anywhere in `internal/ui/`. D6 is introducing a *new* pattern, not extending an existing one.
- **Resolution:** Rewrite D6's evidence to acknowledge `┬`/`┴` are a new pattern this spec introduces, justified by D1's chrome-glyph commitment and D46's BMP-only commitment (both of which already include T-junction glyphs in scope). Update spec's Coordinations table to call out the one element that does not trace to a run-mode precedent.
- **Affected decisions:** D6 evidence rewritten.
- **Changed in spec:** Layout — the persistent frame; Coordinations.
- **Resolved by:** evidence

### F4: Browse-only footer's red "Save disabled" phrase conflicts with `colorShortcutLine` tokenizer

- **Raised by:** JD (F4)
- **Severity:** moderate
- **Finding:** `colorShortcutLine` tokenizes on two-space group separators and applies white-key/gray-description coloring within each group. It cannot render a third color (red) for an arbitrary phrase. The browse-only footer mockup shows `Save disabled — target is read-only` in red, which the existing tokenizer cannot produce.
- **Resolution:** Move the "Save disabled" signal **out of the footer** entirely. The `[ro]` session-header banner already communicates the read-only state with the redundant `[ro]` text prefix and red coloring. The browse-only footer reverts to a normal navigation shortcut bar that the existing tokenizer can render: `↑↓ navigate  Tab detail pane  Esc back  ?  help`. Update mockup `06-status-footer-modes.md` accordingly.
- **Affected decisions:** None (visual mockup edit only).
- **Changed in spec:** Status footer — content per mode (browse-only entry).
- **Resolved by:** evidence

### F5: Top-border title separator color ambiguous; `colorTitle` only handles one separator

- **Raised by:** JD (F5)
- **Severity:** minor
- **Finding:** The builder title has *two* ` — ` separators (`AppTitle — Workflow Builder — <path>`) but `colorTitle` splits on the first separator only. D3 says "the rest renders white" without distinguishing whether the separators are white or gray.
- **Resolution:** Update D3 to explicitly state: the brand name `Power-Ralph.9000` renders **green**; everything from the first ` — ` onward (including both separators and "Workflow Builder" and the path) renders **white**. Implementation note added in D3's evidence: `colorTitle` already produces this result for the two-separator title because its split-on-first-separator rule treats the entire suffix as the white span.
- **Affected decisions:** D3 clarified.
- **Changed in spec:** Layout — the persistent frame.
- **Resolved by:** evidence (`colorTitle` source at `model.go:699-707`)

### F6: D15 banner-panel close-button color rule not explicitly cross-referenced from D37

- **Raised by:** JD (F6)
- **Severity:** minor
- **Finding:** The banner panel (opened via `[N more warnings]`) shows a single Close button. D15 says it follows "dialog conventions" via D36 (overlay shape) but does not explicitly say it follows D37 (keyboard-default green brackets). Mockup 03 Variant H renders `[ Close ]` in green (correct per D37), but the decision text is silent.
- **Resolution:** Add a one-sentence cross-reference to D15: "The banner panel's single Close button follows D37's keyboard-default rendering rule (green `[ ]` brackets)."
- **Affected decisions:** D15 sentence added.
- **Resolved by:** evidence

### F7: `[N more warnings]` and findings summary share visual weight in the session header

- **Raised by:** UX (F2)
- **Severity:** moderate
- **Finding:** Both render in white on the session header row, looking like equivalent affordances despite triggering different overlays.
- **Resolution:** Prefix the findings summary with a bracketed severity tag: `[!] 3 fatal · 2 warn` in **red** when fatals exist, `[i] 2 warn · 1 info` in **cyan** when only warn/info, no prefix when zero counts. This matches the banner vocabulary (`[ro]`, `[ext]`, etc.) and visually disambiguates the two affordances.
- **Affected decisions:** D16 updated to commit to the prefix tags and color rule.
- **Changed in spec:** Session header — visual content and state.
- **Affected mockups:** `01`, `03`, `09`.
- **Resolved by:** evidence

### F8: `[press r to reveal]` brackets imply a clickable affordance

- **Raised by:** UX (F3)
- **Severity:** minor
- **Finding:** Bracket syntax is used elsewhere for clickable handles (`[Ctrl+E open in editor]`, `[ Cancel ]`); using brackets around the static instructional text `press r to reveal` falsely signals interactivity.
- **Resolution:** Drop the brackets. Render as `r to reveal` / `r to mask` in **light gray** without delimiters. Footer continues to advertise `r reveal`.
- **Affected decisions:** D30 updated to remove brackets from the hint.
- **Changed in spec:** Detail pane — secret-mask fields.
- **Affected mockups:** `05` (sections H, I, J, K).
- **Resolved by:** evidence

### F9: Reorder banner suppresses higher-priority `[ro]` / `[ext]` banners

- **Raised by:** UX (F4), EC (E5)
- **Severity:** moderate
- **Finding:** D41 says the reorder banner replaces the priority banner for the duration of reorder mode. Entering reorder on a read-only file silently hides the read-only signal. Footer + reverse-video + green gripper already give three independent reorder signals.
- **Resolution:** Remove the reorder banner from the session header entirely. The three existing signals (footer change, reverse-video row, green gripper) are sufficient per Nielsen visibility, and the priority banner stays put. Add a mockup variant for "reorder while a banner is already active" showing the banner preserved.
- **Affected decisions:** D41 updated.
- **Changed in spec:** Reorder mode — visual treatment.
- **Affected mockups:** `07` (all variants edit the session-header banner state).
- **Resolved by:** evidence (Nielsen heuristic 1 + D14 priority rule)

### F10: Help modal column-count threshold not specified

- **Raised by:** UX (F5)
- **Severity:** minor
- **Finding:** The help modal renders two-column at 120 cols and single-column at 70 cols, but no explicit threshold is stated. Implementers will pick different breakpoints.
- **Resolution:** Add to spec: "The help modal renders its shortcut grid in two columns when the interior width (modal width minus 4 for borders and padding) is at least 56 columns; below that, the grid renders in a single column." Update D40 with the same.
- **Affected decisions:** D40 extended.
- **Changed in spec:** Help modal — visual layout.
- **Resolved by:** evidence

### F11: Path picker inline warning lacks textual prefix

- **Raised by:** UX (F6)
- **Severity:** moderate
- **Finding:** Yellow inline warnings in the path picker (`↳ that is a directory`) violate the spec's own rule (D14, D25) that color is supplementary to a textual severity tag.
- **Resolution:** Prepend `[warn]` (or `[hint]` for non-warning notes like "completed to single match") to every inline notice in the path picker. Keep the `↳` glyph as a visual flow cue.
- **Affected decisions:** D42 updated.
- **Changed in spec:** Path picker — visual layout.
- **Affected mockups:** `11` (variants C, E, F, G).
- **Resolved by:** evidence

### F12: `DialogUnsavedChanges` footer redundantly lists `Enter cancel`

- **Raised by:** UX (F7)
- **Severity:** minor
- **Finding:** Listing `Enter cancel` in the footer is redundant with the green-bracket default and contradicts other dialogs where Enter activates a non-Cancel default.
- **Resolution:** Remove `Enter cancel` from the dialog's footer line. The `[ Cancel ]` green-bracket footer already communicates Enter-activates-default per D37.
- **Affected decisions:** None (mockup edit only).
- **Affected mockups:** `12`.
- **Resolved by:** evidence

### F13: `DialogSaveInProgress` is missing — no DialogKind constant, no mockup

- **Raised by:** GAP (GAP-001), GAP (GAP-004)
- **Severity:** major
- **Finding:** Mode-coverage rows 20–21 commit to a `DialogSaveInProgress` dialog that the impl-decision D-8 enum does not name. No mockup exists.
- **Resolution:** Add `DialogSaveInProgress` to the spec's dialog list (note the impl-decision log will need updating in a follow-up under PR-3 work). Add a new mockup file `23-dialog-save-in-progress.md`.
- **Affected decisions:** D36 updated to include the new dialog kind in the canonical list.
- **Changed in spec:** Dialogs — common visual conventions; Status footer (mode 18 entry).
- **Affected mockups:** new file `23-dialog-save-in-progress.md`.
- **Resolved by:** evidence

### F14: Browse-only full-frame mockup missing

- **Raised by:** GAP (GAP-002)
- **Severity:** major
- **Finding:** Read-only target is a persistent edit-view mode with three simultaneous visual signals (greyed Save, no dirty tracking, `[ro]` banner). No single full-frame mockup assembles all three.
- **Resolution:** Add new mockup file `24-full-layout-browse-only.md` showing the full frame for read-only mode.
- **Affected decisions:** None (new mockup file).
- **Resolved by:** evidence

### F15: Mode 5 help-modal-over-empty-editor variant missing

- **Raised by:** GAP (GAP-010)
- **Severity:** moderate
- **Finding:** Mode 5 starts in `EmptyEditor` and opens the help modal; the existing help-modal mockup uses an edit-view base frame.
- **Resolution:** Add a third variant to mockup `08-help-modal.md` showing the modal centered over the empty-editor frame (the centered hint panel visible in the background, dimmed).
- **Affected decisions:** None (mockup edit).
- **Affected mockups:** `08`.
- **Resolved by:** evidence

### F16: Detail-pane scroll indicator missing from spec and mockups

- **Raised by:** GAP (GAP-008)
- **Severity:** moderate
- **Finding:** Behavioral D52 commits to the detail pane being independently scrollable with a visible indicator. The visual spec defines the outline scroll indicator (D25) but is silent on the detail pane's.
- **Resolution:** Add a new decision D47 specifying the detail-pane scroll indicator (mirrors D25's outline-pane shape). Add a variant to mockup `05-detail-pane-fields.md`.
- **Affected decisions:** new D47.
- **Changed in spec:** Detail pane — field rendering (scroll indicator subsection added).
- **Affected mockups:** `05` (new variant T).
- **Resolved by:** evidence

### F17: `Validating…` transient state has no full-frame mockup

- **Raised by:** GAP (GAP-003)
- **Severity:** moderate
- **Finding:** Mode 18's `validateInProgress` leg is named only as a footer text snippet; the full-frame state is not rendered.
- **Resolution:** Add a section to mockup `06-status-footer-modes.md` showing the full-frame state during validation: same edit-view chrome, footer reads `Validating…`, dialog overlay reads `DialogSaveInProgress` (per F13's new mockup) once the validate phase ends.
- **Affected decisions:** None.
- **Affected mockups:** `06`, `23`.
- **Resolved by:** evidence

### F18: Minimum supported terminal size — width and height floor undefined

- **Raised by:** EC (E1, E2)
- **Severity:** major
- **Finding:** At narrow widths (<40 cols) the pane split formula produces undefined results. At short heights (<10 rows) the pane area is zero or negative. Spec is silent on either floor.
- **Resolution:** Add a new decision D48 committing to a minimum supported terminal size of **60 columns × 16 rows** (60 columns ensures a usable outline pane with the formula `min(40, max(20, ⌊60×0.4⌋)) = 24` plus a detail pane of `60 - 2 - 1 - 24 = 33` cols; 16 rows leaves 8 pane rows after the 8-row chrome). Below this, the builder renders a single-line "Terminal too small — minimum 60×16 required" in the center of the screen and refuses to draw the full frame. Add a mockup file note in the README pointing at this fallback.
- **Affected decisions:** new D48.
- **Changed in spec:** Layout — the persistent frame (Minimum supported terminal size subsection added).
- **Resolved by:** evidence (chrome budget arithmetic + UX standard for "graceful degradation")

### F19: Outline overflow stress test at 80 cols missing real content pressure

- **Raised by:** EC (E3)
- **Severity:** moderate
- **Finding:** Variant L of mockup 03 uses artificially short content. A real overflow stress (long path + full-length banner + multi-banner + findings summary) is not shown.
- **Resolution:** Add a new variant M to mockup `03-session-header-banners.md` showing the full overflow priority order applied with realistic content lengths.
- **Affected mockups:** `03`.
- **Resolved by:** evidence

### F20: Step-name truncation rule undefined

- **Raised by:** EC (E4)
- **Severity:** moderate
- **Finding:** Long step names overflow the outline pane; no rule defines whether name truncates with `…`, model column collapses first, or the row breaks.
- **Resolution:** Add a new decision D49: step name right-truncates with `…` to preserve at least 12 columns for the right-aligned model column. Below 12 cols of model space, the model column drops entirely (replaced by `…`). Add a mockup variant.
- **Affected decisions:** new D49.
- **Changed in spec:** Outline pane — visual structure (truncation subsection added).
- **Affected mockups:** `04` (new variant L).
- **Resolved by:** evidence

### F21: Long containerEnv key label truncation undefined

- **Raised by:** EC (E7)
- **Severity:** moderate
- **Finding:** containerEnv field labels are user-supplied keys; they can be arbitrarily long and overflow the row.
- **Resolution:** Add a new decision D50: detail-pane field labels truncate with `…` after 28 characters. Update mockup with a variant showing a long key.
- **Affected decisions:** new D50.
- **Changed in spec:** Detail pane — field rendering.
- **Affected mockups:** `05` (new variant U).
- **Resolved by:** evidence

### F22: Choice-list dropdown overflow rule undefined

- **Raised by:** EC (E8)
- **Severity:** moderate
- **Finding:** When a field is near the bottom of the pane and its dropdown would extend below the pane, the spec is silent on whether the dropdown clips, scrolls internally, or renders upward.
- **Resolution:** Add a new decision D51: when an open dropdown would extend below the pane bottom, it renders **upward** instead, anchored to the field's bottom edge instead of its top. If the dropdown would also exceed the available space upward, it scrolls internally. Update mockup with a variant.
- **Affected decisions:** new D51.
- **Changed in spec:** Detail pane — field rendering (choice list subsection extended).
- **Affected mockups:** `05` (new variant V).
- **Resolved by:** evidence

### F23: Post-editor-return scroll/cursor preservation undefined

- **Raised by:** EC (E6)
- **Severity:** moderate
- **Finding:** When the external editor exits, the detail pane's scroll position and the focused field's position relative to the pane must be reconciled with the (possibly resized) terminal.
- **Resolution:** Add a sentence to the spec's "External-editor handoff" section: "On post-handoff render, the detail pane scrolls to make the just-edited field visible. If the terminal was resized during the editor session, the resize is treated as a `tea.WindowSizeMsg` on the first post-return render cycle, and the scroll-to-field rule applies after the resize."
- **Affected decisions:** D44 extended.
- **Changed in spec:** External-editor handoff — visual layout.
- **Resolved by:** evidence

### F24: Help modal scroll indicator placement undefined

- **Raised by:** EC (E9)
- **Severity:** minor
- **Finding:** When the help modal content exceeds terminal height, the spec mentions a scroll indicator and a pinned dismiss hint, but does not specify where the scroll indicator renders inside the modal's bordered frame.
- **Resolution:** Extend D40: the help modal's scroll indicator runs down the **second-rightmost column** (one column inside the right border, leaving the `│` border intact). Add a variant to mockup `08-help-modal.md`.
- **Affected decisions:** D40 extended.
- **Affected mockups:** `08`.
- **Resolved by:** evidence

### F25: Reorder mode preserves prior banner — variant missing

- **Raised by:** EC (E5)
- **Severity:** moderate
- **Finding:** Now that F9 resolves the reorder banner to *not* replace the priority banner, the "reorder while [ro] is active" case must be shown.
- **Resolution:** Add variant G to mockup `07-reorder-mode.md` showing reorder active with `[ro]` banner still visible.
- **Affected mockups:** `07`.
- **Resolved by:** evidence
