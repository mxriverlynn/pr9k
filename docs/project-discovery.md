# Project Discovery

- **Last Updated:** 2026-04-08

## Repository

- Default branch: main
- CLAUDE.md: `CLAUDE.md`
- README: `README.md`

## Documentation

- Docs: `docs/`
- Plans: `docs/plans/`
- Coding standards: `docs/coding-standards/` (empty)
- Prompts: `prompts/` (8 markdown prompt files consumed by both orchestrators)

## scripts

- Root: `scripts/`
- Language: Bash
- Helper scripts shared by both orchestrators: `get_next_issue`, `close_gh_issue`, `get_gh_user`, `get_commit_sha`, `box-text`

## ralph-bash

- Root: `ralph-bash/`
- Language: Bash
- Runtime dependencies: `claude` CLI, `gh` CLI, `git`

### Commands and Tests

- Run: `path/to/ralph-loop <iterations>` (from target repo)
- HITL: `path/to/ralph-hitl [prompt-name]` (from target repo)
- No tests, linter, formatter, or build step

### Configuration

- Claude Code settings: `ralph-bash/.claude/settings.local.json`

## ralph-tui

- Root: `ralph-tui/`
- Language: Go 1.23
- Package manager: Go modules
- Dependency manifest: `ralph-tui/go.mod`
- Module: `github.com/mxriverlynn/pr9k/ralph-tui`
- No external dependencies declared yet (Glyph TUI framework planned)

### Frameworks and Tooling

- TUI: Glyph (planned, not yet in go.mod)
- Task runner: none (bare `go` commands)

### Commands and Tests

- Build: `cd ralph-tui && go build -o ../ralph-tui ./cmd/ralph-tui`
- Run: `./ralph-tui <iterations> [-project-dir <path>]`
- Test: `cd ralph-tui && go test ./...`
- Test file pattern: `*_test.go` (co-located with source)
- Test directories: `internal/cli/`, `internal/ui/`, `internal/steps/`, `internal/logger/`, `internal/workflow/`, `configs/`

### Configuration

- Step definitions: `ralph-tui/configs/ralph-steps.json`, `ralph-tui/configs/ralph-finalize-steps.json`
- Claude Code settings: `.claude/settings.json`, `.claude/settings.local.json`
