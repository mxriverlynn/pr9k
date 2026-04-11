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

// --- StepStartBanner ---

func TestStepStartBanner_Format(t *testing.T) {
	heading, underline := StepStartBanner("Feature work")
	wantHeading := "Starting step: Feature work"
	if heading != wantHeading {
		t.Errorf("heading: got %q, want %q", heading, wantHeading)
	}
	// Underline must match the heading's display width, using the same
	// box-drawing character the iteration separator uses.
	wantUnderlineRunes := len([]rune(wantHeading))
	gotUnderlineRunes := len([]rune(underline))
	if gotUnderlineRunes != wantUnderlineRunes {
		t.Errorf("underline rune count: got %d, want %d (heading %q, underline %q)",
			gotUnderlineRunes, wantUnderlineRunes, heading, underline)
	}
	for _, r := range underline {
		if r != '─' {
			t.Errorf("underline contains non-'─' rune %q: %q", r, underline)
			break
		}
	}
}

func TestStepStartBanner_EmptyName(t *testing.T) {
	heading, underline := StepStartBanner("")
	wantHeading := "Starting step: "
	if heading != wantHeading {
		t.Errorf("heading: got %q, want %q", heading, wantHeading)
	}
	if len([]rune(underline)) != len([]rune(wantHeading)) {
		t.Errorf("underline must match heading width: heading %q, underline %q", heading, underline)
	}
}

// Step names with multi-byte runes must still produce a matching-width
// underline (rune count, not byte count).
func TestStepStartBanner_UnicodeName(t *testing.T) {
	name := "αβγ-step"
	heading, underline := StepStartBanner(name)
	wantRunes := len([]rune("Starting step: " + name))
	if len([]rune(underline)) != wantRunes {
		t.Errorf("underline rune count: got %d, want %d (heading %q, underline %q)",
			len([]rune(underline)), wantRunes, heading, underline)
	}
}

// --- PhaseBanner ---

func TestPhaseBanner_HeadingIsPhaseName(t *testing.T) {
	heading, _ := PhaseBanner("Initializing", 40)
	if heading != "Initializing" {
		t.Errorf("heading: got %q, want %q", heading, "Initializing")
	}
}

func TestPhaseBanner_UnderlineMatchesRequestedWidth(t *testing.T) {
	cases := []int{1, 10, 80, 120, 240}
	for _, width := range cases {
		_, underline := PhaseBanner("Initializing", width)
		gotRunes := len([]rune(underline))
		if gotRunes != width {
			t.Errorf("width %d: underline rune count = %d, want %d (%q)", width, gotRunes, width, underline)
		}
		for _, r := range underline {
			if r != '═' {
				t.Errorf("width %d: underline contains non-'═' rune %q", width, r)
				break
			}
		}
	}
}

// A width of 0 or negative clamps to 1 so the underline never has zero length.
func TestPhaseBanner_NonPositiveWidthClampsToOne(t *testing.T) {
	for _, width := range []int{0, -1, -100} {
		_, underline := PhaseBanner("Initializing", width)
		if len([]rune(underline)) != 1 {
			t.Errorf("width %d: expected clamped 1-rune underline, got %q", width, underline)
		}
	}
}

// --- CaptureLog ---

func TestCaptureLog_SimpleValue(t *testing.T) {
	got := CaptureLog("ISSUE_ID", "42")
	want := `Captured ISSUE_ID = "42"`
	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestCaptureLog_EmptyValue(t *testing.T) {
	got := CaptureLog("ISSUE_ID", "")
	want := `Captured ISSUE_ID = ""`
	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

// Multi-line or whitespace-heavy values must render on a single log line via
// %q escaping so the log body stays line-oriented.
func TestCaptureLog_MultiLineValueIsEscapedToSingleLine(t *testing.T) {
	got := CaptureLog("STATUS", "line1\nline2")
	want := `Captured STATUS = "line1\nline2"`
	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}
	if strings.ContainsRune(got, '\n') {
		t.Errorf("CaptureLog output must not contain raw newlines: %q", got)
	}
}

func TestCaptureLog_ValueWithQuotes(t *testing.T) {
	got := CaptureLog("MSG", `he said "hi"`)
	want := `Captured MSG = "he said \"hi\""`
	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}
