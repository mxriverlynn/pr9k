# Documentation

## Feature docs must ship with the feature, not as follow-up PRs

Every code change that adds or modifies a user-visible surface must update the relevant docs in the same commit or PR. "I'll document it later" is never enforced — it becomes technical debt that grows with every subsequent change to the same surface.

User-visible surfaces that require doc updates:
- New or changed CLI flags, exit codes, or `--version` output
- New or changed `ralph-steps.json` schema fields
- New or changed `{{VAR}}` template tokens
- New TUI modes, keyboard shortcuts, or UI regions
- New or changed step error categories (for `code-packages/validator.md`)
- New packages in the architecture graph (for `architecture.md`)
- New how-to workflows (add a guide in `docs/how-to/`)

**How to find the right doc files:** Every doc file that touches the changed surface is listed in `CLAUDE.md` under "Feature Documentation", "Code Package Documentation", and "How-To Guides". Check `CLAUDE.md` first, then search for the specific changed identifier in the docs directory.

A code review finding "docs not updated for the new X" is always a medium-severity issue. The pattern has repeated across every major feature in this codebase; do not let it recur.

## Update CLAUDE.md when adding new doc files

When you create a new doc file in `docs/features/`, `docs/code-packages/`, `docs/how-to/`, `docs/adr/`, or `docs/coding-standards/`, add a link to it in `CLAUDE.md` under the appropriate section. `CLAUDE.md` is the single index that agents and developers use to discover documentation — an unlinked doc file is effectively invisible.

Pick the right directory for new docs:
- `docs/features/` — user-facing behavior, configuration schema, cross-package integration
- `docs/code-packages/` — per-Go-package API reference (types, methods, synchronization, lifecycle). Name the file after the Go package it documents (e.g., `statusline.md` for `internal/statusline`).

```markdown
<!-- In CLAUDE.md under "Feature Documentation": -->
- [`docs/features/my-feature.md`](docs/features/my-feature.md) — One-line description of what this doc covers

<!-- In CLAUDE.md under "Code Package Documentation": -->
- [`docs/code-packages/mypkg.md`](docs/code-packages/mypkg.md) — `internal/mypkg`: one-line description of the package API
```

The one-line description must be specific enough to tell a reader whether this doc is what they are looking for without having to open it.

## Keep doc code blocks consistent with production code

When a doc file contains a code block that shows a sample script, a JSON config example, a CLI invocation, or any other artifact that can be derived from the actual production files, keep the doc block consistent with the real file. A doc block that contradicts production code is actively harmful — it misleads users and makes the feature look broken.

```markdown
<!-- Bad — doc shows the old minimal sample; scripts/statusline is the jq version -->
The sample script prints a static line:
```bash
cat >/dev/null; echo "testing status line"
```

<!-- Good — doc shows what scripts/statusline actually does, or references it -->
See `scripts/statusline` for a working example. It reads the workflow state from
stdin and renders a color-coded status line using `jq`.
```

When you change `scripts/statusline`, check whether `docs/features/status-line.md` and `docs/how-to/configuring-a-status-line.md` reference its behavior and update them in the same commit.

## Document external tool dependencies in user-facing docs

When a script or feature requires an external tool (e.g., `jq`, `git`, `docker`), document the dependency explicitly in the relevant user-facing how-to guide. Missing-tool failures manifest as silent degradation (cold-start fallback, empty output) rather than clear error messages, making them hard to diagnose without reading docs.

```markdown
<!-- docs/how-to/configuring-a-status-line.md -->
## Prerequisites

The sample script in `scripts/statusline` requires:
- **bash** — the shebang is `#!/usr/bin/env bash`; POSIX sh is not sufficient
- **jq** — used to parse the stdin JSON payload

If `jq` is not installed, the status line falls back to the cold-start blank state with no error message.
Install jq with `brew install jq` (macOS) or `apt install jq` (Debian/Ubuntu).
```

This is especially important for scripts: unlike Go code that fails to compile, a bash script that calls a missing binary fails silently at runtime.

## Additional Information

- [Architecture Overview](../architecture.md) — Full architecture reference; update when adding packages or changing the block diagram
- [CLAUDE.md](../../CLAUDE.md) — Index of all doc files; update when adding any new doc
- [Versioning](versioning.md) — When a version bump is required and how to bump
- [Testing](testing.md) — Doc integrity tests for embedded version strings
- [Config Validation](../code-packages/validator.md) — Error categories; update when adding new validation rules
