package overlays

import (
	"strings"
	"testing"
	"time"

	"github.com/charmbracelet/bubbles/key"
	tea "github.com/charmbracelet/bubbletea"
)

// ---- HelpOverlay tests ----

func TestHelpOverlay_Toggle(t *testing.T) {
	h := NewHelpOverlay()
	if h.Visible() {
		t.Error("help should start hidden")
	}
	h.Toggle()
	if !h.Visible() {
		t.Error("help should be visible after toggle")
	}
	h.Toggle()
	if h.Visible() {
		t.Error("help should be hidden after second toggle")
	}
}

func TestHelpOverlay_View_HiddenReturnsEmpty(t *testing.T) {
	h := NewHelpOverlay()
	view := h.View(stubKeyMap{}, 120, 40)
	if view != "" {
		t.Errorf("hidden help overlay should return empty string, got %q", view)
	}
}

func TestHelpOverlay_View_VisibleReturnsContent(t *testing.T) {
	h := NewHelpOverlay()
	h.Show()
	view := h.View(stubKeyMap{}, 120, 40)
	if view == "" {
		t.Error("visible help overlay should return non-empty content")
	}
}

// stubKeyMap satisfies KeyMapProvider with minimal bindings for testing.
type stubKeyMap struct{}

func (s stubKeyMap) ShortHelp() []key.Binding { return nil }
func (s stubKeyMap) FullHelp() [][]key.Binding { return nil }

// ---- GroupPicker tests ----

func TestGroupPicker_NavBounds(t *testing.T) {
	groups := map[string][]string{
		"frontend": {"web"},
		"backend":  {"api", "db"},
	}
	gp := NewGroupPicker(groups)
	gp.Show()

	// Up at top should stay at 0.
	gp2, _ := gp.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'k'}})
	if gp2.cursor != 0 {
		t.Errorf("Up at top: want cursor 0, got %d", gp2.cursor)
	}

	// Down twice.
	gp3, _ := gp2.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'j'}})
	if gp3.cursor != 1 {
		t.Errorf("Down once: want cursor 1, got %d", gp3.cursor)
	}

	// Down again should clamp at last.
	gp4, _ := gp3.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'j'}})
	if gp4.cursor != 1 {
		t.Errorf("Down at bottom: want cursor 1, got %d", gp4.cursor)
	}
}

func TestGroupPicker_Enter_EmitsStartGroupMsg(t *testing.T) {
	groups := map[string][]string{
		"all": {"api", "web"},
	}
	gp := NewGroupPicker(groups)
	gp.Show()

	_, cmd := gp.Update(tea.KeyMsg{Type: tea.KeyEnter})
	if cmd == nil {
		t.Fatal("Enter should emit a Cmd")
	}

	msg := cmd()
	sgm, ok := msg.(StartGroupMsg)
	if !ok {
		t.Fatalf("expected StartGroupMsg, got %T", msg)
	}
	if sgm.Name != "all" {
		t.Errorf("StartGroupMsg.Name: want 'all', got %q", sgm.Name)
	}
}

func TestGroupPicker_Esc_Hides(t *testing.T) {
	groups := map[string][]string{"grp": {"a"}}
	gp := NewGroupPicker(groups)
	gp.Show()

	gp2, _ := gp.Update(tea.KeyMsg{Type: tea.KeyEsc})
	if gp2.Visible() {
		t.Error("group picker should be hidden after Esc")
	}
}

// ---- FilterInput tests ----

func TestFilterInput_EnterWithValidRegex_EmitsCommitMsg(t *testing.T) {
	fi := NewFilterInput()
	fi.Show()
	fi.SetValue("error")

	fi2, cmd := fi.Update(tea.KeyMsg{Type: tea.KeyEnter})
	if cmd == nil {
		t.Fatal("Enter should emit a Cmd")
	}

	msg := cmd()
	commit, ok := msg.(FilterCommitMsg)
	if !ok {
		t.Fatalf("expected FilterCommitMsg, got %T", msg)
	}
	if commit.Text != "error" {
		t.Errorf("FilterCommitMsg.Text: want 'error', got %q", commit.Text)
	}
	if commit.Regex == nil {
		t.Error("FilterCommitMsg.Regex should not be nil for valid input")
	}
	if fi2.Visible() {
		t.Error("filter input should be hidden after Enter")
	}
}

func TestFilterInput_EnterWithInvalidRegex_EmitsToast(t *testing.T) {
	fi := NewFilterInput()
	fi.Show()
	fi.SetValue("(unclosed")

	_, cmd := fi.Update(tea.KeyMsg{Type: tea.KeyEnter})
	if cmd == nil {
		t.Fatal("invalid regex should emit a Cmd (toast)")
	}

	msg := cmd()
	_, ok := msg.(ShowToastMsg)
	if !ok {
		t.Fatalf("expected ShowToastMsg for invalid regex, got %T", msg)
	}
}

func TestFilterInput_Esc_EmitsCancelMsg(t *testing.T) {
	fi := NewFilterInput()
	fi.Show()
	fi.SetValue("something")

	fi2, cmd := fi.Update(tea.KeyMsg{Type: tea.KeyEsc})
	if cmd == nil {
		t.Fatal("Esc should emit a Cmd")
	}
	msg := cmd()
	if _, ok := msg.(FilterCancelMsg); !ok {
		t.Fatalf("expected FilterCancelMsg, got %T", msg)
	}
	if fi2.Visible() {
		t.Error("filter should be hidden after Esc")
	}
}

func TestFilterInput_EmptyEnter_EmitsCommitWithNilRegex(t *testing.T) {
	fi := NewFilterInput()
	fi.Show()
	fi.SetValue("")

	_, cmd := fi.Update(tea.KeyMsg{Type: tea.KeyEnter})
	if cmd == nil {
		t.Fatal("empty Enter should emit Cmd")
	}
	msg := cmd()
	commit, ok := msg.(FilterCommitMsg)
	if !ok {
		t.Fatalf("expected FilterCommitMsg, got %T", msg)
	}
	if commit.Regex != nil {
		t.Error("empty filter should have nil Regex")
	}
}

// ---- Toast tests ----

func TestToastStack_Add_MaxCapped(t *testing.T) {
	ts := &ToastStack{}
	for i := 0; i < 10; i++ {
		ts.Add("msg", ToastInfo)
	}
	if ts.Len() > maxToasts {
		t.Errorf("toast stack should cap at %d, got %d", maxToasts, ts.Len())
	}
}

func TestToastStack_Prune_RemovesExpired(t *testing.T) {
	ts := &ToastStack{}
	ts.AddWithTTL("expires", ToastInfo, 1*time.Millisecond)
	ts.Add("live", ToastInfo) // default TTL

	time.Sleep(5 * time.Millisecond)
	ts.Prune(time.Now())

	if ts.Len() != 1 {
		t.Errorf("after prune want 1 toast, got %d", ts.Len())
	}
}

func TestToastStack_View_Empty(t *testing.T) {
	ts := &ToastStack{}
	view := ts.View(120, 40)
	if view != "" {
		t.Errorf("empty toast stack should return empty view, got %q", view)
	}
}

func TestToastStack_View_NonEmpty(t *testing.T) {
	ts := &ToastStack{}
	ts.Add("hello toast", ToastInfo)
	view := ts.View(120, 40)
	if !strings.Contains(view, "hello toast") {
		t.Errorf("toast view should contain 'hello toast', got %q", view)
	}
}
