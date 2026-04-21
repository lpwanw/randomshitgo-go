package panes

import (
	"regexp"

	"github.com/lpwanw/randomshitgo-go/internal/log"
)

// severityRe tags the first severity token on a log line. Word boundaries
// keep it from firing inside identifiers like `errorHandler`. Kept
// case-sensitive on purpose — lowercase `error` in prose would drown the
// signal.
var severityRe = regexp.MustCompile(`\b(ERROR|ERR|FATAL|WARN|WARNING|INFO|DEBUG|TRACE)\b`)

// severityFG maps a matched token to an 8-bit xterm colour fg SGR. Only fg is
// set — bg would clash with filter-match reverse-video and selection overlays.
var severityFG = map[string]string{
	"ERROR":   "\x1b[38;5;196m",
	"ERR":     "\x1b[38;5;196m",
	"FATAL":   "\x1b[38;5;196m",
	"WARN":    "\x1b[38;5;214m",
	"WARNING": "\x1b[38;5;214m",
	"INFO":    "\x1b[38;5;40m",
	"DEBUG":   "\x1b[38;5;240m",
	"TRACE":   "\x1b[38;5;240m",
}

// severityClose resets only the foreground so other SGR layers (selection,
// filter highlight) survive.
const severityClose = "\x1b[39m"

// applySeverity wraps `line` with a severity-coloured fg when the stripped
// content contains a known level token. Returns the line unchanged when no
// match is found.
func applySeverity(line string) string {
	stripped := log.StripANSI(line)
	m := severityRe.FindStringSubmatchIndex(stripped)
	if m == nil {
		return line
	}
	tag := stripped[m[2]:m[3]]
	open, ok := severityFG[tag]
	if !ok {
		return line
	}
	return open + line + severityClose
}

