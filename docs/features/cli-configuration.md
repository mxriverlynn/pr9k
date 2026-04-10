# CLI & Configuration

Parses command-line flags and resolves the project directory that anchors all relative path resolution throughout ralph-tui.

- **Last Updated:** 2026-04-09
- **Authors:**
  - River Bailey

## Overview

- ralph-tui accepts an optional `--iterations` / `-n` flag (default 0 = run until done) and an optional `--project-dir` / `-p` flag
- Built on [spf13/cobra](https://github.com/spf13/cobra), which handles POSIX-style flags in any position — no custom reordering needed
- When `--project-dir` is not provided, the project directory is resolved from the executable's real path via `os.Executable()` + `filepath.EvalSymlinks` (symlink-safe)
- `ProjectDir` fans out to every subsystem: logger, step loader, prompt builder, command resolver, and the workflow runner

Key files:
- `ralph-tui/internal/cli/args.go` — `Execute`, `NewCommand`, `Config`, `resolveProjectDir`
- `ralph-tui/internal/cli/args_test.go` — 16 test cases covering all argument parsing branches
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

`newCommandImpl` builds the cobra command with `cobra.NoArgs` (positional arguments are rejected) and defines two flags:

```go
cmd.Flags().IntVarP(&cfg.Iterations, "iterations", "n", 0, "number of iterations to run (0 = run until done)")
cmd.Flags().StringVarP(&cfg.ProjectDir, "project-dir", "p", "", "path to the project directory (default: resolved from executable)")
```

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
| `workflow.RunConfig.ProjectDir` | Banner, scripts, prompt files, command resolution |

Within `workflow.Run`, `ProjectDir` resolves additional paths:
- `ralph-art.txt` — startup banner (embedded in the binary via `//go:embed`)
- `{projectDir}/scripts/get_gh_user` — GitHub username script
- `{projectDir}/scripts/get_next_issue` — issue fetch script
- `{projectDir}/prompts/{promptFile}` — prompt files via `steps.BuildPrompt`

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

**Usage:**

```
ralph-tui [--iterations <n>] [--project-dir <path>]
```

## Testing

- `ralph-tui/internal/cli/args_test.go` — 16 test cases covering all `NewCommand` and `Execute` branches

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

## Additional Information

- [Architecture Overview](../architecture.md) — System-level view of ralph-tui with block diagrams and data flow
- [ADR: Use Cobra for CLI Argument Parsing](../adr/20260409135303-cobra-cli-framework.md) — Decision rationale for replacing stdlib `flag` with cobra
- [Building Custom Workflows](../how-to/building-custom-workflows.md) — How ProjectDir affects config and prompt file resolution
- [Step Definitions & Prompt Building](step-definitions.md) — How ProjectDir resolves config and prompt files
- [Subprocess Execution & Streaming](subprocess-execution.md) — How ProjectDir sets the working directory for subprocesses
- [Workflow Orchestration](workflow-orchestration.md) — How RunConfig carries ProjectDir and Iterations into the Run loop
- [File Logging](file-logging.md) — How ProjectDir determines the log file location
- [ralph-tui Plan](../plans/ralph-tui.md) — Original specification including CLI design decisions
- [Go Patterns](../coding-standards/go-patterns.md) — Coding standards for symlink-safe path resolution
