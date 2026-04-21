package tui

import (
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/lpwanw/randomshitgo-go/internal/tui/panes"
)

type e2eClip struct{ got string }

func (c *e2eClip) WriteAll(s string) error { c.got = s; return nil }

func keyRune(r rune) tea.KeyMsg { return tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{r}} }

func keyTab() tea.KeyMsg { return tea.KeyMsg{Type: tea.KeyTab} }
func keyEsc() tea.KeyMsg { return tea.KeyMsg{Type: tea.KeyEsc} }

func TestCopyMode_EndToEnd_Yank(t *testing.T) {
	m := newTestModel()
	m.logPanel.SetLines([]string{"alpha", "beta", "gamma", "delta"})
	fake := &e2eClip{}
	m.logPanel.SetClipboard(fake)

	// Enter log focus.
	m, _ = feedMsg(m, keyTab())
	if m.mode != ModeLogFocus {
		t.Fatalf("after Tab: want ModeLogFocus, got %v", m.mode)
	}
	if !m.logPanel.InCopyMode() {
		t.Fatal("logPanel should be in copy mode")
	}

	// Linewise select 3 lines: V, j, j.
	m, _ = feedMsg(m, keyRune('V'))
	m, _ = feedMsg(m, keyRune('j'))
	m, _ = feedMsg(m, keyRune('j'))

	// Yank.
	_, cmd := feedMsg(m, keyRune('y'))
	if cmd == nil {
		t.Fatal("y should return a tea.Cmd")
	}
	copied, ok := cmd().(panes.CopiedMsg)
	if !ok {
		t.Fatalf("expected CopiedMsg, got %T", cmd())
	}
	if copied.Lines != 3 {
		t.Errorf("want 3 lines, got %d", copied.Lines)
	}
	want := strings.Join([]string{"alpha", "beta", "gamma"}, "\n")
	if fake.got != want {
		t.Errorf("clipboard mismatch\nwant: %q\ngot:  %q", want, fake.got)
	}

	// CopiedMsg should clear the selection but KEEP log focus so the user can
	// keep browsing and yank again.
	m, _ = feedMsg(m, copied)
	if m.mode != ModeLogFocus {
		t.Errorf("after CopiedMsg: want ModeLogFocus, got %v", m.mode)
	}
	if !m.logPanel.InCopyMode() {
		t.Error("logPanel should stay in copy mode after yank")
	}
	if m.logPanel.HasSelection() {
		t.Error("selection should be cleared after yank")
	}
}

func TestCopyMode_DoubleEscExits(t *testing.T) {
	m := newTestModel()
	m.logPanel.SetLines([]string{"a", "b"})
	m, _ = feedMsg(m, keyTab())
	if m.mode != ModeLogFocus {
		t.Fatalf("setup: want ModeLogFocus, got %v", m.mode)
	}
	// First Esc arms only — mode must not flip yet.
	m, _ = feedMsg(m, keyEsc())
	if m.mode != ModeLogFocus {
		t.Errorf("first Esc should arm, got mode=%v", m.mode)
	}
	// Second Esc within window exits.
	m, _ = feedMsg(m, keyEsc())
	if m.mode != ModeNormal {
		t.Errorf("second Esc should return to Normal, got %v", m.mode)
	}
	if m.logPanel.InCopyMode() {
		t.Error("logPanel should exit copy mode")
	}
}

func TestLogFocus_SelectionEscCancels(t *testing.T) {
	m := newTestModel()
	m.logPanel.SetLines([]string{"a", "b"})
	m, _ = feedMsg(m, keyTab())

	m, _ = feedMsg(m, keyRune('v'))
	if !m.logPanel.HasSelection() {
		t.Fatal("v should start a selection")
	}

	m, _ = feedMsg(m, keyEsc())
	if m.logPanel.HasSelection() {
		t.Error("first Esc with selection must clear it")
	}
	if m.mode != ModeLogFocus {
		t.Errorf("selection-cancel Esc must stay in log focus, got mode=%v", m.mode)
	}
	if !m.logEscArmedAt.IsZero() {
		t.Error("selection-cancel Esc must NOT arm the exit")
	}
}

func TestLogFocus_NonEscDisarms(t *testing.T) {
	m := newTestModel()
	m.logPanel.SetLines([]string{"a", "b"})
	m, _ = feedMsg(m, keyTab())

	m, _ = feedMsg(m, keyEsc())
	if m.logEscArmedAt.IsZero() {
		t.Fatal("first Esc should arm")
	}
	m, _ = feedMsg(m, keyRune('j'))
	if !m.logEscArmedAt.IsZero() {
		t.Error("motion key should disarm")
	}
	// Another Esc must re-arm, not exit.
	m, _ = feedMsg(m, keyEsc())
	if m.mode != ModeLogFocus {
		t.Errorf("post-disarm Esc should re-arm, not exit; mode=%v", m.mode)
	}
}

func TestLogFocus_StatusLabelLogToCopy(t *testing.T) {
	m := newTestModel()
	m.logPanel.SetLines([]string{"hello"})
	m, _ = feedMsg(m, keyTab())
	if got := m.displayMode(); got != "LOG" {
		t.Errorf("focus no-selection label: want LOG, got %q", got)
	}
	m, _ = feedMsg(m, keyRune('v'))
	if got := m.displayMode(); got != "COPY" {
		t.Errorf("focus with-selection label: want COPY, got %q", got)
	}
}

func TestLogFocus_NJumpsCursor(t *testing.T) {
	m := newTestModel()
	m.logPanel.SetLines([]string{"alpha", "err one", "gamma", "err two", "delta"})
	// Set filter so handleJumpMatch doesn't short-circuit.
	m.ui.SetFilter("err")
	m.logPanel.SetFilter(m.ui.Snapshot().FilterRegex)

	m, _ = feedMsg(m, keyTab())
	m, _ = feedMsg(m, keyRune('n'))

	line, _ := m.logPanel.Cursor()
	if line != 1 {
		t.Errorf("n should jump cursor to first match (line 1), got %d", line)
	}
	m, _ = feedMsg(m, keyRune('n'))
	line, _ = m.logPanel.Cursor()
	if line != 3 {
		t.Errorf("second n should advance to line 3, got %d", line)
	}
}

func TestSetNu_TogglesGutter(t *testing.T) {
	m := newTestModel()
	m.logPanel.SetLines([]string{"one", "two"})

	// Dispatch :set nu and verify the renderer gains a gutter separator.
	m, _ = feedMsg(m, overlaysPkgCommandRun("set nu"))
	view := m.logPanel.View()
	if !strings.Contains(view, "│") {
		// "│" is used by both gutter and scrollbar rail; assert a line-number
		// digit prefix too.
		if !strings.Contains(stripSGR(view), "1 │ ") {
			t.Errorf("expected gutter line numbers after :set nu; got:\n%s", view)
		}
	}

	m, _ = feedMsg(m, overlaysPkgCommandRun("set nonu"))
	view = m.logPanel.View()
	if strings.Contains(stripSGR(view), "1 │ one") {
		t.Errorf("gutter should be gone after :set nonu; got:\n%s", view)
	}
}

// stripSGR removes SGR escape sequences for substring-based assertions.
func stripSGR(s string) string {
	var b strings.Builder
	for i := 0; i < len(s); {
		if s[i] == 0x1b && i+1 < len(s) && s[i+1] == '[' {
			j := i + 2
			for j < len(s) && !(s[j] >= '@' && s[j] <= '~') {
				j++
			}
			if j < len(s) {
				i = j + 1
				continue
			}
		}
		b.WriteByte(s[i])
		i++
	}
	return b.String()
}
