//go:build !windows

package process

import (
	"context"
	"os"
	"syscall"
	"testing"
	"time"
)

func TestGracefulStop_KillsProcessGroup(t *testing.T) {
	// Spawn "sh -c 'sleep 10'" — sh creates a new session (Setsid), and sleep
	// becomes a child of sh in the same process group.
	cmd, ptmx, err := startPTY(context.Background(), t.TempDir(), "sleep 10", os.Environ(), 80, 24)
	if err != nil {
		t.Fatalf("startPTY: %v", err)
	}
	defer ptmx.Close()

	pid := cmd.Process.Pid

	start := time.Now()
	err = gracefulStop(cmd, 200*time.Millisecond)
	elapsed := time.Since(start)
	if err != nil {
		t.Logf("gracefulStop returned (non-fatal): %v", err)
	}

	// Should finish well within 500ms (SIGTERM should suffice).
	if elapsed > 500*time.Millisecond {
		t.Fatalf("gracefulStop took too long: %v", elapsed)
	}

	// Verify process is gone: kill(pid, 0) returns ESRCH when pid does not exist.
	time.Sleep(20 * time.Millisecond)
	err = syscall.Kill(pid, 0)
	if err == nil {
		t.Fatalf("process %d still alive after gracefulStop", pid)
	}
}
