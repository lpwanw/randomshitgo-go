//go:build !windows

package daemon

import (
	"net"
	"sync"
	"time"

	"github.com/lpwanw/randomshitgo-go/internal/ipc"
)

// maxLogQueue bounds the per-client backlog of log-line envelopes. When full,
// the oldest log line is dropped (lossy). Control events are never dropped.
const maxLogQueue = 4096

// maxCtrlQueue is a safety ceiling on the lossless control backlog. A client so
// wedged that it accumulates this many undelivered control events is treated as
// dead and dropped; on reconnect it receives a fresh snapshot, which resyncs
// state without dropping any individual event silently.
const maxCtrlQueue = 1 << 16

// writeTimeout bounds a single frame write so a wedged client is detected and
// dropped instead of blocking the writer (and growing the control queue)
// forever.
const writeTimeout = 5 * time.Second

// clientConn is the server's view of the single connected TUI client. It holds
// two queues: a lossless control queue and a lossy bounded log queue, drained
// by writeLoop with control priority.
type clientConn struct {
	conn net.Conn
	enc  *ipc.Encoder
	srv  *Server

	mu     sync.Mutex
	cond   *sync.Cond
	ctrl   []*ipc.Envelope
	logq   []*ipc.Envelope
	closed bool
}

func newClientConn(conn net.Conn, srv *Server) *clientConn {
	c := &clientConn{conn: conn, enc: ipc.NewEncoder(conn), srv: srv}
	c.cond = sync.NewCond(&c.mu)
	return c
}

// enqueue queues an envelope. Log lines go to the lossy bounded queue; every
// other envelope (snapshot, state/exit/restart, toast) is lossless.
func (c *clientConn) enqueue(env *ipc.Envelope) {
	c.mu.Lock()
	if c.closed {
		c.mu.Unlock()
		return
	}
	if isLogLine(env) {
		if len(c.logq) >= maxLogQueue {
			c.logq = c.logq[1:] // drop oldest
		}
		c.logq = append(c.logq, env)
	} else {
		if len(c.ctrl) >= maxCtrlQueue {
			// Wedged client: drop it rather than grow without bound. Reconnect
			// resyncs via a fresh snapshot.
			c.mu.Unlock()
			c.close()
			return
		}
		c.ctrl = append(c.ctrl, env)
	}
	c.mu.Unlock()
	c.cond.Signal()
}

func isLogLine(env *ipc.Envelope) bool {
	return env.Kind == ipc.KindEvent && env.Event != nil && env.Event.Kind == ipc.EvLogLine
}

// writeLoop drains queued envelopes (control first) until closed or a write
// fails. A failed write drops the client.
func (c *clientConn) writeLoop() {
	for {
		c.mu.Lock()
		for !c.closed && len(c.ctrl) == 0 && len(c.logq) == 0 {
			c.cond.Wait()
		}
		if c.closed {
			c.mu.Unlock()
			return
		}
		var env *ipc.Envelope
		if len(c.ctrl) > 0 {
			env = c.ctrl[0]
			c.ctrl = c.ctrl[1:]
		} else {
			env = c.logq[0]
			c.logq = c.logq[1:]
		}
		c.mu.Unlock()

		_ = c.conn.SetWriteDeadline(time.Now().Add(writeTimeout))
		if err := c.enc.Write(env); err != nil {
			c.srv.dropClient(c)
			return
		}
	}
}

// readLoop decodes client commands and dispatches them. A decode panic from a
// malformed frame is contained so it drops only this connection. On any read
// error the client is unregistered.
func (c *clientConn) readLoop() {
	defer func() {
		_ = recover() // contain a hostile/malformed-frame decode panic
		c.srv.unregisterClient(c)
		c.close()
	}()
	dec := ipc.NewDecoder(c.conn)
	for {
		env, err := dec.Read()
		if err != nil {
			return
		}
		if env.Kind == ipc.KindCommand && env.Command != nil {
			c.srv.dispatch(env.Command, c)
		}
	}
}

// close marks the connection closed, wakes the writer, and closes the socket.
// Idempotent.
func (c *clientConn) close() {
	c.mu.Lock()
	if c.closed {
		c.mu.Unlock()
		return
	}
	c.closed = true
	c.mu.Unlock()
	c.cond.Broadcast()
	_ = c.conn.Close()
}