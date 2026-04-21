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

// View renders the help overlay as a compact bordered box sized to content.
// Returns "" when not visible. The caller composes the box onto the main
// canvas via tui.overlayCenter — this function MUST NOT call lipgloss.Place
// with width×height, or the surrounding UI would be blanked out.
func (ho *HelpOverlay) View(keymap KeyMapProvider, width, _ int) string {
	if !ho.visible {
		return ""
	}

	boxW := 60
	if maxW := width - 4; maxW > 0 && boxW > maxW {
		boxW = maxW
	}
	ho.h.Width = boxW

	title := styleHelpTitle.Render("Keybindings")
	body := ho.h.FullHelpView(keymap.FullHelp())

	return styleHelpBox.Render(title + "\n" + body)
}
