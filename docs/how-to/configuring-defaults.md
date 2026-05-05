# Configuring Workflow Defaults

The top-level `defaults` block in `config.json` sets workflow-wide values that any individual step can override. This guide is the canonical reference for what fields the block supports, how the override hierarchy works, and what the validator enforces.

← [Back to How-To Guides](README.md)

**Prerequisites**: a working install — see [Getting Started](getting-started.md) — and familiarity with the step schema in [Building Custom Workflows](building-custom-workflows.md).

## Why use a defaults block

A workflow with five Claude steps that all use the same `model` and the same `effort` is verbose: every step duplicates the same two values. The defaults block lets you set each value once at the top of `config.json` and only override on the steps that need something different.

Today the block has two fields — `effort` and `model` — but it is structured as a block so future workflow-wide settings can join it without another schema bump. Both fields apply only to Claude steps; shell steps ignore them.

## Field summary

| Field | Type | Applied to | What it sets |
|-------|------|------------|--------------|
| `effort` | string | Claude steps without their own `effort` | Forwarded to the Claude CLI as `--effort <value>`. See [Setting Claude Effort](setting-claude-effort.md). |
| `model` | string | Claude steps without their own `model` | Forwarded to the Claude CLI as `--model <value>`. The exact string (e.g. `"sonnet"`, `"opus"`) is passed through verbatim. |

Both keys are optional. Omitting the entire `defaults` block is equivalent to setting neither.

## Override hierarchy

For every Claude step, pr9k computes an **effective value** for each defaults-aware field by checking, in order:

1. The step's own field, if set.
2. The corresponding `defaults` field, if set.
3. Otherwise, no value (see the per-field rules below for what "no value" means).

Resolution happens once, at workflow load time. Once `config.json` is loaded, every Claude step's `Effort` and `Model` already represent the effective value — there is no mid-run re-evaluation.

### Effort resolution

| Step `effort` | `defaults.effort` | Effective effort | CLI flag |
|---------------|-------------------|------------------|----------|
| `"high"` | unset | `high` | `--effort high` |
| `"high"` | `"medium"` | `high` | `--effort high` |
| unset | `"medium"` | `medium` | `--effort medium` |
| unset | unset | none | *(no flag)* |

When neither is set, pr9k passes no `--effort` flag and the CLI's own default applies.

### Model resolution

| Step `model` | `defaults.model` | Effective model |
|--------------|-------------------|-----------------|
| `"sonnet"` | unset | `sonnet` |
| `"sonnet"` | `"opus"` | `sonnet` |
| unset | `"opus"` | `opus` |
| unset | unset | **fatal validator error** on every Claude step that hits this row |

Unlike `effort`, `model` is *required* for every Claude step. The defaults block lets the requirement be satisfied once for the whole workflow rather than on every step, but somebody — either the step or the defaults — must supply a value.

## Configuration examples

### Just defaults — every Claude step inherits

```json
{
  "defaults": {
    "model": "sonnet",
    "effort": "medium"
  },
  "initialize": [],
  "iteration": [
    {
      "name": "Feature work",
      "isClaude": true,
      "promptFile": "feature-work.md"
    },
    {
      "name": "Test writing",
      "isClaude": true,
      "promptFile": "test-writing.md"
    }
  ],
  "finalize": []
}
```

Both Claude steps run with `--model sonnet --effort medium`. Notice that neither step has its own `model` or `effort` field — they inherit from `defaults`.

### Mixed — a default with a per-step override

```json
{
  "defaults": {
    "model": "sonnet",
    "effort": "medium"
  },
  "initialize": [],
  "iteration": [
    {
      "name": "Feature work",
      "isClaude": true,
      "promptFile": "feature-work.md"
    },
    {
      "name": "Code review",
      "isClaude": true,
      "model": "opus",
      "promptFile": "code-review.md",
      "effort": "high"
    }
  ],
  "finalize": []
}
```

- "Feature work" inherits both fields → `--model sonnet --effort medium`.
- "Code review" overrides both → `--model opus --effort high`.

### Partial defaults — set one, leave the other per-step

```json
{
  "defaults": {
    "model": "sonnet"
  },
  "iteration": [
    {
      "name": "Feature work",
      "isClaude": true,
      "promptFile": "feature-work.md"
    },
    {
      "name": "Test writing",
      "isClaude": true,
      "promptFile": "test-writing.md",
      "effort": "high"
    }
  ]
}
```

- Both steps inherit `model: "sonnet"` from defaults.
- "Feature work" passes no `--effort` (neither set).
- "Test writing" uses its own `effort: "high"` → `--effort high`.

## Validator constraints

| Field | Constraint |
|-------|------------|
| `defaults.effort` | Must be one of `"low"`, `"medium"`, `"high"`, `"xhigh"`, `"max"` when present. Same value set as the per-step `effort` field. |
| `defaults.model` | Any non-empty string is accepted. The CLI receives the value verbatim. |
| Unknown defaults keys | Rejected by strict-decode (e.g. `"defaults": { "modal": "..." }` fails with an "unknown field" error). |
| Per-step `effort` on shell step | Fatal error — both with and without `defaults`. The flag is meaningless for shell commands. |
| Claude step with no effective model | Fatal error. Either set `step.model` or set `defaults.model`. |

See [Config Validation](../code-packages/validator.md) for the full validator contract.

## Workflow Builder support

The interactive workflow builder (`pr9k workflow`) round-trips the entire `defaults` block when you load and save a `config.json`. Both `defaults.effort` and `defaults.model` are preserved verbatim through the parse/marshal cycle, even though the builder UI does not yet expose dedicated form fields for them. Editing the defaults block today means editing `config.json` directly. (See [Using the Workflow Builder](using-the-workflow-builder.md) for what the builder does expose.)

## Related documentation

- ← [Back to How-To Guides](README.md)
- [Building Custom Workflows](building-custom-workflows.md) — full step schema, including the per-step `model` and `effort` fields
- [Setting Claude Effort](setting-claude-effort.md) — deep dive on the `effort` field, including the meaning of each value
- [Config Validation](../code-packages/validator.md) — the validator rules that enforce valid values and the claude-step model requirement
- [Docker Sandbox](../features/docker-sandbox.md) — where `--model` and `--effort` land inside the runtime `docker run` command
- [Step Definitions & Prompt Building](../code-packages/steps.md) — the `Step.Model` / `Step.Effort` fields, the `Defaults` struct, and load-time fallback resolution
