//go:build darwin

package netinfo

import (
	"context"
	"fmt"
	"os/exec"
	"strconv"
	"strings"
	"time"
)

// PortForPID returns the smallest TCP LISTEN port for the given PID on macOS.
// It shells out to lsof with machine-readable output (-F pn) to obtain a
// system-wide list of LISTEN sockets and filters by the requested PID.
//
// Note: On macOS with SIP/sandbox restrictions, lsof may be unable to inspect
// processes owned by other users. This function returns ErrNoPort in that case.
func PortForPID(pid int) (int, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 500*time.Millisecond)
	defer cancel()

	// -F pn: machine-readable, emit process (p) and name (n) fields only.
	// Not using -p PID because macOS lsof requires elevated privileges to
	// filter by PID; system-wide scan + in-process filter is more reliable.
	cmd := exec.CommandContext(ctx, "lsof",
		"-iTCP", "-sTCP:LISTEN",
		"-P", "-n",
		"-F", "pn",
	)
	out, err := cmd.Output()
	if err != nil {
		if ctx.Err() != nil {
			return 0, fmt.Errorf("netinfo: lsof timeout")
		}
		// lsof exits 1 when no LISTEN sockets exist at all — uncommon but valid.
		if len(out) == 0 {
			return 0, ErrNoPort
		}
	}

	// Parse -F pn output: lines starting with 'p' are process records,
	// lines starting with 'n' are name (address) records for the current fd.
	// We collect 'n' lines only while the current pid matches the target.
	targetPID := strconv.Itoa(pid)
	inTarget := false
	smallest := 0

	for _, line := range strings.Split(string(out), "\n") {
		if strings.HasPrefix(line, "p") {
			// 'p' line: set whether we're in the target PID section.
			inTarget = strings.TrimPrefix(line, "p") == targetPID
			continue
		}
		if !inTarget {
			continue
		}
		if !strings.HasPrefix(line, "n") {
			continue
		}
		addr := line[1:] // strip leading 'n'
		port := parseLastPort(addr)
		if port > 0 && (smallest == 0 || port < smallest) {
			smallest = port
		}
	}

	if smallest == 0 {
		return 0, ErrNoPort
	}
	return smallest, nil
}

// parseLastPort extracts the port from the last ":" segment of an address.
// Handles IPv4 (1.2.3.4:PORT), wildcard (*:PORT), and IPv6 ([::1]:PORT).
func parseLastPort(addr string) int {
	idx := strings.LastIndex(addr, ":")
	if idx < 0 {
		return 0
	}
	portStr := addr[idx+1:]
	p, err := strconv.Atoi(portStr)
	if err != nil || p <= 0 || p > 65535 {
		return 0
	}
	return p
}
