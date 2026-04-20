package gitinfo

import (
	"strings"
)

// Branches returns the list of local branch names for the repo at dir.
// Returns an empty slice (not an error) when the repo has no commits.
func Branches(dir string) ([]string, error) {
	out, err := runGit(dir, "branch", "--list", "--format=%(refname:short)")
	if err != nil {
		return nil, err
	}
	var branches []string
	for _, line := range strings.Split(out, "\n") {
		b := strings.TrimSpace(line)
		if b != "" {
			branches = append(branches, b)
		}
	}
	return branches, nil
}

// Checkout switches the working tree to branch.
// It does NOT force (no --force flag) to avoid discarding local changes.
func Checkout(dir, branch string) error {
	_, err := runGit(dir, "checkout", branch)
	return err
}
