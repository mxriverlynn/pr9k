# Building Custom Workflows

This guide walks you from the smallest possible custom workflow up through the full step schema. By the end you'll know how to write your own `config.json`, where to put prompt files and scripts, and which optional fields exist for the advanced cases.

← [Back to How-To Guides](README.md)

**Prerequisites**: A working install with at least one successful run of the bundled workflow — see [Getting Started](getting-started.md). If you haven't run pr9k yet, do that first; the schema in this guide will make a lot more sense once you've watched a workflow execute.

## The smallest possible workflow

Three iteration steps and one finalize step. This runs once per `ralph`-labeled issue (Claude implements, then `git push`), and pushes one more time when the loop ends:

```json
{
  "initialize": [],
  "iteration": [
    {
      "name": "Get next issue",
      "isClaude": false,
      "command": ["scripts/get_next_issue"],
      "captureAs": "ISSUE_ID",
      "breakLoopIfEmpty": true
    },
    {
      "name": "Implement",
      "isClaude": true,
      "model": "sonnet",
      "promptFile": "implement.md"
    },
    {
      "name": "Push",
      "isClaude": false,
      "command": ["git", "push"]
    }
  ],
  "finalize": []
}
```

The matching `prompts/implement.md` is just a markdown file — Claude reads its content as the prompt. `{{VAR}}` tokens in the file are substituted at runtime with values from the iteration context (see [Workflow Variables](workflow-variables.md)):

```markdown
You are working on GitHub issue #{{ISSUE_ID}}. Read the issue body, implement the change, write the code, and commit it.
```

That's it. Drop those two files into `<your-target-repo>/.pr9k/workflow/` (with `prompts/` for the `.md` file and `scripts/get_next_issue` from the bundled workflow), and pr9k picks them up automatically — no rebuild needed. See [Where the workflow lives](#where-the-workflow-lives) below.

## How a workflow is structured

`config.json` has three top-level arrays plus an optional `statusLine` block:

| Array | When it runs | Typical use |
|-------|-------------|-------------|
| `initialize` | Once, before the iteration loop | Capture variables shared across all iterations (the GitHub user, the starting commit SHA) |
| `iteration` | Once per issue (or once per loop pass) | The work itself: pick up an issue, do the work, push |
| `finalize` | Once, after all iterations end | Cross-iteration cleanup: code review, doc update, deferred-work bookkeeping |

Steps in each array execute top-to-bottom. The optional `statusLine` block configures a custom status-line command displayed in the TUI footer — see [Configuring a Status Line](configuring-a-status-line.md).

The `finalize` array runs even when the iteration loop exits early (no more issues). Iteration-scoped variables (`ISSUE_ID`, `STARTING_SHA`) are *not* visible in finalize steps; using them substitutes the empty string. Built-in variables (`WORKFLOW_DIR`, `PROJECT_DIR`, `ITER`, `MAX_ITER`) remain available.

## Mixing Claude steps and shell steps

Every step is either a Claude invocation (`isClaude: true`) or a shell command (`isClaude: false`).

**Claude steps** invoke the `claude` CLI inside the Docker sandbox with a prompt file:

```json
{
  "name": "Feature work",
  "isClaude": true,
  "model": "sonnet",
  "promptFile": "feature-work.md"
}
```

At runtime pr9k builds the command `claude --permission-mode bypassPermissions --model sonnet -p <prompt-content>`. The prompt content is the file at `prompts/feature-work.md` with all `{{VAR}}` tokens substituted. The `bypassPermissions` flag skips Claude's interactive permission prompts — necessary for unattended runs, and one reason every Claude step is sandboxed.

**Shell steps** run an arbitrary command as a subprocess on the host:

```json
{
  "name": "Close issue",
  "isClaude": false,
  "command": ["scripts/close_gh_issue", "{{ISSUE_ID}}"]
}
```

The first element is the executable; the rest are arguments. `{{VAR}}` tokens in any element are substituted at runtime. Path resolution rules:

- A path containing `/` (e.g. `scripts/close_gh_issue`) is resolved relative to the workflow directory
- A bare command (e.g. `git`) is looked up via `$PATH`

Scripts must be executable (`chmod +x`) and inherit the host's `$PATH` and a curated set of environment variables — see [Passing Environment Variables](passing-environment-variables.md) for what's forwarded by default.

## Where the workflow lives

pr9k looks for the workflow bundle in two places, in order:

1. `<target-repo>/.pr9k/workflow/` — the **per-repo override** (recommended for custom work)
2. `<binary-dir>/.pr9k/workflow/` — the **shipped bundle** from `make build` (fallback)

For your own custom workflow, use the per-repo override. It lives next to the code it operates on, can be version-controlled (or `.gitignore`-d) per repo, and pr9k picks it up with no flags or rebuilds:

```bash
mkdir -p .pr9k/workflow/prompts .pr9k/workflow/scripts
# Add your config.json, prompts/*.md, and any scripts/* helpers here.
```

If you want the workflow committed to the repo, leave it tracked. If you want it private, add `.pr9k/workflow/` to `.gitignore`. Note: the runtime-state entries in [Getting Started](getting-started.md) intentionally ignore `.pr9k/logs/`, `.pr9k/iteration.jsonl`, and `.pr9k/artifacts/` rather than the entire `.pr9k/` folder, precisely so `.pr9k/workflow/` stays trackable by default.

To override both candidates explicitly (for example, when testing a feature branch of pr9k), pass `--workflow-dir <path>`. Editing `bin/.pr9k/workflow/` directly works but `make build` will overwrite it; if you're modifying the bundled workflow itself, edit the source files in the pr9k repo's `workflow/` directory and run `make build`.

## A bigger example: the bundled workflow

The bundled "Ralph" workflow has 11 iteration steps and 7 finalize steps. Reading through it is the fastest way to see how the pieces compose.

**Iteration phase (11 steps):**

1. **Get next issue** (shell, `breakLoopIfEmpty`) — picks the lowest-numbered open `ralph` issue assigned to the user; binds `ISSUE_ID`. When empty, pr9k exits the iteration loop and runs finalize.
2. **Get starting SHA** (shell) — records `HEAD` as `STARTING_SHA` for diff references later in the iteration
3. **Get issue body** (shell, `captureMode: fullStdout`) — fetches the issue title and body via `gh` and binds `ISSUE_BODY`
4. **Get project card** (shell, `captureMode: fullStdout`) — probes build-config files and binds `PROJECT_CARD` (a short tech-stack summary)
5. **Feature work** (sonnet) — implements the issue
6. **Get post-feature diff** (shell, `captureMode: fullStdout`) — captures `git diff {{STARTING_SHA}}..HEAD --stat` as `PRE_REVIEW_DIFF` for the test-writing prompt
7. **Test planning** (opus) — drafts a test plan
8. **Test writing** (sonnet) — writes the tests
9. **Summarize to issue** (shell) — posts a single end-of-iteration comment
10. **Close issue** (shell) — `gh issue close`
11. **Git push** (shell) — pushes the branch

**Finalize phase (7 steps):**

1. **Code review** (opus) — reviews every change on the branch; writes findings (or the `NOTHING-TO-FIX` sentinel) to `code-review.md`
2. **Check review verdict** (shell) — reads `code-review.md` and binds `REVIEW_HAS_FIXES` (empty when the sentinel is present)
3. **Fix review items** (sonnet, `skipIfCaptureEmpty: REVIEW_HAS_FIXES`) — implements review findings; skipped when the reviewer found nothing
4. **Update docs** (sonnet) — updates project docs for the whole branch
5. **Deferred work** (sonnet) — files issues from `deferred.txt`
6. **Lessons learned** (sonnet) — codifies entries from `progress.txt` into coding standards
7. **Final git push** (shell)

The bundled `config.json`, prompts, and scripts live in `workflow/` in the pr9k repo. Reading them alongside this list is the fastest way to learn the conventions.

## Iteration without GitHub

The bundled workflow is GitHub-driven, but pr9k itself doesn't know what GitHub is — see the [narrow-reading principle ADR](../adr/20260410170952-narrow-reading-principle.md). The orchestrator runs steps in order, captures their output, and substitutes variables; everything specific to GitHub lives in the bundled scripts and prompts.

To drive a non-GitHub workflow, replace **Get next issue** with any shell step that prints the next unit of work to stdout (a queue ID, a filename, a Linear issue ID), and replace **Summarize / Close issue / Git push** with whatever closure step makes sense for your queue. The other steps don't change.

## Field reference

This is the full schema for a step object. The first three rows are the basics; everything below `command`/`promptFile` is optional.

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| `name` | string | yes | Display name shown in the TUI checkbox grid and log banners |
| `isClaude` | bool | yes | `true` for Claude CLI steps, `false` for shell commands |
| `model` | string | Claude steps | `"sonnet"` or `"opus"` (passed verbatim to `claude --model`) |
| `promptFile` | string | Claude steps | Filename in `prompts/` (e.g. `"feature-work.md"`) |
| `command` | string[] | Shell steps | Argv: first element is the executable, rest are arguments |
| `captureAs` | string | optional | Bind this step's stdout to a variable for use in later steps. See [Capturing Step Output](capturing-step-output.md). |
| `captureMode` | string | optional | `"lastLine"` (default) or `"fullStdout"`. Only valid on non-Claude steps. |
| `breakLoopIfEmpty` | bool | optional | Exit the iteration loop when this step's captured output is empty. See [Breaking Out of the Loop](breaking-out-of-the-loop.md). |
| `skipIfCaptureEmpty` | string | optional | Skip this step when the named earlier-step capture is empty. See [Skipping Steps Conditionally](skipping-steps-conditionally.md). |
| `timeoutSeconds` | int | optional | Cap wall-clock time for this step (SIGTERM, then SIGKILL). See [Setting Step Timeouts](setting-step-timeouts.md). |
| `onTimeout` | string | optional | `"fail"` (default) or `"continue"`. With `"continue"`, a timeout marks the step `[!]` and advances. |
| `resumePrevious` | bool | optional | (Claude steps only) Attempt to resume the previous Claude step's session. Off by default in the bundled workflow. See [Resuming Sessions](resuming-sessions.md). |
| `env` | string[] | optional | Host environment variable names to forward into the sandbox container. See [Passing Environment Variables](passing-environment-variables.md). |
| `containerEnv` | object | optional | Literal env values to inject into the sandbox container. See [Passing Environment Variables](passing-environment-variables.md). |

For the formal schema and validator rules, see [Config Validation](../code-packages/validator.md). For the runtime semantics of each field, the linked how-tos go deeper.

## Iterating on your workflow

While developing a custom workflow:

1. Start in the per-repo override (`<target-repo>/.pr9k/workflow/`) so changes don't require a rebuild
2. Run with `-n 1` to test a single iteration end-to-end
3. Read the persisted log under `<target-repo>/.pr9k/logs/` if anything looks wrong — see [Debugging a Run](debugging-a-run.md)
4. If a Claude step is taking unusually long, check the iteration log for the per-step `$cost` and duration
5. Once the per-iteration shape is right, run with `-n 0` (the default) to let it loop until done

## Related documentation

- ← [Back to How-To Guides](README.md)
- [Getting Started](getting-started.md) — install, the one-time GitHub setup, first run
- [Workflow Variables](workflow-variables.md) — `{{VAR}}` substitution and file-based handoffs between steps
- [Capturing Step Output](capturing-step-output.md) — `captureAs` and `captureMode`
- [Breaking Out of the Loop](breaking-out-of-the-loop.md) — `breakLoopIfEmpty`
- [Skipping Steps Conditionally](skipping-steps-conditionally.md) — `skipIfCaptureEmpty`
- [Setting Step Timeouts](setting-step-timeouts.md) — `timeoutSeconds` and `onTimeout`
- [Passing Environment Variables](passing-environment-variables.md) — `env` and `containerEnv`
- [Resuming Sessions](resuming-sessions.md) — `resumePrevious` for tightly-coupled Claude steps
- [Debugging a Run](debugging-a-run.md) — read logs, reproduce a failure with `-n 1`
- [Narrow-Reading Principle ADR](../adr/20260410170952-narrow-reading-principle.md) — why workflow content belongs in `config.json`, not Go code
