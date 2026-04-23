# Feature Specification: Workflow Builder

An in-pr9k terminal interface for authoring and editing pr9k workflow bundles — the configuration, the referenced prompt files, and the referenced scripts — without hand-editing JSON, so that a workflow author produces a validated, ready-to-run workflow directly from a pr9k session.

## Outcome

A workflow author ends a workflow-builder session with a valid, saved workflow bundle at a target directory they chose. The bundle is fit to be resolved and executed by the main `pr9k` command immediately after. The user does not open a text editor on the configuration file themselves, does not hand-write JSON, and does not need to know the internal field names to reach a working configuration — constrained fields are presented as choice lists, free-text fields are presented as labeled input boxes, and the same validator the main `pr9k` runs at startup evaluates the edited state before the save lands.

## Actors and Triggers

- **Actors** — the *workflow author*. This is either (a) a pr9k operator tailoring their own iteration loop against their own project, or (b) a pr9k maintainer updating the bundled default workflow in the source tree.
- **Triggers** — the user launches the `pr9k workflow` subcommand ([D1](artifacts/decision-log.md#d1-subcommand-name-pr9k-workflow)). The subcommand accepts `--workflow-dir` and `--project-dir` with the same semantics as the main `pr9k` command; it does not accept `--iterations` or any other run-specific flag ([D19](artifacts/decision-log.md#d19-subcommand-flag-surface)).
- **Preconditions** — pr9k is installed; the terminal supports the same TUI capabilities the main `pr9k` command already requires (alt-screen, cell-motion mouse); the user has write permission to whichever target they ultimately save to (the editor itself does not require write permission just to browse a read-only default).

## Primary Flow

1. The user launches `pr9k workflow`.
2. The builder resolves a **default target** using the same rules the main command uses — the workflow-directory flag if set, otherwise the project-local workflow directory, otherwise the bundled default shipped with pr9k ([D3](artifacts/decision-log.md#d3-default-target-resolution-semantics)).
3. The builder shows a **target selection landing page** with the default target preselected. Up to four target options are listed, with contextually duplicate options collapsed so the page shows only the options that resolve to distinct directories ([D2](artifacts/decision-log.md#d2-target-selection-modes), [D31](artifacts/decision-log.md#d31-landing-page-duplicate-option-suppression)):
   - **Edit the default target in place** — opens the default target directly. If the default target is not writable by the current user, the landing page surfaces that fact in a banner, opens that target in a **browse-only mode** that hides the save affordance entirely (rather than merely disabling it), and recommends "copy to local" as the next action ([D4](artifacts/decision-log.md#d4-read-only-default-fallback), [D30](artifacts/decision-log.md#d30-read-only-targets-open-in-browse-only-mode)).
   - **Copy the default target to the local project and edit the copy** — copies the default target's configuration file, referenced prompt files, and referenced scripts into the project-local workflow directory ([D15](artifacts/decision-log.md#d15-companion-file-copy-scope)), showing a progress status during the copy and handling partial-copy failures cleanly ([D32](artifacts/decision-log.md#d32-copy-progress-and-partial-failure-handling)); then enters edit view on the copy.
   - **Edit the local project's workflow** — opens the project-local workflow directory, whether or not it exists yet.
   - **Edit a workflow in another folder** — prompts for a folder path and opens whatever workflow bundle exists there, or offers to scaffold one ([D8](artifacts/decision-log.md#d8-scaffold-or-copy-or-cancel-for-empty-folder)). If the chosen path resolves outside the user's home directory or outside the current project directory, the builder displays an **external-workflow banner** during the session and requires explicit confirmation at the first save ([D22](artifacts/decision-log.md#d22-external-workflow-warning)).
4. The builder loads the configuration from the selected directory. If the directory contains no configuration file, the builder offers three options before entering edit view: **scaffold a minimal valid workflow**, **copy from the default target**, or **cancel back to landing**.
5. The builder enters its **edit view**. The view has three persistent surfaces:
   - A **workflow outline** on the left — collapsible sections for the top-level environment passthrough list, the top-level container-environment list, the status-line block (when present), and the three ordered phase sections (initialize, iteration, finalize). All sections start expanded on first load. Each section header shows a count of items it contains, which remains visible when the section is collapsed. Collapsing a section with the cursor inside moves the cursor to the section header. The outline is independently scrollable with a visible scroll-position indicator when it exceeds the viewport height ([D28](artifacts/decision-log.md#d28-collapsible-section-behavior), [D29](artifacts/decision-log.md#d29-outline-scrollability)).
   - A **detail pane** on the right — shows the fields of the currently selected outline item, or a section summary when a section header is selected.
   - A **shortcut footer** at the bottom and a **session header** at the top. The session header shows the target path, whether the session is read-only, whether there are unsaved changes, the external-workflow banner (when applicable), and the validator findings summary if any exist. The shortcut footer always shows the keyboard shortcuts available in the current mode ([D24](artifacts/decision-log.md#d24-help-modal-and-shortcut-footer)).
6. On entering edit view, the cursor is placed on the first step of the iteration phase (or on the iteration phase header if the phase is empty) ([D26](artifacts/decision-log.md#d26-initial-cursor-focus)).
7. The user navigates the outline and the detail pane with either keyboard or mouse, both always available ([D13](artifacts/decision-log.md#d13-keyboard-and-mouse-both-supported)), and edits fields:
   - **Plain text fields** are rendered as single-line input boxes with inline constraint hints (e.g., "identifier only", "must be positive integer"). Input is sanitized at input time: embedded newlines are stripped, ANSI escape sequences are stripped, and a soft length cap produces a visible warning when exceeded ([D42](artifacts/decision-log.md#d42-structured-field-input-sanitization)).
   - **Fields constrained to a fixed set of values** — capture mode, timeout policy, whether a step is a Claude step or a shell step, status-line type, and any other enum the schema defines — are rendered as **choice lists** picked by keyboard or mouse. Constrained fields carry a trailing `▾` indicator in their unfocused state so they are visually distinguishable from free-text fields without having to be focused ([D12](artifacts/decision-log.md#d12-constrained-fields-as-choice-lists), [D27](artifacts/decision-log.md#d27-unfocused-field-signifiers)).
   - **The step's model field** is rendered as a free-text input with a suggestion list of known-good values; the builder does not reject values outside the suggestion list, because the underlying schema does not constrain them ([D12](artifacts/decision-log.md#d12-constrained-fields-as-choice-lists)).
   - **Container-environment values whose key name matches a secret pattern** (ending in `_TOKEN`, `_SECRET`, `_KEY`, `_PASSWORD`, `_PASSPHRASE`, `_CREDENTIAL`, `_APIKEY`) render their value as a masked placeholder in the detail pane by default, with a reveal affordance the user can toggle for the specific field ([D20](artifacts/decision-log.md#d20-containerenv-secret-masking)).
   - **Numeric fields** enforce their value ranges at input time (timeout upper and lower bounds, refresh-interval non-negativity).
   - **Multi-line content — prompt files and scripts** — is edited by handing control of the terminal to the user's configured external editor; the builder restores its view when the external editor exits, and re-reads the file from disk ([D5](artifacts/decision-log.md#d5-external-editor-for-multi-line-content), [D16](artifacts/decision-log.md#d16-external-editor-fallback-policy), [T2](artifacts/feature-technical-notes.md#t2-terminal-handoff-to-external-editor)).
   - **Step order within a phase** is changed by keyboard reorder shortcuts (`Alt+↑` / `Alt+↓`) or by mouse drag. Every step row in the outline carries a persistent gripper glyph (`⠿`) on its left edge signifying draggability. Cross-phase drag is not supported in v1; dragging a step past a phase boundary visibly drops it at the phase's edge ([D34](artifacts/decision-log.md#d34-step-reorder-ux)).
   - **Opening a prompt file or script in the external editor** is invoked from the detail pane via a visible shortcut in the footer when a prompt-or-script-path field is focused ([D14](artifacts/decision-log.md#d14-reuse-existing-validator), [D34](artifacts/decision-log.md#d34-step-reorder-ux)).
   - **Adding a step** produces an empty step at the chosen position; the user then chooses its kind (Claude or shell) and fills fields.
   - **Removing a step** requires a confirmation affordance.
8. At any time the user can press `?` to open a **help modal** listing every keyboard shortcut for the current mode. The help modal is unconditionally reachable from the edit view regardless of any other configuration ([D24](artifacts/decision-log.md#d24-help-modal-and-shortcut-footer)).
9. The user invokes save. The builder runs the full workflow-configuration validator against the in-memory state ([D14](artifacts/decision-log.md#d14-reuse-existing-validator), [T3](artifacts/feature-technical-notes.md#t3-in-memory-validation)) and groups the findings by severity ([D6](artifacts/decision-log.md#d6-validation-ux-fatal-blocks-warnings-do-not)):
   - If any finding is **fatal**, save is blocked. The builder opens a **findings panel** listing each finding prefixed with a text-mode severity tag (`[FATAL]`, `[WARN]`, `[INFO]`) alongside any color — severity is never conveyed by color alone ([D25](artifacts/decision-log.md#d25-severity-text-prefixes)). Each finding shows its category, the offending field's location in the outline, and a jump-to-field affordance. When the user jumps to a field, the findings panel stays visible; on each subsequent save attempt, the panel is fully rebuilt from the new validator output. When all fatals are resolved, the panel closes automatically and the save proceeds ([D35](artifacts/decision-log.md#d35-findings-panel-lifecycle)).
   - If the only findings are **warnings** or **informational**, the save proceeds after the user acknowledges the findings panel. A finding acknowledged during a session is not surfaced again for the remainder of that session at the acknowledgment dialog, though it continues to appear in the findings panel the user can open manually ([D23](artifacts/decision-log.md#d23-per-session-warning-suppression)).
   - If there are no findings, the save proceeds silently.
   - A successful save leaves the workflow file on disk either fully containing the previous content or fully containing the new content — a crash or interruption during save never leaves the file torn or empty ([T1](artifacts/feature-technical-notes.md#t1-atomic-configuration-file-save)).
   - When the target is flagged as an external workflow ([D22](artifacts/decision-log.md#d22-external-workflow-warning)), the first save in the session additionally prompts for explicit confirmation before writing.
10. The user quits. If there are unsaved changes, a confirmation dialog intercepts the quit and offers save, discard, or cancel. If the user chooses "save" from the dialog and the save surfaces fatal findings, the quit is cancelled and the user lands back in the edit view with the findings panel open — the dialog is dismissed ([D7](artifacts/decision-log.md#d7-save-semantics-explicit-atomic-unsaved-prompt), [D40](artifacts/decision-log.md#d40-unsaved-quit-compound-state)).

## Alternate Flows and States

### Scaffold-from-empty

- **Entry condition:** The user selects a target folder that exists but contains no configuration file.
- **Sequence:** Builder offers scaffold / copy-default / cancel. On scaffold, the builder creates an in-memory workflow with empty initialize, one placeholder iteration step, and empty finalize, then enters edit view. Nothing is written to disk until the user saves.
- **Exit:** First save writes the configuration file; any prompt or script files the user added during the session are written at the same time.

### Copy-default-to-local

- **Entry condition:** The user selects "Copy the default target to the local project and edit the copy" on the landing page, or selects "copy from default" from the empty-folder dialog.
- **Sequence:** Builder displays a brief status indicator during the copy. Copies the full default bundle — configuration file, all prompt files referenced by any step, all scripts referenced by any step — into the project-local workflow directory ([D15](artifacts/decision-log.md#d15-companion-file-copy-scope)). A prompt or script not referenced by the configuration is not copied. If any file fails to copy, the builder rolls the partial copy back to a clean state, shows an error naming the failed file and reason, and returns to the landing page — it does not enter edit view with a partial bundle ([D32](artifacts/decision-log.md#d32-copy-progress-and-partial-failure-handling)).
- **Exit:** Standard save / quit.

### Read-only target (default or other)

- **Entry condition:** The user selects a target the builder has determined is not writable by the current user. The writability check is applied to all four target options, not only the default ([D4](artifacts/decision-log.md#d4-read-only-default-fallback), [D30](artifacts/decision-log.md#d30-read-only-targets-open-in-browse-only-mode)).
- **Sequence:** Landing page surfaces the read-only state in a visible banner. If the user proceeds, the builder opens the target in **browse-only mode** — the edit view is identical in layout but the save affordance is absent and unsaved-change tracking is disabled. The user can navigate, inspect, and open prompts or scripts in their external editor, but cannot alter the in-memory state through the builder.
- **Exit:** User picks a writable target, copies to local, or quits.

### External-workflow session (target outside user's project and home)

- **Entry condition:** The chosen target path resolves outside the user's project directory and outside the user's home directory.
- **Sequence:** The session header shows an "external workflow" banner for the entire session. At the first save, the builder shows a confirmation dialog warning that the target is outside the user's trusted locations and describing the absolute path. Subsequent saves in the same session do not re-confirm ([D22](artifacts/decision-log.md#d22-external-workflow-warning)).
- **Exit:** User confirms and save proceeds, or cancels and returns to the edit view.

### Symlinked target or companion file

- **Entry condition:** The target directory's `config.json` is a symlink, or any referenced prompt or script file is a symlink (including a symlink that resolves outside the target bundle tree).
- **Sequence:** On load, the builder surfaces a **symlink banner** in the session header listing each affected path. Saves to symlinked targets require confirmation on the first save, following the same pattern as the external-workflow flow ([D17](artifacts/decision-log.md#d17-symlink-policy-follow-with-visibility)). The builder follows symlinks — it does not replace them with regular files.
- **Exit:** User confirms saves, or chooses a different target.

### External-editor invocation

- **Entry condition:** The user opens a prompt file or script for multi-line editing from the detail pane.
- **Sequence:** Builder resolves the configured external editor: `$VISUAL` first, then `$EDITOR`. The resolved value is parsed as a command with arguments (shell-style word splitting) and rejected if it contains shell metacharacters or if it resolves to a relative path that is not on the user's `PATH` ([D33](artifacts/decision-log.md#d33-editor-execution-model)). Before yielding the terminal, the builder displays a brief "Opening editor…" message in the normal-screen context. It yields the terminal to the external editor, waits for it to exit, reclaims the terminal, and re-reads the file from disk ([D5](artifacts/decision-log.md#d5-external-editor-for-multi-line-content), [T2](artifacts/feature-technical-notes.md#t2-terminal-handoff-to-external-editor)).
- **Exit:** Editor exits zero — builder treats the file as updated. Editor exits non-zero — builder still re-reads the file, because the external editor may have written partial content before failing; the user is informed of the non-zero exit.

### Editor binary cannot be spawned

- **Entry condition:** The configured editor value fails validation, points to a missing binary, or the spawn itself fails.
- **Sequence:** Builder shows an error dialog naming the value and the specific problem ("not found on PATH", "contains shell metacharacters", "permission denied"), and offers: retry after the user fixes their environment, or cancel.
- **Exit:** User fixes `$VISUAL`/`$EDITOR` and retries, or cancels.

### No external editor configured

- **Entry condition:** Neither `$VISUAL` nor `$EDITOR` is set.
- **Sequence:** Builder shows a dialog with the absolute path of the file and a copy-pasteable instruction for setting an editor (e.g., `export VISUAL=nano`). The builder does not silently fall back to any editor, because falling back to `vi` (or similar) traps users who do not know its exit sequence ([D16](artifacts/decision-log.md#d16-external-editor-fallback-policy)). The user can close the dialog and continue editing non-multi-line fields, or quit.
- **Exit:** User closes the dialog.

### Validation findings at save

- Covered in Primary Flow step 9.

### Unsaved-changes quit

- **Entry condition:** User invokes quit while in-memory state differs from on-disk state.
- **Sequence:** Confirmation dialog with three choices — save, discard, cancel. Escape is equivalent to cancel. If save produces fatal findings, the quit is cancelled and the user returns to the edit view with the findings panel open ([D40](artifacts/decision-log.md#d40-unsaved-quit-compound-state)).
- **Exit:** Builder quits or returns to edit view per the choice.

### Parse-error recovery

- **Entry condition:** The target's configuration file exists but cannot be parsed.
- **Sequence:** Builder enters a **recovery view** showing the file's raw bytes (including a human-readable note if the file is empty or contains a UTF-8 BOM or non-UTF-8 encoding), the parse error with its location (byte offset or line/column, whichever the parser produces), and four actions: open the raw file in the external editor, reload (re-parse the file as it currently sits on disk), discard and scaffold fresh, cancel back to landing. After a successful open-in-editor invocation from this view, the builder automatically attempts to reload; if parsing succeeds, the builder transitions directly to the edit view without requiring the user to return to the landing page ([D36](artifacts/decision-log.md#d36-parse-error-recovery-reload)).
- **Exit:** Successful reload transitions to edit view; discard-and-scaffold transitions to scaffold-from-empty; cancel returns to landing.

### Unknown-field warning

- **Entry condition:** The configuration file parses successfully but contains fields the builder's schema model does not recognize.
- **Sequence:** On load, the builder shows a non-blocking banner listing the unrecognized fields and warning that saving will remove them from the file. The builder does not preserve unknown fields round-trip — on save, only fields the builder knows about are written ([D18](artifacts/decision-log.md#d18-unknown-fields-warn-and-drop)).
- **Exit:** User acknowledges the banner and continues, or quits without saving.

### Crash-era temporary file on open

- **Entry condition:** On opening a target directory, the builder finds a temporary file left behind by a previous crashed save (matching the temp-file naming convention described in [T1](artifacts/feature-technical-notes.md#t1-atomic-configuration-file-save)).
- **Sequence:** Builder shows a non-blocking notice naming the file and its modification time, and offers to delete it silently or leave it. The builder does not auto-delete ([D42-a](artifacts/decision-log.md#d42-a-crash-era-temp-file-cleanup-contract)).
- **Exit:** User picks an action and proceeds to the landing page / edit view.

## Edge Cases and Failure Modes

| Condition | Required Behavior |
|-----------|-------------------|
| Configuration file cannot be parsed | Enters recovery view (see Alternate Flows — Parse-error recovery). Empty file, UTF-8 BOM, and non-UTF-8 encodings are each surfaced with a human-readable note rather than an opaque parser error ([D43](artifacts/decision-log.md#d43-load-time-integrity-checks)). |
| Configuration file contains duplicate JSON keys | Builder detects duplicates at load time and surfaces a non-blocking banner listing the keys and which value won. No silent data loss ([D43](artifacts/decision-log.md#d43-load-time-integrity-checks)). |
| Configuration file contains trailing content after the JSON object | Builder surfaces a non-blocking banner on load warning that trailing content was found and will be discarded on next save ([D43](artifacts/decision-log.md#d43-load-time-integrity-checks)). |
| A step references a prompt file that does not exist on disk | Detail pane shows a "referenced prompt file not found" state with "create a new empty prompt here" and "choose a different prompt" affordances. Validator surfaces as fatal. |
| A step references a script that does not exist, is not a regular file, has no shebang, or is not executable | Validator surfaces as fatal ([D21](artifacts/decision-log.md#d21-script-executability-validation)). When the only problem is a missing execute bit and the script has a valid shebang, the detail pane offers a "mark executable and continue" action that grants the execute bit and clears the finding. |
| A referenced prompt path is itself a directory | Validator surfaces as fatal with a "path is a directory" message. |
| A prompt or script file is a symlink that resolves outside the target bundle tree | Builder surfaces in the symlink banner on load and requires confirmation on first save (see Alternate Flows — Symlinked target or companion file). |
| User edits a prompt file, and the file is modified externally during the session | On re-open of the file, editor shows the current on-disk content. No in-memory diff or merge UI — the file on disk is the source of truth. |
| Target directory is deleted or renamed during the session | Save fails with a clear error naming the missing directory; the user remains in the edit view with the in-memory state intact and can either retry after restoring the directory or quit to try another target ([D41](artifacts/decision-log.md#d41-cross-session-mutation-detection)). |
| Configuration file is modified on disk since the builder loaded it | At save time, the builder detects the change (by snapshotting file size and modification time at load and comparing at save) and prompts the user with a conflict dialog naming the mismatch; the user can overwrite, reload-and-discard, or cancel the save ([D41](artifacts/decision-log.md#d41-cross-session-mutation-detection)). |
| Target configuration file is a symlink | Builder follows the symlink at save — the save writes to the symlink's target file rather than replacing the symlink with a regular file. Surfaced in the symlink banner on load. |
| Target filesystem runs out of space during save | Save fails with a clear error. Builder removes any partial temporary file it created and preserves the in-memory state. |
| Target filesystem is read-only or save returns a permission error | Save fails with a clear error naming the file and the permission state; in-memory state is preserved; user can retry after fixing permissions or picks another target. |
| Terminal is resized during an external-editor invocation or while a dropdown is open | Builder re-layouts to the current terminal size on resize; overlays and dropdowns re-render within the new bounds. |
| User pastes multi-line text or ANSI escape sequences into a single-line input field | Newlines and ANSI escapes are stripped at input time with a visible "pasted content sanitized" message; the remaining text becomes the field value ([D42](artifacts/decision-log.md#d42-structured-field-input-sanitization)). |
| Reorder invalidates a downstream `skipIfCaptureEmpty` or `{{VAR}}` reference | Validator surfaces at save; finding appears in the findings panel. The builder does not block reorder at input time. |
| Duplicate step name or duplicate capture name within a phase | Validator surfaces at save. |
| User types a disallowed value into a constrained field | Choice lists never offer disallowed values in the first place; numeric inputs clamp at the boundary. Cross-field rules surfaced by the validator at save. |
| User is editing the bundled default on a writable shared install | Session header shows a "shared install" banner alerting the user that saving will affect other users of this pr9k binary ([D39](artifacts/decision-log.md#d39-shared-install-and-observability)). |
| Another `pr9k` process is running against the same project at save time | No cross-process coordination. The configuration-file-mtime check at save (described above) is the only collision signal; it detects on-disk changes but not concurrent in-memory edits across two builder sessions ([D41](artifacts/decision-log.md#d41-cross-session-mutation-detection)). Last-completed-save wins. |
| External editor daemonizes and returns immediately | Documented limitation; see help text and the documentation guide on configuring the external editor (e.g., `code --wait`). The builder does not try to distinguish "daemonized" from "exited normally". |
| External editor hangs indefinitely | User may interrupt with SIGINT, which the foreground process group will receive — the editor exits if it handles SIGINT. If the editor ignores SIGINT, the user must terminate it from another session. |
| SIGHUP (terminal disconnect) during session | Builder exits immediately; unsaved changes are lost. Documented as a known limitation; the how-to guide recommends `nohup` or a terminal multiplexer for long-running edit sessions. |
| SIGTSTP (Ctrl-Z) during session | Builder releases the terminal to the shell and suspends. On resume, it reclaims the terminal and re-renders. |
| User requests a version bump or config migration from within the builder | Not supported in v1; out of scope (see Out of Scope). |

## User Interactions

- **Affordances:**
  - Landing page: up to four target options (deduplicated), default target preselected, banners for read-only / external-workflow / symlink states as applicable.
  - Session header: target path, unsaved-changes indicator, read-only indicator, external-workflow banner, shared-install banner, symlink banner, validator findings summary.
  - Outline: collapsible sections with always-visible item counts; scrollable with position indicator; each step row shows a gripper glyph (`⠿`) and, when applicable, a secret-masked value indicator or a fatal-finding marker.
  - Detail pane: labeled input boxes, choice lists (indicated by `▾`), numeric inputs with visible bounds, masked secret values with reveal toggle, add-step and remove-step affordances at the phase level, open-in-external-editor affordance on any prompt-or-script-path field (always visible in the footer when the field is focused).
  - Step reorder: gripper glyph signifier, `Alt+↑` / `Alt+↓` keyboard shortcuts, mouse drag.
  - Save, quit, toggle findings panel, jump-to-field from a finding, `?` help modal.
- **Feedback:**
  - Shortcut footer shows every available keyboard shortcut for the current mode; updates when focus moves to a field with a focus-specific action.
  - Currently selected outline item and currently focused detail-pane field are visually distinct.
  - Input hints render next to the input (e.g., "must be a positive integer up to 86400").
  - Invalid input shows an inline marker and a one-line reason.
  - Findings panel: each entry has a text-mode severity prefix (`[FATAL]`, `[WARN]`, `[INFO]`), a category tag, the offending location, and a jump-to-field affordance.
  - External editor invocation shows an "Opening editor…" message in normal-screen context before yielding the terminal.
  - Validation runs on save show a brief "Validating…" status when validation takes more than a fraction of a second.
- **Error states:**
  - Read-only banner and browse-only edit view.
  - External-workflow banner.
  - Symlink banner and first-save confirmation.
  - Shared-install banner.
  - Referenced-file-missing states on step fields.
  - Parse-error recovery view with reload action.
  - Unknown-field warning banner.
  - Crash-era temp-file notice.
  - Permission and disk-full errors at save.
  - Editor-binary-cannot-be-spawned dialog.
  - Configuration-file-modified-on-disk conflict dialog.
  - Validation findings panel (a normal part of the save flow, not an exception).

## Coordinations

| Coordinating System | Direction | Interaction | Ordering / Consistency Requirement |
|---------------------|-----------|-------------|-----------------------------------|
| Workflow configuration validator | outbound | Builder passes the in-memory workflow state to the validator and receives fatal / warning / info findings. | Save must not land a new configuration file if any finding is fatal. Warnings and info findings never block the save. The validator sees exactly the state the save will write — no subset, no superset ([T3](artifacts/feature-technical-notes.md#t3-in-memory-validation)). |
| External editor | outbound | Builder yields terminal control to the user's configured external editor for multi-line content edits, then reclaims control. | No builder keystrokes or mouse events are consumed while the external editor holds the terminal. On return, the file on disk is re-read before any further builder action ([T2](artifacts/feature-technical-notes.md#t2-terminal-handoff-to-external-editor)). |
| Filesystem — workflow directory | outbound / inbound | Reads the configuration file and referenced companion files on load and on each external-editor return; writes the configuration file and any newly-created or edited companion files on save. | Every configuration file write is durable against interruption — the file on disk contains either the prior content or the new content, never partial content ([T1](artifacts/feature-technical-notes.md#t1-atomic-configuration-file-save)). |
| Session log — `.pr9k/logs/` | outbound | Builder logs session-level events (start, target chosen, save outcomes, external-editor invocations and exit codes, quit with / without save) to the same `.pr9k/logs/` directory the main `pr9k` uses ([D39](artifacts/decision-log.md#d39-shared-install-and-observability)). | The log line for any save records whether it succeeded, what the validator reported, and whether an external-workflow or shared-install confirmation was required. |
| Main pr9k orchestrator | none during session | The builder does not start, pause, observe, or communicate with an orchestrator process. | — |

## Out of Scope

- Running or dry-running workflows from within the builder, including single-step dry-runs and variable-expansion previews ([D9](artifacts/decision-log.md#d9-v1-scope-boundary)).
- Importing workflows from URLs, git repositories, or network paths ([D9](artifacts/decision-log.md#d9-v1-scope-boundary)).
- Diffing an edited workflow against the default or against any other reference.
- Multi-user or multi-session locking of the workflow directory. Concurrent builder sessions against the same file resolve last-completed-save-wins, with a best-effort mtime-collision warning ([D41](artifacts/decision-log.md#d41-cross-session-mutation-detection)).
- Syntax highlighting inside prompt or script content — the external editor owns that.
- Any version-control operations. The user runs `git` themselves.
- Modifying pr9k itself or the set of built-in substitution tokens the runtime understands.
- Automatic migration of workflows written for older pr9k versions, and schema-field-forward-compat preservation of fields the current builder does not recognize. Unknown fields are warned on load and dropped on save ([D18](artifacts/decision-log.md#d18-unknown-fields-warn-and-drop)).
- Integrity attestation of shared workflow bundles (SBOM, digital signing). Users who share bundles via git rely on the receiving repository's own review process.
- Cross-phase drag-to-move. Users who want to move a step between phases delete it from one and add it to another ([D34](artifacts/decision-log.md#d34-step-reorder-ux)).
- Step templates, step snippets, or any built-in catalog of Ralph-specific steps inside the builder itself. The builder is a generic workflow-structure editor; anything Ralph-specific lives in the configuration the user is editing, not in the builder's code ([D11](artifacts/decision-log.md#d11-no-ralph-specific-knowledge-in-the-builder)).
- Support for terminals that do not meet the same capability bar the main `pr9k` requires today.

## Documentation Obligations

This feature ships with the following documentation artifacts in the same pull request as the implementation ([D38](artifacts/decision-log.md#d38-documentation-obligations)):

- `docs/features/workflow-builder.md` — feature behavior, TUI layout, keyboard map, landing-page modes, interaction with validator and external editor.
- `docs/how-to/using-the-workflow-builder.md` — step-by-step guide for new users: launching the subcommand, picking a target, editing steps, saving.
- `docs/how-to/configuring-external-editor-for-workflow-builder.md` — how the builder resolves `$VISUAL` / `$EDITOR`, what values are rejected, recommended settings (`code --wait`, `nvim`, `nano`).
- An ADR recording the atomic-rename save pattern and its relationship to the rest of the codebase's file-write conventions.
- A code-package doc for any new Go package the builder introduces, following the existing `docs/code-packages/` pattern.
- Updates to `docs/features/cli-configuration.md` adding the `pr9k workflow` subcommand and its flags.
- Updates to `CLAUDE.md` linking every new doc file.
- Updates to `docs/architecture.md` if new top-level packages are introduced.

## Versioning

Adding the `pr9k workflow` subcommand is a backwards-compatible addition to the CLI surface. Per the 0.y.z rules in `docs/coding-standards/versioning.md`, this requires a **patch** version bump at ship time ([D37](artifacts/decision-log.md#d37-version-bump)). The new subcommand name becomes part of pr9k's public API from the moment it ships.

## Testing

The implementation plan must cover:

- The atomic-rename save path with unit tests that simulate write failure between the temporary-file write and the rename step, confirming the on-disk file is unchanged ([T1](artifacts/feature-technical-notes.md#t1-atomic-configuration-file-save)).
- The terminal handoff to the external editor through an injectable runner, so external-editor invocation is tested without a real TTY ([T2](artifacts/feature-technical-notes.md#t2-terminal-handoff-to-external-editor), [D41-b](artifacts/decision-log.md#d41-b-test-strategy-for-t1-t2-and-tui-modes)).
- The validator integration, including the in-memory state being passed to the validator rather than a file path ([T3](artifacts/feature-technical-notes.md#t3-in-memory-validation)).
- Every TUI mode's keyboard and mouse inputs through Bubble Tea model-update unit tests, in the pattern already used by the main TUI.
- Every alternate flow and every row of the edge-case table above.

## Open Items

<!-- populated by project-manager synthesis -->

## Summary

- **Outcome delivered:** A workflow author produces or updates a validated pr9k workflow bundle through an interactive terminal interface, without hand-editing JSON.
- **Primary actors:** Workflow author — pr9k operator tailoring their own loop, or pr9k maintainer updating the bundled default.
- **Decisions settled by evidence:** 7 — see [artifacts/decision-log.md](artifacts/decision-log.md)
- **Decisions settled by user input:** 35 — see [artifacts/decision-log.md](artifacts/decision-log.md)
- **Sub-agents consulted:** user-experience-designer, junior-developer, edge-case-explorer, adversarial-security-analyst, devops-engineer — see [artifacts/team-findings.md](artifacts/team-findings.md)
- **Key adjustments from review:** security hardening around `--workflow-dir` and `$EDITOR`; explicit symlink and secret-masking policies; findings-panel lifecycle and accessibility prefixes; unknown-field handling; documentation, versioning, observability, and testing commitments.
- **Remaining open items:** 0 at initial draft; updated by project-manager synthesis
- **Technical notes:** 3 — see [artifacts/feature-technical-notes.md](artifacts/feature-technical-notes.md)
