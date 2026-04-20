//go:build windows

package process

import (
	"context"
	"fmt"
	"os"
	"os/exec"
)

// startPTY is not supported on Windows in this build.
func startPTY(
	_ context.Context,
	_, _ string,
	_ []string,
	_, _ uint16,
) (*exec.Cmd, *os.File, error) {
	return nil, nil, fmt.Errorf("PTY spawning is not supported on Windows")
}

func resizePTY(_ *os.File, _, _ uint16) error {
	return fmt.Errorf("PTY resize is not supported on Windows")
}
