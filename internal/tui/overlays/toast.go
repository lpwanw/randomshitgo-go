package overlays

import (
	"strings"
	"time"

	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"
)

const (
	ToastInfo = 0
	ToastWarn = 1
	ToastErr  = 2

	maxToasts       = 5
	defaultToastTTL = 3 * time.Second
)

var (
	styleToastInfo = lipgloss.NewStyle().
			Background(lipgloss.Color("22")).
			Foreground(lipgloss.Color("255")).
			Padding(0, 1)

	styleToastWarn = lipgloss.NewStyle().
			Background(lipgloss.Color("130")).
			Foreground(lipgloss.Color("255")).
			Padding(0, 1)

	styleToastErr = lipgloss.NewStyle().
			Background(lipgloss.Color("160")).
			Foreground(lipgloss.Color("255")).
			Padding(0, 1)
)

// Toast is a single notification message.
type Toast struct {
	Text      string
	Level     int
	ExpiresAt time.Time
}

// ToastStack manages a bounded stack of Toast notifications.
type ToastStack struct {
	items []Toast
}

// Add appends a new toast, dropping oldest if the stack exceeds maxToasts.
func (ts *ToastStack) Add(text string, level int) {
	t := Toast{
		Text:      text,
		Level:     level,
		ExpiresAt: time.Now().Add(defaultToastTTL),
	}
	ts.items = append(ts.items, t)
	if len(ts.items) > maxToasts {
		ts.items = ts.items[len(ts.items)-maxToasts:]
	}
}

// AddWithTTL appends a toast with a custom TTL.
func (ts *ToastStack) AddWithTTL(text string, level int, ttl time.Duration) {
	t := Toast{Text: text, Level: level, ExpiresAt: time.Now().Add(ttl)}
	ts.items = append(ts.items, t)
	if len(ts.items) > maxToasts {
		ts.items = ts.items[len(ts.items)-maxToasts:]
	}
}

// Prune removes toasts that have expired relative to now.
func (ts *ToastStack) Prune(now time.Time) {
	kept := ts.items[:0]
	for _, t := range ts.items {
		if t.ExpiresAt.After(now) {
			kept = append(kept, t)
		}
	}
	ts.items = kept
}

// Len returns the number of active toasts.
func (ts *ToastStack) Len() int { return len(ts.items) }

// Last returns the most recently added toast and true, or an empty Toast and
// false if the stack is empty.
func (ts *ToastStack) Last() (Toast, bool) {
	if len(ts.items) == 0 {
		return Toast{}, false
	}
	return ts.items[len(ts.items)-1], true
}

// View renders the toast stack as a compact block sized to its content.
// Returns "" when there are no toasts. The caller is responsible for placing
// the block onto the main canvas (see tui.overlayBottomRight) — this function
// MUST NOT pad to width×height, because that would blank out the main UI
// when composed via string concatenation.
func (ts *ToastStack) View(width, _ int) string {
	if len(ts.items) == 0 {
		return ""
	}

	maxW := width
	if maxW <= 0 {
		maxW = 80
	}

	lines := make([]string, 0, len(ts.items))
	for _, t := range ts.items {
		var s lipgloss.Style
		switch t.Level {
		case ToastWarn:
			s = styleToastWarn
		case ToastErr:
			s = styleToastErr
		default:
			s = styleToastInfo
		}
		rendered := s.Render(t.Text)
		if ansi.StringWidth(rendered) > maxW {
			rendered = ansi.Truncate(rendered, maxW, "…")
		}
		lines = append(lines, rendered)
	}
	return strings.Join(lines, "\n")
}
