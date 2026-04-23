# Feature Technical Notes: Workflow Builder

<!--
Captures load-bearing mechanics whose absence from the code repo means
plan-implementation cannot discover them on its own. Mechanics discoverable
from existing code — Bubble Tea's model/update/view pattern, cobra subcommand
wiring, the validator severity model — are NOT captured here; plan-implementation
finds those from the code.

These notes describe what the mechanic must achieve and sketch a proven
approach. They are not prescriptive — any alternative that delivers the same
observable behavior is acceptable at implementation time.
-->

## T1: Atomic configuration file save

- **Context:** The spec commits to a durability property — "the file on disk contains either the prior content or the new content, never partial content" (Primary Flow step 9; Coordinations — Filesystem). The existing pr9k codebase has no atomic-rename pattern: every file write today uses direct `O_TRUNC` overwrite or `O_APPEND`. A reader of the code therefore cannot infer how the builder should achieve this durability. The mechanic is load-bearing because without it, a crash, signal, or power loss mid-save can truncate or empty the user's primary editing artifact.
- **Technical detail (indicative approach):** One approach that meets the contract is a temp-file-plus-rename sequence — write the new content to a sibling temporary file in the same directory as the target (so the rename stays within one filesystem), force the temporary's content to durable storage, then rename the temporary over the target. POSIX `rename` is atomic within a single filesystem, which guarantees any observer sees either the old inode's content or the new inode's content. For this to hold without introducing a symlink-race surface, the temporary file is created exclusively (never overwriting a pre-existing path), with a name obviously identifiable as a builder scratch file (a suffix containing a process id or timestamp). A leftover temp file from a crashed save is surfaced to the user on the next session's open (see D42-a) rather than silently deleted. When the target is itself a symlink, the save writes through the symlink to its target — the rename replaces the inode the symlink resolves to, preserving the link relationship (see D17). The implementation plan owns the final API shape; other mechanisms that deliver the same durability, atomicity, and no-symlink-races properties are acceptable. The implementation plan should also codify whichever mechanism it picks in the coding standards directory, because this is a new pattern for the codebase.
- **Supports decisions:** D7, D17, D41
- **Driven by findings:** F38, F42
- **Referenced in spec:** Primary Flow step 9; Coordinations — Filesystem; Edge Cases table (several rows); Alternate Flows — Crash-era temporary file on open

## T2: Terminal handoff to external editor

- **Context:** The spec commits to two behavioral properties when the external editor runs: "no builder keystrokes or mouse events are consumed while the external editor holds the terminal" and "on return, the file on disk is re-read before any further builder action" (Alternate Flows — External-editor invocation; Coordinations — External editor). pr9k today never shells out interactively — existing subprocess integrations are either non-interactive (scripts emitting stdout) or fully sandboxed under Docker (claude). A reader of the code therefore cannot discover how an interactive TTY handoff is done within a Bubble Tea program, because pr9k has never done one. The mechanic is load-bearing because without the correct handoff-reclaim sequence, the terminal is left in an inconsistent state (alt-screen corruption, bracketed-paste residue, mouse-mode residue) and the user sees garbage on return; also without it, the builder's keyboard/mouse listener competes with the external editor for events.
- **Technical detail (indicative approach):** One approach that meets the contract is to pause the Bubble Tea program before invoking the editor (exit alt-screen mode, disable mouse reporting, restore cooked terminal mode), execute the editor as a foreground child inheriting stdin/stdout/stderr, wait for it to exit, and then restart Bubble Tea (re-enter alt-screen, re-enable mouse cell-motion, re-render). Bubble Tea exposes a terminal-release / restart pattern through its message types; the implementation plan will pick the specific message and wire it. Non-zero editor exits are surfaced as a non-blocking notice but do not prevent the re-read — the editor may have written partial content before failing, and the file on disk is the source of truth regardless of exit status. If the editor leaves the terminal in an inconsistent state on abnormal exit, the builder's restore pass is best-effort; the first subsequent window resize or full repaint recovers visible state. Other handoff mechanisms that deliver the same properties (no competing input, clean terminal on return, re-read before proceeding) are acceptable at implementation time.
- **Supports decisions:** D5, D16, D33
- **Driven by findings:** —
- **Referenced in spec:** Primary Flow step 7 (multi-line content); Alternate Flows — External-editor invocation, Editor binary cannot be spawned; Coordinations — External editor

## T3: In-memory validation

- **Context:** The spec commits that "the validator sees exactly the state the save will write — no subset, no superset" (Coordinations — Workflow configuration validator; Primary Flow step 9). The existing validator's entry point takes a workflow-directory path and reads `config.json` from disk itself (see `src/internal/validator/validator.go:159` `Validate(workflowDir string)`). A reader of the code would reasonably expect the builder to write its in-memory state to the target file before validating — but that would either create a TOCTOU window between validate and the real atomic save, or validate state that might be overwritten by a concurrent process. The mechanic is load-bearing because the spec's "exactly what will be written" commitment, combined with T1's atomic rename, rules out the "write first, validate second" approach.
- **Technical detail (indicative approach):** Two feasible approaches:
  1. **Extend the validator API** to accept an in-memory representation of the workflow (the parsed struct plus the companion-file bytes) in addition to the existing path-based entry point. The builder calls the in-memory variant. The existing path-based entry point is preserved for the main `pr9k` startup path.
  2. **Validate via a private temporary bundle.** The builder writes its in-memory state to a scratch directory (prompt and script files copied by path, `config.json` serialized from the in-memory model), validates the scratch directory with the existing path-based API, then discards the scratch directory and runs the atomic save independently against the real target.
- The first approach is the narrow-reading-aligned choice: it keeps the schema knowledge inside the validator package and avoids filesystem round-trips for every save. The implementation plan picks between the two based on the validator package's existing code shape at implementation time. Either approach satisfies the spec's commitment. Whichever is chosen, the builder never writes to the real target directory as part of validation — writes happen only under the T1 atomic save path.
- **Supports decisions:** D6, D14
- **Driven by findings:** F30
- **Referenced in spec:** Primary Flow step 9; Coordinations — Workflow configuration validator
