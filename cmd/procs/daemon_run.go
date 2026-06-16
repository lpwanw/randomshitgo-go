//go:build !windows

package main

import (
	"flag"
	"fmt"
	"os"
	"os/signal"
	"strconv"
	"syscall"

	"github.com/lpwanw/randomshitgo-go/internal/config"
	"github.com/lpwanw/randomshitgo-go/internal/daemon"
	"github.com/lpwanw/randomshitgo-go/internal/ipc"
	"github.com/lpwanw/randomshitgo-go/internal/process"
	"github.com/lpwanw/randomshitgo-go/internal/state"
	"golang.org/x/sys/unix"
)

// runDaemon runs the headless supervisor for the given args (everything after
// the "daemon"/"__daemon" subcommand). It blocks until shutdown, then cleans up
// the socket, pidfile, and children file. Returns a process exit code.
func runDaemon(args []string) int {
	fs := flag.NewFlagSet("daemon", flag.ContinueOnError)
	cfgPath := fs.String("config", "", "path to config.yml")
	fs.StringVar(cfgPath, "c", "", "alias for --config")
	if err := fs.Parse(args); err != nil {
		return 2
	}

	resolved, err := config.ResolvePath(*cfgPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "procs daemon: config error: %v\n", err)
		return 1
	}
	cfg, err := config.LoadFromPath(resolved)
	if err != nil {
		fmt.Fprintf(os.Stderr, "procs daemon: config error: %v\n", err)
		return 1
	}

	if _, err := ipc.EnsureSecureDir(); err != nil {
		fmt.Fprintf(os.Stderr, "procs daemon: %v\n", err)
		return 1
	}
	sock, _ := ipc.SocketPath(resolved)
	pid, _ := ipc.PidPath(resolved)
	children, _ := ipc.ChildrenPath(resolved)

	// An advisory file lock is the single source of truth for "is a daemon
	// already running". Unlike an O_EXCL pidfile it auto-releases on process
	// death, so a crashed daemon never blocks the next start. Holding the lock
	// proves no live daemon owns this config, so any leftover socket is stale.
	lockFile, err := acquireDaemonLock(pid)
	if err != nil {
		fmt.Fprintf(os.Stderr, "procs daemon: %v\n", err)
		return 1
	}
	defer lockFile.Close() // releases the flock
	defer os.Remove(pid)

	if err := ipc.RemoveStaleSocket(sock); err != nil {
		fmt.Fprintf(os.Stderr, "procs daemon: %v\n", err)
		return 1
	}
	defer os.Remove(sock)
	defer os.Remove(children)

	reg := state.NewRegistry(cfg.Settings)
	defer reg.Close()
	rt := state.NewRuntimeStore()
	mgr := process.New(cfg, reg)

	ids := make([]string, 0, len(cfg.Projects))
	for id := range cfg.Projects {
		ids = append(ids, id)
	}
	rt.Seed(ids)

	srv := daemon.New(cfg, resolved, rt, mgr, children)

	// SIGTERM/SIGINT request orderly shutdown; SIGHUP is ignored so the daemon
	// survives the launching terminal closing.
	sigs := make(chan os.Signal, 4)
	signal.Notify(sigs, syscall.SIGTERM, syscall.SIGINT)
	signal.Ignore(syscall.SIGHUP)
	go func() {
		<-sigs
		srv.Shutdown()
	}()

	if err := srv.ListenAndServe(sock); err != nil {
		fmt.Fprintf(os.Stderr, "procs daemon: %v\n", err)
		return 1
	}
	return 0
}

// acquireDaemonLock opens the pidfile (0600) and takes a non-blocking exclusive
// advisory lock. The returned file must be kept open for the daemon's lifetime;
// closing it (or process exit) releases the lock. Fails if another live daemon
// holds the lock.
func acquireDaemonLock(path string) (*os.File, error) {
	f, err := os.OpenFile(path, os.O_CREATE|os.O_RDWR, 0o600)
	if err != nil {
		return nil, fmt.Errorf("pidfile %q: %w", path, err)
	}
	if err := unix.Flock(int(f.Fd()), unix.LOCK_EX|unix.LOCK_NB); err != nil {
		f.Close()
		return nil, fmt.Errorf("another daemon is already running (lock held on %q)", path)
	}
	_ = f.Truncate(0)
	_, _ = f.Seek(0, 0)
	_, _ = f.WriteString(strconv.Itoa(os.Getpid()))
	return f, nil
}