# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Project Overview

**pr9k (Power-Ralph.9000)** is an automated development workflow orchestrator that drives the `claude` CLI through multi-step coding loops. It picks up GitHub issues labeled "ralph", implements features, writes tests, runs code reviews, and pushes — all unattended.

Based on [AI Hero's Getting Started with Ralph](https://www.aihero.dev/getting-started-with-ralph).

## Repository Structure

- `ralph-bash/` — Original bash implementation
  - `ralph-loop` — Main orchestrator script. Run from the **target repo**, not from this repo: `path/to/ralph-loop <iterations>`
  - `ralph-hitl` — Human-in-the-loop single-prompt runner: `path/to/ralph-hitl [prompt-name]`
  - `scripts/` — Helper scripts (`get_next_issue`, `close_gh_issue`, `get_gh_user`, `get_commit_sha`, `box-text`)
- `prompts/` — Prompt files consumed by both bash and TUI orchestrators. Each prompt is passed to `claude -p`. Iteration prompts get `ISSUENUMBER=` and `STARTINGSHA=` prepended.
- `docs/plans/` — Implementation plans (e.g., `ralph-tui.md` for the Go/Glyph TUI replacement)

## Workflow: The Ralph Loop

Each iteration:
1. Find next open issue assigned to user with label "ralph" (lowest number first)
2. Feature work (sonnet) → Test planning (opus) → Test writing (sonnet) → Code review (opus) → Review fixes (sonnet) → Close issue → Update docs (sonnet) → Git push

After all iterations, three finalization steps run:
1. Deferred work — creates issues from `deferred.txt`
2. Lessons learned — codifies from `progress.txt`
3. Final git push

Intermediate files (`progress.txt`, `deferred.txt`, `test-plan.md`, `code-review.md`) are created in the **target repo's working directory**, never committed, and consumed/deleted by later steps.

## Key Design Decisions

- Ralph is invoked **from the target repo** — all subprocesses inherit that cwd
- `ralph-loop` resolves its own directory via `$(dirname "$0")` to find prompts and scripts regardless of where it's called from
- The `get_next_issue` script sorts open issues and picks the lowest number
- Non-claude steps (`close_gh_issue`, `git push`) capture stderr with `2>&1`

## Planned: ralph-tui (Go/Glyph)

A Go TUI replacement for `ralph-loop` is planned in `docs/plans/ralph-tui.md`. It will live in `ralph-tui/` and use [Glyph](https://useglyph.sh/) for real-time streaming output. Key architectural notes:
- Uses `io.Pipe` for subprocess streaming (Glyph's `Log` component takes an `io.Reader`)
- Step definitions loaded from `ralph-steps.json`
- Logs to `logs/ralph-YYYY-MM-DD-HHMMSS.log`
- `projectDir` resolved via `os.Executable()` (won't work with `go run` — use `go build` first)
