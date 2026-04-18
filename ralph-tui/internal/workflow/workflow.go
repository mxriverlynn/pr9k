package workflow

import (
	"bufio"
	"bytes"
	"fmt"
	"io"
	"os"
	"os/exec"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/mxriverlynn/pr9k/ralph-tui/internal/claudestream"
	"github.com/mxriverlynn/pr9k/ralph-tui/internal/logger"
	"github.com/mxriverlynn/pr9k/ralph-tui/internal/sandbox"
	"github.com/mxriverlynn/pr9k/ralph-tui/internal/ui"
)

// terminateGracePeriod is the time Terminate() waits after SIGTERM before
// escalating to SIGKILL (or calling the terminator with SIGKILL).
const terminateGracePeriod = 3 * time.Second

// Runner executes workflow steps and forwards subprocess output to the TUI via
// a caller-supplied sendLine callback. The io.Pipe from the earlier architecture
// has been replaced by the sendLine callback path: each scanned line calls
// sendLine directly, which allows the TUI's drain goroutine to batch lines into
// LogLinesMsg messages without going through a pipe EOF dance.
type Runner struct {
	mu         sync.Mutex
	log        *logger.Logger
	projectDir string
	sendLine   func(string) // callback invoked for every forwarded line; never nil

	// processMu guards currentProc, currentTerminator, procDone, and terminated.
	processMu         sync.Mutex
	currentProc       *os.Process
	currentTerminator func(syscall.Signal) error
	procDone          chan struct{}
	terminated        bool // set by Terminate(), reset at start of RunStep/RunSandboxedStep

	// terminateGraceOverride, when non-zero, replaces terminateGracePeriod in
	// Terminate(). Used in tests to avoid waiting the full 3 seconds.
	terminateGraceOverride time.Duration

	// lastCapture holds the last non-empty stdout line from the most recent
	// successful RunStep call. Empty string if the last step failed or produced
	// no non-empty stdout lines.
	lastCapture string

	// lastStats holds the StepStats from the most recent RunSandboxedStep call
	// that used the claudestream pipeline. Reset on each RunSandboxedStep entry.
	lastStats claudestream.StepStats

	// activePipeline is the claudestream.Pipeline for the currently running
	// claude step. Protected by processMu. Nil when no claude step is in flight.
	// Read by HeartbeatSilence() to compute the silence duration for D23.
	activePipeline          *claudestream.Pipeline
	activePipelineStartedAt time.Time
}

// Compile-time assertion that *Runner satisfies ui.HeartbeatReader.
var _ ui.HeartbeatReader = (*Runner)(nil)

// SandboxOptions carries the sandbox-specific parameters for RunSandboxedStep.
type SandboxOptions struct {
	// Terminator, when non-nil, is called by Runner.Terminate() instead of
	// signaling the host process directly. It receives SIGTERM first; if the
	// process does not exit within the grace period, it receives SIGKILL.
	Terminator func(syscall.Signal) error
	// CidfilePath is the path of the Docker --cidfile to clean up after the
	// step exits. May be empty. Cleanup is ENOENT-tolerant.
	CidfilePath string
	// ArtifactPath is the path for the per-step .jsonl file (D14). When
	// non-empty and CaptureMode == CaptureResult, a RawWriter is opened here
	// to persist verbatim NDJSON output. Callers set this from
	// ui.ResolvedStep.ArtifactPath.
	ArtifactPath string
	// CaptureMode selects the capture semantics for the step. CaptureResult
	// activates the claudestream pipeline (D6). Zero value (CaptureLastLine)
	// preserves current non-pipeline behaviour.
	CaptureMode ui.CaptureMode
}

// NewRunner creates a Runner that streams subprocess output through the sendLine
// callback (set via SetSender) and to the file logger. projectDir is set as
// cmd.Dir for every subprocess and must be the target repository being operated
// on, not the install dir where ralph-tui's bundled ralph-steps.json, scripts/,
// and prompts/ live.
//
// NewRunner initializes sendLine to a sentinel that panics with a descriptive
// message so that missing-wire bugs (forgetting to call SetSender before
// RunStep) fail loudly rather than silently dropping all output.
func NewRunner(log *logger.Logger, projectDir string) *Runner {
	return &Runner{
		log:        log,
		projectDir: projectDir,
		sendLine: func(string) {
			panic("workflow.Runner: sendLine not set — call SetSender before running steps")
		},
	}
}

// ProjectDir returns the target repository directory set for this runner.
// Subprocesses run with cmd.Dir set to this path.
func (r *Runner) ProjectDir() string {
	return r.projectDir
}

// SetSender installs a callback that is invoked for every line forwarded
// through forwardPipe and WriteToLog. If send is nil, a no-op is installed
// so callers can safely clear the sender between test cases.
//
// The callback must not panic and must not block. It is called synchronously
// inside scanner goroutines; a blocking callback stalls subprocess output,
// and a panicking callback crashes the process.
func (r *Runner) SetSender(send func(string)) {
	if send == nil {
		send = func(string) {}
	}
	r.mu.Lock()
	r.sendLine = send
	r.mu.Unlock()
}

// WasTerminated reports whether the most recent RunStep was ended by a
// Terminate() call (user-initiated skip). Returns false once the next
// RunStep begins (the flag is reset at the start of each run).
func (r *Runner) WasTerminated() bool {
	r.processMu.Lock()
	defer r.processMu.Unlock()
	return r.terminated
}

// Terminate sends SIGTERM to the currently running subprocess (or invokes the
// installed terminator for sandboxed steps). If the process has not exited
// within the grace period, SIGKILL is sent. Safe to call when no subprocess is
// running (it is a no-op in that case). Keyboard handlers use this to skip a
// step or quit cleanly.
func (r *Runner) Terminate() {
	r.processMu.Lock()
	proc := r.currentProc
	term := r.currentTerminator
	done := r.procDone
	r.terminated = true
	r.processMu.Unlock()

	if proc == nil {
		return
	}

	grace := terminateGracePeriod
	if r.terminateGraceOverride > 0 {
		grace = r.terminateGraceOverride
	}

	if term != nil {
		_ = term(syscall.SIGTERM)
		select {
		case <-done:
		case <-time.After(grace):
			_ = term(syscall.SIGKILL)
		}
	} else {
		_ = proc.Signal(syscall.SIGTERM)
		select {
		case <-done:
		case <-time.After(grace):
			_ = proc.Kill()
		}
	}
}

// RunStep executes command as a subprocess and streams its stdout and stderr in
// real-time to both sendLine and the file logger. Stdout is also captured into
// an in-memory buffer; after the command succeeds, the last non-empty stdout
// line is stored and retrievable via LastCapture. On failure, LastCapture is
// set to "". A WaitGroup ensures both pipes are fully drained before cmd.Wait()
// is called.
//
// RunStep returns an error if command is empty rather than panicking, so callers
// that build commands dynamically get a clear failure instead of a runtime panic.
func (r *Runner) RunStep(stepName string, command []string) error {
	return r.RunStepFull(stepName, command, ui.CaptureLastLine)
}

// RunStepFull is like RunStep but accepts an explicit captureMode.
// Use CaptureFullStdout to bind the complete stdout payload (up to 32 KiB)
// instead of only the last non-empty line.
func (r *Runner) RunStepFull(stepName string, command []string, captureMode ui.CaptureMode) error {
	if len(command) == 0 {
		return fmt.Errorf("workflow: RunStep %q: empty command", stepName)
	}

	r.processMu.Lock()
	r.terminated = false
	r.processMu.Unlock()

	return r.runCommand(stepName, command, nil, nil, nil, captureMode)
}

// RunSandboxedStep executes command inside a Docker sandbox. An explicit empty
// stdin (bytes.NewReader(nil)) is used so that docker does not inherit the
// parent's raw-mode keyboard reader. The cidfile at opts.CidfilePath is removed
// after the step exits (ENOENT-tolerant).
//
// When opts.CaptureMode == CaptureResult, a claudestream.Pipeline is
// constructed and passed to runCommand to handle NDJSON parsing, rendering, and
// JSONL persistence (D14). After the subprocess exits, the aggregator is
// consulted for errors (D15) and the finalize summary line is emitted (D13 2a).
//
// Terminator construction order: opts.Terminator takes precedence when set
// (test-injection path). When opts.Terminator is nil, runCommand constructs a
// terminator closure via sandbox.NewTerminator(cmd, opts.CidfilePath) after
// the *exec.Cmd exists but before cmd.Start() — resolving the construction-
// ordering constraint (the terminator needs the cmd pointer).
func (r *Runner) RunSandboxedStep(stepName string, command []string, opts SandboxOptions) error {
	if len(command) == 0 {
		return fmt.Errorf("workflow: RunSandboxedStep %q: empty command", stepName)
	}

	r.processMu.Lock()
	r.terminated = false
	r.processMu.Unlock()

	// Reset lastStats so LastStats() returns a zero value if no pipeline is used.
	r.mu.Lock()
	r.lastStats = claudestream.StepStats{}
	r.mu.Unlock()

	defer func() {
		_ = sandbox.Cleanup(opts.CidfilePath)
	}()

	if opts.CaptureMode != ui.CaptureResult {
		return r.runCommand(stepName, command, bytes.NewReader(nil), &opts, nil, ui.CaptureLastLine)
	}

	// Construct the claudestream pipeline (D14, D15, D6).
	var rw *claudestream.RawWriter
	if opts.ArtifactPath != "" {
		var err error
		rw, err = claudestream.NewRawWriter(opts.ArtifactPath)
		if err != nil {
			// Log the error but continue — JSONL persistence is best-effort; the
			// step should not fail because the artifact directory is missing.
			_ = r.log.Log(stepName, fmt.Sprintf("[artifact] open failed: %v", err))
		}
	}
	pipeline := claudestream.NewPipeline(rw)
	defer func() {
		_ = pipeline.Close()
		// Log any RawWriter write error — persistence is best-effort (see above),
		// but operators need visibility into JSONL artifact failures (M1).
		if wErr := pipeline.WriteErr(); wErr != nil {
			_ = r.log.Log(stepName, fmt.Sprintf("[artifact] write error: %v", wErr))
		}
	}()

	// Register the active pipeline for heartbeat monitoring (D23). The clear
	// defer is registered last so it executes first (LIFO), before pipeline.Close().
	now := time.Now()
	r.processMu.Lock()
	r.activePipeline = pipeline
	r.activePipelineStartedAt = now
	r.processMu.Unlock()
	defer func() {
		r.processMu.Lock()
		r.activePipeline = nil
		r.activePipelineStartedAt = time.Time{}
		r.processMu.Unlock()
	}()

	cmdErr := r.runCommand(stepName, command, bytes.NewReader(nil), &opts, pipeline, ui.CaptureLastLine)

	// Fold stats into lastStats (D21) — always, regardless of error.
	r.mu.Lock()
	r.lastStats = pipeline.Aggregator().Stats()
	r.mu.Unlock()

	// D15: check aggregator error before subprocess exit code.
	if aggErr := pipeline.Aggregator().Err(); aggErr != nil {
		r.mu.Lock()
		r.lastCapture = ""
		r.mu.Unlock()
		return aggErr
	}

	if cmdErr != nil {
		r.mu.Lock()
		r.lastCapture = ""
		r.mu.Unlock()
		return cmdErr
	}

	// D13 2a: emit the per-step summary line through sendLine.
	for _, line := range pipeline.Renderer().Finalize(pipeline.Aggregator().Stats()) {
		r.mu.Lock()
		send := r.sendLine
		r.mu.Unlock()
		send(line)
		_ = r.log.Log(stepName, line)
	}

	// D6: bind result.result to lastCapture.
	r.mu.Lock()
	r.lastCapture = pipeline.Aggregator().Result()
	r.mu.Unlock()

	return nil
}

// runCommand is the shared private core for RunStep and RunSandboxedStep. It
// executes command as a subprocess, streaming stdout and stderr in real-time.
// If stdin is non-nil it is set on the command; otherwise the subprocess
// inherits the parent's stdin (RunStep behaviour). opts, when non-nil, drives
// terminator selection: if opts.Terminator is set it is used directly (test-
// injection path); if opts.Terminator is nil but opts.CidfilePath is non-empty,
// sandbox.NewTerminator is called after cmd is created (so the closure captures
// the correct *exec.Cmd). The currentTerminator field is cleared before the
// procDone channel is closed so that any Terminate() racing with natural step
// completion sees a nil terminator and short-circuits.
//
// When pipeline is non-nil, the claude-aware path is activated (D3, D20, D27):
//   - stdout is forwarded through a bufio.Reader loop (no 256KB line cap) that
//     feeds each raw line to pipeline.Observe and sends rendered display lines
//     via sendLine.
//   - stderr is forwarded with a 256KB bufio.Scanner and each line is prefixed
//     with "[stderr] " before being sent to sendLine and the file logger (D27).
//
// When pipeline is nil, both stdout and stderr use the existing 256KB-scanner
// path unchanged (D9). captureMode controls how lastCapture is set:
//   - CaptureLastLine (zero value): last non-empty stdout line (default).
//   - CaptureFullStdout: all stdout lines joined with "\n", capped at 32 KiB.
//
// lastCapture is NOT set by runCommand when pipeline is non-nil; the caller
// (RunSandboxedStep) sets it after inspecting the aggregator.
func (r *Runner) runCommand(stepName string, command []string, stdin io.Reader, opts *SandboxOptions, pipeline *claudestream.Pipeline, captureMode ui.CaptureMode) error {
	cmd := exec.Command(command[0], command[1:]...)
	cmd.Dir = r.projectDir
	if stdin != nil {
		cmd.Stdin = stdin
	}

	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return fmt.Errorf("workflow: stdout pipe: %w", err)
	}
	stderr, err := cmd.StderrPipe()
	if err != nil {
		return fmt.Errorf("workflow: stderr pipe: %w", err)
	}

	// Determine the terminator to install. For sandboxed steps, construct it
	// here — after cmd exists — so the closure captures the correct *exec.Cmd.
	var terminator func(syscall.Signal) error
	if opts != nil {
		if opts.Terminator != nil {
			terminator = opts.Terminator
		} else if opts.CidfilePath != "" {
			terminator = sandbox.NewTerminator(cmd, opts.CidfilePath)
		}
	}

	if err := cmd.Start(); err != nil {
		return fmt.Errorf("workflow: start %q: %w", command[0], err)
	}

	done := make(chan struct{})
	r.processMu.Lock()
	r.currentProc = cmd.Process
	r.currentTerminator = terminator
	r.procDone = done
	r.processMu.Unlock()
	defer func() {
		// Clear terminator BEFORE closing done so that any Terminate() racing
		// with natural step completion observes a nil terminator and returns
		// without dispatching a stale signal (iteration 6 F2).
		r.processMu.Lock()
		r.currentTerminator = nil
		r.processMu.Unlock()
		close(done)
		r.processMu.Lock()
		r.currentProc = nil
		r.processMu.Unlock()
	}()

	var wg sync.WaitGroup
	wg.Add(2)

	if pipeline != nil {
		// Claude-aware stdout forwarder (D3, D20): uses bufio.Reader to avoid
		// the 256KB line cap that would truncate large tool_result payloads.
		const maxLineBytes = 64 * 1024 * 1024 // 64MB hard safety cap
		go func() {
			defer wg.Done()
			br := bufio.NewReader(stdout)
			var logErr error
			for {
				line, err := br.ReadString('\n')
				if len(line) > 0 {
					// Trim the trailing newline for display/logging, but keep
					// raw bytes verbatim for pipeline.Observe (which writes to RawWriter).
					raw := []byte(strings.TrimRight(line, "\n"))
					if len(raw) > maxLineBytes {
						// Safety cap: write a truncation sentinel and warn.
						sentinel := fmt.Sprintf(`{"type":"ralph_truncation_marker","reason":"line_too_long","bytes":%d}`, len(raw))
						if logErr == nil {
							logErr = r.log.Log(stepName, fmt.Sprintf("[truncated line: %d bytes]", len(raw)))
						}
						_ = pipeline.Observe([]byte(sentinel))
					} else {
						for _, display := range pipeline.Observe(raw) {
							if logErr == nil {
								logErr = r.log.Log(stepName, display)
								if logErr != nil {
									_ = r.log.Log(stepName, fmt.Sprintf("logger error: %v", logErr))
								}
							}
							r.mu.Lock()
							send := r.sendLine
							r.mu.Unlock()
							send(display)
						}
					}
				}
				if err != nil {
					if err != io.EOF {
						_ = r.log.Log(stepName, fmt.Sprintf("stdout read error: %v", err))
					}
					break
				}
			}
		}()

		// Claude-aware stderr forwarder (D20, D27): 256KB scanner, [stderr] prefix.
		go func() {
			defer wg.Done()
			scanner := bufio.NewScanner(stderr)
			buf := make([]byte, 256*1024)
			scanner.Buffer(buf, 256*1024)
			var logErr error
			for scanner.Scan() {
				line := "[stderr] " + scanner.Text()
				if logErr == nil {
					logErr = r.log.Log(stepName, line)
					if logErr != nil {
						_ = r.log.Log(stepName, fmt.Sprintf("logger error: %v", logErr))
					}
				}
				r.mu.Lock()
				send := r.sendLine
				r.mu.Unlock()
				send(line)
			}
			if err := scanner.Err(); err != nil {
				_ = r.log.Log(stepName, fmt.Sprintf("stderr scanner error: %v", err))
			}
		}()
	} else {
		// Non-claude path: original dual 256KB-scanner forwarder (D9).
		// capturedLines accumulates stdout lines for lastCapture. Written only by
		// the stdout goroutine; read only after wg.Wait(), so no mutex is needed.
		var capturedLines []string

		forwardPipe := func(pipe interface{ Read([]byte) (int, error) }, capture bool) {
			defer wg.Done()
			scanner := bufio.NewScanner(pipe)
			buf := make([]byte, 256*1024)
			scanner.Buffer(buf, 256*1024)
			var logErr error
			for scanner.Scan() {
				line := scanner.Text()
				if capture {
					capturedLines = append(capturedLines, line)
				}
				if logErr == nil {
					logErr = r.log.Log(stepName, line)
					if logErr != nil {
						_ = r.log.Log(stepName, fmt.Sprintf("logger error: %v", logErr))
					}
				}
				// Snapshot sendLine outside r.mu is safe: program.Send is
				// goroutine-safe and the channel adapter never blocks.
				r.mu.Lock()
				send := r.sendLine
				r.mu.Unlock()
				send(line)
			}
			if err := scanner.Err(); err != nil {
				_ = r.log.Log(stepName, fmt.Sprintf("scanner error: %v", err))
			}
		}

		go forwardPipe(stdout, true)
		go forwardPipe(stderr, false)

		wg.Wait()
		waitErr := cmd.Wait()

		r.mu.Lock()
		if waitErr == nil {
			if captureMode == ui.CaptureFullStdout {
				r.lastCapture = fullStdoutCapture(capturedLines)
			} else {
				r.lastCapture = lastNonEmptyLine(capturedLines)
			}
		} else {
			r.lastCapture = ""
		}
		r.mu.Unlock()

		return waitErr
	}

	wg.Wait()
	return cmd.Wait()
}

// LastCapture returns the last non-empty stdout line from the most recent
// successful RunStep call, stripped of trailing carriage returns and whitespace.
// Returns "" if the last step failed or produced no non-empty stdout output.
func (r *Runner) LastCapture() string {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.lastCapture
}

// LastStats returns the StepStats from the most recent RunSandboxedStep call
// that used the claudestream pipeline. Returns a zero value if no such call has
// occurred. The dispatcher calls this immediately after each RunSandboxedStep
// returns to fold the stats into RunStats (D21).
func (r *Runner) LastStats() claudestream.StepStats {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.lastStats
}

// HeartbeatSilence returns the duration since the last observed event from
// the active claude pipeline, and whether a claude step is currently running.
// Returns (0, false) when no pipeline is active.
//
// When a step just started but no event has arrived yet (pipeline.LastEventAt()
// is zero), the duration is measured from the moment the pipeline was created
// so the heartbeat begins counting immediately rather than showing stale zero.
//
// Implements ui.HeartbeatReader. Safe for concurrent use: reads are taken
// under processMu, then released before computing time.Since.
func (r *Runner) HeartbeatSilence() (time.Duration, bool) {
	r.processMu.Lock()
	pipeline := r.activePipeline
	startedAt := r.activePipelineStartedAt
	r.processMu.Unlock()

	if pipeline == nil {
		return 0, false
	}
	t := pipeline.LastEventAt()
	if t.IsZero() {
		// No events observed yet — count silence from pipeline creation.
		return time.Since(startedAt), true
	}
	return time.Since(t), true
}

// lastNonEmptyLine walks lines in reverse, strips trailing \r and whitespace,
// and returns the first non-empty line found. Returns "" if all lines are empty.
func lastNonEmptyLine(lines []string) string {
	for i := len(lines) - 1; i >= 0; i-- {
		line := strings.TrimSpace(strings.TrimRight(lines[i], "\r"))
		if line != "" {
			return line
		}
	}
	return ""
}

// fullStdoutCapture joins lines with "\n". If the result exceeds 32 KiB, the
// first 30 KiB are kept verbatim and a truncation marker is appended so callers
// can detect the truncation. Returns "" when lines is empty.
func fullStdoutCapture(lines []string) string {
	const hardCap = 32 * 1024
	const keepBytes = 30 * 1024
	joined := strings.Join(lines, "\n")
	if len(joined) > hardCap {
		return joined[:keepBytes] + "\n[...truncated, full content exceeds 32 KiB]"
	}
	return joined
}

// WriteToLog writes a single line directly via the sendLine callback, without
// running a subprocess. Use this to inject separator lines between subprocess
// outputs without running a command.
func (r *Runner) WriteToLog(line string) {
	r.mu.Lock()
	send := r.sendLine
	r.mu.Unlock()
	send(line)
}

// WriteRunSummary writes a single line to both the TUI (via sendLine) and the
// file logger. Use this for the run-level cumulative summary (D13 2c) so the
// line is visible in the TUI and persisted to disk — unlike WriteToLog, which
// only sends to the TUI.
func (r *Runner) WriteRunSummary(line string) {
	r.mu.Lock()
	send := r.sendLine
	r.mu.Unlock()
	send(line)
	if err := r.log.Log("run summary", line); err != nil {
		send(fmt.Sprintf("[log] run summary write failed: %v", err))
	}
}

// CaptureOutput runs command in projectDir and returns its trimmed stdout.
// Stderr is discarded. Use this for commands that return a single value
// (e.g., get_next_issue, get_gh_user, git rev-parse HEAD).
func (r *Runner) CaptureOutput(command []string) (string, error) {
	if len(command) == 0 {
		return "", fmt.Errorf("workflow: CaptureOutput: empty command")
	}
	cmd := exec.Command(command[0], command[1:]...)
	cmd.Dir = r.projectDir
	out, err := cmd.Output()
	return strings.TrimSpace(string(out)), err
}
