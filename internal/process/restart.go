package process

import (
	"sync"
	"time"
)

// Action is the decision returned by Policy.OnExit.
type Action int

const (
	// ActionNone means the process should stay stopped (clean exit or never-mode).
	ActionNone Action = iota
	// ActionRestart means the policy wants a restart after Delay.
	ActionRestart
	// ActionGiveUp means max attempts have been exhausted.
	ActionGiveUp
)

// Policy is a thread-safe state machine that mirrors TS RestartPolicy exactly.
// It decides whether a crashed process should be restarted and after how long.
type Policy struct {
	Mode        string        // "never" | "on-failure"
	Backoffs    []time.Duration
	MaxAttempts int
	ResetAfter  time.Duration

	mu         sync.Mutex
	attempts   int
	resetTimer *time.Timer
}

// OnStart records that the process has started and arms the stable-run reset
// timer. Returns the start time (convenience for callers tracking uptime).
func (p *Policy) OnStart() time.Time {
	p.mu.Lock()
	defer p.mu.Unlock()

	p.stopResetTimerLocked()

	if p.ResetAfter > 0 {
		p.resetTimer = time.AfterFunc(p.ResetAfter, func() {
			p.mu.Lock()
			p.attempts = 0
			p.mu.Unlock()
		})
	}
	return time.Now()
}

// OnExit decides what to do when the process exits.
// code is the OS exit code; signalName is the signal name or "" for code exits.
// exitedAt is used for informational purposes only (not used in the decision).
func (p *Policy) OnExit(code int, signalName string, _ time.Time) (Action, time.Duration) {
	p.mu.Lock()
	defer p.mu.Unlock()

	p.stopResetTimerLocked()

	// Clean exit → stay idle, reset counter.
	if code == 0 {
		p.attempts = 0
		return ActionNone, 0
	}

	// Signal kills that are deliberate (SIGTERM/SIGKILL from Stop) also map to
	// ActionNone when mode is never; the caller decides whether to treat them
	// specially.
	_ = signalName

	// User opted out of auto-restart.
	if p.Mode != "on-failure" {
		return ActionNone, 0
	}

	// Max attempts exhausted.
	if p.attempts >= p.MaxAttempts {
		return ActionGiveUp, 0
	}

	// Pick backoff: clamp index to last entry (mirrors TS Math.min).
	idx := p.attempts
	if idx >= len(p.Backoffs) {
		idx = len(p.Backoffs) - 1
	}
	var delay time.Duration
	if idx >= 0 && len(p.Backoffs) > 0 {
		delay = p.Backoffs[idx]
	}
	p.attempts++
	return ActionRestart, delay
}

// ManualRestart resets the attempt counter and returns an immediate restart.
func (p *Policy) ManualRestart() (Action, time.Duration) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.stopResetTimerLocked()
	p.attempts = 0
	return ActionRestart, 0
}

// Stop clears the reset timer (used when the process is stopped intentionally).
func (p *Policy) Stop() {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.stopResetTimerLocked()
}

// Attempts returns the current attempt counter (for testing / display).
func (p *Policy) Attempts() int {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.attempts
}

func (p *Policy) stopResetTimerLocked() {
	if p.resetTimer != nil {
		p.resetTimer.Stop()
		p.resetTimer = nil
	}
}

// NewPolicy builds a Policy from config values.
func NewPolicy(mode string, backoffsMs []int, maxAttempts int, resetAfterMs int) *Policy {
	backoffs := make([]time.Duration, len(backoffsMs))
	for i, ms := range backoffsMs {
		backoffs[i] = time.Duration(ms) * time.Millisecond
	}
	return &Policy{
		Mode:        mode,
		Backoffs:    backoffs,
		MaxAttempts: maxAttempts,
		ResetAfter:  time.Duration(resetAfterMs) * time.Millisecond,
	}
}
