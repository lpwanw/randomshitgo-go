//go:build !windows

package main

import "github.com/lpwanw/randomshitgo-go/internal/config"

// handleSubcommand routes daemon-mode subcommands on unix. Returns (code, true)
// when it handled the subcommand, else (0, false) to fall through to the
// default flow.
func handleSubcommand(name string, args []string) (int, bool) {
	switch name {
	case "daemon", "__daemon":
		return runDaemon(args), true
	case "kill", "stop":
		return runKill(args), true
	case "status":
		return runStatus(args), true
	}
	return 0, false
}

// runDefault attaches to the daemon (spawning it if needed) and runs the TUI.
func runDefault(resolvedCfgPath string, cfg *config.Config) int {
	return runAttach(resolvedCfgPath, cfg)
}
