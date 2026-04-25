# API Design

## Document unused parameters with a comment

If a parameter is intentionally unused (reserved for future use or part of an interface), add a doc comment that says so explicitly. Silent unused parameters are confusing to future callers and reviewers.

```go
// SetContext updates the iteration prefix for subsequent log lines.
// The second parameter is reserved for future use and is currently ignored.
func (l *Logger) SetContext(iteration string, _ string) {
    l.mu.Lock()
    defer l.mu.Unlock()
    l.iteration = iteration
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
func BuildPrompt(workflowDir string, step Step, vt *vars.VarTable, phase vars.Phase) (string, error) {
    if step.PromptFile == "" {
        return "", fmt.Errorf("steps: PromptFile must not be empty")
    }
    // ...
}
```

## Use named constants for template placeholder strings

Template placeholder strings shared between config JSON and Go code (e.g., `{{ISSUE_ID}}`) should be named constants or centralized in a registry. As the number of placeholders grows, scattered string literals become a maintenance hazard. In this codebase, the `reservedNames` map in `internal/vars/vars.go` serves as the single registry of built-in variable names:

```go
var reservedNames = map[string]bool{
    "WORKFLOW_DIR": true,
    "PROJECT_DIR":  true,
    "MAX_ITER":     true,
    "ITER":         true,
    "STEP_NUM":     true,
    "STEP_COUNT":   true,
    "STEP_NAME":    true,
}
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

## Name private helpers with a `Locked` suffix when caller must hold the mutex

When an unexported helper method requires the caller to already hold a mutex, append `Locked` to its name. The suffix communicates the precondition at the declaration site, so reviewers and future callers can immediately see the contract without reading the body.

```go
// Good — name signals the precondition; callers know they must hold h.mu
func (h *KeyHandler) updateShortcutLineLocked() {
    // h.mu must be held by caller
    switch h.mode {
    case ModeNormal:
        h.shortcutLine = NormalShortcuts
    // ...
    }
}

// Callers are explicit about the requirement:
h.mu.Lock()
h.mode = ModeNormal
h.updateShortcutLineLocked()
h.mu.Unlock()

// Bad — nothing at the call site signals the mutex requirement
func (h *KeyHandler) updateShortcutLine() { ... }
```

Apply this naming convention to any unexported method that documents "caller holds X lock" in its comment or body.

## Install a panicking sentinel for required callbacks

When a type requires a callback (e.g., `sendLine`) to be installed via a setter before certain methods are called, initialize the field to a panicking closure at construction time — not `nil`. This fails loudly with a clear message instead of silently no-oping or nil-panicking with no context.

```go
func NewRunner(...) *Runner {
    r := &Runner{
        // Panicking sentinel: callers must call SetSender before RunStep.
        // NewRunner provides this default so a missing SetSender fails loudly
        // in tests and integration rather than silently discarding output.
        sendLine: func(string) {
            panic("workflow.Runner: sendLine not set — call SetSender before running steps")
        },
    }
    // ...
    return r
}
```

This differs from precondition validation (which checks user input and returns an error). Use panicking sentinels for programming errors — misconfigured callers that omit required setup — where a panic is appropriate because the problem is always a code bug, not a runtime condition.

## Document platform-scoped assumptions

If a function uses platform-specific behavior (e.g., `/` as the path separator to detect script paths vs. bare commands), document the assumption at the call site so future maintainers know it is intentional, not an oversight.

```go
// Uses "/" as path separator; assumes Unix. Revise if Windows support is added.
exe := result[0]
if !filepath.IsAbs(exe) && strings.ContainsRune(exe, '/') {
    result[0] = filepath.Join(workflowDir, exe)
}
```

## Use an errSilentExit sentinel when a subcommand owns its own error output

When a cobra subcommand prints its own error message before returning, return a private sentinel error instead of a descriptive error string. This prevents the parent's error handler from printing a second, redundant "error: <message>" line to the user.

```go
// sentinel — signals the subcommand already printed its error; parent should not re-print.
var errSilentExit = errors.New("silent exit")

func newCreateSandboxCmd(...) *cobra.Command {
    return &cobra.Command{
        RunE: func(cmd *cobra.Command, args []string) error {
            if err := checkDocker(); err != nil {
                fmt.Fprintf(cmd.ErrOrStderr(), "error: %v\n", err)
                return errSilentExit // parent sees non-nil, but suppresses the message
            }
            // ...
            return nil
        },
    }
}

// In the parent's error handler:
if err != nil && !errors.Is(err, errSilentExit) {
    fmt.Fprintf(os.Stderr, "error: %v\n", err)
    os.Exit(1)
}
```

Apply this pattern whenever a subcommand:
1. Needs richer formatting for its error (e.g., multi-line output, bullet lists, suggestions), or
2. Has error context that would be misleadingly terse if re-printed by the generic handler.

The sentinel must be unexported. Only the parent's error gate and the subcommand itself need to reference it.

## Install compile-time interface satisfaction assertions in non-test files

When a concrete type must satisfy an interface and the connection between them is made elsewhere (e.g., in `main.go` or at a distant call site), add a compile-time assertion at the type's declaration site. This catches satisfaction failures immediately — at the package where the type lives — rather than at the distant wiring point.

```go
// Good — assertion at the Runner declaration site, not at main.go:140 where it is used
// Compile-time assertion that *Runner satisfies ui.HeartbeatReader.
var _ ui.HeartbeatReader = (*Runner)(nil)

type Runner struct { ... }
```

The pattern `var _ Interface = (*Type)(nil)` is zero-cost at runtime (the variable is discarded). The compiler rejects the package if `*Type` is missing any method required by `Interface`, reporting the error in the correct package instead of a file far from the type definition.

**Place these assertions in production (non-test) files.** An assertion in a `_test.go` file is only verified when the test binary is built — `go build` and the IDE's type checker never see it, so interface drift silently ships to production. The assertion belongs in the same file as the type declaration or in a small `iface.go` / `statusreader.go` companion file in the same package.

```go
// Bad — placed in a _test.go file; only go test catches drift
// run_iface_test.go:
var _ StatusRunner = (*statusline.Runner)(nil) // invisible to go build

// Good — placed in a production file; go build catches drift immediately
// statusreader.go:
var _ StatusReader = (*Runner)(nil) // caught by go build, go vet, and the IDE
```

Apply any time:
- A concrete type is passed as an interface argument anywhere outside its own package.
- The concrete type implements an interface that is tested via a fake, making it easy for the real type to drift.
- The interface is defined in another package (which is the common case in this codebase — `workflow.Runner` satisfying `ui.HeartbeatReader`).

Consistency note: this codebase also uses `var _ tea.Msg = LogLinesMsg{}` for message types — the same pattern, applied to a struct value rather than a pointer.

## Add public accessors when tests need to observe internal state

When a test needs to verify the value of an unexported field, add a public accessor method rather than reaching through a mutex or accessing the field directly from the test. Direct access from a test file is a form of interface coupling — the test binds to the lock layout and field name, so any refactoring of the mutex structure silently breaks the test.

```go
// Bad — test reaches through h.mu to read private h.prevMode
// keys_test.go:
h.mu.Lock()
got := h.prevMode
h.mu.Unlock()
require.Equal(t, ModeNormal, got)

// Good — add a public accessor with the same mutex protection
// ui.go:
func (h *KeyHandler) PrevMode() Mode {
    h.mu.Lock()
    defer h.mu.Unlock()
    return h.prevMode
}

// keys_test.go:
require.Equal(t, ModeNormal, h.PrevMode())
```

The accessor approach:
- Is safe for concurrent tests (mutex is held correctly).
- Survives field renames and lock-layout refactors.
- Makes the intent explicit — the test reads "observable previous mode", not "internal struct field".
- Follows the same pattern as `SelectJustReleased()`, `ShortcutLine()`, and `StatusLineActive()` in this codebase.

Apply any time a test would otherwise need to access a field that is protected by a mutex or intentionally unexported.

## Unexport when all callers are package-internal

When a function or method is only ever called from within its own package, keep it unexported. Exporting unnecessarily widens the public API surface, constrains future refactoring, and misleads callers in other packages about what the package intends to expose.

```go
// Bad — exported but never called outside the package
func MouseToViewport(x, y, yOffset, height int) (pos, bool) { ... }
func CopyToClipboard(text string) error { ... }

// Good — unexported; callers within the package use it directly
func mouseToViewport(x, y, yOffset, height int) (pos, bool) { ... }
func copyToClipboard(text string) error { ... }
```

Apply this audit whenever you add a function: if you can't name a concrete caller outside the package, make it unexported. If a future need arises to call it externally, export it then — the reverse is a breaking change, but exporting is always addable.

## Gate mode transitions on precondition success

When a UI event triggers a mode transition, verify that the precondition for the new mode is actually satisfied before committing the transition. Entering a mode with uninitialized or zero-value state creates a degenerate display and forces every downstream handler to guard against that degenerate state.

```go
// Bad — enters ModeSelect even when HandleMouse found nothing to select
// (click below content returns a zero-value selection with active=false)
m.log.HandleMouse(p, msg.Action, shift)
m.keys.SetMode(ModeSelect)

// Good — only enter ModeSelect if the selection is actually active
m.log.HandleMouse(p, msg.Action, shift)
if m.log.sel.active {
    m.keys.SetMode(ModeSelect)
}
```

Apply any time a mode represents a meaningful state (e.g., "text is selected", "step is in error") — the transition must be predicated on that state being true, not merely on the event that was supposed to produce it.

## Guard catch-all event branches with explicit type checks

When an `else` or `default` branch handles a specific class of events (e.g., left mouse button), add an explicit guard expression. An unguarded `else` accidentally captures unrelated events (right-click, middle-click) that were never intended to enter that path, silently applying inappropriate logic.

```go
// Bad — right-click and middle-click fall through to the selection handler
if msg.Action == tea.MouseActionPress && isScrollWheel(msg) {
    // scroll
} else {
    m.log.HandleMouse(p, msg.Action, false) // selection — but for any button!
}

// Good — explicit button guard prevents non-left events from triggering selection
if msg.Action == tea.MouseActionPress && isScrollWheel(msg) {
    // scroll
} else if msg.Button == tea.MouseButtonLeft || msg.Button == tea.MouseButtonNone {
    m.log.HandleMouse(p, msg.Action, false)
}
```

Apply whenever a catch-all branch is semantically specific to a subset of values that triggered it.

## Document shared guard invariants above method groups

When a group of methods shares a precondition (e.g., all must bail out when the selection is inactive), document the invariant in a comment block above the first method in the group. Distributing the guard silently across six identical checks is error-prone — adding a seventh method may omit it, and reviewers have no signal that the guard is required.

```go
// Movement methods: MoveSelectionCursor, JumpSelectionCursorToLineStart,
// JumpSelectionCursorToLineEnd, ExtendSelectionByLine, PageSelectionCursor.
//
// INVARIANT: every method in this group must bail out early when
// !m.sel.active && !m.sel.committed. The guard is intentionally
// duplicated in each method (not extracted) so each is self-contained,
// but the requirement is not optional — omitting it on a new method
// corrupts the selection state.
func (m logModel) MoveSelectionCursor(dx, dy int) (logModel, tea.Cmd) {
    if !m.sel.active && !m.sel.committed {
        return m, nil
    }
    // ...
}
```

The comment serves as a checklist: anyone adding a method to the group sees the invariant at the declaration site, not buried in a code-review comment.

## Place helpers in the file that matches their domain

When an unexported helper function's logic belongs to a domain that has its own file, place it in that file rather than the file where you happen to be working. Logic that lives in the wrong file is harder to find, harder to test in isolation, and tends to attract unrelated additions.

```go
// Bad — copySelectedText is a clipboard operation but lives in keys.go
// because that is where the y/Enter case was being implemented
func copySelectedText(text string) tea.Cmd { ... } // in keys.go

// Good — move to clipboard.go where all clipboard operations live
func copySelectedText(text string) tea.Cmd { ... } // in clipboard.go
```

When creating a new helper, ask: "which file would a maintainer look in first for this logic?" Place it there, not in the file where you first needed it.

## Mark incomplete commands Hidden before shipping

When a cobra command's `RunE` body is wired to a stub or placeholder (e.g., a struct that is initialized but never used), set `Hidden: true` on the command before it ships. A command that appears in `--help` output but does nothing when invoked is a silent no-op: users run it, see no output, and assume they did something wrong. `Hidden: true` removes the command from help listings while still allowing explicit invocation during development.

```go
cmd := &cobra.Command{
    Use:    "workflow",
    Short:  "Open the interactive workflow builder",
    Hidden: true, // TUI not yet wired; avoids silent-no-op user surprise
    RunE: func(cmd *cobra.Command, args []string) error {
        // ... partial implementation ...
        return nil
    },
}
```

The same principle applies to scaffolding structs: if you add a struct to hold dependencies for a future integration, remove it before the PR merges. A struct with fields that are written at construction but never read is dead code; it accumulates confusion and forces future readers to verify that the struct is truly unused rather than part of an active code path.

```go
// Bad — added as a placeholder for "future use", never read
type workflowDeps struct {
    log *logger.Logger
}

deps := &workflowDeps{log: log}
_ = deps // explicit suppression is a signal the struct is not ready

// Good — remove the struct entirely until it has real consumers
```

The rule: every exported or package-visible symbol that ships must have at least one real read site. If the read site does not exist yet, keep the symbol out of the main branch.

## Always surface visible feedback when a user mutation is rejected or deferred

When a UI action (editing a field, adding an item, selecting a menu option) cannot complete — whether because of a validation failure, a lossy round-trip, or logic that is not yet implemented — always show the user explicit feedback. Closing the dialog or resetting state without any message leaves the user unable to tell whether the action succeeded.

Three categories where silent no-ops are bugs:

**1. Rejected mutation — the edit would corrupt the value.** Use a boolean second return value to communicate rejection, and set an inline error message the user can see.

```go
// Bad — Command round-trip through strings.Fields is lossy for quoted args;
// the edit is discarded silently and the user has no idea why.
step.Command = strings.Fields(val)
return m, true

// Good — detect the lossy case before committing; surface an editMsg.
for _, arg := range step.Command {
    if strings.ContainsAny(arg, " \t\n\r") {
        m.detail.editMsg = "Command has quoted args — edit in external editor (Ctrl+E)"
        return m, false
    }
}
```

**2. Invalid input format.** When a field expects a structured value (e.g., `key=value`), surface an error on malformed input instead of silently skipping the write.

```go
// Bad — silently drops the edit when '=' is absent.
if len(parts) == 2 {
    step.Env[idx].Key = parts[0]
    step.Env[idx].Value = parts[1]
}

// Good — visible feedback on malformed input.
if len(parts) == 2 {
    step.Env[idx].Key = parts[0]
    step.Env[idx].Value = parts[1]
} else {
    m.detail.editMsg = "Expected key=value format"
    return m, false
}
```

**3. Deferred feature — the code path is not yet implemented.** Show an explicit "not yet implemented" error rather than closing the dialog as if the action succeeded.

```go
// Bad — closes dialog without feedback; user assumes the copy succeeded.
m.dialog = dialogState{}
m.focus = m.prevFocus

// Good — explicit message; user knows the action did not complete.
m.dialog = dialogState{
    kind:    DialogError,
    payload: "Copy from default not yet implemented — use Empty or open an existing workflow",
}
```

**4. Add-item with no visible result.** When an "add item" action creates a new entry, insert a real placeholder value so the addition is immediately visible. An empty map entry or zero-value struct produces no change in the rendered view, making the action appear to do nothing.

```go
// Bad — map entry is never inserted; +Add appears to do nothing.
if m.doc.ContainerEnv == nil {
    m.doc.ContainerEnv = make(map[string]string)
}
// (entry not written — map stays empty looking)

// Good — insert a placeholder that the user can edit.
if m.doc.ContainerEnv == nil {
    m.doc.ContainerEnv = make(map[string]string)
}
m.doc.ContainerEnv["NEW_KEY"] = ""
```

The rule: every code path that handles a user-initiated mutation must end with either a visible write or visible feedback. A silent no-op is never acceptable.

## Additional Information

- [Architecture Overview](../architecture.md) — System-level architecture and design principles
- [Workflow Orchestration](../features/workflow-orchestration.md) — Adapter types (trackingOffsetIterHeader/noopHeader) applying the interface narrowing pattern; CaptureOutput removal from StepExecutor interface as an example of unused-method cleanup; RunHeader phase-specific render methods as the canonical phase-splitting example
- [TUI Status Header](../features/tui-display.md) — Bounds guards on SetStepState; SetPhaseSteps panic-on-overflow as the appropriate choice for programming errors
- [Step Definitions & Prompt Building](../code-packages/steps.md) — Precondition validation on empty PromptFile
- [Subprocess Execution & Streaming](../features/subprocess-execution.md) — Platform-scoped path separator assumption in ResolveCommand; panicking sentinel in NewRunner for missing SetSender
- [Keyboard Input & Error Recovery](../features/keyboard-input.md) — `updateShortcutLineLocked` as the canonical `Locked`-suffix example
- [Error Handling](error-handling.md) — Complementary standards for error message formatting
- [Concurrency](concurrency.md) — Complementary standards for mutex-protected getters (unexported fields)
- [Go Patterns](go-patterns.md) — Complementary Go-specific patterns
- [Testing](testing.md) — Standards for testing bounds guards and nil/uninitialized guard paths
- [Stream JSON Pipeline](../code-packages/claudestream.md) — `var _ ui.HeartbeatReader = (*Runner)(nil)` as the canonical compile-time assertion example (issue #94)
- `src/cmd/pr9k/workflow.go` — `Hidden: true` on the `workflow` cobra command is the canonical incomplete-command example (workflow-builder branch, PR-1 scope)
- `src/internal/workflowedit/model.go` — `commitDetailEdit` returning `(Model, bool)` is the canonical rejected-mutation feedback example; `DialogError` for deferred paths; `"NEW_KEY":""` placeholder for add-item visibility (workflow-builder-pt-2 review issues #4, #6, #7, #9)
