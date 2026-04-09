# Plan: Unified Workflow Configuration (`ralph-steps.json`)

## Context

The ralph-tui runtime currently has five hardcoded implicit steps baked into `internal/workflow/run.go`: get GitHub username, get next issue, get starting SHA, display banner, and write completion summary. Step definitions are split across two JSON files (`configs/ralph-steps.json` and `configs/ralph-finalize-steps.json`), and variable prepending (`ISSUENUMBER`, `STARTINGSHA`) is hardcoded in Go.

This plan consolidates the entire workflow into a single `ralph-steps.json` with three phases, eliminates all implicit workflow steps from the runtime, and introduces a variable system that lets steps capture output and inject values into downstream steps.

---

## Design Decisions

### File Structure

- Single `ralph-steps.json` with three top-level keys: `pre-loop`, `loop`, `post-loop`
- Each key holds a flat array of step objects
- Iteration count comes from the CLI argument, not the JSON
- The old `configs/ralph-steps.json` and `configs/ralph-finalize-steps.json` are deleted

### Step Schema

A step is either a **Claude step** or a **command step**, inferred from which fields are present:

```json
// Claude step
{
  "name": "Feature work",
  "model": "sonnet",
  "promptFile": "feature-work.md",
  "permissionMode": "acceptEdits",
  "injectVariables": ["ISSUE_NUMBER", "STARTING_SHA"]
}

// Command step
{
  "name": "Get next issue",
  "command": ["scripts/get_next_issue", "{{GH_USERNAME}}"],
  "outputVariable": "ISSUE_NUMBER",
  "exitLoopIfEmpty": true
}
```

**Field rules:**

| Field | Claude step | Command step |
|-------|------------|--------------|
| `name` | required | required |
| `promptFile` | required | absent |
| `model` | optional (sonnet/opus, default: `sonnet`) | invalid |
| `permissionMode` | optional (default: `acceptEdits`) | invalid |
| `injectVariables` | optional | invalid |
| `command` | absent | required |
| `outputVariable` | invalid | optional |
| `exitLoopIfEmpty` | invalid | optional (requires `outputVariable`) |

Optional fields are omitted when not used (no empty arrays or default values in JSON).

### Variable System

**Capture:** Command steps with `outputVariable` capture trimmed stdout and store the value in a variable pool keyed by the variable name.

**Substitution — two mechanisms, one variable pool:**

1. **`{{VAR}}` template substitution** in `command` arrays — applies to command steps. The runtime replaces `{{VAR}}` with the value from the pool before executing.
2. **`{{VAR}}` template substitution** in prompt file contents — applies to Claude steps. The runtime reads the prompt file, replaces all `{{VAR}}` patterns, then passes the result to the Claude CLI.

**`injectVariables`** on Claude steps serves as a declaration of which variables the step depends on. It enables startup and just-in-time validation but does not change substitution behavior — substitution is driven by `{{VAR}}` patterns in the prompt text.

**Substitution is single-pass.** Build a `strings.NewReplacer` from the full variable pool and apply it once. Do NOT iterate over the pool applying `strings.ReplaceAll` sequentially — that would allow a captured value containing `{{ANOTHER_VAR}}` to be expanded by a later replacement pass. Single-pass prevents template injection from external command output.

**Scoping rules:**

- Pre-loop variables persist for the entire run (available to all three phases)
- Loop variables reset to empty at the start of each iteration
- Post-loop steps can only access pre-loop variables
- A post-loop step referencing a loop-scoped variable is a validation error at startup
- A loop step defining an `outputVariable` with the same name as a pre-loop variable is a validation error at startup — no shadowing allowed

### Validation

**Startup validation (eager, before any step executes):**

1. Every prompt file referenced by a `promptFile` field exists on disk — files are read to scan for `{{VAR}}` patterns but contents are NOT cached
2. Every entry in `injectVariables` corresponds to a `{{VAR}}` pattern in the prompt file
3. Every `{{VAR}}` pattern in prompt files and command arrays is declared (via `injectVariables` for Claude steps, or exists in the variable pool for command steps)
4. Every `{{VAR}}` or `injectVariables` entry references a variable defined by an `outputVariable` on a reachable prior step
5. No post-loop step references a loop-scoped variable
6. No step references a variable defined by a step that runs after it within the same phase

**Structural validation (startup):**

1. Step has both `promptFile` and `command` — error
2. Step has neither `promptFile` nor `command` — error
3. Step has `model` but no `promptFile` — error
4. Step has `injectVariables` but no `promptFile` — error
5. Step has `permissionMode` but no `promptFile` — error
6. Step has `exitLoopIfEmpty` but no `outputVariable` — error
7. Step has `exitLoopIfEmpty` outside of `loop` — error
8. `command` array is empty (`[]`) — error (must have at least one element)
9. A loop step's `outputVariable` name matches a pre-loop step's `outputVariable` name — error (no shadowing)

**Just-in-time validation (immediately before step execution):**

- Re-read the prompt file from disk (do NOT use cached content from startup)
- Re-validate all `{{VAR}}` patterns against `injectVariables` and the variable pool
- If JIT validation fails, enter error mode (retry/continue/quit) — not a hard failure
- This supports live-editing prompt files while a ralph run is in progress

### Path Resolution

**CLI flags:**

- `--project-dir` — base directory for ralph's config files (default: binary's directory via `os.Executable()`)
- `--steps` — steps filename (default: `ralph-steps.json`), resolved relative to `--project-dir`
- Target repo (where commands execute) = current working directory when invoking ralph-tui

**Folder conventions (hardcoded names, relative to `--project-dir`):**

- `prompts/` — prompt files referenced by `promptFile` (e.g., `"promptFile": "feature-work.md"` resolves to `<project-dir>/prompts/feature-work.md`)
- `scripts/` — script files referenced in `command` arrays

**Command resolution:**

- If the first element of `command` contains a `/`, resolve relative to `--project-dir` (e.g., `scripts/get_gh_user` becomes `<project-dir>/scripts/get_gh_user`)
- Otherwise, treat as a system command found on `$PATH` (e.g., `git`)

### Execution Modes

Command steps run in one of two modes based on schema fields:

- **Capture mode** (`outputVariable` is set): Use `CaptureOutput()` — run the command, capture trimmed stdout, store in the variable pool. No streaming to the TUI log (the step runs silently — only the header checkbox shows progress). If the command fails (non-zero exit), enter error mode (retry/continue/quit) — the variable is NOT set.
- **Stream mode** (`outputVariable` is absent): Use `RunStep()` — stream stdout/stderr to the TUI log in real-time. This is the current behavior for all steps.

Claude steps always use stream mode.

### Variable Pool

The variable pool is a `map[string]string` owned by `Run()` and threaded through step resolution. It is NOT passed into `Orchestrate` — instead, `Run()` resolves steps just-in-time (one at a time) before handing each resolved step to the executor.

This changes the current pattern where all steps are resolved up front in `buildIterationSteps()`. Instead, resolution happens per-step inside the iteration/finalization loops, enabling each step to see variables set by prior steps.

**Impact on `Orchestrate`:** The current `Orchestrate([]ResolvedStep, ...)` signature cannot support per-step resolution. Instead, `Run()` drives the step loop directly, resolving and executing one step at a time. `Orchestrate` is replaced by a per-step error-handling helper.

**Responsibility split — what moves where:**

| Responsibility | Current owner (`Orchestrate`) | New owner |
|---|---|---|
| Step loop iteration | `Orchestrate` | `Run()` |
| Pre-step quit drain (non-blocking check for pending `ActionQuit`) | `Orchestrate` lines 29-34 | `Run()`'s step loop |
| Step state transitions (`StepActive`/`StepDone`/`StepFailed`) | `Orchestrate` lines 36, 50, 54 | `Run()`'s step loop |
| Terminated-step bypass (`WasTerminated()` → treat as skip, not failure) | `runStepWithErrorHandling` line 49 | Error-handling helper |
| Error detection and retry loop | `runStepWithErrorHandling` lines 45-71 | Error-handling helper |
| Mode switching (`ModeError` → `ModeNormal`) | `runStepWithErrorHandling` line 58 | Error-handling helper |

### Loop Exit Signaling

When a command step with `exitLoopIfEmpty` produces empty output, `Run()` breaks out of the current iteration's step sequence AND the iteration loop. This is handled in `Run()`'s per-step loop — after executing a capture-mode step, check if `exitLoopIfEmpty` is set and the captured value is empty. No new action enum is needed since `Run()` owns the loop directly.

**Post-loop always runs** regardless of how the loop exits — whether all iterations complete, `exitLoopIfEmpty` triggers on iteration 1, or the user continues past an error. This matches current behavior (`run.go:107` runs finalization unconditionally after the loop).

### Runtime Behavior (not driven by steps)

These remain hardcoded in the runtime — they are UI chrome, not workflow steps:

- ASCII banner display at startup
- Completion summary at end ("Ralph completed after N iteration(s)...")
- Error mode UI (retry/continue/quit)
- Loop exit when `exitLoopIfEmpty` triggers

### Claude CLI Invocation

For Claude steps, the runtime builds the command:

```
["claude", "--permission-mode", "<permissionMode>", "--model", "<model>", "-p", "<substituted-prompt>"]
```

Only `model` and `permissionMode` are configurable. All other Claude CLI flags are hardcoded.

---

## Example `ralph-steps.json`

```json
{
  "pre-loop": [
    {
      "name": "Get GitHub username",
      "command": ["scripts/get_gh_user"],
      "outputVariable": "GH_USERNAME"
    }
  ],
  "loop": [
    {
      "name": "Get next issue",
      "command": ["scripts/get_next_issue", "{{GH_USERNAME}}"],
      "outputVariable": "ISSUE_NUMBER",
      "exitLoopIfEmpty": true
    },
    {
      "name": "Get starting SHA",
      "command": ["git", "rev-parse", "HEAD"],
      "outputVariable": "STARTING_SHA"
    },
    {
      "name": "Feature work",
      "model": "sonnet",
      "promptFile": "feature-work.md",
      "injectVariables": ["ISSUE_NUMBER", "STARTING_SHA"]
    },
    {
      "name": "Test planning",
      "model": "opus",
      "promptFile": "test-planning.md",
      "injectVariables": ["ISSUE_NUMBER", "STARTING_SHA"]
    },
    {
      "name": "Test writing",
      "model": "sonnet",
      "promptFile": "test-writing.md",
      "injectVariables": ["ISSUE_NUMBER", "STARTING_SHA"]
    },
    {
      "name": "Code review",
      "model": "opus",
      "promptFile": "code-review-changes.md",
      "injectVariables": ["ISSUE_NUMBER", "STARTING_SHA"]
    },
    {
      "name": "Review fixes",
      "model": "sonnet",
      "promptFile": "code-review-fixes.md",
      "injectVariables": ["ISSUE_NUMBER", "STARTING_SHA"]
    },
    {
      "name": "Close issue",
      "command": ["scripts/close_gh_issue", "{{ISSUE_NUMBER}}"]
    },
    {
      "name": "Update docs",
      "model": "sonnet",
      "promptFile": "update-docs.md",
      "injectVariables": ["ISSUE_NUMBER", "STARTING_SHA"]
    },
    {
      "name": "Git push",
      "command": ["git", "push"]
    }
  ],
  "post-loop": [
    {
      "name": "Deferred work",
      "model": "sonnet",
      "promptFile": "deferred-work.md"
    },
    {
      "name": "Lessons learned",
      "model": "sonnet",
      "promptFile": "lessons-learned.md"
    },
    {
      "name": "Final git push",
      "command": ["git", "push"]
    }
  ]
}
```

---

## Implementation Scope

### What changes

- **New:** Variable pool system (capture, store, substitute, scope, reset)
- **New:** Dual validation (startup eager + JIT re-validation)
- **New:** `--steps` CLI flag
- **Changed:** `--project-dir` default is unchanged (`os.Executable()`) but now also serves as the base for `--steps` resolution
- **Changed:** Step struct — drop `IsClaude`, `PrependVars`; add `outputVariable`, `exitLoopIfEmpty`, `injectVariables`, `permissionMode`
- **Changed:** `run.go` — remove all hardcoded steps (username, next issue, SHA); drive everything from the three-phase config; take over step loop from `Orchestrate`
- **Changed:** `orchestrate.go` — refactored from step-loop owner to per-step error-handling helper
- **Changed:** `header.go` — `StatusHeader` currently uses `[8]string` for step names and `[4]string` for display rows (hardcoded to exactly 8 iteration steps). Must change to `[]string` slices to support dynamic step counts from config
- **Changed:** Step loading — single file with three arrays replaces two separate files
- **Changed:** Prompt building — `{{VAR}}` substitution replaces `ISSUENUMBER=`/`STARTINGSHA=` prepending
- **Deleted:** `configs/ralph-finalize-steps.json`
- **Deleted:** `configs/ralph-steps.json` (replaced by top-level `ralph-steps.json`)
- **Changed:** Prompt files in `prompts/` — remove reliance on prepended `ISSUENUMBER=`/`STARTINGSHA=` lines; use `{{ISSUE_NUMBER}}`/`{{STARTING_SHA}}` inline patterns instead
- **Deleted:** `configs/` directory

### What stays the same

- ASCII banner and completion summary (runtime chrome)
- Error mode with retry/continue/quit
- Subprocess execution, streaming, and termination
- TUI display, keyboard input, signal handling
- File logging
- Script files in `scripts/` (content unchanged, just resolved differently)

---

## Review Summary

**Iterations completed:** 3 + agent validation
**Assumptions challenged:** 5 in iterations + 8 agent validations
**Consolidations:** 1 (removed contradictory prompt files entry from "What stays the same")

**Gaps filled during iterations:**
- Missing `model` default (`sonnet`) — would have caused empty `--model` flag at runtime
- Missing execution mode distinction (capture vs stream) — no prior specification of how `outputVariable` steps execute differently
- Missing variable pool ownership and threading model — Orchestrate's contract change was unspecified
- Missing `exitLoopIfEmpty` signaling mechanism — no path from step execution back to iteration loop
- Missing Orchestrate refactor in scope — `orchestrate.go` changes were implied but not listed
- Prompt files correctly moved from "stays the same" to "what changes"

**Gaps filled from agent validation:**
- Single-pass substitution requirement — prevents template injection from captured output (V5, moderate)
- Variable name collision rule — loop `outputVariable` cannot shadow pre-loop variable (V1, moderate)
- Orchestrate refactor responsibility split table — enumerates 6 responsibilities and their new owners to prevent losing pre-step quit drain and terminated-step bypass (V3, moderate)
- Empty `command` array validation rule — `"command": []` would pass validation but panic at runtime (V7, low)
- Post-loop execution guarantee — explicitly stated to always run regardless of loop exit (V2, low)
- StatusHeader `[8]string` → `[]string` — hardcoded fixed array must become dynamic for config-driven step counts (E6)
- 14 Orchestrate tests + 9 ResolveCommand tests + header tests will need rewriting (E2, E9, E7)

**Risks acknowledged but not requiring plan changes:**
- JIT partial-read during live editing — atomic editor saves + error-mode retry make this safe (V4, low)
- Capture-mode retry with prior variables still set — correct behavior, failed captures don't leak (V8, low)
- Prompt files use bare-word references to ISSUENUMBER/STARTINGSHA that Claude infers from prepended context — the `{{VAR}}` approach switches to literal text substitution, which is a deliberate design choice (E16/E17)
