package overlays

import (
	"github.com/charmbracelet/bubbles/key"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

var (
	stylePickerBox = lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(lipgloss.Color("62")).
			Padding(1, 2).
			Background(lipgloss.Color("234")).
			Foreground(lipgloss.Color("252"))

	stylePickerTitle = lipgloss.NewStyle().
				Bold(true).
				Foreground(lipgloss.Color("212")).
				MarginBottom(1)

	stylePickerSelected = lipgloss.NewStyle().
				Bold(true).
				Foreground(lipgloss.Color("255")).
				Background(lipgloss.Color("62"))

	stylePickerNormal = lipgloss.NewStyle().
				Foreground(lipgloss.Color("252"))
)

var (
	pickerUp    = key.NewBinding(key.WithKeys("k", "up"))
	pickerDown  = key.NewBinding(key.WithKeys("j", "down"))
	pickerEnter = key.NewBinding(key.WithKeys("enter"))
	pickerEsc   = key.NewBinding(key.WithKeys("esc", "q"))
)

// StartGroupMsg is the message emitted when the user confirms a group selection.
// Defined here to avoid import cycles; tui package will use this type.
type StartGroupMsg struct {
	Name string
}

// GroupPicker presents a list of configured groups and emits StartGroupMsg on confirm.
type GroupPicker struct {
	groups  []string
	cursor  int
	visible bool
}

// NewGroupPicker creates a GroupPicker from the config groups map.
func NewGroupPicker(groups map[string][]string) GroupPicker {
	names := make([]string, 0, len(groups))
	for k := range groups {
		names = append(names, k)
	}
	// Sort for deterministic ordering.
	sortStrings(names)
	return GroupPicker{groups: names}
}

// Show makes the overlay visible, resetting cursor.
func (gp *GroupPicker) Show() {
	gp.visible = true
	gp.cursor = 0
}

// Hide makes the overlay invisible.
func (gp *GroupPicker) Hide() { gp.visible = false }

// Visible reports whether the overlay is shown.
func (gp *GroupPicker) Visible() bool { return gp.visible }

// Update handles keyboard navigation for the group picker.
// Returns the updated GroupPicker and an optional Cmd (StartGroupMsg or nil).
func (gp GroupPicker) Update(msg tea.KeyMsg) (GroupPicker, tea.Cmd) {
	switch {
	case key.Matches(msg, pickerUp):
		if gp.cursor > 0 {
			gp.cursor--
		}
	case key.Matches(msg, pickerDown):
		if gp.cursor < len(gp.groups)-1 {
			gp.cursor++
		}
	case key.Matches(msg, pickerEnter):
		if len(gp.groups) > 0 {
			name := gp.groups[gp.cursor]
			gp.visible = false
			return gp, func() tea.Msg {
				return StartGroupMsg{Name: name}
			}
		}
	case key.Matches(msg, pickerEsc):
		gp.visible = false
	}
	return gp, nil
}

// View renders the group picker as a compact bordered box. Caller composes it
// onto the main canvas (tui.overlayCenter) — do NOT lipgloss.Place to width×height.
func (gp *GroupPicker) View(_ int, _ int) string {
	if !gp.visible {
		return ""
	}

	title := stylePickerTitle.Render("Start Group")

	var rows string
	for i, g := range gp.groups {
		if i == gp.cursor {
			rows += stylePickerSelected.Render("> "+g) + "\n"
		} else {
			rows += stylePickerNormal.Render("  "+g) + "\n"
		}
	}
	if len(gp.groups) == 0 {
		rows = stylePickerNormal.Render("(no groups configured)") + "\n"
	}

	return stylePickerBox.Render(title + "\n" + rows)
}

// sortStrings is a simple insertion sort to avoid importing sort.
func sortStrings(s []string) {
	for i := 1; i < len(s); i++ {
		key := s[i]
		j := i - 1
		for j >= 0 && s[j] > key {
			s[j+1] = s[j]
			j--
		}
		s[j+1] = key
	}
}
