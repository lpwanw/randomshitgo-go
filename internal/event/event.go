// Package event holds shared event types used by the process and state layers.
// Placing them here breaks the potential cyclic import between process (emitter)
// and state (consumer): both import event; neither imports the other.
package event

import (
	"time"
)

// Event is the common interface for all lifecycle events. ProjectID returns the
// config project key the event belongs to.
type Event interface {
	ProjectID() string
}

// StartedEvent is emitted when a child process successfully starts.
type StartedEvent struct {
	ID  string
	PID int
	At  time.Time
}

func (e StartedEvent) ProjectID() string { return e.ID }

// ExitedEvent is emitted when a child process exits (clean or crash).
// Signal is the symbolic signal name (e.g. "SIGTERM") or "" for code exits.
type ExitedEvent struct {
	ID     string
	Code   int
	Signal string
	At     time.Time
}

func (e ExitedEvent) ProjectID() string { return e.ID }

// StateChangedEvent is emitted whenever the process state transitions.
type StateChangedEvent struct {
	ID    string
	State string // matches process.State constants
}

func (e StateChangedEvent) ProjectID() string { return e.ID }

// LogLineEvent carries a single log line from a child's PTY output.
// Bytes may contain ANSI escape sequences; consumers strip as needed.
type LogLineEvent struct {
	ID        string
	Bytes     []byte
	IsPartial bool
	At        time.Time
}

func (e LogLineEvent) ProjectID() string { return e.ID }

// RestartingEvent is emitted when the restart policy schedules a restart.
type RestartingEvent struct {
	ID      string
	Attempt int
	Delay   time.Duration
}

func (e RestartingEvent) ProjectID() string { return e.ID }
