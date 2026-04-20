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
