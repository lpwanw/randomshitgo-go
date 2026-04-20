//go:build linux

package netinfo

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

// PortForPID returns the smallest TCP LISTEN port for the given PID on Linux.
// It reads /proc/<pid>/fd/* to collect socket inodes, then correlates with
// /proc/<pid>/net/tcp (IPv4) and /proc/<pid>/net/tcp6 (IPv6).
// State 0A = TCP_LISTEN.
func PortForPID(pid int) (int, error) {
	inodes, err := socketInodes(pid)
	if err != nil {
		return 0, fmt.Errorf("netinfo: read fds for pid %d: %w", pid, err)
	}
	if len(inodes) == 0 {
		return 0, ErrNoPort
	}

	smallest := 0
	for _, netFile := range []string{
		fmt.Sprintf("/proc/%d/net/tcp", pid),
		fmt.Sprintf("/proc/%d/net/tcp6", pid),
	} {
		port, err := scanNetTCP(netFile, inodes)
		if err == nil && port > 0 {
			if smallest == 0 || port < smallest {
				smallest = port
			}
		}
	}

	if smallest == 0 {
		return 0, ErrNoPort
	}
	return smallest, nil
}

// socketInodes reads /proc/<pid>/fd/* and returns the set of inode numbers
// for entries that are socket:[NNNN] symlinks.
func socketInodes(pid int) (map[string]struct{}, error) {
	fdDir := fmt.Sprintf("/proc/%d/fd", pid)
	entries, err := os.ReadDir(fdDir)
	if err != nil {
		return nil, err
	}

	inodes := make(map[string]struct{})
	for _, e := range entries {
		target, err := os.Readlink(filepath.Join(fdDir, e.Name()))
		if err != nil {
			continue
		}
		// Format: socket:[INODE]
		if strings.HasPrefix(target, "socket:[") && strings.HasSuffix(target, "]") {
			inode := target[len("socket:[") : len(target)-1]
			inodes[inode] = struct{}{}
		}
	}
	return inodes, nil
}

// scanNetTCP parses a /proc/net/tcp* file looking for LISTEN rows (state 0A)
// whose inode is in the provided set. Returns the smallest port found.
func scanNetTCP(path string, inodes map[string]struct{}) (int, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return 0, err
	}

	smallest := 0
	for i, line := range strings.Split(string(data), "\n") {
		if i == 0 {
			continue // header
		}
		fields := strings.Fields(line)
		if len(fields) < 10 {
			continue
		}
		// field[3] = state (hex), field[9] = inode (decimal)
		if fields[3] != "0A" { // TCP_LISTEN
			continue
		}
		inode := fields[9]
		if _, ok := inodes[inode]; !ok {
			continue
		}
		// field[1] = local_address in format HexIP:HexPort
		localAddr := fields[1]
		colonIdx := strings.Index(localAddr, ":")
		if colonIdx < 0 {
			continue
		}
		portHex := localAddr[colonIdx+1:]
		portVal, err := strconv.ParseInt(portHex, 16, 32)
		if err != nil || portVal <= 0 || portVal > 65535 {
			continue
		}
		port := int(portVal)
		if smallest == 0 || port < smallest {
			smallest = port
		}
	}
	return smallest, nil
}
