package attach

import (
	"io"
	"strings"
	"testing"
	"time"

	"github.com/charmbracelet/x/ansi"
	"go.uber.org/goleak"
)

// stripANSI returns s with all ANSI escape sequences removed so tests
// can assert on visible characters without coupling to SGR encoding.
func stripANSI(s string) string { return ansi.Strip(s) }

func TestRender_PlainContent(t *testing.T) {
	defer goleak.VerifyNone(t)
	term := NewVTTerm(10, 3, io.Discard)
	defer term.Close()

	if _, err := term.Write([]byte("\x1b[1;1HHello")); err != nil {
		t.Fatalf("Write: %v", err)
	}
	// Allow the input pipe to settle.
	waitFor(t, func() bool {
		c := term.CellAt(0, 0)
		return c != nil && c.Content == "H"
	}, time.Second)

	out := stripANSI(Render(term, 10, 3))
	rows := strings.Split(out, "\n")
	if len(rows) != 3 {
		t.Fatalf("rows = %d, want 3", len(rows))
	}
	if !strings.HasPrefix(rows[0], "Hello") {
		t.Errorf("row 0 = %q, want prefix Hello", rows[0])
	}
	for i, r := range rows {
		if len(r) != 10 {
			t.Errorf("row %d width = %d, want 10", i, len(r))
		}
	}
}

func TestRender_PadsBeyondEmulator(t *testing.T) {
	defer goleak.VerifyNone(t)
	term := NewVTTerm(5, 2, io.Discard)
	defer term.Close()

	out := stripANSI(Render(term, 8, 4))
	rows := strings.Split(out, "\n")
	if len(rows) != 4 {
		t.Fatalf("rows = %d, want 4", len(rows))
	}
	for i, r := range rows {
		if len(r) != 8 {
			t.Errorf("row %d width = %d, want 8", i, len(r))
		}
	}
}

func TestRender_EmptyDimsReturnEmpty(t *testing.T) {
	defer goleak.VerifyNone(t)
	term := NewVTTerm(10, 3, io.Discard)
	defer term.Close()

	if got := Render(term, 0, 5); got != "" {
		t.Errorf("Render(w=0) = %q, want empty", got)
	}
	if got := Render(term, 10, 0); got != "" {
		t.Errorf("Render(h=0) = %q, want empty", got)
	}
	if got := Render(nil, 10, 5); got != "" {
		t.Errorf("Render(nil) = %q, want empty", got)
	}
}

func TestRender_BatchesSameStyleRuns(t *testing.T) {
	defer goleak.VerifyNone(t)
	term := NewVTTerm(20, 1, io.Discard)
	defer term.Close()

	// Three plain (default-style) bytes followed by three more — all
	// share Style.IsZero() and should emit no SGR escapes at all.
	if _, err := term.Write([]byte("\x1b[1;1Habcdef")); err != nil {
		t.Fatalf("Write: %v", err)
	}
	waitFor(t, func() bool {
		c := term.CellAt(5, 0)
		return c != nil && c.Content == "f"
	}, time.Second)

	raw := Render(term, 20, 1)
	// "abcdef" share default style — they must appear contiguously
	// without any SGR escape splitting them. The cursor cell that
	// follows may emit a reverse-video escape; that's fine.
	idx := strings.Index(raw, "abcdef")
	if idx < 0 {
		t.Fatalf("text not found in render: %q", raw)
	}
}

func TestRender_CursorReverseVideo(t *testing.T) {
	defer goleak.VerifyNone(t)
	term := NewVTTerm(5, 1, io.Discard)
	defer term.Close()

	if _, err := term.Write([]byte("\x1b[1;1Hab")); err != nil {
		t.Fatalf("Write: %v", err)
	}
	waitFor(t, func() bool {
		c := term.CellAt(1, 0)
		return c != nil && c.Content == "b"
	}, time.Second)

	// After writing "ab" the cursor sits at (2,0) on a blank cell. The
	// cursor cell should carry the AttrReverse SGR (\x1b[7m).
	raw := Render(term, 5, 1)
	if !strings.Contains(raw, "\x1b[7m") && !strings.Contains(raw, "\x1b[7;") {
		t.Errorf("expected reverse-video SGR for cursor cell, got %q", raw)
	}
}
