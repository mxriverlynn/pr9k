# 19 — Dialog: Copy Broken Reference

`DialogCopyBrokenRef` — error dialog presented when File > New > Copy from default detects that the bundled default workflow has a broken internal reference. Per behavioral D61 and impl-decision D-39.

## Render

```text
                ╭─ Default bundle has broken references ─────────────────╮
                │                                                        │
                │   The bundled default workflow references files that   │
                │   are not present in the bundle.                       │
                │                                                        │
                │   Missing files:                                        │
                │     · prompts/missing-prompt.md                        │
                │     · scripts/lost-script.sh                           │
                │                                                        │
                │   No edits were lost. You can copy what is present and │
                │   the validator will surface the broken references as  │
                │   fatal findings, or cancel and report the issue.      │
                │                                                        │
                │     (white)y(/)  Copy anyway   land in edit view with fatals known  │
                │     (white)c(/)  Cancel        return to the prior state            │
                │                                                        │
                │                  Copy anyway   [(green) Cancel (/)]                  │
                ╰────────────────────────────────────────────────────────╯
```

## Annotations

- Title `Default bundle has broken references` in **white**
- Body opens with the user-visible problem
- Bullet-list of missing files in **light gray** (one per row, prefixed with `·`)
- Reassurance + recommendation sentence
- Two lettered actions: `y` for "Copy anyway" (proceeds with partial copy), `c` for "Cancel" (default — safe — returns to prior state)
- Footer right-aligned: Copy anyway (in **white**), `[ Cancel ]` (green default)

## Footer in this mode

```text
│  (white)y(/) copy anyway  (white)c(/) cancel  (white)Esc(/) cancel  (white)Enter(/) cancel                                                  (white)v0.7.3(/) │
```

## Outcome paths

- `y` / Copy anyway — closes this dialog, advances to `DialogPathPicker` (PickerKindNew); after path confirmation, builder loads the partial copy and lands in edit view with the validator's fatal findings already visible (the missing files appear as fatal findings)
- `c` / Cancel / Esc / Enter — closes the dialog, returns to whichever state preceded the File > New choice

## Variant: many missing files

If the bundle has more than 8 broken references, the list truncates with `+ N more`:

```text
                │   Missing files:                                        │
                │     · prompts/old-step.md                              │
                │     · prompts/another.md                               │
                │     · scripts/old-helper.sh                            │
                │     · prompts/extra.md                                 │
                │     · scripts/lost.sh                                  │
                │     · prompts/gone.md                                  │
                │     · prompts/foo.md                                   │
                │     · scripts/bar.sh                                   │
                │     + 3 more (see findings panel after copy for full list) │
```

## Cross-references

- Behavioral spec: [Alternate Flows — File > New — copy from default](../../workflow-builder/feature-specification.md#file--new--copy-from-default), [D61](../../workflow-builder/artifacts/decision-log.md#d61-default-bundle-reference-integrity-check-before-copy).
- Impl decisions: [D-39](../../workflow-builder/artifacts/implementation-decision-log.md).
