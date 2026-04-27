# 06 — Status Footer Per Mode

The status footer text changes by focused widget per behavioral impl-decision D-11 (widget-owned `ShortcutLine() string`). This file enumerates the footer for every mode in the 28-mode coverage table plus a few common visual cases not in that table.

The footer row is always row N-1 of the persistent frame. All renders below show only the footer row.

## Default rendering rule

```text
│  (white)<key>(/) <description>  (white)<key>(/) <description>  …                                       (white)v0.7.3(/) │
```

- Two-tone two-space-separated shortcut groups
- Right-aligned version label

## Empty-editor (no workflow loaded)

Mode 1 starting state.

```text
│  (white)F10(/) menu  (white)Ctrl+N(/) new  (white)Ctrl+O(/) open  (white)Ctrl+Q(/) quit  (white)?(/) help                                          (white)v0.7.3(/) │
```

## Edit view — outline focus on a step row

Mode 6, 9 starting state.

```text
│  (white)↑↓(/) navigate  (white)Tab(/) detail pane  (white)Enter(/) edit  (white)Del(/) remove  (white)r(/) reorder  (white)a(/) add step       (white)v0.7.3(/) │
```

## Edit view — outline focus on a section header

```text
│  (white)↑↓(/) navigate  (white)Enter(/) toggle collapse  (white)a(/) add item  (white)Tab(/) detail pane                              (white)v0.7.3(/) │
```

## Edit view — outline focus on a `+ Add` row

```text
│  (white)↑↓(/) navigate  (white)Enter(/) add                                                                        (white)v0.7.3(/) │
```

## Edit view — detail-pane focus on a plain-text field

Mode 7.

```text
│  (white)Tab(/) next field  (white)Shift+Tab(/) prev field  (white)Esc(/) outline  (white)Ctrl+S(/) save                                  (white)v0.7.3(/) │
```

## Edit view — detail-pane focus on a choice-list field

```text
│  (white)Enter(/) open list  (white)Tab(/) next field  (white)Shift+Tab(/) prev field  (white)Esc(/) outline  (white)Ctrl+S(/) save                  (white)v0.7.3(/) │
```

## Edit view — choice-list dropdown open

Mode 14, 15.

```text
│  (white)↑↓(/) navigate  (white)<char>(/) jump  (white)Enter(/) confirm  (white)Esc(/) cancel                                          (white)v0.7.3(/) │
```

## Edit view — detail-pane focus on a numeric field

```text
│  (white)0-9(/) digits only  (white)Tab(/) next field  (white)Esc(/) outline  (white)Ctrl+S(/) save                                  (white)v0.7.3(/) │
```

## Edit view — detail-pane focus on a masked secret field (default)

Mode 16 starting state.

```text
│  (white)r(/) reveal  (white)Tab(/) next field  (white)Esc(/) outline  (white)Ctrl+S(/) save                                       (white)v0.7.3(/) │
```

## Edit view — detail-pane focus on a revealed secret field

After mode 16's `r`.

```text
│  (white)r(/) mask  (white)Tab(/) next field  (white)Esc(/) outline  (white)Ctrl+S(/) save                                          (white)v0.7.3(/) │
```

## Edit view — detail-pane focus on a model-suggestion field (closed)

```text
│  (white)Tab(/) suggestions  (white)Enter(/) commit  (white)Esc(/) outline  (white)Ctrl+S(/) save                                    (white)v0.7.3(/) │
```

## Edit view — detail-pane focus on a model-suggestion field (open)

```text
│  (white)↑↓(/) navigate  (white)<char>(/) filter  (white)Enter(/) pick  (white)Esc(/) commit typed                                  (white)v0.7.3(/) │
```

## Edit view — detail-pane focus on a multi-line / external-editor field

```text
│  (white)Ctrl+E(/) open in editor  (white)Tab(/) next field  (white)Esc(/) outline                                            (white)v0.7.3(/) │
```

## Reorder mode active

Modes 10, 11.

```text
│  (white)↑/↓(/) move step  (white)Enter(/) commit  (white)Esc(/) cancel                                                            (white)v0.7.3(/) │
```

## Save in progress

Mode 18 transient.

```text
│  Saving...                                                                                          (white)v0.7.3(/) │
```

- Single text in **white** (prompt-mode coloring per [D34](../artifacts/decision-log.md#d34-status-footer-shortcut-vs-prompt-rendering-rule))

## Save validation in progress

Briefly between Ctrl+S and the validation result.

```text
│  Validating...                                                                                       (white)v0.7.3(/) │
```

## Findings panel open (no help modal)

Mode 22 starting state.

```text
│  (white)↑↓(/) navigate  (white)Enter(/) jump to field  (white)a(/) acknowledge  (white)Esc(/) close panel                              (white)v0.7.3(/) │
```

## Findings panel + help modal open (only legal coexistence)

Mode 23.

```text
│  (white)?(/) close help  (white)Esc(/) close help                                                                       (white)v0.7.3(/) │
```

## Quit confirm — no unsaved changes

Mode 24, 25 starting state.

```text
│  Quit the workflow builder? (y/n, esc to cancel)                                                     (white)v0.7.3(/) │
```

- Prompt-mode rendering: entire string in **white**

## Unsaved-changes dialog open

Mode 26, 27 starting state.

```text
│  Unsaved changes — save before continuing? (s)ave / (c)ancel / (d)iscard                              (white)v0.7.3(/) │
```

## Path picker open

Mode 3 starting state.

```text
│  (white)Tab(/) complete  (white)Enter(/) confirm  (white)Esc(/) cancel                                                              (white)v0.7.3(/) │
```

## Recovery view (parse error)

Mode 28 starting state.

```text
│  (white)e(/) open in editor  (white)r(/) reload  (white)d(/) discard  (white)Esc(/) cancel                                              (white)v0.7.3(/) │
```

## External-editor opening (transient)

```text
│  Opening editor — terminal will be released momentarily…                                              (white)v0.7.3(/) │
```

- Prompt-mode rendering: entire string in **white**

## Browse-only (read-only target)

```text
│  (white)↑↓(/) navigate  (white)Tab(/) detail pane  (white)?(/) help                                                                  (white)v0.7.3(/) │
```

- The footer renders normally — only navigation shortcuts. The read-only signal is owned by the session-header `[ro]` banner (in **red**) and the greyed `Save` menu item; duplicating it in the footer would conflict with the `colorShortcutLine` two-tone tokenizer (which doesn't support a third color for arbitrary phrases). Per [F4](../artifacts/team-findings.md#f4-browse-only-footers-red-save-disabled-phrase-conflicts-with-colorshortcutline-tokenizer)

## Empty-editor with banner panel open

```text
│  (white)Esc(/) close                                                                                  (white)v0.7.3(/) │
```

## Help modal open over edit view

```text
│  (white)?(/) close help  (white)Esc(/) close help                                                                       (white)v0.7.3(/) │
```

## Cross-references

- Behavioral spec: [Primary Flow §5–§10](../../workflow-builder/feature-specification.md#primary-flow), [D24](../../workflow-builder/artifacts/decision-log.md#d24-help-modal-and-shortcut-footer).
- Impl decisions: [D-11](../../workflow-builder/artifacts/implementation-decision-log.md) (widget-owned ShortcutLine).
- Visual decisions: [D8](../artifacts/decision-log.md#d8-status-footer-row-content-and-ordering), [D34](../artifacts/decision-log.md#d34-status-footer-shortcut-vs-prompt-rendering-rule), [D35](../artifacts/decision-log.md#d35-version-label-position-and-color).
- Mode coverage: every row.
