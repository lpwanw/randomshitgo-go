package tui

import (
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/lpwanw/randomshitgo-go/internal/config"
	"github.com/lpwanw/randomshitgo-go/internal/tui/overlays"
	"github.com/lpwanw/randomshitgo-go/internal/process"
	"github.com/lpwanw/randomshitgo-go/internal/state"
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

// overlaysPkgCommandRun builds an overlays.CommandRunMsg for tests.
func overlaysPkgCommandRun(text string) tea.Msg {
	return overlays.CommandRunMsg{Text: text}
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

// TestModel_QKey_DoesNothing guards the "accidental quit" bug: bare `q` in
// Normal mode must NOT quit or stop processes — quit requires `:q`.
func TestModel_QKey_DoesNothing(t *testing.T) {
	m := newTestModel()
	m, _ = feedMsg(m, tea.WindowSizeMsg{Width: 120, Height: 40})

	_, cmd := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'q'}})
	if cmd != nil {
		t.Fatal("bare q must not return a Cmd in Normal mode")
	}
	if m.mode != ModeNormal {
		t.Errorf("bare q must not change mode, got %s", m.mode)
	}
}

// TestModel_FirstCtrlC_ArmsAndShowsHint verifies the first Ctrl+C arms the
// quit state and surfaces a hint — it must NOT quit yet.
func TestModel_FirstCtrlC_ArmsAndShowsHint(t *testing.T) {
	m := newTestModel()
	m, _ = feedMsg(m, tea.WindowSizeMsg{Width: 120, Height: 40})

	before := m.overlays.Toasts.Len()
	m, cmd := feedMsg(m, tea.KeyMsg{Type: tea.KeyCtrlC})
	if cmd != nil {
		t.Errorf("first Ctrl+C must NOT return a quit Cmd")
	}
	if m.quitArmedAt.IsZero() {
		t.Error("first Ctrl+C must arm quitArmedAt")
	}
	if m.overlays.Toasts.Len() != before+1 {
		t.Errorf("first Ctrl+C should add a hint toast; toasts %d → %d", before, m.overlays.Toasts.Len())
	}
}

// TestModel_SecondCtrlC_WithinWindow_Quits confirms double-tap fires quit.
func TestModel_SecondCtrlC_WithinWindow_Quits(t *testing.T) {
	m := newTestModel()
	m, _ = feedMsg(m, tea.WindowSizeMsg{Width: 120, Height: 40})

	m, _ = feedMsg(m, tea.KeyMsg{Type: tea.KeyCtrlC})
	m2, cmd := feedMsg(m, tea.KeyMsg{Type: tea.KeyCtrlC})
	_ = m2
	if cmd == nil {
		t.Fatal("second Ctrl+C should return a quit Cmd")
	}
	if msg := cmd(); msg == nil {
		t.Errorf("second Ctrl+C Cmd should emit tea.QuitMsg, got nil")
	}
}

// TestModel_CtrlC_OtherKey_Disarms verifies an intervening key resets the arm
// so the next Ctrl+C starts the double-tap dance over.
func TestModel_CtrlC_OtherKey_Disarms(t *testing.T) {
	m := newTestModel()
	m, _ = feedMsg(m, tea.WindowSizeMsg{Width: 120, Height: 40})

	m, _ = feedMsg(m, tea.KeyMsg{Type: tea.KeyCtrlC})
	if m.quitArmedAt.IsZero() {
		t.Fatal("Ctrl+C should have armed the quit state")
	}

	m, _ = feedMsg(m, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'j'}})
	if !m.quitArmedAt.IsZero() {
		t.Fatal("unrelated key should disarm quit state")
	}

	_, cmd := feedMsg(m, tea.KeyMsg{Type: tea.KeyCtrlC})
	if cmd != nil {
		t.Errorf("Ctrl+C after disarm should re-arm, not quit")
	}
}

// TestModel_ColonQ_TriggersGracefulQuit walks through the new command bar:
// `:` opens it, then `CommandRunMsg{q}` fires graceful quit.
func TestModel_ColonQ_TriggersGracefulQuit(t *testing.T) {
	m := newTestModel()
	m, _ = feedMsg(m, tea.WindowSizeMsg{Width: 120, Height: 40})

	m, _ = feedMsg(m, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{':'}})
	if m.mode != ModeCommand {
		t.Fatalf("': ' should enter ModeCommand, got %s", m.mode)
	}
	if !m.overlays.Command.Visible() {
		t.Fatal("command overlay should be visible")
	}

	_, cmd := m.Update(overlaysPkgCommandRun("q"))
	if cmd == nil {
		t.Fatal(":q should return a graceful quit Cmd")
	}
	if msg := cmd(); msg == nil {
		t.Errorf(":q Cmd should emit tea.QuitMsg, got nil")
	}
}

// TestModel_UnknownCommand_ToastsError ensures typos don't quit.
func TestModel_UnknownCommand_ToastsError(t *testing.T) {
	m := newTestModel()
	m, _ = feedMsg(m, tea.WindowSizeMsg{Width: 120, Height: 40})

	before := m.overlays.Toasts.Len()
	m, cmd := feedMsg(m, overlaysPkgCommandRun("bogus"))
	if cmd != nil {
		t.Errorf("unknown command must not return a quit Cmd")
	}
	if m.overlays.Toasts.Len() <= before {
		t.Errorf("unknown command should add an error toast; count %d → %d", before, m.overlays.Toasts.Len())
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

// TestModel_DigitKey_JumpsToProject verifies the 1-9 quick-jump bindings land
// on the right sidebar row and refresh the selection.
func TestModel_DigitKey_JumpsToProject(t *testing.T) {
	m := newTestModel()
	m, _ = feedMsg(m, tea.WindowSizeMsg{Width: 120, Height: 40})
	m.runtime.Seed([]string{"api", "web", "db"})
	m, _ = feedMsg(m, RuntimeChangedMsg{Snapshot: m.runtime.Snapshot()})

	m, _ = feedMsg(m, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'2'}})
	if got := m.sidebar.Selected(); got != "db" && got != "web" {
		// Runtime snapshot is alphabetically sorted so row 1 = "api", row 2 = "db", row 3 = "web".
		t.Errorf("after pressing 2, selection should advance to row 1 (index 1); got %q", got)
	}
	if m.mode != ModeNormal {
		t.Errorf("digit key must not change mode, got %s", m.mode)
	}
}

// TestModel_DigitKey_OutOfRange_NoOp ensures an unmapped digit is a silent no-op.
func TestModel_DigitKey_OutOfRange_NoOp(t *testing.T) {
	m := newTestModel()
	m, _ = feedMsg(m, tea.WindowSizeMsg{Width: 120, Height: 40})
	m.runtime.Seed([]string{"api", "web"})
	m, _ = feedMsg(m, RuntimeChangedMsg{Snapshot: m.runtime.Snapshot()})

	before := m.sidebar.Selected()
	m, cmd := feedMsg(m, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'9'}})
	if cmd != nil {
		t.Errorf("out-of-range digit should not return a Cmd")
	}
	if m.sidebar.Selected() != before {
		t.Errorf("out-of-range digit should not change selection; was %q now %q", before, m.sidebar.Selected())
	}
}

// TestModel_DigitKey_InFilterMode_FallsThroughToFilterInput guards the mode
// scoping: digits typed with the filter bar open must go into the input, not
// the sidebar.
func TestModel_DigitKey_InFilterMode_FallsThroughToFilterInput(t *testing.T) {
	m := newTestModel()
	m, _ = feedMsg(m, tea.WindowSizeMsg{Width: 120, Height: 40})
	m.runtime.Seed([]string{"api", "web"})
	m, _ = feedMsg(m, RuntimeChangedMsg{Snapshot: m.runtime.Snapshot()})

	before := m.sidebar.Selected()
	m, _ = feedMsg(m, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'/'}})
	m, _ = feedMsg(m, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'3'}})

	if m.sidebar.Selected() != before {
		t.Errorf("digit in filter mode must not move sidebar; before=%q after=%q", before, m.sidebar.Selected())
	}
	if !strings.Contains(m.overlays.Filter.Value(), "3") {
		t.Errorf("digit in filter mode should be typed into the input; got %q", m.overlays.Filter.Value())
	}
}

// TestModel_View_OverlayKeepsSidebar guards the "popup takes the whole UI" bug:
// when the help overlay is visible, the sidebar title must still appear in the
// composed frame because the centered box should not blank unrelated rows.
func TestModel_View_OverlayKeepsSidebar(t *testing.T) {
	m := newTestModel()
	m, _ = feedMsg(m, tea.WindowSizeMsg{Width: 140, Height: 40})
	// Seed a runtime snapshot so the sidebar has rows to render.
	m, _ = feedMsg(m, RuntimeChangedMsg{Snapshot: m.runtime.Snapshot()})
	m.runtime.Seed([]string{"api", "web"})
	m, _ = feedMsg(m, RuntimeChangedMsg{Snapshot: m.runtime.Snapshot()})

	// Open help.
	m, _ = feedMsg(m, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'?'}})

	view := m.View()
	if !strings.Contains(view, "PROCESSES") {
		t.Errorf("help overlay blanked the sidebar title; frame:\n%s", view)
	}
}

// TestModel_View_FilterBarRendersInline checks the filter bar sits inline
// (not as a full-screen modal) and does not blank the main content.
func TestModel_View_FilterBarRendersInline(t *testing.T) {
	m := newTestModel()
	m, _ = feedMsg(m, tea.WindowSizeMsg{Width: 140, Height: 40})
	m.runtime.Seed([]string{"api", "web"})
	m, _ = feedMsg(m, RuntimeChangedMsg{Snapshot: m.runtime.Snapshot()})
	m, _ = feedMsg(m, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'/'}})

	view := m.View()
	if !strings.Contains(view, "PROCESSES") {
		t.Errorf("filter bar blanked sidebar; frame:\n%s", view)
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
