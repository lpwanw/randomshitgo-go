package attach

import (
	"io"
	"testing"
	"time"

	"go.uber.org/goleak"
)

// readWithin reads up to len(buf) bytes within d. Returns the slice
// actually filled. Used because the drain pump writes paste bytes
// asynchronously.
func readWithin(t *testing.T, r io.Reader, n int, d time.Duration) []byte {
	t.Helper()
	out := make([]byte, 0, n)
	deadline := time.Now().Add(d)
	for len(out) < n && time.Now().Before(deadline) {
		buf := make([]byte, n-len(out))
		_ = r.(interface{ SetReadDeadline(time.Time) error }).SetReadDeadline(time.Now().Add(50 * time.Millisecond))
		k, err := r.Read(buf)
		if k > 0 {
			out = append(out, buf[:k]...)
		}
		if err != nil {
			break
		}
	}
	return out
}

func TestSession_SendPasteForwardsToPTY(t *testing.T) {
	defer goleak.VerifyNone(t)
	ptmx, ptyRead := pipePTY(t)
	subscribe, _, _ := fakeSubscribe(t)
	sess, err := NewSession("p1", ptmx, 80, 24, subscribe)
	if err != nil {
		t.Fatalf("NewSession: %v", err)
	}
	defer sess.Close()

	if err := sess.SendPaste("hello"); err != nil {
		t.Fatalf("SendPaste: %v", err)
	}
	got := readWithin(t, ptyRead, len("hello"), time.Second)
	if string(got) != "hello" {
		t.Errorf("got %q, want %q", got, "hello")
	}
}

func TestSession_SendPasteNormalisesNewlines(t *testing.T) {
	defer goleak.VerifyNone(t)
	ptmx, ptyRead := pipePTY(t)
	subscribe, _, _ := fakeSubscribe(t)
	sess, err := NewSession("p1", ptmx, 80, 24, subscribe)
	if err != nil {
		t.Fatalf("NewSession: %v", err)
	}
	defer sess.Close()

	if err := sess.SendPaste("a\r\nb\nc"); err != nil {
		t.Fatalf("SendPaste: %v", err)
	}
	got := readWithin(t, ptyRead, 5, time.Second)
	if string(got) != "a\rb\rc" {
		t.Errorf("got %q, want %q", got, "a\rb\rc")
	}
}

func TestSession_SendPasteEmptyIsNoop(t *testing.T) {
	defer goleak.VerifyNone(t)
	ptmx, _ := pipePTY(t)
	subscribe, _, _ := fakeSubscribe(t)
	sess, err := NewSession("p1", ptmx, 80, 24, subscribe)
	if err != nil {
		t.Fatalf("NewSession: %v", err)
	}
	defer sess.Close()

	if err := sess.SendPaste(""); err != nil {
		t.Errorf("empty paste should be no-op, got %v", err)
	}
}
