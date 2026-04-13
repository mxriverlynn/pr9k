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

	"github.com/mxriverlynn/pr9k/ralph-tui/internal/logger"
	"github.com/mxriverlynn/pr9k/ralph-tui/internal/sandbox"
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
}

// SandboxOptions carries the sandbox-specific parameters for RunSandboxedStep.
type SandboxOptions struct {
	// Terminator, when non-nil, is called by Runner.Terminate() instead of
	// signaling the host process directly. It receives SIGTERM first; if the
	// process does not exit within the grace period, it receives SIGKILL.
	Terminator func(syscall.Signal) error
	// CidfilePath is the path of the Docker --cidfile to clean up after the
	// step exits. May be empty. Cleanup is ENOENT-tolerant.
	CidfilePath string
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
	if len(command) == 0 {
		return fmt.Errorf("workflow: RunStep %q: empty command", stepName)
	}

	r.processMu.Lock()
	r.terminated = false
	r.processMu.Unlock()

	return r.runCommand(stepName, command, nil)
}

// RunSandboxedStep executes command inside a Docker sandbox. It installs
// opts.Terminator so that Terminate() dispatches signals through the container
// rather than the host docker CLI process directly. An explicit empty stdin
// (bytes.NewReader(nil)) is used so that docker does not inherit the parent's
// raw-mode keyboard reader. The cidfile at opts.CidfilePath is removed after
// the step exits (ENOENT-tolerant).
func (r *Runner) RunSandboxedStep(stepName string, command []string, opts SandboxOptions) error {
	if len(command) == 0 {
		return fmt.Errorf("workflow: RunSandboxedStep %q: empty command", stepName)
	}

	r.processMu.Lock()
	r.terminated = false
	r.currentTerminator = opts.Terminator
	r.processMu.Unlock()

	defer func() {
		_ = sandbox.Cleanup(opts.CidfilePath)
	}()

	return r.runCommand(stepName, command, bytes.NewReader(nil))
}

// runCommand is the shared private core for RunStep and RunSandboxedStep. It
// executes command as a subprocess, streaming stdout and stderr in real-time.
// If stdin is non-nil it is set on the command; otherwise the subprocess
// inherits the parent's stdin (RunStep behaviour). The currentTerminator field
// is cleared before the procDone channel is closed so that any Terminate()
// racing with natural step completion sees a nil terminator and short-circuits.
func (r *Runner) runCommand(stepName string, command []string, stdin io.Reader) error {
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

	if err := cmd.Start(); err != nil {
		return fmt.Errorf("workflow: start %q: %w", command[0], err)
	}

	done := make(chan struct{})
	r.processMu.Lock()
	r.currentProc = cmd.Process
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
		r.lastCapture = lastNonEmptyLine(capturedLines)
	} else {
		r.lastCapture = ""
	}
	r.mu.Unlock()

	return waitErr
}

// LastCapture returns the last non-empty stdout line from the most recent
// successful RunStep call, stripped of trailing carriage returns and whitespace.
// Returns "" if the last step failed or produced no non-empty stdout output.
func (r *Runner) LastCapture() string {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.lastCapture
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

// WriteToLog writes a single line directly via the sendLine callback, without
// running a subprocess. Use this to inject separator lines between subprocess
// outputs without running a command.
func (r *Runner) WriteToLog(line string) {
	r.mu.Lock()
	send := r.sendLine
	r.mu.Unlock()
	send(line)
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
