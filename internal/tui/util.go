package tui

import (
	"strings"

	"github.com/charmbracelet/x/ansi"
)

func max(a, b int) int {
	if a > b {
		return a
	}
	return b
}

// overlayBottomRight composes overlay onto the right edge of base, anchored so
// the overlay's last line sits on base line (bottomLine). base is a multi-line
// canvas (typically the full frame); overlay is a small block (e.g., toasts).
// Lines in overlay narrower than the canvas are right-aligned against width.
// ANSI escape sequences are respected via the charmbracelet ansi package.
func overlayBottomRight(base, overlay string, width, bottomLine int) string {
	if overlay == "" {
		return base
	}
	baseLines := strings.Split(base, "\n")
	overlayLines := strings.Split(overlay, "\n")

	start := bottomLine - len(overlayLines) + 1
	if start < 0 {
		start = 0
	}

	for i, ov := range overlayLines {
		li := start + i
		if li < 0 || li >= len(baseLines) {
			continue
		}
		ow := ansi.StringWidth(ov)
		if ow >= width {
			baseLines[li] = ov
			continue
		}
		leftW := width - ow
		leftLine := baseLines[li]
		lw := ansi.StringWidth(leftLine)
		switch {
		case lw == leftW:
			// exact fit
		case lw > leftW:
			leftLine = ansi.Truncate(leftLine, leftW, "")
		default:
			leftLine = leftLine + strings.Repeat(" ", leftW-lw)
		}
		baseLines[li] = leftLine + ov
	}
	return strings.Join(baseLines, "\n")
}
