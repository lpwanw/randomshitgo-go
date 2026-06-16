//go:build !windows

package client

import (
	"context"
	"errors"
	"io"
	"os"
	"time"

	"github.com/lpwanw/randomshitgo-go/internal/config"
	"github.com/lpwanw/randomshitgo-go/internal/ipc"
	"github.com/lpwanw/randomshitgo-go/internal/process"
)

// errDetached is returned by operations that are unavailable while detached
// (remote attach is a v2 feature).
var errDetached = errors.New("unavailable in daemon mode — run `procs --no-daemon` to attach")

// RemoteManager implements the process-control surface the TUI depends on by
// shipping each call as a command over the client connection. It mirrors
// *process.Manager's method signatures so the TUI can use either transparently.
type RemoteManager struct {
	c *Client
}

func (m *RemoteManager) Start(id string) error {
	return m.c.send(&ipc.Command{Op: ipc.OpStart, ID: id})
}

func (m *RemoteManager) Stop(id string, _ time.Duration) error {
	// Grace is applied daemon-side from settings; the client just requests stop.
	return m.c.send(&ipc.Command{Op: ipc.OpStop, ID: id})
}

func (m *RemoteManager) Restart(id string) error {
	return m.c.send(&ipc.Command{Op: ipc.OpRestart, ID: id})
}

func (m *RemoteManager) StartGroup(name string, _ time.Duration) error {
	// Inter-member delay is applied daemon-side from settings.
	return m.c.send(&ipc.Command{Op: ipc.OpStartGroup, Group: name})
}

func (m *RemoteManager) StopAll(_ context.Context) {
	_ = m.c.send(&ipc.Command{Op: ipc.OpStopAll})
}

// Resize is a no-op in v1: detached children render at the configured PTY size;
// the client's terminal width does not drive the daemon PTY (resize is wired
// with remote attach in v2).
func (m *RemoteManager) Resize(_ string, _, _ uint16) {}

// Attach is unsupported while detached; the TUI surfaces the error as a toast.
func (m *RemoteManager) Attach(string) (*os.File, error) {
	return nil, errDetached
}

// Subscribe is unsupported while detached.
func (m *RemoteManager) Subscribe(_ string, _ io.Writer) (func(), error) {
	return nil, errDetached
}

// Reload requests a daemon-side config reload. The reconciliation result
// arrives asynchronously as a NotifReload notification; the synchronous return
// is empty (the daemon owns child reconciliation).
func (m *RemoteManager) Reload(_ *config.Config) process.ReloadResult {
	_ = m.c.send(&ipc.Command{Op: ipc.OpReload})
	return process.ReloadResult{}
}

// Close tears down the client connection.
func (m *RemoteManager) Close() { m.c.Close() }