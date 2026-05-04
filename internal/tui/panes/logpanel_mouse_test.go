package panes

import (
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
)

// newMousePanel builds a LogPanel sized so origin (0,0), border 1 cell,
// gutter off → content cell (x,y) maps to MouseMsg{X: x+1, Y: y+1}.
func newMousePanel(t *testing.T, lines []string) *LogPanel {
	t.Helper()
	lp := NewLogPanel(80, 10)
	lp.SetOrigin(0, 0)
	lp.SetWrap(false) // keep coord math simple
	lp.SetLines(lines)
	return &lp
}

func mouse(action tea.MouseAction, button tea.MouseButton, x, y int) tea.MouseMsg {
	return tea.MouseMsg{Action: action, Button: button, X: x, Y: y}
}

func TestMouse_DragSelectCharwise(t *testing.T) {
	lp := newMousePanel(t, []string{"foo bar baz"})
	fc := &fakeClip{}
	lp.SetClipboard(fc)

	// Press at content (0,0) = X=1,Y=1; drag to content (6,0) = X=7,Y=1.
	lp.handleMouse(mouse(tea.MouseActionPress, tea.MouseButtonLeft, 1, 1))
	if !lp.HasSelection() {
		t.Fatal("press should start a selection")
	}
	lp.handleMouse(mouse(tea.MouseActionMotion, tea.MouseButtonLeft, 7, 1))
	cmd := lp.handleMouse(mouse(tea.MouseActionRelease, tea.MouseButtonLeft, 7, 1))
	if cmd == nil {
		t.Fatal("release after drag should produce a yank cmd")
	}
	if msg, ok := cmd().(CopiedMsg); !ok {
		t.Fatalf("want CopiedMsg, got %T", cmd())
	} else if msg.Chars != len("foo bar") {
		t.Fatalf("want %d chars, got %d", len("foo bar"), msg.Chars)
	}
	if fc.text != "foo bar" {
		t.Errorf("clipboard=%q want %q", fc.text, "foo bar")
	}
	if lp.HasSelection() {
		t.Error("release should clear selection (xterm-style)")
	}
}

func TestMouse_ClickClearsSelection(t *testing.T) {
	lp := newMousePanel(t, []string{"hello world"})
	// Pre-set a selection.
	lp.handleMouse(mouse(tea.MouseActionPress, tea.MouseButtonLeft, 1, 1))
	lp.handleMouse(mouse(tea.MouseActionMotion, tea.MouseButtonLeft, 5, 1))
	if !lp.HasSelection() {
		t.Fatal("expected selection mid-drag")
	}
	// Release at same cell as press → no copy, just clear.
	fc := &fakeClip{}
	lp.SetClipboard(fc)
	// Reset drag with a fresh single-cell click.
	lp.handleMouse(mouse(tea.MouseActionPress, tea.MouseButtonLeft, 3, 1))
	cmd := lp.handleMouse(mouse(tea.MouseActionRelease, tea.MouseButtonLeft, 3, 1))
	if cmd != nil {
		t.Errorf("single click should not yank, got cmd")
	}
	if lp.HasSelection() {
		t.Error("single click should leave no selection")
	}
}

func TestMouse_OutOfPaneIgnored(t *testing.T) {
	lp := newMousePanel(t, []string{"abcdef"})
	// X past pane width → ignored.
	lp.handleMouse(mouse(tea.MouseActionPress, tea.MouseButtonLeft, 200, 1))
	if lp.HasSelection() {
		t.Error("press outside pane should not select")
	}
	// Y on border row → ignored.
	lp.handleMouse(mouse(tea.MouseActionPress, tea.MouseButtonLeft, 5, 0))
	if lp.HasSelection() {
		t.Error("press on top border should not select")
	}
}

func TestMouse_GutterClickSnapsToCol0(t *testing.T) {
	lp := newMousePanel(t, []string{"alpha beta"})
	lp.SetGutter(true)
	// Re-paint after gutter toggle so rowToRaw is fresh.
	lp.handleMouse(mouse(tea.MouseActionPress, tea.MouseButtonLeft, 2, 1))
	_, col := lp.Cursor()
	if col != 0 {
		t.Errorf("gutter click should land on col 0, got col=%d", col)
	}
}

func TestMouse_DragMultiline(t *testing.T) {
	lp := newMousePanel(t, []string{"first", "second", "third"})
	fc := &fakeClip{}
	lp.SetClipboard(fc)
	// Press at row 0 col 2; drag to row 2 col 1.
	lp.handleMouse(mouse(tea.MouseActionPress, tea.MouseButtonLeft, 3, 1))
	lp.handleMouse(mouse(tea.MouseActionMotion, tea.MouseButtonLeft, 2, 3))
	cmd := lp.handleMouse(mouse(tea.MouseActionRelease, tea.MouseButtonLeft, 2, 3))
	if cmd == nil {
		t.Fatal("multi-line drag should yank")
	}
	cmd()
	want := strings.Join([]string{"rst", "second", "th"}, "\n")
	if fc.text != want {
		t.Errorf("clipboard=%q want %q", fc.text, want)
	}
}

func TestMouse_WheelStillScrolls(t *testing.T) {
	// Fill enough lines to exceed viewport height.
	lines := make([]string, 50)
	for i := range lines {
		lines[i] = "line"
	}
	lp := newMousePanel(t, lines)
	startOffset := lp.vp.YOffset
	// Wheel up ⇒ scroll up (offset decreases or stays at 0 if already top).
	lp.handleMouse(tea.MouseMsg{
		Action: tea.MouseActionPress,
		Button: tea.MouseButtonWheelUp,
		X:      2,
		Y:      2,
	})
	if lp.vp.YOffset > startOffset {
		t.Errorf("wheel up should not increase offset (start=%d, after=%d)", startOffset, lp.vp.YOffset)
	}
}

func TestMouse_SelectionPausesSticky(t *testing.T) {
	lp := newMousePanel(t, []string{"a", "b"})
	// Force sticky on, then start a selection.
	lp.sticky = true
	lp.handleMouse(mouse(tea.MouseActionPress, tea.MouseButtonLeft, 1, 1))
	beforeOffset := lp.vp.YOffset
	// Append many lines — would normally jump to bottom under sticky.
	more := make([]string, 200)
	for i := range more {
		more[i] = "x"
	}
	lp.SetLines(append([]string{"a", "b"}, more...))
	if lp.vp.YOffset != beforeOffset {
		t.Errorf("active selection should pin viewport (before=%d, after=%d)", beforeOffset, lp.vp.YOffset)
	}
}
