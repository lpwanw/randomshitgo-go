package tui

// Mode represents the current input mode of the TUI application.
// Only one mode is active at a time; modes determine key routing.
type Mode int

const (
	// ModeNormal is the default mode — sidebar navigation + process control.
	ModeNormal Mode = iota
	// ModeGroupPicker is active when the group-start overlay is shown.
	ModeGroupPicker
	// ModeBranchPicker is active when the git branch picker overlay is shown.
	ModeBranchPicker
	// ModeFilter is active when the log filter text-input is open.
	ModeFilter
	// ModeAttach is active while the terminal is handed to a child PTY.
	ModeAttach
	// ModeHelp is active when the keybinding cheatsheet overlay is shown.
	ModeHelp
	// ModeCommand is active when the vim-style `:` command bar is open.
	ModeCommand
)

// String returns a short human-readable label for the mode.
func (m Mode) String() string {
	switch m {
	case ModeNormal:
		return "NORMAL"
	case ModeGroupPicker:
		return "GROUP"
	case ModeBranchPicker:
		return "BRANCH"
	case ModeFilter:
		return "FILTER"
	case ModeAttach:
		return "ATTACH"
	case ModeHelp:
		return "HELP"
	case ModeCommand:
		return "COMMAND"
	default:
		return "UNKNOWN"
	}
}
