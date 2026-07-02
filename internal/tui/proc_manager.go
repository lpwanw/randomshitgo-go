package tui

import (
	"context"
	"io"
	"os"
	"time"

	"github.com/lpwanw/procs/internal/config"
	"github.com/lpwanw/procs/internal/process"
)

// ProcManager is the process-control surface the TUI depends on. The in-process
// supervisor (*process.Manager) and the daemon client (*client.RemoteManager)
// both satisfy it, so the same TUI drives either backend.
//
// It mirrors the full method set the TUI calls — this is a transport seam, not a
// minimal abstraction. In daemon mode the client's Attach/Subscribe/Reload
// return an "unavailable while detached" error (remote attach is a v2 feature),
// which the attach handlers surface as a toast.
type ProcManager interface {
	Start(id string) error
	Stop(id string, grace time.Duration) error
	Restart(id string) error
	StartGroup(name string, delay time.Duration) error
	StopAll(ctx context.Context)
	Resize(id string, cols, rows uint16)
	Attach(id string) (*os.File, error)
	Subscribe(id string, w io.Writer) (func(), error)
	Reload(newCfg *config.Config) process.ReloadResult
	Close()
}

// Compile-time assertion that the in-process Manager satisfies the interface.
var _ ProcManager = (*process.Manager)(nil)
