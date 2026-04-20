package process

import (
	"context"
	"fmt"
	"os"
	"sync"
	"time"

	"github.com/taynguyen/procs/internal/config"
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
