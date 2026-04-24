// Package workflowio provides load, save, and detect operations on a workflow
// bundle on disk. It depends on workflowmodel, atomicwrite, and ansi; it never
// imports internal/statusline or syscall in the TUI layer (all classification
// happens here).
package workflowio

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"syscall"
	"time"

	"github.com/mxriverlynn/pr9k/src/internal/atomicwrite"
	"github.com/mxriverlynn/pr9k/src/internal/workflowmodel"
)

// SaveFS is the injectable filesystem interface used by Save.
// The production implementation delegates to atomicwrite and os.
// Tests inject fakes with per-method counters.
type SaveFS interface {
	WriteAtomic(path string, data []byte, mode os.FileMode) error
	Stat(path string) (os.FileInfo, error)
}

// SaveErrorKind classifies the outcome of a Save call so the TUI can render
// the right recovery prompt without importing syscall.
type SaveErrorKind int

const (
	SaveErrorNone                 SaveErrorKind = iota
	SaveErrorValidatorFatals                    // pre-save validation failed
	SaveErrorPermission                         // EACCES / EPERM
	SaveErrorDiskFull                           // ENOSPC
	SaveErrorEXDEV                              // cross-device rename
	SaveErrorConflictDetected                   // disk changed since load
	SaveErrorSymlinkEscape                      // companion outside workflowDir
	SaveErrorTargetNotRegularFile               // target is a FIFO/socket/device
	SaveErrorParse                              // JSON marshal failed
	SaveErrorOther                              // fallback
)

// SaveSnapshot records the file state observed after a successful config.json
// write, used by the TUI to update its dirty-tracking baseline.
type SaveSnapshot struct {
	ModTime time.Time
	Size    int64
}

// SaveResult is the outcome of a Save call. Snapshot is nil on any error.
type SaveResult struct {
	Kind     SaveErrorKind
	Err      error
	Snapshot *SaveSnapshot
}

// Save atomically writes memDoc and any dirty companion files to workflowDir.
// diskDoc is the in-memory snapshot from the last load; IsDirty is called to
// skip writes when nothing has changed. Companions must be written before
// config.json (D-20). All writes go through fs.WriteAtomic with mode 0o600.
func Save(fs SaveFS, workflowDir string, diskDoc, memDoc workflowmodel.WorkflowDoc, companions map[string][]byte) SaveResult {
	if !workflowmodel.IsDirty(diskDoc, memDoc) && len(companions) == 0 {
		return SaveResult{Kind: SaveErrorNone}
	}

	// Companion-first ordering (D-20).
	for relPath, data := range companions {
		fullPath := filepath.Join(workflowDir, relPath)
		if err := fs.WriteAtomic(fullPath, data, 0o600); err != nil {
			return classifySaveError(err)
		}
	}

	data, err := marshalDoc(memDoc)
	if err != nil {
		return SaveResult{Kind: SaveErrorParse, Err: fmt.Errorf("workflowio: marshal config.json: %w", err)}
	}

	configPath := filepath.Join(workflowDir, "config.json")
	if err := fs.WriteAtomic(configPath, data, 0o600); err != nil {
		return classifySaveError(err)
	}

	fi, err := fs.Stat(configPath)
	if err != nil {
		return SaveResult{Kind: SaveErrorOther, Err: fmt.Errorf("workflowio: stat after save: %w", err)}
	}
	return SaveResult{
		Kind:     SaveErrorNone,
		Snapshot: &SaveSnapshot{ModTime: fi.ModTime(), Size: fi.Size()},
	}
}

func classifySaveError(err error) SaveResult {
	if errors.Is(err, syscall.EXDEV) {
		return SaveResult{Kind: SaveErrorEXDEV, Err: err}
	}
	if errors.Is(err, syscall.ENOSPC) {
		return SaveResult{Kind: SaveErrorDiskFull, Err: err}
	}
	if errors.Is(err, os.ErrPermission) {
		return SaveResult{Kind: SaveErrorPermission, Err: err}
	}
	return SaveResult{Kind: SaveErrorOther, Err: err}
}

// RealSaveFS returns the production SaveFS implementation.
func RealSaveFS() SaveFS { return realSaveFS{} }

type realSaveFS struct{}

func (realSaveFS) WriteAtomic(path string, data []byte, mode os.FileMode) error {
	return atomicwrite.Write(path, data, mode)
}

func (realSaveFS) Stat(path string) (os.FileInfo, error) {
	return os.Stat(path)
}
