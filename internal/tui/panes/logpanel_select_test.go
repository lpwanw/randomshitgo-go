package panes

import (
	"errors"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
)

type fakeClip struct {
	text string
	fail error
}

func (f *fakeClip) WriteAll(s string) error {
	if f.fail != nil {
		return f.fail
	}
	f.text = s
	return nil
}

func runYank(t *testing.T, lp *LogPanel) tea.Msg {
	t.Helper()
	cmd := lp.HandleCopyKey(keyMsg("y"))
	if cmd == nil {
		t.Fatal("y should produce a tea.Cmd")
	}
	return cmd()
}

// runYY yanks the current line via `yy` (vim-style) for tests that exercise
// the no-selection yank path — replacing the old bare-`y` shortcut.
func runYY(t *testing.T, lp *LogPanel) tea.Msg {
	t.Helper()
	if cmd := lp.HandleCopyKey(keyMsg("y")); cmd != nil {
		t.Fatal("first y in yy should not yet produce a tea.Cmd")
	}
	cmd := lp.HandleCopyKey(keyMsg("y"))
	if cmd == nil {
		t.Fatal("second y in yy should produce a tea.Cmd")
	}
	return cmd()
}

func TestYank_YY_CopiesCurrentLine(t *testing.T) {
	lp := newCopyPanel(t, []string{"first", "second", "third"})
	fc := &fakeClip{}
	lp.SetClipboard(fc)
	lp.HandleCopyKey(keyMsg("j"))
	msg := runYY(t, lp)
	if _, ok := msg.(CopiedMsg); !ok {
		t.Fatalf("want CopiedMsg, got %T", msg)
	}
	if fc.text != "second" {
		t.Errorf("want clipboard=%q, got %q", "second", fc.text)
	}
}

func TestYank_CharwiseSingleLine(t *testing.T) {
	lp := newCopyPanel(t, []string{"foo bar baz"})
	fc := &fakeClip{}
	lp.SetClipboard(fc)
	// Select "bar" — anchor at col 4, cursor at col 6.
	lp.HandleCopyKey(keyMsg("l"))
	lp.HandleCopyKey(keyMsg("l"))
	lp.HandleCopyKey(keyMsg("l"))
	lp.HandleCopyKey(keyMsg("l")) // col=4
	lp.HandleCopyKey(keyMsg("v"))
	lp.HandleCopyKey(keyMsg("l"))
	lp.HandleCopyKey(keyMsg("l")) // col=6
	runYank(t, lp)
	if fc.text != "bar" {
		t.Errorf("want clipboard=%q, got %q", "bar", fc.text)
	}
}

func TestYank_Linewise_MultiLine(t *testing.T) {
	lp := newCopyPanel(t, []string{"alpha", "beta", "gamma", "delta"})
	fc := &fakeClip{}
	lp.SetClipboard(fc)
	lp.HandleCopyKey(keyMsg("V"))
	lp.HandleCopyKey(keyMsg("j"))
	lp.HandleCopyKey(keyMsg("j")) // lines 0..2
	runYank(t, lp)
	want := strings.Join([]string{"alpha", "beta", "gamma"}, "\n")
	if fc.text != want {
		t.Errorf("want clipboard=%q, got %q", want, fc.text)
	}
}

func TestYank_YY_ReportsLineMeta(t *testing.T) {
	lp := newCopyPanel(t, []string{"hello"})
	fc := &fakeClip{}
	lp.SetClipboard(fc)
	msg := runYY(t, lp)
	copied, ok := msg.(CopiedMsg)
	if !ok {
		t.Fatalf("want CopiedMsg, got %T", msg)
	}
	if copied.Lines != 1 || copied.Chars != 5 {
		t.Errorf("meta mismatch: %+v", copied)
	}
}

func TestYank_ClipboardErrorSurfaces(t *testing.T) {
	lp := newCopyPanel(t, []string{"hi"})
	lp.SetClipboard(&fakeClip{fail: errors.New("no pbcopy")})
	msg := runYY(t, lp)
	fail, ok := msg.(CopyFailedMsg)
	if !ok {
		t.Fatalf("want CopyFailedMsg, got %T", msg)
	}
	if !strings.Contains(fail.Err, "no pbcopy") {
		t.Errorf("want error to contain 'no pbcopy', got %q", fail.Err)
	}
}

func TestSel_CharMultiLineSpansLineBreak(t *testing.T) {
	lp := newCopyPanel(t, []string{"foo bar", "baz qux"})
	fc := &fakeClip{}
	lp.SetClipboard(fc)
	// anchor at (0,4) start of "bar"
	lp.HandleCopyKey(keyMsg("l"))
	lp.HandleCopyKey(keyMsg("l"))
	lp.HandleCopyKey(keyMsg("l"))
	lp.HandleCopyKey(keyMsg("l"))
	lp.HandleCopyKey(keyMsg("v"))
	lp.HandleCopyKey(keyMsg("j"))
	lp.HandleCopyKey(keyMsg("$"))
	runYank(t, lp)
	want := "bar\nbaz qux"
	if fc.text != want {
		t.Errorf("want %q, got %q", want, fc.text)
	}
}

func TestYank_BigYCopiesCurrentLineLinewise(t *testing.T) {
	lp := newCopyPanel(t, []string{"one", "two", "three"})
	fc := &fakeClip{}
	lp.SetClipboard(fc)
	lp.HandleCopyKey(keyMsg("j"))
	cmd := lp.HandleCopyKey(keyMsg("Y"))
	if cmd == nil {
		t.Fatal("Y should produce a tea.Cmd")
	}
	msg := cmd()
	if _, ok := msg.(CopiedMsg); !ok {
		t.Fatalf("want CopiedMsg, got %T", msg)
	}
	if fc.text != "two" {
		t.Errorf("want clipboard=%q, got %q", "two", fc.text)
	}
}

func TestSel_RendersSelectionSGR(t *testing.T) {
	lp := newCopyPanel(t, []string{"hello world"})
	lp.HandleCopyKey(keyMsg("V"))
	view := lp.vp.View()
	if !strings.Contains(view, "\x1b[7m") || !strings.Contains(view, "\x1b[27m") {
		t.Errorf("linewise selection must emit reverse-video SGR; got %q", view)
	}
}
