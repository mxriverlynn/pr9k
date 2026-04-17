package statusline

import "bytes"

// Sanitize strips disallowed control sequences from script stdout before
// caching. Preserved: SGR color escapes (\x1b[…m) and OSC 8 hyperlinks.
// Stripped: \r, CSI cursor/erase sequences, other OSC sequences, bare \x1b,
// BEL, and trailing whitespace. Mid-sequence truncation at EOF does not panic.
func Sanitize(b []byte) string {
	out := make([]byte, 0, len(b))
	i := 0
	for i < len(b) {
		c := b[i]
		switch {
		case c == '\r':
			i++
		case c == '\x07': // bare BEL
			i++
		case c == '\x1b':
			if i+1 >= len(b) {
				// bare ESC at EOF — drop it
				i++
				break
			}
			next := b[i+1]
			switch next {
			case '[': // CSI
				// scan to the final byte (letter)
				j := i + 2
				for j < len(b) && !isCsiTerminator(b[j]) {
					j++
				}
				if j >= len(b) {
					// unterminated CSI — drop everything
					i = j
					break
				}
				terminator := b[j]
				j++ // include terminator
				if terminator == 'm' {
					// SGR — keep it
					out = append(out, b[i:j]...)
				}
				// all other CSI (cursor movement, erase, …) — drop
				i = j
			case ']': // OSC
				i, out = consumeOSC(b, i, out)
			default:
				// bare ESC not followed by [ or ] — drop ESC, keep next byte
				i++
			}
		default:
			out = append(out, c)
			i++
		}
	}
	return string(bytes.TrimRight(out, " \t"))
}

// isCsiTerminator returns true for bytes that end a CSI sequence (0x40–0x7E).
func isCsiTerminator(c byte) bool {
	return c >= 0x40 && c <= 0x7e
}

// consumeOSC handles OSC sequences starting at b[start] (which is \x1b]).
// OSC 8 hyperlinks are preserved; all other OSC sequences are dropped.
// Returns the new index and the (possibly modified) output slice.
func consumeOSC(b []byte, start int, out []byte) (int, []byte) {
	// Find the OSC number (digits after \x1b])
	i := start + 2
	numStart := i
	for i < len(b) && b[i] >= '0' && b[i] <= '9' {
		i++
	}
	oscNum := string(b[numStart:i])

	// Find the terminator: BEL (\x07) or ST (\x1b\x5c)
	body := i
	terminated := false
	for i < len(b) {
		if b[i] == '\x07' {
			i++ // consume BEL
			terminated = true
			break
		}
		if b[i] == '\x1b' && i+1 < len(b) && b[i+1] == '\\' {
			i += 2 // consume ST
			terminated = true
			break
		}
		i++
	}

	if oscNum == "8" && terminated {
		// OSC 8 hyperlink — preserve entire well-formed sequence
		out = append(out, b[start:i]...)
	}
	// all other OSC, or any unterminated OSC — drop
	_ = body
	return i, out
}
