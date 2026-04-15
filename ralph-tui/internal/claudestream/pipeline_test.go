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

// TestPipeline_CloseIdempotent verifies that calling Close() twice on a
// Pipeline with a real RawWriter returns nil on both calls (TP-PL2, coding std).
func TestPipeline_CloseIdempotent(t *testing.T) {
	dir := t.TempDir()
	artifactPath := filepath.Join(dir, "step.jsonl")

	rw, err := claudestream.NewRawWriter(artifactPath)
	if err != nil {
		t.Fatalf("NewRawWriter: %v", err)
	}
	p := claudestream.NewPipeline(rw)

	if err := p.Close(); err != nil {
		t.Fatalf("first Close: %v", err)
	}
	if err := p.Close(); err != nil {
		t.Fatalf("second Close should return nil: %v", err)
	}
}

// TestPipeline_LastEventAtAdvancesOnMalformed verifies that a malformed line
// updates LastEventAt (any activity counts, D23) (TP-PL3).
func TestPipeline_LastEventAtAdvancesOnMalformed(t *testing.T) {
	p := claudestream.NewPipeline(nil)

	if !p.LastEventAt().IsZero() {
		t.Error("LastEventAt should be zero before any observe")
	}

	p.Observe([]byte(`not json at all`))

	if p.LastEventAt().IsZero() {
		t.Error("LastEventAt should be non-zero after a malformed line")
	}
}

// TestPipeline_CloseNilRawWriter verifies that Close() returns nil on both the
// first and second call when the Pipeline was constructed with nil RawWriter
// (TP-PL1/PL6, coding std).
func TestPipeline_CloseNilRawWriter(t *testing.T) {
	p := claudestream.NewPipeline(nil)

	if err := p.Close(); err != nil {
		t.Fatalf("first Close with nil RawWriter: %v", err)
	}
	if err := p.Close(); err != nil {
		t.Fatalf("second Close with nil RawWriter: %v", err)
	}
}

// TestPipeline_AccessorIdentity verifies that Aggregator() and Renderer()
// return the same internal instances on every call (TP-PL5).
func TestPipeline_AccessorIdentity(t *testing.T) {
	p := claudestream.NewPipeline(nil)

	agg1, agg2 := p.Aggregator(), p.Aggregator()
	if agg1 != agg2 {
		t.Error("Aggregator() should return the same instance on every call")
	}
	rend1, rend2 := p.Renderer(), p.Renderer()
	if rend1 != rend2 {
		t.Error("Renderer() should return the same instance on every call")
	}
}

// TestPipeline_SmokeSuccess_DisplayLines feeds the smoke-success fixture
// through a Pipeline and spot-checks the display lines returned by Observe,
// validating end-to-end Renderer wiring (TP-I1, D5, D11).
func TestPipeline_SmokeSuccess_DisplayLines(t *testing.T) {
	p := claudestream.NewPipeline(nil)
	fixtPath := filepath.Join(fixturesDir(t), "smoke-success.ndjson")

	allLines := readLines(t, fixtPath)
	var nonEmpty []string
	for _, l := range allLines {
		if l != "" {
			nonEmpty = append(nonEmpty, l)
		}
	}
	if len(nonEmpty) == 0 {
		t.Fatal("fixture has no lines")
	}

	type obs struct {
		display []string
		isNil   bool
	}
	results := make([]obs, 0, len(nonEmpty))
	for _, l := range nonEmpty {
		d := p.Observe([]byte(l))
		results = append(results, obs{display: d, isNil: d == nil})
	}

	// The last line is the result event — it must produce nil display.
	last := results[len(results)-1]
	if !last.isNil {
		t.Errorf("result event should return nil display, got %v", last.display)
	}

	// First non-nil return should contain the init banner.
	var firstNonNil []string
	for _, o := range results {
		if !o.isNil {
			firstNonNil = o.display
			break
		}
	}
	if firstNonNil == nil {
		t.Fatal("expected at least one non-nil return from Observe")
	}
	if len(firstNonNil) == 0 || !strings.Contains(firstNonNil[0], "[claude session") {
		t.Errorf("first non-nil return should contain init banner, got %v", firstNonNil)
	}

	// At least one display line must contain assistant text.
	var allDisplay []string
	for _, o := range results {
		allDisplay = append(allDisplay, o.display...)
	}
	foundText := false
	for _, dl := range allDisplay {
		if dl == "Hello there, friend." {
			foundText = true
			break
		}
	}
	if !foundText {
		t.Errorf("expected assistant text in display lines, got %v", allDisplay)
	}
	if len(allDisplay) == 0 {
		t.Error("expected non-zero total display lines")
	}
}

// TestPipeline_ArtifactContainsVerbatimLines verifies that every input line
// fed through Pipeline appears verbatim in the artifact file (TP-I2, D14).
func TestPipeline_ArtifactContainsVerbatimLines(t *testing.T) {
	dir := t.TempDir()
	artifactPath := filepath.Join(dir, "step.jsonl")

	rw, err := claudestream.NewRawWriter(artifactPath)
	if err != nil {
		t.Fatalf("NewRawWriter: %v", err)
	}
	p := claudestream.NewPipeline(rw)

	knownLines := []string{
		`{"type":"system","subtype":"init","session_id":"s1","model":"m"}`,
		`{"type":"assistant","message":{"id":"m1","model":"m","content":[{"type":"text","text":"hi"}],"usage":{}},"session_id":"s1","uuid":"u1"}`,
		`{"type":"result","subtype":"success","is_error":false,"duration_ms":100,"num_turns":1,"result":"hi","session_id":"s1","total_cost_usd":0,"usage":{},"uuid":"u2"}`,
	}
	for _, line := range knownLines {
		p.Observe([]byte(line))
	}
	if err := p.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}

	got, err := os.ReadFile(artifactPath)
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}
	content := string(got)
	for _, line := range knownLines {
		if !strings.Contains(content, line) {
			t.Errorf("artifact missing verbatim line: %q", line)
		}
	}
}

// TestPipeline_SmokeAuthFailure_ErrorMessage verifies that the error returned
// by Aggregator().Err() for the auth-failure fixture contains "is_error=true"
// and the fixture's session ID (TP-I3, D15).
func TestPipeline_SmokeAuthFailure_ErrorMessage(t *testing.T) {
	p := claudestream.NewPipeline(nil)
	fixtPath := filepath.Join(fixturesDir(t), "smoke-auth-failure.ndjson")
	feedFixture(t, p, fixtPath)

	err := p.Aggregator().Err()
	if err == nil {
		t.Fatal("expected non-nil error for auth-failure fixture")
	}
	msg := err.Error()
	if !strings.Contains(msg, "is_error=true") {
		t.Errorf("error message should contain 'is_error=true': %q", msg)
	}
	// Session ID from the smoke-auth-failure.ndjson fixture.
	const wantSession = "004fdbf6-7f5f-4fdb-aa5a-e43c0a50c42d"
	if !strings.Contains(msg, wantSession) {
		t.Errorf("error message should contain session ID %q: %q", wantSession, msg)
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
	defer func() { _ = f.Close() }()

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
