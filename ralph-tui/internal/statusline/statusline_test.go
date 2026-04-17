package statusline_test

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/signal"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"testing"
	"time"

	"github.com/mxriverlynn/pr9k/ralph-tui/internal/logger"
	"github.com/mxriverlynn/pr9k/ralph-tui/internal/statusline"
)

// --- BuildPayload tests ---

func TestBuildPayload_InitializePhase(t *testing.T) {
	s := statusline.State{
		SessionID:     "sess-1",
		Version:       "0.5.0",
		Phase:         "initialize",
		Iteration:     0,
		MaxIterations: 3,
		StepNum:       1,
		StepCount:     2,
		StepName:      "setup",
		WorkflowDir:   "/wf",
		ProjectDir:    "/proj",
		Captures:      map[string]string{"A": "1"},
	}
	b, err := statusline.BuildPayload(s, "normal")
	if err != nil {
		t.Fatalf("BuildPayload error: %v", err)
	}
	var m map[string]any
	if err := json.Unmarshal(b, &m); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if got := m["phase"]; got != "initialize" {
		t.Errorf("phase = %v, want initialize", got)
	}
	if got := m["iteration"]; got != float64(0) {
		t.Errorf("iteration = %v, want 0 for initialize", got)
	}
}

func TestBuildPayload_IterationPhase(t *testing.T) {
	s := statusline.State{
		Phase:     "iteration",
		Iteration: 2,
	}
	b, err := statusline.BuildPayload(s, "normal")
	if err != nil {
		t.Fatalf("BuildPayload error: %v", err)
	}
	var m map[string]any
	if err := json.Unmarshal(b, &m); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if got := m["phase"]; got != "iteration" {
		t.Errorf("phase = %v, want iteration", got)
	}
	if got := m["iteration"]; got != float64(2) {
		t.Errorf("iteration = %v, want 2", got)
	}
}

func TestBuildPayload_FinalizePhase(t *testing.T) {
	s := statusline.State{
		Phase:     "finalize",
		Iteration: 0,
	}
	b, err := statusline.BuildPayload(s, "normal")
	if err != nil {
		t.Fatalf("BuildPayload error: %v", err)
	}
	var m map[string]any
	if err := json.Unmarshal(b, &m); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if got := m["phase"]; got != "finalize" {
		t.Errorf("phase = %v, want finalize", got)
	}
	if got := m["iteration"]; got != float64(0) {
		t.Errorf("iteration = %v, want 0 for finalize", got)
	}
}

func TestBuildPayload_NilCapturesProducesEmptyObject(t *testing.T) {
	s := statusline.State{Captures: nil}
	b, err := statusline.BuildPayload(s, "normal")
	if err != nil {
		t.Fatalf("BuildPayload error: %v", err)
	}
	var m map[string]any
	if err := json.Unmarshal(b, &m); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	captures, ok := m["captures"]
	if !ok {
		t.Fatal("captures key missing")
	}
	if captures == nil {
		t.Error("captures is null, want {}")
	}
	if cm, ok := captures.(map[string]any); !ok || len(cm) != 0 {
		t.Errorf("captures = %v, want empty object", captures)
	}
}

func TestBuildPayload_RoundTrip(t *testing.T) {
	s := statusline.State{
		SessionID:     "20260417-093045-123",
		Version:       "0.5.0",
		Phase:         "iteration",
		Iteration:     1,
		MaxIterations: 5,
		StepNum:       3,
		StepCount:     10,
		StepName:      "Feature work",
		WorkflowDir:   "/path/to/bundle",
		ProjectDir:    "/path/to/target",
		Captures:      map[string]string{"ISSUE_ID": "42", "GITHUB_USER": "mxriverlynn"},
	}
	b, err := statusline.BuildPayload(s, "error")
	if err != nil {
		t.Fatalf("BuildPayload error: %v", err)
	}
	var m map[string]any
	if err := json.Unmarshal(b, &m); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	checks := map[string]any{
		"sessionId":     "20260417-093045-123",
		"version":       "0.5.0",
		"phase":         "iteration",
		"iteration":     float64(1),
		"maxIterations": float64(5),
		"mode":          "error",
		"workflowDir":   "/path/to/bundle",
		"projectDir":    "/path/to/target",
	}
	for k, want := range checks {
		if got := m[k]; got != want {
			t.Errorf("%s = %v, want %v", k, got, want)
		}
	}
	step, ok := m["step"].(map[string]any)
	if !ok {
		t.Fatal("step is not an object")
	}
	if step["num"] != float64(3) {
		t.Errorf("step.num = %v, want 3", step["num"])
	}
	if step["count"] != float64(10) {
		t.Errorf("step.count = %v, want 10", step["count"])
	}
	if step["name"] != "Feature work" {
		t.Errorf("step.name = %v, want Feature work", step["name"])
	}
	caps, ok := m["captures"].(map[string]any)
	if !ok {
		t.Fatal("captures is not an object")
	}
	if caps["ISSUE_ID"] != "42" {
		t.Errorf("captures.ISSUE_ID = %v, want 42", caps["ISSUE_ID"])
	}
}

// --- Sanitize tests ---

func TestSanitize_StripsCR(t *testing.T) {
	got := statusline.Sanitize([]byte("hello\rworld"))
	if got != "helloworld" {
		t.Errorf("got %q, want %q", got, "helloworld")
	}
}

func TestSanitize_StripsEraseDisplay(t *testing.T) {
	got := statusline.Sanitize([]byte("pre\x1b[2Jpost"))
	if got != "prepost" {
		t.Errorf("got %q, want %q", got, "prepost")
	}
}

func TestSanitize_StripsEraseLine(t *testing.T) {
	got := statusline.Sanitize([]byte("pre\x1b[2Kpost"))
	if got != "prepost" {
		t.Errorf("got %q, want %q", got, "prepost")
	}
}

func TestSanitize_StripsCursorMovement(t *testing.T) {
	got := statusline.Sanitize([]byte("pre\x1b[10Apost"))
	if got != "prepost" {
		t.Errorf("got %q, want %q", got, "prepost")
	}
}

func TestSanitize_MidCSITruncation(t *testing.T) {
	// unterminated CSI — must not panic; stray bytes dropped
	defer func() {
		if r := recover(); r != nil {
			t.Errorf("panicked: %v", r)
		}
	}()
	got := statusline.Sanitize([]byte("prefix\x1b[3"))
	if got != "prefix" {
		t.Errorf("got %q, want %q", got, "prefix")
	}
}

func TestSanitize_StripsBareESCAtEOF(t *testing.T) {
	got := statusline.Sanitize([]byte("abc\x1b"))
	if got != "abc" {
		t.Errorf("got %q, want %q", got, "abc")
	}
}

func TestSanitize_StripsBareBEL(t *testing.T) {
	got := statusline.Sanitize([]byte("a\x07b"))
	if got != "ab" {
		t.Errorf("got %q, want %q", got, "ab")
	}
}

func TestSanitize_PreservesSGR(t *testing.T) {
	input := "\x1b[32mgreen\x1b[0m"
	got := statusline.Sanitize([]byte(input))
	if got != input {
		t.Errorf("got %q, want %q", got, input)
	}
}

func TestSanitize_PreservesOSC8Hyperlink(t *testing.T) {
	// OSC 8 with BEL terminator
	link := "\x1b]8;;https://example.com\x07text\x1b]8;;\x07"
	got := statusline.Sanitize([]byte(link))
	if got != link {
		t.Errorf("got %q, want %q", got, link)
	}
}

func TestSanitize_StripsNonOSC8OSC(t *testing.T) {
	// OSC 0 (set window title) should be stripped
	got := statusline.Sanitize([]byte("\x1b]0;My Title\x07visible"))
	if got != "visible" {
		t.Errorf("got %q, want %q", got, "visible")
	}
}

func TestSanitize_TrimsTrailingWhitespace(t *testing.T) {
	got := statusline.Sanitize([]byte("hello   \t "))
	if got != "hello" {
		t.Errorf("got %q, want %q", got, "hello")
	}
}

// T1 — OSC 8 hyperlink terminated by ST (\x1b\) must be preserved.
func TestSanitize_PreservesOSC8HyperlinkSTTerminator(t *testing.T) {
	link := "\x1b]8;;https://example.com\x1b\\text\x1b]8;;\x1b\\"
	got := statusline.Sanitize([]byte(link))
	if got != link {
		t.Errorf("got %q, want %q", got, link)
	}
}

// T2 — Unterminated OSC sequences must not panic and must be dropped.
func TestSanitize_UnterminatedOSCDropped(t *testing.T) {
	defer func() {
		if r := recover(); r != nil {
			t.Errorf("panicked: %v", r)
		}
	}()
	got := statusline.Sanitize([]byte("pre\x1b]0;title-never-ends"))
	if got != "pre" {
		t.Errorf("non-OSC-8 unterminated: got %q, want %q", got, "pre")
	}
	got = statusline.Sanitize([]byte("\x1b]8;;https://example.com"))
	if got != "" {
		t.Errorf("OSC-8 unterminated: got %q, want %q", got, "")
	}
}

// T3 — Multi-parameter SGR sequences (256-color, compound) must round-trip.
func TestSanitize_PreservesMultiParamSGR(t *testing.T) {
	cases := []string{
		"\x1b[38;5;196mred\x1b[0m",
		"\x1b[1;31;4mx\x1b[0m",
	}
	for _, input := range cases {
		got := statusline.Sanitize([]byte(input))
		if got != input {
			t.Errorf("got %q, want %q", got, input)
		}
	}
}

// T4 — Bare ESC followed by an unrecognized byte: ESC dropped, byte kept.
func TestSanitize_BareESCDroppedNextByteKept(t *testing.T) {
	got := statusline.Sanitize([]byte("a\x1bXb"))
	if got != "aXb" {
		t.Errorf("got %q, want %q", got, "aXb")
	}
}

// T5 — Sanitize must not mutate the caller's input slice.
func TestSanitize_DoesNotMutateInput(t *testing.T) {
	in := []byte("pre\x1b[2Jpost\r ")
	snapshot := make([]byte, len(in))
	copy(snapshot, in)
	statusline.Sanitize(in)
	if !bytes.Equal(in, snapshot) {
		t.Error("Sanitize mutated the input slice")
	}
}

// T6 — Nil, empty, and all-whitespace inputs must all return "".
func TestSanitize_EmptyAndWhitespaceInput(t *testing.T) {
	if got := statusline.Sanitize(nil); got != "" {
		t.Errorf("nil: got %q, want %q", got, "")
	}
	if got := statusline.Sanitize([]byte("")); got != "" {
		t.Errorf("empty: got %q, want %q", got, "")
	}
	if got := statusline.Sanitize([]byte("   \t\t")); got != "" {
		t.Errorf("whitespace-only: got %q, want %q", got, "")
	}
}

// T7 — BuildPayload reads Captures by reference; caller owns the snapshot.
func TestBuildPayload_CapturesNotDeepCopied(t *testing.T) {
	s := statusline.State{
		Captures: map[string]string{"key": "val1"},
	}
	b1, err := statusline.BuildPayload(s, "normal")
	if err != nil {
		t.Fatalf("BuildPayload error: %v", err)
	}
	s.Captures["key"] = "val2"
	b2, err := statusline.BuildPayload(s, "normal")
	if err != nil {
		t.Fatalf("BuildPayload error: %v", err)
	}
	if bytes.Equal(b1, b2) {
		t.Error("outputs identical after mutating Captures — expected to differ (no deep copy)")
	}
}

// L3 — OSC 88 starts with "8" but is not OSC 8; must be dropped, not preserved.
func TestSanitize_OSC88IsDropped(t *testing.T) {
	// OSC 88 with BEL terminator — must not be preserved (prefix "8" ≠ number "8")
	got := statusline.Sanitize([]byte("\x1b]88;extra\x07visible"))
	if got != "visible" {
		t.Errorf("got %q, want %q", got, "visible")
	}
	// OSC 88 with ST terminator
	got = statusline.Sanitize([]byte("\x1b]88;extra\x1b\\visible"))
	if got != "visible" {
		t.Errorf("ST-terminated OSC 88: got %q, want %q", got, "visible")
	}
}

// T8 — BuildPayload output is deterministic and contains exactly the schema keys.
func TestBuildPayload_DeterministicSchemaKeys(t *testing.T) {
	s := statusline.State{
		SessionID:     "sess-abc",
		Version:       "0.5.0",
		Phase:         "iteration",
		Iteration:     1,
		MaxIterations: 3,
		StepNum:       2,
		StepCount:     5,
		StepName:      "Feature work",
		WorkflowDir:   "/wf",
		ProjectDir:    "/proj",
		Captures:      map[string]string{"A": "1"},
	}
	b1, err := statusline.BuildPayload(s, "normal")
	if err != nil {
		t.Fatalf("BuildPayload error: %v", err)
	}
	b2, err := statusline.BuildPayload(s, "normal")
	if err != nil {
		t.Fatalf("BuildPayload error: %v", err)
	}
	if !bytes.Equal(b1, b2) {
		t.Error("BuildPayload is not deterministic for identical inputs")
	}

	var m map[string]any
	if err := json.Unmarshal(b1, &m); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	wantTop := []string{"sessionId", "version", "phase", "iteration", "maxIterations", "step", "mode", "workflowDir", "projectDir", "captures"}
	for _, k := range wantTop {
		if _, ok := m[k]; !ok {
			t.Errorf("missing top-level key %q", k)
		}
	}
	if len(m) != len(wantTop) {
		t.Errorf("top-level key count = %d, want %d", len(m), len(wantTop))
	}

	step, ok := m["step"].(map[string]any)
	if !ok {
		t.Fatal("step is not an object")
	}
	wantStep := []string{"num", "count", "name"}
	for _, k := range wantStep {
		if _, ok := step[k]; !ok {
			t.Errorf("missing step key %q", k)
		}
	}
	if len(step) != len(wantStep) {
		t.Errorf("step key count = %d, want %d", len(step), len(wantStep))
	}
}

// =============================================================================
// TestMain + subprocess helper for Runner tests
// =============================================================================

// TestMain intercepts when the test binary is re-executed as a script stub.
// Set STATUSLINE_TEST_HELPER=<mode> in the subprocess environment.
func TestMain(m *testing.M) {
	if mode := os.Getenv("STATUSLINE_TEST_HELPER"); mode != "" {
		runTestHelper(mode)
	}
	os.Exit(m.Run())
}

func runTestHelper(mode string) {
	switch mode {
	case "output":
		fmt.Println(os.Getenv("HELPER_OUTPUT"))
	case "empty":
		// exit 0, no stdout
	case "sleep":
		secs, _ := strconv.Atoi(os.Getenv("HELPER_SLEEP_SEC"))
		if secs <= 0 {
			secs = 10
		}
		time.Sleep(time.Duration(secs) * time.Second)
	case "exit1":
		os.Exit(1)
	case "stderr":
		fmt.Fprintln(os.Stderr, os.Getenv("HELPER_STDERR"))
	case "bigout":
		// emit just over 8 KB on one line
		fmt.Print(strings.Repeat("B", 9*1024+100))
		fmt.Println()
	case "trap":
		// ignore SIGTERM so SIGKILL (via WaitDelay) must fire
		c := make(chan os.Signal, 1)
		signal.Notify(c, syscall.SIGTERM)
		time.Sleep(30 * time.Second)
	case "counter":
		// append one byte to HELPER_COUNTER_FILE, sleep 50 ms, then print
		if f := os.Getenv("HELPER_COUNTER_FILE"); f != "" {
			fp, _ := os.OpenFile(f, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o600)
			if fp != nil {
				_, _ = fp.WriteString("x")
				fp.Close()
			}
		}
		time.Sleep(50 * time.Millisecond)
		fmt.Println("counted")
	case "ansi-multiline":
		// emit a line with ANSI escape sequences and CR, followed by a second line
		fmt.Print("  pre\x1b[2Jclean\r\nignored\n")
	case "blank-lines":
		// emit leading blank/whitespace-only lines followed by content
		fmt.Print("\n\n  \nreal-value\nsecond-line\n")
	case "echo-stdin":
		// read JSON payload from stdin, echo the field named by HELPER_ECHO_FIELD
		data, _ := io.ReadAll(os.Stdin)
		var m map[string]any
		if err := json.Unmarshal(data, &m); err != nil {
			os.Exit(1)
		}
		field := os.Getenv("HELPER_ECHO_FIELD")
		if v, ok := m[field]; ok {
			fmt.Println(v)
		}
	}
	os.Exit(0)
}

// =============================================================================
// Runner test helpers
// =============================================================================

// newRunner returns a Runner backed by the test binary in the given helper
// mode with a temp projectDir and no logger. Extra env vars are set with
// t.Setenv so they are restored after the test.
func newRunner(t *testing.T, helperMode string, extraEnv map[string]string, refreshSecs *int) *statusline.Runner {
	t.Helper()
	t.Setenv("STATUSLINE_TEST_HELPER", helperMode)
	for k, v := range extraEnv {
		t.Setenv(k, v)
	}
	cfg := &statusline.Config{
		Command:                os.Args[0],
		RefreshIntervalSeconds: refreshSecs,
	}
	return statusline.New(cfg, "" /* workflowDir unused: abs path */, t.TempDir(), nil)
}

// waitCondition polls cond up to timeout, sleeping interval between checks.
func waitCondition(timeout, interval time.Duration, cond func() bool) bool {
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if cond() {
			return true
		}
		time.Sleep(interval)
	}
	return cond()
}

// int ptr helper
func intPtr(n int) *int { return &n }

// =============================================================================
// Runner tests
// =============================================================================

// TestRunner_SingleFlight verifies that rapid triggers do not cause panics,
// do not block, and do not overflow the channel. Invocation count is bounded.
func TestRunner_SingleFlight(t *testing.T) {
	counterFile := t.TempDir() + "/count"
	runner := newRunner(t, "counter",
		map[string]string{"HELPER_COUNTER_FILE": counterFile},
		intPtr(0)) // no timer
	t.Cleanup(runner.Shutdown)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	runner.Start(ctx)

	// flood 20 triggers; channel cap is 4 so many will be dropped
	for i := 0; i < 20; i++ {
		runner.Trigger()
	}

	// wait up to 1 s for some executions
	time.Sleep(500 * time.Millisecond)

	// read counter
	data, _ := os.ReadFile(counterFile)
	count := len(data) // each invocation writes one byte
	if count == 0 {
		t.Error("expected at least one invocation, got 0")
	}
	// at most channel-cap+1 queued initially; allow generous upper bound
	if count > 10 {
		t.Errorf("unexpectedly high invocation count %d — possible goroutine leak", count)
	}
}

// TestRunner_Timeout verifies that a slow script is killed within ~3 s and
// the cache is not updated.
func TestRunner_Timeout(t *testing.T) {
	runner := newRunner(t, "sleep",
		map[string]string{"HELPER_SLEEP_SEC": "10"},
		intPtr(0))
	t.Cleanup(runner.Shutdown)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	runner.Start(ctx)
	runner.Trigger()

	// wait for timeout + grace + buffer
	time.Sleep(3500 * time.Millisecond)

	if runner.HasOutput() {
		t.Error("HasOutput() should be false after timeout")
	}
	if runner.LastOutput() != "" {
		t.Errorf("LastOutput() = %q, want empty after timeout", runner.LastOutput())
	}
}

// TestRunner_TimeoutSIGTERMIgnored verifies that a script ignoring SIGTERM is
// eventually killed via SIGKILL (cmd.WaitDelay) and that no goroutine leak
// occurs.
func TestRunner_TimeoutSIGTERMIgnored(t *testing.T) {
	before := runtime.NumGoroutine()

	runner := newRunner(t, "trap", nil, intPtr(0))
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	runner.Start(ctx)
	runner.Trigger()

	// allow 2s timeout + 1s WaitDelay + 1s buffer
	time.Sleep(4500 * time.Millisecond)

	runner.Shutdown()

	// allow goroutines to drain
	time.Sleep(100 * time.Millisecond)
	after := runtime.NumGoroutine()

	// tolerance of 3 to account for Go runtime background goroutines
	if after > before+3 {
		t.Errorf("possible goroutine leak: before=%d after=%d", before, after)
	}
	if runner.HasOutput() {
		t.Error("HasOutput() should be false after SIGKILL")
	}
}

// TestRunner_EmptyStdout verifies that exit 0 with empty stdout sets
// HasOutput() true and LastOutput() "".
func TestRunner_EmptyStdout(t *testing.T) {
	runner := newRunner(t, "empty", nil, intPtr(0))
	t.Cleanup(runner.Shutdown)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	runner.Start(ctx)
	runner.Trigger()

	ok := waitCondition(2*time.Second, 20*time.Millisecond, runner.HasOutput)
	if !ok {
		t.Fatal("HasOutput() did not become true")
	}
	if got := runner.LastOutput(); got != "" {
		t.Errorf("LastOutput() = %q, want empty", got)
	}
}

// TestRunner_NonZeroExit verifies that a non-zero exit does not overwrite a
// previously cached value.
func TestRunner_NonZeroExit(t *testing.T) {
	// first run succeeds and populates the cache
	t.Setenv("STATUSLINE_TEST_HELPER", "output")
	t.Setenv("HELPER_OUTPUT", "good-value")

	cfg := &statusline.Config{Command: os.Args[0], RefreshIntervalSeconds: intPtr(0)}
	runner := statusline.New(cfg, "", t.TempDir(), nil)
	t.Cleanup(runner.Shutdown)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	runner.Start(ctx)
	runner.Trigger()

	if !waitCondition(2*time.Second, 20*time.Millisecond, runner.HasOutput) {
		t.Fatal("cache not populated by first run")
	}
	if runner.LastOutput() != "good-value" {
		t.Fatalf("unexpected initial cache: %q", runner.LastOutput())
	}

	// swap to exit-1 mode
	t.Setenv("STATUSLINE_TEST_HELPER", "exit1")
	runner.Trigger()
	time.Sleep(200 * time.Millisecond)

	if runner.LastOutput() != "good-value" {
		t.Errorf("cache corrupted by failing run: got %q, want %q", runner.LastOutput(), "good-value")
	}
}

// TestRunner_ColdStart verifies HasOutput() is false until the first
// successful run, and a failing first run keeps it false.
func TestRunner_ColdStart(t *testing.T) {
	runner := newRunner(t, "exit1", nil, intPtr(0))
	t.Cleanup(runner.Shutdown)

	if runner.HasOutput() {
		t.Error("HasOutput() should be false before any run")
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	runner.Start(ctx)
	runner.Trigger()
	time.Sleep(200 * time.Millisecond)

	if runner.HasOutput() {
		t.Error("HasOutput() should remain false after failing first run")
	}

	// now switch to a succeeding helper
	t.Setenv("STATUSLINE_TEST_HELPER", "output")
	t.Setenv("HELPER_OUTPUT", "live")
	runner.Trigger()
	if !waitCondition(2*time.Second, 20*time.Millisecond, runner.HasOutput) {
		t.Error("HasOutput() should become true after first successful run")
	}
}

// TestRunner_DefaultInterval verifies that a nil RefreshIntervalSeconds maps
// to DefaultRefreshInterval (5 s) and that the timer goroutine is started.
func TestRunner_DefaultInterval(t *testing.T) {
	if statusline.DefaultRefreshInterval != 5*time.Second {
		t.Errorf("DefaultRefreshInterval = %v, want 5s", statusline.DefaultRefreshInterval)
	}

	t.Setenv("STATUSLINE_TEST_HELPER", "output")
	t.Setenv("HELPER_OUTPUT", "tick")

	// nil RefreshIntervalSeconds → default 5s interval
	cfg := &statusline.Config{Command: os.Args[0], RefreshIntervalSeconds: nil}
	runner := statusline.New(cfg, "", t.TempDir(), nil)
	t.Cleanup(runner.Shutdown)

	before := runtime.NumGoroutine()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	runner.Start(ctx)
	time.Sleep(50 * time.Millisecond)
	after := runtime.NumGoroutine()

	// two goroutines started: worker + timer
	delta := after - before
	if delta < 2 {
		t.Errorf("goroutine delta = %d, want >= 2 (worker + timer)", delta)
	}
}

// TestRunner_DisabledTimer verifies that RefreshIntervalSeconds=0 starts only
// the worker goroutine (no timer).
func TestRunner_DisabledTimer(t *testing.T) {
	runner := newRunner(t, "output", map[string]string{"HELPER_OUTPUT": "x"}, intPtr(0))
	t.Cleanup(runner.Shutdown)

	before := runtime.NumGoroutine()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	runner.Start(ctx)
	time.Sleep(50 * time.Millisecond)
	after := runtime.NumGoroutine()

	delta := after - before
	if delta != 1 {
		t.Errorf("goroutine delta = %d, want 1 (worker only, no timer)", delta)
	}
}

// TestRunner_PositiveInterval verifies that a 1-second interval causes the
// timer to fire at least once within a generous window.
func TestRunner_PositiveInterval(t *testing.T) {
	var mu sync.Mutex
	var received int
	runner := newRunner(t, "output", map[string]string{"HELPER_OUTPUT": "tick"}, intPtr(1))
	t.Cleanup(runner.Shutdown)

	runner.SetSender(func(_ interface{}) {
		mu.Lock()
		received++
		mu.Unlock()
	})

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	runner.Start(ctx)

	// wait up to 3 s for at least one timer-driven trigger
	ok := waitCondition(3*time.Second, 100*time.Millisecond, func() bool {
		mu.Lock()
		defer mu.Unlock()
		return received > 0
	})
	if !ok {
		t.Error("timer did not fire within 3 s")
	}
}

// TestRunner_ShutdownSkipsSend verifies that after Shutdown, a completing
// in-flight run does not invoke the sender.
func TestRunner_ShutdownSkipsSend(t *testing.T) {
	runner := newRunner(t, "output", map[string]string{"HELPER_OUTPUT": "after-shutdown"}, intPtr(0))

	var sends int
	runner.SetSender(func(_ interface{}) { sends++ })

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	runner.Start(ctx)

	// Shutdown before any trigger — no send should happen.
	runner.Shutdown()
	runner.Trigger() // dropped because worker is gone
	time.Sleep(100 * time.Millisecond)

	if sends != 0 {
		t.Errorf("sender called %d times after Shutdown, want 0", sends)
	}
}

// TestRunner_BoundedStdout verifies that script output > 8 KB is truncated
// before caching.
func TestRunner_BoundedStdout(t *testing.T) {
	runner := newRunner(t, "bigout", nil, intPtr(0))
	t.Cleanup(runner.Shutdown)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	runner.Start(ctx)
	runner.Trigger()

	if !waitCondition(3*time.Second, 50*time.Millisecond, runner.HasOutput) {
		t.Fatal("HasOutput() did not become true")
	}
	if got := len(runner.LastOutput()); got > 8*1024 {
		t.Errorf("LastOutput() length = %d, want <= 8192 (8 KB)", got)
	}
}

// TestRunner_StderrForwarded verifies that script stderr is captured and
// written to the file logger with a [statusline] step prefix.
func TestRunner_StderrForwarded(t *testing.T) {
	t.Setenv("STATUSLINE_TEST_HELPER", "stderr")
	t.Setenv("HELPER_STDERR", "test-stderr-message")

	tmpDir := t.TempDir()
	log, err := logger.NewLogger(tmpDir)
	if err != nil {
		t.Fatalf("logger: %v", err)
	}
	t.Cleanup(func() { _ = log.Close() })

	cfg := &statusline.Config{Command: os.Args[0], RefreshIntervalSeconds: intPtr(0)}
	runner := statusline.New(cfg, "", tmpDir, log)
	t.Cleanup(runner.Shutdown)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	runner.Start(ctx)
	runner.Trigger()

	// wait for the run to complete and logger to flush
	time.Sleep(500 * time.Millisecond)
	_ = log.Close()

	// read the log file and check for [statusline] + message
	entries, err := readLogFiles(tmpDir)
	if err != nil {
		t.Fatalf("read log: %v", err)
	}
	found := false
	for _, e := range entries {
		if strings.Contains(e, "[statusline]") && strings.Contains(e, "test-stderr-message") {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("[statusline] stderr log entry not found; log lines: %v", entries)
	}
}

// TestRunner_NoOpDisabled verifies that a no-op Runner is safe to call and
// reports Enabled() == false.
func TestRunner_NoOpDisabled(t *testing.T) {
	r := statusline.NewNoOp()
	if r.Enabled() {
		t.Error("Enabled() should be false for no-op runner")
	}
	// all methods must be safe no-ops
	r.PushState(statusline.State{})
	r.Trigger()
	r.SetSender(func(_ interface{}) {})
	r.SetModeGetter(func() string { return "normal" })
	if r.LastOutput() != "" {
		t.Errorf("LastOutput() = %q, want empty", r.LastOutput())
	}
	if r.HasOutput() {
		t.Error("HasOutput() should be false")
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	before := runtime.NumGoroutine()
	r.Start(ctx)
	time.Sleep(20 * time.Millisecond)
	after := runtime.NumGoroutine()
	if after > before+1 {
		t.Errorf("no-op Start started goroutines: before=%d after=%d", before, after)
	}
	r.Shutdown() // must not block or panic
}

// =============================================================================
// file log helper for TestRunner_StderrForwarded
// =============================================================================

// readLogFiles reads all log lines from files in dir/logs/.
func readLogFiles(dir string) ([]string, error) {
	logsDir := dir + "/logs"
	entries, err := os.ReadDir(logsDir)
	if err != nil {
		return nil, fmt.Errorf("ReadDir %s: %w", logsDir, err)
	}
	var lines []string
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		data, err := os.ReadFile(logsDir + "/" + e.Name())
		if err != nil {
			return nil, err
		}
		for _, l := range strings.Split(string(data), "\n") {
			if l != "" {
				lines = append(lines, l)
			}
		}
	}
	return lines, nil
}

// =============================================================================
// New Runner tests (T1–T13)
// =============================================================================

// T1 — New(nil, ...) must return a no-op runner.
func TestRunner_NewWithNilCfgIsNoOp(t *testing.T) {
	r := statusline.New(nil, "", "", nil)
	if r.Enabled() {
		t.Error("Enabled() should be false for nil-config runner")
	}
}

// T2 — New with an unresolvable bare command name must return a no-op runner.
func TestRunner_NewWithUnresolvableCommandIsNoOp(t *testing.T) {
	r := statusline.New(&statusline.Config{Command: "definitely-not-on-path-xyz"}, "", "", nil)
	if r.Enabled() {
		t.Error("Enabled() should be false for unresolvable command")
	}
}

// T3 — Script output containing ANSI escapes and CR: Sanitize must be wired
// into exec so the cached value is clean.
func TestRunner_SanitizedOutput(t *testing.T) {
	runner := newRunner(t, "ansi-multiline", nil, intPtr(0))
	t.Cleanup(runner.Shutdown)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	runner.Start(ctx)
	runner.Trigger()

	if !waitCondition(2*time.Second, 20*time.Millisecond, runner.HasOutput) {
		t.Fatal("HasOutput() did not become true")
	}
	got := runner.LastOutput()
	// ANSI erase-display and CR must be stripped; leading spaces and text kept.
	if !strings.Contains(got, "preclean") {
		t.Errorf("LastOutput() = %q, want it to contain %q (sanitized)", got, "preclean")
	}
	if strings.ContainsAny(got, "\x1b\r") {
		t.Errorf("LastOutput() = %q still contains raw control bytes", got)
	}
}

// T4 — Leading blank/whitespace-only lines: firstNonEmptyLine must skip them.
func TestRunner_FirstNonEmptyLineFromMultiLineStdout(t *testing.T) {
	runner := newRunner(t, "blank-lines", nil, intPtr(0))
	t.Cleanup(runner.Shutdown)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	runner.Start(ctx)
	runner.Trigger()

	if !waitCondition(2*time.Second, 20*time.Millisecond, runner.HasOutput) {
		t.Fatal("HasOutput() did not become true")
	}
	if got := runner.LastOutput(); got != "real-value" {
		t.Errorf("LastOutput() = %q, want %q", got, "real-value")
	}
}

// T5 — PushState → BuildPayload → cmd.Stdin: the injected state reaches the
// script as a JSON payload on stdin.
func TestRunner_PayloadDeliveredToScriptStdin(t *testing.T) {
	runner := newRunner(t, "echo-stdin",
		map[string]string{"HELPER_ECHO_FIELD": "phase"},
		intPtr(0))
	t.Cleanup(runner.Shutdown)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	runner.Start(ctx)

	runner.PushState(statusline.State{Phase: "iteration"})
	runner.Trigger()

	if !waitCondition(2*time.Second, 20*time.Millisecond, runner.HasOutput) {
		t.Fatal("HasOutput() did not become true")
	}
	if got := runner.LastOutput(); got != "iteration" {
		t.Errorf("LastOutput() = %q, want %q", got, "iteration")
	}
}

// T6 — SetModeGetter: the getter is invoked per run and its value reaches the
// script's stdin payload.
func TestRunner_ModeGetterInvokedPerRun(t *testing.T) {
	var called int
	runner := newRunner(t, "echo-stdin",
		map[string]string{"HELPER_ECHO_FIELD": "mode"},
		intPtr(0))
	t.Cleanup(runner.Shutdown)

	runner.SetModeGetter(func() string {
		called++
		return "error"
	})

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	runner.Start(ctx)
	runner.Trigger()

	if !waitCondition(2*time.Second, 20*time.Millisecond, runner.HasOutput) {
		t.Fatal("HasOutput() did not become true")
	}
	if got := runner.LastOutput(); got != "error" {
		t.Errorf("LastOutput() = %q, want %q", got, "error")
	}
	if called == 0 {
		t.Error("modeGetter was never called")
	}
}

// T7 — SetSender is called exactly once after a single successful Trigger.
func TestRunner_SenderInvokedOnSuccess(t *testing.T) {
	runner := newRunner(t, "output",
		map[string]string{"HELPER_OUTPUT": "hi"},
		intPtr(0))
	t.Cleanup(runner.Shutdown)

	var mu sync.Mutex
	var calls int
	runner.SetSender(func(_ interface{}) {
		mu.Lock()
		calls++
		mu.Unlock()
	})

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	runner.Start(ctx)
	runner.Trigger()

	if !waitCondition(2*time.Second, 20*time.Millisecond, runner.HasOutput) {
		t.Fatal("HasOutput() did not become true")
	}

	mu.Lock()
	got := calls
	mu.Unlock()
	if got != 1 {
		t.Errorf("sender called %d times, want 1", got)
	}
}

// T8 — SetSender must not be called when the script exits non-zero.
func TestRunner_SenderNotInvokedOnFailure(t *testing.T) {
	runner := newRunner(t, "exit1", nil, intPtr(0))
	t.Cleanup(runner.Shutdown)

	var mu sync.Mutex
	var calls int
	runner.SetSender(func(_ interface{}) {
		mu.Lock()
		calls++
		mu.Unlock()
	})

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	runner.Start(ctx)
	runner.Trigger()
	time.Sleep(300 * time.Millisecond)

	mu.Lock()
	got := calls
	mu.Unlock()
	if got != 0 {
		t.Errorf("sender called %d times after non-zero exit, want 0", got)
	}
}

// T9 — Shutdown is idempotent: calling it twice must not panic.
func TestRunner_ShutdownIdempotent(t *testing.T) {
	runner := newRunner(t, "output",
		map[string]string{"HELPER_OUTPUT": "x"},
		intPtr(0))

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	runner.Start(ctx)

	runner.Shutdown()
	runner.Shutdown() // must not panic or block
}

// T10 — Shutdown before Start must not panic (r.cancel is nil).
func TestRunner_ShutdownBeforeStart(t *testing.T) {
	runner := newRunner(t, "output",
		map[string]string{"HELPER_OUTPUT": "x"},
		intPtr(0))
	runner.Shutdown() // cancel is nil — must not panic
}

// T11 — When stdout exceeds 8 KB, the logger receives a truncation message.
func TestRunner_BoundedStdoutLogsTruncation(t *testing.T) {
	t.Setenv("STATUSLINE_TEST_HELPER", "bigout")

	tmpDir := t.TempDir()
	log, err := logger.NewLogger(tmpDir)
	if err != nil {
		t.Fatalf("logger: %v", err)
	}
	t.Cleanup(func() { _ = log.Close() })

	cfg := &statusline.Config{Command: os.Args[0], RefreshIntervalSeconds: intPtr(0)}
	runner := statusline.New(cfg, "", tmpDir, log)
	t.Cleanup(runner.Shutdown)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	runner.Start(ctx)
	runner.Trigger()

	time.Sleep(500 * time.Millisecond)
	_ = log.Close()

	entries, err := readLogFiles(tmpDir)
	if err != nil {
		t.Fatalf("read log: %v", err)
	}
	found := false
	for _, e := range entries {
		if strings.Contains(e, "[statusline]") && strings.Contains(e, "stdout truncated at 8 KB") {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("[statusline] truncation log entry not found; log lines: %v", entries)
	}
}

// T12 — Cancelling the parent context stops the worker without calling Shutdown.
func TestRunner_ParentContextCancelStopsWorker(t *testing.T) {
	runner := newRunner(t, "output",
		map[string]string{"HELPER_OUTPUT": "x"},
		intPtr(0))
	t.Cleanup(runner.Shutdown)

	before := runtime.NumGoroutine()
	ctx, cancel := context.WithCancel(context.Background())
	runner.Start(ctx)
	time.Sleep(20 * time.Millisecond) // let worker goroutine start

	cancel()

	ok := waitCondition(500*time.Millisecond, 10*time.Millisecond, func() bool {
		return runtime.NumGoroutine() <= before+1
	})
	if !ok {
		t.Errorf("worker did not drain within 500 ms after parent context cancel: before=%d after=%d",
			before, runtime.NumGoroutine())
	}
}

// T13 — Relative command path is joined with workflowDir and executed.
func TestRunner_RelativePathResolution(t *testing.T) {
	workflowDir := t.TempDir()
	scriptsDir := filepath.Join(workflowDir, "scripts")
	if err := os.MkdirAll(scriptsDir, 0o755); err != nil {
		t.Fatalf("mkdir scripts: %v", err)
	}
	// Copy the running test binary to scripts/helper so the subprocess helper
	// mechanism (STATUSLINE_TEST_HELPER) works at the resolved path.
	helperPath := filepath.Join(scriptsDir, "helper")
	data, err := os.ReadFile(os.Args[0])
	if err != nil {
		t.Fatalf("read test binary: %v", err)
	}
	if err := os.WriteFile(helperPath, data, 0o755); err != nil {
		t.Fatalf("write helper binary: %v", err)
	}

	t.Setenv("STATUSLINE_TEST_HELPER", "output")
	t.Setenv("HELPER_OUTPUT", "relative-ok")

	cfg := &statusline.Config{
		Command:                "scripts/helper",
		RefreshIntervalSeconds: intPtr(0),
	}
	runner := statusline.New(cfg, workflowDir, t.TempDir(), nil)
	t.Cleanup(runner.Shutdown)

	if !runner.Enabled() {
		t.Fatal("runner should be enabled for relative path within workflowDir")
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	runner.Start(ctx)
	runner.Trigger()

	if !waitCondition(2*time.Second, 20*time.Millisecond, runner.HasOutput) {
		t.Fatal("HasOutput() did not become true")
	}
	if got := runner.LastOutput(); got != "relative-ok" {
		t.Errorf("LastOutput() = %q, want %q", got, "relative-ok")
	}
}
