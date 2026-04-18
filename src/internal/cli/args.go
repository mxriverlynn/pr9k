package cli

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"github.com/spf13/cobra"

	"github.com/mxriverlynn/pr9k/src/internal/version"
)

const flagSplitGuidance = "--project-dir changed meaning in 0.3.0 (now: target repo). Use --workflow-dir for the install dir and --project-dir for the target repo. See docs/adr/20260413162428-workflow-project-dir-split.md."

// Config holds parsed CLI arguments.
type Config struct {
	Iterations  int
	WorkflowDir string
	ProjectDir  string
}

// resolveWorkflowDir returns the directory containing the executable,
// following symlinks.
func resolveWorkflowDir() (string, error) {
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

// resolveProjectDir returns the current working directory, following symlinks.
// It returns an error if the resolved path is not a directory.
func resolveProjectDir() (string, error) {
	cwd, err := os.Getwd()
	if err != nil {
		return "", err
	}
	resolved, err := filepath.EvalSymlinks(cwd)
	if err != nil {
		return "", err
	}
	info, err := os.Stat(resolved)
	if err != nil {
		return "", fmt.Errorf("project dir %q: %w", resolved, err)
	}
	if !info.IsDir() {
		return "", fmt.Errorf("project dir %q is not a directory", resolved)
	}
	return resolved, nil
}

// newCommandImpl builds the cobra command, using ranE to track whether RunE executed.
func newCommandImpl(cfg *Config, ranE *bool) *cobra.Command {
	cmd := &cobra.Command{
		Use:           "pr9k [flags]",
		Short:         "Automated development workflow orchestrator",
		Long:          `pr9k drives the claude CLI through multi-step coding loops. By default, it picks up GitHub issues labeled "ralph", implements features, writes tests, runs code reviews, and pushes — all unattended. Custom workflow definitions can be provided to tailor the steps to your needs.`,
		Version:       version.Version,
		Args:          cobra.NoArgs,
		SilenceErrors: true,
		SilenceUsage:  true,
		RunE: func(cmd *cobra.Command, args []string) error {
			*ranE = true
			if cfg.Iterations < 0 {
				return errors.New("cli: --iterations must be a non-negative integer")
			}
			if cfg.WorkflowDir == "" {
				dir, err := resolveWorkflowDir()
				if err != nil {
					return fmt.Errorf("cli: could not resolve workflow dir: %w", err)
				}
				cfg.WorkflowDir = dir
			} else {
				resolved, err := filepath.EvalSymlinks(cfg.WorkflowDir)
				if err != nil {
					return fmt.Errorf("cli: --workflow-dir %q: %w", cfg.WorkflowDir, err)
				}
				info, err := os.Stat(resolved)
				if err != nil || !info.IsDir() {
					return fmt.Errorf("cli: --workflow-dir %q is not a directory", cfg.WorkflowDir)
				}
				cfg.WorkflowDir = resolved
			}
			if cfg.ProjectDir == "" {
				dir, err := resolveProjectDir()
				if err != nil {
					return fmt.Errorf("cli: could not resolve project dir: %w", err)
				}
				cfg.ProjectDir = dir
			} else {
				resolved, err := filepath.EvalSymlinks(cfg.ProjectDir)
				if err != nil {
					return fmt.Errorf("cli: --project-dir %q: %w", cfg.ProjectDir, err)
				}
				info, err := os.Stat(resolved)
				if err != nil || !info.IsDir() {
					return fmt.Errorf("cli: --project-dir %q is not a directory", cfg.ProjectDir)
				}
				cfg.ProjectDir = resolved
			}
			return nil
		},
	}
	cmd.SetFlagErrorFunc(func(_ *cobra.Command, err error) error {
		return fmt.Errorf("%w\n%s", err, flagSplitGuidance)
	})
	cmd.Flags().IntVarP(&cfg.Iterations, "iterations", "n", 0, "number of iterations to run (0 = run until done)")
	cmd.Flags().StringVar(&cfg.WorkflowDir, "workflow-dir", "", "path to the workflow bundle directory (default: resolved from executable)")
	cmd.Flags().StringVar(&cfg.ProjectDir, "project-dir", "", "path to the target repository (default: current working directory)")
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
// extra subcommands are added to the root command before execution.
func Execute(extra ...*cobra.Command) (*Config, error) {
	cfg := &Config{}
	var ranE bool
	cmd := newCommandImpl(cfg, &ranE)
	for _, sub := range extra {
		cmd.AddCommand(sub)
	}
	if err := cmd.Execute(); err != nil {
		return nil, err
	}
	if !ranE {
		return nil, nil
	}
	return cfg, nil
}
