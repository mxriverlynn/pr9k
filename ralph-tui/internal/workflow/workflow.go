package workflow

import (
	"bufio"
	"fmt"
	"io"
	"os"
	"os/exec"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/mxriverlynn/pr9k/ralph-tui/internal/logger"
)

// Runner executes workflow steps and streams subprocess output through an io.Pipe.
// The read end of the pipe is passed to the UI component; the write end receives
// forwarded stdout/stderr from each subprocess.
type Runner struct {
	logReader  *io.PipeReader
	logWriter  *io.PipeWriter
	mu         sync.Mutex
	log        *logger.Logger
	workingDir string
	sendLine   func(string) // callback invoked for every forwarded line; never nil

	// processMu guards currentProc, procDone, and terminated.
	processMu   sync.Mutex
	currentProc *os.Process
	procDone    chan struct{}
	terminated  bool // set by Terminate(), reset at start of RunStep

	// lastCapture holds the last non-empty stdout line from the most recent
	// successful RunStep call. Empty string if the last step failed or produced
	// no non-empty stdout lines.
	lastCapture string
}

// NewRunner creates a Runner that streams subprocess output to log and through
// an io.Pipe. workingDir is set as cmd.Dir for every subprocess.
func NewRunner(log *logger.Logger, workingDir string) *Runner {
	r, w := io.Pipe()
	return &Runner{
		logReader:  r,
		logWriter:  w,
		log:        log,
		workingDir: workingDir,
		sendLine:   func(string) {},
	}
}

// SetSender installs a callback that is invoked for every line forwarded
// through forwardPipe and WriteToLog. If send is nil, a no-op is installed
// so callers can safely clear the sender between test cases.
func (r *Runner) SetSender(send func(string)) {
	if send == nil {
		send = func(string) {}
	}
	r.mu.Lock()
	r.sendLine = send
	r.mu.Unlock()
}

// LogReader returns the read end of the pipe. Pass this to the UI log component
// for real-time display.
func (r *Runner) LogReader() *io.PipeReader {
	return r.logReader
}

// WasTerminated reports whether the most recent RunStep was ended by a
// Terminate() call (user-initiated skip). Returns false once the next
// RunStep begins (the flag is reset at the start of each run).
func (r *Runner) WasTerminated() bool {
	r.processMu.Lock()
	defer r.processMu.Unlock()
	return r.terminated
}

// Terminate sends SIGTERM to the currently running subprocess. If the process
// has not exited within 3 seconds, SIGKILL is sent. Safe to call when no
// subprocess is running (it is a no-op in that case). Keyboard handlers use
// this to skip a step or quit cleanly.
func (r *Runner) Terminate() {
	r.processMu.Lock()
	proc := r.currentProc
	done := r.procDone
	r.terminated = true
	r.processMu.Unlock()

	if proc == nil {
		return
	}

	_ = proc.Signal(syscall.SIGTERM)

	select {
	case <-done:
	case <-time.After(3 * time.Second):
		_ = proc.Kill()
	}
}

// RunStep executes command as a subprocess and streams its stdout and stderr in
// real-time to both the pipe and the file logger. Stdout is also captured into
// an in-memory buffer; after the command succeeds, the last non-empty stdout
// line is stored and retrievable via LastCapture. On failure, LastCapture is
// set to "". A WaitGroup ensures both pipes are fully drained before cmd.Wait()
// is called. Writes to the shared PipeWriter are mutex-protected because
// io.PipeWriter is not safe for concurrent use.
func (r *Runner) RunStep(stepName string, command []string) error {
	r.processMu.Lock()
	r.terminated = false
	r.processMu.Unlock()

	cmd := exec.Command(command[0], command[1:]...)
	cmd.Dir = r.workingDir

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

	forwardPipe := func(pipe io.Reader, capture bool) {
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
			r.mu.Lock()
			_, _ = fmt.Fprintln(r.logWriter, line)
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

	if waitErr == nil {
		r.lastCapture = lastNonEmptyLine(capturedLines)
	} else {
		r.lastCapture = ""
	}

	return waitErr
}

// LastCapture returns the last non-empty stdout line from the most recent
// successful RunStep call, stripped of trailing carriage returns and whitespace.
// Returns "" if the last step failed or produced no non-empty stdout output.
func (r *Runner) LastCapture() string {
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

// WriteToLog writes a single line directly to the log pipe. Use this to
// inject separator lines between subprocess outputs without running a command.
func (r *Runner) WriteToLog(line string) {
	r.mu.Lock()
	_, _ = fmt.Fprintln(r.logWriter, line)
	send := r.sendLine
	r.mu.Unlock()
	send(line)
}

// Close closes the logWriter, sending EOF to the reader. Call this after all
// steps complete.
func (r *Runner) Close() error {
	return r.logWriter.Close()
}

// CaptureOutput runs command in workingDir and returns its trimmed stdout.
// Stderr is discarded. Use this for commands that return a single value
// (e.g., get_next_issue, get_gh_user, git rev-parse HEAD).
func (r *Runner) CaptureOutput(command []string) (string, error) {
	cmd := exec.Command(command[0], command[1:]...)
	cmd.Dir = r.workingDir
	out, err := cmd.Output()
	return strings.TrimSpace(string(out)), err
}
