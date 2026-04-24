# workflowmodel

The `internal/workflowmodel` package defines the mutable in-memory representation of a workflow bundle that the TUI editor reads and writes. It has no dependencies on other pr9k internal packages.

- **Last Updated:** 2026-04-24
- **Authors:**
  - River Bailey

## Overview

- Defines `WorkflowDoc`, `Step`, `StepKind`, `EnvEntry`, and `StatusLineBlock` types
- `IsDirty(disk, mem WorkflowDoc) bool` — returns true if the in-memory document differs from the on-disk snapshot (used to skip no-op saves)
- `Empty() WorkflowDoc` — returns a scaffold with a single placeholder shell step (D-40)
- `CopyFromDefault(bundlePath string) (WorkflowDoc, error)` — reads `config.json` from `bundlePath` and returns a flat in-memory copy with all phases merged into a single `Steps` slice
- `DefaultScaffoldModel string` — the default Claude model name for new scaffold steps (D-58)
- `ModelSuggestions []string` — ordered list of model name suggestions for the detail pane dropdown; `ModelSuggestions[0] == DefaultScaffoldModel`

Key files:

- `src/internal/workflowmodel/model.go` — type definitions
- `src/internal/workflowmodel/diff.go` — `IsDirty`
- `src/internal/workflowmodel/scaffold.go` — `Empty`, `CopyFromDefault`, `ParseConfig`
- `src/internal/workflowmodel/modelsuggestions.go` — `DefaultScaffoldModel`, `ModelSuggestions`

## Core Types

```go
// StepKind identifies whether a step runs Claude or a shell command.
type StepKind string

const (
    StepKindClaude StepKind = "claude"
    StepKindShell  StepKind = "shell"
)

// EnvEntry represents one entry from the env or containerEnv section.
type EnvEntry struct {
    Key       string
    Value     string
    IsLiteral bool  // true → containerEnv key=value pair; false → env passthrough name
}

// StatusLineBlock holds the optional statusLine configuration block.
type StatusLineBlock struct {
    Type                   string
    Command                string
    RefreshIntervalSeconds int
}

// Step is one workflow step. IsClaudeSet distinguishes three states:
//   - new/untyped: Kind == "", IsClaudeSet == false
//   - shell step:  Kind == StepKindShell, IsClaudeSet == false
//   - claude step: Kind == StepKindClaude, IsClaudeSet == true
type Step struct {
    Name               string
    Kind               StepKind
    IsClaudeSet        bool
    Model              string
    PromptFile         string
    Command            []string
    Env                []EnvEntry
    CaptureAs          string
    CaptureMode        string
    BreakLoopIfEmpty   bool
    SkipIfCaptureEmpty string
    TimeoutSeconds     int
    OnTimeout          string
    ResumePrevious     bool
}

// WorkflowDoc is the mutable in-memory representation of a config.json bundle.
// UnknownFields captures JSON keys not mapped to typed fields; recorded on load,
// discarded on save.
type WorkflowDoc struct {
    DefaultModel  string
    StatusLine    *StatusLineBlock
    Steps         []Step
    UnknownFields map[string]json.RawMessage
}
```

## Key Functions

```go
// IsDirty returns true if disk and memory differ, ignoring UnknownFields.
func IsDirty(disk, memory WorkflowDoc) bool

// Empty returns a WorkflowDoc with a single placeholder shell step.
func Empty() WorkflowDoc

// CopyFromDefault reads the config.json at bundlePath and returns a flat WorkflowDoc.
// All phase sections (initialize, iteration, finalize) are merged into Steps.
func CopyFromDefault(bundlePath string) (WorkflowDoc, error)

// ParseConfig parses raw config.json bytes into a WorkflowDoc.
func ParseConfig(data []byte) (WorkflowDoc, error)
```

## IsClaudeSet Semantics

The three-state `IsClaudeSet` flag distinguishes a step that has never been typed (`""`) from a step that was explicitly set to shell. The TUI uses this to render new steps differently from shell steps, and to determine which fields to show in the detail pane.

| IsClaudeSet | Kind | Meaning |
|-------------|------|---------|
| false | `""` | New/untyped step — user has not yet chosen a step type |
| false | `"shell"` | Shell step |
| true | `"claude"` | Claude step |

## IsDirty Contract

`IsDirty` performs a structural comparison of two `WorkflowDoc` values, field by field. It ignores `UnknownFields` differences — fields loaded from `config.json` but not mapped to typed fields are not tracked for dirtiness. This prevents spurious dirty flags when loading a config that has extra unrecognized keys.

## Flat Steps Slice

`WorkflowDoc.Steps` uses a flat list; phase information (initialize / iteration / finalize) is not stored in the model. `CopyFromDefault` merges all phases into the flat list. `workflowio.marshalDoc` writes all steps under the "iteration" phase key. Phase-aware save/load is deferred to a future slice.

## Testing

- `src/internal/workflowmodel/diff_test.go` — 5 tests (identical, step-added, step-removed, field-changed table, unknown-fields-ignored)
- `src/internal/workflowmodel/scaffold_test.go` — 3 tests (minimal shape, reads default bundle, input immutability)
- `src/internal/workflowmodel/model_test.go` — 1 test (IsClaudeSet distinguishes new/shell/claude)
- `src/internal/workflowmodel/modelsuggestions_test.go` — 2 tests (DefaultScaffoldModel is first entry, ModelSuggestions non-empty)

## Related Documentation

- [`docs/code-packages/workflowio.md`](workflowio.md) — Load and Save operations that consume WorkflowDoc
- [`docs/code-packages/workflowedit.md`](workflowedit.md) — TUI editor that holds the live WorkflowDoc
- [`docs/code-packages/workflowvalidate.md`](workflowvalidate.md) — Validates a WorkflowDoc before save
