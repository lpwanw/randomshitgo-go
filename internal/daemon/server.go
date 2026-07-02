//go:build !windows

package daemon

import (
	"context"
	"net"
	"os"
	"sync"
	"time"

	"github.com/lpwanw/procs/internal/config"
	"github.com/lpwanw/procs/internal/event"
	"github.com/lpwanw/procs/internal/ipc"
	"github.com/lpwanw/procs/internal/log"
	"github.com/lpwanw/procs/internal/process"
	"github.com/lpwanw/procs/internal/state"
)

// Manager is the subset of *process.Manager the server drives. Defined as an
// interface so tests can substitute a fake without spawning real processes.
type Manager interface {
	Start(id string) error
	Stop(id string, grace time.Duration) error
	Restart(id string) error
	StartGroup(name string, delay time.Duration) error
	StopAll(ctx context.Context)
	Reload(newCfg *config.Config) process.ReloadResult
	Events() <-chan event.Event
	Close()
}

// Server owns the supervisor state and serves one client at a time.
type Server struct {
	cfgMu        sync.RWMutex // guards cfg, which a reload swaps
	cfg          *config.Config
	cfgPath      string // resolved config path; "" disables reload
	rt           *state.RuntimeStore
	mgr          Manager
	childrenPath string

	// bring holds per-project broadcast rings, owned exclusively by the source
	// goroutine, used to build snapshots consistently with the live stream.
	bring map[string]*log.RingBuffer[log.Line]

	register   chan registerReq
	unregister chan *clientConn
	client     *clientConn // owned by the source goroutine

	shutdownOnce sync.Once
	shutdownReq  chan struct{}
	sourceDone   chan struct{}
}

type registerReq struct {
	conn *clientConn
	done chan struct{}
}

// New builds a Server. cfgPath is the resolved config path used for reloads
// ("" disables reload). childrenPath is where live child PIDs are persisted for
// orphan detection after a crash (may be "" to disable).
func New(cfg *config.Config, cfgPath string, rt *state.RuntimeStore, mgr Manager, childrenPath string) *Server {
	return &Server{
		cfg:          cfg,
		cfgPath:      cfgPath,
		rt:           rt,
		mgr:          mgr,
		childrenPath: childrenPath,
		bring:        make(map[string]*log.RingBuffer[log.Line]),
		register:     make(chan registerReq),
		unregister:   make(chan *clientConn),
		shutdownReq:  make(chan struct{}),
		sourceDone:   make(chan struct{}),
	}
}

// ListenAndServe binds the socket (chmod 0600), serves clients, and blocks until
// shutdown is requested (via Shutdown command or Shutdown method). It tears down
// in order: stop accepting, Close the Manager, drain remaining events, return.
// The caller is responsible for removing socket/pidfile/children files.
func (s *Server) ListenAndServe(sock string) error {
	ln, err := net.Listen("unix", sock)
	if err != nil {
		return err
	}
	// Restrict the socket to the owner; the 0700 cache dir is the primary gate.
	_ = os.Chmod(sock, 0o600)

	go s.sourceLoop()
	go s.acceptLoop(ln)

	<-s.shutdownReq
	_ = ln.Close()  // stop accepting new clients
	s.mgr.Close()   // StopAll + drain children + close(Events); blocks
	<-s.sourceDone  // source goroutine forwards final events, then exits
	return nil
}

// Shutdown requests an orderly stop. Safe to call multiple times.
func (s *Server) Shutdown() {
	s.shutdownOnce.Do(func() { close(s.shutdownReq) })
}

// acceptLoop accepts connections, verifies the peer uid, and registers each as
// the (single) active client. A new client takes over from any prior one.
func (s *Server) acceptLoop(ln net.Listener) {
	for {
		conn, err := ln.Accept()
		if err != nil {
			return // listener closed
		}
		if uid, perr := peerUID(conn); perr != nil || uid != os.Getuid() {
			_ = conn.Close()
			continue
		}
		c := newClientConn(conn, s)
		go c.writeLoop()
		done := make(chan struct{})
		select {
		case s.register <- registerReq{conn: c, done: done}:
			<-done
			go c.readLoop()
		case <-s.shutdownReq:
			c.close()
			return
		}
	}
}

// sourceLoop is the single goroutine that mutates runtime state, fills broadcast
// rings, forwards events to the client, and handles register/unregister. Serving
// snapshots here keeps them consistent with the live stream.
func (s *Server) sourceLoop() {
	events := s.mgr.Events()
	for {
		select {
		case ev, ok := <-events:
			if !ok {
				if s.client != nil {
					s.client.close()
					s.client = nil
				}
				close(s.sourceDone)
				return
			}
			s.applyAndForward(ev)
		case req := <-s.register:
			if s.client != nil && s.client != req.conn {
				s.client.close()
			}
			req.conn.enqueue(&ipc.Envelope{Kind: ipc.KindSnapshot, Snapshot: s.buildSnapshot()})
			s.client = req.conn
			close(req.done)
		case c := <-s.unregister:
			if s.client == c {
				s.client = nil
			}
			c.close()
		}
	}
}

// applyAndForward updates state for one event, records log lines / child PIDs,
// and forwards the event to the connected client.
func (s *Server) applyAndForward(ev event.Event) {
	s.rt.Apply(ev)
	switch e := ev.(type) {
	case event.LogLineEvent:
		s.ringFor(e.ID).Push(log.Line{
			Bytes:     e.Bytes,
			Rendered:  log.DecodeForRender(e.Bytes),
			IsPartial: e.IsPartial,
			Timestamp: e.At,
		})
	case event.StartedEvent, event.ExitedEvent:
		s.persistChildren()
	}
	if s.client != nil {
		if env := eventEnvelope(ev); env != nil {
			s.client.enqueue(env)
		}
	}
}

// settings returns the current settings under the cfg lock (a reload may swap
// the config concurrently).
func (s *Server) settings() config.Settings {
	s.cfgMu.RLock()
	defer s.cfgMu.RUnlock()
	return s.cfg.Settings
}

// ringFor returns (lazily creating) the broadcast ring for a project.
func (s *Server) ringFor(id string) *log.RingBuffer[log.Line] {
	r, ok := s.bring[id]
	if !ok {
		cap := s.settings().LogBufferLines
		if cap <= 0 {
			cap = 1000
		}
		r = log.NewRingBuffer[log.Line](cap)
		s.bring[id] = r
	}
	return r
}

// buildSnapshot captures current project states (from rt) plus broadcast-ring
// lines. Called only from the source goroutine, so it is consistent with the
// events forwarded immediately after.
func (s *Server) buildSnapshot() *ipc.Snapshot {
	runtimes := s.rt.Snapshot()
	snap := &ipc.Snapshot{Projects: make([]ipc.ProjectSnap, 0, len(runtimes))}
	for _, r := range runtimes {
		ps := ipc.ProjectSnap{ID: r.ID, State: r.State, PID: r.PID}
		if ring, ok := s.bring[r.ID]; ok {
			ps.Lines = ring.Snapshot()
		}
		snap.Projects = append(snap.Projects, ps)
	}
	return snap
}

// persistChildren writes the current live child PIDs for orphan detection.
func (s *Server) persistChildren() {
	if s.childrenPath == "" {
		return
	}
	var recs []ChildRec
	for _, r := range s.rt.Snapshot() {
		if r.PID > 0 {
			recs = append(recs, ChildRec{ID: r.ID, PID: r.PID})
		}
	}
	_ = writeChildren(s.childrenPath, recs)
}

// dropClient is invoked by a client's writer goroutine on write failure.
func (s *Server) dropClient(c *clientConn) {
	select {
	case s.unregister <- c:
	case <-s.shutdownReq:
		c.close()
	}
}

// unregisterClient is invoked by a client's reader goroutine on disconnect.
func (s *Server) unregisterClient(c *clientConn) {
	select {
	case s.unregister <- c:
	case <-s.shutdownReq:
	}
}

// dispatch routes a client command to the Manager. Blocking operations run in
// their own goroutine so the reader stays responsive.
func (s *Server) dispatch(cmd *ipc.Command, c *clientConn) {
	switch cmd.Op {
	case ipc.OpStart:
		go func() {
			if err := s.mgr.Start(cmd.ID); err != nil {
				c.enqueue(toastEnvelope(err.Error(), "error"))
			}
		}()
	case ipc.OpStop:
		go func() { _ = s.mgr.Stop(cmd.ID, 0) }()
	case ipc.OpRestart:
		go func() { _ = s.mgr.Restart(cmd.ID) }()
	case ipc.OpStartGroup:
		delay := time.Duration(s.settings().GroupStartDelayMs) * time.Millisecond
		go func() {
			if err := s.mgr.StartGroup(cmd.Group, delay); err != nil {
				c.enqueue(toastEnvelope(err.Error(), "error"))
			}
		}()
	case ipc.OpStopAll:
		go s.mgr.StopAll(context.Background())
	case ipc.OpStatus:
		// rt is concurrency-safe; reply with states only (no log lines needed).
		runtimes := s.rt.Snapshot()
		snap := &ipc.Snapshot{Projects: make([]ipc.ProjectSnap, 0, len(runtimes))}
		for _, r := range runtimes {
			snap.Projects = append(snap.Projects, ipc.ProjectSnap{ID: r.ID, State: r.State, PID: r.PID})
		}
		c.enqueue(&ipc.Envelope{Kind: ipc.KindSnapshot, Snapshot: snap})
	case ipc.OpReload:
		go s.handleReload(c)
	case ipc.OpShutdown:
		s.Shutdown()
	}
}

// handleReload re-reads the config from the daemon's own resolved path (never a
// client-supplied blob, to keep the source of truth on disk and spoof-safe),
// reconciles children via the Manager, updates daemon state, and streams the
// reconciliation result back to the client.
func (s *Server) handleReload(c *clientConn) {
	if s.cfgPath == "" {
		c.enqueue(toastEnvelope("reload unavailable (no config path)", "warn"))
		return
	}
	newCfg, err := config.LoadFromPath(s.cfgPath)
	if err != nil {
		c.enqueue(toastEnvelope("config error: "+err.Error(), "error"))
		return
	}
	res := s.mgr.Reload(newCfg)

	s.cfgMu.Lock()
	s.cfg = newCfg
	s.cfgMu.Unlock()

	// Keep the daemon's runtime store in step so future snapshots are correct.
	if len(res.Added) > 0 {
		s.rt.Seed(res.Added)
	}
	if len(res.Removed) > 0 {
		s.rt.Delete(res.Removed)
	}

	c.enqueue(&ipc.Envelope{Kind: ipc.KindEvent, Event: &ipc.EventMsg{
		Kind: ipc.EvReloadResult,
		ReloadResult: &ipc.ReloadResult{
			Added:   res.Added,
			Removed: res.Removed,
			Changed: res.Changed,
			Stopped: res.Stopped,
		},
	}})
}

// eventEnvelope wraps a lifecycle event into a wire envelope, or nil to skip.
func eventEnvelope(ev event.Event) *ipc.Envelope {
	em := &ipc.EventMsg{}
	switch e := ev.(type) {
	case event.StartedEvent:
		em.Kind, em.Started = ipc.EvStarted, &e
	case event.ExitedEvent:
		em.Kind, em.Exited = ipc.EvExited, &e
	case event.StateChangedEvent:
		em.Kind, em.StateChanged = ipc.EvStateChanged, &e
	case event.LogLineEvent:
		em.Kind, em.LogLine = ipc.EvLogLine, &e
	case event.RestartingEvent:
		em.Kind, em.Restarting = ipc.EvRestarting, &e
	default:
		return nil
	}
	return &ipc.Envelope{Kind: ipc.KindEvent, Event: em}
}

// toastEnvelope builds a daemon-originated toast envelope.
func toastEnvelope(text, level string) *ipc.Envelope {
	return &ipc.Envelope{Kind: ipc.KindEvent, Event: &ipc.EventMsg{
		Kind:  ipc.EvToast,
		Toast: &ipc.Toast{Text: text, Level: level},
	}}
}

// compile-time assertion that the real Manager satisfies the interface.
var _ Manager = (*process.Manager)(nil)