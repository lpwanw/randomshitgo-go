package process

import (
	"context"
	"fmt"
	"io"
	"maps"
	"os"
	"sync"
	"time"

	"github.com/lpwanw/randomshitgo-go/internal/config"
)

const eventsBufSize = 256

// Manager owns all Child instances, exposes per-id and group operations, and
// fans in all child events to a single channel.
type Manager struct {
	cfg      *config.Config
	reg      Registry
	children map[string]*Child
	events   chan Event
	childWg  sync.WaitGroup // tracks running child goroutines
	mu       sync.RWMutex
	closed   bool
}

// New creates a Manager. reg is the log registry (state.Registry satisfies it).
func New(cfg *config.Config, reg Registry) *Manager {
	return &Manager{
		cfg:      cfg,
		reg:      reg,
		children: make(map[string]*Child, len(cfg.Projects)),
		events:   make(chan Event, eventsBufSize),
	}
}

// Events returns the read-only event channel. Fan-in from all children.
func (m *Manager) Events() <-chan Event {
	return m.events
}

// Start spawns the process for the given project id.
func (m *Manager) Start(id string) error {
	child, err := m.getOrCreate(id)
	if err != nil {
		return err
	}
	return child.Start(context.Background())
}

// Stop sends a graceful shutdown to the given project.
func (m *Manager) Stop(id string, grace time.Duration) error {
	m.mu.RLock()
	child, ok := m.children[id]
	m.mu.RUnlock()
	if !ok {
		return fmt.Errorf("manager: unknown project %q", id)
	}
	_ = grace // Child uses settings.ShutdownGraceMs; callers may pass 0 to use default
	return child.Stop()
}

// Restart stops then restarts the given project with zero backoff.
func (m *Manager) Restart(id string) error {
	child, err := m.getOrCreate(id)
	if err != nil {
		return err
	}
	return child.Restart(context.Background())
}

// StartGroup starts all projects in the named group with the configured delay.
func (m *Manager) StartGroup(name string, delay time.Duration) error {
	m.mu.RLock()
	members, ok := m.cfg.Groups[name]
	m.mu.RUnlock()
	if !ok {
		return fmt.Errorf("manager: unknown group %q", name)
	}
	for i, id := range members {
		if i > 0 && delay > 0 {
			time.Sleep(delay)
		}
		if err := m.Start(id); err != nil {
			return fmt.Errorf("manager: start group %q member %q: %w", name, id, err)
		}
	}
	return nil
}

// StopAll stops all running children and waits for them.
func (m *Manager) StopAll(ctx context.Context) {
	m.mu.RLock()
	ids := make([]string, 0, len(m.children))
	for id := range m.children {
		ids = append(ids, id)
	}
	m.mu.RUnlock()

	var wg sync.WaitGroup
	for _, id := range ids {
		id := id
		wg.Add(1)
		go func() {
			defer wg.Done()
			_ = m.Stop(id, 0)
		}()
	}

	done := make(chan struct{})
	go func() {
		wg.Wait()
		close(done)
	}()

	select {
	case <-done:
	case <-ctx.Done():
	}
}

// Close stops all children, waits for all child goroutines to finish, then
// closes the events channel.
func (m *Manager) Close() {
	m.mu.Lock()
	if m.closed {
		m.mu.Unlock()
		return
	}
	m.closed = true
	m.mu.Unlock()

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	m.StopAll(ctx)
	// Wait for all child goroutines to exit so they can no longer send on events.
	m.childWg.Wait()
	close(m.events)
}

// Resize propagates a PTY window resize to the given project.
func (m *Manager) Resize(id string, cols, rows uint16) {
	m.mu.RLock()
	child, ok := m.children[id]
	m.mu.RUnlock()
	if !ok {
		return
	}
	child.Resize(cols, rows)
}

// Attach returns the PTY master file for the given project.
func (m *Manager) Attach(id string) (*os.File, error) {
	m.mu.RLock()
	child, ok := m.children[id]
	m.mu.RUnlock()
	if !ok {
		return nil, fmt.Errorf("manager: unknown project %q", id)
	}
	return child.Attach()
}

// Subscribe registers w to receive a copy of every byte the project's PTY
// emits. Returns an unsubscribe func. Errors if the project is not running.
func (m *Manager) Subscribe(id string, w io.Writer) (func(), error) {
	m.mu.RLock()
	child, ok := m.children[id]
	m.mu.RUnlock()
	if !ok {
		return nil, fmt.Errorf("manager: unknown project %q", id)
	}
	return child.Subscribe(w), nil
}

// ReloadResult summarizes a config reload diff.
type ReloadResult struct {
	Added   []string // project ids new in newCfg
	Removed []string // project ids gone from newCfg
	Changed []string // project ids whose definition changed (cmd/path/env/restart)
	Stopped []string // subset of Removed that were running and got stopped
}

// Reload swaps the manager's *config.Config for newCfg and reconciles state:
// removed projects are stopped (best-effort), running projects whose cmd/path
// changed are listed in Changed (NOT auto-restarted — the user must do that
// explicitly), and added projects are listed for the caller to seed elsewhere.
// Children retain their original cfg snapshot until their next start/restart.
func (m *Manager) Reload(newCfg *config.Config) ReloadResult {
	if newCfg == nil {
		return ReloadResult{}
	}

	m.mu.Lock()
	oldProjects := map[string]config.Project{}
	if m.cfg != nil {
		for k, v := range m.cfg.Projects {
			oldProjects[k] = v
		}
	}
	runningIDs := make(map[string]struct{}, len(m.children))
	for id := range m.children {
		runningIDs[id] = struct{}{}
	}
	m.cfg = newCfg
	m.mu.Unlock()

	var res ReloadResult
	for id := range oldProjects {
		if _, ok := newCfg.Projects[id]; !ok {
			res.Removed = append(res.Removed, id)
		}
	}
	for id := range newCfg.Projects {
		if _, ok := oldProjects[id]; !ok {
			res.Added = append(res.Added, id)
		}
	}
	for id, oldP := range oldProjects {
		if newP, ok := newCfg.Projects[id]; ok && projectChanged(oldP, newP) {
			res.Changed = append(res.Changed, id)
		}
	}

	// Stop removed-and-running children synchronously per id but parallel across
	// ids. Drop the entry on success regardless of stop error.
	for _, id := range res.Removed {
		if _, running := runningIDs[id]; !running {
			continue
		}
		m.mu.RLock()
		child := m.children[id]
		m.mu.RUnlock()
		if child == nil {
			continue
		}
		_ = child.Stop()
		m.mu.Lock()
		delete(m.children, id)
		m.mu.Unlock()
		res.Stopped = append(res.Stopped, id)
	}

	return res
}

// projectChanged reports whether two Project definitions differ in any field
// that warrants restart for the change to take effect.
func projectChanged(a, b config.Project) bool {
	if a.Path != b.Path || a.Cmd != b.Cmd || a.Restart != b.Restart || a.EnvFile != b.EnvFile {
		return true
	}
	return !maps.Equal(a.Env, b.Env)
}

// getOrCreate returns an existing Child or creates one, wiring its events into
// the manager fan-in channel.
func (m *Manager) getOrCreate(id string) (*Child, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	if child, ok := m.children[id]; ok {
		return child, nil
	}

	proj, ok := m.cfg.Projects[id]
	if !ok {
		return nil, fmt.Errorf("manager: unknown project %q", id)
	}

	// Each child writes to the shared manager events channel.
	child := newChild(id, proj, m.cfg.Settings, m.reg, m.events, &m.childWg)
	m.children[id] = child
	return child, nil
}
