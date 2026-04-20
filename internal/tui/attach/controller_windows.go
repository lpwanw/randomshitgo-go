//go:build windows

package attach

import (
	"context"
	"errors"
	"os"
	"time"
)

const ioBufferSize = 4096

// Controller is a stub for Windows where raw PTY attach is not yet supported.
type Controller struct {
	escapeByte byte
	timeout    time.Duration
}

// NewController returns a Controller.
func NewController(escapeByte byte, timeout time.Duration) *Controller {
	if escapeByte == 0 {
		escapeByte = DefaultEscapeByte
	}
	if timeout == 0 {
		timeout = DefaultTimeout
	}
	return &Controller{escapeByte: escapeByte, timeout: timeout}
}

// Run is a stub on Windows: attach mode is not supported.
func (c *Controller) Run(_ context.Context, _ *os.File) (bool, error) {
	return false, errors.New("attach: PTY bridge not supported on Windows")
}

// isTimeout returns true if err is a deadline-exceeded error.
func isTimeout(err error) bool {
	if err == nil {
		return false
	}
	if nerr, ok := err.(interface{ Timeout() bool }); ok {
		return nerr.Timeout()
	}
	return false
}
