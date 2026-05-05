# Setting Claude Effort

The `effort` field on a Claude step is forwarded to the `claude` CLI as `--effort <value>`. Higher levels let the model spend more reasoning budget on harder steps; lower levels keep cheap steps cheap. The top-level `defaults.effort` block sets a workflow-wide baseline that any individual step can override.

← [Back to How-To Guides](README.md)

**Prerequisites**: a working install — see [Getting Started](getting-started.md) — and familiarity with the step schema in [Building Custom Workflows](building-custom-workflows.md). This page covers the per-step `effort` field and the top-level `defaults.effort` setting.

## When to use it

Use `effort` when the steps in your workflow have meaningfully different reasoning needs:

- A short scripting step ("rewrite this regex") and a deep design step ("refactor the auth layer") should not be billed at the same level.
- An overnight, unattended run is a good candidate for raising effort on the steps that own the bulk of the work, while leaving glue steps at the default.
- An on-call iteration where you want results fast is a good candidate for *lowering* effort on the heaviest step instead of swapping the model.

If the workflow is uniform, set a single `defaults.effort` and stop there. Per-step `effort` is only worth reaching for when one step is the outlier.

## Valid values

| Value | When you might pick it |
|-------|------------------------|
| `low` | Mechanical steps where the model just needs to apply a known recipe |
| `medium` | The middle of the road — a sensible workflow-wide default |
| `high` | Multi-file refactors, tricky test diagnosis, or non-obvious design work |
| `xhigh` | Hard problems where you have already accepted the cost trade-off |
| `max` | Reserved for the rare step where the wall-clock cost is worth any amount of reasoning |

These are the exact strings the `claude` CLI accepts via `--effort`. pr9k passes the value through verbatim — anything else is rejected at config-load time. Omitting the field (and leaving `defaults.effort` unset) means pr9k passes no `--effort` flag at all and the CLI's own default applies.

## Configuration

### Per-step

Add `effort` to any Claude step (`isClaude: true`) in `config.json`:

```json
{
  "name": "Feature work",
  "isClaude": true,
  "model": "sonnet",
  "promptFile": "feature-work.md",
  "effort": "high"
}
```

`effort` is only valid on Claude steps. Setting it on a shell step (`isClaude: false`) is a fatal validator error — the flag has no meaning for a shell command.

### Workflow-wide default

Add a top-level `defaults` block alongside `initialize`, `iteration`, and `finalize`. Today the block has a single key — `effort` — but it is structured as a block so future workflow-wide settings can join it without another schema bump:

```json
{
  "defaults": {
    "effort": "medium"
  },
  "initialize": [],
  "iteration": [
    {
      "name": "Feature work",
      "isClaude": true,
      "model": "sonnet",
      "promptFile": "feature-work.md"
    },
    {
      "name": "Test writing",
      "isClaude": true,
      "model": "sonnet",
      "promptFile": "test-writing.md",
      "effort": "high"
    }
  ],
  "finalize": []
}
```

In the example above:

- "Feature work" inherits `defaults.effort` and runs with `--effort medium`.
- "Test writing" sets its own `effort: "high"` and runs with `--effort high` — the per-step value wins.

## Resolution rules

For each Claude step, pr9k computes an **effective effort** by checking, in order:

1. The step's own `effort` field, if set.
2. The top-level `defaults.effort`, if set.
3. Otherwise, no effort.

The effective effort is what gets passed to the CLI:

| Step `effort` | `defaults.effort` | Effective effort | CLI flag |
|---------------|-------------------|------------------|----------|
| `"high"` | unset | `high` | `--effort high` |
| `"high"` | `"medium"` | `high` | `--effort high` |
| unset | `"medium"` | `medium` | `--effort medium` |
| unset | unset | none | *(no flag)* |
| `""` *(invalid — fatal)* | — | — | — |

Resolution happens once, at workflow load time. There is no mid-run re-evaluation; if you want to change the effort for a step, edit `config.json` and rerun pr9k.

## Validator constraints

- `effort` must be one of `"low"`, `"medium"`, `"high"`, `"xhigh"`, `"max"`. Anything else (including `""`) is a fatal error.
- `effort` is only valid on Claude steps. Setting it on a shell step is fatal.
- `defaults.effort` follows the same value rules. The validator rejects unknown values in the same way.
- Unknown top-level keys remain rejected by the strict-decode rule, so a typo like `"default": { ... }` (singular) will fail with an "unknown field" error rather than silently being ignored.

See [Config Validation](../code-packages/validator.md) for the full validator contract.

## What you'll see in the TUI

`--effort` is part of the docker argv that pr9k constructs, so it shows up in the persisted log under `.pr9k/logs/` as part of the command line for each Claude step. There is no extra TUI affordance — the step runs like any other Claude step. If you want to confirm a particular value reached the CLI, search the per-step log for `--effort`.

## Workflow Builder support

The interactive workflow builder (`pr9k workflow`) round-trips the `effort` and `defaults.effort` fields when you load and save a `config.json` — they are preserved verbatim through the parse/marshal cycle, even though the builder UI does not yet expose dedicated form fields for them. Editing those fields today means editing `config.json` directly. (See [Using the Workflow Builder](using-the-workflow-builder.md) for what the builder does expose.)

## Related documentation

- ← [Back to How-To Guides](README.md)
- [Building Custom Workflows](building-custom-workflows.md) — full step schema, including the `effort` row and the top-level `defaults` block
- [Config Validation](../code-packages/validator.md) — the validator rules that enforce valid effort values and claude-only constraints
- [Docker Sandbox](../features/docker-sandbox.md) — where `--effort` lands inside the runtime `docker run` command
- [Step Definitions & Prompt Building](../code-packages/steps.md) — the `Step.Effort` and `Defaults.Effort` fields and load-time validation
