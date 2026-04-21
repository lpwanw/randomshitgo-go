package tui

import "github.com/charmbracelet/bubbles/key"

// KeyMap declares all keybindings for the TUI application.
// Bindings are used both for key matching in Update and for rendering the help overlay.
type KeyMap struct {
	Up            key.Binding
	Down          key.Binding
	Start         key.Binding
	Restart       key.Binding
	Stop          key.Binding
	Attach        key.Binding
	GroupPicker   key.Binding
	BranchPicker  key.Binding
	StopAll       key.Binding
	Filter        key.Binding
	NextMatch     key.Binding
	PrevMatch     key.Binding
	PageUp        key.Binding
	PageDown      key.Binding
	Top           key.Binding
	Bottom        key.Binding
	Help          key.Binding
	Command       key.Binding
	Quit          key.Binding
	Esc           key.Binding
	Enter         key.Binding
	// QuickJumpKeys are 1..9 — jump sidebar cursor straight to rows 0..8.
	// QuickJump is a synthetic binding used purely to render a single help row.
	QuickJumpKeys [9]key.Binding
	QuickJump     key.Binding
	// CopyEnter opens vim-style copy mode inside the log panel.
	CopyEnter key.Binding
	// CopyMotion, CopyVisual, CopyYank are synthetic bindings used only by
	// the help overlay to describe what's available once inside copy mode.
	CopyMotion key.Binding
	CopyVisual key.Binding
	CopyYank   key.Binding
}

// DefaultKeyMap returns the default keybindings.
func DefaultKeyMap() KeyMap {
	return KeyMap{
		Up: key.NewBinding(
			key.WithKeys("k", "up"),
			key.WithHelp("k/↑", "prev process"),
		),
		Down: key.NewBinding(
			key.WithKeys("j", "down"),
			key.WithHelp("j/↓", "next process"),
		),
		Start: key.NewBinding(
			key.WithKeys("s"),
			key.WithHelp("s", "start"),
		),
		Restart: key.NewBinding(
			key.WithKeys("r"),
			key.WithHelp("r", "restart"),
		),
		Stop: key.NewBinding(
			key.WithKeys("x"),
			key.WithHelp("x", "stop"),
		),
		Attach: key.NewBinding(
			key.WithKeys("a"),
			key.WithHelp("a", "attach"),
		),
		GroupPicker: key.NewBinding(
			key.WithKeys("S"),
			key.WithHelp("S", "start group"),
		),
		BranchPicker: key.NewBinding(
			key.WithKeys("b"),
			key.WithHelp("b", "branch picker"),
		),
		StopAll: key.NewBinding(
			key.WithKeys("X"),
			key.WithHelp("X", "stop all"),
		),
		Filter: key.NewBinding(
			key.WithKeys("/"),
			key.WithHelp("/", "search logs"),
		),
		NextMatch: key.NewBinding(
			key.WithKeys("n"),
			key.WithHelp("n", "next match"),
		),
		PrevMatch: key.NewBinding(
			key.WithKeys("N"),
			key.WithHelp("N", "prev match"),
		),
		PageUp: key.NewBinding(
			key.WithKeys("pgup", "ctrl+b"),
			key.WithHelp("PgUp", "scroll up"),
		),
		PageDown: key.NewBinding(
			key.WithKeys("pgdown", "ctrl+f"),
			key.WithHelp("PgDn", "scroll down"),
		),
		Top: key.NewBinding(
			key.WithKeys("g"),
			key.WithHelp("g", "scroll top"),
		),
		Bottom: key.NewBinding(
			key.WithKeys("G"),
			key.WithHelp("G", "scroll bottom"),
		),
		Help: key.NewBinding(
			key.WithKeys("?"),
			key.WithHelp("?", "toggle help"),
		),
		Command: key.NewBinding(
			key.WithKeys(":"),
			key.WithHelp(":", "command (e.g. :q)"),
		),
		Quit: key.NewBinding(
			key.WithKeys("ctrl+c"),
			key.WithHelp("ctrl+c", "force quit"),
		),
		Esc: key.NewBinding(
			key.WithKeys("esc"),
			key.WithHelp("esc", "cancel"),
		),
		Enter: key.NewBinding(
			key.WithKeys("enter"),
			key.WithHelp("enter", "confirm"),
		),
		QuickJumpKeys: [9]key.Binding{
			key.NewBinding(key.WithKeys("1")),
			key.NewBinding(key.WithKeys("2")),
			key.NewBinding(key.WithKeys("3")),
			key.NewBinding(key.WithKeys("4")),
			key.NewBinding(key.WithKeys("5")),
			key.NewBinding(key.WithKeys("6")),
			key.NewBinding(key.WithKeys("7")),
			key.NewBinding(key.WithKeys("8")),
			key.NewBinding(key.WithKeys("9")),
		},
		QuickJump: key.NewBinding(
			key.WithKeys("1", "2", "3", "4", "5", "6", "7", "8", "9"),
			key.WithHelp("1-9", "jump to project"),
		),
		CopyEnter: key.NewBinding(
			key.WithKeys("tab"),
			key.WithHelp("tab", "log focus"),
		),
		CopyMotion: key.NewBinding(
			key.WithKeys("hjkl"),
			key.WithHelp("hjklwbeWBE0^$+-ggG", "focus: motions"),
		),
		CopyVisual: key.NewBinding(
			key.WithKeys("v"),
			key.WithHelp("v/V / fFtT;,", "focus: visual / find-char"),
		),
		CopyYank: key.NewBinding(
			key.WithKeys("y"),
			key.WithHelp("yy Y yiw ya\" yi(", "focus: yank (operator+text obj)"),
		),
	}
}

// ShortHelp returns the short help bindings shown in the status bar.
func (k KeyMap) ShortHelp() []key.Binding {
	return []key.Binding{k.Up, k.Down, k.Start, k.Stop, k.Filter, k.Help, k.Command}
}

// FullHelp returns bindings grouped by category for the help overlay.
func (k KeyMap) FullHelp() [][]key.Binding {
	return [][]key.Binding{
		{k.Up, k.Down, k.QuickJump},
		{k.Start, k.Restart, k.Stop, k.Attach},
		{k.GroupPicker, k.BranchPicker, k.StopAll},
		{k.Filter, k.NextMatch, k.PrevMatch, k.PageUp, k.PageDown, k.Top, k.Bottom},
		{k.CopyEnter, k.CopyMotion, k.CopyVisual, k.CopyYank},
		{k.Help, k.Command, k.Quit, k.Esc},
	}
}
