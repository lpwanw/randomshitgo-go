package panes

import (
	"fmt"
	"strings"

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

// opKind is the set of vim operators supported. Only yank — logs are
// read-only so d/c/r/s are intentionally absent.
type opKind int

const (
	opNone opKind = iota
	opYank
)

// pendingState tracks what the dispatcher is waiting for after a leader key.
type pendingState int

const (
	pendNone pendingState = iota
	pendG                  // after `g` (second `g` = top, `e`/`E` = ge/gE)
	pendFind               // after f/F/t/T — next key is the target char
	pendOpYankI            // after `yi` — next key is the text-object char
	pendOpYankA            // after `ya` — next key is the text-object char
)

// findState stores an in-flight or last-completed f/F/t/T. `;` and `,` replay
// this with direction optionally inverted.
type findState struct {
	ch      byte
	forward bool
	till    bool // true = t/T (stop one char short)
	active  bool
}

// motionRange describes what a resolved motion or text-object covers. `end`
// is where the cursor lands when the motion is used standalone; when fed to
// an operator, [start, end] (adjusted by `inclusive`) is the yank payload.
type motionRange struct {
	start     cursor
	end       cursor
	linewise  bool
	inclusive bool
}

// classFn classifies a byte for word-motion purposes.
type classFn func(byte) bool

// isWord returns true for vim's word class: [A-Za-z0-9_].
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

// isWORD returns true for vim's WORD class: any non-whitespace byte.
func isWORD(b byte) bool { return b != ' ' && b != '\t' }

// classOf returns a small integer tag for a byte so wordForwardOnce can detect
// class transitions without bool gymnastics. 0 = whitespace, 1 = class(b),
// 2 = punctuation (non-class, non-whitespace).
func classOf(b byte, cls classFn) int {
	if b == ' ' || b == '\t' {
		return 0
	}
	if cls(b) {
		return 1
	}
	return 2
}

// HandleCopyKey is the copy-mode key dispatcher. It runs a small state
// machine over counts, operators (`y`), text-object prefixes (`i`/`a`), and
// motions — aiming for close vim parity on the read-only-log subset.
func (lp *LogPanel) HandleCopyKey(msg tea.KeyMsg) tea.Cmd {
	if !lp.inCopy {
		return nil
	}
	if len(lp.rawLines) == 0 {
		return nil
	}

	// Visual-mode keys (v/V/y-in-visual/Y) short-circuit here.
	if consumed, cmd := lp.handleVisual(msg); consumed {
		return cmd
	}

	s := msg.String()

	// Pending states consume the next key with priority.
	switch lp.pending {
	case pendG:
		return lp.handlePendG(s)
	case pendFind:
		return lp.handlePendFind(s)
	case pendOpYankI:
		return lp.handlePendTextObject(s, false)
	case pendOpYankA:
		return lp.handlePendTextObject(s, true)
	}

	// Count prefix: `1`..`9` always accumulate; `0` only accumulates when a
	// count is already building (else it's the line-start motion below).
	if len(s) == 1 {
		b := s[0]
		if b >= '1' && b <= '9' {
			lp.count = lp.count*10 + int(b-'0')
			return nil
		}
		if b == '0' && lp.count > 0 {
			lp.count *= 10
			return nil
		}
	}

	return lp.dispatchKey(s)
}

// Cursor returns the current copy-mode cursor (line, col). Both 0-based.
func (lp *LogPanel) Cursor() (line, col int) { return lp.cur.line, lp.cur.col }

// ClearPending wipes count/operator/pending-state and returns true when
// anything was actually cleared. The tui layer calls this on Esc before
// running the focus-exit arm ladder — matches vim's "Esc cancels pending".
func (lp *LogPanel) ClearPending() bool {
	if lp.pending == pendNone && lp.op == opNone && lp.count == 0 {
		return false
	}
	lp.clearTransient()
	return true
}

// PendingSummary renders a compact "command buffer" string — e.g. `3yi` —
// for the status bar. Empty when idle.
func (lp *LogPanel) PendingSummary() string {
	if lp.count == 0 && lp.op == opNone && lp.pending == pendNone {
		return ""
	}
	var b strings.Builder
	if lp.count > 0 {
		fmt.Fprintf(&b, "%d", lp.count)
	}
	if lp.op == opYank {
		b.WriteByte('y')
	}
	switch lp.pending {
	case pendG:
		b.WriteByte('g')
	case pendOpYankI:
		b.WriteByte('i')
	case pendOpYankA:
		b.WriteByte('a')
	case pendFind:
		switch {
		case lp.pendingFind.till && lp.pendingFind.forward:
			b.WriteByte('t')
		case lp.pendingFind.till:
			b.WriteByte('T')
		case lp.pendingFind.forward:
			b.WriteByte('f')
		default:
			b.WriteByte('F')
		}
	}
	return b.String()
}

// clearTransient wipes every per-command field so the SM is ready for the
// next input. Call after a motion resolves, an operator applies, or Esc.
func (lp *LogPanel) clearTransient() {
	lp.pending = pendNone
	lp.op = opNone
	lp.count = 0
	lp.pendingFind = findState{}
}

// effectiveCount returns count clamped to ≥ 1 so motions like "w" default to
// one repetition when no explicit digit prefix was typed.
func (lp *LogPanel) effectiveCount() int {
	if lp.count < 1 {
		return 1
	}
	return lp.count
}

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

// ensureCursorVisible scrolls the viewport so cursor.line sits in the visible
// window. No-op when already in range.
func (lp *LogPanel) ensureCursorVisible() {
	if lp.cur.line < lp.vp.YOffset {
		lp.vp.SetYOffset(lp.cur.line)
		return
	}
	if lp.cur.line >= lp.vp.YOffset+lp.vp.Height {
		lp.vp.SetYOffset(lp.cur.line - lp.vp.Height + 1)
	}
}

// dispatchKey routes a fully resolved (non-pending, non-count-digit) key to
// the right handler: operator start, find, search-repeat, or motion. Returns
// the tea.Cmd from any yank that fires.
func (lp *LogPanel) dispatchKey(s string) tea.Cmd {
	n := lp.effectiveCount()

	// Operator start / second-char.
	switch s {
	case "y":
		if lp.op == opYank {
			// `yy` — linewise current line × count.
			return lp.applyRange(lineRange(lp.cur.line, n, len(lp.rawLines)))
		}
		lp.op = opYank
		return nil
	case "i":
		if lp.op == opYank {
			lp.pending = pendOpYankI
			return nil
		}
	case "a":
		if lp.op == opYank {
			lp.pending = pendOpYankA
			return nil
		}
	case "f":
		lp.pending = pendFind
		lp.pendingFind = findState{forward: true, till: false}
		return nil
	case "F":
		lp.pending = pendFind
		lp.pendingFind = findState{forward: false, till: false}
		return nil
	case "t":
		lp.pending = pendFind
		lp.pendingFind = findState{forward: true, till: true}
		return nil
	case "T":
		lp.pending = pendFind
		lp.pendingFind = findState{forward: false, till: true}
		return nil
	case ";":
		if lp.lastFind.active {
			if r, ok := lp.rangeFind(lp.cur, lp.lastFind, n); ok {
				return lp.applyRange(r)
			}
		}
		lp.clearTransient()
		return nil
	case ",":
		if lp.lastFind.active {
			inv := lp.lastFind
			inv.forward = !inv.forward
			if r, ok := lp.rangeFind(lp.cur, inv, n); ok {
				return lp.applyRange(r)
			}
		}
		lp.clearTransient()
		return nil
	}

	r, ok := lp.resolveMotion(s, n)
	if !ok {
		if lp.pending != pendNone {
			// motion set a new pending state (e.g. `g`) — wait for next key.
			return nil
		}
		lp.clearTransient()
		return nil
	}
	return lp.applyRange(r)
}

// applyRange either moves the cursor (no operator pending) or feeds the
// range to the yank operator and clears transient state.
func (lp *LogPanel) applyRange(r motionRange) tea.Cmd {
	if lp.op == opYank {
		text := lp.extractRange(r)
		lp.clearTransient()
		if text == "" {
			return nil
		}
		return lp.yankCmd(text)
	}
	lp.cur = r.end
	lp.clampCol()
	lp.ensureCursorVisible()
	lp.paintMatches()
	lp.clearTransient()
	return nil
}

// extractRange pulls stripped text for the given motion range. For operator
// application only — rendering overlays use selection state.
func (lp *LogPanel) extractRange(r motionRange) string {
	if r.linewise {
		sl, el := r.start.line, r.end.line
		if sl > el {
			sl, el = el, sl
		}
		if sl < 0 {
			sl = 0
		}
		if el >= len(lp.rawLines) {
			el = len(lp.rawLines) - 1
		}
		parts := make([]string, 0, el-sl+1)
		for i := sl; i <= el; i++ {
			parts = append(parts, strippedLine(lp, i))
		}
		return strings.Join(parts, "\n")
	}
	sl, sc, el, ec := normaliseCharRange(r.start, r.end)
	if r.inclusive {
		ec++
	}
	return sliceChar(lp, sl, sc, el, ec)
}

// normaliseCharRange returns (sl, sc, el, ec) with (sl,sc) ≤ (el,ec) in
// line-major order.
func normaliseCharRange(a, b cursor) (int, int, int, int) {
	if a.line < b.line || (a.line == b.line && a.col <= b.col) {
		return a.line, a.col, b.line, b.col
	}
	return b.line, b.col, a.line, a.col
}

// sliceChar returns the stripped text between (sl,sc) and (el,ec), half-open
// on ec.
func sliceChar(lp *LogPanel, sl, sc, el, ec int) string {
	if sl == el {
		s := strippedLine(lp, sl)
		if sc < 0 {
			sc = 0
		}
		if ec > len(s) {
			ec = len(s)
		}
		if sc > ec {
			sc = ec
		}
		return s[sc:ec]
	}
	first := strippedLine(lp, sl)
	last := strippedLine(lp, el)
	if sc < 0 {
		sc = 0
	}
	if sc > len(first) {
		sc = len(first)
	}
	if ec > len(last) {
		ec = len(last)
	}
	parts := []string{first[sc:]}
	for i := sl + 1; i < el; i++ {
		parts = append(parts, strippedLine(lp, i))
	}
	parts = append(parts, last[:ec])
	return strings.Join(parts, "\n")
}

// lineRange builds a linewise motionRange spanning `count` lines starting at
// `start` (clamped to the buffer).
func lineRange(start, count, total int) motionRange {
	if count < 1 {
		count = 1
	}
	end := start + count - 1
	if end >= total {
		end = total - 1
	}
	return motionRange{
		start:    cursor{start, 0},
		end:      cursor{end, 0},
		linewise: true,
	}
}

// handlePendG runs after `g` has been typed once. `gg` jumps to the top;
// `ge`/`gE` step to the end of the previous (WORD-)word; anything else
// clears pending.
func (lp *LogPanel) handlePendG(s string) tea.Cmd {
	lp.pending = pendNone
	n := lp.effectiveCount()
	switch s {
	case "g":
		return lp.applyRange(motionRange{start: lp.cur, end: cursor{0, 0}})
	case "e":
		return lp.applyRange(motionRange{start: lp.cur, end: lp.wordEndBackwardN(lp.cur, isWord, n), inclusive: true})
	case "E":
		return lp.applyRange(motionRange{start: lp.cur, end: lp.wordEndBackwardN(lp.cur, isWORD, n), inclusive: true})
	}
	lp.clearTransient()
	return nil
}

// handlePendFind runs after `f/F/t/T` has set the direction; the next key is
// the target byte. Non-ASCII (> 127) is rejected with a silent drop.
func (lp *LogPanel) handlePendFind(s string) tea.Cmd {
	lp.pending = pendNone
	if len(s) != 1 || s[0] > 127 {
		lp.clearTransient()
		return nil
	}
	n := lp.effectiveCount()
	lp.pendingFind.ch = s[0]
	lp.pendingFind.active = true
	lp.lastFind = lp.pendingFind
	r, ok := lp.rangeFind(lp.cur, lp.pendingFind, n)
	if !ok {
		lp.clearTransient()
		return nil
	}
	return lp.applyRange(r)
}

// handlePendTextObject runs after `yi{X}` or `ya{X}`. Dispatches to the
// appropriate object resolver, or drops silently when X is unsupported.
func (lp *LogPanel) handlePendTextObject(s string, around bool) tea.Cmd {
	lp.pending = pendNone
	if len(s) != 1 {
		lp.clearTransient()
		return nil
	}
	r, ok := lp.rangeTextObject(lp.cur, s[0], around)
	if !ok {
		lp.clearTransient()
		return nil
	}
	return lp.applyRange(r)
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
