package log

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestRotatorRotatesAtMaxSize(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "app.log")
	r, err := NewRotator(p, 1, 3) // 1 MB, 3 backups
	if err != nil {
		t.Fatal(err)
	}
	defer r.Close()

	chunk := make([]byte, 128*1024) // 128 KB
	for i := range chunk {
		chunk[i] = 'a'
	}
	for i := 0; i < 10; i++ { // 1.25 MB total
		if _, err := r.Write(chunk); err != nil {
			t.Fatalf("write %d: %v", i, err)
		}
	}
	r.Close()

	if _, err := os.Stat(p); err != nil {
		t.Fatalf("main file missing: %v", err)
	}
	if _, err := os.Stat(p + ".1"); err != nil {
		t.Fatalf("backup .1 missing: %v", err)
	}
}

func TestRotatorCapsBackups(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "app.log")
	r, err := NewRotator(p, 1, 2)
	if err != nil {
		t.Fatal(err)
	}
	defer r.Close()

	chunk := make([]byte, 600*1024)
	for i := range chunk {
		chunk[i] = 'x'
	}
	for i := 0; i < 10; i++ {
		if _, err := r.Write(chunk); err != nil {
			t.Fatalf("write: %v", err)
		}
	}
	r.Close()

	if _, err := os.Stat(p + ".3"); err == nil {
		t.Fatal(".3 should not exist — cap was 2")
	}
	if _, err := os.Stat(p + ".2"); err != nil {
		t.Fatalf(".2 should exist: %v", err)
	}
}

func TestRotatorAppend(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "app.log")
	// pre-create with content
	if err := os.WriteFile(p, []byte("existing\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	r, err := NewRotator(p, 10, 3)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := r.Write([]byte("new\n")); err != nil {
		t.Fatal(err)
	}
	r.Close()

	b, _ := os.ReadFile(p)
	if !strings.HasPrefix(string(b), "existing\nnew") {
		t.Fatalf("append broke: %q", b)
	}
}

func TestRotatorMakesLogDir(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "nested", "d", "app.log")
	r, err := NewRotator(p, 1, 1)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := r.Write([]byte(fmt.Sprintln("hi"))); err != nil {
		t.Fatal(err)
	}
	r.Close()
	if _, err := os.Stat(p); err != nil {
		t.Fatalf("file not created in nested dir: %v", err)
	}
}
