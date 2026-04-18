package logger

import (
	"bufio"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"testing"
	"time"
)

// timestampPrefix matches "[YYYY-MM-DD HH:MM:SS]"
var timestampRe = regexp.MustCompile(`^\[\d{4}-\d{2}-\d{2} \d{2}:\d{2}:\d{2}\]`)

// runStampRe matches the RunStamp() value: "ralph-YYYY-MM-DD-HHMMSS.mmm"
var runStampRe = regexp.MustCompile(`^ralph-\d{4}-\d{2}-\d{2}-\d{6}\.\d{3}$`)

func TestLogLineHasTimestampAndStepPrefix(t *testing.T) {
	dir := t.TempDir()
	l, err := NewLogger(dir)
	if err != nil {
		t.Fatalf("NewLogger: %v", err)
	}

	if err := l.Log("Feature work", "Starting..."); err != nil {
		t.Fatalf("Log: %v", err)
	}
	if err := l.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}

	lines := readLogLines(t, dir)
	if len(lines) != 1 {
		t.Fatalf("expected 1 line, got %d", len(lines))
	}

	line := lines[0]
	if !timestampRe.MatchString(line) {
		t.Errorf("line missing timestamp: %q", line)
	}
	if !strings.Contains(line, "[Feature work]") {
		t.Errorf("line missing step name: %q", line)
	}
	if !strings.Contains(line, "Starting...") {
		t.Errorf("line missing content: %q", line)
	}
}

func TestSetContextUpdatesPrefix(t *testing.T) {
	dir := t.TempDir()
	l, err := NewLogger(dir)
	if err != nil {
		t.Fatalf("NewLogger: %v", err)
	}

	l.SetContext("Iteration 1/3", "")
	if err := l.Log("Feature work", "first"); err != nil {
		t.Fatalf("Log: %v", err)
	}

	l.SetContext("Iteration 2/3", "")
	if err := l.Log("Test writing", "second"); err != nil {
		t.Fatalf("Log: %v", err)
	}

	if err := l.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}

	lines := readLogLines(t, dir)
	if len(lines) != 2 {
		t.Fatalf("expected 2 lines, got %d", len(lines))
	}
	if !strings.Contains(lines[0], "[Iteration 1/3]") {
		t.Errorf("first line missing iteration context: %q", lines[0])
	}
	if !strings.Contains(lines[1], "[Iteration 2/3]") {
		t.Errorf("second line missing updated iteration context: %q", lines[1])
	}
}

func TestConcurrentWritesNoCorruption(t *testing.T) {
	dir := t.TempDir()
	l, err := NewLogger(dir)
	if err != nil {
		t.Fatalf("NewLogger: %v", err)
	}

	const n = 100
	var wg sync.WaitGroup
	wg.Add(2)

	go func() {
		defer wg.Done()
		for range n {
			_ = l.Log("stdout", "stdout line")
		}
	}()

	go func() {
		defer wg.Done()
		for range n {
			_ = l.Log("stderr", "stderr line")
		}
	}()

	wg.Wait()
	if err := l.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}

	lines := readLogLines(t, dir)
	if len(lines) != 2*n {
		t.Errorf("expected %d lines, got %d", 2*n, len(lines))
	}

	// Each line must start with a valid timestamp — no interleaving.
	for _, line := range lines {
		if !timestampRe.MatchString(line) {
			t.Errorf("corrupted/interleaved line: %q", line)
		}
	}
}

func TestLogFileCreatedWithExpectedPattern(t *testing.T) {
	dir := t.TempDir()
	l, err := NewLogger(dir)
	if err != nil {
		t.Fatalf("NewLogger: %v", err)
	}
	_ = l.Close()

	entries, err := os.ReadDir(filepath.Join(dir, ".pr9k", "logs"))
	if err != nil {
		t.Fatalf("ReadDir logs: %v", err)
	}
	if len(entries) != 1 {
		t.Fatalf("expected 1 log file, got %d", len(entries))
	}

	name := entries[0].Name()
	stem := strings.TrimSuffix(name, ".log")
	if stem == name || !runStampRe.MatchString(stem) {
		t.Errorf("unexpected filename: %q", name)
	}
}

func TestRunStampMatchesLogFilename(t *testing.T) {
	dir := t.TempDir()
	l, err := NewLogger(dir)
	if err != nil {
		t.Fatalf("NewLogger: %v", err)
	}
	_ = l.Close()

	entries, err := os.ReadDir(filepath.Join(dir, ".pr9k", "logs"))
	if err != nil {
		t.Fatalf("ReadDir logs: %v", err)
	}
	if len(entries) != 1 {
		t.Fatalf("expected 1 log file, got %d", len(entries))
	}

	want := l.RunStamp() + ".log"
	got := filepath.Base(entries[0].Name())
	if got != want {
		t.Errorf("RunStamp mismatch: RunStamp()=%q, filename=%q", l.RunStamp(), got)
	}
}

func TestSubsecondRunStampDistinct(t *testing.T) {
	dir := t.TempDir()
	l1, err := NewLogger(dir)
	if err != nil {
		t.Fatalf("NewLogger l1: %v", err)
	}
	_ = l1.Close()

	time.Sleep(1 * time.Millisecond)

	l2, err := NewLogger(dir)
	if err != nil {
		t.Fatalf("NewLogger l2: %v", err)
	}
	_ = l2.Close()

	if l1.RunStamp() == l2.RunStamp() {
		t.Errorf("RunStamp values should differ but both are %q", l1.RunStamp())
	}
}

func TestLogsDirectoryCreatedIfMissing(t *testing.T) {
	dir := t.TempDir()
	logsDir := filepath.Join(dir, ".pr9k", "logs")

	// Confirm it doesn't exist yet.
	if _, err := os.Stat(logsDir); !os.IsNotExist(err) {
		t.Fatalf(".pr9k/logs/ should not exist before NewLogger")
	}

	l, err := NewLogger(dir)
	if err != nil {
		t.Fatalf("NewLogger: %v", err)
	}
	_ = l.Close()

	if _, err := os.Stat(logsDir); err != nil {
		t.Errorf(".pr9k/logs/ not created: %v", err)
	}
}

func TestCloseFlushesAndPreventsSubsequentWrites(t *testing.T) {
	dir := t.TempDir()
	l, err := NewLogger(dir)
	if err != nil {
		t.Fatalf("NewLogger: %v", err)
	}

	if err := l.Log("step", "before close"); err != nil {
		t.Fatalf("Log before Close: %v", err)
	}
	if err := l.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}

	// Content must be flushed.
	lines := readLogLines(t, dir)
	if len(lines) != 1 || !strings.Contains(lines[0], "before close") {
		t.Errorf("content not flushed: %v", lines)
	}

	// Writes after Close must return an error.
	if err := l.Log("step", "after close"); err == nil {
		t.Error("expected error writing to closed logger, got nil")
	}
}

func TestCloseIsIdempotent(t *testing.T) {
	dir := t.TempDir()
	l, err := NewLogger(dir)
	if err != nil {
		t.Fatalf("NewLogger: %v", err)
	}
	if err := l.Log("step", "a line"); err != nil {
		t.Fatalf("Log: %v", err)
	}

	if err := l.Close(); err != nil {
		t.Errorf("first Close: %v", err)
	}
	if err := l.Close(); err != nil {
		t.Errorf("second Close: %v", err)
	}
}

func TestNewLoggerErrorOnUnwritableDirectory(t *testing.T) {
	if os.Getuid() == 0 {
		t.Skip("permission checks do not apply when running as root")
	}

	parent := t.TempDir()
	if err := os.Chmod(parent, 0o444); err != nil {
		t.Fatalf("Chmod: %v", err)
	}
	t.Cleanup(func() { _ = os.Chmod(parent, 0o755) })

	_, err := NewLogger(filepath.Join(parent, "sub"))
	if err == nil {
		t.Fatal("expected error from NewLogger on unwritable directory, got nil")
	}
	if !strings.Contains(err.Error(), "logger:") {
		t.Errorf("error missing 'logger:' prefix: %v", err)
	}
}

func TestLogFormatWithoutIterationContext(t *testing.T) {
	dir := t.TempDir()
	l, err := NewLogger(dir)
	if err != nil {
		t.Fatalf("NewLogger: %v", err)
	}

	if err := l.Log("MyStep", "content"); err != nil {
		t.Fatalf("Log: %v", err)
	}
	if err := l.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}

	lines := readLogLines(t, dir)
	if len(lines) != 1 {
		t.Fatalf("expected 1 line, got %d", len(lines))
	}
	line := lines[0]

	// Must have exactly two bracket groups: [timestamp] [stepName]
	twoGroupRe := regexp.MustCompile(`^\[\d{4}-\d{2}-\d{2} \d{2}:\d{2}:\d{2}\] \[MyStep\] `)
	if !twoGroupRe.MatchString(line) {
		t.Errorf("line does not match expected two-group format: %q", line)
	}

	// Must not have a third bracket group.
	threeGroupRe := regexp.MustCompile(`^\[.*\] \[.*\] \[.*\]`)
	if threeGroupRe.MatchString(line) {
		t.Errorf("line has unexpected third bracket group: %q", line)
	}
}

// TP-001: 3-bracket format when iteration context is set.
func TestLogFormatWithIterationContext(t *testing.T) {
	dir := t.TempDir()
	l, err := NewLogger(dir)
	if err != nil {
		t.Fatalf("NewLogger: %v", err)
	}

	l.SetContext("Iteration 1/3", "")
	if err := l.Log("Feature work", "content"); err != nil {
		t.Fatalf("Log: %v", err)
	}
	if err := l.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}

	lines := readLogLines(t, dir)
	if len(lines) != 1 {
		t.Fatalf("expected 1 line, got %d", len(lines))
	}
	line := lines[0]

	// Must match exactly 3 bracket groups: [timestamp] [iteration] [stepName]
	threeGroupRe := regexp.MustCompile(`^\[.*\] \[Iteration 1/3\] \[Feature work\] content$`)
	if !threeGroupRe.MatchString(line) {
		t.Errorf("line does not match expected three-group format: %q", line)
	}
}

func TestSetContextSecondParameterIsUnused(t *testing.T) {
	dir := t.TempDir()
	l, err := NewLogger(dir)
	if err != nil {
		t.Fatalf("NewLogger: %v", err)
	}

	l.SetContext("Iter 1", "Feature work")
	if err := l.Log("Code review", "line"); err != nil {
		t.Fatalf("Log: %v", err)
	}
	if err := l.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}

	lines := readLogLines(t, dir)
	if len(lines) != 1 {
		t.Fatalf("expected 1 line, got %d", len(lines))
	}
	line := lines[0]

	if !strings.Contains(line, "[Code review]") {
		t.Errorf("line missing step name from Log call: %q", line)
	}
	if strings.Contains(line, "[Feature work]") {
		t.Errorf("line contains ignored SetContext second param: %q", line)
	}
}

// TP-RS1: RunStamp() value matches the expected format pattern.
func TestRunStampFormat(t *testing.T) {
	dir := t.TempDir()
	l, err := NewLogger(dir)
	if err != nil {
		t.Fatalf("NewLogger: %v", err)
	}
	defer func() { _ = l.Close() }()

	if !runStampRe.MatchString(l.RunStamp()) {
		t.Errorf("RunStamp() %q does not match expected pattern", l.RunStamp())
	}
}

// TP-RS2: RunStamp() returns the same value on repeated calls (immutability contract).
func TestRunStampStable(t *testing.T) {
	dir := t.TempDir()
	l, err := NewLogger(dir)
	if err != nil {
		t.Fatalf("NewLogger: %v", err)
	}
	defer func() { _ = l.Close() }()

	first := l.RunStamp()
	second := l.RunStamp()
	if first != second {
		t.Errorf("RunStamp() not stable: first=%q, second=%q", first, second)
	}
}

// TP-RS3: RunStamp() is readable after Close (used by main.go during shutdown).
func TestRunStampReadableAfterClose(t *testing.T) {
	dir := t.TempDir()
	l, err := NewLogger(dir)
	if err != nil {
		t.Fatalf("NewLogger: %v", err)
	}
	if err := l.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}

	stamp := l.RunStamp()
	if stamp == "" {
		t.Fatal("RunStamp() returned empty string after Close")
	}
	if !runStampRe.MatchString(stamp) {
		t.Errorf("RunStamp() after Close %q does not match expected pattern", stamp)
	}
}

// readLogLines reads all non-empty lines from the single log file in dir/.pr9k/logs/.
func readLogLines(t *testing.T, dir string) []string {
	t.Helper()
	entries, err := os.ReadDir(filepath.Join(dir, ".pr9k", "logs"))
	if err != nil {
		t.Fatalf("ReadDir: %v", err)
	}
	if len(entries) == 0 {
		t.Fatal("no log files found")
	}

	f, err := os.Open(filepath.Join(dir, ".pr9k", "logs", entries[0].Name()))
	if err != nil {
		t.Fatalf("Open log: %v", err)
	}
	defer func() { _ = f.Close() }()

	var lines []string
	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		if line := scanner.Text(); line != "" {
			lines = append(lines, line)
		}
	}
	return lines
}
