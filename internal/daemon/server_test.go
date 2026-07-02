//go:build !windows

package daemon

import (
	"bytes"
	"context"
	"encoding/binary"
	"net"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/lpwanw/procs/internal/config"
	"github.com/lpwanw/procs/internal/event"
	"github.com/lpwanw/procs/internal/ipc"
	"github.com/lpwanw/procs/internal/process"
	"github.com/lpwanw/procs/internal/state"
	"go.uber.org/goleak"
)

func TestMain(m *testing.M) {
	goleak.VerifyTestMain(m)
}

// fakeManager is a no-op Manager with a controllable event channel, letting
// lifecycle/transport tests run without spawning real processes.
type fakeManager struct {
	events chan event.Event
	mu     sync.Mutex
	closed bool
}

func newFakeManager() *fakeManager { return &fakeManager{events: make(chan event.Event, 16)} }

func (f *fakeManager) Start(string) error                     { return nil }
func (f *fakeManager) Stop(string, time.Duration) error       { return nil }
func (f *fakeManager) Restart(string) error                   { return nil }
func (f *fakeManager) StartGroup(string, time.Duration) error { return nil }
func (f *fakeManager) StopAll(context.Context)                {}
func (f *fakeManager) Reload(*config.Config) process.ReloadResult {
	return process.ReloadResult{}
}
func (f *fakeManager) Events() <-chan event.Event { return f.events }
func (f *fakeManager) Close() {
	f.mu.Lock()
	defer f.mu.Unlock()
	if !f.closed {
		f.closed = true
		close(f.events)
	}
}

// shortSock returns a socket path under /tmp short enough for sun_path limits.
func shortSock(t *testing.T) string {
	t.Helper()
	dir, err := os.MkdirTemp("/tmp", "procsd")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { os.RemoveAll(dir) })
	return filepath.Join(dir, "s")
}

// startFakeServer starts a server backed by a fakeManager and returns it plus a
// cleanup that shuts down and waits.
func startFakeServer(t *testing.T) (*Server, *fakeManager, string) {
	t.Helper()
	sock := shortSock(t)
	rt := state.NewRuntimeStore()
	rt.Seed([]string{"web"})
	fm := newFakeManager()
	srv := New(&config.Config{Settings: config.DefaultSettings}, "", rt, fm, "")
	done := make(chan error, 1)
	go func() { done <- srv.ListenAndServe(sock) }()
	t.Cleanup(func() {
		srv.Shutdown()
		select {
		case <-done:
		case <-time.After(5 * time.Second):
			t.Error("server did not shut down")
		}
	})
	return srv, fm, sock
}

// readSnapshot dials and reads the initial snapshot, returning the live conn.
func readSnapshot(t *testing.T, sock string) net.Conn {
	t.Helper()
	conn := dialWithRetry(t, sock)
	_ = conn.SetReadDeadline(time.Now().Add(5 * time.Second))
	env, err := ipc.NewDecoder(conn).Read()
	if err != nil {
		t.Fatalf("read snapshot: %v", err)
	}
	if env.Kind != ipc.KindSnapshot {
		t.Fatalf("want snapshot first, got kind=%d", env.Kind)
	}
	return conn
}

// A second client takes over; the first is disconnected.
func TestServer_SecondClientTakesOver(t *testing.T) {
	_, _, sock := startFakeServer(t)

	c1 := readSnapshot(t, sock)
	defer c1.Close()

	c2 := readSnapshot(t, sock)
	defer c2.Close()

	// c1 must be closed by the take-over; its next read should error.
	_ = c1.SetReadDeadline(time.Now().Add(2 * time.Second))
	if _, err := ipc.NewDecoder(c1).Read(); err == nil {
		t.Fatalf("expected first client to be disconnected on take-over")
	}
}

// Shutdown with a client connected returns cleanly and disconnects the client.
func TestServer_ShutdownWithConnectedClient(t *testing.T) {
	srv, _, sock := startFakeServer(t)

	conn := readSnapshot(t, sock)
	defer conn.Close()

	srv.Shutdown()

	_ = conn.SetReadDeadline(time.Now().Add(3 * time.Second))
	if _, err := ipc.NewDecoder(conn).Read(); err == nil {
		t.Fatalf("expected client disconnect on shutdown")
	}
}

// A hostile oversized frame drops only that connection; the daemon stays up.
func TestServer_HostileFrameContained(t *testing.T) {
	_, _, sock := startFakeServer(t)

	bad := dialWithRetry(t, sock)
	// Skip its snapshot, then send a forged oversized length prefix.
	_ = bad.SetReadDeadline(time.Now().Add(2 * time.Second))
	_, _ = ipc.NewDecoder(bad).Read()
	var hdr [4]byte
	binary.BigEndian.PutUint32(hdr[:], ipc.MaxFrameSize+1)
	_, _ = bad.Write(hdr[:])
	bad.Close()

	// Daemon must still serve a fresh client.
	good := readSnapshot(t, sock)
	good.Close()
}

func logEnv(id string) *ipc.Envelope {
	return &ipc.Envelope{Kind: ipc.KindEvent, Event: &ipc.EventMsg{
		Kind:    ipc.EvLogLine,
		LogLine: &event.LogLineEvent{ID: id, Bytes: []byte("x")},
	}}
}

func ctrlEnv() *ipc.Envelope {
	return &ipc.Envelope{Kind: ipc.KindEvent, Event: &ipc.EventMsg{
		Kind:         ipc.EvStateChanged,
		StateChanged: &event.StateChangedEvent{ID: "x", State: "running"},
	}}
}

// Control events must never be dropped; log lines are capped (drop-oldest).
func TestClientConn_ControlLosslessLogLossy(t *testing.T) {
	a, b := net.Pipe()
	defer a.Close()
	defer b.Close()
	c := newClientConn(a, nil)

	for i := 0; i < maxLogQueue+50; i++ {
		c.enqueue(logEnv("web"))
	}
	for i := 0; i < 7; i++ {
		c.enqueue(ctrlEnv())
	}

	c.mu.Lock()
	nlog, nctrl := len(c.logq), len(c.ctrl)
	c.mu.Unlock()

	if nlog != maxLogQueue {
		t.Fatalf("log queue should cap at %d, got %d", maxLogQueue, nlog)
	}
	if nctrl != 7 {
		t.Fatalf("control events must be lossless: want 7, got %d", nctrl)
	}
}

// End-to-end: a real child process; client connects, gets a snapshot, starts a
// project, and receives the running state + its log output over the socket.
func TestServer_SnapshotThenLiveStream(t *testing.T) {
	tmp := t.TempDir()
	// Unix socket paths are length-limited (~104 bytes on macOS); the default
	// temp dir under /var/folders is too long, so bind under a short /tmp dir.
	sockDir, err := os.MkdirTemp("/tmp", "procsd")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(sockDir)
	sock := filepath.Join(sockDir, "s")
	children := filepath.Join(tmp, "test.children")

	settings := config.DefaultSettings
	settings.LogDir = tmp
	settings.ShutdownGraceMs = 500
	cfg := &config.Config{
		Projects: map[string]config.Project{
			"web": {Path: tmp, Cmd: "printf 'hello-from-child\\n'; sleep 5", Restart: config.RestartNever},
		},
		Settings: settings,
	}

	reg := state.NewRegistry(cfg.Settings)
	defer reg.Close()
	rt := state.NewRuntimeStore()
	rt.Seed([]string{"web"})
	mgr := process.New(cfg, reg)

	srv := New(cfg, "", rt, mgr, children)
	serveDone := make(chan error, 1)
	go func() { serveDone <- srv.ListenAndServe(sock) }()
	defer func() {
		srv.Shutdown()
		select {
		case <-serveDone:
		case <-time.After(5 * time.Second):
			t.Error("server did not shut down")
		}
	}()

	conn := dialWithRetry(t, sock)
	defer conn.Close()
	_ = conn.SetReadDeadline(time.Now().Add(8 * time.Second))
	dec := ipc.NewDecoder(conn)
	enc := ipc.NewEncoder(conn)

	// First envelope must be the snapshot.
	first, err := dec.Read()
	if err != nil {
		t.Fatalf("read snapshot: %v", err)
	}
	if first.Kind != ipc.KindSnapshot || first.Snapshot == nil {
		t.Fatalf("first envelope should be a snapshot, got kind=%d", first.Kind)
	}
	if len(first.Snapshot.Projects) != 1 || first.Snapshot.Projects[0].ID != "web" {
		t.Fatalf("snapshot missing seeded project: %+v", first.Snapshot.Projects)
	}

	// Start the project.
	if err := enc.Write(&ipc.Envelope{Kind: ipc.KindCommand, Command: &ipc.Command{Op: ipc.OpStart, ID: "web"}}); err != nil {
		t.Fatalf("send start: %v", err)
	}

	// Expect a running state and the child's log line.
	sawRunning, sawLog := false, false
	for !(sawRunning && sawLog) {
		env, err := dec.Read()
		if err != nil {
			t.Fatalf("read stream (running=%v log=%v): %v", sawRunning, sawLog, err)
		}
		if env.Kind != ipc.KindEvent || env.Event == nil {
			continue
		}
		switch env.Event.Kind {
		case ipc.EvStateChanged:
			if env.Event.StateChanged != nil && env.Event.StateChanged.State == process.StateRunning {
				sawRunning = true
			}
		case ipc.EvStarted:
			if env.Event.Started != nil && env.Event.Started.PID > 0 {
				sawRunning = sawRunning || env.Event.StateChanged != nil
			}
		case ipc.EvLogLine:
			if env.Event.LogLine != nil && bytes.Contains(env.Event.LogLine.Bytes, []byte("hello-from-child")) {
				sawLog = true
			}
		}
	}
}

func dialWithRetry(t *testing.T, sock string) net.Conn {
	t.Helper()
	deadline := time.Now().Add(3 * time.Second)
	for {
		conn, err := net.Dial("unix", sock)
		if err == nil {
			return conn
		}
		if time.Now().After(deadline) {
			t.Fatalf("dial %s: %v", sock, err)
		}
		time.Sleep(20 * time.Millisecond)
	}
}