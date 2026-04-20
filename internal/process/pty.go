//go:build !windows

package process

import (
	"context"
	"os"
	"os/exec"
	"syscall"

	"github.com/creack/pty"
)

// startPTY spawns cmdStr via "sh -c cmdStr" in a fresh PTY session.
// dir is the working directory; env is the full environment slice.
// cols/rows set the initial PTY window size.
// Setsid=true gives the child its own session so kill(-pgid) kills the
// entire process tree without affecting the parent.
func startPTY(
	_ context.Context,
	dir, cmdStr string,
	env []string,
	cols, rows uint16,
) (*exec.Cmd, *os.File, error) {
	cmd := exec.Command("/bin/sh", "-c", cmdStr)
	cmd.Dir = dir
	cmd.Env = env
	cmd.SysProcAttr = &syscall.SysProcAttr{Setsid: true}

	ptmx, err := pty.Start(cmd)
	if err != nil {
		return nil, nil, err
	}
	_ = pty.Setsize(ptmx, &pty.Winsize{Cols: cols, Rows: rows})
	return cmd, ptmx, nil
}

// resizePTY updates the PTY window size; safe to call from any goroutine.
func resizePTY(ptmx *os.File, cols, rows uint16) error {
	return pty.Setsize(ptmx, &pty.Winsize{Cols: cols, Rows: rows})
}
