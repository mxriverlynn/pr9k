# Gap Analysis: Workflow-Builder TUI Visual Design vs. Behavioral References

## Comparison Direction

**Current state:** Visual design spec at `docs/plans/workflow-builder-tui-design/feature-specification.md`, `artifacts/decision-log.md`, and `mockups/` (24 files).

**Desired state:** The three reference inputs the visual spec is obligated to cover:
1. `docs/plans/workflow-builder/feature-specification.md` — behavioral commitments with a visual surface
2. `docs/plans/workflow-builder/artifacts/tui-mode-coverage.md` — the 28 enumerated TUI modes, each requiring a visualizable starting state and ending state
3. The 14 named `DialogKind` constants from `docs/plans/workflow-builder/artifacts/implementation-decision-log.md#D-8`

Default comparison direction applied: current state (visual design) checked against desired state (behavioral references).

---

## Scope

Comparison areas:

1. **Dialog coverage** — one mockup per named `DialogKind` constant
2. **Mode-coverage reconstruction** — every row in the 28-mode table must be reconstructable from the mockups (start state + end state both visualizable)
3. **Behavioral-spec visual surface** — every behavioral commitment in the feature spec that has a visible manifestation
4. **Visual-spec internal consistency** — contradictions between the visual spec and its own decision log

Excluded: behavioral decisions without a visual surface (async save sequencing, atomic-write mechanics, package decomposition, testing strategy). These are implementation concerns outside the visual spec's declared scope.

---

## Summary

The visual design spec and its 24 mockup files cover the large majority of dialogs, modes, and behavioral surface. Gaps cluster in four areas: the `DialogSaveInProgress` dialog (committed by the mode-coverage table but absent from the `DialogKind` enumeration and from all mockups), several mode-coverage rows whose ending states are only implicitly derivable rather than explicitly shown, a handful of behavioral details that the spec describes in prose but that no mockup renders, and two color-cue inconsistencies between the spec and its own decision log.

| Category | Count | Description |
|----------|-------|-------------|
| Missing  | 3     | Elements in the desired state with no current state correspondence |
| Partial  | 7     | Elements present in both but incompletely covered |
| Divergent | 2    | Elements addressing the same concern in incompatible ways |
| Implicit | 4     | Assumed capabilities neither confirmed nor denied |

Full analysis written to: `/Users/mxriverlynn/dev/mxriverlynn/pr9k/docs/plans/workflow-builder-tui-design/artifacts/visual-gaps.md`

---

## Findings

---

### Missing

**GAP-001: `DialogSaveInProgress` dialog has no mockup and no `DialogKind` constant**

- **Category:** Missing
- **Feature/Behavior:** Mode-coverage rows 20 and 21 commit to a dialog that opens when `Ctrl+Q` is pressed while a save is in progress (`saveInProgress=true`). Row 20 names the state `EditView + DialogSaveInProgress` and names the observable change as "dialog shows 'Save in progress — please wait'." Row 21 names the ending state transition when `saveCompleteMsg` arrives.
- **Current State:** No mockup file covers `DialogSaveInProgress`. The `DialogKind` enum in `implementation-decision-log.md#D-8` lists 14 constants (DialogNone through DialogEditorError); `DialogSaveInProgress` is not among them. The footer-modes mockup (`06-status-footer-modes.md`) shows a `Saving...` footer line but does not show a dialog overlay. No file in `mockups/` renders this overlay.
- **Desired State:** `tui-mode-coverage.md` rows 20–21 commit to this state with named transitions. Row 20: "Starting state: `EditView + DialogSaveInProgress`, pendingQuit=true; observable change: dialog shows 'Save in progress — please wait'." The behavioral feature spec (Primary Flow step 9, feedback section) also commits to a "Validating…" status being visible when validation takes time.

---

**GAP-002: Browse-only (read-only) edit-view full-frame mockup is absent**

- **Category:** Missing
- **Feature/Behavior:** The behavioral spec's Alternate Flow "Read-only target" describes a full edit-view mode where Save is greyed, unsaved-change tracking is disabled, and a `[ro]` banner is persistent. This is a distinct persistent mode, not merely a session-header variant. No full-frame mockup shows the read-only edit view as a complete frame with its footer shortcut line, greyed Save in the menu, and disabled tracking simultaneously.
- **Current State:** `03-session-header-banners.md` shows the `[ro]` banner variant in isolation (session-header row only). `02-menu-bar.md` (State D) shows the greyed Save menu item. `06-status-footer-modes.md` shows the browse-only footer line. These are three separate partial renders in three different mockup files. No single full-frame mockup assembles them into the browse-only view that the builder actually presents — a frame where all three visual signals coexist simultaneously.
- **Desired State:** The behavioral spec's "Read-only target" alternate flow: "The builder enters the edit view in browse-only mode: layout is identical to normal edit view, but File > Save is greyed out in the menu (its shortcut is inert), unsaved-change tracking is disabled, and the session header shows a prominent 'read-only' indicator." This is a named mode with three simultaneous visual signals; the visual spec's `feature-specification.md#empty-editor-view` section documents only the empty-editor full-frame case as a comparable situation, not the browse-only case. The `tui-mode-coverage.md` does not explicitly enumerate browse-only as a numbered mode, but the behavioral spec section names it as a persistent alternate flow with its own visual surface.

---

**GAP-003: Validating status — the transient `Validating…` footer state has no mockup frame**

- **Category:** Missing
- **Feature/Behavior:** The behavioral spec's feedback section states: "Validation runs on save show a brief 'Validating…' status when validation takes more than a fraction of a second." The `06-status-footer-modes.md` file lists `Validating...` as a footer label but only as a text entry under "Save validation in progress." There is no full-frame or even partial-frame mockup showing the full screen during the validation transient — crucially, showing what the rest of the frame looks like (is the outline still navigable? does the header update?) during this interstitial window.
- **Current State:** `06-status-footer-modes.md` line: `│  Validating...  (white)v0.7.3(/) │` — this is a footer-row-only snippet, not a full-frame render. The save-flow sequence (mode 18) description in `tui-mode-coverage.md` labels this `validateInProgress` as a transient state before `saveInProgress`.
- **Desired State:** `tui-mode-coverage.md` row 18 commits to a `validateInProgress` transient state in the sequence `EditView + validateInProgress` → `EditView + saveInProgress` → `EditView`. Both `validateInProgress` and `saveInProgress` are named intermediate states. The visual spec's feature-specification.md at "Status footer — content per mode" states the footer "varies across all 28 modes from the mode-coverage table" and that "every mode's footer string is enumerated in the mockups." The Validating state is mode-18's first leg, which should have a frame.

---

### Partial

**GAP-004: Mode-coverage rows 19 and 20–21 (save-gated Ctrl+S and save-in-progress Quit) lack end-state mockups**

- **Category:** Partial
- **Feature/Behavior:** The mode-coverage table specifies end states for modes 19, 20, and 21. Mode 19's end state is "state unchanged" (Ctrl+S silently consumed when save is in progress). Mode 20's end state is the `DialogSaveInProgress` dialog. Mode 21's end state is "dialog transitions based on post-save state" — which must visually branch to either the quit-confirm dialog or the unsaved-changes dialog.
- **Current State:** Mode 19's end state ("no new goroutine spawned; state unchanged") has no distinct mockup because it is visually identical to the pre-keypress state — this is acceptable for a silent no-op. Mode 20's dialog has no mockup (covered by GAP-001). Mode 21's branch transitions (after `saveCompleteMsg` arrives) are described only in prose in the mode-coverage table. The existing `13-dialog-quit-confirm.md` and `12-dialog-unsaved-changes.md` show those dialogs in their base forms, but neither file annotates them as "post-save-complete transition target" or shows the mode-21 entry path explicitly.
- **Desired State:** Mode-coverage table rows 20–21 commit to specific end states with observable changes that the test asserts. The visual spec claims at its Summary section that mockups cover "every full-frame state, every dialog from DialogKind." The mode-21 transition from `DialogSaveInProgress` → (quit-confirm or unsaved-changes dialog) is a named observable change with two visual branches.

---

**GAP-005: Mode 8 (`Del` → `DialogStepRemoveConfirm`) end state — focus-fallthrough rendering not shown**

- **Category:** Partial
- **Feature/Behavior:** Mode 8's end state is `EditView + DialogStepRemoveConfirm`. The behavioral spec's edge case "Focused step is removed" commits to cursor fallthrough: focus falls to the step below, then the step above, then the `+ Add step` row. After `d` is pressed in the remove-confirm dialog, this fallthrough must be visible in the outline. The `DialogRemoveConfirm` mockup (`20-dialog-remove-confirm.md`) shows the dialog only; no mockup shows the post-confirm outline with cursor in its fallthrough position.
- **Current State:** `20-dialog-remove-confirm.md` outcome path text states: "cursor falls through to the next item per the Edge Cases table behavior" — but no frame renders the resulting outline state. The `01-full-layout-edit-view.md` and `04-outline-panel.md` show the outline in its normal state; neither shows the removed-step fallthrough case.
- **Desired State:** `tui-mode-coverage.md` row 8 specifies the end state as `EditView + DialogStepRemoveConfirm` with observable change "confirm dialog 'Delete step? Delete / Cancel'." The behavioral spec edge-case table and `feature-specification.md`'s "Focused step is removed" entry commit to the fallthrough as a distinct visual transition that must be observable.

---

**GAP-006: Mode 9 (`Alt+↑` step move) end state — the one-step reorder via Alt key has no dedicated mockup frame**

- **Category:** Partial
- **Feature/Behavior:** Mode 9 commits to `Alt+↑` as a keyboard shortcut that moves a step up one row in the outline without entering persistent reorder mode. The behavioral spec describes it as a "transient one-keypress reorder cycle." The `07-reorder-mode.md` file covers the persistent `r`-entered reorder mode. There is no mockup showing the frame immediately after `Alt+↑` is pressed — which visually is the same as a committed reorder, but the mode-coverage table treats it as its own transition (row 9).
- **Current State:** `07-reorder-mode.md` covers modes 10–13 (persistent reorder mode: enter, move, commit, cancel). `06-status-footer-modes.md` and `04-outline-panel.md` cover the surrounding visual context. No file shows the frame that results specifically from `Alt+↑` (which, per the behavioral spec, is a "transient" cycle that does not enter the reorder-mode state machine).
- **Desired State:** `tui-mode-coverage.md` row 9: "Starting state: `EditView`, outline focus on step row; Input: `Alt+↑`; Expected next state: `EditView`, step moves up one row; Observable change: outline order updates; step index decremented." This is a distinct end state from reorder-mode (the footer does not change, no reorder banner appears) and requires its own visual example to distinguish it from modes 11–12.

---

**GAP-007: Section-summary view for `statusLine` section in the detail pane — the `[Edit fields]` action path is only partially specified**

- **Category:** Partial
- **Feature/Behavior:** `05-detail-pane-fields.md` variant S shows the statusLine section-summary view with an `[Edit fields]` action. The annotation states this action "moves cursor to first field of the statusLine block in detail-pane edit mode." However, no mockup shows what the detail pane looks like after that action is taken — i.e., the statusLine block's actual editable field view.
- **Current State:** `05-detail-pane-fields.md` variant S shows the section summary view including the `[Edit fields]` action affordance. No mockup shows the statusLine block's field-edit view (the equivalent of the step-field view in `01-full-layout-edit-view.md`'s detail pane). The outline panel mockup (`04-outline-panel.md`) shows the `[≣] script-based` item row in the outline but does not show what appears in the detail pane when that row is focused.
- **Desired State:** The behavioral spec's D51 enumerates section-summary content per section type including the statusLine block ("shows type, command, and refresh interval"), and `feature-specification.md` step 7 lists statusLine fields as editable. The visual spec's `feature-specification.md` at "Detail pane — field rendering" describes numeric fields for `Refresh` and text/choice fields for statusLine; these must be visualized for the statusLine block. The `[Edit fields]` affordance implies a transition, and the target of that transition is not rendered.

---

**GAP-008: Detail pane scroll-position indicator is specified in prose and the behavioral spec but not rendered in any mockup**

- **Category:** Partial
- **Feature/Behavior:** The behavioral spec's D52 commits to: "The detail pane is independently scrollable from the outline, with its own scroll-position indicator when content exceeds pane height." The visual spec's `feature-specification.md` does not have a section defining the detail-pane scroll indicator's visual form (contrast with the outline scroll indicator, which has its own section "D25: Outline scroll indicator rendering" and a full mockup in `04-outline-panel.md` variant G).
- **Current State:** The outline's scroll indicator is fully specified and mocked (visual decision D25, `04-outline-panel.md` variant G, `07-reorder-mode.md` variant F). The findings panel's scroll indicator is shown in `09-findings-panel.md` variant F. The detail pane's scroll indicator has no visual decision entry in the decision log, no mockup variant, and no dedicated section in the visual feature-specification.md.
- **Desired State:** `docs/plans/workflow-builder/feature-specification.md` D52: "The detail pane is independently scrollable from the outline, with its own scroll-position indicator when content exceeds pane height." This is an explicit behavioral commitment with a visual surface. The visual spec's decision log contains D25 (outline scroll indicator) but has no corresponding decision for the detail pane's scroll indicator.

---

**GAP-009: Post-editor-exit warning banner is described in prose but not rendered in any mockup**

- **Category:** Partial
- **Feature/Behavior:** The visual spec's `feature-specification.md` at "External-editor handoff — visual layout" (step 3, post-handoff frame) states: "the dialog is gone; the detail pane shows the just-edited field with any warning/info banners (e.g., 'editor exited non-zero — file re-read anyway') shown briefly." Decision D44 repeats this: "the post-handoff frame's brief warning, when applicable, surfaces the non-zero-exit signal." The `18-dialog-external-editor-opening.md` outcome path also mentions this warning banner. However, no mockup renders what this warning looks like in the session header.
- **Current State:** `18-dialog-external-editor-opening.md` outcome path states: "Other non-zero exit: builder reclaims, dialog dismisses, brief warning banner `editor exited non-zero — file re-read anyway` appears in the session header." The text describes it, but no mockup shows a session-header variant with this transient warning. `03-session-header-banners.md` covers the five persistent banner types and two transient banners (Saved at / No changes) but not the editor-nonzero-exit transient.
- **Desired State:** The behavioral spec's "External-editor invocation" alternate flow states: "Editor exits non-zero — builder still re-reads the file, because the external editor may have written partial content before failing; the user is informed of the non-zero exit." This "user is informed" commitment requires a visual form. The visual spec's decision D44 commits to showing this but provides no mockup frame for it.

---

**GAP-010: Mode 5 (`?` from empty-editor → `EmptyEditor + helpOpen`) end state — help modal over empty-editor not shown**

- **Category:** Partial
- **Feature/Behavior:** Mode-coverage row 5 commits to the transition `EmptyEditor, no overlay` + `?` → `EmptyEditor + helpOpen`, with observable change "help modal renders." The help modal mockup (`08-help-modal.md`) shows the modal centered over the edit view. It does not show the modal centered over the empty-editor state, where the underlying frame looks different (no outline content, the hint panel in the detail pane).
- **Current State:** `08-help-modal.md` shows the help modal centered over a populated edit view (modes 5 in edit-view context, 23). The file's cross-reference section lists "Mode coverage: rows 5, 23." However, row 5 starts in `EmptyEditor`, not `EditView`. The render in `08-help-modal.md` shows outline rows with steps visible behind the modal, which is the edit-view background — not the empty-editor background with its `No workflow open` label and hint panel.
- **Desired State:** `tui-mode-coverage.md` row 5: "Starting state: `EmptyEditor`, no overlay; Input: `?`; Expected next state: `EmptyEditor + helpOpen`; Observable change: help modal renders." The underlying frame behind the help modal differs between empty-editor and edit-view: the outline shows `No workflow open` and the detail pane shows the hint box. This is a distinct visual state.

---

### Divergent

**GAP-011: Focused choice-list field rendering — visual spec uses reverse-video; behavioral spec and decision log specify brightened border**

- **Category:** Divergent
- **Feature/Behavior:** How a focused choice-list field is visually distinguished from an unfocused one.
- **Current State:** `05-detail-pane-fields.md` variant C annotation states: "When focused, the entire input area renders in reverse-video to signal 'press Enter to open'." This appears in the closed-focused state. The same reverse-video signal is used for the secret-mask field (variants I and J). The visual decision log D28 states for choice-list: "When focused, the bracket border brightens to `White`" (reproducing the behavioral spec D27's unfocused-signifier rule). The visual feature-specification.md section "Constrained-fields (choice list)" states: "When focused, the bracket border brightens to `White`."
- **Desired State:** Visual decision D28 and the visual feature-specification.md prose both specify bracket-border brightening to `White` for the focused choice-list. The mockup variant C implements reverse-video on the full input area instead. These are incompatible visual treatments: brightened border means only the `[` and `]` characters change color, while reverse-video inverts the entire cell background within the brackets. The implementation cannot produce both simultaneously.
- **Evidence pair:** Current state — `05-detail-pane-fields.md` variant C: "When focused, the entire input area renders in reverse-video." Desired state — Visual decision D28: "When focused, the bracket border brightens to `White`"; visual feature-specification.md "Constrained-fields (choice list)": "the bracket border brightens to `White` for the focused state."

---

**GAP-012: Session-header findings-summary color — spec says `LightGray`, mockup annotation says `white`**

- **Category:** Divergent
- **Feature/Behavior:** The foreground color of the validator findings summary in the session header row.
- **Current State:** `01-full-layout-edit-view.md` annotation states: "findings summary `3 fatal · 2 warn` right-aligned in **light gray** with the bullet `·` glyph." `03-session-header-banners.md` variant I shows `3 fatal · 2 warn` inline in the session-header row with no explicit color annotation, implying the default (light gray). However, the findings summary is described in the visual feature-specification.md as a clickable, interactive affordance ("Activated by clicking it or pressing the help-modal-equivalent shortcut to open the findings panel"), and visual decision D16 compares it to the version-label's position convention without specifying its color explicitly beyond "right-aligned metadata."
- **Desired State:** The visual feature-specification.md session-header section states "findings summary — right-aligned at the row's right edge. Format `<F> fatal · <W> warn · <I> info` when any non-zero count is present." Decision D16 does not name a foreground color for the findings summary itself. The behavioral spec's User Interactions — Affordances describes the findings summary as interactive (opening the findings panel). The run-mode TUI convention is that interactive affordances render in `White` rather than `LightGray`. The `[N more warnings]` affordance renders in `White` (per visual decision D15: "Renders in `White` so it reads as actionable"). If the findings summary is also interactive, it should render in `White` by the same logic — but the mockup renders it in `LightGray`.
- **Evidence pair:** Current state — `01-full-layout-edit-view.md`: "findings summary `3 fatal · 2 warn` right-aligned in **light gray**." Desired state — visual decision D15: "`[N more warnings]` affordance renders in `White` so it reads as actionable." The findings summary is an equally interactive affordance (activating it opens the findings panel per visual spec prose and D16) and should be colored consistently with the `[N more warnings]` convention.

---

### Implicit

**GAP-013: Reorder mode + help modal coexistence — permitted or not?**

- **Category:** Implicit
- **Feature/Behavior:** The visual spec's decision D8 (implemented from the behavioral impl-decision log D-8) states that `helpOpen` only flips true when `dialog.kind == DialogFindingsPanel`. Reorder mode is not a dialog kind — it is implemented as a separate state in the model. Whether pressing `?` during reorder mode opens the help modal, is suppressed, or exits reorder mode first is unspecified and unvisualized.
- **Current State:** `07-reorder-mode.md` does not include a `?` input or any mention of help-modal interaction during reorder mode. `08-help-modal.md` shows the modal only over edit view and findings panel. The help-modal section in `08-help-modal.md` lists "Outline focus (cursor on a step row)" as a section but does not indicate whether this section is reachable from reorder mode.
- **Desired State:** The behavioral spec's Primary Flow step 8 states: "The help modal is unconditionally reachable from the edit view or the findings panel, regardless of any other configuration." Reorder mode is a sub-state of edit view. Whether reorder mode counts as "edit view" for this purpose — and what the user sees if `?` is pressed during it — is not visualized.

---

**GAP-014: `[N more warnings]` banner panel coexistence with the findings panel — not addressed**

- **Category:** Implicit
- **Feature/Behavior:** The session-header `[N more warnings]` affordance opens "a small panel listing all active banners" (visual decision D15, `03-session-header-banners.md` variant H). The findings panel occupies the detail pane (visual decision D38). Whether both can be open simultaneously — or whether opening the banner panel while the findings panel is active is suppressed — is not specified and not visualized.
- **Current State:** `03-session-header-banners.md` variant H shows the banner panel as a centered overlay. `09-findings-panel.md` shows the findings panel occupying the detail pane. No mockup shows either panel coexisting with the other, nor is there any prose in the visual spec stating they are mutually exclusive. The visual spec's D8 analysis covers the `DialogFindingsPanel` + help-modal coexistence as the "only legal coexistence" — but the banner panel is rendered as a dialog-shaped overlay (variant H uses `DialogNone`-like conventions), making it ambiguous whether it is a `DialogKind` or not.
- **Desired State:** The behavioral spec's D49 commits to the `[N more warnings]` affordance opening "a banner panel listing all active banners." The behavioral spec does not explicitly address this coexistence. The visual spec declares the findings panel as the only dialog over which the help modal may open, but does not address whether the banner panel is a dialog or a separate overlay layer.

---

**GAP-015: Detail-pane focus on `+ Add` affordance row — detail pane content during this focus is unspecified**

- **Category:** Implicit
- **Feature/Behavior:** When the outline cursor is on a `+ Add step` row (or `+ Add env variable`, etc.), the status footer changes to `Enter add`. The behavioral spec states the detail pane shows "a section summary when a section header is selected." It does not state what appears in the detail pane when the `+ Add` affordance row is focused (as opposed to the section header row).
- **Current State:** `04-outline-panel.md` variant D shows the focused `+ Add step` row with the footer showing `Enter add`. No mockup shows what appears in the right (detail) pane when this row is focused. `05-detail-pane-fields.md` variant P shows the section-summary view when the *section header* is focused, and includes a `[+ Add step]` action there — but not when the outline's own `+ Add` row is focused.
- **Desired State:** The visual spec's feature-specification.md states: "The detail pane is a vertically scrolling area showing the fields of the currently selected outline item, or — when an outline section header is selected — a summary of the section's contents." The `+ Add` row is neither a section header nor an item. What the detail pane shows when a `+ Add` row is focused is unspecified in both the visual spec and the behavioral spec. This silence creates an implicit gap.

---

**GAP-016: `DialogNewChoice` interaction path when `--workflow-dir` auto-open is active — no mockup for the overlap**

- **Category:** Implicit
- **Feature/Behavior:** The behavioral spec states: "If the user passed `--workflow-dir` on the command line, the builder auto-opens that file via the File > Open code path (step 4). Otherwise the builder enters an empty-editor state." The `DialogNewChoice` mockup (`10-dialog-new-choice.md`) shows the dialog appearing "after the user invokes File > New." It does not address whether `--workflow-dir` auto-open and a subsequently invoked File > New (with unsaved changes) produces the same dialog with the same visual variants, or whether the session-header path shown in the unsaved-changes variant is always correct.
- **Current State:** `12-dialog-unsaved-changes.md` shows the dialog with a target path in the body text. When `--workflow-dir` is specified and auto-opens a file, then the user invokes File > New, the unsaved-changes dialog appears. The target path shown in `12-dialog-unsaved-changes.md` variant is `~/projects/foo/.pr9k/workflow/config.json` — a manually constructed example. Whether the `--workflow-dir` path (which may be longer or unconventional) renders correctly in the dialog body is implicitly assumed but not visualized.
- **Desired State:** The behavioral spec's Primary Flow step 1 states `--workflow-dir` "additionally acts as an explicit 'open this file at launch' directive." This creates a launch path that differs from the standard empty-editor path. No mockup addresses the full-frame appearance at launch with `--workflow-dir` specified (the top-border title format, session header, and initial cursor position in this case are all governed by the same rules but not shown).

---

## Areas Needing Separate Analysis

1. **Interactive keyboard-contract testing for all 28 modes** — the mode-coverage table is a behavioral test specification; verifying whether each observable change is correctly produced requires exercising the actual `workflowedit.Model.Update` code paths, not just inspecting static mockups. This is a testing-gap analysis, not a visual-gap analysis, and is out of scope for this document.

2. **Color rendering fidelity across terminal emulators** — the visual spec commits to ANSI color codes (245 for LightGray, 15 for White, 10 for Green, etc.) and notes that "backgrounds remain default (terminal background) except where reverse-video is explicitly named." Whether specific terminal emulators render these codes as intended — particularly the `Color("8")` dim treatment of the findings panel under the help modal — requires runtime verification against the target terminal set, which is not derivable from the mockup markdown files.

3. **Outline and detail pane scroll interaction during reorder** — `07-reorder-mode.md` variant F shows the outline scroll indicator shifting during reorder. Whether the detail pane also scrolls (or is replaced by empty content) when the moving step's field view would require the detail pane to update is not addressed by any mockup. This interaction is at the boundary of visual and behavioral concerns and warrants focused analysis if implementation reveals ambiguity.
