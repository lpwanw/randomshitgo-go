//go:build !windows

package e2e

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"go.uber.org/goleak"
)

func TestMain(m *testing.M) {
	// Ensure fixture scripts are executable.
	ensureFixturesExecutable()

	// goleak: ignore goroutines that are part of the Go runtime or test framework.
	opts := []goleak.Option{
		goleak.IgnoreTopFunction("testing.(*M).Run"),
		goleak.IgnoreTopFunction("testing.runTests"),
		goleak.IgnoreTopFunction("os/signal.loop"),
		goleak.IgnoreTopFunction("runtime.goexit"),
	}
	goleak.VerifyTestMain(m, opts...)
}

// fixtureDir returns the absolute path to the fixtures directory.
func fixtureDir() string {
	_, file, _, _ := runtime.Caller(0)
	return filepath.Join(filepath.Dir(file), "fixtures")
}

// fixturePath returns the absolute path to a specific fixture script.
func fixturePath(name string) string {
	return filepath.Join(fixtureDir(), name)
}

// ensureFixturesExecutable ensures all shell fixture scripts are executable.
func ensureFixturesExecutable() {
	scripts := []string{"noisy.sh", "crasher.sh", "repl.sh", "longrun.sh"}
	dir := fixtureDir()
	for _, s := range scripts {
		path := filepath.Join(dir, s)
		_ = os.Chmod(path, 0755)
	}
}
