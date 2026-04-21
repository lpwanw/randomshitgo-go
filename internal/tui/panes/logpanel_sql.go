package panes

import (
	"regexp"
	"strings"

	"github.com/lpwanw/randomshitgo-go/internal/log"
)

// sqlStartRe detects whether (post-trim) a line begins with a SQL verb —
// case-insensitive, word-boundary. The ORM-prefix path strips the prefix
// first before matching.
var sqlStartRe = regexp.MustCompile(`(?i)^\s*(?:SELECT|INSERT|UPDATE|DELETE|WITH|BEGIN|COMMIT|ROLLBACK|ALTER|CREATE|DROP|TRUNCATE|EXPLAIN)\b`)

// ormPrefixRe matches the timing tag common to Rails, Sequel, and GORM log
// formats. The regex ends at the closing bracket/paren; everything after is
// the candidate SQL payload.
//
//	`User Load (0.8ms)  SELECT …`       → Rails
//	`(0.001s) SELECT …`                  → Sequel
//	`[0.352ms] [rows:1] SELECT …`        → GORM (first bracket match)
//	`TRANSACTION (0.2ms)  BEGIN`         → Rails transaction
var ormPrefixRe = regexp.MustCompile(`[\(\[]\s*\d+(?:\.\d+)?\s*(?:ms|s)\s*[\)\]]`)

// extraBracketRe matches a single `[…]` or `(…)` tag (no nesting) — used to
// peel auxiliary info like GORM's `[rows:1]` that sits between the timing
// tag and the actual SQL verb.
var extraBracketRe = regexp.MustCompile(`^\s*[\(\[][^\)\]]*[\)\]]`)

// sqlBlueOpen / sqlBlueClose wrap every pretty-printed SQL row so SQL jumps
// out of surrounding log traffic. Uses xterm-256 azure (38;5;39) — visibly
// "blue" on dark terminals and distinct from Rails' own magenta log colour.
// Close resets only the foreground so severity / selection / filter overlays
// can still paint on top.
const (
	sqlBlueOpen  = "\x1b[38;5;39m"
	sqlBlueClose = "\x1b[39m"
)

// sqlTopKeywords trigger a new line with no indent. Order matters when one is
// a prefix of another (e.g. "GROUP BY" before "GROUP") — we match
// longest-first by putting multi-word forms up front.
var sqlTopKeywords = []string{
	"DELETE FROM",
	"INSERT INTO",
	"GROUP BY",
	"ORDER BY",
	"SELECT",
	"FROM",
	"WHERE",
	"HAVING",
	"LIMIT",
	"OFFSET",
	"UNION ALL",
	"UNION",
	"VALUES",
	"UPDATE",
	"SET",
	"RETURNING",
	"ON CONFLICT",
}

// sqlSubKeywords get a 2-space indent on their new line. JOIN family and
// AND/OR/ON read as clauses hanging off a top-keyword — indent makes the
// structure pop.
var sqlSubKeywords = []string{
	"LEFT OUTER JOIN",
	"RIGHT OUTER JOIN",
	"FULL OUTER JOIN",
	"INNER JOIN",
	"LEFT JOIN",
	"RIGHT JOIN",
	"FULL JOIN",
	"CROSS JOIN",
	"JOIN",
	"AND",
	"OR",
	"ON",
}

// prettifySQL reformats a single log line that contains SQL into a slice of
// lines with keyword-based breaks. Returns ok=false when the line is not SQL;
// caller keeps the original line. Preserves original case — we only insert
// newlines/indents, never re-casing tokens.
//
// Rails / Sequel / GORM-style prefixes (timing tags) are detected via
// ormPrefixRe; the prefix stays on its own line and the trailing SQL is
// formatted on subsequent lines.
func prettifySQL(line string) ([]string, bool) {
	stripped := log.StripANSI(line)
	if prefix, sql, ok := splitORMPrefix(stripped); ok {
		rows := append([]string{prefix}, formatSQL(sql)...)
		return tintBlue(rows), true
	}
	if !sqlStartRe.MatchString(stripped) {
		return nil, false
	}
	return tintBlue(formatSQL(stripped)), true
}

// tintBlue wraps each row with the SQL-row blue SGR so pretty-printed SQL is
// visually distinct from the surrounding log traffic.
func tintBlue(rows []string) []string {
	for i, r := range rows {
		rows[i] = sqlBlueOpen + r + sqlBlueClose
	}
	return rows
}

// splitORMPrefix pulls the timing-tag prefix off an ORM-style log line. The
// trailing portion must parse as SQL (leading verb) for the split to be
// accepted. When multiple bracketed tags appear (GORM's `[Nms] [rows:N] SELECT`),
// we scan all matches and pick the last one whose tail starts with a SQL verb.
func splitORMPrefix(line string) (prefix, sql string, ok bool) {
	matches := ormPrefixRe.FindAllStringIndex(line, -1)
	if len(matches) == 0 {
		return "", line, false
	}
	for i := len(matches) - 1; i >= 0; i-- {
		m := matches[i]
		rest := strings.TrimSpace(line[m[1]:])
		// Peel auxiliary bracket tags (e.g. GORM's `[rows:1]`) so the SQL-verb
		// check hits the first actual keyword.
		for {
			loc := extraBracketRe.FindStringIndex(rest)
			if loc == nil {
				break
			}
			rest = strings.TrimSpace(rest[loc[1]:])
		}
		if sqlStartRe.MatchString(rest) {
			// Keep the full original prefix (including any peeled auxiliary
			// tags) on the prefix line so nothing is lost visually.
			return strings.TrimRight(line[:len(line)-len(rest)], " \t"), rest, true
		}
	}
	return "", line, false
}

// formatSQL is the core single-pass formatter. Quote-aware (single, double,
// backtick) and paren-depth-aware (don't split inside subqueries).
func formatSQL(s string) []string {
	var rows []string
	var cur strings.Builder

	emit := func(indent int) {
		trimmed := strings.TrimRight(cur.String(), " \t")
		trimmed = strings.TrimLeft(trimmed, " \t")
		if trimmed == "" {
			return
		}
		if indent > 0 {
			rows = append(rows, strings.Repeat(" ", indent)+trimmed)
		} else {
			rows = append(rows, trimmed)
		}
		cur.Reset()
	}

	quote := byte(0)   // 0 = not in quote, else the quote char
	parenDepth := 0
	lastIndent := 0
	pendingIndent := 0

	i := 0
	n := len(s)
	for i < n {
		c := s[i]

		// Inside a quoted literal — consume until matching close. Doubled
		// quote = escape (common SQL convention).
		if quote != 0 {
			cur.WriteByte(c)
			if c == quote {
				if i+1 < n && s[i+1] == quote {
					cur.WriteByte(quote)
					i += 2
					continue
				}
				quote = 0
			}
			i++
			continue
		}

		// Enter a quote.
		if c == '\'' || c == '"' || c == '`' {
			quote = c
			cur.WriteByte(c)
			i++
			continue
		}

		// Track paren depth; no splitting while depth > 0 (subquery stays inline).
		if c == '(' {
			parenDepth++
			cur.WriteByte(c)
			i++
			continue
		}
		if c == ')' {
			if parenDepth > 0 {
				parenDepth--
			}
			cur.WriteByte(c)
			i++
			continue
		}

		if parenDepth == 0 && isKeywordBoundary(s, i) {
			if hit, isSub := matchKeyword(s, i); hit != "" {
				// Flush the chunk accumulated so far with its own indent tier.
				emit(lastIndent)
				if isSub {
					lastIndent = 2
				} else {
					lastIndent = 0
				}
				// Advance cur with the keyword text so the new chunk begins
				// with the keyword. pendingIndent tracks whether to prepend
				// spaces at flush time.
				cur.WriteString(s[i : i+len(hit)])
				i += len(hit)
				pendingIndent = lastIndent
				_ = pendingIndent
				continue
			}
		}

		cur.WriteByte(c)
		i++
	}
	emit(lastIndent)
	if len(rows) == 0 {
		return []string{strings.TrimSpace(s)}
	}
	return rows
}

// isKeywordBoundary returns true when s[i] is at a word boundary — i.e. the
// previous byte is non-word (space, paren, start-of-string) so we don't match
// `SELECT` inside `SELECTION`.
func isKeywordBoundary(s string, i int) bool {
	if i == 0 {
		return true
	}
	prev := s[i-1]
	return !isWordByte(prev)
}

// matchKeyword checks whether any known keyword starts at s[i] (case-insensitive
// with a trailing word-boundary). Returns the matched substring (from s, preserving
// case) and whether it's a sub-keyword (indented). Longest-match first.
func matchKeyword(s string, i int) (string, bool) {
	if m := matchAny(s, i, sqlTopKeywords); m != "" {
		return m, false
	}
	if m := matchAny(s, i, sqlSubKeywords); m != "" {
		return m, true
	}
	return "", false
}

func matchAny(s string, i int, keywords []string) string {
	for _, kw := range keywords {
		if len(kw) > len(s)-i {
			continue
		}
		cand := s[i : i+len(kw)]
		if !strings.EqualFold(cand, kw) {
			continue
		}
		// Verify trailing boundary — the char right after the match must not
		// be a word byte (letter/digit/_).
		if end := i + len(kw); end < len(s) && isWordByte(s[end]) {
			continue
		}
		return cand
	}
	return ""
}

// isWordByte — mirrors vim's `\w` class used elsewhere in the file.
func isWordByte(b byte) bool { return isWord(b) }

// flattenSQLLines expands every SQL-looking line in `lines` through
// prettifySQL; non-SQL lines pass through. Mirrors the shape of
// flattenJSONLines so the toggle architecture stays uniform.
func flattenSQLLines(lines []string) []string {
	out := make([]string, 0, len(lines))
	for _, line := range lines {
		if parts, ok := prettifySQL(line); ok {
			out = append(out, parts...)
			continue
		}
		out = append(out, line)
	}
	return out
}
