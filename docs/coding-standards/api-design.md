# API Design

## Document unused parameters with a comment

If a parameter is intentionally unused (reserved for future use or part of an interface), add a doc comment that says so explicitly. Silent unused parameters are confusing to future callers and reviewers.

```go
// SetContext updates the iteration prefix for subsequent log lines.
// The stepName parameter is reserved for future use and is currently ignored.
func (l *Logger) SetContext(iteration int, stepName string) {
    l.mu.Lock()
    defer l.mu.Unlock()
    l.prefix = fmt.Sprintf("[iter %d]", iteration)
}
```

## Add bounds guards to all state-mutating array indexers

Any method that uses a caller-supplied index to mutate an array field must guard against out-of-bounds access. Panic on invalid index is unacceptable in long-running TUI processes.

```go
func (h *StatusHeader) SetStepState(idx int, state StepState) {
    if idx < 0 || idx >= 8 {
        return
    }
    // ...
}
```

## Validate preconditions at the function boundary

Check invariants at the start of a function and return a clear error. Do not let invalid inputs propagate into deeper I/O or OS calls where the resulting error is harder to interpret.

```go
func BuildPrompt(projectDir string, step Step, issueID, sha string) (string, error) {
    if step.PromptFile == "" {
        return "", fmt.Errorf("steps: PromptFile must not be empty")
    }
    // ...
}
```

## Use named constants for template placeholder strings

Template placeholder strings shared between config JSON and Go code (e.g., `{{ISSUE_ID}}`) should be named constants. As the number of placeholders grows, scattered string literals become a maintenance hazard.

```go
const issueIDPlaceholder = "{{ISSUE_ID}}"
```

## Adapter types for interface narrowing

When a single concrete type satisfies multiple interfaces that route the same method name to different behaviors, use a thin adapter struct rather than adding conditional logic to the callee. This keeps each call site unambiguous and the concrete type free of orchestration knowledge.

```go
// iterHeader routes SetStepState to the iteration columns of the header.
type iterHeader struct{ h RunHeader }

func (a iterHeader) SetStepState(idx int, state ui.StepState) {
    a.h.SetStepState(idx, state)
}

// finalHeader routes the same SetStepState call to the finalization columns.
type finalHeader struct{ h RunHeader }

func (a finalHeader) SetStepState(idx int, state ui.StepState) {
    a.h.SetFinalizeStepState(idx, state)
}
```

## Document platform-scoped assumptions

If a function uses platform-specific behavior (e.g., `/` as the path separator to detect script paths vs. bare commands), document the assumption at the call site so future maintainers know it is intentional, not an oversight.

```go
// Uses "/" as path separator; assumes Unix. Revise if Windows support is added.
if strings.Contains(command[0], "/") {
    command[0] = filepath.Join(projectDir, command[0])
}
```

## Additional Information

- [Architecture Overview](../architecture.md) — System-level architecture and design principles
- [Workflow Orchestration](../features/workflow-orchestration.md) — Adapter types (iterHeader/finalHeader) applying the interface narrowing pattern
- [TUI Status Header](../features/tui-display.md) — Bounds guards on SetStepState and SetFinalizeStepState
- [Step Definitions & Prompt Building](../features/step-definitions.md) — Precondition validation on empty PromptFile
- [Subprocess Execution & Streaming](../features/subprocess-execution.md) — Platform-scoped path separator assumption in ResolveCommand
- [Error Handling](error-handling.md) — Complementary standards for error message formatting
- [Concurrency](concurrency.md) — Complementary standards for mutex-protected getters (unexported fields)
- [Go Patterns](go-patterns.md) — Complementary Go-specific patterns
- [Testing](testing.md) — Standards for testing bounds guards and nil/uninitialized guard paths
