package vars_test

import (
	"testing"

	"github.com/mxriverlynn/pr9k/ralph-tui/internal/vars"
)

// newTable is a test helper that creates a VarTable with fixed seeds.
func newTable() *vars.VarTable {
	return vars.New("/project", 5)
}

// --- Built-in seeding ---

func TestNew_seedsProjectDir(t *testing.T) {
	vt := vars.New("/my/project", 3)
	v, ok := vt.Get("PROJECT_DIR")
	if !ok || v != "/my/project" {
		t.Errorf("expected PROJECT_DIR=/my/project, got %q ok=%v", v, ok)
	}
}

func TestNew_seedsMaxIter(t *testing.T) {
	vt := vars.New("/project", 10)
	v, ok := vt.Get("MAX_ITER")
	if !ok || v != "10" {
		t.Errorf("expected MAX_ITER=10, got %q ok=%v", v, ok)
	}
}

func TestNew_maxIterZeroMeansUnbounded(t *testing.T) {
	vt := vars.New("/project", 0)
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
	vt := vars.New("/project", 7)
	vt.SetIteration(2)
	vt.ResetIteration()

	v, ok := vt.Get("PROJECT_DIR")
	if !ok || v != "/project" {
		t.Errorf("ResetIteration must not clear PROJECT_DIR; got %q ok=%v", v, ok)
	}
	v, ok = vt.Get("MAX_ITER")
	if !ok || v != "7" {
		t.Errorf("ResetIteration must not clear MAX_ITER; got %q ok=%v", v, ok)
	}
	v, ok = vt.Get("ITER")
	if !ok || v != "2" {
		t.Errorf("ResetIteration must not clear ITER; got %q ok=%v", v, ok)
	}
}

// --- Reserved name collision ---

func TestBind_panicOnReservedName(t *testing.T) {
	reserved := []string{"PROJECT_DIR", "MAX_ITER", "ITER", "STEP_NUM", "STEP_COUNT", "STEP_NAME"}
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
