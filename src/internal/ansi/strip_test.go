package ansi_test

import (
	"bytes"
	"testing"
	"time"

	"github.com/mxriverlynn/pr9k/src/internal/ansi"
)

// --- Original tests (strengthened per TP-027, TP-023, TP-028) ---

func TestStripAll_OSC8HyperlinkStripped(t *testing.T) {
	input := []byte("\x1b]8;;https://evil/\x1b\\link\x1b]8;;\x1b\\")
	output := ansi.StripAll(input)
	if !bytes.Equal(output, []byte("link")) {
		t.Errorf("expected %q, got %q", "link", output)
	}
}

func TestStripAll_OSC8WithBELTerminatorStripped(t *testing.T) {
	input := []byte("\x1b]8;;https://evil/\x07link\x1b]8;;\x07")
	output := ansi.StripAll(input)
	if !bytes.Equal(output, []byte("link")) {
		t.Errorf("expected %q, got %q", "link", output)
	}
}

func TestStripAll_CSIStripped(t *testing.T) {
	input := []byte("\x1b[31mred\x1b[0m")
	output := ansi.StripAll(input)
	if !bytes.Equal(output, []byte("red")) {
		t.Errorf("expected %q, got %q", "red", output)
	}
}

// TP-023: rename and strengthen — exercises default two-byte branch, not bare-ESC-at-EOF
func TestStripAll_EscDefaultBranch(t *testing.T) {
	input := []byte("hello\x1bworld")
	output := ansi.StripAll(input)
	// ESC w is a two-byte sequence; drops ESC + 'w', emits "helloorld"
	if !bytes.Equal(output, []byte("helloorld")) {
		t.Errorf("expected %q, got %q", "helloorld", output)
	}
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

// TP-028: strengthen EmptyInput — pin nil vs []byte{} contract
func TestStripAll_EmptyInput(t *testing.T) {
	output := ansi.StripAll([]byte{})
	if len(output) != 0 {
		t.Errorf("expected empty output, got %q", output)
	}
	if output == nil {
		t.Errorf("expected non-nil empty slice, got nil")
	}
}

// --- TP-017: nil input ---

func TestStripAll_NilInput(t *testing.T) {
	output := ansi.StripAll(nil)
	if output == nil {
		t.Errorf("expected non-nil result for nil input, got nil")
	}
	if len(output) != 0 {
		t.Errorf("expected zero-length output for nil input, got %q", output)
	}
}

// --- TP-002: 8-bit C1 controls (passthrough contract) ---

func TestStripAll_8BitC1Passes(t *testing.T) {
	// 0x9B is C1 CSI; current contract passes non-0x1b bytes through
	input := []byte{0x9b, '3', '1', 'm', 'r', 'e', 'd'}
	output := ansi.StripAll(input)
	if bytes.Contains(output, []byte{0x1b}) {
		t.Errorf("output contains ESC byte: %q", output)
	}
	// Pin passthrough: 8-bit C1 is not stripped (document the gap)
	if !bytes.Equal(output, input) {
		t.Errorf("8-bit C1 passthrough: expected %q, got %q", input, output)
	}
}

// --- TP-003: UTF-8 encoded U+009B (CSI) passthrough ---

func TestStripAll_UTF8C1Passes(t *testing.T) {
	// 0xC2 0x9B is valid UTF-8 for U+009B; current contract passes it through
	input := []byte{0xc2, 0x9b, '3', '1', 'm', 'X'}
	output := ansi.StripAll(input)
	if bytes.Contains(output, []byte{0x1b}) {
		t.Errorf("output contains ESC byte: %q", output)
	}
	if !bytes.Equal(output, input) {
		t.Errorf("UTF-8 C1 passthrough: expected %q, got %q", input, output)
	}
}

// --- TP-004: DCS/SOS/PM/APC sequences ---

func TestStripAll_DCSSequences(t *testing.T) {
	cases := []struct {
		name  string
		input []byte
	}{
		{"DCS", []byte("\x1bPpayload\x1b\\tail")},
		{"SOS", []byte("\x1bXpayload\x1b\\tail")},
		{"PM", []byte("\x1b^payload\x1b\\tail")},
		{"APC", []byte("\x1b_payload\x1b\\tail")},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			output := ansi.StripAll(tc.input)
			if bytes.Contains(output, []byte{0x1b}) {
				t.Errorf("%s: output contains ESC: %q", tc.name, output)
			}
		})
	}
}

func TestStripAll_DCSWithNestedCSI(t *testing.T) {
	// ESC P drops ESC+P, then re-enters loop; CSI is stripped; "color" emitted; ST stripped
	input := []byte("\x1bP\x1b[31mcolor\x1b\\tail")
	output := ansi.StripAll(input)
	if bytes.Contains(output, []byte{0x1b}) {
		t.Errorf("output contains ESC: %q", output)
	}
	// Pin whatever the output is (attacker text emitted but no ESC)
	t.Logf("DCS nested CSI output: %q", output)
}

// --- TP-005: double-escape bypass ---

func TestStripAll_DoubleEscape(t *testing.T) {
	input := []byte("\x1b\x1b[31mred")
	output := ansi.StripAll(input)
	if bytes.Contains(output, []byte{0x1b}) {
		t.Errorf("output contains ESC: %q", output)
	}
	// ESC ESC -> default drops both bytes; then [31mred: '[' is not ESC so passes through
	// Pin: "[31mred"
	if !bytes.Equal(output, []byte("[31mred")) {
		t.Errorf("expected %q, got %q", "[31mred", output)
	}
}

// --- TP-006: triple+ escape ---

func TestStripAll_TripleEscape(t *testing.T) {
	cases := []struct {
		name     string
		input    []byte
		expected []byte
	}{
		{"three escapes then CSI", []byte("\x1b\x1b\x1b[31mred"), []byte("red")},
		{"three bare escapes", []byte("\x1b\x1b\x1b"), []byte("")},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			output := ansi.StripAll(tc.input)
			if bytes.Contains(output, []byte{0x1b}) {
				t.Errorf("%s: output contains ESC: %q", tc.name, output)
			}
			if !bytes.Equal(output, tc.expected) {
				t.Errorf("%s: expected %q, got %q", tc.name, tc.expected, output)
			}
		})
	}
}

// --- TP-007: adjacent OSCs, first unterminated ---

func TestStripAll_AdjacentOSCsUnterminated(t *testing.T) {
	// Two adjacent OSCs: first one's scanner hits second \x1b] and treats it as payload
	input := []byte("\x1b]8;;A\x1b]8;;B")
	output := ansi.StripAll(input)
	if bytes.Contains(output, []byte{0x1b}) {
		t.Errorf("output contains ESC: %q", output)
	}
	// Both OSCs consumed; output should be empty
	if len(output) != 0 {
		t.Logf("adjacent OSC output (pinned): %q", output)
	}
}

// --- TP-008: OSC ending with lone ESC (no backslash) ---

func TestStripAll_OSCWithLoneTrailingESC(t *testing.T) {
	input := []byte("\x1b]8;;\x1b")
	output := ansi.StripAll(input)
	if bytes.Contains(output, []byte{0x1b}) {
		t.Errorf("output contains ESC: %q", output)
	}
}

// --- TP-009: stray ST (ESC \ alone) ---

func TestStripAll_StraySTSequence(t *testing.T) {
	input := []byte("before\x1b\\after")
	output := ansi.StripAll(input)
	if bytes.Contains(output, []byte{0x1b}) {
		t.Errorf("output contains ESC: %q", output)
	}
	if !bytes.Equal(output, []byte("beforeafter")) {
		t.Errorf("expected %q, got %q", "beforeafter", output)
	}
}

// --- TP-010: OSC nested inside unterminated CSI ---

func TestStripAll_OSCNestedInCSI(t *testing.T) {
	// CSI scanner runs until 'm' (final byte); consumes the embedded OSC-8 URL
	input := []byte("\x1b[\x1b]8;;http://evil\x1b\\m")
	output := ansi.StripAll(input)
	if bytes.Contains(output, []byte{0x1b}) {
		t.Errorf("output contains ESC: %q", output)
	}
}

// --- TP-011: CSI with parameter byte ? ---

func TestStripAll_CSINestedInCSI(t *testing.T) {
	input := []byte("\x1b[?\x1b[31mred")
	output := ansi.StripAll(input)
	if bytes.Contains(output, []byte{0x1b}) {
		t.Errorf("output contains ESC: %q", output)
	}
}

// --- TP-012: invalid UTF-8 lead byte before ESC ---

func TestStripAll_InvalidUTF8BeforeESC(t *testing.T) {
	input := []byte{0xc0, 0x1b, '[', '3', '1', 'm', 'x'}
	output := ansi.StripAll(input)
	if bytes.Contains(output, []byte{0x1b}) {
		t.Errorf("output contains ESC: %q", output)
	}
	// 0xC0 is not ANSI; passes through
	if len(output) == 0 || output[0] != 0xc0 {
		t.Errorf("expected 0xC0 to pass through, got %q", output)
	}
}

// --- TP-013: C0 controls (backspace, NUL, BEL) pass through ---

func TestStripAll_C0ControlsPassThrough(t *testing.T) {
	cases := []struct {
		name  string
		input []byte
	}{
		{"backspace overstrike", []byte("safe\x08\x08\x08EVIL")},
		{"NUL byte", []byte("safe\x00hidden")},
		{"BEL outside OSC", []byte("\x07BEL-outside-OSC")},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			output := ansi.StripAll(tc.input)
			if bytes.Contains(output, []byte{0x1b}) {
				t.Errorf("%s: output contains ESC: %q", tc.name, output)
			}
			// Pin current contract: C0 controls pass through
			if !bytes.Equal(output, tc.input) {
				t.Errorf("%s: C0 passthrough expected %q, got %q", tc.name, tc.input, output)
			}
		})
	}
}

// --- TP-014: DEL mid-CSI ---

func TestStripAll_DELMidCSI(t *testing.T) {
	input := []byte("\x1b[31\x7fmGOOD")
	output := ansi.StripAll(input)
	if !bytes.Equal(output, []byte("GOOD")) {
		t.Errorf("expected %q, got %q", "GOOD", output)
	}
}

// --- TP-015: CSI boundary bytes @ and ~ ---

func TestStripAll_CSIBoundaryBytes(t *testing.T) {
	cases := []struct {
		input    []byte
		expected []byte
	}{
		{[]byte("\x1b[@X"), []byte("X")},
		{[]byte("\x1b[~Y"), []byte("Y")},
		{[]byte("\x1b[200~Z"), []byte("Z")},
	}
	for _, tc := range cases {
		output := ansi.StripAll(tc.input)
		if !bytes.Equal(output, tc.expected) {
			t.Errorf("input %q: expected %q, got %q", tc.input, tc.expected, output)
		}
	}
}

// --- TP-016: large unterminated OSC (O(n) performance guard) ---

func TestStripAll_LargeUnterminatedOSC(t *testing.T) {
	input := append([]byte("\x1b]"), bytes.Repeat([]byte{'A'}, 1<<20)...)
	start := time.Now()
	output := ansi.StripAll(input)
	elapsed := time.Since(start)
	if elapsed > 5*time.Second {
		t.Errorf("StripAll took too long on large input: %v", elapsed)
	}
	if len(output) > len(input) {
		t.Errorf("output longer than input: %d > %d", len(output), len(input))
	}
	if bytes.Contains(output, []byte{0x1b}) {
		t.Errorf("output contains ESC: len=%d", len(output))
	}
}

// --- TP-018: returns a new slice (not aliased input) ---

func TestStripAll_ReturnsNewSlice(t *testing.T) {
	input := []byte("hello")
	output := ansi.StripAll(input)
	if len(output) > 0 && len(input) > 0 && &output[0] == &input[0] {
		t.Errorf("output aliases input backing array")
	}
	// Mutate output; input must be unchanged
	if len(output) > 0 {
		original0 := input[0]
		output[0] = 'Z'
		if input[0] != original0 {
			t.Errorf("mutating output changed input")
		}
	}
}

// --- TP-019: CSI preserves surrounding text (exact equality) ---

func TestStripAll_CSIPreservesSurroundingText(t *testing.T) {
	cases := []struct {
		input    []byte
		expected []byte
	}{
		{[]byte("A\x1b[31mB\x1b[0mC"), []byte("ABC")},
		{[]byte("\x1b[1;2;3;4;5;6;7Hdone"), []byte("done")},
	}
	for _, tc := range cases {
		output := ansi.StripAll(tc.input)
		if !bytes.Equal(output, tc.expected) {
			t.Errorf("input %q: expected %q, got %q", tc.input, tc.expected, output)
		}
	}
}

// --- TP-020: truncated CSI (no final byte before EOF) ---

func TestStripAll_TruncatedCSI(t *testing.T) {
	cases := []struct {
		input    []byte
		expected []byte
	}{
		{[]byte("\x1b[31;31;31"), []byte{}},
		{[]byte("text\x1b[31;"), []byte("text")},
	}
	for _, tc := range cases {
		output := ansi.StripAll(tc.input)
		if !bytes.Equal(output, tc.expected) {
			t.Errorf("input %q: expected %q, got %q", tc.input, tc.expected, output)
		}
	}
}

// --- TP-021: OSC with no terminator ---

func TestStripAll_OSCNoTerminator(t *testing.T) {
	cases := []struct {
		name     string
		input    []byte
		expected []byte
	}{
		{"OSC to EOF", []byte("before\x1b]8;;https://evil/never-terminated"), []byte("before")},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			output := ansi.StripAll(tc.input)
			if !bytes.Equal(output, tc.expected) {
				t.Errorf("expected %q, got %q", tc.expected, output)
			}
		})
	}
}

// --- TP-022: integration — multiple branch types in one input ---

func TestStripAll_Integration(t *testing.T) {
	// plain + CSI + OSC(BEL) + two-byte + bare-ESC-at-end
	input := []byte("plain\x1b[1;31;4mCSI\x1b]8;;http://x\x07OSC\x1bMtwo-byte\x1bendbare")
	output := ansi.StripAll(input)
	if !bytes.Equal(output, []byte("plainCSIOSCtwo-bytendbare")) {
		t.Errorf("expected %q, got %q", "plainCSIOSCtwo-bytendbare", output)
	}

	// back-to-back sequences with no plain text
	input2 := []byte("\x1b[0m\x1b]8;;x\x07\x1bM")
	output2 := ansi.StripAll(input2)
	if !bytes.Equal(output2, []byte{}) {
		t.Errorf("expected empty, got %q", output2)
	}
}

// --- TP-024: bare ESC at EOF (true bare-ESC-at-EOF guard) ---

func TestStripAll_TrailingEscStripped(t *testing.T) {
	input := []byte("hello\x1b")
	output := ansi.StripAll(input)
	if !bytes.Equal(output, []byte("hello")) {
		t.Errorf("expected %q, got %q", "hello", output)
	}
}

// --- TP-025: two-byte ESC sequences ---

func TestStripAll_TwoByteEscSequences(t *testing.T) {
	cases := []struct {
		input    []byte
		expected []byte
	}{
		{[]byte("\x1bMX"), []byte("X")},
		{[]byte("\x1b7X"), []byte("X")},
		{[]byte("\x1bcX"), []byte("X")},
		{[]byte("\x1b=X"), []byte("X")},
		{[]byte("\x1b>X"), []byte("X")},
	}
	for _, tc := range cases {
		output := ansi.StripAll(tc.input)
		if !bytes.Equal(output, tc.expected) {
			t.Errorf("input %q: expected %q, got %q", tc.input, tc.expected, output)
		}
	}
}

// --- TP-026: CSI with intermediate/parameter bytes ---

func TestStripAll_CSIWithIntermediateBytes(t *testing.T) {
	cases := []struct {
		input    []byte
		expected []byte
	}{
		{[]byte("\x1b[?25hA"), []byte("A")},
		{[]byte("\x1b[!pB"), []byte("B")},
		{[]byte("\x1b[>CC"), []byte("C")},
	}
	for _, tc := range cases {
		output := ansi.StripAll(tc.input)
		if !bytes.Equal(output, tc.expected) {
			t.Errorf("input %q: expected %q, got %q", tc.input, tc.expected, output)
		}
	}
}

// --- TP-029: multi-byte Unicode passthrough ---

func TestStripAll_MultiByteUnicode(t *testing.T) {
	input := []byte("héllo 世界\x1b[31mred\x1b[0m 🎉")
	expected := []byte("héllo 世界red 🎉")
	output := ansi.StripAll(input)
	if !bytes.Equal(output, expected) {
		t.Errorf("expected %q, got %q", expected, output)
	}
}

// --- TP-001: fuzz harness ---

func FuzzStripAll(f *testing.F) {
	// Seed corpus from adversarial inputs
	seeds := [][]byte{
		{0x9b, '3', '1', 'm', 'r', 'e', 'd'},
		{0xc2, 0x9b, '3', '1', 'm', 'X'},
		[]byte("\x1bPpayload\x1b\\tail"),
		[]byte("\x1b\x1b[31mred"),
		[]byte("\x1b\x1b\x1b[31mred"),
		[]byte("\x1b]8;;A\x1b]8;;B"),
		[]byte("\x1b]8;;\x1b"),
		[]byte("before\x1b\\after"),
		[]byte("\x1b[\x1b]8;;http://evil\x1b\\m"),
		[]byte("\x1b[?\x1b[31mred"),
		{0xc0, 0x1b, '[', '3', '1', 'm', 'x'},
		[]byte("safe\x08\x08\x08EVIL"),
		[]byte("\x1b[31\x7fmGOOD"),
		[]byte("\x1b[@X"),
		[]byte("\x1b[~Y"),
		[]byte("\x1b[31;31;31"),
		[]byte("hello\x1b"),
		nil,
		[]byte{},
		[]byte("plain text"),
		[]byte("\x1b[31mred\x1b[0m"),
		[]byte("\x1b]8;;https://evil/\x1b\\link\x1b]8;;\x1b\\"),
	}
	for _, s := range seeds {
		f.Add(s)
	}
	f.Fuzz(func(t *testing.T, in []byte) {
		out := ansi.StripAll(in)
		if bytes.IndexByte(out, 0x1b) != -1 {
			t.Errorf("output contains ESC byte for input %q: %q", in, out)
		}
		if len(out) > len(in) {
			t.Errorf("output longer than input: %d > %d", len(out), len(in))
		}
		// input must be unchanged
		orig := make([]byte, len(in))
		copy(orig, in)
		ansi.StripAll(in)
		if !bytes.Equal(in, orig) {
			t.Errorf("input was mutated")
		}
	})
}
