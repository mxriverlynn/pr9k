# Facilitation Summary: Workflow-Builder TUI Visual-Design Implementation Planning

## Scope

Session covers implementation planning for the visual-design layer of the `pr9k workflow` builder TUI.
Artifacts referenced: visual spec `feature-specification.md` (51 decisions D1–D51, 25 mockups), decision-log `artifacts/decision-log.md`, team-findings `artifacts/team-findings.md`, and verbatim R1 specialist outputs from 10 specialists (software-architect, structural-analyst, behavioral-analyst, concurrency-analyst, edge-case-explorer, test-engineer, user-experience-designer, devops-engineer, risk-analyst, junior-developer).
Current branch: `workflow-builder-tui`. Behavioral implementation merged at `ed8203e`.
Date: 2026-04-26.

---

## Outcome and Context

**Primary outcome.** Replace all placeholder strings in `src/internal/workflowedit/` with the full-bordered visual layout committed by D1–D51, so a user launching `pr9k workflow` sees a product-consistent, full-screen TUI rendering all 28 behavioral modes with correct chrome, palette, overlays, and transient signals.

**Driving constraint.** The behavioral implementation is shipped and branch-stable. The visual layer is the remaining PR-sized unit of work before the builder is feature-complete. Every day it ships as placeholder strings degrades the product impression for early users.

**Stakeholders.** Workflow authors who want a usable builder TUI (see a real interface, not debug strings). The maintainer who needs `make ci` green with no lint suppressions. Future contributors who need documented render patterns to extend.

**Future-state concern.** The visual layer adds ~900 LOC to a `model.go` already at 1,831 lines. Without deliberate decomposition into per-concern render files the file becomes a high-churn convergence trap (S9). The `internal/ui` chrome primitives must be made accessible without importing the entire run-mode model package — the shape of that extraction anchors the dependency graph for both TUIs going forward.

**Out-of-scope boundary.** No behavioral changes. D1–D51 are committed mechanics; they are not re-debated here. P2/P3 risk-analyst findings (R19–R33) are deferred to a follow-on PR. Phase-boundary flash (R4/D41 animation path) is deferred (P3).

---

## Participation Record

| Domain | Specialist | Status | Summary of input |
|---|---|---|---|
| Software architecture, package structure | `software-architect` | In discussion | A1–A8: `internal/uichrome` extraction plan, render file map, dialog/field switch decomposition, overlay pattern reuse, test strategy. Full evidence-cited. |
| Static structure, module boundaries, coupling | `structural-analyst` | In discussion | S1–S13: 13 structural findings; confirmed placeholder locations, chrome-budget bug, missing render methods, View() assembly incompatibility with D2. Evidence-cited to specific line numbers. |
| Runtime behavior, data flow, state | `behavioral-analyst` | In discussion | B1–B13: 13 behavioral gap findings covering saveBanner, phase-boundary, chrome budget, D48 guard, banner tags, dirty indicator, browse-only, footer signal, HintEmpty. Evidence-cited. |
| Concurrency, Bubble Tea message safety | `concurrency-analyst` | In discussion | C1–C11: WindowSizeMsg drop, saveBanner permanent, D41 flash pattern, stale width, post-ExecProcess resize, validateCompleteMsg coexistence. Evidence-cited. |
| Edge cases | `edge-case-explorer` | In discussion | EC1–EC20: prioritized critical/high/medium/low edge cases covering chrome budget, D48 fallback, secret leak, dropdown overflow, CJK widths. Evidence-cited. |
| Test planning | `test-engineer` | In discussion | File-by-file triage: 8 modify + 2 new test files; seam requirements (nowFn, clearBannerMsg, bannerGen, assertModalFits); D# → test-name coverage matrix. |
| UX, accessibility, affordance | `user-experience-designer` | In discussion | 10 findings: banner tag contract (color-blind safety), dirty indicator source, browse-only signals, phase-boundary, secret re-mask, Validating footer, reverse-video discipline, resize routing. 5 must-fix. |
| DevOps, lint, rollout, documentation | `devops-engineer` | In discussion | 12 findings: 7 P0-level (panelH bug, named constants required, funlen risk, clearBanner timer, session-event logging, D48 guard, documentation must ship). |
| Risk prioritization | `risk-analyst` | In discussion | P0–P3 register with R1–R33; ordering hazards enumerated; 9 D# contradiction escalations (reclassified below as gap-fill items, not true contradictions). |
| Generalist stress-test | `junior-developer` | In discussion | 14 questions; 4 need decisions (Q1 uichrome vs re-export, Q4 timer pattern, Q9 time injection, Q12 commit graph); 10 resolved by evidence. |
| `adversarial-validator` | — | Not needed — no completed plan yet to validate. Will be dispatched before merge. |
| `gap-analyzer` | — | Not needed — gaps were already resolved in the visual spec's team-findings F1–F25. |
| `information-architect` | — | Not needed — documentation plan is enumerated by DevOps#9; no IA work required beyond those named files. |

---

## Claim Ledger

| # | Claim | State | Citation | Specialist |
|---|---|---|---|---|
| CL-01 | `panelH = m.height - 2` is wrong for D9's 8-row chrome budget | Evidenced | `model.go:1493`; decision-log D9 | S5, B3, DevOps#1, R1 |
| CL-02 | `View()` row assembly with `\n` separators is incompatible with D2's fixed 9-row chrome frame | Evidenced | `model.go:276-296` (flat `\n` join); decision-log D2 | S10, R6 |
| CL-03 | `saveBanner` is never cleared — no `tea.Tick` or clear message | Evidenced | `model.go:124,290-295,1564`; `messages.go` (no `clearBannerMsg` type) | B1, C2, C10, EC4, R3 |
| CL-04 | D48 minimum-size guard absent from `View()` | Evidenced | `model.go:276-296` (no size check); decision-log D48 | B4, EC2, DevOps#6, R2 |
| CL-05 | `allBannerTexts()` emits long-form `[read-only]`, `[external workflow]`, `[symlink → target]`, `[shared install]`, `[unknown fields]`; D14 commits short-form `[ro]`, `[ext]`, `[sym]`, `[shared]`, `[?fields]` | Evidenced | `model.go:51-73` (long-form strings); decision-log D14 | B11, UX#1 |
| CL-06 | `renderSessionHeader` renders `*` not `●` (D13 says `●` in Green) | Evidenced | `model.go:377` (`path += "*"`); decision-log D13 | S7, B6, R9 |
| CL-07 | `m.dirty` (write-through flag) and `m.IsDirty()` (structural diff) can diverge; D13 commits indicator reflects unsaved-changes state | Evidenced | `model.go:90,155-158` (two separate concepts); decision-log D13 | B12, UX#3, JD-Q10 |
| CL-08 | `renderDialog()` returns flat strings; D36 requires bordered centered overlay | Evidenced | `model.go:304-354` (flat switch); decision-log D36 | S3, R7 |
| CL-09 | `menu.go render()` returns flat string; D4/D11 require bordered overlay | Evidenced | `model.go:276` (`sb.WriteString(m.menu.render())`); S13 citing `menu.go:render()` | S13, R8 |
| CL-10 | `renderHelpModal()` returns flat one-line string; D40 requires run-mode shape | Evidenced | `model.go:300-302` | R11 |
| CL-11 | `renderEmptyEditor()` returns `HintEmpty` constant; D43 requires outline+detail layout with bordered hint panel | Evidenced | `model.go:356-358`; `constants.go:31` (`"No workflow open. Ctrl+N new · Ctrl+O open"`); decision-log D43 | S12, EC19, JD-Q6 |
| CL-12 | `findings.go findingsPanel` has no `render()` method; D38/D39 require multi-section bordered panel | Evidenced | R13 citing `findings.go` struct; decision-log D38, D39 | S11, R13 |
| CL-13 | `tea.WindowSizeMsg` handled only in tier-4 (`updateEditView`); dropped during help-modal and dialog states | Evidenced | `model.go:218-244` (tier routing, no tier-0 WindowSizeMsg branch); `model.go:903` (handled only inside updateEditView path) | B13, C1, C4, UX#10, R15 |
| CL-14 | `revealedField` not reset when a dialog opens — potential secret leak | Evidenced | `model.go:1144` (reset only on Tab/focus-leave path, not on dialog entry); `detail.go:37,60` | EC5, R5, UX#8 |
| CL-15 | Chrome primitives (`wrapLine`, `hruleLine`, `renderTopBorder`, `colorTitle`, `colorShortcutLine`, `overlay`, `spliceAt`) are unexported in `internal/ui` | Evidenced | `internal/ui/model.go:382-749` (all private functions); `internal/ui/overlay.go` (unexported) | S2, A1, R12 |
| CL-16 | D45 palette constants `Red`, `Yellow`, `Cyan`, `Dim` not declared in any package | Evidenced | S6 citing `internal/ui/header.go:21-41` (only LightGray, White, Green, ActiveStepFG, ActiveMarkerFG); decision-log D45 | S6, A3 |
| CL-17 | `ShortcutLine()` never reads `m.validateInProgress`; D34/F17 `Validating…` footer unimplemented | Evidenced | `footer.go:7-23` (no validateInProgress guard); decision-log D34 | B9, UX#6 |
| CL-18 | Browse-only mode absent from `View()`: no greyed Save, no dirty suppression, no `[ro]` banner render path in `View()` | Evidenced | `model.go:276-296` (no `m.banners.isReadOnly` branch in View); decision-log D12, D14 | B10, UX#4, R18 |
| CL-19 | Reload paths (Recovery dialog editor, file-conflict reload) return `openFileResultMsg` without `isReadOnly` et al. fields, losing banner signals | Evidenced | `model.go:775-810` (inline `openFileResultMsg` struct literal missing banner fields); `messages.go:24-38` (fields exist but unused in recovery path) | B5 |
| CL-20 | `doMoveStepUp/Down` swap unconditionally with no phase-boundary check; crossing a phase boundary corrupts the workflow document's phase invariant | Evidenced | `model.go:1082-1112` (no Phase comparison before swap); behavioral spec D34 ("visibly drops it at the phase's edge") | B2, EC7, R4 |
| CL-21 | `Ctrl+E` from detail pane unwired for `fieldKindMultiLine`; only invoked from `updateDialogRecovery` | Evidenced | B8 citing `model.go`; decision-log D32 ("D-PR2-8 commits to Ctrl+E in detail-pane handler") | B8, JD-Q8, R17 |
| CL-22 | `saveBanner` rendered as extra row in `View()` outside D9's 8-row chrome budget; D14 places transient banners in session-header banner slot | Evidenced | `model.go:289-295`; decision-log D14 | C10, R3 |
| CL-23 | `fieldKindMultiLine` for D32 is missing from the field-kind enum switch | Evidenced | A5 citing detail render code; decision-log D32 | A5 |
| CL-24 | `m.detail.dropdownOpen` / `m.detail.modelSuggFocus` do NOT introduce a fifth routing tier (positive) | Evidenced | C7 citing `model.go:475` | C7 |
| CL-25 | `tea.Cmd` closures correctly capture inputs by value / read-only refs; no goroutine Model writes (positive, one latent: `makeSaveCmd` captures `m.companions` by reference) | Evidenced | C9 | C9 |
| CL-26 | `makeLoadCmd()` does not forward banner signals (`isSymlink`, `isExternal`, `isReadOnly`, `isSharedInstall`) in its inline return literal; the reload-path leaves these zero | Evidenced | `model.go:1693-1719` (only `doc`, `diskDoc`, `companions`, `workflowDir` fields set) | B5 (verified by code inspection) |
| CL-27 | `outline.go` / `detail.go` separation is structurally clean; render additions go in the right files (positive) | Evidenced | S8 | S8 |
| CL-28 | Named constants absent: `MinTerminalWidth`, `MinTerminalHeight`, `ChromeRows`, `DialogMaxWidth`, `DialogMinWidth`, `HelpModalMaxWidth`, `LabelMaxWidth`, `OutlineMinWidth`, `OutlineMaxWidth`, `OutlinePercent` — `mnd` golangci-lint linter will fire on bare magic numbers | Evidenced | DevOps#2; CLAUDE.md coding-standards/lint-and-tooling.md (no lint suppressions) | DevOps#2, R1 |
| CL-29 | `funlen`/`gocyclo` will fire on long render functions and `renderDialog()` switch without decomposition | Anecdotal | DevOps#3 (no specific golangci-lint config cited — depends on thresholds in `.golangci.yml`) | DevOps#3 |
| CL-30 | Session-event log entries for render-triggering state transitions absent | Evidenced | DevOps#5; `model.go:194-215` (existing event vocabulary shows pattern exists) | DevOps#5 |
| CL-31 | Documentation must ship in the same PR per `docs/coding-standards/documentation.md` | Evidenced | `docs/coding-standards/documentation.md` ("feature docs must ship with the feature"); DevOps#9 | DevOps#9 |
| CL-32 | Mockup variant C of `05-detail-pane-fields.md` shows reverse-video on focused choice-list field body; D22+D28 say "no reverse-video on the field body" | Evidenced | decision-log D22 (clarified), D28; team-findings F1 | A# architect D# note, UX#2 |
| CL-33 | `HintEmpty` constant value (`"No workflow open. Ctrl+N new · Ctrl+O open"`) doesn't match D43 text (`"File > New (Ctrl+N) — create a workflow"` etc.) and `renderEmptyEditor()` returns a flat string rather than the bordered outline+detail layout | Evidenced | `constants.go:31`; decision-log D43 | S12, EC19 |
| CL-34 | No version bump required for visual layer per `docs/coding-standards/versioning.md` | Evidenced | `docs/coding-standards/versioning.md` (public API = CLI flags, config.json schema, VAR language, --version output; visual chrome is not in that list); DevOps#8 | DevOps#8 |
| CL-35 | Codebase has zero `tea.Tick`/`tea.After` in production code under `src/internal/`; using either would be the first Model-owned timer | Evidenced | JD-Q4 (searched `src/internal/`); `messages.go` (no timer messages exist) | JD-Q4 |
| CL-36 | Time-injection precedent absent in `src/internal/`; run-mode heartbeat passes `time.Now()` into the message rather than reading it inside Model | Evidenced | JD-Q9 (searched run-mode code) | JD-Q9 |

---

## RAID Log

### Risks

| ID | Risk | Likelihood | Severity | Blast Radius | Reversibility | Owner | Mitigation |
|---|---|---|---|---|---|---|---|
| R1 | `panelH = m.height - 2` causes pane content to overwrite footer and bottom border at every terminal height | 5 | 4 | 5 | 2 | `structural-analyst` / implementer | Change to `m.height - ChromeRows` atomically with chrome frame landing |
| R2 | No D48 minimum-size guard; pane arithmetic produces undefined layout below 60×16 | 4 | 4 | 4 | 2 | implementer | Add early-return check as first statement in `View()` |
| R3 | `saveBanner` permanent; also rendered outside D9 chrome budget, corrupting the fixed-row frame | 5 | 4 | 3 | 2 | implementer | `clearSaveBannerMsg{gen}` + `tea.Tick(3s)` + move banner into session-header slot |
| R4 | `doMoveStepUp/Down` no phase-boundary check — cross-phase swap corrupts workflow document written to disk | 4 | 5 | 2 | 3 | `behavioral-analyst` / implementer | Phase-equality guard before swap; decline with optional flash |
| R5 | `revealedField` not reset when dialog opens — secret value briefly visible in detail pane behind dialog overlay | 3 | 4 | 1 | 2 | implementer | Reset `m.detail.revealedField = -1` in dialog-open paths |
| R6 | `View()` flat `\n` row assembly incompatible with D2 fixed 9-row chrome frame — entire render must be replaced | 5 | 4 | 5 | 3 | `software-architect` / implementer | Rebuild `View()` as ordered row assembly with D2 row budget |
| R7 | `renderDialog()` flat strings — D36 bordered overlay requirement unimplemented | 5 | 3 | 4 | 3 | implementer | Per-kind dialog body builder + `renderDialogShell` |
| R8 | `menu.go render()` flat string — D4/D11 bordered dropdown unimplemented | 5 | 3 | 2 | 3 | implementer | Overlay splice pattern from `internal/ui/overlay.go` |
| R9 | `funlen`/`gocyclo` lint failure on long render functions if not decomposed | 3 | 3 | 2 | 2 | implementer | Extract per-dialog-kind render functions per A4 |
| R10 | Reload paths in Recovery dialog lose banner signals (isReadOnly, isSymlink, etc.) | 3 | 3 | 2 | 2 | implementer | Mirror `makeLoadCmd` pattern — populate all banner fields in inline `openFileResultMsg` return |

### Assumptions

| ID | Assumption | What changes if wrong | Verifier | Status |
|---|---|---|---|---|
| A1 | `internal/ui` chrome primitives can be extracted to `internal/uichrome` without breaking `internal/ui` callers | If callers break, the extraction is a larger refactor | `structural-analyst` | Unverified — verify before committing extraction |
| A2 | `tea.Tick(3*time.Second, ...)` is acceptable as the first Model-owned timer in `workflowedit` | If the team prefers lazy-timestamp pattern (JD-Q4), the implementation changes; both are correct | PM auto-decision below | Settled (see OD-1) |
| A3 | `nowFn func() time.Time` field on Model is the right time-injection seam | If test-engineer prefers message-carried timestamp, seam changes | PM auto-decision below | Settled (see OD-2) |
| A4 | `funlen`/`gocyclo` thresholds in `.golangci.yml` will fire on un-decomposed render functions | Depends on project's lint config; not verified | DevOps | Unverified — check `.golangci.yml` before assuming decomposition is lint-required vs. good practice |
| A5 | D22's clarification (no reverse-video on mockup variant C) governs over the mockup annotation | The mock is erroneous; decision-log governs per visual spec's authority hierarchy | decision-log D22 (clarified) | Confirmed — decision-log governs |
| A6 | `makeLoadCmd()` is the canonical path for all reload triggers; no other path can load a file | If another load path exists without banner propagation, R10 scope grows | Code search | Confirmed for known paths; `makeLoadCmd` + inline reload in recovery handler are the two sites |
| A7 | `fieldKindMultiLine` does not exist yet and must be added | If it was added in a later commit, no new type is needed | Code search | Anecdotal (A5/JD-Q8 assert it's missing; not directly verified in this session — implementer must confirm) |

### Issues

| ID | Issue | Owner | Next step |
|---|---|---|---|
| I1 | `panelH = m.height - 2` is actively wrong in shipped code; every resize event produces a 6-row-too-tall pane area | Implementer | Fix in first render commit (before or alongside chrome frame, per risk-analyst ordering hazard #2) |
| I2 | Render tests assert against placeholder strings; they will all pass falsely until tests are updated | Implementer | JD-Q14: convert to ANSI-stripped substring assertions as first commit before any production code change |
| I3 | `makeLoadCmd()` reload path does not forward banner signals; browse-only state is lost on recovery-dialog reload | Implementer | Populate all banner fields in the inline `openFileResultMsg` return at `model.go:1704-1712` |

### Decisions / Dependencies

| ID | Item | Rationale | Rejected alternatives | Evidence | Owner | Status |
|---|---|---|---|---|---|---|
| DEP-1 | R12 (export chrome primitives) must precede R6 (View frame), R7 (dialogs), R8 (menu), R11 (help modal) | Overlay and chrome functions needed by all render rebuilds | — | Risk-analyst ordering hazard #1 | implementer | Open |
| DEP-2 | R1 (chrome budget -2→-8) must precede R2 (D48 guard) | Guard test asserts against correct budget | — | Risk-analyst ordering hazard #2 | implementer | Open |
| DEP-3 | R6 (View chrome frame) must precede R7 (dialogs) and R8 (menu) | Overlay splice needs assembled base frame | — | Risk-analyst ordering hazard #3 | implementer | Open |
| DEP-4 | R3 (saveBanner timer) designed alongside R6 | Same render change site; banner moves to session-header slot in same commit | — | Risk-analyst ordering hazard #4 | implementer | Open |
| DEP-5 | EC7 (phase guard) must precede EC20 (phase flash) | Flash is a post-guard decoration | — | Risk-analyst ordering hazard #5 | implementer | Open |
| DEP-6 | R5 (revealedField reset) before any dialog rendering work | Secret must be masked before overlay rendering exposes it | — | Risk-analyst ordering hazard #6 | implementer | Open |

---

## Open Decisions (PM auto-settled under auto-mode authority)

The following four open questions have clear evidence-based answers the team would reach without user input. They are settled here.

**OD-1: Timer pattern for saveBanner clear (JD-Q4, C2, C3)**

Settled: `tea.Tick(3*time.Second, clearSaveBannerMsg{gen})` with a `bannerGen int` counter on Model. Rationale: (a) the generation counter is the only correct pattern — the concurrency-analyst correctly identifies that a lazy-timestamp-in-View approach violates Bubble Tea value semantics (View must be pure and must not read `time.Now()` to decide what to render, because the same Model value must produce the same View output); (b) `tea.After` is not in the standard `bubbletea` API as of the version this project imports — `tea.Tick` is the correct function; (c) the fact that no prior Model-owned timer exists in `src/internal/` is not a reason to avoid the pattern — it is a reason to document it when it is first introduced.

**OD-2: Time injection seam (JD-Q9)**

Settled: `nowFn func() time.Time` field on Model, defaulting to `time.Now`. Rationale: this is the standard Go dependency-injection pattern for time, it does not require message plumbing, and the test-engineer correctly identifies it as the prerequisite for synchronous banner-clear testing without `time.Sleep`. The run-mode heartbeat precedent (passing `time.Now()` in a message) applies to an externally driven tick, not an internal one — the two patterns are not in conflict.

**OD-3: uichrome extraction vs. re-export (JD-Q1, A1, A3)**

Settled: new `src/internal/uichrome/` package as the software-architect (A1, A3) recommends. Rationale: re-exporting from `internal/ui` creates an import cycle risk if `internal/workflowedit` already imports or could import `internal/ui`, and re-exporting is a form of package-level name aliasing that obscures the true source. `internal/uichrome` is the correct dependency direction: both `internal/ui` and `internal/workflowedit` import a shared primitive package, rather than one importing the other. The narrow-reading ADR (`20260410170952`) governs Go-code-vs-config.json separation, not intra-Go package structure; JD-Q2 correctly concluded this is a non-issue.

**OD-4: Commit graph structure (JD-Q12)**

Settled: incremental commit graph, `make ci` green at each step. Sequence: (1) test seam commit — convert 8 render-test files from exact-string to ANSI-stripped substring assertions, add `nowFn` / `bannerGen` / `clearBannerMsg` fields with no production behavior change; (2) `uichrome` package — extract primitives, alias in `internal/ui`, add D45 palette constants; (3) fix panelH + phase guard + revealedField reset — all behavioral corrections that are independent of render; (4) View chrome frame + saveBanner timer (D2/D9/D14 render); (5) session header (D5/D13/D14/D16/D17); (6) menu bar (D4/D10/D11/D12); (7) outline render (D20–D25/D49); (8) detail pane render (D26–D33/D47/D50/D51); (9) dialogs render (D36/D37); (10) help modal + findings panel (D38/D39/D40); (11) empty-editor layout (D43); (12) missing behavioral gaps in one batch (D48 guard, D34/Validating footer, D32/Ctrl+E wiring, browse-only render, makeLoadCmd banner fix, banner tag text); (13) documentation update. Rationale: each step is independently reviewable and `make ci` green; the sequence respects the ordering hazards enumerated in DEP-1 through DEP-6.

---

## Scope, Definition of Done, and Smallest Viable Slice

**Definition of done.** Every one of the 51 D# decisions is rendered correctly in at least one testable path. `make ci` passes with zero lint suppressions. The 8 existing render-test files are updated to ANSI-stripped substring assertions and the 2 new test files (`chrome_render_test.go`, `findings_render_test.go`) exist. Documentation files named in DevOps#9 ship in the same PR.

**Acceptance criteria (measurable):**
1. `go test -race ./...` passes.
2. `make lint` passes with zero suppressions.
3. `TestView_MinimumSizeRendersTooSmallMessage` (D48) passes.
4. `TestView_FrameHas8ChromeRows` (D9) passes.
5. `TestView_SessionHeaderContainsDirtyIndicatorWhenDirty` (D13) passes — uses `IsDirty()` not `m.dirty`.
6. `TestView_SaveBannerClearedAfter3Seconds` (D14) passes without `time.Sleep` (uses `nowFn` override).
7. Dialog render tests contain `╭` chrome assertion per test-engineer file triage.
8. `docs/features/workflow-builder.md` and `docs/code-packages/workflowedit.md` updated.

**Smallest viable slice.** The first two commits (test seam + `uichrome` extraction) are shippable to the branch independently. Subsequent commits are stacked but each leaves `make ci` green.

**Rollback story.** Each commit in the sequence is independently revertable. The visual layer is all in render functions; no behavioral state changes except for the four targeted fixes (panelH, phase guard, revealedField reset, banner tag text). If a render commit regresses, `git revert` restores the placeholder.

**Post-ship owner.** `workflow-builder-tui` branch owner (current: River). After merge, the `workflowedit` package render code is documented in `docs/code-packages/workflowedit.md`.

**Unassigned follow-up work (in-scope but deferred to next PR):**
- P2 risk items: R19 (sub-30-col mid-resize dialog overflow), R20 (CJK widths with `lipgloss.Width`), R21 (mouse-routing off-by-1), R22 (tab/null chars), R23 (path-picker edge cases), R24 (findings panel dim under help modal), R25 (rapid-save tick race — generation counter resolves this; the race itself is P2), R26 (D49 truncation — D49 is committed; implementation goes in detail render commit), R27 (D17 overflow priority), R28 (D51 dropdown flip).
- P3 items: D41 phase-boundary flash animation (depends on EC7 fix being in place first), WG drain wiring, `Validating…` footer (B9 — already in commit sequence above, re-check scope).

Note: R26 (D49 truncation) and R28 (D51 dropdown flip) are committed decisions in the spec; their implementation should be included in the outline and detail render commits respectively, not deferred. The risk-analyst placed them P2 because they are enhancement behaviors, but since D49 and D51 are spec decisions, they belong in this PR.

---

## Inconsistencies and Standards Conflicts

**IC-01: Lint suppressions prohibition.**
`docs/coding-standards/lint-and-tooling.md` — "Lint suppressions are prohibited in any form." The implementation must not introduce `//nolint` comments even for `funlen`/`gocyclo` on long render functions. Resolution: decompose per A2/A4 recommendations before lint fires. If `.golangci.yml` thresholds are generous enough, decomposition remains good practice rather than a lint requirement — but either way, no suppressions.

**IC-02: File-write rule for `clearBannerMsg` timer.**
`docs/coding-standards/file-writes.md` — no direct conflict; timer message does not write files. No action required.

**IC-03: `m.dirty` vs `IsDirty()` for D13 dirty indicator.**
`docs/coding-standards/api-design.md` — no explicit rule, but the `IsDirty()` method was added precisely to provide the correct dirty-detection path. Using `m.dirty` in the render path while `IsDirty()` exists is an API misuse. Resolution: render must read `m.IsDirty()` (B12, UX#3, JD-Q10). `m.dirty` may remain as a performance cache for Update paths that need fast mutation tracking.

**IC-04: Documentation shipping rule.**
`docs/coding-standards/documentation.md` — "feature docs must ship with the feature (not as follow-ups)." The visual layer is a feature completion. DevOps#9 enumerates the four required doc updates. These are P0 from the documentation standard's perspective, not optional polish.

**IC-05: `HintEmpty` constant name mismatch with behavioral D-30.**
`constants.go:30` comment says `// HintEmpty is the centred hint shown when no workflow is loaded (D-30).` The D-30 cited is the *behavioral* D-30, not a visual spec decision. This is a documentation inconsistency in the code comment, not a behavioral conflict. When `HintEmpty` is replaced by the D43 bordered layout, the constant itself is superseded; the comment can be removed.

**IC-06: Reverse-video on mockup variant C.**
Visual spec `05-detail-pane-fields.md` variant C annotation conflicts with decision-log D22 (clarified) and D28. Decision-log governs per the spec's own authority hierarchy. The implementation must follow D22: no reverse-video on field body; reverse-video is reserved for reorder mode and open-dropdown highlighted item. This is a mockup annotation error, not a D#-vs-D# contradiction.

**IC-07: `saveBanner` placement conflicts with D9 chrome budget.**
Current `model.go:289-295` renders `saveBanner` as a separate row, placing it outside the 8-row chrome budget. D14 commits transient banners to the session-header banner slot. These are mutually exclusive placements. Resolution: banner moves to session-header slot in the View chrome frame rebuild (commit 4 in OD-4 sequence). The `saveBanner` field becomes the `m.banners.saveBanner` sub-field or is integrated into banner priority resolution.

---

## Future-State Concerns

**FS-01: `model.go` growth trajectory.**
`model.go` is 1,831 lines. Visual layer adds ~900 lines if absorbed in place. Without decomposition per A2's render-file map, the file becomes unnavigable and every bug report requires context-loading 2,700+ lines. Resolution: implement A2's file map (`render_frame.go`, `render_session_header.go`, etc.) as part of this PR. The test-engineer's test files already assume per-component test files, which reinforces this decomposition.

**FS-02: `internal/ui` and `internal/workflowedit` sharing chrome primitives.**
If `internal/uichrome` is not extracted, any future chrome change (a glyph, a color) requires a coordinated multi-package edit. After extraction, both TUIs inherit changes automatically. This is a maintenance concern that compounds with every future visual iteration.

**FS-03: Bubble Tea value semantics and Model-owned timers.**
Introducing `tea.Tick` as a Model-owned timer (OD-1) establishes a new pattern in `workflowedit`. Future contributors must understand that `clearBannerMsg{gen}` is the generation-counter pattern and that raw timestamps in `View()` are prohibited. This should be documented in the code (a comment above the `bannerGen` field) and in `docs/code-packages/workflowedit.md`.

**FS-04: Resize-during-overlay correctness.**
C5 notes that Bubble Tea does not redeliver `tea.WindowSizeMsg` after `tea.ExecProcess` returns (D44 post-handoff resize). The implementation must explicitly re-query terminal size on post-ExecProcess return or document this as a known limitation. This is a forward-looking reliability concern for editor integrations on window-resizable terminals.

**FS-05: CJK and double-width character widths (P2).**
EC12 identifies that any string-length comparison must use `lipgloss.Width` rather than `len()` for D49 truncation, D50 label truncation, and session-header overflow priority. This is a correctness concern for international users. It is deferred to P2 but the render commit should use `lipgloss.Width` from the start to avoid a later fixup pass.

**FS-06: Version label.**
DevOps#8 confirms no version bump is required per `docs/coding-standards/versioning.md`. The visual layer is not part of the public API. If the team later wants a version-tagged release, a separate `0.7.4` patch commit is the correct vehicle.

---

## Spec-Maturity Classification of All R1 Findings

### Gap-fill items (placeholder code superseded by the visual spec — implement per spec)

These are not D#-vs-D# contradictions. Each is placeholder code that must be replaced by the committed spec behavior.

| Finding IDs | Description | Relevant D# | Classification |
|---|---|---|---|
| S10, R6 | View() flat `\n` assembly | D2 | gap-fill |
| S7, B6, R9 | `renderSessionHeader` wrong glyph `*` not `●`, wrong tags, missing slots | D13, D14, D16 | gap-fill |
| S3, R7 | `renderDialog()` flat strings | D36 | gap-fill |
| S13, R8 | `menu.go render()` flat string | D4, D11 | gap-fill |
| R11 | `renderHelpModal()` flat string | D40 | gap-fill |
| S12, EC19 | `HintEmpty` / `renderEmptyEditor()` | D43 | gap-fill |
| S11, R13 | `findings.go` no render method | D38, D39 | gap-fill |
| B11, UX#1 | Banner long-form tags | D14 | gap-fill |
| S5, B3, DevOps#1, R1 | `panelH = m.height - 2` | D9 | gap-fill |
| B4, EC2, DevOps#6, R2 | D48 guard absent | D48 | gap-fill |
| B9, UX#6 | Validating… footer absent | D34 | gap-fill |
| B10, R18 | Browse-only not in View() | D12, D14 | gap-fill |

### Plan-level items (resolvable in implementation planning — no spec change needed)

| Finding IDs | Description | Resolution path |
|---|---|---|
| A1–A8 | Package structure, file decomposition, test strategy | OD-3 (uichrome), OD-4 (commit graph), A2 (render files) |
| C1, B13, UX#10 | WindowSizeMsg tier-0 routing | Three-line addition to tier-0 pre-dispatch per JD-Q13 |
| C2, C3, B1, EC4 | saveBanner timer and generation counter | OD-1 settled |
| C9 | `makeSaveCmd` companions reference (latent, not yet a bug) | Document; deep-copy if usage pattern changes |
| B2, EC7 | Phase-boundary guard in doMoveStepUp/Down | Add Phase-equality check before swap |
| B12, JD-Q10 | `m.dirty` vs `IsDirty()` in render | Render reads `m.IsDirty()`; `m.dirty` remains for Update fast path |
| B8, JD-Q8 | Ctrl+E from detail pane unwired | Wire `fieldKindMultiLine` handler in updateEditView |
| EC5, R5 | revealedField not reset on dialog open | Reset in dialog-open paths |
| B5, CL-26 | makeLoadCmd reload path loses banner signals | Populate banner fields in Recovery-dialog inline return |
| JD-Q4, C2, C3 | Timer pattern | OD-1 settled |
| JD-Q9 | Time injection seam | OD-2 settled |
| JD-Q12 | Commit graph | OD-4 settled |
| DevOps#2, CL-28 | Named constants required | Add to `uichrome/palette.go` or `workflowedit/constants.go` |
| DevOps#5, CL-30 | Session-event log entries | Add per state-transition, matching existing vocabulary |
| DevOps#9, CL-31 | Documentation must ship | Four files enumerated; P0 per coding-standards |
| A5 | `fieldKindMultiLine` missing from enum switch | Add to enum and render switch |
| S6, A3, CL-16 | D45 palette constants missing | Add to `uichrome/palette.go` |
| C5, FS-04 | Post-ExecProcess resize | Re-query terminal size on post-return cycle |
| EC12, FS-05 | CJK widths | Use `lipgloss.Width` throughout render code |

### Items that are deferred to a follow-on PR (P2/P3 per risk-analyst)

EC12 (CJK) should move to plan-level per FS-05 note — using `lipgloss.Width` from the start costs nothing. R26 (D49 truncation) and R28 (D51 dropdown flip) are committed spec decisions and belong in this PR. All others (R19–R25, R27, R29–R33) are correctly deferred.

---

## D# Contradiction Assessment (Spec-Maturity Gate)

The risk-analyst flagged 9 D# escalations. Assessment per the gate criteria:

**Not true D#-vs-D# contradictions (gap-fill items):**
- D2, D4, D9, D11, D13, D14, D36, D43, D48 — all are cases where placeholder code does not yet implement a committed spec decision. The spec decision is correct; the placeholder must be replaced. No mutual exclusivity between decisions.

**Potential true contradictions — assessed and closed:**

1. **GAP-011 / UX#2 (mockup 05 variant C reverse-video vs. D22+D28):** The mockup annotation is erroneous. Decision-log D22 (clarified) governs explicitly: "no reverse-video on the field body." This is a mockup annotation error, not a D#-vs-D# conflict. Decision-log governs per the spec's own authority hierarchy (the clarification entry in D22 was written precisely to settle this ambiguity). Closed as gap-fill.

2. **HintEmpty / D43 (S12, EC19, JD-Q6, C10):** `renderEmptyEditor()` returns a flat `HintEmpty` constant. D43 commits a bordered outline+detail layout with a hint panel. This is placeholder code vs. spec, not two committed decisions in mutual conflict. D43 is the authoritative commitment; the placeholder must be replaced. Closed as gap-fill.

**Gate result: DOES NOT TRIP.** Zero true D#-vs-D# contradictions identified. Both surface findings are gap-fill items where the spec is correct and the placeholder must be replaced. Proceed to synthesis.

---

## Open Questions

**OQ-1: Does `fieldKindMultiLine` exist in the shipped code, or must it be added?**
- Why it matters: D32 requires a multi-line field kind; A5 and JD-Q8 assert it is missing. If it already exists under a different name, the render commit just adds a render case; if absent, a new constant must be added to the enum.
- Specialist or evidence that would resolve: code search `grep -rn "fieldKind" /Users/mxriverlynn/dev/mxriverlynn/pr9k/src/internal/workflowedit/`.
- Blocks synthesis: No — the question does not change the overall plan; it affects only whether the first commit in the enum group adds a constant or adds a render case.

**OQ-2: Will `funlen`/`gocyclo` golangci-lint thresholds fire on the render functions even with per-kind decomposition per A4?**
- Why it matters: DevOps#3 flags this as a likely P0; without knowing the project's lint thresholds, we cannot be certain decomposition is sufficient.
- Specialist or evidence that would resolve: `cat /Users/mxriverlynn/dev/mxriverlynn/pr9k/.golangci.yml` to read configured thresholds.
- Blocks synthesis: No — decomposition is good practice regardless; if thresholds are exceeded, further decomposition is always available.

**OQ-3: Is the post-ExecProcess terminal-resize gap (C5/FS-04) addressed by any existing Bubble Tea version or workaround in the codebase?**
- Why it matters: D44 commits to "if the terminal was resized during the editor session, the resize is treated as a `tea.WindowSizeMsg` on the first post-return render cycle." If Bubble Tea doesn't redeliver it, this D44 commitment is currently false.
- Specialist or evidence that would resolve: `concurrency-analyst` review of `tea.ExecProcess` semantics in the project's bubbletea version; check `go.mod` for bubbletea version.
- Blocks synthesis: No — it can be documented as a known limitation in this PR with a follow-on issue, or addressed with an explicit terminal-size re-query on return.

**OQ-4: Single-PR vs. multi-PR delivery.**
- Why it matters: The commit graph in OD-4 describes 13 commits. The user may prefer a single PR or two PRs (visual layer + behavioral fixes).
- Why this is a user-preference question and not auto-settled: The risk-analyst and devops perspectives both support incremental delivery, but the user's preference on PR granularity (single large visual PR vs. split into behavioral-fixes PR + visual PR) is a delivery judgment that is not answerable from evidence alone.
- Blocks synthesis: No — the commit graph works either way.

---

## Specialist Handoffs

| Specialist | Question | Evidence they need |
|---|---|---|
| `adversarial-validator` | Validate the completed synthesized implementation plan before it is handed to the implementer | The synthesized plan (output of synthesis mode), all R1 findings, this facilitation summary |
| `concurrency-analyst` | Confirm that `tea.Tick(3s, clearBannerMsg{gen})` with generation counter correctly prevents stale clears under rapid-save, and that the `bannerGen` counter increment/compare is race-free under Bubble Tea's single-goroutine Model contract | `messages.go`, `model.go:Update`, OD-1 decision |
| `test-engineer` | Produce a complete D#-to-test-name coverage matrix for the implementation plan, annotated with which commit in OD-4's sequence each test ships in | Test-engineer R1 output, OD-4 commit sequence, D1–D51 decisions |

---

## Next Step for the Conversation

**Go to synthesis.**

The gate does not trip. All 10 specialists have been heard. The four open questions do not block synthesis — none requires a spec change, and three are answerable during implementation. The one user-preference question (OQ-4, PR granularity) can be recorded as an open item in the synthesized plan with a default recommendation (single PR with 13 incremental commits, which is already OD-4).

The synthesis pass should:
1. Record D1–D51 as the decision baseline (committed, not re-debated).
2. Record OD-1 through OD-4 as PM-settled decisions.
3. Produce the implementation commit sequence per OD-4 with per-commit acceptance criteria.
4. Carry forward the RAID log (risks R1–R10, assumptions A1–A7, issues I1–I3, dependencies DEP-1 through DEP-6).
5. Name the `adversarial-validator` as the final gate before implementation begins.

---

## Summary

Ten R1 specialists covering all relevant domains (architecture, structure, behavior, concurrency, edge cases, testing, UX, DevOps, risk, and a generalist stress-test) were heard across a visual-design implementation planning session for the `pr9k workflow` builder TUI. All D# contradiction escalations (9 total from risk-analyst) were assessed as gap-fill items — placeholder code that must be replaced by committed spec decisions — not true mutual conflicts. The spec-maturity gate does not trip. Four open questions requiring PM decisions were auto-settled under auto-mode authority (OD-1 through OD-4). The plan is ready for synthesis.

| Log category | Count |
|---|---|
| Evidenced / Anecdotal / Disputed claims | 34 / 2 / 0 |
| Risks / Assumptions / Issues | 10 / 7 / 3 |
| Decisions committed (PM auto-settled) | 4 |
| Open Questions | 4 |
| Specialist handoffs | 3 |

Next step: Go to synthesis

Facilitation summary written to: `/Users/mxriverlynn/dev/mxriverlynn/pr9k/docs/plans/workflow-builder-tui-design/artifacts/facilitation-summary.md`
