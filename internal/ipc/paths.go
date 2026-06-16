//go:build !windows

package ipc

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io/fs"
	"net"
	"os"
	"path/filepath"
	"syscall"
)

// cacheSubdir is the procs-owned directory under the user cache root.
const cacheSubdir = "procs"

// hashCfg returns a short stable hex digest of the resolved config path so
// distinct configs map to distinct daemon sockets.
func hashCfg(resolvedCfgPath string) string {
	sum := sha256.Sum256([]byte(resolvedCfgPath))
	return hex.EncodeToString(sum[:4]) // 8 hex chars
}

// cacheDir returns the procs cache directory (not guaranteed to exist).
func cacheDir() (string, error) {
	base, err := os.UserCacheDir()
	if err != nil {
		return "", fmt.Errorf("ipc: user cache dir: %w", err)
	}
	return filepath.Join(base, cacheSubdir), nil
}

// pathFor returns <cacheDir>/<hash><suffix> for the given resolved config path.
func pathFor(resolvedCfgPath, suffix string) (string, error) {
	dir, err := cacheDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, hashCfg(resolvedCfgPath)+suffix), nil
}

// SocketPath returns the unix socket path for the given resolved config path.
func SocketPath(resolvedCfgPath string) (string, error) {
	return pathFor(resolvedCfgPath, ".sock")
}

// PidPath returns the daemon pidfile path for the given resolved config path.
func PidPath(resolvedCfgPath string) (string, error) {
	return pathFor(resolvedCfgPath, ".pid")
}

// DaemonLogPath returns the daemon log path for the given resolved config path.
func DaemonLogPath(resolvedCfgPath string) (string, error) {
	return pathFor(resolvedCfgPath, ".daemon.log")
}

// ChildrenPath returns the persisted-child-PID file path used for orphan
// detection after a daemon crash.
func ChildrenPath(resolvedCfgPath string) (string, error) {
	return pathFor(resolvedCfgPath, ".children")
}

// EnsureSecureDir creates the procs cache directory and guarantees it is
// owner-only (0o700) and not a symlink owned by another user. It chmods even
// when the directory already exists, because the log rotator may have created
// it at a looser mode. Returns the directory path.
func EnsureSecureDir() (string, error) {
	dir, err := cacheDir()
	if err != nil {
		return "", err
	}
	if err := ensureSecureDir(dir); err != nil {
		return "", err
	}
	return dir, nil
}

// ensureSecureDir is the testable core of EnsureSecureDir for an explicit dir.
func ensureSecureDir(dir string) error {
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return fmt.Errorf("ipc: mkdir %q: %w", dir, err)
	}
	// MkdirAll is a no-op when the dir already exists, so force the mode.
	if err := os.Chmod(dir, 0o700); err != nil {
		return fmt.Errorf("ipc: chmod %q: %w", dir, err)
	}
	fi, err := os.Lstat(dir)
	if err != nil {
		return fmt.Errorf("ipc: lstat %q: %w", dir, err)
	}
	if fi.Mode()&os.ModeSymlink != 0 {
		return fmt.Errorf("ipc: cache dir %q is a symlink; refusing to use it", dir)
	}
	if err := checkOwner(fi, dir); err != nil {
		return err
	}
	return nil
}

// DialIfAlive attempts to connect to the daemon socket. It returns (conn, true)
// when a daemon is accepting, or (nil, false) when the socket is missing or
// refusing (stale/dead). A live connection is returned for the caller to reuse.
func DialIfAlive(sock string) (net.Conn, bool) {
	conn, err := net.Dial("unix", sock)
	if err != nil {
		return nil, false
	}
	return conn, true
}

// RemoveStaleSocket unlinks a dead socket only after verifying it is a real
// socket file owned by the current user and not a symlink — refusing otherwise
// to avoid being tricked into deleting/following an attacker-planted entry.
func RemoveStaleSocket(sock string) error {
	fi, err := os.Lstat(sock)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return nil
		}
		return fmt.Errorf("ipc: lstat %q: %w", sock, err)
	}
	if fi.Mode()&os.ModeSymlink != 0 {
		return fmt.Errorf("ipc: refusing to remove symlinked socket %q", sock)
	}
	if fi.Mode()&os.ModeSocket == 0 {
		return fmt.Errorf("ipc: refusing to remove non-socket %q", sock)
	}
	if err := checkOwner(fi, sock); err != nil {
		return err
	}
	return os.Remove(sock)
}

// checkOwner returns an error if the file is not owned by the current uid.
// On platforms without a unix stat, the check is skipped.
func checkOwner(fi os.FileInfo, name string) error {
	st, ok := fi.Sys().(*syscall.Stat_t)
	if !ok {
		return nil
	}
	if int(st.Uid) != os.Getuid() {
		return fmt.Errorf("ipc: %q owned by uid %d, not current user %d", name, st.Uid, os.Getuid())
	}
	return nil
}