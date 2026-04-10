# Config Validation

The `internal/validator` package validates `ralph-steps.json` against all eight D13 validation categories before any workflow step runs. It collects every error in a single pass and returns them as a slice, so the operator sees all problems at once rather than stopping at the first failure.

**Package:** `internal/validator/`

## What it validates

### Category 1 — File presence and parseability

- `ralph-steps.json` must be readable from the project directory.
- The JSON must parse without error.
- Unknown fields (e.g., stale `prependVars`) are rejected via `json.Decoder.DisallowUnknownFields`.
- All three top-level array keys — `initialize`, `iteration`, `finalize` — must be present.

### Category 2 — Schema shape per step

Each step is checked for:

- `name` must be non-empty.
- `isClaude` is required (`*bool` pointer type, so a missing key is distinguished from `false`).
- Claude steps (`isClaude: true`) must have a non-empty `promptFile` and `model`, and must not have a `command`.
- Non-Claude steps (`isClaude: false`) must have a non-empty `command` array, and must not have a `promptFile`.
- `captureAs`, when set, must be non-empty and must not shadow any built-in variable name (`PROJECT_DIR`, `MAX_ITER`, `ITER`, `STEP_NUM`, `STEP_COUNT`, `STEP_NAME`).
- `breakLoopIfEmpty` requires `captureAs` to be set and is only valid in the iteration phase.
- No duplicate step names within a phase (rule 6.1).
- No duplicate `captureAs` values within a phase (rule 6.2).

### Category 3 — Phase size

The `iteration` array must contain at least one step.

### Category 4 — Referenced file existence

- For Claude steps: `prompts/<promptFile>` must exist on disk.
- For non-Claude steps: `command[0]` must be resolvable — either as a relative path under `projectDir`, an absolute path, or a bare name found via `PATH` lookup.

Command path resolution uses `"/"` as a path separator and assumes Unix; revise if Windows support is added.

### Category 5 — Variable scope resolution

The validator walks steps in declaration order and builds a live scope table for each phase:

- **Initialize scope** seeds `PROJECT_DIR`, `MAX_ITER`, `STEP_NUM`, `STEP_COUNT`, `STEP_NAME`. `ITER` is intentionally excluded — it is an iteration-only built-in.
- Each initialize step's `captureAs` name is added to the scope after that step, making it visible to later initialize steps and all finalize steps (the persistent scope).
- **Iteration scope** = persistent scope + `ITER`.
- **Finalize scope** = persistent scope only (no iteration-phase captures, no `ITER`).

Any `{{VAR}}` reference in a prompt file or command argument that is not in scope at that point produces an "unresolved variable reference" error.

## Error type

`Error` is a structured type that implements the `error` interface:

```go
type Error struct {
    Category string  // e.g. "schema", "file", "variable", "phase-size", "parse"
    Phase    string  // e.g. "initialize", "iteration", "finalize", "config"
    StepName string  // empty for file-level errors
    Problem  string  // human-readable description
}
```

Formatted output:

```
config error: schema: iteration step "get-issue": isClaude is required
config error: file: config: missing required top-level array "finalize"
config error: variable: finalize step "push": unresolved variable reference {{ITER}}
```

## Entry point

```go
errs := validator.Validate(projectDir)
```

Returns an empty slice when the config is valid; one `Error` per problem otherwise. Validation always collects all errors before returning — it does not short-circuit.

Scope walk is skipped if any of the three required top-level arrays are missing, since phase ordering cannot be established.

## Wiring

The package is not yet wired into `main.go` — that is tracked in issue #41.
