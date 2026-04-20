//go:build !windows

package process

import (
	"fmt"
	"os/exec"
	"syscall"
	"time"
)

// gracefulStop sends SIGTERM to the entire process group, waits up to grace,
// then force-kills with SIGKILL if still alive.
// Uses negative pgid so the signal reaches all processes in the group.
func gracefulStop(cmd *exec.Cmd, grace time.Duration) error {
	if cmd == nil || cmd.Process == nil {
		return nil
	}

	pgid, err := syscall.Getpgid(cmd.Process.Pid)
	if err != nil {
		// Fall back to just killing the direct process.
		_ = cmd.Process.Kill()
		return fmt.Errorf("getpgid: %w", err)
	}

	// SIGTERM to the whole process group.
	_ = syscall.Kill(-pgid, syscall.SIGTERM)

	done := make(chan error, 1)
	go func() {
		_, werr := cmd.Process.Wait()
		done <- werr
	}()

	select {
	case err := <-done:
		return err
	case <-time.After(grace):
		// Grace expired — force kill.
		_ = syscall.Kill(-pgid, syscall.SIGKILL)
		<-done
		return nil
	}
}
