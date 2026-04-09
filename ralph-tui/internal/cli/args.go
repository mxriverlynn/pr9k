package cli

import (
	"errors"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/spf13/cobra"
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

// newCommandImpl builds the cobra command, using ranE to track whether RunE executed.
func newCommandImpl(cfg *Config, ranE *bool) *cobra.Command {
	cmd := &cobra.Command{
		Use:           "ralph-tui [flags]",
		Short:         "Automated development workflow orchestrator",
		Long:          `ralph-tui drives the claude CLI through multi-step coding loops. By default, it picks up GitHub issues labeled "ralph", implements features, writes tests, runs code reviews, and pushes — all unattended. Custom workflow definitions can be provided to tailor the steps to your needs.`,
		Args:          cobra.NoArgs,
		SilenceErrors: true,
		SilenceUsage:  true,
		RunE: func(cmd *cobra.Command, args []string) error {
			*ranE = true
			if cfg.Iterations < 0 {
				return errors.New("cli: --iterations must be a non-negative integer")
			}
			if cfg.ProjectDir == "" {
				dir, err := resolveProjectDir()
				if err != nil {
					return fmt.Errorf("cli: could not resolve project dir: %w", err)
				}
				cfg.ProjectDir = dir
			}
			return nil
		},
	}
	cmd.Flags().IntVarP(&cfg.Iterations, "iterations", "n", 0, "number of iterations to run (0 = run until done)")
	cmd.Flags().StringVarP(&cfg.ProjectDir, "project-dir", "p", "", "path to the project directory (default: resolved from executable)")
	return cmd
}

// NewCommand returns a configured cobra.Command that parses CLI flags into cfg.
// RunE populates cfg when the command executes successfully.
func NewCommand(cfg *Config) *cobra.Command {
	return newCommandImpl(cfg, new(bool))
}

// Execute creates a Config, runs the cobra command, and returns the parsed config.
// Returns (nil, nil) if --help was requested (RunE was not invoked).
// Returns (nil, err) if parsing or validation failed.
// Returns (cfg, nil) on success.
func Execute() (*Config, error) {
	cfg := &Config{}
	var ranE bool
	cmd := newCommandImpl(cfg, &ranE)
	if err := cmd.Execute(); err != nil {
		return nil, err
	}
	if !ranE {
		return nil, nil
	}
	return cfg, nil
}
