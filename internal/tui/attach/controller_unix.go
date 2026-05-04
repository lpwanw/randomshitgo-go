//go:build !windows

package attach

import (
	"context"
	"io"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/creack/pty"
	"golang.org/x/term"
)

const ioBufferSize = 4096

// Controller manages the raw-mode PTY bridge for attach mode.
// Run blocks until the user detaches (double Esc), the context is cancelled,
// or the PTY EOF fires.
type Controller struct {
	escapeByte byte
	timeout    time.Duration
}

// NewController returns a Controller with the given escape byte and timeout.
// Use 0 for defaults (Esc and 400ms).
func NewController(escapeByte byte, timeout time.Duration) *Controller {
	if escapeByte == 0 {
		escapeByte = DefaultEscapeByte
	}
	if timeout == 0 {
		timeout = DefaultTimeout
	}
	return &Controller{escapeByte: escapeByte, timeout: timeout}
}

// Run enters raw mode, starts bidirectional I/O pumps between stdin/stdout and
// ptmx, and blocks until detach, context cancellation, or I/O EOF.
//
// Returns (true, nil) when the user performs the double-escape detach.
// Returns (false, ctx.Err()) when the context is cancelled.
// Returns (false, err) on unexpected I/O errors.
//
// The caller MUST call ReleaseTerminal on the *tea.Program before Run, and
// RestoreTerminal after Run returns.
func (c *Controller) Run(ctx context.Context, ptmx *os.File) (detached bool, err error) {
	oldState, err := term.MakeRaw(int(os.Stdin.Fd()))
	if err != nil {
		return false, err
	}
	defer term.Restore(int(os.Stdin.Fd()), oldState) //nolint:errcheck

	// Context for the two goroutines.
	runCtx, cancel := context.WithCancel(ctx)
	defer cancel()

	// SIGWINCH: propagate terminal resize to the child PTY.
	winchCh := make(chan os.Signal, 1)
	signal.Notify(winchCh, syscall.SIGWINCH)
	defer signal.Stop(winchCh)

	// Sync channels: each goroutine sends its exit reason.
	type result struct {
		detach bool
		err    error
	}
	resultCh := make(chan result, 2)

	detector := NewEscapeDetector(c.escapeByte, c.timeout)

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
			// Use a short deadline so we can check context; do NOT use blocking
			// Read in a goroutine that ignores ctx without this pattern.
			// SetDeadline allows poll without spinning.
			os.Stdin.SetReadDeadline(time.Now().Add(50 * time.Millisecond)) //nolint:errcheck
			n, readErr := os.Stdin.Read(buf)
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

	// Goroutine B: ptmx → stdout.
	go func() {
		_, err := io.Copy(os.Stdout, ptmx)
		resultCh <- result{err: err}
	}()

	// SIGWINCH watcher.
	go func() {
		for {
			select {
			case <-runCtx.Done():
				return
			case <-winchCh:
				ws, err := pty.GetsizeFull(os.Stdout)
				if err == nil {
					_ = pty.Setsize(ptmx, ws)
				}
			}
		}
	}()

	// Send initial window size.
	if ws, err := pty.GetsizeFull(os.Stdout); err == nil {
		_ = pty.Setsize(ptmx, ws)
	}

	// Wait for first goroutine to finish; cancel the other.
	res := <-resultCh
	cancel()
	return res.detach, res.err
}

// isTimeout returns true if err is a deadline-exceeded network/os error.
func isTimeout(err error) bool {
	if err == nil {
		return false
	}
	if nerr, ok := err.(interface{ Timeout() bool }); ok {
		return nerr.Timeout()
	}
	return false
}
