package overlays

import (
	"github.com/charmbracelet/bubbles/help"
	"github.com/charmbracelet/bubbles/key"
	"github.com/charmbracelet/lipgloss"
)

var (
	styleHelpBox = lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(lipgloss.Color("62")).
			Padding(1, 2).
			Background(lipgloss.Color("234")).
			Foreground(lipgloss.Color("252"))

	styleHelpTitle = lipgloss.NewStyle().
			Bold(true).
			Foreground(lipgloss.Color("212")).
			MarginBottom(1)
)

// KeyMapProvider is satisfied by tui.KeyMap to avoid an import cycle.
type KeyMapProvider interface {
	ShortHelp() []key.Binding
	FullHelp() [][]key.Binding
}

// HelpOverlay renders the full keybinding reference.
type HelpOverlay struct {
	visible bool
	h       help.Model
}

// NewHelpOverlay creates a HelpOverlay.
func NewHelpOverlay() HelpOverlay {
	h := help.New()
	h.ShowAll = true
	return HelpOverlay{h: h}
}

// Toggle flips the visibility state.
func (ho *HelpOverlay) Toggle() {
	ho.visible = !ho.visible
}

// Show makes the overlay visible.
func (ho *HelpOverlay) Show() { ho.visible = true }

// Hide makes the overlay invisible.
func (ho *HelpOverlay) Hide() { ho.visible = false }

// Visible reports whether the overlay is currently shown.
func (ho *HelpOverlay) Visible() bool { return ho.visible }

// View renders the help overlay centred inside width×height.
// Returns "" when not visible.
func (ho *HelpOverlay) View(keymap KeyMapProvider, width, height int) string {
	if !ho.visible {
		return ""
	}

	ho.h.Width = 60

	title := styleHelpTitle.Render("Keybindings")
	body := ho.h.FullHelpView(keymap.FullHelp())

	box := styleHelpBox.Render(title + "\n" + body)

	return lipgloss.Place(width, height,
		lipgloss.Center, lipgloss.Center,
		box,
	)
}
