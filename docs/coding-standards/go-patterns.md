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

## Pre-populate TUI model state before starting the event loop

Populate the Bubble Tea model's initial state before calling `program.Run()`. The first rendered frame calls `View()` immediately, so uninitialized state produces blank or misleading content on the first frame.

```go
// Pre-populate before program.Run() so the first frame shows real content.
if len(stepFile.Initialize) > 0 {
    header.SetPhaseSteps(stepNames(stepFile.Initialize))
    header.SetStepState(0, ui.StepActive)
    header.RenderInitializeLine(1, len(stepFile.Initialize), stepFile.Initialize[0].Name)
} else {
    header.SetPhaseSteps(stepNames(stepFile.Iteration))
    header.SetStepState(0, ui.StepActive)
    header.RenderIterationLine(1, cfg.Iterations, "")
}

model := ui.NewModel(header, keyHandler, "ralph-tui v"+version.Version)
program := tea.NewProgram(model, ...)
program.Run() // first frame renders from already-populated state
```

This mirrors the general principle that display state should be initialized before the display is activated, not after.

## Use strings.NewReplacer for multi-key template substitution

When substituting multiple `{{KEY}}` tokens in a template string, use `strings.NewReplacer` rather than chained `strings.ReplaceAll` calls. It builds the replacement table once and applies all substitutions in a single pass, which is both faster and easier to read as the number of keys grows.

```go
// Good — single pass, one replacement table
func substitute(template string, vals map[string]string) string {
    pairs := make([]string, 0, len(vals)*2)
    for k, v := range vals {
        pairs = append(pairs, "{{"+k+"}}", v)
    }
    return strings.NewReplacer(pairs...).Replace(template)
}

// Usage:
h.IterationLine = substitute("Initializing {{STEP_NUM}}/{{STEP_COUNT}}: {{STEP_NAME}}", map[string]string{
    "STEP_NUM":   strconv.Itoa(stepNum),
    "STEP_COUNT": strconv.Itoa(stepCount),
    "STEP_NAME":  stepName,
})

// Bad — multiple passes, growing number of ReplaceAll calls as keys are added
s := strings.ReplaceAll(template, "{{STEP_NUM}}", strconv.Itoa(stepNum))
s = strings.ReplaceAll(s, "{{STEP_COUNT}}", strconv.Itoa(stepCount))
s = strings.ReplaceAll(s, "{{STEP_NAME}}", stepName)
```

Keys not present in `vals` are left as-is by `strings.NewReplacer` — this is the correct contract for a template engine (missing keys stay visible as unresolved placeholders rather than silently becoming empty strings).

## Restrict file and directory permissions for sensitive output

When creating directories or files that contain sensitive output (log files, captured subprocess output), use restrictive permission bits. World-readable files leak information on shared or multi-user systems.

```go
// Log directory — owner read/write/execute only
if err := os.MkdirAll(dir, 0o700); err != nil {
    return fmt.Errorf("logger: create log dir %s: %w", dir, err)
}

// Log file — owner read/write only
f, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o600)
```

Apply `0o700` for directories and `0o600` for files whenever the content could reveal implementation details, user data, or captured credentials. The default `0o755`/`0o644` is appropriate only for intentionally public artifacts.

## Pin tool dependencies with a tools.go build-tag file

When a Go module needs to pin a tool-only dependency (e.g., a code generator or formatter) without including it in the compiled binary, use a `tools.go` file with a `//go:build tools` build tag and blank imports. Verify the file type-checks under its tag by passing `-tags tools` to `go vet`.

```go
//go:build tools

package main

import (
    _ "github.com/charmbracelet/bubbletea"
)
```

```makefile
vet:
    go vet ./...
    go vet -tags tools .   # type-checks tools.go blank imports
```

The `//go:build tools` tag excludes the file from normal builds and `go build` (which fails by design — no `main` function). `go mod tidy` keeps the dependency pinned in `go.sum`. The `-tags tools` vet step catches import-path typos that `go mod tidy` would miss.

## Additional Information

- [Architecture Overview](../architecture.md) — System-level architecture and design principles
- [CLI & Configuration](../features/cli-configuration.md) — Symlink-safe project directory resolution in `resolveProjectDir`; cobra Execute nil guard in `main.go`
- [Workflow Orchestration](../features/workflow-orchestration.md) — `iterationLabel` conditional format helper applied across log call sites
- [TUI Display](../features/tui-display.md) — Pre-populate TUI model state before program.Run()
- [Subprocess Execution & Streaming](../features/subprocess-execution.md) — 256KB scanner buffer and ResolveCommand slice immutability
- [File Logging](../features/file-logging.md) — 0o700 dir / 0o600 file permission hardening applied to logger
- [Step Definitions & Prompt Building](../features/step-definitions.md) — Slice allocation in buildIterationSteps/buildFinalizeSteps
- [Testing](testing.md) — Standards for runtime.Caller(0) in test helpers and input slice immutability tests
- [API Design](api-design.md) — Complementary standards for platform-scoped assumptions
- [Concurrency](concurrency.md) — Complementary concurrency patterns
- [Error Handling](error-handling.md) — Complementary error handling conventions
- [TUI Display](../features/tui-display.md) — `substitute` helper as the canonical strings.NewReplacer usage example
