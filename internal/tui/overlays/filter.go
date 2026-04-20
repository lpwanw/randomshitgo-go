package overlays

import (
	"fmt"
	"regexp"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

var (
	styleFilterBox = lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(lipgloss.Color("62")).
			Padding(0, 1).
			Width(50).
			Background(lipgloss.Color("234"))

	styleFilterLabel = lipgloss.NewStyle().
				Foreground(lipgloss.Color("212")).
				Bold(true)

	styleFilterErr = lipgloss.NewStyle().
			Foreground(lipgloss.Color("196"))
)

// FilterCommitMsg is emitted when the user presses Enter in the filter input.
type FilterCommitMsg struct {
	// Regex is the compiled pattern, or nil if the input was cleared.
	Regex *regexp.Regexp
	// Text is the raw filter string (empty = clear filter).
	Text string
}

// FilterCancelMsg is emitted when the user presses Esc in the filter input.
type FilterCancelMsg struct{}

// FilterInput wraps a textinput and emits commit/cancel messages.
type FilterInput struct {
	ti      textinput.Model
	visible bool
	errMsg  string
}

// NewFilterInput creates a ready FilterInput.
func NewFilterInput() FilterInput {
	ti := textinput.New()
	ti.Placeholder = "regex filter (case-insensitive)…"
	ti.CharLimit = 200
	ti.Width = 46
	return FilterInput{ti: ti}
}

// Show focuses the text input.
func (fi *FilterInput) Show() {
	fi.visible = true
	fi.errMsg = ""
	fi.ti.Focus()
}

// Hide unfocuses and hides.
func (fi *FilterInput) Hide() {
	fi.visible = false
	fi.ti.Blur()
}

// Visible reports whether the filter input is active.
func (fi *FilterInput) Visible() bool { return fi.visible }

// SetValue sets the current input text (used to restore previous filter text).
func (fi *FilterInput) SetValue(s string) {
	fi.ti.SetValue(s)
}

// Value returns the current input text.
func (fi *FilterInput) Value() string { return fi.ti.Value() }

// Update processes key events.
// Returns the updated FilterInput plus an optional Cmd.
func (fi FilterInput) Update(msg tea.Msg) (FilterInput, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.String() {
		case "esc":
			fi.visible = false
			fi.ti.Blur()
			return fi, func() tea.Msg { return FilterCancelMsg{} }

		case "enter":
			text := fi.ti.Value()
			fi.visible = false
			fi.ti.Blur()
			if text == "" {
				return fi, func() tea.Msg {
					return FilterCommitMsg{Regex: nil, Text: ""}
				}
			}
			rx, err := regexp.Compile("(?i)" + text)
			if err != nil {
				fi.errMsg = fmt.Sprintf("invalid regex: %v", err)
				fi.visible = true
				fi.ti.Focus()
				// Emit show-toast via ShowToastMsg — caller handles toast.
				return fi, func() tea.Msg {
					return ShowToastMsg{Text: fi.errMsg, Level: 1}
				}
			}
			fi.errMsg = ""
			return fi, func() tea.Msg {
				return FilterCommitMsg{Regex: rx, Text: text}
			}
		}
	}

	// Route all other messages to textinput.
	var cmd tea.Cmd
	fi.ti, cmd = fi.ti.Update(msg)
	return fi, cmd
}

// View renders the filter input centred in width×height.
func (fi *FilterInput) View(width, height int) string {
	if !fi.visible {
		return ""
	}

	label := styleFilterLabel.Render("/ Filter: ")
	input := fi.ti.View()

	errLine := ""
	if fi.errMsg != "" {
		errLine = "\n" + styleFilterErr.Render(fi.errMsg)
	}

	box := styleFilterBox.Render(label + input + errLine)
	return lipgloss.Place(width, height, lipgloss.Center, lipgloss.Center, box)
}

// ShowToastMsg duplicates the tui-level type to avoid import cycle.
// The tui package bridges via type assertion or mirrors the type.
type ShowToastMsg struct {
	Text  string
	Level int // 0=info 1=warn 2=err
}
