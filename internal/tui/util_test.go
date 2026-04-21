package tui

import (
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"
)

// TestOverlayBottomRight_PreservesBase ensures the helper does NOT blank out
// unrelated rows — a regression guard for the bug where appending the toast
// view via "\n" made the whole UI disappear.
func TestOverlayBottomRight_PreservesBase(t *testing.T) {
	width := 20
	height := 5
	baseLines := []string{
		"row0 content.......",
		"row1 content.......",
		"row2 content.......",
		"row3 content.......",
		"status bar..........",
	}
	// Pad each line to exact width.
	for i, l := range baseLines {
		if ansi.StringWidth(l) < width {
			baseLines[i] = l + strings.Repeat(" ", width-ansi.StringWidth(l))
		}
	}
	base := strings.Join(baseLines, "\n")

	overlay := "TOAST"
	out := overlayBottomRight(base, overlay, width, height-2) // anchor above status

	lines := strings.Split(out, "\n")
	if len(lines) != height {
		t.Fatalf("overlay must not change line count; got %d want %d", len(lines), height)
	}
	// Status bar untouched.
	if !strings.Contains(lines[4], "status bar") {
		t.Errorf("status bar altered: %q", lines[4])
	}
	// Overlay lands on line 3 (height-2).
	if !strings.HasSuffix(lines[3], "TOAST") {
		t.Errorf("overlay not on anchor line: %q", lines[3])
	}
	// Earlier content still visible on its own rows.
	if !strings.Contains(lines[0], "row0 content") {
		t.Errorf("row0 blanked: %q", lines[0])
	}
	if !strings.Contains(lines[3], "row3 content") {
		t.Errorf("row3 left side blanked: %q", lines[3])
	}
}

func TestOverlayBottomRight_EmptyOverlay(t *testing.T) {
	base := "a\nb\nc"
	if got := overlayBottomRight(base, "", 10, 2); got != base {
		t.Errorf("empty overlay must return base unchanged, got %q", got)
	}
}

// TestOverlayCenter_PreservesBase is the regression guard for the "popup takes
// the whole UI" bug: centered overlays must NOT blank rows above or below the
// box, and must leave the left/right margins of covered rows visible.
func TestOverlayCenter_PreservesBase(t *testing.T) {
	width := 30
	height := 9

	baseLines := make([]string, height)
	for i := range baseLines {
		line := "sidebar|log content " + string(rune('A'+i))
		if ansi.StringWidth(line) < width {
			line += strings.Repeat(" ", width-ansi.StringWidth(line))
		}
		baseLines[i] = line
	}
	base := strings.Join(baseLines, "\n")

	overlay := "+-----+\n| hi  |\n+-----+"
	out := overlayCenter(base, overlay, width, height)
	lines := strings.Split(out, "\n")

	if len(lines) != height {
		t.Fatalf("line count changed: got %d want %d", len(lines), height)
	}
	// Rows outside the centered box (top/bottom) must be untouched.
	if !strings.Contains(lines[0], "sidebar|log content A") {
		t.Errorf("top row blanked: %q", lines[0])
	}
	if !strings.Contains(lines[height-1], "sidebar|log content "+string(rune('A'+height-1))) {
		t.Errorf("bottom row blanked: %q", lines[height-1])
	}
	// Somewhere in the middle, the overlay body 'hi' must appear.
	joined := strings.Join(lines, "\n")
	if !strings.Contains(joined, "| hi  |") {
		t.Errorf("overlay body missing from composed frame")
	}
	// The sidebar prefix must survive on at least one covered row (left margin).
	foundLeftMargin := false
	for _, l := range lines[1 : height-1] {
		if strings.HasPrefix(l, "sidebar") {
			foundLeftMargin = true
			break
		}
	}
	if !foundLeftMargin {
		t.Errorf("left-of-popup content blanked on every covered row; frame:\n%s", out)
	}
}

func TestOverlayCenter_EmptyOverlay(t *testing.T) {
	base := "a\nb\nc"
	if got := overlayCenter(base, "", 10, 3); got != base {
		t.Errorf("empty overlay must return base unchanged, got %q", got)
	}
}
