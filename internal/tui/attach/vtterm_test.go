package attach

import (
	"bytes"
	"io"
	"sync"
	"testing"
	"time"

	uv "github.com/charmbracelet/ultraviolet"
	"go.uber.org/goleak"
)

// safeBuf is a thread-safe bytes.Buffer.
type safeBuf struct {
	mu sync.Mutex
	bytes.Buffer
}

func (s *safeBuf) Write(p []byte) (int, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.Buffer.Write(p)
}

func (s *safeBuf) String() string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.Buffer.String()
}

// waitFor polls until cond returns true or timeout. Helps tests that
// depend on the drain pump goroutine flushing bytes.
func waitFor(t *testing.T, cond func() bool, timeout time.Duration) {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if cond() {
			return
		}
		time.Sleep(2 * time.Millisecond)
	}
	t.Fatalf("condition not met within %s", timeout)
}

func TestVTTerm_CPRRoundTrip(t *testing.T) {
	defer goleak.VerifyNone(t)
	var out safeBuf
	term := NewVTTerm(80, 24, &out)
	defer term.Close()

	if _, err := term.Write([]byte("\x1b[5;10H")); err != nil {
		t.Fatalf("Write cursor pos: %v", err)
	}
	if _, err := term.Write([]byte("\x1b[6n")); err != nil {
		t.Fatalf("Write CPR query: %v", err)
	}

	want := "\x1b[5;10R"
	waitFor(t, func() bool { return out.String() == want }, time.Second)
}

func TestVTTerm_SendKey(t *testing.T) {
	defer goleak.VerifyNone(t)
	var out safeBuf
	term := NewVTTerm(80, 24, &out)
	defer term.Close()

	term.SendKey(uv.KeyPressEvent{Code: 'a', Text: "a"})
	term.SendKey(uv.KeyPressEvent{Code: 'a', Mod: uv.ModCtrl}) // Ctrl+A → \x01

	waitFor(t, func() bool { return out.String() == "a\x01" }, time.Second)
}

func TestVTTerm_CellAt(t *testing.T) {
	defer goleak.VerifyNone(t)
	term := NewVTTerm(80, 24, io.Discard)
	defer term.Close()

	if _, err := term.Write([]byte("\x1b[1;1HHi")); err != nil {
		t.Fatalf("Write: %v", err)
	}
	// Allow internal pipe to settle.
	time.Sleep(20 * time.Millisecond)

	c0 := term.CellAt(0, 0)
	if c0 == nil || c0.Content != "H" {
		t.Fatalf("CellAt(0,0) = %+v, want content 'H'", c0)
	}
	c1 := term.CellAt(1, 0)
	if c1 == nil || c1.Content != "i" {
		t.Fatalf("CellAt(1,0) = %+v, want content 'i'", c1)
	}
}

func TestVTTerm_NotifyCoalesces(t *testing.T) {
	defer goleak.VerifyNone(t)
	term := NewVTTerm(80, 24, io.Discard)
	defer term.Close()

	// Write twice quickly; should at most have one pending ping.
	_, _ = term.Write([]byte("a"))
	_, _ = term.Write([]byte("b"))

	// Drain one ping; channel should now be empty (no second pending).
	select {
	case <-term.Notify():
	case <-time.After(time.Second):
		t.Fatal("expected at least one notification")
	}

	select {
	case <-term.Notify():
		// Possibly a 2nd ping from a parser-state transition; accept if
		// received within 50ms but verify it's coalesced not flooded.
		select {
		case <-term.Notify():
			t.Fatal("notify channel flooded — coalescing broken")
		case <-time.After(50 * time.Millisecond):
		}
	case <-time.After(50 * time.Millisecond):
	}
}

func TestVTTerm_Resize(t *testing.T) {
	defer goleak.VerifyNone(t)
	term := NewVTTerm(80, 24, io.Discard)
	defer term.Close()

	term.Resize(120, 40)
	if w := term.Width(); w != 120 {
		t.Errorf("Width after resize = %d, want 120", w)
	}
	if h := term.Height(); h != 40 {
		t.Errorf("Height after resize = %d, want 40", h)
	}
}

func TestVTTerm_CloseIsIdempotent(t *testing.T) {
	defer goleak.VerifyNone(t)
	term := NewVTTerm(80, 24, io.Discard)
	if err := term.Close(); err != nil {
		t.Errorf("first Close: %v", err)
	}
	if err := term.Close(); err != nil {
		t.Errorf("second Close: %v", err)
	}
	// Operations after Close are no-ops, not panics.
	term.SendKey(uv.KeyPressEvent{Code: 'a'})
	if _, err := term.Write([]byte("x")); err == nil {
		t.Error("Write after Close should error")
	}
}
