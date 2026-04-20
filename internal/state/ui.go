package state

import (
	"fmt"
	"regexp"
	"sync"
	"time"
)

// OverlayKind represents which overlay (modal) the TUI is showing.
type OverlayKind string

const (
	OverlayNone   OverlayKind = ""
	OverlayHelp   OverlayKind = "help"
	OverlayFilter OverlayKind = "filter"
)

// Toast is a brief notification message.
type Toast struct {
	Message  string
	At       time.Time
	Duration time.Duration
}

// UIStore holds transient TUI-level state (selection, filter, scroll, etc.).
// Mutations notify subscribers via the same drop-if-full pattern as RuntimeStore.
type UIStore struct {
	mu sync.RWMutex

	SelectedID   string
	FilterText   string
	FilterRegex  *regexp.Regexp
	FilterErr    error
	LogScroll    int
	StickyBottom bool
	Overlay      OverlayKind
	Toasts       []Toast

	subs []chan struct{}
}

// NewUIStore returns a ready UIStore with sticky-bottom enabled by default.
func NewUIStore() *UIStore {
	return &UIStore{StickyBottom: true}
}

// Subscribe returns a drop-if-full notification channel.
func (u *UIStore) Subscribe() <-chan struct{} {
	ch := make(chan struct{}, 1)
	u.mu.Lock()
	u.subs = append(u.subs, ch)
	u.mu.Unlock()
	return ch
}

// SetSelectedID updates the selected project id.
func (u *UIStore) SetSelectedID(id string) {
	u.mu.Lock()
	u.SelectedID = id
	u.mu.Unlock()
	u.notify()
}

// SetFilter sets the text filter and compiles it as a regex.
// An invalid regex is stored in FilterErr; FilterRegex is set to nil.
func (u *UIStore) SetFilter(text string) error {
	u.mu.Lock()
	u.FilterText = text
	if text == "" {
		u.FilterRegex = nil
		u.FilterErr = nil
	} else {
		re, err := regexp.Compile(text)
		if err != nil {
			u.FilterRegex = nil
			u.FilterErr = fmt.Errorf("invalid filter regex %q: %w", text, err)
			u.mu.Unlock()
			u.notify()
			return u.FilterErr
		}
		u.FilterRegex = re
		u.FilterErr = nil
	}
	u.mu.Unlock()
	u.notify()
	return nil
}

// SetLogScroll updates the log scroll offset (clamped to >= 0).
func (u *UIStore) SetLogScroll(n int) {
	if n < 0 {
		n = 0
	}
	u.mu.Lock()
	u.LogScroll = n
	u.mu.Unlock()
	u.notify()
}

// SetStickyBottom controls whether the log panel auto-scrolls to bottom.
func (u *UIStore) SetStickyBottom(v bool) {
	u.mu.Lock()
	u.StickyBottom = v
	u.mu.Unlock()
	u.notify()
}

// SetOverlay sets the active overlay kind.
func (u *UIStore) SetOverlay(o OverlayKind) {
	u.mu.Lock()
	u.Overlay = o
	u.mu.Unlock()
	u.notify()
}

// PushToast appends a toast notification.
func (u *UIStore) PushToast(msg string, dur time.Duration) {
	u.mu.Lock()
	u.Toasts = append(u.Toasts, Toast{Message: msg, At: time.Now(), Duration: dur})
	u.mu.Unlock()
	u.notify()
}

// PopExpiredToasts removes toasts whose duration has elapsed.
func (u *UIStore) PopExpiredToasts() {
	now := time.Now()
	u.mu.Lock()
	kept := u.Toasts[:0]
	for _, t := range u.Toasts {
		if t.Duration <= 0 || now.Sub(t.At) < t.Duration {
			kept = append(kept, t)
		}
	}
	u.Toasts = kept
	u.mu.Unlock()
}

// Snapshot returns a point-in-time copy of the UI state.
func (u *UIStore) Snapshot() UISnapshot {
	u.mu.RLock()
	defer u.mu.RUnlock()
	toasts := make([]Toast, len(u.Toasts))
	copy(toasts, u.Toasts)
	return UISnapshot{
		SelectedID:   u.SelectedID,
		FilterText:   u.FilterText,
		FilterRegex:  u.FilterRegex,
		LogScroll:    u.LogScroll,
		StickyBottom: u.StickyBottom,
		Overlay:      u.Overlay,
		Toasts:       toasts,
	}
}

// UISnapshot is a value-type copy of UIStore for safe reading outside the lock.
type UISnapshot struct {
	SelectedID   string
	FilterText   string
	FilterRegex  *regexp.Regexp
	LogScroll    int
	StickyBottom bool
	Overlay      OverlayKind
	Toasts       []Toast
}

func (u *UIStore) notify() {
	u.mu.RLock()
	subs := u.subs
	u.mu.RUnlock()
	for _, ch := range subs {
		select {
		case ch <- struct{}{}:
		default:
		}
	}
}
