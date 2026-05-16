package ansi

// StripForTerminalOutput strips ANSI escape sequences and dangerous C0/C1
// control bytes from b for cmux-supplied diagnostic text routed to the
// launching terminal; see SEC-001/SEC-004 for the threat model.
//
// It is a strict superset of StripAll: every sequence stripped by StripAll is
// also stripped here. Additionally stripped:
//   - C0 cursor-movement bytes: NUL (0x00), BEL (0x07), BS (0x08), VT (0x0B),
//     FF (0x0C), CR (0x0D), DEL (0x7F) — closes SEC-001.
//   - All 8-bit C1 controls (0x80–0x9F), with payload consumption for C1 OSC
//     (0x9D) and C1 DCS (0x90) — closes SEC-004.
//   - C1 CSI (0x9B): consumed as a CSI introducer to the next final byte
//     (0x40–0x7E).
//
// Preserved: LF (0x0A) and HT (0x09) — diagnostic text legitimately uses
// these for line breaks and tab alignment.
//
// NON-ASCII (UTF-8) SAFETY: bytes in 0x80–0x9F are dropped unconditionally,
// which means any multi-byte UTF-8 sequence whose continuation byte falls in
// that range is corrupted. For example, the em-dash (U+2014, 0xE2 0x80 0x94)
// would lose its 0x80 byte and become invalid UTF-8. This is a deliberate
// safety-over-fidelity choice for untrusted terminal output: over-stripping is
// preferable to letting 8-bit C1 escape sequences through. Callers must treat
// the return value as potentially lossy when the input contains non-ASCII text.
//
// The input slice is never mutated.
func StripForTerminalOutput(b []byte) []byte {
	if len(b) == 0 {
		return []byte{}
	}
	out := make([]byte, 0, len(b))
	i := 0
	for i < len(b) {
		c := b[i]

		// 8-bit C1 controls (0x80–0x9F)
		if c >= 0x80 && c <= 0x9F {
			i++
			switch c {
			case 0x9D, 0x90: // C1 OSC / C1 DCS: consume payload until BEL or ST
				for i < len(b) {
					t := b[i]
					i++
					if t == 0x07 || t == 0x9C { // BEL or C1-ST
						break
					}
				}
			case 0x9B: // C1 CSI: consume until final byte (0x40–0x7E)
				for i < len(b) && (b[i] < 0x40 || b[i] > 0x7e) {
					i++
				}
				if i < len(b) {
					i++ // consume final byte
				}
			}
			continue
		}

		// C0 cursor-movement bytes — strip (SEC-001)
		switch c {
		case 0x00, 0x07, 0x08, 0x0B, 0x0C, 0x0D, 0x7F:
			i++
			continue
		}

		// 7-bit ESC sequences — same logic as StripAll (strict superset)
		if c == 0x1b {
			if i+1 >= len(b) {
				i++
				continue
			}
			switch b[i+1] {
			case '[': // CSI: ESC [ ... final-byte (0x40–0x7E)
				j := i + 2
				for j < len(b) && (b[j] < 0x40 || b[j] > 0x7e) {
					j++
				}
				if j < len(b) {
					j++
				}
				i = j
			case ']': // OSC: ESC ] ... ST (ESC \) or BEL (0x07)
				j := i + 2
				for j < len(b) {
					if b[j] == 0x07 {
						j++
						break
					}
					if b[j] == 0x1b && j+1 < len(b) && b[j+1] == '\\' {
						j += 2
						break
					}
					j++
				}
				i = j
			default:
				i += 2
			}
			continue
		}

		out = append(out, c)
		i++
	}
	return out
}
