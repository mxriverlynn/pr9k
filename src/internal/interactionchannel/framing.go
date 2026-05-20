package interactionchannel

import (
	"bufio"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"io"
)

const (
	// maxMessageSize is the per-message read cap. Messages larger than this
	// signal a corrupted or malicious frame; the connection is dropped.
	maxMessageSize = 1 << 20 // 1 MiB
)

// MarshalMessage serializes msg to JSON with the "type" discriminator field
// injected at the top level. Exported so tests can inspect the wire form.
func MarshalMessage(msg Message) ([]byte, error) {
	raw, err := json.Marshal(msg)
	if err != nil {
		return nil, fmt.Errorf("interactionchannel: marshal %s: %w", msg.WireType(), err)
	}
	// Inject "type" into the existing JSON object. Unmarshal to a generic
	// map, add the key, re-marshal — clean and correct for any field set.
	var m map[string]json.RawMessage
	if err := json.Unmarshal(raw, &m); err != nil {
		return nil, fmt.Errorf("interactionchannel: re-encode %s: %w", msg.WireType(), err)
	}
	typeVal, err := json.Marshal(msg.WireType())
	if err != nil {
		return nil, fmt.Errorf("interactionchannel: marshal type key: %w", err)
	}
	m["type"] = typeVal
	out, err := json.Marshal(m)
	if err != nil {
		return nil, fmt.Errorf("interactionchannel: final marshal %s: %w", msg.WireType(), err)
	}
	return out, nil
}

// UnmarshalMessage deserializes a JSON payload produced by MarshalMessage.
// Unknown fields are silently ignored (forward-compatibility per R3).
// Exported so tests can exercise the discriminator path directly.
func UnmarshalMessage(data []byte) (Message, error) {
	var disc struct {
		Type string `json:"type"`
	}
	if err := json.Unmarshal(data, &disc); err != nil {
		return nil, fmt.Errorf("interactionchannel: decode discriminator: %w", err)
	}
	switch disc.Type {
	case "ready":
		var m Ready
		if err := json.Unmarshal(data, &m); err != nil {
			return nil, fmt.Errorf("interactionchannel: decode ready: %w", err)
		}
		return m, nil
	case "intent":
		var m Intent
		if err := json.Unmarshal(data, &m); err != nil {
			return nil, fmt.Errorf("interactionchannel: decode intent: %w", err)
		}
		return m, nil
	case "done_ack":
		var m DoneAck
		if err := json.Unmarshal(data, &m); err != nil {
			return nil, fmt.Errorf("interactionchannel: decode done_ack: %w", err)
		}
		return m, nil
	case "state_header":
		var m StateHeader
		if err := json.Unmarshal(data, &m); err != nil {
			return nil, fmt.Errorf("interactionchannel: decode state_header: %w", err)
		}
		return m, nil
	case "state_log":
		var m StateLog
		if err := json.Unmarshal(data, &m); err != nil {
			return nil, fmt.Errorf("interactionchannel: decode state_log: %w", err)
		}
		return m, nil
	case "state_footer":
		var m StateFooter
		if err := json.Unmarshal(data, &m); err != nil {
			return nil, fmt.Errorf("interactionchannel: decode state_footer: %w", err)
		}
		return m, nil
	case "workspace_done":
		var m WorkspaceDone
		if err := json.Unmarshal(data, &m); err != nil {
			return nil, fmt.Errorf("interactionchannel: decode workspace_done: %w", err)
		}
		return m, nil
	default:
		return nil, fmt.Errorf("interactionchannel: unknown message type %q", disc.Type)
	}
}

// writeMessage writes a length-prefixed JSON frame to w.
// Frame format: uint32 big-endian length || JSON bytes.
func writeMessage(bw *bufio.Writer, msg Message) error {
	data, err := MarshalMessage(msg)
	if err != nil {
		return err
	}
	if len(data) > maxMessageSize {
		return fmt.Errorf("interactionchannel: outbound message size %d exceeds limit", len(data))
	}
	var lenBuf [4]byte
	binary.BigEndian.PutUint32(lenBuf[:], uint32(len(data)))
	if _, err := bw.Write(lenBuf[:]); err != nil {
		return fmt.Errorf("interactionchannel: write frame length: %w", err)
	}
	if _, err := bw.Write(data); err != nil {
		return fmt.Errorf("interactionchannel: write frame data: %w", err)
	}
	return bw.Flush()
}

// readMessage reads one length-prefixed JSON frame from r.
func readMessage(br *bufio.Reader) (Message, error) {
	var lenBuf [4]byte
	if _, err := io.ReadFull(br, lenBuf[:]); err != nil {
		return nil, err
	}
	size := binary.BigEndian.Uint32(lenBuf[:])
	if size > maxMessageSize {
		return nil, fmt.Errorf("interactionchannel: inbound message size %d exceeds limit", size)
	}
	data := make([]byte, size)
	if _, err := io.ReadFull(br, data); err != nil {
		return nil, err
	}
	return UnmarshalMessage(data)
}
