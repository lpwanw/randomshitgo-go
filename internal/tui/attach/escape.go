// Package attach provides the PTY bridge and Ctrl-] Ctrl-] escape detector
// used when the user presses 'a' to attach to a running child process.
package attach

import "time"

const (
	// DefaultEscapeByte is Ctrl-] (0x1d), matching the telnet convention.
	DefaultEscapeByte byte = 0x1d
	// DefaultTimeout is the window within which a second escape byte triggers detach.
	DefaultTimeout = 400 * time.Millisecond
)

// EscapeDetector is a two-press state machine for detecting the detach sequence.
//
// Design mirrors escape-detector.ts:
//   - Non-escape byte resets the window; the byte is forwarded.
//   - First escape byte: swallowed, window starts.
//   - Second escape byte within the timeout window: detach=true, no passthrough.
//   - Second escape byte after the timeout has elapsed: treated as the first
//     press of a new window; previous byte silently dropped (documented limitation).
type EscapeDetector struct {
	escapeByte  byte
	timeout     time.Duration
	lastEscapeAt time.Time // zero means "no pending escape"
}

// NewEscapeDetector returns an EscapeDetector with the given escape byte and timeout.
// Pass 0 for either to use the defaults.
func NewEscapeDetector(escapeByte byte, timeout time.Duration) *EscapeDetector {
	if escapeByte == 0 {
		escapeByte = DefaultEscapeByte
	}
	if timeout == 0 {
		timeout = DefaultTimeout
	}
	return &EscapeDetector{escapeByte: escapeByte, timeout: timeout}
}

// Feed processes a single byte read from stdin.
// It returns (passthrough, detach):
//   - passthrough: bytes that should be forwarded to the child PTY (may be nil).
//   - detach: true means the user completed the double-tap escape sequence.
//
// When detach is true, passthrough is always nil.
func (d *EscapeDetector) Feed(b byte, now time.Time) (passthrough []byte, detach bool) {
	if b == d.escapeByte {
		if !d.lastEscapeAt.IsZero() && now.Sub(d.lastEscapeAt) < d.timeout {
			// Second escape within window → detach.
			d.lastEscapeAt = time.Time{}
			return nil, true
		}
		// First escape (or second after timeout): swallow, record time.
		d.lastEscapeAt = now
		return nil, false
	}

	// Non-escape byte cancels the window. Swallowed escape byte is silently
	// dropped (documented limitation, mirrors TS implementation).
	d.lastEscapeAt = time.Time{}
	return []byte{b}, false
}

// FeedChunk processes multiple bytes at once, stopping on detach.
// Returns (passthrough, detach) where passthrough contains all bytes before the
// detach sequence.
func (d *EscapeDetector) FeedChunk(chunk []byte, now time.Time) (passthrough []byte, detach bool) {
	var out []byte
	for _, b := range chunk {
		pass, det := d.Feed(b, now)
		if det {
			return out, true
		}
		out = append(out, pass...)
	}
	return out, false
}
