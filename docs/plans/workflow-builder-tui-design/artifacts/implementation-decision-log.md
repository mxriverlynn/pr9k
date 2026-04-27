# Implementation Decision Log: Workflow-Builder TUI Visual-Design

<!--
This file records every implementation decision committed while planning the
workflow-builder TUI visual-design layer. Behavioral and implementation statements
live in [../feature-implementation-plan.md](../feature-implementation-plan.md);
this file captures the question, rationale, evidence, and rejected alternatives
for each decision. Round-by-round history lives in
[implementation-iteration-history.md](implementation-iteration-history.md).

Note: D1–D51 from the visual-spec [decision-log.md](decision-log.md) are committed
mechanics inherited by this plan and are NOT re-debated here. The decisions below
(D-1 ... D-26) are *implementation* decisions — choices about how to realize the
committed visual mechanics in Go code. They number from D-1 to keep the
implementation log self-contained; references to spec D# entries use the
"visual-spec D{N}" prefix or link to `decision-log.md`.
-->

## D-1: Render code stays in `internal/workflowedit` package, decomposed into per-surface render files

- **Question:** Should render code live in a sub-package (`internal/workflowedit/render/`) or stay in `internal/workflowedit` alongside `Update()`?
- **Decision:** Keep render code in `internal/workflowedit`. Decompose by surface into dedicated files: `render_frame.go`, `render_session_header.go`, `render_outline.go`, `render_detail.go`, `render_menu.go`, `render_dialogs.go`, `render_findings.go`, `render_help.go`, `render_empty.go`, `render_footer.go`.
- **Rationale:** `View()` is architecturally inseparable from `Update()` in Bubble Tea — the View function must read the same Model the Update function mutates. The `internal/ui/model.go:382-749` run-mode precedent keeps render in the same package as the model. A sub-package would require either (a) exposing internal Model fields publicly or (b) passing a large render-context struct on every call — both anti-patterns. Decomposition by surface keeps each file under ~200 lines and makes ownership obvious.
- **Evidence:** software-architect A1, A2; structural-analyst S1 (`model.go` is 1,831 lines, visual layer adds ~900 lines if absorbed in place — file becomes a high-churn convergence trap without decomposition); structural-analyst S8 (positive: `outline.go`/`detail.go` separation is structurally clean — visual additions go in the right files); `internal/ui/model.go:382-749` (run-mode precedent).
- **Rejected alternatives:**
  - Sub-package `internal/workflowedit/render/` — rejected because it requires either exposing Model internals or passing a large render-context on every call; over-abstraction relative to run-mode precedent. Evidence: A2.
  - Single monolithic `view.go` — rejected because it concentrates ~900 lines of churn in one file. Evidence: S1, S9 (15 dialog Update handlers already make `model.go` a high-churn area; co-locating render in same file compounds the trap).
- **Specialist owner:** `software-architect`
- **Revisit criterion:** If a single render file exceeds ~250 lines after the splits, decompose further along orthogonal axes (e.g., `render_dialogs_kinds.go` vs `render_dialogs_shell.go`).
- **Dissent (if any):** None.
- **Driven by rounds:** R-1
- **Dependent decisions:** D-2 (chrome primitives extraction is a dependency of this layout); D-3 (dialog rendering decomposition); D-4 (field rendering decomposition); D-9 (commit graph orders the file-by-file sequence).
- **Referenced in plan:** "Implementation Approach → Architecture and Integration Points"; "Decomposition and Sequencing".

## D-2: Extract chrome primitives to new `internal/uichrome` package

- **Question:** Where should the shared chrome primitives (`wrapLine`, `hruleLine`, `renderTopBorder`, `colorTitle`, `colorShortcutLine`, `overlay`, `spliceAt`) and the D45 palette additions live so both `internal/ui` and `internal/workflowedit` can consume them?
- **Decision:** Create new package `src/internal/uichrome/` exporting the primitives and the full palette (`LightGray`, `White`, `Green`, `Red`, `Yellow`, `Cyan`, `Dim`, `ActiveStepFG`, `ActiveMarkerFG`). Refactor `internal/ui/header.go` to alias the existing palette names from `uichrome`. Refactor `internal/ui/model.go` to delegate the primitive helpers to `uichrome`. `internal/workflowedit` imports `uichrome` directly.
- **Rationale:** Both TUIs (run-mode and workflow-builder) consume the same chrome primitives. Re-exporting from `internal/ui` would force `internal/workflowedit` to import the run-mode model package, dragging in unrelated dependencies and creating a fragile coupling. A shared primitive package puts both consumers at the same dependency depth. The narrow-reading ADR governs Go-code-vs-config separation, not intra-Go package structure (junior-developer Q2 closed this concern).
- **Evidence:** software-architect A1, A3; structural-analyst S2 (chrome primitives currently unexported in `internal/ui`); structural-analyst S6 (D45 palette additions exist in neither package — `internal/uichrome` is canonical placement); facilitation summary OD-3; junior-developer Q1, Q2; ADR `20260410170952-narrow-reading-principle.md` (does not govern intra-TUI package structure).
- **Rejected alternatives:**
  - Re-export from `internal/ui` — rejected because it forces `internal/workflowedit` to import the run-mode model package; risks import cycles; obscures source of truth. Evidence: software-architect A1, OD-3.
  - Duplicate primitives in `internal/workflowedit` — rejected because every future chrome change requires a coordinated multi-package edit; violates DRY. Evidence: facilitation summary FS-02.
  - Add to `internal/ansi` — rejected because `ansi` is a strict ANSI parser; mixing render primitives there violates package responsibility. Evidence: package responsibility convention.
- **Specialist owner:** `software-architect` / `structural-analyst`
- **Revisit criterion:** If a third TUI consumer arrives, re-evaluate primitive surface area (e.g., extract layout helpers separately).
- **Dissent (if any):** None.
- **Driven by rounds:** R-1
- **Dependent decisions:** D-3, D-4, D-5, D-19 (D48 fallback uses `uichrome` primitives); D-24 (`lipgloss.Width` semantics integrated where chrome computes widths).
- **Referenced in plan:** "Implementation Approach → Architecture and Integration Points"; "Decomposition and Sequencing → Commit 2".

## D-3: Dialog rendering — `renderDialogShell` + `dialogBodyFor` per-kind body builders

- **Question:** Should each of the 15 dialog kinds get its own type implementing a `Dialog` interface, or should rendering use a single switch with per-kind body-builder functions?
- **Decision:** Two-level structure: `renderDialogShell(body dialogBody, w, h int) string` renders the D36 chrome (border, title, footer) once. A per-kind builder `dialogBodyFor(kind, payload) dialogBody` produces the inner content. The main entry point `renderDialog(m Model)` switches on `m.dialog.kind` to pick the builder and feeds the result to the shell.
- **Rationale:** ISP — there is no behavioral polymorphism across dialog kinds; only render content varies. 15 separate types implementing a `Dialog` interface buy nothing and cost a per-kind file. The shell-vs-body split factors out the D36 chrome arithmetic (centering, width clamp, overlay splice) into one place.
- **Evidence:** software-architect A4; structural-analyst S3 (`renderDialog()` 50-line placeholder switch would balloon to 600–1000 lines without decomposition); structural-analyst S4 (D11/D28/D31 dropdown shapes are structurally identical; D36/D40 overlay shapes also unify — supports shared shell).
- **Rejected alternatives:**
  - `Dialog` interface with 15 implementations — rejected because there is no behavioral polymorphism; ISP violation in reverse. Evidence: A4.
  - One file per dialog kind — rejected because the shells are identical and bodies are typically <30 lines each. Evidence: A4.
  - One monolithic `renderDialog()` switch with inline body code — rejected because it triggers `gocyclo` warnings on a 15-case switch with non-trivial branching, and concentrates ~600 lines in one function. Evidence: devops-engineer #3; OQ-2 resolution.
- **Specialist owner:** `software-architect`
- **Revisit criterion:** If a dialog needs a non-render method (e.g., per-kind validation), reconsider the interface decision.
- **Dissent (if any):** None.
- **Driven by rounds:** R-1
- **Dependent decisions:** D-13 (revealedField reset before dialog render); D-19 (D48 fallback affects dialog overlay arithmetic).
- **Referenced in plan:** "Implementation Approach → Runtime Behavior"; "Decomposition and Sequencing → Commit 9".

## D-4: Field rendering — `FieldKind` enum switch with `fieldKindMultiLine` added

- **Question:** Should each field kind get its own type implementing a `Field` interface, or should rendering use a switch on the existing `FieldKind` enum, and does `fieldKindMultiLine` need to be added?
- **Decision:** One `renderField(f field, focused bool, width int) string` function that switches on `FieldKind`. Six render functions (`renderTextField`, `renderChoiceField`, `renderNumericField`, `renderSecretMaskField`, `renderModelSuggestField`, `renderMultiLineField`) called from the switch. Add `fieldKindMultiLine` to the enum because OQ-1 confirmed it is absent. `PromptFile` and `Command` form fields move from `fieldKindText` to `fieldKindMultiLine`. D51 dropdown overflow logic lives inside `renderChoiceField`.
- **Rationale:** Same ISP reasoning as D-3 — no behavioral polymorphism, only render variation. Adding `fieldKindMultiLine` is a one-line enum addition plus a new render function; the alternative (overloading `fieldKindText`) hides the multi-line/Ctrl+E intent.
- **Evidence:** software-architect A5; behavioral-analyst B8 (Ctrl+E from detail pane unwired); junior-developer Q8 (D-PR2-8 commits to `fieldKindMultiLine`); OQ-1 resolution (`grep -n "fieldKind" /Users/mxriverlynn/dev/mxriverlynn/pr9k/src/internal/workflowedit/detail.go` shows enum has only `fieldKindText`, `fieldKindChoice`, `fieldKindNumeric`, `fieldKindModelSuggest`, `fieldKindSecretMask`); visual-spec D32.
- **Rejected alternatives:**
  - `Field` interface with N implementations — rejected for ISP reasons (no behavioral polymorphism). Evidence: A5.
  - Overload `fieldKindText` for both single- and multi-line — rejected because Ctrl+E semantics belong only to multi-line; conflating them obscures intent. Evidence: B8.
- **Specialist owner:** `software-architect` / `behavioral-analyst`
- **Revisit criterion:** If a field kind grows behavioral semantics (e.g., async validation that mutates Model from inside render), revisit the interface decision.
- **Dissent (if any):** None.
- **Driven by rounds:** R-1
- **Dependent decisions:** D-16 (Ctrl+E wiring depends on `fieldKindMultiLine` existing); D-13 (secret-mask reset path runs before any field render).
- **Referenced in plan:** "Implementation Approach → Runtime Behavior"; "Decomposition and Sequencing → Commit 8".

## D-5: Render-time geometry computation — D48/D51/D17/D49 in `View()`, not `Update()`

- **Question:** Where should layout decisions (minimum-size guard, dropdown-overflow flip, session-header overflow priority, step-name truncation) be computed — in `Update()` when the trigger arrives, or in `View()` at render time?
- **Decision:** Compute layout at render time in `View()`. `handleWindowSize` only updates `m.width`, `m.height`, and viewport dimensions. The first statement in `View()` is the D48 minimum-size guard. D51 dropdown flip, D17 overflow priority, and D49 truncation are all computed locally in their respective render functions.
- **Rationale:** Layout depends on the Model state at the moment of render, not the moment of resize. Computing at render time keeps `Update()` simple and ensures any state change (focus shift, dropdown open, banner change) renders correctly without an extra Update pass to recompute layout.
- **Evidence:** software-architect A6; concurrency-analyst C3 (Bubble Tea View must be pure of timestamps but not pure of derivations from Model state); `internal/ui/model.go:382-749` (run-mode precedent computes layout in View).
- **Rejected alternatives:**
  - Compute layout in `Update()` and store on Model — rejected because it adds Model fields whose only purpose is render caching; also requires recomputation on every state change. Evidence: A6.
  - Compute layout in a Bubble Tea command — rejected because it adds an extra round-trip per resize. Evidence: A6.
- **Specialist owner:** `software-architect`
- **Revisit criterion:** If render performance becomes an observable issue (run-mode TUI doesn't cache layout and has no perf issues per devops-engineer #11), introduce caching.
- **Dissent (if any):** None.
- **Driven by rounds:** R-1
- **Dependent decisions:** D-19 (D48 guard runs first in View); D-20 (chrome budget consumed in View).
- **Referenced in plan:** "Implementation Approach → Runtime Behavior"; "Decomposition and Sequencing → Commit 3, Commit 4".

## D-6: Test strategy — ANSI-stripped substring + structural assertions

- **Question:** How should render tests assert on `View()` output? Golden files? Exact string equality? Substring after ANSI strip?
- **Decision:** ANSI-stripped substring + structural assertions. Every render test wraps `m.View()` with `ansi.StripAll(...)` before substring checks. Frame-shape tests (`TestView_FrameHas9ChromeRows`, `TestView_SessionHeaderContainsDirtyIndicatorWhenDirty`, `TestView_MinimumSizeRendersTooSmallMessage`) assert structure. Targeted color tests use narrow ANSI sequence checks for D14/D16/D45 palette decisions.
- **Rationale:** Mockup files use annotation syntax, not real ANSI — they are unsuitable as golden targets. Exact string equality breaks on whitespace or width changes. Substring + structural assertions are robust and target the visual invariants that matter (presence of `╭`, presence of `[ro]` tag, count of chrome rows).
- **Evidence:** test-engineer R1 output; software-architect A8; `src/internal/ansi/StripAll` exists and is usable in tests.
- **Rejected alternatives:**
  - Golden-file snapshot tests against mockups — rejected because mockups are annotated ASCII, not real ANSI output. Evidence: A8.
  - Exact string equality on `View()` output — rejected because it brittle-couples tests to whitespace and width formulas. Evidence: A8.
- **Specialist owner:** `test-engineer`
- **Revisit criterion:** If a render bug is found that substring tests would not have caught (e.g., row-order regression), add a structural assertion for it.
- **Dissent (if any):** None.
- **Driven by rounds:** R-1
- **Dependent decisions:** D-7 (banner clear pattern needs synchronous test seam); D-8 (time injection enables banner-clear test); D-9 (commit 1 implements the seam ahead of every other commit).
- **Referenced in plan:** "Testing Strategy"; "Decomposition and Sequencing → Commit 1".

## D-7: saveBanner clear pattern — `tea.Tick(3s, clearSaveBannerMsg{gen})` with bannerGen counter

- **Question:** How should the save-banner auto-clear after ~3 seconds without violating Bubble Tea's pure-View contract and without race hazards on rapid saves?
- **Decision:** On save success, increment `m.bannerGen`, set `m.saveBanner`, and emit `tea.Tick(3*time.Second, func(time.Time) tea.Msg { return clearSaveBannerMsg{gen: m.bannerGen} })`. The `clearSaveBannerMsg` handler clears `m.saveBanner` only if its `gen` field matches the current `m.bannerGen` (otherwise a stale tick would clear a newer banner).
- **Rationale:** A lazy-timestamp-in-View approach was considered (read `nowFn()` in View, hide banner if elapsed > 3s) but violates Bubble Tea's pure-View contract — the same Model value must produce the same View output. The generation counter is the only correct pattern under rapid-save conditions; without it, an in-flight Tick from a prior save would wipe a newer banner. `tea.After` is not in this Bubble Tea version's API; `tea.Tick` is the documented function.
- **Evidence:** facilitation-summary OD-1; concurrency-analyst C2, C3; junior-developer Q4 (no prior Model-owned timer in `src/internal/`); behavioral-analyst B1 (saveBanner never cleared).
- **Rejected alternatives:**
  - Lazy-timestamp-in-View — rejected because View must be pure. Evidence: C3.
  - Goroutine-driven timer with channel send — rejected because Bubble Tea cmds are the canonical async path; goroutines bypass the message bus and require WG drain. Evidence: C3, project concurrency standards.
  - No generation counter — rejected because rapid saves create stale-Tick races. Evidence: C2 (`Recommended fix: tea.Tick(3s, clearBannerMsg{gen}) with generation counter to prevent stale clears from wiping a newer banner on rapid saves`).
- **Specialist owner:** `concurrency-analyst`
- **Revisit criterion:** If save semantics change to support pre-emptive cancel (rapid save during banner shows ack of prior save), reconsider whether incrementing or resetting the counter is correct.
- **Dissent (if any):** None.
- **Driven by rounds:** R-1
- **Dependent decisions:** D-8 (time injection enables synchronous test of the timer); D-23 (session-event log entries include `save_banner_set` and `save_banner_cleared`).
- **Referenced in plan:** "Implementation Approach → Runtime Behavior"; "Decomposition and Sequencing → Commit 4"; "Testing Strategy".

## D-8: Time injection — `nowFn func() time.Time` Model field

- **Question:** How should tests synchronously verify timer-driven behavior (banner clear) without `time.Sleep`?
- **Decision:** Add `nowFn func() time.Time` as a Model field, defaulting to `time.Now`. Tests override `m.nowFn = func() time.Time { return fixed }` to inject a deterministic clock. The `clearSaveBannerMsg` handler does not call `time.Now()` directly; only Update and Cmd code paths that need a timestamp consult `m.nowFn()`.
- **Rationale:** Standard Go DI-for-time pattern. No message plumbing required. The run-mode heartbeat precedent (passing `time.Now()` in a message) applies to externally driven ticks, not internal Model-owned ones; the two patterns are not in conflict.
- **Evidence:** facilitation-summary OD-2; junior-developer Q9 (no prior time-injection in `src/internal/`); test-engineer R1 output (`var nowFn = time.Now` seam recommended).
- **Rejected alternatives:**
  - Package-level `var nowFn = time.Now` — rejected because tests running in parallel can race on the override. Evidence: testing-standards `coding-standards/testing.md` (race detector requirement).
  - Inject through messages — rejected because every code path consuming time would have to plumb the value through. Evidence: Q9.
- **Specialist owner:** `test-engineer`
- **Revisit criterion:** If multiple time-related fields appear (e.g., a separate clock for path-completion debounce), consolidate behind a `Clock` interface.
- **Dissent (if any):** None.
- **Driven by rounds:** R-1
- **Dependent decisions:** D-7 (banner timer test depends on this seam).
- **Referenced in plan:** "Testing Strategy"; "Decomposition and Sequencing → Commit 1".

## D-9: Commit graph — 13 incremental commits, `make ci` green at each

- **Question:** How should the visual-design implementation be sliced into commits so each is independently reviewable, ordering hazards are respected, and `make ci` is green at every step?
- **Decision:** 13 commits in this sequence: (1) test seam migration; (2) `uichrome` package; (3) behavioral fixes (independent of render); (4) View chrome frame + saveBanner timer; (5) session header render; (6) menu bar render; (7) outline pane render; (8) detail pane render; (9) dialogs render; (10) help modal + findings panel; (11) empty-editor view; (12) `Validating…` footer + browse-only render; (13) documentation. Each commit leaves `make ci` green.
- **Rationale:** Respects the six ordering hazards in DEP-1 through DEP-6: chrome primitives (commit 2) precede every render commit; chrome budget fix (commit 3) precedes D48 guard (commit 4); View frame (commit 4) precedes overlay-dependent commits (6, 9, 10); saveBanner timer is designed alongside View frame (commit 4); phase guard (commit 3) precedes any flash decoration (deferred); revealedField reset (commit 3) precedes dialog rendering (commit 9). Test seam (commit 1) lets every subsequent commit ship working tests.
- **Evidence:** facilitation-summary OD-4; risk-analyst ordering hazards 1-6; junior-developer Q14 (smallest shippable slice = test seam first).
- **Rejected alternatives:**
  - Single monolithic commit — rejected because review surface area is unmanageable and bisect resolution drops to "this whole PR." Evidence: devops-engineer #6 (run-mode precedent of incremental landings).
  - Two large commits (behavioral + visual) — rejected because the test seam must precede behavioral fixes, and commit 4's chrome frame is a render concern that depends on chrome primitives. Three boundaries collapse to either "single PR" or "13 commits." Evidence: ordering hazards 1-3.
- **Specialist owner:** `project-manager`
- **Revisit criterion:** If a commit cannot be made green standalone (e.g., a behavioral fix triggers a test that depends on an unrelated render commit), split or reorder that pair.
- **Dissent (if any):** None.
- **Driven by rounds:** R-1
- **Dependent decisions:** D-21 (documentation in same PR is commit 13).
- **Referenced in plan:** "Decomposition and Sequencing".

## D-10: Banner short-form tags — `[ro]`/`[ext]`/`[sym]`/`[shared]`/`[?fields]` in `allBannerTexts()`

- **Question:** Should session-header banners use the long-form strings currently shipped (`[read-only]`, `[external workflow]`, `[symlink → target]`, `[shared install]`, `[unknown fields]`) or the short-form tags committed in visual-spec D14?
- **Decision:** Replace `allBannerTexts()` (`model.go:51-73`) to emit `[ro]`, `[ext]`, `[sym]`, `[shared]`, `[?fields]`. Symlink banner retains its target arrow (`[sym → target.json]` form) per visual-spec D14.
- **Rationale:** Short-form tags are a color-blind-safety contract — banners must be distinguishable from unique character shapes, not from color alone. Visual-spec D14 commits the short-form vocabulary.
- **Evidence:** behavioral-analyst B11; user-experience-designer #1; visual-spec D14; `model.go:51-73` (current long-form strings).
- **Rejected alternatives:**
  - Keep long-form — rejected because visual-spec D14 governs and accessibility contract requires short-form. Evidence: UX#1.
  - Translate at render time — rejected because the canonical text should be one string; render must not know the spec's tag vocabulary independently.
- **Specialist owner:** `user-experience-designer`
- **Revisit criterion:** If localization is added, revisit how short-form tags translate.
- **Dissent (if any):** None.
- **Driven by rounds:** R-1
- **Dependent decisions:** D-15 (reload paths must propagate banner signals these tags depend on); D-18 (browse-only render uses these tags).
- **Referenced in plan:** "Decomposition and Sequencing → Commit 3"; "Definition of Done".

## D-11: Dirty render source — `m.IsDirty()` not `m.dirty`

- **Question:** Which dirty-state source does the render path consult for the D13 dirty indicator (`●`)?
- **Decision:** Render reads `m.IsDirty()` (structural diff). The `m.dirty` write-through flag remains as a fast-path cache for Update logic that needs immediate mutation tracking, but is not consulted by render.
- **Rationale:** `m.dirty` and `m.IsDirty()` can diverge — round-trip edits (add+delete same step) leave `m.dirty=true` while `IsDirty()=false`. D13 commits the dirty indicator to reflect unsaved-changes state, which is the structural condition. Using `m.dirty` in render is an API misuse: `IsDirty()` was added precisely to provide the correct dirty-detection path.
- **Evidence:** behavioral-analyst B12; user-experience-designer #3; junior-developer Q10; visual-spec D13; facilitation-summary IC-03; `model.go:90,155-158` (two separate concepts).
- **Rejected alternatives:**
  - Read `m.dirty` (current code) — rejected because of divergence under round-trip edits. Evidence: B12.
  - Remove `m.dirty` entirely — rejected because Update fast-path uses it; pure structural diff on every keystroke is more expensive than necessary. Evidence: Q10 ("`m.dirty` may remain as performance cache").
- **Specialist owner:** `behavioral-analyst`
- **Revisit criterion:** If `m.dirty` and `IsDirty()` are observed to diverge in a way that affects Update logic, consolidate.
- **Dissent (if any):** None.
- **Driven by rounds:** R-1
- **Dependent decisions:** D-18 (browse-only suppresses dirty render via this same path).
- **Referenced in plan:** "Decomposition and Sequencing → Commit 3"; "Implementation Approach → Runtime Behavior".

## D-12: Phase-boundary guard in `doMoveStepUp/Down` with `boundaryFlash` field + `tea.Tick`-cleared signal

- **Question:** How should phase-boundary crossings be prevented in the step-reorder path, and what visual feedback should replace the silent corruption that currently occurs?
- **Decision:** `doMoveStepUp`/`doMoveStepDown` (`model.go:1082-1112`) check `Phase` equality before swap. If phases differ, the swap is declined and `m.boundaryFlash` is set with a generation counter. A `tea.Tick(150*time.Millisecond, ...)` clears the flash by emitting `clearBoundaryFlashMsg{seq}`. View renders the flash by inverting the cursor row's background for one frame.
- **Rationale:** `doMoveStepUp/Down` currently swap unconditionally — crossing a phase boundary corrupts the workflow document's phase invariant on the next save. Behavioral-spec D34 commits the visible "drops it at the phase's edge" feedback. Generation-counter pattern matches D-7 (saveBanner) — same race-prevention discipline.
- **Evidence:** behavioral-analyst B2; edge-case-explorer EC7; risk-analyst R4 (severity 5 — disk corruption); behavioral-spec D34 ("visibly drops it at the phase's edge"); concurrency-analyst C3 (one-frame flash pattern).
- **Rejected alternatives:**
  - Allow cross-phase swap — rejected because it corrupts the document. Evidence: R4.
  - Block silently with no flash — rejected because user has no feedback. Evidence: EC7, behavioral-spec D34.
  - Flash without generation counter — rejected because rapid up/down keypresses race. Evidence: C3.
- **Specialist owner:** `behavioral-analyst`
- **Revisit criterion:** If the flash duration is reported as too short to perceive, tune the Tick interval.
- **Dissent (if any):** None.
- **Driven by rounds:** R-1
- **Dependent decisions:** D-7 (same `tea.Tick` + gen-counter discipline).
- **Referenced in plan:** "Decomposition and Sequencing → Commit 3"; "RAID Log → R4".

## D-13: revealedField reset on every focus-leave including dialog-open paths

- **Question:** When does `m.detail.revealedField` (the secret-mask reveal index) need to be reset to `-1`?
- **Decision:** Reset `m.detail.revealedField = -1` on every focus-leave path: Tab, Shift-Tab, Esc, Enter (when leaving the field), AND on every dialog-open path (`Ctrl+S`, `Ctrl+O`, `Ctrl+N`, `Ctrl+Q`, `Ctrl+H`, validation-results overlay, etc.).
- **Rationale:** A secret value left revealed when a dialog opens stays visually present in the detail pane behind the dialog overlay. Even with the overlay covering most of the pane, the bracketed value can leak through the gap between dialog and outline. The current code resets only on Tab/focus-leave inside the field state machine, missing all dialog-entry paths.
- **Evidence:** edge-case-explorer EC5; user-experience-designer #8; risk-analyst R5; visual-spec D30; `model.go:1144` (Tab path is template).
- **Rejected alternatives:**
  - Reset only on focus change inside the detail pane — rejected because dialog open does not constitute focus change in the current state machine. Evidence: EC5.
  - Reset by adding a wrapper in dialog-open Update handlers — rejected because there are 15 dialog kinds; consistent reset requires either a wrapper helper called from all 15 paths OR one reset in a tier-0 pre-dispatch when a dialog-open keystroke is detected. Decision uses the former (reset helper called from each dialog-open path).
- **Specialist owner:** `edge-case-explorer`
- **Revisit criterion:** If a future field kind needs to keep state across dialog open (e.g., a long-form preview), revisit.
- **Dissent (if any):** None.
- **Driven by rounds:** R-1
- **Dependent decisions:** D-3 (dialog rendering must trust the reset has occurred before render).
- **Referenced in plan:** "Decomposition and Sequencing → Commit 3"; "Security Posture".

## D-14: WindowSizeMsg routing at tier-0 pre-dispatch

- **Question:** Where in the Update routing tiers should `tea.WindowSizeMsg` be handled so that resize during help-modal or dialog states is not dropped?
- **Decision:** Add a tier-0 pre-dispatch branch: if `msg.(type) == tea.WindowSizeMsg`, call `m.handleWindowSize(msg)` and continue to the active tier (not return early). Tiers 1-4 stop having to know about WindowSizeMsg.
- **Rationale:** Bubble Tea does not redeliver WindowSizeMsg; if dropped during help-modal or dialog states, `m.width`/`m.height` go stale for the rest of the session, breaking dialog overlay arithmetic and the D48 guard. The tier-0 pre-dispatch ensures every state sees the resize.
- **Evidence:** behavioral-analyst B13; concurrency-analyst C1, C4; user-experience-designer #10; risk-analyst R15; junior-developer Q13 (three-line addition; no regression risk).
- **Rejected alternatives:**
  - Add WindowSizeMsg branch to each tier — rejected because it duplicates handler code in 4 places. Evidence: Q13.
  - Re-query terminal size on every render — rejected because Bubble Tea doesn't expose a re-query API in this version. Evidence: C5.
- **Specialist owner:** `concurrency-analyst`
- **Revisit criterion:** If a future tier has a legitimate reason to suppress resize (none currently), add a tier-specific override after.
- **Dissent (if any):** None.
- **Driven by rounds:** R-1
- **Dependent decisions:** D-19 (D48 guard depends on accurate `m.width`/`m.height`).
- **Referenced in plan:** "Decomposition and Sequencing → Commit 3"; "Implementation Approach → Runtime Behavior".

## D-15: makeLoadCmd reload path forwards all banner signals

- **Question:** Why do reload paths (Recovery dialog, file conflict reload) lose banner signals (`isReadOnly`, `isExternal`, `isSharedInstall`, `isSymlink`, `symlinkTarget`)?
- **Decision:** `makeLoadCmd()` and the inline `openFileResultMsg` return literal in the recovery handler must populate all five banner fields. Add the missing field assignments at `model.go:1693-1719` and `model.go:775-810`.
- **Rationale:** Browse-only mode (D12, D14) requires three signals (greyed Save, suppressed dirty, `[ro]` banner). If `makeLoadCmd`'s reload path strips `isReadOnly`, the user enters browse-only mode silently — the banner is gone but the file is still read-only. Same applies to symlink, external, shared-install signals.
- **Evidence:** behavioral-analyst B5; user-experience-designer #4; CL-19, CL-26 in claim ledger (verified by code inspection — `model.go:1704-1712` only sets `doc`, `diskDoc`, `companions`, `workflowDir`); visual-spec D12, D14.
- **Rejected alternatives:**
  - Re-detect signals after every load — rejected because detection cost is non-zero and the original load already detected them; reload should mirror initial-load. Evidence: B5.
  - Pass signals as a side-channel — rejected because the existing `openFileResultMsg` struct has the fields; using them is the obvious fix. Evidence: messages.go:24-38.
- **Specialist owner:** `behavioral-analyst`
- **Revisit criterion:** If a new banner signal is added (e.g., `isLocked`), update both the struct and the two reload paths together.
- **Dissent (if any):** None.
- **Driven by rounds:** R-1
- **Dependent decisions:** D-10 (banner tags depend on signals being forwarded); D-18 (browse-only render).
- **Referenced in plan:** "Decomposition and Sequencing → Commit 3"; "RAID Log → R10".

## D-16: `fieldKindMultiLine` added to enum; `Ctrl+E` wired from detail pane

- **Question:** Is a new field-kind constant needed for the multi-line/Ctrl+E semantics, and where does the Ctrl+E key handler live?
- **Decision:** Add `fieldKindMultiLine` to the FieldKind enum (after `fieldKindSecretMask`). Update `buildDetailFields` to emit `fieldKindMultiLine` for `PromptFile` and `Command` form fields (currently `fieldKindText`). Wire `Ctrl+E` in `updateEditView`'s detail-pane branch: when focus is on a field of kind `fieldKindMultiLine`, dispatch the existing `EditorRunner` path (already used by `updateDialogRecovery`).
- **Rationale:** OQ-1 confirmed `fieldKindMultiLine` is absent from the enum. D-PR2-8 commits to Ctrl+E in the detail-pane handler with `fieldKindMultiLine`. Both gaps are inherited from PR-2; the visual layer depends on `fieldKindMultiLine` for the D32 multi-line action row.
- **Evidence:** behavioral-analyst B8; software-architect A5; junior-developer Q8; visual-spec D32; OQ-1 resolution.
- **Rejected alternatives:**
  - Detect multi-line by introspecting field name — rejected because it tightly couples the render layer to field-name strings. Evidence: A5.
  - Skip Ctrl+E from detail pane — rejected because D32 commits to the action row. Evidence: D32.
- **Specialist owner:** `behavioral-analyst`
- **Revisit criterion:** If a field type emerges that needs Ctrl+E but is not multi-line (e.g., script picker), consider a separate `fieldKindEditable` flag.
- **Dissent (if any):** None.
- **Driven by rounds:** R-1
- **Dependent decisions:** D-4 (field rendering switch includes `fieldKindMultiLine`).
- **Referenced in plan:** "Decomposition and Sequencing → Commit 3"; "Implementation Approach → External Interfaces".

## D-17: `Validating…` footer guard in `ShortcutLine()`

- **Question:** How is the D34/F17 "Validating…" footer signal threaded into the existing footer renderer?
- **Decision:** `ShortcutLine()` (`footer.go:7-23`) gains an early guard: if `m.validateInProgress` is true, return `"Validating…"` (or the equivalent shortcut bar variant). Same pattern for `m.saveInProgress` returning a `"Saving…"` variant. The branches return before the normal shortcut-bar assembly.
- **Rationale:** Footer is the user's primary status surface; `ShortcutLine()` is the single function consulted at every render. Adding the guard at the top is the smallest, least-invasive integration.
- **Evidence:** behavioral-analyst B9; user-experience-designer #6; visual-spec D34; `footer.go:7-23` (no current guard).
- **Rejected alternatives:**
  - Render a separate row above footer when validating — rejected because it consumes a chrome row. Evidence: visual-spec D9 (8-row chrome budget; no extra rows allowed).
  - Use a banner instead of footer — rejected because banners are session-header concerns; this is a transient activity signal.
- **Specialist owner:** `user-experience-designer`
- **Revisit criterion:** If multiple concurrent activities arrive (validate + save), revisit precedence.
- **Dissent (if any):** None.
- **Driven by rounds:** R-1
- **Dependent decisions:** D-23 (session-event log records validate/save state transitions).
- **Referenced in plan:** "Decomposition and Sequencing → Commit 12".

## D-18: Browse-only signals threaded through render

- **Question:** How do the three browse-only signals (greyed Save, suppressed dirty, `[ro]` banner) reach the render layer?
- **Decision:** Pass `m.banners.isReadOnly` (or equivalent boolean accessor) into `renderMenuBar` (greyed Save in File menu; greyed Save shortcut in footer if appropriate), into `renderSessionHeader` (banner emit), and into the dirty-indicator path (suppress `●` when read-only). Browse-only does not block the user from opening dialogs or reading state — it only suppresses save-side affordances.
- **Rationale:** Browse-only is a user-visible mode change; render must signal it consistently across all three surfaces. Threading the boolean is preferable to having each render consult Model state independently — it makes the contract explicit.
- **Evidence:** behavioral-analyst B10; user-experience-designer #4; risk-analyst R18; visual-spec D12, D14.
- **Rejected alternatives:**
  - Each render function reads `m.banners.isReadOnly` directly — rejected only weakly; the threaded approach is preferred for testability and contract clarity, but direct read is acceptable. Evidence: UX#4.
- **Specialist owner:** `user-experience-designer`
- **Revisit criterion:** If browse-only grows additional restrictions (e.g., field-level read-only), revisit threading scope.
- **Dissent (if any):** None.
- **Driven by rounds:** R-1
- **Dependent decisions:** D-15 (signals must be forwarded by reload paths first).
- **Referenced in plan:** "Decomposition and Sequencing → Commit 12".

## D-19: 60×16 minimum-size fallback render

- **Question:** What does `View()` render when the terminal is below the 60×16 minimum?
- **Decision:** First statement in `View()` checks `if m.width < MinTerminalWidth || m.height < MinTerminalHeight`. If true, return a centered "Terminal too small (60×16 required)" message with current dimensions reported. Cursor is clamped to (0,0); no further pane arithmetic runs.
- **Rationale:** Below-minimum rendering produces undefined layout (negative widths, clipped panes). Run-mode TUI uses approach (b) explicit guards (devops-engineer #6 cites this as the reliable pattern). Defer/recover is a last-resort backstop, not a primary path.
- **Evidence:** behavioral-analyst B4; edge-case-explorer EC2; devops-engineer #6; visual-spec D48; `outline.go` (`outlineWidth(0)=20` clamps to 1 — broken).
- **Rejected alternatives:**
  - Defer/recover wrapper — rejected because it masks the failure rather than prevents it. Evidence: devops-engineer #6.
  - Continue with clamped widths — rejected because pane arithmetic produces undefined layout. Evidence: B4.
- **Specialist owner:** `edge-case-explorer`
- **Revisit criterion:** If 60×16 is too aggressive (real users hit it routinely), tune the constants.
- **Dissent (if any):** None.
- **Driven by rounds:** R-1
- **Dependent decisions:** D-20 (chrome budget is part of the same render path); D-14 (resize routing must update `m.width`/`m.height` before guard runs).
- **Referenced in plan:** "Decomposition and Sequencing → Commit 3, Commit 4"; "Definition of Done".

## D-20: chromeRows = 8 chrome budget

- **Question:** What constant value does the chrome-budget arithmetic use, and where is it declared?
- **Decision:** Declare `const ChromeRows = 8` in `internal/workflowedit/constants.go` (or `uichrome/constants.go` if cross-package). `panelH` becomes `m.height - ChromeRows`. The chrome budget covers: top border (1), title row (1), session header (2), menu bar (1), bottom border (1), shortcut/footer (1), separator (1) — totaling 8 rows above the pane area. Visual-spec D9 commits to this budget.
- **Rationale:** The current `m.height - 2` is a placeholder; D9 commits the budget to 8. Naming the constant prevents the bug recurring in future render code.
- **Evidence:** structural-analyst S5; behavioral-analyst B3; devops-engineer #1; risk-analyst R1; visual-spec D9; `model.go:1493`.
- **Rejected alternatives:**
  - Bare literal `8` — rejected per `coding-standards/lint-and-tooling.md` (no lint suppressions; named constants are preferred for readability per OQ-2 resolution).
  - 9-row chrome (top+session header 3 rows) — rejected because D9 commits 8.
- **Specialist owner:** `structural-analyst`
- **Revisit criterion:** If a chrome row is added or removed (e.g., a new banner row), the constant changes atomically.
- **Dissent (if any):** None.
- **Driven by rounds:** R-1
- **Dependent decisions:** D-19 (D48 guard uses the same constant); D-5 (View geometry depends on chrome budget).
- **Referenced in plan:** "Decomposition and Sequencing → Commit 3"; "Definition of Done".

## D-21: Documentation ships in same PR

- **Question:** Do the four documentation updates (workflow-builder feature doc, workflowedit code-package doc, how-to guide, CLAUDE.md pointer) ship in this PR or as a follow-up?
- **Decision:** Documentation ships in the same PR, as commit 13 of the 13-commit sequence. Files: `docs/features/workflow-builder.md` (Visual Layout section), `docs/code-packages/workflowedit.md` (Visual Layout section), `docs/how-to/using-the-workflow-builder.md` (menu-bar states, banner priority), `CLAUDE.md` (pointer to visual spec).
- **Rationale:** `docs/coding-standards/documentation.md` mandates that feature docs ship with the feature, not as follow-ups. The visual layer is a feature completion, not polish.
- **Evidence:** devops-engineer #9; facilitation-summary IC-04; `docs/coding-standards/documentation.md`.
- **Rejected alternatives:**
  - Ship docs as a follow-up PR — rejected because it violates the documentation standard. Evidence: documentation.md.
  - Ship a single doc file — rejected because each affected doc has a distinct audience (feature doc = users; code-package doc = contributors; how-to = users; CLAUDE.md = future agents). Evidence: existing doc structure.
- **Specialist owner:** `devops-engineer`
- **Revisit criterion:** If a doc update is impossible to write before merge (e.g., final screenshot not available), record the gap as an Open Item.
- **Dissent (if any):** None.
- **Driven by rounds:** R-1
- **Dependent decisions:** D-9 (commit 13 is the documentation step in the commit graph).
- **Referenced in plan:** "Operational Readiness"; "Decomposition and Sequencing → Commit 13"; "Definition of Done".

## D-22: No version bump for visual layer

- **Question:** Does the visual-design implementation require a version bump per `coding-standards/versioning.md`?
- **Decision:** No version bump. The visual layer is not part of pr9k's public API (CLI flags, config.json schema, `{{VAR}}` language, `--version` output). If the team later wants a tagged release, a separate `0.7.4` patch commit is the correct vehicle.
- **Rationale:** `coding-standards/versioning.md` enumerates public-API surfaces. Visual chrome is not on that list; bumping the version would imply a contract change that does not exist.
- **Evidence:** devops-engineer #8; `docs/coding-standards/versioning.md`.
- **Rejected alternatives:**
  - Bump to 0.7.4 inline — rejected because it conflates feature completion with release tagging. Evidence: versioning.md.
- **Specialist owner:** `devops-engineer`
- **Revisit criterion:** If the team adopts a release cadence that pairs visual milestones with version bumps, revisit.
- **Dissent (if any):** None.
- **Driven by rounds:** R-1
- **Dependent decisions:** None.
- **Referenced in plan:** "Operational Readiness".

## D-23: Session-event logging per state transition

- **Question:** What session-event log entries does the visual-design implementation add?
- **Decision:** Add log entries for `resize`, `dialog_open`, `dialog_close`, `save_banner_set`, `save_banner_cleared`, `terminal_too_small`, `focus_changed`, `validate_started`, `validate_complete`, `phase_boundary_decline`, `secret_revealed`, `secret_remasked`. Match the existing vocabulary at `model.go:194-215`. Log per state-transition, not per render.
- **Rationale:** Render is the most observable surface for users; logging the state transitions that drive render makes post-hoc debugging tractable. Existing event vocabulary at `model.go:194-215` shows the pattern is established.
- **Evidence:** devops-engineer #5; `model.go:194-215` (existing event vocabulary).
- **Rejected alternatives:**
  - Log on every View call — rejected because View is invoked on every Bubble Tea cycle; volume would drown signal. Evidence: devops-engineer #5.
  - Log only on errors — rejected because non-error transitions (focus, dialog open) are also debug-relevant.
- **Specialist owner:** `devops-engineer`
- **Revisit criterion:** If log volume becomes excessive in long sessions, downsample non-error events.
- **Dissent (if any):** None.
- **Driven by rounds:** R-1
- **Dependent decisions:** D-7 (banner set/cleared events); D-12 (phase-boundary decline event); D-13 (secret reveal/remask events).
- **Referenced in plan:** "Operational Readiness".

## D-24: `lipgloss.Width` everywhere for width measurement

- **Question:** Does the implementation use `len()` or `lipgloss.Width()` for measuring column widths in render code?
- **Decision:** Use `lipgloss.Width` everywhere. All width comparisons for D49 step-name truncation, D50 label truncation, D17 session-header overflow, dropdown widths, and banner widths route through `lipgloss.Width`.
- **Rationale:** `len()` returns byte count, not display width — fails on CJK and double-width characters. `lipgloss.Width` is the established pattern in `internal/ui/model.go:481-515`. Using it from the start avoids a P2 fixup pass per FS-05.
- **Evidence:** edge-case-explorer EC12; facilitation-summary FS-05; `internal/ui/model.go:481-515` (precedent).
- **Rejected alternatives:**
  - `len()` — rejected for CJK / double-width incorrectness. Evidence: EC12.
  - `utf8.RuneCountInString` — rejected because it counts runes, not display columns; double-width characters would still mis-measure. Evidence: EC12.
- **Specialist owner:** `edge-case-explorer`
- **Revisit criterion:** If a perf bottleneck appears (rare; lipgloss.Width is cheap), revisit.
- **Dissent (if any):** None.
- **Driven by rounds:** R-1
- **Dependent decisions:** None.
- **Referenced in plan:** "Implementation Approach → Runtime Behavior"; "Decomposition and Sequencing → Commit 7, Commit 8".

## D-25: Mockup 05 variant C reverse-video annotation overruled by visual-spec D22 + D28

- **Question:** Does the mockup `05-detail-pane-fields.md` variant C reverse-video annotation on the focused choice-list field body override visual-spec D22 + D28?
- **Decision:** No. Visual-spec D22 (clarified) and D28 govern: "no reverse-video on the field body." Reverse-video is reserved for two states only — reorder mode active step and open-dropdown highlighted item. The mockup annotation is erroneous and is treated as superseded.
- **Rationale:** The visual-spec authority hierarchy places decision-log entries above mockup annotations. D22's clarification was added precisely to settle this ambiguity. Treating the mockup as governing would re-debate a closed decision.
- **Evidence:** software-architect D# contradiction note; user-experience-designer #2; team-findings F1 / GAP-011; decision-log D22 (clarified), D28.
- **Rejected alternatives:**
  - Implement variant C as drawn — rejected because visual-spec governs. Evidence: visual-spec authority hierarchy.
  - Re-debate via spec amendment — rejected because the spec phase is closed; this is implementation. Evidence: PM directive ("D1-D51 are committed mechanics").
- **Specialist owner:** `software-architect` / `user-experience-designer`
- **Revisit criterion:** If a future visual review re-opens the reverse-video discipline, revisit.
- **Dissent (if any):** None.
- **Driven by rounds:** R-1
- **Dependent decisions:** None.
- **Referenced in plan:** "Implementation Approach → Architecture and Integration Points" (note in choice-field render).

## D-26: HintEmpty constant superseded by D43 outline+detail layout

- **Question:** Does the HintEmpty constant (and the current flat `renderEmptyEditor()` that returns it) survive into the implementation, or is it superseded by visual-spec D43?
- **Decision:** Superseded. `renderEmptyEditor()` is rewritten to return the D43 layout: an empty outline pane on the left, and a small bordered hint panel on the right inside the detail-pane region. The `HintEmpty` constant is removed; its line-number reference in the comment (`constants.go:30`) is also removed.
- **Rationale:** `HintEmpty` content (`"No workflow open. Ctrl+N new · Ctrl+O open"`) does not match D43 text (`"File > New (Ctrl+N) — create a workflow"` etc.) and is rendered as a flat string instead of the bordered layout. The constant is a placeholder.
- **Evidence:** structural-analyst S12; edge-case-explorer EC19; junior-developer Q6; visual-spec D43; `constants.go:30-31`.
- **Rejected alternatives:**
  - Keep `HintEmpty` as a string constant inside the new render — rejected because the new layout has multiple lines of D43 text; a single constant doesn't fit. Evidence: visual-spec D43.
- **Specialist owner:** `structural-analyst`
- **Revisit criterion:** None (the constant is gone; if a similar pattern emerges elsewhere, treat as a fresh decision).
- **Dissent (if any):** None.
- **Driven by rounds:** R-1
- **Dependent decisions:** None.
- **Referenced in plan:** "Decomposition and Sequencing → Commit 11".
