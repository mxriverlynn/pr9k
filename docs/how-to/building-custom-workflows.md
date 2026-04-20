# Building Custom Workflows

This guide explains how to create and modify workflow step sequences in pr9k. Steps are defined in JSON configuration files and can mix Claude CLI invocations with shell commands.

## Step Configuration Files

pr9k loads step definitions from `config.json` (resolved relative to the workflow directory). This file contains three groups:

- **`initialize`** — Steps run once before the iteration loop begins
- **`iteration`** — Steps run once per issue
- **`finalize`** — Steps run once after all iterations complete

Steps execute in the order they appear in each array.

An optional top-level **`statusLine`** object can also be added to configure a status-line command displayed by the TUI (see [Config Validation — statusLine block](../code-packages/validator.md#statusline-block-category-statusline) for the schema). The runner's output appears in the TUI footer during Normal mode; press `?` to open the help modal. See [Status Line](../features/status-line.md) for runtime behavior, stdin payload schema, and writing custom status-line scripts.

## Step Schema

Each step object has the following fields:

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| `name` | string | yes | Display name shown in the TUI header and log output |
| `isClaude` | bool | yes | `true` for Claude CLI steps, `false` for shell commands |
| `model` | string | Claude steps | Claude model to use (`"sonnet"`, `"opus"`) |
| `promptFile` | string | Claude steps | Filename in `prompts/` directory (e.g., `"feature-work.md"`) |
| `command` | string[] | Shell steps | Command argv (e.g., `["git", "push"]`) |
| `captureAs` | string | optional | Store the step's stdout under this variable name for use in later steps |
| `captureMode` | string | optional | `"lastLine"` (default) or `"fullStdout"` — controls how stdout is bound when `captureAs` is set. Only valid on non-claude steps. See [Capturing Step Output](capturing-step-output.md). |
| `breakLoopIfEmpty` | bool | optional | Exit the iteration loop when the captured output for this step is empty |
| `skipIfCaptureEmpty` | string | optional | Skip this step when the named capture variable is empty |
| `timeoutSeconds` | int | optional | Cap wall-clock time for this step; SIGTERM then SIGKILL if exceeded |
| `resumePrevious` | bool | optional | (Claude steps only) Attempt to resume the previous claude step's session via `--resume <session_id>`. Five runtime gates (G1–G5) must all pass; any failure falls through to a fresh session without aborting. See [Session Resume Gates](../features/workflow-orchestration.md#session-resume-gates-resumeprevious). **Default workflow ships with this unset on all steps** — engine support is present but feature-flagged-off. |
| `env` | string[] | optional | Additional host environment variable names to pass through to the sandbox container (see [Config Validation](../code-packages/validator.md) for allowed names) |

## Claude Steps

A Claude step invokes the `claude` CLI with a prompt file. At runtime, the orchestrator builds the full command:

```
claude --permission-mode bypassPermissions --model <model> -p <prompt-content>
```

The prompt content is read from `prompts/<promptFile>` and all `{{VAR_NAME}}` tokens are substituted at runtime. Use `{{ISSUE_ID}}`, `{{STARTING_SHA}}`, and other built-in variables to inject iteration context (see [Variable Output & Injection](variable-output-and-injection.md) for the full variable list).

### Example: Claude step

```json
{"name": "Feature work", "model": "sonnet", "promptFile": "feature-work.md", "isClaude": true}
```

## Shell Command Steps

A shell command step runs an arbitrary command as a subprocess. The `command` field is an argv array — the first element is the executable, the rest are arguments.

Relative paths containing a `/` separator are resolved against the workflow directory. Bare commands (like `git`) are looked up via `PATH`. Template variables like `{{ISSUE_ID}}` are replaced with actual values at runtime (see [Variable Output & Injection](variable-output-and-injection.md)).

### Example: Shell command steps

```json
{"name": "Close issue", "isClaude": false, "command": ["scripts/close_gh_issue", "{{ISSUE_ID}}"]}
```

```json
{"name": "Git push", "isClaude": false, "command": ["git", "push"]}
```

## Initialize, Iteration, and Finalization Steps

**Initialize steps** (the `"initialize"` array in `config.json`) run once before the iteration loop begins. Use them for setup tasks that must complete before any issue is processed.

**Iteration steps** (the `"iteration"` array in `config.json`) run once per issue. They have access to all built-in and iteration-scoped variables — use `{{ISSUE_ID}}`, `{{STARTING_SHA}}`, `{{ITER}}`, and others in both prompts and shell commands. See [Variable Output & Injection](variable-output-and-injection.md) for the full variable list.

**Finalization steps** (the `"finalize"` array in `config.json`) run once after all iterations complete, even if the iteration loop exits early (e.g., no more issues found). Iteration-scoped variables (`ISSUE_ID`, `STARTING_SHA`) are not visible — using them will substitute the empty string. Built-in variables (`WORKFLOW_DIR`, `PROJECT_DIR`, `MAX_ITER`, `ITER`, etc.) remain available.

## The Default Workflow

The default iteration workflow has 11 steps:

1. **Get next issue** (shell) — Finds the lowest-numbered open GitHub issue labeled "ralph" assigned to the user; exits the loop when none remain
2. **Get starting SHA** (shell) — Records `HEAD` as `{{STARTING_SHA}}` for later diff references
3. **Get issue body** (shell) — Fetches the issue title and body via `gh` and captures them as `{{ISSUE_BODY}}` (fullStdout)
4. **Get project card** (shell) — Runs `scripts/project_card` to probe build-config files and captures a short project summary as `{{PROJECT_CARD}}` (fullStdout)
5. **Feature work** (sonnet) — Implements the GitHub issue
6. **Get post-feature diff** (shell) — Captures `git diff {{STARTING_SHA}}..HEAD --stat` as `{{PRE_REVIEW_DIFF}}` (fullStdout) for use in the test-writing prompt
7. **Test planning** (opus) — Creates a test plan
8. **Test writing** (sonnet) — Writes tests from the plan
9. **Summarize to issue** (shell) — Posts a single end-of-iteration summary comment to the GitHub issue via `scripts/post_issue_summary`
10. **Close issue** (shell) — Closes the GitHub issue via `gh`
11. **Git push** (shell) — Pushes all commits

The default finalization workflow has 7 steps — these run once per pr9k run against the full set of branch changes, not per issue:

1. **Code review** (opus) — Reviews every change on the branch; writes findings (or the `NOTHING-TO-FIX` sentinel) to `code-review.md`
2. **Check review verdict** (shell) — `scripts/review_verdict` reads `code-review.md` and captures `REVIEW_HAS_FIXES` (empty when the sentinel is present)
3. **Fix review items** (sonnet) — Implements the review findings; `skipIfCaptureEmpty: REVIEW_HAS_FIXES` skips this step when the reviewer found nothing
4. **Update docs** (sonnet) — Updates project documentation for the whole branch
5. **Deferred work** (sonnet) — Creates issues from accumulated `deferred.txt`
6. **Lessons learned** (sonnet) — Updates coding standards from `progress.txt`
7. **Final git push** (shell) — Pushes finalization commits

## Creating a Custom Workflow

### 1. Create your prompt files

Add markdown files to the `prompts/` directory. Each file contains the instructions that will be passed to the Claude CLI via `-p`. Prompts can reference local files using the `@filename` syntax (e.g., `@progress.txt`), which tells Claude to include that file's contents as context.

### 2. Define your steps in JSON

Create or modify `config.json`. For example, a minimal workflow:

```json
{
  "initialize": [],
  "iteration": [
    {"name": "Implement", "model": "sonnet", "promptFile": "implement.md", "isClaude": true},
    {"name": "Test", "model": "sonnet", "promptFile": "write-tests.md", "isClaude": true},
    {"name": "Push", "isClaude": false, "command": ["git", "push"]}
  ],
  "finalize": [
    {"name": "Final push", "isClaude": false, "command": ["git", "push"]}
  ]
}
```

### 3. Add custom scripts

Place scripts in the `scripts/` directory. Reference them with a relative path in the `command` array:

```json
{"name": "Deploy", "isClaude": false, "command": ["scripts/deploy", "{{ISSUE_ID}}"]}
```

The orchestrator resolves `scripts/deploy` to `{workflowDir}/scripts/deploy` before execution.

### 4. Build and run

After modifying configs or prompts, rebuild with `make build` to copy everything into `bin/`. Or run directly if building with `go build`.

## Per-Repo Workflow Override

pr9k supports an in-repo workflow override: if `<projectDir>/.pr9k/workflow/` exists and is a directory, pr9k uses it as the workflow directory instead of the shipped bundle inside `bin/.pr9k/workflow/`. This lets individual repos ship their own `config.json`, `prompts/`, and `scripts/` without touching the pr9k install.

### Setup

1. Create the override directory in your target repo:

   ```bash
   mkdir -p .pr9k/workflow/prompts
   mkdir -p .pr9k/workflow/scripts
   ```

2. Add your `config.json` to `.pr9k/workflow/`:

   ```json
   {
     "initialize": [],
     "iteration": [
       {"name": "Implement", "model": "sonnet", "promptFile": "implement.md", "isClaude": true},
       {"name": "Push", "isClaude": false, "command": ["git", "push"]}
     ],
     "finalize": []
   }
   ```

3. Add prompt files to `.pr9k/workflow/prompts/` and scripts to `.pr9k/workflow/scripts/`.

4. Add `.pr9k/` to your `.gitignore` — or commit the override if you want it version-controlled:

   ```bash
   echo '.pr9k/' >> .gitignore
   ```

### Resolution order

When pr9k starts, it checks two candidates in order:

1. `<projectDir>/.pr9k/workflow/` — used if it exists and is a directory (in-repo override)
2. `<executableDir>/.pr9k/workflow/` — the shipped bundle (fallback)

If neither exists, pr9k exits with an error listing both paths. Pass `--workflow-dir <path>` to override both candidates.

### When to use

The in-repo override is useful when:
- Your project needs workflow steps or prompts tailored to its tech stack
- You want to version-control your workflow alongside your code
- You're developing a custom workflow and want quick iteration without rebuilding the pr9k bundle

## TUI Display Constraints

The TUI status header displays steps as a dynamic grid of 4 columns per row, sized at startup to fit the largest phase (initialize, iteration, or finalize). If your iteration phase has 6 steps, the grid has 2 rows; 9 steps gives 3 rows; and so on. Each row's cells are padded to a uniform width so the step list is distributed evenly across the header. If any phase has more steps than the grid was sized to hold (which cannot happen with a correct config — the grid is sized to the maximum across all phases), extra steps will execute but won't appear in the header.

## Error Recovery

When any step fails (non-zero exit code), the TUI enters error mode. The user can:

- **c** — continue to the next step (failed step shows `[✗]`)
- **r** — retry the failed step
- **q** → **y** — quit the workflow

User-initiated skips (pressing **n** during a step) are not treated as failures — the step shows `[✓]` and the workflow advances.

## Related Documentation

- [Getting Started](getting-started.md) — Install, first run, and orientation
- [Variable Output & Injection](variable-output-and-injection.md) — How `{{VAR}}` tokens are resolved into prompts and commands
- [Capturing Step Output](capturing-step-output.md) — How to use `captureAs` to bind step stdout to a variable
- [Passing Environment Variables](passing-environment-variables.md) — How to forward host env vars into the Docker sandbox via the `env` field
- [Breaking Out of the Loop](breaking-out-of-the-loop.md) — Using `breakLoopIfEmpty` to exit the iteration loop dynamically
- [Recovering from Step Failures](recovering-from-step-failures.md) — Error mode keyboard controls and decision-making
- [Debugging a Run](debugging-a-run.md) — Reading logs and reproducing failures
- [Narrow-Reading Principle ADR](../adr/20260410170952-narrow-reading-principle.md) — The architectural decision that workflow content belongs in `config.json`, not Go code; includes documented exceptions
- [Step Definitions & Prompt Building](../code-packages/steps.md) — Implementation details of step loading and prompt construction
- [Workflow Orchestration](../features/workflow-orchestration.md) — The Run loop and Orchestrate step sequencer
- [Subprocess Execution](../features/subprocess-execution.md) — How steps are executed as subprocesses
