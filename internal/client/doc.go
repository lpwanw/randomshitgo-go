//go:build !windows

// Package client connects a TUI to the procs daemon over the control socket.
//
// On Dial it reads the initial Snapshot and seeds a local, ring-only
// state.Registry and RuntimeStore, then streams live events into them. Because
// the local stores are the same types the in-process TUI reads today, the TUI
// renders from them unchanged — only its event source (this socket) and its
// command target (RemoteManager) differ.
//
// The local registry is ring-only (empty LogDir): the daemon owns the on-disk
// log files, so the client must never write them. Log lines from the snapshot
// and the live stream are pushed straight into the in-memory ring as log.Line
// values, preserving line boundaries and partial-line flags.
//
// RemoteManager implements the process-control surface the TUI calls, shipping
// each call as a command over the socket. Attach/Subscribe/Reload return an
// "unavailable while detached" error in v1 (remote attach is a v2 feature; the
// TUI surfaces the error as a toast).
package client