package logger

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// writeRunDir creates a fake run directory structure under parent with the
// given run-stamp name and a set of named JSONL files (name → contents).
func writeRunDir(t *testing.T, parent, runStamp string, files map[string]string) string {
	t.Helper()
	runDir := filepath.Join(parent, runStamp)
	if err := os.MkdirAll(runDir, 0o700); err != nil {
		t.Fatalf("writeRunDir MkdirAll: %v", err)
	}
	for name, contents := range files {
		path := filepath.Join(runDir, name)
		if err := os.WriteFile(path, []byte(contents), 0o600); err != nil {
			t.Fatalf("writeRunDir WriteFile %q: %v", name, err)
		}
	}
	return runDir
}

// TP-D13-1: Two byte-identical run directories return (true, nil).
func TestCompareRunDirs_ByteIdentical(t *testing.T) {
	parent := t.TempDir()
	stamp := "ralph-2026-05-18-120000.000"
	files := map[string]string{
		"feature-work.jsonl": `{"type":"text","content":"hello"}` + "\n",
	}
	refDir := writeRunDir(t, parent, stamp, files)
	candDir := writeRunDir(t, t.TempDir(), stamp, files)

	equal, diffs := CompareRunDirs(refDir, candDir)
	if !equal {
		t.Errorf("expected equal=true for byte-identical dirs; diffs=%v", diffs)
	}
	if len(diffs) != 0 {
		t.Errorf("expected no diffs for byte-identical dirs; got %v", diffs)
	}
}

// TP-D13-2: Two runs differing only in RunStamp directory name and wall-clock
// timestamps return (true, nil).
func TestCompareRunDirs_DiffersOnlyInRunStampAndTimestamps(t *testing.T) {
	// A log line with a wall-clock timestamp embedded.
	const line = `{"type":"text","content":"[2026-05-18 12:00:01] some content"}` + "\n"
	const lineAlt = `{"type":"text","content":"[2026-05-19 15:30:45] some content"}` + "\n"

	refDir := writeRunDir(t, t.TempDir(), "ralph-2026-05-18-120000.000", map[string]string{
		"feature-work.jsonl": line,
	})
	candDir := writeRunDir(t, t.TempDir(), "ralph-2026-05-19-153045.000", map[string]string{
		"feature-work.jsonl": lineAlt,
	})

	equal, diffs := CompareRunDirs(refDir, candDir)
	if !equal {
		t.Errorf("expected equal=true when dirs differ only in timestamps; diffs=%v", diffs)
	}
	if len(diffs) != 0 {
		t.Errorf("expected no diffs; got %v", diffs)
	}
}

// TP-D13-3: Two runs differing in JSONL content (step args differ) return
// (false, [...]) with a precise divergence location.
func TestCompareRunDirs_DiffersInContent_ReturnsDivergenceLocation(t *testing.T) {
	refDir := writeRunDir(t, t.TempDir(), "ralph-2026-05-18-120000.000", map[string]string{
		"feature-work.jsonl": `{"type":"text","content":"step A"}` + "\n",
	})
	candDir := writeRunDir(t, t.TempDir(), "ralph-2026-05-18-130000.000", map[string]string{
		"feature-work.jsonl": `{"type":"text","content":"step B"}` + "\n",
	})

	equal, diffs := CompareRunDirs(refDir, candDir)
	if equal {
		t.Error("expected equal=false when JSONL content differs")
	}
	if len(diffs) == 0 {
		t.Fatal("expected at least one diff entry; got none")
	}
	// The diff must mention the file name and line number.
	joined := strings.Join(diffs, "; ")
	if !strings.Contains(joined, "feature-work.jsonl") {
		t.Errorf("diff does not mention filename: %v", diffs)
	}
	if !strings.Contains(joined, "line 1") {
		t.Errorf("diff does not mention line number: %v", diffs)
	}
}

// TP-D13-4: Width-dependent render content (underline banners) is excluded from
// the comparison so terminal-width differences do not create false divergences.
func TestCompareRunDirs_WidthDependentRendersExcluded(t *testing.T) {
	// A line that looks like a full-width underline banner (all hyphens or equals).
	const widthLine1 = "------------------------------------------------------------\n"
	const widthLine2 = "================================================================\n"

	refDir := writeRunDir(t, t.TempDir(), "ralph-2026-05-18-120000.000", map[string]string{
		"feature-work.jsonl": widthLine1,
	})
	candDir := writeRunDir(t, t.TempDir(), "ralph-2026-05-18-130000.000", map[string]string{
		"feature-work.jsonl": widthLine2,
	})

	equal, diffs := CompareRunDirs(refDir, candDir)
	if !equal {
		t.Errorf("expected equal=true for width-dependent-only differences; diffs=%v", diffs)
	}
	if len(diffs) != 0 {
		t.Errorf("expected no diffs; got %v", diffs)
	}
}

// TP-D13-5: A file present in ref but absent in cand is a divergence.
func TestCompareRunDirs_MissingFileInCand(t *testing.T) {
	refDir := writeRunDir(t, t.TempDir(), "ralph-2026-05-18-120000.000", map[string]string{
		"feature-work.jsonl": `{"type":"text","content":"x"}` + "\n",
		"code-review.jsonl":  `{"type":"text","content":"y"}` + "\n",
	})
	candDir := writeRunDir(t, t.TempDir(), "ralph-2026-05-18-130000.000", map[string]string{
		"feature-work.jsonl": `{"type":"text","content":"x"}` + "\n",
		// code-review.jsonl is absent
	})

	equal, diffs := CompareRunDirs(refDir, candDir)
	if equal {
		t.Error("expected equal=false when a file is missing in cand")
	}
	if len(diffs) == 0 {
		t.Fatal("expected at least one diff entry; got none")
	}
	joined := strings.Join(diffs, "; ")
	if !strings.Contains(joined, "code-review.jsonl") {
		t.Errorf("diff does not mention missing file: %v", diffs)
	}
}

// TP-D13-6: Empty run directories (no JSONL files) are equal.
func TestCompareRunDirs_BothEmpty(t *testing.T) {
	refDir := writeRunDir(t, t.TempDir(), "ralph-2026-05-18-120000.000", nil)
	candDir := writeRunDir(t, t.TempDir(), "ralph-2026-05-18-130000.000", nil)

	equal, diffs := CompareRunDirs(refDir, candDir)
	if !equal {
		t.Errorf("expected equal=true for both-empty dirs; diffs=%v", diffs)
	}
}
