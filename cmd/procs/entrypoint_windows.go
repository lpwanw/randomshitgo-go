//go:build windows

package main

import (
	"fmt"
	"os"

	"github.com/lpwanw/procs/internal/config"
)

// handleSubcommand reports that daemon-mode subcommands are unavailable on
// Windows (no unix-domain-socket daemon); procs runs in-process instead.
func handleSubcommand(name string, _ []string) (int, bool) {
	switch name {
	case "daemon", "__daemon", "kill", "stop", "status":
		fmt.Fprintln(os.Stderr, "procs: detach mode (daemon) is not supported on Windows; procs runs in-process here")
		return 1, true
	}
	return 0, false
}

// runDefault runs the in-process TUI (Windows has no daemon).
func runDefault(resolvedCfgPath string, cfg *config.Config) int {
	return runInProcess(resolvedCfgPath, cfg)
}
