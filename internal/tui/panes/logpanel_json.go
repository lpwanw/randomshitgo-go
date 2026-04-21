package panes

import (
	"bytes"
	"encoding/json"
	"strings"

	"github.com/lpwanw/randomshitgo-go/internal/log"
)

// prettifyJSONLine returns ok=true and the list of indented output lines when
// the stripped input is a valid single-line JSON object or array. Otherwise
// returns ok=false — caller keeps the original line.
func prettifyJSONLine(line string) ([]string, bool) {
	stripped := strings.TrimSpace(log.StripANSI(line))
	if len(stripped) < 2 {
		return nil, false
	}
	first, last := stripped[0], stripped[len(stripped)-1]
	if !((first == '{' && last == '}') || (first == '[' && last == ']')) {
		return nil, false
	}
	var buf bytes.Buffer
	if err := json.Indent(&buf, []byte(stripped), "", "  "); err != nil {
		return nil, false
	}
	return strings.Split(buf.String(), "\n"), true
}

// flattenJSONLines expands every JSON-looking line in `lines` into its pretty
// indented form; non-JSON lines pass through. Cursor / filter / yank operate
// on the returned slice — i.e. in JSON-pretty mode, the pretty output IS the
// buffer, so vim motions navigate the rendered form.
func flattenJSONLines(lines []string) []string {
	out := make([]string, 0, len(lines))
	for _, line := range lines {
		if parts, ok := prettifyJSONLine(line); ok {
			out = append(out, parts...)
			continue
		}
		out = append(out, line)
	}
	return out
}
