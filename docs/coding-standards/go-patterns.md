# Go Patterns

## Resolve directories with os.Executable / os.Getwd + filepath.EvalSymlinks

pr9k resolves two directories at startup — both must go through `filepath.EvalSymlinks` to produce real paths:

**Workflow directory** (install dir — where `config.json`, `prompts/`, `scripts/` live): resolved from the compiled binary's location. Skipping `EvalSymlinks` breaks when the binary is installed as a symlink (e.g., `~/bin/pr9k` → `pr9k/bin/pr9k`).

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

This is why `go run` does not work for pr9k: `go run` places the binary in a temp dir, so `os.Executable()` resolves to that temp dir rather than to the workflow bundle. Use `go build` and invoke the compiled binary directly, or pass `--workflow-dir` explicitly.

## Extract OS-dependent calls into injectable inner functions

Functions that rely on `os.Executable()`, `os.Getwd()`, or other OS state cannot be unit-tested without running a real binary or changing the process working directory. Extract a testable inner function that accepts the already-resolved values; the public function performs the OS call and delegates.

```go
// Public entry point — performs the OS call, then delegates.
func resolveWorkflowDir(projectDir string) (string, error) {
    exe, err := os.Executable()
    if err != nil {
        return "", err
    }
    exe, err = filepath.EvalSymlinks(exe)
    if err != nil {
        return "", err
    }
    return resolveWorkflowDirWith(projectDir, filepath.Dir(exe))
}

// Testable inner function — no OS calls; tests inject arbitrary directories.
func resolveWorkflowDirWith(projectDir, execDir string) (string, error) {
    candidate := filepath.Join(projectDir, ".pr9k", "workflow")
    if info, err := os.Stat(candidate); err == nil && info.IsDir() {
        return candidate, nil
    }
    candidate = filepath.Join(execDir, ".pr9k", "workflow")
    if info, err := os.Stat(candidate); err == nil && info.IsDir() {
        return candidate, nil
    }
    return "", fmt.Errorf("could not locate workflow bundle")
}
```

Apply this split any time the public function's first action is an OS or environment call (`os.Executable`, `os.Getwd`, `os.Hostname`, `time.Now`, etc.) whose result is then threaded through pure logic. The inner function gets the `...With` suffix by convention and is package-private.

Tests call `resolveWorkflowDirWith` with `t.TempDir()`-rooted paths and never need to build or locate a real binary.

## Use runtime.Caller(0) in test helpers for path resolution

See [testing.md](testing.md) — `runtime.Caller(0)` is the correct way to resolve paths in test helpers. Do not use `os.Getwd()`.

## Allocate a new slice in transformation functions

When a function transforms a slice (e.g., replacing template variables), allocate a new slice rather than mutating the input. Callers often reuse the original slice across multiple iterations.

```go
func ResolveCommand(workflowDir string, command []string, vt *vars.VarTable, phase vars.Phase) []string {
    if len(command) == 0 {
        return command
    }
    result := make([]string, len(command))
    for i, arg := range command {
        substituted, _ := vars.Substitute(arg, vt, phase)
        result[i] = substituted
    }
    // ... resolve script paths ...
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

In this codebase the `substitute` helper in `header.go` handles this for initialize and finalize lines via template strings, so the conditional logic lives in the render method and the formatting in the templates. `RenderIterationLine` uses `strings.Builder` with `fmt.Fprintf` directly because its format varies more dynamically (optional issue suffix):

```go
// RenderInitializeLine and RenderFinalizeLine use substitute with template constants:
func (h *StatusHeader) RenderInitializeLine(stepNum, stepCount int, stepName string) {
    h.IterationLine = substitute(initializeHeaderFormat, map[string]string{
        "STEP_NUM":   strconv.Itoa(stepNum),
        "STEP_COUNT": strconv.Itoa(stepCount),
        "STEP_NAME":  stepName,
    })
}

// RenderIterationLine uses strings.Builder for its dynamic format:
func (h *StatusHeader) RenderIterationLine(iter, maxIter int, issueID string) {
    var b strings.Builder
    if maxIter > 0 {
        fmt.Fprintf(&b, "Iteration %d/%d", iter, maxIter)
    } else {
        fmt.Fprintf(&b, "Iteration %d", iter)
    }
    if issueID != "" {
        fmt.Fprintf(&b, " — Issue #%s", issueID)
    }
    h.IterationLine = b.String()
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
    fmt.Fprintf(os.Stderr, "error: %v\nRun 'pr9k --help' for usage.\n", err)
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

versionLabel := "pr9k v" + version.Version
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

## Nil guards must encompass all dependent side effects

When a message handler (or any function) contains a nil guard that conditionally processes a resource, all side effects that are predicated on the resource being non-nil must be inside the guard's positive branch. Placing a dependent operation (such as rescheduling a ticker) outside the guard creates a perpetual loop that does nothing — unreachable in practice, but inconsistent and fragile.

```go
// Bad — reschedule runs unconditionally; if heartbeat is nil, the ticker
// fires forever without doing anything useful
case HeartbeatTickMsg:
    if m.heartbeat != nil {
        m.header.HandleHeartbeatTick()
    }
    return m, heartbeatTick() // ← always reschedules, even when heartbeat == nil

// Good — both the action and the reschedule are inside the non-nil branch
case HeartbeatTickMsg:
    if m.heartbeat != nil {
        m.header.HandleHeartbeatTick()
        return m, heartbeatTick()
    }
    // When heartbeat is nil, do nothing and return no cmd.
```

The general rule: every operation that depends on a resource being non-nil belongs inside the non-nil check for that resource, including cleanup, rescheduling, and return values. An operation placed after the guard implicitly assumes the resource is always available — but the guard was written precisely because that assumption is not always true.

## Extract repeated state-reset patterns into a named helper

When the same flag-clearing or state-reset sequence appears in three or more places, extract it into a named unexported method. Duplicated reset logic drifts: a new field added to the reset must be updated in every copy, and missing one silently introduces a bug.

```go
// Bad — selectJustReleased clearing repeated in 3 places across model.go
h.mu.Lock()
h.selectJustReleased = false
h.updateShortcutLineLocked()
h.mu.Unlock()

// Good — extract into a named helper; apply the Locked suffix if the caller must hold the mutex
func (h *KeyHandler) clearJustReleasedLocked() {
    h.selectJustReleased = false
    h.updateShortcutLineLocked()
}

// All three call sites become a single line:
h.mu.Lock()
h.clearJustReleasedLocked()
h.mu.Unlock()
```

The threshold is three — two copies are often fine, but a third signals that the pattern is a concept and should be named.

## Function comments must accurately describe the implementation

When a function iterates by rune but the comment says "grapheme cluster", when a method clears state "on the next Update" but it actually clears it immediately — those inaccuracies become traps. Future maintainers act on the comment, not the code, and introduce real bugs.

```go
// Bad — misleads readers into thinking grapheme-cluster semantics are applied
for _, r := range s {
    col += runewidth.RuneWidth(r)
    if col > targetCol {
        break
    }
    // Advance one rune (grapheme cluster).  ← wrong label; it's a rune, not a cluster
    byteOffset += utf8.RuneLen(r)
}

// Good — name matches the unit of iteration
    // Advance one rune.
    byteOffset += utf8.RuneLen(r)
```

Checklist when writing a doc comment for a function that processes text:
- Does it say "rune" when it uses `range string`?
- Does it say "grapheme cluster" only when it uses a grapheme-segmenting library (e.g., `rivo/uniseg`)?
- Does it say "byte" only when it indexes with `[]byte` or advances by `utf8.RuneLen`?

The distinction matters because rune iteration and grapheme-cluster iteration produce different results for multi-codepoint characters (e.g., emoji with skin-tone modifiers, combined accent characters). A wrong label here creates a future ANSI/Unicode correctness bug.

## Use exhaustive switches on internal enum types

When a switch statement covers all known values of an internal enum (a set of `const` iota values), do not add a `default` case that returns a fallback like `"unknown"`. A catch-all default silently accepts new enum values that the switch has not been updated to handle — the omission compiles cleanly, ships to production, and produces wrong behavior at runtime with no compiler signal.

```go
// Bad — default "unknown" silently accepts new Mode values
func modeString(m ui.Mode) string {
    switch m {
    case ui.ModeNormal:
        return "normal"
    case ui.ModeHelp:
        return "help"
    // ... other cases ...
    default:
        return "unknown" // new ModeQuitting silently becomes "unknown" in stdin JSON
    }
}

// Good — no default; compiler reports unhandled cases when a new Mode is added
func modeString(m ui.Mode) string {
    switch m {
    case ui.ModeNormal:
        return "normal"
    case ui.ModeHelp:
        return "help"
    case ui.ModeQuitting:
        return "quitting"
    // ... all cases explicit; adding ModeNewMode causes a compile error here
    }
    // unreachable — document why if the compiler does not catch it
    panic(fmt.Sprintf("modeString: unhandled Mode %d", m))
}
```

The panic at the end replaces the `default` branch. It is unreachable in correct code (every valid Mode value is covered), but it fires immediately if an uncovered value somehow reaches the function — which is preferable to silently shipping `"unknown"` in a JSON payload that a downstream script depends on.

Apply to any switch that maps an internal iota-based type to a string, error category, or other derived value. Switches that branch on externally-supplied values (e.g., HTTP status codes) may legitimately need a default.

## Use modern octal literals (0o644, not 0644)

Write file permission bits with the `0o` prefix introduced in Go 1.13. The old bare-zero prefix (`0644`) is a C-era convention that has caused real-world misreads; the `0o` prefix makes the octal base unambiguous.

```go
// Good
os.WriteFile(path, data, 0o644)
os.MkdirAll(dir, 0o755)
os.OpenFile(path, os.O_CREATE|os.O_WRONLY, 0o600)

// Bad — bare zero prefix; easy to misread as decimal
os.WriteFile(path, data, 0644)
os.MkdirAll(dir, 0755)
```

Apply to every `os.Create`, `os.OpenFile`, `os.WriteFile`, `os.MkdirAll`, and `os.Chmod` call — including test files. A reviewer who sees `0644` in a new PR should flag it.

## Use time.NewTimer with defer Stop() for per-operation deadlines

When a goroutine needs to fire an action after a deadline, use `time.NewTimer` with `defer t.Stop()` rather than `time.After`. `time.After` creates a timer that cannot be canceled — it leaks until the channel fires even when the protected operation finishes first, which accumulates goroutine-held allocations over many iterations.

```go
// Good — timer is stopped immediately when the goroutine exits
timer := time.NewTimer(deadline)
defer timer.Stop()

select {
case <-done:
    return
case <-timer.C:
    // deadline fired
}

// Bad — the timer fires after `deadline` regardless of whether done closed first;
// the old timer channel is GC'd only after the duration elapses
select {
case <-done:
    return
case <-time.After(deadline):
    // deadline fired
}
```

Additionally, after a timer fires, do a non-blocking check of the early-exit path before proceeding:

```go
case <-timer.C:
    // Non-blocking re-check: if the step completed in the same scheduler slice
    // that fired the timer, treat it as done rather than timed-out.
    select {
    case <-done:
        return
    default:
    }
    // Now signal the timeout.
    proc.Signal(syscall.SIGTERM)
```

The re-check closes the race window where a fast step and the timer fire in the same goroutine scheduling round.

## Snap byte-length truncation to a rune boundary

When hard-capping a string at a maximum byte length, snap the cut point backward to the nearest valid UTF-8 rune start. Slicing at an arbitrary byte offset splits a multi-byte rune and produces a garbled trailing character.

```go
import "unicode/utf8"

const maxBytes = 30 * 1024

func truncate(s string) string {
    if len(s) <= maxBytes {
        return s
    }
    cut := maxBytes
    // Walk backward until we land on a rune boundary.
    for cut > 0 && !utf8.RuneStart(s[cut]) {
        cut--
    }
    return s[:cut] + "\n[truncated]"
}
```

Apply any time a string is sliced at a byte index rather than a rune index — log truncation, API response capping, capture buffers. The fix is always the same two-line backward walk.

## Kill the process group for host subprocesses

When spawning a host (non-Docker) subprocess that may itself spawn children, set `SysProcAttr.Setpgid = true` so the child runs in its own process group. When sending a signal (SIGTERM, SIGKILL), use `syscall.Kill(-pid, sig)` to deliver it to the entire process group — not just the direct child.

Without `Setpgid`, `syscall.Kill(-pid, sig)` kills the parent process's own process group, potentially delivering signals to the pr9k process itself.

```go
cmd := exec.Command(args[0], args[1:]...)
cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
// ... start ...

// Signal the process group, not just the direct child:
syscall.Kill(-cmd.Process.Pid, syscall.SIGTERM)
```

Docker steps do not need this — Docker's container isolation already provides process-group containment. Apply only to `isClaude: false` (host) steps.

## Extract magic numbers to named constants

When a numeric literal encodes a meaningful domain constraint (e.g., a token budget, a time limit, a buffer size), extract it to a named unexported constant at the top of the file. A bare number forces every reader to infer the meaning; a named constant makes the constraint self-documenting and localizes future changes.

```go
// Bad — 200_000 appears in three places; meaning is inferred from context
if stats.InputTokens > 200_000 {
    ...
}

// Good — name encodes the constraint; change in one place
const resumeInputTokenLimit = 200_000

if stats.InputTokens > resumeInputTokenLimit {
    ...
}
```

The threshold is one: even a single use of a non-obvious number warrants a constant if the number encodes a domain rule. Numbers like `0`, `1`, `-1`, and small loop bounds are exempt.

## Use lipgloss.JoinHorizontal for side-by-side TUI panels

When placing two panels next to each other in a Bubble Tea view, use `lipgloss.JoinHorizontal` rather than string concatenation. String concatenation produces incorrect results when either string contains Lip Gloss styling (ANSI sequences inflate byte length vs. visual width), and it does not align the tops of multi-line panels.

```go
// Bad — string concatenation; breaks when either panel has ANSI styling
return outlineStr + " | " + detailStr

// Good — accounts for visual width and aligns panel tops
return lipgloss.JoinHorizontal(lipgloss.Top, outlineStr, detailStr)
```

`lipgloss.JoinHorizontal(lipgloss.Top, ...)` aligns all panels at their top edge. Use `lipgloss.Center` or `lipgloss.Bottom` when those alignments are needed. Apply any time two rendered strings are placed side by side and at least one may contain ANSI escapes.

## Open log files with O_APPEND

When opening a log file that may be re-opened across the lifetime of a process (e.g., a session log that survives a restart), always include `O_APPEND` in the open flags. Without `O_APPEND`, the write position is reset to the beginning of the file on each `Open` call — subsequent writes overwrite earlier content instead of appending to it.

```go
// Good — O_APPEND ensures new writes go to the end of an existing file
f, err := os.OpenFile(logPath, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o600)

// Bad — without O_APPEND, re-opening an existing file resets the write position;
// the second open session overwrites the first session's log entries
f, err := os.OpenFile(logPath, os.O_CREATE|os.O_WRONLY, 0o600)
```

`O_APPEND` is also safe for the first open (when the file does not yet exist): `O_CREATE` creates the file and `O_APPEND` becomes a no-op because the file starts empty.

Apply to every log file or session-state file that may be re-opened. One-shot write-once files (e.g., an atomic replacement via `atomicwrite.Write`) do not need `O_APPEND` because they are always written from scratch.

## Additional Information

- [Architecture Overview](../architecture.md) — System-level architecture and design principles
- [CLI & Configuration](../features/cli-configuration.md) — Symlink-safe project directory resolution in `resolveProjectDir`; cobra Execute nil guard in `main.go`; `resolveWorkflowDirWith` as the canonical OS-call injection seam
- [Workflow Orchestration](../features/workflow-orchestration.md) — Phase-specific render methods using `substitute` for conditional format strings
- [TUI Display](../features/tui-display.md) — Pre-populate TUI model state before program.Run(); two-pass global maxCellWidth layout for the checkbox grid; write-only `iterationLine` field removed from `headerModel` (issue #75)
- [Subprocess Execution & Streaming](../features/subprocess-execution.md) — 256KB scanner buffer and ResolveCommand slice immutability
- [File Logging](../code-packages/logger.md) — 0o700 dir / 0o600 file permission hardening applied to logger
- [Step Definitions & Prompt Building](../code-packages/steps.md) — Slice allocation in buildIterationSteps/buildFinalizeSteps
- [Testing](testing.md) — Standards for runtime.Caller(0) in test helpers and input slice immutability tests
- [API Design](api-design.md) — Complementary standards for platform-scoped assumptions
- [Concurrency](concurrency.md) — Complementary concurrency patterns
- [Error Handling](error-handling.md) — Complementary error handling conventions
- [TUI Display](../features/tui-display.md) — `substitute` helper as the canonical strings.NewReplacer usage example
- [TUI Display](../features/tui-display.md) — HeartbeatTickMsg handler nil guard fix as the canonical nil-guard completeness example (issue #94)
- [Workflow Orchestration](../features/workflow-orchestration.md) — `timeoutSeconds` goroutine uses `time.NewTimer` + `defer Stop()` with non-blocking done re-check (issue #130)
- [Stream JSON Pipeline](../code-packages/claudestream.md) — `fullStdoutCapture` uses `utf8.RuneStart` backward walk for truncation (issue #123)
- [Docker Sandbox](../features/docker-sandbox.md) — `Setpgid: true` + `syscall.Kill(-pid, sig)` for host subprocess process-group signals (issue #130)
- [Workflow Orchestration](../features/workflow-orchestration.md) — `resumeInputTokenLimit` constant replacing 200_000 magic number (issue #131)
- [File Logging](../code-packages/logger.md) — `O_APPEND` added to logger open flags to prevent write-position reset on re-open (workflow-builder branch)
