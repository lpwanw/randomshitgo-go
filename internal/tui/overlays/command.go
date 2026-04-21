package overlays

import (
	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"
)

var (
	styleCommandPrompt = lipgloss.NewStyle().
				Foreground(lipgloss.Color("214")).
				Bold(true)

	styleCommandBar = lipgloss.NewStyle().
			Background(lipgloss.Color("236")).
			Foreground(lipgloss.Color("252"))
)

// CommandRunMsg is emitted when the user presses Enter with a non-empty command.
type CommandRunMsg struct {
	Text string
}

// CommandCancelMsg is emitted when the user presses Esc or Enter with empty text.
type CommandCancelMsg struct{}

// CommandInput is a vim-style `:` command bar that gates destructive actions
// like quit behind an explicit command. Mirrors the layout of FilterInput and
// is meant to render as a single row above the status bar — not a modal.
type CommandInput struct {
	ti      textinput.Model
	visible bool
}

// NewCommandInput returns a ready CommandInput.
func NewCommandInput() CommandInput {
	ti := textinput.New()
	ti.Prompt = ""
	ti.Placeholder = "q, quit"
	ti.CharLimit = 64
	ti.Width = 40
	return CommandInput{ti: ti}
}

// Show focuses the input and clears any previous text.
func (ci *CommandInput) Show() {
	ci.visible = true
	ci.ti.SetValue("")
	ci.ti.Focus()
}

// Hide blurs and hides the input.
func (ci *CommandInput) Hide() {
	ci.visible = false
	ci.ti.Blur()
}

// Visible reports whether the bar is currently active.
func (ci *CommandInput) Visible() bool { return ci.visible }

// Value returns the current raw input.
func (ci *CommandInput) Value() string { return ci.ti.Value() }

// Update processes key events.
func (ci CommandInput) Update(msg tea.Msg) (CommandInput, tea.Cmd) {
	if km, ok := msg.(tea.KeyMsg); ok {
		switch km.String() {
		case "esc":
			ci.visible = false
			ci.ti.Blur()
			return ci, func() tea.Msg { return CommandCancelMsg{} }
		case "enter":
			text := ci.ti.Value()
			ci.visible = false
			ci.ti.Blur()
			if text == "" {
				return ci, func() tea.Msg { return CommandCancelMsg{} }
			}
			return ci, func() tea.Msg { return CommandRunMsg{Text: text} }
		}
	}
	var cmd tea.Cmd
	ci.ti, cmd = ci.ti.Update(msg)
	return ci, cmd
}

// View renders the command bar as a single row sized to width. Returns "" when
// the bar is hidden. The caller is responsible for positioning the row into
// the frame (app.View drops it into the filter-bar slot).
func (ci *CommandInput) View(width int) string {
	if !ci.visible {
		return ""
	}
	if width <= 0 {
		width = 80
	}
	bar := styleCommandPrompt.Render(":") + ci.ti.View()
	if ansi.StringWidth(bar) > width {
		bar = ansi.Truncate(bar, width, "…")
	}
	return styleCommandBar.Width(width).Render(bar)
}
