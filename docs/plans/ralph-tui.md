# Plan: `ralph-tui` — Go/Glyph TUI for Ralph Loop

## Context

The `ralph-bash/ralph-loop` bash script runs multiple sequential `claude` CLI calls, capturing all output into `$RESULT` and displaying it after-the-fact with `box-text`. This makes it impossible to see what's happening during long-running steps. We're building a Go program using [Glyph](https://useglyph.sh/) that replaces `ralph-loop` as the orchestrator, streaming claude output in real-time into a bordered, scrolling TUI panel with full workflow visibility.

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
- `c` — skip this step, continue to the next one. **Caution:** Skipping a claude step may leave downstream steps in a broken state (e.g., skipping "Code review" means `code-review.md` won't exist for "Review fixes"). The TUI does not prevent this — the user takes responsibility for the consequences. Skipping directly to "Close issue" or "Git push" after a failure could close an issue or push broken code.
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

1. Display `ralph-bash/ralph-art.txt` contents in the output log as a startup banner
2. Get GitHub username via `gh api user` (equivalent to `ralph-bash/scripts/get_gh_user`)
3. Loop N iterations:
   a. Get next issue by calling `ralph-bash/scripts/get_next_issue <USERNAME>` (the username from step 2 is passed as the first argument)
   b. Get current git SHA by calling `ralph-bash/scripts/get_commit_sha` (equivalent to `git rev-parse HEAD`)
   c. Run each step sequentially (spawn subprocess, stream output, wait for exit)
4. Run finalization steps (deferred work, lessons learned, final push)
5. Display completion summary: "Ralph completed after N iteration(s) and 3 finalizing tasks." — where N is the number of iterations actually executed (not the requested count), tracked by incrementing a counter at the end of each iteration loop body.

If `get_next_issue` returns an empty string (no issues labeled "ralph" assigned to the user), the orchestration goroutine should log a message to the TUI output, mark the iteration as skipped, and exit the loop early. **Note:** The bash script calls `exit 0` here, skipping finalization entirely. The TUI intentionally differs — finalization still runs after the loop exits, because `progress.txt` or `deferred.txt` from prior iterations may still need processing.

This goroutine communicates with the TUI by:
- **Outbound (workflow → UI):** Mutating shared state via pointers (which Glyph reads on each render cycle). E.g., updating `iterationLine`, step checkbox states, `shortcutLine`.
- **Inbound (UI → workflow):** A `chan StepAction` channel. After a step fails, the orchestration goroutine blocks on this channel. The keyboard handler sends `ActionRetry`, `ActionContinue`, or `ActionQuit`. For `n` (skip during a running step), the keyboard handler calls `cancel()` directly and the orchestration goroutine detects the cancellation via `cmd.Wait()` returning a context error.

```go
type StepAction int
const (
    ActionRetry StepAction = iota
    ActionContinue
    ActionQuit
)
```

### Finalization phase

After all iterations complete, the orchestration goroutine runs three finalization steps (not part of the per-iteration step list):

1. **Deferred work** — `claude --permission-mode acceptEdits --model sonnet -p <deferred-work.md>` (no ISSUENUMBER/STARTINGSHA prepended)
2. **Lessons learned** — `claude --permission-mode acceptEdits --model sonnet -p <lessons-learned.md>` (no ISSUENUMBER/STARTINGSHA prepended)
3. **Final git push** — `git push`

During finalization, the status header switches to show `Finalizing 1/3`, `2/3`, `3/3` instead of an iteration number. The step tracker row is replaced with the three finalization step names.

These finalization steps are defined separately from `ralph-steps.json` — either as a second JSON array (`configs/ralph-finalize-steps.json`) or as a hardcoded list, since they are fixed and few.

### Directory resolution

The `ralph-tui` executable lives at the repo root alongside `prompts/` and `ralph-bash/`. At startup, the executable resolves `projectDir` — the repo root — using `os.Executable()` (with `filepath.EvalSymlinks`). All paths are resolved relative to `projectDir`:

- Prompt files: `projectDir/prompts/<promptFile>`
- Scripts: `projectDir/ralph-bash/scripts/<script>`
- Logs: `projectDir/logs/`
- Art: `projectDir/ralph-bash/ralph-art.txt`
- Step definitions: `projectDir/ralph-tui/configs/ralph-steps.json`

### Command template variables

Non-claude step commands may contain `{{ISSUE_ID}}` which is replaced at runtime with the current issue number before execution. This allows `close_gh_issue` to receive the issue number as an argument.

Script paths in commands (e.g., `ralph-bash/scripts/close_gh_issue`) are resolved relative to `projectDir` (the repo root), not relative to the executable or the working directory. The orchestration code prepends `projectDir` to script paths before execution.

Non-claude steps (git push, close_gh_issue) must also capture stderr — the bash script uses `2>&1` on these commands. For non-claude subprocess execution, use the same pipe-merging approach as claude steps so error output appears in the TUI log.

### Working directory

All subprocesses (`claude`, `git push`, scripts) must run with `cmd.Dir` set to the user's current working directory (the target repo being worked on), not the ralph directory. This matches how `ralph-bash/ralph-loop` works — it's invoked from the target repo and all commands inherit that cwd. The TUI captures `os.Getwd()` at startup and passes it to all subprocess calls.

### Subprocess streaming

```go
// logReader and logWriter are created once at startup via io.Pipe().
// logReader is passed to Glyph's Log() component; logWriter is shared across all steps.
//
// IMPORTANT: io.PipeWriter is NOT safe for concurrent writes from multiple goroutines.
// A sync.Mutex protects all writes to logWriter to prevent interleaved/corrupted lines.
// logWriter.Close() must be called after all steps (including finalization) complete,
// so Glyph's internal reader goroutine receives EOF and can exit cleanly.

var logMu sync.Mutex  // protects writes to logWriter

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
        logMu.Lock()
        fmt.Fprintln(logWriter, line)   // Glyph's Log reads this via logReader
        logMu.Unlock()
        fileLogger.Log(stepName, line)  // also write to the log file
    }
    if err := scanner.Err(); err != nil {
        logMu.Lock()
        fmt.Fprintf(logWriter, "[warning] scanner error on %s: %v\n", stepName, err)
        logMu.Unlock()
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

**Write-side mutex required.** Although Glyph's `Log` component owns its own internal buffer and mutex on the read side, `io.PipeWriter` is not safe for concurrent writes from multiple goroutines. A `sync.Mutex` (`logMu` above) must protect all writes to `logWriter`. Glyph's internal `readLoop` goroutine consumes from `logReader` safely.

**Pipe lifecycle:** `logWriter.Close()` must be called after all steps (including finalization) complete. This sends EOF to Glyph's internal reader goroutine, allowing it to exit cleanly. Without this, the program will hang on exit.

> **Note:** The `--verbose` flag is intentionally added for the TUI (not present in `ralph-loop`) to provide richer streaming output during real-time display.

### Subprocess termination

When the user presses `n` (skip step) or confirms `q` (quit), the currently running subprocess must be terminated cleanly:

1. Send `syscall.SIGTERM` to the subprocess via `cmd.Process.Signal(syscall.SIGTERM)` — gives claude a chance to flush partial work and exit cleanly
2. Wait up to 3 seconds for the process to exit
3. If still running after 3 seconds, call `cmd.Process.Kill()` (SIGKILL) as a fallback
4. **Do not use `exec.CommandContext` cancellation as the primary termination mechanism** — it sends SIGKILL immediately, which can corrupt files mid-write. Instead, manage termination explicitly with SIGTERM-then-SIGKILL. Use `exec.CommandContext` only for the context plumbing (timeout propagation), and override the kill behavior
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

Steps are loaded from `ralph-tui/configs/ralph-steps.json`. The executable finds `projectDir` (the repo root) at startup using `os.Executable()` (with `filepath.EvalSymlinks` to handle symlinked binaries). All paths — `prompts/`, `ralph-bash/scripts/`, `logs/`, and step definitions — are resolved relative to `projectDir`.

```go
type Step struct {
    Name        string   `json:"name"`
    Model       string   `json:"model,omitempty"`       // "sonnet" or "opus"
    PromptFile  string   `json:"promptFile,omitempty"`   // path relative to prompts/
    IsClaude    bool     `json:"isClaude"`               // false for git push, close issue
    Command     []string `json:"command,omitempty"`      // for non-claude steps
    PrependVars bool     `json:"prependVars,omitempty"`  // true for iteration steps, false for finalization
}

func loadSteps(projectDir string) ([]Step, error) {
    // projectDir is the repo root, resolved at startup via os.Executable().
    // Note: os.Executable() returns a temp path when using `go run`.
    // During development, use `go build` or pass the -project-dir flag (see CLI args below).
    data, err := os.ReadFile(filepath.Join(projectDir, "ralph-tui", "configs", "ralph-steps.json"))
    if err != nil {
        return nil, fmt.Errorf("could not read configs/ralph-steps.json: %w", err)
    }
    var steps []Step
    if err := json.Unmarshal(data, &steps); err != nil {
        return nil, fmt.Errorf("could not parse ralph-steps.json: %w", err)
    }
    return steps, nil
}
```

**`configs/ralph-steps.json`** (lives in `ralph-tui/configs/`):

```json
[
    {"name": "Feature work",  "model": "sonnet", "promptFile": "feature-work.md",       "isClaude": true, "prependVars": true},
    {"name": "Test planning", "model": "opus",   "promptFile": "test-planning.md",       "isClaude": true, "prependVars": true},
    {"name": "Test writing",  "model": "sonnet", "promptFile": "test-writing.md",        "isClaude": true, "prependVars": true},
    {"name": "Code review",   "model": "opus",   "promptFile": "code-review-changes.md", "isClaude": true, "prependVars": true},
    {"name": "Review fixes",  "model": "sonnet", "promptFile": "code-review-fixes.md",   "isClaude": true, "prependVars": true},
    {"name": "Close issue",   "isClaude": false,  "command": ["ralph-bash/scripts/close_gh_issue", "{{ISSUE_ID}}"]},
    {"name": "Update docs",   "model": "sonnet", "promptFile": "update-docs.md",         "isClaude": true, "prependVars": true},
    {"name": "Git push",      "isClaude": false,  "command": ["git", "push"]}
]
```

### Prompt building

Same logic as the bash script (`ralph-bash/ralph-loop`) — reads the prompt file. For iteration steps, prepends `ISSUENUMBER=` and `STARTINGSHA=`. For finalization steps, the prompt content is used as-is (no variables prepended).

> **Note on `\n`:** The bash script constructs prompts with literal two-character `\n` (not actual newlines) via `PROMPT="ISSUENUMBER=$ISSUE_ID\n"`. The Go version uses `fmt.Sprintf` with real newlines, which is arguably more correct. Claude CLI handles both, so this behavioral difference is harmless.

#### Step pipeline dependencies

Some steps produce intermediate files consumed by later steps — all in the working directory:
- **Test planning** creates `test-plan.md` → **Test writing** reads `@test-plan.md` and deletes it
- **Code review** creates `code-review.md` → **Review fixes** reads `@code-review.md` and deletes it
- Multiple steps append to `progress.txt` (never committed, cleared by lessons-learned finalization)
- **Feature work** appends to `deferred.txt` (never committed, consumed by deferred-work finalization)

These dependencies are encoded in the prompt files themselves, not in the TUI. The TUI does not need to manage these files directly — Claude CLI handles `@file` references.

```go
promptContent, _ := os.ReadFile(filepath.Join(projectDir, "prompts", step.PromptFile))  // prompts/ is at repo root
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

Location: `logs/ralph-YYYY-MM-DD-HHMMSS.log` — one file per run. The `logs/` directory is at the repo root, resolved relative to `projectDir`, and created with `os.MkdirAll` at startup if it doesn't exist.

Written via a simple `io.Writer` that timestamps and prefixes each line with the current step name. The log writer is called from the same scanner goroutines that write to the `io.PipeWriter`, so it must be safe for concurrent writes — either use a mutex-protected writer or a dedicated log goroutine consuming from a channel.

---

## File Structure

Follows the [golang-standards/project-layout](https://github.com/golang-standards/project-layout) convention.

```
pr9k/                               # repo root (projectDir)
  ralph-bash/                       # existing bash scripts (fallback)
    ralph-loop                      # original orchestrator
    ralph-art.txt                   # startup banner art
    scripts/                        # helper scripts (get_next_issue, close_gh_issue, etc.)
  ralph-tui/                        # new Go module
    cmd/
      ralph-tui/
        main.go                     # entry point, CLI arg parsing, app setup
    internal/
      workflow/
        workflow.go                 # orchestration logic (the loop, step execution)
        workflow_test.go
      ui/
        ui.go                       # Glyph view definitions, keyboard handlers
        ui_test.go
      steps/
        steps.go                    # step loading from JSON and prompt building
        steps_test.go
      logger/
        logger.go                   # log file writer
        logger_test.go
    configs/
      ralph-steps.json              # default iteration step definitions
      ralph-finalize-steps.json     # finalization step definitions
    go.mod
    go.sum
  prompts/                          # read by both ralph-bash and ralph-tui
  logs/                             # written by ralph-tui (created at runtime)
  docs/plans/                       # this plan and others
```

### Directory purposes (per golang-standards/project-layout)

- **`cmd/ralph-tui/`** — Main application entry point. Minimal code here; imports from `internal/` packages. The subdirectory name matches the desired executable name.
- **`internal/`** — Private application code. The Go compiler enforces that nothing outside `ralph-tui/` can import these packages, keeping implementation details private.
  - `internal/workflow/` — Orchestration loop, subprocess streaming, subprocess termination, command template variable replacement, signal handling.
  - `internal/ui/` — Glyph component tree, keyboard handlers, status header, error state, quit confirmation.
  - `internal/steps/` — Step definition loading from JSON, prompt building.
  - `internal/logger/` — Timestamped log file writer with concurrent write safety.
- **`configs/`** — Configuration files (step definitions). Not Go code — JSON consumed at runtime.

---

## CLI Arguments

```
ralph-tui <iterations> [-project-dir <path>]
```

- **`<iterations>`** (required) — Number of iterations to run. Must be > 0.
- **`-project-dir <path>`** (optional) — Override the auto-detected project directory (repo root). Useful during development with `go run`, where `os.Executable()` returns a temp path. When omitted, `projectDir` is resolved via `os.Executable()` with `filepath.EvalSymlinks`.

---

## Acceptance Criteria & Test Plans

### Steps loading (`internal/steps/steps.go`)

**Acceptance criteria:**
- Loads and parses `configs/ralph-steps.json` from the project directory
- Returns an error if the file is missing or contains invalid JSON
- Each step has a `Name`; claude steps have `Model` and `PromptFile`; non-claude steps have `Command`
- Resolves symlinked executable paths before determining the directory
- Loads finalization steps from `configs/ralph-finalize-steps.json` (or hardcoded list)

**Unit tests:**
- Parse valid JSON into `[]Step` and verify all fields are populated correctly
- Parse JSON with missing optional fields (`model`, `command`) — verify zero values, no error
- Return a descriptive error for malformed JSON
- Return a descriptive error when file does not exist
- Verify step count and ordering matches the JSON array order

### Prompt building (`internal/steps/steps.go`)

**Acceptance criteria:**
- Reads the prompt file from `projectDir/prompts/<promptFile>`
- When `prependVars` is true, prepends `ISSUENUMBER=<id>\nSTARTINGSHA=<sha>\n` before prompt content
- When `prependVars` is false, returns prompt content as-is
- Returns an error if the prompt file cannot be read

**Unit tests:**
- Build a prompt with `prependVars: true` — verify the output starts with the two variable lines followed by file content
- Build a prompt with `prependVars: false` — verify output equals raw file content exactly
- Return error when prompt file path does not exist
- Verify real newlines are used (not literal `\n` characters)

### Command template variables (`internal/workflow/workflow.go`)

**Acceptance criteria:**
- `{{ISSUE_ID}}` in non-claude step commands is replaced with the current issue number
- Script paths are resolved relative to `projectDir`, not cwd or executable dir
- If `{{ISSUE_ID}}` is absent from a command, the command is passed through unchanged

**Unit tests:**
- Replace `{{ISSUE_ID}}` in `["ralph-bash/scripts/close_gh_issue", "{{ISSUE_ID}}"]` with `"42"` — verify result is `["ralph-bash/scripts/close_gh_issue", "42"]`
- Command with no template variables passes through unchanged
- Multiple occurrences of `{{ISSUE_ID}}` in a single command array are all replaced
- Script path `ralph-bash/scripts/close_gh_issue` is resolved to `<projectDir>/ralph-bash/scripts/close_gh_issue`

### Log file writer (`internal/logger/logger.go`)

**Acceptance criteria:**
- Creates `logs/ralph-YYYY-MM-DD-HHMMSS.log` at startup, creating the `logs/` directory if needed
- Each line is prefixed with `[timestamp] [iteration context] [step name]`
- Writer is safe for concurrent use (two goroutines writing stdout/stderr simultaneously)
- Log file is flushed and closed on shutdown

**Unit tests:**
- Write lines from a single goroutine — verify each line has timestamp and step prefix
- Write lines from two goroutines concurrently — verify no interleaved/corrupted lines
- Step name changes between writes — verify the prefix updates accordingly
- Log file is created in the expected directory with the expected filename pattern
- `logs/` directory is created if it doesn't exist

### Subprocess streaming (`internal/workflow/workflow.go`)

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

### Subprocess termination (`internal/workflow/workflow.go`)

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

### Orchestration workflow (`internal/workflow/workflow.go`)

**Acceptance criteria:**
- Runs N iterations as specified by the CLI argument
- Each iteration: fetches next issue, gets current SHA, runs all steps sequentially
- If `get_next_issue` returns empty, the iteration is skipped and the loop exits early
- After all iterations, runs the finalization phase (deferred work, lessons learned, final push)
- Displays startup banner from `ralph-bash/ralph-art.txt`
- Displays completion summary after all work finishes
- All subprocesses run with `cmd.Dir` set to the user's cwd (not ralph dir)

**Unit tests (with stubbed subprocess execution):**
- Run 1 iteration with all steps succeeding — verify each step is called in order
- Run 2 iterations — verify the loop executes twice with correct issue/SHA per iteration
- `get_next_issue` returns empty — verify the loop exits early with a skip message
- Verify `cmd.Dir` is set to the captured working directory for every subprocess
- Verify finalization steps run after iteration loop completes
- Verify startup banner content is written to the log pipe
- Verify `get_next_issue` is called with the GitHub username as its argument
- Verify finalization still runs when `get_next_issue` returns empty (differs from bash `exit 0` behavior)

**Integration tests:**
- Run the orchestration with a mock `gh` / `claude` that exits immediately, verify the full flow from start to completion summary

### UI: Status header (`internal/ui/ui.go`)

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

### UI: Output log (`internal/ui/ui.go`)

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

### UI: Error state (`internal/ui/ui.go`)

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

### UI: Quit confirmation (`internal/ui/ui.go`)

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

### UI: Keyboard shortcuts (`internal/ui/ui.go`)

**Acceptance criteria:**
- `↑`/`k` scrolls log up, `↓`/`j` scrolls log down (handled by Glyph's `BindVimNav`)
- `n` skips the current step (terminates subprocess, advances to next)
- `q` triggers quit confirmation
- In error state: `c` continues, `r` retries

**Unit tests:**
- Press `n` during a running step — verify subprocess cancellation is triggered and workflow advances
- Verify keyboard handler dispatches correctly based on current state (normal vs error vs quit-confirm)

### Signal handling (`internal/workflow/workflow.go` / `cmd/ralph-tui/main.go`)

**Acceptance criteria:**
- SIGINT and SIGTERM cancel the current subprocess context
- Scanner goroutines drain before exit
- Log file is flushed and closed
- Terminal state is restored

**Integration tests:**
- Send SIGINT to the process during a running step — verify clean shutdown (no panic, log file closed, terminal restored)

---

## Verification

1. `cd ralph-tui && go build -o ../ralph-tui ./cmd/ralph-tui` — compiles and places binary at repo root (required for `os.Executable()` directory resolution)
2. From the target repo, run with `path/to/pr9k/ralph-tui 1`
3. Verify: output streams line-by-line in the log panel as claude runs
4. Verify: step checkboxes update as steps complete
5. Verify: `j`/`k`/arrows scroll the log, auto-scroll resumes at bottom
6. Verify: `n` skips current step, `q` prompts confirmation
7. Verify: log file appears in `logs/` with correct timestamps
8. Verify: error state appears on subprocess failure with `c`/`r`/`q` options
9. Verify: finalization phase runs after iterations (deferred work, lessons learned, final push)
10. Verify: if no issue is found, the iteration is skipped and the loop exits gracefully
11. Verify: `close_gh_issue` receives the issue number as an argument
12. Verify: `n` terminates the running subprocess before advancing
13. Verify: SIGINT/SIGTERM kills child processes and exits cleanly
14. Verify: stderr output from `git push` and `close_gh_issue` appears in the TUI

---

## Review Summary

### Original Review (prior to iterative plan review)

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

### Iterative Plan Review (2026-04-08)

**Iterations completed:** 3 (stopped — below 80% probability of further structural improvement)

**Assumptions challenged:** 8 new assumptions across 3 iterations
- Iteration 1 (4): `get_next_issue` missing USERNAME argument (refuted), `get_commit_sha` invocation unspecified (refuted), stale `outputLines` reference (refuted), prompt file locations correct (verified)
- Iteration 2 (2): Build command inconsistent with directory resolution (refuted), exit-loop-early vs bash `exit 0` behavior (uncertain → documented as intentional)
- Iteration 3 (2): `-project-dir` flag undocumented (refuted), orchestration tests missing USERNAME verification (gap)

**Agent validation (Glyph API verification):**
All 11 Glyph API assumptions verified against pkg.go.dev: `Log(r io.Reader) LogC`, `.AutoScroll(bool)`, `.BindVimNav()`, `.OnUpdate(func())`, `.MaxLines(int)`, `.Grow(any)`, `VBox`, `HBox`, `Border(BorderRounded)`, `Spinner`, `app.RequestRender()`.

**Agent validation (adversarial — 10 findings, 6 incorporated):**
- **Critical (2 fixed):** `io.PipeWriter` concurrent writes unsafe — added `sync.Mutex` around all writes; `io.Pipe` never closed — added lifecycle documentation requiring `logWriter.Close()` after finalization
- **High (1 fixed):** Keyboard→orchestration communication unspecified — added `chan StepAction` protocol with `ActionRetry`/`ActionContinue`/`ActionQuit`
- **Medium (2 fixed):** SIGTERM should be default over SIGKILL — updated subprocess termination to use SIGTERM-then-SIGKILL; scanner `ErrTooLong` silently lost — added error check after scan loop
- **Low (1 fixed):** `ITERATIONS_RUN` tracking — clarified completion message uses actual count
- **Acknowledged but not changed (4):** Continue-after-failure guardrails (added caution note, user takes responsibility); empty `ISSUE_ID` validation (covered by early-exit logic); `--verbose` flag output format (needs runtime verification); `go install` directory resolution (mitigated by `-project-dir` flag)
