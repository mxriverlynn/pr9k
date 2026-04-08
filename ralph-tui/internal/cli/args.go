package cli

import (
	"errors"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
)

// Config holds parsed CLI arguments.
type Config struct {
	Iterations int
	ProjectDir string
}

// ParseArgs parses the command-line arguments (os.Args[1:]).
// Returns an error for missing/invalid iterations or invalid flags.
func ParseArgs(args []string) (*Config, error) {
	fs := flag.NewFlagSet("ralph-tui", flag.ContinueOnError)
	projectDir := fs.String("project-dir", "", "path to the project directory (default: resolved from executable)")

	if err := fs.Parse(args); err != nil {
		return nil, err
	}

	positional := fs.Args()
	if len(positional) == 0 {
		return nil, errors.New("missing required argument: iterations")
	}

	iterations, err := strconv.Atoi(positional[0])
	if err != nil {
		return nil, fmt.Errorf("iterations must be an integer, got %q", positional[0])
	}
	if iterations <= 0 {
		return nil, errors.New("iterations must be > 0")
	}

	dir := *projectDir
	if dir == "" {
		dir, err = resolveProjectDir()
		if err != nil {
			return nil, fmt.Errorf("could not resolve project dir: %w", err)
		}
	}

	return &Config{
		Iterations: iterations,
		ProjectDir: dir,
	}, nil
}

// resolveProjectDir returns the directory containing the executable,
// following symlinks.
func resolveProjectDir() (string, error) {
	exe, err := os.Executable()
	if err != nil {
		return "", err
	}
	resolved, err := filepath.EvalSymlinks(exe)
	if err != nil {
		return "", err
	}
	return filepath.Dir(resolved), nil
}
