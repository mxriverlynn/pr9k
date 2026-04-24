package ansi_test

import (
	"bytes"
	"testing"

	"github.com/mxriverlynn/pr9k/src/internal/ansi"
)

func TestStripAll_OSC8HyperlinkStripped(t *testing.T) {
	input := []byte("\x1b]8;;https://evil/\x1b\\link\x1b]8;;\x1b\\")
	output := ansi.StripAll(input)
	if bytes.Contains(output, []byte("\x1b]")) {
		t.Errorf("output still contains ESC ]: %q", output)
	}
}

func TestStripAll_OSC8WithBELTerminatorStripped(t *testing.T) {
	input := []byte("\x1b]8;;https://evil/\x07link\x1b]8;;\x07")
	output := ansi.StripAll(input)
	if bytes.Contains(output, []byte("\x1b]")) {
		t.Errorf("output still contains ESC ]: %q", output)
	}
}

func TestStripAll_CSIStripped(t *testing.T) {
	input := []byte("\x1b[31mred\x1b[0m")
	output := ansi.StripAll(input)
	if !bytes.Equal(output, []byte("red")) {
		t.Errorf("expected %q, got %q", "red", output)
	}
}

func TestStripAll_BareEscStripped(t *testing.T) {
	input := []byte("hello\x1bworld")
	output := ansi.StripAll(input)
	if bytes.Contains(output, []byte("\x1b")) {
		t.Errorf("output still contains ESC byte: %q", output)
	}
}

func TestStripAll_PlainTextUnchanged(t *testing.T) {
	input := []byte("hello, world!")
	output := ansi.StripAll(input)
	if !bytes.Equal(output, input) {
		t.Errorf("expected %q, got %q", input, output)
	}
}

func TestStripAll_DoesNotMutateInput(t *testing.T) {
	input := []byte("\x1b[31mred\x1b[0m")
	original := make([]byte, len(input))
	copy(original, input)
	ansi.StripAll(input)
	if !bytes.Equal(input, original) {
		t.Errorf("input was mutated: got %q, want %q", input, original)
	}
}

func TestStripAll_EmptyInput(t *testing.T) {
	output := ansi.StripAll([]byte{})
	if len(output) != 0 {
		t.Errorf("expected empty output, got %q", output)
	}
}
