# Decision Log: Workflow-Builder TUI Visual Design

Each decision below addresses one visual question. Behavioral questions are owned by `../../workflow-builder/feature-specification.md`; this log is concerned with **what the user sees**, not what the builder does.

All decisions in this log were settled from evidence (the existing `internal/ui` rendering code at `src/internal/ui/model.go`, the behavioral spec's commitments, and the impl-decision log at `../../workflow-builder/artifacts/implementation-decision-log.md`) without requiring user input — auto-mode was active.

---

## D1: Chrome and palette reuse from run-mode TUI

- **Question:** Does the workflow-builder render its frame with the same chrome characters and color palette as the existing `pr9k` run-mode TUI, or define its own?
- **Decision:** Reuse the existing chrome and palette unchanged: glyphs `╭`, `╮`, `╰`, `╯`, `│`, `─`, `├`, `┤`; colors `LightGray = 245`, `White = 15`, `Green = 10`, `ActiveStepFG = 15`, `ActiveMarkerFG = 10`. The builder imports from `internal/ui` rather than re-declaring its own constants.
- **Rationale:** Two TUIs from the same product should look like they belong to the same product. Diverging the palette would create the impression that the builder is a separate tool. The run-mode TUI's chrome is also already tested (lipgloss-rendered via `wrapLine`, `renderTopBorder`, hrule construction in `model.go:382-749`) — reusing it transfers those guarantees instead of recreating them.
- **Evidence:** `src/internal/ui/header.go:21-41` (palette constants); `src/internal/ui/model.go:382-749` (View, wrapLine, renderTopBorder, renderHelpModal). Behavioral spec User Interactions section names the same chrome conventions implicitly via "matches the existing TUI's overlay behavior" (D48 dialog conventions row "Dialogs re-layout on terminal resize").
- **Rejected alternatives:**
  - Builder-specific palette — diverges visual identity, doubles the maintenance surface.
  - Builder-specific chrome glyphs (sharper corners, double-line borders) — visual mismatch on adjacent terminal sessions.
- **Driven by findings:** —
- **Linked technical notes:** —
- **Referenced in spec:** Layout — the persistent frame

---

## D2: Row order of the persistent frame

- **Question:** What rows does the workflow-builder frame contain, and in what order, regardless of the current mode?
- **Decision:** Top-to-bottom: top border, menu bar, hrule, session header, hrule, pane area (multi-row), hrule, status footer, bottom border. Nine fixed-content rows of chrome plus the variable-height pane area.
- **Rationale:** The behavioral spec's D65 commits to "menu bar row (row 1), separator (row 2), session header (row 3), separator (row 4), outline + detail pane (rows 5..N-1), shortcut footer (row N)." This visual-design layer makes the borders explicit (top + bottom) and adds the third hrule below the pane area to match the run-mode chrome's "hrule above the footer" pattern (`model.go:469`).
- **Evidence:** Behavioral spec D65; `internal/ui/model.go:332-352` (chrome row budget calculation: top border + grid rows + hrule + log + hrule + footer + bottom border).
- **Rejected alternatives:**
  - Drop the third hrule — visually inconsistent with run-mode TUI (which has it).
  - Move the menu bar below the session header — buries the always-available File operations under situation-dependent banners.
- **Driven by findings:** —
- **Linked technical notes:** —
- **Referenced in spec:** Layout — the persistent frame

---

## D3: Top-border title format and truncation rule

- **Question:** What text appears in the top-border title slot, and how is it shortened when the terminal is narrow?
- **Decision:** Format: `Power-Ralph.9000 — Workflow Builder` when no workflow is loaded, `Power-Ralph.9000 — Workflow Builder — <target-path>` when a workflow is loaded. Brand name renders green, separator and remainder render white (matching `colorTitle` in `internal/ui/model.go:699-707`). When the title overflows, the path is truncated **from the left** with a `…` prefix so the filename stays visible: `…/projects/foo/.pr9k/workflow/config.json` becomes `…/.pr9k/workflow/config.json` and ultimately `…/config.json`.
- **Rationale:** The run-mode title format is `<AppTitle> — <iteration line>` (model.go:649-655). The builder uses the same `<AppTitle> — <surface name> — <surface detail>` shape so both TUIs read as the same family. Left-truncation with `…` prefix is the standard for paths because the filename is the most-identifying segment.
- **Evidence:** `src/internal/ui/model.go:649-655` (titleString); `colorTitle` (model.go:699-707) splits on first ` — ` and applies brand color to the prefix.
- **Rejected alternatives:**
  - Right-truncate the path — loses the filename, the most identifying part.
  - Show only the filename, no parent directory — loses the disambiguation between identically-named files.
- **Driven by findings:** F5
- **Linked technical notes:** —
- **Referenced in spec:** Layout — the persistent frame; row 1

---

## D4: Menu bar row content and states

- **Question:** What occupies the menu bar row, and what visual states does it have?
- **Decision:** Three states: **closed** (the `File` label rendered with a one-letter mnemonic accent on the `F`, the rest of the row space-padded), **open** (the `File` label reversed, a bordered dropdown extends downward over the underlying frame), **item greyed** (within an open menu, an item that's unavailable in the current state renders in `LightGray` with no shortcut label). Only the `File` menu exists in v1; reserved horizontal space remains to the right for future menus.
- **Rationale:** The behavioral spec's D64-D68 commit to a persistent File menu visible in every mode. The three-state visual model matches every menu-bar pattern in terminal applications going back decades. Reserving space at right keeps the layout stable when future menus are added.
- **Evidence:** Behavioral spec D64, D65, D66, D67, D68; existing menu-bar precedents in widely-used TUIs (mc, ranger, htop) follow this exact pattern.
- **Rejected alternatives:**
  - Always-visible inline shortcuts (no dropdown) — uses excessive horizontal space; doesn't match D66's "click on File" interaction model.
  - Right-aligned menu bar — non-conventional; users habituated from decades of GUIs expect left-aligned File menu.
- **Driven by findings:** —
- **Linked technical notes:** —
- **Referenced in spec:** Menu bar — visual states

---

## D5: Session header row content and ordering

- **Question:** What slots does the session header row contain, and in what left-to-right order?
- **Decision:** Five slots: target path, unsaved-changes indicator, banner (priority-resolved), `[N more warnings]` affordance, findings summary (right-aligned).
- **Rationale:** The behavioral spec's Primary Flow step 5 commits to all five elements being in this row. Left-to-right ordering follows information-priority: identification (path) first, state (dirty) immediate, warnings prominent, action affordances last. Findings summary right-aligned follows the run-mode TUI's "right-pin metadata" convention (status-line footer's version label).
- **Evidence:** Behavioral spec Primary Flow §5 and User Interactions — Affordances; behavioral spec D49 (banner priority); behavioral spec D53 (post-save success feedback in the same slot).
- **Rejected alternatives:**
  - Banners on a separate row — adds permanent vertical chrome for an intermittent-content slot.
  - Findings summary inline with banners — conflates two different states.
- **Driven by findings:** —
- **Linked technical notes:** —
- **Referenced in spec:** Session header — visual content and state

---

## D6: Pane area vertical separator and junction glyphs

- **Question:** How are the outline pane and detail pane visually separated, and how do the hrules around the pane area join the separator?
- **Decision:** A `│` glyph in `LightGray` runs through every pane-area row at the column boundary between the two panes. The hrules above and below the pane area gain `┬` (top) and `┴` (bottom) glyphs at the separator's column so the rules-and-separator read as a continuous box-drawing grid.
- **Rationale:** Without `┬`/`┴` junctions the panes look like two stacked panels rather than two columns of one panel — a visual disconnect that makes the design feel improvised. The junctions cost no rendering performance (one character substitution per hrule) and standard TUIs (mc, ranger) use them. The separator-as-`│` matches the chrome's vertical-bar convention.
- **Evidence:** `internal/ui/model.go:407` shows the existing hrule construction (`├` + repeat `─` + `┤`); inserting one `┬` at the separator column extends that pattern. `lipgloss.Width` is multi-byte-aware, so the substitution does not break width math.
- **Rejected alternatives:**
  - Plain `─` through the hrules at the separator column — visual gap, looks unfinished.
  - Cross-junction `┼` only at separator-meets-rule — would imply the separator continues above and below the rule, which it does not.
  - No separator at all (empty column) — panes blend visually; users lose the column boundary.
- **Driven by findings:** F3
- **Linked technical notes:** —
- **Referenced in spec:** Layout — the persistent frame; row 6

---

## D7: Outline pane width rule

- **Question:** How wide is the outline pane?
- **Decision:** `min(40, max(20, ⌊terminalWidth × 0.4⌋))` columns, inclusive of the leading two-space indent and any cursor-prefix space. The right edge is the column where the `│` separator lives.
- **Rationale:** The impl-decision log D-12 explicitly commits to "fixed width of 40 columns for terminals ≥80 cols, proportional 40% for narrower terminals, minimum 20 cols." This visual-design layer reproduces that contract verbatim so render math matches the model's expectations.
- **Evidence:** `../../workflow-builder/artifacts/implementation-decision-log.md` D-12.
- **Rejected alternatives:** None — D-12 is committed.
- **Driven by findings:** —
- **Linked technical notes:** —
- **Referenced in spec:** Layout — the persistent frame; row 6

---

## D8: Status footer row content and ordering

- **Question:** What renders in the status footer, in what order?
- **Decision:** Left-aligned: the focused widget's `ShortcutLine() string` rendered with the run-mode `colorShortcutLine` two-tone (white keys, gray descriptions). Right-aligned: the `pr9k` version label in `White`. Separator: spaces enough to fill the row.
- **Rationale:** Behavioral impl-decision D-11 commits to widget-owned shortcut content; D-15 commits to a `workflow-` log prefix mirroring the run-mode `ralph-` prefix; the version label's right-align matches `internal/ui/model.go:480-520`. The two-tone rendering rule is reused from `colorShortcutLine` (model.go:665-692).
- **Evidence:** `internal/ui/model.go:480-520` (footer rendering); behavioral impl-decision D-11.
- **Rejected alternatives:**
  - Left-align the version label — non-conventional, conflicts with run-mode rendering.
  - Drop the version label — inconsistent with run-mode TUI.
- **Driven by findings:** —
- **Linked technical notes:** —
- **Referenced in spec:** Layout — the persistent frame; row 8

---

## D9: Chrome row budget and pane area vertical residual

- **Question:** How many rows does the chrome consume, and how is the remainder allocated to the pane area?
- **Decision:** Chrome consumes **8 rows** (5 fixed-content rows: top border, menu bar, session header, status footer, bottom border; plus 3 horizontal rules: below menu bar, below session header, above status footer). The pane area absorbs `terminalHeight - 8` rows. Both panes share that height; each renders a sub-content scroll view as needed.
- **Rationale:** This is the same row-budget approach used for the run-mode TUI: a fixed chrome budget, residual to the central content area. Documenting the count here gives implementers a precise number to assert against in tests.
- **Evidence:** `internal/ui/model.go:332-352`.
- **Rejected alternatives:**
  - Fewer hrules — visual ambiguity between menu, session header, panes, footer.
  - Variable chrome rows (e.g., banner row added when banner active) — frame jumps when banner state changes; bad TUI ergonomics.
- **Driven by findings:** F2
- **Linked technical notes:** —
- **Referenced in spec:** Layout — the persistent frame

---

## D10: Menu bar mnemonic accent and hover feedback

- **Question:** How is the `Alt+F` mnemonic visually conveyed in the closed menu bar, and what does mouse hover do?
- **Decision:** The first letter of `File` (`F`) renders in `White` while the rest of the label renders in `LightGray`, indicating "this is the mnemonic key." On mouse hover (cell-motion), the entire `File` label gets a reverse-video background to signal it's clickable.
- **Rationale:** The mnemonic-letter underline is the cross-platform convention for menu shortcuts, and a brighter color is the closest equivalent in monochrome terminals where underline can conflict with active-row indicators. The hover-reverse pattern is what the run-mode TUI's overlay clickable region uses (`internal/ui/model.go:217-269`).
- **Evidence:** Behavioral D66 (Alt+F mnemonic). `internal/ui/log_panel.go` mouse-handling pattern.
- **Rejected alternatives:**
  - Underline the F — many terminals collapse underline with bold or with active-row treatment, producing visual noise.
  - No mnemonic accent — Alt+F users have no visual confirmation that the shortcut is wired.
- **Driven by findings:** —
- **Linked technical notes:** —
- **Referenced in spec:** Menu bar — visual states

---

## D11: Menu dropdown rendering and overlay anchoring

- **Question:** How is the open menu dropdown drawn, and where is it anchored?
- **Decision:** A bordered overlay (chrome glyphs `╭╮╰╯│─`) anchored to the `File` label's left edge, extending downward. Each item row is `│ Item                    Ctrl+X │` with the item left-aligned at column 2 of the dropdown, the shortcut right-aligned at column `(width - 2 - len(shortcut))`, and the row width sized to the longest combined `<item>  <shortcut>` plus 4 (2 for borders, 2 for inner padding). The dropdown overlays the underlying frame using the same `overlay()` splice helper the run-mode help modal uses.
- **Rationale:** Anchoring to the label's left edge (rather than centering or right-aligning) follows the cross-platform GUI convention; `Alt+F` is the canonical pattern for "File menu drops from File label." Same overlay machinery as the help modal because the dropdown is structurally identical: a bordered ANSI-styled string spliced over the frame.
- **Evidence:** `internal/ui/model.go:529-558` (overlay splice for help modal); `internal/ui/overlay.go` (the splice helper itself).
- **Rejected alternatives:**
  - Centered dropdown — ignores the click-on-label affordance.
  - Inline expansion (replace menu bar row with item list) — conflates two row purposes; user loses sense of where the items came from.
- **Driven by findings:** —
- **Linked technical notes:** —
- **Referenced in spec:** Menu bar — visual states

---

## D12: Greyed menu item rendering and disable rules

- **Question:** When is a menu item greyed, and how does that look?
- **Decision:** A menu item is greyed when its action is unavailable in the current state. For v1: `Save` is greyed when no workflow is loaded (empty-editor) or when the loaded workflow is in browse-only mode (read-only target per spec D30). Greyed items render their label in `LightGray` and **omit** the right-aligned shortcut label.
- **Rationale:** Behavioral spec D30 ("Read-only target — File > Save is greyed out in the menu") and the empty-editor description in Primary Flow §2 ("File > Save is greyed out") commit to this. Hiding the shortcut on greyed items is consistent with the convention "you can't use a shortcut that has no target."
- **Evidence:** Behavioral spec Primary Flow §2; behavioral spec D30 (Alternate Flows — Read-only target).
- **Rejected alternatives:**
  - Show greyed items in the same color as enabled items — fails the visibility heuristic.
  - Hide greyed items entirely — menu height jumps; users can't anticipate where items are.
- **Driven by findings:** —
- **Linked technical notes:** —
- **Referenced in spec:** Menu bar — visual states

---

## D13: Unsaved-changes indicator glyph and color

- **Question:** What glyph and color represent the unsaved-changes state in the session header?
- **Decision:** A `●` (U+25CF, BLACK CIRCLE) glyph rendered in `Green`, immediately after the target path with one space of padding. Hidden when the in-memory state is clean.
- **Rationale:** `●` is the universal "modified" indicator across editors (VS Code, IntelliJ, Sublime). Green is one of the existing palette colors and reads as "noticeable but non-alarming" — it signals state, not error. The single-glyph form keeps the session header dense.
- **Evidence:** Cross-editor convention. Behavioral spec User Interactions — Feedback ("the unsaved-changes indicator in the session header clears" — D53).
- **Rejected alternatives:**
  - `*` asterisk — looks like a footnote marker, not a state indicator.
  - Text label "(modified)" — consumes too much width.
  - Red glyph — implies error; modification is not an error.
- **Driven by findings:** —
- **Linked technical notes:** —
- **Referenced in spec:** Session header — visual content and state

---

## D14: Banner prefix glyphs and severity coloring

- **Question:** How are the five banner types visually distinguished?
- **Decision:** Each banner type gets a short bracket-prefix tag and a foreground color: `[ro]` red, `[ext]` yellow, `[sym]` yellow, `[shared]` yellow, `[?fields]` cyan. Two transient banners use `Saved at HH:MM:SS` green and `No changes to save` `LightGray`.
- **Rationale:** Five overlapping warning categories need at-a-glance differentiation. Color alone fails for color-blind users (and the behavioral spec D25 explicitly bans severity-by-color-only). The bracket-prefix tag is the textual disambiguator. The colors layer on for users who can perceive them. Yellow groups three same-severity banners (external, symlink, shared-install) because they share the same actionable level. Red is reserved for read-only as it most-restricts the user's actions. Cyan is informational (unknown-field warns about a future drop).
- **Evidence:** Behavioral spec D49 (banner priority order). Behavioral spec D25 (severity text prefixes — color is never sole signifier).
- **Rejected alternatives:**
  - All warnings yellow — read-only is more restrictive; color compression hides that.
  - All warnings same color, only icons differ — misses the secondary severity distinction.
- **Driven by findings:** —
- **Linked technical notes:** —
- **Referenced in spec:** Session header — visual content and state

---

## D15: Multi-banner affordance rendering and activation

- **Question:** When more than one banner is active, how does the user see and reach the suppressed banners?
- **Decision:** A `[N more warnings]` text affordance renders immediately after the displayed banner, in `White`. Activating it (Enter when focused, click) opens a small panel listing all active banners; the panel is drawn with the same dialog conventions ([D36](#d36-dialog-centered-overlay-shape-and-borders)) and dismissed with Escape.
- **Rationale:** Behavioral spec D49 commits to "`[N more warnings]` affordance opens a banner panel listing all active banners." Rendering the affordance as `White` (not gray) makes it look interactive rather than chrome.
- **Evidence:** Behavioral spec D49.
- **Rejected alternatives:**
  - Render all banners stacked — consumes vertical space; spec D49 explicitly rejected this.
  - Cycle banners with timed rotation — distracting; users can't read the same banner twice.
- **Driven by findings:** F6
- **Linked technical notes:** —
- **Referenced in spec:** Session header — visual content and state

---

## D16: Findings summary position and format

- **Question:** Where does the validator findings summary render, and in what format?
- **Decision:** Right-aligned at the right edge of the session header row. Format: `<F> fatal · <W> warn · <I> info` for non-zero counts only. When all counts are zero, the slot is empty (no `0 fatal · 0 warn · 0 info` literal). Activating it opens the findings panel.
- **Rationale:** Right-aligned because it's metadata/status, matching the version-label position in the run-mode footer. Format is the same as the findings-panel header to make the relationship obvious. Empty-when-zero avoids visual noise (most workflows in good state have zero findings).
- **Evidence:** Behavioral spec User Interactions — Feedback (findings panel header). `internal/ui/model.go:480-520` (right-aligned metadata convention).
- **Rejected alternatives:**
  - Always render `0 fatal · 0 warn · 0 info` — visual noise; users learn to ignore it.
  - Show only fatal count — hides warning/info presence.
  - Left-align after the banner — buries it under variable-width content.
- **Driven by findings:** F7
- **Linked technical notes:** —
- **Referenced in spec:** Session header — visual content and state

---

## D17: Session header overflow priority

- **Question:** When the session header content exceeds the row width, which slots disappear in what order?
- **Decision:** Drop in order: (1) `[N more warnings]` affordance, (2) findings summary, (3) banner truncated with `…`, (4) target path truncated more aggressively. The unsaved-changes indicator and at-most-one-banner slot are last to go; the dirty indicator is never dropped while dirty.
- **Rationale:** State that the user must respond to (banner content, dirty indicator) is most-important. Affordances (`[N more]`, findings summary) can be reached from other entry points (the help modal lists them). Path truncates last because losing path identification is the most disorienting.
- **Evidence:** Information-priority reasoning grounded in Nielsen heuristic 1 (system status visibility).
- **Rejected alternatives:**
  - Drop dirty indicator first — users could lose track of unsaved work.
  - Drop banner first — silently hides actionable warnings.
- **Driven by findings:** —
- **Linked technical notes:** —
- **Referenced in spec:** Session header — visual content and state

---

## D18: Outline section order and grouping

- **Question:** In what order do the six outline sections appear?
- **Decision:** `env`, `containerEnv`, `statusLine`, `Initialize`, `Iteration`, `Finalize`.
- **Rationale:** Behavioral spec Primary Flow §5 names all six and orders them this way (top-level env passthrough, top-level container-environment list, the statusLine block, then the three ordered phase sections). This visual layer follows the spec's order.
- **Evidence:** Behavioral spec Primary Flow §5; behavioral D51 (section summary content).
- **Rejected alternatives:**
  - Alphabetical — ignores phase semantics.
  - Phases first, then env — buries the env top-level concern below frequently-edited phases.
- **Driven by findings:** —
- **Linked technical notes:** —
- **Referenced in spec:** Outline pane — visual structure

---

## D19: statusLine section conditional body rule

- **Question:** What does the `statusLine` section's body look like when no statusLine is configured vs. configured?
- **Decision:** Configured: a single item row representing the statusLine block, with the same row format as a step row but with a different kind glyph (`[≣]` for statusLine vs `[≡]` for Claude steps). Absent: a single `+ Add statusLine block` affordance row.
- **Rationale:** The statusLine is a single object, not a list, so it never has more than one item. The `+ Add` row when absent matches the impl-decision D-29 commitment ("`+ Add statusLine block` row").
- **Evidence:** Behavioral impl-decision D-29; behavioral spec D29 (statusLine add affordance).
- **Rejected alternatives:**
  - Inline statusLine fields directly under section header (no item row) — inconsistent with phase-section pattern.
- **Driven by findings:** —
- **Linked technical notes:** —
- **Referenced in spec:** Outline pane — visual structure

---

## D20: Section-header row format and collapse glyph

- **Question:** How is a section header rendered, and how does the collapse state appear?
- **Decision:** Format `<glyph> <Section name>  (<count>)` where glyph is `▾` (expanded) or `▸` (collapsed). Section name in `LightGray`, count in `LightGray` parens. Always-visible count satisfies behavioral D28.
- **Rationale:** `▾`/`▸` is the cross-platform convention for expand/collapse triangles. The trailing `(N)` count is an explicit spec D28 commitment ("always-visible item count").
- **Evidence:** Behavioral spec D28; impl-decision D-12.
- **Rejected alternatives:**
  - `[+]`/`[-]` brackets — non-standard for tree views.
  - Count hidden when collapsed — defeats the always-visible requirement.
- **Driven by findings:** —
- **Linked technical notes:** —
- **Referenced in spec:** Outline pane — visual structure

---

## D21: Step item row format and glyphs

- **Question:** What does an outline step row look like at rest?
- **Decision:** `  ⋮⋮ [<kind-glyph>] <step-name>  <model-or-dash>  [F<n>]?` — gripper `⋮⋮` in `LightGray`, kind glyph `[≡]` (Claude) or `[$]` (shell), step name left-justified, model name right-justified at a fixed column (or `—` for shell), optional fatal-finding indicator `[F<n>]` in `Red`.
- **Rationale:** The gripper makes the row's draggability visible at all times (behavioral D34's "persistent gripper glyph" commitment). Kind glyph distinguishes Claude steps (which carry a model) from shell steps (which carry a command). The right-aligned model column lets the user scan kinds and models down the column.
- **Evidence:** Behavioral spec D34; impl-decision D-35 (`GripperGlyph = "⋮⋮"`).
- **Rejected alternatives:**
  - No kind glyph — users can't tell Claude vs shell at a glance.
  - Inline (claude) text label — verbose.
- **Driven by findings:** —
- **Linked technical notes:** —
- **Referenced in spec:** Outline pane — visual structure

---

## D22: Cursor row rendering in outline

- **Question:** How is the focused row visually distinguished?
- **Decision:** The leading two-space indent is replaced with `> ` and the entire row's text is rendered in `White` instead of `LightGray`.
- **Rationale:** This is the same treatment the run-mode header applies to the active step row (`internal/ui/header.go:215-220`'s `cellStyle` switches to `ActiveStepFG = White` for the active step). Consistency keeps the active-row signal recognizable across both TUIs.
- **Evidence:** `src/internal/ui/header.go:215-220, 242-256`.
- **Rejected alternatives:**
  - Reverse video on the entire row — clashes with reorder-mode reverse-video.
  - Background bar — many terminals don't render background colors well in non-dark themes.
- **Driven by findings:** F1
- **Linked technical notes:** —
- **Referenced in spec:** Outline pane — visual structure

---

## D23: Add affordance row format

- **Question:** How does a `+ Add` row look?
- **Decision:** Format `  + Add <item-type>` where item-type is `step`, `env variable`, `container env entry`, or `statusLine block`. Rendered in `LightGray` when unfocused; in `White` and prefix-replaced with `> ` when focused.
- **Rationale:** Behavioral D46 commits to "+ Add <item-type>" and the impl-decision D-35 declares the constants. The same focus-state transition as a step row keeps focus rendering consistent across row types.
- **Evidence:** Behavioral D46; impl-decision D-35 (`AddRowPrefix = "+ Add "`).
- **Rejected alternatives:**
  - Distinct color (e.g., green) — would fight the focus-state coloring.
  - Italic — terminals don't reliably render italic.
- **Driven by findings:** —
- **Linked technical notes:** —
- **Referenced in spec:** Outline pane — visual structure

---

## D24: Empty-section state rendering

- **Question:** What does an empty section render?
- **Decision:** Section header row, then a non-focusable `(empty — no items)` row in `LightGray`, then the focusable `+ Add <item-type>` row.
- **Rationale:** Behavioral spec Edge Cases table row "Empty section (zero items)" commits to "section header is still focusable and collapsible; the detail pane shows a 'no items configured' state alongside the section's `+ Add ...` affordance." This visual layer keeps the same content in the outline so users see the empty state from the outline without having to focus the section to see it in the detail pane.
- **Evidence:** Behavioral spec Edge Cases table; behavioral D51 (section summary content for empty sections).
- **Rejected alternatives:**
  - Show only the `+ Add` row when empty (no `(empty — no items)` line) — less explicit; first-time users might miss that the section is empty rather than collapsed.
- **Driven by findings:** —
- **Linked technical notes:** —
- **Referenced in spec:** Outline pane — visual structure

---

## D25: Outline scroll indicator rendering

- **Question:** How does the outline indicate that more content exists above or below the visible area?
- **Decision:** A single-column scroll-position glyph runs down the rightmost column of the outline pane: `▲` at the top half if scrolled past the top, `█` for the visible-region indicator, `▼` at the bottom if more content exists below; remaining cells in the column render as the chrome `│` glyph in `LightGray`.
- **Rationale:** Behavioral spec D29 commits to "outline is independently scrollable with a visible scroll-position indicator when it exceeds the viewport height." A 1-column scroll bar satisfies this without consuming meaningful horizontal space.
- **Evidence:** Behavioral D29; standard TUI scroll-indicator pattern.
- **Rejected alternatives:**
  - Percentage text in the section header (e.g., `(1-10/40)`) — splits attention.
  - No indicator (rely on cursor position) — fails the visibility commitment.
- **Driven by findings:** —
- **Linked technical notes:** —
- **Referenced in spec:** Outline pane — visual structure

---

## D26: Detail pane field rendering grammar

- **Question:** What is the row-level visual grammar for a field in the detail pane?
- **Decision:** Each field has a row sequence: optional context heading, label-plus-input row, optional below-input warning row, optional below-input action row. The label-plus-input row is the canonical surface; the others appear only when their content does. Labels left-align; inputs are bracketed `[...]` with the input value inside; right-aligned hints follow the input on the same row when they fit, or wrap to the next row if not.
- **Rationale:** Consistent grammar across all six field kinds (text, choice, numeric, secret-mask, model-suggest, multi-line) lets users learn one pattern. Bracketed input boxes are the standard TUI input visualization (curses, dialog, whiptail all use this).
- **Evidence:** Behavioral spec Primary Flow step 7 (every field-kind subsection); cross-TUI convention.
- **Rejected alternatives:**
  - Underlined input area instead of bracket box — terminals render underline inconsistently.
  - Separate label row above input row — doubles vertical space.
- **Driven by findings:** —
- **Linked technical notes:** —
- **Referenced in spec:** Detail pane — field rendering

---

## D27: Plain-text field rendering

- **Question:** What does a plain-text field look like in detail-pane render?
- **Decision:** `  Label: [<value>          ]    <hint>` — label and bracket border in `LightGray`, value in `White`, hint right-aligned in `LightGray`. Cursor `▏` in `White` at the cursor position when focused. Sanitization warning row `↳ pasted content sanitized` below the input when it fired.
- **Rationale:** Behavioral D42 commits to input-time sanitization with a "visible warning when exceeded"; the below-input row is the lowest-friction warning surface (doesn't require an overlay). Right-aligned hint is the standard form for input constraints.
- **Evidence:** Behavioral D42; behavioral User Interactions — Feedback.
- **Rejected alternatives:**
  - Hint above input — doubles vertical space.
  - Hint inside input box (placeholder) — disappears on first keystroke.
- **Driven by findings:** —
- **Linked technical notes:** —
- **Referenced in spec:** Detail pane — field rendering

---

## D28: Choice list closed and open rendering

- **Question:** How is a choice-list field rendered in its two states?
- **Decision:** Closed: `  Label: [<current> ▾]` — `▾` always present (even unfocused) per behavioral D27 unfocused-signifier rule. Open: a small bordered dropdown anchored below the input row, listing every option, with the highlighted option in reverse video.
- **Rationale:** Behavioral D12 + D27 + D45. The `▾` distinguishes choice-list fields from text inputs at unfocused state, satisfying D27. The open-dropdown shape mirrors the menu dropdown shape ([D11](#d11-menu-dropdown-rendering-and-overlay-anchoring)).
- **Evidence:** Behavioral D12, D27, D45; impl-decision D-35 (`ChoiceListIndicator = "▾"`).
- **Rejected alternatives:**
  - `[v]` ASCII pseudo-arrow — less conventional, harder to read.
  - Dropdown to the right of the input — wastes horizontal space.
- **Driven by findings:** —
- **Linked technical notes:** —
- **Referenced in spec:** Detail pane — field rendering

---

## D29: Numeric field rendering

- **Question:** How is a numeric field visually distinguished from a plain-text field?
- **Decision:** Same bracket-input shape as plain text, but the input is right-padded with spaces so digits read right-aligned within the bracket; a units suffix (e.g., `seconds`) appears in `LightGray` after the closing bracket; the right-aligned hint is the valid range or special-value semantics (e.g., `0 disables refresh`).
- **Rationale:** Right-aligning digits matches the convention from financial/numerical inputs; the units suffix prevents the user from typing `60s` (since the unit is implicit). The "0 disables" hint is a behavioral D29 commitment for the statusLine refresh-interval field.
- **Evidence:** Behavioral D29; behavioral D62 (numeric-only input handling).
- **Rejected alternatives:**
  - Distinct color for numeric inputs — adds palette complexity for marginal value.
  - Left-aligned digits — misalignment when comparing two numbers.
- **Driven by findings:** —
- **Linked technical notes:** —
- **Referenced in spec:** Detail pane — field rendering

---

## D30: Secret-mask field rendering and toggle states

- **Question:** How is a secret-mask field rendered, and what visual signal accompanies the reveal toggle?
- **Decision:** Default: `  KEY: [••••••••       ]    r to reveal`. Revealed: `  KEY: [<value>         ]    r to mask`. Re-mask on focus-leave (visible: the value snaps back to dots and the hint reverts to "r to reveal").
- **Rationale:** Behavioral D20 + D47. A visible hint communicating the `r` key must appear per behavioral spec D47. The hint uses no bracket delimiters so it does not look like a clickable affordance (bracket syntax is reserved for interactive handles such as `[Ctrl+E open in editor]` and `[ Cancel ]`).
- **Evidence:** Behavioral D20, D47; impl-decision D-35 (`SecretRevealHint`).
- **Rejected alternatives:**
  - No hint label (silent toggle) — D20 explicitly requires the label.
  - Toggle with `Tab` — conflicts with field navigation.
- **Driven by findings:** F8
- **Linked technical notes:** —
- **Referenced in spec:** Detail pane — field rendering

---

## D31: Model-suggestion field rendering and dropdown

- **Question:** How is the model field — free-text-with-suggestions — rendered?
- **Decision:** Closed: `  Model: [<typed-or-default> ▾]` — same `▾` indicator as choice list. Open: dropdown lists suggestions; typing in the input filters the dropdown; Escape commits the typed value (not necessarily a suggestion).
- **Rationale:** Behavioral D12 ("free-text input with a suggestion list") + D58 ("the field accepts any value regardless"). Visual sameness with choice-list keeps users from learning two dropdown patterns.
- **Evidence:** Behavioral D12, D58; impl-decision D-42.
- **Rejected alternatives:**
  - No `▾` (look like plain text) — hides the suggestion-list affordance.
  - Distinct dropdown shape — multiplies UI patterns without payoff.
- **Driven by findings:** —
- **Linked technical notes:** —
- **Referenced in spec:** Detail pane — field rendering

---

## D32: Multi-line field rendering and edit handle

- **Question:** What does a prompt-file or script-path field look like in the detail pane?
- **Decision:** Three rows: (1) the path on the first row in `White` (or `LightGray` with a `not found` marker if the file does not exist); (2) size and mtime metadata in `LightGray` when the file exists; (3) action row `[Ctrl+E open in editor]` in `White`. Footer additionally displays `Ctrl+E open in editor` when this field is focused.
- **Rationale:** Behavioral spec ("Multi-line content … is edited by handing control of the terminal to the user's configured external editor"). The metadata row gives the user context (size, mtime) without opening the editor. The triple-rendering (action both inline and in footer) satisfies discoverability.
- **Evidence:** Behavioral spec Primary Flow step 7; behavioral User Interactions — Affordances.
- **Rejected alternatives:**
  - Inline content preview (first N bytes) — incentivizes editing without opening; preview can be stale.
  - Action only in footer — users staring at the field can't see how to edit it.
- **Driven by findings:** —
- **Linked technical notes:** —
- **Referenced in spec:** Detail pane — field rendering

---

## D33: Section-summary rendering per section type

- **Question:** What does the detail pane show when an outline section header is focused?
- **Decision:** A header line `<Section name>  ·  <count> <unit>`, then a numbered list of up to 8 items per behavioral D51, then `+ Add <item-type>` action.
- **Rationale:** Behavioral D51 enumerates the content per section type. This visual layer commits to the rendering shape (numbered list, header line, `+ Add` action).
- **Evidence:** Behavioral D51.
- **Rejected alternatives:**
  - Tabular rendering with kind columns — overkill for short summaries.
  - Just the count, no list — D51 commits to the list.
- **Driven by findings:** —
- **Linked technical notes:** —
- **Referenced in spec:** Detail pane — field rendering

---

## D34: Status footer shortcut vs prompt rendering rule

- **Question:** When does the footer render as a shortcut bar vs. a single prompt sentence?
- **Decision:** Default: shortcut-bar two-tone (white keys, gray descriptions). When the footer represents a prompt — a question awaiting yes/no input — the entire string renders in `White`. Examples of prompt mode: `Quit the workflow builder? (y/n, esc to cancel)`. The run-mode TUI's `colorShortcutLine` rule already supports this branch via the `QuitConfirmPrompt` constant.
- **Rationale:** Behavioral spec D73 ("Quit the workflow builder? (Yes / No)") is structured as a prompt; rendering the whole string in `White` distinguishes it from a shortcut bar visually.
- **Evidence:** `src/internal/ui/model.go:665-692` (`colorShortcutLine` branch on `QuitConfirmPrompt`).
- **Rejected alternatives:**
  - Shortcut-bar coloring on prompts — keys/descriptions do not parse for "Quit the workflow builder?".
- **Driven by findings:** —
- **Linked technical notes:** —
- **Referenced in spec:** Status footer — content per mode

---

## D35: Version label position and color

- **Question:** Where does the version label render?
- **Decision:** Right-aligned at the right edge of the status footer row, in `White`. Identical to run-mode footer's right edge (`internal/ui/model.go:480-520`).
- **Rationale:** Reuses the run-mode rendering rule verbatim.
- **Evidence:** `src/internal/ui/model.go:480-520`.
- **Rejected alternatives:** None — identical to run-mode.
- **Driven by findings:** —
- **Linked technical notes:** —
- **Referenced in spec:** Status footer — content per mode

---

## D36: Dialog centered overlay shape and borders

- **Question:** What is the canonical visual shape of every dialog?
- **Decision:** Centered horizontally and vertically over the underlying frame; width `min(terminalWidth - 4, 72)` columns, minimum 30, height grown to content; bordered with `╭`, `╮`, `╰`, `╯`, `│`, `─`; title segment in `White` in the top border (`╭─ Title text ─…─╮`); option footer right-aligned in the bottom border in `White`; blank rows between top border and body, and body and option footer; 2-column inner indent.
- **Rationale:** Mirrors the run-mode help modal's visual shape (`internal/ui/model.go:567-643`) — same overlay machinery, same chrome glyphs, same width budgets. One visual pattern across all dialogs reduces cognitive load.
- **Evidence:** `internal/ui/model.go:567-643` (run-mode help modal `renderHelpModal`).
- **Rejected alternatives:**
  - Per-dialog custom shapes — multiplies maintenance and visual noise.
  - Inline (non-overlay) dialogs — conflict with focus-restoration rule (impl-decision D-8 `prevFocus`).
- **Driven by findings:** F13
- **Linked technical notes:** —
- **Referenced in spec:** Dialogs — common visual conventions

---

## D37: Dialog keyboard-default rendering

- **Question:** How is the keyboard-default option visually distinguished in a dialog footer?
- **Decision:** The default option is wrapped in `[ ]` brackets in `Green` (e.g., `[ Cancel ]`); other options render in `White` without brackets. Behavioral spec D48 commits to "the safe option is the keyboard default" and D7/D73/path-picker dialogs all confirm Cancel is default for safety.
- **Rationale:** The square-bracket-around-default convention is universal across terminal dialog frameworks (whiptail, dialog, ncurses-based installers). Green resonates with the "safe" framing without conflicting with severity semantics on banners (read-only red, etc.).
- **Evidence:** Behavioral spec D48 (Dialog Conventions); whiptail/dialog precedent.
- **Rejected alternatives:**
  - Reverse video on default — conflicts with focused-row reverse video.
  - Just `*` prefix on default — less visually distinct.
- **Driven by findings:** —
- **Linked technical notes:** —
- **Referenced in spec:** Dialogs — common visual conventions

---

## D38: Findings panel occupies detail pane only, not full screen

- **Question:** Does the findings panel replace the entire pane area or only the detail pane?
- **Decision:** Only the detail pane. The outline remains visible (and keeps its scroll position and focus state) so the user can navigate to the offending step from the outline if they prefer not to use the panel's `↩ jump to field` action.
- **Rationale:** Behavioral spec D55 (focus restoration after dismiss) requires that the user's prior outline focus be preserved across the panel session — easiest if the outline is still on screen. Hiding the outline forces a context switch on dismissal that the spec explicitly avoids.
- **Evidence:** Behavioral D55; impl-decision D-31 (findings panel scrollability — implies it lives in the detail pane region).
- **Rejected alternatives:**
  - Findings panel replaces full pane area — loses outline context.
  - Findings panel as a small overlay in a corner — too small for multi-finding lists; D31 commits to "independently scrollable" implying meaningful real estate.
- **Driven by findings:** —
- **Linked technical notes:** —
- **Referenced in spec:** Findings panel — visual layout

---

## D39: Acknowledged-warning visual treatment

- **Question:** How is an acknowledged warning rendered in the findings panel?
- **Decision:** Severity prefix becomes `[WARN ✓]` and the problem-row text renders with strikethrough; the entry remains visible in the panel (per behavioral D23 — ack only suppresses the dialog, not the panel itself).
- **Rationale:** Strikethrough on the problem row signals "user has seen and accepted this," consistent with cross-platform ack patterns (GitHub mark-as-resolved, email mark-as-read). The check glyph in the prefix gives the same signal in the structured prefix slot for users on terminals where strikethrough renders poorly.
- **Evidence:** Behavioral D23.
- **Rejected alternatives:**
  - Hide acknowledged entries — D23 explicitly says they "continue to appear in the findings panel."
  - Color-only signal — fails the severity-text-prefix-not-color-only rule (D25).
- **Driven by findings:** —
- **Linked technical notes:** —
- **Referenced in spec:** Findings panel — visual layout

---

## D40: Help modal mirrors run-mode help modal shape

- **Question:** Does the workflow-builder help modal use the same overlay shape as the run-mode help modal?
- **Decision:** Yes, identical shape (centered, `min(terminalWidth - 4, 72)` width, chrome glyphs, title segment, two-column shortcut grid by mode label). Content varies — workflow-builder modes replace run-mode modes — but rendering rules are identical.
- **Rationale:** One help-modal shape across the product. Implementation can lift `renderHelpModal` from `internal/ui/model.go` and parameterize the section content.
- **Evidence:** `internal/ui/model.go:567-643`.
- **Rejected alternatives:** None — identical-by-design is the explicit goal.
- **Driven by findings:** F10, F24
- **Linked technical notes:** —
- **Referenced in spec:** Help modal — visual layout

---

## D41: Reorder mode visual treatment

- **Question:** What changes visually when reorder mode is active?
- **Decision:** Moving step's gripper rendered in `Green`; the row gets reverse-video background; the status footer shows the reorder shortcuts. Phase-boundary crossing produces a one-frame flash on the boundary row in `Yellow`. The session header is unchanged — any priority banner (`[ro]`, `[ext]`, `[sym]`, `[shared]`, `[?fields]`) remains visible.
- **Rationale:** The active visual signal must be unmistakable to prevent confusion ("am I editing or reordering?"). Three independent signals communicate reorder mode: reverse-video on the moving row, green gripper, and footer change. The session header is left unchanged so higher-priority banners (`[ro]`, `[ext]`) are not suppressed by a transient mode signal — a visibility violation per Nielsen heuristic 1. The phase-boundary flash satisfies D34's "visibly drops it at the phase's edge" commitment.
- **Evidence:** Behavioral D34.
- **Rejected alternatives:**
  - Single signal (just footer change) — easy to miss.
  - Continuous flashing during reorder — distracting.
- **Driven by findings:** F9, F25
- **Linked technical notes:** —
- **Referenced in spec:** Reorder mode — visual treatment

---

## D42: Path picker input line and completion behavior

- **Question:** How does the path picker present its input line and tab-completion?
- **Decision:** A single editable text input rendered in the same plain-text field shape ([D27](#d27-plain-text-field-rendering)) inside a centered dialog ([D36](#d36-dialog-centered-overlay-shape-and-borders)). Tab-completion: when multiple matches exist, a small inline match list appears below the hint row in `LightGray`; cycling on repeated Tab is indicated by a transient `(N/M)` counter; inline-warning row in `Yellow` for typed-path warnings (existing config.json, directory mismatch, path not found).
- **Rationale:** Behavioral D71 commits to "single labeled text input with filesystem tab-completion" with cycling on multiple matches. Inline-match-list is the lowest-friction completion-feedback surface.
- **Evidence:** Behavioral D71; impl-decision D-25 (custom minimal `pathcomplete` helper).
- **Rejected alternatives:**
  - Tree picker — D71 explicitly rejects this.
  - Single match auto-completes silently with no feedback — users wouldn't know whether their tab did anything.
- **Driven by findings:** F11
- **Linked technical notes:** —
- **Referenced in spec:** Path picker — visual layout

---

## D43: Empty-editor view visual layout

- **Question:** What does the screen look like when no workflow is loaded?
- **Decision:** Outline pane: `No workflow open` centered horizontally, near the top, in `LightGray`. Detail pane: a small bordered hint panel (smaller than full dialog, no overlay — sits inside the detail-pane region) with two lines: `File > New (Ctrl+N) — create a workflow` and `File > Open (Ctrl+O) — open an existing config.json`. Session header: target-path slot reads `(no workflow open)` in `LightGray`. Save in menu greyed.
- **Rationale:** Behavioral spec Primary Flow §2 + D68 commit to all four elements. Rendering the hint as a small bordered box (rather than a full overlay) signals "this is your starting point, not a dialog blocking your work."
- **Evidence:** Behavioral spec Primary Flow §2; behavioral D68.
- **Rejected alternatives:**
  - Centered modal dialog at startup — re-introduces the landing page that D64 superseded.
  - Plain text without border — looks less inviting; users miss it as a hint.
- **Driven by findings:** —
- **Linked technical notes:** —
- **Referenced in spec:** Empty-editor view — visual layout

---

## D44: External-editor handoff two-step rendering

- **Question:** What does the user see during the external-editor handoff?
- **Decision:** Three frames: (1) pre-handoff frame with `DialogExternalEditorOpening` overlay reading `Opening editor…` plus a context line in `LightGray` naming the file and resolved binary; (2) editor frame — the editor owns the screen, builder chrome is gone; (3) post-handoff frame — chrome reappears, dialog gone, detail pane shows the just-edited field with any non-zero-exit warning briefly visible.
- **Rationale:** Impl-decision D-26 (two-cycle handoff) requires a render cycle for "Opening editor…" before terminal release. The post-handoff frame's brief warning, when applicable, surfaces the non-zero-exit signal without being a full dialog (D-7 commits to non-zero exits not being a fatal-stop signal — the file is still re-read).
- **Evidence:** Impl-decision D-7, D-26.
- **Rejected alternatives:**
  - Single-cycle handoff (no "Opening editor…") — D-26 explicitly forbids; the notice never reaches the screen.
  - Full dialog for every editor exit — too much friction for the common zero-exit case.
- **Driven by findings:** F23
- **Linked technical notes:** —
- **Referenced in spec:** External-editor handoff — visual layout

---

## D45: Color palette extensions

- **Question:** Beyond the run-mode palette (LightGray, White, Green), what colors does the visual design add?
- **Decision:** `Red = lipgloss.Color("9")` for FATAL findings and read-only banner; `Yellow = lipgloss.Color("11")` for WARN findings and external/symlink/shared-install banners; `Cyan = lipgloss.Color("14")` for INFO findings and unknown-field banner; `Dim = lipgloss.Color("8")` for placeholders, dimmed-when-overlaid content. Backgrounds remain default (terminal background) except where reverse-video is named. No new background colors introduced.
- **Rationale:** Standard ANSI 8-color codes are reliably rendered by every terminal that meets pr9k's existing capability bar. Severity-by-color is well-established (red=danger, yellow=warning, cyan=info). Foreground-only avoids the cross-terminal background-color portability problems.
- **Evidence:** ANSI 8-color reliability standard. Behavioral D25 (severity prefixes; color is supplementary).
- **Rejected alternatives:**
  - 256-color palette extensions — unnecessary granularity for severity classes.
  - Background colors for severity — many terminals render background poorly in light themes.
- **Driven by findings:** —
- **Linked technical notes:** —
- **Referenced in spec:** Color palette extensions

---

## D47: Detail-pane scroll indicator rendering

- **Question:** How does the detail pane indicate that more content exists above or below the visible area?
- **Decision:** A single-column scroll-position glyph runs down the **second-rightmost** column of the detail pane (one column inside the right border, leaving the `│` border intact): `▲` for "more above," `█` for the visible-region indicator, `▼` for "more below," `│` (matching the chrome) elsewhere. Mirrors [D25](#d25-outline-scroll-indicator-rendering)'s outline-pane shape.
- **Rationale:** Behavioral D52 commits to the detail pane being independently scrollable with a visible indicator. D25 already settled the visual form for the outline; reusing it for the detail pane keeps users from learning two scroll-indicator patterns. Placing the indicator one column inside the border rather than on the border itself avoids replacing the chrome `│` with the indicator glyphs, which would visually fracture the frame.
- **Evidence:** Behavioral D52; D25 (outline scroll indicator); team-finding [F16](team-findings.md#f16-detail-pane-scroll-indicator-missing-from-spec-and-mockups).
- **Rejected alternatives:**
  - Indicator on the right border itself — fractures the frame.
  - Indicator on the left edge of the detail pane (just to the right of the pane separator `│`) — visually conflicts with the field labels.
- **Driven by findings:** F16
- **Linked technical notes:** —
- **Referenced in spec:** Detail pane — field rendering (Detail-pane scroll indicator subsection)

---

## D48: Minimum supported terminal size and fallback render

- **Question:** What is the minimum terminal size the builder supports, and what does it render below the threshold?
- **Decision:** Minimum supported terminal size is **60 columns × 16 rows**. The 60-column floor lets the outline pane formula `min(40, max(20, ⌊W × 0.4⌋))` produce a 24-column outline, leaving 33 columns for the detail pane (60 − 2 border − 1 separator − 24 outline = 33). The 16-row floor leaves 8 pane rows after the 8-row chrome (8 + 8 = 16). Below either dimension, the builder renders a single line `Terminal too small — minimum 60×16 required` centered on the screen with no chrome. Resizing back above the threshold restores the full render on the next `tea.WindowSizeMsg`.
- **Rationale:** Without a documented floor, the pane-split arithmetic produces undefined results at small sizes (zero or negative pane area). Setting an explicit minimum lets the implementation guard the renderer with a single boolean check. 60×16 is the smallest size at which all eight chrome rows fit and both panes have meaningful width.
- **Evidence:** Team-finding [F18](team-findings.md#f18-minimum-supported-terminal-size--width-and-height-floor-undefined); chrome budget arithmetic from D9.
- **Rejected alternatives:**
  - Fall through to whatever the renderer produces — undefined behavior at narrow widths and short heights.
  - Lower minimum (e.g., 40×10) — pane area collapses; outline minimum 20 violates the formula.
  - Higher minimum (e.g., 80×24) — excludes legitimate small-window users; the chrome math works at 60×16.
- **Driven by findings:** F18
- **Linked technical notes:** —
- **Referenced in spec:** Layout — the persistent frame (Minimum supported terminal size subsection)

---

## D49: Step name truncation rule

- **Question:** What happens when an outline step name is wider than the available row space?
- **Decision:** The step name right-truncates with a `…` suffix. The right-aligned model column is preserved at its position unless that would leave fewer than 12 columns for the truncated name; below 12 columns, the model column is replaced by `…` and the entire row is given to the name (also truncated with `…` if still too long).
- **Rationale:** Long descriptive names are common in real workflows. Without a truncation rule, the row either wraps (breaking the outline grid) or collides with the model column. Preserving the model column at the cost of name length matches IDE conventions (VS Code's outline view, IntelliJ's project panel). The 12-column floor for the name is a usability choice — below 12 chars the user can't recognize their step, so dropping the model column gives more room.
- **Evidence:** Team-finding [F20](team-findings.md#f20-step-name-truncation-rule-undefined).
- **Rejected alternatives:**
  - Wrap to a second row — breaks the outline's one-row-per-item grid.
  - Truncate the model column first — loses information about step kind / model.
  - Hard-wrap at the pane width — produces unreadable column-misaligned step rows.
- **Driven by findings:** F20
- **Linked technical notes:** —
- **Referenced in spec:** Outline pane — visual structure (Step name truncation subsection)

---

## D50: Detail-pane field label truncation rule

- **Question:** What happens when a detail-pane field label is wider than the row's label budget?
- **Decision:** Field labels right-truncate with a `…` suffix when they exceed **28 characters**. The label budget (28 chars) is sized to fit the longest field label committed by the schema (`refreshIntervalSeconds`, 22 chars) with margin for user-supplied env / containerEnv keys. Above 28 chars, the label is truncated; the input box rendering proceeds at its standard column.
- **Rationale:** containerEnv field labels are user-supplied and unbounded. Without a truncation rule, the input box position drifts across rows or wraps to a second line. A fixed label-width budget keeps every field's input box aligned at the same column, which is a basic readability requirement.
- **Evidence:** Team-finding [F21](team-findings.md#f21-long-containerenv-key-label-truncation-undefined).
- **Rejected alternatives:**
  - No fixed label budget; let labels push the input box right — input boxes drift, hurting scan-readability.
  - Wrap labels to a second row — doubles vertical space for fields with long labels.
  - Truncate at 20 chars — schema labels like `refreshIntervalSeconds` would truncate themselves.
- **Driven by findings:** F21
- **Linked technical notes:** —
- **Referenced in spec:** Detail pane — field rendering (Field label truncation subsection)

---

## D51: Choice-list dropdown overflow rule

- **Question:** What happens when an open dropdown would extend below the pane bottom?
- **Decision:** When a dropdown anchored below its field row would extend past the pane bottom, it instead renders **upward** — anchored to the field row's top edge instead of its bottom edge. If the upward render would also exceed the available space (the field is near the pane top *and* the dropdown is taller than the pane), the dropdown scrolls internally with the same scroll-indicator pattern as the detail pane ([D47](#d47-detail-pane-scroll-indicator-rendering)).
- **Rationale:** Standard menu / dropdown convention across UI frameworks (macOS, Windows, GNOME): a menu near the bottom of a window flips upward instead of clipping. The fallback to internal scrolling handles the pathological case where neither up nor down has enough space.
- **Evidence:** Team-finding [F22](team-findings.md#f22-choice-list-dropdown-overflow-rule-undefined). Cross-platform UI convention.
- **Rejected alternatives:**
  - Clip the dropdown at the pane border — hides options the user needs.
  - Always render upward — visually inconsistent and surprising for fields near the pane top.
  - Open as a centered overlay — breaks the visual link between the field and the dropdown.
- **Driven by findings:** F22
- **Linked technical notes:** —
- **Referenced in spec:** Detail pane — field rendering (Choice-list dropdown overflow subsection)

---

## D6 (revised evidence)

The original D6 entry's evidence claimed `┬`/`┴` extended an existing run-mode hrule pattern. **Corrected**: the run-mode hrules use only `├`/`┤` boundary glyphs with no mid-rule junctions. `┬`/`┴` are a *new* pattern this visual design introduces. The justification rests on D1's chrome-glyph commitment (which already includes T-junctions in scope) and D46's BMP-only commitment (which guarantees these glyphs render reliably). Spec's Coordinations table now explicitly notes the junctions as the one element without a run-mode precedent.

---

## D9 (revised count)

The original D9 entry double-counted hrules in the chrome row budget. **Corrected count**: 8 chrome rows total = 5 fixed-content rows (top border, menu bar, session header, status footer, bottom border) + 3 horizontal rules (below menu bar, below session header, above status footer). Pane area absorbs `terminalHeight - 8` rows.

---

## D22 (clarified)

The original D22 entry's "Rejected alternatives" rejected reverse-video for outline focus. **Reaffirmed**: focused outline rows render as `> ` prefix + white text, **with no reverse-video on the row body**. Reverse-video is reserved for two states only: (1) reorder mode (the moving step), and (2) the highlighted item inside an open dropdown. All mockups updated to match this rule.

---

## D3 (clarified)

The original D3 entry committed to "the rest renders white" for the title text. **Clarified**: the brand name `Power-Ralph.9000` renders **green**; everything from the first ` — ` onward — both ` — ` separators and the surface name `Workflow Builder` and the path — renders **white**. The run-mode `colorTitle` rule (split on first ` — `, color the suffix white) produces this behavior naturally for the two-separator builder title.

---

## D15 (extended)

The banner panel opened via `[N more warnings]` is itself a small dialog. Its single Close button follows [D37](#d37-dialog-keyboard-default-rendering)'s keyboard-default rendering rule (green `[ ]` brackets).

---

## D16 (revised)

The findings-summary slot is prefixed with a textual severity tag in addition to the count text: `[!]` in **red** when fatals are present, `[i]` in **cyan** when only non-fatal findings exist. The prefix is omitted when all counts are zero (the slot is then empty). Color-blind users can disambiguate the findings summary from the `[N more warnings]` affordance via the prefix tag alone, satisfying the same color-blind-safety contract as the banner system (D14).

---

## D40 (extended)

The help modal's shortcut grid renders in **two columns** when the modal's interior width is at least 56 columns; below that, single-column. When the modal content exceeds terminal height, the modal scrolls — a scroll indicator runs down the second-rightmost column inside the right border (matching D25 / D47), and the dismiss-hint row is pinned to the modal's bottom border row regardless of scroll position.

---

## D41 (revised)

The reorder-mode banner that previously replaced the priority banner has been **removed**. The session header in reorder mode is unchanged from its non-reorder state — any priority banner stays put. Reorder mode communicates itself via three signals: footer change, reverse-video on the moving row, and green gripper. This avoids the visibility violation of suppressing a higher-priority status (`[ro]`, `[ext]`, `[sym]`) for a transient mode signal.

---

## D42 (extended)

All inline notices in the path picker carry a textual prefix tag: `[warn]` (yellow) for warnings, `[hint]` (light gray) for non-warning notes such as "completed to single match." The `↳` glyph remains as a visual flow cue but is not the semantic carrier.

---

## D44 (extended)

The post-handoff frame after the external editor exits scrolls the detail pane to make the just-edited field visible; cursor focus lands on the same field that was focused before the editor opened. If the terminal was resized during the editor session, the resize is processed as a `tea.WindowSizeMsg` on the first post-return render cycle, and the scroll-to-field rule applies after the resize has been laid out.

---

## D46: Glyph set BMP-only and fallback policy

- **Question:** Which glyphs are committed to, and what about font-coverage failures?
- **Decision:** Every glyph used in the design is BMP-only (Basic Multilingual Plane). The complete list is enumerated in the Glyph reference section of the spec. If a terminal font does not render one of these glyphs, the user sees a tofu/replacement glyph but the layout is unaffected (every glyph occupies one cell, satisfying lipgloss width math). No fallback glyph substitution is performed at render time.
- **Rationale:** Behavioral D34 sets the precedent ("avoid Braille-font risk"). BMP-only ensures every supported terminal font (DejaVu, Hack, JetBrains Mono, Source Code Pro, Cascadia, etc.) renders the glyphs. No fallback substitution because lipgloss can't tell at render time which glyphs will tofu — and substituting based on guesswork would produce inconsistent output.
- **Evidence:** Behavioral D34; impl-decision D-35 (`GripperGlyph = "⋮⋮"` chosen specifically for font coverage).
- **Rejected alternatives:**
  - Use astral-plane glyphs for richer visual variety — tofu risk on common fonts.
  - Per-terminal glyph fallback table — over-engineering for a documented font-coverage standard.
- **Driven by findings:** —
- **Linked technical notes:** —
- **Referenced in spec:** Glyph reference
