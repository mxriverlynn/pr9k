# Implementation Decision Log: Workflow Builder PR-2 (`pr9k workflow` TUI delivery)

<!--
This file records every implementation decision committed for PR-2 of the
workflow-builder feature. Behavioral and implementation statements live in
[../feature-implementation-plan.md](../feature-implementation-plan.md) — this
file captures the question, rationale, evidence, and rejected alternatives for
each decision. Round-by-round history lives in
[implementation-iteration-history.md](implementation-iteration-history.md).

PR-2 inherits the full D-1..D-47 decision set from the original
[../../workflow-builder/artifacts/implementation-decision-log.md](../../workflow-builder/artifacts/implementation-decision-log.md).
The decisions below are PR-2-specific (D-PR2-N): they refine, supersede, or add
to the inherited set in response to the gap analysis and the two rounds of
specialist review documented in the iteration history.

Cross-referencing invariants:
- `Driven by rounds:` — R# IDs from [implementation-iteration-history.md](implementation-iteration-history.md)
  that added or changed this decision (R1, R2, or Synthesis).
- `Dependent decisions:` — D# IDs of later decisions that rest on this one.
- `Referenced in plan:` — sections of [../feature-implementation-plan.md](../feature-implementation-plan.md)
  that cite this decision with an inline parenthetical link.
-->

## D-PR2-1: Cherry-pick + gap-close strategy with WU-PR2-0 triage table as pre-implementation work

- **Question:** Given the backup tag `backup/workflow-builder-mode-full-2026-04-24` preserves ~4,140 LOC of `workflowedit/` and `cmd/pr9k/editor.go` as the original PR-2 draft, but the gap analysis identifies ~50 numbered gaps against that draft, should PR-2 cherry-pick the backup and close gaps, or rewrite from scratch using the backup as a reference?
- **Decision:** Adopt **cherry-pick + gap-close**. A pre-implementation work unit (WU-PR2-0) produces a function-level triage table classifying every backup file's exported symbols as `as-is` (cherry-pick unchanged), `modify` (cherry-pick and patch), or `replace` (re-author). The triage table itself is committed as a PR-2 artifact (`docs/plans/workflow-builder-pr2/artifacts/cherry-pick-triage.md`) before the first cherry-pick commit lands.
- **Rationale:** R-PR2-001 confirms the file decomposition matches D-1; R-PR2-002 confirms the `EditorRunner` shape is improved over D-6's pseudocode; R-PR2-004 confirms `Update` routing matches D-9 exactly. The architecture is sound; the gaps are wiring, missing handlers, and missed schema fields — not a redesign. Re-authoring would discard correct prior work and risk regression in the modes already proven during PR-1.
- **Evidence:** `/tmp/pr9k-pr2-planning/round1-specialist-outputs.md` R-PR2-001..R-PR2-010; backup tag manifest (4,140 LOC across 27 files); gap analysis at `../../workflow-builder/implementation-gaps.md` (50 numbered gaps, of which OQ-PR2-1 closed GAP-036 as false positive, GAP-052 reduces to a version bump).
- **Rejected alternatives:**
  - **Rewrite from scratch using backup as reference** — rejected because it discards the EditorRunner refactor (R-PR2-002), the Update routing decomposition (R-PR2-004), and the dialog-handler skeleton (R-PR2-010), forcing a rebuild of decisions already made and validated.
  - **Cherry-pick wholesale without per-function triage** — rejected because R-PR2-008 (15 vs 14 DialogKind constants), BEH-009 (DialogUnsavedChanges Discard quits builder), and SEC-003 (`$` rejection) are bugs in the backup that must be addressed at cherry-pick time, not after.
- **Specialist owner:** `software-architect`
- **Revisit criterion:** If the WU-PR2-0 triage table classifies more than 30% of backup functions as `replace`, re-evaluate against the rewrite alternative.
- **Dissent (if any):** None.
- **Driven by rounds:** R1 (raised by JrQ-PR2-001/002/010), R2 (PM resolution OQ-PR2-5), Synthesis (committed)
- **Dependent decisions:** D-PR2-2 (commit graph), D-PR2-9 (gap triage)
- **Referenced in plan:** Implementation Approach > Architecture and Integration Points; Decomposition and Sequencing (WU-PR2-0)

## D-PR2-2: Commit graph with un-hide as the last revertable commit

- **Question:** What is the PR-2 commit sequence such that every intermediate commit passes `make ci` and the un-hide is independently revertable?
- **Decision:** Commit sequence: (1) WU-PR2-0 cherry-pick triage table (artifact-only, no code); (2) **WU-PR2-1 version bump 0.7.2 → 0.7.3** (per D-PR2-3); (3) WU-PR2-2 `workflowmodel` schema additions (`UnknownFields`, top-level `Env`, top-level `ContainerEnv`, `DefaultModel`, step-level `env`/`containerEnv`) with round-trip tests (per D-PR2-4); (4) WU-PR2-3 cherry-pick the `workflowedit` package skeleton with `cmd/pr9k/editor.go`; (5) WU-PR2-4..WU-PR2-12 gap-closure commit series, one commit per gap cluster (each green under `make ci`); (6) **WU-PR2-13 un-hide `pr9k workflow` + extend `rename_guard_test.go`** as the last named commit.
- **Rationale:** DOR-002 flagged that the original plan's rollback story was "revert the full PR" — too coarse. Making the un-hide its own commit lets reviewers verify wiring independently and lets the team revert the user-facing surface without losing the schema additions or atomic-write standard. JrQ-PR2-009 reinforces this: every intermediate commit must be green so bisect remains useful.
- **Evidence:** DOR-002 in `/tmp/pr9k-pr2-planning/round1-specialist-outputs.md`; JrQ-PR2-009; `docs/coding-standards/versioning.md` (version-bump-as-its-own-commit pattern).
- **Rejected alternatives:**
  - **Single squash commit** — rejected because it makes the un-hide irreversible without reverting the schema additions, blocks bisect, and conflicts with `make ci` per-commit guarantee.
  - **Un-hide first, gap-closure after** — rejected because it ships a known-broken user-facing surface to `main` for the duration of the gap-closure series. Violates "ship working software" outcome.
  - **Version bump last** — rejected because D-18 commits version bump as the **first** commit of the feature PR; landing schema or behavior changes before the version bump misrepresents the wire-protocol delta.
- **Specialist owner:** `devops-engineer`
- **Revisit criterion:** If the gap-closure commit series exceeds ~20 commits, re-evaluate cluster boundaries to keep the PR reviewable.
- **Dissent (if any):** None.
- **Driven by rounds:** R1 (DOR-002, JrQ-PR2-009), R2 (PM resolution OQ-PR2-6), Synthesis (committed)
- **Dependent decisions:** D-PR2-3, D-PR2-4
- **Referenced in plan:** Decomposition and Sequencing; Operational Readiness > Rollout; Operational Readiness > Rollback

## D-PR2-3: Version bump 0.7.2 → 0.7.3 as the first commit of PR-2

- **Question:** Does PR-2 require a version bump, and if so, where does it land in the commit graph?
- **Decision:** PR-2 bumps `internal/version.Version` from `0.7.2` → `0.7.3` as its **first** commit. The bump is a patch increment because pr9k is in `0.y.z` — under `docs/coding-standards/versioning.md`, every public-API addition during `0.y.z` is a patch bump. The PR is merged with `--no-ff` to preserve the version-bump commit boundary, matching D-18.
- **Rationale:** Un-hiding `pr9k workflow` adds a new cobra subcommand, a new `--workflow-dir`/`--project-dir` flag pair scoped to that subcommand, and a new `config.json` consumer — all part of pr9k's public API per `docs/coding-standards/versioning.md`. DOR-001 confirmed the version is stale at `0.7.2` (the value PR-1 left in place). The version bump must precede the un-hide so `pr9k --version` reports the correct number for the surface the un-hide makes reachable.
- **Evidence:** `src/internal/version/version.go:7` currently `0.7.2`; `docs/coding-standards/versioning.md` "0.y.z rules"; D-18 inherited.
- **Rejected alternatives:**
  - **Minor bump (0.7.2 → 0.8.0)** — rejected because pr9k is pre-1.0 (`0.y.z`) where every public-surface addition is a patch bump until 1.0 is declared; minor bumps are reserved for breaking changes pre-1.0 per the standards file.
  - **Defer to a follow-up PR** — rejected because un-hiding the subcommand without a version delta makes `pr9k --version` lie about what the binary exposes.
  - **Land in the middle of the series** — rejected per D-18 (version bump is the **first** commit).
- **Specialist owner:** `devops-engineer`
- **Revisit criterion:** If a breaking schema change is forced into PR-2 (none planned), bump to 0.8.0 instead.
- **Dissent (if any):** None.
- **Driven by rounds:** R1 (DOR-001, JrQ-PR2-008), R2 (PM resolution OQ-PR2-6), Synthesis (committed)
- **Dependent decisions:** D-PR2-2
- **Referenced in plan:** Decomposition and Sequencing (WU-PR2-1); Operational Readiness > Versioning

## D-PR2-4: workflowmodel schema extensions land before the workflowedit cherry-pick

- **Question:** GAP-038 (top-level `env`), GAP-039 (`defaultModel`), GAP-040 (step-level `env`/`containerEnv`), and GAP-041 (`UnknownFields`) all describe `WorkflowDoc` schema fields that are referenced by the backup `workflowedit` code but absent from PR-1's `workflowmodel.WorkflowDoc`. Where do these land in the commit graph, and who owns the round-trip test?
- **Decision:** WU-PR2-2 (the second commit after the version bump) extends `workflowmodel.WorkflowDoc` with: `UnknownFields map[string]json.RawMessage`, top-level `Env []EnvEntry`, top-level `ContainerEnv []EnvEntry`, `DefaultModel string`, and per-step `Env []EnvEntry` / `ContainerEnv []EnvEntry`. The same commit ships round-trip marshal tests against the bundled default `workflow/config.json` asserting that load → save → load is byte-equivalent (modulo legitimate normalization). `workflowmodel` owns the test.
- **Rationale:** EC-PR2-004 [P0] is a data-corruption finding: a user opening any `config.json` with a top-level `env` block and saving it via the builder loses that block silently. JrQ-PR2-005 confirms the cherry-pick of `workflowedit` cannot compile against PR-1's `main` until these fields exist (the backup references them). Landing schema additions before the cherry-pick means the cherry-pick commit compiles as soon as it lands.
- **Evidence:** `src/internal/workflowmodel/model.go:64-68` (current shape, missing fields); GAP-038/039/040/041; EC-PR2-004 [P0]; JrQ-PR2-005; validator Category 10 already validates env/containerEnv at the top level — the schema knows about these fields, only the in-memory model is missing them.
- **Rejected alternatives:**
  - **Land schema additions inside the workflowedit cherry-pick commit** — rejected because it bundles unrelated changes and makes the cherry-pick irrevocable independently.
  - **Defer schema additions to a follow-up PR** — rejected because EC-PR2-004 is data corruption: any user who opens-then-saves a workflow with these fields loses data on day one.
  - **Use `interface{}` for unknown fields** — rejected because `json.RawMessage` preserves byte-equivalence on round-trip; `interface{}` re-serializes through `json.Marshal` which can change ordering and float encoding (D63 `IsDirty` regression risk per EC-PR2-010).
- **Specialist owner:** `software-architect` (schema), `test-engineer` (round-trip tests)
- **Revisit criterion:** If a future field addition cannot be made round-trip stable with `json.RawMessage`, revisit the unknown-field type.
- **Dissent (if any):** None.
- **Driven by rounds:** R1 (EC-PR2-004, JrQ-PR2-005, JrQ-PR2-006), Synthesis (committed)
- **Dependent decisions:** D-PR2-1, D-PR2-2, D-PR2-19 (IsDirty replaces ad-hoc dirty mutations)
- **Referenced in plan:** Implementation Approach > Data Model and Persistence; Decomposition and Sequencing (WU-PR2-2); Testing Strategy

## D-PR2-5: Editor message types live in `internal/workflowedit/editor.go`

- **Question:** The backup declares `EditorExitMsg`, `EditorSigintMsg`, and `EditorRestoreFailedMsg` in `cmd/pr9k/editor.go` (`package main`), preventing `workflowedit.Model.Update` from type-switching on them (BEH-002, SEC-001). Where should these types live?
- **Decision:** Move the three message types into `internal/workflowedit/editor.go` as exported types: `workflowedit.EditorExitMsg`, `workflowedit.EditorSigintMsg`, `workflowedit.EditorRestoreFailedMsg`. The `cmd/pr9k/editor.go` `makeExecCallback` constructs them by package-qualified name, since it already imports `workflowedit` to satisfy the `EditorRunner` interface contract.
- **Rationale:** This is a Dependency Inversion Principle correction — the high-level module (`workflowedit`, which owns `Model.Update`) cannot type-switch on its own domain events because the types were owned by `cmd/pr9k` (entry point). Moving the types into `workflowedit` lets `Model.Update` consume them without an envelope or type-erasure step. The backup's `ExecCallback` already imports `workflowedit` for `EditorRunner`, so the dependency direction is unchanged; only the message-type ownership flips.
- **Evidence:** `/tmp/pr9k-pr2-planning/round2-outputs-and-resolutions.md` software-architect R2 verdict; backup `cmd/pr9k/editor.go`; backup `internal/workflowedit/editor.go`; D-7 (ExecCallback three-way switch).
- **Rejected alternatives:**
  - **Wrap in a generic `EditorResultMsg` envelope** — rejected as double indirection: `Update` would type-switch on the envelope's discriminator, which is the same problem at a different layer.
  - **Define an interface with concrete types still in `cmd/pr9k`** — rejected as the same DIP violation re-labeled; `workflowedit.Model.Update` would still fail to handle its own domain events directly.
  - **Define the messages in a shared `internal/workflowmsg` package** — rejected as over-engineering; the messages have one consumer (`workflowedit`) and one producer (`cmd/pr9k`), so the natural home is the consumer.
- **Specialist owner:** `software-architect`
- **Revisit criterion:** If a future caller of `EditorRunner` needs to construct these messages from outside `cmd/pr9k`, revisit; until then the consumer-owned location is correct.
- **Dissent (if any):** None.
- **Driven by rounds:** R1 (BEH-002, SEC-001, R-PR2-D), R2 (architect re-engagement; D-PR2-R2-2 confirmed), Synthesis (committed)
- **Dependent decisions:** D-PR2-15 (editor message handlers in Update)
- **Referenced in plan:** Implementation Approach > External Interfaces; Implementation Approach > Runtime Behavior

## D-PR2-6: rejectShellMeta set is `{backtick, semicolon, pipe, newline}`

- **Question:** Backup `cmd/pr9k/editor.go:84` rejects `` ` `` `;` `|` `$` `\n` (5 chars). SEC-003 flags `$` as a D33 violation; SEC-008 flags `&`/`<`/`>` as missing per D33. What is the correct set?
- **Decision:** The rejection set is exactly four characters: backtick, semicolon, pipe, newline. Drop `$` (D33 forbids rejecting it because `VISUAL='$HOME/bin/myvim'` must work under direct exec). Do **not** add `&`/`<`/`>` — under `exec.Command` the shell is bypassed entirely, so these characters are inert and rejecting them adds no security value while breaking legitimate paths like `Stata/StataSE&Mata` that contain `&`.
- **Rationale:** R2 security re-engagement confirmed the backup's rejection set has both errors. The corrected set codifies the actual threat model: characters that retain interpretation despite `exec.Command` bypassing `/bin/sh` are zero (since `exec.Command` does not invoke a shell at all). The four characters in the corrected set are defense-in-depth — `;` and `|` cannot be expanded by `exec.Command` but are universally interpretable as command separators by users who edit `$VISUAL` and may copy-paste from shell examples; rejecting them prevents footguns. `\n` rejection prevents accidental multi-line entries. Backtick rejection prevents users from pasting command-substitution examples expecting them to work.
- **Evidence:** `/tmp/pr9k-pr2-planning/round2-outputs-and-resolutions.md` security R2 verdict; backup `cmd/pr9k/editor.go:84`; D33 (spec decision-log); `os/exec` Go documentation (`exec.Command` does not invoke a shell).
- **Rejected alternatives:**
  - **Keep `$` rejected (preserve backup behavior)** — rejected: `$VISUAL='$HOME/bin/myvim'` is a legitimate user configuration and D33 explicitly permits `$` since direct exec does not interpret variables.
  - **Add `&`/`<`/`>` to the rejection set** — rejected: under direct exec these are inert; rejecting them adds no security value while breaking legitimate paths (e.g., applications with `&` in their name).
  - **Reject all metacharacters in the POSIX shell grammar** — rejected as security theater that breaks legitimate paths.
- **Specialist owner:** `adversarial-security-analyst`
- **Revisit criterion:** If pr9k ever invokes `$VISUAL` through `/bin/sh` (it does not today), expand the set to match D33 fully.
- **Dissent (if any):** None.
- **Driven by rounds:** R1 (SEC-003, SEC-008), R2 (security re-engagement; D-PR2-R2-4 confirmed), Synthesis (committed)
- **Dependent decisions:** None
- **Referenced in plan:** Security Posture > $VISUAL/$EDITOR word-splitting

## D-PR2-7: D-8 dialog-state set updated to enumerate 14 named DialogKind constants

- **Question:** R-PR2-008 flagged that the backup has 15 `DialogKind` iota values vs D-8's claim of 14. What is the authoritative set?
- **Decision:** D-8 is updated to enumerate exactly 14 **named** non-zero `DialogKind` constants (15 iota values total including `DialogNone`): `DialogPathPicker`, `DialogNewChoice`, `DialogUnsavedChanges`, `DialogQuitConfirm`, `DialogExternalEditorOpening`, `DialogFindingsPanel`, `DialogError`, `DialogCrashTempNotice`, `DialogFirstSaveConfirm`, `DialogRemoveConfirm`, `DialogFileConflict`, `DialogSaveInProgress`, `DialogRecovery`, `DialogAcknowledgeFindings`. The original D-8 entry in the inherited decision log undercounted because three constants (`DialogSaveInProgress`, `DialogRecovery`, `DialogAcknowledgeFindings`) were added during plan iteration after D-8 was committed.
- **Rationale:** R2 architect re-engagement confirmed all 14 named values. PR-2 inherits all of them; the divergence was documentation drift, not implementation drift. Updating D-8 (rather than dropping constants) is the correct correction because every constant has a referenced behavior in either the spec or a later inherited decision (D-13 for `DialogSaveInProgress`; D43 for `DialogRecovery`; D-9/D72 for `DialogAcknowledgeFindings`).
- **Evidence:** Backup `internal/workflowedit/dialogs.go` (15 iota values); `/tmp/pr9k-pr2-planning/round2-outputs-and-resolutions.md` software-architect R2 verdict; inherited D-8 in `../../workflow-builder/artifacts/implementation-decision-log.md`.
- **Rejected alternatives:**
  - **Drop three constants to match the original D-8 count** — rejected because each constant has a documented behavior; dropping them would re-introduce the gaps (BEH-007, GAP-022..025) that motivated their addition.
  - **Renumber to a different organization (e.g., grouped enums)** — rejected as over-engineering; the iota set is sufficient and the `DialogKind` switch reads naturally.
- **Specialist owner:** `software-architect`
- **Revisit criterion:** If the dialog set crosses 20 constants, evaluate splitting into sub-enums by lifecycle (transient vs persistent dialogs).
- **Dissent (if any):** None.
- **Driven by rounds:** R1 (R-PR2-008), R2 (architect re-engagement; D-PR2-R2-3 confirmed), Synthesis (committed)
- **Dependent decisions:** D-PR2-15, D-PR2-16
- **Referenced in plan:** Implementation Approach > Runtime Behavior > Dialog state

## D-PR2-8: External-editor shortcut for prompt/script-path fields is `Ctrl+E`

- **Question:** Spec §7 last bullet says "a visible shortcut in the footer when a prompt-or-script-path field is focused" invokes the external editor on the focused file, but the spec does not pin a specific key. The backup's how-to docs (per UX R2) bind this action to `Ctrl+O` — a collision with File > Open in the R2 menu redesign. What key invokes the external editor?
- **Decision:** Use `Ctrl+E` (mnemonic: "edit") for "open the focused prompt/script in the external editor." `Ctrl+E` does not collide with any existing global shortcut: `Ctrl+N`/`O`/`S`/`Q` are File-menu actions. `Ctrl+E` is reserved as a footer-only contextual shortcut active when a prompt-path or script-path field has focus.
- **Rationale:** UX R2 audit identified the `Ctrl+O` collision in both how-to docs as the only material defect. `Ctrl+E` is unambiguous (no Mac/Linux terminal collision in Bubble Tea's standard input set), mnemonic ("edit"), and contextual (only visible when relevant). The two backup how-to paragraphs that use `Ctrl+O` for this action are rewritten to `Ctrl+E` as part of the cherry-pick.
- **Evidence:** `/tmp/pr9k-pr2-planning/round2-outputs-and-resolutions.md` UX R2 verdict; spec §7 last bullet; D70 (File > Open binds to `Ctrl+O`).
- **Rejected alternatives:**
  - **`Ctrl+O`** (backup default) — rejected: collides with File > Open per D70.
  - **`F2`** — rejected: less discoverable than `Ctrl+E`; pr9k already uses `F10` for menu activation, adding a second F-key is inconsistent.
  - **`e` (single character)** — rejected: collides with normal text input in detail-pane editing.
  - **No shortcut, only mouse / menu-driven** — rejected: spec §7 explicitly commits to a footer shortcut.
- **Specialist owner:** `user-experience-designer`
- **Revisit criterion:** If a future field type (e.g., a JSON snippet) wants its own external-editor shortcut, pick a different key — `Ctrl+E` is owned by prompt/script-path fields.
- **Dissent (if any):** None.
- **Driven by rounds:** R1 (JrQ-PR2-007), R2 (UX re-engagement; D-PR2-R2-1 confirmed), Synthesis (committed)
- **Dependent decisions:** None
- **Referenced in plan:** Implementation Approach > Runtime Behavior > External editor handoff; Specialist Handoffs (UX backup-doc rewrite)

## D-PR2-9: `?` help modal silently suppressed in non-findings dialogs

- **Question:** Spec §8 says the help modal is "unconditionally reachable from the edit view regardless of any other configuration." D-8 R2-C1 (in the inherited log) says `helpOpen` flips `true` only when `dialog.kind == DialogFindingsPanel` and is silently suppressed for all other dialog kinds. EC-PR2-008 surfaces this contradiction.
- **Decision:** Accept D-8 R2-C1: `?` is silently suppressed during all non-findings-panel dialog states. The spec §8 sentence is updated to read "from the edit view or the findings panel, regardless of any other configuration." The shortcut footer for every dialog state must enumerate the available options in full so users have guidance without needing `?`.
- **Rationale:** The UX rationale in D-8 (one-overlay-at-a-time, Escape-pops-one-layer) is sound for keyboard ergonomics. Spec §8's phrasing "regardless of other configuration" was written before D-8 established the overlay model. Allowing `?` to stack over every dialog (Option B) would force every dialog's `updateDialog` handler to forward `?` to `updateHelpModal`, multiplying the layer-management contract. The footer-enumeration requirement (every dialog spells out its actions) gives users equivalent discoverability without the layering complexity.
- **Evidence:** Spec Primary Flow §8; inherited D-8 R2-C1; EC-PR2-008.
- **Rejected alternatives:**
  - **Option B (spec-strict): `?` works during all dialogs; every dialog forwards it to `updateHelpModal`** — rejected as multiplying surface area for marginal UX gain (every dialog already shows its actions in the footer per the dialog template).
  - **Defer to vNext** — rejected because the contradiction is in shipped artifacts; leaving it ambiguous risks inconsistent implementation in this PR.
- **Specialist owner:** `user-experience-designer`
- **Revisit criterion:** If a user research finding shows discoverability is genuinely reduced by the suppression, revisit and consider Option B.
- **Dissent (if any):** None.
- **Driven by rounds:** R1 (EC-PR2-008), R2 (PM resolution OQ-PR2-7), Synthesis (committed)
- **Dependent decisions:** None
- **Referenced in plan:** Implementation Approach > Runtime Behavior > Dialog state; Definition of Done (spec §8 update)

## D-PR2-10: QuitConfirm always shows after pendingQuit successful save

- **Question:** Backup `model.go:771-789` returns `tea.Quit` directly when a save with `pendingQuit=true` completes successfully — bypassing `DialogQuitConfirm`. D73 (inherited spec decision) says the builder always confirms before exiting. BEH-001 surfaces this as a D73 violation.
- **Decision:** D73 is authoritative. After a successful save with `pendingQuit=true`, `handleSaveResult` re-routes to `handleGlobalKey(Ctrl+Q)`. Because `saveInProgress=false` and `dirty=false` at that point, `handleGlobalKey` opens the no-unsaved-changes `DialogQuitConfirm` (two-option Yes/No, No default). The user explicitly confirms exit. The backup's direct `tea.Quit` is a bug PR-2 fixes.
- **Rationale:** D73 commits to "the builder always confirms before exiting. Two dialog shapes" — there is no exception path for pendingQuit. The save flow's invariant (every save produces user-visible feedback before process exit) is preserved either way; D73's invariant (every exit is user-confirmed) is only preserved by re-entering the quit flow. The cost of the extra confirmation is one keystroke; the benefit is a consistent exit story regardless of how the user reached it.
- **Evidence:** Spec D73; backup `model.go:771-789`; BEH-001 in R1 outputs; inherited save flow §step 9; tui-mode-coverage.md mode 21 ("standard quit-flow enters").
- **Rejected alternatives:**
  - **Direct `tea.Quit` after successful save with pendingQuit (backup behavior)** — rejected: D73 violation.
  - **Skip QuitConfirm but show a brief "Saved — exiting" banner** — rejected: still bypasses confirmation; D73 says "always confirm."
  - **Update D73 to add a pendingQuit exception** — rejected: D73's value is its uniform invariant; carving exceptions complicates the user model.
- **Specialist owner:** `behavioral-analyst`
- **Revisit criterion:** If user feedback indicates the extra confirmation after pendingQuit is consistently disruptive, revisit D73.
- **Dissent (if any):** None.
- **Driven by rounds:** R1 (BEH-001), R2 (PM resolution OQ-PR2-8), Synthesis (committed)
- **Dependent decisions:** D-PR2-15 (handleSaveResult re-routing)
- **Referenced in plan:** Implementation Approach > Runtime Behavior > Save flow > step 9; Implementation Approach > Runtime Behavior > Quit interaction

## D-PR2-11: Gap triage classification — 16 P0 blockers, ~30 functional-completeness, 6 deferred, 3 implicit, 1 false-positive

- **Question:** The gap analysis identifies ~50 numbered gaps. Without classification, PR-2 has no bounded scope.
- **Decision:** Adopt the PM's starter classification:
  - **PR-2-required (P0 blockers, 16):** GAP-001, GAP-002, GAP-003, GAP-020, GAP-029, GAP-033, GAP-038, GAP-039, GAP-040, GAP-041, EC-PR2-001, EC-PR2-003, EC-PR2-004, SEC-006, SEC-001, SEC-007.
  - **PR-2-required (functional completeness, ~30):** GAP-005, GAP-006, GAP-007, GAP-008, GAP-009, GAP-010, GAP-013, GAP-014, GAP-015, GAP-016, GAP-017, GAP-018, GAP-019, GAP-021, GAP-022, GAP-023, GAP-024, GAP-025, GAP-027, GAP-028, GAP-030, GAP-031, GAP-034, GAP-035, GAP-037, GAP-046, GAP-045, all P1 EC findings (EC-PR2-006..014), all SEC findings (SEC-002..005, SEC-008..011), all R-PR2 findings, all BEH findings, all CV-PR2 findings, all TST findings.
  - **Deferred to vNext (file issues, 6):** GAP-011 (gripper always-visible glyph), GAP-026 (no-op save feedback), GAP-032 (per-mode help modal detail), GAP-047 (footer context-sensitivity refinement), GAP-048 (statusLine `RefreshIntervalSeconds 0 vs omitempty`), EC-PR2-015 (probe name PID collision after SIGKILL).
  - **Implicitly resolved or accepted (3):** GAP-043 (DI-1/DI-2 reactivated by un-hide commit), GAP-050 (file naming clarified), GAP-052 (version bump is WU-PR2-1).
  - **False positive — closed (1):** GAP-036 (companion key convention — see D-PR2-12).
- **Rationale:** The classification ties every gap to either a PR-2 commit, a deferred-issue ticket, or a documented closure. It bounds scope (P0 blockers must close to ship; vNext gaps are filed but do not block) and gives the team an enforceable Definition of Done.
- **Evidence:** Gap list at `../../workflow-builder/implementation-gaps.md`; specialist priority tags from edge-case-explorer (P0/P1/P2); BEH/SEC/CV/UX P0 labels.
- **Rejected alternatives:**
  - **Defer all functional-completeness gaps to vNext** — rejected: the resulting "PR-2" would un-hide a subcommand whose user-visible behavior is incomplete (no path picker, no Add affordance, no detail-pane editing). That contradicts the outcome ("user can edit, validate, and save without hand-editing JSON").
  - **Lift everything to P0** — rejected: the six vNext-deferred items are genuinely cosmetic or edge-case (e.g., probe name PID collision after SIGKILL is a multi-precondition rare event) and can ship as follow-up issues without compromising the outcome.
- **Specialist owner:** `project-manager`
- **Revisit criterion:** If a deferred gap surfaces as a user-blocking incident in dogfooding, promote to PR-2-required.
- **Dissent (if any):** None.
- **Driven by rounds:** R1 (JrQ-PR2-002, JrQ-PR2-010), R2 (PM resolution OQ-PR2-9), Synthesis (committed)
- **Dependent decisions:** D-PR2-1 (cherry-pick + gap-close strategy)
- **Referenced in plan:** Decomposition and Sequencing; Definition of Done

## D-PR2-12: GAP-036 (companion key convention) is a false positive — closed

- **Question:** EC-PR2-005 [P0] claims the validator uses bare `step.PromptFile` as the companion-files map key while `workflowio.Load` uses `filepath.Join("prompts", step.PromptFile)` — a T3 contract violation. JrQ-PR2-003 disputes the claim.
- **Decision:** Close GAP-036 and EC-PR2-005 as false positives. The validator (`src/internal/validator/validator.go` lines 644, 667, 802) and `workflowio.Load` (`src/internal/workflowio/load.go:119`) both use `filepath.Join("prompts", step.PromptFile)` as the cache key. T3 is satisfied. No PR-2 work is needed in this area.
- **Rationale:** R2 behavioral-analyst code inspection confirmed both code paths use the same key format. The validator's doc comment on `ValidateDoc` (lines 166-168) explicitly states "Companion files keyed by path relative to workflowDir … Keys must use the full relative path — bare filenames like 'step-1.md' are cache misses and fall through to disk (F-121)." The contract is documented and enforced.
- **Evidence:** `/tmp/pr9k-pr2-planning/round2-outputs-and-resolutions.md` behavioral-analyst R2 verdict; `src/internal/validator/validator.go:644,667,802`; `src/internal/workflowio/load.go:119`; F-121 in inherited review-findings.md.
- **Rejected alternatives:**
  - **Bare-filename key convention** — rejected: would require changing both the validator and the loader; would re-open a T3 gap in PR-1 code that is already correct.
  - **Add a regression test** — accepted incidentally; PR-1 already pins the convention with a test per F-121.
- **Specialist owner:** `behavioral-analyst`
- **Revisit criterion:** If a future feature wants to lookup companion bytes by bare filename, add a parallel API rather than changing the key.
- **Dissent (if any):** None.
- **Driven by rounds:** R1 (EC-PR2-005 vs JrQ-PR2-003 dispute), R2 (behavioral re-engagement; OQ-PR2-1 / OQ-PR2-4 resolution), Synthesis (committed)
- **Dependent decisions:** None
- **Referenced in plan:** Implementation Approach > Data Model and Persistence (T3 satisfied); RAID Log (closed)

## D-PR2-13: Goroutine ctx propagation — Option B with Bubble Tea-runtime exemption

- **Question:** Inherited D-33 commits to "every non-stdlib goroutine receives `ctx` and selects on `ctx.Done()`." Backup tea.Cmd closures (scanMatches, validate, save) do not take ctx. CV-PR2-002 surfaces this as a gap; it can be resolved by Option A (document Bubble Tea goroutines as exempt) or Option B (pass ctx into closures with select at blocking points).
- **Decision:** **Option B** with documented exemption: every closure the builder spawns inside a `tea.Cmd` accepts a `ctx context.Context` parameter and selects on `ctx.Done()` at every blocking call (file I/O, validator, save, tab-completion). The Bubble Tea runtime's own goroutines (the tick scheduler, the reader goroutine on stdin) are exempt by construction — the builder does not own them. The exemption is documented in `docs/code-packages/workflowedit.md` so future contributors do not interpret D-33 as requiring ctx threading into framework internals.
- **Rationale:** D-33 intent is that the builder's own concurrency surface is cancellable; framework-owned goroutines are out of scope by definition. Threading ctx into the closures preserves correctness on slow filesystems (NFS/FUSE) where a stuck save closure would otherwise hold a goroutine past program exit until the OS forcibly closed file descriptors. The runtime-exemption documentation prevents the standard from being read as "patch Bubble Tea."
- **Evidence:** D-33 inherited; CV-PR2-002 in R1 outputs; `docs/coding-standards/concurrency.md` "every goroutine receives ctx"; Bubble Tea v1.3.10 source.
- **Rejected alternatives:**
  - **Option A (declare all Bubble Tea-runtime goroutines exempt; closures don't take ctx)** — rejected: the closures are owned by the builder, not Bubble Tea; making them exempt removes the cancellation guarantee for the surface PR-2 owns.
  - **Pass ctx via a Model field rather than closure parameter** — rejected: ctx in a struct is a known anti-pattern (`go vet` flags it); closure parameter is idiomatic.
- **Specialist owner:** `concurrency-analyst`
- **Revisit criterion:** If future profiling shows leaked goroutines after `program.Run()` returns, audit closures for missing `ctx.Done()` selects.
- **Dissent (if any):** None.
- **Driven by rounds:** R1 (CV-PR2-002), R2 (PM resolution OQ-PR2-10), Synthesis (committed)
- **Dependent decisions:** D-PR2-22 (drain pattern after program.Run)
- **Referenced in plan:** Implementation Approach > Runtime Behavior > Goroutine lifecycle; Testing Strategy

## D-PR2-14: editorInProgress flag gates Ctrl+Q during the external-editor window

- **Question:** EC-PR2-001 [P0]: during a `tea.ExecProcess` window (terminal released, editor running), a SIGINT delivered to the builder process or a Ctrl+Q processed before the editor's own input handler claims the terminal can kill the builder while the terminal is still in released state, leaving the user with a corrupted terminal and no `RestoreTerminal` call.
- **Decision:** `Model` carries a private `editorInProgress bool` field, set to `true` when `launchEditorMsg` returns the `tea.ExecProcess` `tea.Cmd` and cleared by the three-way `ExecCallback` switch (any of `editorExitMsg`, `editorSigintMsg`, `editorRestoreFailedMsg`). While `editorInProgress` is true, `handleGlobalKey(Ctrl+Q)` and the SIGINT-delivered `quitMsg` set `pendingQuit=true` and return — they do **not** call `tea.Quit` directly. The editor's own SIGINT handling (via D-7's exit-code-130 branch) drives the eventual quit, ensuring `RestoreTerminal` runs.
- **Rationale:** The Bubble Tea runtime guarantees `RestoreTerminal` is called when the `tea.Cmd` returned by `ExecProcess` completes — but only if the program is still running. A `tea.Quit` issued during the ExecProcess window short-circuits the runtime before `RestoreTerminal` can run, leaving the terminal in alt-screen-released state. Gating on `editorInProgress` defers the quit until after `RestoreTerminal` has run, preserving the terminal-restoration invariant.
- **Evidence:** EC-PR2-001 [P0]; T2 (terminal handoff) in `../../workflow-builder/artifacts/feature-technical-notes.md`; D-7 ExecCallback branches.
- **Rejected alternatives:**
  - **Trust the SIGINT branch in D-7 to handle this** — rejected: D-7 routes exit-130 to `quitMsg`, but a Ctrl+Q delivered to the builder (not the editor) is a separate path that does not go through D-7.
  - **Block all keystrokes during ExecProcess via a Model.View dimming overlay** — rejected: the terminal is released; the builder's `View()` is not on screen during the window. There is nothing for the user to see; the gate must live in `Update`.
  - **Defer to vNext** — rejected: terminal corruption is a P0 user-visible failure mode.
- **Specialist owner:** `concurrency-analyst` (signal handling) jointly with `software-architect` (Model state)
- **Revisit criterion:** If a future Bubble Tea release changes the `RestoreTerminal` ordering, revalidate.
- **Dissent (if any):** None.
- **Driven by rounds:** R1 (EC-PR2-001), Synthesis (committed)
- **Dependent decisions:** D-PR2-15
- **Referenced in plan:** Implementation Approach > Runtime Behavior > External editor handoff; Security Posture > SIGINT during editor window

## D-PR2-15: pendingAction discriminated type replaces pendingQuit bool for unsaved-changes auto-resume

- **Question:** UX-009/BEH-009 surface that the backup uses a single `pendingQuit bool` to track "user initiated an action that requires saving first," conflating File>New / File>Open / Quit. After a successful save, the auto-resume only fires the quit flow — File>New and File>Open are silently dropped (the user's intent destroyed). EC-PR2-003 [P0] adds: even when the right action is fired, fatals during save leave the state machine in the wrong phase.
- **Decision:** Replace `pendingQuit bool` with a discriminated `pendingAction` type:
  ```go
  type pendingAction interface{ isPendingAction() }
  type pendingActionQuit struct{}
  type pendingActionNew struct{ choice newChoiceKind }
  type pendingActionOpen struct{ targetPath string }
  ```
  After a successful save with `pendingAction != nil`, `handleSaveResult` dispatches by type: `pendingActionQuit` re-enters the quit flow (per D-PR2-10); `pendingActionNew` opens `DialogNewChoice` with the captured `choice`; `pendingActionOpen` opens the path picker with the captured `targetPath`. After save with fatals, `pendingAction` is **cleared** and the state machine remains on `DialogFindingsPanel` (no auto-resume — the user must address fatals first).
- **Rationale:** A bool conflating three actions cannot route to the right resumption. The discriminated type makes intent explicit and preserves user data (in-memory state). The clear-on-fatals rule prevents the state machine from re-firing pendingActionNew (which would discard in-memory state) when the user has not yet acknowledged that the save failed.
- **Evidence:** UX-009, BEH-009 in R1 outputs; EC-PR2-003 [P0]; spec D72 (unsaved-changes Save / Discard / Cancel three-option semantics).
- **Rejected alternatives:**
  - **Three separate bool flags (pendingQuit, pendingNew, pendingOpen)** — rejected as a known anti-pattern (booleans-conflated-as-state); invalid combinations (two flags true) become representable.
  - **Keep pendingQuit bool, add a separate "newChoice" field** — rejected as the same anti-pattern with a wider surface.
  - **Don't auto-resume — always return to edit view after save** — rejected: spec D72 commits to the auto-resume semantics ("after Save action, the originally-requested operation proceeds").
- **Specialist owner:** `behavioral-analyst` (state machine) jointly with `user-experience-designer` (auto-resume UX)
- **Revisit criterion:** If a fourth pending-action case emerges (e.g., import-from-URL deferred to vNext), evaluate whether the discriminator pattern still scales.
- **Dissent (if any):** None.
- **Driven by rounds:** R1 (UX-009, BEH-009, EC-PR2-003), Synthesis (committed)
- **Dependent decisions:** D-PR2-10, D-PR2-14
- **Referenced in plan:** Implementation Approach > Runtime Behavior > Save flow; Implementation Approach > Runtime Behavior > Quit interaction

## D-PR2-16: Logger injected into Model via workflowDeps; D27 events fire at all D39 trigger points

- **Question:** SEC-007, DOR-003, GAP-033, BEH-010 all flag that the backup `Model` has no logger field; D-27's session-event catalog (9 trigger points) is unreachable. D-44 commits the log directory; D-15 commits the prefix; nothing wires the logger into `Model`.
- **Decision:** `cmd/pr9k/workflow.go` constructs a `*logger.Logger` via `logger.NewLoggerWithPrefix(projectDir, "workflow")` and passes it to `workflowedit.New` via the `workflowDeps` struct. `Model` carries `logger *logger.Logger` as a private field and emits `log.Log("workflow-builder", line)` at every D-27 trigger point: session start, file open, file save (success), file save (failure with `reason=<short>`), external editor invoked, external editor exit, dialog acknowledged, recovery view shown, session end. The `reason=<short>` enumeration is the closed set from D-27 (`validator_fatals | permission_error | disk_full | cross_device | conflict_detected | symlink_escape | target_not_regular_file | parse_error | other`).
- **Rationale:** Without a logger field, every D-27 trigger point is unreachable; without the trigger points, R7 (containerEnv secret leak via logs) cannot be tested, and DOR-007 (R7 secret-leak test absent) remains open. Injecting via `workflowDeps` matches the existing DI pattern for `SaveFS` and `EditorRunner` and keeps `workflowedit` decoupled from logger construction.
- **Evidence:** SEC-007, DOR-003 (R7 secret-leak), GAP-033, BEH-010, D-27 trigger points (inherited), D-44 log directory.
- **Rejected alternatives:**
  - **Global logger** — rejected: hardcoded global state breaks tests (`fakeLogger` injection becomes impossible per `docs/coding-standards/testing.md`).
  - **Pass logger as a `tea.Msg`** — rejected: misuses the message-passing layer for setup-time DI.
  - **Defer session logging to vNext** — rejected: R7 (secret leak) is a security risk that must be testable in PR-2.
- **Specialist owner:** `devops-engineer` (logger contract) jointly with `adversarial-security-analyst` (R7 mitigation)
- **Revisit criterion:** If the trigger-point set grows past 12 events, consider promoting to a `SessionLogger` interface.
- **Dissent (if any):** None.
- **Driven by rounds:** R1 (SEC-007, DOR-003, GAP-033, BEH-010), Synthesis (committed)
- **Dependent decisions:** None
- **Referenced in plan:** Implementation Approach > External Interfaces > Logger; Operational Readiness > Observability; Security Posture > Session-event logging field exclusion

## D-PR2-17: Conflict-detection dialog handlers for DialogFileConflict / DialogFirstSaveConfirm / DialogCrashTempNotice / DialogRecovery

- **Question:** BEH-007 and GAP-022..025 flag that four DialogKind constants have either no `updateDialog` handler or an Esc-only handler that infinite-loops on Overwrite/Reload selection. The dialogs are reachable but uncloseable.
- **Decision:** Each of the four dialog kinds gets a complete `updateDialog<Kind>` handler:
  - **`DialogFileConflict`** — Overwrite → bypasses snapshot check on the next save; Reload → discards in-memory state and re-loads from disk; Cancel → closes dialog, returns to edit view, preserves in-memory state.
  - **`DialogFirstSaveConfirm`** — Confirm → proceeds to save (sets a session-flag suppressing future first-save confirms); Cancel → closes dialog, save aborted.
  - **`DialogCrashTempNotice`** — Acknowledge → closes dialog, records `ackSet[promptFile]=true` so the same notice does not re-fire this session; Recover → opens the temp file in recovery view; Discard → deletes the temp file, then Acknowledge.
  - **`DialogRecovery`** — Open-in-editor → triggers `tea.ExecProcess` with the malformed file; Reload → re-attempts load (useful if user fixed externally); Discard → discards the malformed file (renames to `.bad`); Cancel → returns to empty-editor.
- **Rationale:** The dialogs already exist in the backup (D-PR2-7 confirms 14 named constants). What's missing is the `updateDialog` switch arm for each. Without these handlers, the dialogs render but every keystroke inside them either does nothing (BEH-007 infinite loop) or escapes via Esc only — making the dialogs useless for resolving the conditions that opened them.
- **Evidence:** BEH-007, GAP-022, GAP-023, GAP-024, GAP-025; backup `internal/workflowedit/model.go` updateDialog (incomplete); spec §"Edge Cases" rows for each scenario.
- **Rejected alternatives:**
  - **Single generic confirm handler with a closure** — rejected: each dialog has distinct actions and side-effects (Reload vs Recover vs Discard); a generic handler would reify the differences anyway.
  - **Defer DialogRecovery actions to vNext** — rejected: the recovery view is a P0 path (parse-error landing); without actions the user has no way out except `Esc → empty editor`, losing the file pointer.
- **Specialist owner:** `behavioral-analyst`
- **Revisit criterion:** If a fifth conflict scenario emerges, follow the same pattern.
- **Dissent (if any):** None.
- **Driven by rounds:** R1 (BEH-007, GAP-022..025), Synthesis (committed)
- **Dependent decisions:** D-PR2-7
- **Referenced in plan:** Implementation Approach > Runtime Behavior > Dialog state; Decomposition and Sequencing (gap-closure cluster)

## D-PR2-18: workflowmodel.IsDirty replaces ad-hoc m.dirty=true mutations

- **Question:** BEH-008 flags that the backup tracks dirty state via 8 scattered `m.dirty=true` mutation sites and never calls `workflowmodel.IsDirty(diskDoc, inMemoryDoc)`. EC-PR2-010 adds: when `IsDirty` is finally called, `reflect.DeepEqual` on `UnknownFields` (a `map[string]json.RawMessage`) yields false-positive dirty after load because `json.RawMessage` byte-slice comparison is order-sensitive.
- **Decision:** Replace every `m.dirty=true` mutation site with a single call site at the end of every `Update` invocation that mutates `m.doc`: `m.dirty = !workflowmodel.IsDirty(m.diskDoc, m.doc)` (or compute lazily on demand in `View()`). `workflowmodel.IsDirty` is the single source of truth. For the `UnknownFields` false-positive: `IsDirty` compares `UnknownFields` by sorted-key equality of `json.RawMessage` byte slices, not by `reflect.DeepEqual`.
- **Rationale:** Centralizing dirty-state computation in `workflowmodel` keeps the dirty-tracking contract testable and prevents the 8-mutation-site drift that produces false positives (e.g., editing a field then editing it back to its original value would leave `m.dirty=true` permanently in the backup). The sorted-key comparison handles `UnknownFields` correctly because JSON object key order is not semantically significant.
- **Evidence:** BEH-008; EC-PR2-010; D63 (inherited spec decision on dirty tracking); `workflowmodel.IsDirty` already exists per `docs/code-packages/workflowmodel.md`.
- **Rejected alternatives:**
  - **Keep ad-hoc mutation sites + add `IsDirty` only on save** — rejected: the View renders the unsaved-changes glyph based on `m.dirty`; if the bool is stale the glyph is wrong.
  - **Compute dirty lazily in View() only** — rejected: View runs every frame; recomputing IsDirty on every frame is wasteful for a 100-step workflow.
  - **Use `reflect.DeepEqual` for UnknownFields** — rejected: produces false-positive dirty on map iteration order (EC-PR2-010).
- **Specialist owner:** `behavioral-analyst`
- **Revisit criterion:** If `IsDirty` becomes a perf hotspot for very large workflows (>1000 steps), evaluate incremental dirty tracking.
- **Dissent (if any):** None.
- **Driven by rounds:** R1 (BEH-008, EC-PR2-010), Synthesis (committed)
- **Dependent decisions:** D-PR2-4 (UnknownFields type)
- **Referenced in plan:** Implementation Approach > Data Model and Persistence; Implementation Approach > Runtime Behavior > Save flow

## D-PR2-19: Routing pre-dispatch for validateCompleteMsg / saveCompleteMsg before dialog tier

- **Question:** CV-PR2-005 flags that when `DialogSaveInProgress` is open and a `validateCompleteMsg` or `saveCompleteMsg` arrives, the D-9 routing tiers (1) helpOpen → (2) dialog → (3) global key → (4) edit view send the message into `updateDialog`, where it is silently swallowed because the dialog handler does not type-switch on those message types.
- **Decision:** Update routing pre-dispatches `validateCompleteMsg` and `saveCompleteMsg` (and only those two types) to dedicated handlers `handleValidateComplete` / `handleSaveComplete` **before** the dialog tier. The pre-dispatch tier is added at the top of `Update`:
  ```
  (0) switch msg.(type) { case validateCompleteMsg, saveCompleteMsg → dispatch }
  (1) helpOpen
  (2) dialog
  (3) global key
  (4) edit view
  ```
  This preserves D-9's tier order for keystrokes while routing async-completion messages to handlers that own the save state machine (which can then transition out of `DialogSaveInProgress` correctly).
- **Rationale:** The dialog tier was designed for keystroke routing; async-completion messages are not keystrokes and should not flow through the same gate. The pre-dispatch tier keeps the save state machine the single owner of save-flow transitions, avoiding the "dialog handler accidentally owns save state" anti-pattern.
- **Evidence:** CV-PR2-005; backup `model.go` `updateEditView` message handling; D-9 Update routing (inherited).
- **Rejected alternatives:**
  - **Forward async messages from updateDialog to handleSaveComplete/handleValidateComplete** — rejected: spreads save-state ownership across dialogs, making the state machine hard to reason about.
  - **Drop the dialog when an async message arrives** — rejected: the dialog (`DialogSaveInProgress`) is exactly what the async message resolves; dropping it before the resolution is jarring.
- **Specialist owner:** `concurrency-analyst` jointly with `behavioral-analyst`
- **Revisit criterion:** If a third async-completion message type emerges (e.g., a future `pathCompletionMsg` redesign), evaluate whether the pre-dispatch tier should be made extensible.
- **Dissent (if any):** None.
- **Driven by rounds:** R1 (CV-PR2-005), Synthesis (committed)
- **Dependent decisions:** D-PR2-15
- **Referenced in plan:** Implementation Approach > Runtime Behavior > Save flow; Implementation Approach > Runtime Behavior > Dialog state

## D-PR2-20: Generation counter for path-completion tea.Cmd

- **Question:** CV-PR2-007 flags that a stale `pathCompletionMsg` from a prior keystroke can overwrite the user's live input if the user types fast enough. The async completion goroutine returns whatever it computed for the input it saw; if the user has typed more characters since, the result is stale.
- **Decision:** `Model` carries a private `pathCompletionGen uint64` counter. Each `Ctrl+I` (Tab) keystroke increments the counter and captures its value into the `tea.Cmd` closure as `gen`. When the closure returns `pathCompletionMsg{gen, completions}`, the receiver compares `gen` against the current counter and discards the message if they don't match.
- **Rationale:** The generation-counter pattern is the standard solution for "stale async result overwrites live state." It costs one `uint64` field and one comparison per message; in exchange, fast typing never overwrites with stale completions. The pattern is documented in `docs/coding-standards/concurrency.md` for similar use cases.
- **Evidence:** CV-PR2-007; backup `pathpicker.go:35-43`; backup `model.go:288-354` (path-picker keystroke handling).
- **Rejected alternatives:**
  - **Cancel the prior tea.Cmd via ctx** — rejected: tea.Cmd does not have a built-in cancel; the closure runs to completion on the goroutine and only the message dispatch is gated.
  - **Block input while completion is pending** — rejected: would freeze the picker on slow filesystems where completion takes >100ms.
  - **Keep the latest result regardless** — rejected: produces the bug.
- **Specialist owner:** `concurrency-analyst`
- **Revisit criterion:** If a future feature adds another async producer with the same staleness problem, factor the counter into a generic `genCounter` helper.
- **Dissent (if any):** None.
- **Driven by rounds:** R1 (CV-PR2-007), Synthesis (committed)
- **Dependent decisions:** None
- **Referenced in plan:** Implementation Approach > Runtime Behavior > Path picker

## D-PR2-21: Capture *tea.Program in local closure before signal goroutine starts

- **Question:** CV-PR2-001 flags that the signal handler in the backup is set up before `tea.NewProgram` is called, so the `program` reference inside the signal closure is the zero value at handler-installation time. The handler's later access to `program.Send(quitMsg{})` therefore races with `program` initialization.
- **Decision:** Restructure `runWorkflowBuilder` so that:
  1. `program := tea.NewProgram(model, opts...)` is called first.
  2. Only then does `signal.Notify(sigCh, ...)` get called and the goroutine spawned.
  3. The goroutine captures `program` by value in its closure (Go's closure capture is by reference for vars, but the var is initialized before the goroutine starts so the deref is race-free).
- **Rationale:** Goroutine-spawn-before-variable-initialization is a textbook race. Reordering installation removes the race entirely. The signal goroutine still needs `ctx.Done()` (D-PR2-13) for clean shutdown, but the program-reference race is closed independently.
- **Evidence:** CV-PR2-001; backup `cmd/pr9k/workflow.go:62-73`; D-34 signal-handler semantics (inherited).
- **Rejected alternatives:**
  - **Use an `atomic.Pointer[tea.Program]`** — rejected as over-engineering: reordering is simpler and removes the need for atomic load/store.
  - **Use a mutex on the program reference** — rejected as over-engineering and slower than the simple reorder.
- **Specialist owner:** `concurrency-analyst`
- **Revisit criterion:** If a future change introduces multiple `tea.NewProgram` instances per command (it does not today), revalidate.
- **Dissent (if any):** None.
- **Driven by rounds:** R1 (CV-PR2-001), Synthesis (committed)
- **Dependent decisions:** D-PR2-13, D-PR2-22
- **Referenced in plan:** Implementation Approach > Runtime Behavior > Signal handler

## D-PR2-22: Drain pattern after program.Run() returns

- **Question:** CV-PR2-008 flags that `tea.Cmd` closures spawned by `Update` can still be running after `program.Run()` returns. Without a drain, those goroutines may attempt to call `program.Send(...)` against a torn-down program (panic) or hold open file descriptors past process exit (descriptor leak).
- **Decision:** `runWorkflowBuilder` holds a `sync.WaitGroup` registered with every closure spawn (via a wrapper `spawn(fn func(ctx context.Context))` helper that increments the WG, calls `fn(ctx)`, and decrements). After `program.Run()` returns, `cancel()` is invoked (signaling closures to exit at their next ctx.Done check) and `wg.Wait()` blocks until they return. Only then does `runWorkflowBuilder` return.
- **Rationale:** The drain pattern is committed in `docs/coding-standards/concurrency.md` ("Wait for background goroutines"). Applying it to the builder closes a known leak window for slow filesystems where save closures can outlast `program.Run()`. The pattern composes cleanly with D-PR2-13 (ctx propagation) and D-PR2-21 (program-ref ordering).
- **Evidence:** CV-PR2-008; `docs/coding-standards/concurrency.md` "Wait for background goroutines"; D-33 inherited.
- **Rejected alternatives:**
  - **Trust ctx.Done() to drain on its own** — rejected: ctx cancellation signals goroutines but does not block `runWorkflowBuilder` until they exit.
  - **Use a channel-based "all done" signal** — rejected: WaitGroup is the standard idiom for this case.
- **Specialist owner:** `concurrency-analyst`
- **Revisit criterion:** If a closure deadlocks the drain (e.g., blocks on a channel send the program would have consumed), document the closure's contract — it must select on ctx.Done at every blocking call.
- **Dissent (if any):** None.
- **Driven by rounds:** R1 (CV-PR2-008), Synthesis (committed)
- **Dependent decisions:** D-PR2-13
- **Referenced in plan:** Implementation Approach > Runtime Behavior > Goroutine lifecycle; Testing Strategy

## D-PR2-23: 10s signal-handler timer cancellation when ctx.Done() fires

- **Question:** CV-PR2-009 flags that the 10-second fallback timer in the signal handler (D-34) leaks across test invocations because the timer is not stopped when the program exits cleanly. In repeated test runs, timers accumulate and the test binary holds them past process exit.
- **Decision:** The signal-handler goroutine wraps the 10-second timer in a `select`:
  ```go
  select {
  case <-time.After(10 * time.Second):
      os.Exit(130)  // hard fallback
  case <-ctx.Done():
      return  // clean exit; timer GC'd
  }
  ```
  When `runWorkflowBuilder`'s `cancel()` fires after `program.Run()` returns, `ctx.Done()` closes and the goroutine returns; the timer is released by the runtime.
- **Rationale:** `time.After` allocates a one-shot timer and channel; without a cancel path the goroutine waits the full 10 seconds, holding the timer alive. Adding `<-ctx.Done()` as a competing case lets the goroutine return immediately on clean shutdown, releasing the timer.
- **Evidence:** CV-PR2-009; D-34 signal-handler 10-second fallback (inherited); Go `time.After` documentation.
- **Rejected alternatives:**
  - **Use `time.NewTimer` and `Stop()` explicitly** — accepted as functionally equivalent; the `select`-with-ctx.Done is more idiomatic for the pattern.
  - **Don't fire the fallback at all** — rejected: D-34 commits to the 10-second hard floor.
- **Specialist owner:** `concurrency-analyst`
- **Revisit criterion:** If the test suite shows residual timer leaks, audit other long-lived `time.After` usages.
- **Dissent (if any):** None.
- **Driven by rounds:** R1 (CV-PR2-009), Synthesis (committed)
- **Dependent decisions:** D-PR2-21, D-PR2-22
- **Referenced in plan:** Implementation Approach > Runtime Behavior > Signal handler; Testing Strategy

<!-- End of decision log. New decisions append below as needed. -->
