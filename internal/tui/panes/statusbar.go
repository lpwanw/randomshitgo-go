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

	// selection counter
	sel := sb.Selected
	if sel == "" {
		sel = "—"
	}
	counter := fmt.Sprintf("%s  %d/%d", sel, sb.cursor(), sb.Total)

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

	parts := []string{mode, " ", styleBar.Render(counter)}
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

// cursor returns a 1-based position of the selected item.
func (sb *StatusBar) cursor() int {
	if sb.Total == 0 {
		return 0
	}
	return 1 // caller should set a real index when known
}
