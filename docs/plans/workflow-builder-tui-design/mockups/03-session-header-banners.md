# 03 — Session Header Banners

The session header row variants: each banner type, multi-banner with `[N more]`, dirty indicator, transient post-save / no-changes feedback.

The session header is row 4 of the persistent frame (between the menu-bar hrule and the pane-area hrule). All variants below show only the session header row.

## Slot ordering

Left-to-right: target path → unsaved-changes indicator (`●`) → at-most-one banner (priority resolved) → `[N more warnings]` affordance → findings summary (right-aligned).

## Variant A: clean session, no banners, no findings

```text
│  ~/projects/foo/.pr9k/workflow/config.json                                                                        │
```

- Target path in **white**
- No `●` (workflow is clean)
- No banner
- No findings summary (counts all zero)

## Variant B: dirty session, no banners

```text
│  ~/projects/foo/.pr9k/workflow/config.json (green)●(/)                                                                      │
```

- `●` glyph in **green**, one space after the path

## Variant C: dirty session with read-only banner (highest priority)

```text
│  ~/projects/foo/.pr9k/workflow/config.json (green)●(/)  (red)[ro](/) workflow target is read-only — File > Save disabled                          │
```

- `[ro]` tag and message in **red** (color 9)
- Banner text describes the user-visible consequence ("File > Save disabled") not the technical cause ("EACCES")

## Variant D: external-workflow banner

```text
│  /tmp/scratch/wf/config.json (green)●(/)  (yellow)[ext](/) workflow is outside your project and home — first save will confirm                    │
```

- `[ext]` tag and message in **yellow** (color 11)

## Variant E: symlink banner

```text
│  ~/projects/foo/.pr9k/workflow/config.json (green)●(/)  (yellow)[sym](/) config.json is a symlink → ~/.shared/wf/config.json                      │
```

- `[sym]` tag in **yellow**, with arrow `→` to the resolved target path

## Variant F: shared-install banner

```text
│  /usr/local/share/pr9k/workflow/config.json (green)●(/)  (yellow)[shared](/) editing the bundled default — saves affect all users of this binary │
```

- `[shared]` tag in **yellow**

## Variant G: unknown-field banner

```text
│  ~/projects/foo/.pr9k/workflow/config.json (green)●(/)  (cyan)[?fields](/) loaded fields not recognized: experimentalFlag, futureField (will drop on save) │
```

- `[?fields]` tag in **cyan** (color 14)

## Variant H: multi-banner with `[N more warnings]` affordance

When multiple banners are simultaneously active, the highest-priority banner shows; suppressed banners are reachable via the `[N more]` affordance.

```text
│  ~/projects/foo/.pr9k/workflow/config.json (green)●(/)  (red)[ro](/) workflow target is read-only — File > Save disabled  (white)[2 more warnings](/) │
```

- The displayed banner is `[ro]` (highest priority among ro / ext / sym / shared / ?fields)
- `[2 more warnings]` affordance immediately follows in **white**
- Activating the affordance opens a small banner panel (dialog-shaped overlay) listing all active banners

### Banner panel (opened via `[N more warnings]`)

```text
                  ╭─ Active warnings ──────────────────────────────────────╮
                  │                                                        │
                  │   (red)[ro](/)      workflow target is read-only — File > Save │
                  │              disabled                                  │
                  │                                                        │
                  │   (yellow)[sym](/)     config.json is a symlink →                  │
                  │              ~/.shared/wf/config.json                  │
                  │                                                        │
                  │   (cyan)[?fields](/) loaded fields not recognized:                │
                  │              experimentalFlag, futureField             │
                  │                                                        │
                  │                                       [(green) Close (/)]              │
                  ╰────────────────────────────────────────────────────────╯
```

- Centered overlay shape per [D36](../artifacts/decision-log.md#d36-dialog-centered-overlay-shape-and-borders)
- Each active banner rendered with its tag and color
- Cancel / Close as keyboard default

## Variant I: dirty + banner + findings summary together

```text
│  ~/projects/foo/.pr9k/workflow/config.json (green)●(/)  (yellow)[ext](/) workflow is outside your project and home — first save  (red)[!](/) 3 fatal · 2 warn │
```

- All slots populated; findings summary right-aligned at row's right edge
- The findings summary's `[!]` prefix renders in **red** because fatals exist; the count text in **light gray**. When only non-fatal findings are present, the prefix is `[i]` in **cyan** (e.g., `[i] 2 warn · 1 info`)
- When the row is too narrow to fit everything, slots drop in order per [D17](../artifacts/decision-log.md#d17-session-header-overflow-priority): `[N more]` first, then findings summary, then banner truncates with `…`, then path truncates more aggressively. Dirty indicator never drops while dirty.

## Variant J: post-save success transient banner

For ~3 seconds immediately after a successful save:

```text
│  ~/projects/foo/.pr9k/workflow/config.json     (green)Saved at 14:32:07(/)                                                          │
```

- Workflow is clean (no `●`)
- `Saved at HH:MM:SS` in **green**, replacing whatever banner was previously in the slot
- After ~3 seconds, the banner clears and the priority-resolved banner (if any) takes over the slot

## Variant K: no-op-save transient banner

When the user invokes Save with no in-memory changes:

```text
│  ~/projects/foo/.pr9k/workflow/config.json     No changes to save                                                          │
```

- `No changes to save` in **light gray**, ~3-second duration
- The file on disk is untouched (per behavioral D63)

## Variant L: narrow terminal — overflow handling (80 cols)

```text
│  …/.pr9k/workflow/config.json (green)●(/)  (yellow)[ext](/) workflow is outside your … (white)[2 more](/)│
```

- Path left-truncated with `…` prefix
- Banner truncated with `…` suffix
- `[N more warnings]` shortened to `[N more]`
- Findings summary dropped (insufficient width)

## Variant M: full overflow stress test at 80 cols

When the row carries a long path AND a `[shared]` banner with full text AND multiple banners AND a findings summary all at once on an 80-column terminal:

```text
│  …deep/proj/.pr9k/wf/config.json (green)●(/)  (red)[ro](/) workflow is read-on… (white)[3 more](/)│
```

- Path is aggressively left-truncated to keep the filename visible
- Banner shows the highest-priority active item (`[ro]` red) truncated at the widest fit
- `[N more]` reflects the remaining 3 banners (sym, shared, ?fields)
- Findings summary is the first slot dropped because the dirty indicator and at-most-one-banner are protected per the overflow priority rule

## Variant N: extreme overflow — only the non-droppable slots remain

When the path itself is so long that even after aggressive truncation only the path + dirty + truncated banner can fit:

```text
│  …config.json (green)●(/)  (red)[ro] read-on…(/)                                            │
```

- Path reduced to just the filename
- Banner truncated to its first few characters
- Both `[N more]` and findings summary dropped

## Cross-references

- Behavioral spec: [Primary Flow §5](../../workflow-builder/feature-specification.md#primary-flow), [D49](../../workflow-builder/artifacts/decision-log.md#d49-session-header-banner-priority), [D53](../../workflow-builder/artifacts/decision-log.md#d53-post-save-success-feedback), [D63](../../workflow-builder/artifacts/decision-log.md#d63-no-op-save-behavior).
- Visual decisions: [D5](../artifacts/decision-log.md#d5-session-header-row-content-and-ordering), [D13](../artifacts/decision-log.md#d13-unsaved-changes-indicator-glyph-and-color), [D14](../artifacts/decision-log.md#d14-banner-prefix-glyphs-and-severity-coloring), [D15](../artifacts/decision-log.md#d15-multi-banner-affordance-rendering-and-activation), [D16](../artifacts/decision-log.md#d16-findings-summary-position-and-format), [D17](../artifacts/decision-log.md#d17-session-header-overflow-priority).
