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
// hasClaudeSteps gates the claude-only prerequisites: CheckProfileDir and
// CheckDocker are only run when the workflow contains at least one claude
// step, so a workflow with zero claude steps runs cleanly on a host that has
// neither a claude profile dir nor docker installed.
//
// Sequence:
//  1. os.MkdirAll(projectDir+"/.ralph-cache") — creates the cache dir on the host
//     so Docker bind-mount subpaths exist before the container starts. Must be
//     pre-created under the host UID before the container runs, to avoid a
//     chmod fight when the container writes its first cache file via the sandbox's
//     UID mapping.
//  2. os.MkdirAll(projectDir+"/.pr9k") — creates the umbrella dir for
//     iteration.jsonl and .pr9k/logs/ on first run. Pre-created under the host
//     UID for the same reason as .ralph-cache.
//  3. CheckProfileDir(profileDir) — only when hasClaudeSteps is true.
//  4. CheckDocker(p) — only when hasClaudeSteps is true.
func Run(projectDir, profileDir string, hasClaudeSteps bool, p Prober) Result {
	var result Result

	// Create .ralph-cache inside the project dir so the Docker bind-mount
	// subpath exists on the host before any Claude step runs. Without this,
	// the container cannot write cache files even when the parent mount is rw.
	cacheDir := filepath.Join(projectDir, ".ralph-cache")
	if err := os.MkdirAll(cacheDir, 0o755); err != nil {
		result.Errors = append(result.Errors, fmt.Errorf("preflight: could not create .ralph-cache in %s: %w", projectDir, err))
	}

	// Create .pr9k umbrella dir so iteration.jsonl and .pr9k/logs/ have a
	// writable parent on first run (same UID pre-creation rationale as above).
	pr9kDir := filepath.Join(projectDir, ".pr9k")
	if err := os.MkdirAll(pr9kDir, 0o755); err != nil {
		result.Errors = append(result.Errors, fmt.Errorf("preflight: could not create .pr9k in %s: %w", projectDir, err))
	}

	if !hasClaudeSteps {
		return result
	}

	if err := CheckProfileDir(profileDir); err != nil {
		result.Errors = append(result.Errors, err)
	}

	result.Errors = append(result.Errors, CheckDocker(p)...)

	return result
}
