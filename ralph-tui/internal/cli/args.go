package cli

import (
	"errors"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
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

	// Reorder args so flags appear before positionals; Go's flag package stops
	// parsing at the first non-flag argument, so "3 -project-dir /tmp" would
	// leave -project-dir unprocessed without this reordering.
	if err := fs.Parse(reorderArgs(args)); err != nil {
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

// reorderArgs moves flag args before positional args so Go's flag package
// can parse them regardless of their original position in args.
func reorderArgs(args []string) []string {
	var flagArgs, positionalArgs []string
	i := 0
	for i < len(args) {
		arg := args[i]
		if arg == "--" {
			positionalArgs = append(positionalArgs, args[i+1:]...)
			break
		}
		if strings.HasPrefix(arg, "-") {
			flagArgs = append(flagArgs, arg)
			// If next arg looks like a flag value (doesn't start with -), include it
			if i+1 < len(args) && !strings.HasPrefix(args[i+1], "-") && args[i+1] != "--" {
				i++
				flagArgs = append(flagArgs, args[i])
			}
		} else {
			positionalArgs = append(positionalArgs, arg)
		}
		i++
	}
	return append(flagArgs, positionalArgs...)
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
