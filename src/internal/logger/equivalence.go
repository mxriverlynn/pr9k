package logger

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

// wallClockRe matches an embedded wall-clock timestamp in a log line:
// "[YYYY-MM-DD HH:MM:SS]" as used by Logger.Log.
var wallClockRe = regexp.MustCompile(`\[\d{4}-\d{2}-\d{2} \d{2}:\d{2}:\d{2}\]`)

// widthLineRe matches lines that consist entirely of repeated single characters
// used for full-width underline banners (e.g., "----" or "===="). These are
// width-dependent renders excluded from comparison per D-13.
var widthLineRe = regexp.MustCompile(`^[-=]{4,}\s*$`)

// normalise strips run-specific fields from a single line so that two lines
// that differ only in wall-clock timestamps or width-dependent renders compare
// as equal.
func normalise(line string) string {
	// Strip wall-clock timestamps embedded in log lines.
	line = wallClockRe.ReplaceAllString(line, "[TIMESTAMP]")
	return line
}

// isWidthDependent reports whether a line should be excluded from comparison
// because its content is entirely determined by the terminal column count.
func isWidthDependent(line string) bool {
	return widthLineRe.MatchString(line)
}

// CompareRunDirs compares two per-run artifact directories for content
// equivalence modulo run-specifics (D-13). refDir and candDir are the paths
// to the two run-stamp directories (e.g. <projectDir>/.pr9k/logs/<run-stamp>/).
//
// Equivalence rules:
//   - For each JSONL file in refDir, a corresponding file with the same base
//     name must exist in candDir.
//   - Per-file comparison is line-by-line after stripping wall-clock timestamps.
//   - Lines that consist entirely of repeated separator characters (width-
//     dependent renders) are skipped.
//   - The RunStamp directory names themselves are not compared (only the
//     contents of the files inside).
//
// Returns (true, nil) on equivalence; (false, diffs) otherwise where each
// diff entry describes the first divergence in the form
// "diverged at file <name> line <N>: ref=<...> cand=<...>".
//
// CompareRunDirs is intended for test use only. Production code must not call it.
func CompareRunDirs(refDir, candDir string) (bool, []string) {
	refEntries, err := os.ReadDir(refDir)
	if err != nil {
		return false, []string{fmt.Sprintf("cannot read refDir %q: %v", refDir, err)}
	}

	var diffs []string

	for _, entry := range refEntries {
		if entry.IsDir() {
			continue
		}
		name := entry.Name()
		if !strings.HasSuffix(name, ".jsonl") && !strings.HasSuffix(name, ".log") {
			continue
		}

		refPath := filepath.Join(refDir, name)
		candPath := filepath.Join(candDir, name)

		fileDiffs := compareFiles(name, refPath, candPath)
		diffs = append(diffs, fileDiffs...)
	}

	return len(diffs) == 0, diffs
}

// compareFiles compares two files line-by-line, applying normalization.
func compareFiles(name, refPath, candPath string) []string {
	refLines, err := readLines(refPath)
	if err != nil {
		return []string{fmt.Sprintf("cannot read ref file %q: %v", name, err)}
	}

	candLines, err := readLines(candPath)
	if err != nil {
		return []string{fmt.Sprintf("missing file %q in cand: %v", name, err)}
	}

	// Filter out width-dependent lines from both sides before comparison.
	refLines = filterWidthDependent(refLines)
	candLines = filterWidthDependent(candLines)

	var diffs []string
	maxN := len(refLines)
	if len(candLines) > maxN {
		maxN = len(candLines)
	}

	for i := range maxN {
		var ref, cand string
		if i < len(refLines) {
			ref = normalise(refLines[i])
		}
		if i < len(candLines) {
			cand = normalise(candLines[i])
		}
		if ref != cand {
			diffs = append(diffs, fmt.Sprintf("diverged at file %s line %d: ref=%q cand=%q", name, i+1, ref, cand))
		}
	}
	return diffs
}

func filterWidthDependent(lines []string) []string {
	out := lines[:0:len(lines)]
	for _, l := range lines {
		if !isWidthDependent(l) {
			out = append(out, l)
		}
	}
	return out
}

func readLines(path string) ([]string, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer func() { _ = f.Close() }()

	var lines []string
	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 0, 64*1024), 256*1024)
	for sc.Scan() {
		lines = append(lines, sc.Text())
	}
	if err := sc.Err(); err != nil {
		return nil, fmt.Errorf("scan %q: %w", path, err)
	}
	return lines, nil
}
