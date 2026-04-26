package gitinfo

import "time"

// networkTimeout caps fetch / pull — long enough for a slow clone, short
// enough that a dead remote can't wedge the UI.
const networkTimeout = 30 * time.Second

// Fetch runs `git fetch --prune` for dir. Returns combined stdout (often empty
// on success) so callers can surface a useful toast.
func Fetch(dir string) (string, error) {
	return runGitWithTimeout(dir, networkTimeout, "fetch", "--prune")
}

// Pull runs `git pull --ff-only` for dir. Never creates a merge commit;
// refuses if the branch has diverged.
func Pull(dir string) (string, error) {
	return runGitWithTimeout(dir, networkTimeout, "pull", "--ff-only")
}
