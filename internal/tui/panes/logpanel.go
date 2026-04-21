package panes

import (
	"fmt"
	"regexp"
	"strings"

	"github.com/charmbracelet/bubbles/key"
	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/lpwanw/randomshitgo-go/internal/log"
)

// logPanelKeyMap defines scrolling bindings for the log panel.
type logPanelKeyMap struct {
	PageUp   key.Binding
	PageDown key.Binding
	Top      key.Binding
	Bottom   key.Binding
	LineUp   key.Binding
	LineDown key.Binding
}

var logKeys = logPanelKeyMap{
	PageUp: key.NewBinding(key.WithKeys("pgup", "ctrl+b")),
	PageDown: key.NewBinding(key.WithKeys("pgdown", "ctrl+f")),
	Top:      key.NewBinding(key.WithKeys("g")),
	Bottom:   key.NewBinding(key.WithKeys("G")),
	LineUp:   key.NewBinding(key.WithKeys("K")),
	LineDown: key.NewBinding(key.WithKeys("J")),
}

var stylePanelBorder = lipgloss.NewStyle().
	Border(lipgloss.RoundedBorder()).
	BorderForeground(lipgloss.Color("238"))

// Scrollbar glyphs + styles. The rail is the empty track; the thumb marks the
// visible viewport window relative to the full log buffer.
const (
	scrollbarRailGlyph  = "│"
	scrollbarThumbGlyph = "█"
)

var (
	styleScrollRail  = lipgloss.NewStyle().Foreground(lipgloss.Color("238"))
	styleScrollThumb = lipgloss.NewStyle().Foreground(lipgloss.Color("62"))
	styleGutter      = lipgloss.NewStyle().Foreground(lipgloss.Color("240"))
)

// Highlight SGR codes. Two styles:
//   - hlOn / hlOff        : reverse-video for non-focused matches.
//   - hlCurOn / hlCurOff  : bright yellow bg + black fg for the CURRENT match
//                           (the one `n` / `N` just landed on).
// Both close sequences reset only the attributes they turned on, preserving
// any surrounding colour SGR embedded in the rendered line.
const (
	hlOn     = "\x1b[7m"
	hlOff    = "\x1b[27m"
	hlCurOn  = "\x1b[48;5;220m\x1b[30m"
	hlCurOff = "\x1b[39;49m"
)

// ansiSeqRe matches any ANSI CSI/OSC/escape sequence. Mirrors log.StripANSI's
// internal regex so highlightLine can step over escape bytes without mutating them.
var ansiSeqRe = regexp.MustCompile(`\x1b\[[0-9;?]*[ -/]*[@-~]|\x1b\][^\x07]*\x07|\x1b[@-_]`)

// LogPanel wraps a bubbles viewport and overlays filter highlights. Filter
// matches do NOT hide non-matching lines — matches are highlighted and the
// viewport can be jumped to each match via JumpNextMatch / JumpPrevMatch
// (vim-style `n` / `N`).
type LogPanel struct {
	vp         viewport.Model
	filter     *regexp.Regexp
	rawLines   []string // rendered lines (DecodeForRender applied)
	sticky     bool
	dirty      bool
	width      int
	height     int
	lastGen    int64 // generation of last full render
	matchLines []int // indices into rawLines that match the current filter
	matchCur   int   // current index into matchLines (-1 when none)
	gutterOn   bool    // render line-number gutter in Normal mode
	inCopy     bool    // copy mode active — forces gutter on
	cur        cursor  // copy-mode cursor position
	pending    pendingState
	op         opKind
	count      int
	pendingFind findState
	lastFind    findState
	sel        selection
	clip       clipboardWriter
	paused     bool // Space-toggle freeze: suppresses sticky auto-scroll
	severityOn bool // true = wrap ERROR/WARN/INFO lines with fg colour at render
	jsonOn     bool // true = expand single-line JSON into pretty multi-line
	sqlOn      bool // true = reformat SQL with keyword line breaks
	wrapOn     bool // true = hard-wrap each rendered row to viewport width
	originalLines []string // pre-pretty snapshot; rebuilt into rawLines on toggle
}

// NewLogPanel initialises a LogPanel with sticky-bottom on. The viewport is
// sized so the rightmost inner column is reserved for the scroll indicator
// rendered in View (width-2 for border, -1 more for the scrollbar column).
func NewLogPanel(width, height int) LogPanel {
	innerH := max(1, height-2)
	vp := viewport.New(max(1, width-3), innerH)
	vp.MouseWheelEnabled = true
	return LogPanel{
		vp:         vp,
		sticky:     true,
		width:      width,
		height:     height,
		matchCur:   -1,
		severityOn: true,
		wrapOn:     true,
	}
}

// SetWrap toggles hard-wrap at render time. Default on — truncation is
// strictly worse than wrap for any workflow.
func (lp *LogPanel) SetWrap(on bool) {
	if lp.wrapOn == on {
		return
	}
	lp.wrapOn = on
	lp.dirty = true
	lp.paintMatches()
}

// SetSeverity toggles the severity-colour render layer. Default is on.
func (lp *LogPanel) SetSeverity(on bool) {
	if lp.severityOn == on {
		return
	}
	lp.severityOn = on
	lp.dirty = true
	lp.paintMatches()
}

// SetSize resizes the viewport. The scrollbar reserves one inner column and
// the optional line-number gutter reserves `gutterWidth()` more, so the
// viewport's usable width is `width - 3 - gutterWidth()`.
func (lp *LogPanel) SetSize(width, height int) {
	lp.width = width
	lp.height = height
	lp.vp.Width = max(1, width-3-lp.gutterWidth())
	lp.vp.Height = max(1, height-2)
	lp.dirty = true
}

// SetGutter toggles the line-number gutter in Normal mode. Copy mode always
// forces the gutter on regardless of this flag.
func (lp *LogPanel) SetGutter(on bool) {
	if lp.gutterOn == on {
		return
	}
	lp.gutterOn = on
	lp.vp.Width = max(1, lp.width-3-lp.gutterWidth())
	lp.dirty = true
	lp.renderContent()
}

// SetCopyMode flips copy-mode on/off. Disables sticky auto-scroll on enter so
// the buffer doesn't jump out from under the cursor.
func (lp *LogPanel) SetCopyMode(on bool) {
	if lp.inCopy == on {
		return
	}
	lp.inCopy = on
	if on {
		lp.sticky = false
		// Start the cursor at the top of the currently visible window so users
		// don't need to scroll to find it.
		lp.cur = cursor{line: minInt(lp.vp.YOffset, maxInt(0, len(lp.rawLines)-1))}
		lp.clampCol()
	} else {
		lp.clearTransient()
		lp.lastFind = findState{}
		lp.sel = selection{}
	}
	lp.vp.Width = max(1, lp.width-3-lp.gutterWidth())
	lp.dirty = true
	lp.renderContent()
}

// InCopyMode reports whether the panel is currently in copy mode.
func (lp *LogPanel) InCopyMode() bool { return lp.inCopy }

// gutterWidth returns the column width reserved for the line-number gutter,
// or 0 when the gutter is not active. Includes the trailing ` │ ` separator.
func (lp *LogPanel) gutterWidth() int {
	if !lp.gutterOn && !lp.inCopy {
		return 0
	}
	return lineNumberWidth(len(lp.rawLines)) + 3
}

// lineNumberWidth returns the minimum decimal width needed to render the
// largest line index (1-based). Clamped to 1 so empty buffers still format.
func lineNumberWidth(total int) int {
	if total < 1 {
		return 1
	}
	w := 0
	for n := total; n > 0; n /= 10 {
		w++
	}
	return w
}

// SetLines replaces all displayed lines (called on log tick with ring snapshot).
// Lines should already have log.DecodeForRender applied.
func (lp *LogPanel) SetLines(lines []string) {
	prevGutter := lp.gutterWidth()
	lp.originalLines = lines
	lp.rebuildDisplayLines()
	// If gutter width shifts with buffer size (e.g. crossing 10 → 100 lines),
	// reclaim / release the matching viewport columns before rendering.
	if newGutter := lp.gutterWidth(); newGutter != prevGutter {
		lp.vp.Width = max(1, lp.width-3-newGutter)
	}
	lp.dirty = true
	lp.renderContent()
	if lp.sticky && !lp.paused {
		lp.vp.GotoBottom()
	}
}

// rebuildDisplayLines derives `rawLines` from `originalLines` by applying
// every active transform (SQL first, then JSON). Order matters: SQL pretty
// can produce fragments that the JSON detector would otherwise mis-grab.
func (lp *LogPanel) rebuildDisplayLines() {
	src := lp.originalLines
	if lp.sqlOn {
		src = flattenSQLLines(src)
	}
	if lp.jsonOn {
		src = flattenJSONLines(src)
	}
	lp.rawLines = src
}

// SetJSONPretty toggles single-line-JSON pretty-print. When on, each log line
// that parses as a JSON object or array is indented and expanded in place;
// cursor / filter / yank all run over the expanded buffer. Default off.
func (lp *LogPanel) SetJSONPretty(on bool) {
	if lp.jsonOn == on {
		return
	}
	lp.jsonOn = on
	lp.rebuildDisplayLines()
	lp.dirty = true
	lp.renderContent()
}

// SetSQLPretty toggles keyword-aware SQL formatting. Default off.
func (lp *LogPanel) SetSQLPretty(on bool) {
	if lp.sqlOn == on {
		return
	}
	lp.sqlOn = on
	lp.rebuildDisplayLines()
	lp.dirty = true
	lp.renderContent()
}

// VisibleLines returns a defensive copy of the rendered log buffer — used by
// `:w {path}` to dump exactly what the user sees (post-filter / post-pretty
// once Phase 03 lands).
func (lp *LogPanel) VisibleLines() []string {
	out := make([]string, len(lp.rawLines))
	copy(out, lp.rawLines)
	return out
}

// Paused reports whether Space-pause is active. When paused, new log ticks
// still refresh rawLines but the viewport is pinned wherever the user left it.
func (lp *LogPanel) Paused() bool { return lp.paused }

// TogglePaused flips the pause flag. No repaint needed — the next log tick
// (or user scroll) picks up the new behavior.
func (lp *LogPanel) TogglePaused() { lp.paused = !lp.paused }

// SetFilter updates the filter regex. Pass nil to clear. Rebuilds match index.
func (lp *LogPanel) SetFilter(rx *regexp.Regexp) {
	lp.filter = rx
	lp.dirty = true
	lp.renderContent()
}

// Update routes key messages to viewport scroll operations.
func (lp *LogPanel) Update(msg tea.Msg) tea.Cmd {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch {
		case key.Matches(msg, logKeys.PageUp):
			lp.vp.PageUp()
			lp.sticky = false
		case key.Matches(msg, logKeys.PageDown):
			lp.vp.PageDown()
			if lp.vp.AtBottom() {
				lp.sticky = true
			}
		case key.Matches(msg, logKeys.Top):
			lp.vp.GotoTop()
			lp.sticky = false
		case key.Matches(msg, logKeys.Bottom):
			lp.vp.GotoBottom()
			lp.sticky = true
		case key.Matches(msg, logKeys.LineUp):
			lp.vp.ScrollUp(5)
			lp.sticky = false
		case key.Matches(msg, logKeys.LineDown):
			lp.vp.ScrollDown(5)
			if lp.vp.AtBottom() {
				lp.sticky = true
			}
		}

	case tea.MouseMsg:
		var cmd tea.Cmd
		lp.vp, cmd = lp.vp.Update(msg)
		if !lp.vp.AtBottom() {
			lp.sticky = false
		}
		return cmd
	}

	return nil
}

// View renders the log panel with an inline scroll indicator on the right
// edge. The indicator is a 1-column vertical track inside the border: a thumb
// marks the visible viewport window against the total number of log lines.
// When content fits entirely the column is blank so the log width stays stable.
func (lp *LogPanel) View() string {
	innerH := max(1, lp.height-2)
	vpW := max(1, lp.width-3)

	vpLines := strings.Split(lp.vp.View(), "\n")
	// Pad to innerH so scrollbar rows always have a partner line.
	for len(vpLines) < innerH {
		vpLines = append(vpLines, strings.Repeat(" ", vpW))
	}
	vpLines = vpLines[:innerH]

	bar := scrollbarColumn(innerH, len(lp.rawLines), lp.vp.YOffset)
	rows := make([]string, innerH)
	for i := 0; i < innerH; i++ {
		rows[i] = vpLines[i] + bar[i]
	}
	content := strings.Join(rows, "\n")
	return stylePanelBorder.Width(lp.width - 2).Height(lp.height - 2).Render(content)
}

// scrollbarColumn returns innerH pre-styled one-rune strings forming a vertical
// scroll indicator. When total ≤ innerH the column is blank so the surrounding
// layout width stays stable without drawing visual noise.
func scrollbarColumn(innerH, total, offset int) []string {
	rows := make([]string, innerH)
	if innerH <= 0 {
		return rows
	}
	if total <= innerH {
		blank := " "
		for i := range rows {
			rows[i] = blank
		}
		return rows
	}
	thumb := innerH * innerH / total
	if thumb < 1 {
		thumb = 1
	}
	if thumb > innerH {
		thumb = innerH
	}
	denom := total - innerH
	if denom < 1 {
		denom = 1
	}
	start := offset * (innerH - thumb) / denom
	if start < 0 {
		start = 0
	}
	if start > innerH-thumb {
		start = innerH - thumb
	}
	rail := styleScrollRail.Render(scrollbarRailGlyph)
	thumbCh := styleScrollThumb.Render(scrollbarThumbGlyph)
	for i := range rows {
		if i >= start && i < start+thumb {
			rows[i] = thumbCh
		} else {
			rows[i] = rail
		}
	}
	return rows
}

// Sticky returns true if auto-scroll-to-bottom is enabled.
func (lp *LogPanel) Sticky() bool { return lp.sticky }

// MatchCount returns the number of lines matching the current filter.
func (lp *LogPanel) MatchCount() int { return len(lp.matchLines) }

// MatchCursor returns the 1-based position of the current jump target, or 0
// when no match has been selected yet (or there are no matches).
func (lp *LogPanel) MatchCursor() int {
	if lp.matchCur < 0 || len(lp.matchLines) == 0 {
		return 0
	}
	return lp.matchCur + 1
}

// JumpNextMatchCursor behaves like JumpNextMatch but also parks the copy-mode
// cursor on the matched line (col 0) when the panel has focus. Returns the
// same (ok, wrapped) contract.
func (lp *LogPanel) JumpNextMatchCursor() (ok, wrapped bool) {
	ok, wrapped = lp.JumpNextMatch()
	if ok && lp.inCopy && lp.matchCur >= 0 && lp.matchCur < len(lp.matchLines) {
		lp.cur.line = lp.matchLines[lp.matchCur]
		lp.cur.col = 0
		lp.ensureCursorVisible()
		lp.paintMatches()
	}
	return
}

// JumpPrevMatchCursor is the backwards twin of JumpNextMatchCursor.
func (lp *LogPanel) JumpPrevMatchCursor() (ok, wrapped bool) {
	ok, wrapped = lp.JumpPrevMatch()
	if ok && lp.inCopy && lp.matchCur >= 0 && lp.matchCur < len(lp.matchLines) {
		lp.cur.line = lp.matchLines[lp.matchCur]
		lp.cur.col = 0
		lp.ensureCursorVisible()
		lp.paintMatches()
	}
	return
}

// JumpNextMatch scrolls the viewport to the next matching line. Returns
// (ok, wrapped): ok=false when there are no matches; wrapped=true when the
// cursor looped from the last match back to the first.
func (lp *LogPanel) JumpNextMatch() (ok bool, wrapped bool) {
	if len(lp.matchLines) == 0 {
		return false, false
	}
	next := lp.matchCur + 1
	if next >= len(lp.matchLines) {
		next = 0
		wrapped = lp.matchCur >= 0
	}
	lp.matchCur = next
	lp.scrollToMatch()
	return true, wrapped
}

// JumpPrevMatch scrolls the viewport to the previous matching line. Returns
// (ok, wrapped). From a fresh state (no prior match selected) the first press
// lands on the FIRST match (like vim's `N` before any `/` search on the
// current line) and is NOT considered a wrap; subsequent presses that roll
// past index 0 wrap to the last match.
func (lp *LogPanel) JumpPrevMatch() (ok bool, wrapped bool) {
	if len(lp.matchLines) == 0 {
		return false, false
	}
	prev := lp.matchCur - 1
	if prev < 0 {
		if lp.matchCur < 0 {
			prev = 0
		} else {
			prev = len(lp.matchLines) - 1
			wrapped = true
		}
	}
	lp.matchCur = prev
	lp.scrollToMatch()
	return true, wrapped
}

// renderContent rebuilds viewport content + match index. All raw lines are
// preserved regardless of filter; matches are highlighted inline. Callers that
// only change matchCur (e.g. Jump*Match) should invoke paintMatches instead to
// avoid recomputing the match index.
func (lp *LogPanel) renderContent() {
	lp.matchLines = lp.matchLines[:0]
	lp.matchCur = -1

	if len(lp.rawLines) == 0 {
		lp.vp.SetContent("")
		return
	}

	if lp.filter != nil {
		for i, line := range lp.rawLines {
			stripped := log.StripANSI(line)
			if lp.filter.MatchString(stripped) {
				lp.matchLines = append(lp.matchLines, i)
			}
		}
	}
	lp.paintMatches()
}

// paintMatches re-renders viewport content using the current matchLines and
// matchCur — the focused match gets the distinct hlCur* style.
func (lp *LogPanel) paintMatches() {
	if len(lp.rawLines) == 0 {
		lp.vp.SetContent("")
		return
	}
	currentLine := -1
	if lp.matchCur >= 0 && lp.matchCur < len(lp.matchLines) {
		currentLine = lp.matchLines[lp.matchCur]
	}
	// Build a set-lookup over match indices for O(1) per-line check.
	isMatch := make(map[int]struct{}, len(lp.matchLines))
	for _, idx := range lp.matchLines {
		isMatch[idx] = struct{}{}
	}

	out := make([]string, len(lp.rawLines))
	for i, line := range lp.rawLines {
		if _, ok := isMatch[i]; ok && lp.filter != nil {
			out[i] = highlightLine(line, lp.filter, i == currentLine)
			continue
		}
		out[i] = line
	}
	// Severity colour is a whole-line fg wrap — applied before selection /
	// cursor so those reverse-video overlays sit on top cleanly.
	if lp.severityOn {
		for i := range out {
			out[i] = applySeverity(out[i])
		}
	}
	if lp.inCopy {
		lp.overlaySelection(out)
		out[lp.cur.line] = overlayCursor(out[lp.cur.line], lp.cur.col)
	}

	gw := lp.gutterWidth()
	contentWidth := lp.vp.Width - gw
	if contentWidth < 1 {
		contentWidth = 1
	}

	// Expand each logical line into one or more visual rows when wrap is on.
	// `origIdx[k] = i` means visual row k came from rawLines[i]; the first row
	// for a given i gets the line-number gutter, continuation rows get a blank
	// gutter of the same width so alignment stays stable.
	type visualRow struct {
		line    string
		origIdx int
		first   bool
	}
	rows := make([]visualRow, 0, len(out))
	for i, line := range out {
		if lp.wrapOn && contentWidth > 0 {
			pieces := wrapLine(line, contentWidth)
			for j, p := range pieces {
				rows = append(rows, visualRow{line: p, origIdx: i, first: j == 0})
			}
			continue
		}
		rows = append(rows, visualRow{line: line, origIdx: i, first: true})
	}

	flat := make([]string, len(rows))
	if gw > 0 {
		numW := gw - 3
		blankLabel := styleGutter.Render(strings.Repeat(" ", numW) + " │ ")
		for k, r := range rows {
			if r.first {
				flat[k] = styleGutter.Render(fmt.Sprintf("%*d │ ", numW, r.origIdx+1)) + r.line
			} else {
				flat[k] = blankLabel + r.line
			}
		}
	} else {
		for k, r := range rows {
			flat[k] = r.line
		}
	}
	lp.vp.SetContent(strings.Join(flat, "\n"))
}

// overlayCursor wraps one cell at the cursor's stripped column with
// reverse-video SGR so the active position stands out. When the cursor sits
// past the last character a synthetic space cell is appended so there's
// always something to invert.
func overlayCursor(rendered string, strippedCol int) string {
	mapping := mapStrippedToRendered(rendered)
	strippedLen := len(mapping) - 1
	if strippedCol > strippedLen {
		strippedCol = strippedLen
	}
	if strippedCol < 0 {
		strippedCol = 0
	}
	// Past-EOL cursor → append a reverse-styled space so users can tell where
	// `$` landed on blank / short lines.
	if strippedCol == strippedLen {
		return rendered + "\x1b[7m \x1b[27m"
	}
	start := mapping[strippedCol]
	end := mapping[strippedCol+1]
	return rendered[:start] + "\x1b[7m" + rendered[start:end] + "\x1b[27m" + rendered[end:]
}

// mapStrippedToRendered returns a slice M where M[i] is the byte offset in
// `rendered` corresponding to stripped-byte index i. M has length
// len(strip)+1 so M[len(strip)] == len(rendered). ANSI SGR sequences are
// skipped; the mapping lands on the visible byte they precede.
func mapStrippedToRendered(rendered string) []int {
	stripped := log.StripANSI(rendered)
	out := make([]int, len(stripped)+1)
	si := 0
	ri := 0
	for ri < len(rendered) {
		loc := ansiSeqRe.FindStringIndex(rendered[ri:])
		if loc != nil && loc[0] == 0 {
			ri += loc[1]
			continue
		}
		out[si] = ri
		si++
		ri++
	}
	out[si] = len(rendered)
	if si != len(stripped) {
		// Mapping drift — fall back to identity to avoid index panics.
		for i := range out {
			if i < len(rendered) {
				out[i] = i
			} else {
				out[i] = len(rendered)
			}
		}
	}
	return out
}

// scrollToMatch positions the viewport so the current match line is visible
// and repaints content so the focused match gets the distinct highlight.
func (lp *LogPanel) scrollToMatch() {
	if lp.matchCur < 0 || lp.matchCur >= len(lp.matchLines) {
		return
	}
	lp.paintMatches()
	target := lp.matchLines[lp.matchCur]
	// Centre the match roughly one third down the viewport.
	offset := target - lp.vp.Height/3
	if offset < 0 {
		offset = 0
	}
	lp.vp.SetYOffset(offset)
	lp.sticky = false
}

// highlightLine injects highlight SGR around regex matches on rendered.
// When current=true, the stronger hlCur* style is used so the focused match
// line stands out from the other highlighted matches. Matching is performed
// on the ANSI-stripped form; highlight boundaries are mapped back to byte
// offsets in the rendered line via a visible-byte walk that skips over
// embedded escape sequences unchanged.
func highlightLine(rendered string, rx *regexp.Regexp, current bool) string {
	if rx == nil || rendered == "" {
		return rendered
	}
	on, off := hlOn, hlOff
	if current {
		on, off = hlCurOn, hlCurOff
	}
	stripped := log.StripANSI(rendered)
	matches := rx.FindAllStringIndex(stripped, -1)
	if len(matches) == 0 {
		return rendered
	}

	// Build lookup: rendered byte offset for each stripped byte offset 0..len.
	renderedOfStripped := make([]int, len(stripped)+1)
	si := 0
	ri := 0
	for ri < len(rendered) {
		loc := ansiSeqRe.FindStringIndex(rendered[ri:])
		if loc != nil && loc[0] == 0 {
			ri += loc[1]
			continue
		}
		renderedOfStripped[si] = ri
		si++
		ri++
	}
	renderedOfStripped[si] = len(rendered)
	if si != len(stripped) {
		// Mapping drift (shouldn't happen with well-formed ANSI); bail safely.
		return rendered
	}

	var sb strings.Builder
	sb.Grow(len(rendered) + len(matches)*8)
	cursor := 0
	for _, m := range matches {
		s, e := renderedOfStripped[m[0]], renderedOfStripped[m[1]]
		if s < cursor {
			continue
		}
		sb.WriteString(rendered[cursor:s])
		sb.WriteString(on)
		sb.WriteString(rendered[s:e])
		sb.WriteString(off)
		cursor = e
	}
	sb.WriteString(rendered[cursor:])
	return sb.String()
}

func max(a, b int) int {
	if a > b {
		return a
	}
	return b
}
