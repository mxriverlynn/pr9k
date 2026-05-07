# Review Findings: useWorktrees feature specification (iterative-plan-review)

This file records every finding raised by the iterative-plan-review team for [`feature-specification.md`](../feature-specification.md). Findings produced during the spec's original drafting (`han:plan-a-feature`) live in [`team-findings.md`](team-findings.md); the round-by-round record of this review lives in [`review-iteration-history.md`](review-iteration-history.md). Plan edits are summarized in `Changed in plan:`; tech-notes edits in `Changed in tech-notes:`.

Numbering starts at F1 for this companion file; it is independent of the F-NN / JD-NNN / V-NN / F-USER-NN scheme used in `team-findings.md`.

## Major findings

### F1: Branch naming inconsistency — `pr9k-<stamp>` (hyphen) vs `pr9k/<stamp>` (slash) used inconsistently across spec, decisions, and tech-notes

- **Agent:** junior-developer + evidence-based-investigator (independently surfaced)
- **Category:** internal contradiction
- **Finding:** The branch name format is inconsistent. Hyphen form (`pr9k-<worktree-stamp>`) appears in the spec Background, Primary Flow step 4, Edge Cases, the `--fresh` Alternate Flow, T4 schema, D13's cleanup command, and D16. Slash form (`pr9k/<run-stamp>`) appears in D2, T2, D5's branch-discovery aside, D14, and the Coordinations table. The two forms are not equivalent — `pr9k/foo` is a hierarchical (namespaced) ref while `pr9k-foo` is a flat ref. Implementation cannot proceed until one is chosen, and several decisions/tech-notes describe filter mechanics that depend on which is chosen.
- **Evidence considered:** spec Background ¶2; Primary Flow steps 4, 12; Alternate Flow `--fresh` step 2; Edge Cases (`pr9k-<worktree-stamp>` collision row); T2 detail; T4 schema field set; D2; D5 ("pr9k/-prefixed branches as a discovery aid"); D13; D14; Coordinations row "Git (remote)".
- **Resolution:** Standardise on the **hyphen form `pr9k-<worktree-stamp>`** throughout. Reasons: (1) T4's schema field is hyphen and is load-bearing for the state-file contract; (2) D13's cleanup command is hyphen; (3) D5's stale-detection is path-prefix-only after F-03 — the `pr9k/` slash-prefix branch-discovery rationale that T2 and D2 cited is no longer load-bearing (see F14). Updated D2, T2, D5, D14, and Coordinations to hyphen.
- **Resolved by:** evidence
- **Raised in round:** R1
- **Changed in plan:** Coordinations (Git remote row); see also F14 for T2/D5 ripple.
- **Changed in tech-notes:** T2 (branch-name examples), T4 (no change — already hyphen).

### F2: `--fresh` worktree-removal conditional ("if worktrees.enabled is true") creates orphan worktrees when config is flipped between runs

- **Agent:** junior-developer + adversarial-validator + edge-case-explorer (V4, EC-2)
- **Category:** internal contradiction / unhandled edge case
- **Finding:** Primary Flow step 3 says `--fresh` "removes the state file (and the worktree it pointed to, if `worktrees.enabled` is true)." D19 in the decision log uses the same conditional. The Alternate Flow for `--fresh` (Alternate Flows section) says removal is unconditional. If the user ran with `enabled: true` (state file + worktree created), then flipped `enabled: false`, then ran `pr9k --fresh`, the state file is removed but the worktree is not — orphaning a worktree that subsequent runs cannot detect (because D5 stale-detection runs only when `enabled: true`).
- **Evidence considered:** Primary Flow step 3 (`--fresh` branch); Alternate Flows (`pr9k --fresh`); D19; Edge Cases (no row covers this); D5 (stale-detection scope).
- **Resolution:** Drop the conditional. `--fresh` removes the worktree referenced in the state file unconditionally (the state file's existence proves a prior `enabled: true` run created the worktree; the current `enabled` value cannot undo that). Updated Primary Flow step 3, Alternate Flows `--fresh` section, and D19. Linked to F10 (still want a warning when state file is present and `enabled: false` without `--fresh`).
- **Resolved by:** evidence
- **Raised in round:** R1
- **Changed in plan:** Primary Flow (step 3, `--fresh` bullet), Alternate Flows (`pr9k --fresh`), D19.
- **Changed in tech-notes:** —

### F3: D17 consumers (`post_issue_summary`, `lessons-learned`) hardcode `iteration.jsonl`; `RunStamp()` is not exported to subprocesses; `lessons-learned` also truncates the path D17 renames

- **Agent:** adversarial-validator + evidence-based-investigator (V1, V5, EBI claim 3, EBI claim 4, I4)
- **Category:** internal contradiction (load-bearing for resume safety)
- **Finding:** D17 commits to per-invocation files at `<worktree>/.pr9k/iteration-<invocation-stamp>.jsonl` and claims consumers use the current invocation's file "naturally derived from `RunStamp()`." But `RunStamp()` is a Go runtime value not exported to subprocesses (`workflow/scripts/post_issue_summary` line 11 hardcodes `JSONL_FILE=".pr9k/iteration.jsonl"`; `workflow/prompts/lessons-learned.md` line 1 hardcodes `@.pr9k/iteration.jsonl` and line 8 instructs Claude to truncate that path). After D17 is implemented, the hardcoded paths point to non-existent files — the spec's resume/forensic-value claim is not actually satisfied by any cited mechanism, and the truncate instruction targets a path that no longer exists.
- **Evidence considered:** `workflow/scripts/post_issue_summary:11`; `workflow/prompts/lessons-learned.md:1,5,8`; `src/internal/workflow/iterationlog.go:48` (`AppendIterationRecord` hardcodes `iteration.jsonl`); `src/internal/logger/logger.go:33-63` (`RunStamp()` is per-`Logger`); D17 rationale.
- **Resolution:** The spec adopts a **stable alias** behaviour: every pr9k invocation maintains a stable `<worktree>/.pr9k/iteration.jsonl` filename for the current invocation, while preserving prior invocations' records under stamped filenames `iteration-<invocation-stamp>.jsonl` for forensic value. The mechanic — how the alias is realised (POSIX symlink, file copy, etc.) — is captured in a new T5. The behavioural commitment is: consumers read `iteration.jsonl` as today; that path always reflects the current invocation; prior invocations' records persist under stamped filenames alongside, removed only by autoCleanup or `pr9k worktree prune`. Lessons-learned's truncate instruction is removed (per-invocation files are cleaned up by autoCleanup/prune, not by truncation). Updated D17 rationale, Outcome, Primary Flow step 6, Edge Cases (per-invocation accumulation row), and added T5.
- **Resolved by:** evidence
- **Raised in round:** R1
- **Changed in plan:** Outcome (per-invocation iteration log bullet), Primary Flow (step 6), Edge Cases (per-invocation accumulation row), D17.
- **Changed in tech-notes:** T5 (new — stable iteration-log alias).

### F4: D7's push-failure surfacing and D18's step reorder are required prerequisites for resume safety, but OI-1 marks the push behaviour change "non-blocking"

- **Agent:** adversarial-validator (V2)
- **Category:** internal contradiction
- **Finding:** D7 commits to "push step surfaces non-zero exit codes; on failure, pr9k enters error-recovery mode" and D18 commits to push-before-close ordering. D15's resume safety relies on both. OI-1 currently asks "should the in-place workflow path also gain push-failure surfacing?" but its framing is ambiguous about whether the worktree-mode push fix is itself blocking. The current `git_push` script (`workflow/scripts/git_push:2,8`) traps all exits to zero — it does not surface failure today. Without an explicit "this script must change as part of this feature" commitment, the implementation could ship the worktree feature without the script fix, leaving resume unsafe.
- **Evidence considered:** OI-1 text; D7 commitment; D18 commitment; `workflow/scripts/git_push` (current trap-to-zero behaviour); `workflow/config.json:28-29` (current `Close issue` then `Git push` order).
- **Resolution:** Reframed OI-1 to scope only the **in-place** workflow's push-failure surfacing as the open question. The worktree-mode push surfacing AND the step reorder are both **in scope** for the worktree feature and **must ship together with it**. Added an explicit "implementation prerequisites" callout to D7 and D18 in the spec body so the dependency is unmissable.
- **Resolved by:** evidence
- **Raised in round:** R1
- **Changed in plan:** Open Items (OI-1), Coordinations (Git remote row, prerequisite call-out), Outcome (push-failure prerequisite explicitly named).
- **Changed in tech-notes:** —

### F5: Concurrent-run TOCTOU race window between state-file read (Primary Flow step 3) and write (step 7) is unguarded

- **Agent:** adversarial-validator + edge-case-explorer (V3, EC-6)
- **Category:** unhandled edge case
- **Finding:** Two pr9k invocations starting in close succession both execute Primary Flow step 3 (read state file → ENOENT) before either reaches step 7 (write state file). Both proceed as fresh, both create worktrees at distinct stamped paths, both write state files (last write wins atomically). The first instance's worktree is orphaned from the state file; D10's PID + binary check protects only the case where the state file already exists. The Deferred lock-file entry acknowledges concurrency exists but does not describe the actual failure outcome.
- **Evidence considered:** Primary Flow steps 3 & 7; D10; Deferred (concurrent-run lock file); `src/cmd/pr9k/main.go:298` (no atomic compare-and-swap exists).
- **Resolution:** Added an Edge Cases row that explicitly names the race window and the resulting state (last write wins, first instance loses its lock anchor, first worktree becomes detectable as stale on next run via D5). Updated the Deferred (concurrent-run lock file) entry's reopen trigger to include this race. The Precondition that "only one pr9k instance should run at a time" already names the user expectation; the new row makes the failure mode visible without committing to a lock file.
- **Resolved by:** evidence
- **Raised in round:** R1
- **Changed in plan:** Edge Cases (new TOCTOU row), Deferred (concurrent-run lock file — reopen trigger expanded).
- **Changed in tech-notes:** —

### F6: Worktree's inheritance of remote git config from the primary is load-bearing for resume's `gh` operations but unstated

- **Agent:** junior-developer
- **Category:** unstated assumption
- **Finding:** The Coordinations table asserts "All `gh` operations resolve the repository from the worktree's git state. No `gh` configuration changes are needed." Resume relies on this: `get_next_issue` running inside the resumed worktree must reach the same GitHub repository. The fact rests on git's worktree model — worktrees share the repository's `.git/` directory, so `git config`, `git remote`, and ref state are shared. This is not stated in the Background or any T#, leaving a load-bearing dependency unanchored.
- **Evidence considered:** Coordinations table (GitHub row); Background; T2 (branch uniqueness, but not config inheritance); `git worktree` documentation (referenced via investigation/01).
- **Resolution:** Added a one-sentence statement to the Background section ("Worktrees share the repository's remotes, hooks, and `git config`...") and cross-referenced it from the Coordinations row.
- **Resolved by:** evidence
- **Raised in round:** R1
- **Changed in plan:** Background, Coordinations (GitHub via `gh` row).
- **Changed in tech-notes:** —

### F7: `--fresh` interaction with the concurrent-run guard is unspecified — `--fresh` while another pr9k is alive can destroy live state

- **Agent:** edge-case-explorer (EC-1)
- **Category:** unhandled edge case
- **Finding:** Spec says concurrent runs (state file + alive PID + matching binary) cause the new invocation to exit. It also says `--fresh` "removes the state file" unconditionally and that the user can "override with `--fresh` if they know the prior process is no longer cooperating." The interaction is undefined: does `--fresh` re-check the liveness gate, or bypass it? If bypass, `--fresh` can remove a running pr9k's state file out from under it.
- **Evidence considered:** D10; D19; Primary Flow step 3 (`--fresh` and concurrent-run guard interaction); Edge Cases.
- **Resolution:** Specified that `--fresh` honours the concurrent-run guard: when the state file's PID is alive and its binary matches the running pr9k, `--fresh` exits with the same "another pr9k appears to be running" error and recommends terminating the live process first. The "override if no longer cooperating" wording is removed because the failure mode (live PID + matching binary) is exactly the case where `--fresh` would damage the running process. Added an Edge Cases row.
- **Resolved by:** evidence
- **Raised in round:** R1
- **Changed in plan:** Primary Flow (step 3, `--fresh` bullet), Alternate Flows (`pr9k --fresh`), Edge Cases (concurrent + `--fresh` row), User Interactions (Error states), D19.
- **Changed in tech-notes:** —

### F8: `pr9k worktree prune` invoked from inside an active worktree resolves the wrong "primary" — risks removing the worktree the user is currently inside

- **Agent:** edge-case-explorer (EC-3)
- **Category:** unhandled edge case
- **Finding:** Spec says the active-worktree exclusion uses `<primary>/.pr9k/active-run.json`. If the user `cd`'s into the active worktree before running `pr9k worktree prune`, the resolved primary is the worktree itself; the state file lookup misses; the active worktree is not excluded; and `prune` removes the directory the user is operating from.
- **Evidence considered:** D14; Alternate Flow (`pr9k worktree prune`); Edge Cases (no row covers this); Coordinations row.
- **Resolution:** Specified that `pr9k worktree prune` detects when its CWD matches the `<basename>-pr9k-*` pattern, walks up to the primary's parent, and resolves the primary from there before continuing. Added a precondition note to the Alternate Flow and an Edge Cases row.
- **Resolved by:** evidence
- **Raised in round:** R1
- **Changed in plan:** Alternate Flows (`pr9k worktree prune`), Edge Cases (prune-from-inside-worktree row), D14.
- **Changed in tech-notes:** —

### F9: Resume when state file points to existing worktree but its branch was deleted manually — no handler

- **Agent:** edge-case-explorer (EC-4)
- **Category:** unhandled edge case
- **Finding:** Spec covers the case where the worktree directory is gone (orphan-cleanup-then-fresh). It does not cover the case where the worktree directory is present but the branch named in the state file no longer exists (user ran `git branch -D pr9k-<stamp>` manually). pr9k would enter the worktree and fail at the first `git` operation with an opaque error.
- **Evidence considered:** Primary Flow step 3; Alternate Flows; Edge Cases.
- **Resolution:** Added a state-file-validation step to Primary Flow step 3: when entering an existing worktree on resume, verify the branch named in the state file still exists. If missing, treat as orphaned-state (rename state file with `.corrupt-` prefix, log a warning, and proceed as fresh). Added an Edge Cases row.
- **Resolved by:** evidence
- **Raised in round:** R1
- **Changed in plan:** Primary Flow (step 3, resume validation), Edge Cases (branch-deleted-mid-resume row).
- **Changed in tech-notes:** T4 (resume validation extended to branch existence).

### F10: `worktrees.enabled: false` skips state-file consult — stale worktrees and prior state files accumulate silently with risk of branch divergence on re-enable

- **Agent:** edge-case-explorer (EC-8, EC-10)
- **Category:** unhandled edge case
- **Finding:** Spec's Edge Cases table says "active-run state file is not consulted or written" when `enabled: false`. If the user toggled `enabled: false` after a killed run, the state file and worktree silently persist; no warning is emitted; and re-enabling later resumes into a worktree whose branch may now diverge from primary commits made in-place.
- **Evidence considered:** Edge Cases (`worktrees` block absent or `enabled: false` row); Primary Flow step 3; Alternate Flows.
- **Resolution:** Spec is amended so that **state-file detection** runs at startup regardless of `enabled`'s value: the file is read, and if present, pr9k prints a one-line warning ("A prior pr9k worktree run is active at `<path>`. Because `worktrees.enabled` is false, this run proceeds in-place. Run `pr9k --fresh` or `pr9k worktree prune` to discard the prior state."). pr9k does **not** modify the state file in `enabled: false` mode — it only reads. Added an Edge Cases row.
- **Resolved by:** evidence
- **Raised in round:** R1
- **Changed in plan:** Edge Cases (state-file present + enabled:false row; modification of the existing "absent or false" row), Primary Flow (step 3 — read happens before enabled gate), User Interactions (Feedback).
- **Changed in tech-notes:** —

### F11: State-file `primaryPath` mismatch with current resolved primary not validated before resume — risk of resuming into wrong directory

- **Agent:** edge-case-explorer (EC-9)
- **Category:** unhandled edge case
- **Finding:** T4's schema includes `primaryPath`, but no spec sentence says the resume path validates it against the currently resolved primary. If the user moved or renamed the repo, the state file may point to a stale primary, and the worktree-path stat could (rarely) succeed at an unrelated directory.
- **Evidence considered:** T4 schema; Primary Flow step 3; Edge Cases.
- **Resolution:** Added `primaryPath` equality to the resume validation set. If the recorded `primaryPath` does not match the currently resolved primary, pr9k treats the state file as stale (rename + warn + proceed fresh). Updated T4 lifecycle ordering and added an Edge Cases row.
- **Resolved by:** evidence
- **Raised in round:** R1
- **Changed in plan:** Primary Flow (step 3 — validation set), Edge Cases (primaryPath-mismatch row).
- **Changed in tech-notes:** T4 (resume validation now includes primaryPath equality).

### F12: `pr9k worktree prune` force-removes uncommitted changes silently — Invariant about local commits doesn't cover uncommitted files

- **Agent:** edge-case-explorer (EC-11)
- **Category:** invariant gap
- **Finding:** Spec's Invariants section says local commits must never be silently discarded by pr9k except on graceful completion with `autoCleanup: true` or explicit user action (`prune`, `--fresh`). Uncommitted files (e.g., `progress.txt`, partial source files) are outside that rule and `prune --force` discards them with no warning.
- **Evidence considered:** Invariants; D14; `git worktree remove --force` semantics.
- **Resolution:** Updated D14's Alternate Flow to require pr9k to print a warning line per worktree that has uncommitted changes (count of modified files), alongside the removal line. Removal still proceeds (the user asked for prune), preserving the explicit-user-action exception. Added an Edge Cases row.
- **Resolved by:** evidence
- **Raised in round:** R1
- **Changed in plan:** Alternate Flows (`pr9k worktree prune`), Edge Cases (prune + uncommitted-files row), User Interactions (Feedback), D14.
- **Changed in tech-notes:** —

### F13: Resume + structural config drift between invocations (new step expecting initialize-phase capture) silently fails

- **Agent:** edge-case-explorer (EC-12)
- **Category:** unhandled edge case
- **Finding:** A resumed run starts at iteration N, not the initialize phase. If the user added a step that consumes a `{{VAR}}` expected to be set by an initialize-phase capture, the variable resolves empty and the step proceeds with bad data.
- **Evidence considered:** Primary Flow step 10 ("the iteration runs again from feature-work onward"); D15 rationale; iteration loop scope (initialize phase not re-run on resume).
- **Resolution:** Added an Out-of-Scope entry covering structural config edits between invocations, with a "use `--fresh` for structural changes" recommendation. Added an Edge Cases row pointing to the same recommendation. Implementation-time validation of variable presence is deferred to `plan-implementation`.
- **Resolved by:** evidence (deferred to open item; recommendation captured in spec)
- **Raised in round:** R1
- **Changed in plan:** Edge Cases (structural-config-drift row), Out of Scope (structural config edits between invocations).
- **Changed in tech-notes:** —

### F14: T2's "branch prefix is load-bearing for stale-detection" rationale is stale — D5 changed to path-prefix-only after F-03

- **Agent:** evidence-based-investigator (I2)
- **Category:** internal contradiction (T# vs D#)
- **Finding:** T2 detail asserts the `pr9k/` branch prefix is "load-bearing because the stale-worktree detection at startup ([D5]) filters by that prefix when listing worktrees from `git worktree list --porcelain`." D5 was rewritten in finding F-03 (plan-a-feature stage) to filter by directory-name pattern only, not branch. T2 was not updated to match. This is also a `mechanics leaking into spec` smell because T2 cites the porcelain command flag.
- **Evidence considered:** T2 detail; D5 (current text); team-findings F-03.
- **Resolution:** Updated T2's detail to state branch uniqueness as the load-bearing fact (the original `pr9k-` prefix is now load-bearing only as a discoverability convention; stale-detection uses path-prefix only). Removed the porcelain-command flag mention.
- **Resolved by:** evidence
- **Raised in round:** R1
- **Changed in plan:** —
- **Changed in tech-notes:** T2.

### F15: Schema-version partial-parse behaviour is YAGNI — corruption-handler path satisfies the same safety property

- **Agent:** junior-developer
- **Category:** YAGNI candidate
- **Finding:** T4 / Edge Cases commit to a partial-parse fallback when `schemaVersion` is unknown ("attempt to resume anyway, reading whatever fields it recognizes"). The justification is "the user might have intentionally downgraded." No user has reported a downgrade scenario. The simpler corruption-handler behaviour (rename + warn + proceed fresh) satisfies the same "don't lose work" property because the worktree is still on disk and only the state file is renamed.
- **Evidence considered:** T4 schema-version handling; Edge Cases (schema-version row); D15 rejected alternative ("Refuse to resume on schema-version mismatch").
- **Resolution:** Replaced partial-parse with corruption-handler treatment. Edge Cases row updated. Added a Deferred entry "schema-version partial-parse and migration tooling" with reopen trigger "a real schema-version mismatch causes data loss in practice."
- **Resolved by:** evidence (YAGNI rule applied)
- **Raised in round:** R1
- **Changed in plan:** Edge Cases (schema-version row), Deferred (schema-version migration tooling — expanded).
- **Changed in tech-notes:** T4 (schema-version handling).

### F16: Mechanics leaking into spec — `git worktree remove --force`, `git branch -D <branch>` flags appear in the spec body

- **Agent:** junior-developer
- **Category:** mechanics leaking into spec
- **Finding:** Primary Flow step 12 and the `pr9k worktree prune` Alternate Flow step 4 cite specific shell commands and flags inside behavioural sentences. JD-005 and JD-009 in `team-findings.md` resolved the same class of issue earlier; these two slipped through.
- **Evidence considered:** Primary Flow step 12; Alternate Flows (`pr9k worktree prune`) step 4; team-findings JD-005, JD-009.
- **Resolution:** Replaced parenthetical command + flag citations with behavioural language ("force-removes the worktree", "deletes the run's branch"). The exact commands stay in D13/D14/T# where mechanics belong.
- **Resolved by:** evidence
- **Raised in round:** R1
- **Changed in plan:** Primary Flow (step 12), Alternate Flows (`pr9k worktree prune` step 4).
- **Changed in tech-notes:** —

### F17: `--fresh` no-op on absent state file does not address worktrees from previously *completed* runs (which removed their state files on graceful exit)

- **Agent:** adversarial-validator (R2 V2)
- **Category:** unstated assumption
- **Finding:** The `--fresh` Alternate Flow says "If absent, `--fresh` is a no-op for cleanup purposes." A user who completed several `autoCleanup: false` runs and now wants to start truly fresh might reasonably expect `--fresh` to also clear those worktrees. It does not — `--fresh` only acts on the worktree referenced in the state file.
- **Evidence considered:** spec Alternate Flow `pr9k --fresh`; D19 rationale ("the state file's existence proves a prior `enabled: true` run created the worktree"); the prune subcommand as the documented escape valve.
- **Resolution:** Added a clarifying sentence to the `--fresh` Alternate Flow step 1: `--fresh` only acts on the state-file-referenced worktree; completed-run worktrees are removed via `pr9k worktree prune`.
- **Resolved by:** evidence
- **Raised in round:** R2
- **Changed in plan:** Alternate Flows (`pr9k --fresh` step 1).
- **Changed in tech-notes:** —

### F18: TOCTOU Edge Cases row was incomplete — both racers can complete; state-file ENOENT-on-delete behaviour was unspecified

- **Agent:** adversarial-validator (R2 V5)
- **Category:** internal contradiction
- **Finding:** The R1-added TOCTOU row claimed only "the first invocation's worktree becomes detectable as stale on the next run via D5." It missed (a) the case where both racing instances complete before another pr9k starts (both worktrees are stale-detectable) and (b) the case where the losing instance's shutdown attempts to remove a state file the winner already removed (ENOENT — needed an explicit benign-on-missing rule).
- **Evidence considered:** R1 TOCTOU row (incomplete); D10; T4 lifecycle ordering.
- **Resolution:** Updated the TOCTOU row to cover both completion orderings and to specify ENOENT on state-file removal as benign.
- **Resolved by:** evidence
- **Raised in round:** R2
- **Changed in plan:** Edge Cases (TOCTOU row).
- **Changed in tech-notes:** —

### F19: Prune walk-up rule produces a false positive when the user's primary directory legitimately contains `-pr9k-` in its basename

- **Agent:** adversarial-validator (R2 V6)
- **Category:** unhandled edge case (high severity — could remove the user's primary directory's siblings)
- **Finding:** The R1-added rule for `pr9k worktree prune` says "if the resolved path's basename matches `<basename>-pr9k-*`, walk one level up." A user whose primary is at `/path/to/myapp-pr9k-experimental/` (legitimate name, not a pr9k worktree) would cause prune to walk up to `/path/to/` and then scan `/path/to/myapp-pr9k-*` for "worktrees" — sweeping up the user's actual primary as a candidate.
- **Evidence considered:** R1 D14 update; spec Alternate Flow `pr9k worktree prune`.
- **Resolution:** Added a disambiguating check to the walk-up rule: pr9k confirms the candidate path is git-recognised as a worktree of the parent repository (via `.git` file → parent's `.git/worktrees/...` link) before walking up. If the check fails, the resolved path is treated as the primary itself. Added an Edge Cases row covering this case.
- **Resolved by:** evidence
- **Raised in round:** R2
- **Changed in plan:** Alternate Flows (`pr9k worktree prune` Primary resolution), Edge Cases (legitimately-named-directory row).
- **Changed in tech-notes:** —

### F20: `primaryPath` (and `worktreePath`) equality semantics unspecified — string vs canonical path

- **Agent:** adversarial-validator + edge-case-explorer (R2 V7, EC item 7)
- **Category:** unstated assumption (high severity for macOS users — symlink ambiguity in `/var`/`/private/var`, `$HOME` aliases, and Docker volume paths is common)
- **Finding:** R1 added `primaryPath` equality to resume validation but did not say whether the comparison was string-based or canonical (symlink-resolved). On macOS, `os.Getwd()` may return either form depending on the shell; without canonicalisation, valid state files would be flagged stale on every alias-mismatched re-invocation.
- **Evidence considered:** T4 schema; `docs/coding-standards/go-patterns.md` ("symlink-safe path resolution"); macOS `/var` ↔ `/private/var` aliasing.
- **Resolution:** Added a "Path-comparison semantics" subsection to T4: both `worktreePath` and `primaryPath` are stored as realpaths at write time; the resume validation resolves the currently observed paths the same way before comparing.
- **Resolved by:** evidence
- **Raised in round:** R2
- **Changed in plan:** —
- **Changed in tech-notes:** T4 (Path-comparison semantics subsection).

### F21: `pr9k worktree prune` "modified files" warning was ambiguous — does it count untracked files?

- **Agent:** adversarial-validator + edge-case-explorer (R2 V8, EC item 6)
- **Category:** unstated assumption
- **Finding:** R1 said the prune feedback line includes the count of "modified files." The spec relies on intermediate workflow artifacts (`progress.txt`, `deferred.txt`, `test-plan.md`, `code-review.md`) that are explicitly untracked by construction (CLAUDE.md). If the count excluded untracked, the user's most-relevant in-flight content would be invisible.
- **Evidence considered:** R1 D14 update; CLAUDE.md (intermediate files); `git status --porcelain` output classes.
- **Resolution:** Spelled out that the count covers both modified-tracked AND untracked files, and named the four intermediate workflow artifacts explicitly to make the intent unambiguous.
- **Resolved by:** evidence
- **Raised in round:** R2
- **Changed in plan:** Alternate Flows (`pr9k worktree prune` step 3), Edge Cases (prune + uncommitted-files row).
- **Changed in tech-notes:** —

### F22: Out-of-Scope structural-config-drift recommendation was overly broad — would cause users to discard work for safe edits

- **Agent:** adversarial-validator (R2 V9)
- **Category:** unstated assumption
- **Finding:** R1's Out-of-Scope entry recommended `--fresh` after any "structural change to `config.json`." The actual risk is narrower: only changes that introduce a new dependency on an initialize-phase capture. Adding a new finalization step or rewording prompts is safe for resume; the blanket recommendation would have users discard in-flight work unnecessarily.
- **Evidence considered:** R1 Out-of-Scope text; iteration-loop scope (initialize phase not re-run on resume); var resolution semantics.
- **Resolution:** Narrowed the Out-of-Scope text and the matching Edge Cases row to the specific dangerous pattern; explicitly listed safe edit categories.
- **Resolved by:** evidence
- **Raised in round:** R2
- **Changed in plan:** Out of Scope (structural config edits), Edge Cases (structural-config-drift row).
- **Changed in tech-notes:** —

### F23: Schema-version unrecognised → renaming with `.corrupt-` suffix was semantically misleading

- **Agent:** adversarial-validator (R2 V10)
- **Category:** unstated assumption
- **Finding:** R1's F15 fix routed unknown schema versions through the corrupt-handler path with the same `.corrupt-<timestamp>` suffix. The data is intact in this case; calling it "corrupt" misleads the user. The user would think their state file was damaged when in fact it was simply from an incompatible pr9k version.
- **Evidence considered:** R1 Edge Cases schema-version row; T4 schema-version handling; `.stale-` and `.corrupt-` suffix conventions in the spec.
- **Resolution:** Switched to a dedicated suffix `.incompatible-<timestamp>` with a warning text that explicitly names the schema version and asks "did you downgrade?" Updated T4 and the Edge Cases row.
- **Resolved by:** evidence
- **Raised in round:** R2
- **Changed in plan:** Edge Cases (schema-version row).
- **Changed in tech-notes:** T4 (schema-version handling).

### F24: `enabled:false` warning persistence not documented — does the warning fire forever or only once?

- **Agent:** adversarial-validator + edge-case-explorer (R2 V11, EC item 2)
- **Category:** unstated assumption
- **Finding:** R1 added a warning when `enabled: false` and a state file is present; the spec said "the state file is not modified." It did not say whether the warning repeats indefinitely or fires once. The intent matters: if persistent, users should know; if one-shot, the spec needed a mechanism.
- **Evidence considered:** R1 Edge Cases row; spec User Interactions (Feedback).
- **Resolution:** Spec now states the warning is **intentionally persistent** — fires on every `enabled: false` invocation until the user runs `--fresh` or `prune`. Persistence is preferred because the silent-divergence risk on `enabled: true` re-enable is a correctness issue.
- **Resolved by:** evidence
- **Raised in round:** R2
- **Changed in plan:** Edge Cases (state-file present + enabled:false row).
- **Changed in tech-notes:** —

### F25: Branch-deleted-mid-resume orphan worktree's fate was implied not stated

- **Agent:** edge-case-explorer (R2 EC item 1)
- **Category:** unstated assumption
- **Finding:** R1's Edge Cases row said the now-branchless orphan is "left on disk for the user to inspect or discard with `pr9k worktree prune`." It did not say whether D5's stale-detection picks it up on the next startup, or whether the fresh start that follows creates a new worktree at a new path.
- **Evidence considered:** R1 Edge Cases row (branch-deleted); D5; Primary Flow step 4.
- **Resolution:** Spelled out that the orphan is included in D5's stale-warning count on the next startup (no active-run anchor), and that the fresh start creates a new sibling worktree as normal.
- **Resolved by:** evidence
- **Raised in round:** R2
- **Changed in plan:** Edge Cases (branch-deleted-mid-resume row).
- **Changed in tech-notes:** —

### F26: T5 alias creation failure handling was unspecified — risk of consumers silently reading a stale or absent path

- **Agent:** edge-case-explorer (R2 EC item 3)
- **Category:** unhandled edge case
- **Finding:** T5 deferred implementation choice between symlink and atomic-rename, but did not say what happens when the chosen mechanism fails (filesystem doesn't support symlinks, permission error, pre-existing regular file blocking the alias). Consumers reading `iteration.jsonl` would silently see the wrong file with no error.
- **Evidence considered:** T5 (R1 addition).
- **Resolution:** T5 now states alias creation failure is fatal at startup — pr9k surfaces the error on stderr (pre-TUI) and exits non-zero before the run loop starts. Consistent with the worktree-creation-failure pattern from m1.
- **Resolved by:** evidence
- **Raised in round:** R2
- **Changed in plan:** —
- **Changed in tech-notes:** T5 (Failure handling subsection).

### F27: `pr9k worktree prune` from inside a STALE (non-active) worktree would still remove the user's CWD

- **Agent:** edge-case-explorer (R2 EC item 4)
- **Category:** unhandled edge case (high severity)
- **Finding:** R1's walk-up rule excluded only the *active* worktree from removal. A user `cd`'d into a stale worktree (one whose stamp does not appear in `active-run.json`) would have walk-up resolve correctly to the primary and prune correctly identify the active worktree to exclude — but the stale worktree the user is sitting in would still be a removal candidate. The user's shell CWD would point into a deleted directory.
- **Evidence considered:** R1 D14; R1 prune Alternate Flow step 2; shell CWD-deleted behaviour.
- **Resolution:** Broadened the exclusion rule: prune excludes (a) the active-run worktree AND (b) any worktree whose path the resolved CWD is inside, regardless of active-run status. The Edge Cases row was rewritten accordingly.
- **Resolved by:** evidence
- **Raised in round:** R2
- **Changed in plan:** Alternate Flows (`pr9k worktree prune` step 2), Edge Cases (prune-from-inside-worktree row).
- **Changed in tech-notes:** —

### F28: `--fresh` refusal when PID is alive — escape hatch for wedged process not documented

- **Agent:** edge-case-explorer (R2 EC item 5)
- **Category:** unstated assumption
- **Finding:** R1 made `--fresh` refuse when the recorded PID is alive and the binary matches. The spec said "stop the running process first" — but offered no advice for the wedged-pr9k case (the original use case for `--fresh`). A user with a hung pr9k could conclude `--fresh` has no escape hatch.
- **Evidence considered:** R1 D10, D19; User Interactions (Error states).
- **Resolution:** Added the explicit `kill -9 <PID>` → `--fresh` recovery sequence to User Interactions (Error states).
- **Resolved by:** evidence
- **Raised in round:** R2
- **Changed in plan:** User Interactions (Error states).
- **Changed in tech-notes:** —

### F29: Cleanup ordering for `autoCleanup: true` was unspecified — risk of orphan branch + worktree on mid-cleanup crash

- **Agent:** edge-case-explorer (R2 EC item 9)
- **Category:** unhandled edge case
- **Finding:** R1's Primary Flow step 12 listed three cleanup actions (worktree removal, branch deletion, state-file removal) but did not specify ordering. A SIGKILL between actions could leak resources without a recovery anchor; in particular, removing the state file first would leak both the worktree and its branch with no resume hint or stale-detection trigger.
- **Evidence considered:** R1 Primary Flow step 12; D5 stale detection (anchored on directory existence); D15 orphan handler (anchored on state file existence).
- **Resolution:** Specified the cleanup order as branch deletion → worktree removal → state-file removal, with a one-sentence explanation of why this ordering is crash-resilient (every intermediate failure leaves either the state file as a resume anchor or the worktree as a D5 stale-detection trigger).
- **Resolved by:** evidence
- **Raised in round:** R2
- **Changed in plan:** Primary Flow (step 12).
- **Changed in tech-notes:** —

## Minor edits

- m1: JD-R1-5 — worktree-creation-failure error surface (TUI vs stderr) under-specified — junior-developer — Alternate Flows (worktree creation fails)
- m2: EBI-1 — D16 wording implies `RunResult` is empty today; it already has `IterationsRun` — evidence-based-investigator — D16 (rationale clarified)
- m3: EBI-6 — T4 wording "atomicwrite.Write requires the parent directory to exist" is imprecise; the open-temp-file step does, not the function itself — evidence-based-investigator — T4 (lifecycle ordering wording)
- m4: I3 — D2 / T2 still say "run-stamp" while the spec body now uses "worktree-stamp" — evidence-based-investigator — D2, T2 (terminology unification)
- m5: EC-5 — autoCleanup + breakLoopIfEmpty zero-iteration case lacks explicit user feedback note — edge-case-explorer — Edge Cases (autoCleanup + LoopBroken row clarified)
- m6: EC-7 — `prune --dry-run` followed by `prune` race not addressed; user-expectation note added — edge-case-explorer — Alternate Flows (`pr9k worktree prune`)
