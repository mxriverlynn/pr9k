// Scratch app to verify Glyph API assumptions from docs/plans/ux-corrections/design.md (V1).
// This file is NOT production code — it exists solely to validate the API before rewriting
// the real UI in subsequent PR2 tickets (#47 onward).
//
// Run from ralph-tui/:
//
//	go run ./scratch/
package main

import (
	"fmt"
	"io"
	"os"
	"strings"
	"time"

	"github.com/kungfusheep/glyph"
)

func main() {
	// Simulate a streaming log source (stands in for an io.Pipe fed by subprocess output).
	pr, pw := io.Pipe()
	go func() {
		for i := 0; i < 20; i++ {
			fmt.Fprintf(pw, "step output line %d\n", i+1)
			time.Sleep(100 * time.Millisecond)
		}
		pw.Close()
	}()

	// Pointer-bound strings that the header will display.
	iterLine := "Iteration 1 / 3"
	step1 := "[x] get_next_issue"
	step2 := "[ ] feature"
	step3 := "[ ] test-plan"

	app := glyph.NewApp()

	// Keyboard: plan assumed app.BindKey("q", fn) — actual API is app.Handle(pattern, fn).
	app.Handle("q", func() {
		app.Stop()
		os.Exit(0)
	})

	// Layout verification:
	//   VBox.Border(BorderRounded).Title("Ralph") — chain on VBoxFn, then call with children.
	//   HBox(...) for the checkbox row.
	//   Log(reader).Grow(1).MaxLines(500).BindVimNav()
	view := glyph.VBox.Border(glyph.BorderRounded).Title("Ralph")(
		// Header: iteration line (pointer-bound string)
		glyph.Text(&iterLine),
		// Checkbox row using HBox
		glyph.HBox(
			glyph.Text(&step1),
			glyph.Text(" | "),
			glyph.Text(&step2),
			glyph.Text(" | "),
			glyph.Text(&step3),
		),
		// Scrollable log panel that fills remaining space
		glyph.Log(pr).Grow(1).MaxLines(500).BindVimNav(),
		// Shortcut hint
		glyph.Text(strings.Repeat("─", 40)),
		glyph.Text("q quit"),
	)

	app.SetView(view)
	if err := app.Run(); err != nil {
		fmt.Fprintln(os.Stderr, "glyph run error:", err)
		os.Exit(1)
	}
}
