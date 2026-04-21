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
		baseLines[li] = padOrTruncate(baseLines[li], leftW) + ov
	}
	return strings.Join(baseLines, "\n")
}

// overlayCenter composes overlay onto the centre of base without touching the
// rows above/below it — the surrounding UI (sidebar, log, status) stays visible.
// base is a canvas of width × height cells; overlay is a small compact block.
func overlayCenter(base, overlay string, width, height int) string {
	if overlay == "" {
		return base
	}
	baseLines := strings.Split(base, "\n")
	overlayLines := strings.Split(overlay, "\n")

	oh := len(overlayLines)
	// Widest overlay line drives horizontal placement.
	ow := 0
	for _, l := range overlayLines {
		if w := ansi.StringWidth(l); w > ow {
			ow = w
		}
	}
	if ow > width {
		ow = width
	}

	topRow := (height - oh) / 2
	if topRow < 0 {
		topRow = 0
	}
	leftCol := (width - ow) / 2
	if leftCol < 0 {
		leftCol = 0
	}

	for i, ov := range overlayLines {
		li := topRow + i
		if li < 0 || li >= len(baseLines) {
			continue
		}
		baseLines[li] = mergeAtColumn(baseLines[li], ov, leftCol, width)
	}
	return strings.Join(baseLines, "\n")
}

// mergeAtColumn replaces a slice of base starting at column col with overlay,
// keeping anything to the left/right of the overlay span visible. width caps
// the total visible width of the returned line.
func mergeAtColumn(base, overlay string, col, width int) string {
	ow := ansi.StringWidth(overlay)
	if ow > width-col {
		overlay = ansi.Truncate(overlay, width-col, "")
		ow = ansi.StringWidth(overlay)
	}

	left := ansi.Truncate(base, col, "")
	lw := ansi.StringWidth(left)
	if lw < col {
		left = left + strings.Repeat(" ", col-lw)
	}

	rightStart := col + ow
	right := ""
	if rightStart < width {
		// Characters col..col+ow in base are covered; keep the tail past rightStart.
		tail := ansi.Cut(base, rightStart, width)
		right = tail
	}
	return left + overlay + right
}

// padOrTruncate clips or right-pads s so its visible width equals w.
func padOrTruncate(s string, w int) string {
	if w <= 0 {
		return ""
	}
	sw := ansi.StringWidth(s)
	switch {
	case sw == w:
		return s
	case sw > w:
		return ansi.Truncate(s, w, "")
	default:
		return s + strings.Repeat(" ", w-sw)
	}
}
