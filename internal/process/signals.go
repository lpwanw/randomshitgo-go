//go:build !windows

package process

import (
	"fmt"
	"os/exec"
	"syscall"
	"time"
)

// gracefulStop sends SIGTERM to the entire process group, waits up to grace
// for the caller to reap the process (via done), then force-kills with SIGKILL.
// It deliberately does NOT call cmd.Wait — the caller's run() goroutine owns
// the single Wait call. done is closed by the caller when cmd.Wait returns.
// Uses negative pgid so the signal reaches all processes in the group.
func gracefulStop(cmd *exec.Cmd, grace time.Duration, done <-chan struct{}) error {
	if cmd == nil || cmd.Process == nil {
		return nil
	}

	pgid, err := syscall.Getpgid(cmd.Process.Pid)
	if err != nil {
		// Fall back to just killing the direct process; caller still reaps.
		_ = cmd.Process.Kill()
		return fmt.Errorf("getpgid: %w", err)
	}

	// SIGTERM to the whole process group.
	_ = syscall.Kill(-pgid, syscall.SIGTERM)

	select {
	case <-done:
		return nil
	case <-time.After(grace):
		// Grace expired — force kill; caller's cmd.Wait will still return.
		_ = syscall.Kill(-pgid, syscall.SIGKILL)
		return nil
	}
}
