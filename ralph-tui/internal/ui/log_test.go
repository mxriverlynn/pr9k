package ui

import (
	"strings"
	"testing"
)

func TestStepSeparator_Format(t *testing.T) {
	got := StepSeparator("Feature work")
	want := "── Feature work ─────────────"
	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestRetryStepSeparator_Format(t *testing.T) {
	got := RetryStepSeparator("Feature work")
	want := "── Feature work (retry) ─────────────"
	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestStepSeparator_ConsistentFormat(t *testing.T) {
	names := []string{"Feature work", "Test planning", "Code review", "Git push"}
	for _, name := range names {
		got := StepSeparator(name)
		if !strings.HasPrefix(got, "── ") {
			t.Errorf("separator for %q missing prefix: %q", name, got)
		}
		if !strings.HasSuffix(got, " ─────────────") {
			t.Errorf("separator for %q missing suffix: %q", name, got)
		}
		if !strings.Contains(got, name) {
			t.Errorf("separator for %q missing step name: %q", name, got)
		}
	}
}

func TestRetryStepSeparator_ConsistentFormat(t *testing.T) {
	names := []string{"Feature work", "Test planning", "Code review", "Git push"}
	for _, name := range names {
		got := RetryStepSeparator(name)
		if !strings.HasPrefix(got, "── ") {
			t.Errorf("retry separator for %q missing prefix: %q", name, got)
		}
		if !strings.HasSuffix(got, " ─────────────") {
			t.Errorf("retry separator for %q missing suffix: %q", name, got)
		}
		if !strings.Contains(got, name) {
			t.Errorf("retry separator for %q missing step name: %q", name, got)
		}
		if !strings.Contains(got, "(retry)") {
			t.Errorf("retry separator for %q missing '(retry)': %q", name, got)
		}
	}
}

// T4 — StepSeparator with empty step name
func TestStepSeparator_WithEmptyName(t *testing.T) {
	got := StepSeparator("")
	want := "──  ─────────────"
	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

// T5 — RetryStepSeparator with empty step name
func TestRetryStepSeparator_WithEmptyName(t *testing.T) {
	got := RetryStepSeparator("")
	want := "──  (retry) ─────────────"
	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}
