package atomicwrite

import (
	"errors"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"syscall"
	"testing"
)

// T1: First save on non-existent target uses ENOENT walkback.
func TestWrite_FirstSave_NonExistentTarget_Succeeds(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	path := filepath.Join(dir, "new-file.txt")
	data := []byte("hello atomicwrite")

	if err := Write(path, data, 0o644); err != nil {
		t.Fatalf("Write: %v", err)
	}

	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}
	if string(got) != string(data) {
		t.Errorf("got %q, want %q", got, data)
	}
}

// T2: Basic replace of an existing file.
func TestWrite_ReplacesExistingFile(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	path := filepath.Join(dir, "existing.txt")

	if err := os.WriteFile(path, []byte("old content"), 0o644); err != nil {
		t.Fatalf("setup: %v", err)
	}

	newData := []byte("new content")
	if err := Write(path, newData, 0o644); err != nil {
		t.Fatalf("Write: %v", err)
	}

	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}
	if string(got) != string(newData) {
		t.Errorf("got %q, want %q", got, newData)
	}
}

// T3: Write via a symlink preserves the symlink entry.
func TestWrite_SymlinkedTarget_SymlinkPreserved(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	actual := filepath.Join(dir, "actual.txt")
	link := filepath.Join(dir, "link.txt")

	if err := os.WriteFile(actual, []byte("initial"), 0o644); err != nil {
		t.Fatalf("setup: %v", err)
	}
	if err := os.Symlink(actual, link); err != nil {
		t.Fatalf("symlink: %v", err)
	}

	newData := []byte("via symlink")
	if err := Write(link, newData, 0o644); err != nil {
		t.Fatalf("Write: %v", err)
	}

	// Symlink entry must still be a symlink (not replaced by a regular file).
	fi, err := os.Lstat(link)
	if err != nil {
		t.Fatalf("Lstat: %v", err)
	}
	if fi.Mode()&os.ModeSymlink == 0 {
		t.Errorf("link.txt is no longer a symlink after Write")
	}

	got, err := os.ReadFile(link)
	if err != nil {
		t.Fatalf("ReadFile via link: %v", err)
	}
	if string(got) != string(newData) {
		t.Errorf("got %q, want %q", got, newData)
	}
}

// T4: Symlink target on a different filesystem — EXDEV surfaced and temp cleaned up.
func TestWrite_SymlinkTargetOnDifferentFS_ReturnsEXDEV(t *testing.T) {
	t.Parallel()
	exdevErr := &os.PathError{Op: "rename", Path: "/tmp/x", Err: syscall.EXDEV}

	var removedPaths []string
	fs := &fakeWriteFS{
		evalSymlinksFunc: func(path string) (string, error) {
			// Simulates a symlink that resolves to a file on another FS.
			if path == "/fake/target.txt" {
				return "/other-fs/target.txt", nil
			}
			return path, nil
		},
		openFileFunc: func(name string, flag int, perm os.FileMode) (fileHandle, error) {
			return &noopFile{}, nil
		},
		renameFunc: func(oldpath, newpath string) error {
			return exdevErr
		},
		removeFunc: func(name string) error {
			removedPaths = append(removedPaths, name)
			return nil
		},
		openDirFunc: func(name string) (fileHandle, error) {
			return &noopFile{}, nil
		},
	}

	err := write(fs, "/fake/target.txt", []byte("data"), 0o644)
	if err == nil {
		t.Fatal("want error, got nil")
	}
	if len(removedPaths) == 0 {
		t.Error("temp file was not cleaned up after EXDEV rename failure")
	}
}

// T5: Rename failure triggers temp file removal.
func TestWrite_RenameFailure_RollsBackTempFile(t *testing.T) {
	t.Parallel()
	renameErr := errors.New("rename failed: disk full")

	var removedPaths []string
	var openedTempPath string

	fs := &fakeWriteFS{
		evalSymlinksFunc: func(path string) (string, error) {
			return path, nil
		},
		openFileFunc: func(name string, flag int, perm os.FileMode) (fileHandle, error) {
			openedTempPath = name
			return &noopFile{}, nil
		},
		renameFunc: func(oldpath, newpath string) error {
			return renameErr
		},
		removeFunc: func(name string) error {
			removedPaths = append(removedPaths, name)
			return nil
		},
		openDirFunc: func(name string) (fileHandle, error) {
			return &noopFile{}, nil
		},
	}

	err := write(fs, "/fake/target.txt", []byte("data"), 0o644)
	if err == nil {
		t.Fatal("want error, got nil")
	}
	if len(removedPaths) == 0 {
		t.Error("temp file was not removed after rename failure")
	}
	if len(removedPaths) > 0 && removedPaths[0] != openedTempPath {
		t.Errorf("removed %q, want temp file %q", removedPaths[0], openedTempPath)
	}
}

// T6: Temp file creation failure returns error without calling rename.
func TestWrite_TempFileCreationFailure_ReturnsError(t *testing.T) {
	t.Parallel()
	openErr := errors.New("open: permission denied")
	var renameCalled bool

	fs := &fakeWriteFS{
		evalSymlinksFunc: func(path string) (string, error) {
			return path, nil
		},
		openFileFunc: func(name string, flag int, perm os.FileMode) (fileHandle, error) {
			return nil, openErr
		},
		renameFunc: func(oldpath, newpath string) error {
			renameCalled = true
			return nil
		},
		removeFunc: func(name string) error {
			return nil
		},
		openDirFunc: func(name string) (fileHandle, error) {
			return &noopFile{}, nil
		},
	}

	err := write(fs, "/fake/target.txt", []byte("data"), 0o644)
	if err == nil {
		t.Fatal("want error, got nil")
	}
	if !errors.Is(err, openErr) {
		t.Errorf("want errors.Is(err, openErr), got %v", err)
	}
	if renameCalled {
		t.Error("rename must not be called after openFile failure")
	}
}

// T7: File fsync is called before rename.
func TestWrite_FsyncCalledBeforeRename(t *testing.T) {
	t.Parallel()
	var callOrder []string

	file := &trackingFile{callOrder: &callOrder}

	fs := &fakeWriteFS{
		evalSymlinksFunc: func(path string) (string, error) {
			return path, nil
		},
		openFileFunc: func(name string, flag int, perm os.FileMode) (fileHandle, error) {
			return file, nil
		},
		renameFunc: func(oldpath, newpath string) error {
			callOrder = append(callOrder, "rename")
			return nil
		},
		removeFunc: func(name string) error {
			return nil
		},
		openDirFunc: func(name string) (fileHandle, error) {
			return &noopFile{}, nil
		},
	}

	if err := write(fs, "/fake/target.txt", []byte("data"), 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}

	syncPos, renamePos := -1, -1
	for i, op := range callOrder {
		if op == "sync" {
			syncPos = i
		}
		if op == "rename" {
			renamePos = i
		}
	}
	if syncPos == -1 {
		t.Error("Sync was never called")
	}
	if renamePos == -1 {
		t.Error("rename was never called")
	}
	if syncPos != -1 && renamePos != -1 && syncPos > renamePos {
		t.Errorf("Sync (pos %d) called after rename (pos %d)", syncPos, renamePos)
	}
}

// T8: Parent directory fsync is called after rename.
func TestWrite_ParentDirSyncCalled(t *testing.T) {
	t.Parallel()
	var dirSynced bool
	dirFile := &callbackFile{onSync: func() { dirSynced = true }}

	fs := &fakeWriteFS{
		evalSymlinksFunc: func(path string) (string, error) {
			return path, nil
		},
		openFileFunc: func(name string, flag int, perm os.FileMode) (fileHandle, error) {
			return &noopFile{}, nil
		},
		renameFunc: func(oldpath, newpath string) error {
			return nil
		},
		removeFunc: func(name string) error {
			return nil
		},
		openDirFunc: func(name string) (fileHandle, error) {
			return dirFile, nil
		},
	}

	if err := write(fs, "/fake/target.txt", []byte("data"), 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}
	if !dirSynced {
		t.Error("parent directory Sync was never called")
	}
}

// T9: openFile is called with O_CREATE|O_EXCL|O_WRONLY and mode 0o600.
func TestWrite_TempFileUsesExplicitO_EXCLAndMode0o600(t *testing.T) {
	t.Parallel()
	var capturedFlag int
	var capturedPerm os.FileMode

	fs := &fakeWriteFS{
		evalSymlinksFunc: func(path string) (string, error) {
			return path, nil
		},
		openFileFunc: func(name string, flag int, perm os.FileMode) (fileHandle, error) {
			capturedFlag = flag
			capturedPerm = perm
			return &noopFile{}, nil
		},
		renameFunc: func(oldpath, newpath string) error {
			return nil
		},
		removeFunc: func(name string) error {
			return nil
		},
		openDirFunc: func(name string) (fileHandle, error) {
			return &noopFile{}, nil
		},
	}

	if err := write(fs, "/fake/target.txt", []byte("data"), 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}
	if capturedFlag&os.O_EXCL == 0 {
		t.Errorf("flag missing O_EXCL: got %v", capturedFlag)
	}
	if capturedFlag&os.O_CREATE == 0 {
		t.Errorf("flag missing O_CREATE: got %v", capturedFlag)
	}
	if capturedFlag&os.O_WRONLY == 0 {
		t.Errorf("flag missing O_WRONLY: got %v", capturedFlag)
	}
	if capturedPerm != 0o600 {
		t.Errorf("want mode 0o600, got %04o", capturedPerm)
	}
}

// T10: Temp file name matches <basename>.<pid>-<ns>.tmp.
func TestWrite_TempFileNameMatchesGlobPattern(t *testing.T) {
	t.Parallel()
	var capturedName string

	fs := &fakeWriteFS{
		evalSymlinksFunc: func(path string) (string, error) {
			return path, nil
		},
		openFileFunc: func(name string, flag int, perm os.FileMode) (fileHandle, error) {
			capturedName = name
			return &noopFile{}, nil
		},
		renameFunc: func(oldpath, newpath string) error {
			return nil
		},
		removeFunc: func(name string) error {
			return nil
		},
		openDirFunc: func(name string) (fileHandle, error) {
			return &noopFile{}, nil
		},
	}

	if err := write(fs, "/fake/dir/target.txt", []byte("data"), 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}

	base := filepath.Base(capturedName)
	// Pattern: target.txt.<digits>-<digits>.tmp
	pattern := regexp.MustCompile(`^target\.txt\.\d+-\d+\.tmp$`)
	if !pattern.MatchString(base) {
		t.Errorf("temp file name %q does not match pattern <basename>.<pid>-<ns>.tmp", base)
	}

	// Verify the PID portion.
	trimmed := strings.TrimSuffix(base, ".tmp")
	// Split on "." to find the pid-ns suffix; it's the last segment.
	parts := strings.Split(trimmed, ".")
	pidNs := parts[len(parts)-1]
	pidStr := strings.SplitN(pidNs, "-", 2)[0]
	gotPid, err := strconv.Atoi(pidStr)
	if err != nil {
		t.Errorf("could not parse PID from %q: %v", pidStr, err)
	}
	if gotPid != os.Getpid() {
		t.Errorf("temp file PID %d != os.Getpid() %d", gotPid, os.Getpid())
	}
}

// T11: Cross-device rename error propagates through errors.Is.
func TestWrite_CrossDeviceRenameSurfacedAsEXDEV(t *testing.T) {
	t.Parallel()
	exdevErr := &os.PathError{Op: "rename", Path: "/tmp/x", Err: syscall.EXDEV}

	fs := &fakeWriteFS{
		evalSymlinksFunc: func(path string) (string, error) {
			return path, nil
		},
		openFileFunc: func(name string, flag int, perm os.FileMode) (fileHandle, error) {
			return &noopFile{}, nil
		},
		renameFunc: func(oldpath, newpath string) error {
			return exdevErr
		},
		removeFunc: func(name string) error {
			return nil
		},
		openDirFunc: func(name string) (fileHandle, error) {
			return &noopFile{}, nil
		},
	}

	err := write(fs, "/fake/target.txt", []byte("data"), 0o644)
	if err == nil {
		t.Fatal("want error, got nil")
	}
	if !errors.Is(err, syscall.EXDEV) {
		t.Errorf("want errors.Is(err, syscall.EXDEV) = true, got err = %v", err)
	}
}

// --- Fake helpers ---

// fakeWriteFS is a configurable implementation of writeFS for tests.
type fakeWriteFS struct {
	evalSymlinksFunc func(path string) (string, error)
	openFileFunc     func(name string, flag int, perm os.FileMode) (fileHandle, error)
	renameFunc       func(oldpath, newpath string) error
	removeFunc       func(name string) error
	openDirFunc      func(name string) (fileHandle, error)
}

func (f *fakeWriteFS) evalSymlinks(path string) (string, error) {
	return f.evalSymlinksFunc(path)
}

func (f *fakeWriteFS) openFile(name string, flag int, perm os.FileMode) (fileHandle, error) {
	return f.openFileFunc(name, flag, perm)
}

func (f *fakeWriteFS) rename(oldpath, newpath string) error {
	return f.renameFunc(oldpath, newpath)
}

func (f *fakeWriteFS) remove(name string) error {
	return f.removeFunc(name)
}

func (f *fakeWriteFS) openDir(name string) (fileHandle, error) {
	return f.openDirFunc(name)
}

// noopFile implements fileHandle doing nothing (success on all ops).
type noopFile struct{}

func (n *noopFile) Write(p []byte) (int, error) { return len(p), nil }
func (n *noopFile) Sync() error                 { return nil }
func (n *noopFile) Close() error                { return nil }

// trackingFile implements fileHandle and records "sync" to a shared call log.
type trackingFile struct {
	callOrder *[]string
}

func (f *trackingFile) Write(p []byte) (int, error) { return len(p), nil }
func (f *trackingFile) Sync() error {
	*f.callOrder = append(*f.callOrder, "sync")
	return nil
}
func (f *trackingFile) Close() error { return nil }

// callbackFile implements fileHandle with a callback on Sync.
type callbackFile struct {
	onSync func()
}

func (f *callbackFile) Write(p []byte) (int, error) { return len(p), nil }
func (f *callbackFile) Sync() error {
	if f.onSync != nil {
		f.onSync()
	}
	return nil
}
func (f *callbackFile) Close() error { return nil }
