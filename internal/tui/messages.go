package tui

import (
	"time"

	"github.com/lpwanw/randomshitgo-go/internal/gitinfo"
	"github.com/lpwanw/randomshitgo-go/internal/state"
)

// RuntimeChangedMsg is sent when the RuntimeStore notifies of a state change.
// Snapshot is a stable, sorted copy of all project runtimes.
type RuntimeChangedMsg struct {
	Snapshot []state.ProjectRuntime
}

// LogTickMsg is the clock tick used to flush log lines into the log panel.
type LogTickMsg time.Time

// ShowToastMsg requests adding a toast notification to the overlay stack.
type ShowToastMsg struct {
	Text  string
	Level int // 0=info 1=warn 2=err
}

// ToastExpiredMsg signals that the toast pruner tick fired.
type ToastExpiredMsg struct{}

// StartGroupMsg requests starting all processes in the named group.
type StartGroupMsg struct {
	Name string
}

// AttachRequestMsg requests attaching the terminal to the given project PTY.
type AttachRequestMsg struct {
	ID string
}

// AttachEndedMsg is sent when the attached PTY session ends or is detached.
type AttachEndedMsg struct{}

// EmbeddedAttachRequestMsg requests entering embedded-attach mode for the
// given project. Distinct from AttachRequestMsg (the legacy fullscreen flow)
// so the routing layer can pick the right handler.
type EmbeddedAttachRequestMsg struct {
	ID string
}

// VTRefreshMsg, EmbeddedAttachEndedMsg, EmbeddedAttachStartedMsg, and
// DetachFlushMsg are declared in package attach because their lifetime is
// owned by attach.Session — see internal/tui/attach/session.go.

// RestartAllMsg requests restarting every project.
type RestartAllMsg struct{}

// stateRefreshMsg is an internal tick used to re-read UIStore after mutations
// originating from goroutines (e.g. after mgr.Start completes).
type stateRefreshMsg struct{}

// CheckoutBranchMsg is emitted by the branch picker when the user selects a branch.
type CheckoutBranchMsg struct {
	Branch string
}

// statusRefreshTickMsg triggers a 2-second status-bar refresh cycle.
type statusRefreshTickMsg struct{}

// GitInfoMsg delivers fresh git info for a project to the TUI.
type GitInfoMsg struct {
	ID   string
	Info gitinfo.Info
}

// PortInfoMsg delivers the detected TCP listen port for a project.
type PortInfoMsg struct {
	ID   string
	Port int
}

// ProcStatsMsg delivers the per-project CPU% + RSS snapshot. CPU is a
// percentage that can exceed 100 on multi-core processes; RSS is in bytes.
// OK=false when sampling failed (process just died, permissions, etc.) so
// the UI can render an empty segment without surfacing a toast.
type ProcStatsMsg struct {
	ID  string
	CPU float64
	RSS uint64
	OK  bool
}
