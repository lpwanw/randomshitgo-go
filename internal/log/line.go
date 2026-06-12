package log

import "time"

// Line = one logical log line. Bytes may contain ANSI escapes (preserve for
// render); IsPartial=true means the upstream forced a split at maxLineSize or
// at stream end. Rendered caches DecodeForRender(Bytes) — set at ingest so
// the TUI repaint tick doesn't re-decode the whole ring buffer.
type Line struct {
	Bytes     []byte
	Rendered  string
	IsPartial bool
	Timestamp time.Time
}
