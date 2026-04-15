package claudestream

import (
	"fmt"
	"unicode/utf8"
)

// Aggregator accumulates state across a single claude step invocation.
//
// Aggregator is not safe for concurrent use. It is owned by the
// stdout-forwarding goroutine and read only after the subprocess exits.
type Aggregator struct {
	result     string
	stats      StepStats
	hasResult  bool
	isError    bool
	subtype    string
	stopReason string
}

// Observe folds one parsed event into the aggregator's internal state.
// nil events are silently ignored (fall through the type switch).
func (a *Aggregator) Observe(ev Event) {
	switch e := ev.(type) {
	case *AssistantEvent:
		a.stats.InputTokens += e.Message.Usage.InputTokens
		a.stats.OutputTokens += e.Message.Usage.OutputTokens
		a.stats.CacheCreationTokens += e.Message.Usage.CacheCreationInputTokens
		a.stats.CacheReadTokens += e.Message.Usage.CacheReadInputTokens
	case *ResultEvent:
		a.hasResult = true
		a.result = e.Result
		a.isError = e.IsError
		a.subtype = e.Subtype
		a.stopReason = e.StopReason
		a.stats.NumTurns = e.NumTurns
		a.stats.TotalCostUSD = e.TotalCostUSD
		a.stats.DurationMS = e.DurationMS
		a.stats.SessionID = e.SessionID
		// The result event carries cumulative usage totals; prefer them over
		// the running tally accumulated from individual assistant events.
		a.stats.InputTokens = e.Usage.InputTokens
		a.stats.OutputTokens = e.Usage.OutputTokens
		a.stats.CacheCreationTokens = e.Usage.CacheCreationInputTokens
		a.stats.CacheReadTokens = e.Usage.CacheReadInputTokens
	case *RateLimitEvent:
		info := e.RateLimitInfo
		a.stats.LastRateLimitInfo = &info
	}
}

// Result returns the final assistant text from the result event's "result"
// field (D6 captureAs semantics).
func (a *Aggregator) Result() string {
	return a.result
}

// Stats returns the accumulated step statistics.
func (a *Aggregator) Stats() StepStats {
	return a.stats
}

// Err returns non-nil in two situations (D15):
//
//  1. result.is_error == true — the error message includes a truncated form of
//     result.result and session_id for log correlation.
//
//  2. No result event was ever observed (stream truncated) — returns the
//     "no result event" sentinel message.
func (a *Aggregator) Err() error {
	if !a.hasResult {
		return fmt.Errorf("claude step produced no result event")
	}
	if a.isError {
		snippet := a.result
		if utf8.RuneCountInString(snippet) > 200 {
			runes := []rune(snippet)
			snippet = string(runes[:200])
		}
		return fmt.Errorf(
			"claude step ended with is_error=true: %s (session=%s, subtype=%s, stop=%s)",
			snippet, a.stats.SessionID, a.subtype, a.stopReason,
		)
	}
	return nil
}
