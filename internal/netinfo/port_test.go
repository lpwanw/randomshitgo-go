package netinfo

import (
	"bufio"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"
)

// TestPortForPID spawns the testdata/listen helper binary, which opens a TCP
// LISTEN socket and prints the port. We then assert PortForPID returns the
// same port for the helper's PID.
//
// The helper is compiled with "go build" first so that cmd.Process.Pid is the
// actual process owning the socket (not the go toolchain wrapper from go run).
//
// This test requires either:
//   - macOS: lsof with sufficient privileges to inspect other processes
//   - Linux: /proc filesystem access
//
// If PortForPID returns ErrNoPort for the spawned PID, we skip rather than fail,
// because in sandboxed CI environments process introspection may not be available.
func TestPortForPID(t *testing.T) {
	// Check that go is available.
	if _, err := exec.LookPath("go"); err != nil {
		t.Skip("go binary not available")
	}

	// Compile the helper binary into a temp dir.
	tmpDir := t.TempDir()
	binPath := filepath.Join(tmpDir, "listen-helper")
	buildCmd := exec.Command("go", "build", "-o", binPath, "./testdata/listen")
	if out, err := buildCmd.CombinedOutput(); err != nil {
		t.Skipf("could not build test helper: %v\n%s", err, out)
	}

	// Spawn the compiled listener helper.
	stdinR, stdinW, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}

	cmd := exec.Command(binPath)
	cmd.Stderr = os.Stderr
	cmd.Stdin = stdinR

	stdoutPipe, err := cmd.StdoutPipe()
	if err != nil {
		t.Fatal(err)
	}

	if err := cmd.Start(); err != nil {
		t.Skipf("could not start test helper: %v", err)
	}
	defer func() {
		stdinW.Close() // signal helper to exit
		cmd.Process.Kill()
		cmd.Wait() //nolint:errcheck
		stdinR.Close()
	}()

	// Read the port number printed by the helper.
	scanner := bufio.NewScanner(stdoutPipe)
	portLine := ""
	done := make(chan struct{})
	go func() {
		if scanner.Scan() {
			portLine = scanner.Text()
		}
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(10 * time.Second):
		t.Fatal("timeout waiting for helper to print port")
	}

	portLine = strings.TrimSpace(portLine)
	wantPort, err := strconv.Atoi(portLine)
	if err != nil || wantPort <= 0 {
		t.Fatalf("invalid port from helper: %q", portLine)
	}

	pid := cmd.Process.Pid

	// Give the OS a moment to register the socket in the system tables.
	time.Sleep(200 * time.Millisecond)

	gotPort, err := PortForPID(pid)
	if err == ErrNoPort {
		// Sandboxed environment or insufficient permissions: skip gracefully.
		t.Skipf("PortForPID: %v — process introspection not available in this environment (PID=%d, want port=%d)", err, pid, wantPort)
	}
	if err != nil {
		t.Fatalf("PortForPID(%d): %v", pid, err)
	}
	if gotPort != wantPort {
		t.Fatalf("PortForPID=%d, want %d", gotPort, wantPort)
	}
}

// TestPortForPID_UnknownPID verifies that an unknown PID returns an error.
func TestPortForPID_UnknownPID(t *testing.T) {
	// PID 999999999 is very unlikely to exist.
	_, err := PortForPID(999999999)
	if err == nil {
		t.Error("expected error for unknown PID")
	}
}
