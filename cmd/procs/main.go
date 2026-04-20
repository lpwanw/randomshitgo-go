package main

import (
	"errors"
	"flag"
	"fmt"
	"os"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/taynguyen/procs/internal/config"
	"github.com/taynguyen/procs/internal/event"
	"github.com/taynguyen/procs/internal/process"
	"github.com/taynguyen/procs/internal/state"
	"github.com/taynguyen/procs/internal/tui"
)

// Build-time variables — injected via ldflags.
var (
	version   = "0.1.0-dev"
	commit    = "unknown"
	buildDate = "unknown"
)

func main() {
	showVersion := flag.Bool("version", false, "print version and exit")
	cfgPath := flag.String("config", "", "path to config.yml (default ~/.config/procs/config.yml)")
	flag.StringVar(cfgPath, "c", "", "alias for --config")

	flag.Usage = func() {
		fmt.Fprintf(os.Stderr, `procs - personal multi-project dev juggler

Usage: procs [flags]

Flags:
  -c, --config PATH    config file (default ~/.config/procs/config.yml)
  --version            print version
  -h, --help           this text

Config: https://github.com/taynguyen/procs/blob/main/examples/config.yml
`)
	}

	flag.Parse()

	if *showVersion {
		fmt.Printf("procs %s (commit %s, built %s)\n", version, commit, buildDate)
		return
	}

	cfg, err := config.Load(*cfgPath)
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

	reg := state.NewRegistry(cfg.Settings)
	rt := state.NewRuntimeStore()
	ui := state.NewUIStore()
	mgr := process.New(cfg, reg)

	// Pump manager events into the runtime store.
	go func() {
		for ev := range mgr.Events() {
			rt.Apply(ev)
			// LogLineEvents are already written to the registry ring by the child
			// via reg.WriteRaw; we just acknowledge them here.
			if _, ok := ev.(event.LogLineEvent); ok {
				// no-op: ring already populated
			}
		}
	}()

	m := tui.New(cfg, mgr, rt, ui, reg)
	p := tea.NewProgram(m, tea.WithAltScreen(), tea.WithMouseCellMotion())
	m.SetProgram(p)

	if _, err := p.Run(); err != nil {
		fmt.Fprintf(os.Stderr, "procs: %v\n", err)
		os.Exit(1)
	}

	mgr.Close()
}
