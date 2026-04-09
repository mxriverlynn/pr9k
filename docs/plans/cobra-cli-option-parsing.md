# Plan: Integrate Cobra for CLI Option Parsing

**Status:** ready
**ADR:** [20260409135303-cobra-cli-framework.md](../adr/20260409135303-cobra-cli-framework.md)

## Goal

Replace the stdlib `flag`-based CLI parsing in `ralph-tui/internal/cli/args.go` with spf13/cobra, gaining POSIX-style flags, subcommands, and auto-generated help.

## Current State

- Entry point: `ralph-tui/cmd/ralph-tui/main.go` calls `cli.ParseArgs(os.Args[1:])`
- Parsing: `ralph-tui/internal/cli/args.go` uses `flag.FlagSet` + custom `reorderArgs()`
- Config struct: `cli.Config{Iterations int, ProjectDir string}`
- Tests: `ralph-tui/internal/cli/args_test.go` (11 test cases)
- Downstream consumer: `workflow.RunConfig` receives `Config.ProjectDir` and `Config.Iterations`

## Open Questions

None — all design decisions resolved.

## Decisions

1. **`iterations` becomes a flag, not a positional argument.** Use `--iterations` / `-n` with automatic int parsing via cobra/pflag. Eliminates manual `strconv.Atoi` and makes help output self-documenting. Command becomes `ralph-tui -n 3` or `ralph-tui --iterations 3`.

2. **`iterations` is optional; omitting it means "run until no work remains."** When `--iterations` is not provided, the loop runs continuously until `get_next_issue` finds no more issues. When provided, it caps at that count. This changes loop semantics from "always fixed count" to "fixed count OR until-done."

3. **Represent "until done" as `Iterations == 0` in `cli.Config`.** Keep `Iterations int` (no pointer, no extra bool). Zero is a safe sentinel since zero iterations is never a valid user request. Cobra's default int value is `0`, so the zero-value semantics align naturally.

4. **Root command only, no subcommands.** There's only one action today (run the loop). Subcommands can be added later trivially. No need to introduce a `run` subcommand with nothing to distinguish it from.

5. **Cobra command definition stays in `internal/cli/`.** No `cmd/` package pattern -- we don't have subcommands to organize. Keeps the diff small, preserves existing package structure, and `internal/` correctly signals this is not a public API. Can move to `cmd/` pattern later if subcommands warrant it.

6. **`cli.Execute() (*Config, error)` returns the parsed config.** The cobra `RunE` validates and stores config in a local variable; `Execute` returns it. Most similar to today's `cli.ParseArgs()` call site, no package-level state, no callback ceremony. `main.go` changes from `cli.ParseArgs(os.Args[1:])` to `cli.Execute()`.

7. **`--project-dir` defaults to executable-relative resolution.** Keep the existing `resolveProjectDir()` logic (os.Executable + EvalSymlinks). This is load-bearing: ralph-steps.json, prompts, and scripts all resolve relative to projectDir.

8. **`--project-dir` / `-p` short flag.** Both long and short forms available.

9. **Defer `--project-dir` resolution to `RunE`.** Default is `""`. In `RunE`, if the flag wasn't provided, call `resolveProjectDir()` and return any error through cobra's error path. Keeps command construction infallible.

10. **Export `NewCommand() *cobra.Command` and test against it.** `Execute()` calls `NewCommand()` internally. Tests create the command via `NewCommand()`, use `cmd.SetArgs()`, and execute. Tests the real cobra wiring including flag parsing, defaults, and validation.

11. **Reject negative `--iterations` values.** `RunE` returns an error for `--iterations < 0`. Error message: `"--iterations must be a non-negative integer"`. Valid range: `0` (until done) or any positive integer.

12. **Use `cobra.NoArgs` to reject unexpected positional arguments.** Both args are flags now, so any positional arg is a user mistake. Fails fast with a clear error.

13. **Loop changes in `workflow/run.go` are included in this plan.** The "run until done" semantics are coupled to the flag change -- shipping `--iterations 0` without loop support would be broken. Changes: replace bounded `for i := 1; i <= N; i++` with conditional loop, and adjust display formatting to omit total when running in until-done mode (e.g., `"Iteration 3"` instead of `"Iteration 3/5"`).

14. **Pass `total=0` to `SetIteration` to mean "unknown total."** No interface signature change. The header implementation checks `if total == 0` to format as `"Iteration 3"` vs `"Iteration 3/5"`. Same sentinel logic as `Config.Iterations`.

15. **Cobra command descriptions:**
    - `Use`: `"ralph-tui [flags]"`
    - `Short`: `"Automated development workflow orchestrator"`
    - `Long`: `"ralph-tui drives the claude CLI through multi-step coding loops. By default, it picks up GitHub issues labeled \"ralph\", implements features, writes tests, runs code reviews, and pushes — all unattended. Custom workflow definitions can be provided to tailor the steps to your needs."`

16. **Replace `args.go` entirely.** Delete `reorderArgs()` and `ParseArgs()` -- both are replaced by the cobra implementation. Keep `resolveProjectDir()` (still needed). Rewrite `args_test.go` to test against `NewCommand()`. Clean break, no dead code.

## Implementation Steps

### Step 1: Add cobra dependency
- `cd ralph-tui && go get github.com/spf13/cobra`
- Verify `go.mod` includes cobra and its transitive deps (pflag, mousetrap, go-md2man, yaml.v3)

### Step 2: Rewrite `internal/cli/args.go` with cobra
- Delete `ParseArgs()` and `reorderArgs()`
- Keep `resolveProjectDir()` and `Config` struct
- Add `NewCommand() *cobra.Command`:
  - `Use: "ralph-tui [flags]"`, `Short`, `Long` per decision #15
  - `Args: cobra.NoArgs`
  - Flags: `--iterations` / `-n` (int, default 0), `--project-dir` / `-p` (string, default "")
  - `RunE`: validate `--iterations >= 0`, resolve project-dir if empty, populate `Config`
- Add `Execute() (*Config, error)`:
  - Calls `NewCommand()`, runs `cmd.Execute()`, returns the config

### Step 3: Rewrite `internal/cli/args_test.go`
- Test via `NewCommand()` + `cmd.SetArgs()` + `cmd.Execute()`
- Cover:
  - No flags → iterations=0, project-dir resolved from executable
  - `--iterations 3` → iterations=3
  - `-n 3` → iterations=3
  - `--iterations -1` → error
  - `--project-dir /tmp/foo` → project-dir=/tmp/foo
  - `-p /tmp/foo` → project-dir=/tmp/foo
  - Both flags together
  - Positional args rejected (cobra.NoArgs)
  - Unknown flags rejected
  - Large iteration counts accepted

### Step 4: Update `cmd/ralph-tui/main.go`
- Replace `cfg, err := cli.ParseArgs(os.Args[1:])` with `cfg, err := cli.Execute()`
- Rest of main.go unchanged — it already consumes `cfg.Iterations` and `cfg.ProjectDir`

### Step 5: Update loop in `workflow/run.go`
- Change `for i := 1; i <= cfg.Iterations; i++` to support both modes:
  - When `cfg.Iterations > 0`: bounded loop (same as today but using the flag value)
  - When `cfg.Iterations == 0`: unbounded loop, exits only when `get_next_issue` returns empty
- Adjust format strings:
  - Bounded: `"Iteration 3/5 — Issue #42"` (same as today)
  - Unbounded: `"Iteration 3 — Issue #42"` (no total)
- Pass `cfg.Iterations` (which is 0 for unbounded) to `header.SetIteration(i, cfg.Iterations, ...)`

### Step 6: Update header formatting in UI
- In `SetIteration` implementation: if `total == 0`, format as `"Iteration %d"` without total

### Step 7: Update workflow tests
- Add/update tests in `workflow/run_test.go` (if exists) to cover unbounded loop behavior
- Verify bounded loop still works as before

### Step 8: Verify and clean up
- `make test` — all tests pass with `-race`
- `make lint` — no lint issues
- `make vet` — no vet issues
- `make build` — binary builds and runs
- Delete any remaining dead code (confirm `reorderArgs` is gone, no orphan imports)
