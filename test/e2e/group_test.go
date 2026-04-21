//go:build !windows

package e2e

import (
	"testing"
	"time"

	"github.com/lpwanw/randomshitgo-go/internal/config"
	"github.com/lpwanw/randomshitgo-go/internal/event"
)

// TestStartGroupOrdering verifies that StartGroup starts processes with proper
// delay spacing (at least 80ms apart for a 100ms configured delay).
func TestStartGroupOrdering(t *testing.T) {
	cfg := &config.Config{
		Projects: map[string]config.Project{
			"a": {Path: t.TempDir(), Cmd: "sh " + fixturePath("longrun.sh"), Restart: config.RestartNever},
			"b": {Path: t.TempDir(), Cmd: "sh " + fixturePath("longrun.sh"), Restart: config.RestartNever},
			"c": {Path: t.TempDir(), Cmd: "sh " + fixturePath("longrun.sh"), Restart: config.RestartNever},
		},
		Groups:   map[string][]string{"abc": {"a", "b", "c"}},
		Settings: testSettings(t),
	}
	// 100ms delay between group members.
	cfg.Settings.GroupStartDelayMs = 100

	mgr, _ := newTestManager(cfg)
	defer mgr.Close()

	type startTime struct {
		id string
		at time.Time
	}
	starts := make([]startTime, 0, 3)
	resultCh := make(chan startTime, 10)

	// Collect Started events in a separate goroutine so we don't miss any.
	go func() {
		deadline := time.After(10 * time.Second)
		for {
			select {
			case ev, ok := <-mgr.Events():
				if !ok {
					return
				}
				if s, ok := ev.(event.StartedEvent); ok {
					resultCh <- startTime{id: s.ID, at: s.At}
				}
			case <-deadline:
				return
			}
		}
	}()

	// Start group with 100ms delay.
	delay := time.Duration(cfg.Settings.GroupStartDelayMs) * time.Millisecond
	if err := mgr.StartGroup("abc", delay); err != nil {
		t.Fatalf("StartGroup: %v", err)
	}

	// Collect 3 started events.
	for len(starts) < 3 {
		select {
		case st := <-resultCh:
			starts = append(starts, st)
		case <-time.After(10 * time.Second):
			t.Fatalf("timeout: only got %d start events", len(starts))
		}
	}

	// Sort by time (should already be in order, but enforce for assertion).
	// Verify that consecutive starts are at least 80ms apart (80% of the 100ms delay).
	for i := 1; i < len(starts); i++ {
		gap := starts[i].at.Sub(starts[i-1].at)
		t.Logf("gap between %s and %s: %v", starts[i-1].id, starts[i].id, gap)
		if gap < 80*time.Millisecond {
			t.Errorf("gap between start %d and %d: want ≥80ms, got %v", i-1, i, gap)
		}
	}
}
