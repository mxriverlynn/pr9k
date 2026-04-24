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
- **Technical detail (indicative approach):** One approach that meets the contract is a temp-file-plus-rename sequence — write the new content to a sibling temporary file in the same directory as the target (so the rename stays within one filesystem), force the temporary's content to durable storage, then rename the temporary over the target. POSIX `rename` is atomic within a single filesystem, which guarantees any observer sees either the old inode's content or the new inode's content. For this to hold without introducing a symlink-race surface, the temporary file is created exclusively (never overwriting a pre-existing path), with a name obviously identifiable as a builder scratch file (a suffix containing a process id or timestamp). A leftover temp file from a crashed save is surfaced to the user on the next session's open (see D42-a) rather than silently deleted.

  **Symlinked target — explicit correction:** When the target path itself is a symlink, the naive rename destroys the symlink relationship. POSIX `rename` operates on the destination directory entry, not on the symlink's target — the new inode replaces the symlink itself. Verified on macOS APFS during spec review. For the save to preserve the symlink (per D17's commitment to "write through the symlink to its target"), the implementation must: (a) at save time, resolve the target via filesystem path-resolution to its real file location; (b) compute the real target directory; (c) place the temporary file in that real directory (still same-filesystem); (d) rename over the resolved real path, not over the symlink path. The symlink entry is untouched; its target inode is atomically replaced. Symlink resolution happens at save, not at load, to avoid a TOCTOU window where the symlink could retarget between load and save.

  Other mechanisms that deliver the same durability, atomicity, and symlink-preservation properties are acceptable. The implementation plan should also codify whichever mechanism it picks in the coding standards directory, because this is a new pattern for the codebase.
- **Supports decisions:** D7, D17, D41
- **Driven by findings:** F27, F28, F38, F42, F49
- **Referenced in spec:** Primary Flow step 9; Coordinations — Filesystem; Edge Cases table (several rows); Alternate Flows — Crash-era temporary file on open

## T2: Terminal handoff to external editor

- **Context:** The spec commits to two behavioral properties when the external editor runs: "no builder keystrokes or mouse events are consumed while the external editor holds the terminal" and "on return, the file on disk is re-read before any further builder action" (Alternate Flows — External-editor invocation; Coordinations — External editor). pr9k today never shells out interactively — existing subprocess integrations are either non-interactive (scripts emitting stdout) or fully sandboxed under Docker (claude). A reader of the code therefore cannot discover how an interactive TTY handoff is done within a Bubble Tea program, because pr9k has never done one. The mechanic is load-bearing because without the correct handoff-reclaim sequence, the terminal is left in an inconsistent state (alt-screen corruption, bracketed-paste residue, mouse-mode residue) and the user sees garbage on return; also without it, the builder's keyboard/mouse listener competes with the external editor for events.
- **Technical detail (indicative approach):** The Bubble Tea framework exposes this handoff via `tea.ExecProcess(*exec.Cmd, ExecCallback) tea.Cmd` — a `tea.Cmd` returned from `Update` (not a `tea.Msg` sent into it). When the runtime executes the returned command, it calls the program's `ReleaseTerminal` hook (exits alt-screen, disables mouse reporting, restores cooked mode), runs the child process as foreground inheriting stdin/stdout/stderr, waits for it to exit, and then calls `RestoreTerminal` (re-enters alt-screen, re-enables mouse cell-motion, triggers a full re-render). No existing pr9k code uses `tea.ExecProcess` — verified by codebase search.

  Non-zero editor exits are surfaced as a non-blocking notice but do not prevent the re-read — the editor may have written partial content before failing, and the file on disk is the source of truth regardless of exit status. If the editor leaves the terminal in an inconsistent state on abnormal exit, `RestoreTerminal` is best-effort; the first subsequent window resize or full repaint recovers visible state. Other handoff mechanisms that deliver the same properties (no competing input, clean terminal on return, re-read before proceeding) are acceptable at implementation time.
- **Supports decisions:** D5, D16, D33
- **Driven by findings:** F29, F36, F56
- **Referenced in spec:** Primary Flow step 7 (multi-line content); Alternate Flows — External-editor invocation, Editor binary cannot be spawned; Coordinations — External editor

## T3: In-memory validation

- **Context:** The spec commits that "the validator sees exactly the state the save will write — no subset, no superset" (Coordinations — Workflow configuration validator; Primary Flow step 9). The existing validator's entry point takes a workflow-directory path and reads `config.json` from disk itself (see `src/internal/validator/validator.go:154` `Validate(workflowDir string)`). A reader of the code would reasonably expect the builder to write its in-memory state to the target file before validating — but that would either create a TOCTOU window between validate and the real atomic save, or validate state that might be overwritten by a concurrent process. The mechanic is load-bearing because the spec's "exactly what will be written" commitment, combined with T1's atomic rename, rules out the "write first, validate second" approach.
- **Technical detail (indicative approach):** Two feasible approaches:
  1. **Extend the validator API** to accept an in-memory representation of the workflow (the parsed struct plus the companion-file bytes) in addition to the existing path-based entry point. The builder calls the in-memory variant. The existing path-based entry point is preserved for the main `pr9k` startup path. This is the narrow-reading-aligned choice — schema knowledge stays in the validator package and no filesystem round-trips are incurred per save.
  2. **Validate via a private temporary bundle.** The builder writes its in-memory state to a scratch directory outside the real target (prompt and script files copied by path, `config.json` serialized from the in-memory model), validates the scratch directory with the existing path-based API, then discards the scratch directory. The real target is never written during validation. This approach incurs per-save I/O proportional to the total size of the companion files in the workflow, which is bounded but non-trivial for workflows with many large prompts.

  The implementation plan picks between the two based on the validator package's existing code shape at implementation time; the first approach is preferred when feasible. Whichever is chosen, the **real target directory** is never written during validation — validation-time writes, if any, land in a disjoint scratch location the user never sees.
- **Supports decisions:** D6, D14
- **Driven by findings:** F30, F50
- **Referenced in spec:** Primary Flow step 9; Coordinations — Workflow configuration validator
