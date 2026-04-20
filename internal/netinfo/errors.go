package netinfo

import "errors"

// ErrNoPort is returned when no LISTEN port can be found for the given PID.
var ErrNoPort = errors.New("netinfo: no listening port found")
