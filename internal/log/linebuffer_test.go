package log

import (
	"bytes"
	"strings"
	"testing"
)

func feedAll(lb *LineBuffer, chunks ...string) []Line {
	var out []Line
	for _, c := range chunks {
		out = append(out, lb.Feed([]byte(c))...)
	}
	return out
}

func TestLineBufferSimpleLF(t *testing.T) {
	lb := NewLineBuffer(0)
	lines := feedAll(lb, "hello\nworld\n")
	if len(lines) != 2 || string(lines[0].Bytes) != "hello" || string(lines[1].Bytes) != "world" {
		t.Fatalf("got %+v", lines)
	}
	if lines[0].IsPartial {
		t.Fatal("complete line shouldn't be partial")
	}
}

func TestLineBufferCRLF(t *testing.T) {
	lb := NewLineBuffer(0)
	lines := feedAll(lb, "a\r\nb\r\n")
	if len(lines) != 2 || string(lines[0].Bytes) != "a" || string(lines[1].Bytes) != "b" {
		t.Fatalf("got %+v", lines)
	}
}

func TestLineBufferBareCR(t *testing.T) {
	lb := NewLineBuffer(0)
	lines := feedAll(lb, "loading\rdone\n")
	if len(lines) != 1 || string(lines[0].Bytes) != "done" {
		t.Fatalf("bare CR overwrite broken: %+v", lines)
	}
}

func TestLineBufferPartialAcrossWrites(t *testing.T) {
	lb := NewLineBuffer(0)
	if lines := lb.Feed([]byte("hel")); len(lines) != 0 {
		t.Fatalf("premature emit: %+v", lines)
	}
	lines := lb.Feed([]byte("lo\n"))
	if len(lines) != 1 || string(lines[0].Bytes) != "hello" {
		t.Fatalf("got %+v", lines)
	}
}

func TestLineBufferUTF8Boundary(t *testing.T) {
	// 吉 = 0xe5 0x90 0x89 (3-byte UTF-8). Force split must not slice inside.
	lb := NewLineBuffer(5) // tiny cap to trigger forced split on a line
	input := strings.Repeat("吉", 5) + "\n"
	lines := lb.Feed([]byte(input))
	// expect 1+ partial forced split + 1 final complete line
	var forced, complete int
	var joined bytes.Buffer
	for _, l := range lines {
		joined.Write(l.Bytes)
		if l.IsPartial {
			forced++
		} else {
			complete++
		}
	}
	if complete != 1 {
		t.Fatalf("want 1 complete line, got %d (lines=%+v)", complete, lines)
	}
	if forced == 0 {
		t.Fatal("want forced split events")
	}
	// decoded as runes, no replacement char — means no mid-codepoint cut.
	got := joined.String()
	if strings.Contains(got, "\uFFFD") {
		t.Fatalf("mid-codepoint cut produced U+FFFD: %q", got)
	}
	if want := strings.Repeat("吉", 5); got != want {
		t.Fatalf("reassembled: want %q got %q", want, got)
	}
}

func TestLineBufferFlush(t *testing.T) {
	lb := NewLineBuffer(0)
	_ = lb.Feed([]byte("dangling"))
	l := lb.Flush()
	if l == nil || string(l.Bytes) != "dangling" || !l.IsPartial {
		t.Fatalf("flush: %+v", l)
	}
	if l := lb.Flush(); l != nil {
		t.Fatalf("flush twice: %+v", l)
	}
}
