// Package ansi provides strict ANSI escape sequence stripping for untrusted bytes.
package ansi

// StripAll removes every ANSI escape sequence from b and returns a new slice.
// It strips CSI sequences (ESC [ ... final), OSC sequences (ESC ] ... ST/BEL),
// bare ESC bytes, and two-byte ESC-prefixed sequences. The input is never mutated.
func StripAll(b []byte) []byte {
	if len(b) == 0 {
		return []byte{}
	}
	out := make([]byte, 0, len(b))
	i := 0
	for i < len(b) {
		if b[i] != 0x1b {
			out = append(out, b[i])
			i++
			continue
		}
		// ESC byte found
		if i+1 >= len(b) {
			// bare ESC at end — drop it
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
				j++ // consume final byte
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
			// Two-byte ESC sequence or unrecognised — drop both bytes
			i += 2
		}
	}
	return out
}
