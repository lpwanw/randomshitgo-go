package attach

import (
	"strings"

	uv "github.com/charmbracelet/ultraviolet"
)

// Render produces a w×h ANSI-styled snapshot of the emulator's active
// screen. Consecutive cells with identical Style are batched into a
// single SGR run so the output has at most a few escapes per row even on
// a 200-cell wide terminal.
//
// The cursor is drawn as a reverse-video cell at the emulator's current
// CursorPosition. Wide grapheme clusters (uv.Cell.Width == 2) emit one
// content rune and consume two grid cells.
//
// Output has h-1 newlines (no trailing newline) so it composes directly
// with lipgloss.JoinVertical / JoinHorizontal.
func Render(t *VTTerm, w, h int) string {
	if w <= 0 || h <= 0 || t == nil {
		return ""
	}
	tw, th := t.Width(), t.Height()
	cw := w
	if tw < cw {
		cw = tw
	}
	ch := h
	if th < ch {
		ch = th
	}
	cur := t.CursorPosition()

	var sb strings.Builder
	sb.Grow(w * h * 2)

	for y := 0; y < ch; y++ {
		written := renderLine(&sb, t, y, cw, cur.X, y == cur.Y)
		if written < w {
			sb.WriteString(strings.Repeat(" ", w-written))
		}
		if y < h-1 {
			sb.WriteByte('\n')
		}
	}
	for y := ch; y < h; y++ {
		sb.WriteString(strings.Repeat(" ", w))
		if y < h-1 {
			sb.WriteByte('\n')
		}
	}
	return sb.String()
}

// renderLine writes one row. Returns the number of display columns
// actually emitted (≤ w) so the caller can right-pad.
func renderLine(sb *strings.Builder, t *VTTerm, y, w, curX int, hasCursor bool) int {
	var run strings.Builder
	var runStyle uv.Style
	runActive := false

	flush := func() {
		if !runActive {
			return
		}
		s := run.String()
		if runStyle.IsZero() {
			sb.WriteString(s)
		} else {
			sb.WriteString(runStyle.Styled(s))
		}
		run.Reset()
		runActive = false
	}

	cols := 0
	for x := 0; x < w; {
		c := t.CellAt(x, y)
		content := " "
		var st uv.Style
		width := 1
		if c != nil {
			if c.Content != "" {
				content = c.Content
			}
			st = c.Style
			if c.Width == 2 {
				width = 2
			}
		}
		// Don't overflow the right edge with the trailing half of a
		// wide glyph.
		if width == 2 && x+1 >= w {
			content = " "
			width = 1
		}

		isCursor := hasCursor && x == curX
		if isCursor {
			cs := st
			cs.Attrs ^= uv.AttrReverse
			flush()
			if cs.IsZero() {
				sb.WriteString(content)
			} else {
				sb.WriteString(cs.Styled(content))
			}
			cols += width
			x += width
			continue
		}

		if !runActive {
			runStyle = st
			runActive = true
			run.WriteString(content)
			cols += width
			x += width
			continue
		}
		if (&st).Equal(&runStyle) {
			run.WriteString(content)
			cols += width
			x += width
		} else {
			flush()
			runStyle = st
			runActive = true
			run.WriteString(content)
			cols += width
			x += width
		}
	}
	flush()
	return cols
}
