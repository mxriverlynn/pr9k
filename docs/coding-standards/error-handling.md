# Error Handling

## Package-prefixed error messages

Wrap errors with `%w` and prefix the message with the package name. This makes it easy to identify the source of an error without a stack trace.

```go
// Good
return fmt.Errorf("logger: failed to create log file: %w", err)
return fmt.Errorf("workflow: start: %w", err)
return fmt.Errorf("steps: PromptFile must not be empty")

// Bad — no package prefix, no context
return fmt.Errorf("failed to create log file: %w", err)
return err
```

## Include file paths in I/O errors

When an error originates from file I/O, include the file path in the message so the caller can act on it without re-deriving the path.

```go
return fmt.Errorf("steps: read prompt file %s: %w", path, err)
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
