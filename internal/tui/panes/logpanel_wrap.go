package panes

import "strings"

// wrapLine hard-wraps `line` so each output row has at most `width` visible
// columns. SGR escape sequences don't count toward the column budget; active
// SGR state is re-emitted at the start of every continuation row so colour
// attributes survive the break. Returns `[]string{line}` when the line
// already fits or `width <= 0`.
//
// Visible-column counting is byte-based — correct for ASCII and one-byte
// UTF-8 continuation; CJK / emoji width is approximate. That's acceptable
// given log output is usually ASCII; runewidth can be added later.
func wrapLine(line string, width int) []string {
	if width <= 0 {
		return []string{line}
	}
	if visibleColumns(line) <= width {
		return []string{line}
	}

	var out []string
	var row strings.Builder
	var activeSGR strings.Builder // accumulated open SGR codes since last reset
	visible := 0
	i := 0
	n := len(line)

	flushRow := func() {
		if row.Len() == 0 {
			return
		}
		s := row.String()
		// Close any active SGR so the row doesn't bleed into the gutter / next row.
		if activeSGR.Len() > 0 {
			s += "\x1b[0m"
		}
		out = append(out, s)
		row.Reset()
		visible = 0
		// Start the next row with the active SGR state re-emitted so colour
		// continues seamlessly across the break.
		if activeSGR.Len() > 0 {
			row.WriteString(activeSGR.String())
		}
	}

	for i < n {
		loc := ansiSeqRe.FindStringIndex(line[i:])
		if loc != nil && loc[0] == 0 {
			seq := line[i : i+loc[1]]
			row.WriteString(seq)
			// Track only CSI "m" sequences (SGR) for re-emit. The reset byte
			// `\x1b[0m` or `\x1b[m` clears state; any other SGR accumulates.
			if isSGR(seq) {
				if isSGRReset(seq) {
					activeSGR.Reset()
				} else {
					activeSGR.WriteString(seq)
				}
			}
			i += loc[1]
			continue
		}
		// Visible byte.
		row.WriteByte(line[i])
		visible++
		i++
		if visible >= width {
			flushRow()
		}
	}
	flushRow()
	if len(out) == 0 {
		return []string{line}
	}
	return out
}

// visibleColumns returns the number of visible bytes in s — i.e. len(s) minus
// any ANSI escape-sequence bytes.
func visibleColumns(s string) int {
	cols := 0
	i := 0
	for i < len(s) {
		loc := ansiSeqRe.FindStringIndex(s[i:])
		if loc != nil && loc[0] == 0 {
			i += loc[1]
			continue
		}
		cols++
		i++
	}
	return cols
}

// isSGR reports whether an ANSI sequence is a Select-Graphic-Rendition
// instruction (the `m`-terminated ones we care about for colour state).
func isSGR(seq string) bool {
	return len(seq) >= 3 && seq[0] == 0x1b && seq[1] == '[' && seq[len(seq)-1] == 'm'
}

// isSGRReset reports whether seq is a SGR reset (`ESC[0m` or `ESC[m`).
func isSGRReset(seq string) bool {
	if !isSGR(seq) {
		return false
	}
	body := seq[2 : len(seq)-1]
	return body == "" || body == "0"
}
