package workflow

import (
	"bufio"
	"bytes"
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
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

	// processMu guards currentProc, procDone, and terminated.
	processMu   sync.Mutex
	currentProc *os.Process
	procDone    chan struct{}
	terminated  bool // set by Terminate(), reset at start of RunStep
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
	}
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
// real-time to both the pipe and the file logger. A WaitGroup ensures both
// pipes are fully drained before cmd.Wait() is called. Writes to the shared
// PipeWriter are mutex-protected because io.PipeWriter is not safe for
// concurrent use.
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

	forward := func(pipe io.Reader) {
		defer wg.Done()
		scanner := bufio.NewScanner(pipe)
		buf := make([]byte, 256*1024)
		scanner.Buffer(buf, 256*1024)
		var logErr error
		for scanner.Scan() {
			line := scanner.Text()
			if logErr == nil {
				logErr = r.log.Log(stepName, line)
				if logErr != nil {
					_ = r.log.Log(stepName, fmt.Sprintf("logger error: %v", logErr))
				}
			}
			r.mu.Lock()
			_, _ = fmt.Fprintln(r.logWriter, line)
			r.mu.Unlock()
		}
		if err := scanner.Err(); err != nil {
			_ = r.log.Log(stepName, fmt.Sprintf("scanner error: %v", err))
		}
	}

	go forward(stdout)
	go forward(stderr)

	wg.Wait()
	return cmd.Wait()
}

// WriteToLog writes a single line directly to the log pipe. Use this to
// inject separator lines between subprocess outputs without running a command.
func (r *Runner) WriteToLog(line string) {
	r.mu.Lock()
	_, _ = fmt.Fprintln(r.logWriter, line)
	r.mu.Unlock()
}

// Close closes the logWriter, sending EOF to the reader. Call this after all
// steps complete.
func (r *Runner) Close() error {
	return r.logWriter.Close()
}

// CaptureOutput runs command in dir and returns its trimmed stdout. Stderr is
// captured and included in the error message on non-zero exit, but is never
// returned as output. This function does not stream to the TUI.
func CaptureOutput(ctx context.Context, command []string, dir string) (string, error) {
	cmd := exec.CommandContext(ctx, command[0], command[1:]...)
	cmd.Dir = dir
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		return "", fmt.Errorf("workflow: capture %q: %w\nstderr: %s", command[0], err, stderr.String())
	}
	return strings.TrimSpace(stdout.String()), nil
}

// CaptureOutput runs command in Runner's workingDir and returns its trimmed
// stdout. It delegates to the package-level CaptureOutput with a background
// context. Use this for commands that return a single value (e.g.,
// get_next_issue, get_gh_user, git rev-parse HEAD).
func (r *Runner) CaptureOutput(command []string) (string, error) {
	return CaptureOutput(context.Background(), command, r.workingDir)
}

// ResolveCommand replaces template variables in command and resolves relative
// script paths against projectDir.
//
// For each element:
//   - All occurrences of "{{ISSUE_ID}}" are replaced with issueID.
//   - The first element (the executable) is resolved relative to projectDir if
//     it is a relative path containing a path separator (i.e. not a bare
//     command like "git").
func ResolveCommand(projectDir string, command []string, issueID string) []string {
	if len(command) == 0 {
		return command
	}

	result := make([]string, len(command))
	for i, arg := range command {
		result[i] = strings.ReplaceAll(arg, "{{ISSUE_ID}}", issueID)
	}

	// Resolve the executable if it looks like a relative script path.
	exe := result[0]
	if !filepath.IsAbs(exe) && strings.ContainsRune(exe, '/') {
		result[0] = filepath.Join(projectDir, exe)
	}

	return result
}
