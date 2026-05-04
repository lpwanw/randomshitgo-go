package panes

import (
	tea "github.com/charmbracelet/bubbletea"

	"github.com/lpwanw/randomshitgo-go/internal/log"
)

// handleMouse drives left-button drag selection on the log pane. Wheel
// events fall through to the viewport scroll path (returned cmd may be
// nil). The convention mirrors xterm: drag selects, release auto-yanks.
func (lp *LogPanel) handleMouse(msg tea.MouseMsg) tea.Cmd {
	if tea.MouseEvent(msg).IsWheel() {
		return lp.scrollMouse(msg)
	}
	if msg.Button != tea.MouseButtonLeft && msg.Action != tea.MouseActionRelease {
		return nil
	}

	switch msg.Action {
	case tea.MouseActionPress:
		if msg.Button != tea.MouseButtonLeft {
			return nil
		}
		cur, ok := lp.mouseToCursor(msg.X, msg.Y)
		if !ok {
			return nil
		}
		lp.cur = cur
		lp.sel = selection{mode: selChar, anchor: cur}
		lp.drag.pressed = true
		lp.drag.anchorAt = cur
		// Selection holds the buffer in place until release / clear.
		lp.sticky = false
		lp.paintMatches()
		return nil

	case tea.MouseActionMotion:
		if !lp.drag.pressed {
			return nil
		}
		cur, ok := lp.mouseToCursor(msg.X, msg.Y)
		if !ok {
			// Drag escaped the pane — pin selection to the last in-pane edge.
			cur = lp.clampToPane(msg.X, msg.Y)
		}
		lp.cur = cur
		lp.paintMatches()
		return nil

	case tea.MouseActionRelease:
		if !lp.drag.pressed {
			return nil
		}
		lp.drag.pressed = false
		// Single click (no drag) clears any prior selection but does not yank.
		if lp.cur == lp.drag.anchorAt {
			lp.sel = selection{}
			lp.paintMatches()
			return nil
		}
		text := lp.yankText()
		lp.sel = selection{}
		lp.paintMatches()
		if text == "" {
			return nil
		}
		return lp.yankCmd(text)
	}
	return nil
}

// scrollMouse routes wheel events through the viewport, mirroring the
// pre-existing handler (sticky disengages when scrolling away from
// bottom).
func (lp *LogPanel) scrollMouse(msg tea.MouseMsg) tea.Cmd {
	var cmd tea.Cmd
	lp.vp, cmd = lp.vp.Update(msg)
	if !lp.vp.AtBottom() {
		lp.sticky = false
	}
	return cmd
}

// mouseToCursor maps an absolute MouseMsg X/Y to a (line, col) inside the
// raw log buffer. Returns ok=false when the click lands on chrome
// (border, scrollbar, gutter) or outside the pane entirely.
func (lp *LogPanel) mouseToCursor(absX, absY int) (cursor, bool) {
	contentX, contentY, ok := lp.absToContent(absX, absY)
	if !ok {
		return cursor{}, false
	}
	if contentY >= lp.vp.Height {
		return cursor{}, false
	}
	renderedRow := lp.vp.YOffset + contentY
	if renderedRow >= len(lp.rowToRaw) {
		// Click below the last rendered row — snap to last raw line.
		if len(lp.rawLines) == 0 {
			return cursor{}, false
		}
		return cursor{line: len(lp.rawLines) - 1, col: 0}, true
	}
	rawLine := lp.rowToRaw[renderedRow]
	col := lp.colForRenderedRow(renderedRow, contentX)
	return cursor{line: rawLine, col: col}, true
}

// absToContent strips border + gutter + scrollbar to get pane-content
// coordinates. Returns ok=false on any chrome cell.
func (lp *LogPanel) absToContent(absX, absY int) (contentX, contentY int, ok bool) {
	relX := absX - lp.xOrigin
	relY := absY - lp.yOrigin
	if relX < 0 || relY < 0 {
		return 0, 0, false
	}
	if relX >= lp.width || relY >= lp.height {
		return 0, 0, false
	}
	// Border cells.
	if relX == 0 || relX == lp.width-1 {
		return 0, 0, false
	}
	if relY == 0 || relY == lp.height-1 {
		return 0, 0, false
	}
	// Inside content area.
	contentY = relY - 1
	innerX := relX - 1
	gw := lp.gutterWidth()
	// Scrollbar reserves the rightmost inner column.
	scrollbarCol := lp.vp.Width + gw
	if innerX >= scrollbarCol {
		return 0, 0, false
	}
	if innerX < gw {
		// Gutter — snap to col 0 of the line by reporting contentX=0 OK.
		return 0, contentY, true
	}
	return innerX - gw, contentY, true
}

// clampToPane returns the cursor at the nearest in-pane cell to an
// out-of-bounds drag position, so selection can extend past the edge
// while a button is held.
func (lp *LogPanel) clampToPane(absX, absY int) cursor {
	relY := absY - lp.yOrigin
	if relY < 1 {
		// Above pane — anchor at first visible row.
		if len(lp.rowToRaw) == 0 || len(lp.rawLines) == 0 {
			return lp.drag.anchorAt
		}
		topRow := lp.vp.YOffset
		if topRow >= len(lp.rowToRaw) {
			topRow = len(lp.rowToRaw) - 1
		}
		return cursor{line: lp.rowToRaw[topRow], col: 0}
	}
	if relY >= lp.height-1 {
		// Below pane — anchor at last visible row, EOL.
		if len(lp.rawLines) == 0 {
			return lp.drag.anchorAt
		}
		botRow := lp.vp.YOffset + lp.vp.Height - 1
		if botRow >= len(lp.rowToRaw) {
			botRow = len(lp.rowToRaw) - 1
		}
		if botRow < 0 {
			return lp.drag.anchorAt
		}
		raw := lp.rowToRaw[botRow]
		stripped := log.StripANSI(lp.rawLines[raw])
		return cursor{line: raw, col: maxInt(0, len(stripped)-1)}
	}
	// Horizontally clamped — keep current Y, pin X to nearest pane edge.
	contentY := relY - 1
	renderedRow := lp.vp.YOffset + contentY
	if renderedRow >= len(lp.rowToRaw) || renderedRow < 0 {
		return lp.drag.anchorAt
	}
	raw := lp.rowToRaw[renderedRow]
	col := 0
	if absX-lp.xOrigin >= lp.width-1 {
		stripped := log.StripANSI(lp.rawLines[raw])
		col = maxInt(0, len(stripped)-1)
	}
	return cursor{line: raw, col: col}
}

// colForRenderedRow translates a content-X within a rendered row into a
// stripped-byte column of the underlying raw line. When wrap is on the
// row may be the Nth piece of its raw line; we add (N * contentWidth) to
// the click X. Defensive clamps keep col inside the raw line.
func (lp *LogPanel) colForRenderedRow(renderedRow, contentX int) int {
	if renderedRow < 0 || renderedRow >= len(lp.rowToRaw) {
		return 0
	}
	raw := lp.rowToRaw[renderedRow]
	stripped := log.StripANSI(lp.rawLines[raw])
	if len(stripped) == 0 {
		return 0
	}
	wrapPiece := 0
	if lp.wrapOn {
		// Walk back to count preceding rows belonging to the same raw line.
		for k := renderedRow - 1; k >= 0 && lp.rowToRaw[k] == raw; k-- {
			wrapPiece++
		}
	}
	gw := lp.gutterWidth()
	contentWidth := lp.vp.Width - gw
	if contentWidth < 1 {
		contentWidth = 1
	}
	col := wrapPiece*contentWidth + contentX
	if col < 0 {
		col = 0
	}
	if col >= len(stripped) {
		col = len(stripped) - 1
	}
	return col
}
