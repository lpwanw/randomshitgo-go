package panes

import (
	"strings"

	"github.com/atotto/clipboard"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/lpwanw/randomshitgo-go/internal/log"
)

// selMode enumerates the visual-selection kinds used in copy mode. `selNone`
// means no selection is active — `y` still yanks the current line.
type selMode int

const (
	selNone selMode = iota
	selChar
	selLine
)

// selection captures an anchored visual range. The cursor is always in
// LogPanel.cur — anchor + cur + mode fully describe what's selected.
type selection struct {
	mode   selMode
	anchor cursor
}

// CopiedMsg is emitted after a successful clipboard write so the TUI can
// flip the mode back to Normal and surface a toast. `Chars` is the byte
// length of the copied text; `Lines` is 1 + count of '\n' separators.
type CopiedMsg struct {
	Lines int
	Chars int
}

// CopyFailedMsg is emitted when the clipboard write returns an error.
type CopyFailedMsg struct {
	Err string
}

// clipboardWriter allows tests to swap the real system clipboard for a fake.
type clipboardWriter interface {
	WriteAll(text string) error
}

type systemClipboard struct{}

func (systemClipboard) WriteAll(text string) error { return clipboard.WriteAll(text) }

// SetClipboard overrides the clipboard writer (tests only).
func (lp *LogPanel) SetClipboard(w clipboardWriter) { lp.clip = w }

// HasSelection reports whether a visual selection is active (any mode).
func (lp *LogPanel) HasSelection() bool { return lp.sel.mode != selNone }

// ClearSelection drops the current selection and repaints. No-op when no
// selection is active.
func (lp *LogPanel) ClearSelection() {
	if lp.sel.mode == selNone {
		return
	}
	lp.sel = selection{}
	lp.paintMatches()
}

// handleVisual returns true and mutates lp.sel when msg is a selection key
// (v / V / y / Y). When `y` or `Y` fires it also produces a tea.Cmd that
// writes the yank payload and emits Copied/CopyFailedMsg.
func (lp *LogPanel) handleVisual(msg tea.KeyMsg) (consumed bool, cmd tea.Cmd) {
	switch msg.String() {
	case "v":
		if lp.sel.mode == selChar {
			lp.sel = selection{}
		} else {
			lp.sel = selection{mode: selChar, anchor: lp.cur}
		}
		lp.paintMatches()
		return true, nil
	case "V":
		if lp.sel.mode == selLine {
			lp.sel = selection{}
		} else {
			lp.sel = selection{mode: selLine, anchor: lp.cur}
		}
		lp.paintMatches()
		return true, nil
	case "y":
		// Visual-y yanks the current selection immediately. No selection? Fall
		// through so the operator state machine can treat `y` as the start of
		// `y{motion}` / `yi{obj}` / `yy`.
		if lp.sel.mode == selNone {
			return false, nil
		}
		text := lp.yankText()
		lp.sel = selection{}
		return true, lp.yankCmd(text)
	case "Y":
		// Force linewise on the current line, then yank. Count-aware via the
		// operator machine: `2Y` yanks two lines.
		n := lp.effectiveCount()
		lp.clearTransient()
		r := lineRange(lp.cur.line, n, len(lp.rawLines))
		text := lp.extractRange(r)
		if text == "" {
			return true, nil
		}
		return true, lp.yankCmd(text)
	}
	return false, nil
}

// yankCmd produces a tea.Cmd that writes text to the clipboard off the event
// loop and returns CopiedMsg / CopyFailedMsg.
func (lp *LogPanel) yankCmd(text string) tea.Cmd {
	w := lp.clipboardWriter()
	return func() tea.Msg {
		if err := w.WriteAll(text); err != nil {
			return CopyFailedMsg{Err: err.Error()}
		}
		return CopiedMsg{
			Lines: strings.Count(text, "\n") + 1,
			Chars: len(text),
		}
	}
}

// clipboardWriter returns the injected writer, falling back to the system
// clipboard when tests haven't provided one.
func (lp *LogPanel) clipboardWriter() clipboardWriter {
	if lp.clip != nil {
		return lp.clip
	}
	return systemClipboard{}
}

// yankText builds the plain-text payload for the current selection. All SGR
// sequences are stripped before joining — clipboards don't want escape codes.
//
// Cases:
//   - no selection → current line (whole stripped line)
//   - charwise single line → substring [startCol, endCol)
//   - charwise multi-line → first[startCol:] + middles + last[:endCol]
//   - linewise → full stripped lines [startLine..endLine] joined by '\n'
func (lp *LogPanel) yankText() string {
	if len(lp.rawLines) == 0 {
		return ""
	}
	if lp.sel.mode == selNone {
		return log.StripANSI(lp.rawLines[lp.cur.line])
	}

	startLine, startCol, endLine, endCol := lp.normalisedSelection()

	if lp.sel.mode == selLine {
		parts := make([]string, 0, endLine-startLine+1)
		for i := startLine; i <= endLine; i++ {
			parts = append(parts, log.StripANSI(lp.rawLines[i]))
		}
		return strings.Join(parts, "\n")
	}

	// Charwise — end is inclusive in vim; we treat endCol as the index PAST
	// the last selected byte (half-open) so single-char selections have
	// length 1 when startCol==endCol+1. Adjust by incrementing endCol by 1.
	inclusiveEnd := endCol + 1
	if startLine == endLine {
		s := log.StripANSI(lp.rawLines[startLine])
		if inclusiveEnd > len(s) {
			inclusiveEnd = len(s)
		}
		if startCol > len(s) {
			startCol = len(s)
		}
		if startCol > inclusiveEnd {
			startCol = inclusiveEnd
		}
		return s[startCol:inclusiveEnd]
	}
	first := log.StripANSI(lp.rawLines[startLine])
	if startCol > len(first) {
		startCol = len(first)
	}
	last := log.StripANSI(lp.rawLines[endLine])
	if inclusiveEnd > len(last) {
		inclusiveEnd = len(last)
	}
	parts := []string{first[startCol:]}
	for i := startLine + 1; i < endLine; i++ {
		parts = append(parts, log.StripANSI(lp.rawLines[i]))
	}
	parts = append(parts, last[:inclusiveEnd])
	return strings.Join(parts, "\n")
}

// normalisedSelection returns the (start, end) of the visual range with
// start ≤ end in line-major order. Columns are stripped-byte offsets.
func (lp *LogPanel) normalisedSelection() (startLine, startCol, endLine, endCol int) {
	a, c := lp.sel.anchor, lp.cur
	if a.line < c.line || (a.line == c.line && a.col <= c.col) {
		return a.line, a.col, c.line, c.col
	}
	return c.line, c.col, a.line, a.col
}

// overlaySelection wraps the selected byte ranges on each line of `out` with
// reverse-video SGR. Called from paintMatches after filter highlights and
// before the gutter prefix — this way selection composes cleanly on top.
func (lp *LogPanel) overlaySelection(out []string) {
	if lp.sel.mode == selNone || len(out) == 0 {
		return
	}
	startLine, startCol, endLine, endCol := lp.normalisedSelection()
	if startLine < 0 {
		startLine = 0
	}
	if endLine >= len(out) {
		endLine = len(out) - 1
	}

	for i := startLine; i <= endLine; i++ {
		line := out[i]
		if lp.sel.mode == selLine {
			out[i] = "\x1b[7m" + line + "\x1b[27m"
			continue
		}
		// charwise — compute stripped-byte range for this line.
		strippedLen := len(log.StripANSI(lp.rawLines[i]))
		sCol, eCol := 0, strippedLen
		if i == startLine {
			sCol = startCol
		}
		if i == endLine {
			eCol = endCol + 1 // inclusive end → half-open
		}
		if sCol < 0 {
			sCol = 0
		}
		if eCol > strippedLen {
			eCol = strippedLen
		}
		if sCol >= eCol {
			continue
		}
		mapping := mapStrippedToRendered(line)
		s := mapping[sCol]
		e := mapping[eCol]
		out[i] = line[:s] + "\x1b[7m" + line[s:e] + "\x1b[27m" + line[e:]
	}
}
