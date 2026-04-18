# Step Definitions & Prompt Building

Loads workflow step definitions from JSON configuration files and builds prompt strings for Claude CLI invocations.

- **Last Updated:** 2026-04-10
- **Authors:**
  - River Bailey

## Overview

- Step definitions are loaded from `ralph-steps.json`, which contains three step groups: initialize (pre-loop), iteration (per-issue), and finalize (post-loop)
- Each step is either a Claude CLI invocation (with model and prompt file) or a shell command (with template variable substitution)
- `BuildPrompt` reads prompt files from `prompts/` and applies `{{VAR}}` substitution using the supplied `VarTable` and phase
- Step definitions are pure data — command resolution and execution happen in the workflow package

Key files:
- `ralph-tui/internal/steps/steps.go` — Step struct, StepFile struct, LoadSteps, BuildPrompt
- `ralph-tui/internal/steps/steps_test.go` — Unit tests for step loading and prompt building
- `ralph-tui/ralph-steps.json` — All step definitions (initialize, iteration, and finalization)

## Architecture

```
┌─────────────────────┐     ┌──────────────────────┐
│ ralph-steps.json     │     │ prompts/              │
│  initialize: [...]   │     │  feature-work.md      │
│  iteration: [...]    │     │  test-planning.md     │
│  finalize:  [...]    │     │  test-writing.md      │
│  statusLine: {...}   │     │  code-review-*.md     │
└─────────┬───────────┘     │  update-docs.md       │
          │                  │  deferred-work.md     │
          ▼                  │  lessons-learned.md   │
   ┌──────────────┐         └──────────┬────────────┘
   │ LoadSteps()  │                    │
   └──────┬───────┘                    │
          │                            │
          ▼                            ▼
   ┌──────────────┐         ┌──────────────────┐
   │  StepFile    │────────▶│  BuildPrompt()   │
   │  .Initialize │         │  read file       │
   │  .Iteration  │         └──────┬───────────┘
   │  .Finalize   │                │
   │  .StatusLine │                ▼
   └──────────────┘         prompt string
                            (passed to claude -p)
```

## Key Files

| File | Purpose |
|------|---------|
| `ralph-tui/internal/steps/steps.go` | Step struct, StepFile struct, loading function, prompt builder |
| `ralph-tui/internal/steps/steps_test.go` | Unit tests for loading and prompt building |
| `ralph-tui/ralph-steps.json` | All step definitions (initialize, iteration, and finalization) |

## Core Types

```go
// Step defines a single step in the ralph workflow.
type Step struct {
    Name             string   `json:"name"`
    Model            string   `json:"model,omitempty"`            // Claude model (e.g., "sonnet", "opus")
    PromptFile       string   `json:"promptFile,omitempty"`       // filename in prompts/ directory
    IsClaude         bool     `json:"isClaude"`                   // true = Claude CLI, false = shell command
    Command          []string `json:"command,omitempty"`          // argv for non-Claude steps
    CaptureAs        string   `json:"captureAs,omitempty"`        // store step output under this variable name
    CaptureMode      string   `json:"captureMode,omitempty"`      // "" or "lastLine" (last non-empty line); "fullStdout" (all stdout, 32 KiB cap)
    BreakLoopIfEmpty bool     `json:"breakLoopIfEmpty,omitempty"` // exit iteration loop when captured output is empty
}

// StatusLineConfig holds the optional status-line configuration from ralph-steps.json.
// Consumed by the statusline package to construct a Runner.
type StatusLineConfig struct {
    Type                   string `json:"type,omitempty"`
    Command                string `json:"command"`
    RefreshIntervalSeconds *int   `json:"refreshIntervalSeconds,omitempty"`
}

// StepFile holds the three groups of steps loaded from ralph-steps.json.
type StepFile struct {
    Env          []string          `json:"env,omitempty"`
    ContainerEnv map[string]string `json:"containerEnv,omitempty"`
    Initialize   []Step            `json:"initialize"`
    Iteration    []Step            `json:"iteration"`
    Finalize     []Step            `json:"finalize"`
    StatusLine   *StatusLineConfig `json:"statusLine,omitempty"`
}
```

## Implementation Details

### Step Loading

`LoadSteps` reads `ralph-steps.json` relative to the workflow directory and unmarshals into a `StepFile`:

```go
func LoadSteps(workflowDir string) (StepFile, error) {
    path := filepath.Join(workflowDir, "ralph-steps.json")
    data, err := os.ReadFile(path)
    // ... unmarshal JSON into StepFile
}
```

### Initialize Steps

Two steps run once before the iteration loop begins:

| # | Name | Type | captureAs |
|---|------|------|-----------|
| 1 | Splash | Shell | — |
| 2 | Get GitHub user | Shell | `GITHUB_USER` |

"Splash" runs `cat {{WORKFLOW_DIR}}/ralph-art.txt` to display the startup banner. "Get GitHub user" runs `scripts/get_gh_user` and captures the result as `GITHUB_USER`, making it available to all subsequent phases.

### Iteration Steps

The 10 iteration steps run in sequence for each GitHub issue:

| # | Name | Type | Model | captureAs |
|---|------|------|-------|-----------|
| 1 | Get next issue | Shell | — | `ISSUE_ID` |
| 2 | Get starting SHA | Shell | — | `STARTING_SHA` |
| 3 | Feature work | Claude | sonnet | — |
| 4 | Test planning | Claude | opus | — |
| 5 | Test writing | Claude | sonnet | — |
| 6 | Code review | Claude | opus | — |
| 7 | Fix review items | Claude | sonnet | — |
| 8 | Close issue | Shell | — | — |
| 9 | Update docs | Claude | sonnet | — |
| 10 | Git push | Shell | — | — |

"Get next issue" has `breakLoopIfEmpty: true` — when `ISSUE_ID` is empty, the iteration loop exits. Shell command steps use template variables (e.g., `{{ISSUE_ID}}`) that are substituted by `ResolveCommand` in the workflow package.

### Finalization Steps

Three steps run once after all iterations complete:

| # | Name | Type | Model |
|---|------|------|-------|
| 1 | Deferred work | Claude | sonnet |
| 2 | Lessons learned | Claude | sonnet |
| 3 | Final git push | Shell | — |

### Prompt Building

`BuildPrompt` reads the prompt file, applies `{{VAR}}` substitution using the supplied `VarTable` and phase, and returns the result:

```go
func BuildPrompt(workflowDir string, step Step, vt *vars.VarTable, phase vars.Phase) (string, error) {
    if step.PromptFile == "" {
        return "", fmt.Errorf("steps: PromptFile must not be empty")
    }
    promptPath := filepath.Join(workflowDir, "prompts", step.PromptFile)
    absPath, absErr := filepath.Abs(promptPath)
    absPrompts, absPromptsErr := filepath.Abs(filepath.Join(workflowDir, "prompts"))
    if absErr != nil || absPromptsErr != nil || !strings.HasPrefix(absPath, absPrompts+string(filepath.Separator)) {
        return "", fmt.Errorf("steps: prompt path escapes prompts directory: %s", step.PromptFile)
    }
    data, err := os.ReadFile(promptPath)
    // ...
    content, err := vars.Substitute(string(data), vt, phase)
    // ...
    return content, nil
}
```

The path containment check prevents `promptFile` values containing `..` segments (e.g., `"../../../etc/passwd"`) from reading files outside the `prompts/` directory. Both the resolved path and the prompts directory are converted to absolute paths before comparison.

All `{{VAR_NAME}}` tokens in the prompt file are replaced with values from `vt` before the string is returned. Unresolved variables log a warning and substitute the empty string.

## Error Handling

| Scenario | Error Message | Behavior |
|----------|---------------|----------|
| Config file unreadable | `"steps: could not read {path}: ..."` | Returned to caller |
| Malformed JSON | `"steps: malformed JSON in {path}: ..."` | Returned to caller |
| Empty PromptFile | `"steps: PromptFile must not be empty"` | Returned to caller |
| Path traversal attempt | `"steps: prompt path escapes prompts directory: {promptFile}"` | Returned to caller |
| Prompt file unreadable | `"steps: could not read prompt {path}: ..."` | Returned to caller |
| Substitution error | `"steps: substitution failed in prompt {path}: ..."` | Returned to caller |

All errors are package-prefixed with `"steps:"` and include the file path.

## Testing

- `ralph-tui/internal/steps/steps_test.go` — Unit tests for LoadSteps, BuildPrompt

### Test Patterns

Tests use `runtime.Caller(0)` to resolve test fixture paths relative to the test file location, following the project's Go testing conventions.

## Additional Information

- [Architecture Overview](../architecture.md) — System-level view of ralph-tui with block diagrams and data flow
- [Building Custom Workflows](../how-to/building-custom-workflows.md) — How to create custom step sequences, add prompts, and mix step types
- [Variable Output & Injection](../how-to/variable-output-and-injection.md) — How variables are injected into prompts and commands, and how steps pass data via files
- [CLI & Configuration](../features/cli-configuration.md) — How ProjectDir is resolved and passed to step loading
- [Workflow Orchestration](../features/workflow-orchestration.md) — How loaded steps are resolved and executed
- [Subprocess Execution & Streaming](../features/subprocess-execution.md) — How ResolveCommand prepares shell commands for execution
- [ralph-tui Plan](../plans/ralph-tui.md) — Original specification including step definition design
- [Passing Environment Variables](../how-to/passing-environment-variables.md) — How to forward host env vars via `env` and inject literal values via `containerEnv` in `ralph-steps.json`
- [Error Handling](../coding-standards/error-handling.md) — Coding standards for package-prefixed errors and file path inclusion
- [API Design](../coding-standards/api-design.md) — Coding standards for precondition validation (e.g., empty PromptFile check)
