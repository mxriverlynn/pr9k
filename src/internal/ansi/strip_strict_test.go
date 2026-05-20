package ansi_test

import (
	"bytes"
	"testing"

	"github.com/mxriverlynn/pr9k/src/internal/ansi"
)

// --- TP-225-001: C0 bytes stripped ---

func TestStripForTerminalOutput_C0BytesStripped(t *testing.T) {
	cases := []struct {
		name  string
		input []byte
	}{
		{"NUL 0x00", []byte("a\x00b")},
		{"BEL 0x07", []byte("a\x07b")},
		{"BS 0x08", []byte("a\x08b")},
		{"VT 0x0B", []byte("a\x0Bb")},
		{"FF 0x0C", []byte("a\x0Cb")},
		{"CR 0x0D", []byte("a\x0Db")},
		{"DEL 0x7F", []byte("a\x7Fb")},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			output := ansi.StripForTerminalOutput(tc.input)
			if !bytes.Equal(output, []byte("ab")) {
				t.Errorf("%s: expected %q, got %q", tc.name, "ab", output)
			}
		})
	}
}

// --- TP-225-002: LF and HT preserved ---

func TestStripForTerminalOutput_LFAndHTPreserved(t *testing.T) {
	cases := []struct {
		name  string
		input []byte
	}{
		{"LF 0x0A", []byte("line1\x0Aline2")},
		{"HT 0x09", []byte("col1\x09col2")},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			output := ansi.StripForTerminalOutput(tc.input)
			if !bytes.Equal(output, tc.input) {
				t.Errorf("%s: expected %q preserved, got %q", tc.name, tc.input, output)
			}
		})
	}
}

// --- TP-225-003: 8-bit C1 CSI (0x9B) stripped including CSI final byte ---

func TestStripForTerminalOutput_8BitC1CSIStripped(t *testing.T) {
	// 0x9B is C1-CSI; treat it as a CSI introducer, consume to the next final byte
	// (0x40–0x7E). Here '[' (0x5B) is the first byte in that range, so it is consumed
	// as the final byte. Remaining bytes "31mred" are not part of any sequence.
	input := []byte{0x9B, '[', '3', '1', 'm', 'r', 'e', 'd'}
	output := ansi.StripForTerminalOutput(input)
	if bytes.IndexByte(output, 0x9B) != -1 {
		t.Errorf("output contains 0x9B (C1 CSI): %q", output)
	}
	// '[' is consumed as the CSI final byte; "31mred" passes through
	if !bytes.Equal(output, []byte("31mred")) {
		t.Errorf("expected %q, got %q", "31mred", output)
	}
}

// --- TP-225-004: 8-bit C1 OSC (0x9D) with BEL terminator stripped (SEC-004) ---

func TestStripForTerminalOutput_8BitC1OSCWithBELStripped(t *testing.T) {
	// \x9D0;fake-title\x07 — entire sequence including BEL stripped
	input := []byte{0x9D, '0', ';', 'f', 'a', 'k', 'e', '-', 't', 'i', 't', 'l', 'e', 0x07, 'o', 'k'}
	output := ansi.StripForTerminalOutput(input)
	if bytes.IndexByte(output, 0x9D) != -1 {
		t.Errorf("output contains 0x9D (C1 OSC): %q", output)
	}
	if bytes.IndexByte(output, 0x07) != -1 {
		t.Errorf("output contains BEL: %q", output)
	}
	if !bytes.Equal(output, []byte("ok")) {
		t.Errorf("expected %q, got %q", "ok", output)
	}
}

// --- TP-225-005: 8-bit C1 OSC with ST terminator (0x9C) stripped ---

func TestStripForTerminalOutput_8BitC1OSCWithSTStripped(t *testing.T) {
	// \x9D0;fake\x9C — ST (0x9C) consumed as terminator
	input := []byte{0x9D, '0', ';', 'f', 'a', 'k', 'e', 0x9C, 'o', 'k'}
	output := ansi.StripForTerminalOutput(input)
	if bytes.IndexByte(output, 0x9D) != -1 {
		t.Errorf("output contains 0x9D (C1 OSC): %q", output)
	}
	if bytes.IndexByte(output, 0x9C) != -1 {
		t.Errorf("output contains 0x9C (C1 ST): %q", output)
	}
	if !bytes.Equal(output, []byte("ok")) {
		t.Errorf("expected %q, got %q", "ok", output)
	}
}

// --- TP-225-006: SEC-001 CR overstrike demonstration ---

func TestStripForTerminalOutput_SEC001CROverstrike(t *testing.T) {
	// CR-overstrike attack: "workspace.create failed\rpr9k: workspace created"
	// Without stripping, the CR would cause the second message to overwrite the first
	// on a terminal, hiding the real failure.
	input := []byte("workspace.create failed\rpr9k: workspace created")
	output := ansi.StripForTerminalOutput(input)
	expected := []byte("workspace.create failedpr9k: workspace created")
	if !bytes.Equal(output, expected) {
		t.Errorf("expected %q, got %q", expected, output)
	}
}

// --- TP-225-007: 7-bit ESC sequences still stripped (superset of StripAll) ---

func TestStripForTerminalOutput_7BitEscSequencesStripped(t *testing.T) {
	cases := []struct {
		name     string
		input    []byte
		expected []byte
	}{
		{"CSI color", []byte("\x1b[31mred\x1b[0m"), []byte("red")},
		{"OSC BEL", []byte("\x1b]0;title\x07after"), []byte("after")},
		{"OSC ST", []byte("\x1b]8;;url\x1b\\link\x1b]8;;\x1b\\"), []byte("link")},
		{"two-byte ESC", []byte("\x1bMX"), []byte("X")},
		{"bare ESC at end", []byte("hello\x1b"), []byte("hello")},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			output := ansi.StripForTerminalOutput(tc.input)
			if bytes.IndexByte(output, 0x1b) != -1 {
				t.Errorf("%s: output contains ESC byte: %q", tc.name, output)
			}
			if !bytes.Equal(output, tc.expected) {
				t.Errorf("%s: expected %q, got %q", tc.name, tc.expected, output)
			}
		})
	}
}

// --- TP-225-008: bare C1 bytes (not OSC/DCS/CSI) stripped ---

func TestStripForTerminalOutput_BareC1BytesStripped(t *testing.T) {
	for b := byte(0x80); b <= 0x9F; b++ {
		// Skip OSC/DCS/CSI — tested separately above
		if b == 0x9D || b == 0x90 || b == 0x9B {
			continue
		}
		input := []byte{'a', b, 'b'}
		output := ansi.StripForTerminalOutput(input)
		if !bytes.Equal(output, []byte("ab")) {
			t.Errorf("C1 byte 0x%02X: expected %q, got %q", b, "ab", output)
		}
	}
}

// --- TP-225-009: plain text and preserved bytes pass through unchanged ---

func TestStripForTerminalOutput_PlainTextPreserved(t *testing.T) {
	input := []byte("hello, world!\x09indented\x0Anewline")
	output := ansi.StripForTerminalOutput(input)
	if !bytes.Equal(output, input) {
		t.Errorf("expected %q, got %q", input, output)
	}
}

// --- TP-225-010: nil and empty input ---

func TestStripForTerminalOutput_NilAndEmpty(t *testing.T) {
	if out := ansi.StripForTerminalOutput(nil); out == nil || len(out) != 0 {
		t.Errorf("nil input: expected non-nil empty slice, got %v", out)
	}
	if out := ansi.StripForTerminalOutput([]byte{}); out == nil || len(out) != 0 {
		t.Errorf("empty input: expected non-nil empty slice, got %v", out)
	}
}

// --- TP-225-011: input not mutated ---

func TestStripForTerminalOutput_DoesNotMutateInput(t *testing.T) {
	input := []byte("\x1b[31mred\x0Dcarriage\x9D0;title\x07")
	original := make([]byte, len(input))
	copy(original, input)
	ansi.StripForTerminalOutput(input)
	if !bytes.Equal(input, original) {
		t.Errorf("input was mutated: got %q, want %q", input, original)
	}
}

// --- TP-001: C1 DCS (0x90) payload stripped (SEC-004) ---

func TestStripForTerminalOutput_8BitC1DCSStripped(t *testing.T) {
	cases := []struct {
		name  string
		input []byte
	}{
		// BEL terminator: 0x90 payload 'p','a','y' BEL 'o','k'
		{"DCS BEL terminator", []byte{0x90, 'p', 'a', 'y', 0x07, 'o', 'k'}},
		// C1-ST terminator: 0x90 payload 'p','a','y' 0x9C 'o','k'
		{"DCS C1-ST terminator", []byte{0x90, 'p', 'a', 'y', 0x9C, 'o', 'k'}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			output := ansi.StripForTerminalOutput(tc.input)
			if !bytes.Equal(output, []byte("ok")) {
				t.Errorf("%s: expected %q, got %q", tc.name, "ok", output)
			}
			if bytes.IndexByte(output, 0x90) != -1 {
				t.Errorf("%s: output contains 0x90 (C1 DCS): %q", tc.name, output)
			}
		})
	}
}

// --- TP-002: double-ESC contract pinned for StripForTerminalOutput (SEC-002) ---

func TestStripForTerminalOutput_DoubleESC(t *testing.T) {
	cases := []struct {
		name     string
		input    []byte
		expected []byte
	}{
		{"CSI color", []byte("\x1b[31mred\x1b[0m"), []byte("red")},
		{"OSC BEL", []byte("\x1b]0;title\x07after"), []byte("after")},
		{"OSC ST", []byte("\x1b]8;;url\x1b\\link\x1b]8;;\x1b\\"), []byte("link")},
		{"two-byte ESC", []byte("\x1bMX"), []byte("X")},
		{"bare ESC at end", []byte("hello\x1b"), []byte("hello")},
		{"double ESC", []byte("\x1b\x1b[31mred"), []byte("[31mred")},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			output := ansi.StripForTerminalOutput(tc.input)
			if bytes.IndexByte(output, 0x1b) != -1 {
				t.Errorf("%s: output contains ESC byte: %q", tc.name, output)
			}
			if !bytes.Equal(output, tc.expected) {
				t.Errorf("%s: expected %q, got %q", tc.name, tc.expected, output)
			}
		})
	}
}

// --- TP-003: unterminated C1 OSC/DCS/CSI sequences (EOF/bounds guard) ---

func TestStripForTerminalOutput_UnterminatedC1(t *testing.T) {
	cases := []struct {
		name     string
		input    []byte
		expected []byte
	}{
		// unterminated C1 OSC: consumed to EOF, nothing leaks
		{"unterminated C1 OSC", []byte{0x9D, 'a', 'b', 'c'}, []byte{}},
		// unterminated C1 DCS: consumed to EOF, nothing leaks
		{"unterminated C1 DCS", []byte{0x90, 'a', 'b', 'c'}, []byte{}},
		// text before unterminated C1 CSI: text passes through, sequence stripped
		{"text before unterminated C1 CSI", []byte{'x', 0x9B, '3', '1'}, []byte("x")},
		// 7-bit ESC ST is NOT a recognized C1 OSC terminator; SEC-004: no payload leak
		{"C1 OSC with embedded 7-bit ST consumed to EOF", []byte{0x9D, '0', ';', 'e', 0x1b, '\\', 'z'}, []byte{}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			output := ansi.StripForTerminalOutput(tc.input)
			if !bytes.Equal(output, tc.expected) {
				t.Errorf("%s: expected %q, got %q", tc.name, tc.expected, output)
			}
		})
	}
}
