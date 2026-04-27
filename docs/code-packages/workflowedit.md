# internal/workflowedit

Bubble Tea Model for the `pr9k workflow` interactive workflow-builder TUI. Implements the full edit-view layout (menu bar, session header, outline, detail pane, shortcut footer), all dialog flows (File > New/Open/Save/Quit, findings panel, error dialogs, crash-temp notice, recovery view), session-event logging, and the load/save pipeline.

## Exported API

### Model

The top-level Bubble Tea model. Satisfies `tea.Model` (Init/Update/View) and exposes two additional methods.

```go
type Model struct { /* unexported fields */ }

func New(saveFS workflowio.SaveFS, editor EditorRunner, projectDir, workflowDir string) Model
func (m Model) Init() tea.Cmd
func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd)
func (m Model) View() string
func (m Model) WithLog(w io.Writer) Model
func (m Model) WithNoValidation() Model
func (m Model) IsDirty() bool
func (m Model) ShortcutLine() string

func LoadResultMsg(doc, diskDoc workflowmodel.WorkflowDoc, companions map[string][]byte, workflowDir string) tea.Msg
```

**New** constructs the Model with dependency injection:
- `saveFS` — filesystem abstraction used by the save pipeline (production: `workflowio.OSFS{}`)
- `editor` — injectable `EditorRunner` interface for external-editor invocation (production: `RealEditorRunner{}`)
- `projectDir` — governs log file location and default path-picker pre-fill
- `workflowDir` — governs `--workflow-dir` auto-open and default path-picker pre-fill

**WithLog** returns a copy of the Model with the session-event `io.Writer` set. The writer receives structured log lines for security-relevant events (symlink detected, external workflow, shared install, editor invoked/exited). `containerEnv` secret values are never written to the log. In production, `runWorkflowBuilder` passes `log.Writer()` here so session events land in the same `.pr9k/logs/` file as the main run log.

**WithNoValidation** returns a copy of the Model that stubs out the validator: validation always returns zero findings, so the save pipeline never opens the findings panel or the acknowledgment dialog. Used by integration tests that need to drive the save pipeline without triggering real disk reads.

**IsDirty** reports whether the in-memory `WorkflowDoc` differs from the on-disk baseline (`diskDoc`, last set at load time or after a successful save). Delegates to `workflowmodel.IsDirty`.

**LoadResultMsg** constructs the `tea.Msg` that delivers a freshly loaded document into the Model's Update loop. The `diskDoc` argument sets the save-baseline for dirty detection; passing an empty `workflowmodel.WorkflowDoc{}` forces `IsDirty()` to return `true` immediately (used in integration tests to guarantee a write on the first save). When `workflowDir` is non-empty it overrides the Model's configured workflow directory.

**ShortcutLine** returns the footer hint string for the current focus state and overlay. Called by the parent `program` runner to update the status bar.

### EditorRunner

Interface for injecting the external-editor runner. Allows the editor invocation to be tested without a real TTY.

```go
type EditorRunner interface {
    Run(filePath string, cb ExecCallback) tea.Cmd
}

type ExecCallback func(err error) tea.Msg
```

`Run` returns a `tea.Cmd` that calls `tea.ExecProcess` with the resolved editor binary and the given file path. The `ExecCallback` is invoked when the editor process exits; it receives the exit error (nil on success, non-nil on non-zero exit or spawn failure) and must return a `tea.Msg` to be dispatched into the Update loop.

### DialogKind

Identifies the currently-active modal dialog. Only one dialog is active at a time; the Model holds `dialog DialogKind` internally.

```go
type DialogKind int

const (
    DialogNone                   DialogKind = iota
    DialogPathPicker                        // File > Open or File > New path input
    DialogNewChoice                         // empty scaffold vs copy-from-default vs cancel
    DialogUnsavedChanges                    // Save / Cancel / Discard interception
    DialogQuitConfirm                       // Quit the workflow builder? Yes / No
    DialogExternalEditorOpening            // "Opening editor…" interstitial
    DialogFindingsPanel                     // validation findings with jump-to-field
    DialogError                             // generic error (D56 template)
    DialogCrashTempNotice                  // crash-era temp file detected
    DialogFirstSaveConfirm                 // external-workflow first-save confirmation
    DialogRemoveConfirm                    // step removal confirmation
    DialogFileConflict                     // on-disk file changed since load
    DialogSaveInProgress                   // async save in flight
    DialogRecovery                         // parse-error recovery view
    DialogAcknowledgeFindings              // warn/info findings acknowledgment
    DialogCopyBrokenRef                    // broken default-bundle reference dialog
)
```

### PickerKind

Discriminates the intent of `DialogPathPicker`.

```go
type PickerKind int

const (
    PickerKindOpen PickerKind = iota // browse to an existing config.json
    PickerKindNew                    // choose where to create a new workflow
)
```

The picker pre-fills differently and labels its confirm button differently (`Open` vs `Create`) depending on the kind.

### Constants

```go
const (
    GlyphGripper          = "⋮⋮"       // drag-handle shown next to steps in reorder mode
    GlyphSectionOpen      = "▾"        // section-collapse chevron, expanded state
    GlyphSectionClose     = "▸"        // section-collapse chevron, collapsed state
    GlyphChevronExpanded  = GlyphSectionOpen
    GlyphChevronCollapsed = GlyphSectionClose
    GlyphAddItem          = "+"        // prefixes "+ Add step" affordance rows
    GlyphMasked           = "••••••••" // replaces visible value of sensitive containerEnv entries
    HintNoName            = "(unnamed)"
    ChromeRows            = 8          // fixed rows consumed by chrome; panelH = height - ChromeRows
    GlyphKindClaude       = "[≡]"      // step-kind glyph for claude steps
    GlyphKindShell        = "[$]"      // step-kind glyph for shell steps
    GlyphKindUnset        = "[?]"      // step-kind glyph when kind is not set
    GlyphScrollUp         = "▲"        // scroll-indicator top
    GlyphScrollDown       = "▼"        // scroll-indicator bottom
    GlyphScrollThumb      = "█"        // scroll-indicator thumb
)
```

`ChromeRows` is shared with `render_frame.go` and is the only source of truth for the chrome budget; changing it automatically adjusts the content panel height.

## Internal Architecture

### Update routing

`Model.Update` dispatches in this order:

1. **Pre-dispatch tier** — messages that must be handled regardless of dialog state: `openFileResultMsg`, `tea.WindowSizeMsg`, `quitMsg`, `validateCompleteMsg` (when save-in-progress), `saveCompleteMsg`.
2. **Help overlay** — if `helpOpen`, `?` toggles it closed; `Esc` closes it; all other keys route to the normal flow.
3. **Dialog tier** — if `dialog != DialogNone`, the active dialog handler consumes the message. Dialog handlers may clear the dialog, chain to another dialog, or dispatch async commands.
4. **Global keys** — `Ctrl+N`, `Ctrl+O`, `Ctrl+S`, `Ctrl+Q`, `F10`, `Alt+F`, `?` are intercepted regardless of which widget has focus.
5. **Edit-view tier** — routed by `focus` (focusOutline, focusDetail, focusMenu).

### Focus targets

```
focusOutline  — outline panel has keyboard focus
focusDetail   — detail pane has keyboard focus
focusMenu     — File menu dropdown is open
```

Tab from a step row in the outline moves focus to the detail pane. Tab from the detail pane cycles to the next field within the pane; when the last field is exited, focus returns to the outline.

### Outline structure

`buildOutlineRows()` produces a flat slice of `outlineRow` values from the current `WorkflowDoc`. Row kinds:

| Row kind | Description |
|----------|-------------|
| `rowKindSectionHeader` | Collapsible section header with item count |
| `rowKindStep` | A step within initialize/iteration/finalize |
| `rowKindEnvItem` | A top-level env passthrough entry |
| `rowKindContainerEnvItem` | A containerEnv key=value entry |
| `rowKindAddRow` | `+ Add <item>` affordance at the end of each section |

Top-level sections (env, containerEnv, statusLine) are shown only when non-empty. Phase sections (initialize, iteration, finalize) are always shown. Each section starts expanded by default; collapse state is stored per `sectionKey` in a map on the Model.

Collapsing a section while the cursor is inside it moves the cursor to the section header.

### Detail pane field kinds

| Field kind | Type | Notes |
|------------|------|-------|
| `fieldKindText` | Single-line plain text | Newlines stripped via `sanitizePlainText`; ANSI escaped via `ansi.StripAll` |
| `fieldKindChoice` | Enum from a fixed set | `▾` suffix when unfocused; `Enter`/`Space` opens dropdown |
| `fieldKindNumeric` | Integer with bounds | Non-numeric input silently dropped; paste sanitized at first non-digit with message |
| `fieldKindModelSuggest` | Free-text + suggestion list | Suggestions are a hardcoded snapshot; any value accepted |
| `fieldKindSecretMask` | Plain text, masked by default | Key matched by `isSensitiveKey` pattern; `r` toggles reveal; re-masks on focus leave |

Secret detection pattern (`isSensitiveKey`): key must end with one of `_TOKEN`, `_SECRET`, `_KEY`, `_PASSWORD`, `_PASSPHRASE`, `_CREDENTIAL`, `_APIKEY` (case-insensitive suffix match).

#### commitDetailEdit rejection rules

`commitDetailEdit` returns `(Model, bool)`. On rejection it returns `false` with an `editMsg` visible in the detail pane; the Model stays in editing mode:

| Field | Rejection condition | Message |
|-------|---------------------|---------|
| `Command` | Any original argv element (before editing) contains whitespace | `"Command has quoted args — edit in external editor (Ctrl+E)"` |
| `containerEnv` value | Committed value does not contain `=` | `"Expected key=value format"` |

The Command rejection prevents a silent round-trip corruption: `strings.Join(argv, " ")` followed by `strings.Fields(joined)` is lossy when any element contains whitespace (e.g. `["bash", "-c", "echo hello"]` → `["bash", "-c", "echo", "hello"]`). Affected Command values must be edited via external editor (`Ctrl+E`).

#### deepCopyDoc and the validator-goroutine data race

Before starting the async validator goroutine, the Model takes a deep copy of the document via `deepCopyDoc`. The copy performs explicit `make`+`copy` for `doc.Env` (slice) and `make`+range-assign for `doc.ContainerEnv` (map), preventing the goroutine from aliasing the UI goroutine's backing arrays. Without this, a concurrent `append` or map write from the UI thread could corrupt the slice or map mid-iteration in the validator.

### Save pipeline

1. `handleSaveRequest()` runs the validator via `validateFn` (injectable for testing).
2. If fatal findings exist, `DialogFindingsPanel` opens.
3. If only warn/info findings exist and not yet acknowledged this session, `DialogAcknowledgeFindings` opens.
4. If no findings (or all acknowledged), `makeSaveCmd()` is called.
5. `makeSaveCmd()` writes the ackSet to disk first, then dispatches an async save via `workflowio.Save`.
6. The async save completes as `saveCompleteMsg`; `handleSaveResult` processes success/failure and updates banners, the dirty flag, and the mtime snapshot.

### Copy-from-default (deferred)

`DialogNewChoice` option `c` (Copy from default) and `DialogCopyBrokenRef` option `y` (Copy anyway) are both deferred to PR-3. In the current build these options open `DialogError` with the message `"Copy from default not yet implemented — use Empty or open an existing workflow"` so the action does not silently no-op.

### Conflict detection

The Model snapshots the `config.json` mtime and size at load time. At save time, `makeSaveCmd()` checks the current on-disk mtime+size against the snapshot. If they differ, `DialogFileConflict` appears with three options: overwrite (set `forceSave`), reload (`makeLoadCmd`), or cancel.

### Session-event logging

`logEvent(format string, args ...any)` writes a structured line to `m.logW` (set via `WithLog`). Format helpers:

- `fmtSymlinkDetected(paths []string) string`
- `fmtExternalWorkflowDetected(path string) string`
- `fmtReadOnlyDetected(path string) string`
- `fmtEditorInvoked(binary string) string`
- `fmtEditorExited(binary string, err error) string`

`containerEnv` values are never passed to any log formatter.

### Load pipeline banner forwarding

`openFileResultMsg` carries load-time detection results from `workflowio`:

```go
type openFileResultMsg struct {
    doc         workflowmodel.WorkflowDoc
    diskDoc     workflowmodel.WorkflowDoc
    companions  map[string][]byte
    workflowDir string
    err         error
    rawBytes    []byte       // non-nil only for parse errors (routes to DialogRecovery)
    isSymlink       bool
    symlinkTarget   string
    isExternal      bool
    isReadOnly      bool
    isSharedInstall bool
}
```

`handleOpenFileResult` sets the `banners` struct from these fields and calls `logEvent` for each detected condition. `banners.activeBanner()` implements the priority order: read-only > external > symlink > shared-install > unknown-field.

### Bubble Tea ctx exemption (D-PR2-13 / F-PR2-55)

The Model does not propagate a `context.Context` into blocking syscalls (filesystem reads/writes, editor spawn). This is intentional: Go's `context.Context` cancels goroutines via cooperative checking, but it does **not** cancel blocking syscalls such as `os.File.Write` or `exec.Cmd.Wait`. The actual cap on blocking duration is:

- **Save operations** — bounded by disk I/O; the OS will complete or fail the write without an application-level timeout
- **Editor invocation** — unbounded by design; the user controls when the editor exits; the builder waits indefinitely
- **Load operations** — bounded by `workflowio.Load`'s file read; no network I/O involved

A `context.Context` parameter would give false safety signals without actually cancelling the underlying operations. If cancellation of a save or load is needed in the future, the correct approach is to run the operation in a goroutine and check a channel, not to propagate ctx.

## Visual Layout

### Per-surface render files

View() is decomposed into one file per visual surface. Each file is responsible for rendering exactly one region of the TUI:

| File | Surface | Key decision |
|------|---------|--------------|
| `render_frame.go` | Top-level `View()` — 9-row chrome assembly, overlay splice, D48 minimum-size guard | Calls all other render functions; applies `uichrome.Overlay` for dialogs, help, and dropdown |
| `render_session_header.go` | Session-header row: path, dirty indicator, banner, `[N more]`, findings+validation right-aligned | D5 5-slot layout; D17 overflow priority (drop [N more] → drop right → drop banner → truncate path) |
| `render_outline.go` | Bordered outline pane with scrolling, sections, step rows, collapse chevrons | D18–D25 section ordering; D49 step-name truncation; scroll indicators |
| `render_detail.go` | Bordered detail pane with field rows, bracket grammar, dropdown flip | D26–D33 field-row anatomy; D47 scroll; D50 label truncation; D51 dropdown flip |
| `render_menu.go` | Menu bar (closed/open states) and D11 dropdown overlay | D4 File label with `Alt` mnemonic accent; D12 greyed-out Save |
| `render_dialogs.go` | All 15 non-None `DialogKind` bodies via `dialogBodyFor` dispatcher and `renderDialogShell` | D36 bordered overlay; D37 button-row `[ Label ]` bracket grammar |
| `render_findings.go` | Findings panel with finding entries and D39 acknowledged-finding `[WARN ✓]` glyph | D38 findings panel; dim-under-help coexistence |
| `render_help.go` | Help modal overlay centered in the frame | D40 help modal; `uichrome.Overlay` positions it |
| `render_empty.go` | Empty-editor state: `(no workflow open)` outline and bordered detail-pane hint | D43 split pane layout |
| `render_footer.go` | `ShortcutLine()` with D34 two-tone palette, D17 transient guards, and D18 browse-only dim hint | Delegates to focused widget's `ShortcutLine`; appends `? help` when no dialog is active; applies `uichrome.ColorShortcutLine` for two-tone styling; appends the pre-styled `Dim` browse-only hint after two-toning |

### Dialog render grammar

All dialogs share the same shell via `renderDialogShell(body dialogBody, w, h int)`:

```
╭── Dialog Title ─────╮
│  body row 1          │
│  body row 2          │
│  [ Confirm ]  [ Cancel ]  │
╰─────────────────────╯
```

The footer row uses `[ Label ]` bracket grammar (D37). Dialog inner width is clamped to `[DialogMinWidth, min(termW-4, DialogMaxWidth)]`. These constants are defined in `internal/uichrome`.

### Detail-pane field render grammar

Each field row follows the D29/D30/D31 anatomy:

```
focus(2) + label(16 chars, D50 truncated) + ": " + "[ " + value + " ]" + optional ▾
```

For `fieldKindSecretMask` the value renders as `••••••••` unless `r` has been pressed. For `fieldKindChoice` an optional dropdown renders below the field row, flipping above (D51) when the field is near the bottom of the visible pane.

### Dependency on internal/uichrome

All render files import `internal/uichrome` for:
- `WrapLine` — every content row inside the outer frame
- `HRuleLine` / `BottomBorder` / `RenderTopBorder` — chrome borders
- `Overlay` — dialog, help modal, and dropdown splicing
- Color palette (`LightGray`, `Green`, `Red`, `Yellow`, `Cyan`, `Dim`, `White`) — all coloring
- Geometry constants (`MinTerminalWidth`, `MinTerminalHeight`, `DialogMaxWidth`, `DialogMinWidth`, `HelpModalMaxWidth`) — guard and sizing logic

See [`docs/code-packages/uichrome.md`](uichrome.md) for the full API.

## Dependencies

```
internal/workflowedit
    ├── internal/workflowio        (Load, Save, detect functions, SaveFS interface)
    ├── internal/workflowmodel     (WorkflowDoc, Step, StepKind, EnvEntry, StatusLineBlock)
    ├── internal/workflowvalidate  (Validate bridge)
    ├── internal/uichrome          (border helpers, color palette, overlay, geometry constants)
    └── internal/ansi              (StripAll for plain-text sanitization)
```

## Testing

Tests are in `src/internal/workflowedit/` alongside the implementation files. Key test files:

| File | Coverage |
|------|----------|
| `model_test.go` | 20+ mode-coverage tests verifying cursor state, dialog transitions, and field-type routing |
| `dialogs_render_test.go` | Render assertions for each `DialogKind` |
| `dialog_update_test.go` | Update handlers for all four dialog categories (FileConflict, FirstSaveConfirm, CrashTempNotice, Recovery) |
| `outline_render_test.go` | Phase section headers, top-level sections, `+ Add` rows |
| `outline_update_test.go` | Collapse toggle, cursor-to-header, `a` on header |
| `detail_pane_render_test.go` | Field rendering for all five field kinds |
| `detail_input_test.go` | 16 acceptance tests for plain-text sanitization, choice-list navigation, numeric field behaviors, secret masking, model suggestions, and path picker |
| `banner_session_test.go` | 10 acceptance tests for load-pipeline banner forwarding and session-event logging |

A `newTestModelWithLog` helper (`helpers_test.go`) constructs a Model with an in-memory `io.Writer` for asserting session-event log output.

## Related Documentation

- [`docs/features/workflow-builder.md`](../features/workflow-builder.md) — user-facing TUI behavior, keyboard map, visual layout, and all dialog flows
- [`docs/code-packages/uichrome.md`](uichrome.md) — shared chrome primitives (border helpers, palette, overlay, geometry constants)
- [`docs/code-packages/workflowio.md`](workflowio.md) — Load, Save, and detect functions
- [`docs/code-packages/workflowmodel.md`](workflowmodel.md) — WorkflowDoc types
- [`docs/code-packages/workflowvalidate.md`](workflowvalidate.md) — validator bridge
- [`docs/adr/20260424120000-workflow-builder-save-durability.md`](../adr/20260424120000-workflow-builder-save-durability.md) — atomic save rationale
