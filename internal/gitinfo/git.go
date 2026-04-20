package gitinfo

import (
	"bytes"
	"context"
	"errors"
	"os/exec"
	"strconv"
	"strings"
	"time"
)

const gitTimeout = 300 * time.Millisecond

// ErrGitNotFound is returned when the git binary is not available.
var ErrGitNotFound = errors.New("gitinfo: git binary not found")

// Info holds the current git status for a repository.
type Info struct {
	Branch string
	Ahead  int
	Behind int
	Dirty  bool
}

// Current returns git info for the repository rooted at dir.
// It is fail-open: if git is missing or a subcommand fails, it returns
// a partial Info and an error. Callers should show empty segments on error.
func Current(dir string) (Info, error) {
	var info Info

	branch, err := gitBranch(dir)
	if err != nil {
		return info, err
	}
	info.Branch = branch

	ahead, behind, err := gitAheadBehind(dir)
	if err == nil {
		info.Ahead = ahead
		info.Behind = behind
	}
	// Tolerate ahead/behind failure (no upstream etc.).

	dirty, err := gitDirty(dir)
	if err == nil {
		info.Dirty = dirty
	}

	return info, nil
}

// gitBranch returns the current branch name.
func gitBranch(dir string) (string, error) {
	out, err := runGit(dir, "rev-parse", "--abbrev-ref", "HEAD")
	if err != nil {
		return "", err
	}
	branch := strings.TrimSpace(out)
	if branch == "" {
		return "", errors.New("gitinfo: empty branch output")
	}
	return branch, nil
}

// gitAheadBehind returns ahead/behind counts relative to @{u} (upstream).
// Returns 0,0 with no error when there is no upstream (exit 128).
func gitAheadBehind(dir string) (ahead, behind int, err error) {
	out, gitErr := runGit(dir, "rev-list", "--left-right", "--count", "@{u}...HEAD")
	if gitErr != nil {
		// No upstream configured — not an error we surface.
		if isNoUpstream(gitErr) {
			return 0, 0, nil
		}
		return 0, 0, gitErr
	}
	fields := strings.Fields(strings.TrimSpace(out))
	if len(fields) < 2 {
		return 0, 0, nil
	}
	// Output is "<behind> <ahead>" (left-right means left=upstream=behind, right=HEAD=ahead).
	beh, _ := strconv.Atoi(fields[0])
	ahd, _ := strconv.Atoi(fields[1])
	return ahd, beh, nil
}

// gitDirty returns true if there are any tracked unstaged or staged changes.
func gitDirty(dir string) (bool, error) {
	out, err := runGit(dir, "status", "--porcelain=v1", "-uno")
	if err != nil {
		return false, err
	}
	return strings.TrimSpace(out) != "", nil
}

// runGit executes git with --no-optional-locks in dir with a 300ms timeout.
func runGit(dir string, args ...string) (string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), gitTimeout)
	defer cancel()

	fullArgs := append([]string{"-C", dir, "--no-optional-locks"}, args...)
	cmd := exec.CommandContext(ctx, "git", fullArgs...)
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		if errors.Is(err, exec.ErrNotFound) {
			return "", ErrGitNotFound
		}
		var exitErr *exec.ExitError
		if errors.As(err, &exitErr) {
			return "", &gitExecError{code: exitErr.ExitCode(), stderr: stderr.String()}
		}
		return "", err
	}
	return stdout.String(), nil
}

// isNoUpstream returns true for exit-128 errors (no upstream configured).
func isNoUpstream(err error) bool {
	var ge *gitExecError
	return errors.As(err, &ge) && ge.code == 128
}

// gitExecError carries the exit code and stderr of a failed git invocation.
type gitExecError struct {
	code   int
	stderr string
}

func (e *gitExecError) Error() string {
	return "git exited " + strconv.Itoa(e.code) + ": " + strings.TrimSpace(e.stderr)
}
