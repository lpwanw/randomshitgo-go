package attach

import (
	tea "github.com/charmbracelet/bubbletea"
	uv "github.com/charmbracelet/ultraviolet"
)

// MsgToKey translates a Bubble Tea KeyMsg into an ultraviolet KeyEvent
// for VTTerm.SendKey. Returns false if the key has no equivalent.
//
// All byte-encoding (DECCKM-aware arrows, F-key sequences, Ctrl-X
// translation, Alt-prefix) is handled inside x/vt — we just supply the
// (Code, Mod) pair.
//
// Switch is exhaustive on bubbletea v1.3.10 KeyType to avoid the
// String()-parsing trap where e.g. KeyCtrlHome would be misclassified
// as ctrl+h (= backspace).
func MsgToKey(msg tea.KeyMsg) (uv.KeyPressEvent, bool) {
	mod := teaModToUV(msg.Alt)

	switch msg.Type {
	case tea.KeyRunes:
		if len(msg.Runes) >= 1 {
			return uv.KeyPressEvent{Code: msg.Runes[0], Text: string(msg.Runes), Mod: mod}, true
		}
		return uv.KeyPressEvent{}, false

	case tea.KeySpace:
		return uv.KeyPressEvent{Code: uv.KeySpace, Text: " ", Mod: mod}, true
	case tea.KeyEnter:
		return uv.KeyPressEvent{Code: uv.KeyEnter, Mod: mod}, true
	case tea.KeyTab:
		return uv.KeyPressEvent{Code: uv.KeyTab, Mod: mod}, true
	case tea.KeyShiftTab:
		return uv.KeyPressEvent{Code: uv.KeyTab, Mod: mod | uv.ModShift}, true
	case tea.KeyBackspace:
		return uv.KeyPressEvent{Code: uv.KeyBackspace, Mod: mod}, true
	case tea.KeyDelete:
		return uv.KeyPressEvent{Code: uv.KeyDelete, Mod: mod}, true
	case tea.KeyEsc:
		return uv.KeyPressEvent{Code: uv.KeyEscape, Mod: mod}, true
	case tea.KeyInsert:
		return uv.KeyPressEvent{Code: uv.KeyInsert, Mod: mod}, true

	// Arrows + nav (no modifier)
	case tea.KeyUp:
		return uv.KeyPressEvent{Code: uv.KeyUp, Mod: mod}, true
	case tea.KeyDown:
		return uv.KeyPressEvent{Code: uv.KeyDown, Mod: mod}, true
	case tea.KeyLeft:
		return uv.KeyPressEvent{Code: uv.KeyLeft, Mod: mod}, true
	case tea.KeyRight:
		return uv.KeyPressEvent{Code: uv.KeyRight, Mod: mod}, true
	case tea.KeyHome:
		return uv.KeyPressEvent{Code: uv.KeyHome, Mod: mod}, true
	case tea.KeyEnd:
		return uv.KeyPressEvent{Code: uv.KeyEnd, Mod: mod}, true
	case tea.KeyPgUp:
		return uv.KeyPressEvent{Code: uv.KeyPgUp, Mod: mod}, true
	case tea.KeyPgDown:
		return uv.KeyPressEvent{Code: uv.KeyPgDown, Mod: mod}, true

	// Shift+nav
	case tea.KeyShiftUp:
		return uv.KeyPressEvent{Code: uv.KeyUp, Mod: mod | uv.ModShift}, true
	case tea.KeyShiftDown:
		return uv.KeyPressEvent{Code: uv.KeyDown, Mod: mod | uv.ModShift}, true
	case tea.KeyShiftLeft:
		return uv.KeyPressEvent{Code: uv.KeyLeft, Mod: mod | uv.ModShift}, true
	case tea.KeyShiftRight:
		return uv.KeyPressEvent{Code: uv.KeyRight, Mod: mod | uv.ModShift}, true
	case tea.KeyShiftHome:
		return uv.KeyPressEvent{Code: uv.KeyHome, Mod: mod | uv.ModShift}, true
	case tea.KeyShiftEnd:
		return uv.KeyPressEvent{Code: uv.KeyEnd, Mod: mod | uv.ModShift}, true

	// Ctrl+nav
	case tea.KeyCtrlUp:
		return uv.KeyPressEvent{Code: uv.KeyUp, Mod: mod | uv.ModCtrl}, true
	case tea.KeyCtrlDown:
		return uv.KeyPressEvent{Code: uv.KeyDown, Mod: mod | uv.ModCtrl}, true
	case tea.KeyCtrlLeft:
		return uv.KeyPressEvent{Code: uv.KeyLeft, Mod: mod | uv.ModCtrl}, true
	case tea.KeyCtrlRight:
		return uv.KeyPressEvent{Code: uv.KeyRight, Mod: mod | uv.ModCtrl}, true
	case tea.KeyCtrlHome:
		return uv.KeyPressEvent{Code: uv.KeyHome, Mod: mod | uv.ModCtrl}, true
	case tea.KeyCtrlEnd:
		return uv.KeyPressEvent{Code: uv.KeyEnd, Mod: mod | uv.ModCtrl}, true
	case tea.KeyCtrlPgUp:
		return uv.KeyPressEvent{Code: uv.KeyPgUp, Mod: mod | uv.ModCtrl}, true
	case tea.KeyCtrlPgDown:
		return uv.KeyPressEvent{Code: uv.KeyPgDown, Mod: mod | uv.ModCtrl}, true

	// Ctrl+Shift+nav
	case tea.KeyCtrlShiftUp:
		return uv.KeyPressEvent{Code: uv.KeyUp, Mod: mod | uv.ModCtrl | uv.ModShift}, true
	case tea.KeyCtrlShiftDown:
		return uv.KeyPressEvent{Code: uv.KeyDown, Mod: mod | uv.ModCtrl | uv.ModShift}, true
	case tea.KeyCtrlShiftLeft:
		return uv.KeyPressEvent{Code: uv.KeyLeft, Mod: mod | uv.ModCtrl | uv.ModShift}, true
	case tea.KeyCtrlShiftRight:
		return uv.KeyPressEvent{Code: uv.KeyRight, Mod: mod | uv.ModCtrl | uv.ModShift}, true
	case tea.KeyCtrlShiftHome:
		return uv.KeyPressEvent{Code: uv.KeyHome, Mod: mod | uv.ModCtrl | uv.ModShift}, true
	case tea.KeyCtrlShiftEnd:
		return uv.KeyPressEvent{Code: uv.KeyEnd, Mod: mod | uv.ModCtrl | uv.ModShift}, true

	// F-keys
	case tea.KeyF1:
		return uv.KeyPressEvent{Code: uv.KeyF1, Mod: mod}, true
	case tea.KeyF2:
		return uv.KeyPressEvent{Code: uv.KeyF2, Mod: mod}, true
	case tea.KeyF3:
		return uv.KeyPressEvent{Code: uv.KeyF3, Mod: mod}, true
	case tea.KeyF4:
		return uv.KeyPressEvent{Code: uv.KeyF4, Mod: mod}, true
	case tea.KeyF5:
		return uv.KeyPressEvent{Code: uv.KeyF5, Mod: mod}, true
	case tea.KeyF6:
		return uv.KeyPressEvent{Code: uv.KeyF6, Mod: mod}, true
	case tea.KeyF7:
		return uv.KeyPressEvent{Code: uv.KeyF7, Mod: mod}, true
	case tea.KeyF8:
		return uv.KeyPressEvent{Code: uv.KeyF8, Mod: mod}, true
	case tea.KeyF9:
		return uv.KeyPressEvent{Code: uv.KeyF9, Mod: mod}, true
	case tea.KeyF10:
		return uv.KeyPressEvent{Code: uv.KeyF10, Mod: mod}, true
	case tea.KeyF11:
		return uv.KeyPressEvent{Code: uv.KeyF11, Mod: mod}, true
	case tea.KeyF12:
		return uv.KeyPressEvent{Code: uv.KeyF12, Mod: mod}, true

	// Ctrl+letter (KeyCtrlA..Z) and Ctrl+special.
	case tea.KeyCtrlAt:
		return uv.KeyPressEvent{Code: '@', Mod: mod | uv.ModCtrl}, true
	case tea.KeyCtrlA:
		return uv.KeyPressEvent{Code: 'a', Mod: mod | uv.ModCtrl}, true
	case tea.KeyCtrlB:
		return uv.KeyPressEvent{Code: 'b', Mod: mod | uv.ModCtrl}, true
	case tea.KeyCtrlC:
		return uv.KeyPressEvent{Code: 'c', Mod: mod | uv.ModCtrl}, true
	case tea.KeyCtrlD:
		return uv.KeyPressEvent{Code: 'd', Mod: mod | uv.ModCtrl}, true
	case tea.KeyCtrlE:
		return uv.KeyPressEvent{Code: 'e', Mod: mod | uv.ModCtrl}, true
	case tea.KeyCtrlF:
		return uv.KeyPressEvent{Code: 'f', Mod: mod | uv.ModCtrl}, true
	case tea.KeyCtrlG:
		return uv.KeyPressEvent{Code: 'g', Mod: mod | uv.ModCtrl}, true
	case tea.KeyCtrlH:
		return uv.KeyPressEvent{Code: 'h', Mod: mod | uv.ModCtrl}, true
	case tea.KeyCtrlJ:
		return uv.KeyPressEvent{Code: 'j', Mod: mod | uv.ModCtrl}, true
	case tea.KeyCtrlK:
		return uv.KeyPressEvent{Code: 'k', Mod: mod | uv.ModCtrl}, true
	case tea.KeyCtrlL:
		return uv.KeyPressEvent{Code: 'l', Mod: mod | uv.ModCtrl}, true
	case tea.KeyCtrlN:
		return uv.KeyPressEvent{Code: 'n', Mod: mod | uv.ModCtrl}, true
	case tea.KeyCtrlO:
		return uv.KeyPressEvent{Code: 'o', Mod: mod | uv.ModCtrl}, true
	case tea.KeyCtrlP:
		return uv.KeyPressEvent{Code: 'p', Mod: mod | uv.ModCtrl}, true
	case tea.KeyCtrlQ:
		return uv.KeyPressEvent{Code: 'q', Mod: mod | uv.ModCtrl}, true
	case tea.KeyCtrlR:
		return uv.KeyPressEvent{Code: 'r', Mod: mod | uv.ModCtrl}, true
	case tea.KeyCtrlS:
		return uv.KeyPressEvent{Code: 's', Mod: mod | uv.ModCtrl}, true
	case tea.KeyCtrlT:
		return uv.KeyPressEvent{Code: 't', Mod: mod | uv.ModCtrl}, true
	case tea.KeyCtrlU:
		return uv.KeyPressEvent{Code: 'u', Mod: mod | uv.ModCtrl}, true
	case tea.KeyCtrlV:
		return uv.KeyPressEvent{Code: 'v', Mod: mod | uv.ModCtrl}, true
	case tea.KeyCtrlW:
		return uv.KeyPressEvent{Code: 'w', Mod: mod | uv.ModCtrl}, true
	case tea.KeyCtrlX:
		return uv.KeyPressEvent{Code: 'x', Mod: mod | uv.ModCtrl}, true
	case tea.KeyCtrlY:
		return uv.KeyPressEvent{Code: 'y', Mod: mod | uv.ModCtrl}, true
	case tea.KeyCtrlZ:
		return uv.KeyPressEvent{Code: 'z', Mod: mod | uv.ModCtrl}, true
	case tea.KeyCtrlBackslash:
		return uv.KeyPressEvent{Code: '\\', Mod: mod | uv.ModCtrl}, true
	case tea.KeyCtrlCloseBracket:
		return uv.KeyPressEvent{Code: ']', Mod: mod | uv.ModCtrl}, true
	case tea.KeyCtrlCaret:
		return uv.KeyPressEvent{Code: '^', Mod: mod | uv.ModCtrl}, true
	case tea.KeyCtrlUnderscore:
		return uv.KeyPressEvent{Code: '_', Mod: mod | uv.ModCtrl}, true
	// Aliases (bubbletea collapses these onto the byte value):
	//   KeyCtrlI == KeyTab           (HT)         → handled by KeyTab case
	//   KeyCtrlM == KeyEnter         (CR)         → handled by KeyEnter case
	//   KeyCtrlOpenBracket == KeyEsc (ESC)        → handled by KeyEsc case
	//   KeyCtrlQuestionMark == KeyBackspace (DEL) → handled by KeyBackspace case
	// In all four cases we send the named key without forcing Ctrl mod;
	// terminal apps universally expect the raw byte.
	}

	return uv.KeyPressEvent{}, false
}

func teaModToUV(alt bool) uv.KeyMod {
	var m uv.KeyMod
	if alt {
		m |= uv.ModAlt
	}
	return m
}
