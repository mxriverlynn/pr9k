# Plan: `ralph-tui` — Go/Glyph TUI for Ralph Loop

## Context

The `ralph-loop` bash script runs multiple sequential `claude` CLI calls, capturing all output into `$RESULT` and displaying it after-the-fact with `box-text`. This makes it impossible to see what's happening during long-running steps. We're building a Go program using [Glyph](https://useglyph.sh/) that replaces `ralph-loop` as the orchestrator, streaming claude output in real-time into a bordered, scrolling TUI panel with full workflow visibility.

---

## Layout

```
┌─ Ralph ──────────────────────────────────────────────────────────────────────┐
│                                                                              │
│  Iteration 1/3 — Issue #42: Add widget support                               │
│                                                                              │
│  [✓] Feature work    [✓] Test plan    [▸ Test writing]    [ ] Code review    │
│  [ ] Review fixes    [ ] Close issue  [ ] Update docs     [ ] Git push       │
│                                                                              │
├──────────────────────────────────────────────────────────────────────────────┤
│                                                                              │
│  I'll start by examining the existing test files to understand the           │
│  patterns used in this project...                                            │
│                                                                              │
│  Reading src/widget.test.ts...                                               │
│  The test suite uses vitest with the following structure:                    │
│  - describe blocks for each public method                                    │
│  - beforeEach for shared setup                                               │
│                                                                              │
│  I'll create tests for the three new methods added in the feature            │
│  work step...                                                                │
│                                                                              │
├──────────────────────────────────────────────────────────────────────────────┤
│  ↑/k up  ↓/j down  n next step  q quit                                       │
└──────────────────────────────────────────────────────────────────────────────┘
```

### Three sections, top to bottom:

**1. Status header (fixed height)**
- Current iteration and total: `Iteration 1/3`
- Issue being worked: `Issue #42: Add widget support`
- Step tracker with checkboxes across two rows showing all 8 steps per iteration
- Status indicators: `[✓]` done, `[▸ ...]` active (with spinner), `[ ]` pending

**2. Output log (fills remaining space, scrollable)**
- Streams claude stdout line-by-line as it arrives
- Auto-scrolls to bottom as new lines appear
- User can scroll up with `↑`/`k` — when scrolled up, auto-scroll pauses
- Scrolling back to the bottom re-enables auto-scroll
- Step transitions insert a visual separator: `── Test writing ─────────────`
- All steps' output accumulates in one continuous log (scroll up to see earlier steps)

**3. Keyboard shortcut bar (fixed, 1 line)**
- Shows available keys: `↑/k up  ↓/j down  n next step  q quit`
- Always visible at the bottom

### Error state

When a step fails (non-zero exit from `claude` or any subprocess), the TUI switches to an error mode:

```
├──────────────────────────────────────────────────────────────────────────────┤
│  ✗ Step "Test writing" failed (exit code 1)                                  │
│                                                                              │
│  ... last lines of output visible above in log ...                           │
│                                                                              │
├──────────────────────────────────────────────────────────────────────────────┤
│  c continue to next step  r retry step  q quit                               │
└──────────────────────────────────────────────────────────────────────────────┘
```

- The step checkbox shows `[✗]` 
- Shortcut bar changes to show error-specific keys
- `c` — skip this step, continue to the next one
- `r` — retry the failed step from scratch
- `q` — quit (with confirmation)

### Quit confirmation

Pressing `q` at any time shows a confirmation overlay or replaces the shortcut bar:

```
├──────────────────────────────────────────────────────────────────────────────┤
│  Quit ralph? (y/n)                                                           │
└──────────────────────────────────────────────────────────────────────────────┘
```

---

## Glyph Component Mapping

```go
VBox(
    // Section 1: Status header
    VBox(
        Text(&iterationLine),           // "Iteration 1/3 — Issue #42: ..."
        HBox(stepCheckboxRow1...),      // first 4 steps
        HBox(stepCheckboxRow2...),      // next 4 steps
    ).Border(BorderRounded).Title("Ralph"),

    // Section 2: Scrollable output log
    // logReader is the read end of an io.Pipe(); subprocess output is written to the write end
    Log(logReader).Grow(1).MaxLines(5000).BindVimNav().OnUpdate(app.RequestRender),

    // Section 3: Keyboard shortcuts
    VBox(
        Text(&shortcutLine),            // "↑/k up  ↓/j down  n next step  q quit"
    ).Border(BorderRounded),
)
```

Key Glyph features:
- **`Log(io.Reader)`** — takes an `io.Reader`, NOT a `*[]string`. Glyph spawns its own internal goroutine to read lines via `bufio.Scanner` and manages its own `sync.Mutex` internally. This means the plan's subprocess streaming architecture must change: instead of appending to a shared slice, pipe subprocess output through an `io.Reader` (e.g., `io.Pipe`) that Glyph consumes directly.
- **`.Grow(1)`** — fills remaining vertical space
- **`.MaxLines(n)`** — ring buffer, default 10000
- **`.AutoScroll(bool)`** — auto-follows new content, pauses when user scrolls up
- **`.BindVimNav()`** — wires j/k (line), Ctrl-d/u (half-page), g/G (top/bottom) automatically
- **`.OnUpdate(func())`** — callback when new lines arrive; use with `app.RequestRender`
- **`Border(BorderRounded)`** — handles all box drawing (replaces `box-text`)
- **`Spinner`** — next to the active step name for visual activity indication
- **Pointer-based reactivity** — for `Text`, `Checkbox`, etc. (NOT for `Log` — it uses `io.Reader`)

---

## Internal Architecture

### Orchestration goroutine

A single goroutine runs the workflow (replicated from `ralph-loop` logic):

1. Display `ralph-art.txt` contents in the output log as a startup banner
2. Get GitHub username via `gh api user`
2. Loop N iterations:
   a. Get next issue via `gh` CLI
   b. Get current git SHA
   c. Run each step sequentially (spawn subprocess, stream output, wait for exit)
3. Run finalization steps (deferred work, lessons learned, final push)
4. Display completion summary: "Ralph completed after N iteration(s) and 3 finalizing tasks."

If `get_next_issue` returns an empty string (no issues labeled "ralph" assigned to the user), the orchestration goroutine should log a message to the TUI output, mark the iteration as skipped, and exit the loop early — mirroring `ralph-loop`'s behavior at line 43-46.

This goroutine communicates with the TUI by mutating shared state (which Glyph reads via pointers).

### Finalization phase

After all iterations complete, the orchestration goroutine runs three finalization steps (not part of the per-iteration step list):

1. **Deferred work** — `claude --permission-mode acceptEdits --model sonnet -p <deferred-work.md>` (no ISSUENUMBER/STARTINGSHA prepended)
2. **Lessons learned** — `claude --permission-mode acceptEdits --model sonnet -p <lessons-learned.md>` (no ISSUENUMBER/STARTINGSHA prepended)
3. **Final git push** — `git push`

During finalization, the status header switches to show `Finalizing 1/3`, `2/3`, `3/3` instead of an iteration number. The step tracker row is replaced with the three finalization step names.

These finalization steps are defined separately from `ralph-steps.json` — either as a second JSON array (`ralph-finalize-steps.json`) or as a hardcoded list, since they are fixed and few.

### Command template variables

Non-claude step commands may contain `{{ISSUE_ID}}` which is replaced at runtime with the current issue number before execution. This allows `close_gh_issue` to receive the issue number as an argument.

Script paths in commands (e.g., `scripts/close_gh_issue`) are resolved relative to `ralphDir` (the ralph directory, parent of `ralph-tui/`), not relative to the executable or the working directory. The orchestration code prepends `ralphDir` to script paths before execution.

Non-claude steps (git push, close_gh_issue) must also capture stderr — the bash script uses `2>&1` on these commands. For non-claude subprocess execution, use the same pipe-merging approach as claude steps so error output appears in the TUI log.

### Working directory

All subprocesses (`claude`, `git push`, scripts) must run with `cmd.Dir` set to the user's current working directory (the target repo being worked on), not the ralph directory. This matches how `ralph-loop` works — it's invoked from the target repo and all commands inherit that cwd. The TUI captures `os.Getwd()` at startup and passes it to all subprocess calls.

### Subprocess streaming

```go
// logReader and logWriter are created once at startup via io.Pipe().
// logReader is passed to Glyph's Log() component; logWriter is shared across all steps.
// Glyph's Log internally spawns its own goroutine with bufio.Scanner and its own sync.Mutex,
// so no external synchronization is needed — just write to logWriter.

ctx, cancel := context.WithCancel(context.Background())
cmd := exec.CommandContext(ctx, "claude", "--permission-mode", "acceptEdits", "--verbose", "--model", model, "-p", prompt)
cmd.Dir = workingDir  // must run in the target repo, not in ralph-tui/

// Capture both stdout and stderr via separate pipes, merged by two goroutines
// writing to the shared logWriter (and the file logger).
// Do NOT use io.MultiReader — it reads sequentially, hiding stderr until process exits.
stdoutPipe, _ := cmd.StdoutPipe()
stderrPipe, _ := cmd.StderrPipe()
cmd.Start()

var wg sync.WaitGroup
forwardPipe := func(pipe io.ReadCloser) {
    defer wg.Done()
    scanner := bufio.NewScanner(pipe)
    scanner.Buffer(make([]byte, 256*1024), 256*1024)
    for scanner.Scan() {
        line := scanner.Text()
        fmt.Fprintln(logWriter, line)   // Glyph's Log reads this via logReader
        fileLogger.Log(stepName, line)  // also write to the log file
    }
}

wg.Add(2)
go forwardPipe(stdoutPipe)
go forwardPipe(stderrPipe)

wg.Wait()    // both pipes must be fully read before Wait()
cmd.Wait()   // collect exit code (safe only after pipes are drained)
cancel()     // clean up context
```

The `cancel` function is stored by the orchestration goroutine so that keyboard handlers (`n` to skip, `q` to quit) can trigger subprocess termination (see "Subprocess termination" below).

**No shared slice or external mutex needed.** Glyph's `Log` component owns its own internal `[]string` buffer and `sync.Mutex`. The orchestration code simply writes lines to the `io.PipeWriter` end, and Glyph's internal `readLoop` goroutine consumes them safely.

> **Note:** The `--verbose` flag is intentionally added for the TUI (not present in `ralph-loop`) to provide richer streaming output during real-time display.

### Subprocess termination

When the user presses `n` (skip step) or confirms `q` (quit), the currently running subprocess must be terminated cleanly:

1. Call `cancel()` on the step's context — this sends SIGKILL via `exec.CommandContext`
2. Alternatively, for a gentler shutdown: call `cmd.Process.Signal(syscall.SIGTERM)`, wait up to 3 seconds, then `cmd.Process.Kill()` if still running
3. The scanner goroutines will exit naturally when the pipes close
4. `wg.Wait()` and `cmd.Wait()` will return, allowing the orchestration goroutine to proceed

**Partial state after termination:** A killed `claude` subprocess may have left uncommitted file changes or partial commits. The plan does NOT automatically roll back — the user pressed `n` intentionally and the next step (or a retry) will see the current state. If the user wants a clean slate, they can quit and manually reset.

### Retry cleanup

When the user presses `r` to retry a failed step:

- The step is re-run from its current state — no automatic `git reset` or rollback
- `STARTING_SHA` is NOT refreshed (it still represents the SHA at the start of the iteration, which is correct — prompts reference it for diffing)
- Output from the failed attempt remains in the log (scroll up to see it); a separator line marks the retry: `── Test writing (retry) ─────────────`
- For idempotent steps (`git push`, `close_gh_issue`), retry is always safe
- For claude steps, the retry runs claude again with the same prompt; claude will see the current file state including any partial changes from the failed attempt

### Signal handling

The TUI must handle SIGINT and SIGTERM gracefully:
- Cancel the current subprocess context (triggering subprocess termination)
- Wait for scanner goroutines to drain
- Flush and close the log file
- Restore terminal state (Glyph likely handles this, but verify)

### Step definitions loaded from JSON

Steps are loaded from a `ralph-steps.json` file located next to the compiled `ralph-tui` executable. The executable finds its own directory at startup using `os.Executable()` (with `filepath.EvalSymlinks` to handle symlinked binaries) and reads the JSON from there. The parent of this directory is `ralphDir` — the root ralph directory used for resolving `prompts/`, `scripts/`, and `logs/`.

```go
type Step struct {
    Name        string   `json:"name"`
    Model       string   `json:"model,omitempty"`       // "sonnet" or "opus"
    PromptFile  string   `json:"promptFile,omitempty"`   // path relative to prompts/
    IsClaude    bool     `json:"isClaude"`               // false for git push, close issue
    Command     []string `json:"command,omitempty"`      // for non-claude steps
    PrependVars bool     `json:"prependVars,omitempty"`  // true for iteration steps, false for finalization
}

func loadSteps() ([]Step, error) {
    exePath, _ := os.Executable()
    exePath, _ = filepath.EvalSymlinks(exePath)  // resolve symlinks
    exeDir := filepath.Dir(exePath)
    // Note: os.Executable() returns a temp path when using `go run`.
    // During development, use `go build && ./ralph-tui` or pass a -ralph-dir flag.
    data, err := os.ReadFile(filepath.Join(exeDir, "ralph-steps.json"))
    if err != nil {
        return nil, fmt.Errorf("could not read ralph-steps.json: %w", err)
    }
    var steps []Step
    if err := json.Unmarshal(data, &steps); err != nil {
        return nil, fmt.Errorf("could not parse ralph-steps.json: %w", err)
    }
    return steps, nil
}
```

**`ralph-steps.json`** (default, lives next to the executable):

```json
[
    {"name": "Feature work",  "model": "sonnet", "promptFile": "feature-work.md",       "isClaude": true, "prependVars": true},
    {"name": "Test planning", "model": "opus",   "promptFile": "test-planning.md",       "isClaude": true, "prependVars": true},
    {"name": "Test writing",  "model": "sonnet", "promptFile": "test-writing.md",        "isClaude": true, "prependVars": true},
    {"name": "Code review",   "model": "opus",   "promptFile": "code-review-changes.md", "isClaude": true, "prependVars": true},
    {"name": "Review fixes",  "model": "sonnet", "promptFile": "code-review-fixes.md",   "isClaude": true, "prependVars": true},
    {"name": "Close issue",   "isClaude": false,  "command": ["scripts/close_gh_issue", "{{ISSUE_ID}}"]},
    {"name": "Update docs",   "model": "sonnet", "promptFile": "update-docs.md",         "isClaude": true, "prependVars": true},
    {"name": "Git push",      "isClaude": false,  "command": ["git", "push"]}
]
```

### Prompt building

Same logic as the bash script — reads the prompt file. For iteration steps, prepends `ISSUENUMBER=` and `STARTINGSHA=`. For finalization steps, the prompt content is used as-is (no variables prepended).

> **Note on `\n`:** The bash script constructs prompts with literal two-character `\n` (not actual newlines) via `PROMPT="ISSUENUMBER=$ISSUE_ID\n"`. The Go version uses `fmt.Sprintf` with real newlines, which is arguably more correct. Claude CLI handles both, so this behavioral difference is harmless.

#### Step pipeline dependencies

Some steps produce intermediate files consumed by later steps — all in the working directory:
- **Test planning** creates `test-plan.md` → **Test writing** reads `@test-plan.md` and deletes it
- **Code review** creates `code-review.md` → **Review fixes** reads `@code-review.md` and deletes it
- Multiple steps append to `progress.txt` (never committed, cleared by lessons-learned finalization)
- **Feature work** appends to `deferred.txt` (never committed, consumed by deferred-work finalization)

These dependencies are encoded in the prompt files themselves, not in the TUI. The TUI does not need to manage these files directly — Claude CLI handles `@file` references.

```go
promptContent, _ := os.ReadFile(filepath.Join(ralphDir, "prompts", step.PromptFile))
var prompt string
if step.PrependVars {
    prompt = fmt.Sprintf("ISSUENUMBER=%s\nSTARTINGSHA=%s\n%s", issueID, startingSHA, promptContent)
} else {
    prompt = string(promptContent)
}
```

---

## Full Log File

Every line goes to both the TUI and a timestamped log file:

```
[2026-04-08 14:23:01] [Iteration 1/3] [Feature work] Starting...
[2026-04-08 14:23:02] [Feature work] I'll examine the issue requirements...
[2026-04-08 14:25:30] [Feature work] Completed (exit 0)
[2026-04-08 14:25:31] [Test planning] Starting...
```

Location: `ralph/logs/ralph-YYYY-MM-DD-HHMMSS.log` — one file per run. The `logs/` directory is resolved relative to `ralphDir` and created with `os.MkdirAll` at startup if it doesn't exist.

Written via a simple `io.Writer` that timestamps and prefixes each line with the current step name. The log writer is called from the same scanner goroutines that append to `outputLines`, so it must be safe for concurrent writes — either use a mutex-protected writer or a dedicated log goroutine consuming from a channel.

---

## File Structure

```
ralph/
  ralph-loop              # existing bash script (unchanged, serves as fallback)
  ralph-tui/              # new Go module
    main.go               # entry point, CLI arg parsing, app setup
    workflow.go            # orchestration logic (the loop, step execution)
    ui.go                  # Glyph view definitions, keyboard handlers
    steps.go              # step loading from JSON and prompt building
    logger.go             # log file writer
    ralph-steps.json      # default step definitions (lives next to compiled binary)
    go.mod
    go.sum
  prompts/                # read by both ralph-loop and ralph-tui
  scripts/                # still used by ralph-loop; ralph-tui calls gh/git directly
  logs/                   # new directory, written by ralph-tui
```

---

## Acceptance Criteria & Test Plans

### Steps loading (`steps.go`)

**Acceptance criteria:**
- Loads and parses `ralph-steps.json` from the directory containing the executable
- Returns an error if the file is missing or contains invalid JSON
- Each step has a `Name`; claude steps have `Model` and `PromptFile`; non-claude steps have `Command`
- Resolves symlinked executable paths before determining the directory
- Loads finalization steps from `ralph-finalize-steps.json` (or hardcoded list)

**Unit tests:**
- Parse valid JSON into `[]Step` and verify all fields are populated correctly
- Parse JSON with missing optional fields (`model`, `command`) — verify zero values, no error
- Return a descriptive error for malformed JSON
- Return a descriptive error when file does not exist
- Verify step count and ordering matches the JSON array order

### Prompt building (`steps.go`)

**Acceptance criteria:**
- Reads the prompt file from `ralphDir/prompts/<promptFile>`
- When `prependVars` is true, prepends `ISSUENUMBER=<id>\nSTARTINGSHA=<sha>\n` before prompt content
- When `prependVars` is false, returns prompt content as-is
- Returns an error if the prompt file cannot be read

**Unit tests:**
- Build a prompt with `prependVars: true` — verify the output starts with the two variable lines followed by file content
- Build a prompt with `prependVars: false` — verify output equals raw file content exactly
- Return error when prompt file path does not exist
- Verify real newlines are used (not literal `\n` characters)

### Command template variables (`workflow.go`)

**Acceptance criteria:**
- `{{ISSUE_ID}}` in non-claude step commands is replaced with the current issue number
- Script paths are resolved relative to `ralphDir`, not cwd or executable dir
- If `{{ISSUE_ID}}` is absent from a command, the command is passed through unchanged

**Unit tests:**
- Replace `{{ISSUE_ID}}` in `["scripts/close_gh_issue", "{{ISSUE_ID}}"]` with `"42"` — verify result is `["scripts/close_gh_issue", "42"]`
- Command with no template variables passes through unchanged
- Multiple occurrences of `{{ISSUE_ID}}` in a single command array are all replaced
- Script path `scripts/close_gh_issue` is resolved to `<ralphDir>/scripts/close_gh_issue`

### Log file writer (`logger.go`)

**Acceptance criteria:**
- Creates `ralph/logs/ralph-YYYY-MM-DD-HHMMSS.log` at startup, creating the `logs/` directory if needed
- Each line is prefixed with `[timestamp] [iteration context] [step name]`
- Writer is safe for concurrent use (two goroutines writing stdout/stderr simultaneously)
- Log file is flushed and closed on shutdown

**Unit tests:**
- Write lines from a single goroutine — verify each line has timestamp and step prefix
- Write lines from two goroutines concurrently — verify no interleaved/corrupted lines
- Step name changes between writes — verify the prefix updates accordingly
- Log file is created in the expected directory with the expected filename pattern
- `logs/` directory is created if it doesn't exist

### Subprocess streaming (`workflow.go`)

**Acceptance criteria:**
- stdout and stderr from subprocesses are both forwarded to the shared `io.PipeWriter`
- Lines appear in the TUI log in real-time as the subprocess produces them
- Both pipes are fully drained before `cmd.Wait()` is called
- Scanner buffer is large enough (256KB) for long lines
- Each line is also written to the file logger

**Unit tests:**
- Run a subprocess that writes to stdout — verify all lines arrive through the pipe reader
- Run a subprocess that writes to stderr — verify stderr lines also arrive through the pipe reader
- Run a subprocess that writes to both stdout and stderr — verify all lines arrive (order may interleave)
- Verify `cmd.Wait()` is not called until both scanner goroutines finish (use `WaitGroup` correctly)

**Integration tests:**
- Run a real subprocess (e.g., `echo` or a small test script) end-to-end through the streaming pipeline and verify output appears in both the pipe and the log file

### Subprocess termination (`workflow.go`)

**Acceptance criteria:**
- Calling `cancel()` on the step context terminates the running subprocess
- Scanner goroutines exit after pipes close
- `wg.Wait()` and `cmd.Wait()` return after termination, allowing the orchestration goroutine to proceed
- No zombie processes are left behind

**Unit tests:**
- Start a long-running subprocess, cancel context, verify `cmd.Wait()` returns within a reasonable timeout
- After cancellation, verify the pipe reader receives EOF (scanner goroutines exit)

**Integration tests:**
- Start a subprocess via the full streaming pipeline, cancel it mid-stream, verify the orchestration can proceed to the next step

### Orchestration workflow (`workflow.go`)

**Acceptance criteria:**
- Runs N iterations as specified by the CLI argument
- Each iteration: fetches next issue, gets current SHA, runs all steps sequentially
- If `get_next_issue` returns empty, the iteration is skipped and the loop exits early
- After all iterations, runs the finalization phase (deferred work, lessons learned, final push)
- Displays startup banner from `ralph-art.txt`
- Displays completion summary after all work finishes
- All subprocesses run with `cmd.Dir` set to the user's cwd (not ralph dir)

**Unit tests (with stubbed subprocess execution):**
- Run 1 iteration with all steps succeeding — verify each step is called in order
- Run 2 iterations — verify the loop executes twice with correct issue/SHA per iteration
- `get_next_issue` returns empty — verify the loop exits early with a skip message
- Verify `cmd.Dir` is set to the captured working directory for every subprocess
- Verify finalization steps run after iteration loop completes
- Verify startup banner content is written to the log pipe

**Integration tests:**
- Run the orchestration with a mock `gh` / `claude` that exits immediately, verify the full flow from start to completion summary

### UI: Status header (`ui.go`)

**Acceptance criteria:**
- Displays current iteration number and total (e.g., `Iteration 1/3`)
- Displays the issue being worked (e.g., `Issue #42: Add widget support`)
- Shows 8 step checkboxes across two rows
- Step indicators: `[✓]` done, `[▸ ...]` active with spinner, `[ ]` pending
- During finalization, shows `Finalizing 1/3` and finalization step names instead

**Unit tests:**
- Set iteration to 2/5 — verify the iteration line string is formatted correctly
- Mark steps 1-3 as done, step 4 as active, rest pending — verify checkbox states
- Switch to finalization mode — verify header shows `Finalizing` and finalization step names
- All 8 steps fit across two rows of 4

### UI: Output log (`ui.go`)

**Acceptance criteria:**
- `Log` component receives the read end of the `io.Pipe`
- Auto-scrolls to bottom as new lines arrive
- User can scroll up; auto-scroll pauses when scrolled up
- Scrolling back to bottom re-enables auto-scroll
- Step transitions insert a visual separator line
- All steps' output accumulates in one continuous log

**Unit tests:**
- Verify the separator line format matches `── <step name> ─────────────`
- Verify retry separator includes `(retry)` suffix

### UI: Error state (`ui.go`)

**Acceptance criteria:**
- When a step fails (non-zero exit), the step checkbox shows `[✗]`
- Shortcut bar changes to `c continue  r retry  q quit`
- `c` skips the failed step and continues to the next
- `r` retries the failed step from scratch (output from failed attempt remains in log)
- `q` triggers quit confirmation

**Unit tests:**
- Trigger error state — verify checkbox text changes to `[✗]`
- Trigger error state — verify shortcut bar text updates
- Press `c` in error state — verify the workflow advances to the next step
- Press `r` in error state — verify the step is re-executed and separator includes `(retry)`

### UI: Quit confirmation (`ui.go`)

**Acceptance criteria:**
- Pressing `q` at any time shows `Quit ralph? (y/n)` in the shortcut bar area
- `y` terminates the current subprocess and exits the TUI
- `n` dismisses the confirmation and returns to normal operation
- Any other key is ignored while confirmation is shown

**Unit tests:**
- Press `q` — verify confirmation prompt is displayed
- Press `q` then `n` — verify normal shortcut bar is restored
- Press `q` then `y` — verify quit is triggered
- Press `q` then any other key — verify confirmation remains shown

### UI: Keyboard shortcuts (`ui.go`)

**Acceptance criteria:**
- `↑`/`k` scrolls log up, `↓`/`j` scrolls log down (handled by Glyph's `BindVimNav`)
- `n` skips the current step (terminates subprocess, advances to next)
- `q` triggers quit confirmation
- In error state: `c` continues, `r` retries

**Unit tests:**
- Press `n` during a running step — verify subprocess cancellation is triggered and workflow advances
- Verify keyboard handler dispatches correctly based on current state (normal vs error vs quit-confirm)

### Signal handling (`workflow.go` / `main.go`)

**Acceptance criteria:**
- SIGINT and SIGTERM cancel the current subprocess context
- Scanner goroutines drain before exit
- Log file is flushed and closed
- Terminal state is restored

**Integration tests:**
- Send SIGINT to the process during a running step — verify clean shutdown (no panic, log file closed, terminal restored)

---

## Verification

1. `cd ralph/ralph-tui && go build` — compiles
2. Run with `./ralph-tui 1` against a real repo with a "ralph" labeled issue
3. Verify: output streams line-by-line in the log panel as claude runs
4. Verify: step checkboxes update as steps complete
5. Verify: `j`/`k`/arrows scroll the log, auto-scroll resumes at bottom
6. Verify: `n` skips current step, `q` prompts confirmation
7. Verify: log file appears in `ralph/logs/` with correct timestamps
8. Verify: error state appears on subprocess failure with `c`/`r`/`q` options
9. Verify: finalization phase runs after iterations (deferred work, lessons learned, final push)
10. Verify: if no issue is found, the iteration is skipped and the loop exits gracefully
11. Verify: `close_gh_issue` receives the issue number as an argument
12. Verify: `n` terminates the running subprocess before advancing
13. Verify: SIGINT/SIGTERM kills child processes and exits cleanly
14. Verify: stderr output from `git push` and `close_gh_issue` appears in the TUI

---

## Review Summary

**Iterations completed:** 3 (stopped at iteration 3 — below 80% probability of meaningful structural improvement)

**Assumptions challenged:** 18 total
- 7 Verified, 8 Refuted, 3 Uncertain
- Key refutations: missing finalization phase, broken stderr merging strategy, missing subprocess termination on skip, missing empty-issue handling, close_gh_issue missing argument

**Agent validation findings incorporated:**
- **Critical (2 fixed):** Replaced `io.MultiReader` with two-goroutine pipe scanning + WaitGroup; added subprocess termination via `context.WithCancel` for skip/quit
- **Medium (4 fixed):** Added Glyph concurrency verification note; added retry cleanup specification; committed to single stderr strategy for all steps; added signal handling section
- **Low (1 noted):** Template variable empty-string edge case covered by early-exit logic

**Consolidations made:** 0 (no internal overlap found)

**Ambiguities surfaced and resolved:** 1 — Glyph `Log` component takes `io.Reader` (not `*[]string`), manages its own internal `sync.Mutex` and background goroutine. Plan architecture updated from shared-slice+mutex to `io.Pipe` pattern, eliminating all concurrency concerns.

**Other additions from evidence-based investigation:**
- Documented step pipeline dependencies (test-plan.md, code-review.md, progress.txt, deferred.txt)
- Added startup banner (ralph-art.txt) and completion summary message
- Documented `\n` literal vs real newline behavioral difference (harmless)
- Added log writer concurrency note
- Added `go run` development caveat for `os.Executable()`
