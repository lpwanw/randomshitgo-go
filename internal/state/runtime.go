package state

import (
	"sort"
	"sync"
	"time"

	"github.com/lpwanw/randomshitgo-go/internal/event"
)

// ProjectRuntime holds the live runtime state for a single project.
type ProjectRuntime struct {
	ID              string
	State           string
	PID             int
	StartedAt       time.Time
	RestartAttempts int
	ExitCode        int
	ExitSignal      string
	LastRestartAt   time.Time
}

// RuntimeStore maintains a map of project runtimes and notifies subscribers on
// every change. Subscriber channels are buffered(1) and drop-if-full to avoid
// blocking the event applier.
type RuntimeStore struct {
	mu   sync.RWMutex
	m    map[string]*ProjectRuntime
	subs []chan struct{}
}

// NewRuntimeStore returns a ready RuntimeStore.
func NewRuntimeStore() *RuntimeStore {
	return &RuntimeStore{m: make(map[string]*ProjectRuntime)}
}

// Subscribe returns a channel that receives an empty struct after every state
// change. The channel has capacity 1; a slow subscriber simply misses a tick
// (coalesce-to-latest pattern).
func (s *RuntimeStore) Subscribe() <-chan struct{} {
	ch := make(chan struct{}, 1)
	s.mu.Lock()
	s.subs = append(s.subs, ch)
	s.mu.Unlock()
	return ch
}

// Seed initializes the store with idle entries for the given IDs. Existing
// entries are left unchanged. Subscribers are notified once after seeding so
// initial snapshots propagate to the TUI.
func (s *RuntimeStore) Seed(ids []string) {
	s.mu.Lock()
	for _, id := range ids {
		if _, ok := s.m[id]; !ok {
			s.m[id] = &ProjectRuntime{ID: id, State: "idle"}
		}
	}
	subs := s.subs
	s.mu.Unlock()

	for _, ch := range subs {
		select {
		case ch <- struct{}{}:
		default:
		}
	}
}

// Apply mutates the store based on the incoming event.
func (s *RuntimeStore) Apply(ev event.Event) {
	s.mu.Lock()
	switch e := ev.(type) {
	case event.StartedEvent:
		r := s.getOrInitLocked(e.ID)
		r.State = "running"
		r.PID = e.PID
		r.StartedAt = e.At
	case event.ExitedEvent:
		r := s.getOrInitLocked(e.ID)
		r.PID = 0
		r.ExitCode = e.Code
		r.ExitSignal = e.Signal
	case event.StateChangedEvent:
		r := s.getOrInitLocked(e.ID)
		r.State = e.State
	case event.RestartingEvent:
		r := s.getOrInitLocked(e.ID)
		r.RestartAttempts = e.Attempt
		r.LastRestartAt = time.Now()
	}
	subs := s.subs
	s.mu.Unlock()

	// Notify subscribers outside the write lock.
	for _, ch := range subs {
		select {
		case ch <- struct{}{}:
		default: // drop; subscriber is slow
		}
	}
}

// Snapshot returns a stable sorted copy of all runtimes.
func (s *RuntimeStore) Snapshot() []ProjectRuntime {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make([]ProjectRuntime, 0, len(s.m))
	for _, r := range s.m {
		out = append(out, *r)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ID < out[j].ID })
	return out
}

// Get returns a copy of the runtime for id, or the zero value if not found.
func (s *RuntimeStore) Get(id string) (ProjectRuntime, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	r, ok := s.m[id]
	if !ok {
		return ProjectRuntime{}, false
	}
	return *r, true
}

func (s *RuntimeStore) getOrInitLocked(id string) *ProjectRuntime {
	r, ok := s.m[id]
	if !ok {
		r = &ProjectRuntime{ID: id, State: "idle"}
		s.m[id] = r
	}
	return r
}
