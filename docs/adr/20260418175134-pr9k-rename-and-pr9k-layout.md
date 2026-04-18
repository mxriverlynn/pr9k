# `ralph-tui` → `pr9k` Rename, `.pr9k/` Runtime Layout, and `config.json` Rename

- **Status:** accepted
- **Date Created:** 2026-04-18 17:51
- **Last Updated:** 2026-04-18
- **Authors:**
  - River Bailey (mxriverlynn, river.bailey@testdouble.com)
- **Reviewers:**

## Context

pr9k's binary, source directory, and Go module carried the name `ralph-tui`
as a holdover from when the TUI component was envisioned as one part of a
larger "ralph" toolbox. That context never materialised; the standalone tool
accumulated its own identity, and the name became a source of confusion for
new contributors and users alike.

Alongside the name mismatch, runtime output was scattered across two
uncoordinated locations:

- **Session logs** wrote to `<projectDir>/logs/`, which users had to
  remember to add to `.gitignore` manually.
- **Iteration records** (`iteration.jsonl`) wrote to
  `<projectDir>/.ralph-cache/`, a directory whose name implied
  it held build cache rather than orchestrator state.
- **The installed bundle** (`config.json`, `prompts/`, `scripts/`,
  `ralph-art.txt`) sat directly under `<executableDir>/`, meaning
  `bin/config.json`, `bin/prompts/`, etc. — no namespacing to indicate
  these files belong to pr9k.
- There was no provision for a per-repo workflow override; the resolver
  always fell back to the executable directory, so users who wanted to
  customise a workflow had to place their bundle alongside the binary.

The step-configuration filename `ralph-steps.json` described the *origin*
of the config (a ralph workflow) rather than what pr9k *does* with it (load
a generic step sequence). This made the filename a leaky internal detail
rather than a self-describing API surface.

## Decision

### 1 — Binary, source tree, and module rename

- Binary: `ralph-tui` → `pr9k`
- Source directory: `ralph-tui/` → `src/`
- Go module path: `github.com/mxriverlynn/pr9k/ralph-tui` →
  `github.com/mxriverlynn/pr9k/src`
- Entry-point package: `cmd/ralph-tui` → `cmd/pr9k`
- Cobra `Use` field and `--version` output prefix updated to `pr9k`
- TUI footer version label updated to `pr9k v<version>`
- CI workflow `working-directory` and `cache-dependency-path` updated
  from `ralph-tui/` to `src/`

Internal package paths under `src/internal/` are **not** renamed (see
Non-goals).

### 2 — Consolidate runtime state under `.pr9k/`

All pr9k-owned directories and files at both the install site and the
target-repo site are moved under a single `.pr9k/` prefix:

| Old path | New path |
|----------|----------|
| `bin/config.json` | `bin/.pr9k/workflow/config.json` |
| `bin/prompts/` | `bin/.pr9k/workflow/prompts/` |
| `bin/scripts/` | `bin/.pr9k/workflow/scripts/` |
| `bin/ralph-art.txt` | `bin/.pr9k/workflow/ralph-art.txt` |
| `<projectDir>/logs/` | `<projectDir>/.pr9k/logs/` |
| `<projectDir>/.ralph-cache/iteration.jsonl` | `<projectDir>/.pr9k/iteration.jsonl` |

The `.pr9k/` directory is added to `.gitignore` in the target repo so
session logs and iteration records are excluded from version control
without any manual step.

The `.ralph-cache/` directory is **preserved** as-is because it is used
as a Docker bind-mount cache inside the sandbox container, and renaming
the host-side mount would require coordinated changes to container toolchain
cache paths (see Non-goals).

### 3 — Two-candidate `resolveWorkflowDir` rule

The workflow-directory resolver implements a priority-ordered two-candidate
search rather than falling back unconditionally to `<executableDir>`:

1. `--workflow-dir <path>` explicit flag override (unchanged from prior
   behaviour, but now resolves with `filepath.EvalSymlinks`)
2. `<projectDir>/.pr9k/workflow/` — per-repo workflow override: if this
   directory exists in the target repo, it is used as the workflow bundle.
3. `<executableDir>/.pr9k/workflow/` — installed bundle fallback.

If neither candidate 2 nor candidate 3 exists (and no `--workflow-dir`
override is given), pr9k exits with an error listing both paths that were
checked.

The `--workflow-dir` flag Usage string documents both candidates so users
can discover the per-repo override without reading this ADR.

### 4 — Rename `ralph-steps.json` → `config.json`

The step-configuration file is renamed from `ralph-steps.json` to
`config.json`. The new name describes what pr9k loads (a generic step
configuration) rather than the particular workflow that happens to live
inside it. This aligns with the narrow-reading principle: pr9k is a
generic step runner; workflow *content* is data, not identity.

The `steps.LoadSteps` and `validator.Validate` functions load `config.json`
by name. The `Makefile` copies `src/config.json` → `bin/.pr9k/workflow/`.

## Non-goals

The following items are explicitly out of scope for this set of decisions:

- Renaming the `ralph` GitHub issue label, `ralph-art.txt`, `progress.txt`,
  `deferred.txt`, the `ralph-` session log filename prefix, per-run artifact
  directory names, `ralph-*.cid` tempfile pattern, or the
  `containerEnv` `.ralph-cache/...` paths inside Docker containers.
- Backwards-compatibility shims for the old binary name, old log path, or
  old bundle layout.
- Renaming packages under `src/internal/` — the rename stops at the
  Go module-path boundary and the `cmd/` entry point.
- Global workflow discovery (`$XDG_CONFIG_HOME`, `~/.config/pr9k`,
  parent-directory walk, or any mechanism beyond the two-candidate rule
  above).
- Renaming or removing `.ralph-cache/` from the host filesystem — that
  directory's name is baked into container toolchain cache paths and
  requires a separate, coordinated change.

## Supersedes

### `docs/adr/20260413162428-workflow-project-dir-split.md`

The WorkflowDir-resolution passages in §Decision and §Notes that describe
`resolveWorkflowDir` returning `filepath.Dir(executable)` are superseded by
the two-candidate rule in §3 above. The `--workflow-dir` / `--project-dir`
split itself and the `{{WORKFLOW_DIR}}` / `{{PROJECT_DIR}}` token semantics
remain in effect without change.

### `docs/adr/20260413160000-require-docker-sandbox.md`

Any log-path passages that reference `<projectDir>/logs/` as the session log
destination are superseded by the `.pr9k/logs/` consolidation in §2 above.
The decision to require Docker as an unconditional runtime dependency remains
in effect without change.

### `docs/adr/20260410170952-narrow-reading-principle.md`

The narrow-reading principle remains fully in effect. This ADR clarifies one
nuance: the workflow-config *filename* (`config.json`) is a tool-identity
surface (it is the name pr9k uses to find its configuration and is part of
the public API per the versioning standard), while the *contents* of that
file remain workflow data that pr9k reads without embedding workflow-specific
knowledge in Go code.

## Apply when

Consult this ADR for any change to binary name, source-tree layout, runtime
output paths, workflow discovery, or the workflow-config filename.

## Notes

### Key files affected

| File / path | Change |
|-------------|--------|
| `src/cmd/pr9k/` | Entry point (was `cmd/ralph-tui/`) |
| `src/go.mod` | Module path updated |
| `.github/workflows/ci.yml` | `working-directory` / `cache-dependency-path` updated |
| `Makefile` | `build` target writes to `bin/.pr9k/workflow/` |
| `src/internal/cli/args.go` | `resolveWorkflowDir` two-candidate implementation |
| `src/internal/logger/logger.go` | `logsDir` → `<projectDir>/.pr9k/logs/` |
| `src/cmd/pr9k/main.go` | `artifactDir` → `<projectDir>/.pr9k/logs/<runStamp>/` |
| `src/internal/workflow/run.go` | `artifactPath` → `<projectDir>/.pr9k/logs/<runStamp>/` |
| `src/internal/workflow/iterationlog.go` | Writes to `<projectDir>/.pr9k/iteration.jsonl` |
| `src/internal/preflight/run.go` | Creates both `.ralph-cache/` and `.pr9k/` on startup |
| `src/config.json` | Renamed from `ralph-steps.json` |
| `.gitignore` | `.pr9k/` entry added |

### Related issues

- Issue #135 — binary / source / module rename (`ralph-tui` → `pr9k`)
- Issue #136 — `ralph-steps.json` → `config.json`
- Issue #137 — log output → `<projectDir>/.pr9k/logs/`
- Issue #138 — `iteration.jsonl` → `<projectDir>/.pr9k/`
- Issue #139 — two-candidate `resolveWorkflowDir` + bundle layout
- Issue #140 — `.gitignore` updated for `.pr9k/`
- Issue #141 — docs swept for all of the above

### Related ADRs

- [Workflow/Project-Dir Split ADR](20260413162428-workflow-project-dir-split.md) —
  introduces the `--workflow-dir` / `--project-dir` split; partially
  superseded by §3 above.
- [Require Docker Sandbox ADR](20260413160000-require-docker-sandbox.md) —
  Docker-sandbox requirement; log-path passages partially superseded by §2.
- [Narrow-Reading Principle ADR](20260410170952-narrow-reading-principle.md) —
  pr9k is a generic step runner; still in full effect.
- [Versioning Standard](../coding-standards/versioning.md) — names the binary,
  CLI flags, `{{VAR}}` language, and `--version` output as public API
  surfaces; governs how the renames in §1 and §4 are versioned.
