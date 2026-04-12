# File Logging

A concurrent-safe file logger that writes timestamped, context-prefixed lines to a log file for post-run analysis and debugging.

- **Last Updated:** 2026-04-08 12:00
- **Authors:**
  - River Bailey

## Overview

- Writes to `logs/ralph-YYYY-MM-DD-HHMMSS.log` under the project directory
- Each line is prefixed with a timestamp, optional iteration context, and step name
- Protected by `sync.Mutex` for concurrent writes from multiple scanner goroutines
- Uses `bufio.Writer` for buffered I/O with explicit flush on close
- Idempotent close — safe to call `Close()` multiple times

Key files:
- `ralph-tui/internal/logger/logger.go` — Logger struct, NewLogger, SetContext, Log, Close
- `ralph-tui/internal/logger/logger_test.go` — Unit tests for logging behavior

## Architecture

```
  Scanner goroutine 1 (stdout)  ──┐
                                   ├──▶ Logger.Log(stepName, line)
  Scanner goroutine 2 (stderr)  ──┘         │
                                            ▼
                                    ┌──────────────┐
                                    │  sync.Mutex  │
                                    └──────┬───────┘
                                           │
                                           ▼
                                    ┌──────────────┐
                                    │ bufio.Writer │
                                    └──────┬───────┘
                                           │
                                           ▼
                                    ┌──────────────┐
                                    │  os.File     │
                                    │  logs/ralph- │
                                    │  YYYY-MM-DD- │
                                    │  HHMMSS.log  │
                                    └──────────────┘
```

## Key Files

| File | Purpose |
|------|---------|
| `ralph-tui/internal/logger/logger.go` | Logger struct and all methods |
| `ralph-tui/internal/logger/logger_test.go` | Unit tests for logging |

## Core Types

```go
// Logger writes timestamped, prefixed log lines to a file.
// It is safe for concurrent use.
type Logger struct {
    mu        sync.Mutex
    file      *os.File
    writer    *bufio.Writer
    iteration string  // context prefix set by SetContext
    closed    bool    // prevents writes after close
}
```

## Implementation Details

### Logger Creation

`NewLogger` creates the `logs/` directory if needed and opens a timestamped log file:

```go
func NewLogger(projectDir string) (*Logger, error) {
    logsDir := filepath.Join(projectDir, "logs")
    if err := os.MkdirAll(logsDir, 0o700); err != nil {
        return nil, fmt.Errorf("logger: could not create logs directory: %w", err)
    }

    filename := time.Now().Format("ralph-2006-01-02-150405.log")
    logPath := filepath.Join(logsDir, filename)

    f, err := os.OpenFile(logPath, os.O_CREATE|os.O_WRONLY, 0o600)
    // ...
    return &Logger{file: f, writer: bufio.NewWriter(f)}, nil
}
```

### Log Line Format

Each line includes a timestamp, optional iteration context, and step name:

```
[2026-04-08 14:30:15] [Iteration 1/3] [Feature work] <line content>
[2026-04-08 14:30:16] [Feature work] <line without iteration context>
```

```go
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
```

### Iteration Context

`SetContext` updates the iteration prefix for subsequent log lines:

```go
func (l *Logger) SetContext(iteration string, _ string) {
    l.mu.Lock()
    defer l.mu.Unlock()
    l.iteration = iteration
}
```

The second parameter is reserved for future use (e.g., a step label) and is intentionally ignored.

### Close

`Close` flushes the buffered writer and closes the file. Idempotent — safe to call multiple times:

```go
func (l *Logger) Close() error {
    l.mu.Lock()
    defer l.mu.Unlock()

    if l.closed { return nil }
    l.closed = true

    if err := l.writer.Flush(); err != nil {
        _ = l.file.Close()
        return fmt.Errorf("logger: flush failed: %w", err)
    }
    return l.file.Close()
}
```

## Error Handling

| Scenario | Error Message | Behavior |
|----------|---------------|----------|
| Cannot create logs directory | `"logger: could not create logs directory: ..."` | Returned to caller (main.go exits) |
| Cannot create log file | `"logger: could not create log file: ..."` | Returned to caller |
| Write to closed logger | `"logger: write to closed logger"` | Error returned; no write |
| Flush fails on close | `"logger: flush failed: ..."` | File still closed; error returned |

## Testing

- `ralph-tui/internal/logger/logger_test.go` — Tests for NewLogger, Log with/without context, Close idempotency, write-after-close error

## Additional Information

- [Architecture Overview](../architecture.md) — Data flow showing logger alongside the `sendLine` streaming path
- [Subprocess Execution & Streaming](subprocess-execution.md) — How scanner goroutines write to the logger
- [CLI & Configuration](cli-configuration.md) — How ProjectDir determines the log file location
- [Workflow Orchestration](workflow-orchestration.md) — Where log context (iteration number) is set during the run loop
- [Concurrency](../coding-standards/concurrency.md) — Coding standards for mutex-protected shared writers
- [Error Handling](../coding-standards/error-handling.md) — Coding standards for bufio.Writer error surfacing and package-prefixed errors
- [Testing](../coding-standards/testing.md) — Coding standard for testing closeable types for idempotency (applies to Logger.Close)
