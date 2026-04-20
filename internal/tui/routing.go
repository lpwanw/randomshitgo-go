package tui

import (
	"context"
	"time"

	"github.com/charmbracelet/bubbles/key"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/taynguyen/procs/internal/tui/overlays"
)

// routeKey dispatches key events to the mode-specific handler.
func routeKey(m Model, msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	// Ctrl+C always quits.
	if msg.String() == "ctrl+c" {
		return m, gracefulQuit(m)
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
	case key.Matches(msg, m.keys.Quit):
		return m, gracefulQuit(m)

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
			go func() {
				if err := m.mgr.Start(id); err != nil {
					m.overlays.Toasts.Add("start: "+err.Error(), overlays.ToastErr)
				}
			}()
		}

	case key.Matches(msg, m.keys.Restart):
		if id := m.sidebar.Selected(); id != "" {
			go func() {
				if err := m.mgr.Restart(id); err != nil {
					m.overlays.Toasts.Add("restart: "+err.Error(), overlays.ToastErr)
				}
			}()
		}

	case key.Matches(msg, m.keys.Stop):
		if id := m.sidebar.Selected(); id != "" {
			grace := time.Duration(m.cfg.Settings.ShutdownGraceMs) * time.Millisecond
			go func() {
				if err := m.mgr.Stop(id, grace); err != nil {
					m.overlays.Toasts.Add("stop: "+err.Error(), overlays.ToastErr)
				}
			}()
		}

	case key.Matches(msg, m.keys.Attach):
		if id := m.sidebar.Selected(); id != "" {
			return m, func() tea.Msg { return AttachRequestMsg{ID: id} }
		}

	case key.Matches(msg, m.keys.StopAll):
		go func() {
			grace := time.Duration(m.cfg.Settings.ShutdownGraceMs) * time.Millisecond
			ctx, cancel := context.WithTimeout(context.Background(), grace+5*time.Second)
			defer cancel()
			m.mgr.StopAll(ctx)
		}()

	case key.Matches(msg, m.keys.PageUp),
		key.Matches(msg, m.keys.PageDown),
		key.Matches(msg, m.keys.Top),
		key.Matches(msg, m.keys.Bottom):
		cmd := m.logPanel.Update(msg)
		return m, cmd
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
