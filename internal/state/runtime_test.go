package state

import (
	"testing"
	"time"

	"github.com/lpwanw/randomshitgo-go/internal/event"
)

func TestRuntimeStore_Seed(t *testing.T) {
	s := NewRuntimeStore()
	ch := s.Subscribe()
	s.Seed([]string{"web", "api"})

	select {
	case <-ch:
	case <-time.After(time.Second):
		t.Fatal("expected subscriber notification after Seed")
	}

	snap := s.Snapshot()
	if len(snap) != 2 {
		t.Fatalf("expected 2 seeded entries, got %d", len(snap))
	}
	for _, r := range snap {
		if r.State != "idle" {
			t.Fatalf("expected seeded state idle, got %q for %s", r.State, r.ID)
		}
	}

	// Re-seeding preserves existing runtime state.
	s.Apply(event.StartedEvent{ID: "web", PID: 42, At: time.Now()})
	s.Seed([]string{"web", "api"})
	r, _ := s.Get("web")
	if r.State != "running" || r.PID != 42 {
		t.Fatalf("seed overwrote running entry: %+v", r)
	}
}

func TestRuntimeStore_ApplyStarted(t *testing.T) {
	s := NewRuntimeStore()
	s.Apply(event.StartedEvent{ID: "web", PID: 1234, At: time.Now()})

	r, ok := s.Get("web")
	if !ok {
		t.Fatal("expected web entry")
	}
	if r.State != "running" {
		t.Fatalf("expected state running, got %q", r.State)
	}
	if r.PID != 1234 {
		t.Fatalf("expected PID 1234, got %d", r.PID)
	}
}

func TestRuntimeStore_ApplyExited(t *testing.T) {
	s := NewRuntimeStore()
	s.Apply(event.StartedEvent{ID: "web", PID: 99, At: time.Now()})
	s.Apply(event.ExitedEvent{ID: "web", Code: 1, Signal: "", At: time.Now()})

	r, ok := s.Get("web")
	if !ok {
		t.Fatal("expected web entry")
	}
	if r.ExitCode != 1 {
		t.Fatalf("expected ExitCode 1, got %d", r.ExitCode)
	}
	if r.PID != 0 {
		t.Fatalf("expected PID 0 after exit, got %d", r.PID)
	}
}

func TestRuntimeStore_ApplyStateChanged(t *testing.T) {
	s := NewRuntimeStore()
	s.Apply(event.StateChangedEvent{ID: "web", State: "crashed"})

	r, ok := s.Get("web")
	if !ok {
		t.Fatal("expected web entry")
	}
	if r.State != "crashed" {
		t.Fatalf("expected crashed, got %q", r.State)
	}
}

func TestRuntimeStore_ApplyRestarting(t *testing.T) {
	s := NewRuntimeStore()
	s.Apply(event.RestartingEvent{ID: "web", Attempt: 2, Delay: 500 * time.Millisecond})

	r, ok := s.Get("web")
	if !ok {
		t.Fatal("expected web entry")
	}
	if r.RestartAttempts != 2 {
		t.Fatalf("expected RestartAttempts 2, got %d", r.RestartAttempts)
	}
}

func TestRuntimeStore_Subscribe_Notified(t *testing.T) {
	s := NewRuntimeStore()
	ch := s.Subscribe()

	s.Apply(event.StartedEvent{ID: "api", PID: 5, At: time.Now()})

	select {
	case <-ch:
		// ok
	case <-time.After(100 * time.Millisecond):
		t.Fatal("expected notification within 100ms")
	}
}

func TestRuntimeStore_Subscribe_DropIfFull(t *testing.T) {
	s := NewRuntimeStore()
	ch := s.Subscribe() // capacity=1

	// Send two events without draining — second should be dropped, no panic.
	s.Apply(event.StartedEvent{ID: "a", PID: 1, At: time.Now()})
	s.Apply(event.StartedEvent{ID: "b", PID: 2, At: time.Now()})

	// Only one notification should be in the channel.
	select {
	case <-ch:
		// ok
	case <-time.After(100 * time.Millisecond):
		t.Fatal("expected at least one notification")
	}
	// Second should have been dropped.
	select {
	case <-ch:
		// also ok if it was buffered (race is fine — the point is no panic)
	default:
	}
}

func TestRuntimeStore_Snapshot_Sorted(t *testing.T) {
	s := NewRuntimeStore()
	s.Apply(event.StartedEvent{ID: "z", PID: 1, At: time.Now()})
	s.Apply(event.StartedEvent{ID: "a", PID: 2, At: time.Now()})
	s.Apply(event.StartedEvent{ID: "m", PID: 3, At: time.Now()})

	snap := s.Snapshot()
	if len(snap) != 3 {
		t.Fatalf("expected 3 entries, got %d", len(snap))
	}
	if snap[0].ID != "a" || snap[1].ID != "m" || snap[2].ID != "z" {
		t.Fatalf("not sorted: %v %v %v", snap[0].ID, snap[1].ID, snap[2].ID)
	}
}
