# Testing

## Always run with the race detector

All tests must pass with `-race`. The race detector is non-negotiable for any type that uses goroutines, mutexes, channels, or shared state.

```bash
go test -race ./...
```

## Test closeable types for idempotency

Every type with a `Close` method must have a test that calls `Close` twice. The second call must return `nil` and must not panic. This documents the contract and prevents resource-management bugs in callers.

```go
func TestClose_IsIdempotent(t *testing.T) {
    r := newRunner(t)
    require.NoError(t, r.Close())
    require.NoError(t, r.Close()) // must not panic or error
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

For any state-mutating method that indexes into a fixed-size array, test the boundary values: index `-1` and index `N` (length). These tests document the guard and prevent panic regressions.

```go
func TestSetStepState_OutOfBounds(t *testing.T) {
    h := NewStatusHeader(...)
    before := h.Row1
    h.SetStepState(-1, StepDone)  // below lower bound
    h.SetStepState(8, StepDone)   // at upper bound (len == 8)
    require.Equal(t, before, h.Row1) // unchanged
}
```

## Test nil/uninitialized guard paths

For methods that require prior initialization (e.g., calling `SetFinalizeStepState` before `SetFinalization`), add a test that exercises the guard path and verifies a no-op, not a panic.

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

## Verify go vet before committing

Run `go vet ./...` before every commit. Vet catches correctness issues that the compiler does not (e.g., misuse of `sync` types, incorrect format strings).
