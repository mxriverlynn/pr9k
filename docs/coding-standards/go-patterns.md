# Go Patterns

## Reorder args to work around Go flag package limitations

Go's `flag` package stops parsing at the first non-flag argument. If flags can appear after positional arguments on the command line, use a `reorderArgs` helper to partition the args slice into flags-first, positionals-last before calling `fs.Parse`.

```go
// reorderArgs moves all -flag and --flag entries before positional args.
func reorderArgs(args []string) []string {
    var flags, positionals []string
    for _, a := range args {
        if strings.HasPrefix(a, "-") {
            flags = append(flags, a)
        } else {
            positionals = append(positionals, a)
        }
    }
    return append(flags, positionals...)
}
```

## Resolve binary path with os.Executable + filepath.EvalSymlinks

When a binary needs to locate sibling files (e.g., configs, scripts) relative to itself, use `os.Executable()` followed by `filepath.EvalSymlinks` to get the real path. Skipping `EvalSymlinks` breaks when the binary is installed as a symlink.

```go
exe, err := os.Executable()
if err != nil {
    return "", err
}
exe, err = filepath.EvalSymlinks(exe)
if err != nil {
    return "", err
}
projectDir := filepath.Dir(exe)
```

## Use runtime.Caller(0) in test helpers for path resolution

See [testing.md](testing.md) — `runtime.Caller(0)` is the correct way to resolve paths in test helpers. Do not use `os.Getwd()`.

## Allocate a new slice in transformation functions

When a function transforms a slice (e.g., replacing template variables), allocate a new slice rather than mutating the input. Callers often reuse the original slice across multiple iterations.

```go
func ResolveCommand(projectDir string, command []string, issueID string) []string {
    if len(command) == 0 {
        return command
    }
    result := make([]string, len(command))
    copy(result, command)
    // ... transform result ...
    return result
}
```

## 256KB scanner buffer for subprocess output

When scanning subprocess stdout/stderr, set the scanner buffer to 256KB. The default 64KB buffer causes `token too long` errors on tools that emit long lines (e.g., minified output, base64 blobs).

```go
const scanBufSize = 256 * 1024
scanner := bufio.NewScanner(pipe)
scanner.Buffer(make([]byte, scanBufSize), scanBufSize)
```

## Additional Information

- [Architecture Overview](../architecture.md) — System-level architecture and design principles
- [CLI & Configuration](../features/cli-configuration.md) — Flag reordering in ParseArgs and symlink-safe project directory resolution
- [Subprocess Execution & Streaming](../features/subprocess-execution.md) — 256KB scanner buffer and ResolveCommand slice immutability
- [Step Definitions & Prompt Building](../features/step-definitions.md) — Slice allocation in buildIterationSteps/buildFinalizeSteps
- [Testing](testing.md) — Standards for runtime.Caller(0) in test helpers and input slice immutability tests
- [API Design](api-design.md) — Complementary standards for platform-scoped assumptions
- [Concurrency](concurrency.md) — Complementary concurrency patterns
- [Error Handling](error-handling.md) — Complementary error handling conventions
