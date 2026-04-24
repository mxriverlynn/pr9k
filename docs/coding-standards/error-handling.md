# Error Handling

## Package-prefixed error messages

Wrap errors with `%w` and prefix the message with the package name. This makes it easy to identify the source of an error without a stack trace.

```go
// Good
return fmt.Errorf("logger: could not create log file: %w", err)
return fmt.Errorf("workflow: start: %w", err)
return fmt.Errorf("steps: PromptFile must not be empty")

// Bad — no package prefix, no context
return fmt.Errorf("failed to create log file: %w", err)
return err
```

## Include file paths in I/O errors

When an error originates from file I/O, include the file path in the message so the caller can act on it without re-deriving the path.

```go
return fmt.Errorf("steps: could not read prompt %s: %w", promptPath, err)
```

## Explicit precondition validation

Validate preconditions with an explicit error rather than relying on OS or platform-specific behavior. Platform-implicit failures (e.g., reading a directory when a file path was expected) produce opaque errors and vary across environments.

```go
// Good — explicit, cross-platform
if step.PromptFile == "" {
    return "", fmt.Errorf("steps: PromptFile must not be empty")
}

// Bad — relies on OS directory-read behavior, error message is unclear
content, err := os.ReadFile(filepath.Join(projectDir, "prompts", step.PromptFile))
```

## Check scanner errors after scan loops

After a `bufio.Scanner` loop, always check `scanner.Err()`. An unchecked scanner error silently drops all remaining input with no indication of failure.

```go
for scanner.Scan() {
    // handle line
}
if err := scanner.Err(); err != nil {
    // log or propagate
}
```

## Track goroutine write errors — do not discard silently

In forwarding goroutines, track the first error rather than discarding every write error. Silent discard makes data loss undetectable.

```go
// Good — track first error
var firstErr error
for scanner.Scan() {
    if _, err := fmt.Fprintln(w, scanner.Text()); err != nil && firstErr == nil {
        firstErr = err
    }
}
```

## bufio.Writer error surfacing

`bufio.Writer` buffers writes; errors from individual writes may not surface until `Flush` or `Close`. Document this in API comments so callers are not surprised when per-write errors are not returned.

## Log warnings for discarded external command output

When a call to `CaptureOutput` (or any helper that runs an external command to fetch a value) fails, log a warning rather than silently continuing with an empty string. Silent discard makes failures invisible in production logs and is indistinguishable from a command that legitimately returned an empty result.

```go
username, err := executor.CaptureOutput([]string{"get_gh_user"})
if err != nil {
    logger.Log("warn: get_gh_user failed: " + err.Error())
}
// username may be empty — callers must tolerate that
```

## Use errors.Is for specific error types — not os.IsPermission or similar helpers

Use `errors.Is(err, fs.ErrPermission)` (or the equivalent `errors.As`) to inspect specific error types when errors may be wrapped with `fmt.Errorf("%w")`. Legacy helpers like `os.IsPermission` and `os.IsNotExist` do **not** traverse wrapped error chains — they only match the outermost error. Once you wrap an error with `%w`, those helpers return false even when the underlying cause is a permission denial.

```go
// Bad — os.IsPermission does not see through fmt.Errorf("%w") wrapping
if os.IsPermission(err) {
    return fmt.Errorf("preflight: not permitted: %w", err)
}

// Good — errors.Is traverses the wrapped chain
if errors.Is(err, fs.ErrPermission) {
    return fmt.Errorf("preflight: not permitted: %w", err)
}
```

Apply `errors.Is(err, fs.ErrPermission)` anywhere you wrap errors with `%w` and need to check the cause downstream. The same applies to `fs.ErrNotExist` instead of `os.IsNotExist`.

## Validate file paths stay within their expected directory

When a config or user-supplied field resolves to a file path (e.g., `PromptFile` in a step definition), confirm that the resolved path remains inside the expected root directory before opening it. A relative path containing `..` can escape the root and read arbitrary files.

**Use `filepath.EvalSymlinks` on both sides.** A `filepath.Rel` check alone is not sufficient: a symlink within the path can make an escaping path look contained. Resolve both the root directory and the candidate path through `EvalSymlinks` before comparing.

```go
// pathContainedIn returns an ErrPathEscape-wrapped error if candidate does not
// resolve to a path strictly inside dir.
func pathContainedIn(dir, candidate string) error {
    absDir, _ := filepath.Abs(dir)
    absCand, _ := filepath.Abs(candidate)

    // EvalSymlinks on both sides — a symlink in either path can defeat a bare
    // prefix check. Fall back to abs path when target doesn't exist yet.
    resolvedDir, err := filepath.EvalSymlinks(absDir)
    if err != nil {
        resolvedDir = absDir
    }
    resolvedCand, err := filepath.EvalSymlinks(absCand)
    if err != nil {
        resolvedCand = absCand
    }

    if !strings.HasPrefix(resolvedCand, resolvedDir+string(filepath.Separator)) {
        return fmt.Errorf("%w: %s", ErrPathEscape, candidate)
    }
    return nil
}
```

The same principle applies to any prefix-based containment check (e.g., `DetectExternalWorkflow`): resolve both the base directory and the candidate through `EvalSymlinks` before comparing with `strings.HasPrefix`. Skipping symlink resolution on either side allows a symlinked directory inside the root to appear external, or a symlink pointing outside the root to appear internal.

Apply at every boundary where a file path is supplied by config or user input and opened from within a bounded directory.

## Check accumulated error accessors after deferred Close

When a type accumulates background write errors and exposes them via an accessor (e.g., `WriteErr()`), check the accessor after calling `Close()` in the defer. Implementing the tracker is only half the job — if the caller never reads it, data loss remains silent.

```go
// Bad — WriteErr() is tracked but never checked; silent data loss on artifact write failures
defer pipeline.Close()

// Good — check WriteErr after Close so disk failures are logged
defer func() {
    pipeline.Close()
    if wErr := pipeline.WriteErr(); wErr != nil {
        r.sendLine("[artifact] write error: " + wErr.Error())
    }
}()
```

This is the caller-side complement to the "track goroutine write errors" standard. Together they form a complete chain: the implementer tracks the first error into an accessor, and the caller reads the accessor at the cleanup point. Either half alone is insufficient.

The log message should match the naming pattern of nearby I/O error messages (e.g., `[artifact] open failed: ...` and `[artifact] write error: ...` are in the same family and share a prefix).

## Use errors.As for type-checking errors, not type assertions

When inspecting whether an error (or a wrapped error) is a specific type, use `errors.As` rather than a direct type assertion (`err.(*T)`). Type assertions only match the outermost error — they return `false` for any error wrapped with `fmt.Errorf("%w", ...)`. `errors.As` traverses the full chain.

```go
// Bad — assertion misses wrapped *exec.ExitError
exitErr, ok := err.(*exec.ExitError)
if !ok {
    return -1
}

// Good — unwraps the chain; works even when err is wrapped
var exitErr *exec.ExitError
if errors.As(err, &exitErr) {
    return exitErr.ExitCode()
}
```

Apply to every error type check in production code and test helpers. Direct assertions are acceptable only when you control the error source and know it is never wrapped (rare).

## Distinguish fatal and non-fatal findings in validators

When a validator collects multiple findings, separate them by severity rather than failing on the first one. A startup gate that blocks on warnings forces users to fix cosmetic issues before they can run at all.

Recommended pattern:
- **Fatal** (`IsFatal() == true`): structurally invalid config; the process cannot run correctly. Block startup and print all fatal errors.
- **Warning / Info**: sub-optimal or risky config; the process can run. Print to stderr and continue.

```go
type Severity int
const (
    SeverityError Severity = iota // fatal
    SeverityWarning
    SeverityInfo
)

type Error struct {
    Severity Severity
    // ...
}

func (e Error) IsFatal() bool { return e.Severity == SeverityError }

// In main: gate startup only on fatals
errs := validator.Validate(cfg)
for _, e := range errs {
    if e.IsFatal() {
        fmt.Fprintln(os.Stderr, e.Error())
    } else {
        fmt.Fprintln(os.Stderr, "warning: "+e.Error())
    }
}
if validator.FatalErrorCount(errs) > 0 {
    os.Exit(1)
}
```

Apply to any validator that collects findings rather than returning on the first error. The pattern prevents a class of "validator blocks everything" regressions when a new warning rule is added.

## Additional Information

- [Architecture Overview](../architecture.md) — System-level architecture and design principles
- [Subprocess Execution & Streaming](../features/subprocess-execution.md) — Scanner error checking, goroutine write error tracking, and package-prefixed error messages
- [Step Definitions & Prompt Building](../code-packages/steps.md) — Package-prefixed errors and file paths in I/O errors for step/prompt loading
- [File Logging](../code-packages/logger.md) — bufio.Writer error surfacing on close and package-prefixed logger errors
- [CLI & Configuration](../features/cli-configuration.md) — Error messages for invalid arguments and workflow/project directory resolution failures
- [Workflow Orchestration](../features/workflow-orchestration.md) — Warning logs for discarded CaptureOutput failures (get_gh_user, get_next_issue)
- [API Design](api-design.md) — Complementary standards for precondition validation
- [Concurrency](concurrency.md) — Complementary standards for goroutine error handling
- [Testing](testing.md) — Standards for testing all file I/O error paths
- [Stream JSON Pipeline](../code-packages/claudestream.md) — `pipeline.WriteErr()` check after `pipeline.Close()` as the canonical accumulated-error accessor example (issue #91)
- [Docker Sandbox](../features/docker-sandbox.md) — `errors.As` for `*exec.ExitError` in bash-subprocess test helpers (issues #127, #128)
- [Config Validation](../code-packages/validator.md) — `IsFatal` / `FatalErrorCount` / `Severity` model as the canonical non-fatal severity tier implementation (issue #122)
- [Workflow IO](../code-packages/workflowio.md) — `pathContainedIn` as the canonical EvalSymlinks-on-both-sides containment check; `DetectExternalWorkflow` as the canonical prefix-check with symlink resolution on both sides
