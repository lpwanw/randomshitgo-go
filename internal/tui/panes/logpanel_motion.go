package panes

import (
	tea "github.com/charmbracelet/bubbletea"
	"github.com/lpwanw/randomshitgo-go/internal/log"
)

// cursor tracks a position inside the log buffer for copy mode. Col is a byte
// index into the ANSI-stripped form of `rawLines[line]`, so motion math is
// immune to embedded SGR sequences; rendering maps it back via
// mapStrippedToRendered.
type cursor struct {
	line int
	col  int
}

// HandleCopyKey dispatches a key press while the panel is in copy mode. It
// mutates cursor state, keeps the viewport aligned, and returns any tea.Cmd
// the caller should forward (always nil for motion-only; phase 03 will yank).
func (lp *LogPanel) HandleCopyKey(msg tea.KeyMsg) tea.Cmd {
	if !lp.inCopy {
		return nil
	}
	if len(lp.rawLines) == 0 {
		return nil
	}

	if consumed, cmd := lp.handleVisual(msg); consumed {
		return cmd
	}

	s := msg.String()
	// `g` is a leader; second `g` jumps to top.
	if lp.pendingG {
		lp.pendingG = false
		if s == "g" {
			lp.cur = cursor{0, 0}
			lp.ensureCursorVisible()
			lp.paintMatches()
			return nil
		}
		// any other key falls through to normal handling
	}

	moved := true
	switch s {
	case "h", "left":
		if lp.cur.col > 0 {
			lp.cur.col--
		}
	case "l", "right":
		if lp.cur.col < lineLen(lp, lp.cur.line) {
			lp.cur.col++
		}
	case "j", "down":
		if lp.cur.line < len(lp.rawLines)-1 {
			lp.cur.line++
			lp.clampCol()
		}
	case "k", "up":
		if lp.cur.line > 0 {
			lp.cur.line--
			lp.clampCol()
		}
	case "0":
		lp.cur.col = 0
	case "$":
		lp.cur.col = lineLen(lp, lp.cur.line)
	case "g":
		lp.pendingG = true
		return nil
	case "G":
		lp.cur.line = len(lp.rawLines) - 1
		lp.cur.col = 0
	case "w":
		lp.motionWordForward()
	case "b":
		lp.motionWordBackward()
	case "ctrl+u":
		lp.cur.line = maxInt(0, lp.cur.line-lp.vp.Height/2)
		lp.clampCol()
	case "ctrl+d":
		lp.cur.line = minInt(len(lp.rawLines)-1, lp.cur.line+lp.vp.Height/2)
		lp.clampCol()
	case "ctrl+b":
		lp.cur.line = maxInt(0, lp.cur.line-lp.vp.Height)
		lp.clampCol()
	case "ctrl+f":
		lp.cur.line = minInt(len(lp.rawLines)-1, lp.cur.line+lp.vp.Height)
		lp.clampCol()
	case "H":
		lp.cur.line = lp.vp.YOffset
		lp.clampCol()
	case "M":
		lp.cur.line = minInt(len(lp.rawLines)-1, lp.vp.YOffset+lp.vp.Height/2)
		lp.clampCol()
	case "L":
		lp.cur.line = minInt(len(lp.rawLines)-1, lp.vp.YOffset+lp.vp.Height-1)
		lp.clampCol()
	default:
		moved = false
	}

	if moved {
		lp.ensureCursorVisible()
		lp.paintMatches()
	}
	return nil
}

// Cursor returns the current copy-mode cursor (line, col). Both are 0-based;
// consumers like the status bar typically display 1-based.
func (lp *LogPanel) Cursor() (line, col int) { return lp.cur.line, lp.cur.col }

// clampCol keeps cursor.col inside [0, len(strippedLine)].
func (lp *LogPanel) clampCol() {
	n := lineLen(lp, lp.cur.line)
	if lp.cur.col > n {
		lp.cur.col = n
	}
	if lp.cur.col < 0 {
		lp.cur.col = 0
	}
}

// ensureCursorVisible scrolls the viewport so cursor.line is inside the
// visible window. No-op when already in range.
func (lp *LogPanel) ensureCursorVisible() {
	if lp.cur.line < lp.vp.YOffset {
		lp.vp.SetYOffset(lp.cur.line)
		return
	}
	if lp.cur.line >= lp.vp.YOffset+lp.vp.Height {
		lp.vp.SetYOffset(lp.cur.line - lp.vp.Height + 1)
	}
}

// motionWordForward moves the cursor to the start of the next word, crossing
// lines when necessary. Words are runs of word-class runes; line boundaries
// count as separators (matches vim's `w` on a simple word class).
func (lp *LogPanel) motionWordForward() {
	line, col := lp.cur.line, lp.cur.col
	total := len(lp.rawLines)

	// Phase 1: skip the word we're currently sitting on.
	for {
		s := strippedLine(lp, line)
		if col < len(s) && isWord(s[col]) {
			col++
			continue
		}
		break
	}

	// Phase 2: skip separators (including newlines) until the next word start.
	for line < total {
		s := strippedLine(lp, line)
		if col >= len(s) {
			line++
			col = 0
			continue
		}
		if !isWord(s[col]) {
			col++
			continue
		}
		lp.cur.line, lp.cur.col = line, col
		return
	}
	// Fell off buffer — clamp to last line EOL.
	lp.cur.line = total - 1
	lp.cur.col = lineLen(lp, lp.cur.line)
}

// motionWordBackward moves the cursor to the start of the previous word.
func (lp *LogPanel) motionWordBackward() {
	line := lp.cur.line
	col := lp.cur.col
	for line >= 0 {
		s := strippedLine(lp, line)
		if col > len(s) {
			col = len(s)
		}
		// step back one if we're sitting on a word start so we actually move
		if col > 0 {
			col--
		}
		// skip separators leftward
		for col > 0 && !isWord(s[col]) {
			col--
		}
		// skip word leftward to its start
		for col > 0 && isWord(s[col-1]) {
			col--
		}
		if col >= 0 && col < len(s) && isWord(s[col]) {
			lp.cur.line = line
			lp.cur.col = col
			return
		}
		line--
		if line >= 0 {
			col = len(strippedLine(lp, line))
		}
	}
	lp.cur.line = 0
	lp.cur.col = 0
}

// strippedLine returns rawLines[i] with ANSI SGR removed. Empty string when
// the index is out of range.
func strippedLine(lp *LogPanel, i int) string {
	if i < 0 || i >= len(lp.rawLines) {
		return ""
	}
	return log.StripANSI(lp.rawLines[i])
}

// lineLen returns len(strippedLine(i)).
func lineLen(lp *LogPanel, i int) int { return len(strippedLine(lp, i)) }

// isWord reports whether b is part of a vim-`w`-class word (alnum + `_`).
func isWord(b byte) bool {
	switch {
	case b >= 'a' && b <= 'z':
	case b >= 'A' && b <= 'Z':
	case b >= '0' && b <= '9':
	case b == '_':
	default:
		return false
	}
	return true
}

func minInt(a, b int) int {
	if a < b {
		return a
	}
	return b
}

func maxInt(a, b int) int {
	if a > b {
		return a
	}
	return b
}
