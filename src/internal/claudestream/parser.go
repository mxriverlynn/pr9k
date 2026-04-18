package claudestream

import (
	"encoding/json"
	"fmt"
)

// MalformedLineError is returned by Parser.Parse when a line cannot be
// interpreted as a valid, known event. It carries the raw bytes so callers
// can log them (D7).
type MalformedLineError struct {
	Raw []byte
	Msg string
}

func (e *MalformedLineError) Error() string {
	return fmt.Sprintf("claudestream: malformed line (%s): %q", e.Msg, e.Raw)
}

// typeProbe extracts only the top-level "type" field for dispatch.
type typeProbe struct {
	Type string `json:"type"`
}

// Parser dispatches raw NDJSON lines to typed event structs.
type Parser struct{}

// Parse converts one raw NDJSON line into a typed Event.
//
// Empty lines and lines with invalid JSON, a missing "type" field, or an
// unknown "type" value all return a *MalformedLineError carrying the raw bytes
// (D7). Unknown sibling fields on known event types are silently ignored (D8).
func (p *Parser) Parse(line []byte) (Event, error) {
	if len(line) == 0 {
		return nil, &MalformedLineError{Raw: line, Msg: "empty line"}
	}

	var probe typeProbe
	if err := json.Unmarshal(line, &probe); err != nil {
		return nil, &MalformedLineError{Raw: line, Msg: "invalid JSON"}
	}
	if probe.Type == "" {
		return nil, &MalformedLineError{Raw: line, Msg: "missing type field"}
	}

	switch probe.Type {
	case "system":
		var ev SystemEvent
		if err := json.Unmarshal(line, &ev); err != nil {
			return nil, &MalformedLineError{Raw: line, Msg: "unmarshal system: " + err.Error()}
		}
		return &ev, nil
	case "assistant":
		var ev AssistantEvent
		if err := json.Unmarshal(line, &ev); err != nil {
			return nil, &MalformedLineError{Raw: line, Msg: "unmarshal assistant: " + err.Error()}
		}
		return &ev, nil
	case "user":
		var ev UserEvent
		if err := json.Unmarshal(line, &ev); err != nil {
			return nil, &MalformedLineError{Raw: line, Msg: "unmarshal user: " + err.Error()}
		}
		return &ev, nil
	case "result":
		var ev ResultEvent
		if err := json.Unmarshal(line, &ev); err != nil {
			return nil, &MalformedLineError{Raw: line, Msg: "unmarshal result: " + err.Error()}
		}
		return &ev, nil
	case "rate_limit_event":
		var ev RateLimitEvent
		if err := json.Unmarshal(line, &ev); err != nil {
			return nil, &MalformedLineError{Raw: line, Msg: "unmarshal rate_limit_event: " + err.Error()}
		}
		return &ev, nil
	default:
		return nil, &MalformedLineError{Raw: line, Msg: "unknown type " + probe.Type}
	}
}
