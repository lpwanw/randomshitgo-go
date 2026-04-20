//go:build !darwin && !linux

package netinfo

import "errors"

// PortForPID is not supported on this OS.
func PortForPID(_ int) (int, error) {
	return 0, errors.New("netinfo: port-from-pid not supported on this OS")
}
