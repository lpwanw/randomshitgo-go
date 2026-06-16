//go:build !windows

package main

import (
	"fmt"
	"os"
	"os/exec"
	"syscall"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/lpwanw/randomshitgo-go/internal/client"
	"github.com/lpwanw/randomshitgo-go/internal/config"
	"github.com/lpwanw/randomshitgo-go/internal/daemon"
	"github.com/lpwanw/randomshitgo-go/internal/ipc"
	"github.com/lpwanw/randomshitgo-go/internal/state"
	"github.com/lpwanw/randomshitgo-go/internal/tui"
	"golang.org/x/sys/unix"
)

// runAttach connects to the daemon (spawning it if needed) and runs the TUI as
// a client. On exit it detaches (leaving the daemon running) unless the user
// asked to shut down.
func runAttach(resolvedCfgPath string, cfg *config.Config) int {
	cl, err := ensureDaemon(resolvedCfgPath, cfg.Settings.LogBufferLines)
	if err != nil {
		fmt.Fprintf(os.Stderr, "procs: %v\n", err)
		return 1
	}

	ui := state.NewUIStore()
	m := tui.New(cfg, cl.Manager(), cl.Runtime(), ui, cl.Reg(), resolvedCfgPath)
	m.SetDetachMode(true)
	p := tea.NewProgram(m, tea.WithAltScreen(), tea.WithMouseCellMotion())
	m.SetProgram(p)

	// Pump daemon notifications into the tea loop.
	go func() {
		for n := range cl.Notifications() {
			if msg := notifToMsg(n); msg != nil {
				p.Send(msg)
			}
		}
	}()

	finalModel, err := p.Run()
	if err != nil {
		fmt.Fprintf(os.Stderr, "procs: %v\n", err)
		cl.Close()
		return 1
	}

	shutdown := false
	if fm, ok := finalModel.(tui.Model); ok && fm.QuitReason() == "shutdown" {
		shutdown = true
	}
	cl.Close() // detach our client first so it isn't taken over mid-shutdown

	if shutdown {
		if sock, serr := ipc.SocketPath(resolvedCfgPath); serr == nil {
			requestDaemonShutdown(sock)
		}
	}
	return 0
}

// requestDaemonShutdown sends OpShutdown on a dedicated connection and drains it
// until the daemon closes it (EOF) on shutdown completion. Keeping this the
// active client connection — rather than sending then closing or probing with
// fresh connections — avoids losing the command to a close race or the
// single-client take-over. Returns true if a daemon was contacted.
func requestDaemonShutdown(sock string) bool {
	conn, alive := ipc.DialIfAlive(sock)
	if !alive {
		return false
	}
	defer conn.Close()
	if err := ipc.NewEncoder(conn).Write(&ipc.Envelope{Kind: ipc.KindCommand, Command: &ipc.Command{Op: ipc.OpShutdown}}); err != nil {
		return true
	}
	_ = conn.SetReadDeadline(time.Now().Add(10 * time.Second))
	dec := ipc.NewDecoder(conn)
	for {
		if _, err := dec.Read(); err != nil {
			return true
		}
	}
}

// ensureDaemon returns a connected client, reattaching to a live daemon or
// spawning a fresh one. It warns about orphaned children left by a crash.
func ensureDaemon(resolvedCfgPath string, bufLines int) (*client.Client, error) {
	if _, err := ipc.EnsureSecureDir(); err != nil {
		return nil, err
	}
	sock, err := ipc.SocketPath(resolvedCfgPath)
	if err != nil {
		return nil, err
	}

	if conn, alive := ipc.DialIfAlive(sock); alive {
		_ = conn.Close() // reconnect cleanly via client.Dial below
		return client.Dial(sock, bufLines)
	}

	// Not accepting yet. A daemon holds the pidfile lock for its whole life
	// (acquired before it binds the socket), so a held lock means a daemon is
	// alive — either mid-startup or bound-but-we-raced. In that case we must NOT
	// remove the socket (doing so would unlink a live daemon's socket and brick
	// it); instead wait for it to accept. Only when the lock is free is there
	// genuinely no daemon, making any leftover socket stale and safe to clear.
	pidPath, err := ipc.PidPath(resolvedCfgPath)
	if err != nil {
		return nil, err
	}
	if !daemonLockFree(pidPath) {
		if werr := waitForSocket(sock, 5*time.Second); werr != nil {
			return nil, fmt.Errorf("a daemon holds the lock but is not accepting (check the daemon log): %w", werr)
		}
		return client.Dial(sock, bufLines)
	}

	warnOrphans(resolvedCfgPath)

	if err := ipc.RemoveStaleSocket(sock); err != nil {
		return nil, err
	}
	if err := spawnDaemon(resolvedCfgPath); err != nil {
		return nil, err
	}
	if err := waitForSocket(sock, 5*time.Second); err != nil {
		return nil, err
	}
	return client.Dial(sock, bufLines)
}

// daemonLockFree reports whether no live daemon holds the pidfile lock. It
// briefly try-locks and releases; a held lock (EWOULDBLOCK) means a daemon is
// alive. On any error it conservatively returns false (assume a daemon may
// exist) so the caller never removes a socket it isn't sure is stale.
func daemonLockFree(pidPath string) bool {
	f, err := os.OpenFile(pidPath, os.O_CREATE|os.O_RDWR, 0o600)
	if err != nil {
		return false
	}
	defer f.Close()
	if err := unix.Flock(int(f.Fd()), unix.LOCK_EX|unix.LOCK_NB); err != nil {
		return false // held by a live daemon
	}
	_ = unix.Flock(int(f.Fd()), unix.LOCK_UN)
	return true
}

// warnOrphans surfaces processes left running by a previously crashed daemon.
func warnOrphans(resolvedCfgPath string) {
	childrenPath, err := ipc.ChildrenPath(resolvedCfgPath)
	if err != nil {
		return
	}
	recs, _ := daemon.ReadChildren(childrenPath)
	alive := daemon.AliveChildren(recs)
	if len(alive) == 0 {
		return
	}
	fmt.Fprintf(os.Stderr, "procs: warning: %d process(es) from a previously crashed daemon are still running:\n", len(alive))
	for _, r := range alive {
		fmt.Fprintf(os.Stderr, "  %s (pid %d)\n", r.ID, r.PID)
	}
	fmt.Fprintf(os.Stderr, "  run `procs kill --orphans` to stop them before they conflict with new processes.\n")
}

// spawnDaemon re-execs this binary as a detached daemon. It uses a new session
// (Setsid) and redirects std streams to the daemon log so it survives the
// launching terminal closing. The daemon inherits the full environment because
// child processes derive their env from the daemon's — stripping it would break
// projects that rely on the developer's shell environment.
func spawnDaemon(resolvedCfgPath string) error {
	exe, err := os.Executable()
	if err != nil {
		return fmt.Errorf("locate executable: %w", err)
	}
	logPath, err := ipc.DaemonLogPath(resolvedCfgPath)
	if err != nil {
		return err
	}
	logf, err := os.OpenFile(logPath, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o600)
	if err != nil {
		return fmt.Errorf("open daemon log: %w", err)
	}
	defer logf.Close()

	cmd := exec.Command(exe, "__daemon", "-c", resolvedCfgPath)
	cmd.SysProcAttr = &syscall.SysProcAttr{Setsid: true}
	cmd.Stdin = nil
	cmd.Stdout = logf
	cmd.Stderr = logf
	// cmd.Env nil => inherit full environment (children need it).

	if err := cmd.Start(); err != nil {
		return fmt.Errorf("spawn daemon: %w", err)
	}
	// Do not Wait — the daemon outlives us; release the handle so we don't reap.
	return cmd.Process.Release()
}

// waitForSocket blocks until the daemon socket accepts a connection or timeout.
func waitForSocket(sock string, timeout time.Duration) error {
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if conn, alive := ipc.DialIfAlive(sock); alive {
			_ = conn.Close()
			return nil
		}
		time.Sleep(30 * time.Millisecond)
	}
	return fmt.Errorf("daemon did not start within %s (check the daemon log)", timeout)
}

// notifToMsg converts a client notification into a TUI message, or nil to skip.
func notifToMsg(n client.Notification) tea.Msg {
	switch n.Kind {
	case client.NotifConnLost:
		return tui.ConnLostMsg{}
	case client.NotifToast:
		if n.Toast == nil {
			return nil
		}
		return tui.DaemonToastMsg{Text: n.Toast.Text, Level: toastLevel(n.Toast.Level)}
	case client.NotifReload:
		if n.Reload == nil {
			return nil
		}
		return tui.ReloadResultMsg{
			Added:   n.Reload.Added,
			Removed: n.Reload.Removed,
			Changed: n.Reload.Changed,
			Stopped: n.Reload.Stopped,
		}
	}
	return nil
}

// toastLevel maps a daemon toast level string to the TUI's numeric level.
func toastLevel(s string) int {
	switch s {
	case "warn":
		return 1
	case "error":
		return 2
	default:
		return 0
	}
}