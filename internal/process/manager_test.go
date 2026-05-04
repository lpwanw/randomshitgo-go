//go:build !windows

package process

import (
	"context"
	"runtime"
	"testing"
	"time"

	"github.com/lpwanw/randomshitgo-go/internal/config"
)

func makeTestConfig(t *testing.T) *config.Config {
	t.Helper()
	return &config.Config{
		Projects: map[string]config.Project{
			"alpha": {
				Path:    t.TempDir(),
				Cmd:     "echo alpha",
				Restart: config.RestartNever,
			},
			"beta": {
				Path:    t.TempDir(),
				Cmd:     "echo beta",
				Restart: config.RestartNever,
			},
		},
		Groups: map[string][]string{
			"all": {"alpha", "beta"},
		},
		Settings: makeTestSettings(),
	}
}

func TestManager_StartAndStop(t *testing.T) {
	cfg := makeTestConfig(t)
	m := New(cfg, &noopRegistry{})

	if err := m.Start("alpha"); err != nil {
		t.Fatalf("Start alpha: %v", err)
	}

	// Drain events briefly then stop.
	time.Sleep(200 * time.Millisecond)
	if err := m.Stop("alpha", 0); err != nil {
		t.Logf("Stop alpha: %v (non-fatal)", err)
	}
}

func TestManager_StartGroup(t *testing.T) {
	cfg := makeTestConfig(t)
	m := New(cfg, &noopRegistry{})

	if err := m.StartGroup("all", 10*time.Millisecond); err != nil {
		t.Fatalf("StartGroup: %v", err)
	}
	// Wait for processes to complete.
	time.Sleep(300 * time.Millisecond)
}

func TestManager_StopAll(t *testing.T) {
	cfg := makeTestConfig(t)
	cfg.Projects["slow"] = config.Project{
		Path:    t.TempDir(),
		Cmd:     "sleep 10",
		Restart: config.RestartNever,
	}
	m := New(cfg, &noopRegistry{})

	_ = m.Start("slow")
	time.Sleep(50 * time.Millisecond)

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	m.StopAll(ctx)
}

func TestManager_Reload_DiffsAndStops(t *testing.T) {
	cfg := makeTestConfig(t)
	m := New(cfg, &noopRegistry{})
	t.Cleanup(m.Close)

	// Start alpha so removal forces a stop.
	if err := m.Start("alpha"); err != nil {
		t.Fatalf("Start alpha: %v", err)
	}
	// Drain events to keep the channel from filling.
	go func() {
		for range m.Events() {
		}
	}()
	time.Sleep(100 * time.Millisecond)

	newCfg := &config.Config{
		Projects: map[string]config.Project{
			// alpha removed
			"beta": {
				Path:    cfg.Projects["beta"].Path,
				Cmd:     "echo beta-new", // changed
				Restart: config.RestartNever,
			},
			"gamma": { // added
				Path:    t.TempDir(),
				Cmd:     "echo gamma",
				Restart: config.RestartNever,
			},
		},
		Settings: cfg.Settings,
	}

	res := m.Reload(newCfg)

	want := func(slice []string, id string) bool {
		for _, s := range slice {
			if s == id {
				return true
			}
		}
		return false
	}
	if !want(res.Removed, "alpha") {
		t.Fatalf("Removed=%v want alpha", res.Removed)
	}
	if !want(res.Added, "gamma") {
		t.Fatalf("Added=%v want gamma", res.Added)
	}
	if !want(res.Changed, "beta") {
		t.Fatalf("Changed=%v want beta", res.Changed)
	}
	if !want(res.Stopped, "alpha") {
		t.Fatalf("Stopped=%v want alpha", res.Stopped)
	}

	// Removed running children must be dropped from the manager.
	m.mu.RLock()
	_, stillThere := m.children["alpha"]
	m.mu.RUnlock()
	if stillThere {
		t.Fatalf("children[alpha] should be dropped after reload")
	}

	// cfg pointer swapped.
	m.mu.RLock()
	if m.cfg != newCfg {
		t.Fatalf("cfg pointer not swapped")
	}
	m.mu.RUnlock()
}

func TestManager_Reload_NoChanges(t *testing.T) {
	cfg := makeTestConfig(t)
	m := New(cfg, &noopRegistry{})
	t.Cleanup(m.Close)

	// Reload same cfg → empty diff.
	same := *cfg
	res := m.Reload(&same)
	if len(res.Added)+len(res.Removed)+len(res.Changed)+len(res.Stopped) != 0 {
		t.Fatalf("expected empty diff, got %+v", res)
	}
}

func TestManager_Close_NoGoroutineLeak(t *testing.T) {
	cfg := makeTestConfig(t)
	m := New(cfg, &noopRegistry{})

	_ = m.Start("alpha")
	_ = m.Start("beta")
	time.Sleep(100 * time.Millisecond)

	before := runtime.NumGoroutine()
	m.Close()
	time.Sleep(200 * time.Millisecond)
	after := runtime.NumGoroutine()

	// Allow a small margin for background GC / test goroutines.
	if after > before+3 {
		t.Fatalf("goroutine leak: before=%d after=%d", before, after)
	}
}
