// Package atomicwrite provides durable, atomic file replacement.
// Write is the only exported symbol; the writeFS interface enables fault injection in tests.
package atomicwrite

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"time"
)

// fileHandle is the minimal injectable file interface: write, sync, close.
type fileHandle interface {
	Write(p []byte) (int, error)
	Sync() error
	Close() error
}

// writeFS is the injectable filesystem interface used by write.
// The real implementation delegates to the os package; tests inject fakes.
type writeFS interface {
	evalSymlinks(path string) (string, error)
	openFile(name string, flag int, perm os.FileMode) (fileHandle, error)
	rename(oldpath, newpath string) error
	remove(name string) error
	openDir(name string) (fileHandle, error)
}

// Write atomically replaces path with data using mode for a newly created file.
// If path is a symlink, the symlink entry is preserved: the file it points to
// is replaced. Cross-device rename errors propagate unwrapped so callers can
// detect them with errors.Is(err, syscall.EXDEV).
func Write(path string, data []byte, mode os.FileMode) error {
	return write(osFS{}, path, data, mode)
}

func write(fs writeFS, path string, data []byte, mode os.FileMode) error {
	realPath, realDir, err := resolveRealPath(fs, path)
	if err != nil {
		return err
	}

	// Explicit temp name: <basename>.<pid>-<nanoseconds>.tmp
	tempName := fmt.Sprintf("%s.%d-%d.tmp", filepath.Base(realPath), os.Getpid(), time.Now().UnixNano())
	tempPath := filepath.Join(realDir, tempName)

	f, err := fs.openFile(tempPath, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o600)
	if err != nil {
		return fmt.Errorf("atomicwrite: create temp %s: %w", tempPath, err)
	}

	if _, err = f.Write(data); err != nil {
		_ = f.Close()
		_ = fs.remove(tempPath)
		return fmt.Errorf("atomicwrite: write %s: %w", tempPath, err)
	}
	if err = f.Sync(); err != nil {
		_ = f.Close()
		_ = fs.remove(tempPath)
		return fmt.Errorf("atomicwrite: sync %s: %w", tempPath, err)
	}
	if err = f.Close(); err != nil {
		_ = fs.remove(tempPath)
		return fmt.Errorf("atomicwrite: close %s: %w", tempPath, err)
	}

	// Rename propagates unwrapped so callers can inspect EXDEV.
	if err = fs.rename(tempPath, realPath); err != nil {
		_ = fs.remove(tempPath)
		return err
	}

	// Parent-directory fsync: best-effort; required by POSIX for the rename
	// directory entry to survive power loss.
	if dir, derr := fs.openDir(realDir); derr == nil {
		_ = dir.Sync()
		_ = dir.Close()
	}

	return nil
}

// resolveRealPath resolves path through symlinks to get the canonical target.
// On ENOENT (first save), it walks back to the lowest existing ancestor,
// resolves it, and appends the unresolved suffix.
func resolveRealPath(fs writeFS, path string) (realPath, realDir string, err error) {
	resolved, err := fs.evalSymlinks(path)
	if err == nil {
		return resolved, filepath.Dir(resolved), nil
	}
	if !errors.Is(err, os.ErrNotExist) {
		return "", "", fmt.Errorf("atomicwrite: eval symlinks %s: %w", path, err)
	}

	// File does not exist yet; walk toward the root until we find an
	// existing ancestor directory that can host the temp file.
	remaining := filepath.Base(path)
	dir := filepath.Dir(path)
	for {
		resolved, err = fs.evalSymlinks(dir)
		if err == nil {
			realPath = filepath.Join(resolved, remaining)
			return realPath, resolved, nil
		}
		if !errors.Is(err, os.ErrNotExist) {
			return "", "", fmt.Errorf("atomicwrite: eval symlinks %s: %w", dir, err)
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return "", "", fmt.Errorf("atomicwrite: no existing ancestor for %s", path)
		}
		remaining = filepath.Join(filepath.Base(dir), remaining)
		dir = parent
	}
}

// osFS is the real writeFS implementation that delegates to the os package.
type osFS struct{}

func (osFS) evalSymlinks(path string) (string, error) {
	return filepath.EvalSymlinks(path)
}

func (osFS) openFile(name string, flag int, perm os.FileMode) (fileHandle, error) {
	return os.OpenFile(name, flag, perm)
}

func (osFS) rename(oldpath, newpath string) error {
	return os.Rename(oldpath, newpath)
}

func (osFS) remove(name string) error {
	return os.Remove(name)
}

func (osFS) openDir(name string) (fileHandle, error) {
	return os.Open(name)
}
