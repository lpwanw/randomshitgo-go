//go:build !windows

package client_test

import (
	"context"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/lpwanw/procs/internal/client"
	"github.com/lpwanw/procs/internal/config"
	"github.com/lpwanw/procs/internal/daemon"
	"github.com/lpwanw/procs/internal/event"
	"github.com/lpwanw/procs/internal/process"
	"github.com/lpwanw/procs/internal/state"
	"github.com/lpwanw/procs/internal/tui"
	"go.uber.org/goleak"
)

// RemoteManager must satisfy the TUI's process-control interface.
var _ tui.ProcManager = (*client.RemoteManager)(nil)

func TestMain(m *testing.M) {
	goleak.VerifyTestMain(m)
}

// fakeMgr is a no-op daemon.Manager whose event channel the test drives.
type fakeMgr struct {
	events  chan event.Event
	mu      sync.Mutex
	started []string
	closed  bool
}

func newFakeMgr() *fakeMgr { return &fakeMgr{events: make(chan event.Event, 16)} }

func (f *fakeMgr) Start(id string) error {
	f.mu.Lock()
	f.started = append(f.started, id)
	f.mu.Unlock()
	return nil
}
func (f *fakeMgr) Stop(string, time.Duration) error       { return nil }
func (f *fakeMgr) Restart(string) error                   { return nil }
func (f *fakeMgr) StartGroup(string, time.Duration) error           { return nil }
func (f *fakeMgr) StopAll(context.Context)                          {}
func (f *fakeMgr) Reload(*config.Config) process.ReloadResult       { return process.ReloadResult{} }
func (f *fakeMgr) Events() <-chan event.Event                       { return f.events }
func (f *fakeMgr) Close() {
	f.mu.Lock()
	defer f.mu.Unlock()
	if !f.closed {
		f.closed = true
		close(f.events)
	}
}
func (f *fakeMgr) startedIDs() []string {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]string(nil), f.started...)
}

func shortSock(t *testing.T) string {
	t.Helper()
	dir, err := os.MkdirTemp("/tmp", "procsc")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { os.RemoveAll(dir) })
	return filepath.Join(dir, "s")
}

// startServer launches a fake-backed daemon and returns its socket + manager.
func startServer(t *testing.T) (string, *fakeMgr) {
	t.Helper()
	sock := shortSock(t)
	rt := state.NewRuntimeStore()
	rt.Seed([]string{"web"})
	fm := newFakeMgr()
	srv := daemon.New(&config.Config{Settings: config.DefaultSettings}, "", rt, fm, "")
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
	// Wait until accepting.
	deadline := time.Now().Add(3 * time.Second)
	for {
		if c, err := os.Stat(sock); err == nil && c.Mode()&os.ModeSocket != 0 {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("socket never appeared")
		}
		time.Sleep(10 * time.Millisecond)
	}
	return sock, fm
}

func waitFor(t *testing.T, what string, fn func() bool) {
	t.Helper()
	deadline := time.Now().Add(4 * time.Second)
	for !fn() {
		if time.Now().After(deadline) {
			t.Fatalf("timeout waiting for %s", what)
		}
		time.Sleep(15 * time.Millisecond)
	}
}

func TestDial_SeedsSnapshot(t *testing.T) {
	sock, _ := startServer(t)
	cl, err := client.Dial(sock, 100)
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	defer cl.Close()

	r, ok := cl.Runtime().Get("web")
	if !ok || r.State != "idle" {
		t.Fatalf("snapshot did not seed web=idle: %+v ok=%v", r, ok)
	}
}

func TestStream_StartedEventDerefsAndMutatesRuntime(t *testing.T) {
	sock, fm := startServer(t)
	cl, err := client.Dial(sock, 100)
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	defer cl.Close()

	// A pointer-wrapped Started on the wire must deref to a value so Apply matches.
	fm.events <- event.StartedEvent{ID: "web", PID: 4242, At: time.Now()}

	waitFor(t, "running state", func() bool {
		r, ok := cl.Runtime().Get("web")
		return ok && r.State == "running" && r.PID == 4242
	})
}

func TestStream_LogLineRingNoDisk(t *testing.T) {
	// Run from a temp cwd so an accidental "web.log" would be detectable here.
	tmp := t.TempDir()
	cwd, _ := os.Getwd()
	if err := os.Chdir(tmp); err != nil {
		t.Fatal(err)
	}
	defer os.Chdir(cwd)

	sock, fm := startServer(t)
	cl, err := client.Dial(sock, 100)
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	defer cl.Close()

	fm.events <- event.LogLineEvent{ID: "web", Bytes: []byte("hello-line"), At: time.Now()}

	waitFor(t, "log line in ring", func() bool {
		lines := cl.Reg().Get("web").Ring.Snapshot()
		for _, l := range lines {
			if string(l.Bytes) == "hello-line" {
				return true
			}
		}
		return false
	})

	if _, err := os.Stat(filepath.Join(tmp, "web.log")); !os.IsNotExist(err) {
		t.Fatalf("client wrote a log file to disk (A3 regression): err=%v", err)
	}
}

func TestStream_PartialLineFlagPreserved(t *testing.T) {
	sock, fm := startServer(t)
	cl, err := client.Dial(sock, 100)
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	defer cl.Close()

	fm.events <- event.LogLineEvent{ID: "web", Bytes: []byte("partial-frag"), IsPartial: true, At: time.Now()}

	waitFor(t, "partial line mirrored", func() bool {
		for _, l := range cl.Reg().Get("web").Ring.Snapshot() {
			if string(l.Bytes) == "partial-frag" {
				if !l.IsPartial {
					t.Fatalf("IsPartial flag lost on mirrored line")
				}
				return true
			}
		}
		return false
	})
}

func TestRemoteManager_StartSendsCommand(t *testing.T) {
	sock, fm := startServer(t)
	cl, err := client.Dial(sock, 100)
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	defer cl.Close()

	if err := cl.Manager().Start("web"); err != nil {
		t.Fatalf("remote start: %v", err)
	}
	waitFor(t, "daemon received start", func() bool {
		for _, id := range fm.startedIDs() {
			if id == "web" {
				return true
			}
		}
		return false
	})
}

func TestRemoteManager_AttachUnsupported(t *testing.T) {
	sock, _ := startServer(t)
	cl, err := client.Dial(sock, 100)
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	defer cl.Close()

	if _, err := cl.Manager().Attach("web"); err == nil {
		t.Fatalf("expected attach to be unsupported while detached")
	}
}

// Core detach semantic: a process started by one client keeps running after
// that client disconnects, and a second client reattaching sees it running.
func TestDetach_ProcessSurvivesClientDisconnect(t *testing.T) {
	tmp := t.TempDir()
	sock := shortSock(t)

	settings := config.DefaultSettings
	settings.LogDir = tmp
	settings.ShutdownGraceMs = 500
	cfg := &config.Config{
		Projects: map[string]config.Project{
			"web": {Path: tmp, Cmd: "sleep 5", Restart: config.RestartNever},
		},
		Settings: settings,
	}
	reg := state.NewRegistry(cfg.Settings)
	defer reg.Close()
	rt := state.NewRuntimeStore()
	rt.Seed([]string{"web"})
	mgr := process.New(cfg, reg)

	srv := daemon.New(cfg, "", rt, mgr, "")
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
	waitFor(t, "socket", func() bool {
		fi, err := os.Stat(sock)
		return err == nil && fi.Mode()&os.ModeSocket != 0
	})

	// Client A starts the project, then detaches.
	a, err := client.Dial(sock, 100)
	if err != nil {
		t.Fatalf("dial A: %v", err)
	}
	if err := a.Manager().Start("web"); err != nil {
		t.Fatalf("start: %v", err)
	}
	waitFor(t, "running (A)", func() bool {
		r, ok := a.Runtime().Get("web")
		return ok && r.State == "running"
	})
	a.Close() // detach

	// Client B reattaches and must see the still-running process.
	b, err := client.Dial(sock, 100)
	if err != nil {
		t.Fatalf("dial B: %v", err)
	}
	defer b.Close()
	r, ok := b.Runtime().Get("web")
	if !ok || r.State != "running" || r.PID <= 0 {
		t.Fatalf("reattached client should see running process, got %+v ok=%v", r, ok)
	}
}

// Reload round-trip: the daemon re-reads its config file, reconciles, and
// streams the result back to the client as a NotifReload.
func TestReload_StreamsAddedProject(t *testing.T) {
	tmp := t.TempDir()
	cfgFile := filepath.Join(tmp, "config.yml")
	writeYAML := func(body string) {
		if err := os.WriteFile(cfgFile, []byte(body), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	writeYAML("projects:\n  web:\n    path: " + tmp + "\n    cmd: sleep 1\n")

	cfg, err := config.LoadFromPath(cfgFile)
	if err != nil {
		t.Fatalf("load cfg: %v", err)
	}
	reg := state.NewRegistry(cfg.Settings)
	defer reg.Close()
	rt := state.NewRuntimeStore()
	rt.Seed([]string{"web"})
	mgr := process.New(cfg, reg)

	sock := shortSock(t)
	srv := daemon.New(cfg, cfgFile, rt, mgr, "")
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
	waitFor(t, "socket", func() bool {
		fi, err := os.Stat(sock)
		return err == nil && fi.Mode()&os.ModeSocket != 0
	})

	cl, err := client.Dial(sock, 100)
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	defer cl.Close()

	// Add a project to the config file, then reload.
	writeYAML("projects:\n  web:\n    path: " + tmp + "\n    cmd: sleep 1\n  api:\n    path: " + tmp + "\n    cmd: sleep 1\n")
	cl.Manager().Reload(nil)

	deadline := time.After(4 * time.Second)
	for {
		select {
		case n := <-cl.Notifications():
			if n.Kind == client.NotifReload && n.Reload != nil {
				found := false
				for _, id := range n.Reload.Added {
					if id == "api" {
						found = true
					}
				}
				if !found {
					t.Fatalf("reload result missing added 'api': %+v", n.Reload)
				}
				return
			}
		case <-deadline:
			t.Fatal("no reload result received")
		}
	}
}

func TestConnLost_OnDaemonGone(t *testing.T) {
	sock := shortSock(t)
	rt := state.NewRuntimeStore()
	rt.Seed([]string{"web"})
	fm := newFakeMgr()
	srv := daemon.New(&config.Config{Settings: config.DefaultSettings}, "", rt, fm, "")
	done := make(chan error, 1)
	go func() { done <- srv.ListenAndServe(sock) }()
	waitFor(t, "socket", func() bool {
		fi, err := os.Stat(sock)
		return err == nil && fi.Mode()&os.ModeSocket != 0
	})

	cl, err := client.Dial(sock, 100)
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	defer cl.Close()

	srv.Shutdown()
	<-done

	select {
	case n := <-cl.Notifications():
		if n.Kind != client.NotifConnLost {
			t.Fatalf("want ConnLost, got %v", n.Kind)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("no ConnLost notification after daemon shutdown")
	}
}