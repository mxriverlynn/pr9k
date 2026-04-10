# Narrow-Reading Architectural Principle: Ralph-tui as a Generic Step Runner

- **Status:** accepted
- **Date Created:** 2026-04-10 17:09
- **Last Updated:** 2026-04-10
- **Authors:**
  - River Bailey (mxriverlynn, river.bailey@testdouble.com)
- **Reviewers:**

## Context

Ralph-tui originally had Ralph workflow knowledge hardcoded in Go: banner prints, `get_gh_user` invocation, `get_next_issue` + empty-issue check, `git rev-parse HEAD`, `ISSUENUMBER` / `STARTINGSHA` prepending, an 8-step cap, and an assumption that the Ralph workflow was THE workflow.

The UX corrections plan (`docs/plans/ux-corrections/design.md`) surfaced this during an audit and asked a structural question: where should workflow content live — in Go code or in `ralph-steps.json`? This ADR records the answer.

## Decision Drivers

- **Separation of concerns** — workflow content (what steps run, with what commands and prompts) should be separate from runtime mechanics (how steps run, how output is captured, how loops work)
- **Configurability** — adding new Ralph workflow steps should not require changing Go code
- **Generality** — a future consumer should be able to use ralph-tui for a non-Ralph workflow by writing their own `ralph-steps.json`
- **Auditability** — every exception to the principle should be documented so it can be evaluated and challenged

## Decision

Ralph-tui **facilitates** the workflow; it does not **define** it. The specific split:

### Ralph-tui owns (hardcoded in Go)

- The three phase names `initialize` / `iteration` / `finalize`.
- The runtime semantics per phase name (runs once / runs N times / runs once).
- The `-n` / `--iterations` CLI flag and loop bound.
- The generic `breakLoopIfEmpty` rule (the only workflow-termination rule ralph-tui understands).
- Generic `{{VAR}}` template substitution inside command argv and prompt file contents.
- Glyph app lifecycle.
- Status header chrome (iteration counter, step checkboxes, shortcut bar).
- Validation of `ralph-steps.json` against the schema.

### Config owns (lives in `ralph-steps.json`)

- Every step that runs at any phase.
- Every variable captured from a step's stdout and referenced by later steps.
- The specific commands, scripts, and prompts that make up the Ralph workflow.

### Close-to-the-line exceptions (kept hardcoded for simplicity)

- The iteration header line format `Iteration N/M — Issue #<id>` — cosmetic chrome; one concession to the Ralph workflow (the fact that `ISSUE_ID` is the variable name displayed). Documented in D8 of the UX corrections plan.
- The completion summary `Ralph completed after N iteration(s) and M finalizing tasks.` — also cosmetic chrome. Documented in D15 of the UX corrections plan.

## Consequences

**Positive:**

- Ralph-tui is a generic config-driven step runner that understands phases, loops, captured-variable substitution, and one loop-exit rule. The Ralph workflow is entirely expressible in `ralph-steps.json`.
- Adding new Ralph workflow steps does not require changing Go code — only `ralph-steps.json` and (optionally) new prompt files.
- A future consumer could use ralph-tui for a non-Ralph workflow by writing their own `ralph-steps.json`, subject to the two cosmetic chrome concessions listed above.
- Any PR that adds Ralph-specific knowledge to Go code (new hardcoded commands, new hardcoded variable names, new workflow-specific rules) should be rejected unless the PR also updates this ADR to document why the exception is warranted.

**Negative:**

- The two cosmetic chrome exceptions (`Iteration N/M — Issue #<id>` and the completion summary) are permanent coupling to the Ralph workflow until explicitly reversed by a future ADR.
- A consumer using ralph-tui for a non-Ralph workflow must accept or work around those two cosmetic lines.

## Alternatives Considered

1. **Broad reading** — keep workflow definitions in Go, treat `ralph-steps.json` as configuration-for-a-Ralph-tool. Rejected because it directly contradicts the separation-of-concerns principle and because the audit showed the hardcoded approach had accumulated significant tech debt.

2. **Middle ground** — move most workflow to config but keep prologue steps (`get_gh_user`, `get_next_issue`, `git rev-parse HEAD`) as hardcoded prelude. Rejected because the principle gets murky fast: every "small exception" re-opens the same argument with no stable stopping point.

## Notes

### Key Files

| File | Purpose |
|------|---------|
| `ralph-tui/ralph-steps.json` | The step definitions that own the Ralph workflow |
| `ralph-tui/internal/workflow/run.go` | The Run loop — phase sequencing, iteration bounds, finalization |
| `ralph-tui/internal/workflow/orchestrate.go` | The Orchestrate step sequencer — drives steps, captures output |
| `ralph-tui/internal/config/loader.go` | Loads and validates `ralph-steps.json` |

### Related Docs

- `docs/plans/ux-corrections/design.md` — D3c is the locked-in decision this ADR captures
- `docs/plans/ux-corrections/design.md` D3b — schema shape; D7 — prompt-file variable injection; D9 — prologue moves to iteration array; D11 — splash becomes initialize step
- [Cobra CLI Framework ADR](./20260409135303-cobra-cli-framework.md) — the other ADR in this project
