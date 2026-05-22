# Versioning

pr9k follows [Semantic Versioning 2.0.0](https://semver.org/spec/v2.0.0.html). The general rules of semver — MAJOR for incompatible changes, MINOR for backwards-compatible additions, PATCH for backwards-compatible fixes, pre-release and build metadata suffixes, precedence, etc. — are not restated here. Read the spec. This standard only records the things that are specific to this repository.

## Version bumps are manual and operator-requested ONLY

**Never bump `version.Version` automatically, proactively, or as part of any other change.** A version bump happens only when the repository operator explicitly asks for one, in that request, naming it as the task. There is no exception.

This rule exists because automated and drive-by version bumps have repeatedly outpaced actual releases — the version constant has moved many times with no release behind it. Bumping the version is not housekeeping and it is not part of "shipping a feature." It is a deliberate, separate, operator-owned act.

Concretely, this means:

- **Do not** bump the version because a PR feels significant, adds a feature, changes a public-API surface, or "ships a phase."
- **Do not** bump the version as a courtesy, a finishing touch, or a "while I'm here" edit.
- **Do not** infer that a version bump is needed from the nature of a change. The change being MAJOR/MINOR/PATCH in semver terms tells you *what number* a future bump would use — it does **not** authorize you to perform the bump.
- A feature PR, a docs PR, a refactor PR, and a bug-fix PR all leave `version.Version` **untouched**. `main` carries the version of the last release between releases.
- When you finish work that *would* warrant a bump at release time, do not bump and do not offer to. If anything, leave it to the operator; they will request a bump when a release is actually being cut.

The semver classification rules below (what counts as the public API, MAJOR vs. MINOR vs. PATCH, `0.y.z` behavior) describe how to *choose the number* when the operator has requested a bump. They never, on their own, trigger one.

## The version constant is the single source of truth

The current version lives in exactly one place: `src/internal/version/version.go`, as the exported constant `version.Version`.

- Every consumer — the `--version` / `-v` CLI flag, the TUI footer, release tooling, any future `About` dialog — **MUST** import `github.com/mxriverlynn/pr9k/src/internal/version` and read `version.Version`. Never hardcode the version string anywhere else.
- Never introduce a second constant, a build-time `-ldflags "-X ..."` override, a `VERSION` file, or a `go:embed`'d text file as an alternate source. One constant, one file.
- Tests that need to assert the version string **MUST** read it from the package, not from a literal. See `src/internal/cli/args_test.go` for the pattern.

## What counts as pr9k's "public API"

Semver rules apply to backwards-compatible vs. backwards-incompatible changes to a declared public API. pr9k is a CLI application, not a Go library, so its "public API" is specifically:

1. **The CLI surface** — every flag on `pr9k` (name, short alias, type, default, accepted values) and the exit codes it returns. This includes both `--workflow-dir` and `--project-dir` (introduced in 0.3.0). Renaming `--iterations`, changing `-n`'s default, or flipping an exit code from `0` to `1` for the same scenario are all **MAJOR** changes.
2. **The `config.json` schema** — every field name, every required-vs-optional rule, every accepted value for `phase`, `type`, `captureAs`, `breakLoopIfEmpty`, and so on. Any existing user's `config.json` that was valid before must still be valid and still produce the same workflow after the upgrade, or it's a **MAJOR** change.
3. **The `{{VAR}}` substitution language** — the token syntax, the built-in variable names (`{{WORKFLOW_DIR}}`, `{{PROJECT_DIR}}`, `{{MAX_ITER}}`, `{{ITER}}`, `{{STEP_NUM}}`, `{{STEP_COUNT}}`, `{{STEP_NAME}}`), and the persistent-vs-iteration scoping rules documented in `docs/how-to/workflow-variables.md` and `docs/how-to/capturing-step-output.md`.
4. **The `--version` output format** — `pr9k version <semver>\n` on stdout. Users may script against this; changing the format is **MAJOR**.

The following are explicitly **NOT** part of the public API and may change in any release without a MAJOR bump:

- Internal Go package layout under `src/internal/**`. Nothing outside this repo should import it.
- The prompt files in `prompts/`. They are replaceable content, not API.
- The TUI layout, colors, chrome, checkbox glyphs, shortcut bar wording, and footer arrangement. Visual polish is not versioned.
- The persisted log file format in the target repo's working directory. It is a debugging aid, not a data interchange format.
- The helper scripts in `scripts/` (`get_next_issue`, `close_gh_issue`, etc.) when invoked directly. They are an implementation detail of the default workflow; users who want stable shell entry points should vendor their own.

When the operator requests a bump, classify the accumulated changes since the last release: if any change broke one of the four items above for an existing user, the bump is MAJOR; if changes only added to them in a backwards-compatible way, it's MINOR; if nothing user-visible changed, it's PATCH. This classification chooses the *number* for an operator-requested bump — it does not, by itself, authorize a bump. See "Version bumps are manual and operator-requested ONLY" above.

## `0.y.z` — initial development

pr9k is in the `0.y.z` initial-development series. Per semver §4, while MAJOR is `0`, **anything may change at any time** and the public API is not considered stable. For this repo, that means:

- Backwards-incompatible changes to the CLI surface or `config.json` schema during `0.y.z` bump the **MINOR** (e.g. `0.6.0` → `0.7.0`), not the major.
- Backwards-compatible additions and bug fixes both bump the **PATCH** (e.g. `0.6.0` → `0.6.1`).
- The first `1.0.0` release is the commitment that the four "public API" items above are stable and will be governed by the full semver rules going forward. Do not bump to `1.0.0` casually — it should be a deliberate decision with a corresponding entry in the repo's plans or ADRs.

## How to bump the version

**Precondition: the operator has explicitly requested a version bump.** If they have not, stop — see "Version bumps are manual and operator-requested ONLY" above. Do not begin these steps on your own initiative, and do not fold them into an unrelated PR.

Once the operator has requested a bump:

1. Confirm the target version with the operator if it was not stated in the request. The semver classification rules in this document tell you which component (MAJOR/MINOR/PATCH) the change history would justify, but the operator owns the final number.
2. Update `const Version` in `src/internal/version/version.go`.
3. Run `make ci` — this rebuilds the binary and runs the existing version tests in `src/internal/cli/args_test.go`, which read from `version.Version` and will pass automatically.
4. Commit the bump as **its own commit**, with a message of the form `Bump version to <semver>`. This commit should contain only the version constant change (and any generated artifacts that track it, if we add them later).
5. Tag the commit `v<semver>` (e.g. `v0.2.0`) on `main`. The tag name **MUST** have a `v` prefix; the constant value **MUST NOT**. `git tag v0.2.0` matches constant `"0.2.0"`.

A release is a deliberate act, and so is the bump that precedes it. Between releases, `main` carries the version of the last release, not an in-progress next-release number.

## Pre-release and build metadata

If a pre-release suffix is ever needed (release candidates, betas, nightly builds), follow semver §9 and §10 verbatim: `0.2.0-rc.1`, `0.2.0+build.2026-04-10`. Do not invent alternate formats like `0.2.0-nightly-2026-04-10-sha1234` — use the dot-separated identifiers the spec requires. Until the repo actually ships a pre-release, `version.Version` stays a plain `MAJOR.MINOR.PATCH` string.

## Additional Information

- [Semantic Versioning 2.0.0](https://semver.org/spec/v2.0.0.html) — The full spec. This standard does not restate it; if a question is not answered above, the answer is in the spec.
- [`src/internal/version/version.go`](../../src/internal/version/version.go) — The single source of truth for the current version.
- [Step Definitions](../code-packages/steps.md) — The `config.json` schema, one of the four public-API surfaces governed by this standard.
- [CLI Configuration](../features/cli-configuration.md) — The CLI flag surface, another public-API surface governed by this standard.
