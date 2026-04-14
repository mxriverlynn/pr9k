# CLI & Configuration

Parses command-line flags and resolves the two directories that anchor all path resolution throughout ralph-tui.

- **Last Updated:** 2026-04-13
- **Authors:**
  - River Bailey

## Overview

- ralph-tui accepts an optional `--iterations` / `-n` flag (default 0 = run until done), an optional `--workflow-dir` flag, an optional `--project-dir` flag, and a `--version` / `-v` flag that prints the version and exits
- Built on [spf13/cobra](https://github.com/spf13/cobra), which handles POSIX-style flags in any position — no custom reordering needed
- When `--workflow-dir` is not provided, the workflow directory is resolved from the executable's real path via `os.Executable()` + `filepath.EvalSymlinks` (symlink-safe)
- When `--project-dir` is not provided, the project directory is resolved from the current working directory via `os.Getwd()` + `filepath.EvalSymlinks`
- Both `WorkflowDir` and `ProjectDir` fan out to every subsystem that needs them
- The version string is sourced from `internal/version.Version` — the single source of truth for both the `--version` output and the TUI footer label. See [Versioning](../coding-standards/versioning.md).

Key files:
- `ralph-tui/internal/cli/args.go` — `Execute`, `NewCommand`, `Config`, `resolveWorkflowDir`, `resolveProjectDir`
- `ralph-tui/internal/cli/args_test.go` — 28 test cases covering all argument parsing branches (including `--version`, `-v`, symlink resolution, file-not-directory guards, `-p` guidance message, and subcommand dispatch)
- `ralph-tui/internal/version/version.go` — The `Version` constant consumed by cobra's built-in version flag
- `ralph-tui/cmd/ralph-tui/main.go` — Entry point that calls `Execute` and distributes `Config`
- `ralph-tui/cmd/ralph-tui/main_test.go` — Tests for the `stepNames` helper and `startup()` wiring

## Architecture

```
                         os.Args[1:]
                             │
                             ▼
                      ┌─────────────┐
                      │    cobra    │  parse --iterations, --workflow-dir,
                      │  (pflag)    │          --project-dir
                      └──────┬──────┘
                             │
               ┌─────────────┴─────────────┐
               │                           │
        --workflow-dir given         not given
               │                           │
               │                   ┌───────▼──────────┐
               │                   │resolveWorkflowDir │
               │                   │ os.Executable()   │
               │                   │ EvalSymlinks()    │
               │                   │ filepath.Dir()    │
               │                   └───────┬──────────┘
               └─────────────┬─────────────┘
                             │
               ┌─────────────┴─────────────┐
               │                           │
        --project-dir given          not given
               │                           │
               │                   ┌───────▼──────────┐
               │                   │resolveProjectDir  │
               │                   │ os.Getwd()        │
               │                   │ EvalSymlinks()    │
               │                   │ Stat + IsDir()    │
               │                   └───────┬──────────┘
               └─────────────┬─────────────┘
                             │
                             ▼
                      ┌─────────────┐
                      │   Config    │
                      │ Iterations  │
                      │ WorkflowDir │
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
| `ralph-tui/internal/cli/args.go` | `Execute`, `NewCommand`, `Config` struct, `resolveWorkflowDir`, `resolveProjectDir` |
| `ralph-tui/internal/cli/args_test.go` | Unit tests for all argument parsing branches |
| `ralph-tui/cmd/ralph-tui/main.go` | Entry point — calls `Execute`, distributes `Config` to subsystems |

## Core Types

```go
// Config holds parsed CLI arguments.
type Config struct {
    Iterations  int    // number of workflow iterations to run (0 = run until done)
    WorkflowDir string // install dir: ralph-steps.json, scripts/, prompts/, ralph-art.txt
    ProjectDir  string // target repository: subprocess cmd.Dir and log file location
}
```

## Implementation Details

### Entry Point

`Execute` creates a `Config`, builds a cobra command via `newCommandImpl`, and runs it against `os.Args`. It uses a `ranE` sentinel to distinguish `--help` (RunE not invoked) from a successful parse:

```go
func Execute(extra ...*cobra.Command) (*Config, error) {
    cfg := &Config{}
    var ranE bool
    cmd := newCommandImpl(cfg, &ranE)
    for _, sub := range extra {
        cmd.AddCommand(sub)
    }
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

`newCommandImpl` builds the cobra command with `cobra.NoArgs` (positional arguments are rejected), sets `cmd.Version` to the compile-time constant (which enables cobra's built-in `--version` / `-v` handling), and defines the user flags:

```go
cmd := &cobra.Command{
    Use:     "ralph-tui [flags]",
    Short:   "Automated development workflow orchestrator",
    Version: version.Version,  // enables --version / -v
    Args:    cobra.NoArgs,
    // ...
}
cmd.Flags().IntVarP(&cfg.Iterations, "iterations", "n", 0, "number of iterations to run (0 = run until done)")
cmd.Flags().StringVar(&cfg.WorkflowDir, "workflow-dir", "", "path to the workflow bundle directory (default: resolved from executable)")
cmd.Flags().StringVar(&cfg.ProjectDir, "project-dir", "", "path to the target repository (default: current working directory)")
```

Note: neither `--workflow-dir` nor `--project-dir` has a short form. The `-p` short form was removed in 0.3.0 when `--project-dir` changed meaning. Using `-p` now triggers a `SetFlagErrorFunc` guidance message pointing to the migration ADR.

When `--version` or `-v` is passed, cobra prints `ralph-tui version <semver>` to stdout and exits **without invoking `RunE`** — the `ranE` sentinel stays `false`, `Execute` returns `(nil, nil)`, and `main` exits cleanly without starting the workflow. This is the contract that the `--version` public-API surface in the [Versioning](../coding-standards/versioning.md) standard commits to.

RunE validates and resolves both directories:

```go
RunE: func(cmd *cobra.Command, args []string) error {
    *ranE = true
    if cfg.Iterations < 0 {
        return errors.New("cli: --iterations must be a non-negative integer")
    }
    if cfg.WorkflowDir == "" {
        dir, err := resolveWorkflowDir()
        if err != nil {
            return fmt.Errorf("cli: could not resolve workflow dir: %w", err)
        }
        cfg.WorkflowDir = dir
    } else {
        // EvalSymlinks + IsDir check for explicit --workflow-dir
        ...
    }
    if cfg.ProjectDir == "" {
        dir, err := resolveProjectDir()
        if err != nil {
            return fmt.Errorf("cli: could not resolve project dir: %w", err)
        }
        cfg.ProjectDir = dir
    } else {
        // EvalSymlinks + IsDir check for explicit --project-dir
        ...
    }
    return nil
},
```

### Workflow Directory Resolution

When `--workflow-dir` is not provided, the workflow directory is inferred from the compiled binary's location:

```go
func resolveWorkflowDir() (string, error) {
    exe, err := os.Executable()              // path of the running binary
    resolved, err := filepath.EvalSymlinks(exe) // dereference symlinks
    return filepath.Dir(resolved), nil          // directory containing the binary
}
```

`filepath.EvalSymlinks` is critical: without it, a symlinked binary (e.g., installed in `~/bin/`) would resolve to the symlink's directory rather than where `configs/`, `prompts/`, and `scripts/` actually live.

This is why `go run` does not work — it places the binary in a temporary directory. Use `go build` and run the compiled binary, or pass `--workflow-dir` explicitly.

### Project Directory Resolution

When `--project-dir` is not provided, the project directory defaults to the shell CWD at startup:

```go
func resolveProjectDir() (string, error) {
    cwd, err := os.Getwd()                       // current working directory
    resolved, err := filepath.EvalSymlinks(cwd)  // dereference symlinks
    info, err := os.Stat(resolved)
    if !info.IsDir() {
        return "", fmt.Errorf("project dir %q is not a directory", resolved)
    }
    return resolved, nil
}
```

### WorkflowDir vs ProjectDir

ralph-tui distinguishes two directories that are often conflated:

- **WorkflowDir** (install dir) — where ralph-tui's bundled `ralph-steps.json`, `scripts/`, `prompts/`, and `ralph-art.txt` live. Resolved from the executable path by default, or overridden by `--workflow-dir`.
- **ProjectDir** (target repo) — the user's shell CWD captured at startup via `os.Getwd()`. Governs subprocess `cmd.Dir` (so `gh`, `git`, and `claude` run against the target repo) and log file location (so `logs/` land alongside the work). Overridden by `--project-dir`.

Consumers in `main.go`:

| Consumer | Dir | Path Resolved |
|----------|-----|---------------|
| `logger.NewLogger(projectDir)` | ProjectDir | `{projectDir}/logs/ralph-*.log` |
| `steps.LoadSteps(workflowDir)` | WorkflowDir | `{workflowDir}/ralph-steps.json` |
| `validator.Validate(workflowDir)` | WorkflowDir | Validates `ralph-steps.json` relative to `{workflowDir}` |
| `workflow.NewRunner(log, projectDir)` | ProjectDir | Sets `cmd.Dir` for all subprocesses |
| `workflow.RunConfig.WorkflowDir` | WorkflowDir | Scripts, prompt files, `{{WORKFLOW_DIR}}` variable |

Within the workflow, `WorkflowDir` anchors two path-resolution mechanisms:
- `{workflowDir}/prompts/{promptFile}` — prompt files via `steps.BuildPrompt` (for Claude steps)
- Relative script paths from step config — resolved against `workflowDir` by `ResolveCommand` (e.g. `scripts/get_gh_user`, `scripts/close_gh_issue`); not hardcoded in `Run()`

The `{{WORKFLOW_DIR}}` template variable resolves to `WorkflowDir` (install dir) — e.g., `{{WORKFLOW_DIR}}/ralph-art.txt` in `ralph-steps.json` refers to the bundled banner file alongside the binary. The `{{PROJECT_DIR}}` template variable resolves to `ProjectDir` (target repo).

## Error Handling

| Scenario | Error Message | Behavior |
|----------|---------------|----------|
| Negative iterations | `"cli: --iterations must be a non-negative integer"` | Exit 1 |
| Unknown flag | pflag error + `flagSplitGuidance` hint | Exit 1 |
| Positional argument given | cobra error (`"unknown command"` / `"accepts 0 arg(s)"`) | Exit 1 |
| Cannot resolve workflow dir | `"cli: could not resolve workflow dir: ..."` | Exit 1 |
| Cannot resolve project dir | `"cli: could not resolve project dir: ..."` | Exit 1 |
| `--workflow-dir` points to a file | `"cli: --workflow-dir %q is not a directory"` | Exit 1 |
| `--project-dir` points to a file | `"cli: --project-dir %q is not a directory"` | Exit 1 |

All errors are written to stderr followed by a `Run 'ralph-tui --help' for usage.` hint, and cause `os.Exit(1)`.

## Configuration

| Flag | Short | Description | Default |
|------|-------|-------------|---------|
| `--iterations` | `-n` | Number of iterations to run (0 = run until done) | `0` |
| `--workflow-dir` | — | Path to the workflow bundle directory (install dir) | Resolved from executable location |
| `--project-dir` | — | Path to the target repository | Current working directory |
| `--version` | `-v` | Print `ralph-tui version <semver>` and exit without running the workflow | — |
| `--help` | `-h` | Print cobra-generated usage and exit without running the workflow | — |

**Usage:**

```
ralph-tui [--iterations <n>] [--workflow-dir <path>] [--project-dir <path>]
ralph-tui create-sandbox [--force]
ralph-tui --version
```

## Testing

- `ralph-tui/internal/cli/args_test.go` — 28 test cases covering all `NewCommand` and `Execute` branches

### Test Cases

| Test | What It Validates |
|------|-------------------|
| `TestNewCommand_NoFlags` | No flags → iterations=0, workflow-dir resolved from executable (non-empty), project-dir resolved from CWD (non-empty) |
| `TestNewCommand_LongIterationsFlag` | `--iterations 3` → iterations=3 |
| `TestNewCommand_ShortIterationsFlag` | `-n 3` → iterations=3 |
| `TestNewCommand_NegativeIterations` | `--iterations -1` → error containing expected message |
| `TestNewCommand_LongWorkflowDirFlag` | `--workflow-dir /tmp/foo` → workflow-dir=/tmp/foo |
| `TestNewCommand_LongProjectDirFlag` | `--project-dir /tmp/foo` → project-dir=/tmp/foo |
| `TestNewCommand_BothFlags` | `-n 3 --workflow-dir /tmp/foo` → both fields set |
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
| `TestNewCommand_WorkflowDirNonexistent` | `--workflow-dir /nonexistent` → error |
| `TestNewCommand_ProjectDirNonexistent` | `--project-dir /nonexistent` → error |
| `TestNewCommand_ShortPFlagFiresGuidanceMessage` | `-p /tmp` → error containing `flagSplitGuidance` message |
| `TestNewCommand_WorkflowDirEvalSymlinks` | Symlinked `--workflow-dir` → resolved to real path |
| `TestNewCommand_ProjectDirEvalSymlinks` | Symlinked `--project-dir` → resolved to real path |
| `TestNewCommand_WorkflowDirIsFile` | `--workflow-dir` pointing to a file → error |
| `TestNewCommand_ProjectDirIsFile` | `--project-dir` pointing to a file → error |
| `TestNewCommand_ArbitraryUnknownFlagFiresGuidanceMessage` | Any unknown flag → error contains `flagSplitGuidance` message |
| `TestNewCommand_NoShortFormsForDirFlags` | `-w` and `-p` are not registered → error for each |
| `TestNewCommandImpl_AddedSubcommandRunsItsRunE` | Subcommand added via `AddCommand` fires its `RunE` when invoked by name |

The two version tests read the expected string from `version.Version` rather than hardcoding `"0.1.0"` — the pattern required by [Versioning](../coding-standards/versioning.md) so a version bump does not require touching the test file.

## Additional Information

- [Architecture Overview](../architecture.md) — System-level view of ralph-tui with block diagrams and data flow
- [ADR: Use Cobra for CLI Argument Parsing](../adr/20260409135303-cobra-cli-framework.md) — Decision rationale for replacing stdlib `flag` with cobra
- [ADR: workflow-dir / project-dir split](../adr/20260413162428-workflow-project-dir-split.md) — Decision rationale for splitting the single `--project-dir` flag into `--workflow-dir` + `--project-dir`
- [Versioning](../coding-standards/versioning.md) — Single-source-of-truth rule for `version.Version`, what counts as ralph-tui's public API (CLI flags, `--version` output format), and how to bump the version
- [Building Custom Workflows](../how-to/building-custom-workflows.md) — How WorkflowDir affects config and prompt file resolution
- [Step Definitions & Prompt Building](step-definitions.md) — How WorkflowDir resolves config and prompt files
- [Subprocess Execution & Streaming](subprocess-execution.md) — How ProjectDir sets the working directory for subprocesses
- [Workflow Orchestration](workflow-orchestration.md) — How RunConfig carries WorkflowDir and Iterations into the Run loop
- [TUI Status Header & Log Display](tui-display.md) — Where the `version.Version` constant is rendered as the footer label
- [File Logging](file-logging.md) — How ProjectDir determines the log file location
- [ralph-tui Plan](../plans/ralph-tui.md) — Original specification including CLI design decisions
- [Go Patterns](../coding-standards/go-patterns.md) — Coding standards for symlink-safe path resolution
