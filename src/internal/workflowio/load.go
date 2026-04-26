package workflowio

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/mxriverlynn/pr9k/src/internal/ansi"
	"github.com/mxriverlynn/pr9k/src/internal/workflowmodel"
)

// ErrNotRegularFile is returned when config.json (or a companion) resolves to
// a non-regular file (FIFO, socket, block/char device, directory).
var ErrNotRegularFile = errors.New("workflowio: target is not a regular file")

// ErrPathEscape is returned when a companion path resolves outside the workflow directory.
var ErrPathEscape = errors.New("workflowio: path escapes workflow directory")

// pathContainedIn returns an ErrPathEscape-wrapped error if candidate does not
// resolve to a path strictly inside dir. EvalSymlinks is used on both sides
// (OI-1 pattern); when the candidate does not exist yet, walkback resolves the
// nearest existing ancestor and rejoins the missing suffix so directory-level
// symlinks (e.g. macOS /var → /private/var) are honored consistently on both
// sides. A non-existent file still cannot escape via an unresolvable symlink
// because the ancestor resolution applies the same symlink topology to both.
func pathContainedIn(dir, candidate string) error {
	absDir, _ := filepath.Abs(dir)
	absCand, _ := filepath.Abs(candidate)

	resolvedDir, err := resolveWithWalkback(absDir)
	if err != nil {
		resolvedDir = absDir
	}
	resolvedCand, err := resolveWithWalkback(absCand)
	if err != nil {
		resolvedCand = absCand
	}

	if !strings.HasPrefix(resolvedCand, resolvedDir+string(filepath.Separator)) {
		return fmt.Errorf("%w: %s", ErrPathEscape, candidate)
	}
	return nil
}

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
	// Companions live in the prompts/ subdirectory; keys are relative to workflowDir
	// (e.g., "prompts/feature-work.md") to match the validator's relKey convention.
	companions := make(map[string][]byte)

	// loadCompanion is a helper that reads a single relative companion into companions.
	// It skips missing files, rejects non-regular files, and guards against path traversal.
	loadCompanion := func(relKey string) error {
		companionPath := filepath.Join(workflowDir, relKey)
		if err := pathContainedIn(workflowDir, companionPath); err != nil {
			return err
		}
		_, err := os.Lstat(companionPath)
		if err != nil {
			if errors.Is(err, os.ErrNotExist) {
				return nil
			}
			return fmt.Errorf("workflowio: stat companion %s: %w", companionPath, err)
		}
		realCompanion, err := filepath.EvalSymlinks(companionPath)
		if err != nil {
			return fmt.Errorf("workflowio: eval symlinks companion %s: %w", companionPath, err)
		}
		rci, err := os.Stat(realCompanion)
		if err != nil {
			return fmt.Errorf("workflowio: stat real companion %s: %w", realCompanion, err)
		}
		if !rci.Mode().IsRegular() {
			return fmt.Errorf("%w: companion %s", ErrNotRegularFile, realCompanion)
		}
		data, err := os.ReadFile(realCompanion)
		if err != nil {
			return fmt.Errorf("workflowio: read companion %s: %w", realCompanion, err)
		}
		companions[relKey] = data
		return nil
	}

	// isWorkflowRelative reports whether a path token is a relative, non-template
	// path that could reside in the workflow bundle directory.
	isWorkflowRelative := func(token string) bool {
		return token != "" &&
			!filepath.IsAbs(token) &&
			!strings.Contains(token, "{{") &&
			!strings.Contains(token, "/..") &&
			token != "." && token != ".."
	}

	for _, step := range doc.Steps {
		// Prompt-file companions (claude steps).
		if step.PromptFile != "" {
			relKey := filepath.Join("prompts", step.PromptFile)
			if err := loadCompanion(relKey); err != nil {
				return LoadResult{}, err
			}
		}
		// Shell-step script companions: Command[0] when workflow-bundle-relative (GAP-037).
		if len(step.Command) > 0 && isWorkflowRelative(step.Command[0]) {
			if err := loadCompanion(step.Command[0]); err != nil {
				return LoadResult{}, err
			}
		}
	}

	// StatusLine script companion: Command when workflow-bundle-relative (GAP-037).
	if doc.StatusLine != nil && isWorkflowRelative(doc.StatusLine.Command) {
		if err := loadCompanion(doc.StatusLine.Command); err != nil {
			return LoadResult{}, err
		}
	}

	if len(companions) > 0 {
		result.Companions = companions
	}

	return result, nil
}
