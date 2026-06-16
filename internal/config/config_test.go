package config

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func writeTmp(t *testing.T, name, body string) string {
	t.Helper()
	dir := t.TempDir()
	p := filepath.Join(dir, name)
	if err := os.WriteFile(p, []byte(body), 0o600); err != nil {
		t.Fatalf("write tmp: %v", err)
	}
	return p
}

func TestLoadMinimal(t *testing.T) {
	p := writeTmp(t, "c.yml", `
projects:
  api:
    path: /tmp
    cmd: echo hi
`)
	cfg, err := Load(p)
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if len(cfg.Projects) != 1 || cfg.Projects["api"].Cmd != "echo hi" {
		t.Fatalf("unexpected projects: %+v", cfg.Projects)
	}
	if cfg.Projects["api"].Restart != RestartNever {
		t.Fatalf("restart default: got %q", cfg.Projects["api"].Restart)
	}
	if cfg.Settings.LogBufferLines != DefaultSettings.LogBufferLines {
		t.Fatalf("defaults not applied: %+v", cfg.Settings)
	}
	if cfg.Settings.PtyCols != 120 {
		t.Fatalf("pty_cols default: %d", cfg.Settings.PtyCols)
	}
}

func TestLoadFullSchema(t *testing.T) {
	p := writeTmp(t, "c.yml", `
projects:
  api:
    path: /tmp
    cmd: bun run dev
    restart: on-failure
  web:
    path: /tmp
    cmd: npm run dev
groups:
  full: [api, web]
settings:
  log_buffer_lines: 500
  log_flush_interval_ms: 100
  pty_cols: 80
  pty_rows: 24
`)
	cfg, err := Load(p)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Settings.LogBufferLines != 500 || cfg.Settings.PtyCols != 80 {
		t.Fatalf("user settings not preserved: %+v", cfg.Settings)
	}
	if cfg.Settings.LogRotateKeep != 5 {
		t.Fatalf("unspecified setting should fall back: %d", cfg.Settings.LogRotateKeep)
	}
	if len(cfg.Groups["full"]) != 2 {
		t.Fatalf("groups: %+v", cfg.Groups)
	}
}

func TestLoadMissingCmd(t *testing.T) {
	p := writeTmp(t, "c.yml", `
projects:
  api:
    path: /tmp
`)
	_, err := Load(p)
	if err == nil || !strings.Contains(err.Error(), "projects.api.cmd") {
		t.Fatalf("expected cmd error, got: %v", err)
	}
}

func TestLoadUnknownKeyStrict(t *testing.T) {
	p := writeTmp(t, "c.yml", `
projects:
  api:
    path: /tmp
    cmd: echo
    mystery: 1
`)
	_, err := Load(p)
	if err == nil {
		t.Fatal("expected strict decode error")
	}
}

func TestLoadGroupUnknownProject(t *testing.T) {
	p := writeTmp(t, "c.yml", `
projects:
  api:
    path: /tmp
    cmd: echo
groups:
  full: [api, web]
`)
	_, err := Load(p)
	if err == nil || !strings.Contains(err.Error(), `unknown project "web"`) {
		t.Fatalf("expected unknown project err, got: %v", err)
	}
}

func TestLoadInvalidRestart(t *testing.T) {
	p := writeTmp(t, "c.yml", `
projects:
  api:
    path: /tmp
    cmd: echo
    restart: sometimes
`)
	_, err := Load(p)
	if err == nil || !strings.Contains(err.Error(), "restart") {
		t.Fatalf("expected restart err, got: %v", err)
	}
}

func TestExpandPathTilde(t *testing.T) {
	home, _ := os.UserHomeDir()
	for _, tc := range []struct{ in, want string }{
		{"~", home},
		{"~/foo", home + "/foo"},
		{"/abs", "/abs"},
	} {
		got, err := ExpandPath(tc.in)
		if err != nil {
			t.Fatalf("%q: %v", tc.in, err)
		}
		if got != tc.want {
			t.Fatalf("%q: got %q want %q", tc.in, got, tc.want)
		}
	}
}

func TestExpandPathEnv(t *testing.T) {
	t.Setenv("PROCS_TEST_VAR", "banana")
	got, err := ExpandPath("/opt/$PROCS_TEST_VAR/bin")
	if err != nil || got != "/opt/banana/bin" {
		t.Fatalf("got %q err %v", got, err)
	}
	got, err = ExpandPath("/opt/${PROCS_TEST_VAR}/bin")
	if err != nil || got != "/opt/banana/bin" {
		t.Fatalf("${} form: got %q err %v", got, err)
	}
}

func TestExpandPathMissingEnv(t *testing.T) {
	os.Unsetenv("PROCS_NOPE_XYZ")
	_, err := ExpandPath("/a/$PROCS_NOPE_XYZ/b")
	if err == nil || !strings.Contains(err.Error(), "undefined env var") {
		t.Fatalf("want undefined err, got: %v", err)
	}
}

func TestExpandPathRejectsTildeUser(t *testing.T) {
	_, err := ExpandPath("~alice/foo")
	if err == nil || !strings.Contains(err.Error(), "~user") {
		t.Fatalf("want ~user err, got: %v", err)
	}
}

func TestResolvePathEnvOverride(t *testing.T) {
	t.Setenv("PROCS_CONFIG", "/tmp/alt.yml")
	got, err := ResolvePath("")
	if err != nil || got != "/tmp/alt.yml" {
		t.Fatalf("got %q err %v", got, err)
	}
}

func TestLoadExpansionHappens(t *testing.T) {
	t.Setenv("PROCS_TEST_ROOT", "/opt/proj")
	p := writeTmp(t, "c.yml", `
projects:
  api:
    path: $PROCS_TEST_ROOT/api
    cmd: echo
`)
	cfg, err := Load(p)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Projects["api"].Path != "/opt/proj/api" {
		t.Fatalf("expand: %q", cfg.Projects["api"].Path)
	}
}

func TestErrorUnwrap(t *testing.T) {
	e := &Error{Msg: "foo", Err: errors.New("bar")}
	if !errors.Is(e, e.Err) {
		t.Fatal("errors.Is should match wrapped err")
	}
}
