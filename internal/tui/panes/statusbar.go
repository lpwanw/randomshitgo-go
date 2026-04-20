package panes

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/lipgloss"
)

var (
	styleBar = lipgloss.NewStyle().
			Background(lipgloss.Color("235")).
			Foreground(lipgloss.Color("252"))

	styleModeIndicator = lipgloss.NewStyle().
				Bold(true).
				Background(lipgloss.Color("62")).
				Foreground(lipgloss.Color("255")).
				Padding(0, 1)

	styleBarSep = lipgloss.NewStyle().
			Foreground(lipgloss.Color("238")).
			Background(lipgloss.Color("235"))

	styleBarDim = lipgloss.NewStyle().
			Foreground(lipgloss.Color("240")).
			Background(lipgloss.Color("235"))
)

// StatusBar holds the data for the single-line status bar at the bottom of the TUI.
type StatusBar struct {
	Mode       string
	Selected   string
	Index      int // zero-based cursor position; negative hides counter
	Total      int
	PID        int
	Port       int
	GitBranch  string
	GitAhead   int
	GitBehind  int
	GitDirty   bool
	FilterText string
	Width      int
}

// View renders a single-line status bar, clipped to Width.
func (sb *StatusBar) View() string {
	if sb.Width <= 0 {
		return ""
	}

	mode := styleModeIndicator.Render(sb.Mode)
	sep := styleBarSep.Render(" │ ")

	// selection counter — only when there are projects to count.
	counter := ""
	if sb.Total > 0 {
		sel := sb.Selected
		if sel == "" {
			sel = "—"
		}
		counter = fmt.Sprintf("%s  %d/%d", sel, sb.position(), sb.Total)
	}

	// pid segment
	pidSeg := ""
	if sb.PID > 0 {
		pidSeg = sep + styleBarDim.Render(fmt.Sprintf("pid:%d", sb.PID))
	}

	// port segment
	portSeg := ""
	if sb.Port > 0 {
		portSeg = sep + styleBarDim.Render(fmt.Sprintf(":%d", sb.Port))
	}

	// git segment
	gitSeg := ""
	if sb.GitBranch != "" {
		dirty := ""
		if sb.GitDirty {
			dirty = "*"
		}
		ahead := ""
		if sb.GitAhead > 0 {
			ahead = fmt.Sprintf(" +%d", sb.GitAhead)
		}
		behind := ""
		if sb.GitBehind > 0 {
			behind = fmt.Sprintf(" -%d", sb.GitBehind)
		}
		gitSeg = sep + styleBarDim.Render(fmt.Sprintf("git:%s%s%s%s", sb.GitBranch, dirty, ahead, behind))
	}

	// filter
	filterSeg := ""
	if sb.FilterText != "" {
		filterSeg = sep + styleBarDim.Render(fmt.Sprintf("filter:%s", sb.FilterText))
	}

	parts := []string{mode, " "}
	if counter != "" {
		parts = append(parts, styleBar.Render(counter))
	}
	if pidSeg != "" {
		parts = append(parts, pidSeg)
	}
	if portSeg != "" {
		parts = append(parts, portSeg)
	}
	if gitSeg != "" {
		parts = append(parts, gitSeg)
	}
	if filterSeg != "" {
		parts = append(parts, filterSeg)
	}

	bar := strings.Join(parts, "")
	// Pad / clip to Width.
	bar = styleBar.Width(sb.Width).Render(bar)
	return bar
}

// position returns the 1-based position of the cursor, clamped to [1, Total].
// Falls back to 1 when the caller did not set Index (negative).
func (sb *StatusBar) position() int {
	if sb.Total <= 0 {
		return 0
	}
	if sb.Index < 0 {
		return 1
	}
	pos := sb.Index + 1
	if pos > sb.Total {
		pos = sb.Total
	}
	return pos
}
