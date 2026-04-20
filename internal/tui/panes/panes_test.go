package panes

import (
	"regexp"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/taynguyen/procs/internal/state"
)

// ---- Sidebar tests ----

func TestSidebar_CursorBounds(t *testing.T) {
	s := &Sidebar{}
	s.SetRows([]state.ProjectRuntime{
		{ID: "a", State: "running"},
		{ID: "b", State: "idle"},
		{ID: "c", State: "crashed"},
	})

	// Start at 0, Up should not go negative.
	s.Up()
	if s.cursor != 0 {
		t.Errorf("Up at top: want cursor=0, got %d", s.cursor)
	}

	// Down twice.
	s.Down()
	s.Down()
	if s.cursor != 2 {
		t.Errorf("Down x2: want cursor=2, got %d", s.cursor)
	}

	// Down past end should clamp.
	s.Down()
	if s.cursor != 2 {
		t.Errorf("Down at bottom: want cursor=2, got %d", s.cursor)
	}
}

func TestSidebar_Selected(t *testing.T) {
	s := &Sidebar{}
	if s.Selected() != "" {
		t.Error("empty sidebar should return empty selected")
	}

	s.SetRows([]state.ProjectRuntime{
		{ID: "api", State: "running"},
		{ID: "web", State: "idle"},
	})
	if s.Selected() != "api" {
		t.Errorf("initial selection: want api, got %q", s.Selected())
	}

	s.Down()
	if s.Selected() != "web" {
		t.Errorf("after Down: want web, got %q", s.Selected())
	}
}

func TestSidebar_SetSelected(t *testing.T) {
	s := &Sidebar{}
	s.SetRows([]state.ProjectRuntime{
		{ID: "api", State: "running"},
		{ID: "web", State: "idle"},
		{ID: "db", State: "crashed"},
	})

	s.SetSelected("db")
	if s.Selected() != "db" {
		t.Errorf("SetSelected: want db, got %q", s.Selected())
	}

	// Non-existent id should not move cursor.
	s.SetSelected("notexist")
	if s.Selected() != "db" {
		t.Errorf("SetSelected non-existent: want db, got %q", s.Selected())
	}
}

func TestSidebar_GlyphMapping(t *testing.T) {
	states := map[string]string{
		"running":    glyphRunning,
		"idle":       glyphIdle,
		"starting":   glyphStarting,
		"stopping":   glyphStopping,
		"crashed":    glyphCrashed,
		"restarting": glyphRestarting,
		"giving-up":  glyphGivingUp,
	}

	for st, expectedGlyph := range states {
		glyph, _ := glyphForState(st)
		if glyph != expectedGlyph {
			t.Errorf("state %q: want glyph %q, got %q", st, expectedGlyph, glyph)
		}
	}
}

func TestSidebar_CursorClampedAfterSetRows(t *testing.T) {
	s := &Sidebar{}
	s.SetRows([]state.ProjectRuntime{
		{ID: "a", State: "idle"},
		{ID: "b", State: "idle"},
		{ID: "c", State: "idle"},
	})
	s.Down()
	s.Down() // cursor = 2

	// Reduce list to 1 item.
	s.SetRows([]state.ProjectRuntime{
		{ID: "a", State: "idle"},
	})
	if s.cursor != 0 {
		t.Errorf("cursor after shrink: want 0, got %d", s.cursor)
	}
}

// ---- LogPanel tests ----

func TestLogPanel_StickyDefault(t *testing.T) {
	lp := NewLogPanel(80, 24)
	if !lp.Sticky() {
		t.Error("LogPanel should start sticky")
	}
}

func TestLogPanel_StickyFlipsOnPageUp(t *testing.T) {
	lp := NewLogPanel(80, 24)

	// Add enough lines to allow scrolling.
	lines := make([]string, 200)
	for i := range lines {
		lines[i] = "log line"
	}
	lp.SetLines(lines)

	// PageUp should disable sticky.
	pgUpMsg := tea.KeyMsg{Type: tea.KeyPgUp}
	lp.Update(pgUpMsg)

	if lp.Sticky() {
		t.Error("sticky should be false after PageUp")
	}
}

func TestLogPanel_StickyRestoredOnG(t *testing.T) {
	lp := NewLogPanel(80, 24)

	lines := make([]string, 200)
	for i := range lines {
		lines[i] = "log line"
	}
	lp.SetLines(lines)

	// Scroll up, then press G.
	lp.Update(tea.KeyMsg{Type: tea.KeyPgUp})
	if lp.Sticky() {
		t.Error("sticky should be false after PageUp")
	}

	lp.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'G'}})
	if !lp.Sticky() {
		t.Error("sticky should be restored after G")
	}
}

func TestLogPanel_FilterHidesNonMatching(t *testing.T) {
	lp := NewLogPanel(80, 24)
	lp.SetLines([]string{"hello world", "error: something", "debug info"})

	rx := regexp.MustCompile(`(?i)error`)
	lp.SetFilter(rx)

	// The viewport content should only contain the error line.
	content := lp.vp.View()
	if !strings.Contains(content, "error: something") {
		t.Error("filtered content should contain 'error: something'")
	}
	if strings.Contains(content, "hello world") {
		t.Error("filtered content should NOT contain 'hello world'")
	}
}

func TestLogPanel_FilterClearShowsAll(t *testing.T) {
	lp := NewLogPanel(80, 24)
	lp.SetLines([]string{"hello world", "error: something"})

	// Apply then clear filter.
	rx := regexp.MustCompile(`(?i)error`)
	lp.SetFilter(rx)
	lp.SetFilter(nil)

	content := lp.vp.View()
	if !strings.Contains(content, "hello world") {
		t.Error("cleared filter should show all lines")
	}
}

// ---- StatusBar tests ----

func TestStatusBar_View_WidthCapped(t *testing.T) {
	sb := &StatusBar{
		Mode:     "NORMAL",
		Selected: "api",
		Total:    3,
		Width:    40,
	}
	view := sb.View()

	// ansi-stripped width check (approximate: lipgloss pads to Width).
	// We just verify it renders without panic and is non-empty.
	if view == "" {
		t.Error("StatusBar.View() returned empty string")
	}
}

func TestStatusBar_View_Placeholders(t *testing.T) {
	sb := &StatusBar{
		Mode:  "NORMAL",
		Total: 0,
		Width: 120,
	}
	view := sb.View()
	// No PID, no port, no git — should still render.
	if view == "" {
		t.Error("StatusBar.View() returned empty string with no PID/port/git")
	}
	// Counter must be hidden when there are no projects to count.
	if strings.Contains(view, "0/0") {
		t.Errorf("expected counter hidden when Total=0, got: %q", view)
	}
}

func TestStatusBar_View_IndexRendersPosition(t *testing.T) {
	sb := &StatusBar{
		Mode:     "NORMAL",
		Selected: "api",
		Index:    2,
		Total:    5,
		Width:    120,
	}
	view := sb.View()
	if !strings.Contains(view, "3/5") {
		t.Errorf("expected counter 3/5, got: %q", view)
	}
}

func TestStatusBar_View_WithGit(t *testing.T) {
	sb := &StatusBar{
		Mode:      "NORMAL",
		Selected:  "api",
		Total:     2,
		GitBranch: "main",
		GitAhead:  1,
		GitDirty:  true,
		Width:     120,
	}
	view := sb.View()
	if !strings.Contains(view, "main") {
		t.Errorf("status bar should contain git branch, got: %q", view)
	}
}
