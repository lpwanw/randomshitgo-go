package gitinfo

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// initRepoPair builds origin + work (clone of origin) with one commit, and
// returns (originDir, workDir). work's upstream is origin/<default-branch>.
func initRepoPair(t *testing.T) (string, string) {
	t.Helper()

	root := t.TempDir()
	originDir := filepath.Join(root, "origin")
	workDir := filepath.Join(root, "work")
	if err := os.Mkdir(originDir, 0o755); err != nil {
		t.Fatal(err)
	}

	gitEnv := append(os.Environ(),
		"GIT_AUTHOR_NAME=Test",
		"GIT_AUTHOR_EMAIL=test@test.com",
		"GIT_COMMITTER_NAME=Test",
		"GIT_COMMITTER_EMAIL=test@test.com",
	)
	run := func(dir string, args ...string) {
		t.Helper()
		cmd := exec.Command("git", args...)
		cmd.Dir = dir
		cmd.Env = gitEnv
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %v in %s: %v\n%s", args, dir, err, out)
		}
	}

	// origin: bare-ish layout via a normal repo with a first commit. We clone
	// from this path; cloning a non-bare repo works for tests.
	run(originDir, "init", "-b", "main")
	run(originDir, "config", "user.email", "test@test.com")
	run(originDir, "config", "user.name", "Test")
	if err := os.WriteFile(filepath.Join(originDir, "README.md"), []byte("v1\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	run(originDir, "add", ".")
	run(originDir, "commit", "-m", "v1")
	// Allow pushing into a non-bare repo's checked-out branch.
	run(originDir, "config", "receive.denyCurrentBranch", "ignore")

	// Clone into work.
	cmd := exec.Command("git", "clone", originDir, workDir)
	cmd.Env = gitEnv
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git clone: %v\n%s", err, out)
	}
	run(workDir, "config", "user.email", "test@test.com")
	run(workDir, "config", "user.name", "Test")

	return originDir, workDir
}

// addCommit writes file with content in dir and commits with msg.
func addCommit(t *testing.T, dir, file, content, msg string) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(dir, file), []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	gitEnv := append(os.Environ(),
		"GIT_AUTHOR_NAME=Test",
		"GIT_AUTHOR_EMAIL=test@test.com",
		"GIT_COMMITTER_NAME=Test",
		"GIT_COMMITTER_EMAIL=test@test.com",
	)
	for _, args := range [][]string{{"add", "."}, {"commit", "-m", msg}} {
		cmd := exec.Command("git", args...)
		cmd.Dir = dir
		cmd.Env = gitEnv
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v\n%s", args, err, out)
		}
	}
}

func TestFetch_AdvancesRemoteRef(t *testing.T) {
	requireGit(t)
	originDir, workDir := initRepoPair(t)

	// Origin gets a new commit AFTER clone; work's remote-tracking ref is stale.
	addCommit(t, originDir, "new.txt", "hello", "v2")

	if _, err := Fetch(workDir); err != nil {
		t.Fatalf("Fetch() error: %v", err)
	}

	// After fetch, work should be behind by 1 via ahead/behind calc against @{u}.
	info, err := Current(workDir)
	if err != nil {
		t.Fatalf("Current() after fetch: %v", err)
	}
	if info.Behind != 1 {
		t.Errorf("behind after fetch: want 1, got %d", info.Behind)
	}
}

func TestPull_FastForward(t *testing.T) {
	requireGit(t)
	originDir, workDir := initRepoPair(t)
	addCommit(t, originDir, "new.txt", "hello", "v2")

	out, err := Pull(workDir)
	if err != nil {
		t.Fatalf("Pull() error: %v", err)
	}
	if out == "" {
		t.Error("Pull() returned empty output on success — expected a summary line")
	}

	// After successful ff pull, ahead/behind collapses to 0/0.
	info, _ := Current(workDir)
	if info.Ahead != 0 || info.Behind != 0 {
		t.Errorf("after ff pull: want 0/0 ahead/behind, got %d/%d", info.Ahead, info.Behind)
	}
}

func TestPull_DivergedRefusesFastForward(t *testing.T) {
	requireGit(t)
	originDir, workDir := initRepoPair(t)

	// Diverge: commit on origin AND on work.
	addCommit(t, originDir, "o.txt", "origin-only", "origin-v2")
	addCommit(t, workDir, "w.txt", "work-only", "work-v2")

	_, err := Pull(workDir)
	if err == nil {
		t.Fatal("Pull() on diverged branch should error with --ff-only")
	}
	msg := err.Error()
	// Accept any of git's diverged messages; just make sure we're not silently swallowing.
	if !strings.Contains(strings.ToLower(msg), "fast-forward") &&
		!strings.Contains(strings.ToLower(msg), "divergent") &&
		!strings.Contains(strings.ToLower(msg), "not possible") {
		t.Logf("diverged pull error message: %q", msg) // not fatal — wording varies by git version
	}
}

func TestPull_NoUpstream(t *testing.T) {
	requireGit(t)
	dir := initRepo(t)

	_, err := Pull(dir)
	if err == nil {
		t.Error("Pull() on repo without upstream should error")
	}
}
