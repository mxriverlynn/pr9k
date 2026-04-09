# Step Definitions & Prompt Building

Loads workflow step definitions from JSON configuration files and builds prompt strings for Claude CLI invocations.

- **Last Updated:** 2026-04-08 12:00
- **Authors:**
  - River Bailey

## Overview

- Step definitions are loaded from `ralph-steps.json`, which contains both iteration steps (8) and finalization steps (3)
- Each step is either a Claude CLI invocation (with model, prompt file, and optional variable prepending) or a shell command (with template variable substitution)
- `BuildPrompt` reads prompt files from `prompts/` and optionally prepends `ISSUENUMBER=` and `STARTINGSHA=` for iteration context
- Step definitions are pure data — command resolution and execution happen in the workflow package

Key files:
- `ralph-tui/internal/steps/steps.go` — Step struct, StepFile struct, LoadSteps, BuildPrompt
- `ralph-tui/internal/steps/steps_test.go` — Unit tests for step loading and prompt building
- `ralph-tui/ralph-steps.json` — All step definitions (iteration and finalization)

## Architecture

```
┌─────────────────────┐     ┌──────────────────────┐
│ ralph-steps.json     │     │ prompts/              │
│  iteration: [...]    │     │  feature-work.md      │
│  finalize:  [...]    │     │  test-planning.md     │
└─────────┬───────────┘     │  test-writing.md      │
          │                  │  code-review-*.md     │
          ▼                  │  update-docs.md       │
   ┌──────────────┐         │  deferred-work.md     │
   │ LoadSteps()  │         │  lessons-learned.md   │
   └──────┬───────┘         └──────────┬────────────┘
          │                            │
          ▼                            ▼
   ┌──────────────┐         ┌──────────────────┐
   │  StepFile    │────────▶│  BuildPrompt()   │
   │  .Iteration  │         │  prepend vars    │
   │  .Finalize   │         │  read file       │
   └──────────────┘         └──────┬───────────┘
                                   │
                                   ▼
                            prompt string
                            (passed to claude -p)
```

## Key Files

| File | Purpose |
|------|---------|
| `ralph-tui/internal/steps/steps.go` | Step struct, StepFile struct, loading function, prompt builder |
| `ralph-tui/internal/steps/steps_test.go` | Unit tests for loading and prompt building |
| `ralph-tui/ralph-steps.json` | All step definitions (iteration and finalization) |

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

// StepFile holds the two groups of steps loaded from ralph-steps.json.
type StepFile struct {
    Iteration []Step `json:"iteration"`
    Finalize  []Step `json:"finalize"`
}
```

## Implementation Details

### Step Loading

`LoadSteps` reads `ralph-steps.json` relative to the project directory and unmarshals into a `StepFile`:

```go
func LoadSteps(projectDir string) (StepFile, error) {
    path := filepath.Join(projectDir, "ralph-steps.json")
    data, err := os.ReadFile(path)
    // ... unmarshal JSON into StepFile
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

- `ralph-tui/internal/steps/steps_test.go` — Unit tests for LoadSteps, BuildPrompt

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
