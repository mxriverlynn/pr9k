package claudestream_test

import (
	"bufio"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/mxriverlynn/pr9k/ralph-tui/internal/claudestream"
)

// sentinel line written after a result event (D26).
const sentinelLine = `{"type":"ralph_end","ok":true,"schema":"v1"}`

func TestPipeline_SmokeSuccess(t *testing.T) {
	dir := t.TempDir()
	artifactPath := filepath.Join(dir, "step.jsonl")

	rw, err := claudestream.NewRawWriter(artifactPath)
	if err != nil {
		t.Fatalf("NewRawWriter: %v", err)
	}
	p := claudestream.NewPipeline(rw)

	fixtPath := filepath.Join(fixturesDir(t), "smoke-success.ndjson")
	feedFixture(t, p, fixtPath)

	if err := p.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}

	// Aggregator should have a successful result.
	if err := p.Aggregator().Err(); err != nil {
		t.Fatalf("Aggregator.Err() should be nil: %v", err)
	}
	if p.Aggregator().Result() == "" {
		t.Error("Aggregator.Result() should be non-empty")
	}
	if p.Aggregator().Result() != "Hello there, friend." {
		t.Errorf("Aggregator.Result() = %q", p.Aggregator().Result())
	}

	// .jsonl must end with the ralph_end sentinel.
	assertSentinelPresent(t, artifactPath)
}

func TestPipeline_SmokeAuthFailure(t *testing.T) {
	dir := t.TempDir()
	artifactPath := filepath.Join(dir, "step.jsonl")

	rw, err := claudestream.NewRawWriter(artifactPath)
	if err != nil {
		t.Fatalf("NewRawWriter: %v", err)
	}
	p := claudestream.NewPipeline(rw)

	fixtPath := filepath.Join(fixturesDir(t), "smoke-auth-failure.ndjson")
	feedFixture(t, p, fixtPath)

	if err := p.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}

	// Auth failure has a result event with is_error=true.
	if err := p.Aggregator().Err(); err == nil {
		t.Fatal("Aggregator.Err() should be non-nil for auth failure fixture")
	}
	// Sentinel IS written because a result event arrived (D26).
	assertSentinelPresent(t, artifactPath)
}

func TestPipeline_TruncatedStream_NoSentinel(t *testing.T) {
	// Feed smoke-success fixture minus the last result line.
	fixtPath := filepath.Join(fixturesDir(t), "smoke-success.ndjson")
	lines := readLines(t, fixtPath)
	// Drop the last non-empty line (the result event).
	for len(lines) > 0 && lines[len(lines)-1] == "" {
		lines = lines[:len(lines)-1]
	}
	if len(lines) == 0 {
		t.Fatal("fixture has no lines")
	}
	truncated := lines[:len(lines)-1]

	dir := t.TempDir()
	artifactPath := filepath.Join(dir, "step.jsonl")
	rw, err := claudestream.NewRawWriter(artifactPath)
	if err != nil {
		t.Fatalf("NewRawWriter: %v", err)
	}
	p := claudestream.NewPipeline(rw)

	for _, line := range truncated {
		p.Observe([]byte(line))
	}
	if err := p.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}

	// No result event → error.
	if err := p.Aggregator().Err(); err == nil {
		t.Fatal("Aggregator.Err() should be non-nil for truncated stream")
	}
	if !strings.Contains(p.Aggregator().Err().Error(), "no result event") {
		t.Errorf("unexpected error: %v", p.Aggregator().Err())
	}

	// No sentinel in the artifact.
	assertSentinelAbsent(t, artifactPath)
}

func TestPipeline_LastEventAtAdvances(t *testing.T) {
	p := claudestream.NewPipeline(nil)

	if !p.LastEventAt().IsZero() {
		t.Error("LastEventAt should be zero before any observe")
	}

	before := time.Now()
	p.Observe([]byte(`{"type":"system","subtype":"init","session_id":"s1","model":"m"}`))
	after := time.Now()

	ts := p.LastEventAt()
	if ts.Before(before) || ts.After(after) {
		t.Errorf("LastEventAt %v not in [%v, %v]", ts, before, after)
	}

	// Monotonically advances on each call.
	first := p.LastEventAt()
	time.Sleep(time.Millisecond)
	p.Observe([]byte(`{"type":"rate_limit_event","rate_limit_info":{"status":"allowed"},"uuid":"u","session_id":"s"}`))
	second := p.LastEventAt()

	if !second.After(first) {
		t.Errorf("LastEventAt should advance: first=%v second=%v", first, second)
	}
}

func TestPipeline_MalformedLineReturnsNil(t *testing.T) {
	p := claudestream.NewPipeline(nil)
	lines := p.Observe([]byte(`not json`))
	if lines != nil {
		t.Errorf("expected nil display lines for malformed input, got %v", lines)
	}
}

// TestPipeline_LargeLine is a V1 regression guard: a 2MB tool_result line
// must be captured verbatim and parsed successfully (D3).
func TestPipeline_LargeLine(t *testing.T) {
	dir := t.TempDir()
	artifactPath := filepath.Join(dir, "large.jsonl")
	rw, err := claudestream.NewRawWriter(artifactPath)
	if err != nil {
		t.Fatalf("NewRawWriter: %v", err)
	}
	p := claudestream.NewPipeline(rw)

	// Build a 2MB tool_result content string.
	bigContent := strings.Repeat("x", 2*1024*1024)
	// Embed it in a user event (tool_result).
	// We build raw JSON manually to avoid escaping overhead.
	line := `{"type":"user","message":{"content":[{"type":"tool_result","tool_use_id":"t1","content":"` +
		bigContent + `"}]},"uuid":"u1"}`

	displayLines := p.Observe([]byte(line))
	_ = displayLines // user events render nothing

	if err := p.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}

	// Verify verbatim bytes in artifact.
	got, err := os.ReadFile(artifactPath)
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}
	// File contains the large line + "\n"
	if !strings.Contains(string(got), bigContent) {
		t.Error("artifact does not contain the large content verbatim")
	}
}

// feedFixture feeds all non-empty lines of a fixture file to the pipeline.
func feedFixture(t *testing.T, p *claudestream.Pipeline, path string) {
	t.Helper()
	lines := readLines(t, path)
	for _, line := range lines {
		if line == "" {
			continue
		}
		p.Observe([]byte(line))
	}
}

// readLines returns all lines (including empty trailing ones) from a file.
func readLines(t *testing.T, path string) []string {
	t.Helper()
	f, err := os.Open(path)
	if err != nil {
		t.Fatalf("open %s: %v", path, err)
	}
	defer f.Close()

	var lines []string
	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 4*1024*1024), 4*1024*1024)
	for sc.Scan() {
		lines = append(lines, sc.Text())
	}
	if err := sc.Err(); err != nil {
		t.Fatalf("scan %s: %v", path, err)
	}
	return lines
}

// assertSentinelPresent reads the artifact file and verifies the last
// non-empty line is the ralph_end sentinel (D26).
func assertSentinelPresent(t *testing.T, path string) {
	t.Helper()
	lines := readLines(t, path)
	// Find last non-empty line.
	last := ""
	for i := len(lines) - 1; i >= 0; i-- {
		if lines[i] != "" {
			last = lines[i]
			break
		}
	}
	if last != sentinelLine {
		t.Errorf("expected sentinel as last line\ngot:  %q\nwant: %q", last, sentinelLine)
	}
}

// assertSentinelAbsent verifies the artifact does not contain the sentinel.
func assertSentinelAbsent(t *testing.T, path string) {
	t.Helper()
	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}
	if strings.Contains(string(got), sentinelLine) {
		t.Error("artifact should not contain sentinel for truncated stream")
	}
}
