# Lint and Tooling

## Never suppress lints — fix the root cause

Lint findings from `golangci-lint`, `go vet`, `gofmt`, `govulncheck`, or any other tool run by `make ci` must be fixed, not silenced. Suppression mechanisms are not permitted in this repo under any circumstances:

- No `//nolint`, `//nolint:<linter>`, or `//lint:ignore` comments in Go source.
- No `//go:build ignore` or build tags used to hide code from linters.
- No `.golangci.yml` / `.golangci.yaml` / `.golangci.toml` clauses that disable linters, add `issues.exclude*` entries, or lower severity to hide a finding.
- No `exclude-rules`, `exclude-dirs`, or per-path skips added to make a warning go away.
- No `--no-verify` on commits, no `-tags` gymnastics to dodge a check, no renaming a symbol to `_x` purely to defeat "unused" diagnostics.
- No "pre-existing error" exception. If `make lint`, `make vet`, or any other check reports a finding — even one inherited from older code — it must be fixed in the current change.

If a lint finding seems wrong, the correct response is one of:

1. **Fix the code** so the finding no longer applies. This is the default.
2. **Remove the code** if the finding reveals that it's genuinely dead or redundant.
3. **Escalate to the human** with a concrete explanation of why the linter is incorrect for this case. Do not silently work around it.

There is no fourth option. A suppression comment is a commitment to carry the problem forever; fixing the code is a commitment to understand it once.

### Why

Suppressions rot. A `//nolint:errcheck` added for one safe call stays in place when the surrounding code is refactored and the call is no longer safe. A disabled linter in `.golangci.yml` hides every future occurrence, not just the one that prompted the change. The cost of suppression is paid by every future reader who has to re-derive whether the suppression is still justified — and by every reviewer who now has to audit suppressions as a separate concern from the code itself.

Fixing the root cause is almost always cheaper than the lifetime cost of the suppression. In the rare case where it isn't, that's a signal the linter rule is mismatched for this codebase, and the right move is to discuss removing or replacing the rule — not to sprinkle silencers.

### How to apply

- Before every commit, run `make lint` (and ideally `make ci`) locally. If anything is reported, fix it in the same commit.
- When reviewing a PR, treat any added `nolint`, any new `.golangci.yml` exclusion, or any linter being disabled as a blocking comment. Ask for the underlying fix.
- If you find an existing suppression while working in a file, remove it and fix the real issue as part of your current change.
- If `golangci-lint` itself is upgraded and new findings appear, fix them — do not pin to an older version to avoid the work.

## Exception: tools-tagged dependency-pinning files

A `//go:build tools` build tag in a standalone file (`tools.go`) that contains only blank imports is explicitly permitted. This pattern pins tool dependencies (e.g. `bubbles/viewport`) in `go.sum` without leaking them into the production binary. The tag is not hiding code from a linter — it is excluding the file from normal builds by design. Run `go vet -tags tools .` (or `make vet`) to verify the file is still correct.

This exception does **not** extend to other uses of build tags. Using `//go:build ignore` or similar to hide code from linters remains prohibited.

## Additional Information

- [Error Handling](error-handling.md) — Fixing `errcheck` findings usually means adding proper error wrapping, not suppression
- [Testing](testing.md) — Fixing `unused` findings in tests usually means the test is wrong, not the linter
- [Go Patterns](go-patterns.md) — Canonical Go patterns used in this repo
