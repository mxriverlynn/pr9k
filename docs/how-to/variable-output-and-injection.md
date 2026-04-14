# Variable Output and Injection

This guide explains how data flows between workflow steps in ralph-tui: how variables are injected into prompts and commands, how step output is captured, and how steps pass data to each other through files.

## {{VAR}} Substitution Engine

All `{{VAR_NAME}}` tokens in prompt files and shell command arguments are expanded at runtime using the `vars.Substitute` function. Substitution is applied by:

- `steps.BuildPrompt()` — replaces tokens in prompt file content before passing the string to Claude
- `workflow.ResolveCommand()` — replaces tokens in each element of a shell command's argv

**Where values come from:**

The `VarTable` is created at the start of `Run` and carries two categories of variables:

- **Built-in variables** — seeded from CLI flags and updated by the orchestrator:

  | Variable | Value |
  |----------|-------|
  | `WORKFLOW_DIR` | Resolved workflow (install) directory path |
  | `PROJECT_DIR` | Resolved project (target repo) directory path |
  | `MAX_ITER` | Value of `--iterations` flag (0 = unbounded) |
  | `ITER` | Current iteration number (1-based) |
  | `STEP_NUM` | Current step number within the phase |
  | `STEP_COUNT` | Total steps in the phase |
  | `STEP_NAME` | Display name of the current step |

> **Sandbox constraint — `{{WORKFLOW_DIR}}` and `{{PROJECT_DIR}}` are valid only in `command` steps.**
> Both tokens expand to host filesystem paths. Inside the Docker sandbox, those paths do not exist:
> `WORKFLOW_DIR` points to the workflow bundle, which is deliberately not bind-mounted; `PROJECT_DIR`
> points to the target repo, which is bind-mounted at `/home/agent/workspace` (not at the host path).
> A prompt file that embeds either token passes a broken path to claude. The config validator
> (Rule B) rejects both tokens in any prompt file referenced by a claude step. Shell command steps,
> which run on the host and see host paths, may use both tokens freely.

- **Iteration-scoped variables** — bound by the orchestrator at the start of each iteration and cleared at the start of the next:

  | Variable | Value |
  |----------|-------|
  | `ISSUE_ID` | Current GitHub issue number |
  | `STARTING_SHA` | HEAD commit SHA at the start of the iteration |

The SHA is not refreshed on retry — a retried step uses the same `STARTING_SHA` from when the iteration started.

**Resolution order:** During iteration steps, `VarTable` checks the iteration table first, then the persistent table. During finalize steps, only the persistent table is visible.

**Unresolved variables** log a warning and substitute the empty string.

### Example: prompt file with substitution

```
@progress.txt
1. Implement github issue {{ISSUE_ID}} in the current branch
2. Commit changes in a single commit
```

At runtime with issue `42`, the token `{{ISSUE_ID}}` is replaced before the prompt is passed to Claude:

```
@progress.txt
1. Implement github issue 42 in the current branch
2. Commit changes in a single commit
```

### Example: shell command with substitution

Config:
```json
{"name": "Close issue", "isClaude": false, "command": ["scripts/close_gh_issue", "{{ISSUE_ID}}"]}
```

At runtime with issue `42`, this resolves to:
```
["{workflowDir}/scripts/close_gh_issue", "42"]
```

Note that the relative script path `scripts/close_gh_issue` is also resolved to an absolute path against the workflow directory. Bare commands like `git` are not modified.

### `{{WORKFLOW_DIR}}` and `{{PROJECT_DIR}}` post-split semantics

After the 0.3.0 split (`docs/adr/20260413162428-workflow-project-dir-split.md`), each token has a distinct, unambiguous meaning:

| Token | Expands to | Default source | Override flag |
|-------|-----------|----------------|---------------|
| `{{WORKFLOW_DIR}}` | The workflow bundle directory — where `ralph-steps.json`, `prompts/`, `scripts/`, and `ralph-art.txt` live | `os.Executable()` + `filepath.EvalSymlinks` | `--workflow-dir` |
| `{{PROJECT_DIR}}` | The target repository — the git repo being modified by the workflow | `os.Getwd()` + `filepath.EvalSymlinks` | `--project-dir` |

In the default pr9k install both directories often share a parent (the binary lives under `bin/` inside the pr9k repo), but they are distinct concepts and may point to entirely different locations when ralph-tui is used from `PATH` or with explicit flags.

Use `{{WORKFLOW_DIR}}` to reach workflow artifacts: `{{WORKFLOW_DIR}}/ralph-art.txt` (the default Splash step), `scripts/get_gh_user` (resolved against `workflowDir` by `ResolveCommand`). Use `{{PROJECT_DIR}}` when a shell command step needs to reference the target repo root explicitly (e.g., running a repo-specific tool). Do not use either token in prompt files (see sandbox constraint above).

### Finalization steps

Finalization steps run after all iterations complete. Iteration-scoped variables (`ISSUE_ID`, `STARTING_SHA`) are not visible during the finalize phase — using them in a finalize step will log a warning and substitute the empty string. Built-in variables (`WORKFLOW_DIR`, `PROJECT_DIR`, `MAX_ITER`, `ITER`, etc.) remain available.

### Escape sequences

To include a literal `{{` or `}}` in prompt content, use `{{{{` (produces `{{`) or `}}}}` (produces `}}`).

## Metadata Capture (CaptureOutput)

The orchestrator uses `CaptureOutput()` to run commands and capture their stdout as single values. This is used exclusively for workflow metadata — not for passing data between steps.

**What gets captured:**

| Script | Output | Used For |
|--------|--------|----------|
| `scripts/get_gh_user` | GitHub username | Passed to `get_next_issue` to filter issues |
| `scripts/get_next_issue <username>` | Issue number (or empty) | Bound as `{{ISSUE_ID}}` for the iteration |
| `git rev-parse HEAD` | Commit SHA | Bound as `{{STARTING_SHA}}` for the iteration |

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
| `code-review.md` | Code review step | Fix review items step (`@code-review.md`) | Deleted by fix review items step |

The consuming step checks whether the file exists and has content. If the file is empty or missing, the step skips to cleanup.

### Data Flow Diagram

```
Iteration N:
  Feature work ──writes──▶ progress.txt, deferred.txt
  Test planning ──writes──▶ test-plan.md, progress.txt
  Test writing ──reads───▶ test-plan.md ──deletes──▶ test-plan.md
  Code review ──writes──▶ code-review.md, progress.txt
  Fix review items ──reads───▶ code-review.md ──deletes──▶ code-review.md
  Update docs ──reads───▶ progress.txt

Finalization:
  Deferred work ──reads───▶ deferred.txt ──clears──▶ deferred.txt
  Lessons learned ──reads───▶ progress.txt ──clears──▶ progress.txt
```

## Adding Variables to a Custom Workflow

To inject iteration context into a custom Claude prompt:

1. Use `{{ISSUE_ID}}` and `{{STARTING_SHA}}` directly in the prompt file text — they are substituted at runtime

To use iteration variables in a custom shell command:

1. Use `{{ISSUE_ID}}` or any other `{{VAR_NAME}}` in the command array: `["my-script", "{{ISSUE_ID}}"]`

To pass data between custom steps:

1. Have the producing step write to a file (instruct Claude in the prompt)
2. Have the consuming step reference that file with `@filename` in its prompt
3. Have the consuming step delete the file when done

## Related Documentation

- [Getting Started](getting-started.md) — Install, first run, and orientation
- [Building Custom Workflows](building-custom-workflows.md) — Step configuration format and workflow structure
- [Capturing Step Output](capturing-step-output.md) — The other direction: binding step stdout to a variable with `captureAs`
- [Breaking Out of the Loop](breaking-out-of-the-loop.md) — Using `breakLoopIfEmpty` with capture steps
- [Debugging a Run](debugging-a-run.md) — Reading capture logs in the log file to trace variable flow
- [Variable State Management](../features/variable-state.md) — VarTable scopes, phase transitions, and `CaptureAs` binding
- [Step Definitions & Prompt Building](../features/step-definitions.md) — Implementation details of `LoadSteps` and `BuildPrompt`
- [Subprocess Execution](../features/subprocess-execution.md) — `ResolveCommand`, `CaptureOutput`, and `RunStep` implementation
- [Workflow Orchestration](../features/workflow-orchestration.md) — How the Run loop captures metadata and builds resolved steps
