# ansi

The `internal/ansi` package provides a strict ANSI escape sequence stripper for untrusted byte slices. It is used by `workflowio.Load` to produce a clean recovery view when `config.json` cannot be parsed.

- **Last Updated:** 2026-04-24
- **Authors:**
  - River Bailey

## Overview

- `StripAll(b []byte) []byte` — removes every ANSI escape sequence from the input and returns a new slice. Does not strip C0/C1 control bytes beyond ESC sequences. Safe for ASCII and UTF-8 input.
- `StripForTerminalOutput(b []byte) []byte` — strict superset of `StripAll`; additionally strips dangerous C0 cursor-movement bytes and all 8-bit C1 controls (0x80–0x9F). **Lossy for non-ASCII input**: UTF-8 continuation bytes in 0x80–0x9F are dropped unconditionally. See Non-ASCII safety note below.
- Neither function mutates the input slice.

Key files: `src/internal/ansi/strip.go`, `src/internal/ansi/strip_strict.go`

## Core API

```go
// StripAll removes every ANSI escape sequence from b and returns a new slice.
// It strips CSI sequences (ESC [ ... final), OSC sequences (ESC ] ... ST/BEL),
// bare ESC bytes, and two-byte ESC-prefixed sequences. The input is never mutated.
func StripAll(b []byte) []byte

// StripForTerminalOutput strips ANSI sequences and dangerous C0/C1 control bytes
// from cmux-supplied diagnostic text. It is a strict superset of StripAll.
// See NON-ASCII safety note below.
func StripForTerminalOutput(b []byte) []byte
```

**`StripForTerminalOutput`** additionally strips:
- C0 cursor-movement bytes: NUL, BEL, BS, VT, FF, CR, DEL (SEC-001).
- All 8-bit C1 controls (0x80–0x9F) with payload consumption for C1 OSC (0x9D), C1 DCS (0x90), and C1 CSI (0x9B) (SEC-004).

Preserved: LF (0x0A) and HT (0x09).

**Non-ASCII (UTF-8) safety note:** `StripForTerminalOutput` drops every byte in 0x80–0x9F unconditionally. UTF-8 continuation bytes fall in this range, so any multi-byte sequence containing a continuation byte in 0x80–0x9F will be corrupted. For example, the em-dash (U+2014, `0xE2 0x80 0x94`) loses its `0x80` byte. This is a deliberate safety-over-fidelity choice: over-stripping untrusted terminal output is preferable to allowing 8-bit C1 escape sequences through. Callers must treat the return value as potentially lossy when the input may contain non-ASCII text.

Key file: `src/internal/ansi/strip_strict.go`

## Sequence Coverage

| Sequence type | Pattern | Handling |
|---------------|---------|----------|
| CSI | `ESC [ <param bytes> <final byte 0x40–0x7E>` | Stripped entirely |
| OSC | `ESC ] <any> BEL` or `ESC ] <any> ESC \` | Stripped entirely |
| Two-byte ESC sequence | `ESC <any single byte>` | Both bytes dropped |
| Bare ESC at end of input | `ESC` (no following byte) | Dropped |

The stripper is written as a single linear scan with no heap allocations beyond the output slice. For an input with no ESC bytes, the output slice is built by appending individual bytes; the function still allocates a new slice.

## Use Case: Recovery View

`workflowio.Load` reads up to 8 KiB of `config.json` and calls `StripAll` before returning it as `LoadResult.RecoveryView`. This produces a human-readable snippet free of terminal control sequences even if the file was accidentally written with embedded escape codes or corrupted.

## Testing

- `src/internal/ansi/strip_test.go`
- Tests cover: CSI stripping, OSC stripping (BEL and ST terminators), SGR codes, OSC 8 hyperlinks, bare ESC, two-byte sequences, empty input, no-ESC passthrough

## Related Documentation

- [`docs/code-packages/workflowio.md`](workflowio.md) — How `StripAll` is used in the load recovery path
