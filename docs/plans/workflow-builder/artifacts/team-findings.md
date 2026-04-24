# Team Findings: Workflow Builder

<!--
Findings from the review team dispatched in Step 6. Resolutions applied in
Step 7 against the spec, decision log, and tech-notes. Findings are
consolidated where multiple reviewers raised the same concern (e.g., F9
merges UX-009, junior-developer-F2, and devops-8 — all three flagged the
same editor-fallback contradiction).
-->

## F1: Severity conveyed by color alone in findings panel

- **Agent:** user-experience-designer (UX-001); adversarial-security-analyst (F6)
- **Finding:** The findings panel surfaces severity (fatal / warning / info) only as a label count and color; color-vision-limited users and color-stripping terminals lose the distinction. Security-meaningful warnings are not visually elevated within the warning tier.
- **Resolution:** Every finding row carries a text-mode severity prefix (`[FATAL]`, `[WARN]`, `[INFO]`) alongside any color. Color is additive, not the sole signal.
- **Resolved by:** evidence (Universal Design Principle 4; existing `src/internal/ui/header.go:242-256` marker-plus-color pattern)
- **Affected decisions:** D25
- **Affected tech-notes:** —
- **Changed in spec:** Primary Flow step 9; User Interactions — Feedback

## F2: No keyboard discovery surface in the builder

- **Agent:** user-experience-designer (UX-002, UX-015)
- **Finding:** The builder has more keyboard shortcuts than the main TUI but specifies no place to discover them. The main TUI's `?` help modal is gated behind `statusLineActive`, which is already a defect; the builder risks inheriting it.
- **Resolution:** Persistent shortcut footer in every edit-view mode showing the shortcuts available at the current focus. A help modal unconditionally reachable via `?` from any edit-view mode (never gated).
- **Resolved by:** evidence
- **Affected decisions:** D24
- **Affected tech-notes:** —
- **Changed in spec:** Primary Flow steps 5 and 8; User Interactions — Feedback

## F3: Landing page shows four options even when two resolve to the same path

- **Agent:** user-experience-designer (UX-003)
- **Finding:** In the no-flags-no-project case, "edit the local project's workflow" and "copy the default to the local project" point to the same directory — two options with identical meaning.
- **Resolution:** Collapse landing-page options that resolve to the same directory; offer a "show all options" affordance so expert users can still see the full set.
- **Resolved by:** evidence (Hick's Law)
- **Affected decisions:** D31
- **Affected tech-notes:** —
- **Changed in spec:** Primary Flow step 3

## F4: No initial focus specified when edit view opens

- **Agent:** user-experience-designer (UX-004)
- **Finding:** A first-time user enters the edit view with no item focused — no entry point for interaction.
- **Resolution:** Cursor on the first step of the iteration phase (or on the iteration phase header if empty).
- **Resolved by:** user input (recommendation accepted as default)
- **Affected decisions:** D26
- **Affected tech-notes:** —
- **Changed in spec:** Primary Flow step 6

## F5: Step reorder affordance entirely unspecified

- **Agent:** user-experience-designer (UX-005)
- **Finding:** The spec says step order is changed by "keyboard reorder shortcuts or by mouse drag" but names no shortcut and describes no visual signifier. Keyboard-only users have no path to discover reorder.
- **Resolution:** Persistent `⠿` gripper glyph on every step row; `Alt+↑` / `Alt+↓` keyboard shortcuts named in the spec and shown in the footer when a step is focused; mouse drag on the gripper or anywhere on the row. Cross-phase drag is out of scope.
- **Resolved by:** evidence (VS Code / JetBrains `Alt+↑/↓` convention) plus user input on cross-phase exclusion
- **Affected decisions:** D34
- **Affected tech-notes:** —
- **Changed in spec:** Primary Flow step 7; User Interactions — Affordances

## F6: Findings panel post-fix lifecycle undefined

- **Agent:** user-experience-designer (UX-006)
- **Finding:** After the user jumps to a field from a fatal finding and fixes it, the spec does not describe whether the panel stays open, re-validates, or requires a fresh save.
- **Resolution:** Panel stays visible as the user edits; rebuilt from fresh validator output on each save attempt; closes automatically when all fatals are resolved and save proceeds. User can also dismiss manually.
- **Resolved by:** evidence
- **Affected decisions:** D35
- **Affected tech-notes:** —
- **Changed in spec:** Primary Flow step 9

## F7: Constrained vs. free-text fields indistinguishable when unfocused

- **Agent:** user-experience-designer (UX-007)
- **Finding:** Choice-list fields (`captureMode`, `onTimeout`, etc.) and free-text-with-suggestions fields (`model`) look identical when unfocused. Users must focus each field to learn its type.
- **Resolution:** Trailing `▾` glyph on constrained fields in their unfocused state. Free-text fields render without.
- **Resolved by:** evidence
- **Affected decisions:** D27
- **Affected tech-notes:** —
- **Changed in spec:** Primary Flow step 7

## F8: External-editor handoff has no failure case

- **Agent:** user-experience-designer (UX-008)
- **Finding:** The spec handles "no editor configured" but not "editor binary cannot be spawned" (bad path, missing binary, permission denied). Users with a stale `$VISUAL` pointing at a deleted binary have no path forward.
- **Resolution:** Dedicated alternate flow for editor-spawn-failure with error dialog naming the value and specific problem; retry or cancel actions.
- **Resolved by:** evidence
- **Affected decisions:** D33
- **Affected tech-notes:** —
- **Changed in spec:** Alternate Flows — Editor binary cannot be spawned

## F9: External-editor fallback contradiction

- **Agent:** user-experience-designer (UX-009); junior-developer (F2); devops-engineer (Finding 8)
- **Finding:** The initial spec said "a safe fallback if neither is set" AND had an alternate flow for "neither is set and no fallback is available" — mutually exclusive. Plus the fallback was never named, leaving implementation and user behavior undefined.
- **Resolution:** No silent fallback. When neither `$VISUAL` nor `$EDITOR` is set, the builder shows a dialog with the file's absolute path and a copy-pasteable configuration hint. Never traps users in `vi`.
- **Resolved by:** user input (Q-A option A)
- **Affected decisions:** D16, D33
- **Affected tech-notes:** —
- **Changed in spec:** Alternate Flows — External-editor invocation, No external editor configured

## F10: Parse-error recovery has no "reload" action

- **Agent:** user-experience-designer (UX-010)
- **Finding:** After the user opens `config.json` in their editor from the recovery view and fixes a typo, the spec has no transition back to edit view — the user must go all the way back to the landing page.
- **Resolution:** After a successful external-editor invocation from the recovery view, the builder auto-reparses. If parsing succeeds, it transitions directly to the edit view; if not, it stays in recovery with the updated error.
- **Resolved by:** evidence
- **Affected decisions:** D36
- **Affected tech-notes:** —
- **Changed in spec:** Alternate Flows — Parse-error recovery

## F11: Unsaved-quit "save" choice encountering fatal findings is undefined

- **Agent:** user-experience-designer (UX-011)
- **Finding:** User picks quit, picks "save" in the unsaved-changes dialog, validator returns fatals — the spec doesn't describe what happens. Dialog still open? Findings panel over the dialog?
- **Resolution:** The quit is cancelled, the dialog is dismissed, and the user returns to the edit view with the findings panel open. Escape in the dialog equals cancel.
- **Resolved by:** evidence
- **Affected decisions:** D40
- **Affected tech-notes:** —
- **Changed in spec:** Primary Flow step 10; Alternate Flows — Unsaved-changes quit

## F12: Collapsible section behavior unspecified

- **Agent:** user-experience-designer (UX-012)
- **Finding:** Outline sections are "collapsible" but initial state, collapse behavior with cursor inside, item-count visibility, and detail-pane response are all unspecified.
- **Resolution:** All sections start expanded on first load; item count chip always visible regardless of collapse; collapsing with the cursor inside moves the cursor to the section header; detail pane shows a section summary when a header is focused; collapse state is per-session, not persisted across sessions.
- **Resolved by:** evidence
- **Affected decisions:** D28
- **Affected tech-notes:** —
- **Changed in spec:** Primary Flow step 5

## F13: Copy-default-to-local has no progress indicator or partial-failure handling

- **Agent:** user-experience-designer (UX-013)
- **Finding:** The copy touches the filesystem synchronously. A slow filesystem produces an unexplained pause; a partial failure (disk full after some files) leaves the user in edit view with missing files.
- **Resolution:** Show a progress status during copy; on any copy failure, roll back the partial copy, return to landing with an error naming the failure, and do not enter edit view with a partial bundle.
- **Resolved by:** evidence
- **Affected decisions:** D32; D15 (scope of what gets copied)
- **Affected tech-notes:** —
- **Changed in spec:** Alternate Flows — Copy-default-to-local

## F14: Open-in-editor affordance has no discoverability story

- **Agent:** user-experience-designer (UX-014)
- **Finding:** Spec says the affordance exists on prompt/script path fields but doesn't say what the user sees to know a key will open the file in their editor.
- **Resolution:** When focus is on a prompt-or-script-path field, the shortcut footer shows the open-in-editor key. The shortcut is present regardless of whether the file exists on disk today.
- **Resolved by:** evidence (covered by D24's shortcut footer commitment)
- **Affected decisions:** D24
- **Affected tech-notes:** —
- **Changed in spec:** Primary Flow step 7; User Interactions — Affordances

## F15: Outline scrollability unspecified

- **Agent:** user-experience-designer (UX-016)
- **Finding:** A workflow with many steps exceeds viewport height; no scroll model specified.
- **Resolution:** Outline is independently scrollable with a visible scroll-position indicator. Keyboard navigation auto-scrolls to keep the focused item visible.
- **Resolved by:** evidence (existing `bubbles/viewport` pattern)
- **Affected decisions:** D29
- **Affected tech-notes:** —
- **Changed in spec:** Primary Flow step 5

## F16: Warning-acknowledgment loop risks warning fatigue

- **Agent:** user-experience-designer (UX-017)
- **Finding:** If every save with the same warning forces a fresh acknowledgment, users click through without reading — a textbook nagging → fatigue pattern.
- **Resolution:** A warning acknowledged during a session is not surfaced again at the acknowledgment dialog for the remainder of that session. It still appears in the findings panel the user can open manually.
- **Resolved by:** user input (Q-H option A)
- **Affected decisions:** D23
- **Affected tech-notes:** —
- **Changed in spec:** Primary Flow step 9

## F17: Mechanics "leaking" into T1/T2

- **Agent:** junior-developer (F1)
- **Finding:** T1 and T2 name specific OS syscalls (`rename`, `fsync`) and framework messages; feels prescriptive in a behavioral spec.
- **Resolution:** **Rejected.** The plan-a-feature skill's operating principles explicitly define T-notes as the home for load-bearing mechanics that are not discoverable from the existing codebase — and pr9k has neither an atomic-save nor a Bubble Tea terminal-handoff precedent. The correct response is to keep the mechanics in T-notes where the skill expects them; however, the prescriptive tone was softened so the notes describe indicative approaches rather than mandates. The behavioral contract in the spec proper stands on its own without the T-links.
- **Resolved by:** project-manager synthesis (pushing back with evidence)
- **Affected decisions:** —
- **Affected tech-notes:** T1, T2 (tone softened)
- **Changed in spec:** —

## F18: Read-only detection timing — open in edit mode and fail at save?

- **Agent:** junior-developer (F3)
- **Finding:** Spec implied a read-only target can still be opened in edit view, with save failing later — but that's a broken promise for the affordance.
- **Resolution:** Read-only targets open in a dedicated **browse-only mode** — same layout as edit view, but the save affordance is absent (not merely disabled), unsaved-change tracking is off, and a clear read-only indicator is in the session header.
- **Resolved by:** evidence
- **Affected decisions:** D30, D4 (updated)
- **Affected tech-notes:** —
- **Changed in spec:** Primary Flow step 3; Alternate Flows — Read-only target

## F19: "Same flags as the main command" is imprecise

- **Agent:** junior-developer (F4)
- **Finding:** The main command also accepts `--iterations`; "same flags" literally would wire that up to `pr9k workflow`, which is meaningless.
- **Resolution:** Name the inherited flags explicitly: `--workflow-dir` and `--project-dir`. The subcommand does not accept `--iterations` or other run-scoped flags. Cobra-global `--version` and `-h`/`--help` remain.
- **Resolved by:** evidence
- **Affected decisions:** D19
- **Affected tech-notes:** —
- **Changed in spec:** Actors and Triggers

## F20: Narrow-reading ADR citation missing

- **Agent:** junior-developer (F5)
- **Finding:** The edit view encodes the three phase names. Without an explicit ADR citation, a reviewer might flag this as Ralph-specific knowledge; an implementer might be uncertain how far phase-structure knowledge can go in Go code.
- **Resolution:** D11 updated to explicitly cite the narrow-reading ADR's "pr9k owns" list (which names the three phase names) and clarify that rendering phases in the outline is an expression of that ownership, not a violation.
- **Resolved by:** evidence (ADR `docs/adr/20260410170952-narrow-reading-principle.md`)
- **Affected decisions:** D11
- **Affected tech-notes:** —
- **Changed in spec:** —

## F21: Version bump not specified

- **Agent:** junior-developer (F6); devops-engineer (Finding 1)
- **Finding:** Adding `pr9k workflow` extends the public CLI surface. `docs/coding-standards/versioning.md` requires an explicit version bump; the spec said nothing.
- **Resolution:** Spec's new Versioning section states that the feature ships with a patch-version bump per the 0.y.z rules, and the feature PR includes the version bump commit.
- **Resolved by:** evidence (`docs/coding-standards/versioning.md`)
- **Affected decisions:** D37
- **Affected tech-notes:** —
- **Changed in spec:** Versioning

## F22: Documentation obligations not enumerated

- **Agent:** junior-developer (F7); devops-engineer (Finding 2)
- **Finding:** `docs/coding-standards/documentation.md` requires docs to ship with the feature, not as follow-up PRs. The spec listed no concrete doc artifacts.
- **Resolution:** Spec's new Documentation Obligations section enumerates: `docs/features/workflow-builder.md`; two how-to guides; an ADR for the atomic-save pattern; a code-package doc for any new Go package; updates to `docs/features/cli-configuration.md`, `CLAUDE.md`, and `docs/architecture.md`.
- **Resolved by:** evidence (`docs/coding-standards/documentation.md`)
- **Affected decisions:** D38
- **Affected tech-notes:** —
- **Changed in spec:** Documentation Obligations

## F23: In-memory edits across landing round-trip — scoped correctly?

- **Agent:** junior-developer (F8)
- **Finding:** An edge-case row committed the builder to preserving unsaved edits across a landing-page round-trip when the target directory disappears. That's a non-trivial state-machine commitment for a narrow case.
- **Resolution:** Remove the cross-landing preservation. When the target disappears at save, the user remains in the edit view with the in-memory state intact and can retry or manually copy edits elsewhere. The landing page always starts fresh.
- **Resolved by:** user input (Q-F option B)
- **Affected decisions:** D41
- **Affected tech-notes:** —
- **Changed in spec:** Edge Cases table (target directory deleted row)

## F24: Target-resolution edge cases

- **Agent:** edge-case-explorer (category 1)
- **Finding:** Edge cases including: `--workflow-dir` pointing at a symlink (resolved by existing `EvalSymlinks`), at a regular file (errors before landing; UX of that error path unspecified), at a device file, at a directory traversable but not readable (EACCES would be mislabeled as parse error); both flags pointing at the same directory (identity case in copy); running `pr9k workflow` from inside a workflow directory; `--project-dir` unresolvable.
- **Resolution:** Covered across edge-case table rows, `D17` (symlink handling), `D4` (writability check for all target choices), and the read error vs. parse error distinction. `EACCES` on read surfaces as a dedicated "configuration file cannot be read" state, not the parse-error recovery view.
- **Resolved by:** evidence
- **Affected decisions:** D4, D17
- **Affected tech-notes:** —
- **Changed in spec:** Edge Cases table

## F25: Config-load edge cases

- **Agent:** edge-case-explorer (category 2)
- **Finding:** Empty file, UTF-8 BOM, non-UTF-8 encoding, duplicate JSON keys, trailing content after JSON, multi-megabyte configs. Go's standard JSON decoder is silent about several of these; the builder must not inherit that silence.
- **Resolution:** `D43` specifies load-time integrity checks for each — BOM stripped with banner; non-UTF-8 refuses parse with human-readable message; empty file enters recovery view with empty-file note; duplicate keys surfaced in non-blocking banner; trailing content surfaced in non-blocking banner with drop-on-save warning.
- **Resolved by:** evidence
- **Affected decisions:** D43
- **Affected tech-notes:** —
- **Changed in spec:** Edge Cases table; Alternate Flows — Parse-error recovery

## F26: Companion-file edge cases (non-executable scripts, directories-as-files, line endings)

- **Agent:** edge-case-explorer (category 3 — non-security subset)
- **Finding:** Script exists but lacks execute bit or shebang; prompt file path is actually a directory; very large prompt files; CRLF line endings.
- **Resolution:** `D21` extends the validator with a "referenced script must be executable with valid shebang" fatal check plus a "chmod +x" offer in the detail pane when only the execute bit is missing. Prompt-path-is-a-directory is surfaced as a fatal finding. Line endings pass through — the external editor owns normalization. Large files are implementation-bounded (no in-TUI rendering of prompt content).
- **Resolved by:** user input (Q-E option A + chmod offer)
- **Affected decisions:** D21
- **Affected tech-notes:** —
- **Changed in spec:** Edge Cases table (script and prompt-as-directory rows)

## F27: Symlink escape in companion files

- **Agent:** edge-case-explorer (category 3 — security subset); adversarial-security-analyst (F4)
- **Finding:** A symlink at `prompts/foo.md` pointing outside the bundle tree can redirect an external-editor open and a builder-initiated write to an arbitrary file. Existing `safePromptPath` does not `EvalSymlinks` before its containment check, and no equivalent check exists for scripts.
- **Resolution:** `D17` — the builder surfaces symlinks in a banner during the session and requires confirmation on first save. Saves to symlinked targets write through the symlink rather than replacing it. The validator's companion-file containment check will be hardened to `EvalSymlinks`-then-check (captured as an implementation-plan concern since it is a validator-internal change).
- **Resolved by:** user input (Q-B option C)
- **Affected decisions:** D17
- **Affected tech-notes:** T1 (save-through-symlink semantics)
- **Changed in spec:** Alternate Flows — Symlinked target or companion file; Edge Cases table

## F28: Save-semantics edge cases

- **Agent:** edge-case-explorer (category 4)
- **Finding:** Cross-device rename, ENOSPC mid-save, target directory removed between validate and save, target file is a symlink, read-only filesystem discovered only at save, concurrent save by a second builder instance.
- **Resolution:** Cross-device prevented by T1's same-directory temp-file invariant. ENOSPC: builder removes any partial temp file and preserves in-memory state. Target removed: `D41` conflict dialog. Symlink target: see F27 / D17 (save through). Read-only discovered at save: `D4` extended to apply writability check to all target choices. Concurrent: `D41` mtime collision signal.
- **Resolved by:** evidence
- **Affected decisions:** D4, D41
- **Affected tech-notes:** T1
- **Changed in spec:** Edge Cases table

## F29: External-editor edge cases

- **Agent:** edge-case-explorer (category 5)
- **Finding:** `$VISUAL` with arguments; `$VISUAL` empty string; GUI editor that daemonizes and exits zero immediately (user sees no edits applied); editor hung indefinitely; editor killed by SIGKILL; editor writes null bytes.
- **Resolution:** `D33` specifies shell-style word splitting with metacharacter rejection (handles `VISUAL="vim -u NONE"`) and treats empty string as unset. Daemonizing editors documented as known limitation with guidance (`code --wait`, `nvim`, etc.). Hung editor: user may SIGINT; if editor ignores, must kill from another session — documented. SIGKILL: re-read from disk per T2; terminal restore is best-effort. Null bytes in prompt content pass through; structured fields sanitize (D42).
- **Resolved by:** evidence
- **Affected decisions:** D33, D42
- **Affected tech-notes:** T2
- **Changed in spec:** Alternate Flows — External-editor invocation, Editor binary cannot be spawned; Edge Cases table; Documentation Obligations (external-editor how-to guide)

## F30: Validation API takes a path, not a struct

- **Agent:** edge-case-explorer (category 6 — `6-C`)
- **Finding:** The existing validator reads `config.json` itself from a workflow directory. The spec's "validator sees exactly what will be written" commitment, combined with T1's atomic save, rules out the naive "write first, validate second" approach — creates a TOCTOU window.
- **Resolution:** Captured as load-bearing mechanic in `T3`. Two feasible approaches (extend API to accept an in-memory struct, or validate via a private scratch directory) — implementation plan picks between them without writing to the real target.
- **Resolved by:** evidence
- **Affected decisions:** D14
- **Affected tech-notes:** T3 (new)
- **Changed in spec:** Primary Flow step 9; Coordinations — Workflow configuration validator

## F31: Input-time edge cases (paste, IME, long values, resize)

- **Agent:** edge-case-explorer (category 7)
- **Finding:** Multi-line paste into single-line input; ANSI-escape paste; very long values; IME composing state during focus change; mouse click during drag animation; terminal resize while dropdown open.
- **Resolution:** `D42` — paste sanitization (strip newlines and ANSI escapes at input time), soft length cap with visible warning. Dropdown re-layout on resize (edge-case table row). IME passes through; documented as a known limitation for complex IMEs.
- **Resolved by:** evidence
- **Affected decisions:** D42
- **Affected tech-notes:** —
- **Changed in spec:** Primary Flow step 7; Edge Cases table

## F32: Step-reorder edge cases

- **Agent:** edge-case-explorer (category 8)
- **Finding:** Cross-phase drag; drag invalidating downstream `skipIfCaptureEmpty` or `{{VAR}}` reference; findings panel references a step by index vs. name after reorder.
- **Resolution:** `D34` — cross-phase drag out of scope; drag stops at phase boundaries. Reorder-induced invalid references flagged by validator at save (not blocked at input time). Findings identify steps by name, not index — resolved in T3's approach.
- **Resolved by:** evidence
- **Affected decisions:** D34
- **Affected tech-notes:** —
- **Changed in spec:** Primary Flow step 7; Edge Cases table

## F33: Process-lifecycle edge cases

- **Agent:** edge-case-explorer (category 9)
- **Finding:** SIGTERM mid-save, SIGHUP, SIGTSTP, OS sleep/resume, external editor corrupts terminal state on crash.
- **Resolution:** `D44` specifies signal handling. T1 durability covers SIGTERM mid-save. SIGHUP loses unsaved changes (documented with `nohup` / multiplexer mitigation). SIGTSTP releases terminal and re-renders on resume. T2 handles editor-corruption cases on a best-effort basis.
- **Resolved by:** evidence
- **Affected decisions:** D44
- **Affected tech-notes:** T1, T2
- **Changed in spec:** Edge Cases table

## F34: Cross-session file mutations

- **Agent:** edge-case-explorer (category 10)
- **Finding:** `config.json` modified on disk between load and save; target directory deleted during editor invocation; companion prompt file deleted during external edit.
- **Resolution:** `D41` specifies mtime/size snapshot at load and conflict dialog at save. Target-directory-deleted: save fails with clear error, in-memory state preserved. Companion-file-deleted during editor: re-read produces an error surfaced in the detail pane as the "referenced file not found" state.
- **Resolved by:** evidence
- **Affected decisions:** D41
- **Affected tech-notes:** —
- **Changed in spec:** Edge Cases table; Alternate Flows — External-editor invocation

## F35: Attacker-controlled `--workflow-dir` → later-execution risk

- **Agent:** adversarial-security-analyst (F1)
- **Finding:** A user who runs `pr9k workflow --workflow-dir /tmp/evil/` on an attacker-placed bundle and then later runs `pr9k --workflow-dir /tmp/evil/` gets arbitrary code execution at their privilege level. The spec had no friction or warning for external bundles.
- **Resolution:** `D22` — external-workflow banner during the entire session when the target is outside the user's project directory and outside their home directory; explicit confirmation required at the first save.
- **Resolved by:** user input (Q-G option A)
- **Affected decisions:** D22
- **Affected tech-notes:** —
- **Changed in spec:** Primary Flow step 3; Alternate Flows — External-workflow session

## F36: `$EDITOR` invocation model unspecified

- **Agent:** adversarial-security-analyst (F2)
- **Finding:** T2 didn't specify whether `$EDITOR` is shell-split (command injection surface) or exec-split (binary-planting surface with relative paths). Either admits privilege abuse from a compromised dotfiles / `.env` setup.
- **Resolution:** `D33` — shell-style word splitting with metacharacter rejection; reject relative paths that don't resolve on `PATH`; invoke directly via `exec`, never `sh -c`.
- **Resolved by:** evidence
- **Affected decisions:** D33
- **Affected tech-notes:** T2
- **Changed in spec:** Alternate Flows — External-editor invocation, Editor binary cannot be spawned

## F37: `containerEnv` secret values exposed in UI and potentially logs

- **Agent:** adversarial-security-analyst (F3)
- **Finding:** The existing validator warns on secret-named `containerEnv` keys but is non-fatal and the builder had no masking requirement. A value typed into a `_TOKEN`-named field renders on screen and lands verbatim in `config.json` committed to the repo.
- **Resolution:** `D20` — mask values for secret-named keys by default in the detail pane with a reveal toggle; findings-panel entries for secret keys never echo the value.
- **Resolved by:** user input (Q-D option A)
- **Affected decisions:** D20
- **Affected tech-notes:** —
- **Changed in spec:** Primary Flow step 7

## F38: TOCTOU on atomic-save temp file

- **Agent:** adversarial-security-analyst (F5)
- **Finding:** In a world-writable workflow directory (e.g., `/tmp/`), an attacker who can predict the temp-file name (PID- or timestamp-based) and create a symlink at that path could redirect the builder's write to an arbitrary file.
- **Resolution:** T1's indicative approach explicitly calls for exclusive-create of the temp file (no overwriting a pre-existing path) and sibling-same-directory location. The implementation plan must honor both invariants.
- **Resolved by:** evidence
- **Affected decisions:** —
- **Affected tech-notes:** T1
- **Changed in spec:** —

## F39: Default bundle editing silently affects all users of a shared install

- **Agent:** adversarial-security-analyst (F7); devops-engineer (Finding 4, multi-user sub-case)
- **Finding:** On a shared install (e.g., `/usr/local/lib/pr9k/workflow/` writable by group `staff` on macOS Homebrew), one user saving the default silently changes it for all users. Also: overlay-filesystem silent discard on container environments; source-repo context ambiguity.
- **Resolution:** `D39` — session header shows a "shared install" banner when saving to a directory not owned by the current user. Observability: session events logged to `.pr9k/logs/`. Overlay-filesystem case documented as a known limitation with recommendation to use copy-to-local.
- **Resolved by:** evidence
- **Affected decisions:** D39
- **Affected tech-notes:** —
- **Changed in spec:** Edge Cases table; Coordinations — Session log

## F40: Read-only browse does not execute anything

- **Agent:** adversarial-security-analyst (F8)
- **Finding:** Confirmation: opening a workflow the user can read but not write does not execute any of its scripts. The builder is passive in read-only mode.
- **Resolution:** No change needed. Spec explicitly out-scopes execution (D9, D10).
- **Resolved by:** evidence
- **Affected decisions:** —
- **Affected tech-notes:** —
- **Changed in spec:** —

## F41: Last-write-wins for concurrent saves is implicit

- **Agent:** adversarial-security-analyst (F9); edge-case-explorer (4-E)
- **Finding:** Two builder sessions saving to the same file silently overwrite each other; the builder has no locking.
- **Resolution:** `D41` — mtime/size snapshot catches one of the two sessions at save time and shows a conflict dialog. The other session (the one that saved first) completes normally. Multi-session locking remains out of scope.
- **Resolved by:** user input (recommendation accepted)
- **Affected decisions:** D41
- **Affected tech-notes:** T1
- **Changed in spec:** Edge Cases table; Out of Scope

## F42: `.tmp` cleanup contract missing

- **Agent:** devops-engineer (Finding 3)
- **Finding:** T1 says cleanup is "on next save," but a user who opens the builder and quits without saving never triggers it. The spec had no load-time detection.
- **Resolution:** `D42-a` — on opening a target directory, the builder scans for leftover temp files, surfaces a non-blocking notice, and offers delete-or-leave. Never auto-deletes without consent.
- **Resolved by:** evidence
- **Affected decisions:** D42-a
- **Affected tech-notes:** T1
- **Changed in spec:** Alternate Flows — Crash-era temporary file on open

## F43: No observability contract

- **Agent:** devops-engineer (Finding 5)
- **Finding:** The builder's failure states (parse error, save failure, editor spawn failure, target deletion) have no logging commitment. Post-hoc diagnosis impossible.
- **Resolution:** `D39` specifies logging session-level events (session start, target, saves with outcomes, editor invocations with exit codes, quits) to the same `.pr9k/logs/` location the main `pr9k` uses.
- **Resolved by:** evidence
- **Affected decisions:** D39
- **Affected tech-notes:** —
- **Changed in spec:** Coordinations — Session log

## F44: Schema version mismatch on round-trip

- **Agent:** devops-engineer (Finding 6)
- **Finding:** A `config.json` written by a newer pr9k may contain fields the builder's schema model doesn't know. On save, those fields would silently disappear.
- **Resolution:** `D18` — on load, non-blocking banner lists unrecognized fields and warns that saving will drop them. On save, only recognized fields are written.
- **Resolved by:** user input (Q-C option B — warn on load, drop on save)
- **Affected decisions:** D18
- **Affected tech-notes:** —
- **Changed in spec:** Alternate Flows — Unknown-field warning; Out of Scope

## F45: Test strategy for T1 / T2 / TUI modes absent

- **Agent:** devops-engineer (Finding 7)
- **Finding:** Both T1 and T2 are new patterns. Without explicit test commitments in the spec, they ship untested — a T2 bug leaves the user's terminal corrupted post-quit.
- **Resolution:** `D41-b` and spec's new Testing section commit to: T1 injection tests simulating write failure; T2 injection-based editor-runner interface (matching existing `sandboxCreateDeps` pattern); Bubble Tea model-update unit tests for every TUI mode; `-race` required.
- **Resolved by:** evidence (existing DI pattern at `src/cmd/pr9k/sandbox_create.go:19`; testing standard)
- **Affected decisions:** D41-b
- **Affected tech-notes:** T1, T2
- **Changed in spec:** Testing

## F46: Supply-chain acknowledgment missing

- **Agent:** devops-engineer (Finding 9)
- **Finding:** Workflows written by the builder and shared (via git, etc.) carry no integrity attestation. The spec did not acknowledge the gap.
- **Resolution:** One-sentence note added to Out of Scope clarifying that integrity attestation (SBOM, signing) is v1 out-of-scope and users who share bundles rely on the receiving repo's CI / review process.
- **Resolved by:** evidence
- **Affected decisions:** —
- **Affected tech-notes:** —
- **Changed in spec:** Out of Scope

## F47: Resource bounds on large workflows

- **Agent:** devops-engineer (Finding 10)
- **Finding:** A pathological config (thousands of steps) has no stated rendering bounds; the TUI could render the entire outline and stall.
- **Resolution:** Outline scrollability (`D29`) plus keyboard-navigation auto-scroll handles the common case. Hard ceilings (step count, file size caps) are deferred to the implementation plan — the spec's "soft length cap" commitment on structured-field inputs (`D42`) is the only spec-level bound.
- **Resolved by:** evidence
- **Affected decisions:** D29, D42
- **Affected tech-notes:** —
- **Changed in spec:** Primary Flow step 5; Edge Cases table
