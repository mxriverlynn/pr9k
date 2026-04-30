# Workflow Variables

This guide explains how to use `{{VAR}}` tokens in your prompts and shell commands, which built-in variables pr9k provides, and how steps pass data to each other through files.

← [Back to How-To Guides](README.md)

**Prerequisites**: a working install and at least one successful run — see [Getting Started](getting-started.md). If you haven't written a custom step yet, [Building Custom Workflows](building-custom-workflows.md) is the natural starting point; this guide is the reference you'll consult while writing one.

## Using `{{VAR}}` tokens

Every `{{VAR_NAME}}` token in a prompt file or in a shell command's argv is replaced at runtime with a value from the iteration context. You write tokens; pr9k substitutes values before the prompt or command leaves the orchestrator.

**In a prompt file** (`prompts/feature-work.md`):

```markdown
You are working on GitHub issue #{{ISSUE_ID}}.

The starting commit was {{STARTING_SHA}}. Read the issue body and implement the change.
```

**In a shell command** (`config.json`):

```json
{
  "name": "Close issue",
  "isClaude": false,
  "command": ["scripts/close_gh_issue", "{{ISSUE_ID}}"]
}
```

At runtime, with issue `42`, the prompt becomes `You are working on GitHub issue #42...` and the command resolves to `["<workflow-dir>/scripts/close_gh_issue", "42"]`. (Relative script paths containing a `/` are resolved against the workflow directory; bare commands like `git` are looked up via `$PATH`.)

If a token references a variable that hasn't been bound, pr9k logs a warning and substitutes the empty string — the command/prompt still runs, but with that token blanked out.

## Built-in variables

pr9k seeds these into every step automatically. They're always available — no `captureAs` needed.

| Variable | Value |
|----------|-------|
| `WORKFLOW_DIR` | Workflow bundle directory (where `config.json` and `prompts/` live) |
| `PROJECT_DIR` | Target repository directory (the working directory pr9k was launched from) |
| `MAX_ITER` | Value of `--iterations` (`0` means unbounded) |
| `ITER` | Current iteration number (1-based) |
| `STEP_NUM` | Current step number within the phase |
| `STEP_COUNT` | Total steps in the phase |
| `STEP_NAME` | Display name of the current step |

> **Sandbox constraint**: `{{WORKFLOW_DIR}}` and `{{PROJECT_DIR}}` may only be used in shell command steps, not in prompt files. Both expand to host filesystem paths, which don't exist inside the Docker sandbox where Claude steps run. The validator rejects either token in any prompt file referenced by a Claude step. Use them freely in `command` arrays — those run on the host.

## Iteration-scoped variables

These are bound at the start of each iteration (or by `captureAs` steps within the iteration) and cleared at the start of the next.

| Variable | Bound by | Visible to |
|----------|----------|------------|
| `ISSUE_ID` | The "Get next issue" step (`captureAs`) | All later steps in the iteration |
| `STARTING_SHA` | The "Get starting SHA" step (`captureAs`) | All later steps in the iteration |
| `ISSUE_BODY` | The "Get issue body" step (`captureAs` + `captureMode: fullStdout`) | All later steps in the iteration |
| `PROJECT_CARD` | The "Get project card" step | All later steps in the iteration |
| `PRE_REVIEW_DIFF` | The "Get post-feature diff" step | All later steps in the iteration |

These names aren't special — they exist because the bundled workflow's steps bind them via `captureAs`. Your custom workflow can bind any name it likes; see [Capturing Step Output](capturing-step-output.md) for the mechanism.

A step retried after a failure sees the same iteration-scoped values as its first attempt — `STARTING_SHA` doesn't refresh on retry.

**Resolution order**: during iteration steps, pr9k checks the iteration table first, then the persistent table. During finalize steps, only the persistent table is visible — using `{{ISSUE_ID}}` in a finalize step substitutes the empty string and logs a warning.

## A worked example

The bundled workflow precomputes three context variables before the first Claude step so each prompt has full context without re-querying GitHub or git:

```json
{
  "name": "Get issue body", "isClaude": false,
  "command": ["gh", "issue", "view", "{{ISSUE_ID}}", "--json", "title,body", "-t", "{{{{.title}}}}\n\n{{{{.body}}}}"],
  "captureAs": "ISSUE_BODY", "captureMode": "fullStdout"
}
```

Two things to notice:

1. `{{ISSUE_ID}}` is pr9k's substitution — it's replaced before `gh` runs.
2. `{{{{.title}}}}` is pr9k's escape syntax. `gh`'s `-t` flag uses Go templates (`{{.title}}`), and pr9k's substitution would otherwise eat the braces. Doubling them up (`{{{{` → `{{`, `}}}}` → `}}`) lets the literal `{{.title}}` survive substitution and reach `gh` intact.

Once captured, the variables are injected into prompt files like any other:

```markdown
# Context
Issue #{{ISSUE_ID}}: {{ISSUE_BODY}}

Project card:
{{PROJECT_CARD}}
```

## File-based handoffs between steps

Steps don't pass data directly to each other through variables — captured values are short, single-purpose strings. For larger data (a full test plan, a code review, accumulated notes), steps write to files in the working directory and later steps read those files back, often via Claude's `@filename` syntax.

### Accumulated files (grow across iterations)

| File | Written by | Read by | Cleared by |
|------|-----------|---------|-----------|
| `progress.txt` | All Claude steps (append) | Multiple steps via `@progress.txt`; the lessons-learned finalize step | Lessons-learned finalize step |
| `deferred.txt` | All Claude steps (append) | The deferred-work finalize step via `@deferred.txt` | Deferred-work finalize step |

### Handoff files (one iteration, one consumer)

| File | Created by | Consumed by | Lifecycle |
|------|-----------|-------------|-----------|
| `test-plan.md` | Test planning step | Test writing step (`@test-plan.md`) | Deleted by test writing step |
| `code-review.md` | Code review step | Fix review items step (`@code-review.md`) | Deleted by fix review items step |

The consuming step checks the file exists and has content. If empty or missing, it skips to cleanup. Leftover handoff files in your working directory often signal that a previous run was interrupted before the consumer ran — see [Debugging a Run](debugging-a-run.md).

### Flow diagram

```
Iteration N:
  Feature work ─writes─▶ progress.txt, deferred.txt
  Test planning ─writes─▶ test-plan.md, progress.txt
  Test writing ─reads── test-plan.md ─deletes─▶ test-plan.md
  Code review ─writes─▶ code-review.md, progress.txt
  Fix review items ─reads── code-review.md ─deletes─▶ code-review.md
  Update docs ─reads── progress.txt

Finalization:
  Deferred work ─reads── deferred.txt ─clears─▶ deferred.txt
  Lessons learned ─reads── progress.txt ─clears─▶ progress.txt
```

### Adding your own handoff

To pass data between two custom steps:

1. Have the producing step write a file (instruct Claude in the prompt, or have a shell step `>` redirect stdout to a path)
2. Have the consuming step reference that file — Claude steps via `@filename`, shell steps by reading the path
3. Have the consuming step delete the file when done so a later interrupted run doesn't see stale data

## Escape sequences

To include a literal `{{` or `}}` in prompt content (rare; mostly relevant when passing through to another templating engine), use `{{{{` (produces `{{`) and `}}}}` (produces `}}`).

## What about `--workflow-dir` and `--project-dir`?

`{{WORKFLOW_DIR}}` and `{{PROJECT_DIR}}` always reflect whatever pr9k resolved at startup — either the defaults (the binary's bundle directory and the current working directory respectively) or the values of `--workflow-dir` and `--project-dir` if you passed them. You rarely need to override either flag in normal use; they exist for unusual layouts (running pr9k from `$PATH` against a target repo not in the current directory, testing a feature branch of pr9k against an installed bundle, etc.). The full split is documented in [the workflow/project-dir-split ADR](../adr/20260413162428-workflow-project-dir-split.md).

## Related documentation

- ← [Back to How-To Guides](README.md)
- [Building Custom Workflows](building-custom-workflows.md) — write a `config.json` that uses these variables
- [Capturing Step Output](capturing-step-output.md) — bind a step's stdout to a new variable name with `captureAs`
- [Breaking Out of the Loop](breaking-out-of-the-loop.md) — `breakLoopIfEmpty` with a capture step
- [Skipping Steps Conditionally](skipping-steps-conditionally.md) — `skipIfCaptureEmpty` consumes a captured value
- [Debugging a Run](debugging-a-run.md) — read `Captured VAR = "..."` lines in the log to trace variable flow
- [Variable State Management](../code-packages/vars.md) — `VarTable` scopes, phase transitions (contributor reference)
