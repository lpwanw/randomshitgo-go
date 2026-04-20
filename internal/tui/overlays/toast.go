package overlays

import (
	"strings"
	"time"

	"github.com/charmbracelet/lipgloss"
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

	styleToastContainer = lipgloss.NewStyle().
				AlignHorizontal(lipgloss.Right)
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

// View renders the toast stack into the bottom-right corner of width×height.
// Returns "" when there are no toasts.
func (ts *ToastStack) View(width, height int) string {
	if len(ts.items) == 0 {
		return ""
	}

	var lines []string
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
		lines = append(lines, s.Render(t.Text))
	}

	stack := strings.Join(lines, "\n")
	return lipgloss.Place(width, height,
		lipgloss.Right, lipgloss.Bottom,
		stack,
	)
}
