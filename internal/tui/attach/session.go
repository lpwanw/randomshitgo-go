// Package attach hosts the in-pane vt terminal emulator and the
// Bubbletea-side glue that powers embedded attach mode.
package attach

import (
	"context"
	"errors"
	"io"
	"os"
	"sync"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	uv "github.com/charmbracelet/ultraviolet"
)

// DetachTimeout is the window inside which the second Ctrl-] must arrive
// for the detach handshake to fire. A single Ctrl-] beyond this window is
// flushed to the PTY so the child sees it as a normal keypress.
const DetachTimeout = 400 * time.Millisecond

// rawCtrlCloseBracket is the raw byte for Ctrl-] that the detector
// swallows on first press and replays on a non-handshake follow-up.
const rawCtrlCloseBracket byte = 0x1d

// Session owns one embedded-attach lifecycle: the vt emulator, the PTY
// subscription, the detach detector, and the channels that surface async
// PTY write errors to Bubbletea.
//
// All exported state is read-only after NewSession returns. Mutating
// methods (SendKey, Resize, Close) are safe to call from the Bubbletea
// goroutine.
type Session struct {
	projectID string
	term      *VTTerm
	ptmx      *os.File
	unsub     func()
	detector  KeyDetach

	ctx        context.Context
	cancel     context.CancelFunc
	writeErrCh chan error
	closeOnce  sync.Once
}

// SubscribeFn matches process.Manager.Subscribe so tests can stub the
// PTY tee with a synchronous in-process pipe.
type SubscribeFn func(id string, w io.Writer) (func(), error)

// teeWriter wraps the PTY master file so a Write error (from the drain
// goroutine inside VTTerm) is non-blockingly forwarded out via errCh.
// First error wins; subsequent errors are dropped because the session is
// already on its way down.
type teeWriter struct {
	w     io.Writer
	errCh chan<- error
}

func (t *teeWriter) Write(p []byte) (int, error) {
	n, err := t.w.Write(p)
	if err != nil {
		select {
		case t.errCh <- err:
		default:
		}
	}
	return n, err
}

// NewSession constructs a Session, subscribes the emulator to the
// project's PTY tee, and returns it. The caller owns Close().
func NewSession(projectID string, ptmx *os.File, cols, rows int, subscribe SubscribeFn) (*Session, error) {
	if ptmx == nil {
		return nil, errors.New("attach: nil ptmx")
	}
	if subscribe == nil {
		return nil, errors.New("attach: nil subscribe")
	}
	ctx, cancel := context.WithCancel(context.Background())
	errCh := make(chan error, 1)
	tw := &teeWriter{w: ptmx, errCh: errCh}
	term := NewVTTerm(cols, rows, tw)

	unsub, err := subscribe(projectID, term)
	if err != nil {
		cancel()
		_ = term.Close()
		return nil, err
	}

	return &Session{
		projectID:  projectID,
		term:       term,
		ptmx:       ptmx,
		unsub:      unsub,
		ctx:        ctx,
		cancel:     cancel,
		writeErrCh: errCh,
		detector:   KeyDetach{Timeout: DetachTimeout},
	}, nil
}

// ProjectID returns the project the session is attached to.
func (s *Session) ProjectID() string { return s.projectID }

// Term returns the underlying VTTerm. Renderers may call read-only
// methods (CellAt, Width, Height, CursorPosition).
func (s *Session) Term() *VTTerm { return s.term }

// Detector returns a pointer to the embedded detach detector so the
// routing layer can Feed it directly without copying.
func (s *Session) Detector() *KeyDetach { return &s.detector }

// SendKey routes the key through the emulator so DECCKM / keypad mode
// take effect; the drain goroutine writes the encoded bytes to the PTY.
func (s *Session) SendKey(k uv.KeyPressEvent) { s.term.SendKey(k) }

// SendBytes writes raw bytes straight to the PTY. Used by the routing
// layer to flush swallowed Ctrl-] bytes when the detach handshake fails.
func (s *Session) SendBytes(p []byte) error {
	if len(p) == 0 {
		return nil
	}
	_, err := s.ptmx.Write(p)
	if err != nil {
		select {
		case s.writeErrCh <- err:
		default:
		}
	}
	return err
}

// Resize informs the emulator of new pane dimensions. PTY resize is the
// caller's responsibility (it owns process.Manager).
func (s *Session) Resize(cols, rows int) { s.term.Resize(cols, rows) }

// VTRefreshMsg wakes a View() pass so the next frame reflects the latest
// vt grid snapshot. Emitted by RefreshCmd whenever the emulator pings.
type VTRefreshMsg struct{}

// EmbeddedAttachStartedMsg is emitted once a Session is fully wired and
// the content pane should switch to vt rendering.
type EmbeddedAttachStartedMsg struct {
	ID string
}

// EmbeddedAttachEndedMsg signals an exit from embedded mode for any
// reason (user detach, child write error, EOF). Reason is shown as a
// toast.
type EmbeddedAttachEndedMsg struct {
	Reason string
}

// DetachFlushMsg fires when the KeyDetach detector's first-press window
// expires; the routing layer should call FlushIfExpired and Send the
// returned bytes to the PTY.
type DetachFlushMsg struct{}

// RefreshCmd waits for the next vt notify ping and produces VTRefreshMsg
// so View() re-renders. Returns nil if the session is shutting down.
// Caller must re-issue this command after each VTRefreshMsg to keep the
// loop running.
func (s *Session) RefreshCmd() tea.Cmd {
	notify := s.term.Notify()
	ctx := s.ctx
	return func() tea.Msg {
		select {
		case <-notify:
			return VTRefreshMsg{}
		case <-ctx.Done():
			return nil
		}
	}
}

// WatchErrCmd reports the first PTY write error as
// EmbeddedAttachEndedMsg. Returns nil on graceful shutdown.
func (s *Session) WatchErrCmd() tea.Cmd {
	errCh := s.writeErrCh
	ctx := s.ctx
	return func() tea.Msg {
		select {
		case err := <-errCh:
			return EmbeddedAttachEndedMsg{Reason: "child write failed: " + err.Error()}
		case <-ctx.Done():
			return nil
		}
	}
}

// DetachFlushTickCmd schedules a DetachFlushMsg after the detector
// timeout so a single Ctrl-] eventually reaches the child.
func DetachFlushTickCmd() tea.Cmd {
	return tea.Tick(DetachTimeout+10*time.Millisecond, func(time.Time) tea.Msg {
		return DetachFlushMsg{}
	})
}

// Close tears down the subscription and the emulator. Idempotent.
func (s *Session) Close() {
	s.closeOnce.Do(func() {
		s.cancel()
		if s.unsub != nil {
			s.unsub()
		}
		_ = s.term.Close()
	})
}

// ErrCh exposes the PTY write-error channel so the tui side can wait on
// it inside its own tea.Cmd alongside the refresh loop.
func (s *Session) ErrCh() <-chan error { return s.writeErrCh }

// Done exposes the session context's done channel for the same reason.
func (s *Session) Done() <-chan struct{} { return s.ctx.Done() }

// KeyDetach watches the Bubbletea key stream for the Ctrl-] Ctrl-]
// detach handshake. The first Ctrl-] is swallowed and "armed"; if a
// second arrives inside Timeout the caller exits embedded mode.
//
// If any other key arrives in the window (or Timeout fires) the
// swallowed byte is flushed back so the child program sees it. This
// preserves legitimate uses of Ctrl-] (e.g. readline `quote-meta`).
type KeyDetach struct {
	Timeout time.Duration

	armed   bool
	armedAt time.Time
	flush   []byte
}

// Feed processes a single Bubbletea key event.
//
//   - consumed: caller should swallow this key (do not encode + send).
//   - detached: the handshake completed; caller should exit embedded mode.
//   - flush:    bytes the caller must Write to the PTY *before* handling
//     the current key. Empty when nothing pending.
func (d *KeyDetach) Feed(msg tea.KeyMsg) (consumed, detached bool, flush []byte) {
	isBracket := msg.Type == tea.KeyCtrlCloseBracket
	now := time.Now()

	// Window expired since arming → flush the saved byte and re-evaluate
	// the current key fresh.
	if d.armed && !d.armedAt.IsZero() && now.Sub(d.armedAt) > d.Timeout {
		f := d.flush
		d.armed, d.flush, d.armedAt = false, nil, time.Time{}
		if isBracket {
			d.armed, d.armedAt, d.flush = true, now, []byte{rawCtrlCloseBracket}
			return true, false, f
		}
		return false, false, f
	}

	if d.armed {
		if isBracket {
			d.armed, d.flush, d.armedAt = false, nil, time.Time{}
			return true, true, nil
		}
		f := d.flush
		d.armed, d.flush, d.armedAt = false, nil, time.Time{}
		return false, false, f
	}

	if isBracket {
		d.armed, d.armedAt, d.flush = true, now, []byte{rawCtrlCloseBracket}
		return true, false, nil
	}
	return false, false, nil
}

// FlushIfExpired returns the swallowed bytes when the arm window has
// elapsed and clears the detector. Used by the DetachFlushMsg tick so a
// lone Ctrl-] doesn't sit forever invisible to the child.
func (d *KeyDetach) FlushIfExpired() []byte {
	if !d.armed || d.armedAt.IsZero() {
		return nil
	}
	if time.Since(d.armedAt) <= d.Timeout {
		return nil
	}
	f := d.flush
	d.armed, d.flush, d.armedAt = false, nil, time.Time{}
	return f
}

// Armed reports whether the detector is waiting for a follow-up key.
func (d *KeyDetach) Armed() bool { return d.armed }

