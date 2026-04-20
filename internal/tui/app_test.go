package tui

import (
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/taynguyen/procs/internal/config"
	"github.com/taynguyen/procs/internal/process"
	"github.com/taynguyen/procs/internal/state"
)

// newTestModel creates a minimal Model suitable for unit tests without requiring
// a real process manager or config file.
func newTestModel() Model {
	cfg := &config.Config{
		Projects: map[string]config.Project{
			"api": {Path: "/tmp", Cmd: "echo api"},
			"web": {Path: "/tmp", Cmd: "echo web"},
		},
		Settings: config.Settings{
			LogBufferLines:     100,
			LogFlushIntervalMs: 150,
			ShutdownGraceMs:    500,
			PtyCols:            80,
			PtyRows:            24,
		},
	}

	rt := state.NewRuntimeStore()
	ui := state.NewUIStore()
	reg := state.NewRegistry(cfg.Settings)
	mgr := process.New(cfg, reg)

	return New(cfg, mgr, rt, ui, reg)
}

// feedMsg feeds a message through Update and returns the updated model and cmd.
func feedMsg(m Model, msg tea.Msg) (Model, tea.Cmd) {
	result, cmd := m.Update(msg)
	return result.(Model), cmd
}

// ---- Window size ----

func TestModel_WindowSizeSetsWidthHeight(t *testing.T) {
	m := newTestModel()
	m, _ = feedMsg(m, tea.WindowSizeMsg{Width: 120, Height: 40})

	if m.width != 120 {
		t.Errorf("width: want 120, got %d", m.width)
	}
	if m.height != 40 {
		t.Errorf("height: want 40, got %d", m.height)
	}
}

// ---- Help mode ----

func TestModel_QuestionMark_EntersHelpMode(t *testing.T) {
	m := newTestModel()
	m, _ = feedMsg(m, tea.WindowSizeMsg{Width: 120, Height: 40})
	m, _ = feedMsg(m, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'?'}})

	if m.mode != ModeHelp {
		t.Errorf("? key: want ModeHelp, got %s", m.mode)
	}
	if !m.overlays.Help.Visible() {
		t.Error("help overlay should be visible")
	}
}

func TestModel_EscFromHelp_ReturnsToNormal(t *testing.T) {
	m := newTestModel()
	m, _ = feedMsg(m, tea.WindowSizeMsg{Width: 120, Height: 40})
	m, _ = feedMsg(m, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'?'}})
	m, _ = feedMsg(m, tea.KeyMsg{Type: tea.KeyEsc})

	if m.mode != ModeNormal {
		t.Errorf("esc from help: want ModeNormal, got %s", m.mode)
	}
}

func TestModel_QuestionMarkFromHelp_Toggles(t *testing.T) {
	m := newTestModel()
	m, _ = feedMsg(m, tea.WindowSizeMsg{Width: 120, Height: 40})
	m, _ = feedMsg(m, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'?'}})
	m, _ = feedMsg(m, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'?'}})

	if m.mode != ModeNormal {
		t.Errorf("double ? should return to normal, got %s", m.mode)
	}
}

// ---- Filter mode ----

func TestModel_Slash_EntersFilterMode(t *testing.T) {
	m := newTestModel()
	m, _ = feedMsg(m, tea.WindowSizeMsg{Width: 120, Height: 40})
	m, _ = feedMsg(m, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'/'}})

	if m.mode != ModeFilter {
		t.Errorf("/ key: want ModeFilter, got %s", m.mode)
	}
	if !m.overlays.Filter.Visible() {
		t.Error("filter overlay should be visible")
	}
}

func TestModel_EscFromFilter_ClearsAndReturnsToNormal(t *testing.T) {
	m := newTestModel()
	m, _ = feedMsg(m, tea.WindowSizeMsg{Width: 120, Height: 40})
	m, _ = feedMsg(m, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'/'}})

	// Esc while in filter mode.
	m, _ = feedMsg(m, tea.KeyMsg{Type: tea.KeyEsc})

	if m.mode != ModeNormal {
		t.Errorf("esc from filter: want ModeNormal, got %s", m.mode)
	}
	if m.overlays.Filter.Visible() {
		t.Error("filter overlay should be hidden after esc")
	}
}

// ---- Group picker mode ----

func TestModel_ShiftS_EntersGroupPicker(t *testing.T) {
	m := newTestModel()
	m, _ = feedMsg(m, tea.WindowSizeMsg{Width: 120, Height: 40})
	m, _ = feedMsg(m, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'S'}})

	if m.mode != ModeGroupPicker {
		t.Errorf("S key: want ModeGroupPicker, got %s", m.mode)
	}
}

// ---- Quit ----

func TestModel_QKey_ReturnsQuitCmd(t *testing.T) {
	m := newTestModel()
	m, _ = feedMsg(m, tea.WindowSizeMsg{Width: 120, Height: 40})

	_, cmd := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'q'}})
	if cmd == nil {
		t.Fatal("q key should return a Cmd")
	}

	// The cmd is the gracefulQuit closure; calling it should return a QuitMsg.
	// Note: gracefulQuit blocks on StopAll, but our test manager has no children.
	msg := cmd()
	if _, ok := msg.(tea.QuitMsg); !ok {
		t.Errorf("q Cmd should emit QuitMsg, got %T", msg)
	}
}

// ---- RuntimeChangedMsg ----

func TestModel_RuntimeChangedMsg_UpdatesSidebar(t *testing.T) {
	m := newTestModel()
	m, _ = feedMsg(m, tea.WindowSizeMsg{Width: 120, Height: 40})

	snap := []state.ProjectRuntime{
		{ID: "api", State: "running"},
		{ID: "web", State: "idle"},
	}
	m, _ = feedMsg(m, RuntimeChangedMsg{Snapshot: snap})

	if m.sidebar.Selected() != "api" {
		t.Errorf("after runtime msg: want selected=api, got %q", m.sidebar.Selected())
	}
}

// ---- LogTickMsg ----

func TestModel_LogTickMsg_RearmsCmd(t *testing.T) {
	m := newTestModel()
	m, _ = feedMsg(m, tea.WindowSizeMsg{Width: 120, Height: 40})

	_, cmd := m.Update(LogTickMsg(time.Now()))
	if cmd == nil {
		t.Error("LogTickMsg should return a re-arm Cmd")
	}
}

// ---- Toast ----

func TestModel_ShowToastMsg_AddsToast(t *testing.T) {
	m := newTestModel()
	m, _ = feedMsg(m, ShowToastMsg{Text: "hello", Level: 0})

	if m.overlays.Toasts.Len() != 1 {
		t.Errorf("want 1 toast, got %d", m.overlays.Toasts.Len())
	}
}

func TestModel_ToastExpiredMsg_PrunesExpired(t *testing.T) {
	m := newTestModel()
	m.overlays.Toasts.AddWithTTL("expires", 0, 1*time.Millisecond)

	time.Sleep(5 * time.Millisecond)
	m, _ = feedMsg(m, ToastExpiredMsg{})

	if m.overlays.Toasts.Len() != 0 {
		t.Errorf("expired toast should be pruned, got %d toasts", m.overlays.Toasts.Len())
	}
}

// ---- View smoke ----

func TestModel_View_NonemptyAfterResize(t *testing.T) {
	m := newTestModel()
	m, _ = feedMsg(m, tea.WindowSizeMsg{Width: 120, Height: 40})

	view := m.View()
	if view == "" {
		t.Error("View() should not return empty after resize")
	}
}

func TestModel_View_TooSmallMessage(t *testing.T) {
	m := newTestModel()
	m, _ = feedMsg(m, tea.WindowSizeMsg{Width: 20, Height: 5})

	view := m.View()
	if view == "" {
		t.Error("View() should return 'too small' message")
	}
}
