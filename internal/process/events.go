package process

import "github.com/lpwanw/randomshitgo-go/internal/event"

// Re-export event types so callers only need one import.
type Event = event.Event
type StartedEvent = event.StartedEvent
type ExitedEvent = event.ExitedEvent
type StateChangedEvent = event.StateChangedEvent
type LogLineEvent = event.LogLineEvent
type RestartingEvent = event.RestartingEvent

// State constants for process lifecycle — match TS status strings.
const (
	StateIdle       = "idle"
	StateStarting   = "starting"
	StateRunning    = "running"
	StateStopping   = "stopping"
	StateCrashed    = "crashed"
	StateRestarting = "restarting"
	StateGivingUp   = "giving-up"
)
