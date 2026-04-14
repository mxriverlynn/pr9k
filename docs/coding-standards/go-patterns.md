# Go Patterns

## Resolve directories with os.Executable / os.Getwd + filepath.EvalSymlinks

ralph-tui resolves two directories at startup — both must go through `filepath.EvalSymlinks` to produce real paths:

**Workflow directory** (install dir — where `ralph-steps.json`, `prompts/`, `scripts/` live): resolved from the compiled binary's location. Skipping `EvalSymlinks` breaks when the binary is installed as a symlink (e.g., `~/bin/ralph-tui` → `pr9k/bin/ralph-tui`).

```go
exe, err := os.Executable()
if err != nil {
    return "", err
}
exe, err = filepath.EvalSymlinks(exe)
if err != nil {
    return "", err
}
workflowDir := filepath.Dir(exe)
```

**Project directory** (target repo — the git repo the workflow operates against): resolved from the shell CWD at startup. `EvalSymlinks` ensures the path matches what Docker receives for bind-mount arguments.

```go
cwd, err := os.Getwd()
if err != nil {
    return "", err
}
projectDir, err := filepath.EvalSymlinks(cwd)
if err != nil {
    return "", err
}
```

This is why `go run` does not work for ralph-tui: `go run` places the binary in a temp dir, so `os.Executable()` resolves to that temp dir rather than to the workflow bundle. Use `go build` and invoke the compiled binary directly, or pass `--workflow-dir` explicitly.

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
buf := make([]byte, 256*1024)
scanner := bufio.NewScanner(pipe)
scanner.Buffer(buf, 256*1024)
```

## Extract conditional format strings to a named pure helper

When the same conditional formatting decision (e.g., bounded vs. unbounded, singular vs. plural) appears in multiple log or UI call sites, extract it as a named unexported function rather than repeating the condition inline. This makes the formatting logic independently testable and keeps the condition in one place.

In this codebase the `substitute` helper in `header.go` handles this for iteration/phase lines via template strings (`iterationHeaderBoundedFormat`, `iterationHeaderUnboundedFormat`, etc.), so the conditional logic lives in `RenderIterationLine` and the formatting in the templates.

```go
// Phase-specific render methods each select the right template and call substitute:
func (h *StatusHeader) RenderIterationLine(iter, maxIter int, issueID string) {
    vals := map[string]string{"ITER": strconv.Itoa(iter), "ISSUE_ID": issueID}
    if maxIter > 0 {
        vals["MAX_ITER"] = strconv.Itoa(maxIter)
        h.IterationLine = substitute(iterationHeaderBoundedFormat, vals)
    } else {
        h.IterationLine = substitute(iterationHeaderUnboundedFormat, vals)
    }
}
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
    fmt.Fprintf(os.Stderr, "error: %v\nRun 'ralph-tui --help' for usage.\n", err)
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
    header.IterationLine = "Initializing 1/" + strconv.Itoa(len(stepFile.Initialize)) + ": " + stepFile.Initialize[0].Name
} else {
    header.SetPhaseSteps(stepNames(stepFile.Iteration))
    header.SetStepState(0, ui.StepActive)
    if cfg.Iterations > 0 {
        header.IterationLine = "Iteration 1/" + strconv.Itoa(cfg.Iterations)
    } else {
        header.IterationLine = "Iteration 1"
    }
}

versionLabel := "ralph-tui v" + version.Version
model := ui.NewModel(header, keyHandler, versionLabel)
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
    _ "github.com/charmbracelet/bubbles"
    _ "github.com/charmbracelet/bubbletea"
    _ "github.com/charmbracelet/lipgloss"
)
```

```makefile
vet:
    go vet ./...
    go vet -tags tools .   # type-checks tools.go blank imports
```

The `//go:build tools` tag excludes the file from normal builds and `go build` (which fails by design — no `main` function). `go mod tidy` keeps the dependency pinned in `go.sum`. The `-tags tools` vet step catches import-path typos that `go mod tidy` would miss.

## Two-pass layout for uniform column width

When rendering a multi-column grid where all columns must be the same width, compute the global maximum cell width in a first pass across all rows and columns, then apply padding in a second pass. Computing the max per-row instead of globally causes rows with shorter content to produce narrower columns, misaligning the grid across rows.

```go
// First pass — measure globally across all rows and columns.
maxCellWidth := 0
for r := range grid.Rows {
    for c := range numCols {
        if w := lipgloss.Width(grid.Rows[r][c]); w > maxCellWidth {
            maxCellWidth = w
        }
    }
}

// Second pass — render with uniform padding.
for r := range grid.Rows {
    var row strings.Builder
    for c := range numCols {
        row.WriteString(renderCell(grid.Rows[r][c]))
        if pad := maxCellWidth - lipgloss.Width(grid.Rows[r][c]); pad > 0 {
            row.WriteString(strings.Repeat(" ", pad))
        }
    }
}
```

Apply this pattern any time a grid or table must keep columns aligned across rows. A single-pass approach that computes per-row max will fail as soon as a later row contains a wider cell than the first row.

## Delete write-only fields — don't cache through a pointer

When a wrapper struct holds a pointer to another struct and exposes an accessor that reads through that pointer, do not also cache the pointed-to value as a separate field. A field that is written at every update site but never read is a write-only field — it creates synchronization burden (every update site must dual-write) and divergence risk (if any update site is missed, the cache and the pointer drift apart).

```go
// Bad — iterationLine is written at every update site but never read;
// iterLine() already reads header.IterationLine directly.
type headerModel struct {
    header        *StatusHeader
    iterationLine string // written but never read — vestigial cache
}

func (m headerModel) apply(msg tea.Msg) headerModel {
    case headerIterationLineMsg:
        m.header.RenderIterationLine(msg.iter, msg.max, msg.issue)
        m.iterationLine = m.header.IterationLine // redundant write
    // ...
}

func (m headerModel) iterLine() string {
    return m.header.IterationLine // reads through pointer — the only reader
}
```

```go
// Good — remove the cache field; the accessor reads through the pointer.
type headerModel struct {
    header *StatusHeader
}

func (m headerModel) apply(msg tea.Msg) headerModel {
    case headerIterationLineMsg:
        m.header.RenderIterationLine(msg.iter, msg.max, msg.issue)
        // no cache to maintain
    // ...
}

func (m headerModel) iterLine() string {
    return m.header.IterationLine
}
```

When reviewing a struct, look for fields that appear only on the left side of assignments (`m.field = ...`) and never in expressions. Those are candidates for deletion. If a pointer accessor already provides the same value, the cached field is always redundant.

## Sanitize external program output before reflecting to the terminal

When displaying output captured from an external program (e.g., a Docker smoke test, a subprocess version check), strip ANSI escape sequences before printing. A malicious or misbehaving image can inject terminal control sequences — cursor repositioning, color resets, title rewrites — that corrupt the TUI display or trick the user.

```go
var ansiEscapeRe = regexp.MustCompile(`\x1b\[[0-9;]*[A-Za-z]`)

func stripANSI(s string) string {
    return ansiEscapeRe.ReplaceAllString(s, "")
}

// Usage — sanitize before printing smoke-test output to stdout:
output := stripANSI(strings.TrimSpace(string(combined)))
fmt.Fprintln(cmd.OutOrStdout(), output)
```

Apply any time your code reflects output from an external binary to a terminal: Docker, subprocess capture, version probes, etc. Pure log-file writes do not need sanitization, but anything that reaches a TTY does.

## Use exec.CommandContext with a timeout for external binary probes

When invoking an external binary solely to probe for its presence or status (e.g., `docker info`, `docker images`), use `exec.CommandContext` with a deadline rather than `exec.Command`. A frozen or hung daemon will block `cmd.Run()` indefinitely otherwise, stalling startup for the user with no feedback.

```go
func (r RealProber) DockerDaemonReachable(ctx context.Context) error {
    ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
    defer cancel()
    cmd := exec.CommandContext(ctx, "docker", "info")
    cmd.Stdout = io.Discard
    cmd.Stderr = io.Discard
    return cmd.Run()
}
```

10 seconds is a reasonable upper bound for a local daemon probe. Use `io.Discard` for stdout/stderr on probes where the output is not needed — only the exit code matters.

## Trim environment variable values before use

When reading a value from an environment variable (especially one that may be set by a human in a shell profile or `.env` file), trim leading and trailing whitespace before using it. Editors and copy-paste operations commonly introduce invisible trailing spaces that cause path resolution, string comparison, and file-open calls to fail silently.

```go
profileDir := strings.TrimSpace(os.Getenv("CLAUDE_CONFIG_DIR"))
if profileDir == "" {
    // fall back to default
}
```

Apply to any `os.Getenv` result that will be used as a file path, URL, identifier, or comparison value.

## Additional Information

- [Architecture Overview](../architecture.md) — System-level architecture and design principles
- [CLI & Configuration](../features/cli-configuration.md) — Symlink-safe project directory resolution in `resolveProjectDir`; cobra Execute nil guard in `main.go`
- [Workflow Orchestration](../features/workflow-orchestration.md) — Phase-specific render methods using `substitute` for conditional format strings
- [TUI Display](../features/tui-display.md) — Pre-populate TUI model state before program.Run(); two-pass global maxCellWidth layout for the checkbox grid; write-only `iterationLine` field removed from `headerModel` (issue #75)
- [Subprocess Execution & Streaming](../features/subprocess-execution.md) — 256KB scanner buffer and ResolveCommand slice immutability
- [File Logging](../features/file-logging.md) — 0o700 dir / 0o600 file permission hardening applied to logger
- [Step Definitions & Prompt Building](../features/step-definitions.md) — Slice allocation in buildIterationSteps/buildFinalizeSteps
- [Testing](testing.md) — Standards for runtime.Caller(0) in test helpers and input slice immutability tests
- [API Design](api-design.md) — Complementary standards for platform-scoped assumptions
- [Concurrency](concurrency.md) — Complementary concurrency patterns
- [Error Handling](error-handling.md) — Complementary error handling conventions
- [TUI Display](../features/tui-display.md) — `substitute` helper as the canonical strings.NewReplacer usage example
