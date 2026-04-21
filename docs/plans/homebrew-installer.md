# Plan: Homebrew Installer for pr9k

## Context

pr9k is currently installed by `git clone` + `make build`, with users invoking the binary via a relative path (`./bin/pr9k`) or a manual PATH symlink. Every release surface (git tag, GitHub Release, cross-compiled binaries, packaging metadata) is absent: no tags on origin, no releases, CI only runs on `ubuntu-latest` and never uploads artifacts. Making pr9k installable via Homebrew removes the Go toolchain from the install prerequisites and gives users `brew upgrade` for version management.

The binary already resolves its sibling workflow bundle through `filepath.EvalSymlinks(os.Executable())` (`src/internal/cli/args.go:23-55`), so Homebrew's standard `bin.install_symlink libexec/"pr9k"` pattern works without application code changes. The `claude` CLI runs inside the `docker/sandbox-templates:claude-code` container, not on the host, so it is not a packaging concern — only Docker itself, `gh`, `jq`, and `git` need host presence.

## Goal

A user on macOS (arm64 or amd64) or Linux (arm64 or amd64) can run:

```bash
brew tap mxriverlynn/pr9k
brew install pr9k
```

…and then `pr9k --version` prints `pr9k version 0.7.1` from their `PATH`. The workflow bundle is discovered automatically; the preflight Docker check surfaces a clear error if Docker is missing.

## Success criteria

1. Pushing a `v*` tag on `main` produces a GitHub Release with four tarballs (`darwin_arm64`, `darwin_amd64`, `linux_arm64`, `linux_amd64`), each containing `pr9k` + `.pr9k/workflow/` with script executable bits preserved, plus a `checksums.txt`.
2. The same tag push commits an updated `Formula/pr9k.rb` to `github.com/mxriverlynn/homebrew-pr9k`, with correct `url` + `sha256` for each platform bottle.
3. `brew install pr9k` from a clean macOS machine produces a working install where `pr9k --version` emits exactly `pr9k version 0.7.1\n` (the format pinned by `docs/coding-standards/versioning.md:20`).
4. `pr9k sandbox create` and `pr9k sandbox login` work post-install (Docker is present; user follows the caveats).

## Scope

### In scope

- Release plumbing in this repo: `.goreleaser.yml`, `.github/workflows/release.yml`.
- README and Getting Started documentation updates so the `brew` path is the primary install story.
- Creation of the external `homebrew-pr9k` tap repo (manual, one-time).
- PAT provisioning for the tap write credential (manual, one-time).
- First `v0.7.1` tag.

### Out of scope

- Application code changes. The `EvalSymlinks` resolver already handles the Homebrew layout.
- Windows support (pr9k depends on POSIX `Setpgid` and `golang.org/x/sys/unix`; out of scope).
- Homebrew Core submission. Blocked by (a) Docker's cask-dep requirement, (b) core's notability thresholds, (c) core's vendored-source compilation rule. A tap is the permanent home.
- `go install` support. The module path is `github.com/mxriverlynn/pr9k/src` and a `go install` would not install the workflow bundle. Documenting `brew` as the only binary-install path avoids this trap.
- Version bump. 0.7.1 is already in `src/internal/version/version.go`; this plan ships 0.7.1 as the first brew-installable release.

---

## Architecture

### Two-repo split

**This repo (`mxriverlynn/pr9k`)** holds source, release automation, and is the release-artifact host.

**New tap repo (`mxriverlynn/homebrew-pr9k`)** holds exactly one file that matters: `Formula/pr9k.rb`. goreleaser overwrites it on every release. The `homebrew-` prefix is mandatory — `brew tap mxriverlynn/pr9k` resolves to `github.com/mxriverlynn/homebrew-pr9k` by Homebrew convention.

### Post-install layout

```
<cellar>/<version>/
├── libexec/
│   ├── pr9k                       # real binary
│   └── .pr9k/
│       └── workflow/
│           ├── config.json
│           ├── ralph-art.txt
│           ├── prompts/           # 9 .md files
│           └── scripts/           # 9 executables (mode 0755)
└── bin/
    └── pr9k -> ../libexec/pr9k    # created by bin.install_symlink
```

At runtime, `filepath.EvalSymlinks(os.Executable())` resolves `/opt/homebrew/bin/pr9k` through Homebrew's link farm to `<cellar>/<version>/libexec/pr9k`. `resolveWorkflowDir` (`src/internal/cli/args.go:26-55`) then finds the bundle at `<cellar>/<version>/libexec/.pr9k/workflow/`.

### Version flow

`version.Version` remains the single source of truth (`docs/coding-standards/versioning.md:5-12` forbids ldflags injection). goreleaser reads the git tag for archive naming and the formula's `version` field but **does not** inject the value into the binary. Bumping a release still follows the existing ritual: edit the const, update `version_test.go`, commit, tag, push. The release workflow takes over from the tag push.

---

## Work items (ordered)

### Phase 1 — external setup (manual, one time)

1. Create the tap repo `mxriverlynn/homebrew-pr9k` on GitHub. Empty is fine. Add an MIT or Apache-2.0 LICENSE and a README stub pointing at the main repo.
2. Create a fine-grained PAT with `Contents: write` scoped to only the `homebrew-pr9k` repo. Add it to `mxriverlynn/pr9k`'s repo secrets as `HOMEBREW_TAP_TOKEN`.

### Phase 2 — release plumbing in this repo (one PR on `homebrew-installer`)

3. Add `.goreleaser.yml` at repo root (see spec below).
4. Add `.github/workflows/release.yml` (see spec below).
5. Update `README.md` — replace the primary install section with `brew tap` / `brew install` and add Docker to the prerequisites list (currently missing, unlike Getting Started).
6. Update `docs/how-to/getting-started.md` — lead with `brew install`, keep `git clone + make build` as a "Build from source" subsection.
7. Optionally add a new `docs/how-to/installing-with-homebrew.md` — follows the existing how-to convention and gives the caveats a permanent home.
8. Update `CLAUDE.md`'s how-to index to list any new how-to file added.

### Phase 3 — ship it

9. Merge the PR to `main`.
10. From a clean `main` checkout, `git tag v0.7.1 && git push origin v0.7.1`.
11. Watch the release workflow. Expected: four archives uploaded to a `v0.7.1` GitHub Release + one commit to `homebrew-pr9k` updating `Formula/pr9k.rb`.
12. Smoke-test `brew tap mxriverlynn/pr9k && brew install pr9k && pr9k --version` on a clean machine (or `brew uninstall` first on your workstation).

---

## File specifications

### `.goreleaser.yml`

```yaml
version: 2

project_name: pr9k

before:
  hooks:
    # Run the bundle integration test so we catch a missing prompt or script
    # before we ship it. Runs against workflow/ (source), not bin/ (build output).
    - bash -c "cd src && go test -tags=integration -race ./cmd/pr9k/..."

builds:
  - id: pr9k
    dir: src
    main: ./cmd/pr9k
    binary: pr9k
    env:
      - CGO_ENABLED=0
    goos:
      - darwin
      - linux
    goarch:
      - amd64
      - arm64
    # IMPORTANT: docs/coding-standards/versioning.md:10 forbids -ldflags "-X ..."
    # version injection. version.Version is the single source of truth.
    # Strip debug info only.
    ldflags:
      - -s -w
    flags:
      - -trimpath

archives:
  - id: pr9k
    formats: [tar.gz]
    name_template: >-
      {{ .ProjectName }}_{{ .Version }}_{{ .Os }}_{{ .Arch }}
    files:
      - src: workflow/config.json
        dst: .pr9k/workflow/config.json
      - src: ralph-art.txt
        dst: .pr9k/workflow/ralph-art.txt
      - src: workflow/prompts/*.md
        dst: .pr9k/workflow/prompts
      - src: workflow/scripts/*
        dst: .pr9k/workflow/scripts
        info:
          mode: 0755
      - LICENSE
      - README.md

checksum:
  name_template: 'checksums.txt'

changelog:
  use: github
  sort: asc
  filters:
    exclude:
      - '^docs:'
      - '^chore:'
      - '^test:'
      - Merge pull request

release:
  github:
    owner: mxriverlynn
    name: pr9k
  draft: false
  prerelease: auto

brews:
  - name: pr9k
    repository:
      owner: mxriverlynn
      name: homebrew-pr9k
      branch: main
      token: "{{ .Env.HOMEBREW_TAP_TOKEN }}"
    directory: Formula
    homepage: "https://github.com/mxriverlynn/pr9k"
    description: "Automated development workflow orchestrator for the claude CLI"
    license: "Apache-2.0"
    commit_author:
      name: pr9k-release-bot
      email: noreply@github.com
    commit_msg_template: "pr9k {{ .Tag }}"
    dependencies:
      - name: gh
      - name: jq
      - name: git
    install: |
      libexec.install Dir["*"] - ["LICENSE", "README.md"]
      doc.install "README.md"
      bin.install_symlink libexec/"pr9k"
    test: |
      assert_match "pr9k version #{version}", shell_output("#{bin}/pr9k --version")
    caveats: |
      pr9k requires Docker to run claude steps inside a sandbox.

      Install Docker:
        brew install --cask docker       (macOS — Docker Desktop)
        https://docs.docker.com/engine/install/   (Linux — Docker Engine)

      Then initialize the sandbox image and authenticate the claude profile:
        pr9k sandbox create
        pr9k sandbox login

      See https://github.com/mxriverlynn/pr9k#getting-started for a walk-through.
```

**Notes:**

- `brews:` is a legacy (deprecated-but-supported) goreleaser block. If goreleaser drops it before we ship, migrate to the equivalent `homebrew_casks:` (note: despite the name, casks can generate formulae for CLIs; check goreleaser docs at implementation time).
- `builds[].dir: src` handles the module-path quirk (module lives under `src/`, not repo root).
- `archives[].files[].info.mode: 0755` force-sets executable bits on scripts, belt-and-braces on top of git's stored mode bits. `.tar.gz` preserves mode; `.zip` does not — don't change the format.
- `before.hooks` runs the existing `bundle_integration_test.go` so goreleaser refuses to cut a release with a missing prompt or script. The test is tagged `integration` and is otherwise gated out of normal `go test ./...` runs.
- `-trimpath` strips local filesystem paths from the binary for reproducibility. Doesn't affect version handling.
- `install:` uses `libexec.install Dir["*"] - ["LICENSE", "README.md"]` so everything in the tarball except docs lands in libexec. The docs go to `doc/` where Homebrew can surface them. If this proves fragile, switch to explicit `libexec.install "pr9k"; libexec.install ".pr9k"`.

### `.github/workflows/release.yml`

```yaml
name: Release

on:
  push:
    tags:
      - 'v*'

permissions:
  contents: write

jobs:
  release:
    name: Release
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v4
        with:
          fetch-depth: 0

      - uses: actions/setup-go@v5
        with:
          go-version: "1.26.2"
          cache-dependency-path: src/go.sum

      - name: Verify version constant matches tag
        run: |
          tag="${GITHUB_REF_NAME#v}"
          constant=$(grep -E '^const Version' src/internal/version/version.go | awk -F'"' '{print $2}')
          if [ "$tag" != "$constant" ]; then
            echo "tag ($tag) does not match version.Version ($constant)" >&2
            exit 1
          fi

      - name: Run full CI suite
        run: make ci

      - name: Run bundle integration test
        working-directory: src
        run: go test -tags=integration -race ./cmd/pr9k/...

      - uses: goreleaser/goreleaser-action@v6
        with:
          distribution: goreleaser
          version: "~> v2"
          args: release --clean
        env:
          GITHUB_TOKEN: ${{ secrets.GITHUB_TOKEN }}
          HOMEBREW_TAP_TOKEN: ${{ secrets.HOMEBREW_TAP_TOKEN }}
```

**Notes:**

- The "Verify version constant matches tag" step catches the most common release mistake: tagging `v0.8.0` without bumping `version.Version`. Fail early, before any artifact is produced.
- `make ci` re-runs the full CI suite (test, lint, format, vet, vulncheck, mod-tidy, build). Yes, it already ran on the merge to `main`, but tagging is a separate user action and deserves a re-check — cheap insurance.
- `permissions: contents: write` is required for goreleaser to create the GitHub Release. The `HOMEBREW_TAP_TOKEN` is a separate PAT because `GITHUB_TOKEN` cannot write cross-repo.

### `README.md` updates

Replace the current "Installation" section (lines 11-26) with:

```markdown
### Prerequisites

- [Docker](https://docs.docker.com/get-docker/) — Docker Desktop (macOS) or Docker Engine (Linux), running. pr9k runs every claude step inside a Docker sandbox
- [GitHub CLI (`gh`)](https://cli.github.com/) — authenticated against your target repo (installed automatically by Homebrew)
- [Claude CLI (`claude`)](https://docs.anthropic.com/en/docs/claude-cli) credentials — pr9k uses your `~/.claude` profile inside the sandbox container
- A GitHub repo with issues labeled `ralph` assigned to your user

### Installation

```bash
brew tap mxriverlynn/pr9k
brew install pr9k

pr9k sandbox create   # pull the claude sandbox image
pr9k sandbox login    # authenticate the claude profile
```

### Building from source

Requires [Go 1.26.2](https://go.dev/dl/).

```bash
git clone https://github.com/mxriverlynn/pr9k.git
cd pr9k
make build
./bin/pr9k
```
```

### `docs/how-to/getting-started.md` updates

Same shape: lead with `brew install`, demote `git clone + make build` to "Build from source". Keep the rest of the doc (prerequisites for Docker, `gh`, `claude`, target-repo setup) unchanged.

### `docs/how-to/installing-with-homebrew.md` (new, optional)

A short how-to covering:

- The two-step `brew tap` + `brew install`.
- Why the caveats ask you to install Docker separately (Homebrew formulae cannot depend on casks).
- How to upgrade (`brew upgrade pr9k`).
- How to uninstall cleanly (`brew uninstall pr9k`, plus a note about the `~/.claude` profile and target-repo `.pr9k/` state not being touched).
- Where the bundle lives (`$(brew --prefix)/opt/pr9k/libexec/.pr9k/workflow/`) for anyone debugging a workflow override.

If added, append a line to `CLAUDE.md`'s how-to section and `docs/coding-standards/documentation.md` is already explicit that new doc files must be indexed.

---

## Prerequisites checklist (external, one-time)

- [ ] `mxriverlynn/homebrew-pr9k` repo exists on GitHub, public, with LICENSE + README stub.
- [ ] Fine-grained PAT created with `Contents: write` on only `homebrew-pr9k`.
- [ ] PAT stored as `HOMEBREW_TAP_TOKEN` secret on `mxriverlynn/pr9k`.

---

## Risks & mitigations

| Risk | Mitigation |
|---|---|
| goreleaser's `brews:` deprecation lands before we ship | Check goreleaser release notes at implementation time; migrate to the replacement block if needed. Behavior is equivalent. |
| Tarball executable bits get lost on extraction | `archives[].files[].info.mode: 0755` force-sets them; `.tar.gz` preserves them; the formula's `test do` won't catch mode regressions but the preflight on first invocation of a workflow script would. Consider adding a test-mode assertion that `libexec/.pr9k/workflow/scripts/get_gh_user` is executable — cheap. |
| Tag is pushed with a `version.Version` mismatch | The "Verify version constant matches tag" step in `release.yml` fails the release early. |
| `HOMEBREW_TAP_TOKEN` expires silently | Fine-grained PATs have expirations; calendar a reminder. goreleaser will fail loudly with a 401/403 and the formula won't update — the GitHub Release still succeeds, so users can still download tarballs manually. |
| User installs pr9k but not Docker | The preflight check at startup (`src/internal/preflight/docker.go:57-74`) emits a clear error. Caveats in the formula are the primary signal at install time. |
| goreleaser archive layout drifts from what `resolveWorkflowDir` expects | The bundle integration test runs in `release.yml` against `workflow/` (source), which is what the archive is built from. A post-release validation would extract one tarball and run the same assertions — worth adding as a follow-up if the risk surfaces. |
| `homebrew/core` adds a `pr9k` in the future | Users would need to use the fully qualified `mxriverlynn/pr9k/pr9k` to disambiguate. Unlikely collision, but worth being aware of. |

---

## Open questions

1. **Tap repo name.** `homebrew-pr9k` is the obvious choice. An alternative is `homebrew-tools` (if you ever want to ship more than one formula from one tap). Pick one before Phase 1.
2. **Should `make ci` re-run in the release workflow, or trust the pre-merge CI?** Default plan runs it; remove if release cycle time becomes an issue.
3. **Docker Desktop via `brew install --cask docker`** or **Colima**? The caveats currently point at Docker Desktop. Colima is a common alternative on Apple Silicon for license-averse users. Consider mentioning both.
4. **Linuxbrew support.** The plan assumes Homebrew on Linux works identically (it does, mostly). Formal smoke-test on a Linux runner is worth doing before announcing the tap.
5. **Auto-update cadence.** goreleaser publishes the formula on every tag. If we start cutting pre-release tags (`v0.8.0-rc1`), `prerelease: auto` keeps them out of the formula — but users on `brew install --HEAD` would still pull tip of `main`. Decide whether to support `head do` in the formula; default is no.

---

## Summary

Four new files in this repo (`.goreleaser.yml`, `.github/workflows/release.yml`, README edits, Getting Started edits), one new repo (`homebrew-pr9k`), one PAT, one tag, one merge. No application code changes. The existing `EvalSymlinks`-based workflow resolver, the coding-standard ban on ldflags version injection, and the bundle integration test all map cleanly onto the Homebrew + goreleaser conventions without friction.
