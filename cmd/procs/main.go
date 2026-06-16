package main

import (
	"errors"
	"flag"
	"fmt"
	"os"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/lpwanw/randomshitgo-go/internal/config"
	"github.com/lpwanw/randomshitgo-go/internal/event"
	"github.com/lpwanw/randomshitgo-go/internal/process"
	"github.com/lpwanw/randomshitgo-go/internal/state"
	"github.com/lpwanw/randomshitgo-go/internal/tui"
)

// Build-time variables — injected via ldflags.
var (
	version   = "0.1.0-dev"
	commit    = "unknown"
	buildDate = "unknown"
)

func main() {
	// Subcommand routing is platform-specific (daemon mode is unix-only).
	if len(os.Args) > 1 {
		if code, handled := handleSubcommand(os.Args[1], os.Args[2:]); handled {
			os.Exit(code)
		}
	}

	showVersion := flag.Bool("version", false, "print version and exit")
	noDaemon := flag.Bool("no-daemon", false, "run the TUI in-process (no daemon; enables attach mode, but processes stop on quit)")
	cfgPath := flag.String("config", "", "path to config.yml (default ~/.config/procs/config.yml)")
	flag.StringVar(cfgPath, "c", "", "alias for --config")

	flag.Usage = func() {
		fmt.Fprintf(os.Stderr, `procs - personal multi-project dev juggler

Usage:
  procs [flags]        attach to the daemon (spawning it if needed)
  procs status         show daemon + project state
  procs kill           stop the daemon and all its children
  procs kill --orphans also stop processes left by a crashed daemon

Flags:
  -c, --config PATH    config file (default ~/.config/procs/config.yml)
  --no-daemon          run in-process (no daemon; enables attach, stops on quit)
  --version            print version
  -h, --help           this text

Inside the TUI: :detach (or :q / Ctrl-C) leaves the daemon running; :shutdown stops it.

Config: https://github.com/lpwanw/randomshitgo-go/blob/main/examples/config.yml
`)
	}

	flag.Parse()

	if *showVersion {
		fmt.Printf("procs %s (commit %s, built %s)\n", version, commit, buildDate)
		return
	}

	resolvedCfgPath, cfg := loadConfigOrExit(*cfgPath)
	if *noDaemon {
		// Explicit in-process mode: no daemon, attach available, but processes
		// stop when the TUI quits.
		os.Exit(runInProcess(resolvedCfgPath, cfg))
	}
	os.Exit(runDefault(resolvedCfgPath, cfg))
}

// loadConfigOrExit resolves and loads the config, printing a friendly message
// and exiting on failure.
func loadConfigOrExit(cfgPath string) (string, *config.Config) {
	resolved, err := config.ResolvePath(cfgPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "procs: config error: %v\n", err)
		os.Exit(1)
	}
	cfg, err := config.LoadFromPath(resolved)
	if err != nil {
		var pathErr *os.PathError
		if errors.As(err, &pathErr) {
			fmt.Fprintf(os.Stderr, "procs: config file not found at %q\n", pathErr.Path)
			fmt.Fprintf(os.Stderr, "\nCreate a config at ~/.config/procs/config.yml, or pass -c path:\n")
			fmt.Fprintf(os.Stderr, "  projects:\n    web:\n      path: ~/my-app\n      cmd: npm start\n")
			os.Exit(1)
		}
		fmt.Fprintf(os.Stderr, "procs: config error: %v\n", err)
		os.Exit(1)
	}
	return resolved, cfg
}

// runInProcess runs the legacy single-process TUI (no daemon). Retained for
// platforms/builds without daemon support and as a fallback.
func runInProcess(resolvedCfgPath string, cfg *config.Config) int {
	reg := state.NewRegistry(cfg.Settings)
	rt := state.NewRuntimeStore()
	ui := state.NewUIStore()
	mgr := process.New(cfg, reg)

	ids := make([]string, 0, len(cfg.Projects))
	for id := range cfg.Projects {
		ids = append(ids, id)
	}
	rt.Seed(ids)

	go func() {
		for ev := range mgr.Events() {
			rt.Apply(ev)
			if _, ok := ev.(event.LogLineEvent); ok {
				// no-op: ring already populated by the child via reg.WriteRaw
			}
		}
	}()

	m := tui.New(cfg, mgr, rt, ui, reg, resolvedCfgPath)
	p := tea.NewProgram(m, tea.WithAltScreen(), tea.WithMouseCellMotion())
	m.SetProgram(p)

	if _, err := p.Run(); err != nil {
		fmt.Fprintf(os.Stderr, "procs: %v\n", err)
		mgr.Close()
		return 1
	}
	mgr.Close()
	return 0
}
