package ipc

import (
	"github.com/lpwanw/randomshitgo-go/internal/event"
	"github.com/lpwanw/randomshitgo-go/internal/log"
)

// Kind tags the top-level Envelope payload.
type Kind uint8

const (
	KindCommand  Kind = iota + 1 // client -> daemon
	KindEvent                    // daemon -> client
	KindSnapshot                 // daemon -> client (sent once on connect)
)

// Op is a client -> daemon command verb. Attach ops are intentionally absent
// in v1 (remote raw-PTY attach is deferred).
type Op uint8

const (
	OpStart Op = iota + 1
	OpStop
	OpRestart
	OpStartGroup
	OpStopAll
	OpReload
	OpShutdown
	OpStatus
)

// Command is a single client -> daemon instruction.
type Command struct {
	Op    Op
	ID    string // project id (Start/Stop/Restart)
	Group string // group name (StartGroup)
}

// EventKind tags which concrete payload an EventMsg carries. Payloads are
// concrete pointer fields (not an interface) so gob needs no type registration.
type EventKind uint8

const (
	EvStarted EventKind = iota + 1
	EvExited
	EvStateChanged
	EvLogLine
	EvRestarting
	EvReloadResult
	EvToast
	EvErr
)

// ReloadResult mirrors process.ReloadResult over the wire (kept local to avoid
// importing the process package into ipc).
type ReloadResult struct {
	Added   []string
	Removed []string
	Changed []string
	Stopped []string
}

// Toast is a daemon-originated notification surfaced in the client UI.
type Toast struct {
	Text  string
	Level string // "info" | "warn" | "error"
}

// EventMsg wraps exactly one event payload, identified by Kind. Wire payloads
// are pointers; the client derefs to the value type before RuntimeStore.Apply,
// which type-switches on value (not pointer) types.
type EventMsg struct {
	Kind         EventKind
	Started      *event.StartedEvent
	Exited       *event.ExitedEvent
	StateChanged *event.StateChangedEvent
	LogLine      *event.LogLineEvent
	Restarting   *event.RestartingEvent
	ReloadResult *ReloadResult
	Toast        *Toast
	Err          string
}

// ProjectSnap is one project's state at connect time. Lines carries full
// log.Line values (Bytes + Rendered + IsPartial + Timestamp) so the client can
// rebuild its ring without re-splitting newline-stripped bytes.
type ProjectSnap struct {
	ID    string
	State string
	PID   int
	Lines []log.Line
}

// Snapshot is the full initial state sent to a client on connect.
type Snapshot struct {
	Projects []ProjectSnap
}

// Envelope is the single framed unit on the wire. Exactly one of the pointer
// fields is non-nil, selected by Kind.
type Envelope struct {
	Kind     Kind
	Command  *Command
	Event    *EventMsg
	Snapshot *Snapshot
}
