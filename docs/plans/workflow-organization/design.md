# Workflow Organization — Design

Status: **Implemented** — issues #135 (rename), #136 (config.json), #137 (logs → .pr9k/logs/), #138 (iteration.jsonl → .pr9k/), #139 (bundle layout + resolver), #140 (.gitignore), #141 (doc sweep) all closed.
Target pr9k version: **0.7.0** (breaking — `y` bump per `docs/coding-standards/versioning.md`; the binary is renamed from `ralph-tui` to `pr9k`, the source directory is renamed from `ralph-tui/` to `src/`, the workflow-config filename is renamed from `config.json` to `config.json`, the `--workflow-dir` default changes, `config.json`/`scripts/`/`prompts/` move, and `logs/` relocates).

## 1. Overview

Two coupled changes ship together in this release.

**(a) Name reconciliation.** The project is named "pr9k" (Power-Ralph.9000), but the Go module, source directory, binary, and the workflow-config filename are still named `ralph-tui` / `config.json` — a holdover from when the TUI was a subcomponent of a larger would-be toolbox. Every user-visible surface that says "ralph-tui" or that names the workflow config after the tool gets renamed:

- The binary (`bin/ralph-tui` → `bin/pr9k`).
- The source directory (`ralph-tui/` → `src/`).
- The Go module path (`github.com/mxriverlynn/pr9k/ralph-tui` → `github.com/mxriverlynn/pr9k/src`).
- The cobra command `Use` string and `--version` output (`ralph-tui [flags]` / `ralph-tui version <x>` → `pr9k [flags]` / `pr9k version <x>`).
- The TUI footer version label (`ralph-tui v0.6.1` → `pr9k v0.7.0`).
- The workflow-config filename (`config.json` → `config.json`). This file is the entry point that pr9k loads — its name should describe what it is to pr9k, not which workflow happens to be inside it. The contents are unchanged; the filename, every Go reference, and every doc reference become `config.json`. See §4.6 for the full surface.
- Every doc, comment, test fixture, and error-message prefix that names the tool.

**(b) Runtime state consolidation under `.pr9k/`.** Today, pr9k reads its workflow definition (`config.json` (renamed to `config.json` per (a)), `scripts/`, `prompts/`, `ralph-art.txt`) from the install directory — the folder that holds the binary — and writes logs directly into the target repo at `<projectDir>/logs/`. Both conventions leak pr9k's presence into the target repo and the user's working environment:

- Running pr9k in any repo produces a top-level `logs/` directory that the user has to gitignore.
- The target repo has no way to ship a custom workflow without replacing the install directory bundle or passing `--workflow-dir` every invocation.
- Cache scaffolding (`.ralph-cache/iteration.jsonl`, `.ralph-cache/go`, etc.) lives under a second, differently-named dotfile folder, so pr9k's footprint is split across two places.

This plan consolidates all pr9k runtime state under a single `.pr9k/` directory, both inside the install bundle and inside the target repo:

- **Install bundle** — `bin/.pr9k/workflow/` holds the shipped workflow (`config.json`, `scripts/`, `prompts/`, `ralph-art.txt`). The `pr9k` executable sits next to `.pr9k/`.
- **Target repo (optional override)** — if `<projectDir>/.pr9k/workflow/` exists, it overrides the bundled workflow. This makes per-repo workflows first-class.
- **Target repo (runtime output)** — logs move from `<projectDir>/logs/` to `<projectDir>/.pr9k/logs/`, joining `.pr9k/iteration.jsonl` and the existing `.ralph-cache/` contents, which are also pulled under `.pr9k/` (see §4.5).

The `{{WORKFLOW_DIR}}` template variable continues to resolve to whichever workflow dir is active, so workflow JSON does not need to care whether it is running from the bundle or an in-repo override.

The rename and the `.pr9k/` consolidation land in the same release because both rewrite paths, imports, and docs. Splitting them doubles the churn against the same files.

## 2. Goals & Non-Goals

### Goals
- **Rename `ralph-tui` to `pr9k` everywhere** — binary, source directory, Go module path, cobra Use string, `--version` output, status-line footer label, and all documentation.
- **Rename `config.json` to `config.json`** — the file pr9k loads at startup is the *pr9k config*, not a "ralph" artifact. The two filename constants in `steps.LoadSteps` and `validator.Validate` change together; the file's JSON shape does not.
- **Centralize pr9k state** under `.pr9k/` in both the install bundle and the target repo.
- **Eliminate the top-level `logs/` footprint** in target repos — nothing pr9k writes should sit above `.pr9k/`.
- **Make per-repo workflow overrides first-class** — a repo can ship its own `.pr9k/workflow/` without flags or environment manipulation.
- **Preserve the narrow-reading principle** (ADR `20260410170952`) — the resolution rule lives in Go, the workflow content stays in JSON.

### Non-goals
- **Keeping a `ralph-tui` binary symlink or wrapper script for backwards compatibility.** This is a breaking release; the binary name changes cleanly, and users update their shell aliases / CI scripts once.
- **Keeping the `ralph-tui` directory name with a stub `go.mod` redirect.** Go modules do not have redirects; the rename is atomic.
- **Renaming internal packages (`internal/workflow`, `internal/ui`, etc.).** The rename stops at the module-path boundary. Everything under `src/internal/` keeps its current package names — touching them is unrelated churn.
- **Renaming `.pen` files, the MCP server namespace, or the GitHub repo.** Out of scope for code; those are user/infra decisions.
- **Renaming the `ralph` issue label, `progress.txt`, `deferred.txt`, or `ralph-art.txt`.** These are workflow-content identifiers, not tool-identity identifiers. "Ralph" is still the name of the workflow even after the tool is renamed. (`config.json` was previously listed here but is now in scope as `config.json` — see §1 and §4.6 — because it names what pr9k *loads*, not what's inside it.)
- **Renaming the `ralph-` log-filename prefix, the per-run artifact-directory prefix, and the `ralph-*.cid` tempfile pattern.** All three are produced by pr9k but carry the workflow identity in their basename:
  - `<projectDir>/.pr9k/logs/ralph-YYYY-MM-DD-HHMMSS.mmm.log` (formatted by `logger.go:33`).
  - `<projectDir>/.pr9k/logs/ralph-YYYY-MM-DD-HHMMSS.mmm/` (the per-run artifact directory, derived from `RunStamp()` at `logger.go:34`).
  - `$TMPDIR/ralph-*.cid` (the docker cidfile pattern at `sandbox/cidfile.go:20`, asserted by `sandbox/cidfile_test.go:35` and `workflow/run_test.go:1110`).
  These basenames stay "ralph-" for the same reason `ralph-art.txt` does: they identify the workflow this tool ran, not the tool itself. Renaming to `pr9k-*` would require a second pass through the logger package, cidfile package, four test files, and a handful of doc assertions; bundling that into 0.7.0 multiplies test surface without corresponding user value. Post-0.7.0, if the user-facing filename prefix becomes confusing, revisit as a follow-up.
- **Renaming `.ralph-cache` inside running Docker containers in this release.** `containerEnv` paths in `config.json` (`/home/agent/workspace/.ralph-cache/...`) stay put for Phase A; the host-side reconciliation is covered in §4.5.
- **Discovery of workflow dirs anywhere other than `<projectDir>/.pr9k/workflow/` or `<executableDir>/.pr9k/workflow/`.** No `$XDG_CONFIG_HOME`, no `~/.config/pr9k`, no parent-walk.
- **Backwards-compat shims for the old `logs/` path or the old bundle layout.**

## 3. The `.pr9k/` layout

### 3.1 Install bundle (output of `make build`)

```
bin/
├── pr9k                      # the executable (renamed from ralph-tui)
└── .pr9k/
    └── workflow/
        ├── config.json
        ├── ralph-art.txt
        ├── prompts/
        │   ├── feature-work.md
        │   ├── test-planning.md
        │   └── …
        └── scripts/
            ├── get_next_issue
            ├── post_issue_summary
            └── …
```

Rationale for pinning to `.pr9k/workflow/` rather than `.pr9k/` directly: leaves room for sibling trees under `.pr9k/` in future releases (e.g., `.pr9k/templates/` for `sandbox create`, `.pr9k/schemas/` for JSON Schema distribution) without another migration.

### 3.2 Target repo (runtime)

```
<projectDir>/
└── .pr9k/
    ├── workflow/             # OPTIONAL — overrides the bundled workflow when present
    │   ├── config.json
    │   ├── prompts/
    │   └── scripts/
    ├── logs/
    │   ├── ralph-2026-04-18-161500.123.log
    │   └── ralph-2026-04-18-161500.123/    # per-run artifact dir (claude .jsonl files)
    └── iteration.jsonl       # formerly .ralph-cache/iteration.jsonl (see §4.5)
```

The `<projectDir>/.pr9k/workflow/` tree is entirely optional. Most consumers will use the bundled workflow and only create `.pr9k/logs/` at runtime.

### 3.3 Source tree (post-rename)

```
<repo>/
├── src/                      # formerly ralph-tui/
│   ├── cmd/
│   │   └── pr9k/             # formerly cmd/ralph-tui/
│   │       └── main.go
│   ├── internal/             # unchanged
│   ├── go.mod                # module github.com/mxriverlynn/pr9k/src
│   ├── go.sum
│   ├── config.json           # formerly config.json
│   └── tools.go
├── prompts/
├── scripts/
├── ralph-art.txt
├── Makefile
└── …
```

Note that `cmd/ralph-tui/` becomes `cmd/pr9k/` so the default `go build` output filename matches the new binary name without needing `-o`.

## 4. Required changes

### 4.1 Rename — source tree and Go module

1. `git mv ralph-tui src`.
2. `git mv src/cmd/ralph-tui src/cmd/pr9k`.
3. In `src/go.mod`, change the module declaration from `module github.com/mxriverlynn/pr9k/ralph-tui` to `module github.com/mxriverlynn/pr9k/src`.
4. Across all `*.go` files, rewrite every import path `github.com/mxriverlynn/pr9k/ralph-tui/…` to `github.com/mxriverlynn/pr9k/src/…`. A grep count at planning time (commit `fc8b054`, `grep -rn github.com/mxriverlynn/pr9k/ralph-tui --include="*.go"`) reports 87 occurrences across 38 `*.go` files, all in the `ralph-tui/` (soon `src/`) subtree. A scripted `sed -i` on the matched files is appropriate.
5. Grep for commented `ralph-tui/…` paths (e.g. `internal/scripts/post_issue_summary_test.go` has a comment referencing the source location). Update those too.
6. Run `go mod tidy` and commit `go.mod` + `go.sum`.

No package-level identifiers (`package workflow`, `package cli`, etc.) change — only the module path.

### 4.2 Rename — binary, cobra metadata, and user-facing strings

cobra and TUI surfaces:

- `src/cmd/pr9k/main.go` — change `versionLabel := "ralph-tui v" + version.Version` (line 158) to `"pr9k v" + version.Version`. This is the string rendered in the TUI footer.
- `src/cmd/pr9k/main.go` — change the help-pointer error string `Run 'ralph-tui --help' for usage.` (line 107) to `Run 'pr9k --help' for usage.`.
- `src/internal/cli/args.go` — change `Use: "ralph-tui [flags]"` (line 61) to `Use: "pr9k [flags]"`. cobra derives `--version` output from this plus the `Version` field, so it will print `pr9k version 0.7.0` after the change.
- `src/internal/cli/args.go` — change the cobra `Long:` description (line 63), which begins `ralph-tui drives the claude CLI through multi-step coding loops...`, to begin `pr9k drives the claude CLI...`. This is rendered by `--help`.

Test fixtures:

- `src/internal/ui/model_test.go` and `src/internal/ui/version_footer_test.go` — the four hardcoded `"ralph-tui v..."` test fixtures (`model_test.go:19`, `model_test.go:449`, `model_test.go:460`, `version_footer_test.go:18`) become `"pr9k v..."`.
- `src/cmd/pr9k/doc_integrity_test.go` — the two `"ralph-tui v" + version.Version` expectations (lines 256, 299) become `"pr9k v" + version.Version`.
- `src/cmd/pr9k/sandbox_login_test.go` — the `"... run 'ralph-tui sandbox create' next time ..."` substring assertion (line 173) becomes `"... run 'pr9k sandbox create' next time ..."`.

User-facing error messages and instructional strings (rename audit, not just prefix audit). Today's error-message prefixes are package-name based (`cli: …`, `steps: …`, `workflow: …`) — `grep ralph-tui:` returns zero hits, confirming there is no `ralph-tui:` *prefix* to rename. But the codebase does contain user-facing strings that name the tool inside the message body, all of which must be rewritten:

- `src/cmd/pr9k/sandbox.go` line 79 — comment `// 'ralph-tui sandbox' prints help.` (cosmetic, but caught by the rename audit).
- `src/cmd/pr9k/sandbox_login.go` line 78 — `Sandbox image not found; pulling it first — run 'ralph-tui sandbox create' next time to separate this step.`
- `src/internal/preflight/docker.go` line 71 — `preflight: claude sandbox image is missing. Run: ralph-tui sandbox create`.
- `src/internal/preflight/profile.go` line 63 — `Warning: %s does not exist. … Run 'ralph-tui sandbox login' to authenticate, …`.
- `src/internal/sandbox/command.go` line 96 — comment `// `claude` REPL session used by `ralph-tui sandbox login`. The user runs …`.
- `src/internal/sandbox/image.go` line 9 — comment `// BuiltinEnvAllowlist is the sandbox-plumbing env var set ralph-tui …`.
- `src/internal/version/version.go` — package and constant doc comments (`// Package version exposes the ralph-tui application version …`; `// Version is the current ralph-tui release version.`).
- `src/internal/scripts/post_issue_summary_test.go` line 16 — comment `// (ralph-tui/internal/scripts/ → three levels up).` becomes `(src/internal/scripts/ → three levels up)`.
- `src/internal/validator/prompts_structure_test.go` line 28 — comment `// test file: ralph-tui/internal/validator/prompts_structure_test.go` becomes `src/internal/validator/prompts_structure_test.go`.
- `scripts/statusline` (the shipped demo script) lines 2–3 — `# ralph-tui status line — demo script…` and `# Reads ralph-tui's JSON payload from stdin…` become `pr9k status line` and `Reads pr9k's JSON payload from stdin`. The script ships inside the bundle under `.pr9k/workflow/scripts/`; its identity surface is part of the 0.7.0 rename even though it is not Go code.

Audit method: after the scripted import-path rewrite (§4.1), run `grep -rn ralph-tui src/ scripts/` and walk every remaining hit. The list above is what that grep returns at planning time on `main` (commit `fc8b054`); a fresh grep before merge catches any new occurrence introduced after this plan was written. The `scripts/` sweep is required because `scripts/` is part of the shipped workflow bundle, not test-only fixtures.

### 4.3 Build — `Makefile`

Rewrite the build target and every path that points into `ralph-tui/`:

```make
build:
	rm -rf bin
	mkdir -p bin/.pr9k/workflow
	cd src && go build -o ../bin/pr9k ./cmd/pr9k
	cp -r prompts bin/.pr9k/workflow/prompts
	cp -r scripts bin/.pr9k/workflow/scripts
	cp src/config.json bin/.pr9k/workflow/
	cp ralph-art.txt bin/.pr9k/workflow/
```

The source-of-truth filename in `src/` is `config.json` after the rename in §4.6 (the `git mv` happens as part of that section, not here).

And update every other target that has `cd ralph-tui &&` to `cd src &&`:

- `test`, `lint`, `format`, `format-check`, `vet`, `vulncheck`, `mod-tidy`, `ci`.
- The `GOFMT_PATHS` list stays `cmd internal tools.go` because those subpaths are unchanged under `src/`.

`CLAUDE.md`'s "Build and run" snippet becomes:

```bash
make build
./bin/pr9k [-n <iterations>] [--workflow-dir <path>] [--project-dir <path>]

# Or build directly:
cd src && go build -o ../pr9k ./cmd/pr9k
./pr9k [-n <iterations>] [--workflow-dir <path>] [--project-dir <path>]
```

The repo's source-of-truth locations at the repo root (`prompts/`, `scripts/`, `ralph-art.txt`) are unchanged — only the bin layout and the Go source dir are reorganized.

Deferred: whether to relocate the source-of-truth copies of `prompts/`, `scripts/`, and `config.json` into a top-level `.pr9k/workflow/` directory in the repo itself, so the repo ships a working override of its own tooling. Doing that plus the bundle move plus the binary rename plus the `config.json` rename in one release multiplies test surface too far. Post-0.7.0, consider a `.pr9k/workflow/` source tree and update `Makefile` to `cp -r .pr9k/workflow bin/.pr9k/`.

### 4.4 CLI — `src/internal/cli/args.go`: new `resolveWorkflowDir`

Replace the current `resolveWorkflowDir` (returns `filepath.Dir(executable)`) with one that:

1. If `--workflow-dir` was provided, use it verbatim (after `EvalSymlinks` + `IsDir` check, as today).
2. Otherwise, resolve `projectDir` first (already done in the same function today), then check `<projectDir>/.pr9k/workflow/`. If it exists **and is a directory**, use it.
3. Otherwise, resolve `<executableDir>/.pr9k/workflow/` (where `executableDir = filepath.Dir(EvalSymlinks(os.Executable()))`). If it exists and is a directory, use it.
4. Otherwise, return an error listing both paths it looked in.

Ordering note: because step 2 depends on `projectDir`, the existing flow that resolves `projectDir` after `workflowDir` must be reordered — `projectDir` is resolved first, then `workflowDir`. This is a small change to the `RunE` body; no public API changes.

Error message shape (to keep it debuggable):

```
cli: could not locate workflow bundle. Checked:
  - <projectDir>/.pr9k/workflow/
  - <executableDir>/.pr9k/workflow/
Install the bundle or pass --workflow-dir.
```

Also update the `--workflow-dir` flag's help string in `cmd.Flags().StringVar(...)` (today line 114): `"path to the workflow bundle directory (default: resolved from executable)"` becomes `"path to the workflow bundle directory (default: <projectDir>/.pr9k/workflow/, then <executableDir>/.pr9k/workflow/)"` so `--help` accurately describes the new resolution order.

### 4.5 Logger, artifact directory, iteration log, and `.ralph-cache/` reconciliation

The host-side `<projectDir>/logs/` footprint is produced by *three* code paths today, not one. All three move together; missing any of them leaves a partial migration where the session log lives under `.pr9k/logs/` but per-step JSONL artifacts still spill into `<projectDir>/logs/`.

**1. Session log file.** `src/internal/logger/logger.go` line 27 — change the hardcoded `logs/` path to `.pr9k/logs/`:

```go
logsDir := filepath.Join(projectDir, ".pr9k", "logs")
```

`os.MkdirAll` already handles the intermediate `.pr9k/` directory. No public API changes; only `NewLogger` internals move. Update the comment on lines 23–24 (`// NewLogger creates a new Logger that writes to logs/...` and `// under projectDir … The logs/ directory is …`) so the docstring matches.

**2. Per-run artifact directory.** `src/cmd/pr9k/main.go` line 90 — change `artifactDir := filepath.Join(projectDir, "logs", log.RunStamp())` to `filepath.Join(projectDir, ".pr9k", "logs", log.RunStamp())`. This is what `os.MkdirAll`s the per-run directory before any claude step runs.

**3. Per-step JSONL artifact path.** `src/internal/workflow/run.go` line 377 — inside the `artifactPath` closure, change `filepath.Join(executor.ProjectDir(), "logs", cfg.RunStamp, filename)` to `filepath.Join(executor.ProjectDir(), ".pr9k", "logs", cfg.RunStamp, filename)`. This is the path written into `SandboxOptions.ArtifactPath`, which `claudestream.Pipeline` opens for every claude step.

If any of these three writers disagree about the prefix, claude-step `.jsonl` files would land outside the per-run directory created in step 2, and the per-run directory itself would not match the session log path. They are coupled and must change together.

**Tests covering the `logs/` move.** All assertions on `logs/` must shift to `.pr9k/logs/`:

- `src/internal/logger/logger_test.go` — three references at lines 200, 210, 419 (the `"logs/ should not exist before NewLogger"` guard, `"logs/ not created"`, and the `readLogLines` doc comment), plus any other path assertions discovered by `grep -n logs/ logger_test.go`.
- `src/cmd/pr9k/main_test.go` — five references at lines 212, 214, 217, 250, 252 (the startup tests that assert `logs/` lives under projectDir, not workflowDir, and that `logs/` is not created when LoadSteps fails).
- `src/internal/statusline/statusline_test.go` line 937 — the `readLogFiles` doc comment referencing `dir/logs/`.
- `src/internal/workflow/run_test.go` lines 3766 and 3811 — `wantPath := "/logs/test-stamp/iter01-01-my-step.jsonl"` and the doc comment `// <projectDir>/logs/<runStamp>/iter<iter>-<stepIdx>-<slug>.jsonl.`.

**Iteration log.** The iteration log currently lives at `<projectDir>/.ralph-cache/iteration.jsonl` (`src/internal/workflow/iterationlog.go:48`). Move it to `<projectDir>/.pr9k/iteration.jsonl`. Reasons:

- The stated goal is "centralize state under `.pr9k/`" — splitting iteration.jsonl off into `.ralph-cache/` contradicts that.
- `preflight.Run` already owns directory creation for the iteration.jsonl parent (`src/internal/preflight/run.go:34`); the change is one constant string in two locations (preflight + iterationlog).

**`.ralph-cache/` story.** `containerEnv` in `config.json` writes Go/XDG cache files at paths like `/home/agent/workspace/.ralph-cache/go` inside the container. Because `<projectDir>` is bind-mounted at `/home/agent/workspace` (`src/internal/sandbox/command.go` and the docker-sandbox docs), those container-side writes materialize at `<projectDir>/.ralph-cache/<subdir>/...` on the host. Two consequences for this release:

1. **`.ralph-cache/` does not disappear from the host filesystem.** As long as `containerEnv` paths reference `.ralph-cache/`, the bind mount creates `.ralph-cache/` on the host on first claude-step run. The §2 non-goal "Renaming `.ralph-cache` inside running Docker containers" is unchanged.
2. **`preflight.Run`'s host-side `os.MkdirAll(.ralph-cache)` call still serves a purpose** — it pre-creates the directory under the host UID before the container runs as the host UID via the sandbox's UID mapping, avoiding a chmod fight when the container writes its first cache file. In this release, leave `preflight.Run`'s `.ralph-cache` MkdirAll **in place** so the cache continues to work; *additionally* MkdirAll the new `.pr9k/` umbrella so iteration.jsonl has a writable parent.

Concretely:

- `src/internal/preflight/run.go` line 34 — keep the existing `cacheDir := filepath.Join(projectDir, ".ralph-cache")` MkdirAll. Add a new MkdirAll for `filepath.Join(projectDir, ".pr9k")` so iteration.jsonl writes succeed on first run. Update the `Run` function's doc comment (lines 16–27 today) so the documented sequence lists the new step 2 (`os.MkdirAll(projectDir+"/.pr9k")` — creates the umbrella dir for iteration.jsonl and `.pr9k/logs/`) after the existing `.ralph-cache` step, with prose explaining that both directories must be pre-created under the host UID for the same reason.
- `src/internal/workflow/iterationlog.go` line 48 — change `filepath.Join(projectDir, ".ralph-cache", "iteration.jsonl")` to `filepath.Join(projectDir, ".pr9k", "iteration.jsonl")`. Update the package doc comments at lines 12, 44, and 46 (which mention `.ralph-cache/iteration.jsonl`) to `.pr9k/iteration.jsonl`.

Tests covering `.ralph-cache` → `.pr9k` for iteration.jsonl:

- `src/internal/preflight/run_test.go` — the four `TestRun_RalphCache_*` tests at lines 286, 305, 324, 353 stay (the `.ralph-cache` MkdirAll is preserved). Add new tests covering the `.pr9k/` MkdirAll: `TestRun_Pr9kDir_CreatedOnFirstRun`, `TestRun_Pr9kDir_IdempotentOnRepeatRun`, `TestRun_Pr9kDir_FileClashSurfacesError` (mirroring the `.ralph-cache` set).
- `src/internal/workflow/iterationlog_test.go` — five `TestAppendIterationRecord_*` tests at lines 25, 52, 89, 128, 146. Every reference to `.ralph-cache` (lines 27, 54, 91, 114, 127, 130, 138, 145, 148) becomes `.pr9k`.
- `src/internal/workflow/run_iterationlog_test.go` — at least the references at lines 146 (doc comment) and 774 (`jsonlDir := filepath.Join(projectDir, ".ralph-cache", "iteration.jsonl")`). Re-grep for `.ralph-cache` after the rename to catch any other paths in the file's 23 tests.

### 4.6a `.gitignore` — repo-side housekeeping

The repo's own `.gitignore` (lines 43–47 today) ignores both `logs/` and `.ralph-cache/`. After this release:

- The pr9k repo itself does not run pr9k against itself by default, but it has done so during dogfooding (the working tree currently shows `logs/`, `.ralph-cache/`, `progress.txt`, `deferred.txt` at the root). The `.gitignore` must continue to keep those traces out of git regardless of which path layout produced them.
- Add a new `.pr9k/` entry covering both the future on-disk runtime output and the optional in-repo `.pr9k/workflow/` override (which, if a contributor adds one to dogfood, should not accidentally get committed in this release).
- Keep the existing `logs/` entry — historical runs may still be in the working tree at upgrade time and there is no value in surfacing them as untracked.
- Keep the existing `.ralph-cache/` entry per the lifecycle reasoning in §4.5 (the bind mount continues to materialize this directory on the host).

Concretely, change the `# Runtime logs` block to:

```
# Runtime logs and runtime state
logs/                # legacy (pre-0.7.0) — kept until users migrate
.pr9k/               # session logs, iteration.jsonl, optional in-repo workflow override

# Build artifact cache (populated by containerEnv Go cache vars via the sandbox bind mount)
.ralph-cache/
```

### 4.6 `config.json` — rename from `config.json`, no schema changes

**Filename rename.** `git mv src/config.json src/config.json`. The JSON contents are byte-for-byte identical; only the filename moves.

**Code constants** — pr9k looks the file up by name in two places (a third reads it by environment variable in tests). Both literal constants change from `"config.json"` to `"config.json"`:

- `src/internal/steps/steps.go` line 72 — inside `LoadSteps`, `path := filepath.Join(workflowDir, "config.json")` becomes `filepath.Join(workflowDir, "config.json")`. Update the function's doc comment to match.
- `src/internal/validator/validator.go` line 157 — inside `Validate`, the same join becomes `filepath.Join(workflowDir, "config.json")`. Update surrounding doc comments.
- `src/internal/validator/production_steps_test.go` lines 26, 36, 38, 40, 41, 57, 63 — the production-config integrity test reads `src/config.json` directly to validate the shipped config; rename every literal and doc reference to `config.json`. The helper variable `ralphTUIDir` (line 36) is unrelated to this rename but should be renamed in the §4.1 import-path pass.
- `src/cmd/pr9k/main_test.go` lines 49, 61, 66, 78, 222, 225, 239, 393, 405, 410, 426 — `writeMinimalStepFile`, `writeInvalidStepFile`, `writeWarningOnlyStepFile`, `writeWarningAndFatalStepFile` all `os.WriteFile` to `filepath.Join(dir, "config.json")`; rename to `config.json`. Doc comments referencing "missing config.json" become "missing config.json".

**`{{WORKFLOW_DIR}}` and script-resolution audit.** The shipped `src/config.json` (post-rename) still uses two conventions that need to be verified post-bundle-move:

- `{{WORKFLOW_DIR}}/ralph-art.txt` (line 15 of the file) — continues to work; `{{WORKFLOW_DIR}}` is whatever `cli.Config.WorkflowDir` resolved to.
- `scripts/get_next_issue`, `scripts/post_issue_summary`, etc. — today these resolve against `workflowDir`. The workflow runner joins `workflowDir + scripts/foo` before executing (see `src/internal/workflow/workflow_test.go:516-518` after rename). After the move, `workflowDir = <bundle>/.pr9k/workflow`, and `<bundle>/.pr9k/workflow/scripts/foo` is exactly where the `Makefile` (§4.3) places them. No JSON edits required.
- The `statusLine.command` field (`scripts/statusline`, line 11 of the file) follows the same resolution as other scripts. No edit required; verify via the statusline integration test.

**Comments and prose in non-test Go code that name the file.** The following references to `config.json` appear in code comments (not just doc files) and become `config.json`:

- `src/cmd/pr9k/wiring.go` line 60 — `// in config.json), which causes statusline.New …`.
- `src/internal/sandbox/command.go` — references in package comments to "config.json" (audit during the rename pass).
- `src/internal/statusline/statusline.go` and `src/internal/workflow/{run,workflow}.go` — same audit.

**Note on workflow content.** The internal `ralph` token (issue label, `progress.txt`, `ralph-art.txt`) inside the JSON's command lines is *workflow content*, not a tool-identity reference, and is intentionally left untouched per §2 non-goals.

### 4.7 Docs

The rename reaches into nearly every doc file; the consolidation reaches fewer. Handle both in a single coordinated pass so the docs are internally consistent.

**Scope rule.** Two doc surfaces are intentionally **not** rewritten:

- Everything under `docs/plans/*.md` — these are historical plan documents that describe the state of the system *at the time the plan was written*. Editing them after the fact rewrites history and breaks the cross-references back from ADRs, issues, and commit messages. This includes the already-merged plans (`docs/plans/ralph-tui.md`, `docs/plans/docker-sandbox/design.md`, `docs/plans/streaming-json-output/design.md`, `docs/plans/status-line/design.md`, `docs/plans/word-wrap-and-select/design.md`, `docs/plans/command-confirmation/design.md`, `docs/plans/use-bubble-tea.md`, `docs/plans/cobra-cli-option-parsing.md`, `docs/plans/ux-corrections/design.md`) **and** `docs/plans/workflow-optimization/design.md`, which was merged on 2026-04-17 but is still a record of past intent under the "plans live forever as written" rule. This plan file itself (`docs/plans/workflow-organization/design.md`) is the only exception; it is the current plan and keeps evolving through the iterative review.
- `docs/adr/*.md` — ADRs are immutable records of *past* decisions; do not retroactively edit them. Instead, add a new ADR that records this rename and the `.pr9k/` layout decision (see §4.7 final bullet), and rely on cross-reference from the new ADR to flag superseded passages.

Doc surfaces that **are** rewritten in this release:

- `README.md` — the screenshot path (`images/ralph-tui-screenshot.png` is not under `ralph-tui/`, so the file itself stays — just confirm the link target), every `path/to/pr9k/bin/ralph-tui` invocation example (lines 34, 37, 48, 51, 54), the `cd path/to/pr9k/ralph-tui && go build -o ../ralph-tui ./cmd/ralph-tui` directions (lines 57–58), the "for ralph-tui" install note (line 13), the "running ralph-tui on your own projects" phrasing (line 85), and the `ralph-tui Plan` link label (line 120). The image filename `ralph-tui-screenshot.png` is *not* renamed in this release — it is a workflow-identity artifact in the same sense as `ralph-art.txt` (§2 non-goals).
- `CLAUDE.md` — the "Project Overview", "ralph-tui (Go/Bubble Tea)" section (retitle to "pr9k (Go/Bubble Tea)"), "Build and run" snippet, "Project Discovery → pr9k" subsection, every `cd ralph-tui &&` command, every path reference like `ralph-tui/internal/...`, and the `./bin/ralph-tui …` invocation lines.
- `docs/architecture.md` — title (line 1, "ralph-tui Architecture" → "pr9k Architecture"), every prose mention of "ralph-tui" (lines 3, 5, 330, 332, 339), the package-layout diagram showing `cmd/ralph-tui/main.go` (line 284), and the WorkflowDir/log-output paragraphs.
- `docs/features/cli-configuration.md` — WorkflowDir resolution order, error message, the table of callers (line 220, `{projectDir}/logs/ralph-*.log` → `{projectDir}/.pr9k/logs/ralph-*.log`), the `--version` output line ("`ralph-tui version <semver>`" → "`pr9k version <semver>`"), the line 214 "logs/ land alongside the work" passage, and the line 318 `TestStartup_LoadStepsFailure` description ("no logs/ directory" → "no .pr9k/logs/ directory").
- `docs/features/workflow-orchestration.md` — line 358 (`<projectDir>/logs/<runStamp>/...`) and line 604 (`TestRun_ClaudeStep_ArtifactPathInSandboxOptions` description with `<projectDir>/logs/<runStamp>/...`) — both gain `.pr9k/` between `<projectDir>` and `logs/`.
- `docs/features/docker-sandbox.md` — `ralph-tui/internal/...` paths (lines 19–25, 99–106), the `ralph-tui (orchestrator)` block-diagram label (line 31), the `ralph-tui constructs the following command...` prose (line 48), the `Run: ralph-tui sandbox create` error string (line 219), and the `ralph-tui sandbox create / login` link label (line 227).
- `docs/features/sandbox-subcommand.md` — every `ralph-tui` reference: file-path bullets (lines 22–28, all use `ralph-tui/cmd/ralph-tui/...`), the example invocations (lines 33–35, 46, 68), and the prose at lines 3 and 14 ("`ralph-tui sandbox` is the parent command" / "running bare `ralph-tui sandbox` prints the help").
- `docs/features/subprocess-execution.md` — file-path bullets at lines 20–23 and the table at lines 74–77 (all `ralph-tui/internal/...`).
- `docs/features/tui-display.md` — the ASCII diagram's `ralph-tui v0.6.0` label.
- `docs/features/status-line.md` — the `version` field description (`"ralph-tui version (version.Version)"` → `"pr9k version (version.Version)"`).
- `docs/how-to/getting-started.md` — the `ralph-tui version 0.4.1` example output, the "ralph-tui v<semver>" footer description, and the **`echo 'logs/' >> .gitignore`** instruction (line 41 + surrounding paragraph at line 38) which becomes `echo '.pr9k/' >> .gitignore` per §4.6a.
- `docs/how-to/reading-the-tui.md` — the ASCII TUI screenshot, the `Model.View()` description, and the `ralph-tui --version` reference.
- `docs/how-to/building-custom-workflows.md` — **new section** on per-repo `.pr9k/workflow/` override. Rename the `--workflow-dir` primary example to the in-repo override where appropriate.
- `docs/how-to/debugging-a-run.md` — every `logs/` reference (lines 9, 11, 17, 19, 44, 72, 78, 84, 90, 98, 167, 173) becomes `.pr9k/logs/`; the `.gitignore` snippet at line 19 changes from `echo 'logs/' >> .gitignore` to `echo '.pr9k/' >> .gitignore`; the table at line 11 (`<project-dir>/logs/<runstamp>/...`) becomes `<project-dir>/.pr9k/logs/<runstamp>/...`.
- `docs/how-to/configuring-a-status-line.md` — line 116 ("under `logs/` in your project directory") → "under `.pr9k/logs/`"; the `tail -f logs/...` example at line 119 → `tail -f .pr9k/logs/...`. The `ralph-tui logs all status-line activity` prose (line 116) becomes `pr9k logs all status-line activity`.
- Every file in `docs/code-packages/` — replace `ralph-tui/internal/...` paths in file-path bullets and tables with `src/internal/...`. Specifically: `logger.md` (lines 11, 18–19, 40, 51–52, 73, 77, 125, 178 — including the `logs/ralph-` ASCII art at line 40 and the `logsDir := filepath.Join(projectDir, "logs")` code block at line 77 which becomes `".pr9k", "logs"`), `claudestream.md` (lines 256, 291, 294 — `logs/` paths and the `cmd/ralph-tui/main.go` heading), `vars.md`, `steps.md`, `validator.md`, `statusline.md`, `sandbox.md`, `preflight.md`, `workflow.md`.
- Every file in `docs/coding-standards/` — grep for `ralph-tui` occurrences; `versioning.md` mentions `ralph-tui` in eight passages (lines 3, 7, 9, 11, 13, 15, 17, 20, 24, 44) including the `ralph-tui/internal/version/version.go` path, the `ralph-tui's "public API"` heading, and the `ralph-tui version <semver>\n` `--version` format. All become `pr9k …` / `src/internal/…` / `pr9k version <semver>\n`.

**`config.json` rename — doc surface.** Every doc that names `config.json` becomes `config.json`. Locations to update (audit via `grep -rn config.json docs/ CLAUDE.md README.md` after the code rename):

- `CLAUDE.md` — every `config.json` mention in the Project Overview, Key Design Decisions, ralph-tui section, and code-package summaries.
- `docs/architecture.md` — references in the package-layout and configuration-loading prose.
- `docs/code-packages/{steps,validator,workflow,sandbox,statusline}.md` — file-path bullets (e.g. `src/config.json` → `src/config.json`) and prose.
- `docs/coding-standards/{versioning,go-patterns,testing,documentation}.md` — every prose mention.
- `docs/features/{docker-sandbox,subprocess-execution,workflow-orchestration,keyboard-input}.md` — every prose mention.
- `docs/how-to/*.md` — every prose mention; particularly `building-custom-workflows.md`, `variable-output-and-injection.md`, `capturing-step-output.md`, `passing-environment-variables.md`, `breaking-out-of-the-loop.md`, `skipping-steps-conditionally.md`, `setting-step-timeouts.md`, `caching-build-artifacts.md`, `resuming-sessions.md`, `configuring-a-status-line.md`.

Historical plan documents (`docs/plans/*.md`) and ADRs (`docs/adr/*.md`) are *not* rewritten — they remain accurate descriptions of past intent. The new ADR (next bullet) records the rename.

**New ADR.** Add `docs/adr/<datestamp>-pr9k-rename-and-pr9k-layout.md` recording (a) the rename of binary, source dir, and module path; (b) the `.pr9k/` umbrella decision; (c) the `config.json` → `config.json` rename and its rationale (filename describes what pr9k loads, not which workflow is inside it); (d) the explicit non-goals from §2; (e) the supersession of the WorkflowDir-resolution and log-path passages in earlier ADRs (`20260413162428-workflow-project-dir-split.md`, `20260413160000-require-docker-sandbox.md`) and of the narrow-reading ADR's claim that "workflow content stays in JSON" — still true, but the *file's name* is now a tool-identity surface, not a workflow-content surface. The new ADR's `Apply when` line: "any change to binary name, source-tree layout, runtime output paths, workflow discovery, or the workflow-config filename."

Per `docs/coding-standards/documentation.md`, feature docs ship with the feature — all doc updates land in the same PRs as the code change, not follow-ups.

### 4.8 Version bump

`src/internal/version/version.go`: `0.6.1` → `0.7.0`.

Per `docs/coding-standards/versioning.md`, this release changes:
- The binary name (`ralph-tui` → `pr9k`). Anyone with a shell alias, symlink, PATH entry, or CI job referencing `ralph-tui` must update. This is the single most-impactful breaking change in the release.
- The `--version` output prefix (`ralph-tui version` → `pr9k version`). Any external script that greps for the prefix breaks.
- The `--workflow-dir` default (old: executable dir; new: two-candidate with in-repo override preferred). Anyone who scripted `--workflow-dir=.` or relied on the executable-dir default is affected.
- The log output location (`logs/` → `.pr9k/logs/`). Anyone with a `logs/` gitignore entry needs to replace it with `.pr9k/`.
- The iteration.jsonl location (§4.5). External tooling that reads it must update the path.
- The Go module path. Anyone who imported packages from `github.com/mxriverlynn/pr9k/ralph-tui/...` (unlikely — pr9k is an app, not a library) must re-import.

- The workflow-config filename (`config.json` → `config.json`). Anyone shipping a custom workflow bundle (or whose `--workflow-dir` points at a directory containing the old name) must rename their copy.

No CLI flag renames, no JSON *schema* changes inside the renamed `config.json` (only the filename moves), no `{{VAR}}` language changes.

## 5. Resolution order — worked examples

| Situation | Winner |
| --- | --- |
| User runs `pr9k` from `~/dev/myrepo`; no `.pr9k/workflow/` in the repo; binary at `/usr/local/bin/pr9k` linked to `/opt/pr9k/bin/pr9k` | `/opt/pr9k/bin/.pr9k/workflow/` |
| Same as above, but the repo has `~/dev/myrepo/.pr9k/workflow/config.json` | `~/dev/myrepo/.pr9k/workflow/` |
| User passes `--workflow-dir=/tmp/custom` | `/tmp/custom` (even if both defaults would also resolve) |
| User passes `--workflow-dir=/tmp/custom` and `/tmp/custom` does not exist | Error — "cli: --workflow-dir /tmp/custom: no such file or directory" (existing error path) |
| In-repo `.pr9k/workflow/` exists but is a file, not a directory | Fall through to executable-dir candidate (step 3 of §4.4's resolver) |
| Neither candidate exists, no flag | Error listing both paths checked |

## 6. Testing plan

Targeted test additions, matched to the packages already touched:

- `src/internal/cli/args_test.go`
  - New: resolver prefers `<projectDir>/.pr9k/workflow/` over `<executableDir>/.pr9k/workflow/` when both exist.
  - New: resolver falls back to executable-dir candidate when project-dir candidate is missing.
  - New: resolver falls back to executable-dir candidate when project-dir candidate is a regular file.
  - New: resolver returns a clear error listing both candidates when neither exists.
  - New: `--workflow-dir` flag still wins over both candidates.
  - New: when `--project-dir` is invalid and no `--workflow-dir` is given, the project-dir error fires (the §4.4 reordering is observable — projectDir is resolved before workflowDir, so a bad `--project-dir` short-circuits before the resolver runs).
  - **Existing tests that exercise `runNewCommand` without `--workflow-dir` (today they rely on the executable-dir always winning) need adjustment** — the resolver now requires a candidate to exist and is not satisfied by `filepath.Dir(testBinary)` alone. The affected tests are `TestNewCommand_NoFlags` (line 28), `TestNewCommand_LongIterationsFlag` (45), `TestNewCommand_ShortIterationsFlag` (56), `TestNewCommand_LongProjectDirFlag` (93), `TestNewCommand_EqualsSyntax` (159), `TestNewCommand_ProjectDirEvalSymlinks` (324), and `TestNewCommandImpl_AddedSubcommandRunsItsRunE` (402). For each, choose one of:
     1. Add `--workflow-dir t.TempDir()` to the args (preferred when the test's intent is iteration / project-dir handling, not workflow-dir resolution).
     2. Use a new test helper `setupWorkflowCandidate(t, dir)` that creates `<dir>/.pr9k/workflow/config.json` and chdirs into it (preferred when the test's intent is "no flags at all").
  - The flag's help text `"path to the workflow bundle directory (default: resolved from executable)"` (line 114 today) becomes `"path to the workflow bundle directory (default: <projectDir>/.pr9k/workflow/, then <executableDir>/.pr9k/workflow/)"`. No assertion change needed — no existing test checks the help string — but the new resolver tests should assert it via `cmd.Flags().Lookup("workflow-dir").Usage`.
- `src/internal/logger/logger_test.go` — update path assertions from `logs/` to `.pr9k/logs/`.
- `src/internal/preflight/run_test.go` — rename `.ralph-cache` assertions to `.pr9k` (three tests).
- `src/internal/workflow/iterationlog_test.go` and `run_iterationlog_test.go` — path assertions.
- `src/internal/ui/model_test.go` and `src/internal/ui/version_footer_test.go` — update the three `"ralph-tui v..."` fixtures to `"pr9k v..."`.
- `src/cmd/pr9k/main_test.go` — the top-level startup test that currently exercises `TestStartup_LoadStepsFailure` needs a new variant for "no workflow bundle anywhere" (distinct from "bundle exists but config.json is missing"). The four `writeXXXStepFile` helpers (lines 49, 66, 393, 410) write to `config.json` after the §4.6 rename.
- `src/cmd/pr9k/doc_integrity_test.go` — update the two `"ralph-tui v"` assertions.
- Integration: the existing `production_steps_test.go` that validates the shipped workflow config already runs against a relative path — verify the path still resolves after the `src/` rename **and** the `config.json` → `config.json` rename, and adjust if needed.

Race detector must stay on (`go test -race ./...`) per `docs/coding-standards/testing.md`.

## 7. Rollout

The rename and the reorg are coupled enough that sequencing them across many PRs produces more churn than it prevents. The recommended order:

1. **PR 1 — Rename.** `git mv ralph-tui src`, `git mv src/cmd/ralph-tui src/cmd/pr9k`, `go.mod` module path, scripted import-path rewrite, `Makefile` path updates (but **not** the `.pr9k/workflow/` layout yet — only the binary name `-o bin/pr9k`), cobra `Use` string, `main.go` versionLabel, test fixture strings, and all doc renames. `go test -race ./...` must pass end-to-end before merge.
2. **PR 2 — Layout and config rename.** `Makefile` target builds `bin/.pr9k/workflow/`; CLI resolver switches to the two-candidate rule; logger writes to `.pr9k/logs/`; per-step JSONL artifact directory and `main.go` artifact-dir creation move to `.pr9k/logs/<runStamp>/`; iteration log moves to `.pr9k/iteration.jsonl`; `git mv src/config.json src/config.json` and the two literal constants in `steps.LoadSteps` and `validator.Validate` flip; `.gitignore` gains `.pr9k/`; docs for both the layout move and the `config.json` rename land; new ADR lands; `version.Version` → `0.7.0`.
3. Doing both in a single PR is also acceptable if the reviewer can handle the diff; the two-PR split is only about reviewability, not correctness.

No in-repo migration script. Users upgrading to 0.7.0 manually delete `<repo>/logs/` and `<repo>/.ralph-cache/` if they want a tidy switch; the new layout populates itself on first run. The binary name change is handled by whatever installer the user uses — there is no automated `ralph-tui` → `pr9k` shim.

## 8. Risks and mitigations

| Risk | Mitigation |
| --- | --- |
| Scripted rewrite of `github.com/mxriverlynn/pr9k/ralph-tui` → `github.com/mxriverlynn/pr9k/src` misses a file because of unusual formatting | `go build ./...` + `go test -race ./...` catch this — an unrewritten import is a compile error. |
| Tests reference the *module path* via reflection / runtime introspection | Low probability. If it shows up, the test breaks loudly — fix in place. |
| A user's target repo has an unrelated `.pr9k/` directory (from another tool) | Unlikely — `.pr9k` is project-specific enough. If it happens, they can pass `--workflow-dir` to bypass the project-dir candidate. |
| External scripts grep `--version` output for `"ralph-tui"` | Called out in §4.8 as a breaking change; doc the new `"pr9k version <x>"` contract in `docs/features/cli-configuration.md`. |
| `docs/adr/20260410170952-narrow-reading-principle.md` — does the resolver violate "narrow reading"? | No. The resolver knows two filesystem paths, not workflow content. |
| Symlink farms — user has `<projectDir>/.pr9k/workflow` as a symlink to a third location | `EvalSymlinks` handles this; the resolver already uses it per §4.4. |
| External tooling reads `<projectDir>/.ralph-cache/iteration.jsonl` | Documented breaking change in §4.8; update `docs/how-to/debugging-a-run.md`. |
| `make build` run against an existing `bin/` with the old layout leaves stale files | `Makefile` already runs `rm -rf bin` before rebuilding. |
| Git history harder to follow after `git mv ralph-tui src` moves all tracked files under that subtree (113 at planning time, 110 of them `.go`) | `git log --follow` still works. The rename is a one-time cost for long-term readability. |
| User has a workflow bundle named `config.json` (in their own `--workflow-dir` or in-repo `.pr9k/workflow/`) and pr9k 0.7.0 silently fails to find it | `steps.LoadSteps` returns a clear error today (`steps: read /<dir>/config.json: no such file or directory`) — surfaced through the same path that `TestStartup_LoadStepsFailure` exercises. Documented as a breaking change in §4.8 with the rename instruction. |

## 9. Out of scope / follow-ups

- Moving the repo's source-of-truth `prompts/`, `scripts/`, `config.json` into a top-level `.pr9k/workflow/` tree (§4.3, deferred).
- Renaming `.ralph-cache` inside containers — requires coordinated edits to `containerEnv` in `config.json` and the docker-sandbox docs.
- Renaming `ralph-art.txt`, the `ralph` issue label, `progress.txt`, `deferred.txt`, and any other *workflow-identity* artifacts from "ralph" to "pr9k". Intentionally left alone: pr9k is the tool, Ralph is the workflow it runs. (`config.json` was on this list pre-iteration — it is now in scope as `config.json` per §4.6, because the filename names what pr9k *loads*, not what the workflow *contains*.)
- JSON Schema distribution under `.pr9k/schemas/` for editor autocompletion on `config.json`.
- `$XDG_CONFIG_HOME`-style global workflow discovery.
- Renaming the GitHub repo from `pr9k` to something else — not applicable; the repo is already `pr9k`.

## 10. Iterative review summary

This plan was sharpened through codebase-grounded iterations on 2026-04-18. Findings and resulting edits:

**Iteration 1 — code-surface verification.**
- Recorded a "134-file" count for `github.com/mxriverlynn/pr9k/ralph-tui` import-path occurrences. (Iteration 4 corrected this to 87 occurrences across 38 files — the original count was wrong.)
- Verified specific line references for cobra `Use` (args.go:61), version label (main.go:158), and the four `"ralph-tui v..."` test fixtures (model_test.go:19/449/460, version_footer_test.go:18). All correct.
- Discovered three host-side `logs/` writers — not one. `logger.go:27` was the only one in the original plan; added `cmd/pr9k/main.go:90` (artifact dir) and `internal/workflow/run.go:377` (per-step JSONL artifact path) to §4.5. Without this, the migration would have been partial (session log under `.pr9k/logs/`, JSONL artifacts still under `logs/`).
- Discovered that the original plan's "error-message prefix audit" (grep for `ralph-tui:`) missed the broader user-facing string surface. Added a full §4.2 inventory of help/error/comment strings (eight files: main.go, sandbox_login.go, sandbox.go, args.go Long, preflight/docker.go, preflight/profile.go, sandbox/command.go, sandbox/image.go, version/version.go, scripts/post_issue_summary_test.go).
- Discovered the original plan's `.ralph-cache/` lifecycle claim ("ceases to exist on host") contradicted the docker bind-mount behavior — `containerEnv` paths materialize on the host via the bind. Rewrote §4.5 to keep `preflight.Run`'s `.ralph-cache` MkdirAll, add a new `.pr9k/` MkdirAll, and updated §4.6a `.gitignore` accordingly.
- Discovered .gitignore was not in the plan. Added §4.6a.
- Corrected test counts: §4.5 said "three" `.ralph-cache` tests in preflight; actually four (TestRun_RalphCache_*: 286/305/324/353). Same for iteration-log tests (5 in iterationlog_test.go, additional in run_iterationlog_test.go).
- Expanded §4.7 docs list with README.md and seven additional doc files that referenced `ralph-tui` or `logs/`.

**Iteration 2 — test-impact verification.**
- Discovered that the new `resolveWorkflowDir` in §4.4 breaks seven existing tests in `args_test.go` (TestNewCommand_NoFlags, _LongIterationsFlag, _ShortIterationsFlag, _LongProjectDirFlag, _EqualsSyntax, _ProjectDirEvalSymlinks, _AddedSubcommandRunsItsRunE) that today rely on the executable-dir always satisfying the resolver. Added explicit migration guidance to §6.
- Added §4.4 update to the `--workflow-dir` flag's `Usage` string to reflect the new resolution order.

**Iteration 3 — user-introduced scope addition.**
- User asked mid-review to also rename `config.json` → `config.json`. This contradicted the original §2 non-goal and required propagation across §1 prose, §2 goals/non-goals, §3 layout diagrams, §4.3 Makefile, §4.6 (rewritten), §4.7 doc surface, §4.8 breaking-changes list, §6 testing plan, §7 rollout, §8 risks, and §9 follow-ups. All occurrences updated.

**Iteration 4 — completeness / ambiguity sweep.**
- Corrected the "134 files" import-path count in §4.1. A fresh `grep -rn github.com/mxriverlynn/pr9k/ralph-tui --include="*.go"` at `fc8b054` returns 87 occurrences across 38 files, all under `ralph-tui/`. Also corrected the corresponding "134 files" reference in the §8 risk row to the actual 113 git-tracked files moved by `git mv ralph-tui src` (110 of them `.go`).
- Resolved a silent ambiguity: the plan's §3.2 layout diagram wrote `ralph-2026-04-18-161500.123.log` but never stated whether the `ralph-` filename prefix (produced by `logger.go:33-34`) stays or changes. Added an explicit §2 non-goal covering three related `ralph-` basename surfaces — log filename prefix, per-run artifact directory prefix (from `RunStamp()`), and `sandbox/cidfile.go:20`'s `ralph-*.cid` tempfile pattern — all of which stay "ralph-" for the same reason `ralph-art.txt` does (workflow identity, not tool identity).
- Filled a §4.2 audit gap. The inventory did not enumerate `scripts/statusline` (shipped demo script, two "ralph-tui" comments on lines 2–3) or `src/internal/validator/prompts_structure_test.go:28`. Added both. Expanded the audit-method grep to cover `scripts/` because `scripts/` is part of the shipped bundle, not just Go test fixtures.
- Tightened the §4.7 scope rule. The original wording called out only two plan files as frozen-history; the rule now covers all of `docs/plans/*.md`, explicitly listing the recently-merged `docs/plans/workflow-optimization/design.md` (merged 2026-04-17) as in-scope-for-freeze, with the plan-in-progress itself (this file) as the only exception.
- Added a `preflight.Run` doc-comment update to §4.5. The function's package doc comment (lines 16–27) describes its MkdirAll sequence and currently names only `.ralph-cache`; after this release it also creates `.pr9k`, and the doc comment must reflect both.

Open questions: none. Every "deferred" item in the plan body is now explicit in §9.
