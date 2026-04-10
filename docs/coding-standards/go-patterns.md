# Go Patterns

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

## Extract conditional format strings to a named pure helper

When the same conditional formatting decision (e.g., bounded vs. unbounded, singular vs. plural) appears in multiple log or UI call sites, extract it as a named unexported function rather than repeating the condition inline. This makes the formatting logic independently testable and keeps the condition in one place.

```go
// iterationLabel returns "Iteration N/M" for bounded mode or "Iteration N" for unbounded (total == 0).
func iterationLabel(i, total int) string {
    if total > 0 {
        return fmt.Sprintf("Iteration %d/%d", i, total)
    }
    return fmt.Sprintf("Iteration %d", i)
}

// Call sites stay readable and stay in sync automatically:
executor.WriteToLog(fmt.Sprintf("%s — No issue found.", iterationLabel(i, cfg.Iterations)))
executor.WriteToLog(ui.StepSeparator(fmt.Sprintf("%s — Issue #%s", iterationLabel(i, cfg.Iterations), issueID)))
```

## Cobra: share command definition with an unexported impl helper

When you need both a `NewCommand()` (for tests or embedding) and an `Execute()` (for `main`), extract an unexported `newCommandImpl(cfg *T, ranE *bool)` that builds the `cobra.Command`. Set `*ranE = true` at the top of `RunE`. `Execute()` uses this flag to distinguish a clean `--help` exit (RunE was never invoked) from a normal successful parse.

```go
func newCommandImpl(cfg *Config, ranE *bool) *cobra.Command {
    cmd := &cobra.Command{
        RunE: func(cmd *cobra.Command, args []string) error {
            *ranE = true
            // validation ...
            return nil
        },
    }
    // flag definitions ...
    return cmd
}

func NewCommand(cfg *Config) *cobra.Command {
    return newCommandImpl(cfg, new(bool))
}

func Execute() (*Config, error) {
    cfg := &Config{}
    var ranE bool
    cmd := newCommandImpl(cfg, &ranE)
    if err := cmd.Execute(); err != nil {
        return nil, err
    }
    if !ranE {
        return nil, nil // --help was invoked; RunE was skipped
    }
    return cfg, nil
}
```

## Cobra: guard against nil config returned by Execute()

`cobra.Command.Execute()` short-circuits when `--help` is requested — it prints usage and returns nil without running `RunE`. Any wrapper that returns a config struct must signal this to its caller. The caller must guard before dereferencing.

```go
// In main:
cfg, err := cli.Execute()
if err != nil {
    fmt.Fprintf(os.Stderr, "Error: %v\nRun 'ralph-tui --help' for usage.\n", err)
    os.Exit(1)
}
if cfg == nil {
    os.Exit(0) // --help was shown; nothing to do
}
```

Do not treat `(nil, nil)` as an error — it is the documented --help path.

## Pre-populate TUI widget state before starting the event loop

When a TUI framework renders widgets by reading from pointer-bound fields, populate those fields before calling the blocking event-loop start function (e.g., `app.Run()`). The first rendered frame reads from those pointers immediately. Uninitialized values produce blank or misleading content on the first frame.

```go
// Pre-populate before app.Run() so the first frame shows real content.
if len(stepFile.Initialize) > 0 {
    header.SetPhaseSteps(stepNames(stepFile.Initialize))
    header.SetStepState(0, ui.StepActive)
    header.IterationLine = "Initializing 1/" + strconv.Itoa(len(stepFile.Initialize)) + ": " + stepFile.Initialize[0].Name
} else {
    header.SetPhaseSteps(stepNames(stepFile.Iteration))
    header.SetStepState(0, ui.StepActive)
    header.IterationLine = "Iteration 1"
}

app := glyph.NewApp()
// ... bind widgets to header fields ...
app.Run() // first frame renders from already-populated state
```

This mirrors the general principle that display state should be initialized before the display is activated, not after.

## Additional Information

- [Architecture Overview](../architecture.md) — System-level architecture and design principles
- [CLI & Configuration](../features/cli-configuration.md) — Symlink-safe project directory resolution in `resolveProjectDir`; cobra Execute nil guard in `main.go`
- [Workflow Orchestration](../features/workflow-orchestration.md) — `iterationLabel` conditional format helper applied across log call sites
- [TUI Display & Glyph Wiring](../features/tui-display.md) — Pre-populate TUI widget state before app.Run() (issue #50)
- [Subprocess Execution & Streaming](../features/subprocess-execution.md) — 256KB scanner buffer and ResolveCommand slice immutability
- [Step Definitions & Prompt Building](../features/step-definitions.md) — Slice allocation in buildIterationSteps/buildFinalizeSteps
- [Testing](testing.md) — Standards for runtime.Caller(0) in test helpers and input slice immutability tests
- [API Design](api-design.md) — Complementary standards for platform-scoped assumptions
- [Concurrency](concurrency.md) — Complementary concurrency patterns
- [Error Handling](error-handling.md) — Complementary error handling conventions
