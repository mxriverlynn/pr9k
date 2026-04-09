# Step Definitions & Prompt Building

Loads workflow step definitions from JSON configuration files and builds prompt strings for Claude CLI invocations.

- **Last Updated:** 2026-04-09 (updated for single-pass {{VAR}} substitution)
- **Authors:**
  - River Bailey

## Overview

- Step definitions are loaded from `configs/ralph-steps.json` (8 iteration steps) and `configs/ralph-finalize-steps.json` (3 finalization steps) via `LoadSteps`/`LoadFinalizeSteps`
- A new `WorkflowConfig` struct supports a three-phase layout (`pre-loop`, `loop`, `post-loop`), loaded via `LoadWorkflowConfig` with 9-rule structural validation
- Each step is either a Claude CLI invocation (`promptFile` set) or a shell command (`command` set); helper methods `IsClaudeStep()` and `IsCommandStep()` distinguish the two
- `BuildPrompt` reads prompt file content and applies single-pass `{{KEY}}` substitution using a caller-supplied `vars` map; unknown placeholders are left as literal text
- `BuildReplacer` is an exported helper that constructs a `strings.Replacer` from a `map[string]string`; substitution is single-pass (a value containing `{{OTHER}}` is never re-expanded)
- Step definitions are pure data — command resolution and execution happen in the workflow package

Key files:
- `ralph-tui/internal/steps/steps.go` — Step struct, WorkflowConfig, LoadWorkflowConfig, LoadSteps, LoadFinalizeSteps, BuildPrompt
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
| `ralph-tui/internal/steps/steps_test.go` | Unit tests for loading, validation, and prompt building |
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

`LoadWorkflowConfig` reads a JSON file with three top-level keys (`pre-loop`, `loop`, `post-loop`), unmarshals into `WorkflowConfig`, and runs structural validation before returning. The `stepsFile` argument comes from `cli.Config.StepsFile` (default: `"ralph-steps.json"`; overridable with the `-steps` flag):

```go
func LoadWorkflowConfig(projectDir, stepsFile string) (*WorkflowConfig, error) {
    path := filepath.Join(projectDir, stepsFile)
    data, err := os.ReadFile(path)
    // ...
    var cfg WorkflowConfig
    json.Unmarshal(data, &cfg)
    validateStructure(&cfg)
    return &cfg, nil
}
```

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

In `buildIterationSteps` (workflow package), the vars map includes `ISSUE_ID`, `ISSUENUMBER`, and `STARTINGSHA`:

```go
vars := map[string]string{
    "ISSUE_ID":    issueID,
    "ISSUENUMBER": issueID,
    "STARTINGSHA": sha,
}
prompt, err := steps.BuildPrompt(projectDir, s, vars)
```

Prompt files reference variables with `{{ISSUENUMBER}}` or `{{STARTINGSHA}}` syntax. The same `vars` map is passed to `ResolveCommand` for shell command steps. Finalization steps receive `nil` vars (no substitution needed).

## Error Handling

| Scenario | Error Message | Behavior |
|----------|---------------|----------|
| Config file unreadable | `"steps: could not read {path}: ..."` | Returned to caller |
| Malformed JSON | `"steps: malformed JSON in {path}: ..."` | Returned to caller |
| Structural validation failure | `"steps: step {name}: {rule}; ..."` | All violations collected, returned as one error |
| Empty PromptFile | `"steps: PromptFile must not be empty"` | Returned to caller |
| Prompt file unreadable | `"steps: could not read prompt {path}: ..."` | Returned to caller |

All errors are package-prefixed with `"steps:"` and include the file path.

## Testing

- `ralph-tui/internal/steps/steps_test.go` — Unit tests for LoadSteps, LoadFinalizeSteps, LoadWorkflowConfig, BuildPrompt (including `{{VAR}}` substitution), BuildReplacer, and all 9 validation rules
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
