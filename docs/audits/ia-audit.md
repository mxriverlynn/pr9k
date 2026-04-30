# IA Analysis: pr9k user-facing documentation set (README + docs/how-to/)

## Scope

Files audited (full read, all content, every cross-reference followed at least one hop):

- `/Users/mxriverlynn/dev/mxriverlynn/pr9k/README.md` (the front door)
- `/Users/mxriverlynn/dev/mxriverlynn/pr9k/docs/how-to/getting-started.md`
- `/Users/mxriverlynn/dev/mxriverlynn/pr9k/docs/how-to/setting-up-docker-sandbox.md`
- `/Users/mxriverlynn/dev/mxriverlynn/pr9k/docs/how-to/reading-the-tui.md`
- `/Users/mxriverlynn/dev/mxriverlynn/pr9k/docs/how-to/building-custom-workflows.md`
- `/Users/mxriverlynn/dev/mxriverlynn/pr9k/docs/how-to/variable-output-and-injection.md`
- `/Users/mxriverlynn/dev/mxriverlynn/pr9k/docs/how-to/capturing-step-output.md`
- `/Users/mxriverlynn/dev/mxriverlynn/pr9k/docs/how-to/breaking-out-of-the-loop.md`
- `/Users/mxriverlynn/dev/mxriverlynn/pr9k/docs/how-to/skipping-steps-conditionally.md`
- `/Users/mxriverlynn/dev/mxriverlynn/pr9k/docs/how-to/setting-step-timeouts.md`
- `/Users/mxriverlynn/dev/mxriverlynn/pr9k/docs/how-to/configuring-a-status-line.md`
- `/Users/mxriverlynn/dev/mxriverlynn/pr9k/docs/how-to/recovering-from-step-failures.md`
- `/Users/mxriverlynn/dev/mxriverlynn/pr9k/docs/how-to/quitting-gracefully.md`
- `/Users/mxriverlynn/dev/mxriverlynn/pr9k/docs/how-to/passing-environment-variables.md`
- `/Users/mxriverlynn/dev/mxriverlynn/pr9k/docs/how-to/copying-log-text.md`
- `/Users/mxriverlynn/dev/mxriverlynn/pr9k/docs/how-to/debugging-a-run.md`
- `/Users/mxriverlynn/dev/mxriverlynn/pr9k/docs/how-to/caching-build-artifacts.md`
- `/Users/mxriverlynn/dev/mxriverlynn/pr9k/docs/how-to/resuming-sessions.md`
- `/Users/mxriverlynn/dev/mxriverlynn/pr9k/docs/how-to/using-the-workflow-builder.md`
- `/Users/mxriverlynn/dev/mxriverlynn/pr9k/docs/how-to/configuring-external-editor-for-workflow-builder.md`

Out of scope (per the brief): `docs/features/`, `docs/code-packages/`, `docs/coding-standards/`, `docs/adr/`, `docs/plans/`, `docs/architecture.md`, `docs/project-discovery.md`. Those are contributor-facing docs.

Branch: `main`.

Recency pass (`git log --since="180 days ago" --name-only` against the in-scope files): all 20 files have been touched in the last 6 months. The most-churned files are `reading-the-tui.md` (33 commits), `building-custom-workflows.md` (22), `README.md` (18), `variable-output-and-injection.md` (14), and `configuring-a-status-line.md` (14). Recently churned files are where structural drift accumulates fastest, so these are weighted slightly higher in severity below.

## Reader Context

- **Primary reader goal (JTBD):** *When I have a GitHub repo with issues I want automated, I want to install pr9k, point it at my repo, and confirm a successful first run, so I can decide whether to adopt it for unattended runs and then customize the workflow for my project.*
- **Audience segments:**
  - **A1 — First-time evaluator** (cold from search or GitHub recommendation): "Should I use this? What does it do? What's the smallest path to seeing it work?"
  - **A2 — First-time installer** (decided to try it): "Walk me from clone to first iteration without surprises."
  - **A3 — Workflow customizer** (default workflow ran; wants to adapt): "How do I write my own steps, prompts, captures, env vars?"
  - **A4 — Operator during a run** (workflow is running on screen right now): "What does this footer mean? How do I quit? My step failed, now what?"
  - **A5 — Debugger after a run** (something went wrong; reconstructing the failure): "Where are the logs? Why did this capture bind the wrong value?"
  - **A6 — Habitual expert** (reference-lookup user): "Remind me of the captureMode rules. What's the validator regex for env names?"
- **Tasks covered:** install; sandbox setup; first run; read the TUI; copy log text; quit; recover from step failure; build a custom workflow; capture step output; inject variables; pass env vars to sandbox; cache build artifacts; break out of the loop; skip steps conditionally; set timeouts; resume sessions; configure a status line; use the workflow builder; configure an external editor; debug a run.
- **Arrival paths considered:** GitHub README (most likely cold path); deep-link to a how-to from a colleague or search result; in-app reference (a footer mention of `?`/help); cross-repo link (someone uses pr9k against another project's docs).

## Content Inventory Summary

All 20 in-scope pages enumerated. Columns: Path, Topic Type, Audience(s), Inbound (from in-scope pages only), Outbound (to in-scope how-to pages only), Lines.

| # | Path | Topic Type | Audience(s) | Inbound (how-to + README) | Outbound (how-to only) | Lines |
|---|------|------------|-------------|---------------------------|------------------------|-------|
| 1 | `README.md` | Index/Concept hybrid | A1, A2 | (front door) | 13 of 20 how-tos linked | 126 |
| 2 | `getting-started.md` | Tutorial | A2 | README, 11 how-tos | 7: docker-sandbox, building-custom, variable-output, capturing, env vars, breaking-out, debugging | 144 |
| 3 | `setting-up-docker-sandbox.md` | Task + Reference | A2 | README, getting-started, env vars | 3: getting-started, env vars, recovering-from-failures | 271 |
| 4 | `reading-the-tui.md` | Concept + Reference | A2, A4 | README, 8 how-tos | 4: copying-log, recovering, quitting, debugging | 261 |
| 5 | `building-custom-workflows.md` | Reference + Tutorial | A3 | README, 10 how-tos | 7: getting-started, variable-output, capturing, env vars, breaking-out, recovering, debugging | 216 |
| 6 | `variable-output-and-injection.md` | Concept + Reference | A3 | README, 8 how-tos | 5: getting-started, building-custom, capturing, breaking-out, debugging | 211 |
| 7 | `capturing-step-output.md` | Task + Reference | A3 | README, 7 how-tos | 4: variable-output, breaking-out, building-custom, debugging | 201 |
| 8 | `breaking-out-of-the-loop.md` | Task | A3 | README, 5 how-tos | 3: capturing, building-custom, recovering | 144 |
| 9 | `skipping-steps-conditionally.md` | Task | A3 | 2 how-tos | 4: breaking-out, capturing, building-custom, recovering | 115 |
| 10 | `setting-step-timeouts.md` | Task + Reference | A3 | 1 how-to (resuming) | 0 (no Related Documentation section) | 73 |
| 11 | `configuring-a-status-line.md` | Task | A3 | README, reading-the-tui | 1: reading-the-tui | 164 |
| 12 | `recovering-from-step-failures.md` | Task | A4 | README, 6 how-tos | 4: reading-the-tui, quitting, debugging, (none for skip) | 143 |
| 13 | `quitting-gracefully.md` | Task | A4 | README, 4 how-tos | 2: reading-the-tui, recovering | 111 |
| 14 | `passing-environment-variables.md` | Reference + Task | A3 | README, 4 how-tos | 3: building-custom, docker-sandbox, caching (one-way only) | 137 |
| 15 | `copying-log-text.md` | Task | A4 | README, reading-the-tui | 1: reading-the-tui | 98 |
| 16 | `debugging-a-run.md` | Task + Reference | A5 | README, 9 how-tos | 5: reading-the-tui, capturing, variable-output, breaking-out, recovering | 286 |
| 17 | `caching-build-artifacts.md` | Task | A3 | env vars (one-way) | 0 (no Related Documentation section) | 106 |
| 18 | `resuming-sessions.md` | Task + Reference | A3 (advanced) | timeouts (one-way), skipping (one-way) | 4: timeouts, skipping, building-custom, debugging | 109 |
| 19 | `using-the-workflow-builder.md` | Tutorial + Task | A3 | external-editor (one-way) | 1: external-editor (and one to building-custom) | 144 |
| 20 | `configuring-external-editor-for-workflow-builder.md` | Task | A3 | using-the-builder (one-way) | 1: using-the-builder | 94 |

**Inventory observations.** The README links to **only 13 of the 20** how-to pages — the seven not enumerated in the README's "How-To" bullet list at `README.md:88-100` are: `skipping-steps-conditionally.md`, `setting-step-timeouts.md`, `caching-build-artifacts.md`, `resuming-sessions.md`, `using-the-workflow-builder.md`, `configuring-external-editor-for-workflow-builder.md`. (The `configuring-a-status-line.md` link IS present but two recently added pages — caching, resuming, the two builder pages, plus skipping and timeouts — never made it into the index.) These pages are reachable only via a sibling page that happens to remember them; from the README front door, they are effectively orphans. See **IA-001** below.

Two pages have **no Related Documentation section at all**: `setting-step-timeouts.md` (ends after "Advisory prompt budget" at line 73) and `caching-build-artifacts.md` (ends after "Target Project `.gitignore`" at line 106). See **IA-007**.

## Question Log

### Arrival Path
- **Q-AR1 [Assumed]:** How does the typical first-time reader arrive? — Assume GitHub README (most common for OSS dev tooling) or a deep-linked how-to from search ("pr9k status line", "pr9k captureAs"). The audit treats both as primary. No analytics were available to confirm.
- **Q-AR2 [Answered]:** Can a reader leave a how-to and return to a hub? — No. None of the 20 pages link **back** to the README. Closest hub is `getting-started.md`, which is reachable from 11 sibling how-tos. See IA-006.

### Audience Segmentation
- **Q-AU1 [Assumed]:** Is the documentation segmenting audiences? — No explicit segmentation anywhere. README headings are activity-flavored ("Getting Started", "How To") rather than audience-flavored. No persona statements exist. The audit assumes the six segments listed in Reader Context above.
- **Q-AU2 [Answered]:** Are contributor docs and user docs separated? — Partially. The README's `## Documentation` section at `README.md:84-122` interleaves user-facing how-tos with feature docs (contributor-facing) and code package docs (deep contributor-facing) under a single H2 with no audience break. See IA-002.

### Reader Task (JTBD)
- **Q-TA1 [Answered]:** Does the README state what pr9k is and why? — Yes, in one sentence at `README.md:5`. Strong opener; no friction.
- **Q-TA2 [Answered]:** Does the README state who it is for? — Implicitly (anyone with GitHub issues + Claude CLI), never explicitly. A reader who is unsure whether their workflow looks like "ralph-labeled GitHub issues + automated implementation" has no decision aid. Friction-level only — see IA-003.

### Usage Pattern
- **Q-US1 [Answered]:** Are how-tos task-oriented or reference-oriented? — Most are task-oriented and chunk well; exceptions are `building-custom-workflows.md` (opens with the schema table before any task example — see IA-008) and `variable-output-and-injection.md` (opens with engine internals — see IA-009).
- **Q-US2 [Open]:** Do readers use the docs linearly (read-through) or random-access (search)? — Materially affects whether the alphabetical filename order matters; assumed mostly random-access from search but with a substantial first-time linear path (clone → install → first run). The recommendation set covers both.

### Prior Knowledge
- **Q-PK1 [Answered]:** What does each how-to assume the reader knows? — `variable-output-and-injection.md:7` opens with "All `{{VAR_NAME}}` tokens... are expanded at runtime using the `vars.Substitute` function" — this names a Go internal before the reader has any reason to know what `vars.Substitute` is. See IA-009.
- **Q-PK2 [Answered]:** Are JSON config field names defined before they are used? — Mostly yes. `building-custom-workflows.md` defines them in a table at line 19; `capturing-step-output.md` introduces `captureAs` cleanly. The exception is `setting-step-timeouts.md` which mentions `onTimeout` at line 26 inside a code block before its policy section explains it (lines 42-50). Friction-level — see IA-013.
- **Q-PK3 [Open]:** What is the assumed Claude/Anthropic familiarity level? — `getting-started.md:10` mentions "Claude CLI" with a link, but never explains what an OAuth flow is, what a "session" is, or what `bypassPermissions` does. Audience-of-One risk if the audience is broader than "developers who already use claude". Resolution requires a product call.

### Context of Reading
- **Q-CO1 [Assumed]:** Are readers usually on a desktop with a GitHub tab and a terminal tab? — Assumed yes. No mobile-specific concerns drove the audit.
- **Q-CO2 [Answered]:** Are readers operating during a live run when they consult the docs? — Some are (audience A4). The TUI footer's `? Help` modal is the in-app reference, but the docs do not mention that the modal exists from any of the operator-facing how-tos (`recovering-from-step-failures.md`, `quitting-gracefully.md`, `copying-log-text.md`). Polish-level — see IA-014.

### Orientation
- **Q-OR1 [Answered]:** Can a reader dropped onto any how-to tell where they are and what comes next? — Mostly yes for the older pages (each has "Related documentation" at the bottom). Two pages fail outright: `setting-step-timeouts.md` and `caching-build-artifacts.md` have no Related-Documentation section. See IA-007.
- **Q-OR2 [Answered]:** Does any page have a "previous step / next step" navigation aid for the linear first-time path? — No. The first-time linear path (install → docker → first run → tui → custom workflow) is not signposted as a sequence anywhere. See IA-005.

### Entry-Point Density
- **Q-EP1 [Answered]:** How many front doors? — One: the README. The how-to directory itself has no `index.md` or `README.md`; running `ls docs/how-to/` is the only directory-level surface. See IA-004.

### Cross-Channel Consistency
- **Q-CC1 [Answered]:** Does the README's prerequisites list match `getting-started.md`'s? — No. README at lines 13-16 lists Go, gh, Claude CLI, and a labeled-issues repo. `getting-started.md:7-12` adds **Docker** as a "**required** runtime dependency" and a Unix-only constraint. The README omits Docker entirely, which is a contradiction the reader notices the moment they try to run. Severity: Degrades comprehension — see IA-015.
- **Q-CC2 [Answered]:** Does the README's "How To" section mirror the `docs/how-to/` directory? — No, see IA-001 (7 of 20 pages missing). Friction grows over time as more how-tos are added without README updates.

### Decision and Action
- **Q-DA1 [Answered]:** Are workflow-customization decisions sequenced? — Partially. The "step schema" table appears in `building-custom-workflows.md:21-34` before the reader has read about iteration phases or seen a minimal example. See IA-008.

### Exit and Completion
- **Q-EC1 [Answered]:** When is a reader "done" with first-run setup? — `getting-started.md` ends with a "Where to go next" list (good) but the list mixes prerequisites (Docker) with strict next-steps (custom workflows) and advanced features (env vars, breaking-out). See IA-016.

### Measurement and Validation
- **Q-MV1 [Open]:** What support questions or issue patterns most often come up? — Unknown. The audit cannot defensibly tier severity for "first-run failure modes" without this data. Findings are scoped to evidence visible in the docs themselves; if measurement data shows that "Docker missing" is the #1 first-run failure, IA-015 escalates from Degrades to Blocks.

## Assumptions

- **AS1:** The primary reader of the README is a developer evaluating pr9k for the first time, arriving from a GitHub link or search result, on a desktop terminal.
- **AS2:** Audience segments listed in Reader Context (A1–A6) are present in roughly that order of frequency. No analytics confirm this.
- **AS3:** The pencil MCP and Auto Mode system reminders that appeared during this session are unrelated to the audit task and are ignored.
- **AS4:** The brief's stated restructure goal — task-ordered sequence (basics → first run → custom → advanced → debug) — is the source-of-truth target shape. All recommendations align with that goal.
- **AS5:** No card-sort, tree-test, or user-research data is available; tree-test-likely-confirms-or-fails reasoning is qualitative based on the audit content.
- **AS6:** The seven how-tos missing from the README's How-To list (skipping, timeouts, caching, resuming, builder, external-editor) are recent additions whose README entry was not added when they shipped, rather than a deliberate exclusion.

## Open Questions

**OQ-1: Is Docker truly mandatory for the first run, or can a reader try pr9k without Docker first?**
- **Why it matters:** README at `README.md:13-16` does not list Docker as a prerequisite, but `getting-started.md:8` says it is "**required**". Either the README or getting-started is wrong. Resolution determines whether IA-015 is a README fix (add Docker), a getting-started fix (soften the "required"), or both.
- **Findings affected:** IA-015
- **How to resolve:** Confirm with the maintainer whether `pr9k -n 1` against the bundled workflow can succeed without Docker. If no, README adds Docker as a top-level prerequisite. If yes, getting-started softens its language and explains the supported-without-Docker path.

**OQ-2: Are `using-the-workflow-builder.md` and `configuring-external-editor-for-workflow-builder.md` "polished, recommended" or "experimental, peripheral"?**
- **Why it matters:** They are absent from the README's How-To list (IA-001). The recommendation in IA-001 is to add them. If they are experimental and intentionally hidden from the front door, the recommendation changes to either marking them as experimental or moving them to a "later/optional" subsection.
- **Findings affected:** IA-001 placement of builder pages
- **How to resolve:** Maintainer judgment; could also be inferred from a "Stability" section in CLAUDE.md if added.

**OQ-3: Is `resumePrevious` a beginner-relevant feature or an advanced/optional one?**
- **Why it matters:** `resuming-sessions.md:5-6` says the engine is implemented but the default workflow ships with the feature off. That signal — "off by default" — implies the page should be in an "advanced" tier, not the same tier as `getting-started.md`. The recommended grouping below puts it in Advanced; if the maintainer disagrees, regroup.
- **Findings affected:** IA-001 placement of `resuming-sessions.md`; recommended grouping in IA Improvement Summary

**OQ-4: Should the "How To" hub be the README, a `docs/how-to/README.md`, or both?**
- **Why it matters:** Today the README is the only hub (IA-004). Adding a second hub creates two-truth risk; not adding one means the README's link list keeps drifting out of sync.
- **Findings affected:** IA-004
- **How to resolve:** Product call. Recommendation defaults to one hub (a new `docs/how-to/README.md`) plus a thin pointer from the main README, because the directory-level hub stays in sync with the directory contents naturally.

## Summary

The pr9k user-facing documentation set is well-written page-by-page (each how-to has a clear topic, examples, and most have Related-Documentation footers) but its **information architecture is alphabetical-by-filename, not task-ordered**. The README front door fails on inventory completeness (7 of 20 how-tos are not linked from the README) and on prerequisite consistency (Docker is missing from README prerequisites but mandatory in `getting-started.md`). No page links back to the README, no page signposts a "previous step / next step" sequence, and there is no `docs/how-to/` index — so a reader navigating sideways from any one how-to has no reliable hub to return to. Two recently added pages (`setting-step-timeouts.md`, `caching-build-artifacts.md`) lack any Related-Documentation section, leaving them as terminal dead-ends.

| Severity               | Count |
|------------------------|-------|
| Blocks comprehension   | 2     |
| Degrades comprehension | 6     |
| Friction               | 8     |
| Polish                 | 4     |

Open Questions: 4 (must be answered before findings are fully actionable)

Full analysis written to: `/Users/mxriverlynn/dev/mxriverlynn/pr9k/docs/audits/ia-audit.md`

## Findings

### Protocol 1 — Critical Inquiry and Reader Context

Findings driven by inquiry are surfaced throughout. The two findings that originate directly from the question log (vs from a protocol pass) are:

**IA-001: README's "How-To Guides" list is missing 7 of the 20 in-scope how-to pages.**
- **Principle:** Rosenfeld/Morville organization + navigation systems; *Orphan Topic* anti-pattern; Dan Brown Principle of Disclosure (a hub must surface what exists below it).
- **Location:** `README.md:88-100` ("How-To Guides" bullet list under `## Documentation`).
- **Evidence:** The list enumerates: getting-started, setting-up-docker-sandbox, reading-the-tui, building-custom-workflows, variable-output-and-injection, capturing-step-output, breaking-out-of-the-loop, configuring-a-status-line, recovering-from-step-failures, quitting-gracefully, passing-environment-variables, copying-log-text, debugging-a-run. Files that exist on disk but are not linked from the README: `skipping-steps-conditionally.md`, `setting-step-timeouts.md`, `caching-build-artifacts.md`, `resuming-sessions.md`, `using-the-workflow-builder.md`, `configuring-external-editor-for-workflow-builder.md`. (Cross-checked against `ls docs/how-to/`.)
- **Reader Impact:** Audience A2/A3 arriving at the README has no path to discover step timeouts, conditional skipping, build-artifact caching, session resume, or the workflow builder. They land in those pages only sideways from sibling how-tos that happen to remember them — and `caching-build-artifacts.md` has only one inbound link from `passing-environment-variables.md` and `setting-step-timeouts.md` has only one inbound link from `resuming-sessions.md`, so for a reader who started at the README, those pages are effectively unreachable.
- **Related questions:** Q-AR2 (answered), Q-CC2 (answered), OQ-2 (open), OQ-3 (open).
- **Severity:** Degrades comprehension.
- **Remediation:** Stop maintaining the link list inside `README.md`. Replace it with a single link to a new `docs/how-to/README.md` that lives inside the how-to directory itself (so it stays in sync with `ls`). The directory-level hub is the canonical inventory; the main README points to it under "How-To Guides".

### Protocol 2 — Content Inventory

**IA-002: README's `## Documentation` section interleaves user-facing how-tos with contributor-facing feature docs and code-package docs without an audience break.**
- **Principle:** Hackos audience-task mapping; Dan Brown Principle of Multiple Classification (one tree, multiple readers, but the tree should signal which audience each branch is for); *Audience-of-One* anti-pattern (ignoring that "the developer reading the README" splits into evaluator/installer/customizer/contributor).
- **Location:** `README.md:84-122` — the `## Documentation` H2 contains "How-To Guides" (user docs), "Feature Documentation" (in `docs/features/`, contributor-leaning), "Code Package Documentation" (deep contributor docs), and three more bullets for Coding Standards, ADRs, and the original plan.
- **Evidence:** `README.md:101` reads "**Feature Documentation** (in `docs/features/`) — user-facing behavior and cross-package integration" — but the linked pages (e.g., `subprocess-execution.md`, `claudestream.md`) are written for someone modifying pr9k Go code, not someone driving pr9k against a target repo. The README's own description blurs the audience.
- **Reader Impact:** Audience A1 (first-time evaluator) and A2 (first-time installer) scrolls down looking for "how do I do X?" and gets pulled into "internal/claudestream Parser, Renderer, Aggregator" — Curse-of-Knowledge prose. They cannot tell which links are for them.
- **Related questions:** Q-AU2 (answered).
- **Severity:** Degrades comprehension.
- **Remediation:** Split `## Documentation` into two H2s: `## User Guides` (the new how-to hub link + architecture overview) and `## Contributor Reference` (features, code packages, coding standards, ADRs, plans). Use the language explicitly: "If you are using pr9k against your own repo, start with User Guides. If you are modifying pr9k itself, see Contributor Reference."

**IA-003: README has no "who is this for / when should you reach for it" frame after the one-sentence opener.**
- **Principle:** *Front-Door Absence* anti-pattern (mild form); EPPO orientation; Hackos audience analysis.
- **Location:** `README.md:5-7` (the one-sentence what + the AI-Hero credit) flowing directly into `## Getting Started` at line 9.
- **Evidence:** Line 5 says what pr9k does in one sentence. Line 9 immediately starts installation prerequisites. There is no "Use pr9k when... Don't use pr9k when..." or "You'll know it's working when..." framing. A reader unsure whether their workflow matches "GitHub issues labeled ralph + Claude CLI + automated implementation" has no decision aid.
- **Reader Impact:** Audience A1 (evaluator) bounces. They cannot tell if pr9k is for them without first installing it.
- **Related questions:** Q-TA2 (answered), Q-AU1 (assumed).
- **Severity:** Friction.
- **Remediation:** Insert a 3-line "When to use pr9k" framing block between line 7 and line 9. Example shape (do not rewrite prose; this is the structural slot): a 1-line "Reach for pr9k when..." + 1-line "Don't reach for it when..." + 1-line "What success looks like (one iteration ends with a closed issue and a pushed commit)".

### Protocol 3 — Audience and Task Analysis

**IA-004: There is no `docs/how-to/` index page; the directory itself has no front door.**
- **Principle:** Rosenfeld/Morville navigation system; *Front-Door Absence*; Dan Brown Principle of Front Doors.
- **Location:** Directory `/Users/mxriverlynn/dev/mxriverlynn/pr9k/docs/how-to/` — contains 19 markdown files and no `README.md` or `index.md`.
- **Evidence:** `ls docs/how-to/` is the only directory-level surface. Anyone who navigates to `docs/how-to/` on GitHub sees a flat alphabetical list with no grouping, no "start here", and no indication of which page is the recommended first read. The README's sibling list at `README.md:88-100` is the only narrative grouping anywhere — and it is incomplete (IA-001).
- **Reader Impact:** Audience A2/A3 arriving at the GitHub directory listing (a common path when sharing a sub-link) sees an alphabetical flat list and cannot tell whether to open `breaking-out-of-the-loop.md` first or `getting-started.md` first.
- **Related questions:** Q-EP1 (answered), OQ-4 (open).
- **Severity:** Degrades comprehension.
- **Remediation:** Add `docs/how-to/README.md` as the canonical hub. Group pages into the five recommended sections (see IA Improvement Summary). The main README links here once instead of maintaining a 13-bullet list inline.

**IA-005: There is no "previous step / next step" sequence signposting on any how-to page, even though the brief's first-time path is sequential (install → sandbox → first run → tui → custom).**
- **Principle:** Rosenfeld/Morville navigation (local nav); EPPO orientation ("what to read next" is part of standing alone); Dan Brown Principle of Sufficiency.
- **Location:** Every how-to page; specifically, the "Where to go next" list at `getting-started.md:135-144` is the only attempt at sequencing — but it lists 7 unordered options, mixing strict-next-steps with optional-later items.
- **Evidence:** `getting-started.md:137` lists "Setting up the Docker sandbox (first-time)" as the first bullet, but that is something most readers should have done **before** running `pr9k` for the first time (the page explicitly says Docker is required at line 8 and the preflight checks Docker at line 207). So the recommended order is sandbox → first run, but `getting-started.md` puts first run before sandbox setup. A reader following the page in order will run pr9k, hit a Docker error, and only then learn there is a separate sandbox-setup page.
- **Reader Impact:** Audience A2 (first-time installer) gets trapped in a cyclic dependency between `getting-started.md` and `setting-up-docker-sandbox.md` — neither is unambiguously first.
- **Related questions:** Q-OR2 (answered), Q-CC1 (answered).
- **Severity:** Degrades comprehension.
- **Remediation:** Establish an explicit linear sequence for "Getting Started" (the new group): (1) prerequisites, (2) install + docker sandbox setup, (3) first run, (4) reading the TUI. Add a thin "Previous: X · Next: Y" line at the top of each page in this group. Move Docker sandbox setup before "First run" inside `getting-started.md` (or split installation+sandbox out as a single page) so the linear path doesn't require sideways jumps.

**IA-006: No how-to page links back to the README; the README is the only hub and reaches it are one-way.**
- **Principle:** Dan Brown Principle of Multiple Classification (a structure with bidirectional links); EPPO ("how does the reader get to the next thing or back to a hub?"); information scent (a "back to top" hub gives readers an out).
- **Location:** All 20 how-to pages — none of them contain a link to `../../README.md` or to the new hub.
- **Evidence:** Grepping the in-scope set for `../../README.md` or `../README.md` returns zero matches. The README points outward 13 times; nothing points back.
- **Reader Impact:** Audience A2/A3 deep-linked into a how-to from search has no labeled path back to "the index of how-tos". They use the browser back button or the GitHub breadcrumb — neither of which is documentation.
- **Related questions:** Q-AR2 (answered).
- **Severity:** Friction.
- **Remediation:** Every how-to page gets a small "← Back to How-To Guides" link in its `## Related Documentation` section (or at the page top), pointing to the new `docs/how-to/README.md` hub.

### Protocol 4 — Topic Typing and Information Model

**IA-007: Two how-to pages have no Related-Documentation section, leaving them as terminal dead-ends.**
- **Principle:** EPPO ("bidirectional cross-references"); DITA topic-type completeness; *Orphan Topic* (outbound side).
- **Location:** `setting-step-timeouts.md` ends at line 73 after "Advisory prompt budget" with no Related-Documentation section. `caching-build-artifacts.md` ends at line 106 after "Target Project `.gitignore`" with no Related-Documentation section.
- **Evidence:** Every other how-to (18 of 20) has a `## Related documentation` (or similar) section as the last H2. `setting-step-timeouts.md` last line: "while the `timeoutSeconds` cap is always enforced by the runtime." `caching-build-artifacts.md` last line: "...artifact cache created separately by the Docker sandbox preflight; it lands at the project root alongside `.pr9k/`." Both pages stop cold.
- **Reader Impact:** Audience A3 reading either page reaches the bottom and has no labeled out — no link to the related "Recovering from Step Failures" (timeouts), to "Resuming Sessions" (timeouts), to "Passing Environment Variables" (caching uses `containerEnv`), or to "Building Custom Workflows" (caching is a custom-workflow concern). The reader has to remember how they got here.
- **Related questions:** Q-OR1 (answered).
- **Severity:** Friction.
- **Remediation:** Add a `## Related documentation` section to both pages. For `setting-step-timeouts.md`: link to `recovering-from-step-failures.md`, `resuming-sessions.md`, `building-custom-workflows.md`, `debugging-a-run.md`. For `caching-build-artifacts.md`: link to `passing-environment-variables.md` (the `containerEnv` mechanism), `setting-up-docker-sandbox.md` (mount layout), `building-custom-workflows.md`, `debugging-a-run.md`.

**IA-008: `building-custom-workflows.md` opens with the full step-schema reference table before the reader has read any conceptual frame or seen a minimal task-shaped example.**
- **Principle:** DITA topic-type boundary (Reference jammed into Tutorial slot); *Reference-As-Tutorial* anti-pattern; Dan Brown Principle of Disclosure (general → specific).
- **Location:** `building-custom-workflows.md:5-34` — opens with `## Step Configuration Files` (3 lines explaining what the file contains), then immediately a `## Step Schema` H2 with an 11-row reference table covering `name`, `isClaude`, `model`, `promptFile`, `command`, `captureAs`, `captureMode`, `breakLoopIfEmpty`, `skipIfCaptureEmpty`, `timeoutSeconds`, `resumePrevious`, `env`. Each cell references a separate how-to. The first end-to-end "minimal workflow" example does not appear until line 112 (`### 2. Define your steps in JSON`).
- **Evidence:** A reader following the page top-down has to absorb 11 fields with cross-refs to 5 other how-tos before seeing what a complete `config.json` looks like or being told that steps run in array order.
- **Reader Impact:** Audience A3 (workflow customizer) — first-time author of a custom workflow — wants "show me the minimum viable file, then explain the fields." Today's order forces them to context-switch into the schema before they have a mental model of what they're configuring.
- **Related questions:** Q-US1 (answered).
- **Severity:** Degrades comprehension.
- **Remediation:** Reorder. Open with: (1) "Where is `config.json`" + array-order fact (already there); (2) the minimal example currently at line 112-124 — show the smallest valid workflow first; (3) initialize/iteration/finalize phase explanation (currently at line 68-74); (4) a second example showing a Claude step alongside a shell step; (5) **then** the full step-schema reference table, framed as "Field reference (every option)". Keep the table — just demote it from the page's opening to its reference appendix.

**IA-009: `variable-output-and-injection.md` opens with engine internals before the reader's task is named.**
- **Principle:** *Curse-of-Knowledge Prose*; *Context Collapse*; DITA Concept-vs-Reference boundary; information scent (the title says "Variable Output and Injection" but the opening explains Go function names).
- **Location:** `variable-output-and-injection.md:7-11` — "All `{{VAR_NAME}}` tokens in prompt files and shell command arguments are expanded at runtime using the `vars.Substitute` function. Substitution is applied by: `steps.BuildPrompt()` — replaces tokens... `workflow.ResolveCommand()` — replaces tokens..."
- **Evidence:** The first three names in the body are Go internals (`vars.Substitute`, `steps.BuildPrompt`, `workflow.ResolveCommand`). The audience for this page is workflow customizers (A3), not pr9k contributors. The names appear before any task example, before any motivating sentence about why a reader cares, before any statement of the page's job. Compare to `capturing-step-output.md:3-5`, which opens with: "This guide shows how to use `captureAs` to bind a step's stdout to a variable so later steps can reference it via `{{VAR_NAME}}` substitution." — that opening names the user task in the first sentence.
- **Reader Impact:** Audience A3 reading top-down sees Go function names and concludes "this page is for pr9k contributors, not for me." They bounce, even though the rest of the page (built-in variable table, capture pattern, file-passing) is exactly what they need.
- **Related questions:** Q-PK1 (answered).
- **Severity:** Degrades comprehension.
- **Remediation:** Replace the engine-internals opener with a task-shaped opener. Move the `vars.Substitute` / `steps.BuildPrompt` / `workflow.ResolveCommand` mentions into a "Implementation note" callout at the bottom of the section (or delete entirely — those names belong in `docs/code-packages/vars.md`, which is already linked at line 208). Keep the Built-in/Iteration variable tables, the substitution example, the `WORKFLOW_DIR`/`PROJECT_DIR` post-split semantics, and the file-passing model — those serve the audience.

**IA-010: `getting-started.md` mixes installation tutorial with TUI primer, keyboard reference table, and "where to go next" — five distinct topic types in 144 lines.**
- **Principle:** *Everything-at-Once Intro*; DITA topic-type boundary (Tutorial + Concept + Reference + Pointer-to-other-pages all in one); *Progressive-Disclosure Failure*.
- **Location:** `getting-started.md:14-39` (Install — Tutorial), `getting-started.md:41-69` (First run — Tutorial), `getting-started.md:71-86` (Pointing at a different workflow — Reference/Concept), `getting-started.md:88-116` (TUI primer — Concept; duplicates `reading-the-tui.md`), `getting-started.md:118-133` (Keyboard controls table — Reference; duplicates `reading-the-tui.md` and other pages), `getting-started.md:135-144` (Where to go next — Pointer list).
- **Evidence:** The page is the recommended first read but covers install + first run + alt workflow override + TUI layout + keyboard controls + next-steps list. The TUI primer at lines 88-116 partially duplicates `reading-the-tui.md`. The keyboard controls table at lines 118-133 partially duplicates content in `reading-the-tui.md`, `recovering-from-step-failures.md`, and `quitting-gracefully.md`.
- **Reader Impact:** Audience A2 doing a first run reads through, hits the TUI primer, and either (a) reads it now (slowing the install) or (b) skims and later doesn't know whether to consult `getting-started.md` or `reading-the-tui.md` for "what does `[▸]` mean again?" Two-source-of-truth risk.
- **Related questions:** Q-US1 (answered), Q-CO2 (answered).
- **Severity:** Friction.
- **Remediation:** Split. Keep `getting-started.md` as a **strict tutorial**: prerequisites → install → sandbox setup pointer → first run → "Open `reading-the-tui.md` next" link. Move the TUI primer at lines 88-116 wholesale into `reading-the-tui.md` (it duplicates content that exists there; delete the duplicate). Move the keyboard-controls-at-a-glance table at lines 118-133 into `reading-the-tui.md` as a single canonical reference, and have `recovering-from-step-failures.md` / `quitting-gracefully.md` point to that canonical table.

**IA-011: `reading-the-tui.md` mixes operator concept (regions, modes) with engine implementation references that belong in feature docs.**
- **Principle:** DITA topic-type boundary; *Curse-of-Knowledge Prose* (mild).
- **Location:** `reading-the-tui.md:34` references `HeaderProxy` and `program.Send`. Line 38: "When the workflow enters a new phase, `SetPhaseSteps` swaps...". Line 51: "uses a `noopHeader` during `Orchestrate`". Line 73: "This is a `bubbles/viewport` sub-model".
- **Evidence:** The page is a user-facing how-to — yet identifies internal Go types (`HeaderProxy`, `SetPhaseSteps`, `noopHeader`, `bubbles/viewport`) that the audience cannot use and does not need.
- **Reader Impact:** Friction-level. Audience A4 (operator) skips these names but they pollute the scent. Most readers can ignore them, but the page is denser than its task requires.
- **Related questions:** Q-PK1 (answered).
- **Severity:** Friction.
- **Remediation:** Move the engine-name references to `docs/features/tui-display.md` (already linked at line 256). The user-facing how-to should describe what the reader sees, not the type names that produce it.

### Protocol 5 — Hierarchy and Progressive Disclosure

**IA-012: README's "Quick Start" at `README.md:28-40` shows the success path before the prerequisites have been verified, and Docker is missing entirely from the prerequisite list.**
- **Principle:** *Progressive-Disclosure Failure* (required first-run information appearing only after the user has run the wrong command); Dan Brown Principle of Disclosure.
- **Location:** `README.md:11-16` (Prerequisites: Go, gh, Claude CLI, GitHub repo with ralph-labeled issues — no Docker). `README.md:28-40` (Quick Start: shows `pr9k` invocation that will fail without Docker because the preflight enforces it — see `setting-up-docker-sandbox.md:204-211`).
- **Evidence:** The README's prerequisites contradict `getting-started.md:8` ("Docker... is a **required** runtime dependency, not optional") and `setting-up-docker-sandbox.md:1-3` ("Docker is required — there is no fallback to direct invocation"). A reader who follows the README's prerequisites alone will see the preflight reject their first run.
- **Reader Impact:** Audience A1 (evaluator) and A2 (installer): they hit a prerequisite that the front door never named. The error is recoverable (the preflight prints a helpful message), but it is the kind of papercut that gets remembered.
- **Related questions:** Q-CC1 (answered), OQ-1 (open).
- **Severity:** Degrades comprehension. (Escalates to Blocks if support data shows this is the #1 first-run friction.)
- **Remediation:** Add Docker to README prerequisites at `README.md:11-16` with a one-line pointer to `setting-up-docker-sandbox.md`. Resolve OQ-1 to confirm the requirement is unconditional. While editing, align the README's "Run the orchestrator" section at `README.md:46-65` with `getting-started.md:50-66` — they currently differ in subtle ways (README mentions `--project-dir`, getting-started mentions `--workflow-dir`).

**IA-013: `setting-step-timeouts.md` introduces `onTimeout` inside a code block before its policy section explains it.**
- **Principle:** Information scent; *Curse-of-Knowledge Prose* (mild).
- **Location:** `setting-step-timeouts.md:25-26` (the example config block adds `"onTimeout": "continue"` to the JSON sample). Lines 42-50 (the "Soft-fail on timeout" section that explains what `onTimeout` is and the `""`/`"fail"`/`"continue"` values).
- **Evidence:** The example introduces a field whose semantics are not defined for ~16 more lines. Most readers will scan past, but a careful reader pauses on "what does `continue` mean here?" and has to scroll forward.
- **Reader Impact:** Friction-level for audience A3.
- **Related questions:** Q-PK2 (answered).
- **Severity:** Polish.
- **Remediation:** Either move the `onTimeout` line out of the introductory example (showing only `timeoutSeconds: 1800` first) or add a one-line forward-reference: "(`onTimeout` is explained in the next section)".

### Protocol 6 — Labeling and Navigation Systems

**IA-014: The filename `variable-output-and-injection.md` and its title carry weak information scent for the audience task.**
- **Principle:** Rosenfeld/Morville labeling system; information scent; *Ghost Navigation* (mild).
- **Location:** `docs/how-to/variable-output-and-injection.md` (filename); `variable-output-and-injection.md:1` (title `# Variable Output and Injection`).
- **Evidence:** "Injection" is a programming term (think SQL injection / dependency injection) that does not predict the page's actual content for a workflow-customizer reader. The page is about: (a) how `{{VAR}}` tokens are substituted in prompts and commands, (b) the built-in variable list, (c) iteration vs persistent variable scope, (d) file-based data passing between steps. None of those concepts naturally surface from the word "injection". Compare with `capturing-step-output.md` — title and filename clearly predict the task.
- **Reader Impact:** Audience A3 scanning the README's how-to list looks for "how do I use `{{ISSUE_ID}}` in my prompt?" and may skip past "Variable Output and Injection" because it sounds engine-internal. Friction.
- **Related questions:** Q-PK1 (answered).
- **Severity:** Friction.
- **Remediation:** Rename to `using-variables-in-prompts-and-commands.md` (or `workflow-variables.md`, kept short). Title becomes `Using Variables in Prompts and Commands`. Update inbound references (the audit counts 8 inbound how-to references plus the README at `README.md:92`). Or split the page along its two distinct jobs (substitution + file-passing) — see also IA-018 below.

**IA-015: The operator-facing pages do not mention the in-app `?` help modal, even though the modal exists and shows the canonical keybinding reference.**
- **Principle:** Cross-channel consistency; information scent (multiple paths to the same answer).
- **Location:** `recovering-from-step-failures.md`, `quitting-gracefully.md`, `copying-log-text.md` — none of these pages mention pressing `?` while in the TUI to see the keybinding modal. The `?` modal is described only in `reading-the-tui.md:194-200` and `using-the-workflow-builder.md:138`.
- **Evidence:** A reader in error mode wondering "what does `r` do?" cannot tell from `recovering-from-step-failures.md` that the modal even exists. They have to leave the running TUI to read the docs to learn that pressing `?` would have shown them the answer in-app.
- **Reader Impact:** Audience A4 (operator during a run) — small but real friction.
- **Related questions:** Q-CO2 (answered).
- **Severity:** Polish.
- **Remediation:** Add a one-line "Press `?` in the TUI to see the live keybinding reference" footer to the operator-facing pages.

### Protocol 7 — Every-Page-Is-Page-One Check

**IA-016: `getting-started.md`'s "Where to go next" list at lines 135-144 mixes prerequisites, strict next-steps, and advanced features without ordering.**
- **Principle:** EPPO (a page should send the reader somewhere useful next); Dan Brown Principle of Disclosure (general → specific); information foraging (the highest-scent option should be first).
- **Location:** `getting-started.md:135-144` — the "Where to go next" section lists 7 bullets in this order: Docker sandbox (prerequisite), Building custom workflows (next major step), Variable substitution (advanced concept), Capturing step output (advanced concept), Forwarding env vars (advanced concept), Breaking out of the loop (advanced concept), Reading the run's log (debugging), Architecture (contributor docs).
- **Evidence:** Bullet 1 ("Setting up the Docker sandbox") is something most readers should have done **before** the first run. Bullet 8 (Architecture overview) is contributor-facing. The list mixes audiences and prerequisites with next-step advice.
- **Reader Impact:** Audience A2 finishing first run cannot tell which link to follow first. They might read the Docker sandbox page (already done), then jump to env vars (premature), then to architecture (wrong audience).
- **Related questions:** Q-EC1 (answered).
- **Severity:** Friction.
- **Remediation:** Replace the flat list with a 3-tier structure: **Recommended next:** (1 link to "Reading the TUI"). **Customize your workflow:** (3 links to building-custom + variables + capturing). **Operate and debug:** (2 links to recovering + debugging). Drop the Architecture link entirely from this section (audience mismatch).

**IA-017: `caching-build-artifacts.md` does not state its audience or prerequisites at the top.**
- **Principle:** *Context Collapse*; EPPO orientation.
- **Location:** `caching-build-artifacts.md:1-8` — opens with "## The Problem" and a description of permission errors, with no indication of who this is for or what the reader should already know.
- **Evidence:** The page assumes the reader knows what `containerEnv` is (links to `passing-environment-variables.md`'s `containerEnv` section but the link is implicit), what the bind-mounted project directory is, what UID mapping is, and why caches matter for unattended runs. None of those prerequisites are stated.
- **Reader Impact:** Audience A3 landed cold (e.g., from a Google search for "claude container go cache permission denied") gets task content but no orientation. They cannot tell whether this page is the right starting point or whether they need to read something else first.
- **Related questions:** Q-OR1 (answered).
- **Severity:** Friction.
- **Remediation:** Add a 2-3 line opener: "**Audience:** custom-workflow authors. **Prerequisites:** read [Passing Environment Variables](passing-environment-variables.md) for the `containerEnv` mechanism. **When to use:** if your unattended runs hit `permission denied` cache errors or you want build caches to persist across iterations." Then keep the existing Problem/How-It-Works structure.

### Protocol 8 — Minimalism Sweep

**IA-018: `variable-output-and-injection.md` covers two distinct user tasks — `{{VAR}}` substitution and file-based data passing — that warrant separate pages or a clearer split.**
- **Principle:** Carroll minimalism (task-oriented chunking); DITA topic-type boundary.
- **Location:** `variable-output-and-injection.md:5-100` (Substitution Engine + Built-ins + post-split semantics + finalization rules). Lines 119-159 (File-Based Data Passing — `progress.txt`, `deferred.txt`, `test-plan.md`, handoff lifecycle, data-flow diagram).
- **Evidence:** Sections 1 ("{{VAR}} Substitution Engine") and 3 ("File-Based Data Passing Between Steps") solve different reader problems. A reader who came for "how do I pass data between steps via files" is forced to scroll past 100 lines of substitution semantics. A reader who came for "what does `{{ISSUE_ID}}` mean" doesn't need the file-passing model.
- **Reader Impact:** Audience A3 is forced into a longer-than-necessary read for either task. Discovery suffers — a future reader searching for "progress.txt" won't think to open `variable-output-and-injection.md`.
- **Related questions:** Q-US1 (answered), Q-PK1 (answered).
- **Severity:** Friction.
- **Remediation (conservative):** Keep one page but split the title into a clearer two-section frame: rename to `Workflow Variables and Step Data` (or split into two pages: `workflow-variables.md` for substitution and `passing-data-between-steps.md` for file-based handoffs). The latter is cleaner and aligns with single-task-per-page. Either choice fixes IA-014's labeling problem and IA-009's curse-of-knowledge opener.

**IA-019: README's `## How To` mid-section at `README.md:44-82` duplicates content already in `getting-started.md` and the keyboard-controls section of `reading-the-tui.md`.**
- **Principle:** Carroll minimalism (cut redundancy); single-source-of-truth; Dan Brown Principle of Disclosure (the README should orient, not duplicate).
- **Location:** `README.md:44-82` — `### Run the orchestrator` (duplicates `getting-started.md:50-66`) and `### Keyboard controls (TUI)` (duplicates `getting-started.md:118-133` AND `reading-the-tui.md:204-217`).
- **Evidence:** Three sources of truth for "what does `q` do" — README, getting-started, reading-the-tui. When the keybinding map drifts (e.g., the `v` Select-mode entry was added), all three need updating in lockstep. (Spot-check: README at line 71-73 lists `↑/k`, `↓/j`, `n`, `q` but does **not** mention `v` for Select mode. `reading-the-tui.md:209` does. Drift confirmed.)
- **Reader Impact:** Audience A4 reading the README for keyboard help gets a stale subset (no `v`). Audience A2 reading the README's How-To section thinks they already learned the keyboard map and never opens `reading-the-tui.md`, which has the canonical version.
- **Related questions:** Q-CC1 (answered), Q-CO2 (answered).
- **Severity:** Degrades comprehension.
- **Remediation:** Replace `README.md:44-82` with a tight 4-line "Quick Start" pointer block: "After install, run `pr9k` from your target repo. For a guided first run see [Getting Started]; for the keyboard map see [Reading the TUI]; for failure recovery see [Recovering from Step Failures]." Delete the embedded keyboard table.

### Protocol 9 — Recency and Cross-Reference Integrity

Cross-reference integrity (in-scope set, internal links only):
- All in-scope `.md` link targets resolve to files that exist (spot-checked: `../features/...`, `../code-packages/...`, `../adr/...`, sibling `how-to/...md` paths).
- One mismatch worth noting: `README.md:107` links to `keyboard-input.md` ("Eight-mode keyboard state machine") but `copying-log-text.md:97` describes it as a "Seven-mode state machine". This is a feature-doc concern (out of scope) but the README's claim and the how-to's claim disagree — a reader who notices will lose trust. Polish.
- `caching-build-artifacts.md` is one-way linked from `passing-environment-variables.md:127` but does not link back; `setting-step-timeouts.md` is one-way linked from `resuming-sessions.md:106` but does not link back. Both are covered by IA-007.

Recency-weighted re-prioritization: the most-churned in-scope files (reading-the-tui, building-custom-workflows, README, variable-output-and-injection, configuring-a-status-line) hold IA-001, IA-002, IA-008, IA-009, IA-010, IA-011, IA-012, IA-019. High churn + high finding density → these are the highest-leverage targets for the restructure.

**IA-020: README's "How-To Guides" link list and the `docs/how-to/` directory drift apart with each new how-to addition.**
- **Principle:** Pace layering (the README changes faster than the how-to directory but is also the only inventory); single-source-of-truth.
- **Location:** `README.md:88-100` — manually curated link list whose maintenance burden is invisible at PR time.
- **Evidence:** Six pages on disk are missing from the README list (IA-001). The pattern — "add a new how-to file, forget to update the README" — is the visible mechanism. Each missed update degrades the front door's trustworthiness.
- **Reader Impact:** Cumulative; the longer the drift goes uncorrected, the lower the README's information scent for "what how-tos exist".
- **Related questions:** Q-CC2 (answered).
- **Severity:** Friction (this run); compounds toward Degrades over time.
- **Remediation:** Stop curating the list inside the README. Replace it with a single link to `docs/how-to/README.md`. The hub page is owned by the docs team alongside the how-tos themselves; updating it is part of the same PR that adds a new how-to. (See IA-001.)

---

## IA Improvement Summary

### What Was Found

The pages themselves are well-written: each how-to has a clear topic, useful examples, and (mostly) Related-Documentation cross-references. The architectural problems are at the **inventory** layer (IA-001, IA-007, IA-020), the **front-door** layer (IA-002, IA-003, IA-012, IA-019), the **sequencing** layer (IA-005, IA-006, IA-016), and the **topic-type boundary** layer (IA-008, IA-009, IA-010, IA-011, IA-018). Two pages are terminal dead-ends (IA-007), the README contradicts `getting-started.md` on prerequisites (IA-012), and the README's embedded keyboard map has already drifted from the how-to's canonical version (IA-019). All findings are actionable without rewriting prose — they are structural moves.

### How to Improve

The recommended target shape is a 5-group, task-ordered structure with a single canonical hub at `docs/how-to/README.md`. Order is **not** alphabetical; it follows the brief's task sequence: get oriented → install → first run → customize → operate → debug.

**Proposed `docs/how-to/README.md` outline (the new hub):**

```
# pr9k How-To Guides

Pick the path that matches what you're doing.

## 1. Getting Started — install pr9k and run the default workflow
1. [Getting Started](getting-started.md) — prerequisites, install, first run
2. [Setting Up the Docker Sandbox](setting-up-docker-sandbox.md) — Docker, sandbox image, Claude profile auth
3. [Reading the TUI](reading-the-tui.md) — regions, modes, keyboard map, status line

## 2. Operating a Run — what to do while pr9k is running
4. [Recovering from Step Failures](recovering-from-step-failures.md)
5. [Quitting Gracefully](quitting-gracefully.md)
6. [Copying Log Text](copying-log-text.md)

## 3. Customizing Your Workflow — write your own steps and prompts
7. [Building Custom Workflows](building-custom-workflows.md) — the step schema
8. [Using Variables in Prompts and Commands](workflow-variables.md) — renamed from variable-output-and-injection.md (IA-014)
9. [Capturing Step Output](capturing-step-output.md)
10. [Using the Workflow Builder](using-the-workflow-builder.md)
11. [Configuring an External Editor for the Workflow Builder](configuring-external-editor-for-workflow-builder.md)

## 4. Advanced Step Configuration — opt-in features for production runs
12. [Breaking Out of the Loop](breaking-out-of-the-loop.md)
13. [Skipping Steps Conditionally](skipping-steps-conditionally.md)
14. [Setting Step Timeouts](setting-step-timeouts.md)
15. [Passing Environment Variables](passing-environment-variables.md)
16. [Caching Build Artifacts](caching-build-artifacts.md)
17. [Configuring a Status Line](configuring-a-status-line.md)
18. [Resuming Sessions](resuming-sessions.md) — engine-supported, off in default workflow

## 5. Debugging — reconstruct what happened
19. [Debugging a Run](debugging-a-run.md)
```

(19 pages because the rename of `variable-output-and-injection.md` to `workflow-variables.md` is one file. Optional: split into `workflow-variables.md` + `passing-data-between-steps.md` per IA-018; that would make 20.)

**Concrete change list, ordered by severity-then-leverage:**

1. **(Blocks → Degrades) Add Docker to README prerequisites** at `README.md:11-16`, with a top-of-page pointer to `setting-up-docker-sandbox.md`. Resolve OQ-1 first to confirm the requirement is truly unconditional. (IA-012)

2. **(Degrades) Create `docs/how-to/README.md` as the canonical hub**, populated with the 5-group outline above. Replace the inline link list at `README.md:88-100` with a single link to the hub. (IA-001, IA-004, IA-020)

3. **(Degrades) Split `README.md`'s `## Documentation` section into `## User Guides` and `## Contributor Reference`**, with a one-line audience statement above each. Move feature-docs and code-package links under Contributor Reference. (IA-002)

4. **(Degrades) Reorder `building-custom-workflows.md`**: minimal example first, phase explanation second, Claude+shell example third, full schema reference table demoted to a "Field reference" appendix at the bottom. (IA-008)

5. **(Degrades) Replace `variable-output-and-injection.md`'s engine-internals opener** with a task-shaped opener; rename the file (and update 9 inbound references) to `workflow-variables.md`. Optionally split file-passing into its own page. (IA-009, IA-014, IA-018)

6. **(Degrades) Trim `README.md:44-82`** to a 4-line pointer block. Delete the embedded keyboard table; canonicalize keyboard reference in `reading-the-tui.md`. (IA-019)

7. **(Degrades) Re-sequence the install path**: clarify in `getting-started.md` that Docker sandbox setup happens **before** first run; either inline-pointer it or merge sandbox setup into the getting-started linear flow. Add "Previous: X · Next: Y" lines to the Getting-Started group pages. (IA-005)

8. **(Friction) Add Related-Documentation sections** to `setting-step-timeouts.md` and `caching-build-artifacts.md`. (IA-007)

9. **(Friction) Add "← Back to How-To Guides" link** in every how-to's Related Documentation section. (IA-006)

10. **(Friction) Restructure `getting-started.md`'s "Where to go next" list** into 3 tiers (Recommended Next / Customize / Debug). Remove the Architecture link. (IA-016)

11. **(Friction) Move TUI primer and keyboard table out of `getting-started.md`** into `reading-the-tui.md` (canonical). (IA-010)

12. **(Friction) Add audience+prerequisite opener** to `caching-build-artifacts.md`. (IA-017)

13. **(Friction) Add a "When to use pr9k" framing block** between README lines 7 and 9. (IA-003)

14. **(Polish) Mention the `?` help modal** on operator-facing pages (`recovering-from-step-failures.md`, `quitting-gracefully.md`, `copying-log-text.md`). (IA-015)

15. **(Polish) Demote engine-name references** in `reading-the-tui.md` (`HeaderProxy`, `SetPhaseSteps`, `noopHeader`, `bubbles/viewport`). (IA-011)

16. **(Polish) Reorder or annotate `onTimeout`** in `setting-step-timeouts.md`'s opening example. (IA-013)

**Cross-reference link plan, per how-to (prerequisites in / next-steps out):** the hub already implies the linear sequence; add explicit links where the dependency is non-obvious.

| Page | Add prerequisite link to | Add follow-up link to |
|------|-------------------------|------------------------|
| `getting-started.md` | (none — it is the entry point) | `setting-up-docker-sandbox.md`, `reading-the-tui.md` |
| `setting-up-docker-sandbox.md` | (already links back to getting-started) | `reading-the-tui.md` |
| `reading-the-tui.md` | `getting-started.md` (already) | `recovering-from-step-failures.md`, `quitting-gracefully.md`, `copying-log-text.md` (already) |
| `recovering-from-step-failures.md` | `reading-the-tui.md` (already) | `setting-step-timeouts.md`, `debugging-a-run.md` (already) |
| `quitting-gracefully.md` | `reading-the-tui.md` (already) | (already complete) |
| `copying-log-text.md` | `reading-the-tui.md` (already) | (already complete) |
| `building-custom-workflows.md` | `getting-started.md` (already) | `workflow-variables.md`, `capturing-step-output.md` (already) |
| `workflow-variables.md` (renamed) | `building-custom-workflows.md` (already) | `capturing-step-output.md` (already) |
| `capturing-step-output.md` | `workflow-variables.md` (already as variable-output) | `breaking-out-of-the-loop.md`, `skipping-steps-conditionally.md` |
| `using-the-workflow-builder.md` | `building-custom-workflows.md` (already) | `configuring-external-editor-for-workflow-builder.md` (already) |
| `configuring-external-editor-for-workflow-builder.md` | `using-the-workflow-builder.md` (already) | (terminal — fine) |
| `breaking-out-of-the-loop.md` | `capturing-step-output.md` (already) | `skipping-steps-conditionally.md` |
| `skipping-steps-conditionally.md` | `capturing-step-output.md` (already) | `setting-step-timeouts.md`, `resuming-sessions.md` |
| `setting-step-timeouts.md` | `recovering-from-step-failures.md` **(NEW)** | `resuming-sessions.md` **(NEW)**, `debugging-a-run.md` **(NEW)** |
| `passing-environment-variables.md` | `building-custom-workflows.md` (already) | `caching-build-artifacts.md` (already) |
| `caching-build-artifacts.md` | `passing-environment-variables.md` **(NEW)** | `building-custom-workflows.md` **(NEW)**, `debugging-a-run.md` **(NEW)** |
| `configuring-a-status-line.md` | `reading-the-tui.md` (already) | (terminal — fine) |
| `resuming-sessions.md` | `building-custom-workflows.md`, `setting-step-timeouts.md` (already) | `debugging-a-run.md` (already) |
| `debugging-a-run.md` | `recovering-from-step-failures.md` (already) | (terminal — fine) |

### How to Prevent This Going Forward

1. **Stop curating the link list inside the README.** Move the list into `docs/how-to/README.md`, which sits next to the files it indexes. PR-level reviewers naturally notice "you added a how-to but did not update the directory hub."
2. **Add a doc-template for new how-tos** with required sections: `Audience`, `Prerequisites`, `Task` (the body), `Related documentation`. Two of today's pages would have failed this template (IA-007, IA-017).
3. **Add a tiny CI check** that every `docs/how-to/*.md` is linked at least once from `docs/how-to/README.md`. Catches IA-001-class drift at PR time.
4. **Add a tiny CI check for terminal pages**: every how-to ends with a `## Related documentation` H2. Catches IA-007.
5. **Keyboard-map single source of truth**: `reading-the-tui.md` is canonical; other pages link to its anchor instead of duplicating the table. (Today: README, getting-started, reading-the-tui all maintain copies — IA-019.)
6. **Add a brief contributor note in `docs/coding-standards/documentation.md`**: "When you add or rename a how-to, update `docs/how-to/README.md` in the same PR." Aligns with the project's existing standard that "feature docs must ship with the feature, not as follow-ups".
7. **Tree-test the proposed grouping** with 5–10 readers (or use a card-sort: write each how-to title on a card, ask new users to group them). Validates the 5-group structure before adoption.

### Balancing Shipping vs Improving

**Must-fix-now (blocks or degrades comprehension on the most-trafficked pages):**

- IA-012 (README missing Docker prerequisite — contradicts getting-started; first-run friction)
- IA-001 + IA-004 + IA-020 (create `docs/how-to/README.md`, link from main README, stop curating inline) — one structural move resolves three findings
- IA-007 (add Related-Documentation sections to the two terminal pages — 5-minute fix, prevents future deep dead-ends)

**Track-and-improve (degrades or friction; not blocking adoption but improves first-time success rate):**

- IA-002 + IA-019 (split README's Documentation section; trim duplicated content)
- IA-008 (reorder `building-custom-workflows.md`)
- IA-009 + IA-014 + IA-018 (rename + reopen + maybe split `variable-output-and-injection.md`)
- IA-005 + IA-006 + IA-016 (sequence signposting + back-to-hub links + fix Where-to-go-next list)
- IA-010 + IA-017 (split getting-started's TUI primer; orient caching-build-artifacts)

**Polish (do alongside any of the above):**

- IA-003 (When-to-use framing on README)
- IA-011 (engine-name cleanup in reading-the-tui)
- IA-013 (onTimeout intro reorder)
- IA-015 (mention `?` modal on operator pages)

**One-PR scope recommendation:** ship the four must-fix-now items as a single docs PR (no prose rewrites — only structural moves and the two missing Related-Documentation sections). Track the rest as follow-up tickets with finding IDs cited so each PR is small and reviewable. The OQ-1 (Docker required?) and OQ-2 (builder pages experimental?) and OQ-3 (resumePrevious advanced?) questions should be answered before the must-fix-now PR is opened, so the README's grouping reflects product intent.
