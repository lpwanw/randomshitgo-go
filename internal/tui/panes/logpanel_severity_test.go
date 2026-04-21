package panes

import (
	"strings"
	"testing"
)

func TestSeverity_MatchesTokens(t *testing.T) {
	cases := map[string]string{
		"2026-04-21 ERROR db dead":    severityFG["ERROR"],
		"WARN disk almost full":       severityFG["WARN"],
		"INFO started listener":       severityFG["INFO"],
		"DEBUG trace packet":          severityFG["DEBUG"],
		"TRACE step 1":                severityFG["TRACE"],
		"FATAL panic":                 severityFG["FATAL"],
		"WARNING low memory":          severityFG["WARNING"],
	}
	for line, want := range cases {
		got := applySeverity(line)
		if !strings.HasPrefix(got, want) {
			t.Errorf("line=%q\n  want prefix %q\n  got %q", line, want, got)
		}
		if !strings.HasSuffix(got, severityClose) {
			t.Errorf("line=%q missing close SGR, got %q", line, got)
		}
	}
}

func TestSeverity_LowercaseIgnored(t *testing.T) {
	got := applySeverity("error: lowercase noise")
	if got != "error: lowercase noise" {
		t.Errorf("lowercase should not trigger, got %q", got)
	}
}

func TestSeverity_WordBoundary(t *testing.T) {
	// "errorHandler" should NOT match — word boundary rejects identifier.
	got := applySeverity("call errorHandler()")
	if got != "call errorHandler()" {
		t.Errorf("inside identifier should not match, got %q", got)
	}
}

func TestSeverity_Toggle(t *testing.T) {
	lp := NewLogPanel(80, 10)
	lp.SetLines([]string{"ERROR boom"})
	view := lp.vp.View()
	if !strings.Contains(view, severityFG["ERROR"]) {
		t.Errorf("severity on by default should colour ERROR line; got %q", view)
	}
	lp.SetSeverity(false)
	view = lp.vp.View()
	if strings.Contains(view, severityFG["ERROR"]) {
		t.Errorf("after SetSeverity(false) the colour should be gone; got %q", view)
	}
}
