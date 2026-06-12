package panes

import (
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"
)

// window is a minimal vertical scroller over pre-rendered rows. It replaces
// bubbles/viewport for the log panel: SetRows hands over the row slice
// directly (no join → split → measure-every-line round trip per tick), and
// View pads + truncates only the rows inside the visible window, so render
// cost scales with the pane height instead of the whole log buffer.
//
// Horizontal scrolling is intentionally absent — the viewport build it
// replaces never set horizontalStep, so its shift-wheel path was a no-op.
type window struct {
	Width   int
	Height  int
	YOffset int

	rows       []string
	wheelDelta int
}

// newWindow mirrors viewport.New defaults (wheel scrolls 3 lines).
func newWindow(width, height int) window {
	return window{Width: width, Height: height, wheelDelta: 3}
}

// SetRows replaces the content. The slice is retained as-is — callers hand
// over ownership. Offset is re-clamped so a shrinking buffer can't leave the
// window past the end.
func (w *window) SetRows(rows []string) {
	w.rows = rows
	if w.YOffset > w.maxYOffset() {
		w.YOffset = w.maxYOffset()
	}
}

func (w *window) maxYOffset() int { return maxInt(0, len(w.rows)-w.Height) }

// AtTop reports whether the window shows the first row.
func (w *window) AtTop() bool { return w.YOffset <= 0 }

// AtBottom reports whether the window shows the last row.
func (w *window) AtBottom() bool { return w.YOffset >= w.maxYOffset() }

// SetYOffset sets the scroll position, clamped to the content bounds.
func (w *window) SetYOffset(n int) {
	if n < 0 {
		n = 0
	}
	if m := w.maxYOffset(); n > m {
		n = m
	}
	w.YOffset = n
}

func (w *window) GotoTop()    { w.YOffset = 0 }
func (w *window) GotoBottom() { w.YOffset = w.maxYOffset() }

func (w *window) PageUp()          { w.SetYOffset(w.YOffset - w.Height) }
func (w *window) PageDown()        { w.SetYOffset(w.YOffset + w.Height) }
func (w *window) ScrollUp(n int)   { w.SetYOffset(w.YOffset - n) }
func (w *window) ScrollDown(n int) { w.SetYOffset(w.YOffset + n) }

// Update handles mouse wheel scrolling, mirroring viewport's wheel handling
// (press-action wheel events, 3-line delta). Always returns a nil Cmd; the
// signature keeps the call site shape familiar.
func (w *window) Update(msg tea.Msg) tea.Cmd {
	m, ok := msg.(tea.MouseMsg)
	if !ok || m.Action != tea.MouseActionPress {
		return nil
	}
	switch m.Button { //nolint:exhaustive
	case tea.MouseButtonWheelUp:
		w.ScrollUp(w.wheelDelta)
	case tea.MouseButtonWheelDown:
		w.ScrollDown(w.wheelDelta)
	}
	return nil
}

// View renders the visible rows padded/truncated to Width × Height. Matches
// viewport.View's lipgloss treatment so downstream layout code is unchanged,
// but only the on-screen slice pays the styling cost.
func (w *window) View() string {
	top := w.YOffset
	if top < 0 {
		top = 0
	}
	if top > len(w.rows) {
		top = len(w.rows)
	}
	bottom := top + w.Height
	if bottom > len(w.rows) {
		bottom = len(w.rows)
	}
	visible := w.rows[top:bottom]
	// Rows wider than the pane (wrap off) must be hard-truncated before
	// lipgloss sees them — its Width() would re-wrap them onto extra rows.
	// Mirrors viewport's ansi.Cut pass, but only over the visible slice.
	copied := false
	for i, row := range visible {
		if len(row) > w.Width && ansi.StringWidth(row) > w.Width {
			if !copied { // copy on first mutation so stored rows stay intact
				visible = append([]string(nil), visible...)
				copied = true
			}
			visible[i] = ansi.Truncate(row, w.Width, "")
		}
	}
	return lipgloss.NewStyle().
		Width(w.Width).
		Height(w.Height).
		MaxHeight(w.Height).
		MaxWidth(w.Width).
		Render(strings.Join(visible, "\n"))
}
