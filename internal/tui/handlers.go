package tui

import (
	"context"
	"fmt"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/lpwanw/randomshitgo-go/internal/gitinfo"
	"github.com/lpwanw/randomshitgo-go/internal/netinfo"
	"github.com/lpwanw/randomshitgo-go/internal/tui/attach"
	"github.com/lpwanw/randomshitgo-go/internal/tui/overlays"
	"github.com/lpwanw/randomshitgo-go/internal/tui/panes"
)

// handleMsg is the central message dispatcher for the root Model.
func handleMsg(m Model, msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		return handleResize(m, msg), nil

	case tea.KeyMsg:
		return routeKey(m, msg)

	case tea.MouseMsg:
		cmd := m.logPanel.Update(msg)
		return m, cmd

	case RuntimeChangedMsg:
		return handleRuntimeChanged(m, msg)

	case LogTickMsg:
		m.refreshLogPanel()
		return m, logTick(logFlushInterval(m.cfg))

	case ToastExpiredMsg:
		m.overlays.Toasts.Prune(time.Now())
		return m, toastPruneTick()

	case ShowToastMsg:
		m.overlays.Toasts.Add(msg.Text, msg.Level)
		return m, nil

	case overlays.ShowToastMsg:
		m.overlays.Toasts.Add(msg.Text, msg.Level)
		return m, nil

	case StartGroupMsg:
		return handleStartGroupByName(m, msg.Name)

	case overlays.StartGroupMsg:
		return handleStartGroupByName(m, msg.Name)

	case overlays.CheckoutBranchMsg:
		return handleCheckoutBranch(m, msg.Branch)

	case GitInfoMsg:
		if m.lastGitInfo == nil {
			m.lastGitInfo = make(map[string]gitInfoCache)
		}
		m.lastGitInfo[msg.ID] = gitInfoCache{
			branch: msg.Info.Branch,
			ahead:  msg.Info.Ahead,
			behind: msg.Info.Behind,
			dirty:  msg.Info.Dirty,
		}
		return m, nil

	case PortInfoMsg:
		if m.lastPort == nil {
			m.lastPort = make(map[string]int)
		}
		m.lastPort[msg.ID] = msg.Port
		return m, nil

	case statusRefreshTickMsg:
		return m, handleStatusRefresh(m)

	case overlays.FilterCommitMsg:
		m.mode = ModeNormal
		m.ui.SetFilter(msg.Text)
		uiSnap := m.ui.Snapshot()
		m.logPanel.SetFilter(uiSnap.FilterRegex)
		m.statusBar.FilterText = msg.Text
		// Auto-jump to first match so the user sees it immediately.
		if uiSnap.FilterRegex != nil {
			if ok, _ := m.logPanel.JumpNextMatch(); !ok {
				m.overlays.Toasts.Add("no matches", overlays.ToastWarn)
			}
		}
		return m, nil

	case overlays.FilterCancelMsg:
		m.mode = ModeNormal
		return m, nil

	case overlays.CommandRunMsg:
		m.mode = ModeNormal
		return dispatchCommand(m, msg.Text)

	case overlays.CommandCancelMsg:
		m.mode = ModeNormal
		return m, nil

	case AttachRequestMsg:
		return handleAttachRequest(m, msg)

	case AttachEndedMsg:
		m.mode = ModeNormal
		return m, nil

	case panes.CopiedMsg:
		// Stay in log focus after a yank — user keeps browsing / yanking. Only
		// double-Esc returns control to the sidebar.
		m.logPanel.ClearSelection()
		unit := "lines"
		if msg.Lines == 1 {
			unit = "line"
		}
		m.overlays.Toasts.Add(
			fmt.Sprintf("copied %d %s (%d chars)", msg.Lines, unit, msg.Chars),
			overlays.ToastInfo,
		)
		return m, nil

	case panes.CopyFailedMsg:
		m.overlays.Toasts.Add("copy failed: "+msg.Err, overlays.ToastErr)
		return m, nil

	case stateRefreshMsg:
		m.syncSelectedFromStore()
		return m, nil

	case branchesLoadedMsg:
		m.overlays.Branch.SetBranches(msg.branches)
		m.overlays.Branch.Show()
		return m, nil
	}

	return m, nil
}

// handleResize updates all pane sizes when the terminal is resized.
func handleResize(m Model, msg tea.WindowSizeMsg) Model {
	m.width = msg.Width
	m.height = msg.Height

	sidebarW := sidebarWidth(m.width, m.cfg)
	logW := m.width - sidebarW
	contentH := m.height - statusBarHeight

	m.sidebar.SetSize(sidebarW, contentH)
	m.logPanel.SetSize(logW, contentH)
	m.statusBar.Width = m.width

	if id := m.sidebar.Selected(); id != "" {
		ptyCols := uint16(max(40, logW-4))
		ptyRows := uint16(max(10, contentH-4))
		m.mgr.Resize(id, ptyCols, ptyRows)
	}
	return m
}

// handleRuntimeChanged updates sidebar and re-arms the subscription.
func handleRuntimeChanged(m Model, msg RuntimeChangedMsg) (Model, tea.Cmd) {
	m.lastRuntimeSnap = msg.Snapshot
	m.sidebar.SetRows(msg.Snapshot)
	m.syncSelectedFromStore()
	return m, rearmRuntimeSubscribe(m.runtime)
}

// handleStartGroupByName starts a group and shows a toast.
func handleStartGroupByName(m Model, name string) (Model, tea.Cmd) {
	delay := time.Duration(m.cfg.Settings.GroupStartDelayMs) * time.Millisecond
	mgr := m.mgr
	capturedName := name
	return m, func() tea.Msg {
		if err := mgr.StartGroup(capturedName, delay); err != nil {
			return ShowToastMsg{Text: "group start: " + err.Error(), Level: overlays.ToastErr}
		}
		return ShowToastMsg{Text: "started group: " + capturedName, Level: overlays.ToastInfo}
	}
}

// handleAttachRequest starts the attach flow for the given project.
// It calls program.ReleaseTerminal, runs the attach controller (blocking),
// then calls program.RestoreTerminal and emits AttachEndedMsg.
func handleAttachRequest(m Model, msg AttachRequestMsg) (tea.Model, tea.Cmd) {
	// Verify the process is running before proceeding.
	snap := m.runtime.Snapshot()
	running := false
	for _, r := range snap {
		if r.ID == msg.ID && r.State == "running" {
			running = true
			break
		}
	}
	if !running {
		m.overlays.Toasts.Add("attach: process not running", overlays.ToastErr)
		return m, nil
	}

	m.mode = ModeAttach
	prog := m.prog
	mgr := m.mgr
	id := msg.ID

	return m, func() tea.Msg {
		if prog != nil {
			if err := prog.ReleaseTerminal(); err != nil {
				return AttachEndedMsg{}
			}
		}

		ptmx, err := mgr.Attach(id)
		if err == nil {
			ctrl := attach.NewController(0, 0)
			ctrl.Run(context.Background(), ptmx) //nolint:errcheck
		}

		if prog != nil {
			prog.RestoreTerminal() //nolint:errcheck
		}
		return AttachEndedMsg{}
	}
}

// handleCheckoutBranch runs git checkout for the selected project and shows a toast.
func handleCheckoutBranch(m Model, branch string) (Model, tea.Cmd) {
	id := m.sidebar.Selected()
	if id == "" {
		m.overlays.Toasts.Add("checkout: no project selected", overlays.ToastErr)
		return m, nil
	}

	// Look up project path.
	proj, ok := m.cfg.Projects[id]
	if !ok {
		m.overlays.Toasts.Add(fmt.Sprintf("checkout: unknown project %q", id), overlays.ToastErr)
		return m, nil
	}
	dir := proj.Path

	return m, func() tea.Msg {
		if err := gitinfo.Checkout(dir, branch); err != nil {
			return ShowToastMsg{Text: "checkout failed: " + err.Error(), Level: overlays.ToastErr}
		}
		return ShowToastMsg{Text: fmt.Sprintf("checked out %s", branch), Level: overlays.ToastInfo}
	}
}

// handleStatusRefresh fans out git+port queries for the selected project as tea.Cmds.
func handleStatusRefresh(m Model) tea.Cmd {
	id := m.sidebar.Selected()
	if id == "" {
		return statusRefreshTick()
	}

	proj, ok := m.cfg.Projects[id]
	if !ok {
		return statusRefreshTick()
	}
	dir := proj.Path

	// Find PID for port query.
	pid := 0
	for _, r := range m.lastRuntimeSnap {
		if r.ID == id {
			pid = r.PID
			break
		}
	}

	var cmds []tea.Cmd

	// Git info query.
	capturedID := id
	capturedDir := dir
	cmds = append(cmds, func() tea.Msg {
		info, _ := gitinfo.Current(capturedDir)
		return GitInfoMsg{ID: capturedID, Info: info}
	})

	// Port query (only when process is running with a known PID).
	if pid > 0 {
		capturedPID := pid
		cmds = append(cmds, func() tea.Msg {
			port, err := netinfo.PortForPID(capturedPID)
			if err != nil {
				port = 0
			}
			return PortInfoMsg{ID: capturedID, Port: port}
		})
	}

	cmds = append(cmds, statusRefreshTick())
	return tea.Batch(cmds...)
}

// handleCtrlC implements the double-tap-to-quit pattern. First press arms the
// quit state and surfaces a warn-level toast hint that lives as long as the
// arm window. A second Ctrl+C within the window fires gracefulQuit; after the
// window elapses the next press re-arms instead of quitting.
func handleCtrlC(m Model) (tea.Model, tea.Cmd) {
	if !m.quitArmedAt.IsZero() && time.Since(m.quitArmedAt) <= quitArmWindow {
		return m, gracefulQuit(m)
	}
	m.quitArmedAt = time.Now()
	m.overlays.Toasts.AddWithTTL("Press Ctrl+C again to quit", overlays.ToastWarn, quitArmWindow)
	return m, nil
}

// handleQuickJump moves the sidebar cursor to the given 1-based row index and
// refreshes the selected-project-dependent panes. Out-of-range indices are
// silently ignored so a stray digit keystroke never surprises the user.
func handleQuickJump(m Model, n int) (tea.Model, tea.Cmd) {
	if n < 1 || n-1 >= m.sidebar.Len() {
		return m, nil
	}
	m.sidebar.SetCursor(n - 1)
	m.syncSelectedToStore()
	m.refreshLogPanel()
	return m, nil
}

// handleJumpMatch moves to the next/previous filter match in the log panel.
// Emits soft toasts for "no filter", "no matches", and "wrapped".
func handleJumpMatch(m Model, forward bool) (tea.Model, tea.Cmd) {
	if m.ui.Snapshot().FilterRegex == nil {
		m.overlays.Toasts.Add("no filter — press / to search", overlays.ToastInfo)
		return m, nil
	}
	var ok, wrapped bool
	switch {
	case m.mode == ModeLogFocus && forward:
		ok, wrapped = m.logPanel.JumpNextMatchCursor()
	case m.mode == ModeLogFocus:
		ok, wrapped = m.logPanel.JumpPrevMatchCursor()
	case forward:
		ok, wrapped = m.logPanel.JumpNextMatch()
	default:
		ok, wrapped = m.logPanel.JumpPrevMatch()
	}
	if !ok {
		m.overlays.Toasts.Add("no matches", overlays.ToastWarn)
		return m, nil
	}
	if wrapped {
		m.overlays.Toasts.Add("search wrapped", overlays.ToastInfo)
	}
	return m, nil
}

// handleBranchPickerOpen opens the branch picker for the selected project.
func handleBranchPickerOpen(m Model) (Model, tea.Cmd) {
	id := m.sidebar.Selected()
	if id == "" {
		return m, nil
	}
	proj, ok := m.cfg.Projects[id]
	if !ok {
		return m, nil
	}
	dir := proj.Path

	m.mode = ModeBranchPicker
	return m, func() tea.Msg {
		branches, err := gitinfo.Branches(dir)
		if err != nil || len(branches) == 0 {
			branches = []string{}
		}
		return branchesLoadedMsg{branches: branches}
	}
}

// branchesLoadedMsg is sent when async branch list is ready.
type branchesLoadedMsg struct {
	branches []string
}

// dispatchCommand runs a parsed `:` command. Unknown commands produce a
// non-destructive error toast so typos don't kill processes.
func dispatchCommand(m Model, text string) (tea.Model, tea.Cmd) {
	name := strings.Join(strings.Fields(text), " ")
	switch name {
	case "":
		return m, nil
	case "q", "quit":
		return m, gracefulQuit(m)
	case "set nu", "set number":
		m.logPanel.SetGutter(true)
		return m, nil
	case "set nonu", "set nonumber":
		m.logPanel.SetGutter(false)
		return m, nil
	default:
		m.overlays.Toasts.Add("unknown command: "+name, overlays.ToastErr)
		return m, nil
	}
}

// gracefulQuit stops all processes then emits QuitMsg.
func gracefulQuit(m Model) tea.Cmd {
	grace := time.Duration(m.cfg.Settings.ShutdownGraceMs) * time.Millisecond
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), grace+5*time.Second)
		defer cancel()
		m.mgr.StopAll(ctx)
		return tea.QuitMsg{}
	}
}
