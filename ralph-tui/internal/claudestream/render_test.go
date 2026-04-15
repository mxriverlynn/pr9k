package claudestream_test

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/mxriverlynn/pr9k/ralph-tui/internal/claudestream"
)

func TestRenderer_SystemInit(t *testing.T) {
	r := &claudestream.Renderer{}
	ev := &claudestream.SystemEvent{Type: "system", Subtype: "init", SessionID: "ses1", Model: "claude-sonnet-4-6"}
	lines := r.Render(ev)
	if len(lines) != 1 {
		t.Fatalf("expected 1 line, got %d: %v", len(lines), lines)
	}
	want := "[claude session ses1 started, model claude-sonnet-4-6]"
	if lines[0] != want {
		t.Errorf("got %q, want %q", lines[0], want)
	}
}

func TestRenderer_SystemAPIRetry(t *testing.T) {
	r := &claudestream.Renderer{}
	ev := &claudestream.SystemEvent{
		Type: "system", Subtype: "api_retry",
		Attempt: 2, MaxRetries: 5, RetryDelayMS: 1000, Error: "rate_limit",
	}
	lines := r.Render(ev)
	if len(lines) != 1 {
		t.Fatalf("expected 1 line, got %d: %v", len(lines), lines)
	}
	want := "⚠ retry 2/5 in 1000ms — rate_limit"
	if lines[0] != want {
		t.Errorf("got %q, want %q", lines[0], want)
	}
}

func TestRenderer_AssistantText(t *testing.T) {
	r := &claudestream.Renderer{}
	ev := &claudestream.AssistantEvent{
		Type: "assistant",
		Message: claudestream.AssistantMsg{
			Content: []claudestream.ContentBlock{
				{Type: "text", Text: "line one\nline two"},
			},
		},
	}
	lines := r.Render(ev)
	if len(lines) != 2 {
		t.Fatalf("expected 2 lines, got %d: %v", len(lines), lines)
	}
	if lines[0] != "line one" || lines[1] != "line two" {
		t.Errorf("got %v", lines)
	}
}

func TestRenderer_AssistantMultiTurnBlankLine(t *testing.T) {
	r := &claudestream.Renderer{}
	turn1 := &claudestream.AssistantEvent{
		Type:    "assistant",
		Message: claudestream.AssistantMsg{Content: []claudestream.ContentBlock{{Type: "text", Text: "turn1"}}},
	}
	turn2 := &claudestream.AssistantEvent{
		Type:    "assistant",
		Message: claudestream.AssistantMsg{Content: []claudestream.ContentBlock{{Type: "text", Text: "turn2"}}},
	}

	lines1 := r.Render(turn1)
	lines2 := r.Render(turn2)

	if len(lines1) != 1 || lines1[0] != "turn1" {
		t.Errorf("turn1: got %v", lines1)
	}
	// Turn 2 should be prepended with a blank separator line.
	if len(lines2) != 2 {
		t.Fatalf("turn2 expected 2 lines (blank + content), got %d: %v", len(lines2), lines2)
	}
	if lines2[0] != "" {
		t.Errorf("turn2[0] should be blank, got %q", lines2[0])
	}
	if lines2[1] != "turn2" {
		t.Errorf("turn2[1] should be turn2, got %q", lines2[1])
	}
}

func TestRenderer_AssistantToolUse(t *testing.T) {
	input, _ := json.Marshal(map[string]string{"command": "ls -la"})
	r := &claudestream.Renderer{}
	ev := &claudestream.AssistantEvent{
		Type: "assistant",
		Message: claudestream.AssistantMsg{
			Content: []claudestream.ContentBlock{
				{Type: "tool_use", Name: "Bash", Input: input},
			},
		},
	}
	lines := r.Render(ev)
	if len(lines) != 1 {
		t.Fatalf("expected 1 line, got %d: %v", len(lines), lines)
	}
	if !strings.HasPrefix(lines[0], "→ Bash ") {
		t.Errorf("expected tool indicator prefix, got %q", lines[0])
	}
	if !strings.Contains(lines[0], "ls -la") {
		t.Errorf("expected command summary, got %q", lines[0])
	}
}

func TestRenderer_AssistantThinkingSkipped(t *testing.T) {
	r := &claudestream.Renderer{}
	ev := &claudestream.AssistantEvent{
		Type: "assistant",
		Message: claudestream.AssistantMsg{
			Content: []claudestream.ContentBlock{
				{Type: "thinking", Text: "internal thoughts"},
			},
		},
	}
	lines := r.Render(ev)
	if len(lines) != 0 {
		t.Errorf("expected no lines for thinking block, got %v", lines)
	}
}

func TestRenderer_UserEventEmpty(t *testing.T) {
	r := &claudestream.Renderer{}
	ev := &claudestream.UserEvent{Type: "user"}
	lines := r.Render(ev)
	if len(lines) != 0 {
		t.Errorf("expected no lines for user event, got %v", lines)
	}
}

func TestRenderer_ResultEventEmpty(t *testing.T) {
	r := &claudestream.Renderer{}
	ev := &claudestream.ResultEvent{Type: "result"}
	lines := r.Render(ev)
	if len(lines) != 0 {
		t.Errorf("expected no lines for result event, got %v", lines)
	}
}

func TestRenderer_RateLimitAllowed(t *testing.T) {
	r := &claudestream.Renderer{}
	ev := &claudestream.RateLimitEvent{
		Type:          "rate_limit_event",
		RateLimitInfo: claudestream.RateLimitInfo{Status: "allowed"},
	}
	lines := r.Render(ev)
	if len(lines) != 0 {
		t.Errorf("expected no lines for allowed rate limit, got %v", lines)
	}
}

func TestRenderer_RateLimitNonAllowed(t *testing.T) {
	r := &claudestream.Renderer{}
	ev := &claudestream.RateLimitEvent{
		Type: "rate_limit_event",
		RateLimitInfo: claudestream.RateLimitInfo{
			Status:        "warning",
			RateLimitType: "five_hour",
			ResetsAt:      1776272400,
		},
	}
	lines := r.Render(ev)
	if len(lines) != 1 {
		t.Fatalf("expected 1 warning line, got %d: %v", len(lines), lines)
	}
	if !strings.HasPrefix(lines[0], "⚠ rate limit five_hour: warning") {
		t.Errorf("unexpected warning line: %q", lines[0])
	}
}

// TestRenderer_ToolSummaryTable verifies per-tool field selection (D12).
func TestRenderer_ToolSummaryTable(t *testing.T) {
	tests := []struct {
		toolName    string
		inputJSON   string
		wantContain string
	}{
		{"Bash", `{"command":"go test ./..."}`, "go test ./..."},
		{"Read", `{"file_path":"/foo/bar.go"}`, "/foo/bar.go"},
		{"Edit", `{"file_path":"/baz.go","old_string":"x","new_string":"y"}`, "/baz.go"},
		{"Write", `{"file_path":"/out.txt","content":"hi"}`, "/out.txt"},
		{"NotebookEdit", `{"file_path":"/nb.ipynb"}`, "/nb.ipynb"},
		{"Glob", `{"pattern":"**/*.go"}`, "**/*.go"},
		{"Grep", `{"pattern":"func Foo"}`, "func Foo"},
		{"Task", `{"description":"run tests"}`, "run tests"},
		{"Agent", `{"description":"explore codebase"}`, "explore codebase"},
		{"WebFetch", `{"url":"https://example.com"}`, "https://example.com"},
		// Unknown tool → compact JSON fallback
		{"UnknownTool", `{"arbitrary":"field"}`, `"arbitrary"`},
	}

	for _, tc := range tests {
		t.Run(tc.toolName, func(t *testing.T) {
			r := &claudestream.Renderer{}
			ev := &claudestream.AssistantEvent{
				Type: "assistant",
				Message: claudestream.AssistantMsg{
					Content: []claudestream.ContentBlock{
						{Type: "tool_use", Name: tc.toolName, Input: json.RawMessage(tc.inputJSON)},
					},
				},
			}
			lines := r.Render(ev)
			if len(lines) != 1 {
				t.Fatalf("expected 1 line, got %d: %v", len(lines), lines)
			}
			if !strings.Contains(lines[0], tc.wantContain) {
				t.Errorf("line %q does not contain %q", lines[0], tc.wantContain)
			}
		})
	}
}

// TestRenderer_ToolSummaryTruncation verifies long summaries are truncated to ≤80 runes + "…".
func TestRenderer_ToolSummaryTruncation(t *testing.T) {
	longCommand := strings.Repeat("x", 100)
	input, _ := json.Marshal(map[string]string{"command": longCommand})
	r := &claudestream.Renderer{}
	ev := &claudestream.AssistantEvent{
		Type: "assistant",
		Message: claudestream.AssistantMsg{
			Content: []claudestream.ContentBlock{
				{Type: "tool_use", Name: "Bash", Input: input},
			},
		},
	}
	lines := r.Render(ev)
	if len(lines) != 1 {
		t.Fatalf("expected 1 line, got %d", len(lines))
	}
	// "→ Bash " prefix + 80 summary chars + "…"
	indicator := lines[0]
	// The summary portion (after "→ Bash ") should be truncated.
	if !strings.HasSuffix(indicator, "…") {
		t.Errorf("expected truncated summary ending in …, got %q", indicator)
	}
}

func TestRenderer_Finalize(t *testing.T) {
	r := &claudestream.Renderer{}
	stats := claudestream.StepStats{
		NumTurns:            3,
		InputTokens:         100,
		OutputTokens:        50,
		CacheCreationTokens: 200,
		CacheReadTokens:     400,
		TotalCostUSD:        0.0012345,
		DurationMS:          2500,
	}
	lines := r.Finalize(stats)
	if len(lines) != 1 {
		t.Fatalf("expected 1 line, got %d: %v", len(lines), lines)
	}
	line := lines[0]
	// Verify key fragments are present.
	for _, fragment := range []string{"3 turns", "100/50 tokens", "cache: 200/400", "$0.0012345", "2.5s"} {
		if !strings.Contains(line, fragment) {
			t.Errorf("Finalize line %q missing fragment %q", line, fragment)
		}
	}
}
