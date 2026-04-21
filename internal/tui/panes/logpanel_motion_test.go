package panes

import (
	"testing"

	tea "github.com/charmbracelet/bubbletea"
)

func keyMsg(s string) tea.KeyMsg {
	if len(s) == 1 {
		return tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(s)}
	}
	switch s {
	case "ctrl+u":
		return tea.KeyMsg{Type: tea.KeyCtrlU}
	case "ctrl+d":
		return tea.KeyMsg{Type: tea.KeyCtrlD}
	case "ctrl+b":
		return tea.KeyMsg{Type: tea.KeyCtrlB}
	case "ctrl+f":
		return tea.KeyMsg{Type: tea.KeyCtrlF}
	}
	return tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(s)}
}

func newCopyPanel(t *testing.T, lines []string) *LogPanel {
	t.Helper()
	lp := NewLogPanel(80, 6)
	lp.SetLines(lines)
	lp.SetCopyMode(true)
	return &lp
}

func TestCopy_BasicHJKL(t *testing.T) {
	lp := newCopyPanel(t, []string{"hello", "world wide", "bye"})
	lp.HandleCopyKey(keyMsg("l"))
	lp.HandleCopyKey(keyMsg("l"))
	if line, col := lp.Cursor(); line != 0 || col != 2 {
		t.Fatalf("after ll want (0,2), got (%d,%d)", line, col)
	}
	lp.HandleCopyKey(keyMsg("j"))
	if line, col := lp.Cursor(); line != 1 || col != 2 {
		t.Fatalf("after j want (1,2), got (%d,%d)", line, col)
	}
	lp.HandleCopyKey(keyMsg("h"))
	if _, col := lp.Cursor(); col != 1 {
		t.Fatalf("after h want col=1, got %d", col)
	}
	lp.HandleCopyKey(keyMsg("k"))
	if line, col := lp.Cursor(); line != 0 || col != 1 {
		t.Fatalf("after k want (0,1), got (%d,%d)", line, col)
	}
}

func TestCopy_ZeroDollar(t *testing.T) {
	lp := newCopyPanel(t, []string{"abcdef"})
	lp.HandleCopyKey(keyMsg("$"))
	if _, col := lp.Cursor(); col != 6 {
		t.Errorf("$ want col=6, got %d", col)
	}
	lp.HandleCopyKey(keyMsg("0"))
	if _, col := lp.Cursor(); col != 0 {
		t.Errorf("0 want col=0, got %d", col)
	}
}

func TestCopy_GgG(t *testing.T) {
	lp := newCopyPanel(t, []string{"a", "b", "c", "d"})
	lp.HandleCopyKey(keyMsg("G"))
	if line, _ := lp.Cursor(); line != 3 {
		t.Errorf("G want line=3, got %d", line)
	}
	lp.HandleCopyKey(keyMsg("g"))
	lp.HandleCopyKey(keyMsg("g"))
	if line, _ := lp.Cursor(); line != 0 {
		t.Errorf("gg want line=0, got %d", line)
	}
}

func TestCopy_WordForwardCrossLine(t *testing.T) {
	lp := newCopyPanel(t, []string{"foo bar", "baz"})
	lp.HandleCopyKey(keyMsg("w"))
	if line, col := lp.Cursor(); line != 0 || col != 4 {
		t.Fatalf("w within line want (0,4), got (%d,%d)", line, col)
	}
	lp.HandleCopyKey(keyMsg("w"))
	if line, col := lp.Cursor(); line != 1 || col != 0 {
		t.Fatalf("w across line want (1,0), got (%d,%d)", line, col)
	}
}

func TestCopy_WordBackward(t *testing.T) {
	lp := newCopyPanel(t, []string{"foo bar baz"})
	lp.HandleCopyKey(keyMsg("$"))
	lp.HandleCopyKey(keyMsg("b"))
	if _, col := lp.Cursor(); col != 8 {
		t.Errorf("b from eol want col=8, got %d", col)
	}
	lp.HandleCopyKey(keyMsg("b"))
	if _, col := lp.Cursor(); col != 4 {
		t.Errorf("bb want col=4, got %d", col)
	}
}

func TestCopy_HalfPageAndClamp(t *testing.T) {
	lines := make([]string, 50)
	for i := range lines {
		lines[i] = "x"
	}
	lp := newCopyPanel(t, lines)
	lp.HandleCopyKey(keyMsg("ctrl+d"))
	if line, _ := lp.Cursor(); line == 0 {
		t.Error("ctrl+d should advance line")
	}
	lp.HandleCopyKey(keyMsg("G"))
	lp.HandleCopyKey(keyMsg("ctrl+d"))
	if line, _ := lp.Cursor(); line != 49 {
		t.Errorf("ctrl+d past end want clamped to 49, got %d", line)
	}
}

func TestCopy_EnsureCursorVisibleScrolls(t *testing.T) {
	lines := make([]string, 100)
	for i := range lines {
		lines[i] = "x"
	}
	lp := newCopyPanel(t, lines)
	lp.HandleCopyKey(keyMsg("G"))
	if lp.vp.YOffset+lp.vp.Height <= 99 {
		t.Errorf("after G, viewport should cover line 99; off=%d h=%d", lp.vp.YOffset, lp.vp.Height)
	}
	lp.HandleCopyKey(keyMsg("g"))
	lp.HandleCopyKey(keyMsg("g"))
	if lp.vp.YOffset != 0 {
		t.Errorf("after gg, YOffset should be 0, got %d", lp.vp.YOffset)
	}
}

func TestCopy_PendingGClearsOnExit(t *testing.T) {
	lp := newCopyPanel(t, []string{"a", "b"})
	lp.HandleCopyKey(keyMsg("g"))
	if !lp.pendingG {
		t.Error("pendingG should be set after first g")
	}
	lp.SetCopyMode(false)
	if lp.pendingG {
		t.Error("pendingG should be cleared on copy-mode exit")
	}
}

func TestCopy_CursorRendersReverseVideo(t *testing.T) {
	lp := NewLogPanel(80, 6)
	lp.SetLines([]string{"hello"})
	lp.SetCopyMode(true)
	view := lp.vp.View()
	if !containsSGR(view, "\x1b[7m") {
		t.Errorf("copy-mode cursor should emit reverse-video SGR; view=%q", view)
	}
}

func containsSGR(s, seq string) bool {
	return indexOf(s, seq) >= 0
}

func indexOf(s, sub string) int {
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return i
		}
	}
	return -1
}
