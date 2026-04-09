# Step Definitions & Prompt Building

Loads workflow step definitions from JSON configuration files and builds prompt strings for Claude CLI invocations.

- **Last Updated:** 2026-04-09 (updated for variable scoping validation)
- **Authors:**
  - River Bailey

## Overview

- Step definitions are loaded from `configs/ralph-steps.json` (8 iteration steps) and `configs/ralph-finalize-steps.json` (3 finalization steps) via `LoadSteps`/`LoadFinalizeSteps`
- A new `WorkflowConfig` struct supports a three-phase layout (`pre-loop`, `loop`, `post-loop`), loaded via `LoadWorkflowConfig` with 9-rule structural validation followed by variable scoping validation
- Each step is either a Claude CLI invocation (`promptFile` set) or a shell command (`command` set); helper methods `IsClaudeStep()` and `IsCommandStep()` distinguish the two
- `BuildPrompt` reads prompt file content and applies single-pass `{{KEY}}` substitution using a caller-supplied `vars` map; unknown placeholders are left as literal text
- `BuildReplacer` is an exported helper that constructs a `strings.Replacer` from a `map[string]string`; substitution is single-pass (a value containing `{{OTHER}}` is never re-expanded)
- Step definitions are pure data — command resolution and execution happen in the workflow package

Key files:
- `ralph-tui/internal/steps/steps.go` — Step struct, WorkflowConfig, LoadWorkflowConfig, LoadSteps, LoadFinalizeSteps, BuildPrompt
- `ralph-tui/internal/steps/validate.go` — ValidateVariables: startup variable scoping and prompt consistency checks
- `ralph-tui/internal/steps/steps_test.go` — Unit tests for step loading, prompt building, and validation
- `ralph-tui/configs/ralph-steps.json` — 8 iteration step definitions
- `ralph-tui/configs/ralph-finalize-steps.json` — 3 finalization step definitions

## Architecture

```
┌─────────────────────┐     ┌──────────────────────┐
│ configs/             │     │ prompts/              │
│  ralph-steps.json    │     │  feature-work.md      │
│  ralph-finalize-     │     │  test-planning.md     │
│    steps.json        │     │  test-writing.md      │
└─────────┬───────────┘     │  code-review-*.md     │
          │                  │  update-docs.md       │
          ▼                  │  deferred-work.md     │
   ┌──────────────┐         │  lessons-learned.md   │
   │ LoadSteps()  │         └──────────┬────────────┘
   │ LoadFinalize │                    │
   │  Steps()     │                    │
   │              │                    │
   │ LoadWorkflow │                    │
   │  Config()    │                    │
   └──────┬───────┘                    │
          │                            │
          ▼                            ▼
   ┌──────────────┐         ┌──────────────────────┐
   │  []Step /    │────────▶│  BuildPrompt()       │
   │  Workflow    │         │  read file +         │
   │  Config      │         │  {{KEY}} substitution│
   └──────────────┘         └──────────┬───────────┘
                                        │
                            ┌───────────▼──────────┐
                            │  BuildReplacer(vars) │
                            │  single-pass replace │
                            └───────────┬──────────┘
                                        │
                                        ▼
                              substituted prompt string
                              (passed to claude -p)
```

## Key Files

| File | Purpose |
|------|---------|
| `ralph-tui/internal/steps/steps.go` | Step struct, WorkflowConfig, loading functions, prompt builder |
| `ralph-tui/internal/steps/validate.go` | ValidateVariables: variable scoping and prompt consistency checks |
| `ralph-tui/internal/steps/validate_test.go` | Unit tests for variable validation |
| `ralph-tui/internal/steps/steps_test.go` | Unit tests for loading, structural validation, and prompt building |
| `ralph-tui/configs/ralph-steps.json` | Iteration step definitions (8 steps) |
| `ralph-tui/configs/ralph-finalize-steps.json` | Finalization step definitions (3 steps) |
| `ralph-tui/configs/configs_test.go` | Validates JSON structure of config files |

## Core Types

```go
// Step defines a single step in the ralph workflow.
type Step struct {
    Name           string   `json:"name"`
    Model          string   `json:"model,omitempty"`
    PromptFile     string   `json:"promptFile,omitempty"`
    PermissionMode string   `json:"permissionMode,omitempty"`
    InjectVars     []string `json:"injectVariables,omitempty"`
    // Command holds the argv for non-claude steps. Arguments may contain
    // template placeholders (e.g. "{{ISSUE_ID}}") that are substituted by
    // ResolveCommand in the workflow package using a vars map.
    Command         []string `json:"command,omitempty"`
    OutputVariable  string   `json:"outputVariable,omitempty"`
    ExitLoopIfEmpty bool     `json:"exitLoopIfEmpty,omitempty"`
}

// WorkflowConfig holds the three-phase step configuration.
type WorkflowConfig struct {
    PreLoop  []Step `json:"pre-loop"`
    Loop     []Step `json:"loop"`
    PostLoop []Step `json:"post-loop"`
}
```

### Step Helper Methods

| Method | Returns |
|--------|---------|
| `IsClaudeStep()` | `true` when `PromptFile` is set |
| `IsCommandStep()` | `true` when `Command` is non-empty |
| `DefaultModel()` | `Model` if set, otherwise `"sonnet"` |
| `DefaultPermissionMode()` | `PermissionMode` if set, otherwise `"acceptEdits"` |

## Implementation Details

### Step Loading

`LoadSteps` and `LoadFinalizeSteps` read flat JSON arrays relative to the project directory (backward-compatible with the existing `configs/` layout):

```go
func LoadSteps(projectDir string) ([]Step, error) {
    return loadStepsFile(filepath.Join(projectDir, "configs", "ralph-steps.json"))
}

func loadStepsFile(path string) ([]Step, error) {
    data, err := os.ReadFile(path)
    // ... unmarshal JSON into []Step
}
```

### Three-Phase WorkflowConfig

`LoadWorkflowConfig` reads a JSON file with three top-level keys (`pre-loop`, `loop`, `post-loop`), unmarshals into `WorkflowConfig`, and runs both structural validation and variable validation before returning. The `stepsFile` argument comes from `cli.Config.StepsFile` (default: `"ralph-steps.json"`; overridable with the `-steps` flag):

```go
func LoadWorkflowConfig(projectDir, stepsFile string) (*WorkflowConfig, error) {
    path := filepath.Join(projectDir, stepsFile)
    data, err := os.ReadFile(path)
    // ...
    var cfg WorkflowConfig
    json.Unmarshal(data, &cfg)
    validateStructure(&cfg)
    ValidateVariables(&cfg, projectDir)
    return &cfg, nil
}
```

Structural validation runs first; if it fails, `LoadWorkflowConfig` returns immediately without running variable validation.

### Structural Validation

`validateStructure` checks every step in all three phases and collects all violations before returning a combined error. The 9 rules:

| # | Rule |
|---|------|
| 1 | A step may not have both `promptFile` and `command` |
| 2 | A step must have at least one of `promptFile` or `command` |
| 3 | `model` requires `promptFile` |
| 4 | `injectVariables` requires `promptFile` |
| 5 | `permissionMode` requires `promptFile` |
| 6 | `exitLoopIfEmpty` requires `outputVariable` |
| 7 | `exitLoopIfEmpty` is only valid in the `loop` phase |
| 8 | `command` array must not be empty if present |
| 9 | `outputVariable` requires `command`, not `promptFile` |

Errors are collected across all phases and returned as a single combined message:

```
steps: step "bad-pre": must have promptFile or command; step "bad-loop": must have promptFile or command
```

### Variable Validation

`ValidateVariables` (in `validate.go`) runs at startup after structural validation, before any step executes. It reads prompt files from disk and validates variable scoping across all three phases. All errors are collected and returned as a single combined error.

#### Scoping Rules

Variables are declared by `outputVariable` fields on steps. Their scope depends on which phase declared them:

| Declared in | Available in |
|-------------|-------------|
| `pre-loop` | pre-loop (steps after the declaring step), loop, post-loop |
| `loop` | loop only (steps after the declaring step within the same iteration) |
| `post-loop` | post-loop (steps after the declaring step) |

Loop-scoped variables are **not** available in post-loop. Pre-loop variables cannot be shadowed by loop `outputVariable` declarations.

#### Checks Performed

For every step, `ValidateVariables` checks:

1. **Shadowing** — a loop-phase `outputVariable` must not duplicate a pre-loop `outputVariable`
2. **Prompt/injectVariables consistency** (Claude steps only):
   - Every entry in `injectVariables` must appear as `{{VAR}}` in the prompt file
   - Every `{{VAR}}` in the prompt file must be listed in `injectVariables`
3. **Reachability** (Claude steps and command steps):
   - Every variable referenced (via `injectVariables` or `{{VAR}}` in command args) must be declared somewhere in the config
   - Referenced variables must be reachable at the current step position per the scoping rules above
   - Forward references within the same phase are rejected

#### Variable Pattern

`{{VAR}}` placeholders in prompt files and command args must match `[A-Z_][A-Z0-9_]*` (uppercase identifiers only). The regex is `\{\{([A-Z_][A-Z0-9_]*)\}\}`.

### Iteration Steps

The 8 iteration steps run in sequence for each GitHub issue:

| # | Name | Type | Model |
|---|------|------|-------|
| 1 | Feature work | Claude | sonnet |
| 2 | Test planning | Claude | opus |
| 3 | Test writing | Claude | sonnet |
| 4 | Code review | Claude | opus |
| 5 | Review fixes | Claude | sonnet |
| 6 | Close issue | Shell | — |
| 7 | Update docs | Claude | sonnet |
| 8 | Git push | Shell | — |

Shell command steps use template variables (e.g., `{{ISSUE_ID}}`, `{{ISSUENUMBER}}`, `{{STARTINGSHA}}`) that are substituted by `ResolveCommand` in the workflow package using a `vars` map.

### Finalization Steps

Three steps run once after all iterations complete:

| # | Name | Type | Model |
|---|------|------|-------|
| 1 | Deferred work | Claude | sonnet |
| 2 | Lessons learned | Claude | sonnet |
| 3 | Final git push | Shell | — |

### Prompt Building

`BuildPrompt` reads the prompt file and applies single-pass `{{KEY}}` substitution using the provided `vars` map. Unknown placeholders are left as literal text. Substitution is single-pass, so a value containing `{{OTHER}}` is never re-expanded (template injection safe):

```go
func BuildPrompt(projectDir string, step Step, vars map[string]string) (string, error) {
    promptPath := filepath.Join(projectDir, "prompts", step.PromptFile)
    data, err := os.ReadFile(promptPath)
    // ...
    return BuildReplacer(vars).Replace(string(data)), nil
}
```

`BuildReplacer` constructs the `strings.Replacer` from the vars map:

```go
func BuildReplacer(vars map[string]string) *strings.Replacer {
    pairs := make([]string, 0, len(vars)*2)
    for k, v := range vars {
        pairs = append(pairs, "{{"+k+"}}", v)
    }
    return strings.NewReplacer(pairs...)
}
```

In `executeStep` (workflow package), the vars map is obtained from `pool.All()` just before each step runs. The pool is populated by prior steps' `outputVariable` captures (e.g. an issue-ID step storing `ISSUE_ID`). Both `BuildPrompt` (for Claude steps) and `ResolveCommand` (for shell steps) receive the same snapshot:

```go
vars := pool.All()  // snapshot: includes all captured outputVariable values so far

// Claude step
prompt, err := steps.BuildPrompt(projectDir, step, vars)

// Shell step
cmd = ResolveCommand(projectDir, step.Command, vars)
```

Prompt files and command args reference variables with `{{VAR}}` syntax (e.g. `{{ISSUE_ID}}`, `{{STARTINGSHA}}`). Variables are declared in the JSON config via `outputVariable` and captured at runtime by `CaptureOutput`.

## Error Handling

| Scenario | Error Message | Behavior |
|----------|---------------|----------|
| Config file unreadable | `"steps: could not read {path}: ..."` | Returned to caller |
| Malformed JSON | `"steps: malformed JSON in {path}: ..."` | Returned to caller |
| Structural validation failure | `"steps: step {name}: {rule}; ..."` | All violations collected, returned as one error |
| Empty PromptFile | `"steps: PromptFile must not be empty"` | Returned to caller |
| Prompt file unreadable | `"steps: could not read prompt {path}: ..."` | Returned to caller |
| Loop var shadows pre-loop var | `'steps: step "{name}": outputVariable "{var}" shadows pre-loop variable; ...'` | All violations collected |
| injectVariables entry not in prompt | `'steps: step "{name}": injectVariables entry "{var}" not found as {{VAR}} in prompt file; ...'` | All violations collected |
| `{{VAR}}` in prompt not in injectVariables | `'steps: step "{name}": {{VAR}} in prompt file not listed in injectVariables; ...'` | All violations collected |
| Undefined variable reference | `'steps: step "{name}": {{VAR}} references undefined variable; ...'` | All violations collected |
| Forward reference within same phase | `'steps: step "{name}": references variable "{var}" declared by later step "{step}"; ...'` | All violations collected |
| Loop var referenced from post-loop | `'steps: step "{name}": references loop-scoped variable "{var}" from post-loop; ...'` | All violations collected |

All errors are package-prefixed with `"steps:"` and include the file path.

## Testing

- `ralph-tui/internal/steps/steps_test.go` — Unit tests for LoadSteps, LoadFinalizeSteps, LoadWorkflowConfig, BuildPrompt (including `{{VAR}}` substitution), BuildReplacer, and all 9 structural validation rules
- `ralph-tui/internal/steps/validate_test.go` — Unit tests for ValidateVariables: scoping rules, shadowing, forward references, prompt/injectVariables consistency, and edge cases
- `ralph-tui/configs/configs_test.go` — Validates that JSON config files parse correctly

### Test Patterns

Tests use `runtime.Caller(0)` to resolve test fixture paths relative to the test file location, following the project's Go testing conventions.

## Additional Information

- [Architecture Overview](../architecture.md) — System-level view of ralph-tui with block diagrams and data flow
- [CLI & Configuration](cli-configuration.md) — How ProjectDir is resolved and passed to step loading
- [Workflow Orchestration](workflow-orchestration.md) — How loaded steps are resolved and executed
- [Subprocess Execution & Streaming](subprocess-execution.md) — How ResolveCommand prepares shell commands for execution
- [ralph-tui Plan](../plans/ralph-tui.md) — Original specification including step definition design
- [Error Handling](../coding-standards/error-handling.md) — Coding standards for package-prefixed errors and file path inclusion
- [API Design](../coding-standards/api-design.md) — Coding standards for precondition validation (e.g., empty PromptFile check)
