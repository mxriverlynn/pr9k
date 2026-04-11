package vars

import (
	"testing"
)

// newVTWithBindings creates a VarTable in Iteration phase with the given
// key/value pairs bound.  Keys and values alternate: newVTWithBindings("K","V",...).
func newVTWithBindings(pairs ...string) *VarTable {
	vt := &VarTable{
		persistent: make(map[string]string),
		iteration:  make(map[string]string),
		phase:      Iteration,
	}
	for i := 0; i+1 < len(pairs); i += 2 {
		vt.iteration[pairs[i]] = pairs[i+1]
	}
	return vt
}

// Substitute tests

func TestSubstitute_LiteralPassthrough(t *testing.T) {
	vt := newVTWithBindings()
	got, err := Substitute("hello world", vt, Iteration)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != "hello world" {
		t.Errorf("got %q, want %q", got, "hello world")
	}
}

func TestSubstitute_SingleVarReplaced(t *testing.T) {
	vt := newVTWithBindings("NAME", "alice")
	got, err := Substitute("hello {{NAME}}", vt, Iteration)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != "hello alice" {
		t.Errorf("got %q, want %q", got, "hello alice")
	}
}

func TestSubstitute_MultipleDistinctVarsReplaced(t *testing.T) {
	vt := newVTWithBindings("FIRST", "foo", "SECOND", "bar")
	got, err := Substitute("{{FIRST}} and {{SECOND}}", vt, Iteration)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != "foo and bar" {
		t.Errorf("got %q, want %q", got, "foo and bar")
	}
}

func TestSubstitute_SameVarMultipleTimes(t *testing.T) {
	vt := newVTWithBindings("X", "42")
	got, err := Substitute("{{X}}-{{X}}-{{X}}", vt, Iteration)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != "42-42-42" {
		t.Errorf("got %q, want %q", got, "42-42-42")
	}
}

func TestSubstitute_UnresolvedVarBecomesEmpty(t *testing.T) {
	vt := newVTWithBindings()
	got, err := Substitute("value: {{MISSING}}", vt, Iteration)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != "value: " {
		t.Errorf("got %q, want %q", got, "value: ")
	}
}

func TestSubstitute_EscapeOpenBrace(t *testing.T) {
	vt := newVTWithBindings()
	got, err := Substitute("{{{{", vt, Iteration)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != "{{" {
		t.Errorf("got %q, want %q", got, "{{")
	}
}

func TestSubstitute_EscapeCloseBrace(t *testing.T) {
	vt := newVTWithBindings()
	got, err := Substitute("}}}}", vt, Iteration)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != "}}" {
		t.Errorf("got %q, want %q", got, "}}")
	}
}

func TestSubstitute_EscapesPreserveLiteralBraces(t *testing.T) {
	vt := newVTWithBindings("VAR", "value")
	// {{{{ → {{ then VAR}} followed by }}}} → }}
	// So "{{{{VAR}}}}" → "{{VAR}}"
	got, err := Substitute("{{{{VAR}}}}", vt, Iteration)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != "{{VAR}}" {
		t.Errorf("got %q, want %q", got, "{{VAR}}")
	}
}

func TestSubstitute_AdjacentTokens(t *testing.T) {
	vt := newVTWithBindings("A", "1", "B", "2")
	got, err := Substitute("{{A}}{{B}}", vt, Iteration)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != "12" {
		t.Errorf("got %q, want %q", got, "12")
	}
}

func TestSubstitute_TokenAtStartOfString(t *testing.T) {
	vt := newVTWithBindings("GREET", "hello")
	got, err := Substitute("{{GREET}} world", vt, Iteration)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != "hello world" {
		t.Errorf("got %q, want %q", got, "hello world")
	}
}

func TestSubstitute_TokenAtEndOfString(t *testing.T) {
	vt := newVTWithBindings("END", "!")
	got, err := Substitute("hello {{END}}", vt, Iteration)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != "hello !" {
		t.Errorf("got %q, want %q", got, "hello !")
	}
}

func TestSubstitute_EmptyInput(t *testing.T) {
	vt := newVTWithBindings()
	got, err := Substitute("", vt, Iteration)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != "" {
		t.Errorf("got %q, want %q", got, "")
	}
}

func TestSubstitute_NilVarTableReturnsInputUnchanged(t *testing.T) {
	got, err := Substitute("{{VAR}}", nil, Iteration)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != "{{VAR}}" {
		t.Errorf("got %q, want %q", got, "{{VAR}}")
	}
}

func TestSubstitute_UnclosedTokenOutputLiterally(t *testing.T) {
	vt := newVTWithBindings()
	// "{{VAR" has no closing }}, so {{ is output literally char by char
	got, err := Substitute("{{VAR", vt, Iteration)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// The scanner sees {{ with no closing }}, outputs { then continues
	if got != "{{VAR" {
		t.Errorf("got %q, want %q", got, "{{VAR")
	}
}

func TestSubstitute_PhaseIterationResolvesIterationVarFirst(t *testing.T) {
	vt := &VarTable{
		persistent: map[string]string{"X": "persistent"},
		iteration:  map[string]string{"X": "iteration"},
		phase:      Iteration,
	}
	got, err := Substitute("{{X}}", vt, Iteration)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != "iteration" {
		t.Errorf("got %q, want %q", got, "iteration")
	}
}

func TestSubstitute_PhaseFinalizeUsesOnlyPersistent(t *testing.T) {
	vt := &VarTable{
		persistent: map[string]string{"X": "persistent"},
		iteration:  map[string]string{"X": "iteration"},
		phase:      Finalize,
	}
	got, err := Substitute("{{X}}", vt, Finalize)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != "persistent" {
		t.Errorf("got %q, want %q", got, "persistent")
	}
}

// ExtractReferences tests

func TestExtractReferences_NoTokens(t *testing.T) {
	refs := ExtractReferences("hello world")
	if len(refs) != 0 {
		t.Errorf("expected no refs, got %v", refs)
	}
}

func TestExtractReferences_SingleToken(t *testing.T) {
	refs := ExtractReferences("hello {{NAME}}")
	if len(refs) != 1 || refs[0] != "NAME" {
		t.Errorf("got %v, want [NAME]", refs)
	}
}

func TestExtractReferences_MultipleTokens(t *testing.T) {
	refs := ExtractReferences("{{A}} and {{B}}")
	if len(refs) != 2 || refs[0] != "A" || refs[1] != "B" {
		t.Errorf("got %v, want [A B]", refs)
	}
}

func TestExtractReferences_DuplicatesReturned(t *testing.T) {
	refs := ExtractReferences("{{X}}-{{X}}")
	if len(refs) != 2 || refs[0] != "X" || refs[1] != "X" {
		t.Errorf("got %v, want [X X]", refs)
	}
}

func TestExtractReferences_EscapesNotIncluded(t *testing.T) {
	// {{{{ and }}}} are escapes, not variable references
	refs := ExtractReferences("{{{{VAR}}}}")
	// {{{{ → escape (skipped), then VAR}}}} →  VAR}} has no closing }},
	// wait: after consuming {{{{, we're at "VAR}}}}". Then }}}} is an escape.
	// But "VAR" is just plain text between {{{{ escape and }}}} escape.
	// So no variable references should be extracted.
	if len(refs) != 0 {
		t.Errorf("expected no refs from escape sequences, got %v", refs)
	}
}

func TestExtractReferences_EmptyInput(t *testing.T) {
	refs := ExtractReferences("")
	if len(refs) != 0 {
		t.Errorf("expected no refs, got %v", refs)
	}
}
