# CLI & Configuration

Parses command-line flags and resolves the project directory that anchors all relative path resolution throughout ralph-tui.

- **Last Updated:** 2026-04-10 23:25
- **Authors:**
  - River Bailey

## Overview

- ralph-tui accepts an optional `--iterations` / `-n` flag (default 0 = run until done), an optional `--project-dir` / `-p` flag, and a `--version` / `-v` flag that prints the version and exits
- Built on [spf13/cobra](https://github.com/spf13/cobra), which handles POSIX-style flags in any position — no custom reordering needed
- When `--project-dir` is not provided, the project directory is resolved from the executable's real path via `os.Executable()` + `filepath.EvalSymlinks` (symlink-safe)
- `ProjectDir` fans out to every subsystem: logger, step loader, prompt builder, command resolver, and the workflow runner
- The version string is sourced from `internal/version.Version` — the single source of truth for both the `--version` output and the TUI footer label. See [Versioning](../coding-standards/versioning.md).

Key files:
- `ralph-tui/internal/cli/args.go` — `Execute`, `NewCommand`, `Config`, `resolveProjectDir`
- `ralph-tui/internal/cli/args_test.go` — 18 test cases covering all argument parsing branches (including `--version` and `-v`)
- `ralph-tui/internal/version/version.go` — The `Version` constant consumed by cobra's built-in version flag
- `ralph-tui/cmd/ralph-tui/main.go` — Entry point that calls `Execute` and distributes `Config`

## Architecture

```
                         os.Args[1:]
                             │
                             ▼
                      ┌─────────────┐
                      │    cobra    │  parse --iterations, --project-dir
                      │  (pflag)    │
                      └──────┬──────┘
                             │
                  ┌──────────┴──────────┐
                  │                     │
           --project-dir given     not given
                  │                     │
                  │              ┌──────▼──────────┐
                  │              │resolveProjectDir │
                  │              │ os.Executable()  │
                  │              │ EvalSymlinks()   │
                  │              │ filepath.Dir()   │
                  │              └──────┬───────────┘
                  │                     │
                  └──────────┬──────────┘
                             │
                             ▼
                      ┌─────────────┐
                      │   Config    │
                      │ Iterations  │
                      │ ProjectDir  │
                      └──────┬──────┘
                             │
           ┌─────────┬──────┼──────┬──────────┐
           ▼         ▼      ▼      ▼          ▼
        Logger    LoadSteps  LoadFinalize  Runner  RunConfig
                             Steps
```

## Key Files

| File | Purpose |
|------|---------|
| `ralph-tui/internal/cli/args.go` | `Execute`, `NewCommand`, `Config` struct, `resolveProjectDir` |
| `ralph-tui/internal/cli/args_test.go` | Unit tests for all argument parsing branches |
| `ralph-tui/cmd/ralph-tui/main.go` | Entry point — calls `Execute`, distributes `Config` to subsystems |

## Core Types

```go
// Config holds parsed CLI arguments.
type Config struct {
    Iterations int    // number of workflow iterations to run (0 = run until done)
    ProjectDir string // root directory for all relative path resolution
}
```

## Implementation Details

### Entry Point

`Execute` creates a `Config`, builds a cobra command via `newCommandImpl`, and runs it against `os.Args`. It uses a `ranE` sentinel to distinguish `--help` (RunE not invoked) from a successful parse:

```go
func Execute() (*Config, error) {
    cfg := &Config{}
    var ranE bool
    cmd := newCommandImpl(cfg, &ranE)
    if err := cmd.Execute(); err != nil {
        return nil, err
    }
    if !ranE {
        return nil, nil // --help was requested
    }
    return cfg, nil
}
```

Return values:
- `(cfg, nil)` — successful parse, workflow should start
- `(nil, nil)` — `--help` requested, exit cleanly
- `(nil, err)` — flag parsing or validation failed, exit 1

### Argument Parsing

`newCommandImpl` builds the cobra command with `cobra.NoArgs` (positional arguments are rejected), sets `cmd.Version` to the compile-time constant (which enables cobra's built-in `--version` / `-v` handling), and defines the two user flags:

```go
cmd := &cobra.Command{
    Use:     "ralph-tui [flags]",
    Short:   "Automated development workflow orchestrator",
    Version: version.Version,  // enables --version / -v
    Args:    cobra.NoArgs,
    // ...
}
cmd.Flags().IntVarP(&cfg.Iterations, "iterations", "n", 0, "number of iterations to run (0 = run until done)")
cmd.Flags().StringVarP(&cfg.ProjectDir, "project-dir", "p", "", "path to the project directory (default: resolved from executable)")
```

When `--version` or `-v` is passed, cobra prints `ralph-tui version <semver>` to stdout and exits **without invoking `RunE`** — the `ranE` sentinel stays `false`, `Execute` returns `(nil, nil)`, and `main` exits cleanly without starting the workflow. This is the contract that the `--version` public-API surface in the [Versioning](../coding-standards/versioning.md) standard commits to.

RunE validates and resolves:

```go
RunE: func(cmd *cobra.Command, args []string) error {
    *ranE = true
    if cfg.Iterations < 0 {
        return errors.New("cli: --iterations must be a non-negative integer")
    }
    if cfg.ProjectDir == "" {
        dir, err := resolveProjectDir()
        if err != nil {
            return fmt.Errorf("cli: could not resolve project dir: %w", err)
        }
        cfg.ProjectDir = dir
    }
    return nil
},
```

### Project Directory Resolution

When `--project-dir` is not provided, the project directory is inferred from the compiled binary's location:

```go
func resolveProjectDir() (string, error) {
    exe, err := os.Executable()              // path of the running binary
    resolved, err := filepath.EvalSymlinks(exe) // dereference symlinks
    return filepath.Dir(resolved), nil          // directory containing the binary
}
```

`filepath.EvalSymlinks` is critical: without it, a symlinked binary (e.g., installed in `~/bin/`) would resolve to the symlink's directory rather than where `configs/`, `prompts/`, and `scripts/` actually live.

This is why `go run` does not work — it places the binary in a temporary directory. Use `go build` and run the compiled binary, or pass `--project-dir` explicitly.

### ProjectDir Fan-Out

After parsing, `Config.ProjectDir` is distributed to five consumers in `main.go`:

| Consumer | Path Resolved |
|----------|---------------|
| `logger.NewLogger(projectDir)` | `{projectDir}/logs/ralph-*.log` |
| `steps.LoadSteps(projectDir)` | `{projectDir}/ralph-steps.json` |
| `workflow.NewRunner(log, projectDir)` | Sets `cmd.Dir` for all subprocesses |
| `workflow.RunConfig.ProjectDir` | Scripts, prompt files, command resolution |

Within the workflow, `ProjectDir` anchors two path-resolution mechanisms:
- `{projectDir}/prompts/{promptFile}` — prompt files via `steps.BuildPrompt` (for Claude steps)
- Relative script paths from step config — resolved against `projectDir` by `ResolveCommand` (e.g. `scripts/get_gh_user`, `scripts/close_gh_issue`); not hardcoded in `Run()`

## Error Handling

| Scenario | Error Message | Behavior |
|----------|---------------|----------|
| Negative iterations | `"cli: --iterations must be a non-negative integer"` | Exit 1 |
| Unknown flag | pflag error (e.g., `"unknown flag: --foo"`) | Exit 1 |
| Positional argument given | cobra error (`"unknown command"` / `"accepts 0 arg(s)"`) | Exit 1 |
| Cannot resolve executable | `"cli: could not resolve project dir: ..."` | Exit 1 |

All errors are written to stderr followed by a `Run 'ralph-tui --help' for usage.` hint, and cause `os.Exit(1)`.

## Configuration

| Flag | Short | Description | Default |
|------|-------|-------------|---------|
| `--iterations` | `-n` | Number of iterations to run (0 = run until done) | `0` |
| `--project-dir` | `-p` | Path to the project root directory | Resolved from executable location |
| `--version` | `-v` | Print `ralph-tui version <semver>` and exit without running the workflow | — |
| `--help` | `-h` | Print cobra-generated usage and exit without running the workflow | — |

**Usage:**

```
ralph-tui [--iterations <n>] [--project-dir <path>]
ralph-tui --version
```

## Testing

- `ralph-tui/internal/cli/args_test.go` — 18 test cases covering all `NewCommand` and `Execute` branches

### Test Cases

| Test | What It Validates |
|------|-------------------|
| `TestNewCommand_NoFlags` | No flags → iterations=0, project-dir resolved from executable (non-empty) |
| `TestNewCommand_LongIterationsFlag` | `--iterations 3` → iterations=3 |
| `TestNewCommand_ShortIterationsFlag` | `-n 3` → iterations=3 |
| `TestNewCommand_NegativeIterations` | `--iterations -1` → error containing expected message |
| `TestNewCommand_LongProjectDirFlag` | `--project-dir /tmp/foo` → project-dir=/tmp/foo |
| `TestNewCommand_ShortProjectDirFlag` | `-p /tmp/foo` → project-dir=/tmp/foo |
| `TestNewCommand_BothFlags` | `-n 3 -p /tmp/foo` → both fields set |
| `TestNewCommand_PositionalArgRejected` | `["somearg"]` → error from cobra.NoArgs |
| `TestNewCommand_UnknownFlag` | `["--unknown"]` → error |
| `TestExecute_HelpReturnsNilNil` | `--help` → RunE not invoked (ranE=false) |
| `TestNewCommand_EqualsSyntax` | `--iterations=3` → iterations=3 |
| `TestNewCommand_IterationsNoValue` | `--iterations` with no value → error |
| `TestNewCommand_ShortIterationsNoValue` | `-n` with no value → error |
| `TestNewCommand_ArgsAfterSeparatorRejected` | `-- extraarg` → error from cobra.NoArgs |
| `TestNewCommand_ExplicitZeroIterations` | `-n 0` → iterations=0 (until-done mode) |
| `TestNewCommand_LargeIterations` | `-n 1000` → accepted |
| `TestNewCommand_VersionFlag` | `--version` → RunE not invoked, output contains `version.Version`, no error |
| `TestNewCommand_VersionShortFlag` | `-v` → behaves identically to `--version` |

The two version tests read the expected string from `version.Version` rather than hardcoding `"0.1.0"` — the pattern required by [Versioning](../coding-standards/versioning.md) so a version bump does not require touching the test file.

## Additional Information

- [Architecture Overview](../architecture.md) — System-level view of ralph-tui with block diagrams and data flow
- [ADR: Use Cobra for CLI Argument Parsing](../adr/20260409135303-cobra-cli-framework.md) — Decision rationale for replacing stdlib `flag` with cobra
- [Versioning](../coding-standards/versioning.md) — Single-source-of-truth rule for `version.Version`, what counts as ralph-tui's public API (CLI flags, `--version` output format), and how to bump the version
- [Building Custom Workflows](../how-to/building-custom-workflows.md) — How ProjectDir affects config and prompt file resolution
- [Step Definitions & Prompt Building](step-definitions.md) — How ProjectDir resolves config and prompt files
- [Subprocess Execution & Streaming](subprocess-execution.md) — How ProjectDir sets the working directory for subprocesses
- [Workflow Orchestration](workflow-orchestration.md) — How RunConfig carries ProjectDir and Iterations into the Run loop
- [TUI Status Header & Log Display](tui-display.md) — Where the `version.Version` constant is rendered as the footer label
- [File Logging](file-logging.md) — How ProjectDir determines the log file location
- [ralph-tui Plan](../plans/ralph-tui.md) — Original specification including CLI design decisions
- [Go Patterns](../coding-standards/go-patterns.md) — Coding standards for symlink-safe path resolution
