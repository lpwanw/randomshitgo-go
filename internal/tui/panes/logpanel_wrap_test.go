package panes

import (
	"strings"
	"testing"
)

func TestWrap_ShortLinePassesThrough(t *testing.T) {
	got := wrapLine("hello", 10)
	if len(got) != 1 || got[0] != "hello" {
		t.Errorf("short line: want [hello], got %v", got)
	}
}

func TestWrap_HardSplit(t *testing.T) {
	got := wrapLine("hello world", 5)
	if len(got) != 3 {
		t.Fatalf("want 3 rows, got %d: %v", len(got), got)
	}
	want := []string{"hello", " worl", "d"}
	for i, row := range got {
		if row != want[i] {
			t.Errorf("row %d: want %q, got %q", i, want[i], row)
		}
	}
}

func TestWrap_PreservesANSIAcrossBreak(t *testing.T) {
	in := "\x1b[31mred text that is long\x1b[0m"
	rows := wrapLine(in, 6)
	if len(rows) < 3 {
		t.Fatalf("want ≥3 rows, got %d: %v", len(rows), rows)
	}
	for i, r := range rows {
		if !strings.Contains(r, "\x1b[31m") {
			t.Errorf("row %d missing SGR open: %q", i, r)
		}
	}
}

func TestWrap_ZeroWidthReturnsOriginal(t *testing.T) {
	got := wrapLine("anything", 0)
	if len(got) != 1 || got[0] != "anything" {
		t.Errorf("width=0 should pass through, got %v", got)
	}
}

func TestWrap_VisibleColumnsIgnoresSGR(t *testing.T) {
	if c := visibleColumns("\x1b[31mabc\x1b[0m"); c != 3 {
		t.Errorf("visibleColumns want 3, got %d", c)
	}
}

func TestWrap_PanelRendersWrappedRows(t *testing.T) {
	lp := NewLogPanel(20, 6) // inner width = 20 - 3 = 17
	long := strings.Repeat("x", 50)
	lp.SetLines([]string{long})
	view := lp.vp.View()
	// With wrap on (default) the viewport content should contain multiple rows,
	// each shorter than the raw 50-char line.
	rows := strings.Split(view, "\n")
	if len(rows) < 2 {
		t.Errorf("expected wrapped rows, got single row: %q", view)
	}
}

func TestWrap_SetWrapOffKeepsOneRow(t *testing.T) {
	lp := NewLogPanel(20, 6)
	lp.SetWrap(false)
	long := strings.Repeat("x", 50)
	lp.SetLines([]string{long})
	view := lp.vp.View()
	// With wrap off the single logical line stays one row in content.
	rows := strings.Split(view, "\n")
	nonBlank := 0
	for _, r := range rows {
		if strings.TrimSpace(stripANSI(r)) != "" {
			nonBlank++
		}
	}
	if nonBlank != 1 {
		t.Errorf("wrap off: want 1 non-blank row, got %d (view=%q)", nonBlank, view)
	}
}
