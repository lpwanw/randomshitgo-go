package log

import "time"

const (
	lf byte = 0x0a
	cr byte = 0x0d

	defaultMaxLineSize = 64 * 1024
)

// LineBuffer splits a byte stream into lines.
//
// Rules (mirrors src/log/line-buffer.ts):
//   - LF ends a line.
//   - CRLF collapses to a single line terminator.
//   - Bare CR overwrites: accumulated prefix since line start is dropped.
//   - Lines longer than MaxLineSize are force-split on a safe UTF-8 boundary
//     and emitted with IsPartial=true.
type LineBuffer struct {
	buf         []byte
	MaxLineSize int
	now         func() time.Time
}

func NewLineBuffer(maxLineSize int) *LineBuffer {
	if maxLineSize <= 0 {
		maxLineSize = defaultMaxLineSize
	}
	return &LineBuffer{MaxLineSize: maxLineSize, now: time.Now}
}

// safeSplitOffset returns an offset ≤ cap that does not land inside a
// multi-byte UTF-8 codepoint. Backtracks up to 3 continuation bytes.
func safeSplitOffset(buf []byte, cap, floor int) int {
	off := cap
	for n := 0; n < 3 && off > floor; n++ {
		if off >= len(buf) {
			break
		}
		b := buf[off]
		if (b & 0xC0) != 0x80 { // not a continuation byte
			break
		}
		off--
	}
	if off == floor {
		return cap
	}
	return off
}

// Feed appends chunk and returns any newly-completed lines. Incomplete tail is
// retained for the next Feed call.
func (lb *LineBuffer) Feed(chunk []byte) []Line {
	if len(chunk) == 0 {
		return nil
	}
	combined := concat(lb.buf, chunk)
	var lines []Line
	lineStart := 0
	i := 0
	now := lb.now()

	for i < len(combined) {
		b := combined[i]

		switch b {
		case cr:
			if i+1 < len(combined) && combined[i+1] == lf {
				lines = append(lines, Line{Bytes: dup(combined[lineStart:i]), Timestamp: now})
				i += 2
				lineStart = i
				continue
			}
			// Bare CR: overwrite — drop accumulated prefix, resume after the CR.
			lineStart = i + 1
			i++
			continue
		case lf:
			lines = append(lines, Line{Bytes: dup(combined[lineStart:i]), Timestamp: now})
			i++
			lineStart = i
			continue
		}

		if i-lineStart >= lb.MaxLineSize {
			cap := lineStart + lb.MaxLineSize
			safe := safeSplitOffset(combined, cap, lineStart)
			lines = append(lines, Line{Bytes: dup(combined[lineStart:safe]), IsPartial: true, Timestamp: now})
			lineStart = safe
		}
		i++
	}
	lb.buf = dup(combined[lineStart:])
	return lines
}

// Flush emits any trailing partial line and clears state.
func (lb *LineBuffer) Flush() *Line {
	if len(lb.buf) == 0 {
		return nil
	}
	out := Line{Bytes: lb.buf, IsPartial: true, Timestamp: lb.now()}
	lb.buf = nil
	return &out
}

func concat(a, b []byte) []byte {
	if len(a) == 0 {
		return append([]byte(nil), b...)
	}
	if len(b) == 0 {
		return append([]byte(nil), a...)
	}
	out := make([]byte, len(a)+len(b))
	copy(out, a)
	copy(out[len(a):], b)
	return out
}

func dup(b []byte) []byte {
	if len(b) == 0 {
		return nil
	}
	out := make([]byte, len(b))
	copy(out, b)
	return out
}
