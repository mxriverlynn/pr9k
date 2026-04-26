# 08 — Help Modal

The workflow-builder help modal. Mirrors the run-mode help modal's centered-overlay shape (per [D40](../artifacts/decision-log.md#d40-help-modal-mirrors-run-mode-help-modal-shape)).

The help modal is reachable from edit view (mode 5 starting state when in empty editor), or over the findings panel (mode 23 — the only legal coexistence per impl-decision D-8).

## Render (centered over edit view)

```text
            ╭─ Help: Keyboard Shortcuts ─────────────────────────────────────────╮
            │                                                                    │
            │  Global                                                            │
            │    F10           open File menu          Alt+F  open File menu     │
            │    Ctrl+N        File > New              Ctrl+O File > Open         │
            │    Ctrl+S        File > Save             Ctrl+Q File > Quit         │
            │    ?             show this help                                    │
            │                                                                    │
            │  Outline focus (cursor on a step row)                              │
            │    ↑ / ↓         navigate rows           Tab    focus detail pane   │
            │    Enter         edit selected step      Del    remove step         │
            │    Alt+↑ / Alt+↓ move step up / down     r      enter reorder mode  │
            │    a             add step to phase                                 │
            │                                                                    │
            │  Outline focus (cursor on section header)                          │
            │    ↑ / ↓         navigate                Enter  toggle collapse     │
            │    a             add item to section     Tab    focus detail pane   │
            │                                                                    │
            │  Detail pane focus (text / numeric field)                          │
            │    Tab           next field              Shift+Tab  prev field      │
            │    Esc           focus outline           Ctrl+S  save               │
            │                                                                    │
            │  Detail pane focus (choice list field)                             │
            │    Enter / Space open choice list        Esc    focus outline       │
            │    ↑ / ↓         navigate options        <char> jump to option      │
            │    Enter         confirm choice          Esc    cancel and revert   │
            │                                                                    │
            │  Detail pane focus (secret-mask field)                             │
            │    r             reveal / mask           Tab    next field          │
            │                                                                    │
            │  Detail pane focus (multi-line / prompt-file field)                │
            │    Ctrl+E        open in external editor                           │
            │                                                                    │
            │  Reorder mode                                                      │
            │    ↑ / ↓         move step               Enter  commit              │
            │    Esc           cancel                                            │
            │                                                                    │
            │  Findings panel                                                    │
            │    ↑ / ↓         navigate findings       Enter  jump to field       │
            │    a             acknowledge warning     Esc    close panel         │
            │                                                                    │
            │  Path picker                                                       │
            │    Tab           complete                Enter  open / create       │
            │    Esc           cancel                                            │
            │                                                                    │
            │                                                  ?  close help     │
            ╰────────────────────────────────────────────────────────────────────╯
```

## Annotations

- Top border `╭─ Help: Keyboard Shortcuts ─…─╮` with the title in **white**
- Section labels (`Global`, `Outline focus …`, etc.) in **white**, two-space indent inside the modal
- Each shortcut row: key in **white**, description in **light gray**, two-column grid where possible
- Bottom right: `?  close help` — the dismiss hint, in two-tone

## Behavior summary (visual)

- Centered over the underlying frame; underlying frame's pixels remain visible at the edges
- Width: `min(terminalWidth - 4, 72)` columns
- Height grows to content; if content exceeds terminal height, the help modal scrolls (the dismiss hint is pinned to the bottom border row regardless)
- Dismiss: `?` toggles closed; `Esc` also closes

## Render at narrow terminal (terminal width 70 cols)

```text
        ╭─ Help: Keyboard Shortcuts ─────────────────────────────╮
        │                                                        │
        │  Global                                                │
        │    F10           open File menu                        │
        │    Alt+F         open File menu                        │
        │    Ctrl+N        File > New                            │
        │    Ctrl+O        File > Open                           │
        │    Ctrl+S        File > Save                           │
        │    Ctrl+Q        File > Quit                           │
        │    ?             show this help                        │
        │                                                        │
        │  Outline focus (cursor on a step row)                  │
        │    ↑ / ↓         navigate rows                         │
        │    Tab           focus detail pane                     │
        │    Enter         edit selected step                    │
        │    Del           remove step                           │
        │    Alt+↑ / Alt+↓ move step up / down                   │
        │    r             enter reorder mode                    │
        │    a             add step to phase                     │
        │                                                        │
        │  …                                                     │
        │                                                        │
        │                              ?  close help             │
        ╰────────────────────────────────────────────────────────╯
```

- Single-column shortcut grid when the terminal is too narrow for two columns
- Sections beyond the visible area are accessible via scroll (mouse wheel or arrow keys); a small scroll indicator on the right edge appears

## Render over empty-editor frame (mode 5)

Mode 5's starting state is `EmptyEditor`, not the populated edit view. The help modal renders identically over the empty-editor frame; the underlying frame's centered hint panel is dimmed using `Color("8")` (Dim) while the modal is on top:

```text
╭── (green)Power-Ralph.9000(/) — Workflow Builder ──────────────────────────────────────────────────────────────────╮
│  (white)F(/)ile                                                                                                   │
├───────────────────────────────────────────────────────────────────────────────────────────────────────────────────┤
│  (no workflow open)                                                                                               │
├──────────────────────────────────────┬────────────────────────────────────────────────────────────────────────────┤
│  No workflow open                    │                                                                            │
│             ╭─ Help: Keyboard Shortcuts ─────────────────────────────────────────╮                                │
│             │                                                                    │                                │
│             │  Empty editor                                                      │                                │
│             │    F10           open File menu                                    │                                │
│             │    Ctrl+N        File > New                                        │                                │
│             │    Ctrl+O        File > Open                                       │                                │
│             │    Ctrl+Q        File > Quit                                       │                                │
│             │    ?             show this help                                    │                                │
│             │                                                                    │                                │
│             │  (gray)…(/)                                                                  │                                │
│             │                                                                    │                                │
│             │                                                  ?  close help     │                                │
│             ╰────────────────────────────────────────────────────────────────────╯                                │
│                                                                                                                   │
…                                                                                                                   │
├──────────────────────────────────────┴────────────────────────────────────────────────────────────────────────────┤
│  (white)?(/) close help  (white)Esc(/) close help                                                                       (white)v0.7.3(/) │
╰───────────────────────────────────────────────────────────────────────────────────────────────────────────────────╯
```

- Underlying empty-editor frame visible at the modal's edges (the centered hint panel from mockup 00 sits behind the modal but is dimmed)
- The help modal's first body section is "Empty editor" instead of "Outline focus" — the section list is mode-aware
- All other dialog conventions identical to the edit-view variant

## Render with internal scroll (modal taller than terminal)

When the help modal's content exceeds terminal height, the modal scrolls internally. A single-column scroll indicator runs down the second-rightmost column inside the right border (per F24 / D40 extension):

```text
            ╭─ Help: Keyboard Shortcuts ─────────────────────────────────────────╮
            │                                                                  ▲ │
            │  Outline focus (cursor on a step row)                            │ │
            │    ↑ / ↓         navigate rows           Tab    focus detail pane│█│
            │    Enter         edit selected step      Del    remove step      │█│
            │    Alt+↑ / Alt+↓ move step up / down     r      enter reorder    │█│
            │    a             add step to phase                               │ │
            │                                                                  │ │
            │  Outline focus (cursor on section header)                        │ │
            │    ↑ / ↓         navigate                Enter  toggle collapse  │ │
            │    a             add item to section     Tab    focus detail pane│ │
            │                                                                  │ │
            │  Detail pane focus (text / numeric field)                        │ │
            │    Tab           next field              Shift+Tab  prev field   │ │
            │    Esc           focus outline           Ctrl+S  save            │▼│
            │                                                  ?  close help     │
            ╰────────────────────────────────────────────────────────────────────╯
```

- Scroll indicator: `▲` at top, `█` for visible region, `▼` at bottom, `│` (chrome) elsewhere — same shape as the outline scroll indicator (D25)
- The `?  close help` dismiss-hint row is **pinned** to the modal's bottom border row regardless of scroll position (the modal scrolls *inside* the bordered frame, not the frame itself)

## Cross-references

- Behavioral spec: [Primary Flow §8](../../workflow-builder/feature-specification.md#primary-flow), [D24](../../workflow-builder/artifacts/decision-log.md#d24-help-modal-and-shortcut-footer).
- Run-mode reference: `internal/ui/model.go:567-643` (`renderHelpModal`).
- Visual decisions: [D40](../artifacts/decision-log.md#d40-help-modal-mirrors-run-mode-help-modal-shape) (revised — column-threshold and scroll-indicator rules added).
- Team findings: [F10](../artifacts/team-findings.md#f10-help-modal-column-count-threshold-not-specified), [F15](../artifacts/team-findings.md#f15-mode-5-help-modal-over-empty-editor-variant-missing), [F24](../artifacts/team-findings.md#f24-help-modal-scroll-indicator-placement-undefined).
- Mode coverage: rows 5, 23.
