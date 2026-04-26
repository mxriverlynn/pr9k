# 05 — Detail Pane Fields

Every field kind: text, choice list (closed + open), numeric, secret-mask (masked + revealed), model-suggestion (closed + open), multi-line / external-editor, section-summary.

The detail pane is the right pane in the loaded-workflow render, taking the remaining width after the outline. All renders below show the detail pane only — the `│` at the left is the pane separator and the `│` at the right is the right border.

## A: Plain-text field (unfocused vs focused)

Unfocused:

```text
│   (white)Name:(/)         [iterate                                        ]   identifier only │
```

Focused (cursor at end):

```text
│   (white)Name:(/)         [iterate(white)▏(/)                                       ]   identifier only │
```

- Label in **white**, colon, single space, bracketed input box
- Input border `[` / `]` in **light gray**
- Input value in **white**
- Cursor `▏` in **white** at the cursor position when focused
- Right-aligned hint `identifier only` in **light gray**

## B: Plain-text field with sanitization warning

After the user pastes text containing newlines:

```text
│   (white)Capture as:(/)   [iteration_output(white)▏(/)                              ]                 │
│                  (gray)↳ pasted content sanitized — newlines stripped(/)                       │
```

- A second row below the input renders the warning in **light gray** with a `↳` glyph
- Warning auto-clears after the user types one more character

## C: Choice list — closed (unfocused vs focused)

Unfocused:

```text
│   (white)Capture mode:(/) [lastLine                                       ▾]                 │
```

Focused:

```text
│   (white)Capture mode:(/) (white)[(/)lastLine                                       ▾(white)](/)                 │
```

- The `▾` is **always present** for choice-list fields, focused or not, per behavioral D27
- When focused, the bracket border `[` `]` brightens to **white** (matches plain-text-field focus per [D27](../artifacts/decision-log.md#d27-plain-text-field-rendering)). **No reverse-video on the field body** — reverse-video is reserved for the highlighted item inside the open dropdown (Section D)

## D: Choice list — open

After the user presses Enter on a focused choice-list field:

```text
│   (white)Capture mode:(/) (white)[(/)lastLine                                       ▾(white)](/)                 │
│                  ╭────────────────────╮                                                   │
│                  │ (reverse)lastLine            (/)│                                                   │
│                  │ fullStdout           │                                                   │
│                  ╰────────────────────╯                                                   │
│                                                                                            │
│   (white)Timeout:(/)      [180         ] seconds   1..86400                                       │
```

- Bordered dropdown anchored below the input row, left-edge aligned with the input's left bracket
- Each choice on its own row in **white**
- The highlighted choice (cursor) renders in reverse-video — this is the legitimate use of reverse-video inside the detail pane (one of two reserved states with reorder mode)
- Dropdown width sized to the longest choice plus 4 (2 for borders, 2 for padding)
- Subsequent fields below the dropdown are visually pushed (the dropdown overlays them; the underlying content remains in its position)
- If the dropdown would extend past the pane bottom, it instead renders **upward** anchored to the field's top edge (per [D51](../artifacts/decision-log.md#d51-choice-list-dropdown-overflow-rule))

## E: Choice list — character-jump active

When the user types a character inside an open dropdown, the cursor jumps to the next option starting with that character:

```text
│   (white)On timeout:(/)   (white)[(/)continue                                       ▾(white)](/)                 │
│                  ╭────────────────────╮                                                   │
│                  │ continue             │                                                   │
│                  │ (reverse)fail                (/)│   ← cursor jumped here on `f`                       │
│                  ╰────────────────────╯                                                   │
```

## F: Numeric field

Unfocused:

```text
│   (white)Timeout:(/)      [180         ] seconds   1..86400                                       │
│   (white)Refresh:(/)      [0           ] seconds   0 disables refresh                            │
```

- Same bracket-input shape as plain text
- Input area is right-padded so digits read right-aligned within the bracket
- Units suffix (`seconds`) in **light gray** after the closing bracket
- Right-aligned hint `1..86400` or `0 disables refresh` in **light gray**

Focused with cursor:

```text
│   (white)Timeout:(/)      [(white)180▏        (/)] seconds   1..86400                                       │
```

## G: Numeric field — paste sanitized

After the user pastes `120abc`:

```text
│   (white)Timeout:(/)      [(white)120▏         (/)] seconds   1..86400                                       │
│                  (gray)↳ pasted content sanitized at first non-digit(/)                              │
```

- Digits before the first non-digit are accepted; everything from the first non-digit onward is discarded
- Warning row in **light gray** below the input
- Warning auto-clears after next keystroke

## H: Secret-mask field — default (masked)

```text
│   (white)ANTHROPIC_API_KEY:(/) [••••••••                                  ]   r to reveal │
```

- The mask glyph `•` (U+2022) repeats 8 times regardless of actual value length
- The hint `r to reveal` right-aligned in **light gray** — **no bracket delimiters** (per F8 resolution; bracket syntax is reserved for clickable affordances)

## I: Secret-mask field — focused

```text
│   (white)ANTHROPIC_API_KEY:(/) (white)[(/)••••••••                                  (white)](/)   r to reveal │
```

- The bracket border `[` `]` brightens to **white** to signal focus
- **No reverse-video** on the field body (reverse-video is reserved for the highlighted item inside an open dropdown)

## J: Secret-mask field — revealed

After the user presses `r` while the field is focused:

```text
│   (white)ANTHROPIC_API_KEY:(/) (white)[(/)sk-ant-api03-AbC1234DefGhI5678JkL(white)▏(/)        (white)](/)   r to mask │
```

- The actual value rendered (truncated with `…` if it exceeds the input width)
- The hint changes to `r to mask` in **light gray**

## K: Secret-mask field — re-mask on focus-leave

When the user navigates away (Tab to next field), the field re-masks automatically:

```text
│   (white)ANTHROPIC_API_KEY:(/) [••••••••                                  ]   r to reveal │
│   (white)DEBUG:(/)            (white)[(/)false                                       ▾(white)](/)                 │
```

## L: Model-suggestion field — closed

```text
│   (white)Model:(/)        [claude-sonnet-4-6                              ▾]                 │
```

- Same shape as choice-list-closed, but accepts free text on Enter (in addition to suggestion picks)

## M: Model-suggestion field — open

After the user presses Tab or Enter on a focused model field:

```text
│   (white)Model:(/)        [(reverse)claude-sonnet-4-6(white)▏(/)                              ▾(/)]                 │
│                  ╭───────────────────────────╮                                            │
│                  │ (reverse)claude-opus-4-7           (/)│                                            │
│                  │ claude-sonnet-4-6           │                                            │
│                  │ claude-haiku-4-5            │                                            │
│                  │ claude-opus-4-6             │                                            │
│                  │ claude-sonnet-4-5           │                                            │
│                  ╰───────────────────────────╯                                            │
│                  (gray)or type any value · Esc commits typed value(/)                              │
```

- Same dropdown shape as choice-list, but the input remains editable while open
- A hint row below the dropdown explains the free-text-acceptance contract
- Esc closes the dropdown and commits the typed value (which need not match a suggestion)

## N: Model-suggestion field — typed filter

When the user types `op` in an open model field:

```text
│   (white)Model:(/)        [(reverse)op(white)▏(/)                                            ▾(/)]                 │
│                  ╭───────────────────────────╮                                            │
│                  │ (reverse)claude-opus-4-7           (/)│                                            │
│                  │ claude-opus-4-6             │                                            │
│                  ╰───────────────────────────╯                                            │
│                  (gray)or type any value · Esc commits typed value(/)                              │
```

- The dropdown filters to suggestions containing the typed substring
- The user can still commit `op` literally with Esc (rare but supported)

## O: Multi-line / external-editor field

File exists:

```text
│   (white)Prompt file:(/) prompts/iterate.md                                                       │
│                  (gray)5,237 bytes · last modified 2026-04-26 14:32(/)                              │
│                  [(white)Ctrl+E(/) open in editor]                                                       │
```

File missing:

```text
│   (white)Prompt file:(/) (gray)prompts/missing.md(/)  (red)not found(/)                                          │
│                  [(white)Ctrl+E(/) create and open in editor]                                            │
```

- Path on first row in **white** (or **light gray** with red `not found` marker if absent)
- Metadata row in **light gray** (size · last modified)
- Action row in **white**, with the action label adapting to "create and open" when the file is missing
- When this field is focused, the status footer also shows `Ctrl+E open in editor`

## P: Section-summary view (Iteration phase header focused)

When the cursor in the outline is on the `Iteration` section header (not on an item), the detail pane shows:

```text
│   (white)Iteration phase  ·  3 steps(/)                                                              │
│                                                                                            │
│    1. iterate              sonnet                                                          │
│    2. test-plan            opus                                                            │
│    3. commit               shell                                                           │
│                                                                                            │
│    [(white)+ Add step(/)]                                                                              │
```

- Section title in **white** with bullet separator
- Numbered list of items in **light gray** (kind annotation on each row)
- `[+ Add step]` action box in **white**, focusable

## Q: Section-summary view (env section focused, with secret keys)

```text
│   (white)containerEnv  ·  3 entries(/)                                                              │
│                                                                                            │
│    ANTHROPIC_API_KEY (gray)(masked)(/)                                                              │
│    DEBUG                                                                                   │
│    LOG_LEVEL                                                                               │
│                                                                                            │
│    [(white)+ Add container env entry(/)]                                                              │
```

- Secret keys (those matching the secret pattern) annotated with `(masked)` in **light gray** to indicate their values are not shown in summary

## R: Section-summary view (empty section)

```text
│   (white)env  ·  0 entries(/)                                                                       │
│                                                                                            │
│    (gray)(empty — no entries configured)(/)                                                          │
│                                                                                            │
│    [(white)+ Add env variable(/)]                                                                   │
```

- Empty notice in **light gray**

## S: Section-summary view (statusLine block configured)

```text
│   (white)statusLine block(/)                                                                         │
│                                                                                            │
│    Type:                  command                                                          │
│    Command:               (gray)scripts/statusline(/)                                                 │
│    Refresh:               5 seconds                                                        │
│                                                                                            │
│    [(white)Edit fields(/)]                                                                              │
```

- Three fields rendered as a name-value table
- `[Edit fields]` action moves cursor to first field of the statusLine block in detail-pane edit mode

## Cross-references

- Behavioral spec: [Primary Flow §7](../../workflow-builder/feature-specification.md#primary-flow), [D12](../../workflow-builder/artifacts/decision-log.md#d12-constrained-fields-as-choice-lists-model-as-free-text-with-suggestions), [D20](../../workflow-builder/artifacts/decision-log.md#d20-containerenv-secret-masking), [D27](../../workflow-builder/artifacts/decision-log.md#d27-unfocused-field-signifiers), [D42](../../workflow-builder/artifacts/decision-log.md#d42-structured-field-input-sanitization), [D45](../../workflow-builder/artifacts/decision-log.md#d45-choice-list-keyboard-contract), [D47](../../workflow-builder/artifacts/decision-log.md#d47-secret-reveal-keyboard-binding), [D51](../../workflow-builder/artifacts/decision-log.md#d51-section-summary-content), [D58](../../workflow-builder/artifacts/decision-log.md#d58-model-suggestion-list-maintenance), [D62](../../workflow-builder/artifacts/decision-log.md#d62-numeric-field-non-numeric-input-behavior).
- Visual decisions: [D26](../artifacts/decision-log.md#d26-detail-pane-field-rendering-grammar)–[D33](../artifacts/decision-log.md#d33-section-summary-rendering-per-section-type).
- Mode coverage: rows 7, 14, 15, 16, 17.
