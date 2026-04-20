package overlays

import (
	"github.com/charmbracelet/bubbles/key"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

// CheckoutBranchMsg is emitted when the user selects a branch.
type CheckoutBranchMsg struct {
	Branch string
}

// BranchPicker presents a list of git branches and emits CheckoutBranchMsg on confirm.
// Phase 10 will wire real branch data; until then a stub list is used.
type BranchPicker struct {
	branches []string
	cursor   int
	visible  bool
}

// NewBranchPicker creates a BranchPicker with a stub branch list.
func NewBranchPicker(branches []string) BranchPicker {
	if len(branches) == 0 {
		branches = []string{"main", "dev"}
	}
	return BranchPicker{branches: branches}
}

// Show makes the overlay visible.
func (bp *BranchPicker) Show() {
	bp.visible = true
	bp.cursor = 0
}

// Hide makes the overlay invisible.
func (bp *BranchPicker) Hide() { bp.visible = false }

// Visible reports whether the overlay is shown.
func (bp *BranchPicker) Visible() bool { return bp.visible }

// SetBranches replaces the branch list (called by phase 10 git integration).
func (bp *BranchPicker) SetBranches(branches []string) {
	bp.branches = branches
	if bp.cursor >= len(bp.branches) {
		bp.cursor = 0
	}
}

// Update handles keyboard navigation.
func (bp BranchPicker) Update(msg tea.KeyMsg) (BranchPicker, tea.Cmd) {
	switch {
	case key.Matches(msg, pickerUp):
		if bp.cursor > 0 {
			bp.cursor--
		}
	case key.Matches(msg, pickerDown):
		if bp.cursor < len(bp.branches)-1 {
			bp.cursor++
		}
	case key.Matches(msg, pickerEnter):
		if len(bp.branches) > 0 {
			branch := bp.branches[bp.cursor]
			bp.visible = false
			return bp, func() tea.Msg {
				return CheckoutBranchMsg{Branch: branch}
			}
		}
	case key.Matches(msg, pickerEsc):
		bp.visible = false
	}
	return bp, nil
}

// View renders the branch picker centred in width×height.
func (bp *BranchPicker) View(width, height int) string {
	if !bp.visible {
		return ""
	}

	title := stylePickerTitle.Render("Checkout Branch")
	var rows string
	for i, b := range bp.branches {
		if i == bp.cursor {
			rows += lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("255")).Background(lipgloss.Color("62")).Render("> "+b) + "\n"
		} else {
			rows += stylePickerNormal.Render("  "+b) + "\n"
		}
	}
	if len(bp.branches) == 0 {
		rows = stylePickerNormal.Render("(no branches)") + "\n"
	}

	box := stylePickerBox.Render(title + "\n" + rows)
	return lipgloss.Place(width, height, lipgloss.Center, lipgloss.Center, box)
}
