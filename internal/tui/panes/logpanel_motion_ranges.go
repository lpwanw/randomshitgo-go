package panes

// resolveMotion maps a single motion key to a motionRange. Returns ok=false
// for unknown keys AND for keys that set a new pending state (like `g`). The
// caller inspects lp.pending to disambiguate.
func (lp *LogPanel) resolveMotion(s string, n int) (motionRange, bool) {
	start := lp.cur
	mul := n
	if mul < 1 {
		mul = 1
	}

	switch s {
	case "h", "left":
		end := start
		for i := 0; i < mul && end.col > 0; i++ {
			end.col--
		}
		return motionRange{start: start, end: end}, true
	case "l", "right":
		end := start
		maxC := lineLen(lp, end.line)
		for i := 0; i < mul && end.col < maxC; i++ {
			end.col++
		}
		return motionRange{start: start, end: end}, true
	case "j", "down":
		end := start
		for i := 0; i < mul && end.line < len(lp.rawLines)-1; i++ {
			end.line++
		}
		if end.col > lineLen(lp, end.line) {
			end.col = lineLen(lp, end.line)
		}
		return motionRange{start: start, end: end, linewise: true}, true
	case "k", "up":
		end := start
		for i := 0; i < mul && end.line > 0; i++ {
			end.line--
		}
		if end.col > lineLen(lp, end.line) {
			end.col = lineLen(lp, end.line)
		}
		return motionRange{start: start, end: end, linewise: true}, true
	case "0":
		return motionRange{start: start, end: cursor{start.line, 0}}, true
	case "^":
		return motionRange{start: start, end: cursor{start.line, firstNonBlank(strippedLine(lp, start.line))}}, true
	case "$":
		return motionRange{start: start, end: cursor{start.line, lineLen(lp, start.line)}, inclusive: true}, true
	case "+":
		target := start.line + mul
		if target >= len(lp.rawLines) {
			target = len(lp.rawLines) - 1
		}
		return motionRange{start: start, end: cursor{target, firstNonBlank(strippedLine(lp, target))}, linewise: true}, true
	case "-":
		target := start.line - mul
		if target < 0 {
			target = 0
		}
		return motionRange{start: start, end: cursor{target, firstNonBlank(strippedLine(lp, target))}, linewise: true}, true
	case "g":
		lp.pending = pendG
		return motionRange{}, false
	case "G":
		return motionRange{start: start, end: cursor{len(lp.rawLines) - 1, 0}, linewise: true}, true
	case "w":
		return motionRange{start: start, end: lp.wordForwardN(start, isWord, mul)}, true
	case "W":
		return motionRange{start: start, end: lp.wordForwardN(start, isWORD, mul)}, true
	case "b":
		return motionRange{start: start, end: lp.wordBackwardN(start, isWord, mul)}, true
	case "B":
		return motionRange{start: start, end: lp.wordBackwardN(start, isWORD, mul)}, true
	case "e":
		return motionRange{start: start, end: lp.wordEndForwardN(start, isWord, mul), inclusive: true}, true
	case "E":
		return motionRange{start: start, end: lp.wordEndForwardN(start, isWORD, mul), inclusive: true}, true
	case "ctrl+u":
		end := start
		half := lp.vp.Height / 2
		if half < 1 {
			half = 1
		}
		end.line = maxInt(0, end.line-half*mul)
		if end.col > lineLen(lp, end.line) {
			end.col = lineLen(lp, end.line)
		}
		return motionRange{start: start, end: end, linewise: true}, true
	case "ctrl+d":
		end := start
		half := lp.vp.Height / 2
		if half < 1 {
			half = 1
		}
		end.line = minInt(len(lp.rawLines)-1, end.line+half*mul)
		if end.col > lineLen(lp, end.line) {
			end.col = lineLen(lp, end.line)
		}
		return motionRange{start: start, end: end, linewise: true}, true
	case "ctrl+b":
		end := start
		end.line = maxInt(0, end.line-lp.vp.Height*mul)
		if end.col > lineLen(lp, end.line) {
			end.col = lineLen(lp, end.line)
		}
		return motionRange{start: start, end: end, linewise: true}, true
	case "ctrl+f":
		end := start
		end.line = minInt(len(lp.rawLines)-1, end.line+lp.vp.Height*mul)
		if end.col > lineLen(lp, end.line) {
			end.col = lineLen(lp, end.line)
		}
		return motionRange{start: start, end: end, linewise: true}, true
	case "H":
		end := cursor{lp.vp.YOffset, start.col}
		if end.col > lineLen(lp, end.line) {
			end.col = lineLen(lp, end.line)
		}
		return motionRange{start: start, end: end, linewise: true}, true
	case "M":
		end := cursor{minInt(len(lp.rawLines)-1, lp.vp.YOffset+lp.vp.Height/2), start.col}
		if end.col > lineLen(lp, end.line) {
			end.col = lineLen(lp, end.line)
		}
		return motionRange{start: start, end: end, linewise: true}, true
	case "L":
		end := cursor{minInt(len(lp.rawLines)-1, lp.vp.YOffset+lp.vp.Height-1), start.col}
		if end.col > lineLen(lp, end.line) {
			end.col = lineLen(lp, end.line)
		}
		return motionRange{start: start, end: end, linewise: true}, true
	}
	return motionRange{}, false
}

// firstNonBlank returns the byte index of the first non-space, non-tab char
// in s, or len(s) when the line is entirely whitespace.
func firstNonBlank(s string) int {
	for i := 0; i < len(s); i++ {
		if s[i] != ' ' && s[i] != '\t' {
			return i
		}
	}
	return len(s)
}

// wordForwardN advances `start` by n words using class predicate cls.
func (lp *LogPanel) wordForwardN(start cursor, cls classFn, n int) cursor {
	cur := start
	for i := 0; i < n; i++ {
		cur = lp.wordForwardOnce(cur, cls)
	}
	return cur
}

// wordForwardOnce advances one word start. Vim `w` semantics: an element run
// (word-class) and a punctuation run (non-class, non-whitespace) are each
// separate "words"; whitespace is always skipped. With the WORD class any
// non-whitespace counts, so `W` skips the whole run.
func (lp *LogPanel) wordForwardOnce(start cursor, cls classFn) cursor {
	line, col := start.line, start.col
	total := len(lp.rawLines)
	s := strippedLine(lp, line)

	if col >= len(s) {
		if line+1 >= total {
			return cursor{total - 1, lineLen(lp, total-1)}
		}
		line++
		col = 0
		s = strippedLine(lp, line)
	}
	if col < len(s) {
		curClass := classOf(s[col], cls)
		if curClass != 0 {
			for col < len(s) && classOf(s[col], cls) == curClass {
				col++
			}
		}
	}
	for line < total {
		s = strippedLine(lp, line)
		if col >= len(s) {
			if line+1 >= total {
				return cursor{total - 1, lineLen(lp, total-1)}
			}
			line++
			col = 0
			continue
		}
		if classOf(s[col], cls) == 0 {
			col++
			continue
		}
		return cursor{line, col}
	}
	return cursor{total - 1, lineLen(lp, total-1)}
}

// wordBackwardN retreats `start` by n words using class predicate cls.
func (lp *LogPanel) wordBackwardN(start cursor, cls classFn, n int) cursor {
	cur := start
	for i := 0; i < n; i++ {
		cur = lp.wordBackwardOnce(cur, cls)
	}
	return cur
}

// wordBackwardOnce retreats to the start of the previous word.
func (lp *LogPanel) wordBackwardOnce(start cursor, cls classFn) cursor {
	line, col := start.line, start.col
	// Step back one byte (cross line if at col 0).
	if col == 0 {
		if line == 0 {
			return cursor{0, 0}
		}
		line--
		col = lineLen(lp, line)
	}
	col--
	if col < 0 {
		col = 0
	}
	// Skip whitespace leftward.
	for {
		s := strippedLine(lp, line)
		for col >= 0 && col < len(s) && (s[col] == ' ' || s[col] == '\t') {
			col--
		}
		if col >= 0 && col < len(s) {
			break
		}
		if line == 0 {
			return cursor{0, 0}
		}
		line--
		col = lineLen(lp, line) - 1
	}
	s := strippedLine(lp, line)
	curClass := classOf(s[col], cls)
	for col > 0 {
		prev := classOf(s[col-1], cls)
		if prev == curClass {
			col--
			continue
		}
		break
	}
	return cursor{line, col}
}

// wordEndForwardN advances `start` by n end-of-word positions using class cls.
func (lp *LogPanel) wordEndForwardN(start cursor, cls classFn, n int) cursor {
	cur := start
	for i := 0; i < n; i++ {
		cur = lp.wordEndForwardOnce(cur, cls)
	}
	return cur
}

// wordEndForwardOnce lands on the LAST byte of the current or next word.
func (lp *LogPanel) wordEndForwardOnce(start cursor, cls classFn) cursor {
	line, col := start.line, start.col
	total := len(lp.rawLines)
	s := strippedLine(lp, line)

	// Always step one char forward first so back-to-back `e` keeps moving.
	if col < len(s) {
		col++
	} else if line+1 < total {
		line++
		col = 0
	}
	for line < total {
		s = strippedLine(lp, line)
		// Skip whitespace.
		for col < len(s) && (s[col] == ' ' || s[col] == '\t') {
			col++
		}
		if col >= len(s) {
			if line+1 >= total {
				return cursor{line, maxInt(0, len(s)-1)}
			}
			line++
			col = 0
			continue
		}
		curClass := classOf(s[col], cls)
		for col+1 < len(s) && classOf(s[col+1], cls) == curClass {
			col++
		}
		return cursor{line, col}
	}
	return cursor{total - 1, maxInt(0, lineLen(lp, total-1)-1)}
}

// wordEndBackwardN retreats `start` by n end-of-word positions (vim `ge`).
func (lp *LogPanel) wordEndBackwardN(start cursor, cls classFn, n int) cursor {
	cur := start
	for i := 0; i < n; i++ {
		cur = lp.wordEndBackwardOnce(cur, cls)
	}
	return cur
}

// wordEndBackwardOnce lands on the end byte of the previous word.
func (lp *LogPanel) wordEndBackwardOnce(start cursor, cls classFn) cursor {
	line, col := start.line, start.col
	if col == 0 {
		if line == 0 {
			return cursor{0, 0}
		}
		line--
		col = lineLen(lp, line)
	}
	col--
	if col < 0 {
		col = 0
	}
	// Walk left skipping whitespace; cross lines.
	for {
		s := strippedLine(lp, line)
		for col >= 0 && col < len(s) && (s[col] == ' ' || s[col] == '\t') {
			col--
		}
		if col >= 0 && col < len(s) {
			_ = classOf(s[col], cls) // landed on end-of-word already
			return cursor{line, col}
		}
		if line == 0 {
			return cursor{0, 0}
		}
		line--
		col = lineLen(lp, line) - 1
	}
}

// rangeFind resolves an f/F/t/T motion on the CURRENT LINE only. Count
// determines the nth occurrence; missing matches return ok=false so the
// caller can no-op silently.
func (lp *LogPanel) rangeFind(start cursor, f findState, n int) (motionRange, bool) {
	if n < 1 {
		n = 1
	}
	s := strippedLine(lp, start.line)
	pos := start.col
	found := -1
	if f.forward {
		i := pos + 1
		for k := 0; k < n && i < len(s); {
			if s[i] == f.ch {
				found = i
				k++
				if k == n {
					break
				}
			}
			i++
		}
	} else {
		i := pos - 1
		for k := 0; k < n && i >= 0; {
			if s[i] == f.ch {
				found = i
				k++
				if k == n {
					break
				}
			}
			i--
		}
	}
	if found < 0 {
		return motionRange{}, false
	}
	if f.till {
		if f.forward {
			found--
		} else {
			found++
		}
	}
	if found < 0 || found > len(s) {
		return motionRange{}, false
	}
	return motionRange{start: start, end: cursor{start.line, found}, inclusive: f.forward}, true
}
