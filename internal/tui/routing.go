package tui

import (
	"context"
	"time"

	"github.com/charmbracelet/bubbles/key"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/lpwanw/randomshitgo-go/internal/process"
	"github.com/lpwanw/randomshitgo-go/internal/tui/attach"
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
	// In embedded-attach mode Ctrl+C must reach the child program (so
	// `^C` interrupts e.g. a running rails console) instead of arming
	// our double-tap-to-quit. The detach handshake (Ctrl-] Ctrl-]) is
	// the only way out.
	if msg.String() == "ctrl+c" && m.mode != ModeEmbeddedAttach {
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
	case ModeEmbeddedAttach:
		return routeEmbeddedAttach(m, msg)
	case ModeLogFocus:
		return routeLogFocus(m, msg)
	}
	return m, nil
}

// routeEmbeddedAttach forwards every keystroke into the child PTY,
// intercepting only the Ctrl-] Ctrl-] detach handshake. The detector is
// fed first; if it consumes the key we're done. If it produces flush
// bytes (the saved Ctrl-] from a non-handshake follow-up) those go out
// to the PTY before the current key. PTY write errors trigger an
// auto-detach via EmbeddedAttachEndedMsg.
func routeEmbeddedAttach(m Model, msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	if m.attach == nil {
		// Defensive — should never happen, but recover gracefully.
		m.mode = ModeNormal
		return m, nil
	}
	// Bracketed paste from the host terminal arrives as a single KeyMsg
	// with Paste=true; forward to the child via the emulator's Paste
	// path so the child sees mode-2004 wrappers when it asked for them.
	// Skip the detach detector — paste content can never be Ctrl-].
	if msg.Paste {
		text := string(msg.Runes)
		if err := m.attach.SendPaste(text); err != nil {
			id := m.attach.ProjectID()
			return m, func() tea.Msg {
				return attach.EmbeddedAttachEndedMsg{Reason: "paste failed: " + err.Error() + " (" + id + ")"}
			}
		}
		return m, nil
	}
	det := m.attach.Detector()
	consumed, detached, flush := det.Feed(msg)
	id := m.attach.ProjectID()

	if len(flush) > 0 {
		if err := m.attach.SendBytes(flush); err != nil {
			return m, func() tea.Msg {
				return attach.EmbeddedAttachEndedMsg{Reason: "child write failed: " + err.Error() + " (" + id + ")"}
			}
		}
	}

	if detached {
		return m, func() tea.Msg {
			return attach.EmbeddedAttachEndedMsg{Reason: "detached from " + id}
		}
	}

	if consumed {
		// Schedule a flush tick so a lone Ctrl-] eventually reaches the
		// child if no follow-up key arrives.
		return m, attach.DetachFlushTickCmd()
	}

	if k, ok := attach.MsgToKey(msg); ok {
		m.attach.SendKey(k)
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

	case key.Matches(msg, m.keys.GitFetch):
		model, cmd := handleGitFetch(m)
		return model, cmd

	case key.Matches(msg, m.keys.GitPull):
		model, cmd := handleGitPull(m)
		return model, cmd

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
		// Repurposed in phase 04: 'a' enters the embedded vt session
		// in the content pane. The legacy fullscreen attach flow
		// (AttachRequestMsg → handleAttachRequest) is still wired but
		// no longer reachable from a key binding.
		if id := m.sidebar.Selected(); id != "" {
			return m, func() tea.Msg { return EmbeddedAttachRequestMsg{ID: id} }
		}

	case key.Matches(msg, m.keys.ClearLog):
		return handleClearBuffer(m)

	case key.Matches(msg, m.keys.StopAll):
		grace := time.Duration(m.cfg.Settings.ShutdownGraceMs) * time.Millisecond
		return m, stopAllCmd(m.mgr, grace)

	case key.Matches(msg, m.keys.PageUp),
		key.Matches(msg, m.keys.PageDown),
		key.Matches(msg, m.keys.Top),
		key.Matches(msg, m.keys.Bottom):
		cmd := m.logPanel.Update(msg)
		return m, cmd

	case key.Matches(msg, m.keys.CopyEnter):
		m.mode = ModeLogFocus
		m.logPanel.SetCopyMode(true)
		return m, nil
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

// routeBranchPicker delegates to the branch picker overlay. The overlay owns
// Esc so "first Esc clears filter, second Esc closes" works — intercepting it
// here would always collapse the picker on the first press.
func routeBranchPicker(m Model, msg tea.KeyMsg) (tea.Model, tea.Cmd) {
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

// routeLogFocus handles keys while the log panel owns the keyboard. Vim
// motions, visual select, and yank all happen here. Exit back to sidebar /
// process-switching requires a **double-Esc** within quitArmWindow —
// mirrors the double-Ctrl+C quit pattern. Any non-Esc key disarms.
//
// First-Esc semantics:
//   - If a selection is active → clear it, stay focused, do NOT arm.
//   - Else → arm + warn toast "Press Esc again to exit log focus".
//
// Second Esc inside the window returns to ModeNormal.
func routeLogFocus(m Model, msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	if key.Matches(msg, m.keys.Esc) {
		// Pending operator / count / text-object state takes precedence: Esc
		// there only clears the command buffer, same as vim.
		if m.logPanel.ClearPending() {
			m.logEscArmedAt = time.Time{}
			return m, nil
		}
		if m.logPanel.HasSelection() {
			m.logPanel.ClearSelection()
			m.logEscArmedAt = time.Time{}
			return m, nil
		}
		if !m.logEscArmedAt.IsZero() && time.Since(m.logEscArmedAt) <= quitArmWindow {
			m.mode = ModeNormal
			m.logPanel.SetCopyMode(false)
			m.logEscArmedAt = time.Time{}
			return m, nil
		}
		m.logEscArmedAt = time.Now()
		m.overlays.Toasts.AddWithTTL("Press Esc again to exit log focus", overlays.ToastWarn, quitArmWindow)
		return m, nil
	}

	// Space toggles follow-pause without touching the vim state machine.
	if s := msg.String(); s == " " || s == "space" {
		m.logEscArmedAt = time.Time{}
		m.logPanel.TogglePaused()
		return m, nil
	}

	// Filter-match jumps stay wired so `/ n N` works inside log focus too.
	if key.Matches(msg, m.keys.NextMatch) {
		m.logEscArmedAt = time.Time{}
		return handleJumpMatch(m, true)
	}
	if key.Matches(msg, m.keys.PrevMatch) {
		m.logEscArmedAt = time.Time{}
		return handleJumpMatch(m, false)
	}

	// Any other key disarms before routing so a motion never leaves the user
	// one tap away from exiting.
	m.logEscArmedAt = time.Time{}
	cmd := m.logPanel.HandleCopyKey(msg)
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
