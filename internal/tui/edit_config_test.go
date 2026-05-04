package tui

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/lpwanw/randomshitgo-go/internal/config"
	"github.com/lpwanw/randomshitgo-go/internal/process"
	"github.com/lpwanw/randomshitgo-go/internal/state"
	"github.com/lpwanw/randomshitgo-go/internal/tui/overlays"
)

const baseYAML = `projects:
  api:
    path: /tmp
    cmd: echo api
  web:
    path: /tmp
    cmd: echo web
`

func newReloadModel(t *testing.T, body string) (Model, string) {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, "config.yml")
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatalf("seed config: %v", err)
	}
	cfg, err := config.LoadFromPath(path)
	if err != nil {
		t.Fatalf("LoadFromPath: %v", err)
	}
	rt := state.NewRuntimeStore()
	ui := state.NewUIStore()
	reg := state.NewRegistry(cfg.Settings)
	mgr := process.New(cfg, reg)
	t.Cleanup(mgr.Close)
	return New(cfg, mgr, rt, ui, reg, path), path
}

func TestHandleConfigEdited_EditorError(t *testing.T) {
	m, _ := newReloadModel(t, baseYAML)
	beforeProjects := len(m.cfg.Projects)
	out, _ := handleConfigEdited(m, ConfigEditedMsg{Err: errors.New("rc=130")})
	got := out.(Model)
	if got.cfg == nil || len(got.cfg.Projects) != beforeProjects {
		t.Fatalf("cfg should not change on editor error")
	}
	last, ok := got.overlays.Toasts.Last()
	if !ok || !strings.Contains(last.Text, "edit cancelled") || last.Level != overlays.ToastErr {
		t.Fatalf("expected edit-cancelled toast, got %+v ok=%v", last, ok)
	}
}

func TestHandleConfigEdited_AddsProject(t *testing.T) {
	m, path := newReloadModel(t, baseYAML)
	updated := baseYAML + `  worker:
    path: /tmp
    cmd: echo worker
`
	if err := os.WriteFile(path, []byte(updated), 0o644); err != nil {
		t.Fatalf("rewrite config: %v", err)
	}
	out, _ := handleConfigEdited(m, ConfigEditedMsg{})
	got := out.(Model)
	if _, ok := got.cfg.Projects["worker"]; !ok {
		t.Fatalf("worker not present in reloaded cfg: %v", got.cfg.Projects)
	}
	last, ok := got.overlays.Toasts.Last()
	if !ok || !strings.Contains(last.Text, "+1 added") {
		t.Fatalf("expected +1 added toast, got %+v ok=%v", last, ok)
	}
	// Runtime store should expose the new id so the sidebar picks it up.
	if _, ok := got.runtime.Get("worker"); !ok {
		t.Fatalf("runtime store missing worker after reload")
	}
}

func TestReloadConfig_ParseError(t *testing.T) {
	m, path := newReloadModel(t, baseYAML)
	if err := os.WriteFile(path, []byte("projects: [::not yaml::"), 0o644); err != nil {
		t.Fatalf("rewrite: %v", err)
	}
	out, _ := reloadConfig(m)
	got := out.(Model)
	last, ok := got.overlays.Toasts.Last()
	if !ok || !strings.Contains(last.Text, "config error") || last.Level != overlays.ToastErr {
		t.Fatalf("expected config error toast, got %+v ok=%v", last, ok)
	}
	// Old config retained.
	if _, ok := got.cfg.Projects["api"]; !ok {
		t.Fatalf("old cfg should be retained on parse error")
	}
}

func TestReloadConfig_RemovesProject(t *testing.T) {
	m, path := newReloadModel(t, baseYAML)
	// Drop "web".
	stripped := `projects:
  api:
    path: /tmp
    cmd: echo api
`
	if err := os.WriteFile(path, []byte(stripped), 0o644); err != nil {
		t.Fatalf("rewrite: %v", err)
	}
	// Seed runtime so removal has something to delete.
	m.runtime.Seed([]string{"api", "web"})
	out, _ := reloadConfig(m)
	got := out.(Model)
	if _, ok := got.cfg.Projects["web"]; ok {
		t.Fatalf("web should be removed from cfg")
	}
	if _, ok := got.runtime.Get("web"); ok {
		t.Fatalf("web should be removed from runtime store")
	}
	last, ok := got.overlays.Toasts.Last()
	if !ok || !strings.Contains(last.Text, "-1 removed") {
		t.Fatalf("expected -1 removed toast, got %+v ok=%v", last, ok)
	}
}

func TestPickEditor_PrefersVisualOverEditor(t *testing.T) {
	t.Setenv("VISUAL", "nano")
	t.Setenv("EDITOR", "vim")
	got := pickEditor()
	if len(got) == 0 || got[0] != "nano" {
		t.Fatalf("expected nano, got %v", got)
	}
}

func TestPickEditor_SplitsArgs(t *testing.T) {
	t.Setenv("VISUAL", "")
	t.Setenv("EDITOR", "code -w")
	got := pickEditor()
	if len(got) != 2 || got[0] != "code" || got[1] != "-w" {
		t.Fatalf("expected [code -w], got %v", got)
	}
}
