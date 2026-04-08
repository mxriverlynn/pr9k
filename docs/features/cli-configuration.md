# CLI & Configuration

Parses command-line arguments and resolves the project directory that anchors all relative path resolution throughout ralph-tui.

- **Last Updated:** 2026-04-08 12:00
- **Authors:**
  - River Bailey

## Overview

- ralph-tui accepts a required `<iterations>` positional argument and an optional `-project-dir` flag
- The `reorderArgs` function works around Go's `flag` package limitation of stopping at the first positional argument, allowing flags in any position
- When `-project-dir` is not provided, the project directory is resolved from the executable's real path via `os.Executable()` + `filepath.EvalSymlinks` (symlink-safe)
- `ProjectDir` fans out to every subsystem: logger, step loader, prompt builder, command resolver, and the workflow runner

Key files:
- `ralph-tui/internal/cli/args.go` — ParseArgs, Config, reorderArgs, resolveProjectDir
- `ralph-tui/internal/cli/args_test.go` — 10 test cases covering all argument parsing branches
- `ralph-tui/cmd/ralph-tui/main.go` — Entry point that calls ParseArgs and distributes Config

## Architecture

```
                         os.Args[1:]
                             │
                             ▼
                      ┌─────────────┐
                      │ reorderArgs │  move flags before positionals
                      └──────┬──────┘
                             │
                             ▼
                      ┌─────────────┐
                      │  flag.Parse  │  extract -project-dir
                      └──────┬──────┘
                             │
                  ┌──────────┴──────────┐
                  │                     │
           -project-dir given     not given
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
| `ralph-tui/internal/cli/args.go` | ParseArgs, Config struct, reorderArgs, resolveProjectDir |
| `ralph-tui/internal/cli/args_test.go` | Unit tests for all argument parsing branches |
| `ralph-tui/cmd/ralph-tui/main.go` | Entry point — calls ParseArgs, distributes Config to subsystems |

## Core Types

```go
// Config holds parsed CLI arguments.
type Config struct {
    Iterations int    // number of workflow iterations to run (must be > 0)
    ProjectDir string // root directory for all relative path resolution
}
```

## Implementation Details

### Argument Parsing

`ParseArgs` creates an isolated `flag.FlagSet` with `flag.ContinueOnError` (returns errors instead of calling `os.Exit`). It defines one flag (`-project-dir`) and validates the positional `iterations` argument:

```go
func ParseArgs(args []string) (*Config, error) {
    fs := flag.NewFlagSet("ralph-tui", flag.ContinueOnError)
    projectDir := fs.String("project-dir", "", "path to the project directory")

    // Reorder so flags work in any position
    if err := fs.Parse(reorderArgs(args)); err != nil {
        return nil, err
    }

    positional := fs.Args()
    if len(positional) == 0 {
        return nil, errors.New("missing required argument: iterations")
    }

    iterations, err := strconv.Atoi(positional[0])
    // ... validates iterations > 0, resolves projectDir if empty
}
```

### Flag Reordering

Go's `flag` package stops parsing at the first non-flag token. Without `reorderArgs`, `ralph-tui 3 -project-dir /tmp` would leave `-project-dir` unprocessed. The function splits args into flag and positional slices, then returns flags first:

```go
func reorderArgs(args []string) []string {
    var flagArgs, positionalArgs []string
    // Walk args: tokens starting with "-" are flags (with their values),
    // "--" terminates flag scanning, everything else is positional.
    return append(flagArgs, positionalArgs...)
}
```

This enables both argument orders:
- `ralph-tui -project-dir /tmp 3`
- `ralph-tui 3 -project-dir /tmp`

Edge case: a negative number like `-1` is treated as a flag by `reorderArgs`. Use `--` to force it as positional: `ralph-tui -- -1` (which then fails validation since iterations must be > 0).

### Project Directory Resolution

When `-project-dir` is not provided, the project directory is inferred from the compiled binary's location:

```go
func resolveProjectDir() (string, error) {
    exe, err := os.Executable()              // path of the running binary
    resolved, err := filepath.EvalSymlinks(exe) // dereference symlinks
    return filepath.Dir(resolved), nil          // directory containing the binary
}
```

`filepath.EvalSymlinks` is critical: without it, a symlinked binary (e.g., installed in `~/bin/`) would resolve to the symlink's directory rather than where `configs/`, `prompts/`, and `scripts/` actually live.

This is why `go run` does not work — it places the binary in a temporary directory. Use `go build` and run the compiled binary, or pass `-project-dir` explicitly.

### ProjectDir Fan-Out

After parsing, `Config.ProjectDir` is distributed to five consumers in `main.go`:

| Consumer | Path Resolved |
|----------|---------------|
| `logger.NewLogger(projectDir)` | `{projectDir}/logs/ralph-*.log` |
| `steps.LoadSteps(projectDir)` | `{projectDir}/configs/ralph-steps.json` |
| `steps.LoadFinalizeSteps(projectDir)` | `{projectDir}/configs/ralph-finalize-steps.json` |
| `workflow.NewRunner(log, projectDir)` | Sets `cmd.Dir` for all subprocesses |
| `workflow.RunConfig.ProjectDir` | Banner, scripts, prompt files, command resolution |

Within `workflow.Run`, `ProjectDir` resolves additional paths:
- `{projectDir}/ralph-bash/ralph-art.txt` — startup banner
- `{projectDir}/scripts/get_gh_user` — GitHub username script
- `{projectDir}/scripts/get_next_issue` — issue fetch script
- `{projectDir}/prompts/{promptFile}` — prompt files via `steps.BuildPrompt`

## Error Handling

| Scenario | Error Message | Behavior |
|----------|---------------|----------|
| No arguments | `"missing required argument: iterations"` | Exit 1 |
| Non-integer iterations | `"iterations must be an integer, got %q"` | Exit 1 |
| Zero or negative iterations | `"iterations must be > 0"` | Exit 1 |
| Unknown flag | flag package error (e.g., `"flag provided but not defined: -foo"`) | Exit 1 |
| Cannot resolve executable | `"could not resolve project dir: ..."` | Exit 1 |

All errors are written to stderr and cause `os.Exit(1)`.

## Configuration

| Flag | Description | Default |
|------|-------------|---------|
| `-project-dir` | Path to the project root directory | Resolved from executable location |

**Usage:**

```
ralph-tui <iterations> [-project-dir <path>]
```

## Testing

- `ralph-tui/internal/cli/args_test.go` — 10 test cases covering all ParseArgs branches

### Test Cases

| Test | What It Validates |
|------|-------------------|
| `TestParseArgs_ValidIterations` | Happy path with `-project-dir` flag |
| `TestParseArgs_MissingIterations` | Empty args returns error |
| `TestParseArgs_ZeroIterations` | `"0"` is rejected |
| `TestParseArgs_ProjectDirOverride` | `-project-dir` value is used directly |
| `TestParseArgs_DefaultProjectDir` | Without flag, `ProjectDir` is non-empty (from executable) |
| `TestParseArgs_NonIntegerIterations` | `"abc"` returns error |
| `TestParseArgs_NegativeIterationsViaFlag` | `-1` treated as unknown flag |
| `TestParseArgs_NegativeIterationsViaSeparator` | `-- -1` reaches integer validator |
| `TestParseArgs_FlagBeforePositional` | Confirms `reorderArgs` works |
| `TestParseArgs_UnknownFlag` | Unknown flag returns error |
| `TestParseArgs_LargeIterations` | `"1000"` is accepted |

## Additional Information

- [Architecture Overview](../architecture.md) — System-level view of ralph-tui with block diagrams and data flow
- [Step Definitions & Prompt Building](step-definitions.md) — How ProjectDir resolves config and prompt files
- [Subprocess Execution & Streaming](subprocess-execution.md) — How ProjectDir sets the working directory for subprocesses
- [Workflow Orchestration](workflow-orchestration.md) — How RunConfig carries ProjectDir and Iterations into the Run loop
- [File Logging](file-logging.md) — How ProjectDir determines the log file location
- [ralph-tui Plan](../plans/ralph-tui.md) — Original specification including CLI design decisions
- [Go Patterns](../coding-standards/go-patterns.md) — Coding standards for flag reordering and symlink-safe path resolution
