package process

import (
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
	"sync"
	"sync/atomic"
	"syscall"
	"time"

	"github.com/taynguyen/procs/internal/config"
	"github.com/taynguyen/procs/internal/log"
)

// wgDone is an interface for sync.WaitGroup so tests can pass a no-op.
type wgDone interface {
	Add(int)
	Done()
}

const (
	readBufSize = 16 * 1024
)

// Registry is the interface the Child uses to write raw PTY bytes into the
// per-project log ring and rotator. Provided by internal/state.Registry.
type Registry interface {
	WriteRaw(id string, b []byte)
}

// Child owns a single PTY child process: spawn, stream, restart, stop.
type Child struct {
	ID       string
	cfg      config.Project
	settings config.Settings
	reg      Registry
	policy   *Policy
	wg       wgDone // tracks the child's run goroutine in the manager WaitGroup

	mu       sync.Mutex
	state    string
	pid      int
	cmd      *exec.Cmd
	ptmx     *os.File
	cancelFn context.CancelFunc
	// waitDone is closed by run() immediately after cmd.Wait() returns.
	// Stop() passes it to gracefulStop so that function never calls Wait itself.
	waitDone chan struct{}

	// stopRequested is set to 1 when Stop is called to suppress auto-restart.
	stopRequested atomic.Int32

	events chan<- Event
}

func newChild(id string, cfg config.Project, settings config.Settings, reg Registry, events chan<- Event, wg wgDone) *Child {
	return &Child{
		ID:       id,
		cfg:      cfg,
		settings: settings,
		reg:      reg,
		policy: NewPolicy(
			string(cfg.Restart),
			settings.RestartBackoffMs,
			settings.RestartMaxAttempts,
			settings.RestartResetAfterMs,
		),
		state:  StateIdle,
		events: events,
		wg:     wg,
	}
}

func (c *Child) setState(s string) {
	c.mu.Lock()
	c.state = s
	c.mu.Unlock()
	c.emit(StateChangedEvent{ID: c.ID, State: s})
}

func (c *Child) emit(ev Event) {
	// Non-blocking send to avoid deadlock if consumer is slow.
	select {
	case c.events <- ev:
	default:
	}
}

// Start spawns the process in a PTY and begins the reader loop.
// It returns once the process has started (or failed to start).
// The lifecycle goroutine (reader + restart loop) runs in the background.
func (c *Child) Start(ctx context.Context) error {
	c.mu.Lock()
	if c.state != StateIdle && c.state != StateCrashed {
		c.mu.Unlock()
		return fmt.Errorf("child %s: already in state %s", c.ID, c.state)
	}
	c.state = StateStarting
	c.stopRequested.Store(0)
	c.mu.Unlock()

	childCtx, cancel := context.WithCancel(ctx)
	c.mu.Lock()
	c.cancelFn = cancel
	c.mu.Unlock()

	env := buildEnv()
	cols := uint16(c.settings.PtyCols)
	rows := uint16(c.settings.PtyRows)

	cmd, ptmx, err := startPTY(childCtx, c.cfg.Path, c.cfg.Cmd, env, cols, rows)
	if err != nil {
		cancel()
		c.setState(StateIdle)
		return fmt.Errorf("start PTY for %s: %w", c.ID, err)
	}

	c.mu.Lock()
	c.cmd = cmd
	c.ptmx = ptmx
	c.pid = cmd.Process.Pid
	c.waitDone = make(chan struct{})
	c.mu.Unlock()

	c.setState(StateRunning)
	c.policy.OnStart()
	c.emit(StartedEvent{ID: c.ID, PID: cmd.Process.Pid, At: time.Now()})

	// Launch reader goroutine — exits when PTY EOF / error.
	// Register with the manager WaitGroup so Close() can wait for it.
	c.wg.Add(1)
	go c.run(childCtx, cancel)
	return nil
}

// run is the per-child goroutine: reads PTY, feeds log, handles exit/restart.
// It calls wg.Done() exactly once before returning. For the restart case it
// calls wg.Add(1) (via Start) before calling wg.Done() to prevent the
// WaitGroup from reaching zero prematurely.
func (c *Child) run(ctx context.Context, cancel context.CancelFunc) {
	restarting := false
	defer func() {
		cancel()
		if !restarting {
			c.wg.Done()
		}
	}()

	ptmx := c.ptmx
	cmd := c.cmd
	lb := log.NewLineBuffer(0)
	buf := make([]byte, readBufSize)

	// Reader loop.
	for {
		n, err := ptmx.Read(buf)
		if n > 0 {
			chunk := make([]byte, n)
			copy(chunk, buf[:n])
			// Write raw bytes to registry (ring + rotator).
			c.reg.WriteRaw(c.ID, chunk)
			// Emit log line events.
			lines := lb.Feed(chunk)
			for _, line := range lines {
				c.emit(LogLineEvent{
					ID:        c.ID,
					Bytes:     line.Bytes,
					IsPartial: line.IsPartial,
					At:        line.Timestamp,
				})
			}
		}
		if err != nil {
			if err != io.EOF {
				// EIO/EBADF are normal when child exits.
			}
			break
		}
	}

	// Flush trailing partial line.
	if tail := lb.Flush(); tail != nil {
		c.emit(LogLineEvent{
			ID:        c.ID,
			Bytes:     tail.Bytes,
			IsPartial: true,
			At:        tail.Timestamp,
		})
	}

	// Wait for process exit to collect exit code.
	// close(waitDone) immediately after so that a concurrent Stop/gracefulStop
	// knows the process has been reaped and must not call Wait itself.
	_ = cmd.Wait()
	c.mu.Lock()
	if c.waitDone != nil {
		close(c.waitDone)
	}
	c.mu.Unlock()
	ptmx.Close()

	code := 0
	sig := ""
	if ps := cmd.ProcessState; ps != nil {
		code = ps.ExitCode()
		if !ps.Exited() {
			if status, ok := ps.Sys().(syscall.WaitStatus); ok && status.Signaled() {
				sig = status.Signal().String()
				code = -1
			}
		}
	}

	exitedAt := time.Now()
	c.emit(ExitedEvent{ID: c.ID, Code: code, Signal: sig, At: exitedAt})

	// If stop was requested deliberately, go idle without restart.
	if c.stopRequested.Load() == 1 {
		c.mu.Lock()
		c.cmd = nil
		c.ptmx = nil
		c.pid = 0
		c.mu.Unlock()
		c.setState(StateIdle)
		c.policy.Stop()
		return
	}

	action, delay := c.policy.OnExit(code, sig, exitedAt)
	switch action {
	case ActionNone:
		c.mu.Lock()
		c.cmd = nil
		c.ptmx = nil
		c.pid = 0
		c.mu.Unlock()
		if code != 0 {
			c.setState(StateCrashed)
		} else {
			c.setState(StateIdle)
		}
	case ActionGiveUp:
		c.mu.Lock()
		c.cmd = nil
		c.ptmx = nil
		c.pid = 0
		c.mu.Unlock()
		c.setState(StateGivingUp)
	case ActionRestart:
		attempt := c.policy.Attempts()
		c.setState(StateRestarting)
		c.emit(RestartingEvent{ID: c.ID, Attempt: attempt, Delay: delay})

		if delay > 0 {
			select {
			case <-ctx.Done():
				c.mu.Lock()
				c.cmd = nil
				c.ptmx = nil
				c.pid = 0
				c.mu.Unlock()
				c.setState(StateIdle)
				return
			case <-time.After(delay):
			}
		}

		// Check context again after delay.
		select {
		case <-ctx.Done():
			c.mu.Lock()
			c.cmd = nil
			c.ptmx = nil
			c.pid = 0
			c.mu.Unlock()
			c.setState(StateIdle)
			return
		default:
		}

		c.mu.Lock()
		c.cmd = nil
		c.ptmx = nil
		c.pid = 0
		c.state = StateIdle
		c.mu.Unlock()

		// Respawn — use background ctx since the child ctx is done.
		// Set restarting=true before Start so the deferred wg.Done is skipped here;
		// Start will call wg.Add(1) for the new goroutine, then we call wg.Done
		// manually to keep the count balanced.
		restarting = true
		if err := c.Start(context.Background()); err != nil {
			c.setState(StateCrashed)
		}
		// Now call Done for this goroutine (Start already did Add for the next one).
		c.wg.Done()
	}
}

// Stop gracefully shuts down the child using shutdown_grace_ms.
func (c *Child) Stop() error {
	c.stopRequested.Store(1)
	c.mu.Lock()
	cmd := c.cmd
	cancel := c.cancelFn
	waitDone := c.waitDone
	c.mu.Unlock()

	if cancel != nil {
		cancel()
	}
	if cmd == nil {
		return nil
	}
	grace := time.Duration(c.settings.ShutdownGraceMs) * time.Millisecond
	c.setState(StateStopping)
	return gracefulStop(cmd, grace, waitDone)
}

// Restart does a manual restart (resets attempt counter, zero delay).
func (c *Child) Restart(ctx context.Context) error {
	_ = c.Stop()
	// Give the run goroutine a moment to clean up.
	time.Sleep(50 * time.Millisecond)
	c.stopRequested.Store(0)
	c.policy.ManualRestart()
	c.mu.Lock()
	c.state = StateIdle
	c.mu.Unlock()
	return c.Start(ctx)
}

// Resize updates the PTY window size.
func (c *Child) Resize(cols, rows uint16) {
	c.mu.Lock()
	ptmx := c.ptmx
	c.mu.Unlock()
	if ptmx == nil {
		return
	}
	_ = resizePTY(ptmx, cols, rows)
}

// Attach returns the PTY master file so the caller can do raw I/O.
func (c *Child) Attach() (*os.File, error) {
	c.mu.Lock()
	ptmx := c.ptmx
	c.mu.Unlock()
	if ptmx == nil {
		return nil, fmt.Errorf("child %s: not running", c.ID)
	}
	return ptmx, nil
}

// State returns the current state string.
func (c *Child) State() string {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.state
}

// PID returns the current process PID or 0 if not running.
func (c *Child) PID() int {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.pid
}

// buildEnv returns the current environment with PTY-friendly overrides.
func buildEnv() []string {
	env := os.Environ()
	// Ensure colour output.
	env = append(env, "TERM=xterm-256color", "FORCE_COLOR=1", "COLORTERM=truecolor")
	return env
}
