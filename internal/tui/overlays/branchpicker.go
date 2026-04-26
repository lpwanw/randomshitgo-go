package overlays

import (
	"strings"

	"github.com/charmbracelet/bubbles/key"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

var stylePickerFilter = lipgloss.NewStyle().
	Foreground(lipgloss.Color("212")).
	MarginBottom(1)

// Branch-picker-local bindings. Arrow keys only for navigation so printable
// letters (incl. j/k/q) fall through to the filter. Esc excludes `q` for the
// same reason — branch names can contain `q`.
var (
	branchPickerUp   = key.NewBinding(key.WithKeys("up"))
	branchPickerDown = key.NewBinding(key.WithKeys("down"))
	branchPickerEsc  = key.NewBinding(key.WithKeys("esc"))
)

// CheckoutBranchMsg is emitted when the user selects a branch.
type CheckoutBranchMsg struct {
	Branch string
}

// BranchPicker presents a list of git branches with a live substring filter
// and emits CheckoutBranchMsg on confirm. Type any printable char to narrow
// the list; Backspace pops; Esc clears a non-empty filter, then closes.
type BranchPicker struct {
	branches []string // full list, index-stable
	filtered []int    // indexes into branches after filter application
	filter   string
	cursor   int // index into filtered
	visible  bool
}

// NewBranchPicker creates a BranchPicker with an initial branch list.
func NewBranchPicker(branches []string) BranchPicker {
	if len(branches) == 0 {
		branches = []string{"main", "dev"}
	}
	bp := BranchPicker{branches: branches}
	bp.rebuildFilter()
	return bp
}

// Show makes the overlay visible and resets filter + cursor.
func (bp *BranchPicker) Show() {
	bp.visible = true
	bp.filter = ""
	bp.cursor = 0
	bp.rebuildFilter()
}

// Hide makes the overlay invisible.
func (bp *BranchPicker) Hide() { bp.visible = false }

// Visible reports whether the overlay is shown.
func (bp *BranchPicker) Visible() bool { return bp.visible }

// SetBranches replaces the branch list and resets filter state.
func (bp *BranchPicker) SetBranches(branches []string) {
	bp.branches = branches
	bp.filter = ""
	bp.cursor = 0
	bp.rebuildFilter()
}

// rebuildFilter recomputes filtered indexes from branches + filter.
// Case-insensitive substring match; preserves original order.
func (bp *BranchPicker) rebuildFilter() {
	bp.filtered = bp.filtered[:0]
	if bp.filter == "" {
		for i := range bp.branches {
			bp.filtered = append(bp.filtered, i)
		}
	} else {
		needle := strings.ToLower(bp.filter)
		for i, b := range bp.branches {
			if strings.Contains(strings.ToLower(b), needle) {
				bp.filtered = append(bp.filtered, i)
			}
		}
	}
	if bp.cursor >= len(bp.filtered) {
		bp.cursor = 0
	}
}

// Update handles keyboard navigation, filter input, and confirmation.
func (bp BranchPicker) Update(msg tea.KeyMsg) (BranchPicker, tea.Cmd) {
	switch {
	case key.Matches(msg, branchPickerUp):
		if bp.cursor > 0 {
			bp.cursor--
		}
		return bp, nil
	case key.Matches(msg, branchPickerDown):
		if bp.cursor < len(bp.filtered)-1 {
			bp.cursor++
		}
		return bp, nil
	case key.Matches(msg, pickerEnter):
		if len(bp.filtered) == 0 {
			return bp, nil
		}
		branch := bp.branches[bp.filtered[bp.cursor]]
		bp.visible = false
		return bp, func() tea.Msg {
			return CheckoutBranchMsg{Branch: branch}
		}
	case key.Matches(msg, branchPickerEsc):
		// First Esc with non-empty filter only clears the filter so the user
		// can type a fresh query without reopening the picker.
		if bp.filter != "" {
			bp.filter = ""
			bp.cursor = 0
			bp.rebuildFilter()
			return bp, nil
		}
		bp.visible = false
		return bp, nil
	}

	// Backspace pops the last rune from the filter.
	if msg.Type == tea.KeyBackspace {
		if bp.filter != "" {
			r := []rune(bp.filter)
			bp.filter = string(r[:len(r)-1])
			bp.cursor = 0
			bp.rebuildFilter()
		}
		return bp, nil
	}

	// Printable runes append to the filter. Guard with Type == KeyRunes so
	// control keys (enter, tab, arrows) don't leak through — bubbletea sets
	// non-rune events to distinct Type values already handled above.
	if msg.Type == tea.KeyRunes && len(msg.Runes) > 0 {
		bp.filter += string(msg.Runes)
		bp.cursor = 0
		bp.rebuildFilter()
		return bp, nil
	}
	// Space also arrives as KeySpace on some terminals; treat as filter char.
	if msg.Type == tea.KeySpace {
		bp.filter += " "
		bp.cursor = 0
		bp.rebuildFilter()
		return bp, nil
	}

	return bp, nil
}

// View renders the branch picker as a compact bordered box. Caller composes it
// onto the main canvas (tui.overlayCenter) — do NOT lipgloss.Place to width×height.
func (bp *BranchPicker) View(_ int, _ int) string {
	if !bp.visible {
		return ""
	}

	title := stylePickerTitle.Render("Checkout Branch")

	filterRow := stylePickerFilter.Render("/ " + bp.filter)

	var rows string
	for i, idx := range bp.filtered {
		b := bp.branches[idx]
		if i == bp.cursor {
			rows += stylePickerSelected.Render("> "+b) + "\n"
		} else {
			rows += stylePickerNormal.Render("  "+b) + "\n"
		}
	}
	if len(bp.filtered) == 0 {
		if len(bp.branches) == 0 {
			rows = stylePickerNormal.Render("(no branches)") + "\n"
		} else {
			rows = stylePickerNormal.Render("(no matches)") + "\n"
		}
	}

	return stylePickerBox.Render(title + "\n" + filterRow + "\n" + rows)
}
