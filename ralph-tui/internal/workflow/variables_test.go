package workflow

import (
	"testing"

	"github.com/mxriverlynn/pr9k/ralph-tui/internal/steps"
)

// T1 — Set and Get
func TestVariablePool_SetAndGet(t *testing.T) {
	pool := NewVariablePool()
	pool.Set("ISSUE_ID", "42")
	v, ok := pool.Get("ISSUE_ID")
	if !ok {
		t.Fatal("expected ok == true for a set variable")
	}
	if v != "42" {
		t.Errorf("expected %q, got %q", "42", v)
	}
}

// T2 — Get missing variable
func TestVariablePool_GetMissingVariable(t *testing.T) {
	pool := NewVariablePool()
	_, ok := pool.Get("NONEXISTENT")
	if ok {
		t.Error("expected ok == false for a variable that was never set")
	}
}

// T3 — Set overwrites previous value
func TestVariablePool_SetOverwrites(t *testing.T) {
	pool := NewVariablePool()
	pool.Set("KEY", "first")
	pool.Set("KEY", "second")
	v, ok := pool.Get("KEY")
	if !ok {
		t.Fatal("expected ok == true after Set")
	}
	if v != "second" {
		t.Errorf("expected %q (second value), got %q", "second", v)
	}
}

// T4 — All returns a copy; mutations do not affect the pool
func TestVariablePool_AllReturnsCopy(t *testing.T) {
	pool := NewVariablePool()
	pool.Set("A", "1")
	pool.Set("B", "2")

	all := pool.All()
	all["A"] = "mutated"
	all["C"] = "injected"

	if v, _ := pool.Get("A"); v != "1" {
		t.Errorf("pool was mutated via All() map: expected %q, got %q", "1", v)
	}
	if _, ok := pool.Get("C"); ok {
		t.Error("injected key appeared in pool via All() map mutation")
	}
}

// T5 — Clear removes specified keys; other keys remain
func TestVariablePool_ClearRemovesSpecifiedKeys(t *testing.T) {
	pool := NewVariablePool()
	pool.Set("X", "x")
	pool.Set("Y", "y")
	pool.Set("Z", "z")

	pool.Clear([]string{"X", "Y"})

	if _, ok := pool.Get("X"); ok {
		t.Error("expected X to be cleared")
	}
	if _, ok := pool.Get("Y"); ok {
		t.Error("expected Y to be cleared")
	}
	if v, ok := pool.Get("Z"); !ok || v != "z" {
		t.Errorf("expected Z to remain with value %q, got ok=%v v=%q", "z", ok, v)
	}
}

// T6 — Clear with nonexistent keys does not panic
func TestVariablePool_ClearNonexistentKeys(t *testing.T) {
	pool := NewVariablePool()
	pool.Set("PRESENT", "value")

	// Should not panic.
	pool.Clear([]string{"NONEXISTENT", "ALSO_MISSING"})

	if v, ok := pool.Get("PRESENT"); !ok || v != "value" {
		t.Errorf("expected PRESENT to remain, got ok=%v v=%q", ok, v)
	}
}

// T12 — LoopVariableNames returns all outputVariable names from loop steps.
func TestLoopVariableNames(t *testing.T) {
	cfg := &steps.WorkflowConfig{
		PreLoop: []steps.Step{
			{Name: "SetPre", Command: []string{"set-pre"}, OutputVariable: "PRE_VAR"},
		},
		Loop: []steps.Step{
			{Name: "Step1", Command: []string{"step1"}, OutputVariable: "LOOP_A"},
			{Name: "Step2", Command: []string{"step2"}},
			{Name: "Step3", Command: []string{"step3"}, OutputVariable: "LOOP_B"},
		},
		PostLoop: []steps.Step{
			{Name: "PostStep", Command: []string{"post"}, OutputVariable: "POST_VAR"},
		},
	}

	got := LoopVariableNames(cfg)
	if len(got) != 2 {
		t.Fatalf("expected 2 loop variable names, got %d: %v", len(got), got)
	}
	if got[0] != "LOOP_A" {
		t.Errorf("expected got[0] == %q, got %q", "LOOP_A", got[0])
	}
	if got[1] != "LOOP_B" {
		t.Errorf("expected got[1] == %q, got %q", "LOOP_B", got[1])
	}
}
