package attach

import (
	"testing"
	"time"
)

// fixedNow is a stable base time used in all table-driven tests.
var fixedNow = time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)

// within returns a time within the 400ms window.
func within(base time.Time, ms int) time.Time {
	return base.Add(time.Duration(ms) * time.Millisecond)
}

// after returns a time past the 400ms window.
func after(base time.Time, ms int) time.Time {
	return base.Add(time.Duration(ms) * time.Millisecond)
}

func TestEscapeDetector_ForwardsNonEscapeBytes(t *testing.T) {
	d := NewEscapeDetector(DefaultEscapeByte, DefaultTimeout)
	pass, detach := d.FeedChunk([]byte{0x61, 0x62, 0x63}, fixedNow)
	if detach {
		t.Fatal("expected no detach")
	}
	if string(pass) != "abc" {
		t.Fatalf("expected abc, got %v", pass)
	}
}

func TestEscapeDetector_SwallowsSingleEscape(t *testing.T) {
	d := NewEscapeDetector(DefaultEscapeByte, DefaultTimeout)
	pass, detach := d.FeedChunk([]byte{0x1b}, fixedNow)
	if detach {
		t.Fatal("expected no detach")
	}
	if len(pass) != 0 {
		t.Fatalf("expected empty passthrough, got %v", pass)
	}
}

func TestEscapeDetector_DetachOnTwoConsecutiveInOneChunk(t *testing.T) {
	d := NewEscapeDetector(DefaultEscapeByte, DefaultTimeout)
	_, detach := d.FeedChunk([]byte{0x1b, 0x1b}, fixedNow)
	if !detach {
		t.Fatal("expected detach on two consecutive escape bytes")
	}
}

func TestEscapeDetector_DetachAcrossTwoChunksWithinWindow(t *testing.T) {
	d := NewEscapeDetector(DefaultEscapeByte, DefaultTimeout)
	t0 := fixedNow
	_, det1 := d.FeedChunk([]byte{0x1b}, t0)
	if det1 {
		t.Fatal("first chunk: expected no detach")
	}
	// Second chunk arrives 100ms later — within the 400ms window.
	t1 := within(t0, 100)
	_, det2 := d.FeedChunk([]byte{0x1b}, t1)
	if !det2 {
		t.Fatal("second chunk: expected detach within window")
	}
}

func TestEscapeDetector_NoDetachIfNonEscapeBetween(t *testing.T) {
	d := NewEscapeDetector(DefaultEscapeByte, DefaultTimeout)
	t0 := fixedNow
	d.FeedChunk([]byte{0x1b}, t0)                    // first escape
	d.FeedChunk([]byte{0x61}, within(t0, 50))        // non-escape resets window
	_, detach := d.FeedChunk([]byte{0x1b}, within(t0, 100)) // new first escape
	if detach {
		t.Fatal("expected no detach after reset by non-escape byte")
	}
}

func TestEscapeDetector_ResetsWindowAfterTimeout(t *testing.T) {
	d := NewEscapeDetector(DefaultEscapeByte, 20*time.Millisecond)
	t0 := fixedNow
	d.FeedChunk([]byte{0x1b}, t0)
	// Second press arrives AFTER the 20ms timeout — should not detach.
	t1 := after(t0, 50)
	_, detach := d.FeedChunk([]byte{0x1b}, t1)
	if detach {
		t.Fatal("expected no detach after timeout expired")
	}
}

func TestEscapeDetector_DropsBytesAfterMidChunkEscapePair(t *testing.T) {
	d := NewEscapeDetector(DefaultEscapeByte, DefaultTimeout)
	// Mirrors TS test: "drops bytes after a mid-paste escape pair".
	pass, detach := d.FeedChunk([]byte{0x61, 0x62, 0x1b, 0x1b, 0x63, 0x64}, fixedNow)
	if !detach {
		t.Fatal("expected detach")
	}
	if string(pass) != "ab" {
		t.Fatalf("expected ab, got %q", pass)
	}
}

func TestEscapeDetector_CustomEscapeByte(t *testing.T) {
	d := NewEscapeDetector(0x1c, DefaultTimeout)
	// Default 0x1b should NOT trigger detach with custom escape byte.
	_, det1 := d.FeedChunk([]byte{0x1b, 0x1b}, fixedNow)
	if det1 {
		t.Fatal("0x1b should not detach when escape byte is 0x1c")
	}
	// 0x1c 0x1c should detach.
	d2 := NewEscapeDetector(0x1c, DefaultTimeout)
	_, det2 := d2.FeedChunk([]byte{0x1c, 0x1c}, fixedNow)
	if !det2 {
		t.Fatal("0x1c 0x1c should detach with escape byte 0x1c")
	}
}

func TestEscapeDetector_TwoEscapesBeyondTimeoutIsNotDetach(t *testing.T) {
	d := NewEscapeDetector(DefaultEscapeByte, DefaultTimeout)
	t0 := fixedNow
	d.FeedChunk([]byte{0x1b}, t0)
	// >400ms later — window expired; second press starts a fresh window.
	t1 := after(t0, 500)
	_, detach := d.FeedChunk([]byte{0x1b}, t1)
	if detach {
		t.Fatal("expected no detach when gap > timeout")
	}
}

// TestEscapeDetector_FeedSingleByte exercises the Feed method directly.
func TestEscapeDetector_FeedSingleByte(t *testing.T) {
	d := NewEscapeDetector(DefaultEscapeByte, DefaultTimeout)
	t0 := fixedNow

	pass, det := d.Feed(0x1b, t0)
	if det || len(pass) != 0 {
		t.Fatal("first escape: expect swallow, no detach")
	}

	pass2, det2 := d.Feed(0x1b, within(t0, 100))
	if !det2 || len(pass2) != 0 {
		t.Fatal("second escape within window: expect detach, no passthrough")
	}
}
