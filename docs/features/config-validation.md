# Config Validation

The `internal/validator` package validates `ralph-steps.json` against all ten D13 validation categories before any workflow step runs. It collects every error in a single pass and returns them as a slice, so the operator sees all problems at once rather than stopping at the first failure.

**Package:** `internal/validator/`

## What it validates

### Category 1 — File presence and parseability

- `ralph-steps.json` must be readable from the workflow directory.
- The JSON must parse without error.
- Unknown fields (e.g., stale `prependVars`) are rejected via `json.Decoder.DisallowUnknownFields`.
- All three top-level array keys — `initialize`, `iteration`, `finalize` — must be present.

### Category 2 — Schema shape per step

Each step is checked for:

- `name` must be non-empty.
- `isClaude` is required (`*bool` pointer type, so a missing key is distinguished from `false`).
- Claude steps (`isClaude: true`) must have a non-empty `promptFile` and `model`, and must not have a `command`.
- Non-Claude steps (`isClaude: false`) must have a non-empty `command` array, and must not have a `promptFile`.
- `captureAs`, when set, must be non-empty and must not shadow any built-in variable name (`WORKFLOW_DIR`, `PROJECT_DIR`, `MAX_ITER`, `ITER`, `STEP_NUM`, `STEP_COUNT`, `STEP_NAME`).
- `breakLoopIfEmpty` requires `captureAs` to be set and is only valid in the iteration phase.
- No duplicate step names within a phase (rule 6.1).
- No duplicate `captureAs` values within a phase (rule 6.2).

### Category 3 — Phase size

The `iteration` array must contain at least one step.

### Category 4 — Referenced file existence

- For Claude steps: `prompts/<promptFile>` must exist on disk. Additionally, `promptFile` values containing path traversal segments (e.g., `../`) that would resolve outside the `prompts/` directory are rejected with a "prompt path escapes prompts directory" error.
- For non-Claude steps: `command[0]` must be resolvable — either as a relative path under `workflowDir`, an absolute path, or a bare name found via `PATH` lookup.

Command path resolution uses `"/"` as a path separator and assumes Unix; revise if Windows support is added.

### Category 5 — Variable scope resolution

The validator walks steps in declaration order and builds a live scope table for each phase:

- **Initialize scope** seeds `WORKFLOW_DIR`, `PROJECT_DIR`, `MAX_ITER`, `STEP_NUM`, `STEP_COUNT`, `STEP_NAME`. `ITER` is intentionally excluded — it is an iteration-only built-in.
- Each initialize step's `captureAs` name is added to the scope after that step, making it visible to later initialize steps and all finalize steps (the persistent scope).
- **Iteration scope** = persistent scope + `ITER`.
- **Finalize scope** = persistent scope only (no iteration-phase captures, no `ITER`).

Any `{{VAR}}` reference in a prompt file or command argument that is not in scope at that point produces an "unresolved variable reference" error.

### Category 10 — env passthrough names

The optional top-level `env` array lists host environment variable names that ralph-tui passes through into the sandbox. Each name is validated:

- Must not be empty.
- Must match `^[A-Za-z_][A-Za-z0-9_]*$` (standard POSIX identifier format — no spaces, dots, or hyphens).
- Must not be in the sandbox-reserved set (`CLAUDE_CONFIG_DIR`, `HOME`).
- Must not be in the isolation denylist (`PATH`, `USER`, `LOGNAME`, `SSH_AUTH_SOCK`, `LD_PRELOAD`, `LD_LIBRARY_PATH`, `DYLD_INSERT_LIBRARIES`, `DYLD_LIBRARY_PATH`).

Duplicates within the `env` array and overlap with built-in variable names are harmless and produce no errors. Env validation runs before the scope walk; errors here do not block phase validation.

### statusLine block (Category "statusline")

The optional top-level `statusLine` object configures a status-line command displayed by the TUI. Validation runs before the phase walk; errors use `Category="statusline"`, `Phase="config"`, no `StepName`.

- `type`, when present, must be `"command"`. Omitting it is valid.
- `command` is required and must resolve — either as a path relative to `workflowDir`, an absolute path, or a bare name found via `PATH` lookup (same resolution as non-claude step commands).
- `refreshIntervalSeconds`, when present, must be `>= 0` (`0` disables the timer). The unit is seconds.

Unknown subfields are rejected (strict decode). Absent `statusLine` is valid and produces no errors.

See [Status Line](statusline.md) for the full runtime contract, including the `Runner` lifecycle, stdin payload schema, and ANSI sanitization.

### Sandbox Rules B, C

Two additional rules protect sandbox isolation. Both fire regardless of which phase the step is in.

*(Rule A — captureAs on a claude step rejected — was removed in issue #91. Under stream-json output, captureAs on a claude step binds to `result.result` from the Aggregator, not docker stdout. See `docs/features/stream-json-pipeline.md` (Aggregator section) for the full capture contract, and D6 in `docs/plans/streaming-json-output/design.md` for the original rationale.)*

**Rule B — prompt files must not reference `{{WORKFLOW_DIR}}` or `{{PROJECT_DIR}}`.**
These tokens expand to host filesystem paths. Inside the sandbox, those paths do not exist. A prompt that embeds them would pass a broken path to claude, causing silent substitution failures or confusing instructions. The check uses token-aware parsing (via `vars.ExtractReferences`) so escaped sequences like `{{{{WORKFLOW_DIR}}}}` (which render as the literal text `{{WORKFLOW_DIR}}`) are not flagged. The error message names only the token(s) actually found.

**Rule C — a command step that both references `{{WORKFLOW_DIR}}`/`{{PROJECT_DIR}}` in argv AND sets `captureAs` is rejected.**
Even though dir tokens are valid in command argv (Rule B does not apply to shell commands), capturing a host path into a variable and then consuming that variable inside a later claude prompt forwards the stale host path into the sandbox — the same isolation break that Rule B prevents directly.

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
errs := validator.Validate(workflowDir)
```

Returns an empty slice when the config is valid; one `Error` per problem otherwise. Validation always collects all errors before returning — it does not short-circuit.

Scope walk is skipped if any of the three required top-level arrays are missing, since phase ordering cannot be established.

## Wiring

Wired into `main.go` immediately after `steps.LoadSteps`; validation failures exit 1 with structured errors on stderr before the TUI starts.
