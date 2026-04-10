# Variable Output and Injection

This guide explains how data flows between workflow steps in ralph-tui: how variables are injected into prompts and commands, how step output is captured, and how steps pass data to each other through files.

## Variable Injection into Prompts

Iteration context variables (`ISSUENUMBER`, `STARTINGSHA`) are injected by including them directly at the top of each prompt file that needs them. Claude reads the variable assignments and substitutes them wherever the prompt references those names.

**Where the values come from:**

- **ISSUENUMBER** — The current GitHub issue number, fetched at the start of each iteration by running `scripts/get_next_issue <username>`
- **STARTINGSHA** — The HEAD commit SHA at the start of each iteration, captured via `git rev-parse HEAD`

These values are captured once per iteration and reused for all steps in that iteration. The SHA is not refreshed on retry — if a step is retried, it uses the same SHA from when the iteration started.

**Implementation:** `steps.BuildPrompt()` in `internal/steps/steps.go` reads the prompt file from `prompts/<promptFile>` and returns its raw content unchanged. Variable injection is the responsibility of the prompt file itself or the orchestrator level.

> **Upcoming:** A general-purpose `{{VAR}}` substitution engine (issue #39) will expand named variables in both prompt content and shell command arguments. This will replace the current convention of hardcoding variable lines in prompt files.

### Example

A prompt file `prompts/feature-work.md` that provides its own context:

```
ISSUENUMBER=<issue-id>
STARTINGSHA=<starting-sha>
@progress.txt
1. Implement github issue ISSUENUMBER in the current branch
2. Commit changes in a single commit
```

The prompt text uses `ISSUENUMBER` as a literal reference that Claude reads and understands — it is not a template substitution. Claude sees the `ISSUENUMBER=42` line and uses `42` wherever the prompt mentions `ISSUENUMBER`.

## Template Substitution in Commands ({{ISSUE_ID}})

Shell command steps can use the `{{ISSUE_ID}}` placeholder in their `command` arrays. At runtime, `ResolveCommand()` replaces all occurrences of `{{ISSUE_ID}}` with the actual issue number in every element of the command array.

**Implementation:** `workflow.ResolveCommand()` in `internal/workflow/workflow.go`.

### Example

Config:
```json
{"name": "Close issue", "isClaude": false, "command": ["scripts/close_gh_issue", "{{ISSUE_ID}}"]}
```

At runtime with issue `42`, this resolves to:
```
["{projectDir}/scripts/close_gh_issue", "42"]
```

Note that the relative script path `scripts/close_gh_issue` is also resolved to an absolute path against the project directory. Bare commands like `git` are not modified.

### Finalization steps

Finalization steps do not have a current issue. If a finalization command uses `{{ISSUE_ID}}`, it resolves to an empty string. Finalization steps should generally not use `{{ISSUE_ID}}`.

## Metadata Capture (CaptureOutput)

The orchestrator uses `CaptureOutput()` to run commands and capture their stdout as single values. This is used exclusively for workflow metadata — not for passing data between steps.

**What gets captured:**

| Script | Output | Used For |
|--------|--------|----------|
| `scripts/get_gh_user` | GitHub username | Passed to `get_next_issue` to filter issues |
| `scripts/get_next_issue <username>` | Issue number (or empty) | Used as `ISSUENUMBER` / `{{ISSUE_ID}}` for the iteration |
| `git rev-parse HEAD` | Commit SHA | Used as `STARTINGSHA` for the iteration |

An empty issue number signals that no more issues are available, and the iteration loop exits early.

**Implementation:** `workflow.Runner.CaptureOutput()` in `internal/workflow/workflow.go` runs the command and returns trimmed stdout.

## File-Based Data Passing Between Steps

Steps do not pass data directly to each other through variables or command output. Instead, they communicate through files in the working directory. Claude prompts reference these files using the `@filename` syntax, which tells the Claude CLI to include the file's contents as context.

### Accumulated Files

These files grow across all iterations:

| File | Written By | Read By | Committed |
|------|-----------|---------|-----------|
| `progress.txt` | All Claude steps (append) | Multiple steps via `@progress.txt`; `lessons-learned` finalization step | Never |
| `deferred.txt` | All Claude steps (append) | `deferred-work` finalization step via `@deferred.txt` | Never |

Both files are cleared (contents deleted, file left in place) by their respective finalization steps.

### Handoff Files

These files are created by one step and consumed by a later step within the same iteration:

| File | Created By | Consumed By | Lifecycle |
|------|-----------|-------------|-----------|
| `test-plan.md` | Test planning step | Test writing step (`@test-plan.md`) | Deleted by test writing step |
| `code-review.md` | Code review step | Review fixes step (`@code-review.md`) | Deleted by review fixes step |

The consuming step checks whether the file exists and has content. If the file is empty or missing, the step skips to cleanup.

### Data Flow Diagram

```
Iteration N:
  Feature work ──writes──▶ progress.txt, deferred.txt
  Test planning ──writes──▶ test-plan.md, progress.txt
  Test writing ──reads───▶ test-plan.md ──deletes──▶ test-plan.md
  Code review ──writes──▶ code-review.md, progress.txt
  Review fixes ──reads───▶ code-review.md ──deletes──▶ code-review.md
  Update docs ──reads───▶ progress.txt

Finalization:
  Deferred work ──reads───▶ deferred.txt ──clears──▶ deferred.txt
  Lessons learned ──reads───▶ progress.txt ──clears──▶ progress.txt
```

## Adding Variables to a Custom Workflow

To inject issue and SHA context into a custom Claude step:

1. Include `ISSUENUMBER=<issue-id>` and `STARTINGSHA=<starting-sha>` lines at the top of your prompt file
2. Reference `ISSUENUMBER` and `STARTINGSHA` by name in the rest of the prompt text

To use the issue number in a custom shell command:

1. Use `{{ISSUE_ID}}` in the command array: `["my-script", "{{ISSUE_ID}}"]`

To pass data between custom steps:

1. Have the producing step write to a file (instruct Claude in the prompt)
2. Have the consuming step reference that file with `@filename` in its prompt
3. Have the consuming step delete the file when done

## Related Documentation

- [Building Custom Workflows](building-custom-workflows.md) — Step configuration format and workflow structure
- [Step Definitions & Prompt Building](../features/step-definitions.md) — Implementation details of `LoadSteps` and `BuildPrompt`
- [Subprocess Execution](../features/subprocess-execution.md) — `ResolveCommand`, `CaptureOutput`, and `RunStep` implementation
- [Workflow Orchestration](../features/workflow-orchestration.md) — How the Run loop captures metadata and builds resolved steps
