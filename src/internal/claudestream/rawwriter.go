package claudestream

import (
	"bufio"
	"fmt"
	"os"
)

// RawWriter opens a per-step .jsonl file and appends verbatim bytes followed
// by a newline for each call to WriteLine. The file is opened with O_TRUNC so
// a retry invocation overwrites the prior attempt's bytes (D14).
//
// Wraps os.File with a bufio.Writer for throughput; does NOT fsync per line
// (D26 uses the ralph_end sentinel for crash-resilience instead).
//
// RawWriter is not safe for concurrent use. All writes come from a single
// goroutine (the stdout-forwarding goroutine in runCommand), and Close is
// called via defer on the same goroutine path.
type RawWriter struct {
	f      *os.File
	bw     *bufio.Writer
	path   string
	closed bool
}

// NewRawWriter opens path for writing, truncating any existing content.
// The caller must call Close when the step is done.
func NewRawWriter(path string) (*RawWriter, error) {
	f, err := os.OpenFile(path, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0o600)
	if err != nil {
		return nil, fmt.Errorf("claudestream: open raw writer %s: %w", path, err)
	}
	return &RawWriter{f: f, bw: bufio.NewWriter(f), path: path}, nil
}

// WriteLine appends the verbatim bytes followed by a newline.
func (w *RawWriter) WriteLine(b []byte) error {
	if _, err := w.bw.Write(b); err != nil {
		return fmt.Errorf("claudestream: raw writer write %s: %w", w.path, err)
	}
	if err := w.bw.WriteByte('\n'); err != nil {
		return fmt.Errorf("claudestream: raw writer newline %s: %w", w.path, err)
	}
	return nil
}

// Close flushes buffered data and closes the underlying file. Subsequent
// calls to Close return nil (idempotent — D26).
func (w *RawWriter) Close() error {
	if w.closed {
		return nil
	}
	w.closed = true
	if err := w.bw.Flush(); err != nil {
		_ = w.f.Close()
		return fmt.Errorf("claudestream: raw writer flush %s: %w", w.path, err)
	}
	if err := w.f.Close(); err != nil {
		return fmt.Errorf("claudestream: raw writer close %s: %w", w.path, err)
	}
	return nil
}
