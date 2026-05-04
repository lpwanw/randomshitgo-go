package tui

import (
	"fmt"
	"os"
	"os/exec"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/lpwanw/randomshitgo-go/internal/config"
	"github.com/lpwanw/randomshitgo-go/internal/process"
	"github.com/lpwanw/randomshitgo-go/internal/tui/overlays"
)

// handleEditConfig suspends the TUI, runs the user's $EDITOR on the resolved
// config path, and queues a ConfigEditedMsg with the editor exit status.
func handleEditConfig(m Model) (tea.Model, tea.Cmd) {
	if m.cfgPath == "" {
		m.overlays.Toasts.Add("edit: no config path resolved", overlays.ToastErr)
		return m, nil
	}
	parts := pickEditor()
	if len(parts) == 0 {
		m.overlays.Toasts.Add("edit: set $VISUAL or $EDITOR (e.g. 'export EDITOR=vim')", overlays.ToastErr)
		return m, nil
	}
	args := append(parts[1:], m.cfgPath)
	c := exec.Command(parts[0], args...)
	c.Stdin = os.Stdin
	c.Stdout = os.Stdout
	c.Stderr = os.Stderr
	return m, tea.ExecProcess(c, func(err error) tea.Msg {
		return ConfigEditedMsg{Err: err}
	})
}

// pickEditor resolves the editor command line. Whitespace-split so values like
// `code -w` produce ["code", "-w"]. Falls back to vi when nothing is set.
// Returns nil when no usable editor is found.
func pickEditor() []string {
	for _, k := range []string{"VISUAL", "EDITOR"} {
		if v := strings.TrimSpace(os.Getenv(k)); v != "" {
			return strings.Fields(v)
		}
	}
	if _, err := exec.LookPath("vi"); err == nil {
		return []string{"vi"}
	}
	return nil
}

// handleConfigEdited dispatches after the editor exits: error → toast and
// keep old config; success → reload from disk.
func handleConfigEdited(m Model, msg ConfigEditedMsg) (tea.Model, tea.Cmd) {
	if msg.Err != nil {
		m.overlays.Toasts.Add("edit cancelled: "+msg.Err.Error(), overlays.ToastErr)
		return m, nil
	}
	return reloadConfig(m)
}

// reloadConfig parses cfgPath, applies the diff via Manager.Reload, swaps the
// cached *config.Config, and reseeds the runtime store so newly added projects
// appear in the sidebar. Parse errors keep the old config intact.
func reloadConfig(m Model) (tea.Model, tea.Cmd) {
	if m.cfgPath == "" {
		m.overlays.Toasts.Add("reload: no config path", overlays.ToastErr)
		return m, nil
	}
	cfg, err := config.LoadFromPath(m.cfgPath)
	if err != nil {
		m.overlays.Toasts.Add("config error: "+err.Error(), overlays.ToastErr)
		return m, nil
	}
	res := m.mgr.Reload(cfg)
	m.cfg = cfg

	if len(res.Added) > 0 {
		m.runtime.Seed(res.Added)
	}
	if len(res.Removed) > 0 {
		m.runtime.Delete(res.Removed)
	}

	m.overlays.Toasts.Add(reloadSummary(res), reloadLevel(res))

	// Pure-change reloads (no add/remove) don't notify subscribers, but the
	// sidebar still needs no refresh — definitions changed, not runtime state.
	return m, nil
}

func reloadSummary(r process.ReloadResult) string {
	if len(r.Added) == 0 && len(r.Removed) == 0 && len(r.Changed) == 0 {
		return "config reloaded (no changes)"
	}
	parts := []string{}
	if n := len(r.Added); n > 0 {
		parts = append(parts, fmt.Sprintf("+%d added", n))
	}
	if n := len(r.Removed); n > 0 {
		parts = append(parts, fmt.Sprintf("-%d removed", n))
	}
	if n := len(r.Changed); n > 0 {
		parts = append(parts, fmt.Sprintf("~%d changed (restart to apply)", n))
	}
	return "reloaded: " + strings.Join(parts, ", ")
}

func reloadLevel(r process.ReloadResult) int {
	if len(r.Changed) > 0 {
		return overlays.ToastWarn
	}
	return overlays.ToastInfo
}
