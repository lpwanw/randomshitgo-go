package state

import (
	"fmt"
	"path/filepath"
	"sync"

	"github.com/lpwanw/randomshitgo-go/internal/config"
	"github.com/lpwanw/randomshitgo-go/internal/log"
)

// Entry holds the per-project log pipeline: ring buffer, rotator, line splitter.
type Entry struct {
	Ring *log.RingBuffer[log.Line]
	Rot  *log.Rotator
	line *log.LineBuffer
}

// Registry manages per-project log pipelines. Lazy-inits on first access.
// Implements process.Registry (WriteRaw).
type Registry struct {
	mu  sync.RWMutex
	m   map[string]*Entry
	cfg config.Settings
}

// NewRegistry creates a Registry using settings for defaults.
func NewRegistry(cfg config.Settings) *Registry {
	return &Registry{
		m:   make(map[string]*Entry),
		cfg: cfg,
	}
}

// Get returns the Entry for id, lazy-initialising it on first call.
// LogDir must already be expanded (config.Load does this).
func (r *Registry) Get(id string) *Entry {
	r.mu.RLock()
	if e, ok := r.m[id]; ok {
		r.mu.RUnlock()
		return e
	}
	r.mu.RUnlock()

	r.mu.Lock()
	defer r.mu.Unlock()
	// Double-check under write lock.
	if e, ok := r.m[id]; ok {
		return e
	}

	cap := r.cfg.LogBufferLines
	if cap <= 0 {
		cap = 1000
	}
	ring := log.NewRingBuffer[log.Line](cap)
	lb := log.NewLineBuffer(0)

	logPath := filepath.Join(r.cfg.LogDir, fmt.Sprintf("%s.log", id))
	rot, err := log.NewRotator(logPath, r.cfg.LogRotateSizeMB, r.cfg.LogRotateKeep)
	if err != nil {
		// Log directory may not exist yet; best-effort — rotator will be nil.
		fmt.Printf("procs: registry: open rotator for %s: %v\n", id, err)
		rot = nil
	}

	e := &Entry{Ring: ring, Rot: rot, line: lb}
	r.m[id] = e
	return e
}

// WriteRaw splits raw PTY bytes into lines, pushes each to the ring, and
// writes the ANSI-stripped version to the rotator.
func (r *Registry) WriteRaw(id string, b []byte) {
	e := r.Get(id)
	lines := e.line.Feed(b)
	for _, l := range lines {
		e.Ring.Push(l)
		if e.Rot != nil {
			stripped := log.StripANSI(string(l.Bytes))
			_, _ = e.Rot.Write([]byte(stripped + "\n"))
		}
	}
}

// Close flushes and closes all rotators.
func (r *Registry) Close() {
	r.mu.Lock()
	defer r.mu.Unlock()
	for _, e := range r.m {
		if e.Rot != nil {
			_ = e.Rot.Close()
		}
	}
}
