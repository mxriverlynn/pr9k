package workflowedit

// menuBar is the top-of-screen File menu bar.
type menuBar struct {
	open bool
}

func newMenuBar() menuBar { return menuBar{} }

// ShortcutLine returns shortcut hints for menu-bar focus (D-11).
func (mb menuBar) ShortcutLine() string {
	return "↑/↓  navigate  ·  Enter  select  ·  Esc  close"
}
