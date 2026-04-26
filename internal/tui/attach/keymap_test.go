package attach

import (
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	uv "github.com/charmbracelet/ultraviolet"
)

func TestMsgToKey(t *testing.T) {
	tests := []struct {
		name string
		in   tea.KeyMsg
		want uv.KeyPressEvent
		ok   bool
	}{
		{"rune", tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'a'}}, uv.KeyPressEvent{Code: 'a', Text: "a"}, true},
		{"rune+alt", tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'a'}, Alt: true}, uv.KeyPressEvent{Code: 'a', Text: "a", Mod: uv.ModAlt}, true},
		{"enter", tea.KeyMsg{Type: tea.KeyEnter}, uv.KeyPressEvent{Code: uv.KeyEnter}, true},
		{"tab", tea.KeyMsg{Type: tea.KeyTab}, uv.KeyPressEvent{Code: uv.KeyTab}, true},
		{"shift+tab", tea.KeyMsg{Type: tea.KeyShiftTab}, uv.KeyPressEvent{Code: uv.KeyTab, Mod: uv.ModShift}, true},
		{"backspace", tea.KeyMsg{Type: tea.KeyBackspace}, uv.KeyPressEvent{Code: uv.KeyBackspace}, true},
		{"esc", tea.KeyMsg{Type: tea.KeyEsc}, uv.KeyPressEvent{Code: uv.KeyEscape}, true},
		{"up", tea.KeyMsg{Type: tea.KeyUp}, uv.KeyPressEvent{Code: uv.KeyUp}, true},
		{"shift+up", tea.KeyMsg{Type: tea.KeyShiftUp}, uv.KeyPressEvent{Code: uv.KeyUp, Mod: uv.ModShift}, true},
		{"ctrl+up", tea.KeyMsg{Type: tea.KeyCtrlUp}, uv.KeyPressEvent{Code: uv.KeyUp, Mod: uv.ModCtrl}, true},
		{"down", tea.KeyMsg{Type: tea.KeyDown}, uv.KeyPressEvent{Code: uv.KeyDown}, true},
		{"home", tea.KeyMsg{Type: tea.KeyHome}, uv.KeyPressEvent{Code: uv.KeyHome}, true},
		{"end", tea.KeyMsg{Type: tea.KeyEnd}, uv.KeyPressEvent{Code: uv.KeyEnd}, true},
		{"pgup", tea.KeyMsg{Type: tea.KeyPgUp}, uv.KeyPressEvent{Code: uv.KeyPgUp}, true},
		{"pgdown", tea.KeyMsg{Type: tea.KeyPgDown}, uv.KeyPressEvent{Code: uv.KeyPgDown}, true},
		{"insert", tea.KeyMsg{Type: tea.KeyInsert}, uv.KeyPressEvent{Code: uv.KeyInsert}, true},
		{"delete", tea.KeyMsg{Type: tea.KeyDelete}, uv.KeyPressEvent{Code: uv.KeyDelete}, true},
		{"f1", tea.KeyMsg{Type: tea.KeyF1}, uv.KeyPressEvent{Code: uv.KeyF1}, true},
		{"f12", tea.KeyMsg{Type: tea.KeyF12}, uv.KeyPressEvent{Code: uv.KeyF12}, true},
		{"ctrl+a", tea.KeyMsg{Type: tea.KeyCtrlA}, uv.KeyPressEvent{Code: 'a', Mod: uv.ModCtrl}, true},
		{"ctrl+z", tea.KeyMsg{Type: tea.KeyCtrlZ}, uv.KeyPressEvent{Code: 'z', Mod: uv.ModCtrl}, true},
		{"ctrl+]", tea.KeyMsg{Type: tea.KeyCtrlCloseBracket}, uv.KeyPressEvent{Code: ']', Mod: uv.ModCtrl}, true},
		{"ctrl+@", tea.KeyMsg{Type: tea.KeyCtrlAt}, uv.KeyPressEvent{Code: '@', Mod: uv.ModCtrl}, true},
		{"ctrl+\\", tea.KeyMsg{Type: tea.KeyCtrlBackslash}, uv.KeyPressEvent{Code: '\\', Mod: uv.ModCtrl}, true},
		{"ctrl+^", tea.KeyMsg{Type: tea.KeyCtrlCaret}, uv.KeyPressEvent{Code: '^', Mod: uv.ModCtrl}, true},
		{"ctrl+_", tea.KeyMsg{Type: tea.KeyCtrlUnderscore}, uv.KeyPressEvent{Code: '_', Mod: uv.ModCtrl}, true},
		// HIGH bug fix: ctrl+home was misclassified as ctrl+h (backspace).
		{"ctrl+home", tea.KeyMsg{Type: tea.KeyCtrlHome}, uv.KeyPressEvent{Code: uv.KeyHome, Mod: uv.ModCtrl}, true},
		{"ctrl+end", tea.KeyMsg{Type: tea.KeyCtrlEnd}, uv.KeyPressEvent{Code: uv.KeyEnd, Mod: uv.ModCtrl}, true},
		{"ctrl+pgup", tea.KeyMsg{Type: tea.KeyCtrlPgUp}, uv.KeyPressEvent{Code: uv.KeyPgUp, Mod: uv.ModCtrl}, true},
		{"shift+home", tea.KeyMsg{Type: tea.KeyShiftHome}, uv.KeyPressEvent{Code: uv.KeyHome, Mod: uv.ModShift}, true},
		{"ctrl+shift+up", tea.KeyMsg{Type: tea.KeyCtrlShiftUp}, uv.KeyPressEvent{Code: uv.KeyUp, Mod: uv.ModCtrl | uv.ModShift}, true},
		// alt+ctrl combo via Alt: true on a Ctrl* type
		{"alt+ctrl+a", tea.KeyMsg{Type: tea.KeyCtrlA, Alt: true}, uv.KeyPressEvent{Code: 'a', Mod: uv.ModAlt | uv.ModCtrl}, true},
		{"space", tea.KeyMsg{Type: tea.KeySpace}, uv.KeyPressEvent{Code: uv.KeySpace, Text: " "}, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, ok := MsgToKey(tt.in)
			if ok != tt.ok {
				t.Fatalf("ok = %v, want %v", ok, tt.ok)
			}
			if got != tt.want {
				t.Errorf("MsgToKey(%v) = %+v, want %+v", tt.in, got, tt.want)
			}
		})
	}
}
