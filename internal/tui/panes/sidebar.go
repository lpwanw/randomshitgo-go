package panes

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/lipgloss"
	"github.com/taynguyen/procs/internal/state"
)

// Row holds display data for a single sidebar entry.
type Row struct {
	ID       string
	State    string
	Attempts int
}

// Status glyph constants — unicode symbols matching the TS source.
const (
	glyphRunning    = "●"
	glyphIdle       = "◯"
	glyphStarting   = "…"
	glyphStopping   = "…"
	glyphCrashed    = "✕"
	glyphRestarting = "↻"
	glyphGivingUp   = "⛔"
)

// Sidebar styles — package-level vars so lipgloss only allocates once.
var (
	styleSelected = lipgloss.NewStyle().
			Bold(true).
			Background(lipgloss.Color("62")).
			Foreground(lipgloss.Color("255"))

	styleNormal = lipgloss.NewStyle().
			Foreground(lipgloss.Color("252"))

	styleGlyphRunning = lipgloss.NewStyle().Foreground(lipgloss.Color("40"))  // green
	styleGlyphIdle    = lipgloss.NewStyle().Foreground(lipgloss.Color("240")) // grey
	styleGlyphStart   = lipgloss.NewStyle().Foreground(lipgloss.Color("220")) // yellow
	styleGlyphCrash   = lipgloss.NewStyle().Foreground(lipgloss.Color("196")) // red
	styleGlyphRestart = lipgloss.NewStyle().Foreground(lipgloss.Color("51"))  // cyan
	styleGlyphGiveUp  = lipgloss.NewStyle().Foreground(lipgloss.Color("196")) // red

	styleTitle = lipgloss.NewStyle().
			Bold(true).
			Foreground(lipgloss.Color("212")).
			PaddingBottom(1)

	styleBorder = lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(lipgloss.Color("238"))
)

// Sidebar is the left pane listing all projects.
type Sidebar struct {
	rows   []Row
	cursor int
	width  int
	height int
}

// SetSize updates sidebar dimensions.
func (s *Sidebar) SetSize(width, height int) {
	s.width = width
	s.height = height
}

// SetRows replaces all rows from a runtime snapshot, preserving cursor bounds.
func (s *Sidebar) SetRows(snapshot []state.ProjectRuntime) {
	s.rows = make([]Row, len(snapshot))
	for i, r := range snapshot {
		s.rows[i] = Row{ID: r.ID, State: r.State, Attempts: r.RestartAttempts}
	}
	// Clamp cursor.
	if s.cursor >= len(s.rows) && len(s.rows) > 0 {
		s.cursor = len(s.rows) - 1
	}
}

// Up moves the cursor up by one, clamped to the top.
func (s *Sidebar) Up() {
	if s.cursor > 0 {
		s.cursor--
	}
}

// Down moves the cursor down by one, clamped to the bottom.
func (s *Sidebar) Down() {
	if s.cursor < len(s.rows)-1 {
		s.cursor++
	}
}

// Cursor returns the zero-based index of the highlighted row, or -1 if empty.
func (s *Sidebar) Cursor() int {
	if len(s.rows) == 0 {
		return -1
	}
	return s.cursor
}

// Selected returns the ID of the currently highlighted row, or "" if empty.
func (s *Sidebar) Selected() string {
	if len(s.rows) == 0 || s.cursor >= len(s.rows) {
		return ""
	}
	return s.rows[s.cursor].ID
}

// SetSelected moves the cursor to the row matching id.
func (s *Sidebar) SetSelected(id string) {
	for i, r := range s.rows {
		if r.ID == id {
			s.cursor = i
			return
		}
	}
}

// View renders the sidebar into a fixed-width string.
func (s *Sidebar) View() string {
	innerW := s.width - 2 // account for border
	if innerW < 1 {
		innerW = 1
	}

	var b strings.Builder
	b.WriteString(styleTitle.Width(innerW).Render("PROCESSES"))
	b.WriteString("\n")

	for i, row := range s.rows {
		glyph, gStyle := glyphForState(row.State)
		styledGlyph := gStyle.Render(glyph)

		suffix := ""
		if row.Attempts > 0 {
			suffix = fmt.Sprintf(" (×%d)", row.Attempts)
		}

		label := truncate(row.ID+suffix, innerW-3) // 3 = glyph + space + space

		var line string
		if i == s.cursor {
			line = styleSelected.Width(innerW).Render(styledGlyph + " " + label)
		} else {
			line = styleNormal.Width(innerW).Render(styledGlyph + " " + label)
		}
		b.WriteString(line)
		b.WriteString("\n")
	}

	if len(s.rows) == 0 {
		b.WriteString(styleNormal.Width(innerW).Render("(no projects)"))
		b.WriteString("\n")
	}

	content := b.String()
	return styleBorder.Width(s.width - 2).Height(s.height - 2).Render(content)
}

// glyphForState returns the glyph character and its lipgloss style for the given state.
func glyphForState(state string) (string, lipgloss.Style) {
	switch state {
	case "running":
		return glyphRunning, styleGlyphRunning
	case "starting":
		return glyphStarting, styleGlyphStart
	case "stopping":
		return glyphStopping, styleGlyphStart
	case "crashed":
		return glyphCrashed, styleGlyphCrash
	case "restarting":
		return glyphRestarting, styleGlyphRestart
	case "giving-up":
		return glyphGivingUp, styleGlyphGiveUp
	default: // idle, unknown
		return glyphIdle, styleGlyphIdle
	}
}

// truncate clips s to maxLen runes, appending "…" if truncated.
func truncate(s string, maxLen int) string {
	if maxLen <= 0 {
		return ""
	}
	runes := []rune(s)
	if len(runes) <= maxLen {
		return s
	}
	if maxLen <= 1 {
		return "…"
	}
	return string(runes[:maxLen-1]) + "…"
}
