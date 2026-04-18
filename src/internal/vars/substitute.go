package vars

import (
	"log"
	"strings"
)

// Substitute replaces all {{VAR_NAME}} tokens in input with values from vt
// using the given phase's resolution order.
//
// Escape rules:
//   - {{{{ → literal {{
//   - }}}} → literal }}
//
// Unresolved variable references log a warning and substitute the empty string.
// If vt is nil, input is returned unchanged.
func Substitute(input string, vt *VarTable, phase Phase) (string, error) {
	if vt == nil {
		return input, nil
	}

	var b strings.Builder
	b.Grow(len(input))
	i := 0

	for i < len(input) {
		// Check for escape sequences before checking for tokens.
		if i+4 <= len(input) && input[i:i+4] == "{{{{" {
			b.WriteString("{{")
			i += 4
			continue
		}
		if i+4 <= len(input) && input[i:i+4] == "}}}}" {
			b.WriteString("}}")
			i += 4
			continue
		}
		// Check for a variable token.
		if i+2 <= len(input) && input[i:i+2] == "{{" {
			// Find the closing }}.
			closeIdx := strings.Index(input[i+2:], "}}")
			if closeIdx == -1 {
				// No closing }}; output the character literally and move on.
				b.WriteByte(input[i])
				i++
				continue
			}
			varName := input[i+2 : i+2+closeIdx]
			val, ok := vt.GetInPhase(phase, varName)
			if !ok {
				log.Printf("vars: unresolved variable %q, substituting empty string", varName)
				val = ""
			}
			b.WriteString(val)
			i = i + 2 + closeIdx + 2
			continue
		}
		b.WriteByte(input[i])
		i++
	}

	return b.String(), nil
}

// ExtractReferences returns all variable names referenced by {{VAR_NAME}} tokens
// in input. Escape sequences ({{{{ and }}}}) are not treated as references.
// The returned slice may contain duplicates if the same variable appears more
// than once.
func ExtractReferences(input string) []string {
	var refs []string
	i := 0

	for i < len(input) {
		// Skip escape sequences.
		if i+4 <= len(input) && input[i:i+4] == "{{{{" {
			i += 4
			continue
		}
		if i+4 <= len(input) && input[i:i+4] == "}}}}" {
			i += 4
			continue
		}
		// Variable token.
		if i+2 <= len(input) && input[i:i+2] == "{{" {
			closeIdx := strings.Index(input[i+2:], "}}")
			if closeIdx == -1 {
				i++
				continue
			}
			varName := input[i+2 : i+2+closeIdx]
			refs = append(refs, varName)
			i = i + 2 + closeIdx + 2
			continue
		}
		i++
	}

	return refs
}
