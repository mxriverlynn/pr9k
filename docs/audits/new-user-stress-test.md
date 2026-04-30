# New-User Stress Test: User-Facing Documentation Walkthrough

This audit follows the path a new user would take through the user-facing docs, reading them in the order presented in the `how-to/` index. The reviewer is a generalist with three to five years of experience who has never heard of pr9k or Ralph and is trying to use it on their own GitHub repo.

For each doc, findings are grouped under:

- **Assumed knowledge** — terms or concepts used before being introduced
- **Missing prerequisites** — things the doc requires done that aren't linked or stated
- **Confusing ordering** — material out of teaching order
- **Gaps for the new user** — things the reader will have to guess, search, or trial-and-error
- **Crosslink gaps** — places where a forward or back link would unblock the reader
- **Ambiguous instructions** — commands, paths, or steps that could be misread

A "Top open questions for a new user" section closes the audit.

---

## 1. `README.md`

### Assumed knowledge

- **"Ralph"** appears in the very first sentence ("issues labeled 'ralph'") with no explanation. The repo is named pr9k / Power-Ralph.9000, but a brand-new reader does not know that "ralph" is a label string they need to create themselves on their issues. The blog link to AI Hero is offered as backstory, but reading an external blog post should not be a prerequisite to making sense of the README.
- **"the next open issue"** — implies there is some ordering rule. The README does not say "lowest issue number" until much further down (and the rule is buried in `scripts/get_next_issue`'s behavior). A new user does not know whether issues will be picked in created-at order, label order, or numeric order.
- **"all unattended"** — sets an expectation of zero-touch operation, but later docs describe explicit human-in-the-loop pause points (Error mode, quit confirmation, Done mode that does not auto-exit). The README does not mention this tension.
- **"the `claude` CLI"** — Prerequisites link to `https://docs.anthropic.com/en/docs/claude-cli` but never explain that the CLI must be authenticated and that on macOS the credentials live in Keychain, not on disk. The reader will not know they have a problem until step 3 of the Docker sandbox doc.

### Missing prerequisites

- **Docker is not in the README's Prerequisites list.** The README says only Go, gh, and the claude CLI. The Getting Started doc lists Docker as a hard requirement, and the architecture doc says every Claude step runs in a sandbox. A user who installs only what the README asks for will fail at the first claude step.
- **No mention of `jq`.** The default workflow's `post_issue_summary` script and the iteration-log debugging queries require `jq`. A user with no `jq` installed will hit a bare shell error mid-run.
- **No mention of needing a `ralph` label on the GitHub repo.** The Prerequisites bullet says "A GitHub repo with issues labeled `ralph` assigned to your user", but does not say "you have to create this label first if it doesn't exist".
- **No mention of `.gitignore` setup.** Logs land under `<projectDir>/.pr9k/`. The README does not mention adding `.pr9k/` (or `.ralph-cache/`) to `.gitignore`. The Getting Started doc covers it but the README is the first thing a user reads.

### Confusing ordering

- **"Quick Start" tells you to run `path/to/pr9k/bin/pr9k` immediately**, but the Docker sandbox setup (which is a hard prerequisite) is not described until the Getting Started doc, which the reader may not click through to. The Quick Start should at minimum link to `setting-up-docker-sandbox.md` before showing the run command.
- **The `make build` step does not say what to do if `make` is not installed.** A new user on a fresh macOS install with only Xcode CLT may not have GNU make conveniences working. The README's only fallback advice is "Or build directly: cd src && go build" buried in the "Run the orchestrator" section.

### Gaps for the new user

- **What happens on a repo with zero `ralph`-labeled issues?** The README does not say. The user will run pr9k, watch nothing happen, and have to guess.
- **How do I stop pr9k once it's running?** "Keyboard controls" is in the README but a brand-new reader skimming for the install steps may miss it.
- **What is the exit code on success?** Not mentioned in the README. The `Quitting Gracefully` doc has the table.
- **What is the cost?** The completion summary mentions a per-step `$cost` line, but the README does not warn that each iteration spends money against an Anthropic account.

### Crosslink gaps

- The README's bullet list links to many docs but does not link to **`setting-up-docker-sandbox.md`** from the Quick Start section — only from the documentation list at the bottom. A user who jumps straight to Quick Start will miss it.
- No link to **`debugging-a-run.md`** from the Quick Start / How To section — the user who hits a snag during their first run will not know where the log lives.

### Ambiguous instructions

- `git clone https://github.com/mxriverlynn/pr9k.git` followed by `cd pr9k` then `make build`. Compare this with the Getting Started doc, which says `cd src` (not `cd pr9k`). The two top-level guides disagree about which directory `make build` runs in. A new user following the README will see "make: *** No targets" or similar; following Getting Started they will see the build succeed (assuming `Makefile` is in `src/` — needs verification).
- `path/to/pr9k/bin/pr9k` — placeholder uses a forward slash and unspecified absolute/relative form. New user is left to guess whether they should `cd` into the install dir, symlink the binary into `PATH`, or use absolute paths.

---

## 2. `docs/how-to/getting-started.md`

### Assumed knowledge

- **`os.Executable()`, `filepath.EvalSymlinks`** — appears in the `go run` warning. A non-Go reader does not need this level of detail to understand "use `go build`, not `go run`".
- **`ioctl TIOCGWINSZ`** — appears in the prerequisites box. This is gratuitous detail for a getting-started doc; "macOS and Linux only, no Windows" is the actionable fact.
- **"workflow bundle"** — used before being defined. The reader does not know what's in a bundle until later in the same doc.
- **"phase banners"**, **"per-step banners"**, **"capture logs"** — referenced in the TUI region 2 bullet but not defined here. The user has to click through to `reading-the-tui.md` to learn what they look like.
- **"Splash step"**, **"ralph-art.txt"** — mentioned in the build output box. A new user does not know these names yet.

### Missing prerequisites

- **`gh auth status` is suggested as a verification step** but the doc does not mention that the GitHub CLI must be authenticated against the same account that owns the issues. A user with a corporate `gh auth` and a personal repo will silently fail.
- **The doc does not explain that `claude --version` is **not** sufficient** — the credentials need to be bind-mountable into a container. The macOS Keychain trap is only mentioned in the Docker sandbox doc.

### Confusing ordering

- **Build-output tree is shown before `make build` is run.** The doc says "`make build` produces:" and then shows the tree. Reasonable, but the tree includes filenames (`ralph-art.txt`, `prompts/`, `scripts/`) that have no meaning to a first-time reader.
- **`--workflow-dir` and `--project-dir` are introduced in the same section as `make build`**, before the user has run pr9k once and could make sense of why they would need to override either flag. The "in-repo override" pattern is interesting but is the wrong thing to introduce in a getting-started doc.
- **`.gitignore` step is buried under "First run against the default workflow"** rather than treated as a preflight. A reader who skims will miss it.

### Gaps for the new user

- **No example of what the very first run looks like.** There is a screenshot reference in the README, but Getting Started shows only the chrome rhythm in a code block — no walkthrough of "you should see X, then Y, then Z, this is normal".
- **No "things that go wrong on the first run" section.** Common first-run failures (Docker not running, no `ralph` label, no issues assigned, claude not authenticated) are split across the Docker sandbox doc, the debugging doc, and the validator's structured errors.
- **The doc does not say where `progress.txt`, `deferred.txt`, `test-plan.md`, `code-review.md` live or whether the user should clean them up.** These files appear in the working directory as intermediate state.
- **The doc does not say how long a run takes.** A user who runs the default workflow and does not see anything for two minutes does not know whether to wait or to suspect a hang.

### Crosslink gaps

- The doc links to `reading-the-tui.md#selecting-log-text-to-copy` from a prereq aside, but does not link to the **chrome-rhythm explanation** from the "What the TUI shows" section, even though that section's diagram only makes sense if you already understand the rhythm.
- No link to **`caching-build-artifacts.md`** even though that doc says the default workflow already includes Go cache settings — a Go user reading Getting Started for the first time would benefit from a forward link.

### Ambiguous instructions

- **`cd src` vs `cd pr9k`** disagreement with the README (already noted above).
- `/path/to/pr9k/bin/pr9k --workflow-dir /path/to/pr9k/bin/.pr9k/workflow` — the second path is `bin/.pr9k/workflow`, but the doc has been calling that directory `bin/.pr9k/` containing `workflow/`. Both paths describe the same location, but the inconsistency between `bin/.pr9k/workflow/` and `bin/` makes the user re-read.
- "`-v` is accepted as a short alias" — short alias for `--version`, but a Bubble Tea user might assume `-v` is verbose. State both meanings explicitly.

---

## 3. `docs/how-to/setting-up-docker-sandbox.md`

### Assumed knowledge

- **`UID:GID`** in `Sandbox verified: claude 2.1.101 under UID 501:20` — fine for someone who knows Unix file permissions, but the doc never says why this matters or what to do if their UID is different.
- **`OAuth flow`**, **`/login`** — assumes the user knows the Claude REPL has slash commands and that `/login` is the canonical authentication command.
- **`bracketed-paste sequences`** — used in the manual-docker-run note about `-e TERM`. Generalist will not know what bracketed-paste is and will not understand why they are being warned.
- **`ANTHROPIC_API_KEY`** is mentioned as an alternative to OAuth, but the doc does not say where to obtain one or whether it has different cost/rate-limit semantics.
- **`BuiltinEnvAllowlist`** — leaked implementation detail in the "ANTHROPIC_API_KEY" alternative. Reader does not need to know the name of the Go variable.
- **`SIGKILL mid-OAuth-refresh`** — describes a failure mode but not how to avoid causing it.

### Missing prerequisites

- **The doc does not say which Docker version is required.** "Docker Desktop / Docker Engine" — minimum supported version unstated.
- **The doc does not say how much disk space the sandbox image needs**, which is relevant for a CI machine or a small VPS.
- **It does not explain whether the daemon must remain running for the whole pr9k run** (yes, it does, since each step starts a new container).

### Confusing ordering

- **Step 3 (Authenticate the Claude profile) comes after Step 2 (Pull the image),** but the smoke test inside `sandbox create` runs `claude --version` inside the container. The reader does not understand whether the smoke test would have already failed if their profile is unauthenticated. Spoiler: `--version` does not require auth, but the doc does not say so.
- **`sandbox create --force` is explained before the basic auth flow finishes.** A first-time reader trying to authenticate is briefly distracted by "to update the image to the latest upstream tag".
- **The "Debugging fallback: manual `docker run`" block appears before the user has even succeeded with the recommended path.** It also dumps a 7-line `docker run` invocation that a reader who is debugging probably should not be staring at as their second exposure to the sandbox.

### Gaps for the new user

- **What does "the OAuth flow in your browser" look like?** The doc says to type `/login` and complete the flow, but does not warn that the browser may need to be on the same machine, or that the OAuth code has to be pasted back into the REPL.
- **What does "exit" mean inside the REPL?** The doc says `/exit`. A user who types `Ctrl+D` or closes the terminal may interrupt the credentials write and trigger the "empty credentials file" warning later.
- **The doc does not say how to log out / switch profiles** after the first authentication. `CLAUDE_CONFIG_DIR` is described, but rotating credentials inside an existing profile dir is not.

### Crosslink gaps

- No link to **`passing-environment-variables.md`** for users who want to set `ANTHROPIC_API_KEY` long-term — only to the "Passing Environment Variables" doc at the very bottom.
- The "Files written by claude are owned by root" troubleshooting tip says to `--force` re-pull, but does not link to **`caching-build-artifacts.md`** which has a more detailed UID/GID discussion.

### Ambiguous instructions

- **`docker run -it --rm --init -u $(id -u):$(id -g) ...`** — the command is split across multiple lines with backslash continuations. New users on macOS may copy-paste this and have shell quoting fail (especially if they have a non-default `IFS`).
- The expected output shows `Sandbox verified: claude 2.1.101 under UID 501:20.` — the version `2.1.101` is hard-coded. A new user who sees a different version may wonder if their install is broken.

---

## 4. `docs/how-to/reading-the-tui.md`

### Assumed knowledge

- **`Model.View()`**, **`HeaderProxy`**, **`program.Send`**, **`SetPhaseSteps`**, **`noopHeader`**, **`tea.SetWindowTitle`**, **`tea.WithMouseCellMotion()`**, **`bubbles/viewport`** — implementation names leaked into a user-facing doc. A user trying to understand the TUI does not need any of these.
- **`Lip Gloss layout`** — referenced when describing the footer.
- **`StatusHeader struct`** — leaked into the related docs section.
- **`captureAs`**, **`breakLoopIfEmpty`**, **`onTimeout: "continue"`**, **`timeoutSeconds`** — all referenced before a forward link appears. Reader will see `[!]` "Timed-out, continuing" and not know where to learn more without scrolling.
- **`stream-json event`** — used in the heartbeat indicator section without definition.
- **`OSC 52`** — three references in this doc alone. Defined briefly later, but a reader who hits the term first time will not know what it is.

### Missing prerequisites

- **No prerequisites listed.** The doc does not say "this assumes you already have a run going" — a reader who lands on this page first will be confused by the diagram.
- **Mouse selection:** the doc does not say which terminals support `tea.WithMouseCellMotion()` — Apple Terminal, iTerm2, Alacritty, Kitty all behave slightly differently.

### Confusing ordering

- **The chrome rhythm (Region 2) is the most important part of the TUI for understanding what's happening**, but it is buried below the very long checkbox-grid table. The new user wants to learn "what does the screen mean" before "what color the title is rendered in".
- **The status-line footer path is described before the user has been told what a `statusLine` block is.** A forward link to `configuring-a-status-line.md` is given but the section assumes context the reader does not have.
- **Select mode is documented in this doc and again in `copying-log-text.md`.** The two cover the same keybindings with slight differences in framing — confusing for a reader following the index in order.

### Gaps for the new user

- **The doc lists six checkbox states (`[ ]`, `[▸]`, `[✓]`, `[✗]`, `[-]`, `[!]`)** but does not explain that `[!]` is rare and tied to `onTimeout: "continue"`. A user who sees `[!]` for the first time will spend time hunting.
- **"The completion summary is not in the header, it's the last line of the log panel"** — a load-bearing fact buried in a parenthetical. Worth its own note box.
- **"Auto-scrolls one line per event" while drag-selecting** — does not say what an "event" is in this context. A user dragging fast vs slow may see different behavior.

### Crosslink gaps

- No back link to **`getting-started.md`** from the chrome-rhythm section — a reader who clicked the deep link from Getting Started loses their place.
- No forward link to **`recovering-from-step-failures.md`** from the `[✗]` checkbox row.
- No forward link to **`setting-step-timeouts.md`** from the `[!]` checkbox row.

### Ambiguous instructions

- **"Hold Option on macOS or Shift on Linux/Windows"** — the doc lists this twice, in two different sections, with no acknowledgment that it is a duplication. Reader may wonder if the two notes describe different scenarios.
- **"OSC 52 escape sequence" sent to "stderr"** — a user who runs pr9k with `2>somefile` will be surprised that copying does not work. The doc does not warn about that.

---

## 5. `docs/how-to/recovering-from-step-failures.md`

### Assumed knowledge

- **`KeyHandler.SetMode(ModeError)`**, **`runStepWithErrorHandling`**, **`<-h.Actions`**, **`buildStep`**, **`Orchestrate`**, **`Runner.Terminate`**, **`WasTerminated()`**, **`ActionContinue`**, **`onTimeout: "continue"`**, **`StepTimedOutContinuing`** — all leaked implementation names in a how-to doc.
- **`SIGTERM`, `SIGKILL`** — appear without explanation. Generalist will know roughly what these are, but the doc could say "the subprocess gets a 3-second window to clean up before being force-killed".
- **`STARTING_SHA`, `ISSUE_ID`** — referenced before the user has read `variable-output-and-injection.md`. A user reading docs in the index order will hit these here first.
- **"breakLoopIfEmpty"**, **"`promptFile`"** — referenced without definition.

### Missing prerequisites

- The doc does not say "you should have already read `reading-the-tui.md`". The mode names (`ModeError`, `ModeQuitConfirm`, `ModeNextConfirm`) only make sense in the TUI context.

### Confusing ordering

- **The "edge cases" section discusses `buildStep` failures** — a much rarer scenario than transient retries — before the basic three-choice (`c`/`r`/`q`) decision is fully practiced. The decision tree at the end repeats material from the top.

### Gaps for the new user

- **What is a "transient" failure in this context?** The decision tree says "rate limit, file-lock race" — the user does not know whether a `gh` 502 counts as transient.
- **What does "downstream steps can handle the failed state" mean concretely?** No example of a downstream step that handles failure gracefully.
- **No example of what happens to `git push` if `Feature work` had nothing to commit** — the doc says "the failure is expected/benign" but does not show the log output.

### Crosslink gaps

- No link to **`debugging-a-run.md`** from the "after-the-fact debugging" line in the timeout section.
- No link to **`setting-step-timeouts.md`** from the first mention of `onTimeout: "continue"` (the link is at the end).

### Ambiguous instructions

- "**Use `r` when:** ... You just fixed something out-of-band (edited a file, rebooted a service)" — does not say whether the file edit will be picked up. A user who edits a script in the workflow dir between attempts may or may not see the new content depending on when the script is re-read.

---

## 6. `docs/how-to/quitting-gracefully.md`

### Assumed knowledge

- **`KeyHandler.handleQuitConfirm`**, **`ForceQuit()`**, **`tea.QuitMsg`**, **`program.Run()`**, **`signal.Stop`**, **`os.Exit`**, **`Runner.Terminate`**, **`Runner.SetSender`** — extensive implementation leakage.
- **`signaled` one-shot channel** — internal name.
- **`drain channel`**, **`drain point`**, **`<-h.Actions`** — concurrency vocabulary that a generalist may not know.
- **`Bubble Tea program`** — referenced without saying it's the Go TUI library.

### Missing prerequisites

- The doc does not state that the reader should have completed `getting-started.md` and `reading-the-tui.md` first. The mode-name table is the same one in those docs.

### Confusing ordering

- **The exit-code table comes after the signal-path narrative** but the most user-actionable fact (`SIGINT exits 1, q→y exits 0`) belongs higher. A user scripting pr9k for CI will scan for "exit code" first.
- **The "Not every keypress quits" section is at the bottom**, but it should be near the top — it pre-empts the most common new-user confusion ("I pressed `n`, why did it not quit?").

### Gaps for the new user

- **No timeline for "how long does shutdown take"?** "Up to 3 seconds" is mentioned for SIGKILL but the doc does not say "expect a 1–3 second pause after pressing y".
- **What if the user pressed `q` → `y` in error mode but the subprocess was already in mid-write to a handoff file?** The doc does not say whether `progress.txt` may be left half-written.

### Crosslink gaps

- No link to **`debugging-a-run.md`** for "after a quit, what's left in the log file?".
- No link to **`recovering-from-step-failures.md`** from the "`Esc` only cancels a quit confirmation" note (the user reading this doc may have just discovered Escape does nothing in normal mode).

### Ambiguous instructions

- **The diagram of the `q` flow** is ASCII art with arrows, but the alternative path "n or Esc → prev mode (Normal or Error)" is a single arrow. A reader may not understand that *which* mode it returns to depends on how they got into QuitConfirm.

---

## 7. `docs/how-to/debugging-a-run.md`

### Assumed knowledge

- **`logger.Log()`, `RunStamp`, `claudestream.Pipeline`, `result.result`, `is_error`, `ralph_end` sentinel** — implementation names.
- **`stream-json` NDJSON format** — used heavily in the JSONL artifacts section without a definition.
- **`@filename` syntax** — referenced as the way Claude steps read handoff files, but never defined here. The user has to flip to `variable-output-and-injection.md`.
- **`workflow.Run`**, **`ui.Orchestrate`**, **`buildStep`**, **`scripts/get_next_issue`**, **`scripts/get_gh_user`** — names a brand-new reader has not yet seen.
- **`captureAs`, `captureMode`, `breakLoopIfEmpty`** — referenced before the dedicated docs.
- **`jq` queries** with `select(.type == "result")` — the reader is expected to know NDJSON shape and `jq` filter syntax.

### Missing prerequisites

- **`jq` is required** for many of the example queries. The doc has a Prerequisites note for this, but it is buried inside the "Iteration log" section. Should be at the top of the file.
- **`awk`, `grep`, `tail`, `less`** — assumed available on the user's system.
- **The `claude -p --output-format stream-json --verbose` invocation shape** is assumed knowledge.

### Confusing ordering

- **The four sources of evidence table is the right starting frame**, but then the doc dives into `.jsonl` artifact internals before the user has even looked at the human-readable `.log` file. A reader debugging their first failure wants `less .pr9k/logs/*.log`, not `jq` filters.
- **"Reproducing a single iteration" is at the bottom**, but it is the most actionable section. Should be promoted.
- **"Validator errors before a run" is at the bottom**, but for a new user, validator errors are the most likely first-time failure mode.

### Gaps for the new user

- **`<runstamp>` is used as a placeholder** without a clear example of what it looks like (`2026-04-14-173022.123`). The reader has to infer from the example filenames.
- **No description of what to do if there is no `.pr9k/logs/` directory at all** — meaning pr9k crashed before logger init.
- **The `jq -s '[.[].input_tokens // 0] | add'` query** is given without explaining what it does.
- **No mention of where `code-review.md`, `test-plan.md`, `progress.txt`, `deferred.txt`** are at debug time — these are mentioned in passing, but a user who finds an orphan `code-review.md` does not know whether to open it, delete it, or ignore it.

### Crosslink gaps

- No link to **`copying-log-text.md`** for "I want to grab the failing output and paste it into a bug report".
- No link to **`reading-the-tui.md`** from the "TUI log panel" row of the evidence table — the row says "scrollable" but doesn't say where to learn the keys.

### Ambiguous instructions

- **`awk '/── Iteration 3 ─/,/── Iteration 4 ─|^Finalizing$/'`** — the awk regex uses Unicode box-drawing characters. The doc does not say "you may need to make sure your terminal/awk handles UTF-8 correctly".
- **`tail -1 ... | jq .type`** — `jq .type` without quotes is shell-fragile; some shells will glob `.type`.

---

## 8. `docs/how-to/building-custom-workflows.md`

### Assumed knowledge

- **`captureAs`, `captureMode`, `breakLoopIfEmpty`, `skipIfCaptureEmpty`, `timeoutSeconds`, `resumePrevious`, `env`, `containerEnv`, `statusLine`** — every advanced field is enumerated in the schema table before any one of them has been taught. The reader has to forward-link out to seven other docs from a single table.
- **"five runtime gates (G1–G5)"** — referenced in the `resumePrevious` row without explanation.
- **`isClaude: true` claude steps**, **the `claude` CLI being invoked with `--permission-mode bypassPermissions --model <model> -p <prompt-content>`** — fine to share, but `bypassPermissions` is a security-relevant flag and the doc does not flag the implication.
- **`@progress.txt` syntax** — used in a code example before the @-syntax is explained.
- **`{{{{.title}}}}` Go-template / gh-template escape rule** — the doc shows the trick in an example but the explanation is offloaded to `variable-output-and-injection.md`.

### Missing prerequisites

- **No statement that the user must have `make build` succeeded first** — modifying `config.json` and prompts in `bin/.pr9k/workflow/` and not running `make build` will work; modifying them in source and not building won't be picked up.
- **No statement that the in-repo override (`<projectDir>/.pr9k/workflow/`) is the recommended path for custom work** — it is described, but the doc treats it as a footnote rather than the default best practice.

### Confusing ordering

- **The full schema table is shown before any explanation of what makes a step a step.** A user new to JSON config formats wants to see one minimal valid step, then see the optional fields. The current ordering is reference-first.
- **The default workflow's 11 iteration steps and 7 finalization steps** are listed before "Creating a Custom Workflow". A new user wanting to write their own tiny workflow will have to scroll past the entire default before reaching the example.
- **`finalize` is described twice** — once in the "Initialize, Iteration, and Finalization Steps" section, then again in the example default workflow's finalization table.

### Gaps for the new user

- **Where do I put my prompt files?** "Add markdown files to the `prompts/` directory" — but the doc does not say *which* `prompts/` directory (workflow bundle's? in-repo override's? both?).
- **Can I use any model name with `model`?** The schema says `"sonnet"` or `"opus"`, but pr9k passes the value to claude verbatim — what about `haiku`, `claude-3.5-sonnet-20241022`?
- **What is the minimum viable initialize phase?** The doc shows `"initialize": []` — empty is allowed, but does the workflow still run? What about `"finalize": []`?
- **No example of a workflow with no GitHub interaction at all** — pr9k claims to be a generic step runner per the narrow-reading ADR, but the docs all assume GitHub-issue-driven workflows.
- **What goes in `scripts/`?** "Place scripts in the `scripts/` directory" — the doc does not say they have to be executable, what shebang, what platform, or what env they inherit.

### Crosslink gaps

- No link to **`setting-up-docker-sandbox.md`** for "your custom Claude steps will run in the sandbox; here's the layout".
- No link to **`debugging-a-run.md`** for "your custom workflow doesn't behave as expected".
- The `containerEnv` field is mentioned in the env field row but **`caching-build-artifacts.md`** is not linked from this doc at all (it appears only in `passing-environment-variables.md`).

### Ambiguous instructions

- **"Relative paths containing a `/` separator are resolved against the workflow directory. Bare commands (like `git`) are looked up via `PATH`."** — what about a relative path on Windows / with backslash? (Moot since Windows isn't supported, but the doc never says so here.)
- **"After modifying configs or prompts, rebuild with `make build` to copy everything into `bin/`."** — confusing if the user is using the in-repo override (`<projectDir>/.pr9k/workflow/`), where `make build` does nothing.

---

## 9. `docs/how-to/variable-output-and-injection.md`

### Assumed knowledge

- **`vars.Substitute`, `steps.BuildPrompt()`, `workflow.ResolveCommand()`, `VarTable`, `CaptureOutput()`, `Runner.CaptureOutput`** — implementation names leaking into a user doc.
- **`os.Executable() + filepath.EvalSymlinks`, `os.Getwd() + filepath.EvalSymlinks`** — implementation detail in the {{WORKFLOW_DIR}}/{{PROJECT_DIR}} table.
- **"Rule B" of the validator** — referenced in the sandbox constraint without explanation.
- **`fullStdout`, `lastLine`, `captureMode`** — used in the precomputed-context-variables example before `capturing-step-output.md` has introduced them.
- **"ralph's escape rule"** — the `{{{{.title}}}}` example is opaque without scrolling down to the Escape Sequences subsection. Reader hits the example before the rule.

### Missing prerequisites

- **The doc does not say "you should read `building-custom-workflows.md` first"** even though the substitution engine is meaningless without a step config to substitute into.

### Confusing ordering

- **The "Sandbox constraint" callout is right after the built-in variable table**, but before any example of using the variables. The reader sees a warning before they understand what the warning is preventing.
- **The 0.3.0 split section appears after the basic substitution examples**, but for a new user the question is "what do these tokens mean today?" — the historical split is noise.
- **Metadata Capture and File-Based Data Passing are interleaved with substitution** — three different topics in one doc. The new user has to keep three mental models open at once.

### Gaps for the new user

- **What happens if I have a `{{` in actual prompt content?** The Escape Sequences subsection has the answer but it's a one-line note.
- **What is the maximum length of a captured value?** The doc mentions a 32 KiB cap for `fullStdout` only in `capturing-step-output.md` — not here.
- **What does "logs a warning" look like for an unresolved variable?** No example.
- **Can I declare my own custom built-in variables?** Doc does not address.

### Crosslink gaps

- No link to **`capturing-step-output.md`** from the first appearance of `captureAs` in the doc.
- The data-flow diagram in the "File-Based Data Passing" section does not link to **`debugging-a-run.md#handoff-files`** for "a leftover test-plan.md tells you …".

### Ambiguous instructions

- **"Resolution order: During iteration steps, VarTable checks the iteration table first, then the persistent table."** — a user who captures `ISSUE_ID` in initialize and re-captures it in iteration will not know which one wins. The doc says iteration first, but does not show an example.
- The {{WORKFLOW_DIR}} / {{PROJECT_DIR}} table reads as if the user might want to override these flags routinely. In practice, the override is rare; the table makes it sound more common than it is.

---

## 10. `docs/how-to/capturing-step-output.md`

### Assumed knowledge

- **`runner.LastCapture()`, `claudestream.Aggregator`, `ResultEvent`, `result.result`, `is_error`, `claudestream` aggregator path** — implementation names.
- **"trimmed last non-empty stdout line"** — the meaning of "trimmed" is not specified (whitespace? newlines? control chars?).
- **`%q` formatter** — Go-specific. The doc says "Go's `%q` formatter, which escapes newlines and control characters" — fine, but the reader does not know whether their captured value is going to be `\\n`-escaped when used in a downstream prompt. (Spoiler: the format is for *display* in the log; the actual value is unescaped.) Doc does not make this distinction.
- **`StepDone` state** — used in the breakLoopIfEmpty interaction.

### Missing prerequisites

- This doc and `variable-output-and-injection.md` are mutually referential. A new user reading the index in order arrives here second; the doc says "if you're looking for how `{{VAR}}` tokens get *resolved*, see Variable Output & Injection" — a reader who just came from that doc bounces back.

### Confusing ordering

- **The schema for `captureMode` is described after the "Non-claude steps" subsection has already used `lastLine` as a default.** Reader sees `lastLine` referenced before the value is defined.
- **The "Bad capture script" example precedes the "Good capture script" example in the section header**, although text-wise the good one comes first. Cognitively jarring.

### Gaps for the new user

- **What if I want to capture stderr?** Doc says stderr is discarded for capture but streamed to the log. No way to capture it.
- **What if my output has trailing whitespace I want to preserve?** Doc says "trimmed" — no escape hatch.
- **What about a step that prints binary data?** Probably an edge case but the doc is silent.
- **What if I want to capture a JSON payload and pass it via {{VAR}}?** The 32 KiB cap and `\n` joining rules are described, but the reader has to figure out for themselves whether their JSON survives the round trip.

### Crosslink gaps

- No link to **`skipping-steps-conditionally.md`** from the "Once captured, reference the variable" section, even though `skipIfCaptureEmpty` consumes captures.
- No link to **`resuming-sessions.md`** from the claude-steps section — `resumePrevious` reads a session ID that has nothing to do with `captureAs`, but a reader may conflate the two.

### Ambiguous instructions

- **"For claude steps, `captureAs` binds to `result.result`"** — the user who does not read the linked feature doc will not know what `result.result` looks like for a multi-paragraph response.
- The two example files (`scripts/get_next_issue` and "Bad capture script") use slightly different shebang lines (`#!/bin/bash` vs `#!/usr/bin/env bash`) without explanation. A new user may pick the wrong one.

---

## 11. `docs/how-to/passing-environment-variables.md`

### Assumed knowledge

- **`sandbox.BuiltinEnvAllowlist`, D13 config validator, Category 10, `os.LookupEnv`** — implementation names.
- **"OAuth", "API key" auth** — alternatives mentioned but not explained.
- **`-e NAME` Docker syntax** — described but not all readers know Docker CLI flags.
- **`containerEnv` constraints** — the `_TOKEN`/`_KEY`/etc. suffix-warning list is unique to this doc; a reader writing their first config will likely trip the warning.

### Missing prerequisites

- The doc assumes the reader has already worked through `setting-up-docker-sandbox.md`. A user landing here first will not know what "the Docker container" means in context.

### Confusing ordering

- **`env` is described before `containerEnv`**, which is correct teaching order — but the "How merging works" / "Validation rules" sections come *between* `env` and `containerEnv`, splitting a topic across two halves.
- **The `.ralph-cache` directory note is in this doc**, but its primary explanation is in `caching-build-artifacts.md`. New user gets it twice with a forward link in the middle.

### Gaps for the new user

- **What is "Reserved sandbox name"?** Table cell says `CLAUDE_CONFIG_DIR`, `HOME`. No explanation of how to find the full reserved list.
- **What is "Denied for safety"?** `PATH`, `USER`, `SSH_AUTH_SOCK`, `LD_PRELOAD` — a reader who has a use case for forwarding `SSH_AUTH_SOCK` will not know this is denied until they hit it.
- **No example of forwarding multiple custom variables together** — the "GitHub token" example shows only one.

### Crosslink gaps

- No link to **`setting-up-docker-sandbox.md`** from the lead paragraph (only at the bottom).
- No link to **`caching-build-artifacts.md`** until the very last subsection.

### Ambiguous instructions

- **"Set `ANTHROPIC_API_KEY` in the host environment to satisfy the sandbox without a credentials file"** — but the doc does not say where to obtain an API key, what tier the user must be on, or whether it costs differently from OAuth.
- **The validator runs at startup, but `env` is also validated when "you save"** — there is no save in pr9k workflow runs (vs the workflow builder TUI). A new user may confuse the two contexts.

---

## 12. `docs/how-to/breaking-out-of-the-loop.md`

### Assumed knowledge

- **`StepDone`, `executor.LastCapture()`, `RunResult.IterationsRun`, `workflow.Run`** — implementation names.
- **"the iteration loop"** — used as a noun before defined.
- **`scripts/get_next_issue`** — referenced as if the reader has already seen it.

### Missing prerequisites

- The doc assumes the reader has read `capturing-step-output.md` (it gets the link at the top, but the reader following the index order arrives here just after the capture doc, so this is fine).

### Confusing ordering

- The "Multiple break steps" section is at the bottom but is a useful early example for users who want to add a kill-switch mechanism.

### Gaps for the new user

- **What does "no more issues found" actually look like at runtime?** The example shows `Captured ISSUE_ID = ""` — but what if the script writes an empty string vs prints a newline vs exits 0 with no stdout? They all collapse to `""`, but a user writing their own break script may not know.
- **Can I `breakLoopIfEmpty` from a Claude step?** The schema says it can be set on any step but the doc only shows non-claude examples. Validator behavior unstated here.

### Crosslink gaps

- No link to **`skipping-steps-conditionally.md`** even though they are sibling features mentioned together everywhere else.

### Ambiguous instructions

- **"`--iterations 0`"** vs **"`-n N`"** — the doc uses both forms. A reader who sees both in the same doc may wonder if there's a difference. (There isn't.)
- The infinite-loop warning ("If `--iterations 0` and no step ever breaks the loop, pr9k will run forever") is a one-line note. A startup-cost-sensitive user might appreciate stronger language.

---

## 13. `docs/how-to/skipping-steps-conditionally.md`

### Assumed knowledge

- **`captureStates` map** — implementation name. The "Interaction with captureStates isolation" section uses internal vocabulary.
- **`StepDone`** — same as above.
- **"per-phase `captureStates` maps"** — leaks the implementation's per-phase isolation strategy.
- **The phrase "the trimmed last non-empty stdout line"** — defined once in `capturing-step-output.md` but used here as if known.

### Missing prerequisites

- The doc assumes the reader knows that `captureAs` works on non-claude steps in the iteration phase (true, but the constraint table here adds "in iteration and finalize phases — not initialize" which is a *separate* constraint, easily conflated).

### Confusing ordering

- **The "Producing an empty capture" example with grep on `code-review.md`** assumes the reader has seen the default workflow's review step. A reader inventing their own use case has to mentally replace this with their own example.

### Gaps for the new user

- **Why is `skipIfCaptureEmpty` not allowed in initialize?** Doc states the constraint but does not explain *why* — meanwhile the validator's reasoning ("per-phase captureStates maps mean cross-phase references would silently never fire") is in a parenthetical.
- **Can multiple steps reference the same `captureAs`?** Implied yes, but no example.

### Crosslink gaps

- No link to **`recovering-from-step-failures.md`** from the "If the source step **failed** ... the dependent step runs normally" sentence — a reader debugging "why did my fix step run when the verdict failed?" needs that link.

### Ambiguous instructions

- **"It must reference a `captureAs` name bound by an **earlier step in the same phase**."** — what about a step that is supposed to be skipped if a *later* step fails? Not supported, but the doc doesn't acknowledge that direction.

---

## 14. `docs/how-to/setting-step-timeouts.md`

### Assumed knowledge

- **`syscall.Kill(-pid, SIGTERM)`, `Setpgid: true`, `process group`, "grandchildren are included"** — Unix-process-management vocabulary.
- **`docker kill --signal=SIGTERM`** — fine.
- **`status: "failed"` in `.pr9k/iteration.jsonl`** — the user will not have read the iteration-log section of `debugging-a-run.md` yet if they are following the index in order.
- **`StepTimedOutContinuing`** — internal Go enum value.
- **`Runner.SessionBlacklisted`, `Runner.BlacklistedSessions`** — internal API.
- **`claudestream` pipeline** — internal package.

### Missing prerequisites

- The doc references `resumePrevious` and `onTimeout` as if the reader has seen the full step schema.
- The doc has no Related Documentation section (unlike every other doc in the set).

### Confusing ordering

- **The "Partial session-ID blacklist" section** is implementation-internal trivia in the middle of a user-facing how-to. It belongs in the feature doc.
- **`onTimeout` policy is described after the timeout mechanics**, but before the user has internalised that timeouts default to "fail". The default behavior should come first; the soft-fail variant is a later refinement.

### Gaps for the new user

- **What's the minimum useful `timeoutSeconds`?** Validator says "positive integer" — but a 1-second timeout on a 30-minute step does not produce a useful diagnostic.
- **No example of a non-claude timeout** — every example is a claude step.
- **No guidance on how to choose a value.** The doc says "1800 is sized ~2.5× the observed organic p95" but the user has no organic p95 yet.
- **The advisory prompt budget pattern** is shown as text inside a prompt — but the reader does not know whether claude actually respects it.

### Crosslink gaps

- No "Related documentation" footer at all — every other doc in the set has one.
- No link to **`debugging-a-run.md`** for "checking timeout behavior in `iteration.jsonl`".
- No link to **`resuming-sessions.md`** despite the explicit "Gotcha: interaction with `resumePrevious`" callout.

### Ambiguous instructions

- **"1800 seconds (30 minutes) is the default applied to the bundled "Test writing" step"** — "default" here means "what ships in the bundled `config.json`", not "the schema default". Rewording would help.

---

## 15. `docs/how-to/resuming-sessions.md`

### Assumed knowledge

- **"five gates G1–G5"** — described inside the doc, but the gate-table format with internal-acronym labels (G1, G2, ...) is a usability hazard. A user reading the runtime log line `resume gate blocked (G4: ...)` will at least see the name; a user trying to *configure* `resumePrevious` will not naturally remember which gate is which.
- **"context window"** — used as a constraint reason without explanation.
- **`session_id`, `--resume <session_id>`, `claudestream` pipeline** — implementation surfaces.
- **"per-phase tracking is reset"** — internal mechanism.

### Missing prerequisites

- The doc says the feature is shipped off by default — so a user reading docs in order may wonder why this doc is included at all on a first read. Consider marking the page as "Advanced" or "Not used by default workflow".

### Confusing ordering

- **The G3 row "(covered by G2)"** in the gate table is confusing — listed for completeness but contributes nothing the reader needs to configure.
- **The "Interaction with skipped steps" section** assumes the reader has already read `skipping-steps-conditionally.md` — fine if they followed the index, but the cross-doc dependency is one-way.

### Gaps for the new user

- **The doc does not say what counts as a tightly-coupled pair worth resuming.** The two examples are concrete (test planning → test writing, code review → fix review items) but a user with a different workflow has no general rule.
- **"200 000 input tokens" in G4** — where does this number come from? Is it the model context window minus a margin, or a pr9k-specific cap?
- **No guidance for verifying that a resume *worked* end-to-end** beyond "session IDs match". Did claude actually use the prior context? The user can't tell from the log.

### Crosslink gaps

- No link to **`debugging-a-run.md#jsonl-artifacts-for-claude-steps`** for "reading the `session_id` field from the JSONL".

### Ambiguous instructions

- **"Setting it on a non-claude step is a fatal validator error"** but **"The validator emits a warning if the preceding step is non-claude"** — fatal vs warning is a real difference. A reader scanning may conflate them.

---

## 16. `docs/how-to/configuring-a-status-line.md`

### Assumed knowledge

- **`stdin JSON payload`** — the script contract is described but the JSON schema is on this page only. A user wanting to write a script in Python or Node has to translate the bash example.
- **`ANSI color codes` in the sample script** — the `\033[36m` escape is used without explaining it works because pr9k passes raw bytes through.
- **`8 KB stdout limit`, `2-second command timeout`** — pr9k-specific runtime caps stated in passing.
- **"cold-start"** — used twice; not defined here. (Defined in the feature doc.)
- **"sanitized first non-empty line"** — what sanitation? (Defined in the feature doc.)

### Missing prerequisites

- The doc requires `jq` — listed in Prerequisites — but the sample script also calls `git`. Listed as optional, fine.
- No statement that the status-line script runs **on the host**, not in the sandbox. (It is mentioned in the security note at the bottom but should be earlier.)

### Confusing ordering

- **The Step 2 sample-script block is 30 lines long** before any explanation of what each piece does. A reader who is not a bash expert will glaze over.
- **"Tuning `refreshIntervalSeconds`"** is a section title separate from "Step 3"; the doc is not entirely clear whether it's a fourth step or an aside.
- **The "Recovering the shortcut bar" section** is at the bottom, but it is the first thing a user wants to know after enabling the status line ("how do I get my shortcuts back?"). Should be much higher.

### Gaps for the new user

- **What if my script needs a value not in the JSON payload?** Doc does not say whether the payload is the complete API or whether more fields will be added.
- **Can I write a script in another language?** Yes (per the path-resolution rules), but no example.
- **What are the stdin guarantees?** Doc says "drain stdin before processing, which is required" but does not say "stdin closes after the JSON is written" — a user writing in Python may use `sys.stdin.read()` and need to know that EOF is signaled.

### Crosslink gaps

- No link to **`reading-the-tui.md#help-and-the-help-modal`** from the "Press `?`" instruction.

### Ambiguous instructions

- **`cp /path/to/pr9k/workflow/scripts/statusline scripts/statusline`** — the source path uses a placeholder, the destination is relative. A user running this command from the wrong directory will silently put the script in the wrong place.

---

## 17. `docs/how-to/caching-build-artifacts.md`

### Assumed knowledge

- **"the host user's UID via `-u`"** — Docker UID mapping vocabulary.
- **"`/etc/passwd` entry inside the container"** — assumes a deep familiarity with Linux user resolution.
- **"~88 `permission denied` retries per 8-iteration run (observed on gearjot-v2)"** — `gearjot-v2` is an internal repo name.
- **"`preflight.Run`"** — internal Go function.
- **"defense-in-depth fallback"** — security vocabulary applied to a build-cache topic.

### Missing prerequisites

- The doc does not say the bundled Go config already does this — wait, it does, in the Go subsection. But the user has to read past two paragraphs to find that out.
- No statement that the user must already have `containerEnv` understanding from `passing-environment-variables.md`. The link is at the bottom only.

### Confusing ordering

- **"The Problem" leads with a specific bug observation** instead of "what cache directories does each language use, and why are they broken in the sandbox by default?". A reader who is on a Node project will not see themselves in the Go-toolchain story.
- **"Host-Writability Precondition"** is at the bottom, but it is upstream of every cache subdir working at all.

### Gaps for the new user

- **No estimate of cache size on disk.** A 6 GB Go cache may surprise someone who set up a small VPS.
- **No mention of cache invalidation.** When does the cache get blown away? Never? After a `make build`?
- **No "I'm on Node, the bundled config doesn't help me" walkthrough.**

### Crosslink gaps

- No "Related documentation" section.
- No link to **`setting-up-docker-sandbox.md`** from the UID precondition section.

### Ambiguous instructions

- **The default Go config is included in `config.json`** but the doc shows a snippet with `containerEnv` *only* — a user who pastes this snippet into a custom config without the rest of the file structure may break their config.

---

## 18. `docs/how-to/copying-log-text.md`

### Assumed knowledge

- **"alt-screen", "cell-motion mouse"** — appear in `using-the-workflow-builder.md` but assumed knowledge here.
- **`pbcopy`, `xclip`, `xsel`** — fine for Unix users; doc does explain.
- **OSC 52** — explained, but only briefly.
- **`tmux with set-clipboard on`** — assumes tmux config knowledge.
- **`stty -ixon`** — referenced in `using-the-workflow-builder.md` but a user reading this doc first will not know.

### Missing prerequisites

- No statement of which terminals support OSC 52 in concrete terms — the doc names iTerm2, Kitty, WezTerm, Windows Terminal, recent xterm, but does not say what to do on Apple Terminal (default macOS Terminal).

### Confusing ordering

- The doc opens with three "Common paths" which is great, but each path uses keyboard symbols (`v`, `$`, `Shift+↓`) without saying they work only in Select mode. A new user will press `v` from another mode and wonder why nothing happens.

### Gaps for the new user

- **What is the maximum copy size?** Not stated.
- **What does the clipboard receive on Windows?** Doc focuses on macOS and Linux.
- **What if my terminal supports OSC 52 but I'm running through an SSH bastion?** OSC 52 should still work in theory but the doc does not provide the troubleshooting steps.

### Crosslink gaps

- No link to **`reading-the-tui.md#using-select-mode`** even though that doc has the same keybinding table.

### Ambiguous instructions

- **"The viewport auto-scrolls to keep the cursor visible"** — good behaviour, but the doc does not say what happens if the cursor reaches the top or bottom of the log buffer (capped at 2000 lines, per `reading-the-tui.md`).

---

## 19. `docs/how-to/using-the-workflow-builder.md`

### Assumed knowledge

- **"alt-screen", "cell-motion mouse"** — terminal capabilities phrased in TUI-internal vocabulary.
- **"empty-editor hint state", "session header banner", "edit view", "outline pane", "detail pane"** — the doc introduces these names but does not show a screenshot or wireframe. A new user has to construct the layout in their head.
- **"banner priority and colors"** — `[ro]`, `[ext]`, `[sym]`, `[shared]`, `[?fields]` — five tag vocabularies introduced at once. Without a visual, the user has to guess.
- **"companion file"** — used before defined. (A companion is a prompt file or script associated with a step.)
- **"validator integration", "conflict detection", "session-event logging"** — feature-doc terminology in a how-to.
- **"D-PR2-13", "D-20", "D-41"** — design-doc IDs from the feature spec, leaked.
- **"findings panel"** — displayed only "when there are fatal validation findings" — but the user has not been shown one.
- **"Bubble Tea ctx exemption"** — code-package reference.

### Missing prerequisites

- The doc says "pr9k installed and on your `PATH`" — but earlier docs say `path/to/pr9k/bin/pr9k`. The two postures conflict (no `PATH` install instruction has been given anywhere upstream).

### Confusing ordering

- **The launch flag table is shown before the "Creating a New Workflow" section**, even though `--workflow-dir` is a power-user flag.
- **"Saving" is described before "Quitting"** — fine — but the dirty-state dialog (offered during quit) is not foreshadowed in the Saving section.
- **"Note on `Ctrl+S` in some terminals"** — XON/XOFF caveat is a footnote at the bottom of Saving. Should be much more prominent.

### Gaps for the new user

- **No screenshot or wireframe of the builder.** Compared to `reading-the-tui.md` which has an ASCII diagram, this doc is text-only.
- **What happens if I press `Ctrl+E` on a step that doesn't have a prompt file?** Implicit, not stated.
- **What does "atomic write" mean for the user?** Doc says the file writes atomically — fine, but the implication ("you won't get a half-written config.json") is left to the reader.
- **What does the "findings panel" look like?** Only described abstractly.
- **What is the "session header (third row)"?** Numbering implies there are rows 1 and 2 — what are they? (Menu bar and... something? The feature doc has the layout; this doc does not.)

### Crosslink gaps

- No link to **`building-custom-workflows.md`** from the description of the outline sections (env, containerEnv, statusLine, initialize/iteration/finalize) — those concepts are explained there.
- No link to **`debugging-a-run.md`** for "after I save, where does the run-time log go?".

### Ambiguous instructions

- **`pr9k workflow`** vs **`/path/to/pr9k/bin/pr9k workflow`** — the doc uses the former, prior docs use the latter. The user will not know which one to type.
- **"The path picker appears, pre-filled with `<projectDir>/.pr9k/workflow/`"** — reader has to know what "the path picker" looks like.

---

## 20. `docs/how-to/configuring-external-editor-for-workflow-builder.md`

### Assumed knowledge

- **`$VISUAL`, `$EDITOR`** — Unix tradition. Defined enough.
- **"shell metacharacters"** — defined by example.
- **"daemonization"** — used in the GUI Editors section without definition.
- **"foreground process group", "SIGINT"** — Unix-process vocabulary in the SIGINT section.

### Missing prerequisites

- No statement that the user must have already launched the workflow builder (`pr9k workflow`) once — the doc opens with environment variables before the user has any context for `Ctrl+E`.

### Confusing ordering

- **The "Accepted Values" and "Rejection Set" tables are very thorough** but appear before the user has ever set `VISUAL` once. A reader following the index just learned how to launch the builder; they want a one-line "set `VISUAL=nano` in your shell rc and you're done", not a security-rule-set.
- **The `tmux and SSH: Alt Modifier Caveat` section** is interesting but unrelated to the editor topic. Belongs in the workflow-builder doc proper.

### Gaps for the new user

- **What happens if my editor has a TUI of its own (vim, nano, helix)?** Should work fine, but the doc does not show a "expected experience" walkthrough.
- **What if my system has VS Code installed but the `code` CLI is not on PATH?** (macOS users have to install the CLI helper.)
- **What about Windows-style editors mapped through WSL?** Out of scope, but the doc does not say so.

### Crosslink gaps

- No link to **`debugging-a-run.md`** for "after editing a prompt, my run uses the new content — how do I verify?"

### Ambiguous instructions

- **"in some shells, you can re-export the variable in the same shell session without restarting"** — vague. Either it works in the shell that launched pr9k or it doesn't. Document the actual rule.

---

# Top open questions for a new user

These are questions a generalist would still not be able to answer cleanly after reading all 20 docs in order. Each is a candidate for an explicit answer in the restructured docs.

1. **What exactly counts as a "ralph" issue, and how do I create the label?** The README says "labeled `ralph`" but no doc walks the user through the one-time GitHub setup (creating the label, assigning the issue to themselves, linking it to a project board if they want one).

2. **What does a successful first run look like end-to-end, and how long does it take?** None of the docs show a complete log of a successful single-iteration run with realistic timing. A user does not know whether 5 minutes of silence on "Feature work" is normal or a hang.

3. **How much will this cost me?** Per-step `$cost` lines exist, but no doc gives an expected dollar figure for a typical run, the difference between sonnet and opus, or how to set a spending cap.

4. **What is the actual difference between `--workflow-dir`, `--project-dir`, the in-repo override, and the executable-dir fallback?** Four concepts with overlapping vocabulary; no single diagram shows the precedence and the typical use case for each.

5. **Where does my workflow live for the default install — and which copy do I edit?** A user who runs `make build` and edits `bin/.pr9k/workflow/config.json` will have their edits clobbered on the next build. A user who edits `src/...workflow/config.json` won't see changes until they rebuild. A user who creates `<projectDir>/.pr9k/workflow/` may not realize the override is per-repo. The docs describe each path but do not give a "you should do X" recommendation.

6. **What happens if I have no `ralph`-labeled issues open?** Pr9k will run `get_next_issue`, get an empty string, fire `breakLoopIfEmpty`, skip the iteration phase, run finalization, and exit. None of the docs walk this through as a happy path — a user will see "Captured ISSUE_ID = """ on the first iteration and assume something broke.

7. **Can I use pr9k against a workflow that does not involve GitHub at all?** The narrow-reading ADR says yes; every how-to assumes GitHub issues. A new user wanting to drive Claude through a non-GitHub queue (Linear, Jira, a flat file) has no recipe.

8. **What is the relationship between the `claude` CLI's authentication and the sandbox's authentication?** macOS Keychain, `.credentials.json`, `ANTHROPIC_API_KEY`, `CLAUDE_CONFIG_DIR` — four vocabularies, three working configurations, no decision tree.

9. **What is the recommended day-2 operations workflow?** What logs should I keep? When should I prune `.pr9k/logs/`? Should I commit `iteration.jsonl`? When does `.ralph-cache/` need to be invalidated?

10. **What is the smallest possible custom workflow I can write?** The Building Custom Workflows doc shows an example with 3 iteration steps, but a new user wanting to test "can pr9k run one shell command in a loop" has to construct that themselves.

11. **What does "session resume" actually buy me, and should I turn it on?** The doc explains the gates but does not give a side-by-side comparison of a workflow with vs without `resumePrevious` enabled. A new user has no basis to decide.

12. **How do I tell the difference between "pr9k is broken" and "my workflow is broken"?** Validator errors, build errors, claude errors, network errors, and shell errors all have different signatures, and no doc gives a flat troubleshooting table for the symptom-to-cause walk.
