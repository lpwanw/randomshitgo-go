package log

import "time"

// Line = one logical log line. Bytes may contain ANSI escapes (preserve for
// render); IsPartial=true means the upstream forced a split at maxLineSize or
// at stream end.
type Line struct {
	Bytes     []byte
	IsPartial bool
	Timestamp time.Time
}
