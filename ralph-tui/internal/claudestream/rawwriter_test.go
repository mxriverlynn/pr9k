package claudestream_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/mxriverlynn/pr9k/ralph-tui/internal/claudestream"
)

func TestRawWriter_VerbatimWrite(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "out.jsonl")

	w, err := claudestream.NewRawWriter(path)
	if err != nil {
		t.Fatalf("NewRawWriter: %v", err)
	}

	lines := [][]byte{
		[]byte(`{"type":"system","subtype":"init"}`),
		[]byte(`{"type":"result","is_error":false}`),
		[]byte(`not valid json at all`),
	}
	for _, line := range lines {
		if err := w.WriteLine(line); err != nil {
			t.Fatalf("WriteLine: %v", err)
		}
	}
	if err := w.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}

	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}

	want := string(lines[0]) + "\n" + string(lines[1]) + "\n" + string(lines[2]) + "\n"
	if string(got) == want {
		return
	}
	t.Errorf("file contents mismatch\ngot:  %q\nwant: %q", got, want)
}

func TestRawWriter_OTruncOverwrites(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "out.jsonl")

	// First write.
	w1, err := claudestream.NewRawWriter(path)
	if err != nil {
		t.Fatalf("NewRawWriter (1): %v", err)
	}
	if err := w1.WriteLine([]byte(`{"attempt":1}`)); err != nil {
		t.Fatalf("WriteLine (1): %v", err)
	}
	if err := w1.Close(); err != nil {
		t.Fatalf("Close (1): %v", err)
	}

	// Second write to same path — should overwrite, not append.
	w2, err := claudestream.NewRawWriter(path)
	if err != nil {
		t.Fatalf("NewRawWriter (2): %v", err)
	}
	if err := w2.WriteLine([]byte(`{"attempt":2}`)); err != nil {
		t.Fatalf("WriteLine (2): %v", err)
	}
	if err := w2.Close(); err != nil {
		t.Fatalf("Close (2): %v", err)
	}

	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}

	want := `{"attempt":2}` + "\n"
	if string(got) != want {
		t.Errorf("file should only contain second write\ngot:  %q\nwant: %q", got, want)
	}
}

func TestRawWriter_CloseIdempotent(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "out.jsonl")

	w, err := claudestream.NewRawWriter(path)
	if err != nil {
		t.Fatalf("NewRawWriter: %v", err)
	}
	if err := w.Close(); err != nil {
		t.Fatalf("first Close: %v", err)
	}
	if err := w.Close(); err != nil {
		t.Fatalf("second Close should return nil: %v", err)
	}
}

func TestRawWriter_OpenCloseNoWrite(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "empty.jsonl")

	w, err := claudestream.NewRawWriter(path)
	if err != nil {
		t.Fatalf("NewRawWriter: %v", err)
	}
	if err := w.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}

	fi, err := os.Stat(path)
	if err != nil {
		t.Fatalf("file should exist: %v", err)
	}
	if fi.Size() != 0 {
		t.Errorf("expected zero-byte file, got %d bytes", fi.Size())
	}
}
