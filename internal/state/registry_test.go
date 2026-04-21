package state

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/lpwanw/randomshitgo-go/internal/config"
)

func makeTestRegistrySettings(t *testing.T) config.Settings {
	t.Helper()
	dir := t.TempDir()
	return config.Settings{
		LogBufferLines:  50,
		LogDir:          dir,
		LogRotateSizeMB: 1,
		LogRotateKeep:   2,
	}
}

func TestRegistry_Get_LazyInit(t *testing.T) {
	s := makeTestRegistrySettings(t)
	r := NewRegistry(s)

	e := r.Get("web")
	if e == nil {
		t.Fatal("expected non-nil entry")
	}
	if e.Ring == nil {
		t.Fatal("Ring should be initialised")
	}
	if e.Rot == nil {
		t.Fatal("Rotator should be initialised")
	}

	// Second call returns same entry.
	e2 := r.Get("web")
	if e != e2 {
		t.Fatal("expected same entry on second Get")
	}
}

func TestRegistry_Get_LogFileCreated(t *testing.T) {
	s := makeTestRegistrySettings(t)
	r := NewRegistry(s)
	r.Get("api")

	expected := filepath.Join(s.LogDir, "api.log")
	if _, err := os.Stat(expected); err != nil {
		t.Fatalf("log file not created at %s: %v", expected, err)
	}
}

func TestRegistry_WriteRaw_RoundTrip(t *testing.T) {
	s := makeTestRegistrySettings(t)
	r := NewRegistry(s)

	r.WriteRaw("svc", []byte("hello\nworld\n"))

	e := r.Get("svc")
	if e.Ring.Len() != 2 {
		t.Fatalf("expected 2 ring entries, got %d", e.Ring.Len())
	}

	snap := e.Ring.Snapshot()
	if string(snap[0].Bytes) != "hello" {
		t.Fatalf("expected first line 'hello', got %q", string(snap[0].Bytes))
	}
	if string(snap[1].Bytes) != "world" {
		t.Fatalf("expected second line 'world', got %q", string(snap[1].Bytes))
	}

	// Verify log file has content.
	logPath := filepath.Join(s.LogDir, "svc.log")
	data, err := os.ReadFile(logPath)
	if err != nil {
		t.Fatalf("read log file: %v", err)
	}
	content := string(data)
	if !strings.Contains(content, "hello") || !strings.Contains(content, "world") {
		t.Fatalf("log file missing expected content: %q", content)
	}
}

func TestRegistry_WriteRaw_ANSIStripped(t *testing.T) {
	s := makeTestRegistrySettings(t)
	r := NewRegistry(s)

	r.WriteRaw("color", []byte("\x1b[32mgreen\x1b[0m\n"))

	logPath := filepath.Join(s.LogDir, "color.log")
	data, err := os.ReadFile(logPath)
	if err != nil {
		t.Fatalf("read log file: %v", err)
	}
	if strings.Contains(string(data), "\x1b") {
		t.Fatalf("log file should not contain ANSI escapes, got: %q", string(data))
	}
	if !strings.Contains(string(data), "green") {
		t.Fatalf("log file should contain 'green', got: %q", string(data))
	}
}

func TestRegistry_Close(t *testing.T) {
	s := makeTestRegistrySettings(t)
	r := NewRegistry(s)
	r.Get("a")
	r.Get("b")
	r.Close() // should not panic
}
