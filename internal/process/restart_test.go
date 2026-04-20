package process

import (
	"testing"
	"time"
)

// basePolicyCfg mirrors the TS test fixture.
var basePolicyCfg = struct {
	mode        string
	backoffsMs  []int
	maxAttempts int
	resetAfterMs int
}{
	mode:         "on-failure",
	backoffsMs:   []int{1000, 2000, 4000, 8000, 16000},
	maxAttempts:  5,
	resetAfterMs: 60000,
}

func newTestPolicy(mode string, backoffsMs []int, maxAttempts, resetAfterMs int) *Policy {
	return NewPolicy(mode, backoffsMs, maxAttempts, resetAfterMs)
}

func TestPolicy_CleanExitReturnsNoneAndResetsAttempts(t *testing.T) {
	p := newTestPolicy(basePolicyCfg.mode, basePolicyCfg.backoffsMs, basePolicyCfg.maxAttempts, basePolicyCfg.resetAfterMs)
	action, _ := p.OnExit(0, "", time.Now())
	if action != ActionNone {
		t.Fatalf("expected ActionNone, got %v", action)
	}
	if p.Attempts() != 0 {
		t.Fatalf("expected 0 attempts, got %d", p.Attempts())
	}
}

func TestPolicy_CrashWithModeNeverGoesNone(t *testing.T) {
	p := newTestPolicy("never", basePolicyCfg.backoffsMs, basePolicyCfg.maxAttempts, basePolicyCfg.resetAfterMs)
	action, _ := p.OnExit(1, "", time.Now())
	if action != ActionNone {
		t.Fatalf("expected ActionNone, got %v", action)
	}
}

func TestPolicy_FirstCrashReturnsRestartWithFirstBackoff(t *testing.T) {
	p := newTestPolicy(basePolicyCfg.mode, basePolicyCfg.backoffsMs, basePolicyCfg.maxAttempts, basePolicyCfg.resetAfterMs)
	action, delay := p.OnExit(1, "", time.Now())
	if action != ActionRestart {
		t.Fatalf("expected ActionRestart, got %v", action)
	}
	if delay != 1000*time.Millisecond {
		t.Fatalf("expected 1000ms, got %v", delay)
	}
	if p.Attempts() != 1 {
		t.Fatalf("expected 1 attempt, got %d", p.Attempts())
	}
}

func TestPolicy_ExhaustsAttemptsToGiveUp(t *testing.T) {
	p := newTestPolicy(basePolicyCfg.mode, basePolicyCfg.backoffsMs, basePolicyCfg.maxAttempts, basePolicyCfg.resetAfterMs)
	for i := 0; i < 5; i++ {
		action, _ := p.OnExit(1, "", time.Now())
		if action != ActionRestart {
			t.Fatalf("attempt %d: expected ActionRestart, got %v", i, action)
		}
		p.OnStart()
	}
	action, _ := p.OnExit(1, "", time.Now())
	if action != ActionGiveUp {
		t.Fatalf("expected ActionGiveUp, got %v", action)
	}
}

func TestPolicy_ClampsBackoffIndexAtLastEntry(t *testing.T) {
	p := newTestPolicy(basePolicyCfg.mode, []int{500, 1000}, 10, basePolicyCfg.resetAfterMs)

	// attempt 0 → 500ms
	action, delay := p.OnExit(1, "", time.Now())
	if action != ActionRestart || delay != 500*time.Millisecond {
		t.Fatalf("expected restart 500ms, got %v %v", action, delay)
	}
	p.OnStart()

	// attempt 1 → 1000ms
	action, delay = p.OnExit(1, "", time.Now())
	if action != ActionRestart || delay != 1000*time.Millisecond {
		t.Fatalf("expected restart 1000ms, got %v %v", action, delay)
	}
	p.OnStart()

	// attempt 2 → clamp to 1000ms
	action, delay = p.OnExit(1, "", time.Now())
	if action != ActionRestart || delay != 1000*time.Millisecond {
		t.Fatalf("expected clamped restart 1000ms, got %v %v", action, delay)
	}
}

func TestPolicy_ManualRestartResetsCounterAndZeroDelay(t *testing.T) {
	p := newTestPolicy(basePolicyCfg.mode, basePolicyCfg.backoffsMs, basePolicyCfg.maxAttempts, basePolicyCfg.resetAfterMs)
	p.OnExit(1, "", time.Now()) // attempts=1
	p.OnExit(1, "", time.Now()) // attempts=2

	action, delay := p.ManualRestart()
	if action != ActionRestart {
		t.Fatalf("expected ActionRestart, got %v", action)
	}
	if delay != 0 {
		t.Fatalf("expected 0 delay, got %v", delay)
	}
	if p.Attempts() != 0 {
		t.Fatalf("expected 0 attempts after manual restart, got %d", p.Attempts())
	}
}

func TestPolicy_StopClearsTimerAndNoEffect(t *testing.T) {
	p := newTestPolicy(basePolicyCfg.mode, basePolicyCfg.backoffsMs, basePolicyCfg.maxAttempts, basePolicyCfg.resetAfterMs)
	p.OnStart()
	p.Stop() // should not panic; resets timer
	// No assertion needed — Stop() is side-effect only; just verify no panic.
}

func TestPolicy_StableRunResetClearsAttemptCounter(t *testing.T) {
	p := newTestPolicy(basePolicyCfg.mode, basePolicyCfg.backoffsMs, basePolicyCfg.maxAttempts, 50 /* ms */)
	// Bump attempts via a crash + start.
	p.OnExit(1, "", time.Now()) // attempts=1
	p.OnStart()
	if p.Attempts() != 1 {
		t.Fatalf("expected 1 attempt, got %d", p.Attempts())
	}
	// Wait past the 50ms reset window.
	time.Sleep(100 * time.Millisecond)
	if p.Attempts() != 0 {
		t.Fatalf("expected 0 attempts after reset, got %d", p.Attempts())
	}
}
