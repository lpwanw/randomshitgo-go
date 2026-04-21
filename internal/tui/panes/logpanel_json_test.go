package panes

import (
	"strings"
	"testing"
)

func TestJSON_PrettyObject(t *testing.T) {
	parts, ok := prettifyJSONLine(`{"a":1,"b":[2,3]}`)
	if !ok {
		t.Fatal("prettifyJSONLine should succeed on valid object")
	}
	if len(parts) < 3 {
		t.Errorf("expected multi-line output, got %v", parts)
	}
	if !strings.Contains(strings.Join(parts, "\n"), "  \"a\": 1") {
		t.Errorf("missing indented key; got %q", parts)
	}
}

func TestJSON_PrettyArray(t *testing.T) {
	parts, ok := prettifyJSONLine(`[1,2,{"k":"v"}]`)
	if !ok {
		t.Fatal("array should pretty-print")
	}
	if len(parts) < 4 {
		t.Errorf("expected multi-line output, got %v", parts)
	}
}

func TestJSON_NonJSONPassesThrough(t *testing.T) {
	_, ok := prettifyJSONLine("plain text line")
	if ok {
		t.Error("plain text must not pretty-print")
	}
	_, ok = prettifyJSONLine("{malformed")
	if ok {
		t.Error("incomplete JSON must not pretty-print")
	}
}

func TestJSON_Toggle_ExpandsAndRestores(t *testing.T) {
	lp := NewLogPanel(80, 20)
	lp.SetLines([]string{`plain`, `{"a":1,"b":2}`})
	if len(lp.rawLines) != 2 {
		t.Fatalf("default raw=%d want 2", len(lp.rawLines))
	}
	lp.SetJSONPretty(true)
	if len(lp.rawLines) <= 2 {
		t.Errorf("json on should expand; got raw=%d", len(lp.rawLines))
	}
	lp.SetJSONPretty(false)
	if len(lp.rawLines) != 2 {
		t.Errorf("json off should restore; got raw=%d", len(lp.rawLines))
	}
}
