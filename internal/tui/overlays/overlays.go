// Package overlays contains modal overlay sub-models: help, group picker,
// branch picker, filter input, and toast stack. Each overlay owns its own
// rendering and key handling; the root tui.Model composes them via Set.
package overlays

// Set composes all four overlay sub-models and the toast stack.
// It provides a single point of integration for the root tui.Model.
type Set struct {
	Help    HelpOverlay
	Group   GroupPicker
	Branch  BranchPicker
	Filter  FilterInput
	Toasts  ToastStack
}

// NewSet constructs a Set with sane defaults.
func NewSet(groups map[string][]string, branches []string) Set {
	return Set{
		Help:   NewHelpOverlay(),
		Group:  NewGroupPicker(groups),
		Branch: NewBranchPicker(branches),
		Filter: NewFilterInput(),
	}
}
