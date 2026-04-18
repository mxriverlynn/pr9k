package sandbox

import (
	"errors"
	"os"
)

// Path reserves a unique, non-existent path for `docker run --cidfile`.
// It creates a temp file under the system temp dir with pattern
// "ralph-*.cid", then removes it so the filename is reserved but the
// path does not exist (docker requires --cidfile to point at a
// non-existent file).
//
// There is a small race between Remove and docker's own O_CREAT|O_EXCL
// where another process could claim the name. Parallelism is out of
// scope (design §2), so the accepted failure mode is a loud
// "container ID file found" error from docker on collision — not a
// silent corruption.
func Path() (string, error) {
	f, err := os.CreateTemp("", "ralph-*.cid")
	if err != nil {
		return "", err
	}
	path := f.Name()
	if err := f.Close(); err != nil {
		return "", err
	}
	if err := os.Remove(path); err != nil {
		return "", err
	}
	return path, nil
}

// Cleanup removes the cidfile. ENOENT is tolerated (file may not
// exist if docker run failed before writing it).
func Cleanup(path string) error {
	err := os.Remove(path)
	if err != nil && errors.Is(err, os.ErrNotExist) {
		return nil
	}
	return err
}
