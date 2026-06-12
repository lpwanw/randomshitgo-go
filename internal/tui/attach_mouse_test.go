package tui

import (
	"io"
	"os"
	"sync"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/lpwanw/randomshitgo-go/internal/tui/attach"
	"github.com/lpwanw/randomshitgo-go/internal/tui/panes"
)

// fakeClip records the last clipboard write.
type fakeClip struct {
	mu   sync.Mutex
	text string
	err  error
}

func (f *fakeClip) WriteAll(text string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.err != nil {
		return f.err
	}
	f.text = text
	return nil
}

func (f *fakeClip) get() string {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.text
}

// attachTestModel returns a Model in embedded-attach mode wired to a live
// Session whose VTTerm is driven by feed(). origin/size match a 100×30 window.
func attachTestModel(t *testing.T) (m Model, feed func([]byte), ptyRead *os.File) {
	t.Helper()
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatalf("pipe: %v", err)
	}
	t.Cleanup(func() { _ = r.Close(); _ = w.Close() })

	var (
		mu  sync.Mutex
		dst io.Writer
	)
	subscribe := func(_ string, writer io.Writer) (func(), error) {
		mu.Lock()
		dst = writer
		mu.Unlock()
		return func() {}, nil
	}
	feed = func(p []byte) {
		mu.Lock()
		d := dst
		mu.Unlock()
		if d != nil {
			_, _ = d.Write(p)
		}
	}

	m = newTestModel()
	m.width, m.height = 100, 30
	// sidebarW=16 → paneW=84, paneH=29.
	sess, err := attach.NewSession("api", w, 84, 29, subscribe)
	if err != nil {
		t.Fatalf("NewSession: %v", err)
	}
	t.Cleanup(sess.Close)
	m.attach = sess
	m.mode = ModeEmbeddedAttach
	return m, feed, r
}

func TestAttachMouse_LocalSelectCopies(t *testing.T) {
	m, feed, _ := attachTestModel(t)
	fc := &fakeClip{}
	m.SetAttachClipboard(fc)

	feed([]byte("\x1b[1;1Hhello"))
	time.Sleep(20 * time.Millisecond)

	originX := 16 // sidebarW for the test cfg at width 100

	// Press at first cell, drag to 5th cell, release.
	m, _ = feedMsg(m, tea.MouseMsg{X: originX, Y: 0, Button: tea.MouseButtonLeft, Action: tea.MouseActionPress})
	if !m.attachSel.Active {
		t.Fatal("press should activate selection")
	}
	m, _ = feedMsg(m, tea.MouseMsg{X: originX + 4, Y: 0, Button: tea.MouseButtonLeft, Action: tea.MouseActionMotion})
	m, cmd := feedMsg(m, tea.MouseMsg{X: originX + 4, Y: 0, Button: tea.MouseButtonNone, Action: tea.MouseActionRelease})
	if m.attachSel.Active {
		t.Fatal("release should clear selection")
	}
	if cmd == nil {
		t.Fatal("release with drag should yield a yank cmd")
	}
	msg := cmd()
	if _, ok := msg.(panes.CopiedMsg); !ok {
		t.Fatalf("want CopiedMsg, got %T", msg)
	}
	if got := fc.get(); got != "hello" {
		t.Fatalf("clipboard = %q want %q", got, "hello")
	}
}

func TestAttachMouse_BareClickNoCopy(t *testing.T) {
	m, feed, _ := attachTestModel(t)
	fc := &fakeClip{}
	m.SetAttachClipboard(fc)
	feed([]byte("\x1b[1;1Hhello"))
	time.Sleep(20 * time.Millisecond)

	originX := 16
	m, _ = feedMsg(m, tea.MouseMsg{X: originX, Y: 0, Button: tea.MouseButtonLeft, Action: tea.MouseActionPress})
	m, cmd := feedMsg(m, tea.MouseMsg{X: originX, Y: 0, Button: tea.MouseButtonNone, Action: tea.MouseActionRelease})
	if cmd != nil {
		t.Fatal("bare click should not yank")
	}
	if fc.get() != "" {
		t.Fatalf("bare click copied %q, want empty", fc.get())
	}
}

func TestAttachMouse_ForwardsWhenChildTracksMouse(t *testing.T) {
	m, feed, ptyRead := attachTestModel(t)
	fc := &fakeClip{}
	m.SetAttachClipboard(fc)

	// Child enables Normal mouse tracking + SGR ext encoding.
	feed([]byte("\x1b[?1000h\x1b[?1006h"))
	deadline := time.Now().Add(time.Second)
	for !m.attach.MouseTracking() && time.Now().Before(deadline) {
		time.Sleep(2 * time.Millisecond)
	}
	if !m.attach.MouseTracking() {
		t.Fatal("child mouse tracking should be on")
	}

	// A left click should be forwarded to the PTY, not local-selected.
	m, _ = feedMsg(m, tea.MouseMsg{X: 16, Y: 0, Button: tea.MouseButtonLeft, Action: tea.MouseActionPress})
	if m.attachSel.Active {
		t.Fatal("should not local-select while child tracks mouse")
	}

	// Read the forwarded SGR mouse sequence from the PTY.
	got := readWithTimeout(t, ptyRead, time.Second)
	if !containsSGRMouse(got) {
		t.Fatalf("expected SGR mouse report on PTY, got %q", got)
	}
	if fc.get() != "" {
		t.Fatalf("forwarding should not touch clipboard, got %q", fc.get())
	}
}

// readWithTimeout reads available bytes from r, returning what arrived before
// the deadline.
func readWithTimeout(t *testing.T, r *os.File, d time.Duration) string {
	t.Helper()
	ch := make(chan string, 1)
	go func() {
		buf := make([]byte, 256)
		n, _ := r.Read(buf)
		ch <- string(buf[:n])
	}()
	select {
	case s := <-ch:
		return s
	case <-time.After(d):
		return ""
	}
}

// containsSGRMouse reports whether s holds an SGR mouse report (ESC [ < ... M/m).
func containsSGRMouse(s string) bool {
	i := 0
	for i < len(s) {
		if s[i] == 0x1b && i+2 < len(s) && s[i+1] == '[' && s[i+2] == '<' {
			return true
		}
		i++
	}
	return false
}
