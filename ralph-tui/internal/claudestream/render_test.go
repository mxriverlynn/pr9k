package claudestream_test

import (
	"encoding/json"
	"strings"
	"testing"
	"unicode/utf8"

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
	// "→ Bash " prefix + 80 summary runes + "…"
	indicator := lines[0]
	// The summary portion (after "→ Bash ") should be truncated.
	if !strings.HasSuffix(indicator, "…") {
		t.Errorf("expected truncated summary ending in …, got %q", indicator)
	}
	summary := strings.TrimPrefix(indicator, "→ Bash ")
	// 80 runes of content + 1 rune "…" = 81 total runes.
	if got := utf8.RuneCountInString(summary); got != 81 {
		t.Errorf("expected 81-rune summary (80 + …), got %d runes: %q", got, summary)
	}
}

// TestRenderer_SystemUnknownSubtype verifies the default branch of renderSystem
// returns nil for an unrecognised subtype (TP-R1, D8).
func TestRenderer_SystemUnknownSubtype(t *testing.T) {
	r := &claudestream.Renderer{}
	ev := &claudestream.SystemEvent{Type: "system", Subtype: "future_thing"}
	lines := r.Render(ev)
	if lines != nil {
		t.Errorf("expected nil for unknown system subtype, got %v", lines)
	}
}

// TestRenderer_AssistantEmptyContent verifies an assistant event with an empty
// content slice produces no lines and does not panic (TP-R2).
func TestRenderer_AssistantEmptyContent(t *testing.T) {
	r := &claudestream.Renderer{}
	ev := &claudestream.AssistantEvent{
		Type:    "assistant",
		Message: claudestream.AssistantMsg{Content: []claudestream.ContentBlock{}},
	}
	lines := r.Render(ev)
	if len(lines) != 0 {
		t.Errorf("expected no lines for empty content, got %v", lines)
	}
}

// TestRenderer_ToolSummaryMissingField verifies that a Bash tool_use without
// the "command" field falls back to compact JSON of the full input (TP-R3).
func TestRenderer_ToolSummaryMissingField(t *testing.T) {
	r := &claudestream.Renderer{}
	ev := &claudestream.AssistantEvent{
		Type: "assistant",
		Message: claudestream.AssistantMsg{
			Content: []claudestream.ContentBlock{
				{Type: "tool_use", Name: "Bash", Input: json.RawMessage(`{"other":"value"}`)},
			},
		},
	}
	lines := r.Render(ev)
	if len(lines) != 1 {
		t.Fatalf("expected 1 line, got %d: %v", len(lines), lines)
	}
	// Falls back to compact JSON containing the actual key.
	if !strings.Contains(lines[0], `"other"`) {
		t.Errorf("expected compact JSON fallback containing 'other', got %q", lines[0])
	}
}

// TestRenderer_ToolSummaryEmptyInput verifies that a tool_use with input `{}`
// does not panic and produces a tool indicator line (TP-R4).
func TestRenderer_ToolSummaryEmptyInput(t *testing.T) {
	r := &claudestream.Renderer{}
	ev := &claudestream.AssistantEvent{
		Type: "assistant",
		Message: claudestream.AssistantMsg{
			Content: []claudestream.ContentBlock{
				{Type: "tool_use", Name: "Bash", Input: json.RawMessage(`{}`)},
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
}

// TestRenderer_ToolSummaryNonStringField verifies that a Bash tool_use with a
// non-string "command" value falls back gracefully via strings.Trim (TP-R5).
func TestRenderer_ToolSummaryNonStringField(t *testing.T) {
	r := &claudestream.Renderer{}
	ev := &claudestream.AssistantEvent{
		Type: "assistant",
		Message: claudestream.AssistantMsg{
			Content: []claudestream.ContentBlock{
				{Type: "tool_use", Name: "Bash", Input: json.RawMessage(`{"command":42}`)},
			},
		},
	}
	lines := r.Render(ev)
	if len(lines) != 1 {
		t.Fatalf("expected 1 line, got %d: %v", len(lines), lines)
	}
	// json.Unmarshal of the numeric 42 into string fails; falls back to
	// strings.Trim(string(val), `"`), which produces "42".
	if !strings.Contains(lines[0], "42") {
		t.Errorf("expected fallback to contain numeric value 42, got %q", lines[0])
	}
}

// TestRenderer_AssistantMultiBlock verifies an assistant event with text +
// tool_use + thinking blocks in one message: text and tool indicator are
// rendered, thinking is silently dropped (TP-R6, D19).
func TestRenderer_AssistantMultiBlock(t *testing.T) {
	input, _ := json.Marshal(map[string]string{"command": "ls"})
	r := &claudestream.Renderer{}
	ev := &claudestream.AssistantEvent{
		Type: "assistant",
		Message: claudestream.AssistantMsg{
			Content: []claudestream.ContentBlock{
				{Type: "text", Text: "Analyzing the files."},
				{Type: "tool_use", Name: "Bash", Input: input},
				{Type: "thinking", Text: "internal reasoning"},
			},
		},
	}
	lines := r.Render(ev)
	// Expect text line + tool indicator; thinking is skipped.
	if len(lines) != 2 {
		t.Fatalf("expected 2 lines (text + tool indicator), got %d: %v", len(lines), lines)
	}
	if lines[0] != "Analyzing the files." {
		t.Errorf("lines[0]: got %q, want text content", lines[0])
	}
	if !strings.HasPrefix(lines[1], "→ Bash ") {
		t.Errorf("lines[1]: expected tool indicator prefix, got %q", lines[1])
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

// TestRenderer_FinalizeZeroStats verifies that Finalize with a zero-valued
// StepStats does not panic and produces a valid format string (TP-R7).
func TestRenderer_FinalizeZeroStats(t *testing.T) {
	r := &claudestream.Renderer{}
	lines := r.Finalize(claudestream.StepStats{})
	if len(lines) != 1 {
		t.Fatalf("expected 1 line, got %d: %v", len(lines), lines)
	}
	if !strings.Contains(lines[0], "0 turns") {
		t.Errorf("expected '0 turns' in zero stats line, got %q", lines[0])
	}
}

// --- FinalizeRun tests (D13 2c) ---

// TestRenderer_FinalizeRun_ZeroInvocations verifies that FinalizeRun returns nil
// when no claude steps ran (invocations == 0), suppressing the summary line.
func TestRenderer_FinalizeRun_ZeroInvocations(t *testing.T) {
	var r claudestream.Renderer
	lines := r.FinalizeRun(0, 0, claudestream.StepStats{})
	if lines != nil {
		t.Errorf("expected nil for zero invocations, got %v", lines)
	}
}

// TestRenderer_FinalizeRun_NoRetries verifies the summary line for multiple
// invocations with no retries omits the retries parenthetical.
func TestRenderer_FinalizeRun_NoRetries(t *testing.T) {
	var r claudestream.Renderer
	total := claudestream.StepStats{
		NumTurns:            5,
		InputTokens:         200,
		OutputTokens:        100,
		CacheCreationTokens: 50,
		CacheReadTokens:     300,
		TotalCostUSD:        0.0056789,
		DurationMS:          10000,
	}
	lines := r.FinalizeRun(3, 0, total)
	if len(lines) != 1 {
		t.Fatalf("expected 1 line, got %d: %v", len(lines), lines)
	}
	line := lines[0]
	for _, fragment := range []string{
		"total claude spend across 3 step invocations",
		"5 turns",
		"200/100 tokens",
		"cache: 50/300",
		"$0.0056789",
		"10s",
	} {
		if !strings.Contains(line, fragment) {
			t.Errorf("FinalizeRun line %q missing fragment %q", line, fragment)
		}
	}
	// No retries parenthetical.
	if strings.Contains(line, "retr") {
		t.Errorf("FinalizeRun line %q should not contain retry info when retries=0", line)
	}
}

// TestRenderer_FinalizeRun_WithRetries verifies the summary line includes the
// retries parenthetical when retries > 0.
func TestRenderer_FinalizeRun_WithRetries(t *testing.T) {
	var r claudestream.Renderer
	lines := r.FinalizeRun(4, 2, claudestream.StepStats{NumTurns: 8, TotalCostUSD: 0.01})
	if len(lines) != 1 {
		t.Fatalf("expected 1 line, got %d: %v", len(lines), lines)
	}
	line := lines[0]
	if !strings.Contains(line, "4 step invocations") {
		t.Errorf("FinalizeRun line %q missing invocation count", line)
	}
	if !strings.Contains(line, "including 2 retries") {
		t.Errorf("FinalizeRun line %q missing retries clause", line)
	}
}

// TestRenderer_FinalizeRun_SingleInvocationSingular verifies singular
// "invocation" (not "invocations") when invocations == 1.
func TestRenderer_FinalizeRun_SingleInvocationSingular(t *testing.T) {
	var r claudestream.Renderer
	lines := r.FinalizeRun(1, 0, claudestream.StepStats{})
	if len(lines) != 1 {
		t.Fatalf("expected 1 line, got %d: %v", len(lines), lines)
	}
	if !strings.Contains(lines[0], "1 step invocation") {
		t.Errorf("expected singular 'invocation' in %q", lines[0])
	}
	if strings.Contains(lines[0], "1 step invocations") {
		t.Errorf("unexpected plural 'invocations' in %q", lines[0])
	}
}

// TestRenderer_FinalizeRun_SingleRetrySingular verifies singular "retry" (not
// "retries") when retries == 1.
func TestRenderer_FinalizeRun_SingleRetrySingular(t *testing.T) {
	var r claudestream.Renderer
	lines := r.FinalizeRun(2, 1, claudestream.StepStats{})
	if len(lines) != 1 {
		t.Fatalf("expected 1 line, got %d: %v", len(lines), lines)
	}
	if !strings.Contains(lines[0], "including 1 retry") {
		t.Errorf("expected singular 'retry' in %q", lines[0])
	}
	if strings.Contains(lines[0], "including 1 retries") {
		t.Errorf("unexpected plural 'retries' in %q", lines[0])
	}
}
