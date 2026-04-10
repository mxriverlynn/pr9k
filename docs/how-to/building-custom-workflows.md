# Building Custom Workflows

This guide explains how to create and modify workflow step sequences in ralph-tui. Steps are defined in JSON configuration files and can mix Claude CLI invocations with shell commands.

## Step Configuration Files

Ralph-tui loads step definitions from `ralph-steps.json` (resolved relative to the project directory). This file contains three groups:

- **`initialize`** — Steps run once before the iteration loop begins
- **`iteration`** — Steps run once per issue
- **`finalize`** — Steps run once after all iterations complete

Steps execute in the order they appear in each array.

## Step Schema

Each step object has the following fields:

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| `name` | string | yes | Display name shown in the TUI header and log output |
| `isClaude` | bool | yes | `true` for Claude CLI steps, `false` for shell commands |
| `model` | string | Claude steps | Claude model to use (`"sonnet"`, `"opus"`) |
| `promptFile` | string | Claude steps | Filename in `prompts/` directory (e.g., `"feature-work.md"`) |
| `command` | string[] | Shell steps | Command argv (e.g., `["git", "push"]`) |
| `captureAs` | string | optional | Store the step's stdout under this variable name for use in later steps (see issue #39) |
| `breakLoopIfEmpty` | bool | optional | Exit the iteration loop when the captured output for this step is empty (see issue #39) |

## Claude Steps

A Claude step invokes the `claude` CLI with a prompt file. At runtime, the orchestrator builds the full command:

```
claude --permission-mode acceptEdits --model <model> -p <prompt-content>
```

The prompt content is read verbatim from `prompts/<promptFile>`. To provide iteration context (issue number, starting SHA), include those values directly in the prompt file or reference them via the upcoming `{{VAR}}` substitution engine (see [Variable Output & Injection](variable-output-and-injection.md) for details).

### Example: Claude step

```json
{"name": "Feature work", "model": "sonnet", "promptFile": "feature-work.md", "isClaude": true}
```

## Shell Command Steps

A shell command step runs an arbitrary command as a subprocess. The `command` field is an argv array — the first element is the executable, the rest are arguments.

Relative paths containing a `/` separator are resolved against the project directory. Bare commands (like `git`) are looked up via `PATH`. Template variables like `{{ISSUE_ID}}` are replaced with actual values at runtime (see [Variable Output & Injection](variable-output-and-injection.md)).

### Example: Shell command steps

```json
{"name": "Close issue", "isClaude": false, "command": ["scripts/close_gh_issue", "{{ISSUE_ID}}"]}
```

```json
{"name": "Git push", "isClaude": false, "command": ["git", "push"]}
```

## Initialize, Iteration, and Finalization Steps

**Initialize steps** (the `"initialize"` array in `ralph-steps.json`) run once before the iteration loop begins. Use them for setup tasks that must complete before any issue is processed.

**Iteration steps** (the `"iteration"` array in `ralph-steps.json`) run once per issue. They have access to the current issue number and starting SHA — shell steps can use `{{ISSUE_ID}}` for the issue number; prompt-based variable injection is described in [Variable Output & Injection](variable-output-and-injection.md).

**Finalization steps** (the `"finalize"` array in `ralph-steps.json`) run once after all iterations complete, even if the iteration loop exits early (e.g., no more issues found). They do not have access to issue-specific variables — `{{ISSUE_ID}}` will resolve to an empty string.

## The Default Workflow

The default iteration workflow has 8 steps:

1. **Feature work** (sonnet) — Implements the GitHub issue
2. **Test planning** (opus) — Creates a test plan
3. **Test writing** (sonnet) — Writes tests from the plan
4. **Code review** (opus) — Reviews changes since the starting SHA
5. **Review fixes** (sonnet) — Implements review findings
6. **Close issue** (shell) — Closes the GitHub issue via `gh`
7. **Update docs** (sonnet) — Updates project documentation
8. **Git push** (shell) — Pushes all commits

The default finalization workflow has 3 steps:

1. **Deferred work** (sonnet) — Creates issues from accumulated `deferred.txt`
2. **Lessons learned** (sonnet) — Updates coding standards from `progress.txt`
3. **Final git push** (shell) — Pushes finalization commits

## Creating a Custom Workflow

### 1. Create your prompt files

Add markdown files to the `prompts/` directory. Each file contains the instructions that will be passed to the Claude CLI via `-p`. Prompts can reference local files using the `@filename` syntax (e.g., `@progress.txt`), which tells Claude to include that file's contents as context.

### 2. Define your steps in JSON

Create or modify `ralph-steps.json`. For example, a minimal workflow:

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

The orchestrator resolves `scripts/deploy` to `{projectDir}/scripts/deploy` before execution.

### 4. Build and run

After modifying configs or prompts, rebuild with `make build` to copy everything into `bin/`. Or run directly if building with `go build`.

## TUI Display Constraints

The TUI status header displays steps as two rows of 4 checkboxes (8 slots total). If your workflow has more than 8 iteration steps, the extra steps will execute but won't appear in the header. The same 8-slot layout applies to finalization steps.

## Error Recovery

When any step fails (non-zero exit code), the TUI enters error mode. The user can:

- **c** — continue to the next step (failed step shows `[✗]`)
- **r** — retry the failed step
- **q** → **y** — quit the workflow

User-initiated skips (pressing **n** during a step) are not treated as failures — the step shows `[✓]` and the workflow advances.

## Related Documentation

- [Step Definitions & Prompt Building](../features/step-definitions.md) — Implementation details of step loading and prompt construction
- [Variable Output & Injection](variable-output-and-injection.md) — How variables are injected into prompts and commands
- [Workflow Orchestration](../features/workflow-orchestration.md) — The Run loop and Orchestrate step sequencer
- [Subprocess Execution](../features/subprocess-execution.md) — How steps are executed as subprocesses
