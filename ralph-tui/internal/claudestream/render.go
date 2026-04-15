package claudestream

import (
	"encoding/json"
	"fmt"
	"strings"
	"time"
	"unicode/utf8"
)

const toolSummaryMaxLen = 80

// Renderer converts typed events into human-readable display lines for the
// TUI log panel and file logger (D11, D12, D19, D28).
//
// Renderer is not safe for concurrent use. It is owned by a single goroutine
// (the stdout-forwarding goroutine in runCommand).
type Renderer struct {
	sawAssistant bool
}

// Render returns zero or more display lines for the given event.
// Rules per D11/D12/D19/D28:
//   - SystemEvent init    → one banner line
//   - SystemEvent api_retry → one warning line
//   - AssistantEvent text → each \n-split line verbatim (blank inter-turn separator prepended on 2nd+ turns)
//   - AssistantEvent tool_use → one indicator line
//   - AssistantEvent thinking → nothing
//   - UserEvent → nothing
//   - RateLimitEvent status==allowed → nothing; otherwise one warning line
//   - ResultEvent → nothing (summary comes from Finalize)
func (r *Renderer) Render(ev Event) []string {
	switch e := ev.(type) {
	case *SystemEvent:
		return r.renderSystem(e)
	case *AssistantEvent:
		return r.renderAssistant(e)
	case *UserEvent:
		return nil
	case *ResultEvent:
		return nil
	case *RateLimitEvent:
		return r.renderRateLimit(e)
	default:
		return nil
	}
}

// Finalize returns the single closing summary line for the step (D13 2a).
// Format: "<turns> turns · <in>/<out> tokens (cache: <creation>/<read>) · $<cost> · <duration>"
func (r *Renderer) Finalize(stats StepStats) []string {
	dur := time.Duration(stats.DurationMS) * time.Millisecond
	line := fmt.Sprintf(
		"%d turns · %d/%d tokens (cache: %d/%d) · $%.7f · %s",
		stats.NumTurns,
		stats.InputTokens,
		stats.OutputTokens,
		stats.CacheCreationTokens,
		stats.CacheReadTokens,
		stats.TotalCostUSD,
		dur,
	)
	return []string{line}
}

// FinalizeRun returns the run-level cumulative summary line (D13 2c).
// Format: "total claude spend across N step invocations[ (including R retries)]:
//
//	<turns> turns · <in>/<out> tokens (cache: <creation>/<read>) · $<cost> · <duration>"
//
// Returns nil when invocations is zero (no claude steps ran this run).
// FinalizeRun is a value-receiver method (uses no Renderer state) so it can be
// called on a zero-value Renderer{} at the Run() call site.
func (r Renderer) FinalizeRun(invocations, retries int, total StepStats) []string {
	if invocations == 0 {
		return nil
	}
	invLabel := "invocation"
	if invocations != 1 {
		invLabel = "invocations"
	}
	prefix := fmt.Sprintf("total claude spend across %d step %s", invocations, invLabel)
	if retries > 0 {
		retryLabel := "retry"
		if retries != 1 {
			retryLabel = "retries"
		}
		prefix += fmt.Sprintf(" (including %d %s)", retries, retryLabel)
	}
	dur := time.Duration(total.DurationMS) * time.Millisecond
	line := fmt.Sprintf(
		"%s: %d turns · %d/%d tokens (cache: %d/%d) · $%.7f · %s",
		prefix,
		total.NumTurns,
		total.InputTokens,
		total.OutputTokens,
		total.CacheCreationTokens,
		total.CacheReadTokens,
		total.TotalCostUSD,
		dur,
	)
	return []string{line}
}

func (r *Renderer) renderSystem(e *SystemEvent) []string {
	switch e.Subtype {
	case "init":
		return []string{fmt.Sprintf("[claude session %s started, model %s]", e.SessionID, e.Model)}
	case "api_retry":
		return []string{fmt.Sprintf("⚠ retry %d/%d in %dms — %s", e.Attempt, e.MaxRetries, e.RetryDelayMS, e.Error)}
	default:
		return nil
	}
}

func (r *Renderer) renderAssistant(e *AssistantEvent) []string {
	var lines []string

	// Blank line between turns (D19): if we've already seen an assistant
	// event, prepend an empty separator before this turn's content.
	if r.sawAssistant {
		lines = append(lines, "")
	}
	r.sawAssistant = true

	for _, block := range e.Message.Content {
		switch block.Type {
		case "text":
			// Split on newlines and emit each part as its own line; preserve
			// empty lines as empty strings (D19 inner spacing).
			parts := strings.Split(block.Text, "\n")
			lines = append(lines, parts...)
		case "tool_use":
			lines = append(lines, "→ "+block.Name+" "+toolSummary(block.Name, block.Input))
		case "thinking":
			// Not displayed (D11).
		}
	}

	return lines
}

func (r *Renderer) renderRateLimit(e *RateLimitEvent) []string {
	if e.RateLimitInfo.Status == "allowed" {
		return nil
	}
	// ResetsAt is a Unix timestamp in seconds (verified against claude CLI output).
	resetTime := time.Unix(e.RateLimitInfo.ResetsAt, 0).Local().Format("15:04:05")
	line := fmt.Sprintf("⚠ rate limit %s: %s (resets %s)",
		e.RateLimitInfo.RateLimitType,
		e.RateLimitInfo.Status,
		resetTime,
	)
	return []string{line}
}

// toolSummary extracts the most useful field from a tool_use block's input
// and returns it truncated to toolSummaryMaxLen runes (D12).
func toolSummary(toolName string, input json.RawMessage) string {
	if len(input) == 0 {
		return ""
	}

	var raw map[string]json.RawMessage
	if err := json.Unmarshal(input, &raw); err != nil {
		return truncate(string(input))
	}

	var field string
	switch toolName {
	case "Bash":
		field = "command"
	case "Read", "Edit", "Write", "NotebookEdit":
		field = "file_path"
	case "Glob", "Grep":
		field = "pattern"
	case "Task", "Agent":
		field = "description"
	case "WebFetch":
		field = "url"
	default:
		// Compact JSON of the full input as fallback.
		b, err := json.Marshal(raw)
		if err != nil {
			return truncate(string(input))
		}
		return truncate(string(b))
	}

	val, ok := raw[field]
	if !ok {
		// Field not present — fall back to compact JSON.
		b, err := json.Marshal(raw)
		if err != nil {
			return truncate(string(input))
		}
		return truncate(string(b))
	}

	// Unquote the JSON string value.
	var s string
	if err := json.Unmarshal(val, &s); err != nil {
		return truncate(strings.Trim(string(val), `"`))
	}
	return truncate(s)
}

// truncate clips s to toolSummaryMaxLen runes, appending "…" if clipped.
func truncate(s string) string {
	if utf8.RuneCountInString(s) <= toolSummaryMaxLen {
		return s
	}
	runes := []rune(s)
	return string(runes[:toolSummaryMaxLen]) + "…"
}
