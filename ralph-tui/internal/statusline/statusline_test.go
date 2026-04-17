package statusline_test

import (
	"encoding/json"
	"testing"

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
