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
// newTestKeyHandler creates a KeyHandler with a buffered channel and a no-op
// cancel function. The channel capacity (10) absorbs any actions injected by
// the orchestration loop (non-blocking drains) without deadlocking.
func newTestKeyHandler() *ui.KeyHandler {
    actions := make(chan ui.StepAction, 10)
    return ui.NewKeyHandler(func() {}, actions)
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
// not application logic. Adding a test here would test Bubble Tea, not pr9k.
model := ui.NewModel(header, keyHandler, "pr9k v"+version.Version)
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

## Extract shared test patterns into package-level variables

When the same regex pattern or format string appears in three or more test functions within the same file, extract it to a package-level `var`. Duplicated patterns diverge silently: one test's pattern gets updated while others lag behind, creating invisible gaps in the contract they document.

```go
// Bad — same regex compiled independently in three tests; updating one leaves others stale
func TestLogFileCreatedWithExpectedPattern(t *testing.T) {
    re := regexp.MustCompile(`^ralph-\d{4}-\d{2}-\d{2}-\d{6}\.\d{3}\.log$`)
    // ...
}

func TestRunStampFormat(t *testing.T) {
    re := regexp.MustCompile(`^ralph-\d{4}-\d{2}-\d{2}-\d{6}\.\d{3}$`)
    // ...
}

// Good — extract to a package-level variable; all tests stay in sync
var runStampRe = regexp.MustCompile(`^ralph-\d{4}-\d{2}-\d{2}-\d{6}\.\d{3}$`)

func TestLogFileCreatedWithExpectedPattern(t *testing.T) {
    re := regexp.MustCompile(runStampRe.String() + `\.log$`) // builds on the shared base
    // ...
}

func TestRunStampFormat(t *testing.T) {
    if !runStampRe.MatchString(l.RunStamp()) { ... }
}
```

Apply to regex patterns, format strings, and any other literal that expresses an invariant tested in multiple functions. Two copies of a pattern can diverge; one source of truth cannot.

## Each fake method must have its own call counter

When a fake or spy implements multiple interface methods, give each method its own dedicated counter or capture slice. Sharing a single `logLines` or `calls` slice across methods means a call to `WriteToLog` can satisfy an assertion that was meant to verify `WriteRunSummary`, producing a false pass.

```go
// Bad — WriteToLog and WriteRunSummary share logLines;
// a WriteToLog call passes any assertion that only checks len(logLines)
type fakeExecutor struct {
    logLines []string
}
func (f *fakeExecutor) WriteToLog(line string)      { f.logLines = append(f.logLines, line) }
func (f *fakeExecutor) WriteRunSummary(line string) { f.logLines = append(f.logLines, line) }

// Good — separate counter per method; assertions are unambiguous
type fakeExecutor struct {
    logLines              []string // shared for content inspection
    writeRunSummaryCalls  int      // dedicated call count for WriteRunSummary
}
func (f *fakeExecutor) WriteToLog(line string) {
    f.logLines = append(f.logLines, line)
}
func (f *fakeExecutor) WriteRunSummary(line string) {
    f.writeRunSummaryCalls++
    f.logLines = append(f.logLines, line)
}

// Assertion is now specific:
if exec.writeRunSummaryCalls != 1 {
    t.Errorf("WriteRunSummary called %d times, want 1", exec.writeRunSummaryCalls)
}
```

A shared slice can still be useful for content inspection (verifying what was written), but the call count for each distinct method must be independent so the test distinguishes which method was called.

## Test helpers should return all values callers need

When a test helper constructs a composite value (e.g., a `Model` plus its `*KeyHandler`), return all values that individual test cases will need rather than just the primary one. Returning only the primary value forces callers to duplicate inline construction to obtain the secondary value, and that duplication drifts when the helper changes.

```go
// Bad — returns only Model; tests that need *KeyHandler duplicate the full setup inline
func newSelectTestModel(t *testing.T, mode Mode) Model { ... }

// Good — returns both; callers ignore what they don't need with _
func newSelectTestModel(t *testing.T, mode Mode) (Model, *KeyHandler) { ... }

// Call sites stay concise:
m, _ := newSelectTestModel(t, ModeNormal)       // most tests ignore kh
m, kh := newSelectTestModel(t, ModeNormal)      // tests that need kh get it cleanly
```

When reviewing a test helper: if any test calls the helper and then rebuilds part of its setup to get a value the helper discards, expand the helper's return signature.

## Assert that triggering conditions actually occurred

When a test relies on a side effect (eviction, insertion, truncation) having happened before asserting the behavior that depends on it, add an assertion that the side effect itself occurred. Without it, the test may pass vacuously — producing a green result even when the eviction logic is broken and the buffer never reached capacity.

```go
// Bad — if logRingBufferCap is larger than expected, pushes don't evict and
// the test passes vacuously (it tests nothing about eviction behavior)
for i := 0; i < logRingBufferCap+1; i++ {
    m = pushLine(m, fmt.Sprintf("line %d", i))
}
require.Equal(t, 0, m.log.sel.anchor.rawIdx) // could be trivially true

// Good — assert eviction happened before asserting the dependent behavior
for i := 0; i < logRingBufferCap+1; i++ {
    m = pushLine(m, fmt.Sprintf("line %d", i))
}
require.Equal(t, logRingBufferCap, len(m.log.lines), "eviction must have occurred")
require.Equal(t, 0, m.log.sel.anchor.rawIdx)
```

Apply whenever a test's assertion is meaningful only if a prior operation had a particular effect. State that effect explicitly.

## Comment tests that verify known limitations

When a test verifies behavior that is a known limitation rather than a desired feature — typically a deferred-work item explicitly noted in code comments or the design doc — add a comment in the test body stating this. Without it, future readers may mistake the test for a feature specification and wonder why they can't improve the behavior.

```go
// Good — makes the deferred nature explicit in the test itself
func TestSetSize_DoesNotRecomputeSelectionVisualCoords(t *testing.T) {
    // This test verifies a known limitation (P1 from the design doc):
    // SetSize does not call recomputeSelectionVisualCoords after rewrap,
    // so visual coords on sel.anchor/cursor are stale until the next
    // LogLinesMsg arrives. This is intentional; the recompute-on-resize
    // work is tracked in issue #XXX. Do not "fix" this test without
    // addressing the underlying deferred work.
    ...
}
```

## Use t.Fatal not t.Skip when a skip condition is unreachable

`t.Skip` is for tests that are legitimately skipped under certain runtime conditions (missing tools, wrong OS, etc.). If the skip condition cannot be reached given the test setup, use `t.Fatal` instead. A `t.Skip` on an unreachable branch silently masks regressions — the test body stops executing without any failure signal.

```go
// Bad — skip is unreachable with 20 lines at viewport height=5;
// if the setup changes and this fires, the test silently passes
if m.log.viewport.YOffset != 0 {
    t.Skip("viewport already not at bottom — test precondition not met")
}

// Good — t.Fatal signals a test-setup bug; the condition should never be true
if m.log.viewport.YOffset != 0 {
    t.Fatal("test setup error: viewport is not at bottom before auto-scroll test")
}
```

Ask: "Can this condition ever be true given the test setup?" If no, use `t.Fatal`. Reserve `t.Skip` for conditions that legitimately vary by environment.

## Document no-parallel constraint for package-level var mutation

When tests mutate package-level variables (e.g., function pointers used as seams for dependency injection), those tests must not use `t.Parallel()`. Document this constraint next to the variable declarations in production code so the restriction is visible to everyone who might add a test, not just those who read the test file.

```go
// In clipboard.go — production code:

// copyFn, isTTYFn, and stderrWriter are package-level vars to allow test injection.
// Tests that mutate these vars must NOT call t.Parallel() — concurrent mutation
// produces data races. Each test must save and restore the original value.
var copyFn = clipboard.WriteAll
var isTTYFn = func() bool { return term.IsTerminal(int(os.Stderr.Fd())) }
var stderrWriter io.Writer = os.Stderr
```

The constraint lives in the production file because that is where a new test author discovers the seam. A comment only in the test file is invisible until they've already introduced a parallel test.

## Never hardcode version strings in tests

Tests that verify version correctness must read from the authoritative package constant, not hardcode the expected value as a string literal. Hardcoded version strings cause a class of false positives: after a version bump the hardcoded literal goes stale and the test fails with a misleading message that suggests the bump is wrong, rather than that the test is wrong.

```go
// Bad — hardcodes "0.5.0"; the test fails after every version bump
func TestVersion_IsCurrentRelease(t *testing.T) {
    require.Equal(t, "0.5.0", version.Version)
}

// Good — tests structural correctness; survives any valid version bump
func TestVersion_FollowsSemver(t *testing.T) {
    parts := strings.Split(version.Version, ".")
    require.Len(t, parts, 3, "version must be MAJOR.MINOR.PATCH")
    for _, p := range parts {
        _, err := strconv.Atoi(p)
        require.NoError(t, err, "each version component must be numeric")
    }
}
```

If you need to assert a specific version (e.g., as a one-time audit in a doc-integrity test), express it as `version.Version == "0.5.0"` inside a conditional, not as the expected argument to `require.Equal` — and add a comment explaining that the test will need to be updated after the next version bump.

## Add doc integrity tests when docs embed version strings

When a doc file contains an embedded version string (e.g., a sample payload, an ASCII diagram, a CLI invocation example), add a test in `doc_integrity_test.go` that reads the doc file and asserts the version string matches `version.Version`. Without this pin, the doc drifts silently after every version bump.

```go
// doc_integrity_test.go
func TestDocIntegrity_StatuslineMd_PayloadVersionMatchesCurrent(t *testing.T) {
    root := projectRoot(t)
    data, err := os.ReadFile(filepath.Join(root, "docs/code-packages/statusline.md"))
    require.NoError(t, err)
    want := `"version": "` + version.Version + `"`
    require.Contains(t, string(data), want,
        "docs/code-packages/statusline.md payload example must match current version")
}
```

The test reads the raw file and checks for the exact string, so it fails immediately when:
- A doc is updated to a future version without a corresponding code bump, or
- A version bump forgets to update the doc.

Add one test per doc file that embeds a version literal. Group them in `doc_integrity_test.go` under the same package that owns the version constant.

## Use fixture validation helpers when test setup depends on implementation dimensions

When a test helper constructs a fixture whose correctness depends on a count of rows, columns, or other implementation-defined dimensions (e.g., the number of rows in a rendered modal), add a validation helper that asserts the fixture is consistent with the current implementation. Without it, a future change to the implementation silently produces a broken fixture that causes misleading assertion failures elsewhere in the test.

```go
// Bad — magic height 24 = 6 frame rows + 18 modal rows; breaks silently
// if modal gains or loses a row in the future
func newTestModelModeHelp(t *testing.T) Model {
    m := newTestModel(t)
    m.height = 24 // magic number — will silently break if modal row count changes
    return m
}

// Good — compute the required height and validate it fits
func assertModalFits(t *testing.T, m Model) {
    t.Helper()
    modalH := strings.Count(renderHelpModal(m.width), "\n") + 1
    frameH := m.height
    if modalH > frameH {
        t.Fatalf("test fixture is too short: modal height %d > frame height %d — "+
            "update newTestModelModeHelp height when modal row count changes", modalH, frameH)
    }
}

func newTestModelModeHelp(t *testing.T) Model {
    m := newTestModel(t)
    m.height = 30 // generous floor; assertModalFits will catch if it's still too short
    assertModalFits(t, m)
    return m
}
```

When reviewing test helpers that hardcode a count tied to a rendered output size, add the validation helper so future modal/grid changes produce a clear fixture error instead of a confusing wrong-assertion failure.

## Test that error-returning helpers propagate the error to callers

When you extract a helper function that returns an `error`, add at least one test that seeds a non-nil error and verifies the helper propagates it to the caller. Without this test, a refactor that discards the error (e.g., replacing `return err` with `_ = err; return nil`) compiles cleanly and all other tests still pass.

```go
// wiring.go
func runWithShutdown(prog teaProgram, sd shutdownable, done <-chan struct{}) error {
    runErr := prog.Run()
    sd.Shutdown()
    select {
    case <-done:
    case <-time.After(workflowDoneTimeout):
    }
    return runErr // ← the return that must be pinned by a test
}

// wiring_test.go — pins that runErr is returned, not swallowed
func TestRunWithShutdown_PropagatesRunError(t *testing.T) {
    sentinel := errors.New("run failed")
    prog := &fakeTeaProgram{runErr: sentinel}
    err := runWithShutdown(prog, &fakeShutdownable{}, make(chan struct{}))
    require.ErrorIs(t, err, sentinel)
}
```

Apply whenever a helper wraps an operation that can fail and the caller is expected to act on the error. This is a specialized case of the general "assert that triggering conditions actually occurred" principle.

## Add a test to verify the async pattern when it is critical

When correctness depends on a function being called asynchronously (inside a `tea.Cmd` closure) rather than synchronously in `Update()`, add a test that proves the synchronous path does not call the function. This guards against future refactors that accidentally move the call back into the synchronous path.

```go
// Verifies that copyFn is not called during Update() — only after cmd() is invoked.
// Without this guard, a refactor that moves CopyToClipboard out of the Cmd closure
// would freeze the TUI under slow clipboard daemons and all tests would still pass.
func TestCopySelectedText_CopyFnNotCalledBeforeCmdInvoked(t *testing.T) {
    called := false
    copyFn = func(text string) error { called = true; return nil }
    defer resetClipboardFns()

    cmd := copySelectedText("hello")
    require.False(t, called, "copyFn must not be called before cmd() is invoked")
    _ = cmd() // now invoke the cmd; copyFn must fire
    require.True(t, called)
}
```

Apply this pattern any time a correctness property is "this must happen asynchronously" and a synchronous implementation would compile and pass all other tests.

## Use t.Run subtests for per-item scoping

When a test function iterates over files, keys, or table rows, wrap each iteration body in `t.Run(name, ...)`. Flat test functions fail at the first failing item and hide whether later items pass or fail. Subtests let the runner continue past the first failure and report each item individually.

```go
// Bad — stops at first failure; later files untested in that run
for _, pf := range promptFiles {
    content := pf.Body
    if !strings.Contains(content, hint) {
        t.Errorf("%s: hint not found", pf.Name)
    }
}

// Good — all items run; failure output names the specific subtest
for _, pf := range promptFiles {
    t.Run(pf.Name, func(t *testing.T) {
        if !strings.Contains(pf.Body, hint) {
            t.Errorf("hint not found")
        }
    })
}
```

Apply any time a test loops over a collection and the loop body can independently fail for each element.

## Sort before iterating for deterministic test output

When a test iterates over a map or uses a function that returns results in non-deterministic order, sort the keys or results before iterating. Non-deterministic order produces flaky failure messages that name different items on different runs, making failures hard to reproduce and bisect.

```go
// Bad — map iteration order is non-deterministic; failure message names a random file
for name, body := range promptFiles {
    t.Run(name, func(t *testing.T) { ... })
}

// Good — sorted order; failure output is stable across runs
names := make([]string, 0, len(promptFiles))
for name := range promptFiles {
    names = append(names, name)
}
sort.Strings(names)
for _, name := range names {
    t.Run(name, func(t *testing.T) { ... })
}
```

The same applies to any slice of structs keyed by name: sort by `Name` field before the loop.

## Test the shipped production configuration file end-to-end

Every project that ships a configuration file (`ralph-steps.json`, `config.yaml`, etc.) must have at least one integration test that loads the real file from the source tree and runs it through the full validation and loading pipeline. This closes the gap between "unit tests pass" and "the config the user actually ships is valid."

```go
// production_steps_test.go — loads from the live source tree
func TestValidate_ProductionStepsJSON(t *testing.T) {
    dir := assembleWorkflowDir(t) // points at src/ in source tree
    errs := validator.Validate(dir)
    for _, e := range errs {
        if e.IsFatal() {
            t.Errorf("fatal: %s", e.Error())
        }
    }
}
```

Checklist:
1. The test must load the file from its real source-tree path (via `runtime.Caller(0)` resolution, not a test fixture copy).
2. The test must assert zero fatal errors.
3. The test must be updated whenever the config schema gains a new field that requires a value — a schema change that breaks the production file should fail this test immediately, not after a deploy.

## Additional Information

- [Architecture Overview](../architecture.md) — System-level architecture and interface-driven testability design principle; assembly-only wiring in main.go (issues #49, #50)
- [Workflow Orchestration](../features/workflow-orchestration.md) — `TestIterationLabel` as an example of a test name matching full scope (bounded + unbounded); fakeRunHeader as the canonical capturing fake pattern
- [File Logging](../code-packages/logger.md) — Close idempotency testing applied to Logger
- [TUI Status Header](../features/tui-display.md) — Bounds guard testing on SetStepState; phase transition testing via SetPhaseSteps; grid layout structural variants (TP-001–TP-005)
- [Subprocess Execution & Streaming](../features/subprocess-execution.md) — WasTerminated flag reset testing, input slice immutability in ResolveCommand; stdout-only capture contract (D4) tested via TestLastCapture_StderrNotCaptured
- [Keyboard Input & Error Recovery](../features/keyboard-input.md) — Test doubles with shared state (spy patterns with mutexes); newTestKeyHandler as the canonical async signal injection pattern
- [Workflow Orchestration](../features/workflow-orchestration.md) — continue-on-error recovery tested in TestRun_InitializeBuildErrorContinuesToNextInitStep; positive scope visibility in TestRun_InitializeCaptureAvailableInIteration
- [Config Validation](../code-packages/validator.md) — Positive and negative scope-visibility tests for variable table phase propagation
- [Go Patterns](go-patterns.md) — Complementary Go-specific patterns including runtime.Caller(0) usage
- [Concurrency](concurrency.md) — Complementary concurrency patterns that tests must verify; channel priming before blocking receives
- [API Design](api-design.md) — Standards for bounds guards and nil guards that need explicit tests; public accessors over private field access from tests
- [Error Handling](error-handling.md) — Standards for file I/O errors that need test coverage
- [Documentation](documentation.md) — Doc integrity test patterns for files with embedded version strings
- [File Logging](../code-packages/logger.md) — `runStampRe` package-level variable as the canonical shared-regex example (issue #90)
- [Stream JSON Pipeline](../code-packages/claudestream.md) — `fakeExecutor.writeRunSummaryCalls` counter added to distinguish `WriteRunSummary` from `WriteToLog` call assertions (issue #93)
- [Status Line](../code-packages/statusline.md) — `TestRunWithShutdown_PropagatesRunError` as the canonical error-propagation test example; `assertModalFits` as the canonical fixture validation helper example (issue #118/119)
- [Config Validation](../code-packages/validator.md) — `prompts_structure_test.go` as the canonical t.Run + sorted-iteration test example (issue #125); `production_steps_test.go` as the canonical production-config integration test (issue #124)
