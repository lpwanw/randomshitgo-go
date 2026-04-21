package tui

import (
	"fmt"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/lpwanw/randomshitgo-go/internal/config"
	"github.com/lpwanw/randomshitgo-go/internal/log"
	"github.com/lpwanw/randomshitgo-go/internal/process"
	"github.com/lpwanw/randomshitgo-go/internal/state"
	"github.com/lpwanw/randomshitgo-go/internal/tui/overlays"
	"github.com/lpwanw/randomshitgo-go/internal/tui/panes"
)

const (
	sidebarMinWidth = 16
	sidebarMaxWidth = 36
	statusBarHeight = 1
	// quitArmWindow is how long the first Ctrl+C stays "armed" waiting for a
	// second press. After the window elapses the next Ctrl+C re-arms instead
	// of quitting, matching Claude CLI's double-tap-to-exit pattern.
	quitArmWindow = 2 * time.Second
)

// Model is the root Bubble Tea model for the procs TUI.
type Model struct {
	cfg     *config.Config
	mgr     *process.Manager
	runtime *state.RuntimeStore
	ui      *state.UIStore
	reg     *state.Registry
	prog    *tea.Program // set via SetProgram after NewProgram

	keys   KeyMap
	mode   Mode
	width  int
	height int

	sidebar   panes.Sidebar
	logPanel  panes.LogPanel
	statusBar panes.StatusBar
	overlays  overlays.Set

	// snapshot caches
	lastRuntimeSnap []state.ProjectRuntime
	lastLogGen      int64

	// status-bar live data (keyed by project ID, refreshed on 2s tick)
	lastGitInfo map[string]gitInfoCache
	lastPort    map[string]int

	// quitArmedAt is the timestamp of the most recent Ctrl+C that did NOT
	// quit. Zero value = disarmed. A second Ctrl+C within quitArmWindow
	// triggers graceful quit; any other routed key resets it.
	quitArmedAt time.Time
}

// gitInfoCache stores git info for a project.
type gitInfoCache struct {
	branch string
	ahead  int
	behind int
	dirty  bool
}

// New constructs a root Model wiring all sub-models.
func New(cfg *config.Config, mgr *process.Manager, runtime *state.RuntimeStore, ui *state.UIStore, reg *state.Registry) Model {
	groups := make(map[string][]string)
	if cfg.Groups != nil {
		for k, v := range cfg.Groups {
			groups[k] = v
		}
	}
	return Model{
		cfg:         cfg,
		mgr:         mgr,
		runtime:     runtime,
		ui:          ui,
		reg:         reg,
		keys:        DefaultKeyMap(),
		mode:        ModeNormal,
		logPanel:    panes.NewLogPanel(80, 24),
		overlays:    overlays.NewSet(groups, nil),
		lastGitInfo: make(map[string]gitInfoCache),
		lastPort:    make(map[string]int),
	}
}

// SetProgram stores the *tea.Program reference so attach mode can release the terminal.
func (m *Model) SetProgram(p *tea.Program) { m.prog = p }

// Init is called by Bubble Tea to get the initial Cmd.
func (m Model) Init() tea.Cmd {
	flushInterval := logFlushInterval(m.cfg)
	rt := m.runtime
	initialSnap := func() tea.Msg {
		return RuntimeChangedMsg{Snapshot: rt.Snapshot()}
	}
	return tea.Batch(
		tea.EnterAltScreen,
		initialSnap,
		subscribeRuntime(m.runtime),
		logTick(flushInterval),
		toastPruneTick(),
		statusRefreshTick(),
	)
}

// Update is the Bubble Tea update handler — routes messages to sub-handlers.
func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	return handleMsg(m, msg)
}

// View renders the full TUI layout with optional overlay.
func (m Model) View() string {
	if m.width == 0 || m.height == 0 {
		return "loading…"
	}
	if m.width < 60 || m.height < 10 {
		return lipgloss.NewStyle().Foreground(lipgloss.Color("220")).
			Render(fmt.Sprintf("Terminal too small: %d×%d (need ≥60×10)", m.width, m.height))
	}

	sidebarW := sidebarWidth(m.width, m.cfg)
	logW := m.width - sidebarW
	contentH := m.height - statusBarHeight
	filterVisible := m.overlays.Filter.Visible()
	commandVisible := m.overlays.Command.Visible()
	if filterVisible || commandVisible {
		contentH -= 1 // reserve one row for the search / command bar
	}

	m.sidebar.SetSize(sidebarW, contentH)
	m.logPanel.SetSize(logW, contentH)
	m.statusBar.Width = m.width
	m.statusBar.Mode = m.mode.String()

	uiSnap := m.ui.Snapshot()
	m.statusBar.Selected = uiSnap.SelectedID
	m.statusBar.Total = len(m.lastRuntimeSnap)
	m.statusBar.Index = m.sidebar.Cursor()
	m.statusBar.FilterText = uiSnap.FilterText
	m.statusBar.FilterTotal = m.logPanel.MatchCount()
	m.statusBar.FilterIndex = m.logPanel.MatchCursor()

	// Populate live status-bar segments from cached git+port info.
	if sel := uiSnap.SelectedID; sel != "" {
		if gi, ok := m.lastGitInfo[sel]; ok {
			m.statusBar.GitBranch = gi.branch
			m.statusBar.GitAhead = gi.ahead
			m.statusBar.GitBehind = gi.behind
			m.statusBar.GitDirty = gi.dirty
		} else {
			m.statusBar.GitBranch = ""
			m.statusBar.GitAhead = 0
			m.statusBar.GitBehind = 0
			m.statusBar.GitDirty = false
		}
		m.statusBar.Port = m.lastPort[sel]
		// Populate PID from runtime snapshot.
		for _, r := range m.lastRuntimeSnap {
			if r.ID == sel {
				m.statusBar.PID = r.PID
				break
			}
		}
	}

	main := lipgloss.JoinHorizontal(lipgloss.Top,
		m.sidebar.View(),
		m.logPanel.View(),
	)
	var base string
	switch {
	case commandVisible:
		cmdRow := m.overlays.Command.View(m.width)
		base = lipgloss.JoinVertical(lipgloss.Left, main, cmdRow, m.statusBar.View())
	case filterVisible:
		filterRow := m.overlays.Filter.View(m.width)
		base = lipgloss.JoinVertical(lipgloss.Left, main, filterRow, m.statusBar.View())
	default:
		base = lipgloss.JoinVertical(lipgloss.Left, main, m.statusBar.View())
	}
	base = applyOverlay(m, base)
	base = applyToasts(m, base)
	return base
}

// applyOverlay composes the active popup onto base as a centered floating box.
// The surrounding UI (sidebar, log, status, filter bar) stays visible around
// the popup — see overlayCenter. First visible wins. The filter bar is
// rendered inline in View and intentionally skipped here.
func applyOverlay(m Model, base string) string {
	if m.overlays.Help.Visible() {
		if v := m.overlays.Help.View(m.keys, m.width, m.height); v != "" {
			return overlayCenter(base, v, m.width, m.height)
		}
	}
	if m.overlays.Group.Visible() {
		if v := m.overlays.Group.View(m.width, m.height); v != "" {
			return overlayCenter(base, v, m.width, m.height)
		}
	}
	if m.overlays.Branch.Visible() {
		if v := m.overlays.Branch.View(m.width, m.height); v != "" {
			return overlayCenter(base, v, m.width, m.height)
		}
	}
	return base
}

// applyToasts overlays the toast stack onto base, anchored just above the
// status bar in the bottom-right corner. base must already be a full frame of
// m.width × m.height characters; the toast block replaces the right edge of
// the lines it covers so the rest of the UI remains visible.
func applyToasts(m Model, base string) string {
	v := m.overlays.Toasts.View(m.width, m.height)
	if v == "" {
		return base
	}
	// Status bar occupies the last line (m.height-1); anchor toasts one line above.
	return overlayBottomRight(base, v, m.width, m.height-2)
}

// ---- helpers ----

// sidebarWidth returns the sidebar width tailored to fit the widest configured
// project ID (plus the glyph + restart suffix + border). Clamped so it never
// shrinks below a readable minimum or dominates the log panel on narrow
// terminals. Falls back to a small default when no projects are configured.
func sidebarWidth(totalWidth int, cfg *config.Config) int {
	const titleLen = len("PROCESSES")
	// Per-row layout inside the sidebar (see panes/sidebar.go):
	//   "<glyph> <id>[ (×N)]"  fits within innerW-3, so required content
	//   width = longestID + 3 (glyph+spaces) + 5 (worst case " (×99)").
	const rowOverhead = 3 + 5

	longest := titleLen
	if cfg != nil {
		for id := range cfg.Projects {
			if l := len(id) + rowOverhead; l > longest {
				longest = l
			}
		}
	}
	w := longest + 2 // +2 for the border cells

	if w < sidebarMinWidth {
		w = sidebarMinWidth
	}
	if w > sidebarMaxWidth {
		w = sidebarMaxWidth
	}
	// Never let the sidebar eat more than half the terminal.
	if w > totalWidth/2 {
		w = totalWidth / 2
	}
	return w
}

// logFlushInterval reads the configured flush interval.
func logFlushInterval(cfg *config.Config) time.Duration {
	d := time.Duration(cfg.Settings.LogFlushIntervalMs) * time.Millisecond
	if d <= 0 {
		d = 150 * time.Millisecond
	}
	return d
}

// syncSelectedToStore writes the sidebar selection to UIStore.
func (m *Model) syncSelectedToStore() {
	if id := m.sidebar.Selected(); id != "" {
		m.ui.SetSelectedID(id)
		m.statusBar.Selected = id
	}
}

// syncSelectedFromStore reads UIStore.SelectedID and updates the sidebar cursor.
func (m *Model) syncSelectedFromStore() {
	uiSnap := m.ui.Snapshot()
	if uiSnap.SelectedID != "" {
		m.sidebar.SetSelected(uiSnap.SelectedID)
	} else if id := m.sidebar.Selected(); id != "" {
		m.ui.SetSelectedID(id)
	}
}

// refreshLogPanel re-reads the ring buffer for the selected project.
func (m *Model) refreshLogPanel() {
	id := m.sidebar.Selected()
	if id == "" {
		return
	}
	entry := m.reg.Get(id)
	if entry == nil {
		return
	}
	gen := entry.Ring.Generation()
	if gen == m.lastLogGen {
		return
	}
	m.lastLogGen = gen
	lines := entry.Ring.Snapshot()
	rendered := make([]string, len(lines))
	for i, l := range lines {
		rendered[i] = log.DecodeForRender(l.Bytes)
	}
	m.logPanel.SetLines(rendered)
}

