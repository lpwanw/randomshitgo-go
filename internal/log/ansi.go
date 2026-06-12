package log

import (
	"bytes"
	"regexp"
	"strings"
)

// Full ANSI SGR + CSI sequence matcher — strips color, cursor-move, erase, etc.
// Covers ESC[ <params> <final-byte> where final is any byte in 0x40..0x7E.
var ansiSeq = regexp.MustCompile(`\x1b\[[0-9;?]*[ -/]*[@-~]|\x1b\][^\x07]*\x07|\x1b[@-_]`)

// StripANSI removes ANSI escape sequences. Used for regex match input.
// Lines without an ESC byte (the common case for log output) are returned
// as-is without invoking the regex engine.
func StripANSI(s string) string {
	if !strings.ContainsRune(s, 0x1b) {
		return s
	}
	return ansiSeq.ReplaceAllString(s, "")
}

// cursor move / erase / save-restore escapes that would corrupt viewport layout.
// Colour/attribute SGR (ESC[ … m) is intentionally left intact.
var cursorMoveSeq = regexp.MustCompile(`\x1b\[[\d;]*[ABCDEFGHJKSTfhlnsu]`)

// DecodeForRender strips layout-corrupting escapes but keeps colour SGR codes.
// Mirrors ansi-render.ts. ESC-free input skips the regex engine entirely.
func DecodeForRender(b []byte) string {
	if !bytes.ContainsRune(b, 0x1b) {
		return string(b)
	}
	return cursorMoveSeq.ReplaceAllString(string(b), "")
}
