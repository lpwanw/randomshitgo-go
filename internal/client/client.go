//go:build !windows

package client

import (
	"fmt"
	"net"
	"sync"
	"sync/atomic"
	"time"

	"github.com/lpwanw/procs/internal/config"
	"github.com/lpwanw/procs/internal/ipc"
	"github.com/lpwanw/procs/internal/log"
	"github.com/lpwanw/procs/internal/state"
)

// NotifKind classifies a non-state notification surfaced to the TUI. Ordinary
// state and log updates flow through the local stores (RuntimeStore.Subscribe
// drives repaints); only these out-of-band signals need an explicit channel.
type NotifKind int

const (
	NotifConnLost NotifKind = iota + 1
	NotifToast
	NotifReload
)

// Notification is a non-state message from the daemon (or the transport).
type Notification struct {
	Kind   NotifKind
	Toast  *ipc.Toast
	Reload *ipc.ReloadResult
}

// Client mirrors daemon state into local stores and sends commands back.
type Client struct {
	conn net.Conn
	enc  *ipc.Encoder
	reg  *state.Registry
	rt   *state.RuntimeStore

	notif  chan Notification
	closed atomic.Bool
	once   sync.Once
}

// Dial connects to the daemon socket, reads the initial snapshot to seed local
// stores, and starts streaming events. bufLines sizes the local log rings to
// match the daemon's log_buffer_lines.
func Dial(sock string, bufLines int) (*Client, error) {
	conn, err := net.Dial("unix", sock)
	if err != nil {
		return nil, err
	}
	c := &Client{
		conn:  conn,
		enc:   ipc.NewEncoder(conn),
		reg:   state.NewRegistry(config.Settings{LogBufferLines: bufLines}), // LogDir "" => ring-only
		rt:    state.NewRuntimeStore(),
		notif: make(chan Notification, 64),
	}

	dec := ipc.NewDecoder(conn)
	_ = conn.SetReadDeadline(time.Now().Add(10 * time.Second))
	env, err := dec.Read()
	if err != nil {
		_ = conn.Close()
		return nil, fmt.Errorf("client: read snapshot: %w", err)
	}
	if env.Kind != ipc.KindSnapshot || env.Snapshot == nil {
		_ = conn.Close()
		return nil, fmt.Errorf("client: expected snapshot, got kind=%d", env.Kind)
	}
	_ = conn.SetReadDeadline(time.Time{})
	c.applySnapshot(env.Snapshot)

	go c.readLoop(dec)
	return c, nil
}

// Reg returns the local ring-only registry the TUI renders log panes from.
func (c *Client) Reg() *state.Registry { return c.reg }

// Runtime returns the local runtime store the TUI renders the sidebar from.
func (c *Client) Runtime() *state.RuntimeStore { return c.rt }

// Notifications returns the channel of out-of-band daemon/transport messages.
func (c *Client) Notifications() <-chan Notification { return c.notif }

// Manager returns a RemoteManager that ships commands over this connection.
func (c *Client) Manager() *RemoteManager { return &RemoteManager{c: c} }

// Close stops the stream and closes the connection. Idempotent.
func (c *Client) Close() {
	c.once.Do(func() {
		c.closed.Store(true)
		_ = c.conn.Close()
	})
}

// send writes one command envelope.
func (c *Client) send(cmd *ipc.Command) error {
	return c.enc.Write(&ipc.Envelope{Kind: ipc.KindCommand, Command: cmd})
}

// readLoop applies streamed events/snapshots to local stores until the
// connection ends, then signals connection loss (unless closed deliberately).
func (c *Client) readLoop(dec *ipc.Decoder) {
	for {
		env, err := dec.Read()
		if err != nil {
			if !c.closed.Load() {
				c.emit(Notification{Kind: NotifConnLost})
			}
			return
		}
		switch env.Kind {
		case ipc.KindSnapshot:
			if env.Snapshot != nil {
				c.applySnapshot(env.Snapshot)
			}
		case ipc.KindEvent:
			c.applyEvent(env.Event)
		}
	}
}

// applySnapshot seeds local stores from a snapshot. Lines are pushed as
// log.Line values directly into the ring (no WriteRaw → no re-split, no disk).
func (c *Client) applySnapshot(snap *ipc.Snapshot) {
	for _, p := range snap.Projects {
		c.rt.Set(p.ID, p.State, p.PID)
		if len(p.Lines) > 0 {
			c.reg.Get(p.ID).Ring.PushMany(p.Lines)
		}
	}
}

// applyEvent applies one streamed event. Wire payloads are pointers; they are
// dereferenced to the value type before RuntimeStore.Apply, which type-switches
// on value types (a pointer would silently no-op).
func (c *Client) applyEvent(em *ipc.EventMsg) {
	if em == nil {
		return
	}
	switch em.Kind {
	case ipc.EvStarted:
		if em.Started != nil {
			c.rt.Apply(*em.Started)
		}
	case ipc.EvExited:
		if em.Exited != nil {
			c.rt.Apply(*em.Exited)
		}
	case ipc.EvStateChanged:
		if em.StateChanged != nil {
			c.rt.Apply(*em.StateChanged)
		}
	case ipc.EvRestarting:
		if em.Restarting != nil {
			c.rt.Apply(*em.Restarting)
		}
	case ipc.EvLogLine:
		if em.LogLine != nil {
			ll := em.LogLine
			// Rendered is left empty; the log panel decodes lazily on repaint.
			c.reg.Get(ll.ID).Ring.Push(log.Line{Bytes: ll.Bytes, IsPartial: ll.IsPartial, Timestamp: ll.At})
		}
	case ipc.EvToast:
		if em.Toast != nil {
			c.emit(Notification{Kind: NotifToast, Toast: em.Toast})
		}
	case ipc.EvReloadResult:
		if em.ReloadResult != nil {
			c.emit(Notification{Kind: NotifReload, Reload: em.ReloadResult})
		}
	case ipc.EvErr:
		c.emit(Notification{Kind: NotifToast, Toast: &ipc.Toast{Text: em.Err, Level: "error"}})
	}
}

// emit delivers a notification without blocking the read loop.
func (c *Client) emit(n Notification) {
	select {
	case c.notif <- n:
	default:
	}
}