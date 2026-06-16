//go:build windows

package process

import (
	"os/exec"
	"time"
)

// gracefulStop mirrors the unix signature (done is the run goroutine's
// waitDone channel) but Windows has no graceful signal path, so it simply kills
// the process. grace and done are unused here.
func gracefulStop(cmd *exec.Cmd, grace time.Duration, done <-chan struct{}) error {
	if cmd == nil || cmd.Process == nil {
		return nil
	}
	_ = grace
	_ = done
	return cmd.Process.Kill()
}
