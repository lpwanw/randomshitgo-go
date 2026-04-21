package panes

import (
	"strings"
	"testing"
)

// seed drives a sequence of single-char keystrokes through HandleCopyKey and
// returns the last tea.Cmd (may be nil for non-terminal keys). Used by the
// operator/state-machine tests below.
func drive(t *testing.T, lp *LogPanel, keys string) *fakeClip {
	t.Helper()
	fc := &fakeClip{}
	lp.SetClipboard(fc)
	for _, r := range keys {
		cmd := lp.HandleCopyKey(keyMsg(string(r)))
		if cmd != nil {
			cmd()
		}
	}
	return fc
}

func TestOp_Yiw(t *testing.T) {
	lp := newCopyPanel(t, []string{"foo BAR baz"})
	// Cursor at col 5 (inside "BAR").
	lp.HandleCopyKey(keyMsg("l"))
	lp.HandleCopyKey(keyMsg("l"))
	lp.HandleCopyKey(keyMsg("l"))
	lp.HandleCopyKey(keyMsg("l"))
	lp.HandleCopyKey(keyMsg("l"))
	fc := drive(t, lp, "yiw")
	if fc.text != "BAR" {
		t.Errorf("yiw: want %q, got %q", "BAR", fc.text)
	}
}

func TestOp_YaQuote(t *testing.T) {
	lp := newCopyPanel(t, []string{`log="hello world" rest`})
	// Cursor at col 6 — inside the quoted string.
	for i := 0; i < 6; i++ {
		lp.HandleCopyKey(keyMsg("l"))
	}
	fc := drive(t, lp, `ya"`)
	want := `"hello world" `
	if fc.text != want {
		t.Errorf(`ya": want %q, got %q`, want, fc.text)
	}
}

func TestOp_YiParen(t *testing.T) {
	lp := newCopyPanel(t, []string{"fn(alpha, beta)"})
	// Cursor at col 4 (inside parens).
	for i := 0; i < 4; i++ {
		lp.HandleCopyKey(keyMsg("l"))
	}
	fc := drive(t, lp, "yi(")
	if fc.text != "alpha, beta" {
		t.Errorf("yi(: want %q, got %q", "alpha, beta", fc.text)
	}
}

func TestOp_YiBrace(t *testing.T) {
	lp := newCopyPanel(t, []string{"config={key:val}"})
	for i := 0; i < 10; i++ {
		lp.HandleCopyKey(keyMsg("l"))
	}
	fc := drive(t, lp, "yi{")
	if fc.text != "key:val" {
		t.Errorf("yi{: want %q, got %q", "key:val", fc.text)
	}
}

func TestOp_YY_LinewiseWithCount(t *testing.T) {
	lp := newCopyPanel(t, []string{"one", "two", "three", "four"})
	fc := drive(t, lp, "2yy")
	want := strings.Join([]string{"one", "two"}, "\n")
	if fc.text != want {
		t.Errorf("2yy: want %q, got %q", want, fc.text)
	}
}

func TestOp_YWithCount(t *testing.T) {
	lp := newCopyPanel(t, []string{"alpha bravo charlie delta"})
	fc := drive(t, lp, "y3w")
	want := "alpha bravo charlie "
	if fc.text != want {
		t.Errorf("y3w: want %q, got %q", want, fc.text)
	}
}

func TestOp_EscCancelsPending(t *testing.T) {
	lp := newCopyPanel(t, []string{"foo bar"})
	// Arm an operator: `y`.
	lp.HandleCopyKey(keyMsg("y"))
	if lp.op != opYank {
		t.Fatalf("op should be opYank, got %v", lp.op)
	}
	// ClearPending (what routeLogFocus calls on Esc) should clear op.
	if !lp.ClearPending() {
		t.Fatal("ClearPending should report state was cleared")
	}
	if lp.op != opNone || lp.pending != pendNone {
		t.Errorf("state not cleared: op=%v pending=%v", lp.op, lp.pending)
	}
}

func TestSM_CountAccumulatesAndResets(t *testing.T) {
	lp := newCopyPanel(t, []string{"alpha bravo charlie delta echo"})
	lp.HandleCopyKey(keyMsg("3"))
	if lp.count != 3 {
		t.Fatalf("count after 3: want 3, got %d", lp.count)
	}
	lp.HandleCopyKey(keyMsg("w"))
	// alpha(0-4) bravo(6-10) charlie(12-18) delta(20-24) → 3w lands on 'd'.
	if line, col := lp.Cursor(); line != 0 || col != 20 {
		t.Errorf("3w: want col=20 (start of delta), got (%d,%d)", line, col)
	}
	if lp.count != 0 {
		t.Errorf("count should reset after motion, got %d", lp.count)
	}
}

func TestSM_ZeroMotionWithoutCount(t *testing.T) {
	lp := newCopyPanel(t, []string{"  indented"})
	lp.HandleCopyKey(keyMsg("$"))
	lp.HandleCopyKey(keyMsg("0"))
	if _, col := lp.Cursor(); col != 0 {
		t.Errorf("0 as motion: want col=0, got %d", col)
	}
}

func TestSM_ZeroAsCountDigit(t *testing.T) {
	lp := newCopyPanel(t, []string{"foo bar baz qux"})
	lp.HandleCopyKey(keyMsg("1"))
	lp.HandleCopyKey(keyMsg("0"))
	if lp.count != 10 {
		t.Errorf("count after 10: want 10, got %d", lp.count)
	}
}

func TestMotion_WordVsWORD(t *testing.T) {
	lp := newCopyPanel(t, []string{"foo-bar baz"})
	// `w` stops at '-' (col 3).
	lp.HandleCopyKey(keyMsg("w"))
	if _, col := lp.Cursor(); col != 3 {
		t.Errorf("w on foo-bar: want col=3, got %d", col)
	}
	// Reset cursor.
	lp.cur = cursor{0, 0}
	lp.HandleCopyKey(keyMsg("W"))
	if _, col := lp.Cursor(); col != 8 {
		t.Errorf("W on foo-bar baz: want col=8, got %d", col)
	}
}

func TestMotion_EndOfWord(t *testing.T) {
	lp := newCopyPanel(t, []string{"foo bar"})
	lp.HandleCopyKey(keyMsg("e"))
	if _, col := lp.Cursor(); col != 2 {
		t.Errorf("e on foo bar: want col=2 (end of foo), got %d", col)
	}
}

func TestMotion_Ge(t *testing.T) {
	lp := newCopyPanel(t, []string{"foo bar"})
	lp.HandleCopyKey(keyMsg("$")) // col=7
	lp.HandleCopyKey(keyMsg("g"))
	lp.HandleCopyKey(keyMsg("e"))
	// End of prev word (bar end) = col 6.
	if _, col := lp.Cursor(); col != 6 {
		t.Errorf("ge from eol: want col=6, got %d", col)
	}
}

func TestMotion_FirstNonBlank(t *testing.T) {
	lp := newCopyPanel(t, []string{"   hello"})
	lp.HandleCopyKey(keyMsg("^"))
	if _, col := lp.Cursor(); col != 3 {
		t.Errorf("^ first-non-blank: want col=3, got %d", col)
	}
}

func TestFind_ForwardF(t *testing.T) {
	lp := newCopyPanel(t, []string{"foo.bar.baz"})
	lp.HandleCopyKey(keyMsg("f"))
	lp.HandleCopyKey(keyMsg("."))
	if _, col := lp.Cursor(); col != 3 {
		t.Errorf("f.: want col=3, got %d", col)
	}
	lp.HandleCopyKey(keyMsg(";"))
	if _, col := lp.Cursor(); col != 7 {
		t.Errorf("; (repeat f.): want col=7, got %d", col)
	}
}

func TestFind_Till(t *testing.T) {
	lp := newCopyPanel(t, []string{"foo.bar"})
	lp.HandleCopyKey(keyMsg("t"))
	lp.HandleCopyKey(keyMsg("."))
	if _, col := lp.Cursor(); col != 2 {
		t.Errorf("t.: want col=2, got %d", col)
	}
}

func TestFind_Backward(t *testing.T) {
	lp := newCopyPanel(t, []string{"foo.bar.baz"})
	lp.HandleCopyKey(keyMsg("$"))
	lp.HandleCopyKey(keyMsg("F"))
	lp.HandleCopyKey(keyMsg("."))
	// From col=11 (eol) search backward for '.' → col 7.
	if _, col := lp.Cursor(); col != 7 {
		t.Errorf("F. from eol: want col=7, got %d", col)
	}
}

func TestFind_CommaReverses(t *testing.T) {
	lp := newCopyPanel(t, []string{"foo.bar.baz"})
	lp.HandleCopyKey(keyMsg("f"))
	lp.HandleCopyKey(keyMsg("."))
	lp.HandleCopyKey(keyMsg(";"))
	// Now on col 7. Reverse direction.
	lp.HandleCopyKey(keyMsg(","))
	if _, col := lp.Cursor(); col != 3 {
		t.Errorf(", reverse find: want col=3, got %d", col)
	}
}

func TestFind_Count(t *testing.T) {
	lp := newCopyPanel(t, []string{"a.b.c.d.e"})
	lp.HandleCopyKey(keyMsg("3"))
	lp.HandleCopyKey(keyMsg("f"))
	lp.HandleCopyKey(keyMsg("."))
	// Third '.' is at col 5.
	if _, col := lp.Cursor(); col != 5 {
		t.Errorf("3f.: want col=5, got %d", col)
	}
}

func TestPendingSummary(t *testing.T) {
	lp := newCopyPanel(t, []string{"abc"})
	if got := lp.PendingSummary(); got != "" {
		t.Errorf("empty: want \"\", got %q", got)
	}
	lp.HandleCopyKey(keyMsg("3"))
	if got := lp.PendingSummary(); got != "3" {
		t.Errorf("after 3: want \"3\", got %q", got)
	}
	lp.HandleCopyKey(keyMsg("y"))
	if got := lp.PendingSummary(); got != "3y" {
		t.Errorf("after 3y: want \"3y\", got %q", got)
	}
	lp.HandleCopyKey(keyMsg("i"))
	if got := lp.PendingSummary(); got != "3yi" {
		t.Errorf("after 3yi: want \"3yi\", got %q", got)
	}
}
