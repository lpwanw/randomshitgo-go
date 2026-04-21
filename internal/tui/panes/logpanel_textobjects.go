package panes

// rangeTextObject dispatches an `{i,a}{char}` text object to the appropriate
// resolver, scoped to the cursor's current line (multi-line objects are
// intentionally out of scope — logs are usually independent lines).
func (lp *LogPanel) rangeTextObject(at cursor, ch byte, around bool) (motionRange, bool) {
	line := strippedLine(lp, at.line)
	var sCol, eCol int
	var ok bool
	switch ch {
	case 'w':
		sCol, eCol, ok = tobjWordLike(line, at.col, isWord, around)
	case 'W':
		sCol, eCol, ok = tobjWordLike(line, at.col, isWORD, around)
	case '"', '\'', '`':
		sCol, eCol, ok = tobjQuote(line, at.col, ch, around)
	case '(', ')', 'b':
		sCol, eCol, ok = tobjBracket(line, at.col, '(', ')', around)
	case '[', ']':
		sCol, eCol, ok = tobjBracket(line, at.col, '[', ']', around)
	case '{', '}', 'B':
		sCol, eCol, ok = tobjBracket(line, at.col, '{', '}', around)
	case '<', '>':
		sCol, eCol, ok = tobjBracket(line, at.col, '<', '>', around)
	default:
		return motionRange{}, false
	}
	if !ok || sCol >= eCol {
		return motionRange{}, false
	}
	// The range is half-open on eCol; encode as inclusive=false so extractRange
	// slices [sCol, eCol) as-is.
	return motionRange{
		start: cursor{at.line, sCol},
		end:   cursor{at.line, eCol},
	}, true
}

// tobjWordLike computes the iw/aw (or iW/aW) range for cursor on `line`.
// Returns byte offsets [sCol, eCol). `around` adds trailing whitespace when
// present, else leading whitespace (vim rule).
func tobjWordLike(line string, col int, cls classFn, around bool) (int, int, bool) {
	if len(line) == 0 {
		return 0, 0, false
	}
	if col >= len(line) {
		col = len(line) - 1
	}
	if col < 0 {
		col = 0
	}
	class := classOf(line[col], cls)
	if class == 0 {
		// On whitespace: iw selects the whitespace run; aw extends into the
		// next word.
		s, e := col, col
		for s > 0 && classOf(line[s-1], cls) == 0 {
			s--
		}
		for e+1 < len(line) && classOf(line[e+1], cls) == 0 {
			e++
		}
		end := e + 1
		if around {
			for end < len(line) && classOf(line[end], cls) != 0 {
				end++
			}
		}
		return s, end, true
	}
	s, e := col, col
	for s > 0 && classOf(line[s-1], cls) == class {
		s--
	}
	for e+1 < len(line) && classOf(line[e+1], cls) == class {
		e++
	}
	end := e + 1
	if around {
		orig := end
		for end < len(line) && (line[end] == ' ' || line[end] == '\t') {
			end++
		}
		if end == orig {
			for s > 0 && (line[s-1] == ' ' || line[s-1] == '\t') {
				s--
			}
		}
	}
	return s, end, true
}

// tobjQuote picks the quote pair that contains the cursor. Quotes are paired
// greedily left-to-right (0,1),(2,3),… — matches vim's heuristic for a
// single-line `i"`.
func tobjQuote(line string, col int, q byte, around bool) (int, int, bool) {
	positions := []int{}
	for i := 0; i < len(line); i++ {
		if line[i] == q {
			positions = append(positions, i)
		}
	}
	if len(positions) < 2 {
		return 0, 0, false
	}
	for i := 0; i+1 < len(positions); i += 2 {
		open, closeIdx := positions[i], positions[i+1]
		if col >= open && col <= closeIdx {
			if around {
				end := closeIdx + 1
				for end < len(line) && (line[end] == ' ' || line[end] == '\t') {
					end++
				}
				return open, end, true
			}
			return open + 1, closeIdx, true
		}
	}
	// Cursor outside any pair — fall back to first pair if cursor is before it.
	if col < positions[0] {
		open, closeIdx := positions[0], positions[1]
		if around {
			return open, closeIdx + 1, true
		}
		return open + 1, closeIdx, true
	}
	return 0, 0, false
}

// tobjBracket stack-scans a line for balanced open/close pairs, then returns
// the innermost pair enclosing the cursor column.
func tobjBracket(line string, col int, openCh, closeCh byte, around bool) (int, int, bool) {
	type pairT struct{ s, e int }
	stack := []int{}
	pairs := []pairT{}
	for i := 0; i < len(line); i++ {
		switch line[i] {
		case openCh:
			if openCh == closeCh {
				// same char — treated as quote; should not hit here
				continue
			}
			stack = append(stack, i)
		case closeCh:
			if len(stack) == 0 {
				continue
			}
			s := stack[len(stack)-1]
			stack = stack[:len(stack)-1]
			pairs = append(pairs, pairT{s, i})
		}
	}
	best := pairT{-1, -1}
	for _, p := range pairs {
		if col >= p.s && col <= p.e {
			// innermost = smallest span that encloses col
			if best.s == -1 || (p.s >= best.s && p.e <= best.e) {
				best = p
			}
		}
	}
	if best.s == -1 {
		return 0, 0, false
	}
	if around {
		return best.s, best.e + 1, true
	}
	return best.s + 1, best.e, true
}
