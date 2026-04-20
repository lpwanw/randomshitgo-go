package gitinfo

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

// requireGit skips the test if git is not installed.
func requireGit(t *testing.T) {
	t.Helper()
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git binary not available")
	}
}

// initRepo creates a temporary git repo with a single commit and returns its path.
func initRepo(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()

	run := func(args ...string) {
		t.Helper()
		cmd := exec.Command("git", args...)
		cmd.Dir = dir
		cmd.Env = append(os.Environ(),
			"GIT_AUTHOR_NAME=Test",
			"GIT_AUTHOR_EMAIL=test@test.com",
			"GIT_COMMITTER_NAME=Test",
			"GIT_COMMITTER_EMAIL=test@test.com",
		)
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v\n%s", args, err, out)
		}
	}

	run("init")
	run("config", "user.email", "test@test.com")
	run("config", "user.name", "Test")
	// Write a file and commit so HEAD is not unborn.
	if err := os.WriteFile(filepath.Join(dir, "README.md"), []byte("hello"), 0644); err != nil {
		t.Fatal(err)
	}
	run("add", ".")
	run("commit", "-m", "init")
	return dir
}

func TestCurrent_Branch(t *testing.T) {
	requireGit(t)
	dir := initRepo(t)

	info, err := Current(dir)
	if err != nil {
		t.Fatalf("Current() error: %v", err)
	}
	if info.Branch == "" {
		t.Error("expected non-empty branch")
	}
}

func TestCurrent_NoUpstream(t *testing.T) {
	requireGit(t)
	dir := initRepo(t)

	// No upstream configured — ahead/behind should be 0,0 without error.
	info, err := Current(dir)
	if err != nil {
		t.Fatalf("Current() error: %v", err)
	}
	if info.Ahead != 0 || info.Behind != 0 {
		t.Errorf("expected 0/0 ahead/behind, got %d/%d", info.Ahead, info.Behind)
	}
}

func TestCurrent_DirtyRepo(t *testing.T) {
	requireGit(t)
	dir := initRepo(t)

	// Stage a change to make the repo dirty.
	if err := os.WriteFile(filepath.Join(dir, "new.txt"), []byte("dirty"), 0644); err != nil {
		t.Fatal(err)
	}
	cmd := exec.Command("git", "add", ".")
	cmd.Dir = dir
	if err := cmd.Run(); err != nil {
		t.Fatal(err)
	}

	info, err := Current(dir)
	if err != nil {
		t.Fatalf("Current() error: %v", err)
	}
	if !info.Dirty {
		t.Error("expected dirty=true")
	}
}

func TestCurrent_CleanRepo(t *testing.T) {
	requireGit(t)
	dir := initRepo(t)

	info, err := Current(dir)
	if err != nil {
		t.Fatalf("Current() error: %v", err)
	}
	if info.Dirty {
		t.Error("expected dirty=false on clean repo")
	}
}

func TestBranches_ReturnsList(t *testing.T) {
	requireGit(t)
	dir := initRepo(t)

	branches, err := Branches(dir)
	if err != nil {
		t.Fatalf("Branches() error: %v", err)
	}
	if len(branches) == 0 {
		t.Error("expected at least one branch")
	}
}

func TestBranches_NotAvailableForNonRepo(t *testing.T) {
	requireGit(t)
	dir := t.TempDir() // not a git repo

	_, err := Branches(dir)
	if err == nil {
		t.Error("expected error for non-git directory")
	}
}

func TestGitNotFound_ReturnsError(t *testing.T) {
	requireGit(t)
	_, err := Current("/nonexistent-path-12345")
	if err == nil {
		t.Error("expected error for invalid path")
	}
}
