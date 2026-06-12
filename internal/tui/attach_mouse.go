package tui

import (
	"strings"

	"github.com/atotto/clipboard"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/lpwanw/randomshitgo-go/internal/tui/attach"
	"github.com/lpwanw/randomshitgo-go/internal/tui/panes"
)

// attachClipboard writes text to the system clipboard. Tests inject a fake.
type attachClipboard interface {
	WriteAll(text string) error
}

type sysAttachClipboard struct{}

func (sysAttachClipboard) WriteAll(text string) error { return clipboard.WriteAll(text) }

// SetAttachClipboard overrides the attach-pane clipboard writer (tests only).
func (m *Model) SetAttachClipboard(w attachClipboard) { m.attachClip = w }

// attachPaneGeometry returns the origin-X, width and height the attach pane
// renders with — recomputed from the same inputs View() uses. In embedded
// attach mode no filter/command bar is visible, so contentH is simply the
// terminal height minus the status bar.
func (m Model) attachPaneGeometry() (originX, w, h int) {
	sidebarW := sidebarWidth(m.width, m.cfg)
	return sidebarW, m.width - sidebarW, m.height - statusBarHeight
}

// routeEmbeddedAttachMouse handles mouse events while an embedded-attach
// session is active. When the child program has mouse tracking on, events are
// forwarded to it (tmux-style); otherwise left-drag selects pane cells and
// release copies the text to the clipboard.
func routeEmbeddedAttachMouse(m Model, msg tea.MouseMsg) (tea.Model, tea.Cmd) {
	if m.attach == nil {
		return m, nil
	}
	originX, paneW, paneH := m.attachPaneGeometry()

	// Child owns the mouse — forward and never local-select. Only forward
	// events that land on the grid; sidebar / status-bar / right-of-grid
	// clicks are dropped rather than reported with out-of-grid coords.
	if m.attach.MouseTracking() {
		if m.attachSel.Active {
			m.attachSel = attach.Selection{}
		}
		if _, in := attachCellAt(msg.X, msg.Y, originX, paneW, paneH); in {
			if ev, ok := attach.MsgToMouse(msg, originX, 0); ok {
				m.attach.SendMouse(ev)
			}
		}
		return m, nil
	}

	// Local drag-select. Wheel has nothing to scroll in the attach grid.
	if tea.MouseEvent(msg).IsWheel() {
		return m, nil
	}

	switch msg.Action {
	case tea.MouseActionPress:
		if msg.Button != tea.MouseButtonLeft {
			return m, nil
		}
		cell, ok := attachCellAt(msg.X, msg.Y, originX, paneW, paneH)
		if !ok {
			return m, nil
		}
		m.attachSel = attach.Selection{Anchor: cell, Cursor: cell, Active: true}
		return m, nil

	case tea.MouseActionMotion:
		if !m.attachSel.Active {
			return m, nil
		}
		m.attachSel.Cursor = attachClampCell(msg.X, msg.Y, originX, paneW, paneH)
		return m, nil

	case tea.MouseActionRelease:
		if !m.attachSel.Active {
			return m, nil
		}
		sel := m.attachSel
		m.attachSel = attach.Selection{}
		// Bare click (no drag) clears selection without copying.
		if sel.Anchor == sel.Cursor {
			return m, nil
		}
		text := attach.SelectionText(m.attach.Term(), sel)
		if text == "" {
			return m, nil
		}
		return m, attachYankCmd(m.attachClipboard(), text)
	}
	return m, nil
}

// attachCellAt maps absolute mouse coords to a pane cell, returning false when
// the click lands outside the attach pane.
func attachCellAt(absX, absY, originX, paneW, paneH int) (attach.Cell, bool) {
	x := absX - originX
	y := absY
	if x < 0 || y < 0 || x >= paneW || y >= paneH {
		return attach.Cell{}, false
	}
	return attach.Cell{X: x, Y: y}, true
}

// attachClampCell maps absolute coords to a pane cell, clamping out-of-pane
// drags to the nearest in-pane edge so a selection can extend past the border
// while the button is held.
func attachClampCell(absX, absY, originX, paneW, paneH int) attach.Cell {
	x := absX - originX
	y := absY
	x = clampInt(x, 0, paneW-1)
	y = clampInt(y, 0, paneH-1)
	return attach.Cell{X: x, Y: y}
}

func clampInt(v, lo, hi int) int {
	if v < lo {
		return lo
	}
	if v > hi {
		return hi
	}
	return v
}

// attachClipboard returns the injected writer or the system clipboard.
func (m Model) attachClipboard() attachClipboard {
	if m.attachClip != nil {
		return m.attachClip
	}
	return sysAttachClipboard{}
}

// attachYankCmd writes text to the clipboard off the event loop, reusing the
// panes Copied/CopyFailed messages so the existing toast handlers fire.
func attachYankCmd(w attachClipboard, text string) tea.Cmd {
	return func() tea.Msg {
		if err := w.WriteAll(text); err != nil {
			return panes.CopyFailedMsg{Err: err.Error()}
		}
		return panes.CopiedMsg{
			Lines: strings.Count(text, "\n") + 1,
			Chars: len(text),
		}
	}
}
