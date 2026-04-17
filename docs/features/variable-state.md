# Variable State Management

Owns and resolves runtime variable state for a ralph-tui run, providing two scoped tables (persistent and iteration) plus a set of built-in variables seeded from CLI flags and updated by the orchestrator.

- **Last Updated:** 2026-04-17
- **Authors:**
  - River Bailey

## Overview

- `VarTable` holds all runtime variable state for a single run
- Variables belong to one of two scopes: persistent (survives the whole run) or iteration (cleared at the start of each iteration)
- Seven built-in variables are seeded or updated by the orchestrator: `WORKFLOW_DIR`, `PROJECT_DIR`, `MAX_ITER`, `ITER`, `STEP_NUM`, `STEP_COUNT`, `STEP_NAME`
- `captureAs` bindings from step output are routed to the correct scope based on the active workflow phase
- Resolution order during an iteration step: iteration table → persistent table; during initialize or finalize: persistent table only

Key files:
- `ralph-tui/internal/vars/vars.go` — `VarTable`, `Phase`, built-in constants, all public methods
- `ralph-tui/internal/vars/substitute.go` — `Substitute` and `ExtractReferences` functions
- `ralph-tui/internal/vars/vars_test.go` — Unit tests for scoping, phase contract, overwrite semantics, and built-in protection
- `ralph-tui/internal/vars/substitute_test.go` — Unit tests for token substitution and escape sequences

## Architecture

```
┌─────────────────────────────────────────────────────────┐
│                       VarTable                          │
│                                                         │
│  persistent map[string]string                           │
│    WORKFLOW_DIR, PROJECT_DIR, MAX_ITER, ITER,           │
│    STEP_NUM, STEP_COUNT, STEP_NAME (set by orchestrator)│
│    + initialize-phase captureAs bindings                │
│                                                         │
│  iteration  map[string]string                           │
│    iteration-phase captureAs bindings                   │
│    cleared by ResetIteration() each new iteration       │
│                                                         │
│  phase  Phase  (Initialize | Iteration | Finalize)      │
└─────────────────────────────────────────────────────────┘

Resolution (Get):
  Iteration phase  →  iteration map → persistent map
  Initialize/Finalize  →  persistent map only
```

## Core Types

```go
// Phase identifies which workflow phase is active.
type Phase int

const (
    Initialize Phase = iota  // startup; captureAs → persistent
    Iteration                 // repeating loop; captureAs → iteration
    Finalize                  // teardown; captureAs not valid
)

// VarTable holds runtime variable state for a single ralph-tui run.
type VarTable struct {
    persistent map[string]string
    iteration  map[string]string
    phase      Phase
}
```

## Built-In Variables

| Variable | Set By | When |
|----------|--------|------|
| `WORKFLOW_DIR` | `New()` | Once at startup from `--workflow-dir` flag (install dir) |
| `PROJECT_DIR` | `New()` | Once at startup from `--project-dir` flag (target repo) |
| `MAX_ITER` | `New()` | Once at startup from `--iterations` flag (0 = unbounded) |
| `ITER` | `SetIteration(n)` | Start of each iteration |
| `STEP_NUM` | `SetStep(num, count, name)` | Just before each step runs |
| `STEP_COUNT` | `SetStep(num, count, name)` | Just before each step runs |
| `STEP_NAME` | `SetStep(num, count, name)` | Just before each step runs |

Built-in names are reserved: `Bind` panics if a `captureAs` binding attempts to overwrite any of them. The step validator is the primary enforcement point; the panic is a defense-in-depth check.

## captureAs Binding Source

For non-claude steps (`isClaude: false`), `captureAs` binds to the **last non-empty stdout line** from the step (via `runner.LastCapture()`). This is the historical behavior: a shell script prints its result as the last line and ralph-tui captures it.

For claude steps (`isClaude: true`), `captureAs` binds to **`result.result`** — the `result` field of the `ResultEvent` emitted by `claude -p --output-format stream-json --verbose`. This is the authoritative final answer text, parsed from the NDJSON stream by the `claudestream.Aggregator`. The raw JSON on stdout is never meaningful for binding; `result.result` is.

The `captureAs` variable scoping rules (phase routing to persistent vs. iteration table) apply identically to both step types.

See [Capturing Step Output](../how-to/capturing-step-output.md) for usage examples.

## Key Methods

### New

```go
func New(workflowDir, projectDir string, maxIter int) *VarTable
```

Creates a `VarTable` seeded with `WORKFLOW_DIR`, `PROJECT_DIR`, and `MAX_ITER` in the persistent table. The initial phase is `Initialize`.

### SetPhase

```go
func (vt *VarTable) SetPhase(phase Phase)
```

Transitions the active phase. The orchestrator calls this as it moves between workflow phases. The phase controls both `Get` resolution order and which table `Bind` writes to.

### Get / GetInPhase

```go
func (vt *VarTable) Get(name string) (string, bool)
func (vt *VarTable) GetInPhase(phase Phase, name string) (string, bool)
```

`Get` uses the current phase's resolution order. `GetInPhase` lets the caller specify the phase explicitly (useful for testing or lookahead). Both return the value and a boolean indicating whether the variable was found.

### Bind

```go
func (vt *VarTable) Bind(phase Phase, name, value string) 
```

Records a `captureAs` binding for the given phase:
- `Initialize` → writes to persistent table
- `Iteration` → writes to iteration table
- `Finalize` → panics (captureAs is not valid in the finalize phase)

Panics if `name` is a reserved built-in name.

### ResetIteration

```go
func (vt *VarTable) ResetIteration()
```

Clears the iteration-scoped table. Called by the orchestrator at the start of each new iteration before any iteration steps run.

### SetIteration / SetStep

```go
func (vt *VarTable) SetIteration(n int)
func (vt *VarTable) SetStep(num, count int, name string)
```

Orchestrator-facing setters that update the built-in variables in the persistent table.

### AllCaptures

```go
func (vt *VarTable) AllCaptures(phase Phase) map[string]string
```

Returns a defensive copy of all non-built-in (user-defined) variables visible in the given phase. Reserved built-in names (`WORKFLOW_DIR`, `PROJECT_DIR`, `MAX_ITER`, `ITER`, `STEP_NUM`, `STEP_COUNT`, `STEP_NAME`) are excluded from the result.

Resolution mirrors `Get`:
- During `Iteration`: both the persistent and iteration tables contribute; iteration entries shadow persistent ones.
- During `Initialize` or `Finalize`: only the persistent table is included.

Used by `workflow.buildState` to populate the `Captures` field of `statusline.State` before each `PushState` call. Callers own the returned map — mutations do not affect the VarTable.

## Substitution Engine

`vars.Substitute` expands `{{VAR_NAME}}` tokens in a string using the variable values from a `VarTable`:

```go
func Substitute(input string, vt *VarTable, phase Phase) (string, error)
```

- Uses `GetInPhase` for resolution, so the phase's lookup order applies.
- `{{{{` → literal `{{`; `}}}}` → literal `}}` (escape sequences).
- Unresolved variables log a warning and substitute the empty string.
- If `vt` is nil, the input is returned unchanged.

`vars.ExtractReferences` returns all variable names referenced by `{{VAR_NAME}}` tokens in a string (used for validation and tooling). The returned slice may contain duplicates if the same variable appears more than once:

```go
func ExtractReferences(input string) []string
```

## Phase Contract

| Phase | Bind target | Get sees |
|-------|-------------|----------|
| Initialize | persistent | persistent only |
| Iteration | iteration | iteration → persistent |
| Finalize | panics | persistent only |

## Error Handling

`Bind` panics (rather than returning an error) for two precondition violations:

| Violation | Panic Message |
|-----------|---------------|
| Reserved name | `"vars: attempt to bind reserved variable %q via captureAs"` |
| Finalize-phase captureAs | `"vars: captureAs binding %q is not valid in the finalize phase"` |

These are programming errors, not runtime conditions — the step validator (issue #40) should catch them before execution. The panics are a defense-in-depth guard.

## Testing

- `ralph-tui/internal/vars/vars_test.go` — Unit tests for `VarTable` with race detector
- `ralph-tui/internal/vars/substitute_test.go` — Unit tests for `Substitute` and `ExtractReferences`

`VarTable` covered behaviors:
- Built-in seeding via `New` (both `WORKFLOW_DIR` and `PROJECT_DIR`)
- Phase transitions and resolution-order contract
- `captureAs` routing to the correct scope
- Overwrite semantics (iteration shadows persistent; `Bind` overwrites previous value in the same scope)
- `ResetIteration` clears iteration scope without touching persistent scope
- `SetIteration` and `SetStep` update the correct built-ins
- Empty-string returns for unknown variables
- Reserved-name protection (`Bind` panics)
- Finalize-phase `Bind` panic
- `AllCaptures`: reserved names excluded, non-nil map on fresh table, defensive copy (mutation does not affect VarTable), iteration-scoped entries shadow persistent ones, iteration entries excluded after `SetPhase(Finalize)`, reserved names excluded even after `SetIteration`/`SetStep`

`Substitute` / `ExtractReferences` covered behaviors:
- Token replacement using iteration and persistent scopes
- Escape sequences (`{{{{` → `{{`, `}}}}` → `}}`)
- Unresolved variable warning and empty-string substitution
- Nil `VarTable` pass-through
- `ExtractReferences` duplicate preservation (same variable appearing twice produces two entries) and escape handling

## Additional Information

- [Architecture Overview](../architecture.md) — System-level view of ralph-tui with block diagrams
- [Workflow Orchestration](workflow-orchestration.md) — How the orchestrator calls `SetPhase`, `SetIteration`, `SetStep`, and `ResetIteration`
- [Step Definitions & Prompt Building](step-definitions.md) — `CaptureAs` field on `Step` that feeds `Bind`
- [Variable Output & Injection](../how-to/variable-output-and-injection.md) — End-to-end guide to how variables flow through the workflow
- [API Design](../coding-standards/api-design.md) — Precondition validation patterns (panic vs. error)
- [Testing](../coding-standards/testing.md) — Race detector requirement and testing conventions
