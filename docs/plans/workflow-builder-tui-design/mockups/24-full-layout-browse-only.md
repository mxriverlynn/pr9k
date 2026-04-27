# 24 — Full Layout: Browse-Only (Read-Only Target)

The full-frame view when the loaded workflow's destination is read-only. Three simultaneous visual signals fire: greyed `Save` in the menu, `[ro]` banner in the session header, and dirty-tracking disabled. Per F14 / GAP-002.

## State

- Workflow loaded from `/usr/local/share/pr9k/workflow/config.json` (system-installed bundle, read-only to current user)
- File > Save greyed in menu
- `[ro]` banner in session header
- No `●` dirty indicator (dirty-tracking is disabled in browse-only)
- Outline cursor on iteration step `iterate`
- Detail pane shows the step's fields (read-only — same rendering as edit mode but the user cannot commit changes)
- 0 findings (validator runs on save and saves are disabled)

## Render (120×32 terminal)

```text
╭── (green)Power-Ralph.9000(/) — Workflow Builder — /usr/local/share/pr9k/workflow/config.json ─────────────────────╮
│  (white)F(/)ile                                                                                                   │
├───────────────────────────────────────────────────────────────────────────────────────────────────────────────────┤
│  /usr/local/share/pr9k/workflow/config.json    (red)[ro](/) workflow target is read-only — File > Save disabled         │
├──────────────────────────────────────┬────────────────────────────────────────────────────────────────────────────┤
│  ▾ env  (1)                          │  Step  ·  iterate                                                          │
│    ⋮⋮ MY_TOKEN                       │                                                                            │
│  ▾ containerEnv  (2)                 │   (white)Name:(/)         [iterate                            ]   identifier only │
│    ⋮⋮ ANTHROPIC_API_KEY              │                                                                            │
│    ⋮⋮ DEBUG                          │   (white)Kind:(/)         [Claude (≡)                       ▾]                   │
│  ▾ statusLine  (1)                   │                                                                            │
│    ⋮⋮ [≣] script-based               │   (white)Model:(/)        [claude-sonnet-4-6                ▾]                   │
│  ▾ Initialize  (1)                   │                                                                            │
│    ⋮⋮ [≡] splash      sonnet         │   (white)Prompt file:(/) prompts/iterate.md                                      │
│  ▾ Iteration  (3)                    │                  (gray)5,237 bytes · last modified 2026-04-26 14:32(/)               │
│ (white)> ⋮⋮ [≡] iterate     sonnet        (/)│                  [(white)Ctrl+E(/) open in editor]                                  │
│    ⋮⋮ [≡] test-plan   opus           │                                                                            │
│    ⋮⋮ [$] commit                  — │   (white)Capture as:(/)   [iteration_output                  ]                    │
│  ▾ Finalize  (2)                     │   (white)Capture mode:(/) [lastLine                       ▾]                   │
│    ⋮⋮ [≡] code-review opus           │                                                                            │
│    ⋮⋮ [$] update-docs             — │   (white)Timeout:(/)      [180         ] seconds   1..86400                       │
│                                      │   (white)On timeout:(/)   [continue                       ▾]                   │
│                                      │                                                                            │
│                                      │   (white)Skip if capture empty:(/) [yes                        ▾]                   │
│                                      │   (white)Resume previous:(/)       [no                         ▾]                   │
│                                      │                                                                            │
│                                      │   (white)Break loop if empty:(/)   [no                         ▾]                   │
│                                      │                                                                            │
│                                      │                                                                            │
├──────────────────────────────────────┴────────────────────────────────────────────────────────────────────────────┤
│  (white)↑↓(/) navigate  (white)Tab(/) detail pane  (white)?(/) help                                                                  (white)v0.7.3(/) │
╰───────────────────────────────────────────────────────────────────────────────────────────────────────────────────╯
```

## Annotations

- Top border title: target path is the system-shared install location, rendered identically to a writable location (the read-only signal is in the session header, not the title)
- Menu bar: closed state, `File` label normal — the greying happens **inside** the menu when opened (see Variant B)
- Session header: `[ro]` tag in **red** (color 9), banner text in **red** explaining the consequence ("File > Save disabled"). **No `●` dirty indicator** — dirty-tracking is disabled in browse-only mode, so no edit can ever set the dirty flag. **No findings summary** — validator does not run on save (because save is unavailable)
- Outline pane: identical rendering to writable mode; the user can navigate and inspect freely
- Detail pane: identical rendering to writable mode; the user can focus fields, open dropdowns, reveal secrets — all non-destructive operations work normally. Editing a field's value is allowed (the in-memory state mutates) but Ctrl+S is inert and there's no way to persist
- Status footer: standard outline-step-focused shortcuts **without** the `Save disabled` red phrase from the original mockup (per F4 — moved to the session-header banner). The footer renders cleanly through `colorShortcutLine`'s two-tone tokenizer

## Variant B: File menu open in browse-only mode (Save greyed)

```text
│  (reverse)File(/)                                                                                                   │
├──╮                                                                                                                │
│   ╭────────────────────────╮                                                                                      │
│   │ (reverse)New                Ctrl+N(/)│                                                                                      │
│   │ Open               Ctrl+O│                                                                                      │
│   │ (gray)Save                      (/)│                                                                                      │
│   │ Quit               Ctrl+Q│                                                                                      │
│   ╰────────────────────────╯                                                                                      │
```

- `Save` rendered in **light gray** with **no shortcut label** (per [D12](../artifacts/decision-log.md#d12-greyed-menu-item-rendering-and-disable-rules))
- Cursor can navigate over the greyed item but Enter on it is a no-op
- Ctrl+S keypress is also inert globally while in browse-only

## Behavior summary (visual)

The browse-only frame is structurally identical to the standard edit view; the differences are all *what's missing* or *what's marked unavailable*: no dirty indicator, no findings summary, greyed Save in menu, and the prominent `[ro]` banner. This matches the behavioral spec's commitment that "layout is identical to normal edit view" while the read-only state is communicated through three independent signals.

## Cross-references

- Behavioral spec: [Alternate Flows — Read-only target](../../workflow-builder/feature-specification.md#read-only-target-any-source), [D4](../../workflow-builder/artifacts/decision-log.md#d4-read-only-target--load-time-detection), [D30](../../workflow-builder/artifacts/decision-log.md#d30-read-only-targets-open-in-browse-only-mode).
- Visual decisions: [D12](../artifacts/decision-log.md#d12-greyed-menu-item-rendering-and-disable-rules), [D14](../artifacts/decision-log.md#d14-banner-prefix-glyphs-and-severity-coloring).
- Team findings: [F4](../artifacts/team-findings.md#f4-browse-only-footers-red-save-disabled-phrase-conflicts-with-colorshortcutline-tokenizer), [F14](../artifacts/team-findings.md#f14-browse-only-full-frame-mockup-missing).
