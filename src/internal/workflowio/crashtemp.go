package workflowio

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"time"
)

// CrashTempClassification indicates whether the process that created a temp
// file is still alive or has died (leaving a crash-era residue).
type CrashTempClassification int

const (
	// CrashTempActive means the owning process is still running.
	CrashTempActive CrashTempClassification = iota
	// CrashTempCrash means the owning process has exited (crash or normal stop).
	CrashTempCrash
)

// CrashTempFile describes a temp file left by a previous atomicwrite operation.
type CrashTempFile struct {
	Path           string
	PID            int
	MTime          time.Time
	Classification CrashTempClassification
}

// DetectCrashTempFiles scans workflowDir for atomicwrite temp files belonging
// to config.json and any companion files. It globs "config.json.*.tmp" (one
// glob per target file — narrower than *.*.*.tmp, F-110), parses the PID token,
// and classifies each file via syscall.Kill(pid, 0) liveness. PID reuse is a
// known accepted limitation (F-117).
func DetectCrashTempFiles(workflowDir string) ([]CrashTempFile, error) {
	// Determine the set of target basenames to glob.
	targetBasenames := []string{"config.json"}
	if companions, err := companionBasenames(workflowDir); err == nil {
		targetBasenames = append(targetBasenames, companions...)
	}

	seen := map[string]bool{}
	var results []CrashTempFile

	for _, base := range targetBasenames {
		pattern := filepath.Join(workflowDir, base+".*.tmp")
		matches, err := filepath.Glob(pattern)
		if err != nil {
			return nil, fmt.Errorf("workflowio: DetectCrashTempFiles glob %s: %w", pattern, err)
		}
		for _, match := range matches {
			if seen[match] {
				continue
			}
			seen[match] = true

			pid, ok := parsePIDFromTempName(filepath.Base(match), base)
			if !ok {
				continue
			}

			fi, err := os.Stat(match)
			if err != nil {
				continue
			}

			class := classifyPID(pid)
			results = append(results, CrashTempFile{
				Path:           match,
				PID:            pid,
				MTime:          fi.ModTime(),
				Classification: class,
			})
		}
	}
	return results, nil
}

// companionBasenames returns the basenames of companion files listed in
// config.json. Errors are silently ignored so callers still get config.json
// crash-temp detection even when config.json is unreadable.
func companionBasenames(workflowDir string) ([]string, error) {
	result, err := Load(workflowDir)
	if err != nil || result.RecoveryView != nil {
		return nil, err
	}
	seen := map[string]bool{}
	var names []string
	for _, step := range result.Doc.Steps {
		if step.PromptFile == "" {
			continue
		}
		base := filepath.Base(step.PromptFile)
		if !seen[base] {
			seen[base] = true
			names = append(names, base)
		}
	}
	return names, nil
}

// parsePIDFromTempName extracts the PID from a temp file name of the form
// <targetBase>.<pid>-<nanoseconds>.tmp. Returns 0, false if the name doesn't
// match the expected pattern.
func parsePIDFromTempName(tempBase, targetBase string) (int, bool) {
	// Remove the leading targetBase + "."
	prefix := targetBase + "."
	if !strings.HasPrefix(tempBase, prefix) {
		return 0, false
	}
	rest := strings.TrimPrefix(tempBase, prefix)
	// rest should be "<pid>-<ns>.tmp"
	rest = strings.TrimSuffix(rest, ".tmp")
	if rest == tempBase {
		// .tmp suffix was not present
		return 0, false
	}
	// Split on "-" to isolate the PID portion (first segment).
	parts := strings.SplitN(rest, "-", 2)
	if len(parts) != 2 {
		return 0, false
	}
	pid, err := strconv.Atoi(parts[0])
	if err != nil || pid <= 0 {
		return 0, false
	}
	return pid, true
}

// classifyPID checks process liveness via syscall.Kill(pid, 0).
func classifyPID(pid int) CrashTempClassification {
	err := syscall.Kill(pid, 0)
	if err == nil {
		return CrashTempActive
	}
	return CrashTempCrash
}
