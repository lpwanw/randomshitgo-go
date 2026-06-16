//go:build !windows

package ipc

import (
	"os"
	"path/filepath"
	"testing"
)

func TestSocketPath_DeterministicPerConfig(t *testing.T) {
	a1, err := SocketPath("/home/u/.config/procs/config.yml")
	if err != nil {
		t.Fatal(err)
	}
	a2, err := SocketPath("/home/u/.config/procs/config.yml")
	if err != nil {
		t.Fatal(err)
	}
	if a1 != a2 {
		t.Fatalf("same config path produced different sockets: %q vs %q", a1, a2)
	}
	b, err := SocketPath("/home/u/other.yml")
	if err != nil {
		t.Fatal(err)
	}
	if a1 == b {
		t.Fatalf("distinct configs collided on socket path: %q", a1)
	}
	if filepath.Ext(a1) != ".sock" {
		t.Fatalf("socket path missing .sock suffix: %q", a1)
	}
}

func TestPathSuffixesDistinct(t *testing.T) {
	cfg := "/x/config.yml"
	sock, _ := SocketPath(cfg)
	pid, _ := PidPath(cfg)
	dlog, _ := DaemonLogPath(cfg)
	children, _ := ChildrenPath(cfg)
	seen := map[string]bool{}
	for _, p := range []string{sock, pid, dlog, children} {
		if seen[p] {
			t.Fatalf("path collision: %q", p)
		}
		seen[p] = true
	}
}

func TestEnsureSecureDir_ForcesOwnerOnlyMode(t *testing.T) {
	base := t.TempDir()
	dir := filepath.Join(base, "procs")
	// Pre-create at a loose mode, mimicking the log rotator's 0o755.
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := ensureSecureDir(dir); err != nil {
		t.Fatalf("ensureSecureDir: %v", err)
	}
	fi, err := os.Stat(dir)
	if err != nil {
		t.Fatal(err)
	}
	if fi.Mode().Perm() != 0o700 {
		t.Fatalf("want 0700, got %o", fi.Mode().Perm())
	}
}

func TestRemoveStaleSocket_RefusesSymlink(t *testing.T) {
	dir := t.TempDir()
	target := filepath.Join(dir, "real.txt")
	if err := os.WriteFile(target, []byte("keep me"), 0o600); err != nil {
		t.Fatal(err)
	}
	link := filepath.Join(dir, "daemon.sock")
	if err := os.Symlink(target, link); err != nil {
		t.Skipf("symlink unsupported: %v", err)
	}
	if err := RemoveStaleSocket(link); err == nil {
		t.Fatalf("expected refusal to remove symlinked socket")
	}
	if _, err := os.Stat(target); err != nil {
		t.Fatalf("symlink target was deleted: %v", err)
	}
}

func TestRemoveStaleSocket_MissingIsNoError(t *testing.T) {
	dir := t.TempDir()
	if err := RemoveStaleSocket(filepath.Join(dir, "nope.sock")); err != nil {
		t.Fatalf("missing socket should be a no-op, got %v", err)
	}
}

func TestRemoveStaleSocket_RefusesNonSocket(t *testing.T) {
	dir := t.TempDir()
	regular := filepath.Join(dir, "regular.sock")
	if err := os.WriteFile(regular, nil, 0o600); err != nil {
		t.Fatal(err)
	}
	if err := RemoveStaleSocket(regular); err == nil {
		t.Fatalf("expected refusal to remove non-socket file")
	}
}