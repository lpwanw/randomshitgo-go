package panes

import (
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
)

func windowRows(n int) []string {
	rows := make([]string, n)
	for i := range rows {
		rows[i] = strings.Repeat("x", 5)
	}
	return rows
}

func TestWindow_ScrollClamping(t *testing.T) {
	w := newWindow(10, 4)
	w.SetRows(windowRows(10))

	if !w.AtTop() {
		t.Error("fresh window should be at top")
	}
	w.ScrollDown(100)
	if w.YOffset != 6 { // 10 rows - 4 height
		t.Errorf("ScrollDown should clamp to maxYOffset 6, got %d", w.YOffset)
	}
	if !w.AtBottom() {
		t.Error("should be at bottom after over-scroll")
	}
	w.ScrollUp(100)
	if w.YOffset != 0 {
		t.Errorf("ScrollUp should clamp to 0, got %d", w.YOffset)
	}
	w.PageDown()
	if w.YOffset != 4 {
		t.Errorf("PageDown should advance by height, got %d", w.YOffset)
	}
	w.GotoBottom()
	if w.YOffset != 6 {
		t.Errorf("GotoBottom: want 6, got %d", w.YOffset)
	}
	w.GotoTop()
	if w.YOffset != 0 {
		t.Errorf("GotoTop: want 0, got %d", w.YOffset)
	}
}

func TestWindow_SetRowsReclampsShrunkBuffer(t *testing.T) {
	w := newWindow(10, 4)
	w.SetRows(windowRows(20))
	w.GotoBottom() // offset 16
	w.SetRows(windowRows(5))
	if w.YOffset != 1 { // 5 rows - 4 height
		t.Errorf("offset should re-clamp after shrink, got %d", w.YOffset)
	}
	w.SetRows(nil)
	if w.YOffset != 0 {
		t.Errorf("offset should be 0 on empty rows, got %d", w.YOffset)
	}
}

func TestWindow_ViewWindowsAndPads(t *testing.T) {
	w := newWindow(6, 3)
	w.SetRows([]string{"aa", "bb", "cc", "dd", "ee"})
	w.SetYOffset(2)
	lines := strings.Split(w.View(), "\n")
	if len(lines) != 3 {
		t.Fatalf("View should emit exactly height rows, got %d", len(lines))
	}
	for i, want := range []string{"cc", "dd", "ee"} {
		if !strings.HasPrefix(lines[i], want) {
			t.Errorf("row %d: want prefix %q, got %q", i, want, lines[i])
		}
		if len(lines[i]) != 6 {
			t.Errorf("row %d should be padded to width 6, got %d", i, len(lines[i]))
		}
	}
}

func TestWindow_WheelScrolls(t *testing.T) {
	w := newWindow(10, 4)
	w.SetRows(windowRows(20))
	down := tea.MouseMsg{Action: tea.MouseActionPress, Button: tea.MouseButtonWheelDown}
	if cmd := w.Update(down); cmd != nil {
		t.Error("wheel update should return nil cmd")
	}
	if w.YOffset != 3 {
		t.Errorf("wheel down should scroll 3 lines, got %d", w.YOffset)
	}
	up := tea.MouseMsg{Action: tea.MouseActionPress, Button: tea.MouseButtonWheelUp}
	w.Update(up)
	if w.YOffset != 0 {
		t.Errorf("wheel up should scroll back to 0, got %d", w.YOffset)
	}
	// Motion (non-press) wheel events are ignored.
	w.Update(tea.MouseMsg{Action: tea.MouseActionMotion, Button: tea.MouseButtonWheelDown})
	if w.YOffset != 0 {
		t.Errorf("non-press wheel should not scroll, got %d", w.YOffset)
	}
}
