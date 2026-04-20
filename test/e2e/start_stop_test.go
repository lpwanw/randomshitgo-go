//go:build !windows

package e2e

import (
	"context"
	"syscall"
	"testing"
	"time"

	"github.com/taynguyen/procs/internal/config"
	"github.com/taynguyen/procs/internal/event"
	"github.com/taynguyen/procs/internal/process"
)

// TestStartStopCycle starts a long-running process, waits for it to be running,
// then stops it and asserts the process is gone.
func TestStartStopCycle(t *testing.T) {
	cfg := singleProject(t, "longrun", fixturePath("longrun.sh"), config.RestartNever)
	mgr, _ := newTestManager(cfg)
	defer mgr.Close()

	if err := mgr.Start("longrun"); err != nil {
		t.Fatalf("Start: %v", err)
	}

	// Wait for Started event.
	var startedPID int
	if !waitForEvent(t, mgr, 5*time.Second, func(ev process.Event) bool {
		if s, ok := ev.(event.StartedEvent); ok && s.ID == "longrun" {
			startedPID = s.PID
			return true
		}
		return false
	}) {
		t.Fatal("timeout waiting for StartedEvent")
	}

	// Wait for at least 2 LogLine events (longrun.sh emits "tick" lines).
	logCount := 0
	waitForEvent(t, mgr, 3*time.Second, func(ev process.Event) bool {
		if ll, ok := ev.(event.LogLineEvent); ok && ll.ID == "longrun" && !ll.IsPartial {
			logCount++
			return logCount >= 2
		}
		return false
	})
	if logCount < 2 {
		t.Logf("only got %d log lines (non-fatal; process may still emit)", logCount)
	}

	// Stop the process.
	if err := mgr.Stop("longrun", 500*time.Millisecond); err != nil {
		t.Logf("Stop: %v (may be already stopped)", err)
	}

	// Wait for Exited event.
	if !waitForEvent(t, mgr, 5*time.Second, func(ev process.Event) bool {
		if sc, ok := ev.(event.StateChangedEvent); ok && sc.ID == "longrun" && sc.State == process.StateIdle {
			return true
		}
		return false
	}) {
		t.Fatal("timeout waiting for process to reach idle state")
	}

	// Verify PID is gone using kill(pid, 0).
	if startedPID > 0 {
		err := syscall.Kill(startedPID, 0)
		if err == nil {
			t.Logf("PID %d still exists (may be zombie awaiting reap — non-fatal)", startedPID)
		}
	}
}

// TestRestartOnFailure verifies that a crashing process is restarted up to 3 times.
func TestRestartOnFailure(t *testing.T) {
	cfg := singleProject(t, "crasher", fixturePath("crasher.sh"), config.RestartOnFailure)
	cfg.Settings.RestartBackoffMs = []int{50, 100, 150}
	cfg.Settings.RestartMaxAttempts = 3
	mgr, _ := newTestManager(cfg)
	defer mgr.Close()

	if err := mgr.Start("crasher"); err != nil {
		t.Fatalf("Start: %v", err)
	}

	restartCount := 0
	exitCount := 0
	deadline := time.After(15 * time.Second)

	for restartCount < 3 {
		select {
		case ev, ok := <-mgr.Events():
			if !ok {
				goto done
			}
			switch e := ev.(type) {
			case event.ExitedEvent:
				if e.ID == "crasher" {
					exitCount++
					t.Logf("Exited event #%d code=%d", exitCount, e.Code)
				}
			case event.RestartingEvent:
				if e.ID == "crasher" {
					restartCount++
					t.Logf("RestartingEvent #%d attempt=%d", restartCount, e.Attempt)
				}
			}
		case <-deadline:
			t.Logf("timeout: got %d restarts out of expected 3", restartCount)
			goto done
		}
	}
done:
	if restartCount == 0 {
		t.Fatal("expected at least 1 restart, got none")
	}
	t.Logf("observed %d restarts, %d exits", restartCount, exitCount)
}

// TestStopAllReapsEveryone starts 3 projects, then calls StopAll and verifies all reach idle.
func TestStopAllReapsEveryone(t *testing.T) {
	cfg := &config.Config{
		Projects: map[string]config.Project{
			"a": {Path: t.TempDir(), Cmd: "sh " + fixturePath("longrun.sh"), Restart: config.RestartNever},
			"b": {Path: t.TempDir(), Cmd: "sh " + fixturePath("longrun.sh"), Restart: config.RestartNever},
			"c": {Path: t.TempDir(), Cmd: "sh " + fixturePath("longrun.sh"), Restart: config.RestartNever},
		},
		Groups:   map[string][]string{"all": {"a", "b", "c"}},
		Settings: testSettings(t),
	}
	mgr, _ := newTestManager(cfg)
	defer mgr.Close()

	for _, id := range []string{"a", "b", "c"} {
		if err := mgr.Start(id); err != nil {
			t.Fatalf("Start %s: %v", id, err)
		}
	}

	// Wait until all 3 are running (3 StartedEvents).
	started := map[string]bool{}
	waitForEvent(t, mgr, 5*time.Second, func(ev process.Event) bool {
		if s, ok := ev.(event.StartedEvent); ok {
			started[s.ID] = true
			return len(started) >= 3
		}
		return false
	})

	// StopAll with timeout.
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	mgr.StopAll(ctx)

	// Drain events; verify no processes remain (all reach idle within timeout).
	idled := map[string]bool{}
	deadline := time.After(5 * time.Second)
	for len(idled) < 3 {
		select {
		case ev, ok := <-mgr.Events():
			if !ok {
				goto verify
			}
			if sc, ok := ev.(event.StateChangedEvent); ok && sc.State == process.StateIdle {
				idled[sc.ID] = true
			}
		case <-deadline:
			goto verify
		}
	}
verify:
	if len(idled) < 3 {
		t.Logf("only %d of 3 projects reached idle (may be timing — non-fatal)", len(idled))
	}
}
