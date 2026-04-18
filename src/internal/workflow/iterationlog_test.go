package workflow

import (
	"bufio"
	"encoding/json"
	"os"
	"path/filepath"
	"sync"
	"testing"
)

func makeRecord(stepName, status string, iterNum int) IterationRecord {
	return IterationRecord{
		SchemaVersion: 1,
		IssueID:       "42",
		IterationNum:  iterNum,
		StepName:      stepName,
		Status:        status,
		DurationS:     1.5,
	}
}

// TestAppendIterationRecord_OneRecord verifies that appending one record
// produces exactly one JSON line in the file.
func TestAppendIterationRecord_OneRecord(t *testing.T) {
	dir := t.TempDir()
	cacheDir := filepath.Join(dir, ".pr9k")
	if err := os.Mkdir(cacheDir, 0o755); err != nil {
		t.Fatal(err)
	}

	rec := makeRecord("feature-work", "done", 1)
	if err := AppendIterationRecord(dir, rec); err != nil {
		t.Fatalf("AppendIterationRecord: %v", err)
	}

	path := filepath.Join(cacheDir, "iteration.jsonl")
	lines := readLines(t, path)
	if len(lines) != 1 {
		t.Fatalf("want 1 line, got %d", len(lines))
	}
	var got IterationRecord
	if err := json.Unmarshal([]byte(lines[0]), &got); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if got.StepName != rec.StepName || got.Status != rec.Status {
		t.Errorf("got %+v, want %+v", got, rec)
	}
}

// TestAppendIterationRecord_TwoRecords verifies two records appear in order.
func TestAppendIterationRecord_TwoRecords(t *testing.T) {
	dir := t.TempDir()
	cacheDir := filepath.Join(dir, ".pr9k")
	if err := os.Mkdir(cacheDir, 0o755); err != nil {
		t.Fatal(err)
	}

	recs := []IterationRecord{
		makeRecord("step-one", "done", 1),
		makeRecord("step-two", "failed", 1),
	}
	for _, r := range recs {
		if err := AppendIterationRecord(dir, r); err != nil {
			t.Fatalf("AppendIterationRecord: %v", err)
		}
	}

	lines := readLines(t, filepath.Join(cacheDir, "iteration.jsonl"))
	if len(lines) != 2 {
		t.Fatalf("want 2 lines, got %d", len(lines))
	}
	for i, line := range lines {
		var got IterationRecord
		if err := json.Unmarshal([]byte(line), &got); err != nil {
			t.Fatalf("line %d unmarshal: %v", i, err)
		}
		if got.StepName != recs[i].StepName {
			t.Errorf("line %d: want step_name %q, got %q", i, recs[i].StepName, got.StepName)
		}
		if got.Status != recs[i].Status {
			t.Errorf("line %d: want status %q, got %q", i, recs[i].Status, got.Status)
		}
	}
}

// TestAppendIterationRecord_ConcurrentAppends verifies all lines are present
// and parseable after concurrent writes (tests the O_APPEND atomicity contract).
func TestAppendIterationRecord_ConcurrentAppends(t *testing.T) {
	dir := t.TempDir()
	if err := os.Mkdir(filepath.Join(dir, ".pr9k"), 0o755); err != nil {
		t.Fatal(err)
	}

	const n = 20
	var wg sync.WaitGroup
	wg.Add(n)
	errs := make([]error, n)
	for i := 0; i < n; i++ {
		i := i
		go func() {
			defer wg.Done()
			errs[i] = AppendIterationRecord(dir, makeRecord("step", "done", i))
		}()
	}
	wg.Wait()

	for i, err := range errs {
		if err != nil {
			t.Errorf("goroutine %d: %v", i, err)
		}
	}

	lines := readLines(t, filepath.Join(dir, ".pr9k", "iteration.jsonl"))
	if len(lines) != n {
		t.Fatalf("want %d lines, got %d", n, len(lines))
	}
	for i, line := range lines {
		var rec IterationRecord
		if err := json.Unmarshal([]byte(line), &rec); err != nil {
			t.Errorf("line %d not valid JSON: %v", i, err)
		}
	}
}

// TestAppendIterationRecord_PathUsesFilepathJoin verifies the file is written
// to <projectDir>/.pr9k/iteration.jsonl without double-slash artifacts.
func TestAppendIterationRecord_PathUsesFilepathJoin(t *testing.T) {
	dir := t.TempDir()
	if err := os.Mkdir(filepath.Join(dir, ".pr9k"), 0o755); err != nil {
		t.Fatal(err)
	}

	if err := AppendIterationRecord(dir, makeRecord("x", "done", 0)); err != nil {
		t.Fatal(err)
	}

	want := filepath.Join(dir, ".pr9k", "iteration.jsonl")
	if _, err := os.Stat(want); err != nil {
		t.Errorf("expected file at %s: %v", want, err)
	}
}

// TestAppendIterationRecord_MissingCacheDir verifies a clear error when
// .pr9k does not exist (precondition violated — preflight must run first).
func TestAppendIterationRecord_MissingCacheDir(t *testing.T) {
	dir := t.TempDir()
	// intentionally do NOT create .pr9k

	err := AppendIterationRecord(dir, makeRecord("step", "done", 1))
	if err == nil {
		t.Fatal("expected error when .pr9k/ is missing, got nil")
	}
}

// readLines reads all non-empty lines from path.
func readLines(t *testing.T, path string) []string {
	t.Helper()
	f, err := os.Open(path)
	if err != nil {
		t.Fatalf("open %s: %v", path, err)
	}
	defer func() { _ = f.Close() }()
	var lines []string
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		if line := sc.Text(); line != "" {
			lines = append(lines, line)
		}
	}
	if err := sc.Err(); err != nil {
		t.Fatalf("scan %s: %v", path, err)
	}
	return lines
}
