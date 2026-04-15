package claudestream_test

import (
	"strings"
	"testing"

	"github.com/mxriverlynn/pr9k/ralph-tui/internal/claudestream"
)

func feed(a *claudestream.Aggregator, events ...claudestream.Event) {
	for _, ev := range events {
		a.Observe(ev)
	}
}

func TestAggregator_SuccessPath(t *testing.T) {
	a := &claudestream.Aggregator{}
	feed(a,
		&claudestream.AssistantEvent{
			Type: "assistant",
			Message: claudestream.AssistantMsg{
				Usage: claudestream.Usage{InputTokens: 10, OutputTokens: 5},
			},
		},
		&claudestream.ResultEvent{
			Type:         "result",
			Subtype:      "success",
			IsError:      false,
			Result:       "Hello there, friend.",
			NumTurns:     1,
			TotalCostUSD: 0.01,
			DurationMS:   1200,
			SessionID:    "ses1",
			Usage: claudestream.Usage{
				InputTokens:              3,
				OutputTokens:             8,
				CacheCreationInputTokens: 4964,
				CacheReadInputTokens:     11991,
			},
		},
	)

	if err := a.Err(); err != nil {
		t.Fatalf("Err() should be nil, got: %v", err)
	}
	if a.Result() != "Hello there, friend." {
		t.Errorf("Result(): got %q, want %q", a.Result(), "Hello there, friend.")
	}
	stats := a.Stats()
	if stats.NumTurns != 1 {
		t.Errorf("NumTurns: got %d, want 1", stats.NumTurns)
	}
	if stats.TotalCostUSD != 0.01 {
		t.Errorf("TotalCostUSD: got %f", stats.TotalCostUSD)
	}
	if stats.SessionID != "ses1" {
		t.Errorf("SessionID: got %q", stats.SessionID)
	}
	// Result event overrides the tally from assistant events.
	if stats.InputTokens != 3 {
		t.Errorf("InputTokens: got %d, want 3", stats.InputTokens)
	}
	if stats.OutputTokens != 8 {
		t.Errorf("OutputTokens: got %d, want 8", stats.OutputTokens)
	}
	if stats.CacheCreationTokens != 4964 {
		t.Errorf("CacheCreationTokens: got %d, want 4964", stats.CacheCreationTokens)
	}
	if stats.CacheReadTokens != 11991 {
		t.Errorf("CacheReadTokens: got %d, want 11991", stats.CacheReadTokens)
	}
}

func TestAggregator_IsErrorTrue(t *testing.T) {
	a := &claudestream.Aggregator{}
	longResult := strings.Repeat("x", 300)
	feed(a,
		&claudestream.ResultEvent{
			Type:       "result",
			Subtype:    "success",
			IsError:    true,
			Result:     longResult,
			SessionID:  "ses-err",
			StopReason: "stop_sequence",
		},
	)

	err := a.Err()
	if err == nil {
		t.Fatal("Err() should be non-nil when is_error=true")
	}
	msg := err.Error()
	if !strings.Contains(msg, "is_error=true") {
		t.Errorf("error message should mention is_error=true: %q", msg)
	}
	if !strings.Contains(msg, "ses-err") {
		t.Errorf("error message should contain session id: %q", msg)
	}
	// Result is truncated to 200 chars in the message.
	if strings.Contains(msg, longResult) {
		t.Error("error message should NOT contain the full 300-char result")
	}
	// Message should contain the 200-char prefix.
	if !strings.Contains(msg, strings.Repeat("x", 200)) {
		t.Error("error message should contain 200-char truncated result")
	}
}

func TestAggregator_NoResultEvent(t *testing.T) {
	a := &claudestream.Aggregator{}
	// Feed only an assistant event — no result event.
	feed(a, &claudestream.AssistantEvent{
		Type: "assistant",
		Message: claudestream.AssistantMsg{
			Content: []claudestream.ContentBlock{{Type: "text", Text: "hi"}},
		},
	})

	err := a.Err()
	if err == nil {
		t.Fatal("Err() should be non-nil when no result event observed")
	}
	if !strings.Contains(err.Error(), "no result event") {
		t.Errorf("unexpected error message: %q", err.Error())
	}
}

func TestAggregator_RateLimitRecorded(t *testing.T) {
	a := &claudestream.Aggregator{}
	feed(a,
		&claudestream.RateLimitEvent{
			Type: "rate_limit_event",
			RateLimitInfo: claudestream.RateLimitInfo{
				Status:        "warning",
				RateLimitType: "five_hour",
			},
		},
		&claudestream.ResultEvent{
			Type: "result", IsError: false, Result: "ok",
		},
	)

	stats := a.Stats()
	if stats.LastRateLimitInfo == nil {
		t.Fatal("LastRateLimitInfo should be set")
	}
	if stats.LastRateLimitInfo.Status != "warning" {
		t.Errorf("LastRateLimitInfo.Status: got %q, want warning", stats.LastRateLimitInfo.Status)
	}
}

func TestAggregator_RateLimitAllowedAlsoRecorded(t *testing.T) {
	a := &claudestream.Aggregator{}
	feed(a,
		&claudestream.RateLimitEvent{
			Type:          "rate_limit_event",
			RateLimitInfo: claudestream.RateLimitInfo{Status: "allowed"},
		},
		&claudestream.ResultEvent{Type: "result", IsError: false, Result: "ok"},
	)
	stats := a.Stats()
	if stats.LastRateLimitInfo == nil {
		t.Fatal("LastRateLimitInfo should be set even for allowed status")
	}
	if stats.LastRateLimitInfo.Status != "allowed" {
		t.Errorf("unexpected status: %q", stats.LastRateLimitInfo.Status)
	}
}

func TestAggregator_EmptyAggregate(t *testing.T) {
	a := &claudestream.Aggregator{}
	// No events at all.
	if err := a.Err(); err == nil {
		t.Fatal("fresh Aggregator with no events should return an error")
	}
	if a.Result() != "" {
		t.Errorf("Result() should be empty, got %q", a.Result())
	}
}

// TestAggregator_ObserveIgnoresSystemAndUser verifies that SystemEvent and
// UserEvent are no-ops: no state change, no panic (TP-A1).
func TestAggregator_ObserveIgnoresSystemAndUser(t *testing.T) {
	a := &claudestream.Aggregator{}
	feed(a,
		&claudestream.SystemEvent{Type: "system", Subtype: "init", SessionID: "s1"},
		&claudestream.UserEvent{Type: "user"},
	)
	stats := a.Stats()
	if stats.InputTokens != 0 || stats.OutputTokens != 0 {
		t.Errorf("unexpected token accumulation from system/user events: %+v", stats)
	}
	if stats.SessionID != "" {
		t.Errorf("SessionID should be empty, got %q", stats.SessionID)
	}
}

// TestAggregator_MultiAssistantThenResult verifies that when multiple assistant
// events accumulate a running tally and then a result event arrives, Stats()
// reflects the result event's token counts (not the sum of assistant events)
// (TP-A2, D13).
func TestAggregator_MultiAssistantThenResult(t *testing.T) {
	a := &claudestream.Aggregator{}
	feed(a,
		&claudestream.AssistantEvent{
			Type:    "assistant",
			Message: claudestream.AssistantMsg{Usage: claudestream.Usage{InputTokens: 10, OutputTokens: 5}},
		},
		&claudestream.AssistantEvent{
			Type:    "assistant",
			Message: claudestream.AssistantMsg{Usage: claudestream.Usage{InputTokens: 20, OutputTokens: 15}},
		},
		&claudestream.AssistantEvent{
			Type:    "assistant",
			Message: claudestream.AssistantMsg{Usage: claudestream.Usage{InputTokens: 30, OutputTokens: 25}},
		},
		&claudestream.ResultEvent{
			Type:         "result",
			IsError:      false,
			Result:       "done",
			NumTurns:     3,
			TotalCostUSD: 0.05,
			Usage: claudestream.Usage{
				InputTokens:  7,
				OutputTokens: 12,
			},
		},
	)
	if err := a.Err(); err != nil {
		t.Fatalf("Err() should be nil: %v", err)
	}
	stats := a.Stats()
	// Result event overrides the running tally; should NOT be sum (60/45) of
	// the three assistant events.
	if stats.InputTokens != 7 {
		t.Errorf("InputTokens: got %d, want 7 (from result event, not 60)", stats.InputTokens)
	}
	if stats.OutputTokens != 12 {
		t.Errorf("OutputTokens: got %d, want 12 (from result event, not 45)", stats.OutputTokens)
	}
	if stats.NumTurns != 3 {
		t.Errorf("NumTurns: got %d, want 3", stats.NumTurns)
	}
}

// TestAggregator_ErrIncludesSubtypeAndStopReason verifies that Err() includes
// subtype and stop_reason in the error message (TP-A3, D15).
func TestAggregator_ErrIncludesSubtypeAndStopReason(t *testing.T) {
	a := &claudestream.Aggregator{}
	feed(a, &claudestream.ResultEvent{
		Type:       "result",
		IsError:    true,
		Result:     "failed",
		SessionID:  "ses-xyz",
		Subtype:    "error",
		StopReason: "max_tokens",
	})
	err := a.Err()
	if err == nil {
		t.Fatal("Err() should be non-nil")
	}
	msg := err.Error()
	if !strings.Contains(msg, "subtype=error") {
		t.Errorf("error message should contain subtype, got %q", msg)
	}
	if !strings.Contains(msg, "stop=max_tokens") {
		t.Errorf("error message should contain stop_reason, got %q", msg)
	}
}

// TestAggregator_ErrTruncationBoundary verifies the 200-rune truncation guard:
// a result string of exactly 200 runes must NOT be truncated in Err() (TP-A4).
func TestAggregator_ErrTruncationBoundary(t *testing.T) {
	a := &claudestream.Aggregator{}
	exactly200 := strings.Repeat("a", 200)
	feed(a, &claudestream.ResultEvent{
		Type:      "result",
		IsError:   true,
		Result:    exactly200,
		SessionID: "ses1",
	})
	err := a.Err()
	if err == nil {
		t.Fatal("Err() should be non-nil")
	}
	// The full 200-rune string must appear verbatim (≤200 means no truncation).
	if !strings.Contains(err.Error(), exactly200) {
		t.Errorf("200-rune result should not be truncated in error message")
	}
}
