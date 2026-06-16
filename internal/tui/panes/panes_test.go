package panes

import (
	"fmt"
	"regexp"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/lpwanw/randomshitgo-go/internal/state"
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

func TestSidebar_SetCursor_ClampsBounds(t *testing.T) {
	s := &Sidebar{}

	// Empty sidebar: no panic, no movement.
	s.SetCursor(5)
	if s.cursor != 0 {
		t.Errorf("empty SetCursor must not move cursor, got %d", s.cursor)
	}

	s.SetRows([]state.ProjectRuntime{
		{ID: "a", State: "idle"},
		{ID: "b", State: "idle"},
		{ID: "c", State: "idle"},
	})

	s.SetCursor(2)
	if s.cursor != 2 {
		t.Errorf("SetCursor(2): want 2, got %d", s.cursor)
	}

	// Out of range — ignored, cursor stays.
	s.SetCursor(9)
	if s.cursor != 2 {
		t.Errorf("SetCursor(9) should be ignored, cursor=%d", s.cursor)
	}
	s.SetCursor(-1)
	if s.cursor != 2 {
		t.Errorf("SetCursor(-1) should be ignored, cursor=%d", s.cursor)
	}
}

func TestSidebar_View_NumbersFirstNineRows(t *testing.T) {
	s := &Sidebar{}
	rows := make([]state.ProjectRuntime, 12)
	for i := range rows {
		rows[i] = state.ProjectRuntime{ID: fmt.Sprintf("p%02d", i), State: "idle"}
	}
	s.SetRows(rows)
	s.SetSize(24, 20)

	view := s.View()
	// Each numeric prefix is followed by a status glyph (◯ / ● / …). Use the
	// glyph as an anchor so the check doesn't trip over IDs like "p10".
	for i := 1; i <= 9; i++ {
		anchor := fmt.Sprintf("%d %s", i, glyphIdle)
		if !strings.Contains(view, anchor) {
			t.Errorf("sidebar view must contain prefix anchor %q; got:\n%s", anchor, view)
		}
	}
	if strings.Contains(view, "10 "+glyphIdle) {
		t.Errorf("row 10+ must not render a numeric prefix; got:\n%s", view)
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

func TestLogPanel_FilterHighlightsKeepsAllLines(t *testing.T) {
	lp := NewLogPanel(80, 24)
	lp.SetLines([]string{"hello world", "error: something", "debug info"})

	rx := regexp.MustCompile(`(?i)error`)
	lp.SetFilter(rx)

	// All lines must remain visible; matched text must be wrapped in a
	// highlight SGR sequence (ESC[7m…ESC[27m). Note: the match substring is
	// split by injected SGR so we assert segments, not the contiguous phrase.
	content := lp.vp.View()
	if !strings.Contains(content, "hello world") {
		t.Error("filter must not hide non-matching lines; 'hello world' missing")
	}
	if !strings.Contains(content, "error") || !strings.Contains(content, "something") {
		t.Errorf("matched line text missing: %q", content)
	}
	if !strings.Contains(content, "\x1b[7m") {
		t.Error("expected reverse-video SGR around highlighted match")
	}
	if !strings.Contains(content, "\x1b[27m") {
		t.Error("expected reverse-video SGR close sequence")
	}
}

func TestLogPanel_FilterClearRemovesHighlight(t *testing.T) {
	lp := NewLogPanel(80, 24)
	lp.SetLines([]string{"hello world", "error: something"})

	rx := regexp.MustCompile(`(?i)error`)
	lp.SetFilter(rx)
	lp.SetFilter(nil)

	content := lp.vp.View()
	if !strings.Contains(content, "hello world") {
		t.Error("cleared filter should show all lines")
	}
	if strings.Contains(content, "\x1b[7m") {
		t.Error("cleared filter should remove highlight SGR")
	}
	if lp.MatchCount() != 0 {
		t.Errorf("cleared filter should zero match count, got %d", lp.MatchCount())
	}
}

func TestLogPanel_FocusedMatchUsesDistinctStyle(t *testing.T) {
	lp := NewLogPanel(80, 24)
	lp.SetLines([]string{"alpha err one", "beta err two"})
	lp.SetFilter(regexp.MustCompile(`err`))

	// Before any jump: no focused match, only the default reverse-video style.
	before := lp.vp.View()
	if strings.Contains(before, hlCurOn) {
		t.Errorf("no match focused yet, but current-style present: %q", before)
	}
	if !strings.Contains(before, hlOn) {
		t.Errorf("expected default highlight %q in: %q", hlOn, before)
	}

	// After jumping to the first match, that line should carry hlCurOn.
	if ok, _ := lp.JumpNextMatch(); !ok {
		t.Fatal("JumpNextMatch should succeed")
	}
	after := lp.vp.View()
	if !strings.Contains(after, hlCurOn) {
		t.Errorf("focused match must use hlCurOn style, got: %q", after)
	}
	// The non-focused match should still have the default style.
	if !strings.Contains(after, hlOn) {
		t.Errorf("non-focused matches must still use hlOn, got: %q", after)
	}
}

func TestLogPanel_JumpNextMatch_WrapsAtEnd(t *testing.T) {
	lp := NewLogPanel(80, 10)
	lines := []string{
		"line 0",
		"error one",
		"line 2",
		"line 3",
		"ERROR two",
		"line 5",
	}
	lp.SetLines(lines)
	lp.SetFilter(regexp.MustCompile(`(?i)error`))

	if got := lp.MatchCount(); got != 2 {
		t.Fatalf("match count: want 2, got %d", got)
	}

	ok, wrapped := lp.JumpNextMatch()
	if !ok || wrapped {
		t.Fatalf("first jump: want ok=true wrapped=false, got ok=%v wrapped=%v", ok, wrapped)
	}
	ok, wrapped = lp.JumpNextMatch()
	if !ok || wrapped {
		t.Fatalf("second jump: want ok=true wrapped=false, got ok=%v wrapped=%v", ok, wrapped)
	}
	ok, wrapped = lp.JumpNextMatch()
	if !ok || !wrapped {
		t.Fatalf("third jump should wrap, got ok=%v wrapped=%v", ok, wrapped)
	}
}

func TestLogPanel_JumpPrevMatch_WrapsAtStart(t *testing.T) {
	lp := NewLogPanel(80, 10)
	lp.SetLines([]string{"err 1", "ok", "err 2"})
	lp.SetFilter(regexp.MustCompile(`err`))

	ok, wrapped := lp.JumpPrevMatch()
	if !ok || wrapped {
		t.Fatalf("first prev from fresh state: want ok=true wrapped=false, got ok=%v wrapped=%v", ok, wrapped)
	}
	ok, wrapped = lp.JumpPrevMatch()
	if !ok || !wrapped {
		t.Fatalf("second prev should wrap, got ok=%v wrapped=%v", ok, wrapped)
	}
}

func TestLogPanel_JumpEmpty_NotOK(t *testing.T) {
	lp := NewLogPanel(80, 10)
	lp.SetLines([]string{"nothing matches"})
	lp.SetFilter(regexp.MustCompile(`zzz`))
	if ok, _ := lp.JumpNextMatch(); ok {
		t.Error("JumpNextMatch with zero matches should return ok=false")
	}
}

// ---- Gutter / copy-mode tests ----

func TestLineNumberWidth(t *testing.T) {
	cases := map[int]int{0: 1, 1: 1, 9: 1, 10: 2, 99: 2, 100: 3, 999: 3, 1000: 4}
	for n, want := range cases {
		if got := lineNumberWidth(n); got != want {
			t.Errorf("lineNumberWidth(%d): want %d, got %d", n, want, got)
		}
	}
}

func TestLogPanel_GutterOffByDefault(t *testing.T) {
	lp := NewLogPanel(80, 10)
	lp.SetLines([]string{"alpha", "beta"})
	if got := lp.gutterWidth(); got != 0 {
		t.Errorf("gutter must be 0 when off, got %d", got)
	}
	if strings.Contains(lp.vp.View(), "│") {
		t.Error("viewport should not contain gutter separator when gutter off")
	}
}

func TestLogPanel_SetGutterRendersNumbers(t *testing.T) {
	lp := NewLogPanel(80, 10)
	lp.SetLines([]string{"first", "second", "third"})
	lp.SetGutter(true)
	view := lp.vp.View()
	// Strip ANSI so we compare plain text only.
	for _, want := range []string{"1 │ first", "2 │ second", "3 │ third"} {
		if !strings.Contains(stripANSI(view), want) {
			t.Errorf("gutter view missing %q; got:\n%s", want, stripANSI(view))
		}
	}
}

func TestLogPanel_CopyModeForcesGutter(t *testing.T) {
	lp := NewLogPanel(80, 10)
	lp.SetLines([]string{"a", "b"})
	lp.SetCopyMode(true)
	if !lp.InCopyMode() {
		t.Error("InCopyMode should be true")
	}
	if lp.gutterWidth() == 0 {
		t.Error("copy mode must force gutter on")
	}
	if lp.Sticky() {
		t.Error("copy mode must disable sticky auto-scroll")
	}
	lp.SetCopyMode(false)
	if lp.InCopyMode() {
		t.Error("InCopyMode should be false after exit")
	}
}

func TestLogPanel_GutterShrinksViewportWidth(t *testing.T) {
	lp := NewLogPanel(80, 10)
	lp.SetLines([]string{"x"})
	before := lp.vp.Width
	lp.SetGutter(true)
	after := lp.vp.Width
	if after >= before {
		t.Errorf("viewport width should shrink with gutter: before=%d after=%d", before, after)
	}
}

// stripANSI removes SGR escape sequences from s for substring assertions on
// rendered gutter output.
func stripANSI(s string) string {
	return regexp.MustCompile(`\x1b\[[0-9;]*m`).ReplaceAllString(s, "")
}

// ---- Scrollbar tests ----

func TestScrollbarColumn_HidesWhenFits(t *testing.T) {
	rows := scrollbarColumn(10, 5, 0)
	for i, r := range rows {
		if r != " " {
			t.Errorf("row %d should be blank when content fits, got %q", i, r)
		}
	}
}

func TestScrollbarColumn_ThumbAtTop(t *testing.T) {
	rows := scrollbarColumn(10, 100, 0)
	if !strings.Contains(rows[0], scrollbarThumbGlyph) {
		t.Errorf("thumb should be on row 0 at top, got %q", rows[0])
	}
	if !strings.Contains(rows[len(rows)-1], scrollbarRailGlyph) {
		t.Errorf("last row should be rail at top, got %q", rows[len(rows)-1])
	}
}

func TestScrollbarColumn_ThumbAtBottom(t *testing.T) {
	// offset == total - innerH → thumb at bottom.
	innerH, total := 10, 100
	rows := scrollbarColumn(innerH, total, total-innerH)
	if !strings.Contains(rows[len(rows)-1], scrollbarThumbGlyph) {
		t.Errorf("thumb should touch last row at bottom, got %q", rows[len(rows)-1])
	}
	if !strings.Contains(rows[0], scrollbarRailGlyph) {
		t.Errorf("first row should be rail at bottom, got %q", rows[0])
	}
}

func TestScrollbarColumn_MidPosition(t *testing.T) {
	innerH, total := 10, 100
	rows := scrollbarColumn(innerH, total, (total-innerH)/2)
	// Neither first nor last row should hold the thumb exclusively.
	firstIsThumb := strings.Contains(rows[0], scrollbarThumbGlyph)
	lastIsThumb := strings.Contains(rows[len(rows)-1], scrollbarThumbGlyph)
	if firstIsThumb && lastIsThumb {
		t.Errorf("thumb should not span the full column in mid scroll")
	}
	if firstIsThumb {
		t.Errorf("thumb must not be pinned to top mid-scroll")
	}
	if lastIsThumb {
		t.Errorf("thumb must not be pinned to bottom mid-scroll")
	}
}

func TestLogPanel_View_ShowsThumbWhenOverflow(t *testing.T) {
	lp := NewLogPanel(40, 10)
	lines := make([]string, 200)
	for i := range lines {
		lines[i] = "line"
	}
	lp.SetLines(lines)

	view := lp.View()
	if !strings.Contains(view, scrollbarThumbGlyph) {
		t.Errorf("overflowing log panel should render scrollbar thumb, got: %q", view)
	}
}

func TestLogPanel_View_NoScrollbarWhenFits(t *testing.T) {
	lp := NewLogPanel(40, 10)
	lp.SetLines([]string{"only", "a", "few", "lines"})

	view := lp.View()
	if strings.Contains(view, scrollbarThumbGlyph) {
		t.Errorf("content fits — no thumb should be rendered; got: %q", view)
	}
	// Rail glyph overlaps with the lipgloss border vertical — assert only
	// that the scrollbar helper returns blanks when content fits.
	blanks := scrollbarColumn(8, 4, 0)
	for i, r := range blanks {
		if r != " " {
			t.Errorf("scrollbarColumn row %d should be blank when content fits, got %q", i, r)
		}
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

func TestStatusBar_View_FilterCounter(t *testing.T) {
	sb := &StatusBar{
		Mode:        "NORMAL",
		Total:       1,
		Width:       140,
		FilterText:  "error",
		FilterIndex: 2,
		FilterTotal: 5,
	}
	view := sb.View()
	if !strings.Contains(view, "/error") {
		t.Errorf("filter segment missing pattern: %q", view)
	}
	if !strings.Contains(view, "[2/5]") {
		t.Errorf("filter segment missing counter [2/5]: %q", view)
	}
}

func TestStatusBar_View_FilterNoMatch(t *testing.T) {
	sb := &StatusBar{
		Mode:        "NORMAL",
		Width:       140,
		FilterText:  "xyz",
		FilterTotal: 0,
	}
	view := sb.View()
	if !strings.Contains(view, "[no match]") {
		t.Errorf("want [no match] marker, got: %q", view)
	}
}

func TestStatusBar_FormatRSS(t *testing.T) {
	cases := map[uint64]string{
		512:                "512B",
		1 << 10:            "1K",
		2 << 10:            "2K",
		1 << 20:            "1M",
		128 << 20:          "128M",
		1 << 30:            "1.0G",
		uint64(1.5 * (1 << 30)): "1.5G",
	}
	for in, want := range cases {
		if got := formatRSS(in); got != want {
			t.Errorf("formatRSS(%d): want %q, got %q", in, want, got)
		}
	}
}

func TestStatusBar_View_StatsSegment(t *testing.T) {
	sb := &StatusBar{
		Mode:     "NORMAL",
		Selected: "api",
		Total:    1,
		PID:      1234,
		HasStats: true,
		CPU:      42.3,
		RSS:      128 << 20,
		Width:    160,
	}
	view := sb.View()
	if !strings.Contains(view, "cpu:42.3%") {
		t.Errorf("expected cpu segment, got: %q", view)
	}
	if !strings.Contains(view, "mem:128M") {
		t.Errorf("expected mem segment, got: %q", view)
	}
}

func TestStatusBar_View_StatsHiddenWhenFlagOff(t *testing.T) {
	sb := &StatusBar{Mode: "NORMAL", Width: 140}
	view := sb.View()
	if strings.Contains(view, "cpu:") || strings.Contains(view, "mem:") {
		t.Errorf("stats segment must be hidden when HasStats=false: %q", view)
	}
}

func TestStatusBar_View_CopyCursor(t *testing.T) {
	sb := &StatusBar{
		Mode:       "COPY",
		Width:      140,
		CopyCursor: "L12:3",
	}
	view := sb.View()
	if !strings.Contains(view, "L12:3") {
		t.Errorf("copy cursor segment missing: %q", view)
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
