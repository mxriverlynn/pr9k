package statusline

import (
	"bytes"
	"context"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"syscall"
	"time"

	"github.com/mxriverlynn/pr9k/ralph-tui/internal/logger"
)

// DefaultRefreshInterval is applied when cfg.RefreshIntervalSeconds is nil.
const DefaultRefreshInterval = 5 * time.Second

// Config holds the parsed statusLine block from ralph-steps.json.
// Callers construct this from steps.StatusLineConfig before calling New.
type Config struct {
	// Command is the resolved path or bare name of the script to execute.
	Command string
	// RefreshIntervalSeconds controls the timer interval. nil → default (5 s);
	// pointer to 0 → timer disabled; pointer to N → N-second interval.
	RefreshIntervalSeconds *int
}

// stdoutLimit caps the bytes read from the script's stdout.
const stdoutLimit = 8 * 1024

// StatusLineUpdatedMsg is sent to the Bubble Tea program after the cache is
// updated. Injected via SetSender to avoid importing bubbletea here.
type StatusLineUpdatedMsg struct{}

// Runner executes the status-line command, caches the output, and notifies
// the TUI on updates. All exported methods are goroutine-safe. A no-op Runner
// (Enabled() == false) ignores all method calls without panicking or starting
// any goroutines.
type Runner struct {
	enabled    bool
	path       string
	projectDir string
	interval   time.Duration
	log        *logger.Logger
	sender     func(interface{})
	modeGetter func() string

	triggerCh chan struct{}

	stateMu sync.Mutex
	state   State

	outputMu   sync.Mutex
	lastOutput string
	hasOutput  bool

	running atomic.Bool
	stopped atomic.Bool

	cancel context.CancelFunc
	wg     sync.WaitGroup
}

// New constructs a Runner from cfg. workflowDir is used to resolve relative
// command paths; projectDir sets the script's working directory. log must
// outlive the runner (see Shutdown). Returns a no-op runner if cfg is nil or
// its Command cannot be resolved.
func New(cfg *Config, workflowDir, projectDir string, log *logger.Logger) *Runner {
	if cfg == nil {
		return NewNoOp()
	}

	path := resolvePath(workflowDir, cfg.Command)
	if path == "" {
		return NewNoOp()
	}

	var interval time.Duration
	switch {
	case cfg.RefreshIntervalSeconds == nil:
		interval = DefaultRefreshInterval
	case *cfg.RefreshIntervalSeconds == 0:
		interval = 0 // timer disabled
	default:
		interval = time.Duration(*cfg.RefreshIntervalSeconds) * time.Second
	}

	return &Runner{
		enabled:    true,
		path:       path,
		projectDir: projectDir,
		interval:   interval,
		log:        log,
		triggerCh:  make(chan struct{}, 4),
	}
}

// NewNoOp returns a disabled Runner whose methods are all safe no-ops.
// Used when statusLine is absent from ralph-steps.json.
func NewNoOp() *Runner {
	return &Runner{enabled: false, triggerCh: make(chan struct{}, 4)}
}

// Enabled reports whether the runner has a configured command.
func (r *Runner) Enabled() bool { return r.enabled }

// SetSender injects the callback that notifies the Bubble Tea program.
// fn is called with a StatusLineUpdatedMsg after each successful cache update.
func (r *Runner) SetSender(fn func(interface{})) {
	if !r.enabled {
		return
	}
	r.sender = fn
}

// SetModeGetter injects the function the worker calls to read the current UI
// mode string at invocation time. If unset, the payload mode field is "".
func (r *Runner) SetModeGetter(fn func() string) {
	if !r.enabled {
		return
	}
	r.modeGetter = fn
}

// PushState stores the latest workflow state snapshot. Call this on the
// workflow goroutine immediately before Trigger.
func (r *Runner) PushState(s State) {
	if !r.enabled {
		return
	}
	r.stateMu.Lock()
	r.state = s
	r.stateMu.Unlock()
}

// Trigger enqueues a refresh. Drops silently when the channel is full.
func (r *Runner) Trigger() {
	if !r.enabled {
		return
	}
	select {
	case r.triggerCh <- struct{}{}:
	default:
	}
}

// LastOutput returns the cached output from the last successful run, or ""
// when no successful run has occurred.
func (r *Runner) LastOutput() string {
	r.outputMu.Lock()
	defer r.outputMu.Unlock()
	return r.lastOutput
}

// HasOutput reports whether at least one exit-0 run has populated the cache.
// False during cold-start; remains false after a failing first run.
func (r *Runner) HasOutput() bool {
	r.outputMu.Lock()
	defer r.outputMu.Unlock()
	return r.hasOutput
}

// Start launches the worker goroutine and, when interval > 0, the timer
// goroutine. ctx is the parent context; the runner derives its own child.
func (r *Runner) Start(ctx context.Context) {
	if !r.enabled {
		return
	}
	ctx, r.cancel = context.WithCancel(ctx)

	r.wg.Add(1)
	go r.worker(ctx)

	if r.interval > 0 {
		r.wg.Add(1)
		go r.timerLoop(ctx)
	}
}

// Shutdown cancels the worker context, marks the runner as stopped (so
// in-flight runs do not call the sender after this returns), and waits up to
// 2 s for goroutines to drain.
func (r *Runner) Shutdown() {
	if !r.enabled {
		return
	}
	r.stopped.Store(true)
	if r.cancel != nil {
		r.cancel()
	}
	done := make(chan struct{})
	go func() {
		r.wg.Wait()
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(2 * time.Second):
	}
}

// worker drains triggerCh and executes the script for each trigger.
func (r *Runner) worker(ctx context.Context) {
	defer r.wg.Done()
	for {
		select {
		case <-ctx.Done():
			return
		case <-r.triggerCh:
			r.execScript(ctx)
		}
	}
}

// timerLoop fires into triggerCh at r.interval; drops when full.
func (r *Runner) timerLoop(ctx context.Context) {
	defer r.wg.Done()
	ticker := time.NewTicker(r.interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			select {
			case r.triggerCh <- struct{}{}:
			default:
			}
		}
	}
}

// execScript runs the configured command once. It is a no-op when another
// invocation is already in flight (single-flight via atomic CAS).
func (r *Runner) execScript(ctx context.Context) {
	if !r.running.CompareAndSwap(false, true) {
		return
	}
	defer r.running.Store(false)

	s := r.snapshotState()
	mode := ""
	if r.modeGetter != nil {
		mode = r.modeGetter()
	}

	payload, err := BuildPayload(s, mode)
	if err != nil {
		r.logLine("BuildPayload error: " + err.Error())
		return
	}

	start := time.Now()
	cmdCtx, cancel := context.WithTimeout(ctx, 2*time.Second)
	defer cancel()

	cmd := exec.CommandContext(cmdCtx, r.path)
	cmd.Cancel = func() error { return cmd.Process.Signal(syscall.SIGTERM) }
	cmd.WaitDelay = 1 * time.Second
	cmd.Stdin = bytes.NewReader(payload)
	cmd.Dir = r.projectDir
	cmd.Env = os.Environ()

	stdoutPipe, err := cmd.StdoutPipe()
	if err != nil {
		r.logLine("stdout pipe error: " + err.Error())
		return
	}
	stderrPipe, err := cmd.StderrPipe()
	if err != nil {
		r.logLine("stderr pipe error: " + err.Error())
		return
	}

	if err := cmd.Start(); err != nil {
		r.logLine("start error: " + err.Error())
		return
	}

	// Read stdout up to 8 KB.
	limited := io.LimitReader(stdoutPipe, stdoutLimit+1)
	rawOut, readErr := io.ReadAll(limited)
	truncated := len(rawOut) > stdoutLimit
	if truncated {
		rawOut = rawOut[:stdoutLimit]
	}

	// Read stderr concurrently with stdout drain.
	stderrBytes, _ := io.ReadAll(stderrPipe)

	runErr := cmd.Wait()
	dur := time.Since(start)

	if len(stderrBytes) > 0 {
		for _, line := range strings.Split(strings.TrimRight(string(stderrBytes), "\n"), "\n") {
			if line != "" {
				r.logLine("stderr: " + line)
			}
		}
	}
	if readErr != nil {
		r.logLine("stdout read error: " + readErr.Error())
	}
	if truncated {
		r.logLine("stdout truncated at 8 KB")
	}

	if runErr != nil {
		r.logLine("exit error (duration " + dur.String() + "): " + runErr.Error())
		return
	}

	// Exit 0: extract first non-empty line and sanitize.
	firstLine := firstNonEmptyLine(rawOut)
	sanitized := Sanitize([]byte(firstLine))

	r.outputMu.Lock()
	r.lastOutput = sanitized
	r.hasOutput = true
	r.outputMu.Unlock()

	if !r.stopped.Load() && r.sender != nil {
		r.sender(StatusLineUpdatedMsg{})
	}
}

func (r *Runner) snapshotState() State {
	r.stateMu.Lock()
	defer r.stateMu.Unlock()
	return r.state
}

// logLine writes to the file logger with the [statusline] step name, or is a
// no-op when no logger was injected.
func (r *Runner) logLine(msg string) {
	if r.log == nil {
		return
	}
	_ = r.log.Log("statusline", msg)
}

// firstNonEmptyLine returns the first non-empty line of b (split on \n),
// or "" when b is empty or all-whitespace.
func firstNonEmptyLine(b []byte) string {
	for _, line := range strings.Split(string(b), "\n") {
		trimmed := strings.TrimRight(line, " \t")
		if trimmed != "" {
			return trimmed
		}
	}
	return ""
}

// resolvePath resolves cmd using the same logic as validateCommandPath:
// a path containing "/" is joined with workflowDir (or used as-is if
// absolute); a bare name is looked up via exec.LookPath.
// Returns "" when the path cannot be resolved.
func resolvePath(workflowDir, cmd string) string {
	if strings.Contains(cmd, "/") {
		if filepath.IsAbs(cmd) {
			return cmd
		}
		return filepath.Join(workflowDir, cmd)
	}
	p, err := exec.LookPath(cmd)
	if err != nil {
		return ""
	}
	return p
}
