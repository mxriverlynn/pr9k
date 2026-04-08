# Test Plan — Issue #3: Prompt building with variable prepending

## Scope

Function under test: `BuildPrompt(projectDir string, step Step, issueID string, startingSHA string) (string, error)` in `ralph-tui/internal/steps/steps.go:37-49`.

## Existing Test Coverage

The current test suite has **5 tests** covering `BuildPrompt`:

| Test | What it covers |
|------|----------------|
| `TestBuildPrompt_PrependVarsTrue` | Happy path: prepend vars + file content concatenation |
| `TestBuildPrompt_PrependVarsFalse` | Happy path: raw file content returned unchanged |
| `TestBuildPrompt_FileNotFound` | Error returned when prompt file does not exist |
| `TestBuildPrompt_RealNewlines` | Real `\n` characters present, no literal `\n` sequences |
| `TestBuildPrompt_CorrectInterpolation` | Issue ID and SHA correctly embedded in prepended lines |

All 5 acceptance criteria from the issue have at least basic coverage:
- [x] Reads prompt file from `projectDir/prompts/<promptFile>` (T1, T2)
- [x] PrependVars true prepends variable lines (T1)
- [x] PrependVars false returns raw content (T2)
- [x] Returns error on unreadable prompt file (T3)
- [x] Uses real newlines (T4)

## Identified Test Gaps

### T1 — Empty prompt file content (High)
**What's missing:** No test verifies behavior when the prompt file exists but is empty (0 bytes). With `PrependVars=true`, the result should be exactly `ISSUENUMBER=<id>\nSTARTINGSHA=<sha>\n` with no trailing content. With `PrependVars=false`, the result should be an empty string.
**Why it matters:** Empty prompt files could surface during development or misconfiguration. The function should handle this gracefully without panicking or returning unexpected output.
**Suggested test:**
- Create temp prompt file with empty content
- Call `BuildPrompt` with `PrependVars=true` → assert result equals `"ISSUENUMBER=42\nSTARTINGSHA=abc\n"`
- Call `BuildPrompt` with `PrependVars=false` → assert result equals `""`

### T2 — Special characters in issueID and startingSHA (Medium)
**What's missing:** All existing tests use simple alphanumeric values for `issueID` and `startingSHA`. No test verifies behavior with values containing special characters (e.g., whitespace, newlines, equals signs). Since `BuildPrompt` uses simple string concatenation, any value is inserted verbatim — but callers should know this.
**Why it matters:** If upstream passes malformed values (e.g., an issueID containing `\n`), the prepended output could have unexpected line structure. This test documents the current contract: `BuildPrompt` does no validation/escaping of its inputs.
**Suggested test:**
- Call `BuildPrompt` with `issueID="1\n2"` and verify the literal string `"1\n2"` appears (confirming no double-escaping or transformation)

### T3 — Prompt file with no trailing newline (Medium)
**What's missing:** All existing test fixtures end with `\n`. No test checks a prompt file that lacks a trailing newline. When `PrependVars=true`, the prepended variables end with `\n` so the content is still joined cleanly, but the overall result won't end with `\n`.
**Why it matters:** Prompt files authored by humans may or may not have trailing newlines. The function should handle both without corruption.
**Suggested test:**
- Create temp prompt file with content `"no trailing newline"` (no `\n` at end)
- Call `BuildPrompt` with `PrependVars=true` → assert result is `"ISSUENUMBER=42\nSTARTINGSHA=abc\nno trailing newline"`

### T4 — Large prompt file content (Low)
**What's missing:** No test verifies behavior with a realistically large prompt file (e.g., multi-KB). The current tests use single-line fixtures.
**Why it matters:** Real prompt files may be substantial. This is a confidence test — `os.ReadFile` handles large files fine, but the test documents it.
**Suggested test:**
- Create temp prompt file with ~5KB of content (repeated lines)
- Call `BuildPrompt` with `PrependVars=true` → assert result starts with variable lines and ends with full file content

### T5 — PromptFile with subdirectory path (Low)
**What's missing:** No test checks whether `step.PromptFile` can include a subdirectory path (e.g., `"sub/feature.txt"`). The current implementation uses `filepath.Join(projectDir, "prompts", step.PromptFile)` which supports this, but no test documents the behavior.
**Why it matters:** If prompt organization changes to use subdirectories, this test confirms the function already supports it.
**Suggested test:**
- Create temp project with `prompts/sub/feature.txt`
- Call `BuildPrompt` with `PromptFile: "sub/feature.txt"` → assert success

## Priority Summary

| Priority | Test | Rationale |
|----------|------|-----------|
| High | T1 — Empty prompt file | Edge case in core logic path |
| Medium | T2 — Special chars in variables | Documents input contract |
| Medium | T3 — No trailing newline | Common real-world file variation |
| Low | T4 — Large prompt file | Confidence test only |
| Low | T5 — Subdirectory prompt path | Documents implicit capability |

## Recommendation

Implement T1 and T3 (high and medium priority, directly relevant to `BuildPrompt` behavior). T2 is worth adding to document the contract. T4 and T5 can be deferred — they test behaviors guaranteed by the Go stdlib rather than by this function's logic.
