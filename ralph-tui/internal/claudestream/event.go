// Package claudestream parses, renders, aggregates, and persists the
// newline-delimited JSON stream emitted by claude -p --output-format
// stream-json --verbose.
package claudestream

import "encoding/json"

// Event is the common interface for all parsed stream events.
type Event interface {
	eventType() string
}

// SystemEvent covers both "init" and "api_retry" subtypes.
type SystemEvent struct {
	Type      string `json:"type"`
	Subtype   string `json:"subtype"`
	SessionID string `json:"session_id"`
	// init fields
	Model string `json:"model"`
	// api_retry fields
	Attempt      int    `json:"attempt"`
	MaxRetries   int    `json:"max_retries"`
	RetryDelayMS int    `json:"retry_delay_ms"`
	ErrorStatus  int    `json:"error_status"`
	Error        string `json:"error"`
	UUID         string `json:"uuid"`
}

func (e *SystemEvent) eventType() string { return e.Type }

// AssistantEvent is one complete assistant turn.
type AssistantEvent struct {
	Type    string       `json:"type"`
	Message AssistantMsg `json:"message"`
	// Error is a top-level field present on some failure paths
	// (e.g. "authentication_failed"). Not rendered; the authoritative
	// failure signal is result.is_error (D15).
	Error     string `json:"error"`
	SessionID string `json:"session_id"`
	UUID      string `json:"uuid"`
}

func (e *AssistantEvent) eventType() string { return e.Type }

// AssistantMsg is the message payload inside an AssistantEvent.
type AssistantMsg struct {
	ID      string         `json:"id"`
	Model   string         `json:"model"`
	Content []ContentBlock `json:"content"`
	Usage   Usage          `json:"usage"`
}

// Usage holds token counters from a message.
type Usage struct {
	InputTokens              int `json:"input_tokens"`
	OutputTokens             int `json:"output_tokens"`
	CacheCreationInputTokens int `json:"cache_creation_input_tokens"`
	CacheReadInputTokens     int `json:"cache_read_input_tokens"`
}

// ContentBlock is a discriminated union on the "type" field.
// Only the fields relevant to each type are populated.
type ContentBlock struct {
	Type string `json:"type"`
	// text block
	Text string `json:"text"`
	// tool_use block
	Name  string          `json:"name"`
	Input json.RawMessage `json:"input"`
	// tool_result block
	ToolUseID string `json:"tool_use_id"`
	// Content may be a string or an array; we capture the raw form.
	Content json.RawMessage `json:"content"`
	// thinking block — no extra fields rendered
}

// UserEvent carries tool results being fed back to the model.
type UserEvent struct {
	Type    string  `json:"type"`
	Message UserMsg `json:"message"`
	UUID    string  `json:"uuid"`
}

func (e *UserEvent) eventType() string { return e.Type }

// UserMsg is the message payload inside a UserEvent.
type UserMsg struct {
	Content []ContentBlock `json:"content"`
}

// ResultEvent is the last event emitted by claude; it carries the final answer.
type ResultEvent struct {
	Type          string  `json:"type"`
	Subtype       string  `json:"subtype"`
	IsError       bool    `json:"is_error"`
	DurationMS    int64   `json:"duration_ms"`
	DurationAPIMS int64   `json:"duration_api_ms"`
	NumTurns      int     `json:"num_turns"`
	SessionID     string  `json:"session_id"`
	TotalCostUSD  float64 `json:"total_cost_usd"`
	Usage         Usage   `json:"usage"`
	Result        string  `json:"result"`
	StopReason    string  `json:"stop_reason"`
	UUID          string  `json:"uuid"`
}

func (e *ResultEvent) eventType() string { return e.Type }

// RateLimitEvent is emitted once per invocation, typically between the system
// init and the first assistant turn (D28).
type RateLimitEvent struct {
	Type          string        `json:"type"`
	RateLimitInfo RateLimitInfo `json:"rate_limit_info"`
	UUID          string        `json:"uuid"`
	SessionID     string        `json:"session_id"`
}

func (e *RateLimitEvent) eventType() string { return e.Type }

// RateLimitInfo holds the payload inside a RateLimitEvent.
type RateLimitInfo struct {
	Status                string `json:"status"`
	ResetsAt              int64  `json:"resetsAt"`
	RateLimitType         string `json:"rateLimitType"`
	OverageStatus         string `json:"overageStatus"`
	OverageDisabledReason string `json:"overageDisabledReason"`
	IsUsingOverage        bool   `json:"isUsingOverage"`
}

// StepStats accumulates usage and timing across a single claude step.
type StepStats struct {
	NumTurns            int
	InputTokens         int
	OutputTokens        int
	CacheCreationTokens int
	CacheReadTokens     int
	TotalCostUSD        float64
	DurationMS          int64
	SessionID           string
	LastRateLimitInfo   *RateLimitInfo
}
