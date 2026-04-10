# ralph-tui UX Corrections — Design Plan

**Status:** complete — all 17 design decisions locked. Ready for GitHub issue creation and PR1 implementation.

**Origin:** this plan was produced via a grill-me interview session between the user (River) and Claude (Opus 4.6). The user answered each gating question one at a time; each decision below was locked in only after the user explicitly confirmed it. No decision was pre-committed or quietly added without user sign-off.

**Scope:** addresses two issues in the original user ask — (1) the `ralph-art.txt` banner is printed immediately on startup, (2) the `## Layout` TUI from `docs/plans/ralph-tui.md` never actually appears when running the app — and all the entangled architectural gaps discovered during the audit (no Glyph integration, no keyboard input, no config validation, no pre-loop phase, dead error recovery, hardcoded 8-step cap, hardcoded workflow steps in Go).

---

## Quick decision index (for slicing into GitHub issues)

Each decision below is self-contained and references earlier decisions by number where relevant. Use this index to map design decisions to implementation tickets.

| #    | Decision                                       | Touches (primary files)                                                              | Implemented in PR |
|------|------------------------------------------------|--------------------------------------------------------------------------------------|-------------------|
| D1   | Use Glyph as the rendering framework          | `ralph-tui/go.mod`, `cmd/ralph-tui/main.go`                                          | PR2               |
| D2   | Broad scope — build the complete app          | all                                                                                  | PR1–PR3           |
| D3a  | Validation runs pre-Glyph (stderr + exit 1)   | `cmd/ralph-tui/main.go`, new `internal/validator/`                                   | PR1               |
| D3b  | `initialize` array in `ralph-steps.json`      | `ralph-tui/ralph-steps.json`, `bin/ralph-steps.json`, `internal/steps/steps.go`       | PR1               |
| D3c  | Narrow principle — ralph-tui facilitates, doesn't define | architectural (touches everything)                                       | PR1               |
| D4   | `captureAs` — final non-empty stdout line      | `internal/steps/steps.go`, `internal/workflow/workflow.go`                           | PR1               |
| D5   | Variable scope (Model Y) + built-ins          | new `internal/vars/` or equivalent, `internal/workflow/workflow.go`                  | PR1               |
| D6   | `breakLoopIfEmpty` + `StepSkipped` state      | `internal/steps/steps.go`, `internal/ui/header.go`, `internal/ui/orchestrate.go`, `internal/workflow/run.go` | PR1 (break) + PR3 (StepSkipped marking) |
| D7   | Drop `prependVars`; `{{VAR}}` in prompt files | `internal/steps/steps.go`, `prompts/*.md` (Migration B rewrite)                      | PR1               |
| D8   | Reactive iteration header line                | `internal/ui/header.go`, `internal/workflow/run.go`                                  | PR3               |
| D9   | Per-iteration prologue → first two iteration steps | `ralph-tui/ralph-steps.json`, `bin/ralph-steps.json`, `internal/workflow/run.go` | PR1               |
| D10  | Dynamic checkbox row layout                   | `internal/ui/header.go`, `cmd/ralph-tui/main.go`                                     | PR2               |
| D11  | Move `ralph-art.txt` to repo root + Splash step | `ralph-tui/internal/workflow/ralph-art.txt` → `{repo-root}/ralph-art.txt`, `Makefile`, `internal/workflow/run.go`, `ralph-steps.json` | PR1 |
| D12  | Header line formats per phase (mixed)         | `internal/ui/header.go`                                                              | PR3               |
| D13  | Config validation scope (8 categories)        | new `internal/validator/`                                                            | PR1               |
| D14  | Keyboard wiring via Glyph + error mode        | `cmd/ralph-tui/main.go`, `internal/ui/ui.go`                                         | PR2               |
| D15  | Wait-for-keypress completion sequence         | `internal/ui/ui.go`, `internal/workflow/run.go`                                      | PR3               |
| D16  | Pre-populate first-frame header state         | `cmd/ralph-tui/main.go`                                                              | PR2               |
| D17  | Three-PR phasing (schema → TUI → polish)      | all                                                                                  | the plan itself   |

---

## Problem statement

When running `ralph-tui`, two things do not match expectations:

1. `prompts/ralph-art.txt` (or the embedded equivalent) is printed to the screen immediately on startup. The user still wants to keep the file around but does not want it printed.
2. The TUI layout described in `docs/plans/ralph-tui.md` under `## Layout` — a bordered status header, a scrollable log region, and a shortcut bar — never appears. The app just streams plain text to stdout.

Expected workflow:

1. On startup, validate the config files the app is going to use.
2. Once validation passes, the layout should appear immediately, with the **list of steps** in the header.
3. The header should first show the **pre-loop steps** while they run.
4. When the loop starts, the header should show the **loop (iteration) steps**, updating the iteration number as the loop progresses.
5. After the loop, the header should show the **post-loop steps** as they execute.
6. When everything finishes, the app should exit.

## Audit findings (evidence-based)

The user challenged my initial diagnosis, so before continuing to plan, I audited the codebase. Each finding below is backed by a file read or grep result against `ralph-tui/`.

### A1. Glyph integration — **does not exist**

- `grep -r 'glyph\|bubbletea\|tcell\|termbox\|lipgloss\|charm' ralph-tui/` → zero matches.
- `ralph-tui/go.mod` lists only `github.com/spf13/cobra v1.10.2` and its transitive deps (`mousetrap`, `pflag`).
- The only place `glyph` / `VBox` / `HBox` / `Text(&...)` appear in the whole repo is inside the docs and inside *comments* in `internal/ui/header.go:21-23` that describe how Glyph *would* read the fields.

No TUI framework is wired in at all. Nothing renders.

### A2. TUI rendering code — **does not exist**

- `grep -n 'VBox\|HBox\|Text(&\|Border\|App\.\|\.Grow(\|MaxLines' ralph-tui/` → matches in `header.go` are doc-comment only; matches in `workflow_test.go` are unrelated string helpers (`TestRunStep_StepNameAppearsInLogFile`, etc.).
- `cmd/ralph-tui/main.go:56-63` is the entire "display" path — a `bufio.Scanner` loop that calls `fmt.Println(scanner.Text())` on each line read from the log pipe.

The whole "TUI" right now is literally `fmt.Println` in a goroutine.

### A3. Keyboard input at runtime — **does not exist**

- `grep -n 'os\.Stdin\|tty\|termios\|raw\|Raw' ralph-tui/` → zero matches.
- `grep -n '\.Handle(' ralph-tui/` → 21 matches, **all** in `internal/ui/ui_test.go`. Zero production callers.
- `main.go` only touches `keyHandler` in two places: line 44 (`NewKeyHandler(...)`) and line 81 (`keyHandler.ForceQuit()` inside the signal handler).
- `grep -n 'ShortcutLine' ralph-tui/` → matches in tests and inside `ui.go` itself. No production reader.
- `grep -n 'IterationLine\|Row1\|Row2' ralph-tui/` → matches in tests and inside `header.go` itself. No production reader.

**Implication:** `n`, `q`, `r`, `c`, `y` keys mentioned in the Layout plan do nothing at runtime. The only way to interrupt the process is SIGINT/SIGTERM (`main.go:75-87`).

### A4. Error recovery — **effectively dead**

`internal/ui/orchestrate.go:54-57` enters error mode and blocks on `<-h.Actions`:

```go
header.SetStepState(idx, StepFailed)
h.SetMode(ModeError)

action := <-h.Actions
```

The only production writer to `h.Actions` is the signal handler's `ForceQuit()` (`ui.go:122-130`), which only ever sends `ActionQuit`. Since keyboard input is never wired, `ActionContinue` and `ActionRetry` can never arrive. **On any step failure, the orchestration goroutine blocks forever until the user SIGINTs.** The retry/continue logic is fully tested but unreachable in production.

### A5. Config validation — **does not exist**

- `grep -n 'Valid\|validate\|Validate' ralph-tui/` → zero matches in source or tests.
- `steps.LoadSteps` at `internal/steps/steps.go:31-44` only does `os.ReadFile` + `json.Unmarshal`. There is no check that `promptFile` values reference files under `prompts/`, no check that `command[0]` paths exist, no check on `model` values, no semantic schema validation.

**Implication:** If `ralph-steps.json` references a missing prompt, the error surfaces mid-loop when `steps.BuildPrompt` fails inside `buildIterationSteps` (`internal/workflow/run.go:141-147`). If a non-claude script path is wrong, the error surfaces when `exec.Command` fails inside `RunStep`.

### A6. Pre-loop phase — **does not exist as a concept**

- `grep -n 'preLoop\|pre_loop\|pre-loop\|PreLoop' ralph-tui/` → zero matches.
- `ralph-tui/ralph-steps.json` has exactly two top-level arrays: `"iteration"` and `"finalize"`. No third category.
- `workflow.Run()` at `internal/workflow/run.go:56-92` does three things before entering the loop: (1) `executor.WriteToLog(bannerArt lines)`, (2) `executor.CaptureOutput(get_gh_user script)`, (3) falls straight into `for i := 1; ...`. Neither of those is modeled as a "step" — they have no header state, no step index, no error recovery, no user visibility beyond log output.

The "pre-loop steps" the user expects to see in the header have no representation anywhere in the code.

### A7. Hardcoded 8-step cap — **is a real limit**

- `cmd/ralph-tui/main.go:46-52`:
  ```go
  var stepNames [8]string
  for i, s := range stepFile.Iteration {
      if i >= 8 {
          break
      }
      stepNames[i] = s.Name
  }
  ```
- `internal/ui/header.go:24-28`:
  ```go
  type StatusHeader struct {
      IterationLine string
      Row1          [4]string
      Row2          [4]string
      stepNames     [8]string
      ...
  }
  ```
- `SetFinalization` (`header.go:65-82`) also assumes ≤ 8 finalize steps; anything past index 7 is silently dropped.

**Implication:** Adding pre-loop steps, or letting users define more than 8 iteration steps, requires unhardcoding this. The current structure was designed around exactly the 8-step plan from `ralph-tui.md`.

### A8. Banner art file location — **lives inside the Go package**

- `find . -name ralph-art*` → only `ralph-tui/internal/workflow/ralph-art.txt` exists in the working tree (the other matches are git refs).
- It is embedded at build time via `//go:embed ralph-art.txt` in `run.go:13-14` — must live next to `run.go` for Go's embed to find it.
- It is **not** in `prompts/` and **not** at the repo root.

**Implication for the user's ask:** "I'm still going to use that specific file" is easy to honor — the file stays where it is. Killing the immediate print means removing the `//go:embed` + loop at `run.go:58-60`, or keeping the embed and only rendering it somewhere else (e.g., a transient startup splash before the TUI takes over).

### A9. What actually runs today (authoritative trace)

1. `cli.Execute()` parses flags into a `Config` (`main.go:19`).
2. `logger.NewLogger(cfg.ProjectDir)` opens a timestamped file in `logs/` (`main.go:28`).
3. `steps.LoadSteps(cfg.ProjectDir)` JSON-parses `ralph-steps.json` — no validation (`main.go:34`).
4. `workflow.NewRunner(log, cfg.ProjectDir)` creates the runner with an `io.Pipe` (`main.go:41`).
5. A `StatusHeader` is created but never rendered (`main.go:46-53`).
6. A `KeyHandler` is created but never receives keyboard input (`main.go:43-44`).
7. A stdout-drain goroutine starts scanning the log pipe and calling `fmt.Println` per line (`main.go:56-63`).
8. SIGINT/SIGTERM handler set up (`main.go:75-87`).
9. `workflow.Run(runner, header, keyHandler, runCfg)` runs the workflow (`main.go:91`).
10. Inside `Run`: banner printed → `get_gh_user` captured → iteration loop (each iteration: get issue, get SHA, `SetIteration`, `Orchestrate(steps)`, which calls `runner.RunStep` per step) → finalization phase → `executor.Close()`.
11. `<-done` unblocks, log is closed, process exits.

That is the entire runtime. Everything else — keyboard shortcuts, the layout, error recovery, validation, pre-loop phase — is either dead code or absent.

---

## Root cause (discovered during grill-me)

Grepping the `ralph-tui/` module for `glyph` / `useglyph` returns **zero matches**. `ralph-tui/go.mod` lists only `cobra` and its transitive deps:

```
require github.com/spf13/cobra v1.10.2
```

In other words: **Glyph is not actually integrated.** The UI packages (`internal/ui/header.go`, `internal/ui/ui.go`, `internal/ui/orchestrate.go`) exist as pure data/state structures — `StatusHeader` holds `IterationLine`, `Row1`, `Row2` strings, and `KeyHandler` holds a mode field — but nothing ever *renders* them.

`cmd/ralph-tui/main.go` only does this for "display":

```go
go func() {
    scanner := bufio.NewScanner(runner.LogReader())
    buf := make([]byte, 256*1024)
    scanner.Buffer(buf, 256*1024)
    for scanner.Scan() {
        fmt.Println(scanner.Text())
    }
}()
```

It drains the log pipe to stdout via `fmt.Println`. There is no render loop, no HBox/VBox, no border, no shortcut bar. `keyHandler.Handle` is also never called anywhere outside of unit tests — stdin is never read, so `n`, `q`, `r`, `c` shortcuts do nothing at runtime. Only `SIGINT`/`SIGTERM` are wired (via `main.go` lines 75–87).

The banner print the user is seeing comes from `internal/workflow/run.go:58-60`:

```go
for _, line := range strings.Split(bannerArt, "\n") {
    executor.WriteToLog(line)
}
```

`bannerArt` is `//go:embed ralph-art.txt` in `internal/workflow/run.go:13-14`. It writes to the log pipe, which (because there's no TUI) is drained straight to stdout by the goroutine above.

### Summary of where we went wrong

- The plan (`docs/plans/ralph-tui.md`) described a Glyph-based TUI with pointer-mutable state reads.
- The state structures were built — but the Glyph integration step was never completed.
- The fallback "drain pipe to stdout" code in `main.go` was apparently left in place as a placeholder and shipped.
- As a result: no layout, no iteration header, no shortcut bar, no keyboard handling, no phases visible — just a banner followed by line-by-line subprocess output.

There is **no "pre-loop" phase at all** in the current code either. `Run()` in `internal/workflow/run.go` does these things in order before the loop:
1. Print the banner to the log (lines 58–60)
2. Call `get_gh_user` via `CaptureOutput` (lines 63–67)

Then straight into the iteration loop. There is no validation pass, no grouping as "pre-loop steps", nothing that would populate a "pre-loop" section of a header.

## Decisions (captured as the grill-me interview proceeds)

### D1. Rendering approach: **Glyph** (option A)

Finish the Glyph integration originally scoped in `docs/plans/ralph-tui.md`. Reasons:

- `StatusHeader` in `internal/ui/header.go` was explicitly shaped for Glyph's pointer-mutable `Text(&field)` model (`IterationLine`, `Row1`, `Row2` are exported mutable fields).
- `KeyHandler` was written assuming Glyph wires keypresses into `Handle(key string)`.
- The streaming log design relies on Glyph's `Log(io.Reader)`, which matches the existing `io.Pipe` plumbing in `workflow.Runner`.

Switching to Bubble Tea or staying on stdout would both require rewriting these components. Glyph is the lowest-friction path back to the intended Layout.

**Implication:** `cmd/ralph-tui/main.go` lines 56–63 (the stdout-draining goroutine) and the dependency set in `ralph-tui/go.mod` both need to change. The drain goroutine disappears; `Log(runner.LogReader())` inside a Glyph `VBox` replaces it. Glyph becomes a direct dependency.

### D2. Scope: **Broad (Scope B — build the complete app)**

This plan covers everything needed to land a usable app in one go. Partial implementations that require further effort to become usable are explicitly rejected. The scope is:

1. **Glyph integration (W1).** Adopt Glyph as the rendering framework. Replace the stdout-drain goroutine with a Glyph app that renders the header, log, and shortcut bar.
2. **Keyboard input wiring (W1 cont'd).** Wire Glyph's keypress dispatch into `KeyHandler.Handle(...)` so `n`/`q`/`y`/`r`/`c` actually work. Fixes A4 (error recovery dead channel) as a side effect.
3. **Unhardcode the 8-step cap (W2).** Replace `[4]string`/`[8]string` arrays in `internal/ui/header.go` and `var stepNames [8]string` in `cmd/ralph-tui/main.go` with dynamic sizing so pre-loop + loop + post-loop phases can each hold their own step count.
4. **Config validation (W3).** Add a startup validation pass that checks `ralph-steps.json` loads, every `promptFile` exists under `prompts/`, and every non-claude `command[0]` is resolvable (either exists as a file under `projectDir` if it contains a `/`, or is resolvable on `PATH` otherwise). Failures print to stderr and exit before Glyph starts.
5. **Phase-aware workflow (W4).** Split `workflow.Run()` into distinct pre-loop / loop / post-loop phases, each of which drives a corresponding header state. Pre-loop is a new concept — it did not previously exist.
6. **Kill the immediate banner print.** Remove `run.go:58-60`. Keep `ralph-tui/internal/workflow/ralph-art.txt` in place; its future use is decided in a later decision below.

**Items intentionally in scope, not deferred:**

- Error recovery machinery (`c`/`r` keys in error mode) becomes reachable once keyboard input is wired.
- Completion behavior (clean exit after post-loop) gets an explicit design pass rather than "it just returns from Run()".

**Nothing is deferred.** If a decision below reveals more entangled work, that work joins the scope rather than being pushed to a follow-up plan.

### D3a. Validation happens **before** Glyph starts (not as a header step)

Validation runs in plain Go at startup. On failure: print to stderr and `os.Exit(1)` — the Glyph app is never constructed. On success: proceed to Glyph startup, which is the first moment the layout becomes visible.

Reasons:

- The user's literal wording — *"once they pass validation, the Layout UI should immediately show up"* — maps to a gate, not an in-layout step.
- Validation failures are one-line filesystem / JSON errors that are easier to read, copy, and act on as plain stderr than as a log line buried inside a TUI panel the user has to quit first.
- Validation is ~5–10ms (filesystem stat + JSON parse). Rendering `[▸] → [✓]` inside a Glyph render cycle would flicker past in a single frame — it's not feedback, it's noise.
- Showing a TUI when the config is already known to be invalid is pointless.

**Accepted consequence:** ralph-tui gains exactly one class of error that appears outside the TUI (pre-Glyph validation failures). Every other error — step failures, SIGINT, panics — still goes through the Glyph log panel. This inconsistency is acceptable because validation is categorically a precondition check, not a runtime failure.

### D3b. Pre-loop phase is defined in `ralph-steps.json` as an `"initialize"` array

The new third phase is named `"initialize"` (not `"preLoop"`), matching the user's edit to `bin/ralph-steps.json`. The array is empty in the current file and will be populated with step definitions for `get_gh_user`, etc., as part of this plan.

**Schema shape:**

```json
{
  "initialize": [ ...steps run once at startup, after validation, before the loop... ],
  "iteration":  [ ...steps run per iteration... ],
  "finalize":   [ ...steps run once after the loop... ]
}
```

**File-copy note:** the user edited `bin/ralph-steps.json` (build output). The source copy at `ralph-tui/ralph-steps.json` still has only `iteration` and `finalize`. The source copy must be synced as part of implementation so the Makefile build output reproduces the correct structure.

### D3c. Architectural principle — "narrow" reading: ralph-tui facilitates the workflow, does not define it

**The principle in one sentence:** ralph-tui owns workflow *structure* (phase names, loop semantics, CLI flags, basic display chrome). Config owns workflow *content* (every step, every captured variable, every command).

**What ralph-tui owns (hardcoded):**

- The three phase names `initialize` / `iteration` / `finalize`.
- The hardcoded semantics per phase name: `initialize` runs once before the loop; `iteration` runs N times in a loop; `finalize` runs once after the loop.
- The `-n` / `--iterations` CLI flag capping the loop count.
- The generic rule "after a step flagged with `breakLoopIfEmpty: true`, if its captured variable is empty, exit the iteration loop". This is the only workflow-termination rule ralph-tui understands.
- Generic `{{VAR}}` template substitution inside command argv and inside prompt file contents.
- Glyph app lifecycle: construct → render → dispatch keypresses → tear down on exit.
- The status header chrome (iteration counter, step checkboxes, shortcut bar). The *text* of the iteration/finalize counter line may still be hardcoded — the narrow reading does not require templating the header chrome itself, just the steps inside it.
- Validation of `ralph-steps.json` against the schema above.
- Config file location (`<projectDir>/ralph-steps.json`).

**What config owns (must move out of Go code):**

- Every step that runs at any phase, including what are currently hardcoded in `run.go`:
  - `get_gh_user` (currently `run.go:63-67`) → becomes an `initialize` step.
  - `get_next_issue` (currently `run.go:72-73`) → becomes the first `iteration` step, flagged `breakLoopIfEmpty: true`.
  - `git rev-parse HEAD` (currently `run.go:80-83`) → becomes the second `iteration` step.
- Every variable captured from a step's stdout and referenced by later steps.
- The list of variables prepended to any claude prompt file. The current `"prependVars": true` mechanism (which hardcodes `ISSUENUMBER=` and `STARTINGSHA=` in `steps.BuildPrompt`) is replaced by `{{VAR}}` substitution inside the prompt file contents themselves. Ralph-tui no longer knows which vars go where — the prompt author decides.

**What ralph-tui keeps that's close to the line but not over it:**

- The hardcoded iteration header line format `Iteration N/M — Issue #X` and the completion summary text `Ralph completed after N iteration(s)...`. Technically these embed assumptions about the Ralph workflow, but they're cosmetic chrome and making them config-driven adds complexity without meaningful benefit. If the user later wants to use ralph-tui for a different workflow, this can be revisited.

**Design consequence: ralph-tui becomes a generic config-driven step runner that happens to understand phases, loops, captured-variable substitution, and one loop-exit rule. The Ralph workflow is entirely expressible in `ralph-steps.json`.**

### D4. Variable capture: Shape A (`captureAs` field), value is the final non-empty stdout line

Each step may declare an optional `"captureAs": "VAR_NAME"` field. When the step completes with zero exit status, ralph-tui parses its captured stdout and binds the resulting value to `VAR_NAME` in the variable table. Later steps reference the variable via `{{VAR_NAME}}` in command argv and in prompt file contents.

**Capture rule — final non-empty line, not the full stdout.**

Scripts frequently emit progress/debug/API-call chatter on stdout before echoing the final value. Example: `scripts/get_gh_user` runs `gh api user`, which writes JSON or progress lines to stdout, then the script ends with `echo $username`. The intent is for `$username` (the last line) to be the captured value — not the entire multi-line blob.

Algorithm:

1. Collect the subprocess's full stdout as seen by the log panel (so every line, including chatter, is still displayed in the Glyph log).
2. Split the collected stdout on `\n`.
3. Walk the lines in reverse; strip trailing `\r` (for CRLF tolerance).
4. Return the first line that is non-empty after trimming surrounding whitespace.
5. If every line is empty or whitespace-only, the captured value is the empty string `""`.

**Pseudocode:**

```go
// in workflow.Runner, per-step state:
//   var capturedBuf strings.Builder (written in parallel with log-pipe forwarding)
// after cmd.Wait() returns nil:
lines := strings.Split(capturedBuf.String(), "\n")
var value string
for i := len(lines) - 1; i >= 0; i-- {
    line := strings.TrimSpace(strings.TrimRight(lines[i], "\r"))
    if line != "" {
        value = line
        break
    }
}
vars[step.CaptureAs] = value
```

**Design consequences of "last non-empty line":**

- **Capture is independent of display.** Every line of stdout still flows to the Glyph log panel the normal way. Capture happens alongside, not instead of.
- **Empty captured value is meaningful.** A step that successfully exits with no non-empty lines on stdout produces an empty string. This is what the future `breakLoopIfEmpty` flag (D-pending) keys off.
- **No capture on failure.** If the step exits non-zero, nothing is captured — the step goes into error mode first, and only on a successful run (either original or via retry) does the variable get bound. Rationale: binding a variable from a failed step creates confusing downstream state.
- **Claude steps can also use `captureAs`** in principle, but it's rarely useful — Claude's stdout is huge and the "last non-empty line" heuristic doesn't map to any meaningful value. The schema allows it; the validation doesn't forbid it; we just don't expect real configs to use it.

**Trim behavior:** `strings.TrimSpace` on the selected line (trims leading and trailing whitespace). The current `workflow.Runner.CaptureOutput` at `workflow.go:178` uses `TrimSpace` on the whole output — the new rule changes *which lines* are considered, not how they're trimmed.

**Naming convention (soft recommendation, not enforced):** variable names are `UPPER_SNAKE_CASE`. Validation doesn't enforce this — `{{VAR_NAME}}` substitution is purely string-based — but the convention keeps configs readable and matches the existing `{{ISSUE_ID}}` pattern.

### D5. Variable scope: Model Y (phase-scoped with initialize promotion), plus ralph-tui-provided built-ins

Ralph-tui maintains two variable tables during a run:

- **Persistent table** — written by all `initialize` steps' `captureAs` bindings; visible to all subsequent phases (iteration, finalize); never cleared. Also seeded at startup and updated as needed by ralph-tui itself with **built-in variables** (see below).
- **Iteration table** — written by `iteration` steps' `captureAs` bindings; visible only within the current iteration; **cleared at the start of each iteration**.

**Resolution order** when a `{{VAR_NAME}}` reference is expanded:

1. If currently running an iteration step, check the iteration table first.
2. Fall through to the persistent table.
3. If still unresolved, it is a **validation error at startup** — not a runtime error. The validation pass walks every `{{VAR}}` reference in command argv and prompt files and verifies the name is in the set of "known names in the relevant scope at that point in the config".

**`finalize` scope:** finalize steps see only the persistent table. They cannot reference iteration variables — those are stale (last iteration's value) or empty (if the loop exited via `breakLoopIfEmpty`) by the time finalize runs. Making them invisible prevents accidental stale-value reads.

**Intra-phase ordering:** inside the `iteration` array, step N can reference variables captured by any step M where M < N. Validation walks the iteration array in declaration order and grows the symbol set as it goes. The same rule applies to `initialize` and `finalize`.

**Built-in variables (always in the persistent table, provided by ralph-tui):**

These are pre-populated at startup and (where relevant) updated by ralph-tui each iteration. They are the mechanism by which CLI flags and runtime state become visible to prompts and scripts — the user explicitly asked that all CLI/config values be referenceable everywhere.

| Variable         | Source                                      | Updated when                 | Notes                                                          |
|------------------|---------------------------------------------|------------------------------|----------------------------------------------------------------|
| `{{PROJECT_DIR}}`| `--project-dir` flag or resolved exe dir    | Once at startup              | Absolute path, symlinks resolved.                              |
| `{{MAX_ITER}}`   | `--iterations` flag value                   | Once at startup              | Literal integer; `0` means unbounded. Scripts can test for `0`.|
| `{{ITER}}`       | Runtime iteration counter                   | Rebound at the start of each iteration | 1-based. Undefined (validation error) if referenced inside `initialize` or `finalize`. |
| `{{STEP_NUM}}`   | Current step's 1-based index within its phase | Rebound just before each step runs | Phase-scoped: in initialize, 1..len(initialize); in iteration, 1..len(iteration), reset each iteration; in finalize, 1..len(finalize). |
| `{{STEP_COUNT}}` | Total number of steps in the current phase  | Rebound at phase start, constant during the phase | Phase-scoped: `len(initialize)` during initialize, `len(iteration)` during iteration, `len(finalize)` during finalize. |
| `{{STEP_NAME}}`  | Name of the currently-running step          | Rebound just before each step runs | Phase-scoped: the `name` field of the step about to run. A step may reference its own `{{STEP_NAME}}` — the value is already bound by the time the command is resolved and exec'd. |

**Scope rule for `{{ITER}}`:** although it lives in the persistent table (so the resolution logic stays uniform), references to `{{ITER}}` are validation errors inside `initialize` steps and `finalize` steps, because the counter has no meaningful value outside the iteration loop. Validation enforces this by treating `{{ITER}}` as a reference that's only legal in the `iteration` phase.

**Scope rule for `{{STEP_NUM}}`, `{{STEP_COUNT}}`, `{{STEP_NAME}}`:** these three are always in scope in every step in every phase. Their values are phase-scoped — they always refer to the current phase's list. A step in initialize sees `STEP_COUNT = len(initialize)`; a step in iteration sees `STEP_COUNT = len(iteration)`; a step in finalize sees `STEP_COUNT = len(finalize)`. The `STEP_NUM` counter resets at phase boundaries and, within iteration, resets at every iteration start. The `STEP_NAME` is the name of the currently-running step at the moment the command is resolved — safe to self-reference.

**Name collisions:** if a user config's `captureAs` shadows a built-in name (e.g., `"captureAs": "ITER"`, `"captureAs": "STEP_NAME"`), **validation fails** at startup. Built-in names are reserved. The full reserved set is: `PROJECT_DIR`, `MAX_ITER`, `ITER`, `STEP_NUM`, `STEP_COUNT`, `STEP_NAME`.

**Extensibility:** the set of built-ins is small and explicit. Future built-ins (e.g., `{{PHASE}}` for the current phase name, `{{STEP_NAME}}` for the currently-running step) can be added later without breaking configs — they're additive. Each future addition requires (a) a new entry in this table, (b) a line in the validation pass's reserved-names set, and (c) an update site in the runtime.

### D6. Loop termination: `breakLoopIfEmpty` flag + new `StepSkipped` state

**Schema:** `iteration` steps may declare `"breakLoopIfEmpty": true`. The flag is only legal on steps that also declare `captureAs` (validation enforces this at startup — `breakLoopIfEmpty: true` without `captureAs` is a config error).

**Runtime rule:** after a step with `breakLoopIfEmpty: true` completes successfully (exit code zero), ralph-tui checks the value bound to the step's `captureAs` variable. If the value is the empty string:

1. The step itself is marked `StepDone` — it ran and produced a legitimate "nothing to do" signal.
2. **All remaining steps in the current iteration are marked `StepSkipped`** (a new state added to the `StepState` enum in `internal/ui/header.go:7-13`, with display marker `[-]`).
3. The iteration loop exits. No further iterations begin, regardless of `--iterations` value.
4. The `finalize` phase still runs — per the existing plan, finalization is not skipped on early loop exit.

**New `StepState` value:**

```go
const (
    StepPending StepState = iota
    StepActive
    StepDone
    StepFailed
    StepSkipped // new — displayed as "[-] <name>"
)
```

`checkboxLabel` in `header.go:107-118` gains a `StepSkipped` case that returns `fmt.Sprintf("[-] %s", name)`.

**Validation:** the validator rejects configs where `breakLoopIfEmpty: true` appears on a step without `captureAs`, or on a step outside the `iteration` array (flag is meaningless in `initialize` and `finalize` — there's no loop to break).

**Interaction with bounded mode:** `breakLoopIfEmpty` is absolute. If `--iterations 5` is set and the flag fires on iteration 2, iteration 2's remaining steps are skipped, the loop exits, and finalize runs. The remaining 3 iterations are not attempted. Rationale: the flag's semantic is "there is nothing left to do", and "nothing left to do" doesn't care about the bound.

**Interaction with error recovery:** if the triggering step itself fails (non-zero exit), the `breakLoopIfEmpty` check never runs — the normal error-mode path takes over. The flag is only consulted on successful completion.

### D7. Prompt-file variable injection: Shape P with Migration B (rewrite prompts to use `{{VAR}}` inline)

The existing `"prependVars": true` mechanism (hardcoded prepending of `ISSUENUMBER=` and `STARTINGSHA=` in `internal/steps/steps.go:59-61`) is **removed entirely**. It is replaced by a single unified substitution engine that processes `{{VAR}}` references in two targets:

1. **Command argv elements** (for non-claude steps) — extending the existing single-variable `{{ISSUE_ID}}` substitution in `workflow.ResolveCommand` (`workflow.go:189-206`) to handle arbitrary variable names.
2. **Prompt file contents** (for claude steps) — `steps.BuildPrompt` reads the file and runs the same engine over the text.

**Engine rules:**

- Token syntax: `{{VAR_NAME}}`. Double braces, no spaces inside, variable name is `[A-Z_][A-Z0-9_]*`-ish but not strictly enforced — any character sequence that isn't `}}` is accepted between the braces.
- The substitution is whole-token replacement. Each `{{VAR}}` match is replaced by the variable's string value; no partial-match, no recursion.
- Lookup uses the resolution order from D5: iteration table first (if in an iteration step), persistent table second, built-ins last (built-ins live in the persistent table).
- Unresolved references are a **validation error at startup** — the validator walks every prompt file and every command argv, extracts all `{{VAR}}` tokens, and checks each against the known-variable set in its scope.
- Runtime passthrough behavior if an unresolved reference somehow escapes validation: log a warning to the Glyph log panel and substitute the empty string. (Defense-in-depth — validation is the authoritative check.)
- Escape: `{{{{` → literal `{{`, `}}}}` → literal `}}`. Handles the rare case of wanting literal double-braces in a prompt.

**Migration B — prompt files are rewritten to use `{{VAR}}` inline.**

The existing five `prompts/*.md` files that reference `ISSUENUMBER` and `STARTINGSHA` as bare words in prose are rewritten to substitute the values directly. No header line, no `prependVars`, no bridge. The prose references the actual value.

Examples of the migration (concrete diffs, not aspirational):

**`prompts/feature-work.md`** — `ISSUENUMBER` appears once:
```
Before:  1. Implement github issue ISSUENUMBER in the current branch (do not switch to a different branch)
After:   1. Implement github issue #{{ISSUE_ID}} in the current branch (do not switch to a different branch)
```

**`prompts/code-review-changes.md`** — `STARTINGSHA` and `ISSUENUMBER` each appear once:
```
Before:  1. run a /code-review for the changes made since commit sha STARTINGSHA, and write the full review content to code-review.md
         4. Update the github issue ISSUENUMBER with what was done.
After:   1. run a /code-review for the changes made since commit sha {{STARTING_SHA}}, and write the full review content to code-review.md
         4. Update the github issue #{{ISSUE_ID}} with what was done.
```

**`prompts/test-planning.md`** — same pattern:
```
Before:  1. Run /test-planning against commits starting with STARTINGSHA, without the edge case testing agent, and write the test plan to test-plan.md
         4. Update the github issue ISSUENUMBER with what was done.
After:   1. Run /test-planning against commits starting with {{STARTING_SHA}}, without the edge case testing agent, and write the test plan to test-plan.md
         4. Update the github issue #{{ISSUE_ID}} with what was done.
```

**Files that don't reference either variable** (`code-review-fixes.md`, `test-writing.md`, `deferred-work.md`, `lessons-learned.md`, `update-docs.md`) — no changes needed.

**Variable name mapping for the migration:**

| Old bare word    | New variable name | Source                                                       |
|------------------|-------------------|--------------------------------------------------------------|
| `ISSUENUMBER`    | `{{ISSUE_ID}}`    | Captured from the `get_next_issue` iteration step (`captureAs: "ISSUE_ID"`, `breakLoopIfEmpty: true`) |
| `STARTINGSHA`    | `{{STARTING_SHA}}`| Captured from the `git rev-parse HEAD` iteration step (`captureAs: "STARTING_SHA"`) |

**What ralph-tui loses by this decision:**

- `steps.Step.PrependVars` field — removed from the struct.
- The `if step.PrependVars { content = ... }` block at `internal/steps/steps.go:59-61` — removed.
- Any test coverage for `prependVars` behavior — removed or rewritten.
- The implicit assumption that prompts expect a two-line header — gone. Prompts are now plain content with `{{VAR}}` tokens.

**Backward compatibility explicitly rejected.** The user does not want existing prompts preserved unchanged. Migration B edits prose directly; prior behavior of silent prepending is not retained via any shim.

### D8. Iteration header line: Behavior II — reactive, updates when referenced variables change

The iteration header line is **reactive**. It re-renders whenever a referenced variable's binding changes, so the user sees the issue number appear in the header as soon as the `get_next_issue` step captures it, without having to scan the log panel.

**User-facing behavior (the UX contract):**

- At the start of iteration N, the header line shows `Iteration N/M` (or `Iteration N` in unbounded mode) with **no issue suffix**, because no iteration step has run yet in this iteration and `ISSUE_ID` is unbound in the iteration-scoped table (per D5, iteration variables reset at iteration start).
- After the `get_next_issue` step captures `ISSUE_ID`, the header line updates to `Iteration N/M — Issue #<id>`.
- The line is updated event-driven, not polled: each time a `captureAs` binding completes, ralph-tui re-renders the iteration header line.
- On entering finalize, the header switches to the finalize template (`Finalizing K/T`) — a separate code path, not a conditional inside the iteration template.

**Implementation mechanism (honest about the narrow-principle implications):**

Since per D3c the header format string is *not* config-driven (it's ralph-tui-owned cosmetic chrome), the practical implementation is **direct Go code that reads specific variable names from the variable tables** — not a fully generic templating engine. This is the simplest thing that produces the reactive UX the user asked for:

```go
// internal/ui/header.go — new method on StatusHeader
//
// Called by the workflow runtime:
//   - once at iteration start (when ITER is rebound)
//   - once after each captureAs binding completes in that iteration
//
func (h *StatusHeader) RenderIterationLine(iter, maxIter int, issueID string) {
    var b strings.Builder
    if maxIter > 0 {
        fmt.Fprintf(&b, "Iteration %d/%d", iter, maxIter)
    } else {
        fmt.Fprintf(&b, "Iteration %d", iter)
    }
    if issueID != "" {
        fmt.Fprintf(&b, " — Issue #%s", issueID)
    }
    h.IterationLine = b.String()
}
```

And the workflow runtime looks up `ISSUE_ID` from the iteration-scoped table before calling `RenderIterationLine`. The call site:

```go
// after each captureAs binding in the iteration phase:
issueID, _ := vars.GetIteration("ISSUE_ID") // empty string if unbound
header.RenderIterationLine(currentIter, cfg.Iterations, issueID)
```

**Why not a "generic template engine" approach:**

Behavior II and Behavior III produce *identical* user-visible output. The only difference would be whether the header format string lives in a template (`"Iteration {{ITER}}/{{MAX_ITER}} — Issue #{{ISSUE_ID}}"`) processed by the same substitution engine used for command argv and prompt files, or whether it's built via direct Go code. Under the narrow principle, the template string cannot live in config; it would have to be hardcoded in Go either way. Given that, direct Go code is simpler, more explicit, handles conditionals (unbounded mode, missing issue ID) natively without needing a conditional-template mini-language, and costs nothing in flexibility we're not using.

**This specific piece of hardcoded knowledge — that `ISSUE_ID` is the variable name the header displays — is the one concession ralph-tui makes to the Ralph workflow beyond phase names.** It's narrow, documented, and easy to revisit later if the user ever wants a different workflow. The alternative (fully template-based header, config-driven format string) is broad-reading territory and was explicitly rejected in D3c.

**Reactive-update trigger points:**

| Event                                  | Header line recomputed?                               |
|----------------------------------------|-------------------------------------------------------|
| Iteration N begins                     | Yes — `ITER` just changed; suffix is empty.           |
| Any iteration step's `captureAs` fires | Yes — `ISSUE_ID` may have just been bound.            |
| A step without `captureAs` completes   | No — nothing referenced has changed.                  |
| An iteration step fails                | No — header state unchanged; error mode takes over.   |
| A step enters `StepActive` state       | No — active state affects the checkbox row, not the line. |
| Finalize phase begins                  | No — `RenderIterationLine` is no longer called; the header switches to `SetFinalization`. |

**Finalize phase header line is unchanged from the existing implementation.** `SetFinalization(current, total, steps)` at `header.go:65-82` continues to produce `"Finalizing 1/3"`-style text. No reactive binding is needed in finalize because finalize doesn't display captured-variable info in the header line.

### D9. Per-iteration prologue becomes the first two entries in `iteration`

The two hardcoded prologue calls at `internal/workflow/run.go:72-83` are deleted from Go and moved into `ralph-steps.json` as the first two entries of the `iteration` array. After this change, the `iteration` array grows from 8 entries to 10:

```json
"iteration": [
  {
    "name": "Get next issue",
    "isClaude": false,
    "command": ["scripts/get_next_issue", "{{GITHUB_USER}}"],
    "captureAs": "ISSUE_ID",
    "breakLoopIfEmpty": true
  },
  {
    "name": "Get starting SHA",
    "isClaude": false,
    "command": ["git", "rev-parse", "HEAD"],
    "captureAs": "STARTING_SHA"
  },
  {"name": "Feature work",  "model": "sonnet", "promptFile": "feature-work.md",       "isClaude": true},
  {"name": "Test planning", "model": "opus",   "promptFile": "test-planning.md",      "isClaude": true},
  {"name": "Test writing",  "model": "sonnet", "promptFile": "test-writing.md",       "isClaude": true},
  {"name": "Code review",   "model": "opus",   "promptFile": "code-review-changes.md","isClaude": true},
  {"name": "Review fixes",  "model": "sonnet", "promptFile": "code-review-fixes.md",  "isClaude": true},
  {"name": "Close issue",   "isClaude": false, "command": ["scripts/close_gh_issue", "{{ISSUE_ID}}"]},
  {"name": "Update docs",   "model": "sonnet", "promptFile": "update-docs.md",        "isClaude": true},
  {"name": "Git push",      "isClaude": false, "command": ["git", "push"]}
]
```

Note that `"prependVars": true` has been **removed from every existing claude step** in the iteration array — this field no longer exists per D7. Prompts now reference `{{ISSUE_ID}}` and `{{STARTING_SHA}}` inline via Migration B.

**Semantics (locked):**

- **9a. The `iteration` array contains 10 entries.** This forces the header checkbox row layout to unhardcode the existing 4+4 = 8-slot assumption (see forthcoming decision on header layout).
- **9b. `get_next_issue` exit semantics: least surprise.** Empty stdout on a zero exit = `breakLoopIfEmpty` fires (no more work). Non-zero exit = real failure, triggers error mode normally.
- **9c. `git rev-parse HEAD` failure is a real error, not a warning.** The old tolerance at `run.go:81-83` (warn and continue with empty SHA) is dropped. If `git rev-parse HEAD` fails, the step goes into error mode and the user decides what to do. Rationale: the only ways this can fail are "not in a git repo" or "git not installed" — both are setup errors that should be surfaced, not suppressed.
- **9d. No new phase.** The prologue steps are just the first two iteration steps. No fourth phase (`iterationPrologue`, `perIteration`) is added. Schema stays at three phases.
- **9e. Step names are the same as the JSON `name` field** — `Get next issue`, `Get starting SHA`. No header-specific abbreviations.

**Dead Go code removed as part of this decision:**

- `run.go:62-67` — the hardcoded `get_gh_user` capture. The username now comes from the `GITHUB_USER` persistent variable.
- `run.go:72-78` — the hardcoded `get_next_issue` call and the empty-issue early-exit check. The first iteration step and `breakLoopIfEmpty` replace them.
- `run.go:80-83` — the hardcoded `git rev-parse HEAD` call and its error-tolerant warning. The second iteration step replaces it.
- The `username`, `issueID`, and `sha` local variables that threaded these values through `Run()`. Replaced by the variable table.
- `buildIterationSteps(projectDir, stepsConfig, issueID, sha)` at `run.go:139-159` — its `issueID` and `sha` parameters go away; substitution reads from the variable table instead.
- `iterationLabel(i, total)` at `run.go:131-137` — likely goes away, replaced by the reactive `RenderIterationLine` from D8.

### D10. Header checkbox layout: dynamic rows of 4, row count sized at startup to the largest phase

`StatusHeader` holds a dynamically-sized grid of checkbox slots. The grid height is computed *once* at startup, after validation, from the largest phase's step count. Each row holds at most 4 slots (horizontally, to fit 80-column terminals). The grid width is constant (4), but the grid height varies with the config.

**No maximum step count is enforced.** Per D13, only the minimum (iteration ≥ 1) is validated. If the user adds 20 iteration steps, the grid becomes 5 rows tall. If they add 40, it becomes 10 rows tall. The Glyph tree is sized to `⌈max(len(initialize), len(iteration), len(finalize)) / 4⌉` rows at construction time.

**New struct shape:**

```go
// internal/ui/header.go

const HeaderCols = 4 // checkboxes per row; constant to fit 80-column terminals

type StatusHeader struct {
    IterationLine string
    Rows          [][HeaderCols]string // row count computed at startup; each row has HeaderCols slots
    stepNames     []string              // the current phase's step name list
}

// NewStatusHeader constructs a header sized to fit the largest phase.
// Call this once at startup, after validation, with the max across all three phases.
func NewStatusHeader(maxStepsAcrossPhases int) *StatusHeader {
    rowCount := (maxStepsAcrossPhases + HeaderCols - 1) / HeaderCols // ceil division
    if rowCount < 1 {
        rowCount = 1
    }
    return &StatusHeader{
        Rows: make([][HeaderCols]string, rowCount),
    }
}
```

**Deletions from the current struct:**

- `Row1 [4]string` → replaced by `Rows[0]`.
- `Row2 [4]string` → replaced by `Rows[1]`.
- `stepNames [8]string` → replaced by dynamic `stepNames []string`.
- `finalizeNames []string` → merged into the unified `stepNames` field.
- The entire hardcoded 8-step cap in `cmd/ralph-tui/main.go:46` (`var stepNames [8]string`) goes away — main.go computes `maxStepsAcrossPhases := max(len(initialize), len(iteration), len(finalize))` after validation succeeds and passes it to `NewStatusHeader`.

**New unified phase-transition API:**

```go
// SetPhaseSteps replaces the current step name list and re-renders all
// checkbox slots. Call at the start of each phase (initialize, iteration,
// finalize) to swap the header to the new phase's step set.
//
// The caller guarantees len(names) <= cap(h.Rows) * HeaderCols — this is
// enforced at startup because NewStatusHeader was sized to the largest phase
// in the validated config. If len(names) overflows, panic (a bug indicator).
func (h *StatusHeader) SetPhaseSteps(names []string) {
    totalSlots := len(h.Rows) * HeaderCols
    if len(names) > totalSlots {
        panic(fmt.Sprintf("ui: phase has %d steps, exceeds allocated grid capacity %d", len(names), totalSlots))
    }
    h.stepNames = append(h.stepNames[:0], names...)
    for r := 0; r < len(h.Rows); r++ {
        for c := 0; c < HeaderCols; c++ {
            idx := r*HeaderCols + c
            if idx < len(names) {
                h.Rows[r][c] = checkboxLabel(StepPending, names[idx])
            } else {
                h.Rows[r][c] = "" // trailing empty slots render as blank padding
            }
        }
    }
}

// SetStepState updates the checkbox label for step idx in the current phase.
// Replaces the per-phase SetStepState/SetFinalizeStepState split.
func (h *StatusHeader) SetStepState(idx int, state StepState) {
    if idx < 0 || idx >= len(h.stepNames) {
        return
    }
    r, c := idx/HeaderCols, idx%HeaderCols
    h.Rows[r][c] = checkboxLabel(state, h.stepNames[idx])
}
```

**Deleted methods:** `SetFinalization` and `SetFinalizeStepState` merge into `SetPhaseSteps` + `SetStepState`. The iteration/finalize split in `run.go`'s `iterHeader` / `finalHeader` adapter structs (`run.go:42-51`) also goes away — one header, one step-state method, one phase-transition method.

**Why dynamic row count (no hard cap):**

- Per D13, there is no maximum step count for any phase. Only the minimum (iteration ≥ 1) is validated.
- Row count is computed at startup: `rowCount = ⌈max(len(initialize), len(iteration), len(finalize)) / 4⌉`.
- For the current config (initialize=2, iteration=10, finalize=3), the max is 10, so `rowCount = ⌈10/4⌉ = 3` rows.
- A future config with 20 iteration steps would get `rowCount = ⌈20/4⌉ = 5` rows. No code change needed.
- The Glyph tree is still constructed once at app startup (after validation) with the correct row count, so the tree shape is stable for the lifetime of the app.
- Phases with fewer steps than the global max leave trailing rows blank (slots are `""` which render as padding).

**Why 4 columns, not 5:**

The longest step labels (`Get starting SHA`, `Code review`, `Review fixes`) with the `[▸] ` prefix run ~20 characters. 4 columns × ~20 chars = 80 chars + separator padding = fits 80-column terminals. 5 columns would push past 80 and truncate on narrow terminals.

**Validation enforcement:**

Per D13, the validator enforces **only the minimum** — iteration must have ≥ 1 step. There is no maximum. The Glyph tree is sized to whatever the largest phase contains. The panic in `SetPhaseSteps` is defense-in-depth for bugs (ralph-tui wrote a step count larger than it sized the grid for), not a user-reachable path.

**Glyph tree shape (constructed once at startup, after validation):**

```go
// Pseudocode — actual Glyph API may differ slightly.
headerChildren := []glyph.Widget{Text(&header.IterationLine)}
for r := 0; r < len(header.Rows); r++ {
    row := make([]glyph.Widget, HeaderCols)
    for c := 0; c < HeaderCols; c++ {
        row[c] = Text(&header.Rows[r][c])
    }
    headerChildren = append(headerChildren, HBox(row...))
}
VBox(headerChildren...).Border(...).Title("Ralph")
```

`rowCount × 4` total `Text` widgets, all pointer-bound to the header's `Rows` grid. The number of rows is computed from the largest phase at startup; after construction, the tree shape is fixed for the lifetime of the app. Phases with fewer steps leave some slots pointing at empty strings, which render as blank padding. No tree-shape changes at runtime — only the string contents change.

### D11. `ralph-art.txt` — move to repo root, display via config-driven initialize step

**Location:** the file moves from `ralph-tui/internal/workflow/ralph-art.txt` to `{repo-root}/ralph-art.txt`. It sits alongside `prompts/`, `scripts/`, `ralph-steps.json`, and the other repo-root assets.

**Embed removal:** `//go:embed ralph-art.txt` at `internal/workflow/run.go:13-14` is deleted. The `bannerArt` variable and the `for _, line := range strings.Split(bannerArt, "\n") { executor.WriteToLog(line) }` block at `run.go:57-60` are deleted. Ralph-tui's source code loses all knowledge of the file's existence.

**Display:** the art is shown via a config-defined `initialize` step — the **first** entry in the `initialize` array — that `cat`s the file to stdout. The output flows through the normal subprocess stdout pipe into the Glyph log panel, identical to any other step's output.

```json
"initialize": [
  {
    "name": "Splash",
    "isClaude": false,
    "command": ["cat", "{{PROJECT_DIR}}/ralph-art.txt"]
  },
  {
    "name": "Get GitHub user",
    "isClaude": false,
    "command": ["scripts/get_gh_user"],
    "captureAs": "GITHUB_USER"
  }
]
```

`{{PROJECT_DIR}}` resolves (per D5) to the directory containing the ralph-tui executable — i.e., `bin/` after `make build`. So `{{PROJECT_DIR}}/ralph-art.txt` becomes `bin/ralph-art.txt` at runtime.

**`make build` update:** the `build` target in the root `Makefile` must copy `ralph-art.txt` from the repo root to `bin/` alongside the other build output. Current `build` target:

```makefile
build:
	rm -rf bin
	mkdir -p bin
	cd ralph-tui && go build -o ../bin/ralph-tui ./cmd/ralph-tui
	cp -r prompts bin/prompts
	cp -r scripts bin/scripts
	cp ralph-tui/ralph-steps.json bin/
```

Updated `build` target adds one line:

```makefile
build:
	rm -rf bin
	mkdir -p bin
	cd ralph-tui && go build -o ../bin/ralph-tui ./cmd/ralph-tui
	cp -r prompts bin/prompts
	cp -r scripts bin/scripts
	cp ralph-tui/ralph-steps.json bin/
	cp ralph-art.txt bin/
```

**Documentation updates required (scope of this plan):**

Two feature docs describe the art as embedded and must be updated to match the new reality:

- `docs/features/cli-configuration.md:158` — currently says `"ralph-art.txt — startup banner (embedded in the binary via //go:embed)"`. Update to describe it as a repo-root asset copied into `bin/` by `make build` and displayed via the first initialize step.
- `docs/features/workflow-orchestration.md:120` — currently describes phase 1 of orchestration as "displays the embedded ralph-art.txt banner". Update to remove this claim; the new orchestration has no built-in banner step. The splash is just a config-defined initialize step like any other.

Historical references in `docs/plans/ralph-tui.md` are left untouched — that file is a record of the original design, not current architecture. The current design lives in this file and in the feature docs.

### D12. Header line formats per phase (Candidate 2 — mixed)

Each phase has its own hardcoded header line format string in ralph-tui Go code. The three new built-ins from D5 (`STEP_NUM`, `STEP_COUNT`, `STEP_NAME`) are always available for reference in commands/prompts/scripts but only two of the three phase headers surface them in the header line itself.

| Phase       | Header line format (conceptual)                                   | Concrete example                     |
|-------------|-------------------------------------------------------------------|--------------------------------------|
| Initialize  | `Initializing {{STEP_NUM}}/{{STEP_COUNT}}: {{STEP_NAME}}`          | `Initializing 1/2: Splash`           |
| Iteration   | `Iteration {{ITER}}/{{MAX_ITER}} — Issue #{{ISSUE_ID}}` (from D8; `/{{MAX_ITER}}` omitted if unbounded, ` — Issue #...` omitted if `ISSUE_ID` unbound) | `Iteration 2/3 — Issue #42`          |
| Finalize    | `Finalizing {{STEP_NUM}}/{{STEP_COUNT}}: {{STEP_NAME}}`            | `Finalizing 2/3: Lessons learned`    |

**Why iteration is different:** the iteration phase already surfaces current-step info via the checkbox row (`[▸] Feature work` vs `[✓] Feature work`). The header line's job in iteration is to show the iteration counter and the issue being worked on — that's unique info that isn't visible elsewhere. Adding step info to the iteration header line would duplicate what the checkbox row already conveys and make the line cramped.

**Why initialize and finalize show step info:** their header lines were previously bare counters (or nonexistent in initialize's case). The checkbox row is still present for them, but the step-name-in-header redundancy is acceptable for initialize and finalize because their phases are shorter (2 and 3 steps currently) and the extra explicitness helps reassure the user during startup/shutdown that progress is happening.

**Implementation mechanism:** the header line format strings are hardcoded Go format strings, substituted via the existing template engine from D7 (the same engine used for command argv and prompt files). Reusing the engine keeps one substitution code path for all three uses:

```go
// internal/ui/header.go

const (
    initializeHeaderFormat = "Initializing {{STEP_NUM}}/{{STEP_COUNT}}: {{STEP_NAME}}"
    finalizeHeaderFormat   = "Finalizing {{STEP_NUM}}/{{STEP_COUNT}}: {{STEP_NAME}}"
    // Iteration uses direct Go formatting in RenderIterationLine (D8) because
    // its conditional logic (optional /MAX_ITER suffix, optional issue suffix)
    // is easier to express in Go than in the template string.
)
```

At each header update trigger point (see table below), ralph-tui runs the substitution engine over the active phase's format string with the current variable table values and assigns the result to `h.IterationLine` (which Glyph reads on the next render tick via pointer binding).

**Re-render trigger points (extending D8's table):**

| Event                                  | Initialize line re-renders? | Iteration line re-renders? | Finalize line re-renders? |
|----------------------------------------|----------------------------|---------------------------|---------------------------|
| Phase begins                           | Yes — STEP_NUM=1, STEP_COUNT bound, STEP_NAME is first step | Yes — ITER changes | Yes — STEP_NUM=1 |
| A step begins (just before exec)       | Yes — STEP_NUM and STEP_NAME advance | No — iteration line doesn't reference STEP_NUM/STEP_NAME | Yes — STEP_NUM and STEP_NAME advance |
| `captureAs` binding completes          | No — initialize line doesn't reference captured vars | Yes if `ISSUE_ID` just bound | No |
| Step completes (between steps)         | No — nothing to update until the next step begins | No | No |

**What this means for the Go code:** the header has three phase-specific re-render methods rather than one unified one. Each method reads only the variables its format string uses, so the dependency set is explicit:

```go
// Called at phase start and before each step exec in the initialize phase.
func (h *StatusHeader) RenderInitializeLine(stepNum, stepCount int, stepName string) {
    h.IterationLine = substitute(initializeHeaderFormat, map[string]string{
        "STEP_NUM":   strconv.Itoa(stepNum),
        "STEP_COUNT": strconv.Itoa(stepCount),
        "STEP_NAME":  stepName,
    })
}

// Called at iteration start and after each captureAs binding in the iteration phase.
// This is the D8 method, unchanged — kept as direct Go formatting for the conditional logic.
func (h *StatusHeader) RenderIterationLine(iter, maxIter int, issueID string) {
    // ... as spec'd in D8
}

// Called at phase start and before each step exec in the finalize phase.
func (h *StatusHeader) RenderFinalizeLine(stepNum, stepCount int, stepName string) {
    h.IterationLine = substitute(finalizeHeaderFormat, map[string]string{
        "STEP_NUM":   strconv.Itoa(stepNum),
        "STEP_COUNT": strconv.Itoa(stepCount),
        "STEP_NAME":  stepName,
    })
}
```

The `substitute` function is the shared template engine from D7 — used for command argv, prompt file contents, and now header format strings.

**Edge case — empty initialize phase.** If `initialize` is empty (zero steps), `RenderInitializeLine` is never called. The header transitions directly from "Glyph startup" (blank IterationLine, or a static "Starting..." placeholder) to the first iteration's line. The initialize phase passes through in a single frame without visible animation. Acceptable because an empty initialize array is a config choice the user made.

**Edge case — self-referential STEP_NAME.** A step can reference `{{STEP_NAME}}` in its own command. Example: `"command": ["echo", "running {{STEP_NAME}}"]`. At the time the command is resolved (immediately before `exec.Command`), STEP_NAME is already bound to the step's own name. The substitution produces `echo running Splash` for the Splash step. No chicken-and-egg.

### D13. Config validation scope (runs pre-Glyph, fails fast with collected errors)

The validator runs in plain Go at startup, before Glyph is constructed. It collects all errors in a single pass and prints them to stderr; on any error, it exits with status 1. On success, ralph-tui proceeds to Glyph startup.

**Category 1 — File presence and parseability:**

- **1.1.** `{{PROJECT_DIR}}/ralph-steps.json` exists and is readable.
- **1.2.** The file is valid JSON.
- **1.3.** The top-level object has `initialize`, `iteration`, and `finalize` keys; each must be a JSON array.

**Category 2 — Schema shape (per-step):**

- **2.1.** `name` is a non-empty string.
- **2.2.** `isClaude` is a boolean (missing → error, not silent default).
- **2.3.** Exactly one of `{command, promptFile}` is set per `isClaude`:
  - `isClaude: false` → `command` is a non-empty array of strings; `promptFile` and `model` absent.
  - `isClaude: true` → `promptFile` and `model` are non-empty strings; `command` absent.
- **2.4.** `captureAs`, if present, is a non-empty string and does not shadow any reserved built-in from D5 (`PROJECT_DIR`, `MAX_ITER`, `ITER`, `STEP_NUM`, `STEP_COUNT`, `STEP_NAME`).
- **2.5.** `breakLoopIfEmpty: true` requires (a) `captureAs` also set, and (b) the step to be in the `iteration` array.
- **2.6.** No unknown fields — typos like `"capturesAs"` are caught. Implemented via `json.Decoder.DisallowUnknownFields()`.

**Category 3 — Phase-size checks:**

- **3.1.** `len(iteration) >= 1` — an empty iteration array means no loop work would ever run.
- **3.2.** No maximum on any phase. `initialize`, `iteration`, and `finalize` may each contain any number of steps. The header row count is computed from the largest phase at startup (per D10).

**Category 4 — Referenced files exist:**

- **4.1.** For each claude step, `{{PROJECT_DIR}}/prompts/{promptFile}` exists and is readable.
- **4.2.** For each non-claude step, `command[0]` is resolvable:
  - Relative path with `/` → resolved as `{{PROJECT_DIR}}/command[0]`; file must exist.
  - Absolute path → file must exist at that path.
  - Bare command (no `/`) → `exec.LookPath(command[0])` must succeed.

**Category 5 — Variable reference resolution (walking symbol tables):**

The validator builds per-phase symbol tables by walking steps in declaration order and checks every `{{VAR}}` reference against the current scope.

- **5.1.** Seed the initialize symbol table with: `PROJECT_DIR`, `MAX_ITER`, `STEP_NUM`, `STEP_COUNT`, `STEP_NAME`. (`ITER` is excluded — invalid in initialize.)
- **5.2.** Walk `initialize` in declaration order:
  a. Extract every `{{VAR}}` reference from the step's `command` array (non-claude) or from the contents of `prompts/{promptFile}` (claude).
  b. Each reference must be in the current symbol table.
  c. After validating the step, add its `captureAs` (if any) to the initialize table so later initialize steps can reference it.
- **5.3.** Build the persistent symbol table: initialize built-ins (`PROJECT_DIR`, `MAX_ITER`) + every `captureAs` from initialize.
- **5.4.** Seed the iteration symbol table with: persistent table + iteration built-ins (`ITER`, `STEP_NUM`, `STEP_COUNT`, `STEP_NAME`). Walk `iteration` in declaration order, growing the table with each step's `captureAs` as the walk proceeds.
- **5.5.** Seed the finalize symbol table with: persistent table + finalize built-ins (`STEP_NUM`, `STEP_COUNT`, `STEP_NAME`). **Iteration captures are NOT in scope.** Walk `finalize` in declaration order.
- **5.6.** Unresolved references produce errors with the step name and the unresolved variable name. All errors are collected; validation does not stop at the first one.

**Category 6 — Consistency checks:**

- **6.1.** No duplicate step names within the same phase. Duplicates across phases are legal.
- **6.2.** No duplicate `captureAs` names within the same phase.

**Category 7 — Deliberately out of scope:**

- Claude model name validity (Anthropic changes these over time).
- Script execute bit (filesystem-dependent).
- Network preconditions (`gh auth`, GitHub reachability).
- Git repo preconditions (handled by the `git rev-parse HEAD` iteration step failing clearly).
- Prompt file prose content beyond variable extraction.
- Unused `captureAs` detection (warning-level; validator is strict pass/fail only).

**Category 8 — Error reporting format:**

Each error prints to stderr in this shape:

```
config error: <category>: <phase> step "<step name>": <specific problem>
```

After the pass, a summary line:

```
<N> validation error(s)
```

Then exit with status 1.

**Example error output:**

```
config error: schema: iteration step "Get next issue": breakLoopIfEmpty requires captureAs
config error: references: iteration step "Feature work": prompts/feature-work.md references undefined variable {{ISSUE_ID}} (not in scope yet — earliest iteration step that captures it is declared later)
config error: references: finalize step "Deferred work": prompts/deferred-work.md references iteration-scope variable {{ISSUE_ID}} (not visible in finalize)
config error: files: claude step "Feature work": prompt file prompts/feature-work.md does not exist
4 validation error(s)
```

### D14. Keyboard input wiring and error-mode presentation

**D14a — Glyph owns the keyboard event loop.** Ralph-tui does not spawn its own stdin reader. At app construction, ralph-tui registers per-key callbacks via `app.Handle("q", func(){ keyHandler.Handle("q") })` — the verified Glyph API (see `docs/notes/glyph-api-findings.md`). Every keypress Glyph receives is forwarded to `KeyHandler.Handle(key)`, which then dispatches based on current mode (Normal / Error / QuitConfirm) as already implemented in `internal/ui/ui.go:71-117`. Glyph handles tty raw-mode setup, resize events, and cleanup on exit.

**Rationale:** Glyph is already going to handle `BindVimNav` on the Log panel (`docs/plans/ralph-tui.md:117`). If ralph-tui also read stdin, `j`/`k` keypresses would be delivered twice. A single reader is required, and Glyph has to own tty state anyway.

**D14b — Error mode is expressed by checkbox + shortcut bar, not by a modal overlay.** When a step fails:

1. `header.SetStepState(failedIdx, StepFailed)` flips the failed step's checkbox to `[✗]` (already implemented at `orchestrate.go:54`).
2. `keyHandler.SetMode(ModeError)` swaps the shortcut bar text from `NormalShortcuts` (`↑/k up  ↓/j down  n next step  q quit`) to `ErrorShortcuts` (`c continue  r retry  q quit`) via the existing `updateShortcutLine` logic in `ui.go:132-143`.
3. The Glyph `Text` widget for the shortcut bar is **pointer-bound** to a string field on `KeyHandler`, so step 2 automatically causes the visible text to change on the next render tick — no extra calls needed.

No popup, no modal, no overlay widget. The layout does not shift. The Glyph log panel continues to show subprocess output as a scrollable history, so the last lines of the failed step's output are visible above.

**Minor refactor to `KeyHandler`:** the current `shortcutLine` field is private and mutex-protected (`ui.go:37-38`) because `ShortcutLine()` is callable from arbitrary goroutines. For pointer-binding, Glyph needs to read the field directly via a `*string`. Options:

- **Option P** — change `shortcutLine` to an exported field `ShortcutLine string`, drop the mutex. Safe because: writes only happen from the workflow goroutine (and the signal-handler goroutine via `ForceQuit`, which uses `updateShortcutLine` too); reads happen from Glyph's render goroutine. Go strings are immutable and assignment is atomic; the existing `ShortcutLine()` method is removed.
- **Option Q** — keep the private field and mutex, add a `ShortcutLinePtr() *string` method that returns a pointer to the field. Glyph holds the pointer and reads through it. Still a data race technically, but string assignment is a single word and Go's memory model tolerates this in practice. The mutex becomes dead code.
- **Option R** — stage the shortcut line update: when `SetMode` is called, write the new value to a separate pointer-bound field `DisplayedShortcutLine` that Glyph reads, and still update the private mutex-protected field for `ShortcutLine()` calls. Two sources of truth; overkill.

**Recommendation: Option P.** Remove the mutex, export the field, verify with `go test -race` that no real race exists. If the race detector flags it, switch to Option Q and accept the mutex as dead code for render-path correctness.

**D14c — Workflow goroutine wakeup after error-mode decision.** The existing channel mechanism survives unchanged:

- `Orchestrate.runStepWithErrorHandling` at `orchestrate.go:57` blocks on `action := <-h.Actions` after entering error mode.
- Glyph dispatches a keypress → `keyHandler.Handle("c")` → `handleError` sends `ActionContinue` to `h.Actions` (`ui.go:98`).
- The workflow goroutine's receive unblocks; it sets mode back to Normal and either continues (ActionContinue), retries the step (ActionRetry), or returns (ActionQuit).

**Channel buffer size at `main.go:43` (`make(chan ui.StepAction, 10)`) is unchanged.** 10 is overkill for the actual use case (in practice we only need size 1, since the workflow goroutine is always receiving when in error mode), but the existing size is fine and no resize is needed.

**Goroutine ownership summary:**

| Goroutine                | Role                                                                 |
|--------------------------|----------------------------------------------------------------------|
| Main goroutine           | Runs Glyph's app loop (`app.Run()` blocks). Handles tty, rendering, key dispatch. |
| Workflow goroutine       | Runs `workflow.Run()` — spawns subprocesses, updates header/variable state, blocks on `<-h.Actions` in error mode. |
| Glyph internal goroutine | Reads subprocess log pipe via `Log(io.Reader)`, manages ring buffer, triggers render ticks. (Owned by Glyph, not by ralph-tui.) |
| Signal-handler goroutine | Listens for SIGINT/SIGTERM, calls `keyHandler.ForceQuit()` (which sends `ActionQuit` to the channel and calls `runner.Terminate()`). |

Four goroutines total. Two shared channels (`h.Actions` for user decisions, the log pipe for subprocess output). One shared pointer-bound state (`StatusHeader` struct fields + `KeyHandler.ShortcutLine` field). All communication is via channels or pointer-read single-writer state.

### D15. Completion and exit: wait for keypress, show summary, then tear down

When the last finalize step completes (or the finalize phase is skipped because it has zero steps and ralph-tui falls through to completion), ralph-tui does **not** auto-exit. Instead:

1. **Header line becomes the completion summary.** `header.IterationLine` is replaced with the hardcoded string `"Ralph completed after N iteration(s) and M finalizing tasks."` — the same format that `internal/workflow/run.go:124-125` currently writes to the log pipe. `N` is the count of actually-run iterations (not the `--iterations` flag value); `M` is `len(finalizeSteps)`.
2. **Step checkbox rows are left as-is.** Finalize's final state (all green `[✓]` checkboxes) stays on screen. The user sees proof that every finalize step succeeded.
3. **Shortcut bar switches to a new `ModeDone` state.** Text becomes `"done — press any key to exit"`. The new `ModeDone` state is added to the `Mode` enum in `internal/ui/ui.go:17-21`. Its key handler accepts any key and sends `ActionQuit` to `h.Actions`:
   ```go
   const ModeDone Mode = iota + ModeQuitConfirm + 1 // after existing modes
   const DoneShortcuts = "done — press any key to exit"

   func (h *KeyHandler) handleDone(key string) {
       // Any key exits. No filtering.
       h.Actions <- ActionQuit
   }
   ```
   `SetMode(ModeDone)` updates `ShortcutLine` via `updateShortcutLine` (which needs a new case for `ModeDone` returning `DoneShortcuts`).
4. **Workflow goroutine blocks on `<-h.Actions` one last time.** After posting the completion state (steps 1–3), the workflow goroutine does not return yet. It waits for the user to press any key.
5. **Any keypress exits.** Glyph dispatches → `keyHandler.Handle(k)` → `handleDone` → `ActionQuit` → workflow goroutine unblocks → closes the log pipe → returns from `workflow.Run`.
6. **Main goroutine completes cleanup.** `<-done` unblocks in `main.go`; `app.Run()` returns; cleanup runs; process exits with status 0.

**Exception paths (user quit before completion):**

- User pressed `q` / `y` in quit-confirm mode during any phase → `ActionQuit` is sent → workflow returns immediately, skipping the completion state. Exit happens via the existing `case <-signaled` or default-exit path in `main.go:98-103`.
- SIGINT / SIGTERM → signal handler calls `keyHandler.ForceQuit()` → `ActionQuit` injected → workflow returns → exit with status 1 via the `signaled` channel.
- A validation error at startup → no Glyph app ever runs → plain stderr message + exit status 1. This path does not touch any of the completion logic.

**Dead code to remove as a consequence:**

- The log-pipe write at `run.go:124-125` (`executor.WriteToLog("Ralph completed after ...")`) is removed. The same string now goes into the header line instead.
- The completion text is the only piece of hardcoded Ralph-specific copy that survives in ralph-tui, per the narrow-principle carve-out in D3c.

**Counter tracking:**

`iterationsRun` (currently local to `Run()` at `run.go:70`) still needs to exist to produce the completion count. It's incremented at the end of each successful iteration loop body, as it is today. The new completion-summary code reads it at the end.

### D16. First-frame header state: pre-populate synchronously before `app.Run()`

Before the Glyph app starts rendering, `main.go` populates the header with the first phase's state so the first frame shows real content, not an empty box.

**Startup sequence (ordered, main goroutine):**

1. `cli.Execute()` parses CLI flags into `Config`.
2. The config validator (D13) runs against `projectDir/ralph-steps.json`. On failure: stderr + exit 1, no Glyph.
3. `logger.NewLogger(projectDir)` opens the log file.
4. `steps.LoadSteps(projectDir)` parses the JSON (now already validated).
5. `maxStepsAcrossPhases := max(len(initialize), len(iteration), len(finalize))`.
6. `header := NewStatusHeader(maxStepsAcrossPhases)` — grid sized to the largest phase.
7. `keyHandler := NewKeyHandler(runner.Terminate, actions)` — starts in `ModeNormal`, `ShortcutLine` is `NormalShortcuts`.
8. **Pre-populate the first visible phase state:**
   ```go
   if len(stepFile.Initialize) > 0 {
       header.SetPhaseSteps(stepNames(stepFile.Initialize))
       header.RenderInitializeLine(1, len(stepFile.Initialize), stepFile.Initialize[0].Name)
   } else {
       header.SetPhaseSteps(stepNames(stepFile.Iteration))
       header.RenderIterationLine(1, cfg.Iterations, "")
   }
   ```
   The `else` branch assumes `len(iteration) >= 1`, which is guaranteed by the D13 validator rule 3.1.
9. The Glyph app tree is constructed with pointer bindings to `header`'s fields and `keyHandler.ShortcutLine`.
10. The workflow goroutine is spawned; it will eventually enter the first phase and call the same `SetPhaseSteps` + `RenderInitializeLine` methods (redundantly, idempotently).
11. `app.Run()` is called on the main goroutine. The first rendered frame shows the pre-populated state.

**Why pre-populate synchronously and idempotently:**

- **No loading transient.** The user never sees an empty header. First frame = meaningful content.
- **No race to render.** Whether the workflow goroutine is fast or slow to start its first step doesn't matter — the header's starting state is already correct when `app.Run()` takes over.
- **Idempotent redundancy is simpler than "special-case the first step".** The workflow goroutine's phase setup code is the same for the first iteration of the phase as for all subsequent ones. Having main.go call `SetPhaseSteps` + `RenderInitializeLine` and then the workflow goroutine call them again is slightly wasteful (two writes where one would suffice) but avoids any "step 1 is special" branching in the workflow goroutine, which is a much worse complexity tax.

**`stepNames` helper:** a small utility in `main.go` (or `internal/workflow`) that walks a `[]steps.Step` and returns a `[]string` of just the `Name` fields. Replaces the existing hardcoded `[8]string` loop at `main.go:46-52`.

**Empty-initialize fallback:** if the initialize array is empty, the pre-populate step shows iteration's first step pending, and the workflow goroutine skips the initialize phase entirely (D12 edge case). The iteration phase's first frame is `Iteration 1/3 — Issue #<still unbound>` (or without the `/MAX_ITER` suffix in unbounded mode, without the issue suffix). Then the first iteration step runs and the reactive header update from D8 fills in the issue ID.

### D17. Implementation phasing: three sequenced PRs — schema → TUI → polish

The plan ships in three PRs, in order. Each PR ends in a self-consistent, shippable state. No attempt is made to deliver everything in one branch.

**PR1 — Schema + validator + step migration (backend only, stdout mode preserved)**

Scope:

- Add the `initialize` array to the `steps.StepFile` struct and its JSON schema; `ralph-steps.json` (both source copy at `ralph-tui/ralph-steps.json` and the build-output copy at `bin/ralph-steps.json`) gains the new top-level key.
- Add the `captureAs` and `breakLoopIfEmpty` fields to the `steps.Step` struct. Remove the `prependVars` field. Remove the `buildIterationSteps` / `buildFinalizeSteps` functions' `issueID` and `sha` parameters.
- Implement the variable table (`VarTable` type — scoped per D5) and the `{{VAR}}` substitution engine (used for command argv and prompt file contents per D7). Used by both non-claude and claude steps.
- Implement the full validator (D13 Categories 1-8). Validator runs in a new `internal/validator` package, called from `cmd/ralph-tui/main.go` immediately after `steps.LoadSteps`.
- Delete the hardcoded `get_gh_user` capture at `run.go:62-67`. Delete the hardcoded `get_next_issue` prologue at `run.go:72-78`. Delete the hardcoded `git rev-parse HEAD` prologue at `run.go:80-83`. Delete the `iterationLabel` helper at `run.go:131-137` if nothing references it.
- Add corresponding JSON entries to `initialize` (Splash + Get GitHub user) and `iteration` (Get next issue + Get starting SHA as the first two entries) per D9 and D11.
- Rewrite the five affected prompt files (`feature-work.md`, `code-review-changes.md`, `test-planning.md`, and any others that reference `ISSUENUMBER` or `STARTINGSHA`) using Migration B from D7.
- Delete `//go:embed ralph-art.txt` and the `bannerArt` variable + the `for _, line := range strings.Split(bannerArt, "\n") { executor.WriteToLog(line) }` block at `run.go:13-14, 57-60`. Move `ralph-tui/internal/workflow/ralph-art.txt` to `{repo-root}/ralph-art.txt`. Update the `Makefile`'s `build` target to copy it into `bin/`.
- Implement the `breakLoopIfEmpty` runtime check in the orchestrator (when an iteration step with the flag succeeds and its captured value is empty, mark remaining iteration steps as `StepSkipped` — note: `StepSkipped` state is still added in PR3; in PR1 the orchestrator can just `break` out of the iteration loop without marking, since the checkbox UI isn't rendered yet).
- `ralph-tui` still drains the log pipe to stdout via `fmt.Println` in the same goroutine at `main.go:56-63`. No Glyph yet. No layout. But the workflow is now fully config-driven.
- **End state:** ugly stdout mode, but with the full config-driven backend working. Every piece of workflow content lives in `ralph-steps.json`. Validation runs at startup and fails fast with actionable messages.

**PR2 — Glyph integration + dynamic header rendering**

Scope:

- Add Glyph as a Go module dependency. Update `ralph-tui/go.mod` and `go.sum`.
- Restructure `StatusHeader` per D10 — dynamic `Rows [][HeaderCols]string`, sized at construction via `NewStatusHeader(maxStepsAcrossPhases)`. Delete `Row1 [4]string`, `Row2 [4]string`, `stepNames [8]string`, `finalizeNames []string`, `SetFinalization`, `SetFinalizeStepState`.
- Unify the phase-transition API: `SetPhaseSteps(names []string)` + `SetStepState(idx, state)` replace the per-phase method split. Delete the `iterHeader` / `finalHeader` adapter structs at `run.go:42-51`.
- Delete the stdout-drain goroutine at `main.go:56-63`. Replace it with a Glyph `Log(runner.LogReader())` widget inside the VBox tree.
- Construct the Glyph VBox tree in `main.go` with the header iteration line, checkbox rows, log panel, and shortcut bar. Wire pointer-bindings to `StatusHeader` fields and `KeyHandler.ShortcutLine`.
- Expose `KeyHandler.ShortcutLine` as an exported field (D14 Option P). Remove the `ShortcutLine()` method and the mutex if the race detector does not flag the exported-field approach.
- Wire Glyph's key dispatch (`app.Handle(key, fn)`) to call `keyHandler.Handle(key)`.
- Implement `NewStatusHeader(maxStepsAcrossPhases)` computing row count from the largest phase.
- Pre-populate first-frame state (D16) in `main.go` before `app.Run()`.
- Update `cmd/ralph-tui/main.go`: delete the 8-element hardcoded `var stepNames [8]string` at lines 46-52; compute `maxStepsAcrossPhases` from the loaded config instead.
- **End state:** the TUI layout appears on launch. Subprocess output streams to the log panel. Header shows current phase and checkbox rows. Error mode (existing mechanism) can be reached via step failures. Keyboard shortcuts work. But reactive header lines with `{{STEP_NUM}}`/`{{STEP_NAME}}`/`{{ISSUE_ID}}` are not yet fully implemented — header may still show simple static text during each phase.

**PR3 — Reactive header lines, completion state, polish, docs**

Scope:

- Implement `RenderInitializeLine(stepNum, stepCount, stepName)`, `RenderIterationLine(iter, maxIter, issueID)`, `RenderFinalizeLine(stepNum, stepCount, stepName)` methods on `StatusHeader` per D12.
- Wire the reactive update points per D8/D12: call the appropriate render method at phase start, step start, and after each `captureAs` binding.
- Add `StepSkipped` to the `StepState` enum (`header.go:7-13`) with display marker `[-]`. Update `checkboxLabel` at `header.go:107-118` with the new case.
- Update the orchestrator's `breakLoopIfEmpty` path to mark remaining iteration steps as `StepSkipped` (upgrading PR1's plain `break`).
- Add `ModeDone` to the `Mode` enum (`ui.go:17-21`) with `DoneShortcuts = "done — press any key to exit"`. Add `handleDone(key)` method to `KeyHandler` that sends `ActionQuit` on any key. Update `updateShortcutLine` with the new case.
- Implement the completion sequence per D15: after the last finalize step (or when falling through without a finalize phase), write the completion summary to `header.IterationLine`, call `keyHandler.SetMode(ModeDone)`, and block the workflow goroutine on `<-h.Actions` one last time.
- Remove the old hardcoded log-pipe write at `run.go:124-125` (`executor.WriteToLog("Ralph completed after ...")`).
- Update documentation:
  - `docs/features/cli-configuration.md:158` — remove the claim that `ralph-art.txt` is embedded; update to describe it as a repo-root asset copied into `bin/` by `make build` and displayed via the first initialize step.
  - `docs/features/workflow-orchestration.md:120` — remove the claim that phase 1 displays an embedded banner; rewrite to describe the new three-phase config-driven orchestration.
  - Any other feature doc that references `prependVars`, the 8-step cap, the stdout drain path, or hardcoded prologue calls.
  - Add a new ADR under `docs/adr/` documenting the narrow-reading architectural principle (D3c), in case future contributors need to know why ralph-tui is a generic step runner rather than a Ralph-specific workflow tool.
- **End state:** fully polished TUI matching the entire D1–D16 spec. Every user-facing behavior from the original ask is delivered.

**Why schema-first (recap):**

1. Moving workflow from Go to config is the highest-risk change (silent correctness bugs possible). Doing it first, in stdout mode, makes debugging tractable — stdout is greppable in a way a TUI isn't.
2. Glyph is a new dependency with unknown sharp edges. Isolating it in PR2, against a known-good config-driven backend, keeps framework bugs and schema bugs from tangling together.
3. Each PR is independently useful and shippable. PR1 ships a config-driven stdout orchestrator; PR2 ships a TUI; PR3 ships polish.
4. Every rejected alternative (single big-bang PR, TUI-first) either loses the "isolated risk" property or reverses the work order in a way that produces rework.

**Caveats worth calling out on merge of each PR:**

- **After PR1 merges**, ralph-tui's stdout output looks different from today. The banner is gone; the new config-driven phases may produce slightly different log lines. A user who runs ralph-tui between PR1 and PR2 will see this regression in visual polish, though functionality should be unchanged or improved.
- **After PR2 merges**, the TUI is live but reactive lines and completion state may feel a bit bare. Users get the layout back but don't yet see issue IDs updating in the header line.
- **After PR3 merges**, the entire spec is delivered. No further PRs required for the scope of this design.

---

## Target `ralph-steps.json` (complete example — the end state after PR1)

This is the exact shape `ralph-tui/ralph-steps.json` (source) and `bin/ralph-steps.json` (build output) should have after PR1 merges. Both files must be kept in sync; the `make build` target copies the source to `bin/` so the build output reproduces this content.

```json
{
  "initialize": [
    {
      "name": "Splash",
      "isClaude": false,
      "command": ["cat", "{{PROJECT_DIR}}/ralph-art.txt"]
    },
    {
      "name": "Get GitHub user",
      "isClaude": false,
      "command": ["scripts/get_gh_user"],
      "captureAs": "GITHUB_USER"
    }
  ],
  "iteration": [
    {
      "name": "Get next issue",
      "isClaude": false,
      "command": ["scripts/get_next_issue", "{{GITHUB_USER}}"],
      "captureAs": "ISSUE_ID",
      "breakLoopIfEmpty": true
    },
    {
      "name": "Get starting SHA",
      "isClaude": false,
      "command": ["git", "rev-parse", "HEAD"],
      "captureAs": "STARTING_SHA"
    },
    {"name": "Feature work",  "model": "sonnet", "promptFile": "feature-work.md",       "isClaude": true},
    {"name": "Test planning", "model": "opus",   "promptFile": "test-planning.md",      "isClaude": true},
    {"name": "Test writing",  "model": "sonnet", "promptFile": "test-writing.md",       "isClaude": true},
    {"name": "Code review",   "model": "opus",   "promptFile": "code-review-changes.md","isClaude": true},
    {"name": "Review fixes",  "model": "sonnet", "promptFile": "code-review-fixes.md",  "isClaude": true},
    {"name": "Close issue",   "isClaude": false, "command": ["scripts/close_gh_issue", "{{ISSUE_ID}}"]},
    {"name": "Update docs",   "model": "sonnet", "promptFile": "update-docs.md",        "isClaude": true},
    {"name": "Git push",      "isClaude": false, "command": ["git", "push"]}
  ],
  "finalize": [
    {"name": "Deferred work",   "model": "sonnet", "promptFile": "deferred-work.md"},
    {"name": "Lessons learned", "model": "sonnet", "promptFile": "lessons-learned.md"},
    {"name": "Final git push",  "isClaude": false, "command": ["git", "push"]}
  ]
}
```

**Note on the finalize claude steps:** the `"isClaude": true` field is implied on those two entries because `promptFile` and `model` are set. Strictly speaking, per D13 rule 2.2, `isClaude` must be present explicitly (missing → error, not silent default). If validation is strict on this, the finalize array should be written as:

```json
"finalize": [
  {"name": "Deferred work",   "model": "sonnet", "promptFile": "deferred-work.md",   "isClaude": true},
  {"name": "Lessons learned", "model": "sonnet", "promptFile": "lessons-learned.md", "isClaude": true},
  {"name": "Final git push",  "isClaude": false, "command": ["git", "push"]}
]
```

Verify this during PR1 implementation — make sure the validator's 2.2 rule and the JSON file agree.

**`prependVars` is absent from every claude step.** The field no longer exists per D7. Prompts reference variables via `{{ISSUE_ID}}` and `{{STARTING_SHA}}` inline (Migration B rewrite).

**Variable flow through this config:**

- Startup → `PROJECT_DIR` and `MAX_ITER` populated by ralph-tui from CLI flags.
- Initialize step 1 (Splash) → runs `cat {{PROJECT_DIR}}/ralph-art.txt`; no capture.
- Initialize step 2 (Get GitHub user) → runs `scripts/get_gh_user`; last non-empty line of stdout → `GITHUB_USER` in persistent table.
- Iteration start → `ITER` updated to current iteration number.
- Iteration step 1 (Get next issue) → runs `scripts/get_next_issue mxriverlynn` (substituted from `{{GITHUB_USER}}`); last non-empty line → `ISSUE_ID` in iteration table. If empty → `breakLoopIfEmpty` fires, remaining iteration steps marked `StepSkipped`, loop exits.
- Iteration step 2 (Get starting SHA) → runs `git rev-parse HEAD`; last non-empty line → `STARTING_SHA` in iteration table.
- Iteration steps 3–10 → claude prompts substitute `{{ISSUE_ID}}` and `{{STARTING_SHA}}` inline; shell commands substitute `{{ISSUE_ID}}` in argv.
- Finalize → `ISSUE_ID` and `STARTING_SHA` are NOT in scope (iteration table is cleared); only persistent vars (`GITHUB_USER`, `PROJECT_DIR`, `MAX_ITER`) + finalize built-ins (`STEP_NUM`, `STEP_COUNT`, `STEP_NAME`) visible.
- Completion → `iterationsRun` counter and `len(finalize)` fill the completion summary line.

---

## Files touched by this plan (authoritative list)

Every file that needs to be added, modified, deleted, or moved as part of implementing D1–D17. Organized by PR.

### PR1 — schema + validator + step migration

**Modified:**

- `ralph-tui/ralph-steps.json` — add `initialize` array; rewrite `iteration` with two prologue steps + remove `prependVars` from claude steps; align with the target JSON above.
- `bin/ralph-steps.json` — same changes; kept in sync by `make build`.
- `ralph-tui/internal/steps/steps.go` — add `Step.CaptureAs`, `Step.BreakLoopIfEmpty` fields; add `StepFile.Initialize` field; remove `Step.PrependVars` field; update `BuildPrompt` to run `{{VAR}}` substitution instead of prepending header lines.
- `ralph-tui/internal/workflow/workflow.go` — update `ResolveCommand` to handle arbitrary `{{VAR}}` tokens (replacing the single-variable `{{ISSUE_ID}}` substitution at `workflow.go:189-206`); add stdout capture buffer for the "last non-empty line" extraction (D4).
- `ralph-tui/internal/workflow/run.go` — delete the hardcoded `get_gh_user` capture (lines 62-67), `get_next_issue` call (lines 72-78), `git rev-parse HEAD` call (lines 80-83); delete `iterationLabel` helper (lines 131-137) if unused; delete the `//go:embed ralph-art.txt` line (13-14), the `bannerArt` variable, and the banner-to-log loop (lines 57-60); delete the completion-summary log write (lines 124-125) — this string moves to the header line in PR3 but the log write is deleted now; restructure `Run()` to drive the three phases via config and the new variable table.
- `ralph-tui/cmd/ralph-tui/main.go` — call validator immediately after `steps.LoadSteps`; on validator failure, stderr + exit 1; stdout-drain goroutine stays (removed in PR2).
- `Makefile` — add `cp ralph-art.txt bin/` to the `build` target.
- `prompts/feature-work.md` — Migration B: replace bare `ISSUENUMBER` with `#{{ISSUE_ID}}`.
- `prompts/code-review-changes.md` — Migration B: replace bare `STARTINGSHA` with `{{STARTING_SHA}}` and bare `ISSUENUMBER` with `#{{ISSUE_ID}}`.
- `prompts/test-planning.md` — Migration B: replace bare `STARTINGSHA` with `{{STARTING_SHA}}` and bare `ISSUENUMBER` with `#{{ISSUE_ID}}`.
- Any other `prompts/*.md` files that reference `ISSUENUMBER` or `STARTINGSHA` as bare words — grep first, rewrite any matches. Files known to NOT need changes based on the audit: `code-review-fixes.md`, `test-writing.md`, `deferred-work.md`, `lessons-learned.md`, `update-docs.md`.

**Added (new files):**

- `{repo-root}/ralph-art.txt` — moved from `ralph-tui/internal/workflow/ralph-art.txt`.
- `ralph-tui/internal/validator/validator.go` (or equivalent package) — new validator package implementing all of D13's categories.
- `ralph-tui/internal/validator/validator_test.go` — table-driven tests for valid configs, every failure mode from D13, and the error-collection pass.
- `ralph-tui/internal/vars/vars.go` (or merged into `internal/workflow/`) — `VarTable` type with persistent + iteration table, built-in seeding, resolution order, `Substitute(s string) (string, error)` method for the `{{VAR}}` engine.
- `ralph-tui/internal/vars/vars_test.go` — tests for scope rules, resolution order, unresolved references, built-in initialization.

**Deleted:**

- `ralph-tui/internal/workflow/ralph-art.txt` — moved to repo root.

**Tests to update or rewrite:**

- `ralph-tui/internal/steps/steps_test.go` — tests for the old `prependVars` mechanism are deleted; new tests for `{{VAR}}` substitution in prompt files.
- `ralph-tui/internal/workflow/workflow_test.go` — update `ResolveCommand` tests to cover the generalized `{{VAR}}` engine.
- `ralph-tui/internal/workflow/run_test.go` — update tests that assumed the hardcoded prologue, variable threading through locals, or the banner print path.

### PR2 — Glyph integration + dynamic header

**Modified:**

- `ralph-tui/go.mod` + `go.sum` — add Glyph dependency.
- `ralph-tui/internal/ui/header.go` — remove `Row1`, `Row2`, `stepNames [8]`, `finalizeNames` fields; add dynamic `Rows [][HeaderCols]string` and `stepNames []string`; `NewStatusHeader(maxStepsAcrossPhases int)` computes row count; add unified `SetPhaseSteps(names []string)` method; unify `SetStepState` across phases; delete `SetFinalization` and `SetFinalizeStepState`.
- `ralph-tui/internal/ui/ui.go` — change `shortcutLine` to exported `ShortcutLine` field (Option P from D14b); drop the mutex; delete the `ShortcutLine()` method accessor.
- `ralph-tui/internal/workflow/run.go` — delete the `iterHeader` / `finalHeader` adapter structs (lines 42-51) since `StatusHeader` now has a unified API; simplify orchestration to call `SetPhaseSteps` once per phase transition.
- `ralph-tui/cmd/ralph-tui/main.go` — delete the stdout-drain goroutine at lines 56-63; delete the hardcoded `var stepNames [8]string` at lines 46-52; compute `maxStepsAcrossPhases` from the loaded config; construct the Glyph VBox tree with pointer bindings; wire Glyph keypress dispatch to `keyHandler.Handle`; pre-populate the first-frame header state per D16; call `app.Run()` on the main goroutine.

**Tests to update:**

- `ralph-tui/internal/ui/header_test.go` — rewrite tests for the new dynamic-rows API (`SetPhaseSteps` replaces `SetFinalization` etc.); add tests for `NewStatusHeader` row-count computation.
- `ralph-tui/internal/ui/ui_test.go` — update tests that used `ShortcutLine()` method to read the new exported field directly.
- `ralph-tui/internal/ui/orchestrate_test.go` — adjust any tests that depended on the adapter structs.

### PR3 — reactive header lines, completion state, polish, docs

**Modified:**

- `ralph-tui/internal/ui/header.go` — add `RenderInitializeLine`, `RenderIterationLine`, `RenderFinalizeLine` methods per D8/D12; add `StepSkipped` to the `StepState` enum with `[-]` marker; update `checkboxLabel` with the new case.
- `ralph-tui/internal/ui/ui.go` — add `ModeDone` to the `Mode` enum; add `DoneShortcuts` constant (`"done — press any key to exit"`); add `handleDone(key)` method that sends `ActionQuit` on any key; update `updateShortcutLine` with the new case.
- `ralph-tui/internal/ui/orchestrate.go` — update `breakLoopIfEmpty` path to mark remaining iteration steps as `StepSkipped` (upgrading PR1's plain `break`).
- `ralph-tui/internal/workflow/run.go` — wire reactive header-line updates at the trigger points from D8/D12 (phase start, step start, captureAs completion); implement the completion sequence from D15 (set completion summary in header, switch to ModeDone, block on `<-h.Actions`); remove the old hardcoded log-pipe write at lines 124-125.
- `docs/features/cli-configuration.md` (line 158 area) — remove the claim that `ralph-art.txt` is embedded; describe it as a repo-root asset copied by `make build`; mention that it's displayed via the first `initialize` step.
- `docs/features/workflow-orchestration.md` (line 120 area) — remove the "displays the embedded ralph-art.txt banner" claim; rewrite phase 1 description to describe the config-driven three-phase orchestration; update the flow to match the new architecture.
- Any other feature doc that references `prependVars`, the 8-step cap, the stdout drain path, or hardcoded prologue calls — grep all `docs/features/*.md` for these terms and update.

**Added (new files):**

- `docs/adr/NNN-narrow-reading-principle.md` — new ADR documenting D3c's architectural principle (ralph-tui facilitates workflow, doesn't define it). Include references to D3c, D7, D9, and the rejected alternatives (broad reading). Use the same ADR format as the existing `docs/adr/20260409135303-cobra-cli-framework.md`.

**Tests to update:**

- `ralph-tui/internal/ui/header_test.go` — add tests for the new render methods, the `StepSkipped` state, and `checkboxLabel`'s new case.
- `ralph-tui/internal/ui/ui_test.go` — add tests for `ModeDone` and `handleDone`.
- `ralph-tui/internal/ui/orchestrate_test.go` — add tests for the `breakLoopIfEmpty` → `StepSkipped` path.
- `ralph-tui/internal/workflow/run_test.go` — add tests for reactive header updates, completion sequence, and the `block-on-<-h.Actions-one-more-time` pattern.

---

## Implementation assumptions that need verification during PR1/PR2

These are claims I made during the grill-me interview without running code or reading the Glyph source. Each one is backed by what I could infer from the existing plan + audit, but none is fully verified. Implementers should check these before or during the relevant PR.

**V1. Glyph's actual API shape. ✓ VERIFIED (issue #46)**

Verified via `go doc`, source inspection, and a scratch app (`ralph-tui/scratch/main.go`). Full findings in `docs/notes/glyph-api-findings.md`. The design shape survived — all elements exist with matching names, with two nuances:

- `Text(&stringField)` — exists, name matches.
- `HBox(...children)` / `VBox(...children)` — exist, name matches. Modifiers chain on the **function**, not the result: `glyph.VBox.Border(glyph.BorderRounded).Title("Ralph")(child1, child2)`.
- `Log(io.Reader)` — exists, name matches.
- `Border(...).Title("Ralph")` — exists, name matches.
- `.Grow(1)` — exists, name matches.
- `.MaxLines(n)` — exists, name matches.
- `.BindVimNav()` — exists, name matches.
- `.OnUpdate(callback)` — exists, name matches.
- `app.Handle(key, fn)` — **verified API**; `app.BindKey` and `app.OnKey` do not exist.
- `app.Run()` — exists, name matches.

**V2. Race detector behavior on exported `ShortcutLine`.**

What I assumed: D14b Option P (exported string field, no mutex) will not trigger the Go race detector because the read/write pattern is single-writer (the workflow goroutine writes via `updateShortcutLine`; the signal-handler goroutine writes via `ForceQuit` → `updateShortcutLine`; Glyph's render goroutine reads through the pointer) and Go string assignment is atomic in practice.

**What to verify:** run `go test -race ./...` after PR2 merges the exported-field refactor. If the race detector flags it, switch to Option Q (keep private field + mutex, add `ShortcutLinePtr()` accessor) and accept the mutex as dead code from the render path's perspective.

**Note on the "two writers" concern:** there are actually two writer goroutines — the workflow goroutine (normal mode transitions) and the signal-handler goroutine (via `ForceQuit`). This could in principle race write-with-write. But both writers ultimately funnel through `updateShortcutLine`, and the signal handler only writes once per process lifetime (at SIGINT). The probability of a real observable race is low, but the race detector may still flag it. **Plan:** start with Option P; if `-race` flags, fall back to Option Q.

**V3. The `ralph-tui/ralph-steps.json` source copy is stale.**

What I observed during the audit: `bin/ralph-steps.json` (build output) was edited by the user to add the `initialize: []` key; `ralph-tui/ralph-steps.json` (source) was NOT updated. These two files are normally kept in sync by `make build` (which copies source → bin), but the user edited the bin copy by hand.

**What to do during PR1:** manually sync the source copy to match the bin copy, then apply the full target JSON from the "Target `ralph-steps.json`" section above. After PR1 merges, any future edits should be made to the source copy only, and `make build` should be run to regenerate bin. Don't edit bin directly.

**V4. Prompt file scan for `ISSUENUMBER` / `STARTINGSHA`.**

What I verified during the audit: three files (`feature-work.md`, `code-review-changes.md`, `test-planning.md`) reference one or both of these as bare words. I did NOT exhaustively check the other five files (`code-review-fixes.md`, `test-writing.md`, `deferred-work.md`, `lessons-learned.md`, `update-docs.md`). The audit only read the first 5 lines of each.

**What to do during PR1:** run `grep -l 'ISSUENUMBER\|STARTINGSHA' prompts/*.md` to get the authoritative list of files needing Migration B rewrites. Update every match. If any file references these variables inside prose that's further than line 5, it still needs to be rewritten.

**V5. The `breakLoopIfEmpty` / `StepSkipped` split between PR1 and PR3.**

What I assumed: PR1 implements the `break` semantics (workflow exits the loop when a flagged step's captured variable is empty) without marking remaining steps as `StepSkipped` (because the checkbox UI doesn't exist in PR1 yet). PR3 upgrades the `break` to mark remaining steps.

**What to verify:** whether this split is actually cleaner than implementing the whole thing in one PR. If `StepSkipped` is a trivial addition to the orchestrator (maybe it is — it's just a new enum value and a loop iteration change), it could move entirely to PR2 or PR3. Implementers may rebalance the split during implementation. The design doesn't depend on which PR lands it, only that the final state is consistent.

**V6. The finalize array's `isClaude` field — strict vs permissive validation.**

What I assumed: the validator rule 2.2 is strict — missing `isClaude` is an error. The target JSON example shows `isClaude: true` explicitly on every claude step for clarity.

**What to verify:** the existing `ralph-steps.json` might omit `isClaude` from the finalize array (the current Go struct tag is `json:"isClaude"` with no `omitempty`, so it should be required, but this isn't actually enforced by the current code). If the existing config omits it, the strict validator will reject the old config on first run after PR1 merges. Two fixes:

- **Option A:** update every step in `ralph-steps.json` to include `isClaude` explicitly (what the target JSON shows).
- **Option B:** make the validator permissive — if `isClaude` is missing, infer from the presence of `promptFile` (true) or `command` (false).

I'd lean on Option A for explicitness. Implementer's call during PR1.

**V7. The `{{VAR}}` substitution engine's escape behavior.**

What I specified: `{{{{` → literal `{{`, `}}}}` → literal `}}`. Chosen because it's the minimum needed to handle literal double-braces in a prompt (rare but possible).

**What to verify:** whether any existing prompt file contains literal `{{` or `}}` sequences that would be misinterpreted by the substitution engine. Grep `prompts/*.md` for `{{` and `}}` — anything that matches needs to be either (a) a legitimate `{{VAR}}` reference I intended, or (b) an escape.

**What to do during PR1:** add a pre-commit check or implementation-time scan that flags ambiguous double-brace sequences in prompts. If none exist today, the escape rule is a defensive addition for future prompts.

---

## Information to preserve when creating GitHub issues from this plan

Each of the following is a piece of information that lives in this doc but might not survive conversion to a GitHub issue if the issue creator is terse. Each one is something an implementer will need.

1. **The narrow-reading principle (D3c).** This is the single most important architectural constraint in the plan. Every PR implementer should read D3c before writing any code — it explains *why* certain things live in config instead of Go, and why the Go code is simpler than it could be. Without understanding the principle, an implementer might "helpfully" add hardcoded workflow knowledge that contradicts the design.

2. **The user's explicit rejection of backward compatibility (D7).** Migration B rewrites existing prompt files; there is no shim for the old `prependVars` behavior. Issue text should flag this so no reviewer tries to re-add backwards compat as a "safety net".

3. **The three new built-in variables (`STEP_NUM`, `STEP_COUNT`, `STEP_NAME` from D5/D12).** These are available everywhere and phase-scoped, with `STEP_COUNT` meaning "count of steps in the current phase" not "grand total". Easy to get wrong if the implementer doesn't read the scope rule carefully.

4. **The file mismatch warning (V3).** `bin/ralph-steps.json` was edited by hand; the source copy is stale. PR1 must sync them. Don't assume the source copy is correct.

5. **The audit findings (A1–A9).** These document the pre-plan state with evidence. They are what a reviewer would need to know to understand why PR1 deletes so much code — without the audit, the deletions look aggressive.

6. **The wait-for-keypress completion sequence (D15).** Easy to skip if the implementer assumes "when workflow.Run() returns, the app exits". The whole point is that `workflow.Run()` does NOT return after finalize — it blocks one more time until a keypress arrives.

7. **The `{{VAR}}` escape rule (`{{{{` → `{{`).** Tiny detail but it affects the substitution engine's implementation. Implementers may otherwise forget to handle the escape.

8. **The Glyph API verification task (V1).** PR2 should start with a minimal "hello world" Glyph app to confirm the assumed API shape before rewriting the existing UI code.

9. **The reserved variable name list (`PROJECT_DIR`, `MAX_ITER`, `ITER`, `STEP_NUM`, `STEP_COUNT`, `STEP_NAME`).** Validator needs this exact list. Don't add or remove without updating D5, D13, and validation tests simultaneously.

10. **The 3-PR phasing rationale (D17).** If a reviewer pushes back on the phasing and wants a single PR or a different order, D17 has the reasoning for why schema-first is correct. Refer them to it.

---

## Grill-me session history (for auditing the decision trail)

This plan was built through an interview of 17 focused questions, each answered explicitly by the user. The intermediate states of the file were saved incrementally as each decision was locked. For each decision D1–D17:

- The **question** was posed with 2–4 candidate options.
- A **recommendation** was made with explicit reasoning.
- **Caveats** were flagged where the recommendation involved tradeoffs or uncertainty.
- The **user's answer** was recorded verbatim (in the conversation, not this doc — the doc captures only the locked-in decision).

If a future decision needs to be relitigated, the original options and reasoning are preserved in each decision's prose above. Look for the "Reasons:" or "Why:" sections to understand the tradeoff space.

No decision was "filled in" by the assistant without the user's confirmation. Every D-numbered section represents a confirmed choice.
