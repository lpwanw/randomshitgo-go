package panes

import (
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
}

// NewLogPanel initialises a LogPanel with sticky-bottom on. The viewport is
// sized so the rightmost inner column is reserved for the scroll indicator
// rendered in View (width-2 for border, -1 more for the scrollbar column).
func NewLogPanel(width, height int) LogPanel {
	innerW := max(1, width-3)
	innerH := max(1, height-2)
	vp := viewport.New(innerW, innerH)
	vp.MouseWheelEnabled = true
	return LogPanel{
		vp:       vp,
		sticky:   true,
		width:    width,
		height:   height,
		matchCur: -1,
	}
}

// SetSize resizes the viewport. The scrollbar reserves one inner column, so
// the viewport's usable width is `width-3` (2 border cells + 1 scrollbar cell).
func (lp *LogPanel) SetSize(width, height int) {
	lp.width = width
	lp.height = height
	lp.vp.Width = max(1, width-3)
	lp.vp.Height = max(1, height-2)
	lp.dirty = true
}

// SetLines replaces all displayed lines (called on log tick with ring snapshot).
// Lines should already have log.DecodeForRender applied.
func (lp *LogPanel) SetLines(lines []string) {
	lp.rawLines = lines
	lp.dirty = true
	lp.renderContent()
	if lp.sticky {
		lp.vp.GotoBottom()
	}
}

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
	lp.vp.SetContent(strings.Join(out, "\n"))
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
