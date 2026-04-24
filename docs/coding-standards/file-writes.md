# File-Write Coding Standard

Rules for durable file writes in pr9k. Apply when writing any code that creates
or replaces files on disk.

---

## Rule 1 — Use `atomicwrite.Write` for durable file replacement

Any code that durably replaces an existing file on disk MUST use
`atomicwrite.Write` (`internal/atomicwrite`). This guarantees that readers
always see either the complete old content or the complete new content — never a
partial write — because the package uses a write-to-temp-then-rename pattern
with file and directory fsyncs.

```go
// Correct
if err := atomicwrite.Write(path, data, 0o644); err != nil { … }

// Prohibited
os.WriteFile(path, data, 0o644)          // no atomicity
os.OpenFile(path, os.O_TRUNC|…, 0o644)  // no atomicity
```

## Rule 2 — Use `O_CREATE|O_EXCL|O_WRONLY, 0o600` for initial temp files

Any code that creates a new file that later code will `atomicwrite.Write` to
(e.g. initializing a blank file before the first atomic save) MUST use:

```go
os.OpenFile(path, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o600)
```

`O_EXCL` prevents silent truncation of a file that already exists.
`0o600` limits initial access to the owning process.

## Rule 3 — `O_TRUNC` is prohibited in new code

`O_TRUNC` is prohibited in new code unless the file falls into one of the
exempt categories listed in [§ O_TRUNC exemptions](#otrunc-exemptions) below.
New code that needs to replace a file must use `atomicwrite.Write` (Rule 1).

## Rule 4 — No `os.WriteFile` on externally sourced paths

No new path-handling code may call `os.WriteFile` on a path received from user
input or a workflow file. Use `atomicwrite.Write` instead so that the
write-path invariants (atomicity, fsync, same-filesystem temp) are enforced at
the boundary.

---

## O_TRUNC exemptions

Sites listed here are exempt from Rule 3 because they fall into an established
exempt category. No new sites may be added without updating this list.

### `src/internal/claudestream/rawwriter.go` — streaming JSONL log

**Category:** short-lived streaming log

**Rationale:** `RawWriter` opens a per-step `.jsonl` file and streams verbatim
bytes into it for the duration of a single Claude step invocation. The file is
always written from scratch on each invocation; `O_TRUNC` is intentional so
that a retry overwrites the prior attempt's bytes (design decision D14). The
file is a debugging artifact, not a durable record — crash resilience is
provided by the `ralph_end` sentinel (D26), not by atomic rename.

```go
// src/internal/claudestream/rawwriter.go:29
f, err := os.OpenFile(path, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0o600)
```
