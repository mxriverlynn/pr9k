package vars_test

import (
	"testing"

	"github.com/mxriverlynn/pr9k/ralph-tui/internal/vars"
)

// newTable is a test helper that creates a VarTable with fixed seeds.
func newTable() *vars.VarTable {
	return vars.New("/workflow", "/project", 5)
}

// --- Built-in seeding ---

func TestNew_SeedsBothBuiltins(t *testing.T) {
	vt := vars.New("/my/workflow", "/my/project", 3)

	w, wok := vt.Get("WORKFLOW_DIR")
	if !wok || w != "/my/workflow" {
		t.Errorf("expected WORKFLOW_DIR=/my/workflow, got %q ok=%v", w, wok)
	}

	p, pok := vt.Get("PROJECT_DIR")
	if !pok || p != "/my/project" {
		t.Errorf("expected PROJECT_DIR=/my/project, got %q ok=%v", p, pok)
	}
}

func TestNew_seedsMaxIter(t *testing.T) {
	vt := vars.New("/workflow", "/project", 10)
	v, ok := vt.Get("MAX_ITER")
	if !ok || v != "10" {
		t.Errorf("expected MAX_ITER=10, got %q ok=%v", v, ok)
	}
}

func TestNew_maxIterZeroMeansUnbounded(t *testing.T) {
	vt := vars.New("/workflow", "/project", 0)
	v, ok := vt.Get("MAX_ITER")
	if !ok || v != "0" {
		t.Errorf("expected MAX_ITER=0 for unbounded, got %q ok=%v", v, ok)
	}
}

// --- SetIteration ---

func TestSetIteration_bindsITER(t *testing.T) {
	vt := newTable()
	vt.SetIteration(3)
	v, ok := vt.Get("ITER")
	if !ok || v != "3" {
		t.Errorf("expected ITER=3, got %q ok=%v", v, ok)
	}
}

// --- SetStep ---

func TestSetStep_bindsStepBuiltins(t *testing.T) {
	vt := newTable()
	vt.SetStep(2, 7, "Feature work")
	cases := []struct{ key, want string }{
		{"STEP_NUM", "2"},
		{"STEP_COUNT", "7"},
		{"STEP_NAME", "Feature work"},
	}
	for _, c := range cases {
		v, ok := vt.Get(c.key)
		if !ok || v != c.want {
			t.Errorf("expected %s=%s, got %q ok=%v", c.key, c.want, v, ok)
		}
	}
}

// --- Bind: initialize phase ---

func TestBind_initializePhasePersists(t *testing.T) {
	vt := newTable()
	vt.Bind(vars.Initialize, "GITHUB_USER", "octocat")
	v, ok := vt.Get("GITHUB_USER")
	if !ok || v != "octocat" {
		t.Errorf("expected GITHUB_USER=octocat, got %q ok=%v", v, ok)
	}
}

func TestBind_initializeVisibleInFinalize(t *testing.T) {
	vt := newTable()
	vt.Bind(vars.Initialize, "GITHUB_USER", "octocat")
	vt.SetPhase(vars.Finalize)
	v, ok := vt.Get("GITHUB_USER")
	if !ok || v != "octocat" {
		t.Errorf("initialize binding should be visible in finalize, got %q ok=%v", v, ok)
	}
}

// --- Bind: iteration phase ---

func TestBind_iterationPhaseInIterationTable(t *testing.T) {
	vt := newTable()
	vt.SetPhase(vars.Iteration)
	vt.Bind(vars.Iteration, "ISSUE_ID", "42")
	v, ok := vt.Get("ISSUE_ID")
	if !ok || v != "42" {
		t.Errorf("expected ISSUE_ID=42 in iteration phase, got %q ok=%v", v, ok)
	}
}

func TestBind_iterationNotVisibleInFinalize(t *testing.T) {
	vt := newTable()
	vt.Bind(vars.Iteration, "ISSUE_ID", "42")

	// Finalize only sees the persistent table.
	_, ok := vt.GetInPhase(vars.Finalize, "ISSUE_ID")
	if ok {
		t.Error("iteration-scoped variable ISSUE_ID must not be visible in finalize phase")
	}
}

func TestBind_iterationNotVisibleInInitialize(t *testing.T) {
	vt := newTable()
	vt.Bind(vars.Iteration, "ISSUE_ID", "42")

	_, ok := vt.GetInPhase(vars.Initialize, "ISSUE_ID")
	if ok {
		t.Error("iteration-scoped variable ISSUE_ID must not be visible in initialize phase")
	}
}

// --- New starts in Initialize phase ---

func TestNew_startsInInitializePhase(t *testing.T) {
	vt := newTable()
	vt.Bind(vars.Iteration, "ISSUE_ID", "99")
	// Iteration-scoped variables are not visible in the Initialize phase.
	_, ok := vt.Get("ISSUE_ID")
	if ok {
		t.Error("expected iteration-scoped variable to be invisible in Initialize phase")
	}
}

// --- Bind overwrite semantics ---

func TestBind_initializePhaseOverwritesExistingValue(t *testing.T) {
	vt := newTable()
	vt.Bind(vars.Initialize, "MY_VAR", "first")
	vt.Bind(vars.Initialize, "MY_VAR", "second")
	v, ok := vt.Get("MY_VAR")
	if !ok || v != "second" {
		t.Errorf("expected MY_VAR=second after overwrite, got %q ok=%v", v, ok)
	}
}

func TestBind_iterationPhaseOverwritesExistingValue(t *testing.T) {
	vt := newTable()
	vt.Bind(vars.Iteration, "ISSUE_ID", "1")
	vt.Bind(vars.Iteration, "ISSUE_ID", "2")
	vt.SetPhase(vars.Iteration)
	v, ok := vt.Get("ISSUE_ID")
	if !ok || v != "2" {
		t.Errorf("expected ISSUE_ID=2 after overwrite, got %q ok=%v", v, ok)
	}
}

// --- Resolution order ---

func TestGet_iterationShadowsPersistent(t *testing.T) {
	vt := newTable()
	vt.Bind(vars.Initialize, "FOO", "persistent-value")
	vt.Bind(vars.Iteration, "FOO", "iteration-value")
	vt.SetPhase(vars.Iteration)

	v, ok := vt.Get("FOO")
	if !ok || v != "iteration-value" {
		t.Errorf("iteration table must shadow persistent; got %q ok=%v", v, ok)
	}
}

func TestGet_persistentFallbackWhenNotInIteration(t *testing.T) {
	vt := newTable()
	vt.Bind(vars.Initialize, "FOO", "persistent-value")
	vt.SetPhase(vars.Iteration)

	v, ok := vt.Get("FOO")
	if !ok || v != "persistent-value" {
		t.Errorf("persistent fallback should work in iteration phase; got %q ok=%v", v, ok)
	}
}

func TestGet_missingVariableReturnsFalse(t *testing.T) {
	vt := newTable()
	_, ok := vt.Get("NONEXISTENT")
	if ok {
		t.Error("expected ok=false for unbound variable")
	}
}

func TestGet_missingVariableReturnsEmptyString(t *testing.T) {
	vt := newTable()
	v, ok := vt.Get("NONEXISTENT")
	if ok {
		t.Error("expected ok=false for unbound variable")
	}
	if v != "" {
		t.Errorf("expected empty string for unbound variable, got %q", v)
	}
}

// --- ResetIteration ---

func TestResetIteration_clearsIterationTable(t *testing.T) {
	vt := newTable()
	vt.Bind(vars.Iteration, "ISSUE_ID", "42")
	vt.ResetIteration()
	vt.SetPhase(vars.Iteration)

	_, ok := vt.Get("ISSUE_ID")
	if ok {
		t.Error("ResetIteration must clear iteration-scoped variables")
	}
}

func TestResetIteration_preservesPersistentTable(t *testing.T) {
	vt := newTable()
	vt.Bind(vars.Initialize, "GITHUB_USER", "octocat")
	vt.ResetIteration()

	v, ok := vt.Get("GITHUB_USER")
	if !ok || v != "octocat" {
		t.Errorf("ResetIteration must not clear the persistent table; got %q ok=%v", v, ok)
	}
}

func TestResetIteration_preservesBuiltins(t *testing.T) {
	vt := vars.New("/workflow", "/project", 7)
	vt.SetIteration(2)
	vt.ResetIteration()

	cases := []struct{ key, want string }{
		{"WORKFLOW_DIR", "/workflow"},
		{"PROJECT_DIR", "/project"},
		{"MAX_ITER", "7"},
		{"ITER", "2"},
	}
	for _, c := range cases {
		v, ok := vt.Get(c.key)
		if !ok || v != c.want {
			t.Errorf("ResetIteration must not clear %s; got %q ok=%v", c.key, v, ok)
		}
	}
}

// --- Reserved name collision ---

func TestBind_panicOnReservedName(t *testing.T) {
	reserved := []string{"WORKFLOW_DIR", "PROJECT_DIR", "MAX_ITER", "ITER", "STEP_NUM", "STEP_COUNT", "STEP_NAME"}
	for _, name := range reserved {
		t.Run(name, func(t *testing.T) {
			vt := newTable()
			defer func() {
				if r := recover(); r == nil {
					t.Errorf("expected panic when binding reserved name %q", name)
				}
			}()
			vt.Bind(vars.Initialize, name, "bad")
		})
	}
}

func TestBind_panicOnReservedNameInIterationPhase(t *testing.T) {
	vt := newTable()
	defer func() {
		if r := recover(); r == nil {
			t.Error("expected panic when binding reserved name ITER in iteration phase")
		}
	}()
	vt.Bind(vars.Iteration, "ITER", "99")
}

func TestBind_panicInFinalizePhase(t *testing.T) {
	vt := newTable()
	defer func() {
		if r := recover(); r == nil {
			t.Error("expected panic when binding any variable in finalize phase")
		}
	}()
	vt.Bind(vars.Finalize, "MY_VAR", "value")
}

// --- GetInPhase ---

func TestGetInPhase_iterationPhaseResolvesIterationFirst(t *testing.T) {
	vt := newTable()
	vt.Bind(vars.Initialize, "FOO", "persistent")
	vt.Bind(vars.Iteration, "FOO", "iteration")

	v, ok := vt.GetInPhase(vars.Iteration, "FOO")
	if !ok || v != "iteration" {
		t.Errorf("GetInPhase(Iteration) should return iteration value; got %q ok=%v", v, ok)
	}
}

func TestGetInPhase_finalizePhaseOnlyPersistent(t *testing.T) {
	vt := newTable()
	vt.Bind(vars.Initialize, "FOO", "persistent")
	vt.Bind(vars.Iteration, "FOO", "iteration")

	v, ok := vt.GetInPhase(vars.Finalize, "FOO")
	if !ok || v != "persistent" {
		t.Errorf("GetInPhase(Finalize) should return persistent value; got %q ok=%v", v, ok)
	}
}

// --- Step built-ins are visible in their phase ---

func TestSetStep_visibleDuringIteration(t *testing.T) {
	vt := newTable()
	vt.SetStep(3, 8, "Code review")
	vt.SetPhase(vars.Iteration)

	cases := []struct{ key, want string }{
		{"STEP_NUM", "3"},
		{"STEP_COUNT", "8"},
		{"STEP_NAME", "Code review"},
	}
	for _, c := range cases {
		v, ok := vt.Get(c.key)
		if !ok || v != c.want {
			t.Errorf("expected %s=%s in iteration phase, got %q ok=%v", c.key, c.want, v, ok)
		}
	}
}

func TestSetStep_visibleDuringFinalize(t *testing.T) {
	vt := newTable()
	vt.SetStep(1, 3, "Finalize step")
	vt.SetPhase(vars.Finalize)

	v, ok := vt.Get("STEP_NAME")
	if !ok || v != "Finalize step" {
		t.Errorf("STEP_NAME should be visible in finalize phase; got %q ok=%v", v, ok)
	}
}

// --- AllCaptures ---

// TP-016: AllCaptures on a fresh table returns a non-nil empty map for all phases.
func TestAllCaptures_FreshTable_NonNilEmptyMap(t *testing.T) {
	for _, phase := range []vars.Phase{vars.Initialize, vars.Iteration, vars.Finalize} {
		vt := vars.New("/wf", "/pj", 5)
		got := vt.AllCaptures(phase)
		if got == nil {
			t.Errorf("phase %v: expected non-nil map, got nil", phase)
		}
		if len(got) != 0 {
			t.Errorf("phase %v: expected empty map, got %v", phase, got)
		}
	}
}

// TP-002: AllCaptures for the Iteration phase returns only non-reserved captures
// and excludes all built-in names.
func TestAllCaptures_ReservedNamesFiltered(t *testing.T) {
	vt := vars.New("/wf", "/pj", 10)
	vt.SetIteration(3)
	vt.SetStep(1, 2, "n")
	vt.Bind(vars.Initialize, "FOO", "bar")

	got := vt.AllCaptures(vars.Iteration)

	if got["FOO"] != "bar" {
		t.Errorf("expected FOO=bar in Iteration captures, got %v", got)
	}

	for _, reserved := range []string{"WORKFLOW_DIR", "PROJECT_DIR", "MAX_ITER", "ITER", "STEP_NUM", "STEP_COUNT", "STEP_NAME"} {
		if _, ok := got[reserved]; ok {
			t.Errorf("reserved name %q must not appear in AllCaptures result", reserved)
		}
	}
}

// TP-003: AllCaptures respects iteration-shadows-persistent for Iteration phase
// and returns only persistent for Initialize/Finalize.
func TestAllCaptures_IterationShadowsPersistent(t *testing.T) {
	vt := vars.New("/wf", "/pj", 1)
	vt.Bind(vars.Initialize, "K", "p")
	vt.Bind(vars.Iteration, "K", "i")

	if got := vt.AllCaptures(vars.Iteration)["K"]; got != "i" {
		t.Errorf("AllCaptures(Iteration)[K]: want %q, got %q", "i", got)
	}
	if got := vt.AllCaptures(vars.Initialize)["K"]; got != "p" {
		t.Errorf("AllCaptures(Initialize)[K]: want %q, got %q", "p", got)
	}
	if got := vt.AllCaptures(vars.Finalize)["K"]; got != "p" {
		t.Errorf("AllCaptures(Finalize)[K]: want %q, got %q", "p", got)
	}
}

// TP-017: AllCaptures returns a defensive copy — mutating the returned map must
// not affect the VarTable state or subsequent AllCaptures calls.
func TestAllCaptures_DefensiveCopy(t *testing.T) {
	vt := vars.New("/wf", "/pj", 1)
	vt.Bind(vars.Iteration, "FOO", "original")

	snap1 := vt.AllCaptures(vars.Iteration)

	// Mutate the first snapshot.
	snap1["FOO"] = "mutated"
	snap1["EXTRA"] = "injected"

	// Second snapshot must reflect the original VarTable state.
	snap2 := vt.AllCaptures(vars.Iteration)
	if snap2["FOO"] != "original" {
		t.Errorf("second AllCaptures: FOO: want %q, got %q", "original", snap2["FOO"])
	}
	if _, ok := snap2["EXTRA"]; ok {
		t.Error("second AllCaptures: EXTRA must not be present after mutating first snapshot")
	}

	// A Bind call after the first snapshot must not mutate it.
	vt.Bind(vars.Iteration, "NEW", "v")
	if _, ok := snap1["NEW"]; ok {
		t.Error("snap1 must not contain NEW added after snapshot was taken")
	}
}

// TP-030: reserved names are excluded from AllCaptures even when they exist in the
// persistent table via SetIteration and SetStep.
func TestAllCaptures_ReservedNamesExcludedAfterSetIterationSetStep(t *testing.T) {
	vt := vars.New("/wf", "/pj", 3)
	vt.SetIteration(9)
	vt.SetStep(2, 4, "my-step")

	for _, phase := range []vars.Phase{vars.Initialize, vars.Iteration, vars.Finalize} {
		got := vt.AllCaptures(phase)
		for _, reserved := range []string{"WORKFLOW_DIR", "PROJECT_DIR", "MAX_ITER", "ITER", "STEP_NUM", "STEP_COUNT", "STEP_NAME"} {
			if _, ok := got[reserved]; ok {
				t.Errorf("phase %v: reserved name %q must not appear in AllCaptures after SetIteration/SetStep", phase, reserved)
			}
		}
	}
}
