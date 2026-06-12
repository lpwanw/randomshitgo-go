package attach

import (
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	uv "github.com/charmbracelet/ultraviolet"
)

// Cell is a 0-based (col, row) coordinate inside the vt grid.
type Cell struct {
	X, Y int
}

// Selection captures an anchored, reading-order character selection in the
// attach pane. Anchor is where the drag began; Cursor is the current edge.
// Active is false until a drag actually starts.
type Selection struct {
	Anchor Cell
	Cursor Cell
	Active bool
}

// Normalized returns the selection endpoints in reading order (lo precedes hi
// row-major), independent of drag direction.
func (s Selection) Normalized() (lo, hi Cell) {
	a, c := s.Anchor, s.Cursor
	if a.Y < c.Y || (a.Y == c.Y && a.X <= c.X) {
		return a, c
	}
	return c, a
}

// Contains reports whether grid cell (x, y) falls inside the selection, using
// reading-order (row-major) flow: every cell from lo to hi inclusive is
// selected, wrapping across full rows. Returns false when inactive.
func (s Selection) Contains(x, y int) bool {
	if !s.Active {
		return false
	}
	lo, hi := s.Normalized()
	if y < lo.Y || y > hi.Y {
		return false
	}
	if y == lo.Y && x < lo.X {
		return false
	}
	if y == hi.Y && x > hi.X {
		return false
	}
	return true
}

// MsgToMouse converts a Bubble Tea mouse event into an ultraviolet mouse
// event with pane-relative 0-based coordinates (origin subtracted). Returns
// false when the event lands outside the pane (negative relative coords) or
// has no uv equivalent.
func MsgToMouse(msg tea.MouseMsg, originX, originY int) (uv.MouseEvent, bool) {
	x := msg.X - originX
	y := msg.Y - originY
	if x < 0 || y < 0 {
		return nil, false
	}

	btn, ok := teaButtonToUV(msg.Button)
	if !ok {
		return nil, false
	}
	m := uv.Mouse{X: x, Y: y, Button: btn, Mod: teaMouseMod(msg)}

	switch msg.Action {
	case tea.MouseActionPress:
		if msg.Button == tea.MouseButtonWheelUp || msg.Button == tea.MouseButtonWheelDown ||
			msg.Button == tea.MouseButtonWheelLeft || msg.Button == tea.MouseButtonWheelRight {
			return uv.MouseWheelEvent(m), true
		}
		return uv.MouseClickEvent(m), true
	case tea.MouseActionRelease:
		return uv.MouseReleaseEvent(m), true
	case tea.MouseActionMotion:
		return uv.MouseMotionEvent(m), true
	}
	return nil, false
}

// teaButtonToUV maps a Bubble Tea mouse button to its ultraviolet equivalent.
func teaButtonToUV(b tea.MouseButton) (uv.MouseButton, bool) {
	switch b {
	case tea.MouseButtonNone:
		return uv.MouseNone, true
	case tea.MouseButtonLeft:
		return uv.MouseLeft, true
	case tea.MouseButtonMiddle:
		return uv.MouseMiddle, true
	case tea.MouseButtonRight:
		return uv.MouseRight, true
	case tea.MouseButtonWheelUp:
		return uv.MouseWheelUp, true
	case tea.MouseButtonWheelDown:
		return uv.MouseWheelDown, true
	case tea.MouseButtonWheelLeft:
		return uv.MouseWheelLeft, true
	case tea.MouseButtonWheelRight:
		return uv.MouseWheelRight, true
	case tea.MouseButtonBackward:
		return uv.MouseBackward, true
	case tea.MouseButtonForward:
		return uv.MouseForward, true
	}
	return uv.MouseNone, false
}

// teaMouseMod translates the modifier flags on a Bubble Tea mouse event into
// an ultraviolet KeyMod.
func teaMouseMod(msg tea.MouseMsg) uv.KeyMod {
	var mod uv.KeyMod
	if msg.Shift {
		mod |= uv.ModShift
	}
	if msg.Alt {
		mod |= uv.ModAlt
	}
	if msg.Ctrl {
		mod |= uv.ModCtrl
	}
	return mod
}

// SelectionText extracts the plain text covered by sel from the emulator grid.
// Per-row trailing spaces are trimmed and rows are joined with '\n'. Wide
// glyphs (Cell.Width == 2) contribute their content once and advance two
// columns. Returns "" for an inactive selection or nil term.
func SelectionText(t *VTTerm, sel Selection) string {
	if t == nil || !sel.Active {
		return ""
	}
	lo, hi := sel.Normalized()
	w := t.Width()
	var rows []string
	for y := lo.Y; y <= hi.Y; y++ {
		startX := 0
		if y == lo.Y {
			startX = lo.X
		}
		endX := w - 1
		if y == hi.Y {
			endX = hi.X
		}
		rows = append(rows, rowText(t, y, startX, endX))
	}
	return strings.Join(rows, "\n")
}

// rowText reads cells [startX, endX] of grid row y into a string, honouring
// wide glyphs and trimming trailing spaces.
func rowText(t *VTTerm, y, startX, endX int) string {
	var sb strings.Builder
	for x := startX; x <= endX; {
		c := t.CellAt(x, y)
		if c == nil {
			sb.WriteByte(' ')
			x++
			continue
		}
		content := c.Content
		if content == "" {
			content = " "
		}
		sb.WriteString(content)
		if c.Width == 2 {
			x += 2
		} else {
			x++
		}
	}
	return strings.TrimRight(sb.String(), " ")
}
