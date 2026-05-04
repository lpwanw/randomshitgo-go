package tui

import (
	"context"
	"fmt"
	"os"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/lpwanw/randomshitgo-go/internal/config"
	"github.com/lpwanw/randomshitgo-go/internal/gitinfo"
	logpkg "github.com/lpwanw/randomshitgo-go/internal/log"
	"github.com/lpwanw/randomshitgo-go/internal/netinfo"
	"github.com/lpwanw/randomshitgo-go/internal/procstats"
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
		// In embedded-attach mode the log panel is replaced by the vt
		// render; mouse goes nowhere (PTY mouse forwarding TBD).
		if m.mode == ModeEmbeddedAttach || m.mode == ModeAttach {
			return m, nil
		}
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

	case ProcStatsMsg:
		if msg.OK {
			m.lastProcStats[msg.ID] = procstats.Stats{CPU: msg.CPU, RSS: msg.RSS}
		} else {
			delete(m.lastProcStats, msg.ID)
		}
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

	case EmbeddedAttachRequestMsg:
		return handleEmbeddedAttachRequest(m, msg)

	case attach.EmbeddedAttachStartedMsg:
		// Start the refresh + write-error loops once the session is wired.
		if m.attach == nil {
			return m, nil
		}
		return m, tea.Batch(m.attach.RefreshCmd(), m.attach.WatchErrCmd())

	case attach.VTRefreshMsg:
		if m.attach == nil {
			return m, nil
		}
		// Re-arm the refresh wait — single-shot per ping.
		return m, m.attach.RefreshCmd()

	case attach.EmbeddedAttachEndedMsg:
		return handleEmbeddedAttachEnd(m, msg)

	case attach.DetachFlushMsg:
		return handleDetachFlush(m)

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

	case ConfigEditedMsg:
		return handleConfigEdited(m, msg)
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
	m.logPanel.SetOrigin(sidebarW, 0)
	m.statusBar.Width = m.width

	if id := m.sidebar.Selected(); id != "" {
		ptyCols := uint16(max(40, logW-4))
		ptyRows := uint16(max(10, contentH-4))
		m.mgr.Resize(id, ptyCols, ptyRows)
	}
	if m.mode == ModeEmbeddedAttach && m.attach != nil {
		// The embedded grid fills the log pane exactly — no internal
		// border — so the emulator dims must match logW × contentH.
		cols := uint16(max(20, logW))
		rows := uint16(max(5, contentH))
		m.mgr.Resize(m.attach.ProjectID(), cols, rows)
		m.attach.Resize(int(cols), int(rows))
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

// handleEmbeddedAttachRequest brings up the in-pane vt session for the
// given project. It verifies the child is running, grabs the ptmx,
// resizes both PTY and emulator to match the content pane, subscribes
// the emulator to the PTY tee, and stashes the Session on the model.
func handleEmbeddedAttachRequest(m Model, msg EmbeddedAttachRequestMsg) (tea.Model, tea.Cmd) {
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
	if m.attach != nil {
		m.overlays.Toasts.Add("attach: already attached — detach first", overlays.ToastWarn)
		return m, nil
	}

	ptmx, err := m.mgr.Attach(msg.ID)
	if err != nil {
		m.overlays.Toasts.Add("attach: "+err.Error(), overlays.ToastErr)
		return m, nil
	}

	sidebarW := sidebarWidth(m.width, m.cfg)
	logW := m.width - sidebarW
	contentH := m.height - statusBarHeight
	cols := max(20, logW)
	rows := max(5, contentH)

	// Resize the PTY before subscribing so the first frame the emulator
	// sees has the right dims and apps redraw cleanly into the pane.
	m.mgr.Resize(msg.ID, uint16(cols), uint16(rows))

	sess, err := attach.NewSession(msg.ID, ptmx, cols, rows, m.mgr.Subscribe)
	if err != nil {
		m.overlays.Toasts.Add("attach: "+err.Error(), overlays.ToastErr)
		return m, nil
	}

	m.attach = sess
	m.mode = ModeEmbeddedAttach
	id := msg.ID
	return m, func() tea.Msg { return attach.EmbeddedAttachStartedMsg{ID: id} }
}

// handleEmbeddedAttachEnd tears the session down (Close unsubscribes and
// shuts the emulator), surfaces a toast, and returns to ModeNormal.
func handleEmbeddedAttachEnd(m Model, msg attach.EmbeddedAttachEndedMsg) (tea.Model, tea.Cmd) {
	if m.attach != nil {
		m.attach.Close()
		m.attach = nil
	}
	m.mode = ModeNormal
	reason := strings.TrimSpace(msg.Reason)
	if reason == "" {
		reason = "detached"
	}
	level := overlays.ToastInfo
	if strings.Contains(reason, "fail") || strings.Contains(reason, "error") {
		level = overlays.ToastErr
	}
	m.overlays.Toasts.Add(reason, level)
	m.refreshLogPanel()
	return m, nil
}

// handleDetachFlush is the timer-driven fallback for a lone Esc: if
// the detector arm window has elapsed, the swallowed byte is written to
// the PTY so the child sees the keypress.
func handleDetachFlush(m Model) (tea.Model, tea.Cmd) {
	if m.attach == nil {
		return m, nil
	}
	if bytes := m.attach.Detector().FlushIfExpired(); len(bytes) > 0 {
		if err := m.attach.SendBytes(bytes); err != nil {
			id := m.attach.ProjectID()
			return m, func() tea.Msg {
				return attach.EmbeddedAttachEndedMsg{Reason: "child write failed: " + err.Error() + " (" + id + ")"}
			}
		}
	}
	return m, nil
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

	// Proc-stats query. Invalidate the gopsutil handle cache if the PID
	// changed since the last tick (child restarted) so the next CPU%
	// baseline is fresh rather than spiking against the dead process.
	if pid > 0 && m.stats != nil {
		if prev, ok := m.lastRuntimePID[id]; ok && prev != pid {
			m.stats.Forget(prev)
			delete(m.lastProcStats, id)
		}
		m.lastRuntimePID[id] = pid
		sampler := m.stats
		capturedPID := pid
		cmds = append(cmds, func() tea.Msg {
			st, err := sampler.Sample(capturedPID)
			if err != nil {
				return ProcStatsMsg{ID: capturedID}
			}
			return ProcStatsMsg{ID: capturedID, CPU: st.CPU, RSS: st.RSS, OK: true}
		})
	} else if pid == 0 {
		// Process not running — drop stale stats so the status bar clears.
		delete(m.lastProcStats, id)
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
	case "set severity", "set sev":
		m.logPanel.SetSeverity(true)
		return m, nil
	case "set nosev", "set noseverity":
		m.logPanel.SetSeverity(false)
		return m, nil
	case "set json":
		m.logPanel.SetJSONPretty(true)
		return m, nil
	case "set nojson":
		m.logPanel.SetJSONPretty(false)
		return m, nil
	case "set wrap":
		m.logPanel.SetWrap(true)
		return m, nil
	case "set nowrap":
		m.logPanel.SetWrap(false)
		return m, nil
	case "set sql":
		m.logPanel.SetSQLPretty(true)
		return m, nil
	case "set nosql":
		m.logPanel.SetSQLPretty(false)
		return m, nil
	case "clear", "c":
		return handleClearBuffer(m)
	case "fetch":
		return handleGitFetch(m)
	case "pull":
		return handleGitPull(m)
	case "edit", "e":
		return handleEditConfig(m)
	case "reload":
		return reloadConfig(m)
	}

	// Commands with arguments — split on first space.
	if head, rest, ok := strings.Cut(name, " "); ok {
		switch head {
		case "w", "write":
			return handleWriteBuffer(m, rest)
		}
	}

	m.overlays.Toasts.Add("unknown command: "+name, overlays.ToastErr)
	return m, nil
}

// handleClearBuffer empties the ring buffer for the selected project and
// refreshes the log panel. No-op when nothing is selected.
func handleClearBuffer(m Model) (tea.Model, tea.Cmd) {
	id := m.sidebar.Selected()
	if id == "" {
		m.overlays.Toasts.Add("clear: no project selected", overlays.ToastErr)
		return m, nil
	}
	m.reg.ClearBuffer(id)
	m.refreshLogPanel()
	m.overlays.Toasts.Add("cleared log buffer: "+id, overlays.ToastInfo)
	return m, nil
}

// handleWriteBuffer dumps the current log panel content to disk. Path is
// run through the same tilde / env-var expansion as the config layer.
func handleWriteBuffer(m Model, rawPath string) (tea.Model, tea.Cmd) {
	rawPath = strings.TrimSpace(rawPath)
	if rawPath == "" {
		m.overlays.Toasts.Add("w: missing path", overlays.ToastErr)
		return m, nil
	}
	expanded, err := config.ExpandPath(rawPath)
	if err != nil {
		m.overlays.Toasts.Add("w: "+err.Error(), overlays.ToastErr)
		return m, nil
	}
	lines := m.logPanel.VisibleLines()
	// Strip ANSI from the dump — clipboards/editors want plain text.
	stripped := make([]string, len(lines))
	for i, line := range lines {
		stripped[i] = logpkg.StripANSI(line)
	}
	body := strings.Join(stripped, "\n")
	if len(stripped) > 0 {
		body += "\n"
	}
	if err := os.WriteFile(expanded, []byte(body), 0o644); err != nil {
		m.overlays.Toasts.Add("w: "+err.Error(), overlays.ToastErr)
		return m, nil
	}
	m.overlays.Toasts.Add(fmt.Sprintf("wrote %d lines → %s", len(stripped), expanded), overlays.ToastInfo)
	return m, nil
}

// handleGitFetch runs `git fetch --prune` against the selected project's
// repo asynchronously and surfaces the result as a toast.
func handleGitFetch(m Model) (tea.Model, tea.Cmd) {
	id := m.sidebar.Selected()
	if id == "" {
		m.overlays.Toasts.Add("fetch: no project selected", overlays.ToastErr)
		return m, nil
	}
	proj, ok := m.cfg.Projects[id]
	if !ok {
		m.overlays.Toasts.Add(fmt.Sprintf("fetch: unknown project %q", id), overlays.ToastErr)
		return m, nil
	}
	dir := proj.Path
	return m, func() tea.Msg {
		out, err := gitinfo.Fetch(dir)
		if err != nil {
			return ShowToastMsg{Text: "fetch: " + firstLine(err.Error()), Level: overlays.ToastErr}
		}
		text := firstLine(out)
		if text == "" {
			text = "fetched"
		}
		return ShowToastMsg{Text: text, Level: overlays.ToastInfo}
	}
}

// handleGitPull runs `git pull --ff-only` against the selected project's
// repo asynchronously and surfaces the result as a toast. Never merges.
func handleGitPull(m Model) (tea.Model, tea.Cmd) {
	id := m.sidebar.Selected()
	if id == "" {
		m.overlays.Toasts.Add("pull: no project selected", overlays.ToastErr)
		return m, nil
	}
	proj, ok := m.cfg.Projects[id]
	if !ok {
		m.overlays.Toasts.Add(fmt.Sprintf("pull: unknown project %q", id), overlays.ToastErr)
		return m, nil
	}
	dir := proj.Path
	return m, func() tea.Msg {
		out, err := gitinfo.Pull(dir)
		if err != nil {
			return ShowToastMsg{Text: "pull: " + firstLine(err.Error()), Level: overlays.ToastErr}
		}
		text := firstLine(out)
		if text == "" {
			text = "pulled"
		}
		return ShowToastMsg{Text: text, Level: overlays.ToastInfo}
	}
}

// firstLine returns the first non-empty line of s, trimmed. Used to keep
// toasts terse when git stdout/stderr carries a multi-line report.
func firstLine(s string) string {
	for _, line := range strings.Split(s, "\n") {
		line = strings.TrimSpace(line)
		if line != "" {
			return line
		}
	}
	return ""
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
