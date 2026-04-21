//go:build !windows

package e2e

import (
	"testing"
	"time"

	"github.com/lpwanw/randomshitgo-go/internal/config"
	"github.com/lpwanw/randomshitgo-go/internal/process"
	"github.com/lpwanw/randomshitgo-go/internal/state"
)

// testSettings returns minimal settings suitable for fast e2e tests.
func testSettings(t *testing.T) config.Settings {
	t.Helper()
	return config.Settings{
		LogBufferLines:      500,
		LogDir:              t.TempDir(),
		LogRotateSizeMB:     10,
		LogRotateKeep:       2,
		ShutdownGraceMs:     500,
		GroupStartDelayMs:   100,
		RestartBackoffMs:    []int{50, 100},
		RestartMaxAttempts:  5,
		RestartResetAfterMs: 60000,
		PtyCols:             80,
		PtyRows:             24,
		LogFlushIntervalMs:  100,
	}
}

// singleProject builds a Config with one project running the given fixture.
func singleProject(t *testing.T, id string, script string, restart config.RestartMode) *config.Config {
	t.Helper()
	return &config.Config{
		Projects: map[string]config.Project{
			id: {
				Path:    t.TempDir(),
				Cmd:     "sh " + script,
				Restart: restart,
			},
		},
		Groups:   map[string][]string{},
		Settings: testSettings(t),
	}
}

// newTestManager builds a Manager + Registry for a given Config.
func newTestManager(cfg *config.Config) (*process.Manager, *state.Registry) {
	reg := state.NewRegistry(cfg.Settings)
	mgr := process.New(cfg, reg)
	return mgr, reg
}

// waitForEvent drains the manager event channel until predicate returns true or
// timeout elapses. Returns false on timeout.
func waitForEvent(t *testing.T, mgr *process.Manager, timeout time.Duration, pred func(process.Event) bool) bool {
	t.Helper()
	deadline := time.After(timeout)
	for {
		select {
		case ev, ok := <-mgr.Events():
			if !ok {
				return false
			}
			if pred(ev) {
				return true
			}
		case <-deadline:
			return false
		}
	}
}

// collectEvents drains events for up to timeout and returns all received.
func collectEvents(mgr *process.Manager, timeout time.Duration) []process.Event {
	var events []process.Event
	deadline := time.After(timeout)
	for {
		select {
		case ev, ok := <-mgr.Events():
			if !ok {
				return events
			}
			events = append(events, ev)
		case <-deadline:
			return events
		}
	}
}
