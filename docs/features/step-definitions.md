# Step Definitions & Prompt Building

Loads workflow step definitions from JSON configuration files and builds prompt strings for Claude CLI invocations.

- **Last Updated:** 2026-04-08 12:00
- **Authors:**
  - River Bailey

## Overview

- Step definitions are loaded from `configs/ralph-steps.json` (8 iteration steps) and `configs/ralph-finalize-steps.json` (3 finalization steps)
- Each step is either a Claude CLI invocation (with model, prompt file, and optional variable prepending) or a shell command (with template variable substitution)
- `BuildPrompt` reads prompt files from `prompts/` and optionally prepends `ISSUENUMBER=` and `STARTINGSHA=` for iteration context
- Step definitions are pure data — command resolution and execution happen in the workflow package

Key files:
- `ralph-tui/internal/steps/steps.go` — Step struct, LoadSteps, LoadFinalizeSteps, BuildPrompt
- `ralph-tui/internal/steps/steps_test.go` — Unit tests for step loading and prompt building
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
   └──────┬───────┘                    │
          │                            │
          ▼                            ▼
   ┌──────────────┐         ┌──────────────────┐
   │  []Step      │────────▶│  BuildPrompt()   │
   │  (parsed     │         │  prepend vars    │
   │   structs)   │         │  read file       │
   └──────────────┘         └──────┬───────────┘
                                   │
                                   ▼
                            prompt string
                            (passed to claude -p)
```

## Key Files

| File | Purpose |
|------|---------|
| `ralph-tui/internal/steps/steps.go` | Step struct, loading functions, prompt builder |
| `ralph-tui/internal/steps/steps_test.go` | Unit tests for loading and prompt building |
| `ralph-tui/configs/ralph-steps.json` | Iteration step definitions (8 steps) |
| `ralph-tui/configs/ralph-finalize-steps.json` | Finalization step definitions (3 steps) |
| `ralph-tui/configs/configs_test.go` | Validates JSON structure of config files |

## Core Types

```go
// Step defines a single step in the ralph workflow.
type Step struct {
    Name        string   `json:"name"`
    Model       string   `json:"model,omitempty"`       // Claude model (e.g., "sonnet", "opus")
    PromptFile  string   `json:"promptFile,omitempty"`   // filename in prompts/ directory
    IsClaude    bool     `json:"isClaude"`               // true = Claude CLI, false = shell command
    Command     []string `json:"command,omitempty"`      // argv for non-Claude steps
    PrependVars bool     `json:"prependVars,omitempty"`  // prepend ISSUENUMBER/STARTINGSHA
}
```

## Implementation Details

### Step Loading

`LoadSteps` and `LoadFinalizeSteps` read JSON files relative to the project directory and unmarshal into `[]Step`:

```go
func LoadSteps(projectDir string) ([]Step, error) {
    return loadStepsFile(filepath.Join(projectDir, "configs", "ralph-steps.json"))
}

func loadStepsFile(path string) ([]Step, error) {
    data, err := os.ReadFile(path)
    // ... unmarshal JSON into []Step
}
```

### Iteration Steps

The 8 iteration steps run in sequence for each GitHub issue:

| # | Name | Type | Model | Prepend Vars |
|---|------|------|-------|--------------|
| 1 | Feature work | Claude | sonnet | yes |
| 2 | Test planning | Claude | opus | yes |
| 3 | Test writing | Claude | sonnet | yes |
| 4 | Code review | Claude | opus | yes |
| 5 | Review fixes | Claude | sonnet | yes |
| 6 | Close issue | Shell | — | — |
| 7 | Update docs | Claude | sonnet | yes |
| 8 | Git push | Shell | — | — |

Shell command steps use template variables (e.g., `{{ISSUE_ID}}`) that are substituted by `ResolveCommand` in the workflow package.

### Finalization Steps

Three steps run once after all iterations complete:

| # | Name | Type | Model |
|---|------|------|-------|
| 1 | Deferred work | Claude | sonnet |
| 2 | Lessons learned | Claude | sonnet |
| 3 | Final git push | Shell | — |

### Prompt Building

`BuildPrompt` reads the prompt file and optionally prepends iteration context variables:

```go
func BuildPrompt(projectDir string, step Step, issueID, startingSHA string) (string, error) {
    if step.PromptFile == "" {
        return "", fmt.Errorf("steps: PromptFile must not be empty")
    }
    promptPath := filepath.Join(projectDir, "prompts", step.PromptFile)
    data, err := os.ReadFile(promptPath)
    // ...
    content := string(data)
    if step.PrependVars {
        content = "ISSUENUMBER=" + issueID + "\nSTARTINGSHA=" + startingSHA + "\n" + content
    }
    return content, nil
}
```

When `PrependVars` is true, the resulting prompt looks like:

```
ISSUENUMBER=42
STARTINGSHA=abc123f
<original prompt file content>
```

## Error Handling

| Scenario | Error Message | Behavior |
|----------|---------------|----------|
| Config file unreadable | `"steps: could not read {path}: ..."` | Returned to caller |
| Malformed JSON | `"steps: malformed JSON in {path}: ..."` | Returned to caller |
| Empty PromptFile | `"steps: PromptFile must not be empty"` | Returned to caller |
| Prompt file unreadable | `"steps: could not read prompt {path}: ..."` | Returned to caller |

All errors are package-prefixed with `"steps:"` and include the file path.

## Testing

- `ralph-tui/internal/steps/steps_test.go` — Unit tests for LoadSteps, LoadFinalizeSteps, BuildPrompt
- `ralph-tui/configs/configs_test.go` — Validates that JSON config files parse correctly

### Test Patterns

Tests use `runtime.Caller(0)` to resolve test fixture paths relative to the test file location, following the project's Go testing conventions.

## Additional Information

- [Architecture Overview](../architecture.md) — System-level view of ralph-tui with block diagrams and data flow
- [Building Custom Workflows](../how-to/building-custom-workflows.md) — How to create custom step sequences, add prompts, and mix step types
- [Variable Output & Injection](../how-to/variable-output-and-injection.md) — How variables are injected into prompts and commands, and how steps pass data via files
- [CLI & Configuration](cli-configuration.md) — How ProjectDir is resolved and passed to step loading
- [Workflow Orchestration](workflow-orchestration.md) — How loaded steps are resolved and executed
- [Subprocess Execution & Streaming](subprocess-execution.md) — How ResolveCommand prepares shell commands for execution
- [ralph-tui Plan](../plans/ralph-tui.md) — Original specification including step definition design
- [Error Handling](../coding-standards/error-handling.md) — Coding standards for package-prefixed errors and file path inclusion
- [API Design](../coding-standards/api-design.md) — Coding standards for precondition validation (e.g., empty PromptFile check)
