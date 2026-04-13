# Project Discovery

- **Last Updated:** 2026-04-13

## Repository

- Default branch: main
- CLAUDE.md: `CLAUDE.md`
- README: `README.md`

## Documentation

- Docs: `docs/`
- Plans: `docs/plans/`
- Coding standards: `docs/coding-standards/` (`error-handling.md`, `testing.md`, `concurrency.md`, `api-design.md`, `go-patterns.md`, `lint-and-tooling.md`, `versioning.md`)
- Prompts: `prompts/` (markdown prompt files consumed by the orchestrator)

## scripts

- Root: `scripts/`
- Language: Bash
- Helper scripts used by the orchestrator: `get_next_issue`, `close_gh_issue`, `get_gh_user`, `get_commit_sha`, `box-text`

## ralph-tui

- Root: `ralph-tui/`
- Language: Go 1.26.2
- Package manager: Go modules
- Dependency manifest: `ralph-tui/go.mod`
- Module: `github.com/mxriverlynn/pr9k/ralph-tui`
- Current version: `0.2.3` (single source of truth: `ralph-tui/internal/version/version.go`)
- External dependencies: `github.com/charmbracelet/bubbletea` v1.3.10 (TUI framework), `github.com/charmbracelet/lipgloss` v1.1.0 (styling), `github.com/charmbracelet/bubbles` v1.0.0 (viewport widget), `github.com/spf13/cobra` v1.10.2, `golang.org/x/sys` v0.40.0

### Frameworks and Tooling

- CLI: spf13/cobra v1.10.2 (ADR: [20260409135303-cobra-cli-framework](adr/20260409135303-cobra-cli-framework.md))
- TUI: [Bubble Tea](https://github.com/charmbracelet/bubbletea) + [Lip Gloss](https://github.com/charmbracelet/lipgloss) + [bubbles/viewport](https://github.com/charmbracelet/bubbles) (see ADR below)
- Terminal size detection: `golang.org/x/sys/unix` (ioctl TIOCGWINSZ) for full-width phase banners
- Task runner: Make (`Makefile` at repo root)
- Linter: golangci-lint v2.11.4 (pinned in CI)

### Architecture Decision Records

- [Cobra CLI Framework](adr/20260409135303-cobra-cli-framework.md) — Decision to use spf13/cobra for CLI argument parsing
- [Narrow-Reading Principle](adr/20260410170952-narrow-reading-principle.md) — Ralph-tui is a generic step runner; workflow content lives in `ralph-steps.json`, not Go code
- [Bubble Tea TUI Framework](adr/20260411070907-bubble-tea-tui-framework.md) — Decision to migrate from Glyph to Bubble Tea + Lip Gloss + bubbles for dynamic window title, mouse-wheel scrolling, and ecosystem stability
- [Workflow/Project Dir Split](adr/20260413162428-workflow-project-dir-split.md) — Decision to split `--project-dir` into `--workflow-dir` (workflow bundle) and `--project-dir` (target repo)

### Commands and Tests

- Build: `make build` or `cd ralph-tui && go build -o ../ralph-tui ./cmd/ralph-tui`
- Run: `./bin/ralph-tui [-n <iterations>] [--workflow-dir <path>] [--project-dir <path>]` (omit `-n` for until-done mode)
- Setup (Docker sandbox): `./bin/ralph-tui create-sandbox [--force]` — pulls sandbox image and runs smoke test
- Test: `make test` or `cd ralph-tui && go test -race ./...`
- Lint: `make lint` (requires golangci-lint)
- Format check: `make format`
- Vet: `make vet`
- Vulnerability check: `make vulncheck` (requires govulncheck)
- CI (all checks): `make ci`
- Test file pattern: `*_test.go` (co-located with source)
- Test directories: `internal/cli/`, `internal/ui/`, `internal/steps/`, `internal/logger/`, `internal/workflow/`, `internal/vars/`, `internal/validator/`, `internal/sandbox/`, `internal/preflight/`, `cmd/ralph-tui/`

### Configuration

- Step definitions: `ralph-tui/ralph-steps.json`
- Claude Code settings: `.claude/settings.json`, `.claude/settings.local.json`
- Dependency pinning: `ralph-tui/tools.go` — blank imports under `//go:build tools` to pin Bubble Tea dependencies before production code imports them; verified via `go vet -tags tools .` (as `make vet` does)

## Additional Information

- [Architecture Overview](architecture.md) — System-level architecture of ralph-tui with block diagrams and feature summaries
- [CLI & Configuration](features/cli-configuration.md) — CLI argument parsing and project directory resolution details
- [Step Definitions & Prompt Building](features/step-definitions.md) — JSON step configuration format and prompt building
- [Variable State Management](features/variable-state.md) — `VarTable` scoped variable tables, built-in variables, and phase-based resolution
- [Config Validation](features/config-validation.md) — D13 validator: ten categories, sandbox rules A/B/C, env passthrough validation
- [Docker Sandbox](features/sandbox.md) — `BuildRunArgs`, `BuiltinEnvAllowlist`, cidfile lifecycle, and `NewTerminator`
- [Preflight Checks](features/preflight.md) — `Prober` interface, `CheckDocker`, profile dir validation, collect-all-errors `Run`
- [create-sandbox Subcommand](features/create-sandbox.md) — Docker image pull and smoke test for sandbox setup
- **How-To Guides:**
  - [Building Custom Workflows](how-to/building-custom-workflows.md) — Creating custom step sequences, adding prompts, mixing Claude and shell steps
  - [Variable Output & Injection](how-to/variable-output-and-injection.md) — Variable injection into prompts/commands and file-based data passing between steps
- **Coding Standards** — Conventions governing Go code in ralph-tui:
  - [API Design](coding-standards/api-design.md), [Concurrency](coding-standards/concurrency.md), [Error Handling](coding-standards/error-handling.md), [Go Patterns](coding-standards/go-patterns.md), [Testing](coding-standards/testing.md), [Lint and Tooling](coding-standards/lint-and-tooling.md), [Versioning](coding-standards/versioning.md)
- [ralph-tui Plan](plans/ralph-tui.md) — Original specification and design rationale
