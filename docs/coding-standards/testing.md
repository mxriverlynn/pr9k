# Testing

## Always run with the race detector

All tests must pass with `-race`. The race detector is non-negotiable for any type that uses goroutines, mutexes, channels, or shared state.

```bash
go test -race ./...
```

## Test closeable types for idempotency

Every type with a `Close` method must have a test that calls `Close` twice. The second call must return `nil` and must not panic. This documents the contract and prevents resource-management bugs in callers.

```go
func TestCloseIsIdempotent(t *testing.T) {
    l, err := NewLogger(t.TempDir())
    require.NoError(t, err)
    require.NoError(t, l.Close())
    require.NoError(t, l.Close()) // must not panic or error
}
```

## Test input slice immutability

When a function produces a transformed copy of a slice rather than mutating in place, verify that the original slice is unchanged. Callers (such as iterative loops over step definitions) rely on this contract.

```go
func TestResolveCommand_DoesNotMutateInput(t *testing.T) {
    input := []string{"script/{{ISSUE_ID}}", "{{ISSUE_ID}}"}
    original := slices.Clone(input)
    _ = ResolveCommand(projectDir, input, "42")
    require.Equal(t, original, input)
}
```

## Test array bounds guards explicitly

For any state-mutating method that indexes into a fixed-size array or slice, test the boundary values: index `-1` and index `N` (length). These tests document the guard and prevent panic regressions.

```go
func TestSetStepState_OutOfBoundsIsNoOp(t *testing.T) {
    h := NewStatusHeader(4)  // 4 steps → 1 row of 4 slots
    h.SetPhaseSteps([]string{"a", "b", "c", "d"})
    before := h.Rows[0]
    h.SetStepState(-1, StepDone)  // below lower bound
    h.SetStepState(4, StepDone)   // at upper bound (len == 4)
    require.Equal(t, before, h.Rows[0]) // unchanged
}
```

## Test nil/uninitialized guard paths

For methods that require prior initialization (e.g., calling `SetStepState` before `SetPhaseSteps`), add a test that exercises the guard path and verifies a no-op, not a panic.

## Test all file I/O error paths

Every function that reads or writes files must have tests for:
- File not found / missing path
- Unwritable directory (for `os.MkdirAll` / `os.Create` paths)
- Malformed content (for JSON or structured file parsers)

## Use runtime.Caller(0) for test helper path resolution

Use `runtime.Caller(0)` to resolve the test file's own path, then derive the project root from it. `os.Getwd()` returns the package directory during `go test` — which is correct locally — but breaks in some CI environments and when tests are run from a different working directory.

```go
func projectRoot(t *testing.T) string {
    t.Helper()
    _, file, _, ok := runtime.Caller(0)
    require.True(t, ok)
    // file is e.g. /abs/path/to/pkg/foo_test.go
    // walk up to repo root as needed
    return filepath.Join(filepath.Dir(file), "..", "..")
}
```

## Test doubles with shared state need mutexes

Spy and fake types used in tests are not exempt from the race detector. If a spy collects calls in a slice that is written by one goroutine and read by another (e.g., in an assertion after the goroutine finishes), protect the slice with a `sync.Mutex`. A data race in a test double produces false results as reliably as a race in production code.

```go
type spyHeader struct {
    mu    sync.Mutex
    calls []string
}

func (s *spyHeader) SetStepState(idx int, state StepState) {
    s.mu.Lock()
    defer s.mu.Unlock()
    s.calls = append(s.calls, fmt.Sprintf("step %d=%v", idx, state))
}

func (s *spyHeader) getCalls() []string {
    s.mu.Lock()
    defer s.mu.Unlock()
    return slices.Clone(s.calls)
}
```

## Test names must match actual test scope

Name tests by what they actually exercise. A test named `TestFoo_BoundedMode` that also covers the unbounded path misleads reviewers and future maintainers about what is and isn't tested.

- If a test covers a single path, name it for that path: `TestIterationLabel_Bounded`, `TestIterationLabel_Unbounded`.
- If a test covers multiple paths in one table, name it for the function: `TestIterationLabel` or `TestIterationLabel_Modes`.
- Never let the name claim narrower coverage than the test body provides — that gap is how blind spots form.

```go
// Bad: name implies bounded-only, but the table includes unbounded rows
func TestIterationLabel_BoundedMode(t *testing.T) { ... }

// Good: name matches actual scope
func TestIterationLabel(t *testing.T) { ... }
```

## Test both positive and negative cases for scope/visibility rules

When testing whether a value is visible in a given scope, test both directions: that it IS visible where it should be, and that it is NOT visible where it should not be. Testing only the negative case leaves the positive contract unverified — a bug in the propagation path will be invisible.

```go
// Bad: only the negative direction is tested
func TestValidate_IterCaptureNotInFinalize(t *testing.T) { ... }

// Good: both directions are tested
func TestValidate_IterCaptureNotInFinalize(t *testing.T) { ... }
func TestValidate_InitCaptureVisibleInFinalize(t *testing.T) { ... }
```

This pattern arises wherever data flows through phases, scopes, or propagation rules (e.g., variable tables, permission systems, initialization ordering).

## Test continue-on-error recovery explicitly

When an error loop uses `continue` rather than `return` or `break`, add a test that verifies the loop continues past the failing item and processes subsequent items. The `continue` contract is distinct from `return` and is not implicitly verified by tests that only check the error message.

```go
// run.go uses continue on buildStep failure during the initialize phase:
//   if err != nil { log.Printf(...); continue }
//
// This test must confirm that subsequent init steps AND the iteration loop still run.
func TestRun_InitializeBuildErrorContinuesToNextInitStep(t *testing.T) {
    // First init step fails to build; second init step and iteration steps must still execute.
    ...
}
```

## Avoid time.Sleep for test synchronization

Do not use `time.Sleep` to wait for goroutines or background work in tests. Sleep-based synchronization is inherently racy: it fails under load and passes when the system happens to be fast enough.

Use channels, `sync.WaitGroup`, or other signaling primitives instead. If a test currently uses sleep as a pragmatic shortcut, note it explicitly and expect to replace it if the test becomes flaky.

```go
// Bad — inherently racy under load
time.Sleep(30 * time.Millisecond)
require.Equal(t, expected, actual)

// Good — deterministic signal
done := make(chan struct{})
go func() { defer close(done); doWork() }()
<-done
require.Equal(t, expected, actual)
```

## Fake methods must capture calls, not swallow them

When a fake or test double implements an interface method that a test needs to assert was invoked (with the correct arguments), the method must record its calls into a slice. A silent no-op makes tests pass even when the production code never calls the method at all — creating a gap between "all tests pass" and "code is correct."

```go
// Bad — no-op stub hides whether RenderInitializeLine is ever called
func (h *fakeRunHeader) RenderInitializeLine(stepNum, stepCount int, stepName string) {}

// Good — captures every call so tests can assert on them
type renderPhaseCall struct {
    stepNum, stepCount int
    stepName           string
}

type fakeRunHeader struct {
    renderInitializeCalls []renderPhaseCall
}

func (h *fakeRunHeader) RenderInitializeLine(stepNum, stepCount int, stepName string) {
    h.renderInitializeCalls = append(h.renderInitializeCalls, renderPhaseCall{stepNum, stepCount, stepName})
}
```

Capturing is always the right default. If a test genuinely does not need to assert on a particular method, it can ignore the slice — but capturing prevents a future test from having to restructure the fake.

## Update all fakes when an interface gains a new method

When a new method is added to an interface, immediately update every test double that implements it to capture calls rather than compiling a no-op stub. A stub that does nothing is worse than a missing stub: it makes tests pass silently for code paths that are never actually exercised.

```
Checklist when extending an interface:
1. Add the method to the interface.
2. Add a capturing implementation to every fake/spy.
3. Add or update at least one test that asserts on the captured calls.
```

If step 3 is deferred (because the production caller doesn't exist yet), document the gap explicitly in `deferred.txt` — do not leave a no-op stub with no note.

## Inject an additional signal for each new blocking receive

When you add a new blocking channel receive (`<-ch`) to code already covered by tests, every test that exercises that code path must send one additional signal to unblock it. Failure to do so causes the test to hang.

```go
// Before: Run() has no blocking receive — newTestKeyHandler injects no signals
func newTestKeyHandler() *ui.KeyHandler { ... }

// After: Run() added <-keyHandler.Actions as its completion handoff —
// inject ActionQuit asynchronously so it unblocks without racing the
// non-blocking pre-step drains that Orchestrate performs.
func newTestKeyHandler() *ui.KeyHandler {
    actions := make(chan ui.StepAction, 10)
    kh := ui.NewKeyHandler(func() {}, actions)
    go func() {
        time.Sleep(10 * time.Millisecond)
        actions <- ui.ActionQuit
    }()
    return kh
}
```

Count the blocking receives in the path under test, and ensure the test injects that many signals. When reviewing a PR that adds a blocking receive, check whether existing tests need an additional send.

## Do not test assembly-only code in func main()

When `func main()` only wires together already-tested components — no new logic, no new error paths, no new state machines — adding tests requires either mocking the framework or extracting premature abstractions for one-time assembly code. Neither is worthwhile.

Signs that code falls into this category:
- Every call site is a constructor or configuration call on a type that already has its own tests.
- There is no conditional logic, error branching, or transformation that hasn't been tested elsewhere.
- Extracting the wiring into a function would exist solely to make it testable, not because it is reused.

```go
// Example: assembling a Bubble Tea program from tested components.
// StatusHeader, KeyHandler, Runner, and workflow.Run all have thorough tests.
// The wiring itself — constructing the Model, calling program.Run — is framework usage,
// not application logic. Adding a test here would test Bubble Tea, not ralph-tui.
model := ui.NewModel(header, keyHandler, "ralph-tui v"+version.Version)
program := tea.NewProgram(model, tea.WithMouseCellMotion(), tea.WithAltScreen(), tea.WithoutSignalHandler())
_, err = program.Run()
```

This standard applies to `func main()` and to any analogous one-time assembly function that exclusively assembles tested components without adding new behavior.

## Verify go vet before committing

Run `go vet ./...` before every commit. Vet catches correctness issues that the compiler does not (e.g., misuse of `sync` types, incorrect format strings).

## Test grid layout with structural variants

When testing multi-row grid rendering, cover five structural variants. Each targets a distinct failure mode that single-case tests miss:

| Variant | What it catches |
|---|---|
| Multi-row with unequal names | Global max computed per-row instead of globally — later rows misalign |
| Sparse trailing cells (last row not full) | Empty slots collapsing column width for that row |
| All names equal width | Padding added unnecessarily, inflating row width |
| Long name with truncation active | Truncation interacting with padding, producing wrong widths |
| Single item | Off-by-one on column index 0; degenerate row with no separators |

```go
// Multi-row: maxCellWidth must use the wider name from row 1, not the shorter names in row 0.
func TestView_CheckboxGrid_MultiRow_GlobalMaxCellWidth(t *testing.T) { ... }

// Sparse: empty trailing slot in row 1 must still align with the filled slots in row 0.
func TestView_CheckboxGrid_EmptyTrailingCells_AlignWithFilledRow(t *testing.T) { ... }

// Equal: no extra spaces when all names are the same width.
func TestView_CheckboxGrid_EqualWidthNames_NoPaddingWithinCells(t *testing.T) { ... }

// Truncation: verify padding is added after truncation, not before.
func TestView_CheckboxGrid_LongNames_TruncatedToTerminalWidth(t *testing.T) { ... }

// Single item: must not crash and cell must appear at offset 0.
func TestView_CheckboxGrid_SingleStep_NoCrashAndCellAtOffset0(t *testing.T) { ... }
```

Apply this checklist any time you write or modify a grid/table rendering function that pads or aligns cells.

## Test validation rules across all applicable lifecycle phases

When a validator rule applies to multiple lifecycle phases (initialize, iteration, finalize), write a test for each phase separately — not just the one that was convenient at the time. Phase-specific behavior is a common source of gaps: a rule may fire correctly in the iteration phase but silently miss the initialize or finalize phase due to how the validation loop is structured.

```go
// Covers iteration — but that's only one of three phases.
func TestValidate_RuleA_CaptureAsOnClaudeStepInIterationPhase(t *testing.T) { ... }

// Required: cover all phases the rule applies to.
func TestValidate_RuleA_CaptureAsOnClaudeStepInInitializePhase(t *testing.T) { ... }
func TestValidate_RuleA_CaptureAsOnClaudeStepInFinalizationPhase(t *testing.T) { ... }
```

Checklist when adding a new validator rule:
1. Identify all phases the rule applies to.
2. Write one test per phase.
3. If the implementation shares a single `validatePhase` helper, a per-phase test still matters — it verifies the helper is actually called for each phase and receives the right data.

The same principle applies to any code that processes items phase-by-phase: a loop over phases can silently skip a phase if the data structure is inconsistent.

## Test empty-array vs. absent-key distinction for optional JSON fields

When a Go struct field is a slice (`[]T`) with an `omitempty` JSON tag (or no tag), test both:
- **Absent key** — field omitted from JSON entirely → should deserialize to `nil`
- **Empty array** — key present with `[]` value → should deserialize to a non-nil empty slice

These two states often need different treatment downstream (e.g., "user did not specify" vs. "user explicitly said none"), and the distinction is easy to lose when one case is untested.

```go
func TestLoadSteps_EnvAbsentKeyIsNil(t *testing.T) {
    // JSON step with no "env" key
    step := loadStep(t, `{"name":"s","isClaude":true,"command":["c"]}`)
    require.Nil(t, step.Env)
}

func TestLoadSteps_EnvEmptyArrayIsNonNil(t *testing.T) {
    // JSON step with "env": []
    step := loadStep(t, `{"name":"s","isClaude":true,"command":["c"],"env":[]}`)
    require.NotNil(t, step.Env)
    require.Empty(t, step.Env)
}
```

Apply whenever a new optional slice field is added to a JSON-deserialized struct.

## Additional Information

- [Architecture Overview](../architecture.md) — System-level architecture and interface-driven testability design principle; assembly-only wiring in main.go (issues #49, #50)
- [Workflow Orchestration](../features/workflow-orchestration.md) — `TestIterationLabel` as an example of a test name matching full scope (bounded + unbounded); fakeRunHeader as the canonical capturing fake pattern
- [File Logging](../features/file-logging.md) — Close idempotency testing applied to Logger
- [TUI Status Header](../features/tui-display.md) — Bounds guard testing on SetStepState; phase transition testing via SetPhaseSteps; grid layout structural variants (TP-001–TP-005)
- [Subprocess Execution & Streaming](../features/subprocess-execution.md) — WasTerminated flag reset testing, input slice immutability in ResolveCommand; stdout-only capture contract (D4) tested via TestLastCapture_StderrNotCaptured
- [Keyboard Input & Error Recovery](../features/keyboard-input.md) — Test doubles with shared state (spy patterns with mutexes); newTestKeyHandler as the canonical async signal injection pattern
- [Workflow Orchestration](../features/workflow-orchestration.md) — continue-on-error recovery tested in TestRun_InitializeBuildErrorContinuesToNextInitStep; positive scope visibility in TestRun_InitializeCaptureAvailableInIteration
- [Config Validation](../features/config-validation.md) — Positive and negative scope-visibility tests for variable table phase propagation
- [Go Patterns](go-patterns.md) — Complementary Go-specific patterns including runtime.Caller(0) usage
- [Concurrency](concurrency.md) — Complementary concurrency patterns that tests must verify; channel priming before blocking receives
- [API Design](api-design.md) — Standards for bounds guards and nil guards that need explicit tests
- [Error Handling](error-handling.md) — Standards for file I/O errors that need test coverage
