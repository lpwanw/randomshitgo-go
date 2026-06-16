//go:build !windows

package main

import (
	"flag"
	"fmt"
	"os"
	"syscall"
	"time"

	"github.com/lpwanw/randomshitgo-go/internal/config"
	"github.com/lpwanw/randomshitgo-go/internal/daemon"
	"github.com/lpwanw/randomshitgo-go/internal/ipc"
)

// resolveCfgArg parses -c/--config and --orphans from args and resolves the
// config path. orphans is meaningful only for the kill subcommand.
func resolveCfgArg(name string, args []string) (resolved string, orphans bool, err error) {
	fs := flag.NewFlagSet(name, flag.ContinueOnError)
	cfgPath := fs.String("config", "", "path to config.yml")
	fs.StringVar(cfgPath, "c", "", "alias for --config")
	orphansFlag := fs.Bool("orphans", false, "also stop processes left by a crashed daemon")
	if perr := fs.Parse(args); perr != nil {
		return "", false, perr
	}
	resolved, err = config.ResolvePath(*cfgPath)
	return resolved, *orphansFlag, err
}

// runKill stops the daemon (and its children) for the active config. With
// --orphans it also kills processes left behind by a crashed daemon.
func runKill(args []string) int {
	resolved, orphans, err := resolveCfgArg("kill", args)
	if err != nil {
		fmt.Fprintf(os.Stderr, "procs kill: %v\n", err)
		return 1
	}
	sock, _ := ipc.SocketPath(resolved)

	if requestDaemonShutdown(sock) {
		fmt.Println("procs: daemon stopped")
	} else {
		fmt.Println("procs: no daemon running")
	}

	if orphans {
		killOrphans(resolved)
	}
	return 0
}

// killOrphans sends SIGTERM to processes recorded by a crashed daemon.
func killOrphans(resolvedCfgPath string) {
	childrenPath, err := ipc.ChildrenPath(resolvedCfgPath)
	if err != nil {
		return
	}
	recs, _ := daemon.ReadChildren(childrenPath)
	alive := daemon.AliveChildren(recs)
	if len(alive) == 0 {
		fmt.Println("procs: no orphaned processes")
		return
	}
	for _, r := range alive {
		if err := syscall.Kill(r.PID, syscall.SIGTERM); err != nil {
			fmt.Fprintf(os.Stderr, "procs: kill orphan %s (pid %d): %v\n", r.ID, r.PID, err)
			continue
		}
		fmt.Printf("procs: stopped orphan %s (pid %d)\n", r.ID, r.PID)
	}
	_ = os.Remove(childrenPath)
}

// runStatus prints the daemon's project states, or reports no daemon running.
func runStatus(args []string) int {
	resolved, _, err := resolveCfgArg("status", args)
	if err != nil {
		fmt.Fprintf(os.Stderr, "procs status: %v\n", err)
		return 1
	}
	sock, _ := ipc.SocketPath(resolved)

	conn, alive := ipc.DialIfAlive(sock)
	if !alive {
		fmt.Println("procs: no daemon running")
		return 0
	}
	defer conn.Close()

	// The daemon sends a snapshot on connect.
	_ = conn.SetReadDeadline(time.Now().Add(5 * time.Second))
	env, err := ipc.NewDecoder(conn).Read()
	if err != nil || env.Kind != ipc.KindSnapshot || env.Snapshot == nil {
		fmt.Fprintf(os.Stderr, "procs status: daemon did not respond with state\n")
		return 1
	}
	fmt.Printf("procs: daemon running (%d project(s))\n", len(env.Snapshot.Projects))
	for _, p := range env.Snapshot.Projects {
		pid := ""
		if p.PID > 0 {
			pid = fmt.Sprintf("  pid %d", p.PID)
		}
		fmt.Printf("  %-20s %s%s\n", p.ID, p.State, pid)
	}
	return 0
}