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
