//go:build tools

// This file pins Bubble Tea dependencies to specific versions before production
// code imports them. It intentionally has no main() function — use `go vet -tags tools .`
// to verify it compiles (as the Makefile does). Running `go build -tags tools .` will
// fail with a missing main error; that is expected and not the intended verification path.

package main

import (
	_ "github.com/charmbracelet/bubbles"
	_ "github.com/charmbracelet/bubbletea"
	_ "github.com/charmbracelet/lipgloss"
)
