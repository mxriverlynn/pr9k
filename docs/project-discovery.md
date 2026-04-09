# Project Discovery

- **Last Updated:** 2026-04-09

## Repository

- Default branch: main
- CLAUDE.md: `CLAUDE.md`
- README: `README.md`

## Documentation

- Docs: `docs/`
- Plans: `docs/plans/`
- Coding standards: `docs/coding-standards/` (`error-handling.md`, `testing.md`, `concurrency.md`, `api-design.md`, `go-patterns.md`)
- Prompts: `prompts/` (8 markdown prompt files consumed by both orchestrators)

## scripts

- Root: `scripts/`
- Language: Bash
- Helper scripts used by the orchestrator: `get_next_issue`, `close_gh_issue`, `get_gh_user`, `get_commit_sha`, `box-text`

## ralph-tui

- Root: `ralph-tui/`
- Language: Go 1.23
- Package manager: Go modules
- Dependency manifest: `ralph-tui/go.mod`
- Module: `github.com/mxriverlynn/pr9k/ralph-tui`
- External dependencies: `github.com/spf13/cobra` v1.10.2 (and transitive deps: pflag v1.0.9, mousetrap v1.1.0)

### Frameworks and Tooling

- CLI: spf13/cobra v1.10.2 (ADR: [20260409135303-cobra-cli-framework](adr/20260409135303-cobra-cli-framework.md))
- TUI: Glyph (planned, not yet in go.mod)
- Task runner: Make (`Makefile` at repo root)

### Commands and Tests

- Build: `make build` or `cd ralph-tui && go build -o ../ralph-tui ./cmd/ralph-tui`
- Run: `./bin/ralph-tui <iterations>` or `./ralph-tui <iterations> [-project-dir <path>]`
- Test: `make test` or `cd ralph-tui && go test -race ./...`
- Lint: `make lint` (requires golangci-lint)
- Format check: `make format`
- Vet: `make vet`
- Vulnerability check: `make vulncheck` (requires govulncheck)
- CI (all checks): `make ci`
- Test file pattern: `*_test.go` (co-located with source)
- Test directories: `internal/cli/`, `internal/ui/`, `internal/steps/`, `internal/logger/`, `internal/workflow/`, `configs/`

### Configuration

- Step definitions: `ralph-tui/ralph-steps.json`
- Claude Code settings: `.claude/settings.json`, `.claude/settings.local.json`

## Additional Information

- [Architecture Overview](architecture.md) — System-level architecture of ralph-tui with block diagrams and feature summaries
- [CLI & Configuration](features/cli-configuration.md) — CLI argument parsing and project directory resolution details
- [Step Definitions & Prompt Building](features/step-definitions.md) — JSON step configuration format and prompt building
- **How-To Guides:**
  - [Building Custom Workflows](how-to/building-custom-workflows.md) — Creating custom step sequences, adding prompts, mixing Claude and shell steps
  - [Variable Output & Injection](how-to/variable-output-and-injection.md) — Variable injection into prompts/commands and file-based data passing between steps
- **Coding Standards** — Conventions governing Go code in ralph-tui:
  - [API Design](coding-standards/api-design.md), [Concurrency](coding-standards/concurrency.md), [Error Handling](coding-standards/error-handling.md), [Go Patterns](coding-standards/go-patterns.md), [Testing](coding-standards/testing.md)
- [ralph-tui Plan](plans/ralph-tui.md) — Original specification and design rationale
