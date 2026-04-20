package attach

import (
	"context"
	"io"
	"os"
	"testing"
	"time"
)

// TestController_DetachOnDoubleEscape verifies that sending 0x1d 0x1d to the
// stdin side triggers detach. We use two os.Pipe pairs as a fake PTY so no
// real terminal is needed (and term.MakeRaw is skipped via pipeController).
func TestController_DetachOnDoubleEscape(t *testing.T) {
	// Use pipeController which bypasses term.MakeRaw.
	stdinR, stdinW, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	defer stdinR.Close()
	defer stdinW.Close()

	ptmxR, ptmxW, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	defer ptmxR.Close()
	defer ptmxW.Close()

	// Discard PTY output.
	go io.Copy(io.Discard, ptmxR) //nolint:errcheck

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	done := make(chan struct {
		det bool
		err error
	}, 1)

	go func() {
		det, err := runWithPipes(ctx, stdinR, ptmxW, DefaultEscapeByte, DefaultTimeout)
		done <- struct {
			det bool
			err error
		}{det, err}
	}()

	// Give the goroutine time to start.
	time.Sleep(20 * time.Millisecond)
	// Write two escape bytes quickly.
	stdinW.Write([]byte{0x1d, 0x1d}) //nolint:errcheck

	select {
	case res := <-done:
		if !res.det {
			t.Fatalf("expected detach=true, err=%v", res.err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("timeout: controller did not detach")
	}
}

// TestController_PassthroughBytes verifies that non-escape bytes reach the PTY.
func TestController_PassthroughBytes(t *testing.T) {
	stdinR, stdinW, _ := os.Pipe()
	defer stdinR.Close()
	defer stdinW.Close()

	ptmxR, ptmxW, _ := os.Pipe()
	defer ptmxR.Close()
	defer ptmxW.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	got := make(chan []byte, 1)
	go func() {
		buf := make([]byte, 32)
		n, _ := ptmxR.Read(buf)
		got <- buf[:n]
	}()

	// Run controller in background; cancel after reading.
	go runWithPipes(ctx, stdinR, ptmxW, DefaultEscapeByte, DefaultTimeout) //nolint:errcheck

	time.Sleep(20 * time.Millisecond)
	stdinW.Write([]byte("hello")) //nolint:errcheck

	select {
	case b := <-got:
		if string(b) != "hello" {
			t.Fatalf("expected hello, got %q", b)
		}
	case <-time.After(1 * time.Second):
		t.Fatal("timeout waiting for passthrough bytes")
	}
}

// TestController_ContextCancel checks that the controller exits when ctx is cancelled.
func TestController_ContextCancel(t *testing.T) {
	stdinR, stdinW, _ := os.Pipe()
	defer stdinR.Close()
	defer stdinW.Close()

	ptmxR, ptmxW, _ := os.Pipe()
	defer ptmxR.Close()
	defer ptmxW.Close()

	go io.Copy(io.Discard, ptmxR) //nolint:errcheck

	ctx, cancel := context.WithCancel(context.Background())

	done := make(chan struct{}, 1)
	go func() {
		runWithPipes(ctx, stdinR, ptmxW, DefaultEscapeByte, DefaultTimeout) //nolint:errcheck
		done <- struct{}{}
	}()

	time.Sleep(30 * time.Millisecond)
	cancel()

	select {
	case <-done:
		// good
	case <-time.After(1 * time.Second):
		t.Fatal("controller did not exit after ctx cancel")
	}
}

// runWithPipes is a test-only variant of Controller.Run that reads from
// an explicit io.Reader/Writer pair instead of os.Stdin, bypassing MakeRaw.
func runWithPipes(
	ctx context.Context,
	stdin *os.File,
	ptmx *os.File,
	escapeByte byte,
	timeout time.Duration,
) (detached bool, err error) {
	runCtx, cancel := context.WithCancel(ctx)
	defer cancel()

	type result struct {
		detach bool
		err    error
	}
	resultCh := make(chan result, 2)

	detector := NewEscapeDetector(escapeByte, timeout)

	// Goroutine A: stdin → escape filter → ptmx.
	go func() {
		buf := make([]byte, ioBufferSize)
		for {
			select {
			case <-runCtx.Done():
				resultCh <- result{err: runCtx.Err()}
				return
			default:
			}
			stdin.SetReadDeadline(time.Now().Add(50 * time.Millisecond)) //nolint:errcheck
			n, readErr := stdin.Read(buf)
			if n > 0 {
				now := time.Now()
				pass, det := detector.FeedChunk(buf[:n], now)
				if len(pass) > 0 {
					_, _ = ptmx.Write(pass)
				}
				if det {
					resultCh <- result{detach: true}
					return
				}
			}
			if readErr != nil {
				if isTimeout(readErr) {
					continue
				}
				resultCh <- result{err: readErr}
				return
			}
		}
	}()

	// Goroutine B: ptmx read side — not connected here; instead we rely on
	// the test to drain ptmxR. PTY output goroutine would normally copy ptmx→stdout.
	// In tests we don't need that, so simulate it finishing quickly.
	// We need at least one goroutine sending to resultCh to avoid blocking forever
	// when ptmx write-side closes (which will happen on test teardown via defer).
	go func() {
		<-runCtx.Done()
		resultCh <- result{err: runCtx.Err()}
	}()

	res := <-resultCh
	cancel()
	return res.detach, res.err
}
