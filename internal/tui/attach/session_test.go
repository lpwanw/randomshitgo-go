package attach

import (
	"errors"
	"io"
	"os"
	"sync"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"go.uber.org/goleak"
)

// fakeSubscribe routes bytes the test writes through `feed` directly
// into the supplied writer (the VTTerm). The returned unsubscribe func
// flips a flag so the test can verify Close unsubscribed.
func fakeSubscribe(t *testing.T) (subscribe SubscribeFn, feed func([]byte), unsubbed func() bool) {
	t.Helper()
	var (
		mu       sync.Mutex
		w        io.Writer
		didUnsub bool
	)
	subscribe = func(_ string, writer io.Writer) (func(), error) {
		mu.Lock()
		w = writer
		mu.Unlock()
		return func() {
			mu.Lock()
			didUnsub = true
			w = nil
			mu.Unlock()
		}, nil
	}
	feed = func(p []byte) {
		mu.Lock()
		dst := w
		mu.Unlock()
		if dst == nil {
			return
		}
		_, _ = dst.Write(p)
	}
	unsubbed = func() bool {
		mu.Lock()
		defer mu.Unlock()
		return didUnsub
	}
	return
}

// pipePTY returns a *os.File wrapping the write end of a pipe plus a
// reader so tests can assert what bytes Session writes to the PTY.
func pipePTY(t *testing.T) (writeFile *os.File, readFile *os.File) {
	t.Helper()
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatalf("pipe: %v", err)
	}
	t.Cleanup(func() {
		_ = r.Close()
		_ = w.Close()
	})
	return w, r
}

func TestSession_LifecycleSubscribesAndUnsubscribes(t *testing.T) {
	defer goleak.VerifyNone(t)
	ptmx, _ := pipePTY(t)
	subscribe, feed, unsubbed := fakeSubscribe(t)

	sess, err := NewSession("p1", ptmx, 80, 24, subscribe)
	if err != nil {
		t.Fatalf("NewSession: %v", err)
	}

	// Feed PTY → emulator.
	feed([]byte("\x1b[1;1HHi"))
	waitFor(t, func() bool {
		c := sess.Term().CellAt(0, 0)
		return c != nil && c.Content == "H"
	}, time.Second)

	if unsubbed() {
		t.Fatal("unsubscribed before Close")
	}
	sess.Close()
	if !unsubbed() {
		t.Fatal("Close did not unsubscribe")
	}

	// Idempotent.
	sess.Close()
}

func TestSession_NewSessionRejectsNilArgs(t *testing.T) {
	defer goleak.VerifyNone(t)
	if _, err := NewSession("p1", nil, 80, 24, func(string, io.Writer) (func(), error) { return func() {}, nil }); err == nil {
		t.Error("expected error for nil ptmx")
	}
	ptmx, _ := pipePTY(t)
	if _, err := NewSession("p1", ptmx, 80, 24, nil); err == nil {
		t.Error("expected error for nil subscribe")
	}
}

func TestSession_SubscribeErrorPropagates(t *testing.T) {
	defer goleak.VerifyNone(t)
	ptmx, _ := pipePTY(t)
	wantErr := errors.New("boom")
	sub := func(string, io.Writer) (func(), error) { return nil, wantErr }
	if _, err := NewSession("p1", ptmx, 80, 24, sub); !errors.Is(err, wantErr) {
		t.Errorf("err = %v, want %v", err, wantErr)
	}
}

func TestSession_SendBytesWritesToPTY(t *testing.T) {
	defer goleak.VerifyNone(t)
	ptmx, ptyRead := pipePTY(t)
	subscribe, _, _ := fakeSubscribe(t)
	sess, err := NewSession("p1", ptmx, 80, 24, subscribe)
	if err != nil {
		t.Fatalf("NewSession: %v", err)
	}
	defer sess.Close()

	if err := sess.SendBytes([]byte("hi")); err != nil {
		t.Fatalf("SendBytes: %v", err)
	}
	buf := make([]byte, 2)
	if _, err := io.ReadFull(ptyRead, buf); err != nil {
		t.Fatalf("read: %v", err)
	}
	if string(buf) != "hi" {
		t.Errorf("got %q, want %q", buf, "hi")
	}
}

func TestSession_RefreshCmdReturnsOnNotify(t *testing.T) {
	defer goleak.VerifyNone(t)
	ptmx, _ := pipePTY(t)
	subscribe, feed, _ := fakeSubscribe(t)
	sess, err := NewSession("p1", ptmx, 80, 24, subscribe)
	if err != nil {
		t.Fatalf("NewSession: %v", err)
	}
	defer sess.Close()

	cmd := sess.RefreshCmd()
	got := make(chan tea.Msg, 1)
	go func() { got <- cmd() }()

	// Trigger a screen change.
	feed([]byte("a"))
	select {
	case msg := <-got:
		if _, ok := msg.(VTRefreshMsg); !ok {
			t.Errorf("got %T, want VTRefreshMsg", msg)
		}
	case <-time.After(time.Second):
		t.Fatal("RefreshCmd did not return")
	}
}

func TestSession_RefreshCmdReturnsNilOnClose(t *testing.T) {
	defer goleak.VerifyNone(t)
	ptmx, _ := pipePTY(t)
	subscribe, _, _ := fakeSubscribe(t)
	sess, err := NewSession("p1", ptmx, 80, 24, subscribe)
	if err != nil {
		t.Fatalf("NewSession: %v", err)
	}

	got := make(chan tea.Msg, 1)
	go func() { got <- sess.RefreshCmd()() }()
	sess.Close()
	select {
	case msg := <-got:
		if msg != nil {
			t.Errorf("got %v, want nil after Close", msg)
		}
	case <-time.After(time.Second):
		t.Fatal("RefreshCmd did not unblock on Close")
	}
}

func TestSession_WatchErrCmdSurfacesPTYError(t *testing.T) {
	defer goleak.VerifyNone(t)
	ptmx, ptyRead := pipePTY(t)
	subscribe, _, _ := fakeSubscribe(t)
	sess, err := NewSession("p1", ptmx, 80, 24, subscribe)
	if err != nil {
		t.Fatalf("NewSession: %v", err)
	}
	defer sess.Close()

	// Close the read side so subsequent writes EPIPE.
	_ = ptyRead.Close()

	got := make(chan tea.Msg, 1)
	go func() { got <- sess.WatchErrCmd()() }()

	// Provoke a write error.
	_ = sess.SendBytes([]byte("x"))

	select {
	case msg := <-got:
		end, ok := msg.(EmbeddedAttachEndedMsg)
		if !ok {
			t.Fatalf("got %T, want EmbeddedAttachEndedMsg", msg)
		}
		if end.Reason == "" {
			t.Error("empty reason on error")
		}
	case <-time.After(time.Second):
		t.Fatal("WatchErrCmd did not fire on PTY error")
	}
}

func TestKeyDetach_HandshakeDetaches(t *testing.T) {
	d := KeyDetach{Timeout: 100 * time.Millisecond}

	consumed, detached, flush := d.Feed(tea.KeyMsg{Type: tea.KeyCtrlCloseBracket})
	if !consumed || detached || len(flush) != 0 {
		t.Fatalf("first ] = (%v,%v,%v); want (true,false,nil)", consumed, detached, flush)
	}
	consumed, detached, flush = d.Feed(tea.KeyMsg{Type: tea.KeyCtrlCloseBracket})
	if !consumed || !detached || len(flush) != 0 {
		t.Fatalf("second ] = (%v,%v,%v); want (true,true,nil)", consumed, detached, flush)
	}
}

func TestKeyDetach_NonBracketFollowupFlushes(t *testing.T) {
	d := KeyDetach{Timeout: 100 * time.Millisecond}

	consumed, _, _ := d.Feed(tea.KeyMsg{Type: tea.KeyCtrlCloseBracket})
	if !consumed {
		t.Fatal("first ] not consumed")
	}
	consumed, detached, flush := d.Feed(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'a'}})
	if consumed || detached {
		t.Fatalf("got consumed=%v detached=%v; want both false", consumed, detached)
	}
	if len(flush) != 1 || flush[0] != rawCtrlCloseBracket {
		t.Errorf("flush = %x; want [1d]", flush)
	}
	if d.Armed() {
		t.Error("detector still armed after flush")
	}
}

func TestKeyDetach_TimeoutThenBracketRearms(t *testing.T) {
	d := KeyDetach{Timeout: 10 * time.Millisecond}
	_, _, _ = d.Feed(tea.KeyMsg{Type: tea.KeyCtrlCloseBracket})
	time.Sleep(20 * time.Millisecond)
	consumed, detached, flush := d.Feed(tea.KeyMsg{Type: tea.KeyCtrlCloseBracket})
	if !consumed {
		t.Fatal("re-arm: second ] should be consumed")
	}
	if detached {
		t.Fatal("re-arm: should NOT detach across timeout boundary")
	}
	if len(flush) != 1 || flush[0] != rawCtrlCloseBracket {
		t.Errorf("flush = %x; want [1d]", flush)
	}
	if !d.Armed() {
		t.Fatal("re-arm: detector should be armed again")
	}
}

func TestKeyDetach_FlushIfExpired(t *testing.T) {
	d := KeyDetach{Timeout: 5 * time.Millisecond}
	_, _, _ = d.Feed(tea.KeyMsg{Type: tea.KeyCtrlCloseBracket})

	if got := d.FlushIfExpired(); got != nil {
		t.Errorf("immediate flush = %x; want nil", got)
	}
	time.Sleep(15 * time.Millisecond)
	got := d.FlushIfExpired()
	if len(got) != 1 || got[0] != rawCtrlCloseBracket {
		t.Errorf("expired flush = %x; want [1d]", got)
	}
	if d.Armed() {
		t.Error("still armed after expired flush")
	}
}

func TestKeyDetach_PassThroughWhenIdle(t *testing.T) {
	d := KeyDetach{Timeout: 100 * time.Millisecond}
	consumed, detached, flush := d.Feed(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'x'}})
	if consumed || detached || len(flush) != 0 {
		t.Fatalf("idle pass-through = (%v,%v,%v); want all zero", consumed, detached, flush)
	}
}
