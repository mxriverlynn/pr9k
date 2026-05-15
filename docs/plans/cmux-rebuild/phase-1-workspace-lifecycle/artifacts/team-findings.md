# Team Findings: Phase 1 — Cmux Mode Launch and Workspace Lifecycle

Records every finding raised by the review team for Phase 1, and how each was resolved. Behavioral outcomes live in [../feature-specification.md](../feature-specification.md); decisions the findings affected live in [decision-log.md](decision-log.md). A Phase-1-specific feature-technical-notes file does not exist — the load-bearing mechanics that Phase 1 relies on are already captured in the parent [../../artifacts/feature-technical-notes.md](../../artifacts/feature-technical-notes.md) and are referenced from Phase 1's spec via `parent T#` links.

Reviewers consulted: `junior-developer` (generalist behavioral / scope review), `edge-case-explorer` (boundary cases and failure-mode coverage).

## Major findings

### F1: Dismissal-detection mechanism is unspecified

- **Agent:** junior-developer (F1) + edge-case-explorer (F4, F13) — flagged independently by both reviewers as the same root gap.
- **Finding:** The spec commits to a launching pr9k process that "waits for the workspace to be dismissed" but never names the observable event the process is waiting on. The gap propagates into multiple adjacent edge cases (operator dismisses before setup completes, placeholder process exits unexpectedly, close-all-panes vs. close-workspace gestures): each of those depends on a dismissal-detection contract that the spec did not state.
- **Resolution:** Added [D9: Dismissal observation contract](decision-log.md#d9-dismissal-observation-contract) committing to two observable events — the pr9k workspace's name disappearing from cmux's workspace list, or any of the four placeholder processes exiting — either of which is treated as dismissal. Updated the spec's Primary Flow step 8, the Coordinations table's cmux row, and the placeholder-exit edge-case row to reference D9. The implementation mechanism (polling vs. async subscription) is intentionally left to plan-implementation per the spec-content rule.
- **Resolved by:** evidence
- **Affected decisions:** D9 (new), D7 (cross-reference added)
- **Changed in spec:** Primary Flow (step 8, step 9), Coordinations (cmux row, placeholder processes row), Edge Cases and Failure Modes (placeholder process exit row, operator dismisses before setup completes row)

### F2: No prior workspace exists when launching from a fresh cmux session

- **Agent:** edge-case-explorer (F1)
- **Finding:** Primary Flow step 2 captures the prior workspace unconditionally; step 9 restores it unconditionally. But a developer launching `pr9k --cmux` from a freshly-opened cmux session with no other workspaces would have no prior workspace to capture. The spec was silent on this very-reachable first-launch case.
- **Resolution:** Added [D10: "No prior workspace" state handled gracefully](decision-log.md#d10-no-prior-workspace-state-handled-gracefully). pr9k records "no prior workspace" at step 2 if applicable; on dismissal, the explicit focus-restore is skipped and cmux's default focus behavior applies. Added an edge-case row covering both "no prior workspace at capture" and "captured prior workspace no longer exists at restore time" (the latter resolves F6 below).
- **Resolved by:** evidence
- **Affected decisions:** D10 (new)
- **Changed in spec:** Primary Flow (step 2, step 9 — now step 10 after orphan-deferral note), Edge Cases and Failure Modes (two new rows: no prior workspace; captured prior workspace no longer exists)

### F3: Repo basename character handling unspecified — invalid characters could produce a malformed workspace name

- **Agent:** edge-case-explorer (F2, F8) — combined two related findings
- **Finding:** [Parent D29](../../artifacts/decision-log.md#d29-workspace-name-pattern) commits to `pr9k-<repo-basename>-<nanosecond-timestamp>` but does not specify what characters cmux accepts in workspace names. Repos with spaces, dots, slashes, control characters, Unicode, or degenerate paths (`--project-dir /`, trailing slashes) could produce invalid or confusing names. The "cmux's API rejects a request mid-flow" partial-setup failure path would catch the cmux-side rejection, but the operator would see a confusing diagnostic that does not name the basename as the cause.
- **Resolution:** Added [D11: Repo basename sanitized before use in workspace name](decision-log.md#d11-repo-basename-sanitized-before-use-in-workspace-name). pr9k sanitizes the basename to `[a-zA-Z0-9._-]`, collapses hyphens, trims edges. Empty sanitized basename falls back to the literal string `repo`. Added an edge-case row covering this. Updated Primary Flow step 5 to mention sanitization.
- **Resolved by:** evidence
- **Affected decisions:** D11 (new)
- **Changed in spec:** Primary Flow (step 5), Edge Cases and Failure Modes (new row for repo basename character handling), User Interactions (workspace name pattern now reads `pr9k-<sanitized-repo-basename>-<nanosecond-timestamp>`)

### F4: Orphan advisory step omitted from Primary Flow without cross-reference

- **Agent:** junior-developer (F4)
- **Finding:** The parent feature spec's Primary Flow has an orphan-advisory step (parent step 4). Phase 1's Primary Flow simply skipped it without acknowledging the omission, leaving Phase 1's flow numbering inconsistent with the parent's and forcing readers to discover the deferral by tracing back to the Out of Scope section. The Out of Scope section did note the deferral but the Primary Flow did not cross-reference it.
- **Resolution:** Added a step 4 placeholder to Phase 1's Primary Flow that explicitly says the orphan advisory step from the parent flow is intentionally absent in Phase 1 and points at Phase 7 of the build outline. Subsequent steps were renumbered accordingly.
- **Resolved by:** evidence
- **Affected decisions:** —
- **Changed in spec:** Primary Flow (new step 4 placeholder; renumber of subsequent steps)

### F5: Operator close-all-panes gesture vs. close-workspace gesture — only one was handled

- **Agent:** edge-case-explorer (F13) + junior-developer (F3, F8) — same root gap surfaced by both reviewers
- **Finding:** cmux exposes at least two dismissal gestures (close workspace, close pane). The spec used "cmux's own workspace-close controls" as if it were a single known operation. If the operator closes all four panes individually without closing the workspace itself, the workspace persists per [parent T5](../../artifacts/feature-technical-notes.md#t5-cmux-workspace-persists-after-pane-exit) — and a dismissal-detection contract based purely on workspace-list disappearance would leave pr9k waiting indefinitely.
- **Resolution:** F1's resolution ([D9](decision-log.md#d9-dismissal-observation-contract)) makes both gestures reach pr9k's dismissal observation contract: the workspace-close gesture triggers the workspace-list-disappearance observation, the close-all-panes sequence triggers the placeholder-exit observation. Updated the User Interactions section to name both gestures explicitly. Added [OI-3](../feature-specification.md#open-items) to track the dependency on the cmux setup how-to to document the actual cmux key bindings for each gesture.
- **Resolved by:** evidence (D9 covers the behavior; OI-3 tracks the documentation dependency)
- **Affected decisions:** D9 (also F1)
- **Changed in spec:** User Interactions (new Dismissal gestures affordance), Primary Flow (step 9 names both gestures), Open Items (new OI-3)

### F6: Captured prior workspace becomes stale before restore

- **Agent:** edge-case-explorer (F3) + junior-developer (F6) — flagged by both
- **Finding:** The operator could close the prior workspace from another cmux session during pr9k's lifetime, leaving the captured prior-workspace identifier stale at restore time. The spec's unconditional "pr9k explicitly selects the prior workspace" language did not say what happens when the call fails.
- **Resolution:** Covered by [D10](decision-log.md#d10-no-prior-workspace-state-handled-gracefully) (same decision as F2): pr9k ignores any error from the restore call and exits cleanly, letting cmux's default focus behavior take over. The same edge-case row covers both the "never captured a prior workspace" and "prior workspace closed before restore" cases.
- **Resolved by:** evidence
- **Affected decisions:** D10
- **Changed in spec:** Edge Cases and Failure Modes (captured prior workspace no longer exists row)

### F7: SIGHUP citation issue — D6 covered only SIGINT/SIGTERM

- **Agent:** junior-developer (F7)
- **Finding:** The edge-case row covering "the launching terminal session is closed" cited D6, but D6 only discussed SIGINT/SIGTERM and the double-signal case. SIGHUP delivery semantics are different (shell-configuration-dependent) and D6 did not address them.
- **Resolution:** Extended [D6](decision-log.md#d6-double-signal-leaves-an-orphan-workspace-no-cleanup-attempted) to cover SIGHUP explicitly: all three signal kinds trigger the same graceful-shutdown path on first delivery, a second signal of any kind during shutdown triggers immediate exit. Added an explicit note that SIGHUP delivery is shell-configuration-dependent and called out the orphaned-process consequence when the shell does not forward SIGHUP. Updated the alternate-flow section to name all three signals and the "ignore workspace.close failure during shutdown" semantic from F14 below.
- **Resolved by:** evidence
- **Affected decisions:** D6 (extended)
- **Changed in spec:** Alternate Flows and States (SIGINT/SIGTERM flow renamed to "Operator sends SIGINT, SIGTERM, or SIGHUP")

## Minor edits

- F8: D8 wording clarified — "standard preflight succeeds" now explicitly means "zero fatal errors" with non-fatal warnings printed and the launch proceeding. — junior-developer F5 / edge-case-explorer F5 — decision-log.md D8
- F9: D8 rationale extended — Phase 1 runs the Docker preflight when `config.json` contains claude steps, consistent with non-cmux launches; operators without Docker can demo Phase 1 only with a claude-step-free config. — junior-developer F5 — decision-log.md D8
- F10: Coordinations `config.json` row reworded — replaced "unchanged" with explicit "loaded and validated by standard preflight; no step content is read or executed in Phase 1." Combined with the `.pr9k/logs/` row's note that the umbrella directory is still created by standard preflight on first run. — junior-developer F10 — feature-specification.md#coordinations
- F11: Open Items extended — added OI-3 tracking the dependency on the cmux setup how-to to document the workspace-dismissal gesture. — junior-developer F3, F8 — feature-specification.md#open-items
- F12: Alternate-flow consolidation — removed the "Workspace creation succeeds but pane setup fails partway" alternate-flow section; the equivalent edge-case row was extended to absorb the cleanup-sequence detail. — junior-developer F9 — feature-specification.md#alternate-flows-and-states, #edge-cases-and-failure-modes
- F13: Nanosecond-collision retry added — D29's "prevents collisions" claim refined; pr9k now retries `workspace.create` once with a fresh timestamp if it hits a duplicate-name error, then routes to partial-setup failure. — edge-case-explorer F6 — decision-log.md D12 (new), feature-specification.md#primary-flow (step 5), #edge-cases-and-failure-modes (new row)
- F14: `workspace.close` failure during shutdown — alternate-flow text extended to say pr9k ignores a workspace-close failure during shutdown teardown (e.g., when the workspace was already dismissed) and continues to the focus-restore step. — edge-case-explorer F7 — feature-specification.md#alternate-flows-and-states (SIGINT/SIGTERM/SIGHUP flow)
- F15: Degenerate `--project-dir` paths covered by sanitization — `--project-dir /` (basename `/` sanitizes to empty) and trailing-slash paths route through D11's sanitization fallback (literal string `repo`); covered by the same edge-case row as F3. — edge-case-explorer F8 — decision-log.md D11
- F16: `pane.hide` failure added to partial-setup coverage — the edge-case row now reads "a pane-split, pane-spawn, or pane-hide call" so a failed hide for the orchestrator pane routes through the same teardown path. — edge-case-explorer F9 — feature-specification.md#edge-cases-and-failure-modes (partial-setup row)

## YAGNI dismissals (raised but not adopted)

The following items were raised by reviewers and explicitly dropped under the YAGNI rule. They are recorded here so the rejection is auditable.

- **Docker password prompt during standard preflight** (edge-case-explorer F10). No evidence cmux mode changes this scenario relative to the existing non-cmux launch path. If Docker prompts for a password today on a normal pr9k launch, it prompts the same way with `--cmux`; this is not a Phase-1-introduced behavior.
- **SIGSTOP/SIGCONT and SIGPIPE handling** (edge-case-explorer F11). No realistic production trigger named. SIGSTOP cannot be caught and is not a signal an operator sends to pr9k in normal use; SIGPIPE on a closed cmux socket already routes through the per-call timeout and error-handling paths covered by parent D15.
- **ANSI escape sequences in cmux error messages or repo basenames** (edge-case-explorer F12). Repo-basename case is covered by D11 sanitization. Error-message case is an implementation concern — pr9k already has `internal/ansi.StripAll` for the recovery-view use case; the spec does not need to commit to ANSI-stripping at the behavioral level (though the Coordinations launching-terminal row was updated to note that cmux-supplied diagnostics are stripped before printing).
