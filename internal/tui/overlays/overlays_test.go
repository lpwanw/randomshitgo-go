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

// ---- BranchPicker tests ----

func TestBranchPicker_Filter_NarrowsList(t *testing.T) {
	bp := NewBranchPicker([]string{"main", "dev", "feature/login", "feature/signup"})
	bp.Show()

	// Type 'f' — filter to two feature branches.
	bp2, _ := bp.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'f'}})
	if bp2.filter != "f" {
		t.Errorf("filter after 'f': want %q, got %q", "f", bp2.filter)
	}
	if len(bp2.filtered) != 2 {
		t.Errorf("filtered len after 'f': want 2, got %d", len(bp2.filtered))
	}

	// Type 'eature/l' — narrow to one.
	bp3 := bp2
	for _, r := range "eature/l" {
		bp3, _ = bp3.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{r}})
	}
	if len(bp3.filtered) != 1 {
		t.Errorf("filtered len after 'feature/l': want 1, got %d", len(bp3.filtered))
	}
	if got := bp3.branches[bp3.filtered[0]]; got != "feature/login" {
		t.Errorf("match: want feature/login, got %q", got)
	}
}

func TestBranchPicker_Filter_CaseInsensitive(t *testing.T) {
	bp := NewBranchPicker([]string{"Main", "DevBranch"})
	bp.Show()

	bp2, _ := bp.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'d'}})
	if len(bp2.filtered) != 1 {
		t.Fatalf("case-insensitive match: want 1 match, got %d", len(bp2.filtered))
	}
	if bp2.branches[bp2.filtered[0]] != "DevBranch" {
		t.Errorf("match: want DevBranch, got %q", bp2.branches[bp2.filtered[0]])
	}
}

func TestBranchPicker_Backspace_PopsFilter(t *testing.T) {
	bp := NewBranchPicker([]string{"main", "dev"})
	bp.Show()

	for _, r := range "de" {
		bp, _ = bp.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{r}})
	}
	if bp.filter != "de" {
		t.Fatalf("setup filter: want 'de', got %q", bp.filter)
	}

	bp, _ = bp.Update(tea.KeyMsg{Type: tea.KeyBackspace})
	if bp.filter != "d" {
		t.Errorf("after backspace: want 'd', got %q", bp.filter)
	}

	bp, _ = bp.Update(tea.KeyMsg{Type: tea.KeyBackspace})
	if bp.filter != "" {
		t.Errorf("after backspace to empty: want \"\", got %q", bp.filter)
	}

	// Backspace on empty filter is a no-op (no panic).
	bp, _ = bp.Update(tea.KeyMsg{Type: tea.KeyBackspace})
	if bp.filter != "" {
		t.Errorf("backspace on empty: want \"\", got %q", bp.filter)
	}
}

func TestBranchPicker_Esc_ClearsFilterThenHides(t *testing.T) {
	bp := NewBranchPicker([]string{"main", "dev"})
	bp.Show()
	bp, _ = bp.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'d'}})
	if bp.filter == "" {
		t.Fatal("setup: expected non-empty filter")
	}

	// First Esc clears filter, stays visible.
	bp, _ = bp.Update(tea.KeyMsg{Type: tea.KeyEsc})
	if bp.filter != "" {
		t.Errorf("first Esc: want filter cleared, got %q", bp.filter)
	}
	if !bp.Visible() {
		t.Error("first Esc with filter: picker should stay visible")
	}

	// Second Esc closes.
	bp, _ = bp.Update(tea.KeyMsg{Type: tea.KeyEsc})
	if bp.Visible() {
		t.Error("second Esc: picker should hide")
	}
}

func TestBranchPicker_Enter_UsesFilteredIndex(t *testing.T) {
	bp := NewBranchPicker([]string{"main", "dev", "feature/login"})
	bp.Show()

	// Type 'login' — filtered to ["feature/login"], cursor at 0.
	for _, r := range "login" {
		bp, _ = bp.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{r}})
	}

	_, cmd := bp.Update(tea.KeyMsg{Type: tea.KeyEnter})
	if cmd == nil {
		t.Fatal("Enter should emit a Cmd")
	}
	msg := cmd()
	cb, ok := msg.(CheckoutBranchMsg)
	if !ok {
		t.Fatalf("expected CheckoutBranchMsg, got %T", msg)
	}
	if cb.Branch != "feature/login" {
		t.Errorf("checkout branch: want feature/login, got %q", cb.Branch)
	}
}

func TestBranchPicker_Enter_NoMatches_IsNoop(t *testing.T) {
	bp := NewBranchPicker([]string{"main", "dev"})
	bp.Show()

	// Type 'zzz' — no matches.
	for _, r := range "zzz" {
		bp, _ = bp.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{r}})
	}
	if len(bp.filtered) != 0 {
		t.Fatalf("setup: want 0 matches, got %d", len(bp.filtered))
	}

	bp2, cmd := bp.Update(tea.KeyMsg{Type: tea.KeyEnter})
	if cmd != nil {
		t.Error("Enter on empty filter should not emit a Cmd")
	}
	if !bp2.Visible() {
		t.Error("Enter on empty filter: picker should stay visible")
	}
}

func TestBranchPicker_ArrowNav_RespectFilteredBounds(t *testing.T) {
	bp := NewBranchPicker([]string{"main", "dev", "feature/login"})
	bp.Show()

	// Filter to "feature/login" only (single match).
	for _, r := range "login" {
		bp, _ = bp.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{r}})
	}
	if len(bp.filtered) != 1 {
		t.Fatalf("setup: want 1 match, got %d", len(bp.filtered))
	}

	// KeyDown must not overflow past the single element.
	bp, _ = bp.Update(tea.KeyMsg{Type: tea.KeyDown})
	if bp.cursor != 0 {
		t.Errorf("KeyDown on 1-match list: want cursor 0, got %d", bp.cursor)
	}
	// KeyUp at top stays at 0.
	bp, _ = bp.Update(tea.KeyMsg{Type: tea.KeyUp})
	if bp.cursor != 0 {
		t.Errorf("KeyUp at top: want cursor 0, got %d", bp.cursor)
	}
}

func TestBranchPicker_LettersAreFilter_NotNav(t *testing.T) {
	// Regression: 'j'/'k' used to hit pickerUp/Down — that made branches
	// containing those letters unreachable via filter.
	bp := NewBranchPicker([]string{"main", "jumbo"})
	bp.Show()

	bp, _ = bp.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'j'}})
	if bp.filter != "j" {
		t.Errorf("after 'j' key: want filter 'j', got %q", bp.filter)
	}
	if len(bp.filtered) != 1 || bp.branches[bp.filtered[0]] != "jumbo" {
		t.Errorf("after 'j' key: want only 'jumbo' filtered, got %v", bp.filtered)
	}
}

func TestBranchPicker_SetBranches_ResetsFilterAndCursor(t *testing.T) {
	bp := NewBranchPicker([]string{"main"})
	bp.Show()
	bp, _ = bp.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'x'}})
	if bp.filter != "x" {
		t.Fatal("setup: expected filter x")
	}

	bp.SetBranches([]string{"a", "b", "c"})
	if bp.filter != "" {
		t.Errorf("SetBranches: filter should reset, got %q", bp.filter)
	}
	if bp.cursor != 0 {
		t.Errorf("SetBranches: cursor should reset, got %d", bp.cursor)
	}
	if len(bp.filtered) != 3 {
		t.Errorf("SetBranches: filtered len should be 3, got %d", len(bp.filtered))
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

func TestFilterInput_View_IsSingleLineBar(t *testing.T) {
	fi := NewFilterInput()
	fi.Show()
	fi.SetValue("err")

	view := fi.View(60)
	if view == "" {
		t.Fatal("visible filter should render non-empty view")
	}
	if strings.Contains(view, "\n") {
		t.Errorf("filter bar must be single-line, got newline in %q", view)
	}
	if !strings.Contains(view, "/") {
		t.Errorf("filter bar must start with '/' prompt, got %q", view)
	}
}

func TestFilterInput_View_HiddenReturnsEmpty(t *testing.T) {
	fi := NewFilterInput()
	if v := fi.View(60); v != "" {
		t.Errorf("hidden filter must return empty view, got %q", v)
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

// ---- CommandInput tests ----

func TestCommandInput_EnterWithText_EmitsRunMsg(t *testing.T) {
	ci := NewCommandInput()
	ci.Show()
	ci.ti.SetValue("q")

	ci2, cmd := ci.Update(tea.KeyMsg{Type: tea.KeyEnter})
	if cmd == nil {
		t.Fatal("Enter should return a Cmd")
	}
	msg := cmd()
	run, ok := msg.(CommandRunMsg)
	if !ok {
		t.Fatalf("expected CommandRunMsg, got %T", msg)
	}
	if run.Text != "q" {
		t.Errorf("CommandRunMsg.Text: want %q, got %q", "q", run.Text)
	}
	if ci2.Visible() {
		t.Error("command bar must hide after Enter")
	}
}

func TestCommandInput_EmptyEnter_EmitsCancel(t *testing.T) {
	ci := NewCommandInput()
	ci.Show()

	_, cmd := ci.Update(tea.KeyMsg{Type: tea.KeyEnter})
	if cmd == nil {
		t.Fatal("Enter with empty input should still return a Cmd")
	}
	if _, ok := cmd().(CommandCancelMsg); !ok {
		t.Fatalf("empty Enter should emit CommandCancelMsg, got %T", cmd())
	}
}

func TestCommandInput_Esc_EmitsCancel(t *testing.T) {
	ci := NewCommandInput()
	ci.Show()
	ci.ti.SetValue("anything")

	ci2, cmd := ci.Update(tea.KeyMsg{Type: tea.KeyEsc})
	if cmd == nil {
		t.Fatal("Esc should return a Cmd")
	}
	if _, ok := cmd().(CommandCancelMsg); !ok {
		t.Fatalf("Esc should emit CommandCancelMsg, got %T", cmd())
	}
	if ci2.Visible() {
		t.Error("command bar must hide after Esc")
	}
}

func TestCommandInput_View_IsSingleLineBar(t *testing.T) {
	ci := NewCommandInput()
	ci.Show()
	view := ci.View(60)
	if view == "" {
		t.Fatal("visible command bar should render non-empty")
	}
	if strings.Contains(view, "\n") {
		t.Errorf("command bar must be single-line, got newline: %q", view)
	}
	if !strings.Contains(view, ":") {
		t.Errorf("command bar must start with ':' prompt, got %q", view)
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
