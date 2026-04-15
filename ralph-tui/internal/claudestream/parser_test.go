package claudestream_test

import (
	"bufio"
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/mxriverlynn/pr9k/ralph-tui/internal/claudestream"
)

// fixturesDir returns the path to docs/plans/streaming-json-output/fixtures/
// relative to this test file (D coding-standards/testing.md test helper path
// resolution — use runtime.Caller, not os.Getwd).
func fixturesDir(t *testing.T) string {
	t.Helper()
	_, thisFile, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("runtime.Caller failed")
	}
	// thisFile is .../ralph-tui/internal/claudestream/parser_test.go
	// Navigate up 3 levels to workspace root then into the fixtures directory.
	root := filepath.Join(filepath.Dir(thisFile), "..", "..", "..")
	return filepath.Join(root, "docs", "plans", "streaming-json-output", "fixtures")
}

func TestParser_SystemInit(t *testing.T) {
	p := &claudestream.Parser{}
	line := []byte(`{"type":"system","subtype":"init","session_id":"abc123","model":"claude-sonnet-4-6"}`)
	ev, err := p.Parse(line)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	sys, ok := ev.(*claudestream.SystemEvent)
	if !ok {
		t.Fatalf("expected *SystemEvent, got %T", ev)
	}
	if sys.Subtype != "init" {
		t.Errorf("subtype: got %q, want %q", sys.Subtype, "init")
	}
	if sys.SessionID != "abc123" {
		t.Errorf("session_id: got %q, want %q", sys.SessionID, "abc123")
	}
	if sys.Model != "claude-sonnet-4-6" {
		t.Errorf("model: got %q, want %q", sys.Model, "claude-sonnet-4-6")
	}
}

func TestParser_SystemAPIRetry(t *testing.T) {
	p := &claudestream.Parser{}
	line := []byte(`{"type":"system","subtype":"api_retry","attempt":2,"max_retries":5,"retry_delay_ms":1000,"error_status":429,"error":"rate_limit","uuid":"u1","session_id":"s1"}`)
	ev, err := p.Parse(line)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	sys, ok := ev.(*claudestream.SystemEvent)
	if !ok {
		t.Fatalf("expected *SystemEvent, got %T", ev)
	}
	if sys.Subtype != "api_retry" {
		t.Errorf("subtype: got %q, want %q", sys.Subtype, "api_retry")
	}
	if sys.Attempt != 2 {
		t.Errorf("attempt: got %d, want 2", sys.Attempt)
	}
	if sys.MaxRetries != 5 {
		t.Errorf("max_retries: got %d, want 5", sys.MaxRetries)
	}
	if sys.Error != "rate_limit" {
		t.Errorf("error: got %q, want %q", sys.Error, "rate_limit")
	}
}

func TestParser_RateLimitEvent(t *testing.T) {
	p := &claudestream.Parser{}
	line := []byte(`{"type":"rate_limit_event","rate_limit_info":{"status":"allowed","resetsAt":1776272400,"rateLimitType":"five_hour","overageStatus":"rejected","overageDisabledReason":"out_of_credits","isUsingOverage":false},"uuid":"u1","session_id":"s1"}`)
	ev, err := p.Parse(line)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	rl, ok := ev.(*claudestream.RateLimitEvent)
	if !ok {
		t.Fatalf("expected *RateLimitEvent, got %T", ev)
	}
	if rl.RateLimitInfo.Status != "allowed" {
		t.Errorf("status: got %q, want %q", rl.RateLimitInfo.Status, "allowed")
	}
	if rl.RateLimitInfo.RateLimitType != "five_hour" {
		t.Errorf("rateLimitType: got %q, want %q", rl.RateLimitInfo.RateLimitType, "five_hour")
	}
}

func TestParser_AssistantTextBlock(t *testing.T) {
	p := &claudestream.Parser{}
	line := []byte(`{"type":"assistant","message":{"id":"m1","model":"claude-sonnet-4-6","content":[{"type":"text","text":"Hello"}],"usage":{"input_tokens":3,"output_tokens":1}},"session_id":"s1","uuid":"u1"}`)
	ev, err := p.Parse(line)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	a, ok := ev.(*claudestream.AssistantEvent)
	if !ok {
		t.Fatalf("expected *AssistantEvent, got %T", ev)
	}
	if len(a.Message.Content) != 1 {
		t.Fatalf("content len: got %d, want 1", len(a.Message.Content))
	}
	if a.Message.Content[0].Type != "text" {
		t.Errorf("block type: got %q, want text", a.Message.Content[0].Type)
	}
	if a.Message.Content[0].Text != "Hello" {
		t.Errorf("text: got %q, want Hello", a.Message.Content[0].Text)
	}
}

func TestParser_AssistantToolUseBlock(t *testing.T) {
	p := &claudestream.Parser{}
	line := []byte(`{"type":"assistant","message":{"id":"m1","model":"m","content":[{"type":"tool_use","name":"Bash","input":{"command":"ls"}}],"usage":{}},"session_id":"s1","uuid":"u1"}`)
	ev, err := p.Parse(line)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	a, ok := ev.(*claudestream.AssistantEvent)
	if !ok {
		t.Fatalf("expected *AssistantEvent, got %T", ev)
	}
	block := a.Message.Content[0]
	if block.Type != "tool_use" {
		t.Errorf("block type: got %q, want tool_use", block.Type)
	}
	if block.Name != "Bash" {
		t.Errorf("name: got %q, want Bash", block.Name)
	}
}

func TestParser_AssistantThinkingBlock(t *testing.T) {
	p := &claudestream.Parser{}
	line := []byte(`{"type":"assistant","message":{"id":"m1","model":"m","content":[{"type":"thinking","text":"thinking..."}],"usage":{}},"session_id":"s1","uuid":"u1"}`)
	ev, err := p.Parse(line)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	a, ok := ev.(*claudestream.AssistantEvent)
	if !ok {
		t.Fatalf("expected *AssistantEvent, got %T", ev)
	}
	if a.Message.Content[0].Type != "thinking" {
		t.Errorf("block type: got %q, want thinking", a.Message.Content[0].Type)
	}
}

func TestParser_AssistantErrorField(t *testing.T) {
	// Auth failure path: top-level "error" field is present, model is "<synthetic>".
	p := &claudestream.Parser{}
	line := []byte(`{"type":"assistant","message":{"id":"m1","model":"<synthetic>","content":[{"type":"text","text":"Failed"}],"usage":{}},"error":"authentication_failed","session_id":"s1","uuid":"u1"}`)
	ev, err := p.Parse(line)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	a, ok := ev.(*claudestream.AssistantEvent)
	if !ok {
		t.Fatalf("expected *AssistantEvent, got %T", ev)
	}
	if a.Error != "authentication_failed" {
		t.Errorf("error field: got %q, want authentication_failed", a.Error)
	}
	if a.Message.Model != "<synthetic>" {
		t.Errorf("model: got %q, want <synthetic>", a.Message.Model)
	}
}

func TestParser_UserToolResult(t *testing.T) {
	p := &claudestream.Parser{}
	line := []byte(`{"type":"user","message":{"content":[{"type":"tool_result","tool_use_id":"t1","content":"ok"}]},"uuid":"u1"}`)
	ev, err := p.Parse(line)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	u, ok := ev.(*claudestream.UserEvent)
	if !ok {
		t.Fatalf("expected *UserEvent, got %T", ev)
	}
	if u.Message.Content[0].Type != "tool_result" {
		t.Errorf("block type: got %q, want tool_result", u.Message.Content[0].Type)
	}
}

func TestParser_ResultSuccess(t *testing.T) {
	p := &claudestream.Parser{}
	line := []byte(`{"type":"result","subtype":"success","is_error":false,"duration_ms":1200,"duration_api_ms":900,"num_turns":2,"result":"done","stop_reason":"end_turn","session_id":"s1","total_cost_usd":0.01,"usage":{"input_tokens":5,"output_tokens":3},"uuid":"u1"}`)
	ev, err := p.Parse(line)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	r, ok := ev.(*claudestream.ResultEvent)
	if !ok {
		t.Fatalf("expected *ResultEvent, got %T", ev)
	}
	if r.IsError {
		t.Error("is_error should be false")
	}
	if r.Result != "done" {
		t.Errorf("result: got %q, want done", r.Result)
	}
	if r.NumTurns != 2 {
		t.Errorf("num_turns: got %d, want 2", r.NumTurns)
	}
}

func TestParser_ResultIsError(t *testing.T) {
	p := &claudestream.Parser{}
	line := []byte(`{"type":"result","subtype":"success","is_error":true,"duration_ms":100,"num_turns":1,"result":"auth failed","session_id":"s1","total_cost_usd":0,"usage":{},"uuid":"u1"}`)
	ev, err := p.Parse(line)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	r, ok := ev.(*claudestream.ResultEvent)
	if !ok {
		t.Fatalf("expected *ResultEvent, got %T", ev)
	}
	if !r.IsError {
		t.Error("is_error should be true")
	}
}

func TestParser_MalformedEmptyLine(t *testing.T) {
	p := &claudestream.Parser{}
	_, err := p.Parse([]byte{})
	if err == nil {
		t.Fatal("expected error for empty line")
	}
	mle, ok := err.(*claudestream.MalformedLineError)
	if !ok {
		t.Fatalf("expected *MalformedLineError, got %T", err)
	}
	if len(mle.Raw) != 0 {
		t.Errorf("Raw should be empty, got %q", mle.Raw)
	}
}

func TestParser_MalformedTruncatedJSON(t *testing.T) {
	p := &claudestream.Parser{}
	_, err := p.Parse([]byte(`{"type":"system"`))
	if err == nil {
		t.Fatal("expected error for truncated JSON")
	}
	if _, ok := err.(*claudestream.MalformedLineError); !ok {
		t.Fatalf("expected *MalformedLineError, got %T", err)
	}
}

func TestParser_MalformedMissingType(t *testing.T) {
	p := &claudestream.Parser{}
	_, err := p.Parse([]byte(`{"subtype":"init"}`))
	if err == nil {
		t.Fatal("expected error for missing type")
	}
	mle, ok := err.(*claudestream.MalformedLineError)
	if !ok {
		t.Fatalf("expected *MalformedLineError, got %T", err)
	}
	if string(mle.Raw) != `{"subtype":"init"}` {
		t.Errorf("Raw: got %q", mle.Raw)
	}
}

func TestParser_MalformedUnknownType(t *testing.T) {
	p := &claudestream.Parser{}
	_, err := p.Parse([]byte(`{"type":"foobar"}`))
	if err == nil {
		t.Fatal("expected error for unknown type")
	}
	if _, ok := err.(*claudestream.MalformedLineError); !ok {
		t.Fatalf("expected *MalformedLineError, got %T", err)
	}
}

func TestParser_UnknownFieldsTolerated(t *testing.T) {
	// Extra fields on a known event type must not cause an error (D8).
	p := &claudestream.Parser{}
	line := []byte(`{"type":"result","subtype":"success","is_error":false,"duration_ms":100,"num_turns":1,"result":"ok","session_id":"s1","total_cost_usd":0,"usage":{},"uuid":"u1","unknownFuture":"ignored","modelUsage":{}}`)
	_, err := p.Parse(line)
	if err != nil {
		t.Fatalf("unexpected error for line with extra fields: %v", err)
	}
}

// TestParser_SmokeSuccess feeds the committed smoke-success fixture line-by-line
// and asserts every line parses without error.
func TestParser_SmokeSuccess(t *testing.T) {
	parseFixture(t, filepath.Join(fixturesDir(t), "smoke-success.ndjson"))
}

// TestParser_SmokeAuthFailure feeds the committed smoke-auth-failure fixture
// line-by-line and asserts every line parses without error.
func TestParser_SmokeAuthFailure(t *testing.T) {
	parseFixture(t, filepath.Join(fixturesDir(t), "smoke-auth-failure.ndjson"))
}

func parseFixture(t *testing.T, path string) {
	t.Helper()
	f, err := os.Open(path)
	if err != nil {
		t.Fatalf("open fixture %s: %v", path, err)
	}
	defer f.Close()

	p := &claudestream.Parser{}
	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 1024*1024), 1024*1024) // 1MB for large fixture lines
	lineNum := 0
	for sc.Scan() {
		lineNum++
		line := sc.Bytes()
		if len(line) == 0 {
			continue
		}
		if _, err := p.Parse(line); err != nil {
			t.Errorf("line %d: unexpected error: %v", lineNum, err)
		}
	}
	if err := sc.Err(); err != nil {
		t.Fatalf("scanner error: %v", err)
	}
	if lineNum == 0 {
		t.Fatalf("fixture %s had no lines", path)
	}
}
