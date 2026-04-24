# ansi

The `internal/ansi` package provides a strict ANSI escape sequence stripper for untrusted byte slices. It is used by `workflowio.Load` to produce a clean recovery view when `config.json` cannot be parsed.

- **Last Updated:** 2026-04-24
- **Authors:**
  - River Bailey

## Overview

- `StripAll(b []byte) []byte` removes every ANSI escape sequence from the input and returns a new slice
- The input is never mutated
- Strips: CSI sequences (`ESC [ ... final`), OSC sequences (`ESC ] ... ST/BEL`), bare ESC bytes, and two-byte ESC-prefixed sequences
- Does not preserve SGR colors — all control sequences are removed unconditionally

Key file: `src/internal/ansi/strip.go`

## Core API

```go
// StripAll removes every ANSI escape sequence from b and returns a new slice.
// It strips CSI sequences (ESC [ ... final), OSC sequences (ESC ] ... ST/BEL),
// bare ESC bytes, and two-byte ESC-prefixed sequences. The input is never mutated.
func StripAll(b []byte) []byte
```

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
