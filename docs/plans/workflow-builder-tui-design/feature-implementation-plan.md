# Feature Implementation Plan: Workflow-Builder TUI Visual-Design

Replace all placeholder render strings in `src/internal/workflowedit/` with the full bordered, multi-pane visual layout committed by the visual spec's 51 D# decisions, shipped as a single PR with 13 incremental commits each leaving `make ci` green and accompanied by the four required documentation updates.

## Source Specification

- **Feature specification:** [feature-specification.md](feature-specification.md)
- **Specification decision log:** [artifacts/decision-log.md](artifacts/decision-log.md)
- **Specification team findings:** [artifacts/team-findings.md](artifacts/team-findings.md)
- **Specification visual gaps:** [artifacts/visual-gaps.md](artifacts/visual-gaps.md)
- **Facilitation summary (R1):** [artifacts/facilitation-summary.md](artifacts/facilitation-summary.md)
- **Specification decisions this plan inherits:** D1–D51 (the full visual-spec decision log; treated as committed mechanics; not re-debated).
- **Specification open items this plan must respect or resolve:** OI-1, OI-2 from the visual spec (deferred — see Open Items below).
- **No `feature-technical-notes.md` exists for this spec.** The 51 D# entries in `artifacts/decision-log.md` are the load-bearing committed mechanics.
- **Inherited behavioral context (do not modify):** [`../workflow-builder/feature-specification.md`](../workflow-builder/feature-specification.md) (behavioral spec — what the builder *does*); [`../workflow-builder/feature-implementation-plan.md`](../workflow-builder/feature-implementation-plan.md) (behavioral implementation plan); [`../workflow-builder/artifacts/implementation-decision-log.md`](../workflow-builder/artifacts/implementation-decision-log.md) (behavioral D-1..D-47); [`../workflow-builder-pr2/feature-implementation-plan.md`](../workflow-builder-pr2/feature-implementation-plan.md) (PR-2 cherry-pick + gap-close that delivered the behavioral implementation, merged at `ed8203e`).

## Outcome

When this plan is executed, a user launching `pr9k workflow` against any of the 28 behavioral states sees a product-consistent, full-screen TUI: a 9-row chrome frame with menu bar, session header (title, banner slot, dirty indicator, findings summary), bordered outline pane, bordered detail pane with field bracket grammar, dialog overlays with the D36 chrome, the D40 help modal, the D38/D39 findings panel, and the D43 empty-editor layout. The fixed chrome budget (`ChromeRows = 8`) is enforced; the D48 minimum-size guard short-circuits below 60×16; transient banners auto-clear via `tea.Tick` with generation-counter race protection; `Validating…` shows in the footer during validation; browse-only mode signals greyed Save, suppressed dirty indicator, and `[ro]` banner; reorder mode flashes on phase-boundary decline. `make ci` passes with zero lint suppressions. The four required doc updates ship in the same PR ([D-21](artifacts/implementation-decision-log.md#d-21-documentation-ships-in-same-pr)).

## Context

- **Driving constraint:** The behavioral implementation merged at commit `ed8203e` (`Workflow builder pt 2`). The visual layer is the remaining PR-sized unit before the workflow-builder is feature-complete. Every day it ships as placeholder strings degrades the product impression for early users and erodes the contract the visual spec was written to satisfy.
- **Stakeholders:** Workflow authors who want a usable builder TUI (see a real interface, not debug strings). The maintainer who needs `make ci` green with no lint suppressions. Future contributors who need documented render patterns to extend.
- **Future-state concern:** `model.go` is at 1,831 lines today; visual layer adds ~900 lines if absorbed in place. Without the per-surface render-file decomposition ([D-1](artifacts/implementation-decision-log.md#d-1-render-code-stays-in-internalworkflowedit-package-decomposed-into-per-surface-render-files)), `model.go` becomes an unnavigable convergence trap. Without the `internal/uichrome` extraction ([D-2](artifacts/implementation-decision-log.md#d-2-extract-chrome-primitives-to-new-internaluichrome-package)), every future chrome change requires a coordinated multi-package edit; both TUIs (run-mode and workflow-builder) must inherit chrome changes automatically. The shape of the extraction anchors the dependency graph for both TUIs going forward.
- **Out-of-scope boundary:** No behavioral changes (the behavioral spec is shipped). D1–D51 are committed mechanics; they are not re-debated. P2/P3 risk-analyst items (R19–R25, R27, R29–R33) are deferred to a follow-on PR. D49 step-name truncation and D51 dropdown overflow flip are committed visual decisions and ship in this PR (commits 7 and 8 respectively). The D41 phase-boundary flash decoration depends on the EC7 phase guard (this PR delivers the guard at commit 3; flash decoration is a thin add-on that is included in commit 3 since the same `tea.Tick` + generation-counter discipline applies).

## Team Composition and Participation

| Specialist | Status | Key Input |
|---|---|---|
| `project-manager` | Coordinator | Facilitated R-1 (10 parallel specialists), built the claim ledger of 36 entries (34 evidenced, 2 anecdotal), settled OD-1..OD-4 under auto-mode authority, ran the spec-maturity gate (did not trip), synthesized this plan. |
| `software-architect` | Active | A1–A8: `internal/uichrome` extraction plan; render file map (D-1); dialog and field switch decomposition (D-3, D-4); render-time geometry computation (D-5); test strategy (D-6); D# contradiction note on mockup variant C reverse-video (D-25). |
| `structural-analyst` | Active | S1–S13: 13 structural findings on file growth, unexported chrome primitives, missing render methods, View() flat assembly incompatibility with D2, palette gaps, dialog-handler convergence trap. Cited line numbers throughout. |
| `behavioral-analyst` | Active | B1–B13: 13 behavioral gap findings — saveBanner clear (D-7), phase-boundary guard (D-12), chrome budget (D-20), D48 guard (D-19), banner tags (D-10), dirty indicator (D-11), browse-only (D-18), Validating… footer (D-17), HintEmpty (D-26), revealedField reset (D-13), WindowSizeMsg routing (D-14), reload-path banner forwarding (D-15), Ctrl+E wiring (D-16). |
| `concurrency-analyst` | Active | C1–C11: WindowSizeMsg drop and tier-0 routing fix (D-14); saveBanner permanent + generation-counter pattern (D-7); D41 flash one-frame discipline (D-12); stale `m.width` after missed resize; post-ExecProcess resize (OQ-3); validateCompleteMsg coexistence with help modal. |
| `edge-case-explorer` | Active | EC1–EC20: prioritized critical/high/medium/low edge cases — chrome budget (D-20), D48 fallback (D-19), revealedField secret leak (D-13), dropdown overflow flip (visual-spec D51 in commit 8), step-name truncation (visual-spec D49 in commit 7), CJK widths (D-24). |
| `test-engineer` | Active | File-by-file triage of 8 modify + 2 new render-test files; seam requirements (`nowFn` D-8, `clearSaveBannerMsg{gen}`, `bannerGen` D-7, `assertModalFits`); D# → test-name coverage matrix; ANSI-stripped substring + structural assertion strategy (D-6). |
| `user-experience-designer` | Active | 10 findings: banner short-form tag contract (D-10, color-blind safety), reverse-video discipline (D-25), dirty indicator source (D-11), browse-only signals (D-18), save-flow feedback (D-7), Validating… footer (D-17), phase-boundary feedback (D-12), secret re-mask (D-13), findings-panel dim coexistence, resize-during-overlay (D-14). |
| `devops-engineer` | Active | 12 findings: panelH bug (D-20), named constants requirement, funlen/gocyclo risk (D-3 decomposition), clearSaveBanner timer (D-7), session-event log entries (D-23), D48 guard (D-19), version policy (D-22), documentation-in-same-PR (D-21). |
| `risk-analyst` | Active | P0–P3 register (R1–R33) carried forward as the plan's RAID risks; ordering hazards 1–6 carried forward as DEP-1..DEP-6; 9 D# escalations re-classified as gap-fill (zero true contradictions). |
| `junior-developer` | Active | 14 questions; 4 escalated to PM auto-decisions (uichrome, timer, time-injection, commit graph); 10 resolved by direct evidence. |
| `adversarial-validator` | Queued (post-synthesis) | Will validate this plan before the implementer begins. Input: this plan, the decision log, the iteration history, the facilitation summary, and the visual spec. See "Specialist Handoffs for Implementation". |
| `gap-analyzer` | Stood down | Visual-spec gaps were already resolved in team-findings F1–F25; no gap-analysis pass required for this implementation plan. |
| `information-architect` | Stood down | Documentation plan is enumerated by D-21 (four named files); no IA work required beyond updating those files. |

## Implementation Approach

### Architecture and Integration Points

Render code stays in `internal/workflowedit` and is decomposed by surface into per-file render modules ([D-1](artifacts/implementation-decision-log.md#d-1-render-code-stays-in-internalworkflowedit-package-decomposed-into-per-surface-render-files)). Shared chrome primitives are extracted into a new package `src/internal/uichrome/` consumed by both `internal/ui` (run-mode TUI) and `internal/workflowedit` (workflow-builder TUI) ([D-2](artifacts/implementation-decision-log.md#d-2-extract-chrome-primitives-to-new-internaluichrome-package)).

**Render file map** (post-implementation `internal/workflowedit/`):

```
model.go                 (existing — Update routing, Model struct, Cmd factories)
render_frame.go          (NEW — View() entry, 9-row chrome assembly, D48 guard)
render_session_header.go (NEW — D5/D13/D14/D15/D16/D17)
render_outline.go        (REPLACES outline.go:render — D18..D25, D49)
render_detail.go         (REPLACES detail.go:render — D26..D33, D47, D50, D51)
render_menu.go           (REPLACES menu.go:render — D4, D10, D11, D12)
render_dialogs.go        (NEW — D36, D37; renderDialogShell + dialogBodyFor)
render_findings.go       (NEW — D38, D39)
render_help.go           (NEW — D40)
render_empty.go          (NEW — D43, supersedes HintEmpty + renderEmptyEditor)
render_footer.go         (REPLACES footer.go:ShortcutLine — D34 Validating guard)
```

**`internal/uichrome/` package** (new):

```
palette.go        — LightGray, White, Green, Red, Yellow, Cyan, Dim,
                    ActiveStepFG, ActiveMarkerFG (D45 additions live here)
chrome.go         — WrapLine, HRuleLine, BottomBorder, RenderTopBorder
title.go          — ColorTitle
shortcut.go       — ColorShortcutLine
overlay.go        — Overlay, SpliceAt
constants.go      — MinTerminalWidth=60, MinTerminalHeight=16,
                    DialogMaxWidth=72, DialogMinWidth=30,
                    HelpModalMaxWidth=72 (others stay in workflowedit)
```

`internal/ui/header.go` is refactored to alias the existing palette names to `uichrome` values (no behavioral change in run-mode). `internal/ui/model.go` delegates the existing helpers to `uichrome` (functions stay the same; bodies become thin wrappers). `internal/workflowedit` imports `uichrome` directly.

**Why `uichrome` and not re-export from `internal/ui`?** Re-exporting forces `internal/workflowedit` to import `internal/ui`, dragging in run-mode model dependencies and risking import cycles. Both TUIs at the same dependency depth on a shared primitive package is the correct dependency direction ([D-2](artifacts/implementation-decision-log.md#d-2-extract-chrome-primitives-to-new-internaluichrome-package)). The narrow-reading ADR (`20260410170952-narrow-reading-principle.md`) governs Go-code-vs-config.json separation, not intra-Go package structure.

**Dialog rendering grammar** ([D-3](artifacts/implementation-decision-log.md#d-3-dialog-rendering--renderdialogshell--dialogbodyfor-per-kind-body-builders)):

```go
// render_dialogs.go (sketch — not final code)
type dialogBody struct {
    title  string
    rows   []string
    footer string
    width  int
}

func renderDialog(m Model) string {
    body := dialogBodyFor(m.dialog.kind, m.dialog.payload)
    return renderDialogShell(body, m.width, m.height)
}

func dialogBodyFor(kind DialogKind, payload any) dialogBody {
    switch kind {
    case DialogQuitConfirm:    return buildQuitConfirmBody(payload)
    case DialogSaveConfirm:    return buildSaveConfirmBody(payload)
    case DialogOpenPath:       return buildOpenPathBody(payload)
    case DialogNewPath:        return buildNewPathBody(payload)
    case DialogRecovery:       return buildRecoveryBody(payload)
    case DialogValidationResults: return buildValidationResultsBody(payload)
    case DialogChoosePath:     return buildChoosePathBody(payload)
    case DialogConflictReload: return buildConflictReloadBody(payload)
    case DialogFindingsPanel:  return buildFindingsPanelBody(payload)
    case DialogReorderPrompt:  return buildReorderPromptBody(payload)
    case DialogStepKindPicker: return buildStepKindPickerBody(payload)
    case DialogModelSuggest:   return buildModelSuggestBody(payload)
    case DialogConfirmDelete:  return buildConfirmDeleteBody(payload)
    case DialogUnsavedExit:    return buildUnsavedExitBody(payload)
    case DialogSaveError:      return buildSaveErrorBody(payload)
    }
    return dialogBody{} // unreachable; switch is exhaustive
}

func renderDialogShell(body dialogBody, w, h int) string {
    // D36: bordered overlay, centered, max width 72, min width 30,
    // chrome includes top border with title, body rows, footer line.
    width  := clamp(body.width, DialogMinWidth, min(w-4, DialogMaxWidth))
    height := len(body.rows) + 4 // top border + title + body + footer + bottom border
    overlay := buildOverlay(body, width, height)
    top, left := centerOffset(w, h, width, height)
    return uichrome.SpliceAt(baseFrame(), overlay, top, left)
}
```

**Field rendering grammar** ([D-4](artifacts/implementation-decision-log.md#d-4-field-rendering--fieldkind-enum-switch-with-fieldkindmultiline-added)):

```go
// render_detail.go (sketch — not final code)
type FieldKind int
const (
    fieldKindText FieldKind = iota
    fieldKindChoice
    fieldKindNumeric
    fieldKindModelSuggest
    fieldKindSecretMask
    fieldKindMultiLine // NEW (D-16)
)

func renderField(f field, focused bool, width int) string {
    switch f.kind {
    case fieldKindText:        return renderTextField(f, focused, width)
    case fieldKindChoice:      return renderChoiceField(f, focused, width)
    case fieldKindNumeric:     return renderNumericField(f, focused, width)
    case fieldKindModelSuggest:return renderModelSuggestField(f, focused, width)
    case fieldKindSecretMask:  return renderSecretMaskField(f, focused, width)
    case fieldKindMultiLine:   return renderMultiLineField(f, focused, width)
    }
    return "" // unreachable
}
```

`renderChoiceField` owns the D51 dropdown overflow flip logic. `renderMultiLineField` renders the D32 `↩ Ctrl+E to edit` action row. Reverse-video is reserved for two states only — reorder-mode active step and open-dropdown highlighted item — per [D-25](artifacts/implementation-decision-log.md#d-25-mockup-05-variant-c-reverse-video-annotation-overruled-by-visual-spec-d22--d28).

### Data Model and Persistence

No data-model changes. The visual layer is a pure render concern. Two Model fields are added in commit 1 with no behavior:
- `nowFn func() time.Time` — time-injection seam ([D-8](artifacts/implementation-decision-log.md#d-8-time-injection--nowfn-functime-time-model-field)), defaults to `time.Now`.
- `bannerGen int` — generation counter for save-banner clear discipline ([D-7](artifacts/implementation-decision-log.md#d-7-savebanner-clear-pattern--teatick3s-clearsavebannermsggen-with-bannergen-counter)).

Commit 3 adds:
- `boundaryFlash uint64` — phase-boundary flash sequence ([D-12](artifacts/implementation-decision-log.md#d-12-phase-boundary-guard-in-domovestepupdown-with-boundaryflash-field--teatick-cleared-signal)).

The `m.dirty` write-through field stays as a fast-path cache; render reads `m.IsDirty()` ([D-11](artifacts/implementation-decision-log.md#d-11-dirty-render-source--misdirty-not-mdirty)).

The save durability contract (atomic temp+rename, companion-first ordering) is unchanged; visual changes do not touch `internal/workflowio` or `internal/atomicwrite`. ADR `20260424120000-workflow-builder-save-durability.md` is preserved.

### Runtime Behavior

**`View()` row assembly** ([D-1](artifacts/implementation-decision-log.md#d-1-render-code-stays-in-internalworkflowedit-package-decomposed-into-per-surface-render-files), [D-5](artifacts/implementation-decision-log.md#d-5-render-time-geometry-computation--d48d51d17d49-in-view-not-update), [D-19](artifacts/implementation-decision-log.md#d-19-60×16-minimum-size-fallback-render), [D-20](artifacts/implementation-decision-log.md#d-20-chromerows--8-chrome-budget)):

```
1. Guard: if width<60 or height<16 → renderTooSmall()
2. Compute layout: panelH = m.height - ChromeRows; outlineW, detailW
3. Top border (1 row)
4. Menu bar (1 row)
5. Session header (2 rows: title row, banner row)
6. Separator (1 row)
7. Outline pane | Detail pane (panelH rows)
8. Separator (1 row)
9. Shortcut/footer (1 row)
10. Overlay splice: dialog if active, help modal if open, findings panel
```

**Layout decisions are computed in `View()`, not `Update()`** ([D-5](artifacts/implementation-decision-log.md#d-5-render-time-geometry-computation--d48d51d17d49-in-view-not-update)). `handleWindowSize` only updates `m.width`, `m.height`, viewport dimensions. Run-mode precedent at `internal/ui/model.go:382-749`.

**Tier-0 WindowSizeMsg routing** ([D-14](artifacts/implementation-decision-log.md#d-14-windowsizemsg-routing-at-tier-0-pre-dispatch)): Add a tier-0 pre-dispatch check before any state-specific tier (help-modal, dialog, edit-view, error). When a `tea.WindowSizeMsg` arrives, call `handleWindowSize` first, then continue routing to the active tier. Three-line addition; no regression risk because current code already ignores resize during dialogs (the change is to fix, not preserve, that behavior).

**Save-banner timer** ([D-7](artifacts/implementation-decision-log.md#d-7-savebanner-clear-pattern--teatick3s-clearsavebannermsggen-with-bannergen-counter)): On save success, increment `m.bannerGen`, set `m.saveBanner`, emit `tea.Tick(3*time.Second, ...) → clearSaveBannerMsg{gen: m.bannerGen}`. The handler clears `m.saveBanner` only if `msg.gen == m.bannerGen` (else stale tick from a prior save would wipe a newer banner). View renders `m.saveBanner` in the session-header banner slot per visual-spec D14, not as a separate row outside the chrome budget.

**Phase-boundary guard** ([D-12](artifacts/implementation-decision-log.md#d-12-phase-boundary-guard-in-domovestepupdown-with-boundaryflash-field--teatick-cleared-signal)): `doMoveStepUp`/`doMoveStepDown` (`model.go:1082-1112`) check `Phase` equality before swap; if phases differ, decline the swap and increment `m.boundaryFlash`. A `tea.Tick(150ms, ...) → clearBoundaryFlashMsg{seq}` clears only when `seq == m.boundaryFlash`. View inverts the cursor-row background for one frame when `m.boundaryFlash != 0`.

**revealedField reset** ([D-13](artifacts/implementation-decision-log.md#d-13-revealedfield-reset-on-every-focus-leave-including-dialog-open-paths)): Every dialog-open path resets `m.detail.revealedField = -1`. A small helper `resetSecretMask(m *Model)` is called from each of the 15 dialog-open Update paths. Tab/Shift-Tab/Esc paths already reset (`model.go:1144`).

**Dirty source** ([D-11](artifacts/implementation-decision-log.md#d-11-dirty-render-source--misdirty-not-mdirty)): Render reads `m.IsDirty()`; `m.dirty` remains the Update fast-path cache. The session-header `●` indicator and the Ctrl+Q quit-confirm both consult `m.IsDirty()`.

**Banner short-form tags** ([D-10](artifacts/implementation-decision-log.md#d-10-banner-short-form-tags--roextsymsharedfields-in-allbannertexts)): `allBannerTexts()` (`model.go:51-73`) emits `[ro]`, `[ext]`, `[sym → target]`, `[shared]`, `[?fields]`. Color-blind-safety contract.

**Reload-path banner forwarding** ([D-15](artifacts/implementation-decision-log.md#d-15-makeloadcmd-reload-path-forwards-all-banner-signals)): `makeLoadCmd` and the recovery-handler inline `openFileResultMsg` literal (`model.go:775-810`, `model.go:1693-1719`) populate all five banner fields (`isReadOnly`, `isExternal`, `isSharedInstall`, `isSymlink`, `symlinkTarget`).

**Browse-only render** ([D-18](artifacts/implementation-decision-log.md#d-18-browse-only-signals-threaded-through-render)): Three signals — greyed Save in File menu (`renderMenuBar`), suppressed dirty indicator (`renderSessionHeader`), `[ro]` banner emit (`allBannerTexts`). The `m.banners.isReadOnly` boolean is threaded into the render functions that need it.

**`Validating…` / `Saving…` footer** ([D-17](artifacts/implementation-decision-log.md#d-17-validating-footer-guard-in-shortcutline)): `ShortcutLine()` early-returns when `m.validateInProgress` or `m.saveInProgress` with the appropriate transient text.

### External Interfaces

**`Ctrl+E` from detail pane** ([D-16](artifacts/implementation-decision-log.md#d-16-fieldkindmultiline-added-to-enum-ctrle-wired-from-detail-pane)): When focus is on a field of kind `fieldKindMultiLine`, `Ctrl+E` dispatches the existing `EditorRunner` path. The handler is the same one already invoked from `updateDialogRecovery`. `PromptFile` and `Command` form fields move from `fieldKindText` to `fieldKindMultiLine`.

No new external surfaces (CLI flags, config schema, env vars, file formats). The visual layer is purely internal rendering.

## Decomposition and Sequencing

The 13-commit graph respects the six ordering hazards in DEP-1..DEP-6 ([D-9](artifacts/implementation-decision-log.md#d-9-commit-graph--13-incremental-commits-make-ci-green-at-each)). Each commit leaves `make ci` green.

| # | Work Unit | Delivers | Depends On | Verification |
|---|---|---|---|---|
| 1 | Test seam migration | (a) 8 render-test files converted from exact-string to ANSI-stripped substring assertions; (b) `nowFn func() time.Time` Model field defaulting to `time.Now`; (c) `bannerGen int` Model field; (d) `clearSaveBannerMsg{gen int}` message type. No production behavior change. | — | All tests pass; `go test -race ./...`; `make lint`; existing render tests still cover existing placeholder strings via substring assertions. |
| 2 | `uichrome` package | New package `src/internal/uichrome/` with `WrapLine`, `HRuleLine`, `BottomBorder`, `RenderTopBorder`, `ColorTitle`, `ColorShortcutLine`, `Overlay`, `SpliceAt`; full palette (`LightGray`, `White`, `Green`, `Red`, `Yellow`, `Cyan`, `Dim`, `ActiveStepFG`, `ActiveMarkerFG` — D45 additions); constants (`MinTerminalWidth=60`, `MinTerminalHeight=16`, `DialogMaxWidth=72`, `DialogMinWidth=30`, `HelpModalMaxWidth=72`). `internal/ui/header.go` aliases existing palette names. `internal/ui/model.go` delegates helpers to `uichrome`. | 1 | Run-mode TUI tests still pass; `make ci` green; `internal/workflowedit` does not yet import `uichrome`. ([D-2](artifacts/implementation-decision-log.md#d-2-extract-chrome-primitives-to-new-internaluichrome-package)) |
| 3 | Behavioral fixes (independent of render) | (a) `panelH = m.height - ChromeRows` ([D-20](artifacts/implementation-decision-log.md#d-20-chromerows--8-chrome-budget)); (b) D48 minimum-size guard added as first line of `View()` returning a centered "Terminal too small" message ([D-19](artifacts/implementation-decision-log.md#d-19-60×16-minimum-size-fallback-render)); (c) phase-boundary guard in `doMoveStepUp/Down` with `boundaryFlash` field and `tea.Tick`-cleared signal ([D-12](artifacts/implementation-decision-log.md#d-12-phase-boundary-guard-in-domovestepupdown-with-boundaryflash-field--teatick-cleared-signal)); (d) `m.detail.revealedField = -1` reset helper called from every dialog-open path ([D-13](artifacts/implementation-decision-log.md#d-13-revealedfield-reset-on-every-focus-leave-including-dialog-open-paths)); (e) `allBannerTexts()` emits short-form tags ([D-10](artifacts/implementation-decision-log.md#d-10-banner-short-form-tags--roextsymsharedfields-in-allbannertexts)); (f) render's dirty source switches to `m.IsDirty()` ([D-11](artifacts/implementation-decision-log.md#d-11-dirty-render-source--misdirty-not-mdirty)); (g) `makeLoadCmd` and recovery-handler inline `openFileResultMsg` literal forward all five banner fields ([D-15](artifacts/implementation-decision-log.md#d-15-makeloadcmd-reload-path-forwards-all-banner-signals)); (h) `fieldKindMultiLine` added to enum; `buildDetailFields` emits it for `PromptFile`/`Command`; Ctrl+E wired from detail pane ([D-16](artifacts/implementation-decision-log.md#d-16-fieldkindmultiline-added-to-enum-ctrle-wired-from-detail-pane)); (i) tier-0 WindowSizeMsg routing fix ([D-14](artifacts/implementation-decision-log.md#d-14-windowsizemsg-routing-at-tier-0-pre-dispatch)). | 1 (test seam absorbs the new tests for these fixes) | Tests pass for D48 guard, phase-boundary decline, banner-tag short-form, dirty-from-IsDirty, banner-forwarding-on-reload, multi-line Ctrl+E dispatch, tier-0 WindowSizeMsg. `make ci` green. |
| 4 | View chrome frame + saveBanner timer | `render_frame.go` with the D2 9-row chrome assembly. `View()` rewritten as ordered row assembly using `uichrome` primitives. `saveBanner` integrated into session-header banner slot per D14 ([D-7](artifacts/implementation-decision-log.md#d-7-savebanner-clear-pattern--teatick3s-clearsavebannermsggen-with-bannergen-counter)). `tea.Tick(3s, clearSaveBannerMsg{gen})` wired on save success. Banner-clear handler validates `msg.gen == m.bannerGen` before clearing. | 2, 3 | `TestView_FrameHas9ChromeRows`, `TestView_SaveBannerClearedAfter3Seconds` (uses `nowFn` override; no `time.Sleep`), `TestView_BannerStaleTickIgnored` (rapid-save race) all pass. |
| 5 | Session header render | `render_session_header.go` implementing D5 5-slot layout (title, banner slot, dirty indicator, findings summary, validation indicator). D13 `●` in Green; D14 short-form banners; D15 banner priority; D16 findings summary; D17 overflow priority. | 4 | `TestSessionHeader_DirtyIndicatorWhenDirty`, `TestSessionHeader_BannerPriority`, `TestSessionHeader_FindingsSummaryFormat` pass. |
| 6 | Menu bar render | `render_menu.go` implementing D4 menu bar with `File` label and dropdown overlay (D11) using `uichrome.SpliceAt`. D10 keyboard activation already in Update. D12 greyed Save when read-only. | 4, 5 | `TestMenuBar_FileLabelRendered`, `TestMenuBar_DropdownOpenOverlay`, `TestMenuBar_GreyedSaveWhenReadOnly` pass. |
| 7 | Outline pane render | Replace `outline.go:render()` content with the D18–D25 render: bordered pane, scroll indicator, step-name truncation (D49 — uses `lipgloss.Width` per [D-24](artifacts/implementation-decision-log.md#d-24-lipglosswidth-everywhere-for-width-measurement)), gripper, kind glyphs, focus prefix. | 4 | `TestOutline_StepNameTruncated` (D49), `TestOutline_ScrollIndicator`, `TestOutline_KindGlyphs`, `TestOutline_FocusPrefix` pass. |
| 8 | Detail pane render | Replace `detail.go:render()` content with the D26–D33, D47, D50, D51 render: bracket grammar, `▾` indicators, multi-line action row (D32), scroll indicator, label truncation (D50 — `lipgloss.Width`), dropdown flip (D51). Six per-kind field render functions + the `renderField` switch. | 4, 7 | `TestDetail_BracketGrammar`, `TestDetail_DropdownIndicator`, `TestDetail_LabelTruncation`, `TestDetail_DropdownOverflowFlip`, `TestDetail_MultiLineActionRow` pass. |
| 9 | Dialogs render | `render_dialogs.go` with `renderDialogShell` and `dialogBodyFor` per-kind body builders for all 15 dialog kinds. D36 chrome (border, title, footer); D37 dialog button row. | 4 | All 15 dialog render tests assert `╭` chrome, `[ ` bracket grammar, ANSI-stripped substring matches. `assertModalFits` helper validates height-fits-frame for each kind. ([D-3](artifacts/implementation-decision-log.md#d-3-dialog-rendering--renderdialogshell--dialogbodyfor-per-kind-body-builders)) |
| 10 | Help modal + findings panel | `render_help.go` (D40) and `render_findings.go` (D38, D39 acknowledged-finding `[WARN ✓]`). Findings panel dim under help-modal coexistence (Color("8")). | 4, 9 | `TestHelpModal_RenderShape`, `TestFindingsPanel_AcknowledgedGlyph`, `TestFindingsPanel_DimUnderHelp` pass. |
| 11 | Empty-editor view | `render_empty.go` replaces `renderEmptyEditor()` with the D43 split outline + bordered detail-pane hint layout. `HintEmpty` constant removed ([D-26](artifacts/implementation-decision-log.md#d-26-hintempty-constant-superseded-by-d43-outline-detail-layout)). | 4 | `TestEmpty_OutlinePaneAndHintPanel` passes. |
| 12 | `Validating…` footer + browse-only render | `ShortcutLine()` guards `m.validateInProgress` and `m.saveInProgress` with appropriate transient text ([D-17](artifacts/implementation-decision-log.md#d-17-validating-footer-guard-in-shortcutline)). Browse-only signals (greyed Save shortcut, suppressed dirty, `[ro]` banner emit) threaded through render ([D-18](artifacts/implementation-decision-log.md#d-18-browse-only-signals-threaded-through-render)). Session-event log entries added per D-23. | 4, 5, 6 | `TestFooter_ValidatingTransient`, `TestFooter_SavingTransient`, `TestRender_BrowseOnlyAllSignals`, session-event log assertions all pass. |
| 13 | Documentation | Update `docs/features/workflow-builder.md` (Visual Layout section); add Visual Layout section to `docs/code-packages/workflowedit.md`; update `docs/how-to/using-the-workflow-builder.md` (menu-bar states, banner priority); add pointer to `CLAUDE.md` ([D-21](artifacts/implementation-decision-log.md#d-21-documentation-ships-in-same-pr)). | 1–12 | Manual review; doc code blocks consistent with shipped code per `coding-standards/documentation.md`. |

## RAID Log

### Risks

| ID | Risk | Likelihood | Severity | Blast Radius | Reversibility | Owner | Mitigation |
|---|---|---|---|---|---|---|---|
| R1 | `panelH = m.height - 2` causes pane content to overwrite footer and bottom border at every terminal height | 5 | 4 | 5 | 2 | implementer | Commit 3: change to `m.height - ChromeRows` atomically with chrome-frame landing in commit 4 ([D-20](artifacts/implementation-decision-log.md#d-20-chromerows--8-chrome-budget)) |
| R2 | No D48 minimum-size guard; pane arithmetic produces undefined layout below 60×16 | 4 | 4 | 4 | 2 | implementer | Commit 3: D48 guard as first statement in `View()` ([D-19](artifacts/implementation-decision-log.md#d-19-60×16-minimum-size-fallback-render)) |
| R3 | `saveBanner` permanent + rendered outside D9 chrome budget | 5 | 4 | 3 | 2 | implementer | Commit 4: `clearSaveBannerMsg{gen}` + `tea.Tick(3s)` + move banner into session-header slot ([D-7](artifacts/implementation-decision-log.md#d-7-savebanner-clear-pattern--teatick3s-clearsavebannermsggen-with-bannergen-counter)) |
| R4 | `doMoveStepUp/Down` no phase-boundary check — cross-phase swap corrupts workflow document | 4 | 5 | 2 | 3 | implementer | Commit 3: phase-equality guard before swap ([D-12](artifacts/implementation-decision-log.md#d-12-phase-boundary-guard-in-domovestepupdown-with-boundaryflash-field--teatick-cleared-signal)) |
| R5 | `revealedField` not reset when dialog opens — secret leak | 3 | 4 | 1 | 2 | implementer | Commit 3: reset helper called from every dialog-open path ([D-13](artifacts/implementation-decision-log.md#d-13-revealedfield-reset-on-every-focus-leave-including-dialog-open-paths)) |
| R6 | `View()` flat `\n` row assembly incompatible with D2 fixed 9-row chrome frame | 5 | 4 | 5 | 3 | implementer | Commit 4: rebuild `View()` as ordered row assembly ([D-1](artifacts/implementation-decision-log.md#d-1-render-code-stays-in-internalworkflowedit-package-decomposed-into-per-surface-render-files), [D-5](artifacts/implementation-decision-log.md#d-5-render-time-geometry-computation--d48d51d17d49-in-view-not-update)) |
| R7 | `renderDialog()` flat strings — D36 bordered overlay unimplemented | 5 | 3 | 4 | 3 | implementer | Commit 9: per-kind body builder + `renderDialogShell` ([D-3](artifacts/implementation-decision-log.md#d-3-dialog-rendering--renderdialogshell--dialogbodyfor-per-kind-body-builders)) |
| R8 | `menu.go render()` flat string — D4/D11 bordered dropdown unimplemented | 5 | 3 | 2 | 3 | implementer | Commit 6: overlay splice pattern from `uichrome.SpliceAt` |
| R9 | `funlen`/`gocyclo` lint failure on long render functions | 3 | 3 | 2 | 2 | implementer | Per-kind body builder decomposition (sufficient for default `gocyclo` threshold 30; `funlen`/`mnd` not in defaults per OQ-2 resolution); no lint suppressions per `coding-standards/lint-and-tooling.md` |
| R10 | Reload paths in Recovery dialog lose banner signals | 3 | 3 | 2 | 2 | implementer | Commit 3: populate banner fields in inline literal ([D-15](artifacts/implementation-decision-log.md#d-15-makeloadcmd-reload-path-forwards-all-banner-signals)) |

### Assumptions

| ID | Assumption | What changes if wrong | Verifier | Status |
|---|---|---|---|---|
| A1 | `internal/ui` chrome primitives can be extracted to `internal/uichrome` without breaking `internal/ui` callers | If callers break, the extraction is a larger refactor | `structural-analyst` review of commit 2 PR | Unverified — verify before committing extraction |
| A2 | `tea.Tick(3*time.Second, ...)` is acceptable as the first Model-owned timer in `workflowedit` | If the team prefers lazy-timestamp pattern, the implementation changes; both are correct | Settled by [D-7](artifacts/implementation-decision-log.md#d-7-savebanner-clear-pattern--teatick3s-clearsavebannermsggen-with-bannergen-counter) | Settled |
| A3 | `nowFn func() time.Time` field on Model is the right time-injection seam | If test-engineer prefers message-carried timestamp, seam changes | Settled by [D-8](artifacts/implementation-decision-log.md#d-8-time-injection--nowfn-functime-time-model-field) | Settled |
| A4 | `funlen`/`gocyclo` thresholds in golangci-lint defaults will not fire on decomposed render functions | If thresholds are tighter than expected, further decomposition is required | OQ-2 resolution: no project-level `.golangci.yml`; defaults include `gocyclo` at 30 (sufficient for per-kind decomposition); `funlen`/`mnd` not in defaults | Verified |
| A5 | `D22`'s clarification (no reverse-video on mockup variant C) governs over the mockup annotation | The mockup is erroneous; decision-log governs per visual spec's authority hierarchy | Settled by [D-25](artifacts/implementation-decision-log.md#d-25-mockup-05-variant-c-reverse-video-annotation-overruled-by-visual-spec-d22--d28) | Confirmed |
| A6 | `makeLoadCmd()` and the recovery-handler inline literal are the only two reload paths | If another load path exists without banner propagation, R10 scope grows | Code search by implementer in commit 3 | Confirmed for known paths |
| A7 | `fieldKindMultiLine` does not exist yet | If it was added in a later commit, no new constant is needed | OQ-1 resolution: code search confirmed absent | Verified |

### Issues

| ID | Issue | Owner | Next Step |
|---|---|---|---|
| I1 | `panelH = m.height - 2` actively wrong in shipped code | implementer | Fix in commit 3 |
| I2 | Render tests assert against placeholder strings; pass falsely until updated | implementer | Convert to ANSI-stripped substring assertions in commit 1 |
| I3 | `makeLoadCmd()` reload path does not forward banner signals | implementer | Populate banner fields in inline `openFileResultMsg` return at `model.go:1704-1712` in commit 3 |

### Dependencies

| ID | Dependency | Owner | Status |
|---|---|---|---|
| DEP-1 | Commit 2 (`uichrome` extraction) must precede commits 4, 6, 9, 10 (overlay-using render commits) | implementer | Open |
| DEP-2 | Commit 3 (chrome budget fix) must precede commit 4 (D48 guard test asserts against correct budget) | implementer | Open |
| DEP-3 | Commit 4 (View chrome frame) must precede commits 6, 9 (overlay splice needs assembled base frame) | implementer | Open |
| DEP-4 | Commit 4 (saveBanner timer) co-designed with View frame in same commit | implementer | Open |
| DEP-5 | Commit 3 (phase guard) precedes any phase-flash decoration (delivered in commit 3 alongside guard) | implementer | Open |
| DEP-6 | Commit 3 (revealedField reset) precedes commit 9 (dialog rendering) | implementer | Open |

## Testing Strategy

**Strategy** ([D-6](artifacts/implementation-decision-log.md#d-6-test-strategy--ansi-stripped-substring--structural-assertions)): ANSI-stripped substring + structural assertions. Every render test wraps `m.View()` with `ansi.StripAll(...)` before substring checks. No mockup golden files (mockups use annotation syntax, not real ANSI).

**Test infrastructure additions** (commit 1):
- `nowFn func() time.Time` Model field — time-injection seam ([D-8](artifacts/implementation-decision-log.md#d-8-time-injection--nowfn-functime-time-model-field))
- `clearSaveBannerMsg{gen int}` message type — synchronous transient-banner testing without `time.Sleep`
- `bannerGen int` Model field — generation-counter stale-event testing ([D-7](artifacts/implementation-decision-log.md#d-7-savebanner-clear-pattern--teatick3s-clearsavebannermsggen-with-bannergen-counter))
- `assertModalFits(t, m)` helper — dialog/modal height-fits-frame validation

**Modified files** (8):
- `dialogs_render_test.go` — modify all 15 test functions: add `╭` chrome assertion, `[ ` bracket assertion, ANSI strip wrapper.
- `menu_bar_render_test.go` — modify 2, add 2 (greyed Save D12 + browse-only).
- `outline_render_test.go` — modify 5, add 4 (truncation, scroll indicator, collapse, empty-section).
- `detail_pane_render_test.go` — modify 3, add 5 (dropdown indicator, label truncation, scroll indicator, dropdown flip, multi-line action row).
- `findings_panel_test.go` — keep 5, add 3 render tests (prefix tags, ack glyph).
- `footer_test.go` — keep 5, add 2 (version-label, prompt-mode/Validating-transient).
- `viewport_test.go` — extend with D9 chrome-budget test, D48 fallback test, D48 full-render-at-minimum.
- `banner_session_test.go` — modify 3, add 8 (D14 short-form tags, D13 dirty indicator, D16 findings summary, transient-banner gen-counter race tests).

**New files** (2):
- `chrome_render_test.go` — D1 chrome glyphs, D2 row order, D3 title format, D6 junctions, D40 help modal, D41 reorder visuals, D46 BMP-only.
- `findings_render_test.go` — D38 panel-in-detail-pane, D39 acknowledged finding `[WARN ✓]`, findings-summary integration.

**Coverage matrix.** A complete D# → test-name mapping for all 51 visual-spec decisions ships in commit 1 as a comment header in `chrome_render_test.go`, expanded incrementally as each commit lands. The `test-engineer` is named in Specialist Handoffs to produce the final mapping.

**Race detector** (`coding-standards/testing.md`): `go test -race ./...` is the default test invocation; new generation-counter tests for `bannerGen` and `boundaryFlash` exercise rapid-fire patterns that the detector will flag if synchronization is wrong.

## Security Posture

The visual layer adds no new external-input surfaces. Existing surfaces (config.json validation, CLI flag parsing, file path normalization, env-var passthrough) are unchanged.

**Secret-mask leak fix** ([D-13](artifacts/implementation-decision-log.md#d-13-revealedfield-reset-on-every-focus-leave-including-dialog-open-paths)): Lands in commit 3 before any dialog rendering work (commit 9) per DEP-6 ordering hazard. The current code resets `m.detail.revealedField` only on Tab/focus-leave inside the field state machine; opening a dialog does not constitute focus-leave, so a revealed secret value remains visually present in the detail pane behind the dialog overlay. Commit 3 introduces a `resetSecretMask(m *Model)` helper called from each of the 15 dialog-open Update paths. Test assertion: open each dialog from a state with `revealedField != -1` and confirm `revealedField == -1` after the dialog opens.

**Save durability** preserved: visual changes do not touch `internal/workflowio` or `internal/atomicwrite`; ADR `20260424120000-workflow-builder-save-durability.md` (atomic temp+rename, companion-first ordering) remains the canonical save discipline.

**Session-event log** ([D-23](artifacts/implementation-decision-log.md#d-23-session-event-logging-per-state-transition)): adds `secret_revealed` and `secret_remasked` events so post-hoc audit can detect anomalous reveal patterns.

## Operational Readiness

- **Observability:** Session-event log entries for `resize`, `dialog_open`, `dialog_close`, `save_banner_set`, `save_banner_cleared`, `terminal_too_small`, `focus_changed`, `validate_started`, `validate_complete`, `phase_boundary_decline`, `secret_revealed`, `secret_remasked` ([D-23](artifacts/implementation-decision-log.md#d-23-session-event-logging-per-state-transition)). Match the existing vocabulary at `model.go:194-215`. Log per state-transition, not per render. No new dashboards.
- **SLO impact:** None. Render performance fine without caching; run-mode TUI doesn't cache and has no perf issues per devops-engineer #11.
- **Feature flag:** None. The visual layer is the canonical render; no toggle exists for falling back to placeholder strings. The `--workflow-dir`, `--project-dir`, `-n`, `--version` CLI surfaces are unchanged.
- **Rollout:** Single PR with 13 incremental commits. Each commit independently revertible. The visual layer is all in render functions; no behavioral state changes except for the targeted fixes (panelH, phase guard, revealedField reset, banner tag text, makeLoadCmd forwarding, fieldKindMultiLine + Ctrl+E wiring, tier-0 WindowSizeMsg routing).
- **Rollback:** `git revert` on a render commit restores the placeholder. Behavioral-fix commits in commit 3 are independently revertible if a regression appears (revert order: visual commits first, behavioral commit 3 last).
- **Cost and scale:** No infrastructure changes. No third-party dependencies added. No file-format changes. No env-var changes.
- **Version policy** ([D-22](artifacts/implementation-decision-log.md#d-22-no-version-bump-for-visual-layer)): No version bump required per `docs/coding-standards/versioning.md`. Visual chrome is not part of the public API surface (CLI flags, config.json schema, `{{VAR}}` language, `--version` output). If the team later wants a tagged release, a separate `0.7.4` patch commit is the correct vehicle.
- **Documentation** ([D-21](artifacts/implementation-decision-log.md#d-21-documentation-ships-in-same-pr)): Per `coding-standards/documentation.md`, ships in same PR. Files: `docs/features/workflow-builder.md` (Visual Layout section); `docs/code-packages/workflowedit.md` (Visual Layout section); `docs/how-to/using-the-workflow-builder.md` (menu-bar states, banner priority); `CLAUDE.md` (pointer to visual spec).
- **Pre-existing race-detector test contract** preserved (`go test -race ./...`).
- **`make ci`** green at every commit (`coding-standards/lint-and-tooling.md` — no suppressions).

## Definition of Done

- [ ] `go test -race ./...` passes.
- [ ] `make lint` passes with zero suppressions.
- [ ] `TestView_MinimumSizeRendersTooSmallMessage` (D48) passes ([D-19](artifacts/implementation-decision-log.md#d-19-60×16-minimum-size-fallback-render)).
- [ ] `TestView_FrameHas9ChromeRows` (D9) passes ([D-20](artifacts/implementation-decision-log.md#d-20-chromerows--8-chrome-budget); panelH = `m.height - ChromeRows` where `ChromeRows = 8` and the assembled frame uses 9 rows including the pane area separator).
- [ ] `TestView_SessionHeaderContainsDirtyIndicatorWhenDirty` (D13) passes — uses `m.IsDirty()` not `m.dirty` ([D-11](artifacts/implementation-decision-log.md#d-11-dirty-render-source--misdirty-not-mdirty)).
- [ ] `TestView_SaveBannerClearedAfter3Seconds` (D14) passes without `time.Sleep` (uses `nowFn` override) ([D-7](artifacts/implementation-decision-log.md#d-7-savebanner-clear-pattern--teatick3s-clearsavebannermsggen-with-bannergen-counter), [D-8](artifacts/implementation-decision-log.md#d-8-time-injection--nowfn-functime-time-model-field)).
- [ ] All dialog render tests contain `╭` chrome assertion (D36) per test-engineer file triage ([D-3](artifacts/implementation-decision-log.md#d-3-dialog-rendering--renderdialogshell--dialogbodyfor-per-kind-body-builders), [D-6](artifacts/implementation-decision-log.md#d-6-test-strategy--ansi-stripped-substring--structural-assertions)).
- [ ] `docs/features/workflow-builder.md`, `docs/code-packages/workflowedit.md`, `docs/how-to/using-the-workflow-builder.md` updated, and `CLAUDE.md` carries a pointer to the visual spec ([D-21](artifacts/implementation-decision-log.md#d-21-documentation-ships-in-same-pr)).
- [ ] Every D1–D51 visual decision has at least one passing render test (coverage matrix shipped in `chrome_render_test.go` header per [D-6](artifacts/implementation-decision-log.md#d-6-test-strategy--ansi-stripped-substring--structural-assertions)).
- [ ] `adversarial-validator` post-synthesis gate run before merge.
- [ ] Post-ship owner named (River, branch owner of `workflow-builder-tui`).

## Specialist Handoffs for Implementation

- **`adversarial-validator`** — dispatch immediately on completion of this synthesis, before any commit lands. Needs: this plan, [artifacts/implementation-decision-log.md](artifacts/implementation-decision-log.md), [artifacts/implementation-iteration-history.md](artifacts/implementation-iteration-history.md), [artifacts/facilitation-summary.md](artifacts/facilitation-summary.md), the visual spec, and the inherited behavioral plan. Validates that (a) each D# in the visual spec maps to a commit in the 13-commit graph; (b) each P0 risk has a mitigation row in the RAID log; (c) the test strategy covers each D# at least once; (d) the documentation plan satisfies `coding-standards/documentation.md`.
- **`concurrency-analyst`** — dispatch when commit 4 (saveBanner timer) and commit 3 (phase-boundary flash) are drafted, prior to merge. Needs: the diff for `tea.Tick`-driven message handlers and the generation-counter increment/compare logic for `bannerGen` and `boundaryFlash`. Validates: race-free under Bubble Tea single-goroutine Model contract; stale-tick-ignored discipline; no goroutines bypassing the message bus.
- **`test-engineer`** — dispatch when commit 1 (test seam) is drafted. Needs: the visual spec D1–D51 list, the commit graph in this plan, the file triage in the Testing Strategy section. Produces: the complete D# → test-name coverage matrix annotated with which commit each test ships in. The matrix lives as a comment header in `chrome_render_test.go` and is expanded incrementally per commit.

## Open Items

- **OI-1 (OQ-3): Bubble Tea ExecProcess resize** — The Bubble Tea v1.3.10 (project's version per `go.mod`) does not document whether `tea.WindowSizeMsg` is redelivered after `tea.ExecProcess` returns. Visual-spec D44 commits to "if the terminal was resized during the editor session, the resize is treated as a `tea.WindowSizeMsg` on the first post-return render cycle." If Bubble Tea does not redeliver, this commitment is false in the current implementation.
  - **Resolves when:** the implementer either (a) confirms by experiment that resize-during-editor is redelivered, (b) adds an explicit `tea.WindowSize()` re-query on post-ExecProcess return, or (c) documents the gap as a known limitation in `docs/features/workflow-builder.md` with a follow-on issue.
  - **Blocks implementation:** No — the default disposition is to document as a known limitation and ship; the gap does not break any commit's `make ci` verification.

- **OI-2 (OQ-4): PR granularity** — Single PR vs split. Default per [D-9](artifacts/implementation-decision-log.md#d-9-commit-graph--13-incremental-commits-make-ci-green-at-each) is a single PR with 13 incremental commits. The user may prefer two PRs (behavioral fixes in commit 3 separately, visual layer separately).
  - **Resolves when:** the user signals a preference, or the implementer decides during PR-prep based on review-load considerations.
  - **Blocks implementation:** No — the commit graph works either way; single-PR is the default disposition.

- **OI-3 (visual-spec OI-1):** Inherited from the visual spec; deferred per spec authority.
  - **Blocks implementation:** No.

- **OI-4 (visual-spec OI-2):** Inherited from the visual spec; deferred per spec authority.
  - **Blocks implementation:** No.

- **Deferred P2/P3 risks** (from facilitation summary): R19 (sub-30-col mid-resize dialog overflow), R20 (some CJK width regressions beyond what `lipgloss.Width` covers), R21 (mouse-routing column off-by-1), R22 (tab/null chars in keys), R23 (path-picker tab-completion edge cases), R24 (findings panel dim variants), R25 (rapid-save tick race subset not covered by gen-counter), R27 (D17 overflow priority secondary cases), R29 (S1 model.go growth — discipline ongoing), R30 (D41 phase-boundary flash advanced animation — basic flash ships in commit 3), R31 (HintEmpty split — covered by [D-26](artifacts/implementation-decision-log.md#d-26-hintempty-constant-superseded-by-d43-outline-detail-layout)), R32 (WG drain — not in current code), R33 (Validating… footer — covered by [D-17](artifacts/implementation-decision-log.md#d-17-validating-footer-guard-in-shortcutline) in this PR).
  - Note: D49 step-name truncation and D51 dropdown overflow flip are NOT deferred — they ship in commits 7 and 8 respectively as part of the committed visual mechanics.

## Summary

- **Outcome delivered:** Full bordered, multi-pane visual rendering for the `pr9k workflow` builder TUI satisfying all 51 visual-spec decisions, with `make ci` green and documentation shipping in the same PR.
- **Team size:** 10 specialists + project-manager + adversarial-validator queued for post-synthesis gate — see [artifacts/implementation-iteration-history.md](artifacts/implementation-iteration-history.md).
- **Rounds of facilitation:** 1 (R-1; spec-maturity gate did not trip) — see [artifacts/implementation-iteration-history.md](artifacts/implementation-iteration-history.md).
- **Decisions committed:** 26 (D-1..D-26 in this plan; the 51 D# visual-spec decisions are inherited as committed mechanics) — see [artifacts/implementation-decision-log.md](artifacts/implementation-decision-log.md).
- **Decisions settled by evidence:** 22 — see [artifacts/implementation-decision-log.md](artifacts/implementation-decision-log.md).
- **Decisions settled by junior-developer reframing:** 0 (junior-developer questions were resolved by evidence or escalated to PM auto-decisions).
- **Decisions settled by user input:** 0 (PM auto-decisions OD-1..OD-4 settled the four candidate user-input decisions under auto-mode authority).
- **Decisions settled by PM auto-decision:** 4 (became D-2, D-7, D-8, D-9) — see [artifacts/implementation-decision-log.md](artifacts/implementation-decision-log.md).
- **Rejected alternatives recorded:** 50+ across the 26 decisions — see [artifacts/implementation-decision-log.md](artifacts/implementation-decision-log.md).
- **Open items remaining:** 4 (OI-1 ExecProcess resize, OI-2 PR granularity, OI-3 / OI-4 inherited from visual spec) — none blocking.
- **Recommendation:** **Ship as planned.** Execute the 13-commit graph; dispatch `adversarial-validator` on this plan before commit 1 lands; dispatch `test-engineer` for the coverage matrix during commit 1; dispatch `concurrency-analyst` for race review during commits 3 and 4.
