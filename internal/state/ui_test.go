package state

import (
	"testing"
	"time"
)

func TestUIStore_SetFilter_ValidRegex(t *testing.T) {
	u := NewUIStore()
	if err := u.SetFilter("err.*"); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	snap := u.Snapshot()
	if snap.FilterRegex == nil {
		t.Fatal("FilterRegex should be set")
	}
	if snap.FilterText != "err.*" {
		t.Fatalf("expected filter text 'err.*', got %q", snap.FilterText)
	}
}

func TestUIStore_SetFilter_InvalidRegex(t *testing.T) {
	u := NewUIStore()
	err := u.SetFilter("[invalid")
	if err == nil {
		t.Fatal("expected error for invalid regex")
	}
	snap := u.Snapshot()
	if snap.FilterRegex != nil {
		t.Fatal("FilterRegex should be nil for invalid pattern")
	}
}

func TestUIStore_SetFilter_EmptyClears(t *testing.T) {
	u := NewUIStore()
	_ = u.SetFilter("foo")
	_ = u.SetFilter("")
	snap := u.Snapshot()
	if snap.FilterRegex != nil {
		t.Fatal("FilterRegex should be nil after clearing filter")
	}
}

func TestUIStore_SetSelectedID_Notifies(t *testing.T) {
	u := NewUIStore()
	ch := u.Subscribe()

	u.SetSelectedID("web")

	select {
	case <-ch:
		// ok
	case <-time.After(100 * time.Millisecond):
		t.Fatal("expected notification within 100ms")
	}
	if u.Snapshot().SelectedID != "web" {
		t.Fatalf("expected SelectedID 'web'")
	}
}

func TestUIStore_SetLogScroll_ClampsNegative(t *testing.T) {
	u := NewUIStore()
	u.SetLogScroll(-5)
	if u.Snapshot().LogScroll != 0 {
		t.Fatalf("expected clamped to 0")
	}
	u.SetLogScroll(10)
	if u.Snapshot().LogScroll != 10 {
		t.Fatalf("expected 10")
	}
}

func TestUIStore_PushAndPopToasts(t *testing.T) {
	u := NewUIStore()
	u.PushToast("hello", 50*time.Millisecond)
	if len(u.Snapshot().Toasts) != 1 {
		t.Fatal("expected 1 toast")
	}
	time.Sleep(80 * time.Millisecond)
	u.PopExpiredToasts()
	if len(u.Snapshot().Toasts) != 0 {
		t.Fatal("expected toast to be expired and removed")
	}
}

func TestUIStore_SetOverlay(t *testing.T) {
	u := NewUIStore()
	u.SetOverlay(OverlayHelp)
	if u.Snapshot().Overlay != OverlayHelp {
		t.Fatalf("expected OverlayHelp")
	}
}

func TestUIStore_Subscribe_DropIfFull(t *testing.T) {
	u := NewUIStore()
	ch := u.Subscribe()

	// Fire two rapid mutations without draining — no deadlock/panic expected.
	u.SetSelectedID("a")
	u.SetSelectedID("b")

	select {
	case <-ch:
	case <-time.After(100 * time.Millisecond):
		t.Fatal("expected notification")
	}
}
