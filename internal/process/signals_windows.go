//go:build windows

package process

import (
	"os/exec"
	"time"
)

func gracefulStop(cmd *exec.Cmd, grace time.Duration) error {
	if cmd == nil || cmd.Process == nil {
		return nil
	}
	_ = grace
	return cmd.Process.Kill()
}
