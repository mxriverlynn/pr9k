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

## Accepted Risks

1. **Unbounded loop with no safety cap.** When `--iterations` is omitted (until-done mode), the loop relies entirely on `get_next_issue` returning empty to terminate. If issue processing fails to make progress (e.g., same issue returned repeatedly due to a close-issue failure), the loop runs indefinitely. This is accepted because: (a) the user can always Ctrl-C to exit, (b) adding an arbitrary cap would undermine the "run until done" intent, and (c) duplicate-issue detection adds complexity disproportionate to the risk. Can be revisited if real-world usage reveals a problem.

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

10. **Export `NewCommand(cfg *Config) *cobra.Command` and test against it.** `Execute()` creates a `&Config{}` and passes it to `NewCommand()`. `RunE` populates the config through the pointer. Tests create their own `&Config{}`, call `NewCommand(cfg)`, use `cmd.SetArgs()`, execute, and inspect `cfg`. Tests the real cobra wiring including flag parsing, defaults, and validation.

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
- Add `NewCommand(cfg *Config) *cobra.Command` (accepts a pointer that `RunE` populates — allows `Execute()` to return the config and tests to inspect it):
  - `Use: "ralph-tui [flags]"`, `Short`, `Long` per decision #15
  - `Args: cobra.NoArgs`
  - `SilenceErrors: true`, `SilenceUsage: true` — prevents cobra from printing its own error/usage output, since `main.go` already handles error formatting
  - Flags: `--iterations` / `-n` (int, default 0), `--project-dir` / `-p` (string, default "")
  - `RunE`: validate `--iterations >= 0`, resolve project-dir if empty, populate `Config`
- Add `Execute() (*Config, error)`:
  - Creates `&Config{}`, calls `NewCommand(cfg)`, runs `cmd.Execute()`, returns the config
  - **Guard against `--help`/no-`RunE` path:** Track whether `RunE` actually executed (e.g., a `bool` set inside `RunE`). If `cmd.Execute()` returns nil but `RunE` never ran (help was printed), return `nil, nil` — `main.go` checks for `cfg == nil` and exits cleanly without starting the workflow

### Step 3: Rewrite `internal/cli/args_test.go`
- Test via `cfg := &Config{}` + `NewCommand(cfg)` + `cmd.SetArgs()` + `cmd.Execute()` + assert on `cfg`
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
  - `--help` → Execute() returns nil config, nil error (no workflow started)
  - Large iteration counts accepted

### Step 4: Update `cmd/ralph-tui/main.go`
- Replace `cfg, err := cli.ParseArgs(os.Args[1:])` with `cfg, err := cli.Execute()`
- Add nil check: if `cfg == nil && err == nil`, exit 0 (help was printed, no work to do)
- Rest of main.go unchanged — it already consumes `cfg.Iterations` and `cfg.ProjectDir`

### Step 5: Update loop in `workflow/run.go`
- Replace `for i := 1; i <= cfg.Iterations; i++` with `for i := 1; cfg.Iterations == 0 || i <= cfg.Iterations; i++` — runs unbounded when 0, bounded otherwise. Preserve all existing early-exit paths:
  - `break` when `get_next_issue` returns empty (line 75-78)
  - `break` when `buildIterationSteps` returns an error (line 94-96)
  - `return` when `Orchestrate` returns `ActionQuit` (line 99-101)
- Adjust all format strings that use `%d/%d` iteration formatting:
  - Separator log: bounded → `"Iteration 3/5 — Issue #42"`, unbounded → `"Iteration 3 — Issue #42"`
  - "No issue found" log (line 76): bounded → `"Iteration 3/5 — No issue found."`, unbounded → `"Iteration 3 — No issue found."`
  - Use a helper or inline conditional to format `iterationLabel(i, total)` for both messages
- Pass `cfg.Iterations` (which is 0 for unbounded) to `header.SetIteration(i, cfg.Iterations, ...)`

### Step 6: Update header formatting in UI
- In `SetIteration` implementation, use conditional formatting:
  - `total > 0`: `fmt.Sprintf("Iteration %d/%d — Issue #%s: %s", current, total, issueID, issueTitle)`
  - `total == 0`: `fmt.Sprintf("Iteration %d — Issue #%s: %s", current, issueID, issueTitle)`

### Step 7: Update workflow tests
- Add/update tests in `workflow/run_test.go` (if exists) to cover unbounded loop behavior
- Verify bounded loop still works as before

### Step 8: Update documentation
- Update all files that reference the old `ralph-tui <iterations>` positional syntax:
  - `CLAUDE.md` (lines 48, 52) — change to `ralph-tui [-n <iterations>] [-p <project-dir>]`
  - `docs/project-discovery.md` (line 41) — same
  - `docs/features/cli-configuration.md` — rewrite to reflect cobra flags, remove `reorderArgs` documentation
  - `docs/plans/ralph-tui.md` (lines 432, 438, 648) — update examples
  - `docs/architecture.md` — update if it references CLI syntax
- Document the new "until done" mode (omitting `--iterations` runs until no issues remain)

### Step 9: Verify and clean up
- `make test` — all tests pass with `-race`
- `make lint` — no lint issues
- `make vet` — no vet issues
- `make build` — binary builds and runs
- Delete any remaining dead code (confirm `reorderArgs` is gone, no orphan imports)

## Review Summary

**Iterations completed:** 3 (stopped at iteration 3 — below 80% chance of structural improvement)

**Assumptions challenged:** 13 primary and secondary assumptions evaluated across iterations:
- 10 verified against codebase evidence
- 2 revealed gaps that required plan changes (A9: SilenceErrors, A12: NewCommand signature)
- 1 uncertain, clarified (A11: loop construct)

**Agent validation findings incorporated:**
- Evidence-based investigator: confirmed 5/6 assumptions, found 1 missing loop exit path (`buildIterationSteps` error break) — added to Step 5
- Adversarial validator: found 3 actionable gaps:
  - V1: `--help` returns unpopulated config — added `RunE`-executed guard to `Execute()` and nil-config check to `main.go`
  - V3: 5+ documentation files reference old CLI syntax — added Step 8 (documentation update)
  - V4: `SetIteration` format variants not explicit — specified both in Step 6
  - V2: unbounded loop infinite-loop risk — documented as accepted risk with rationale

**Consolidations:** None needed (no internal or external overlap detected)

**Ambiguities resolved:** 0 surfaced (all design questions were pre-resolved in the decisions section)
