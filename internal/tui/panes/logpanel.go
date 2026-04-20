package panes

import (
	"regexp"
	"strings"

	"github.com/charmbracelet/bubbles/key"
	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/taynguyen/procs/internal/log"
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

// LogPanel wraps a bubbles viewport and handles filtered log content.
type LogPanel struct {
	vp       viewport.Model
	filter   *regexp.Regexp
	rawLines []string // rendered lines (DecodeForRender applied)
	sticky   bool
	dirty    bool
	width    int
	height   int
	lastGen  int64 // generation of last full render
}

// NewLogPanel initialises a LogPanel with sticky-bottom on.
func NewLogPanel(width, height int) LogPanel {
	innerW := max(1, width-2)
	innerH := max(1, height-2)
	vp := viewport.New(innerW, innerH)
	vp.MouseWheelEnabled = true
	return LogPanel{
		vp:     vp,
		sticky: true,
		width:  width,
		height: height,
	}
}

// SetSize resizes the viewport.
func (lp *LogPanel) SetSize(width, height int) {
	lp.width = width
	lp.height = height
	lp.vp.Width = max(1, width-2)
	lp.vp.Height = max(1, height-2)
	lp.dirty = true
}

// SetLines replaces all displayed lines (called on log tick with ring snapshot).
// Lines should already have log.DecodeForRender applied.
func (lp *LogPanel) SetLines(lines []string) {
	lp.rawLines = lines
	lp.dirty = true
	lp.applyFilter()
	if lp.sticky {
		lp.vp.GotoBottom()
	}
}

// SetFilter updates the filter regex. Pass nil to clear.
func (lp *LogPanel) SetFilter(rx *regexp.Regexp) {
	lp.filter = rx
	lp.dirty = true
	lp.applyFilter()
	if lp.sticky {
		lp.vp.GotoBottom()
	}
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

// View renders the log panel.
func (lp *LogPanel) View() string {
	content := lp.vp.View()
	return stylePanelBorder.Width(lp.width - 2).Height(lp.height - 2).Render(content)
}

// Sticky returns true if auto-scroll-to-bottom is enabled.
func (lp *LogPanel) Sticky() bool { return lp.sticky }

// applyFilter re-renders viewport content applying the current filter.
// Non-matching lines are hidden (default per spec).
func (lp *LogPanel) applyFilter() {
	if len(lp.rawLines) == 0 {
		lp.vp.SetContent("")
		return
	}

	if lp.filter == nil {
		lp.vp.SetContent(strings.Join(lp.rawLines, "\n"))
		return
	}

	var matched []string
	for _, line := range lp.rawLines {
		stripped := log.StripANSI(line)
		if lp.filter.MatchString(stripped) {
			matched = append(matched, line)
		}
	}

	if len(matched) == 0 {
		lp.vp.SetContent(lipgloss.NewStyle().Faint(true).Render("(no lines match filter)"))
		return
	}
	lp.vp.SetContent(strings.Join(matched, "\n"))
}

func max(a, b int) int {
	if a > b {
		return a
	}
	return b
}
