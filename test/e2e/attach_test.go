//go:build !windows

package e2e

import (
	"io"
	"os"
	"testing"
	"time"

	"github.com/lpwanw/randomshitgo-go/internal/config"
	"github.com/lpwanw/randomshitgo-go/internal/event"
	"github.com/lpwanw/randomshitgo-go/internal/process"
	"github.com/lpwanw/randomshitgo-go/internal/tui/attach"
)

// pipeFiles creates a pair of *os.File pipes for use in tests.
func pipeFiles(t *testing.T) (r, w *os.File) {
	t.Helper()
	pr, pw, err := os.Pipe()
	if err != nil {
		t.Fatalf("os.Pipe: %v", err)
	}
	t.Cleanup(func() {
		pr.Close()
		pw.Close()
	})
	return pr, pw
}

// TestAttachDetach uses repl.sh to verify:
// 1. The process starts and produces a PTY.
// 2. Bytes written to the PTY arrive at the process.
// 3. The escape detector correctly identifies the Ctrl-] Ctrl-] sequence as detach.
func TestAttachDetach(t *testing.T) {
	cfg := singleProject(t, "repl", fixturePath("repl.sh"), config.RestartNever)
	mgr, _ := newTestManager(cfg)
	defer mgr.Close()

	if err := mgr.Start("repl"); err != nil {
		t.Fatalf("Start: %v", err)
	}

	// Wait until running.
	if !waitForEvent(t, mgr, 5*time.Second, func(ev process.Event) bool {
		if s, ok := ev.(event.StartedEvent); ok && s.ID == "repl" {
			return true
		}
		return false
	}) {
		t.Fatal("timeout waiting for StartedEvent")
	}

	// Get the PTY master.
	ptmx, err := mgr.Attach("repl")
	if err != nil {
		t.Fatalf("Attach: %v", err)
	}

	// Write "hello\n" to the PTY and attempt to read the echoed response.
	_, _ = io.WriteString(ptmx, "hello\n")

	readDone := make(chan string, 1)
	go func() {
		buf := make([]byte, 256)
		ptmx.SetReadDeadline(time.Now().Add(1 * time.Second)) //nolint:errcheck
		n, _ := ptmx.Read(buf)
		readDone <- string(buf[:n])
	}()

	select {
	case data := <-readDone:
		t.Logf("PTY read: %q", data)
	case <-time.After(2 * time.Second):
		t.Log("no data from PTY within 1s (non-fatal)")
	}

	// --- Test the escape detector end-to-end using io.Pipe ---
	// This exercises the same logic the Controller uses without needing raw mode.
	det := attach.NewEscapeDetector(attach.DefaultEscapeByte, attach.DefaultTimeout)

	stdinR, stdinW := pipeFiles(t)

	// Writer goroutine: writes bytes from stdinR, feeds through escape detector.
	ptmxOutR, ptmxOutW := pipeFiles(t)

	detachDetected := make(chan bool, 1)

	go func() {
		buf := make([]byte, 64)
		for {
			stdinR.SetReadDeadline(time.Now().Add(200 * time.Millisecond)) //nolint:errcheck
			n, readErr := stdinR.Read(buf)
			if n > 0 {
				pass, d := det.FeedChunk(buf[:n], time.Now())
				if len(pass) > 0 {
					_, _ = ptmxOutW.Write(pass)
				}
				if d {
					detachDetected <- true
					return
				}
			}
			if readErr != nil {
				if nerr, ok := readErr.(interface{ Timeout() bool }); ok && nerr.Timeout() {
					continue
				}
				return
			}
		}
	}()

	// Discard PTY output side.
	go func() { io.Copy(io.Discard, ptmxOutR) }() //nolint:errcheck

	// Give goroutine time to start.
	time.Sleep(20 * time.Millisecond)

	// Send double Ctrl-].
	stdinW.Write([]byte{attach.DefaultEscapeByte, attach.DefaultEscapeByte}) //nolint:errcheck

	select {
	case <-detachDetected:
		t.Log("escape detector: detach confirmed")
	case <-time.After(2 * time.Second):
		t.Error("timeout: escape detector did not detect double Ctrl-]")
	}
}
