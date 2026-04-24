# workflowvalidate

The `internal/workflowvalidate` package is a thin bridge between the TUI editor packages and `internal/validator`. It converts a `workflowmodel.WorkflowDoc` to the shape `validator.ValidateDoc` expects and delegates — keeping `workflowedit` from importing `internal/validator` directly (D-4).

- **Last Updated:** 2026-04-24
- **Authors:**
  - River Bailey

## Overview

- Exports a single function: `Validate(doc, workflowDir, companions) []validator.Error`
- Prevents `workflowedit` from taking a direct dependency on `internal/validator`, keeping the package dependency graph acyclic
- `companions` is a map of in-memory file bytes keyed by path relative to `workflowDir` (e.g., `"prompts/step-1.md"`); when a key is present, its bytes are used for Rule B validation instead of reading from disk

Key file: `src/internal/workflowvalidate/validate.go`

## Core API

```go
// Validate runs all D13 validation categories against doc and returns any
// errors found. workflowDir is the workflow bundle directory. companions is
// an optional map of in-memory file bytes keyed by path relative to workflowDir
// (e.g., "prompts/step-1.md"); when a key is present its bytes are used
// instead of reading from disk.
func Validate(doc workflowmodel.WorkflowDoc, workflowDir string, companions map[string][]byte) []validator.Error
```

## Dependency Isolation

The isolation boundary is critical:

```
workflowedit  →  workflowvalidate  →  validator
                  (bridge only)
```

Without `workflowvalidate`, `workflowedit` would import `internal/validator` directly, creating a dependency that crosses the TUI/validator boundary. The bridge keeps the import graph clean and the packages independently testable.

## Companion Map Semantics

- Keys must be paths **relative to workflowDir** (e.g., `"prompts/step-1.md"`, not `"step-1.md"`)
- A bare filename key (`"step-1.md"`) is a cache miss — the validator reads from disk instead
- Values are the current in-memory bytes of the companion file (what the user has typed but not yet saved)
- `nil` companions map is equivalent to an empty map

## Validation Categories

The function delegates to `validator.ValidateDoc`, which runs all D13 categories:

1. JSON schema shape (required fields, unknown keys)
2. Step name non-empty
3. Claude step model non-empty
4. Prompt file exists (from companion map or disk)
5. Command script exists and is within `workflowDir` (OI-1)
6. Environment variable names valid
7. `captureMode` valid values
8. `containerEnv` key format
9. `skipIfCaptureEmpty` refers to a defined capture variable
10. `env` passthrough names non-empty
11. `statusLine.command` resolvable
12. Sandbox isolation Rule B (no `{{WORKFLOW_DIR}}` in prompt files outside `workflowDir`)
13. Sandbox isolation Rule C (no direct network access in containerEnv)

## Testing

- `src/internal/workflowvalidate/validate_test.go`
- Tests: round-trip (valid doc produces no errors), error ordering (fatals before warnings)
- Additional companion-bytes test: `TestValidateDoc_CompanionBytesScannedForRuleB`

## Related Documentation

- [`docs/code-packages/validator.md`](validator.md) — `internal/validator` API reference
- [`docs/code-packages/workflowedit.md`](workflowedit.md) — TUI editor that calls `Validate` before save
- [`docs/code-packages/workflowmodel.md`](workflowmodel.md) — `WorkflowDoc` passed to `Validate`
