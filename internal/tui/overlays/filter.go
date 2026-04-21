package overlays

import (
	"fmt"
	"regexp"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"
)

var (
	styleFilterPrompt = lipgloss.NewStyle().
				Foreground(lipgloss.Color("212")).
				Bold(true)

	styleFilterErr = lipgloss.NewStyle().
			Foreground(lipgloss.Color("196"))

	styleFilterBar = lipgloss.NewStyle().
			Background(lipgloss.Color("236")).
			Foreground(lipgloss.Color("252"))
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

// FilterInput wraps a textinput and emits commit/cancel messages. It renders as
// a single-line bar meant to sit at the bottom of the frame (vim `/` style),
// NOT a modal — do not wrap the output in lipgloss.Place.
type FilterInput struct {
	ti      textinput.Model
	visible bool
	errMsg  string
}

// NewFilterInput creates a ready FilterInput.
func NewFilterInput() FilterInput {
	ti := textinput.New()
	ti.Prompt = ""
	ti.Placeholder = "regex (case-insensitive)"
	ti.CharLimit = 200
	ti.Width = 60
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

// View renders the filter bar as a single row sized to width. Returns "" when
// the bar is hidden. The caller is responsible for placing the row into the
// frame (e.g. above the status bar) — this method MUST NOT pad vertically.
func (fi *FilterInput) View(width int) string {
	if !fi.visible {
		return ""
	}
	if width <= 0 {
		width = 80
	}

	prompt := styleFilterPrompt.Render("/")
	input := fi.ti.View()
	left := prompt + input

	suffix := ""
	if fi.errMsg != "" {
		suffix = "  " + styleFilterErr.Render(fi.errMsg)
	}

	bar := left + suffix
	// Clamp to width (ansi-aware) and pad right so the bar fills the row.
	if ansi.StringWidth(bar) > width {
		bar = ansi.Truncate(bar, width, "…")
	}
	return styleFilterBar.Width(width).Render(bar)
}

// ShowToastMsg duplicates the tui-level type to avoid import cycle.
// The tui package bridges via type assertion or mirrors the type.
type ShowToastMsg struct {
	Text  string
	Level int // 0=info 1=warn 2=err
}
