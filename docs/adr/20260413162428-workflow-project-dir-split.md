# Split `--project-dir` into `--workflow-dir` (Workflow Bundle) and `--project-dir` (Target Repo)

- **Status:** accepted
- **Date Created:** 2026-04-13 16:24
- **Last Updated:** 2026-04-13
- **Authors:**
  - River Bailey (mxriverlynn, river.bailey@testdouble.com)
- **Reviewers:**

## Context

pr9k today has a single `--project-dir` flag (cobra registration at
`src/internal/cli/args.go:60`), a single `Config.ProjectDir` field,
and a single `{{PROJECT_DIR}}` built-in variable in its substitution
language. The name "project-dir" conflates two distinct concepts that
have coexisted implicitly:

1. **The workflow bundle.** The directory containing the artifacts that
   define the workflow being run: `ralph-steps.json`, `prompts/`,
   `scripts/`, `ralph-art.txt`. This is what today's flag actually
   resolves to, via `os.Executable()` + `filepath.EvalSymlinks`
   (`args.go:resolveProjectDir`). It is seeded into the VarTable as
   `PROJECT_DIR` (`src/internal/vars/vars.go:63`).

2. **The target repo.** The working directory pr9k operates
   against — the git repo being modified, whose cwd is inherited by
   every subprocess. Today this is implicit: `CLAUDE.md:35` states
   *"Ralph is invoked from the target repo — all subprocesses inherit
   that cwd,"* and there is no `TargetRepo` field in `cli.Config` or
   `workflow.RunConfig`, and no flag for it. An internal `workingDir`
   capture via `os.Getwd()` was introduced in commit `4f4481b` (0.2.3)
   at `cmd/src/main.go:77` and routed to the logger and runner,
   but it is not surfaced as a user-facing flag or variable. The split
   promotes that internal capture to first-class surface: `--project-dir`
   / `{{PROJECT_DIR}}` / `ProjectDir` add `filepath.EvalSymlinks`, an
   override flag, and VarTable seeding on top of the existing
   `os.Getwd()` call. The `workingDir` identifier in current code is
   renamed to `projectDir` as part of the split.

These are two distinct directories. In the pr9k project itself they
happen to be siblings (the binary lives under `bin/`, which is a
subdirectory of the target repo), but nothing in the design requires
that — a user can install pr9k anywhere and run it against any
target repo.

The ambiguity became load-bearing during design review of the Docker
sandbox plan (`docs/plans/docker-sandbox/design.md`). That plan's
docker-run template uses `-v <PROJECT_DIR>:/home/agent/workspace` to
mean the target repo — the thing being bind-mounted — while the rest
of the codebase uses `{{PROJECT_DIR}}` to mean the workflow bundle.
A careful reader noticed the plan was quietly redefining the token.
Continuing to use one name for both concepts would ship that confusion
into production code (the sandbox plan introduces a new
`BuildRunArgs(projectDir, profileDir, ...)` helper whose first
parameter is the target repo, not the workflow bundle — directly
contradicting every other site in the codebase that spells `projectDir`).

## Decision Drivers

- **Each concept needs a unique name.** The sandbox plan makes the
  distinction structurally load-bearing: the mount argument must be
  the target repo, and the validator must reject workflow-bundle
  tokens in prompt bodies (because the workflow bundle is not
  mounted). Both concepts have to be addressable by name — the
  ambiguity isn't tolerable.
- **Public API names are versioned surface.** `docs/coding-standards/versioning.md:19`
  enumerates CLI flags, the `{{VAR}}` substitution language, and
  `--version` output as pr9k's public API. A flag rename or a
  new variable token is a breaking change, so it must be decided
  deliberately, not drift.
- **Narrow-reading-principle alignment.** The existing ADR at
  `docs/adr/20260410170952-narrow-reading-principle.md` says pr9k
  should keep the substitution language uniform and phase-only.
  Context-dependent resolution of `{{PROJECT_DIR}}` (different value
  in prompts vs. commands, or different value in sandboxed vs.
  unsandboxed steps) would violate that principle. Two cleanly-named
  tokens with stable meanings do not.
- **No regression for existing users.** The default `ralph-steps.json`
  shipped with pr9k uses `{{PROJECT_DIR}}/ralph-art.txt` at
  `src/ralph-steps.json:3` to reach a file in the workflow
  bundle. Whatever names are chosen, the migration path for that file
  — and any user-authored equivalents — must be documented.

## Considered Options

1. **Keep a single `--project-dir`; document its meaning more
   carefully.** Leave the flag/token name alone, add prose explaining
   that "project-dir" means "workflow bundle," and introduce a separate
   name (e.g. `TargetRepo`) for the target repo — but internally only,
   not surfaced as a flag or variable.

   - Pros: zero CLI churn; no breaking change; no migration; no new ADR
     needed beyond docs updates.
   - Cons: the sandbox mount argument (`-v <?>:/home/agent/workspace`)
     still needs a name, and it cannot be `{{PROJECT_DIR}}` (workflow
     bundle) without perpetuating the ambiguity. Prose cannot fix a
     name that is structurally wrong. And it leaves the target repo
     second-class — no flag, no override, no representation in the
     VarTable — which means `command` steps that want to reach target
     repo files have no clean way to do so other than via cwd, which
     the sandbox's `-w` flag obscures.

2. **Rename `--project-dir` → `--workflow-dir` with no replacement;
   keep the target repo implicit as cwd.** Rename the flag/token to
   match today's semantics. Don't surface the target repo as a flag.

   - Pros: minimal surface change; one clear name for what's there.
   - Cons: target repo remains implicit, yet the sandbox needs to
     bind-mount it. The mount argument in the plan would have to be
     called something ad-hoc (e.g. `<CWD>` or `<TARGET_REPO>`),
     creating a new token that exists only in plan prose and not as a
     first-class user-facing concept. Users still can't override the
     target-repo path for testing or scripting. The asymmetry ("I can
     override where my workflow lives but not which repo I'm running
     against") invites future confusion.

3. **Split into two flags and two variables: `--workflow-dir` +
   `--project-dir`, with `{{WORKFLOW_DIR}}` + `{{PROJECT_DIR}}`.**
   `--workflow-dir` inherits today's semantics (workflow bundle,
   default: executable-path + `EvalSymlinks`). `--project-dir` is
   reintroduced with a new meaning (target repo, default: `os.Getwd()`
   + `EvalSymlinks`). Short forms (`-p`) dropped. No deprecation
   alias for the old `--project-dir`.

   - Pros: each concept has a unique, readable name. The sandbox mount
     argument is a natural `{{PROJECT_DIR}}`. The target repo is
     first-class and overridable. Substitution stays phase-only. No
     in-code `IsClaude`-aware special-casing. The validator rule
     generalizes cleanly (ban both tokens in prompt bodies; allow both
     in `command` argv).
   - Cons: breaking CLI change — scripts passing `-p` or
     `--project-dir <workflow-bundle>` will break loudly at flag-parse
     or silently mount the wrong directory. Requires a MINOR bump
     under the `0.y.z` escape hatch (`0.2.2` → `0.3.0`, bundled with
     the sandbox release). Two tokens instead of one in the
     substitution language.

4. **Keep `--project-dir` as an alias for `--workflow-dir` during a
   deprecation window.** Same as (3), but accept the old flag for one
   release with a deprecation warning before removal.

   - Pros: gentler migration for users who script around pr9k.
   - Cons: **silently dangerous**. Post-split, `--project-dir` means
     target repo. If the old flag is accepted as an alias for
     `--workflow-dir`, a script passing
     `--project-dir /path/to/workflow-bundle` would continue to be
     parsed, but the VarTable would then seed `PROJECT_DIR` to the
     wrong path, and the sandbox would mount the wrong directory. The
     same invocation string means two different things in two
     different versions, with no visible error. A clean loud break is
     strictly safer than a silent alias.

## Decision

Adopt **Option 3**: split into `--workflow-dir` + `--project-dir` with
`{{WORKFLOW_DIR}}` + `{{PROJECT_DIR}}`, delivered in the same release
as the sandbox (`0.3.0`). No deprecation alias. No short forms for
either flag. Rationale:

- Each concept gets a unique, unambiguous name. The sandbox mount
  template and the workflow-resource token can each use the name that
  correctly describes them, with no plan-prose redefinition.
- The target repo becomes a first-class concept — surfaced as a flag,
  seeded into the VarTable, and overridable by the user (default:
  `os.Getwd()`). This is symmetric with how the workflow bundle is
  treated today.
- The substitution language stays phase-only. Neither
  `{{WORKFLOW_DIR}}` nor `{{PROJECT_DIR}}` resolves differently
  depending on where it appears; both are banned in claude prompts
  (because neither host path is valid inside the sandbox container)
  and allowed in `command` argv (which runs on the host). This
  aligns with the narrow-reading-principle ADR.
- The deprecation alias is rejected because the same flag string
  would mean different things before and after. A loud flag-parse
  error is safer than a silent mount redirection.
- Bundling into the sandbox release is acceptable under the `0.y.z`
  escape hatch in the versioning standard — `0.3.0` absorbs multiple
  breaking changes, and the first `1.0.0` release is explicitly
  forbidden from doing the same.

The delivery plan, file-level inventory, versioning rationale, and
follow-up doc updates are recorded in
`docs/plans/docker-sandbox/design.md` §4.15, §9 (Rename subsection),
§11, and §12.

## Consequences

**Positive:**

- Unambiguous names for each concept — readers will no longer have to
  infer which kind of directory `{{PROJECT_DIR}}` refers to.
- The sandbox plan's docker-run template becomes self-describing:
  `-v <PROJECT_DIR>:/home/agent/workspace` means what it says.
- Target repo is overridable, which simplifies testing (tempdirs can
  be passed as `--project-dir`) and future scripting use cases.
- Substitution language stays pure and phase-only — the validator
  rule for the prompt-token ban is one generalized scan pass, not
  two context-dependent rules.

**Negative:**

- Breaking CLI change. Users with scripts passing `-p` or
  `--project-dir <workflow-bundle>` must update. `0.2.2` → `0.3.0`
  carries this break alongside the sandbox env-requirement break.
- Users with custom `ralph-steps.json` files using `{{PROJECT_DIR}}`
  to mean "workflow bundle" must rename those tokens to
  `{{WORKFLOW_DIR}}`. The default shipped workflow has one such site
  (`src/ralph-steps.json:3`) which is migrated as part of the
  implementation.
- Two tokens in the substitution language instead of one. Slightly
  more to learn, but the names are self-describing and the per-token
  semantics are cleaner than a single ambiguous token.

**Neutral:**

- `docs/adr/20260409135303-cobra-cli-framework.md` is amended in place:
  the two `-project-dir` references at lines 12 and 79 are updated to
  the post-split names, and a trailing "Updates" note points at this
  ADR. The cobra decision itself is unchanged — only the flag names
  it mentions.
- Historical plans (`docs/plans/pr9k.md`,
  `docs/plans/cobra-cli-option-parsing.md`,
  `docs/plans/ux-corrections/design.md`) are left untouched; current
  docs, ADRs, and the delivery plan are the source of truth for
  post-split vocabulary.

## Notes

### Key Files

| File | Role |
|------|------|
| `src/internal/cli/args.go` | Flag registration for both `--workflow-dir` and `--project-dir`; `resolveWorkflowDir()` + `resolveProjectDir()` |
| `src/internal/vars/vars.go` | Seeds `WORKFLOW_DIR` and `PROJECT_DIR` persistent-scope variables |
| `src/internal/validator/validator.go` | Prompt-scan pass rejecting both tokens in claude prompts |
| `src/internal/workflow/run.go` | `RunConfig.WorkflowDir`; target repo reaches `BuildRunArgs` via `Runner.ProjectDir()` / `StepExecutor.ProjectDir()` per Option B |
| `src/ralph-steps.json` | Default workflow; line 3's Splash step migrated from `{{PROJECT_DIR}}` to `{{WORKFLOW_DIR}}` |

### Related Docs

- [Sandbox Design Plan](../plans/docker-sandbox/design.md) — delivers
  the split in §4.15, §9 Rename subsection, §11, §12.
- [Cobra CLI Framework ADR](20260409135303-cobra-cli-framework.md) —
  original flag surface; amended in place to reflect the split.
- [Narrow-Reading Principle ADR](20260410170952-narrow-reading-principle.md) —
  motivates keeping substitution phase-only rather than
  context-dependent.
- [Versioning Standard](../coding-standards/versioning.md) — names CLI
  flags and the `{{VAR}}` language as public API; defines the `0.y.z`
  escape hatch this release uses.
