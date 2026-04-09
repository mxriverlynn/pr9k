# Use Cobra for CLI Argument Parsing

- **Status:** proposed
- **Date Created:** 2026-04-09 13:53
- **Last Updated:** 2026-04-09 13:53
- **Authors:**
  - River Bailey (mxriverlynn, river.bailey@testdouble.com)
- **Reviewers:**

## Context

The ralph-tui Go application currently uses Go's stdlib `flag` package for CLI argument parsing (`ralph-tui/internal/cli/args.go`). This works for the current minimal interface (one positional `iterations` argument and an optional `-project-dir` flag), but requires a custom `reorderArgs()` function to work around `flag`'s limitation of stopping at the first non-flag argument.

The project needs to add POSIX-style flags (`--long-flag`) and subcommand support, neither of which stdlib `flag` provides. A third-party CLI framework is needed.

## Decision Drivers

- **Long-term ecosystem support** — the chosen library must be actively maintained with a large community to avoid future migration headaches
- **POSIX-style flags** — support for `--long-flag` and `-s` short flags
- **Subcommand support** — ability to define subcommands (e.g., `ralph-tui run`, `ralph-tui config`)
- **Help generation** — automatic, well-formatted help output

## Considered Options

1. **spf13/cobra** — Industry-standard Go CLI framework used by kubectl, Docker CLI, Hugo, and GitHub CLI. Provides subcommands, POSIX flags (via pflag), auto-generated help/man pages, and shell completions. Uses a multi-file `cmd/` package pattern with `init()` registration functions. 4 transitive dependencies (pflag, mousetrap, go-md2man, yaml.v3).

   - Pros: Largest Go CLI ecosystem; battle-tested in major projects; extensive documentation; shell completion generation; active maintenance
   - Cons: More boilerplate than alternatives (~100 lines across 3+ files for a basic subcommand); `init()` registration pattern adds ceremony; positional args require manual type conversion

2. **alecthomas/kong** — Struct-tag-driven declarative CLI framework. Subcommands defined via embedded structs. Automatic type conversion from struct field types. ~30 lines for the same functionality as cobra's ~100.

   - Pros: Minimal boilerplate; existing `Config` struct maps naturally to kong's model; automatic type parsing; near-zero dependencies
   - Cons: Significantly smaller community; struct-tag errors are opaque to debug; less documentation; fewer projects using it in production

3. **urfave/cli v3** — Procedural API with `Command` slices. ~60 lines for the same functionality. Zero external dependencies.

   - Pros: Less boilerplate than cobra; no external dependencies; straightforward procedural API
   - Cons: Smaller ecosystem than cobra; v2-to-v3 migration caused community disruption; less battle-tested at v3

## Decision

We will use **spf13/cobra** because long-term ecosystem stability and community support are the primary concerns. Cobra is the de facto standard for Go CLI applications, backed by the largest community, the most production usage, and the most active maintenance. The additional boilerplate compared to kong or urfave/cli is an acceptable tradeoff for the confidence that the framework will remain supported and well-documented for years to come.

Migration effort from stdlib `flag` to the chosen framework was explicitly not a factor in this decision.

## Consequences

**Positive:**

- POSIX-style flags and subcommands out of the box, eliminating the custom `reorderArgs()` workaround
- Auto-generated help text, shell completions, and man pages
- Extensive community resources, tutorials, and examples for onboarding contributors
- Reduced risk of needing to migrate again due to an abandoned or poorly-supported library

**Negative:**

- More boilerplate than kong or urfave/cli — requires `cmd/` package structure with `init()` functions per subcommand
- Positional arguments are received as `[]string` and require manual type conversion and validation
- Adds 4 transitive dependencies (pflag, mousetrap, go-md2man, yaml.v3)

**Neutral:**

- The existing `cli.Config` struct and `args_test.go` test suite will need to be rewritten to use cobra's patterns
- The `reorderArgs()` function and its tests can be deleted entirely

## Notes

### Key Files

| File | Purpose |
|------|---------|
| `ralph-tui/internal/cli/args.go` | Current CLI parsing logic to be replaced |
| `ralph-tui/internal/cli/args_test.go` | Current test suite (11 cases) to be rewritten |
| `ralph-tui/cmd/ralph-tui/main.go` | Entry point that calls `cli.ParseArgs()` and wires config into the app |
| `ralph-tui/internal/workflow/run.go` | Downstream consumer of `cli.Config` via `workflow.RunConfig` |

### Related Docs

- [CLI Configuration Feature Doc](../features/cli-configuration.md) — documents current flag parsing and project directory resolution
- [Architecture Overview](../architecture.md) — system-level architecture including CLI entry point
