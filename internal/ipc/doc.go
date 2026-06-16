// Package ipc defines the wire protocol and socket-path resolution shared by
// the procs daemon and TUI client.
//
// Transport is a single unix-domain-socket control connection per client.
// Each direction is an independent stream of length-prefixed frames; every
// frame carries one gob-encoded Envelope. The length prefix lets the reader
// reject oversized frames before allocating, containing hostile/buggy peers
// (gob alone pre-allocates from attacker-controlled lengths).
//
// Socket and pidfile paths live under the user cache dir, keyed by a hash of
// the resolved config path so distinct configs get distinct daemons. Path
// helpers enforce owner-only permissions and refuse symlinked targets to
// avoid TOCTOU/symlink-swap attacks in a shared cache directory.
package ipc
