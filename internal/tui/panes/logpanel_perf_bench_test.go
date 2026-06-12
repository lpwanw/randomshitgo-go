package panes

import (
	"fmt"
	"regexp"
	"strings"
	"testing"
)

// benchLines builds a realistic log buffer: mostly plain ASCII lines, some
// long, a few with embedded SGR colour.
func benchLines(n int) []string {
	lines := make([]string, n)
	for i := range lines {
		switch i % 10 {
		case 0:
			lines[i] = fmt.Sprintf("\x1b[32mINFO\x1b[0m request id=%d path=/api/v1/items status=200 dur=12ms", i)
		case 1:
			lines[i] = fmt.Sprintf("ERROR upstream timeout id=%d retrying with backoff attempt=3 %s", i, strings.Repeat("ctx=abcdef ", 30))
		default:
			lines[i] = fmt.Sprintf("worker=%d processed batch of 128 events in 4.2ms queue_depth=7", i)
		}
	}
	return lines
}

func BenchmarkWrapLineLongPlain(b *testing.B) {
	line := strings.Repeat("abcdef ghij ", 50) // 600 visible bytes, no ANSI
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		wrapLine(line, 80)
	}
}

func BenchmarkWrapLineLongANSI(b *testing.B) {
	line := "\x1b[31m" + strings.Repeat("abcdef \x1b[32mghij\x1b[0m ", 40)
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		wrapLine(line, 80)
	}
}

func BenchmarkApplySeverity(b *testing.B) {
	line := "2026-01-02 15:04:05 ERROR something went wrong in the handler path=/x"
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		applySeverity(line)
	}
}

func BenchmarkHighlightLine(b *testing.B) {
	rx := regexp.MustCompile("ERROR")
	line := "\x1b[31mERROR\x1b[0m upstream timeout " + strings.Repeat("ctx=abcdef ", 30)
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		highlightLine(line, rx, false)
	}
}

// BenchmarkSetLines measures the full per-tick repaint path: severity pass +
// wrap + gutterless flatten over a 2000-line buffer.
func BenchmarkSetLines(b *testing.B) {
	lines := benchLines(2000)
	lp := NewLogPanel(120, 40)
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		lp.SetLines(lines)
	}
}

// BenchmarkLogPanelView measures the per-frame render: invoked on every
// Bubble Tea update, so it must stay O(visible window), not O(buffer).
func BenchmarkLogPanelView(b *testing.B) {
	lines := benchLines(2000)
	lp := NewLogPanel(120, 40)
	lp.SetLines(lines)
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		lp.View()
	}
}

// BenchmarkSetLinesFiltered adds an active filter so the match-index +
// highlight path is exercised too.
func BenchmarkSetLinesFiltered(b *testing.B) {
	lines := benchLines(2000)
	lp := NewLogPanel(120, 40)
	lp.SetFilter(regexp.MustCompile("ERROR"))
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		lp.SetLines(lines)
	}
}
