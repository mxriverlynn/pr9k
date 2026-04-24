package workflowio

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"github.com/mxriverlynn/pr9k/src/internal/ansi"
	"github.com/mxriverlynn/pr9k/src/internal/workflowmodel"
)

// ErrNotRegularFile is returned when config.json (or a companion) resolves to
// a non-regular file (FIFO, socket, block/char device, directory).
var ErrNotRegularFile = errors.New("workflowio: target is not a regular file")

// LoadResult holds the outcome of a Load call. RecoveryView is non-nil when
// parsing failed; it contains up to 8 KiB of ANSI-stripped raw bytes.
type LoadResult struct {
	Doc           workflowmodel.WorkflowDoc
	IsSymlink     bool
	SymlinkTarget string
	RecoveryView  []byte
	Companions    map[string][]byte
}

const recoveryViewMaxBytes = 8 * 1024

// Load reads workflowDir/config.json, detects symlinks (D-23 ordering),
// rejects non-regular targets (F-109), and on parse failure returns a
// recovery view of up to 8 KiB of ANSI-stripped raw bytes (F-94).
func Load(workflowDir string) (LoadResult, error) {
	configPath := filepath.Join(workflowDir, "config.json")

	// Symlink detection FIRST (D-23), before any read or parse.
	var result LoadResult
	lfi, err := os.Lstat(configPath)
	if err != nil {
		return LoadResult{}, fmt.Errorf("workflowio: stat %s: %w", configPath, err)
	}
	if lfi.Mode()&os.ModeSymlink != 0 {
		result.IsSymlink = true
		target, err := os.Readlink(configPath)
		if err != nil {
			return LoadResult{}, fmt.Errorf("workflowio: readlink %s: %w", configPath, err)
		}
		result.SymlinkTarget = target
	}

	// Resolve the real path to verify the target is a regular file.
	realPath, err := filepath.EvalSymlinks(configPath)
	if err != nil {
		return LoadResult{}, fmt.Errorf("workflowio: eval symlinks %s: %w", configPath, err)
	}
	rfi, err := os.Stat(realPath)
	if err != nil {
		return LoadResult{}, fmt.Errorf("workflowio: stat real path %s: %w", realPath, err)
	}
	if !rfi.Mode().IsRegular() {
		return LoadResult{}, fmt.Errorf("%w: %s", ErrNotRegularFile, realPath)
	}

	raw, err := os.ReadFile(realPath)
	if err != nil {
		return LoadResult{}, fmt.Errorf("workflowio: read %s: %w", realPath, err)
	}

	doc, parseErr := workflowmodel.ParseConfig(raw)
	if parseErr != nil {
		// Return recovery view: up to 8 KiB, ANSI-stripped.
		view := raw
		if len(view) > recoveryViewMaxBytes {
			view = view[:recoveryViewMaxBytes]
		}
		result.RecoveryView = ansi.StripAll(view)
		return result, nil
	}

	result.Doc = doc

	// Load companion files referenced by steps; skip missing, reject non-regular.
	companions := make(map[string][]byte)
	for _, step := range doc.Steps {
		if step.PromptFile == "" {
			continue
		}
		companionPath := filepath.Join(workflowDir, step.PromptFile)
		cfi, err := os.Lstat(companionPath)
		if err != nil {
			if errors.Is(err, os.ErrNotExist) {
				continue
			}
			return LoadResult{}, fmt.Errorf("workflowio: stat companion %s: %w", companionPath, err)
		}
		realCompanion, err := filepath.EvalSymlinks(companionPath)
		if err != nil {
			return LoadResult{}, fmt.Errorf("workflowio: eval symlinks companion %s: %w", companionPath, err)
		}
		_ = cfi
		rci, err := os.Stat(realCompanion)
		if err != nil {
			return LoadResult{}, fmt.Errorf("workflowio: stat real companion %s: %w", realCompanion, err)
		}
		if !rci.Mode().IsRegular() {
			return LoadResult{}, fmt.Errorf("%w: companion %s", ErrNotRegularFile, realCompanion)
		}
		data, err := os.ReadFile(realCompanion)
		if err != nil {
			return LoadResult{}, fmt.Errorf("workflowio: read companion %s: %w", realCompanion, err)
		}
		companions[step.PromptFile] = data
	}
	if len(companions) > 0 {
		result.Companions = companions
	}

	return result, nil
}
