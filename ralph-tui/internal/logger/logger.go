package logger

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"
)

// Logger writes timestamped, prefixed log lines to a file.
// It is safe for concurrent use.
type Logger struct {
	mu        sync.Mutex
	file      *os.File
	writer    *bufio.Writer
	iteration string
	closed    bool
}

// NewLogger creates a new Logger that writes to logs/ralph-YYYY-MM-DD-HHMMSS.log
// under projectDir. The logs/ directory is created if it does not exist.
func NewLogger(projectDir string) (*Logger, error) {
	logsDir := filepath.Join(projectDir, "logs")
	if err := os.MkdirAll(logsDir, 0o755); err != nil {
		return nil, fmt.Errorf("logger: could not create logs directory: %w", err)
	}

	filename := time.Now().Format("ralph-2006-01-02-150405.log")
	logPath := filepath.Join(logsDir, filename)

	f, err := os.Create(logPath)
	if err != nil {
		return nil, fmt.Errorf("logger: could not create log file: %w", err)
	}

	return &Logger{
		file:   f,
		writer: bufio.NewWriter(f),
	}, nil
}

// SetContext updates the iteration prefix used by subsequent Log calls.
// iteration is a label like "Iteration 1/3".
func (l *Logger) SetContext(iteration string, _ string) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.iteration = iteration
}

// Log writes a single line to the log file prefixed with timestamp, iteration
// context (if set), and step name.
func (l *Logger) Log(stepName string, line string) error {
	l.mu.Lock()
	defer l.mu.Unlock()

	if l.closed {
		return fmt.Errorf("logger: write to closed logger")
	}

	ts := time.Now().Format("2006-01-02 15:04:05")
	var prefix string
	if l.iteration != "" {
		prefix = fmt.Sprintf("[%s] [%s] [%s] ", ts, l.iteration, stepName)
	} else {
		prefix = fmt.Sprintf("[%s] [%s] ", ts, stepName)
	}

	_, err := fmt.Fprintln(l.writer, prefix+line)
	return err
}

// Close flushes buffered content and closes the log file.
func (l *Logger) Close() error {
	l.mu.Lock()
	defer l.mu.Unlock()

	if l.closed {
		return nil
	}
	l.closed = true

	if err := l.writer.Flush(); err != nil {
		_ = l.file.Close()
		return fmt.Errorf("logger: flush failed: %w", err)
	}
	return l.file.Close()
}
