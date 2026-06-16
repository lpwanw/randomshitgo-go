//go:build !windows

// Package daemon implements the headless procs supervisor: it owns the config,
// log registry, runtime store, and process Manager, and serves a single TUI
// client over a unix-domain-socket control connection.
//
// A single source goroutine drains the Manager's event channel. For each event
// it updates the runtime store, appends log lines to per-project broadcast
// rings, and forwards the event to the connected client. Because the snapshot
// sent on connect is also built inside this same goroutine, snapshot and live
// stream are serialized — no event is lost or duplicated at the handoff.
//
// The client lane separates lossless control events (state/exit/restart) from
// lossy log lines: a stalled client may miss log lines (ring replay on
// reconnect covers them) but never a state transition, so the UI can't get
// permanently stuck on a wrong state.
//
// Shutdown is ordered: stop accepting, Close the Manager (stops children and
// closes the event channel), let the source goroutine drain the final events to
// the client, then tear the client down. Socket/pidfile cleanup runs in a defer
// so it executes even if a stop is cut short. SIGHUP is ignored so the daemon
// survives the launching terminal closing.
package daemon