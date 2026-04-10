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

Any method that uses a caller-supplied index to mutate an array or slice field must guard against out-of-bounds access. Panic on invalid index is unacceptable in long-running TUI processes.

```go
func (h *StatusHeader) SetStepState(idx int, state StepState) {
    if idx < 0 || idx >= len(h.stepNames) {
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

When a caller needs to route an interface method call to a specific position in a larger data structure (e.g. a single step's checkbox within a multi-step grid), use a thin adapter struct rather than adding conditional logic to the callee. This keeps each call site unambiguous and the concrete type free of orchestration knowledge.

```go
// trackingOffsetIterHeader adapts RunHeader to ui.StepHeader for a single
// step at absolute index idx. It pins the absolute TUI checkbox position at
// construction time, because Orchestrate always calls SetStepState with a
// local index i (not the global position). It also records the last StepState
// so Run can check whether the step ended as StepDone before evaluating
// BreakLoopIfEmpty.
type trackingOffsetIterHeader struct {
    h         RunHeader
    idx       int
    lastState ui.StepState
}

func (a *trackingOffsetIterHeader) SetStepState(_ int, state ui.StepState) {
    a.lastState = state
    a.h.SetStepState(a.idx, state)
}
```

## Split phase-specific render methods rather than one conditional setter

When a render or display method handles distinct phases (e.g., initialize, iterate, finalize) each with its own format, split it into one method per phase rather than a single method with a phase parameter or internal conditional branching. Phase-specific methods:

- Name their intent at the call site (`RenderInitializeLine` vs. `SetHeader(PhaseInit, ...)`)
- Accept only the parameters relevant to that phase — no unused arguments padded with zero values
- Can be added or changed independently without risk of breaking the other phases

```go
// Bad — monolithic setter with internal conditionals; callers pass phase-dependent zeros
func (h *StatusHeader) SetIteration(current, total int, issueID, issueTitle string) {
    if issueID == "" {
        h.IterationLine = fmt.Sprintf("Iteration %d/%d", current, total)
    } else {
        h.IterationLine = fmt.Sprintf("Iteration %d/%d — Issue #%s: %s", current, total, issueID, issueTitle)
    }
}

// Good — one method per phase, parameters scoped to that phase only
func (h *StatusHeader) RenderInitializeLine(stepNum, stepCount int, stepName string)
func (h *StatusHeader) RenderIterationLine(iter, maxIter int, issueID string)
func (h *StatusHeader) RenderFinalizeLine(stepNum, stepCount int, stepName string)
func (h *StatusHeader) RenderCompletionLine(iterationsRun, finalizeCount int)
```

Apply this when an existing method starts accumulating conditional branches keyed on which lifecycle phase is active. That branching belongs in the method name, not the method body.

## Remove unused methods from interfaces

When a method is removed from an interface's concrete callers, remove it from the interface too. A method that exists only on the concrete type — not consumed through the interface anywhere — is dead weight. It forces all test doubles to implement a no-op, misleads readers about what the interface contract covers, and signals that the abstraction boundary is drifting.

```go
// Bad — CaptureOutput removed from all interface call sites but left on the interface
type StepExecutor interface {
    RunStep(...)
    LastCapture() string
    CaptureOutput(...) (string, error) // no longer called through this interface
}

// Good — interface matches the actual usage contract
type StepExecutor interface {
    RunStep(...)
    LastCapture() string
}
// CaptureOutput can remain on the concrete Runner if it is still used directly.
```

When reviewing a PR that removes a method from concrete callers: check whether the method should also be removed from the interface.

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
- [Workflow Orchestration](../features/workflow-orchestration.md) — Adapter types (trackingOffsetIterHeader/noopHeader) applying the interface narrowing pattern; CaptureOutput removal from StepExecutor interface as an example of unused-method cleanup; RunHeader phase-specific render methods as the canonical phase-splitting example
- [TUI Status Header](../features/tui-display.md) — Bounds guards on SetStepState; SetPhaseSteps panic-on-overflow as the appropriate choice for programming errors
- [Step Definitions & Prompt Building](../features/step-definitions.md) — Precondition validation on empty PromptFile
- [Subprocess Execution & Streaming](../features/subprocess-execution.md) — Platform-scoped path separator assumption in ResolveCommand
- [Error Handling](error-handling.md) — Complementary standards for error message formatting
- [Concurrency](concurrency.md) — Complementary standards for mutex-protected getters (unexported fields)
- [Go Patterns](go-patterns.md) — Complementary Go-specific patterns
- [Testing](testing.md) — Standards for testing bounds guards and nil/uninitialized guard paths
