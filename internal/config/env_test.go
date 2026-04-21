package config

import (
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"
)

func writeTemp(t *testing.T, name, body string) string {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, name)
	if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
		t.Fatalf("writeTemp: %v", err)
	}
	return path
}

func TestParseEnvFile_Basic(t *testing.T) {
	path := writeTemp(t, "a.env", `
# comment
FOO=bar
BAZ="quoted"
export QUX='single'
EMPTY=
`)
	env, err := parseEnvFile(path)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	want := map[string]string{"FOO": "bar", "BAZ": "quoted", "QUX": "single", "EMPTY": ""}
	for k, v := range want {
		if env[k] != v {
			t.Errorf("%s: want %q got %q", k, v, env[k])
		}
	}
}

func TestParseEnvFile_Malformed(t *testing.T) {
	path := writeTemp(t, "bad.env", "GOOD=1\nno_equals_here\n")
	_, err := parseEnvFile(path)
	if err == nil {
		t.Fatal("want parse error, got nil")
	}
	if !strings.Contains(err.Error(), "line 2") {
		t.Errorf("error should mention line number, got %v", err)
	}
}

func TestBuildEnv_Precedence(t *testing.T) {
	envFile := writeTemp(t, "override.env", "A=file\nB=file\n")
	p := Project{
		Env:     map[string]string{"B": "inline", "C": "inline"},
		EnvFile: envFile,
	}
	got, err := p.BuildEnv([]string{"A=base", "D=base"})
	if err != nil {
		t.Fatalf("BuildEnv: %v", err)
	}
	sort.Strings(got)
	want := map[string]string{
		"A": "file",    // env_file overrides base
		"B": "inline",  // inline overrides env_file
		"C": "inline",  // inline-only
		"D": "base",    // base passthrough
	}
	for _, kv := range got {
		eq := strings.IndexByte(kv, '=')
		k, v := kv[:eq], kv[eq+1:]
		if exp, ok := want[k]; ok && exp != v {
			t.Errorf("%s: want %q got %q", k, exp, v)
		}
	}
}

func TestBuildEnv_NoFile(t *testing.T) {
	p := Project{Env: map[string]string{"X": "y"}}
	got, err := p.BuildEnv([]string{"PATH=/usr/bin"})
	if err != nil {
		t.Fatalf("BuildEnv: %v", err)
	}
	if len(got) != 2 {
		t.Errorf("want 2 entries, got %d: %v", len(got), got)
	}
}
