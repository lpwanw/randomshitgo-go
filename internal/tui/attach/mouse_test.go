package attach

import (
	"io"
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	uv "github.com/charmbracelet/ultraviolet"
	"go.uber.org/goleak"
)

func TestSelection_Normalized(t *testing.T) {
	// Backwards drag (cursor before anchor) normalises to reading order.
	s := Selection{Anchor: Cell{X: 5, Y: 2}, Cursor: Cell{X: 1, Y: 1}, Active: true}
	lo, hi := s.Normalized()
	if lo != (Cell{X: 1, Y: 1}) || hi != (Cell{X: 5, Y: 2}) {
		t.Fatalf("Normalized = %v,%v", lo, hi)
	}
	// Same row, anchor right of cursor.
	s = Selection{Anchor: Cell{X: 9, Y: 0}, Cursor: Cell{X: 2, Y: 0}, Active: true}
	lo, hi = s.Normalized()
	if lo.X != 2 || hi.X != 9 {
		t.Fatalf("same-row Normalized = %v,%v", lo, hi)
	}
}

func TestSelection_Contains(t *testing.T) {
	s := Selection{Anchor: Cell{X: 2, Y: 1}, Cursor: Cell{X: 4, Y: 3}, Active: true}
	cases := []struct {
		x, y int
		want bool
	}{
		{2, 1, true},    // lo edge
		{1, 1, false},   // before lo on lo row
		{0, 2, true},    // full middle row, col 0
		{99, 2, true},   // full middle row, far col
		{4, 3, true},    // hi edge
		{5, 3, false},   // after hi on hi row
		{0, 0, false},   // above
		{0, 4, false},   // below
	}
	for _, c := range cases {
		if got := s.Contains(c.x, c.y); got != c.want {
			t.Errorf("Contains(%d,%d)=%v want %v", c.x, c.y, got, c.want)
		}
	}
	// Inactive selection contains nothing.
	if (Selection{Anchor: Cell{0, 0}, Cursor: Cell{9, 9}}).Contains(3, 3) {
		t.Error("inactive selection should contain nothing")
	}
}

func TestMsgToMouse_OriginAndButtons(t *testing.T) {
	// Left press at abs (12,5) with pane origin (10,0) → cell (2,5) click.
	ev, ok := MsgToMouse(tea.MouseMsg{X: 12, Y: 5, Button: tea.MouseButtonLeft, Action: tea.MouseActionPress}, 10, 0)
	if !ok {
		t.Fatal("MsgToMouse press not ok")
	}
	click, isClick := ev.(uv.MouseClickEvent)
	if !isClick {
		t.Fatalf("want MouseClickEvent, got %T", ev)
	}
	if click.X != 2 || click.Y != 5 || click.Button != uv.MouseLeft {
		t.Fatalf("click = %+v", click)
	}

	// Motion → MouseMotionEvent; mods carried.
	ev, _ = MsgToMouse(tea.MouseMsg{X: 11, Y: 1, Button: tea.MouseButtonLeft, Action: tea.MouseActionMotion, Ctrl: true}, 10, 0)
	if mo, okm := ev.(uv.MouseMotionEvent); !okm || !mo.Mod.Contains(uv.ModCtrl) {
		t.Fatalf("motion = %T %+v", ev, ev)
	}

	// Release → MouseReleaseEvent.
	ev, _ = MsgToMouse(tea.MouseMsg{X: 10, Y: 0, Button: tea.MouseButtonNone, Action: tea.MouseActionRelease}, 10, 0)
	if _, okr := ev.(uv.MouseReleaseEvent); !okr {
		t.Fatalf("release = %T", ev)
	}

	// Wheel press → MouseWheelEvent.
	ev, _ = MsgToMouse(tea.MouseMsg{X: 15, Y: 3, Button: tea.MouseButtonWheelUp, Action: tea.MouseActionPress}, 10, 0)
	if _, okw := ev.(uv.MouseWheelEvent); !okw {
		t.Fatalf("wheel = %T", ev)
	}
}

func TestMsgToMouse_OutOfPane(t *testing.T) {
	if _, ok := MsgToMouse(tea.MouseMsg{X: 5, Y: 5, Button: tea.MouseButtonLeft, Action: tea.MouseActionPress}, 10, 0); ok {
		t.Error("X left of origin should reject")
	}
	if _, ok := MsgToMouse(tea.MouseMsg{X: 12, Y: -1, Button: tea.MouseButtonLeft, Action: tea.MouseActionPress}, 10, 0); ok {
		t.Error("negative relative Y should reject")
	}
}

func TestSelectionText_SingleAndMultiRow(t *testing.T) {
	defer goleak.VerifyNone(t)
	term := NewVTTerm(20, 5, io.Discard)
	defer term.Close()
	// Row 0: "hello world", row 1: "foo".
	_, _ = term.Write([]byte("\x1b[1;1Hhello world\x1b[2;1Hfoo"))
	time.Sleep(20 * time.Millisecond)

	// Single row sub-span "world" = cols 6..10.
	got := SelectionText(term, Selection{Anchor: Cell{6, 0}, Cursor: Cell{10, 0}, Active: true})
	if got != "world" {
		t.Fatalf("single-row = %q want %q", got, "world")
	}

	// Multi-row: from (6,0) to (2,1) → "world" + rest-of-row0 (trimmed) + row1 up to col2.
	got = SelectionText(term, Selection{Anchor: Cell{6, 0}, Cursor: Cell{2, 1}, Active: true})
	if got != "world\nfoo" {
		t.Fatalf("multi-row = %q want %q", got, "world\nfoo")
	}

	// Inactive → empty.
	if SelectionText(term, Selection{Anchor: Cell{0, 0}, Cursor: Cell{5, 0}}) != "" {
		t.Error("inactive SelectionText should be empty")
	}
}

func TestVTTerm_MouseTracking(t *testing.T) {
	defer goleak.VerifyNone(t)
	term := NewVTTerm(80, 24, io.Discard)
	defer term.Close()

	if term.MouseTracking() {
		t.Fatal("mouse tracking should start off")
	}
	// Enable button-event tracking (?1002h).
	_, _ = term.Write([]byte("\x1b[?1002h"))
	waitFor(t, term.MouseTracking, time.Second)

	// Also enable SGR ext (?1006h) — not a tracking mode, should stay on.
	_, _ = term.Write([]byte("\x1b[?1006h"))
	if !term.MouseTracking() {
		t.Fatal("tracking should remain on after SGR ext")
	}
	// Disable button-event tracking (?1002l) → back off.
	_, _ = term.Write([]byte("\x1b[?1002l"))
	waitFor(t, func() bool { return !term.MouseTracking() }, time.Second)
}

func TestRenderWithSelection_Highlights(t *testing.T) {
	defer goleak.VerifyNone(t)
	term := NewVTTerm(10, 2, io.Discard)
	defer term.Close()
	_, _ = term.Write([]byte("\x1b[1;1Habcde"))
	time.Sleep(20 * time.Millisecond)

	plain := Render(term, 10, 2)
	sel := RenderWithSelection(term, 10, 2, Selection{Anchor: Cell{0, 0}, Cursor: Cell{2, 0}, Active: true})
	if sel == plain {
		t.Fatal("selection render should differ from plain")
	}
	// Reverse-video SGR (\x1b[7m) must appear when a selection is active.
	if !strings.Contains(sel, "\x1b[7m") {
		t.Fatalf("expected reverse SGR in selection render, got %q", sel)
	}
}
