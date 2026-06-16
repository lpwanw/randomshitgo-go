//go:build !windows

package main

import (
	"path/filepath"
	"testing"
)

// daemonLockFree must report false while a daemon holds the pidfile lock and
// true once released. This is the guard that prevents a concurrent launcher
// from unlinking a live daemon's socket (the F6 spawn-race).
func TestDaemonLockFree(t *testing.T) {
	pid := filepath.Join(t.TempDir(), "test.pid")

	if !daemonLockFree(pid) {
		t.Fatalf("expected lock free before any daemon holds it")
	}

	// Simulate a running daemon holding the lock.
	lock, err := acquireDaemonLock(pid)
	if err != nil {
		t.Fatalf("acquireDaemonLock: %v", err)
	}
	if daemonLockFree(pid) {
		t.Fatalf("lock must read as held while a daemon owns it")
	}

	// Releasing (daemon exit) frees it again.
	lock.Close()
	if !daemonLockFree(pid) {
		t.Fatalf("lock must read as free after release")
	}
}

// acquireDaemonLock must refuse a second holder (double-start guard).
func TestAcquireDaemonLock_DoubleStartRefused(t *testing.T) {
	pid := filepath.Join(t.TempDir(), "test.pid")

	a, err := acquireDaemonLock(pid)
	if err != nil {
		t.Fatalf("first acquire: %v", err)
	}
	defer a.Close()

	if _, err := acquireDaemonLock(pid); err == nil {
		t.Fatalf("second acquire must fail while the first holds the lock")
	}
}