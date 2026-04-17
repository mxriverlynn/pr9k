package preflight

import (
	"fmt"
	"os"
	"path/filepath"
)

// Result holds the collected warnings and errors from a preflight run.
// All checks run before returning — no short-circuit on failure.
type Result struct {
	Warnings []string
	Errors   []error
}

// Run performs all preflight checks against projectDir and profileDir using
// p as the docker prober. All errors and warnings are collected before returning.
//
// Sequence:
//  1. os.MkdirAll(projectDir+"/.ralph-cache") — creates the cache dir on the host
//     so Docker bind-mount subpaths exist before the container starts.
//  2. CheckProfileDir(profileDir)
//  3. CheckDocker(p)
//  4. CheckCredentials(profileDir) — warnings only, not fatal; only run
//     when CheckProfileDir succeeds, so that a missing profile directory
//     produces a single clear error rather than both an error and a
//     redundant "credentials file missing" warning.
func Run(projectDir, profileDir string, p Prober) Result {
	var result Result

	// Create .ralph-cache inside the project dir so the Docker bind-mount
	// subpath exists on the host before any Claude step runs. Without this,
	// the container cannot write cache files even when the parent mount is rw.
	cacheDir := filepath.Join(projectDir, ".ralph-cache")
	if err := os.MkdirAll(cacheDir, 0o755); err != nil {
		result.Errors = append(result.Errors, fmt.Errorf("preflight: could not create .ralph-cache in %s: %w", projectDir, err))
	}

	profileErr := CheckProfileDir(profileDir)
	if profileErr != nil {
		result.Errors = append(result.Errors, profileErr)
	}

	result.Errors = append(result.Errors, CheckDocker(p)...)

	if profileErr == nil {
		if w, err := CheckCredentials(profileDir); err != nil {
			result.Errors = append(result.Errors, err)
		} else if w != "" {
			result.Warnings = append(result.Warnings, w)
		}
	}

	return result
}
