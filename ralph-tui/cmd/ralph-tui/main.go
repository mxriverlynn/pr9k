package main

import (
	"fmt"
	"os"

	"github.com/mxriverlynn/pr9k/ralph-tui/internal/cli"
)

func main() {
	cfg, err := cli.ParseArgs(os.Args[1:])
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}

	fmt.Printf("iterations: %d\n", cfg.Iterations)
	fmt.Printf("project-dir: %s\n", cfg.ProjectDir)
}
