//go:build !windows

package e2e

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"
)

var (
	buildOnce sync.Once
	binPath   string
	buildErr  error
)

// procsBinary builds the procs binary once per test run and returns its path.
func procsBinary(t *testing.T) string {
	t.Helper()
	buildOnce.Do(func() {
		dir, err := os.MkdirTemp("/tmp", "procsbin")
		if err != nil {
			buildErr = err
			return
		}
		binPath = filepath.Join(dir, "procs")
		out, err := exec.Command("go", "build", "-o", binPath, "github.com/lpwanw/randomshitgo-go/cmd/procs").CombinedOutput()
		if err != nil {
			buildErr = err
			t.Logf("build output: %s", out)
		}
	})
	if buildErr != nil {
		t.Fatalf("build procs: %v", buildErr)
	}
	return binPath
}

// TestDaemonCLILifecycle drives the real binary through status → spawn → status
// → kill → status, isolating its cache dir via HOME/XDG_CACHE_HOME.
func TestDaemonCLILifecycle(t *testing.T) {
	bin := procsBinary(t)

	home, err := os.MkdirTemp("/tmp", "procshome")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { os.RemoveAll(home) })

	cfg := filepath.Join(home, "config.yml")
	if err := os.WriteFile(cfg, []byte("projects:\n  web:\n    path: "+home+"\n    cmd: sleep 60\n"), 0o600); err != nil {
		t.Fatal(err)
	}

	env := append(os.Environ(),
		"HOME="+home,
		"XDG_CACHE_HOME="+filepath.Join(home, ".cache"),
	)
	run := func(args ...string) (string, error) {
		c := exec.Command(bin, args...)
		c.Env = env
		out, err := c.CombinedOutput()
		return string(out), err
	}

	// No daemon yet.
	if out, _ := run("status", "-c", cfg); !strings.Contains(out, "no daemon running") {
		t.Fatalf("status before start: want 'no daemon running', got %q", out)
	}

	// Spawn a headless daemon (background; outlives this call).
	dcmd := exec.Command(bin, "__daemon", "-c", cfg)
	dcmd.Env = env
	if err := dcmd.Start(); err != nil {
		t.Fatalf("start daemon: %v", err)
	}
	t.Cleanup(func() {
		_, _ = run("kill", "-c", cfg)
		_ = dcmd.Process.Kill()
		_, _ = dcmd.Process.Wait()
	})

	// Wait until status reports it running.
	deadline := time.Now().Add(5 * time.Second)
	up := false
	for time.Now().Before(deadline) {
		if out, _ := run("status", "-c", cfg); strings.Contains(out, "daemon running") && strings.Contains(out, "web") {
			up = true
			break
		}
		time.Sleep(50 * time.Millisecond)
	}
	if !up {
		t.Fatal("daemon never reported running via status")
	}

	// Kill it.
	if out, _ := run("kill", "-c", cfg); !strings.Contains(out, "daemon stopped") {
		t.Fatalf("kill: want 'daemon stopped', got %q", out)
	}

	// Status confirms it's gone.
	deadline = time.Now().Add(5 * time.Second)
	gone := false
	for time.Now().Before(deadline) {
		if out, _ := run("status", "-c", cfg); strings.Contains(out, "no daemon running") {
			gone = true
			break
		}
		time.Sleep(50 * time.Millisecond)
	}
	if !gone {
		t.Fatal("daemon still reported after kill")
	}
}
