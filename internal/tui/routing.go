package tui

import (
	"context"
	"time"

	"github.com/charmbracelet/bubbles/key"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/lpwanw/randomshitgo-go/internal/process"
	"github.com/lpwanw/randomshitgo-go/internal/tui/overlays"
)

// startCmd returns a tea.Cmd that calls mgr.Start and returns a ShowToastMsg on error.
func startCmd(mgr *process.Manager, id string) tea.Cmd {
	return func() tea.Msg {
		if err := mgr.Start(id); err != nil {
			return ShowToastMsg{Text: "start: " + err.Error(), Level: overlays.ToastErr}
		}
		return nil
	}
}

// restartCmd returns a tea.Cmd that calls mgr.Restart and returns a ShowToastMsg on error.
func restartCmd(mgr *process.Manager, id string) tea.Cmd {
	return func() tea.Msg {
		if err := mgr.Restart(id); err != nil {
			return ShowToastMsg{Text: "restart: " + err.Error(), Level: overlays.ToastErr}
		}
		return nil
	}
}

// stopCmd returns a tea.Cmd that calls mgr.Stop and returns a ShowToastMsg on error.
func stopCmd(mgr *process.Manager, id string, grace time.Duration) tea.Cmd {
	return func() tea.Msg {
		if err := mgr.Stop(id, grace); err != nil {
			return ShowToastMsg{Text: "stop: " + err.Error(), Level: overlays.ToastErr}
		}
		return nil
	}
}

// stopAllCmd returns a tea.Cmd that calls mgr.StopAll.
func stopAllCmd(mgr *process.Manager, grace time.Duration) tea.Cmd {
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), grace+5*time.Second)
		defer cancel()
		mgr.StopAll(ctx)
		return nil
	}
}

// routeKey dispatches key events to the mode-specific handler.
func routeKey(m Model, msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	// Ctrl+C arms the quit state on first press and fires gracefulQuit only
	// on the second press within quitArmWindow — see handleCtrlC.
	if msg.String() == "ctrl+c" {
		return handleCtrlC(m)
	}
	// Any non-Ctrl+C key disarms a previous Ctrl+C so an unrelated keystroke
	// does not silently leave the user one tap away from quitting.
	if !m.quitArmedAt.IsZero() {
		m.quitArmedAt = time.Time{}
	}

	switch m.mode {
	case ModeNormal:
		return routeNormal(m, msg)
	case ModeHelp:
		return routeHelp(m, msg)
	case ModeGroupPicker:
		return routeGroupPicker(m, msg)
	case ModeBranchPicker:
		return routeBranchPicker(m, msg)
	case ModeFilter:
		return routeFilter(m, msg)
	case ModeCommand:
		return routeCommand(m, msg)
	case ModeAttach:
		if key.Matches(msg, m.keys.Esc) {
			m.mode = ModeNormal
		}
		return m, nil
	}
	return m, nil
}

// routeNormal handles all keys in ModeNormal.
func routeNormal(m Model, msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch {
	// Bare `q` is intentionally NOT bound here. Quit requires `:q` via the
	// command bar — see handleCommandRun — so an accidental keystroke cannot
	// stop every managed process. Ctrl+C (m.keys.Quit) is handled in routeKey
	// as an always-on emergency exit.

	case key.Matches(msg, m.keys.Command):
		m.mode = ModeCommand
		m.overlays.Command.Show()

	case key.Matches(msg, m.keys.Help):
		m.mode = ModeHelp
		m.overlays.Help.Show()

	case key.Matches(msg, m.keys.Filter):
		m.mode = ModeFilter
		uiSnap := m.ui.Snapshot()
		m.overlays.Filter.SetValue(uiSnap.FilterText)
		m.overlays.Filter.Show()

	case key.Matches(msg, m.keys.GroupPicker):
		m.mode = ModeGroupPicker
		m.overlays.Group.Show()

	case key.Matches(msg, m.keys.BranchPicker):
		return handleBranchPickerOpen(m)

	case key.Matches(msg, m.keys.NextMatch):
		return handleJumpMatch(m, true)

	case key.Matches(msg, m.keys.PrevMatch):
		return handleJumpMatch(m, false)

	case key.Matches(msg, m.keys.Up):
		m.sidebar.Up()
		m.syncSelectedToStore()
		m.refreshLogPanel()

	case key.Matches(msg, m.keys.Down):
		m.sidebar.Down()
		m.syncSelectedToStore()
		m.refreshLogPanel()

	case key.Matches(msg, m.keys.Start):
		if id := m.sidebar.Selected(); id != "" {
			return m, startCmd(m.mgr, id)
		}

	case key.Matches(msg, m.keys.Restart):
		if id := m.sidebar.Selected(); id != "" {
			return m, restartCmd(m.mgr, id)
		}

	case key.Matches(msg, m.keys.Stop):
		if id := m.sidebar.Selected(); id != "" {
			grace := time.Duration(m.cfg.Settings.ShutdownGraceMs) * time.Millisecond
			return m, stopCmd(m.mgr, id, grace)
		}

	case key.Matches(msg, m.keys.Attach):
		if id := m.sidebar.Selected(); id != "" {
			return m, func() tea.Msg { return AttachRequestMsg{ID: id} }
		}

	case key.Matches(msg, m.keys.StopAll):
		grace := time.Duration(m.cfg.Settings.ShutdownGraceMs) * time.Millisecond
		return m, stopAllCmd(m.mgr, grace)

	case key.Matches(msg, m.keys.PageUp),
		key.Matches(msg, m.keys.PageDown),
		key.Matches(msg, m.keys.Top),
		key.Matches(msg, m.keys.Bottom):
		cmd := m.logPanel.Update(msg)
		return m, cmd
	}

	// Quick-jump digits (1..9) — handled after the main switch so the explicit
	// bindings above win in case of future collisions.
	for i, k := range m.keys.QuickJumpKeys {
		if key.Matches(msg, k) {
			return handleQuickJump(m, i+1)
		}
	}

	return m, nil
}

// routeHelp handles keys in ModeHelp.
func routeHelp(m Model, msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	if key.Matches(msg, m.keys.Help) || key.Matches(msg, m.keys.Esc) || key.Matches(msg, m.keys.Quit) {
		m.mode = ModeNormal
		m.overlays.Help.Hide()
	}
	return m, nil
}

// routeGroupPicker delegates to the group picker overlay.
func routeGroupPicker(m Model, msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	if key.Matches(msg, m.keys.Esc) {
		m.mode = ModeNormal
		m.overlays.Group.Hide()
		return m, nil
	}
	updated, cmd := m.overlays.Group.Update(msg)
	m.overlays.Group = updated
	if !m.overlays.Group.Visible() {
		m.mode = ModeNormal
	}
	return m, cmd
}

// routeBranchPicker delegates to the branch picker overlay.
func routeBranchPicker(m Model, msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	if key.Matches(msg, m.keys.Esc) {
		m.mode = ModeNormal
		m.overlays.Branch.Hide()
		return m, nil
	}
	updated, cmd := m.overlays.Branch.Update(msg)
	m.overlays.Branch = updated
	if !m.overlays.Branch.Visible() {
		m.mode = ModeNormal
	}
	return m, cmd
}

// routeFilter delegates to the filter overlay.
func routeFilter(m Model, msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	updated, cmd := m.overlays.Filter.Update(tea.KeyMsg(msg))
	m.overlays.Filter = updated
	if !m.overlays.Filter.Visible() {
		m.mode = ModeNormal
	}
	return m, cmd
}

// routeCommand delegates to the command overlay. The mode drops back to Normal
// when the bar closes (Enter/Esc).
func routeCommand(m Model, msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	updated, cmd := m.overlays.Command.Update(tea.KeyMsg(msg))
	m.overlays.Command = updated
	if !m.overlays.Command.Visible() {
		m.mode = ModeNormal
	}
	return m, cmd
}
