package cli

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"github.com/spf13/cobra"

	"github.com/mxriverlynn/pr9k/ralph-tui/internal/version"
)

// Config holds parsed CLI arguments.
type Config struct {
	Iterations int
	ProjectDir string
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
		Version:       version.Version,
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
