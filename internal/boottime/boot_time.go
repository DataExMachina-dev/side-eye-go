package boottime

import (
	"errors"
	"time"
)

// ErrNotImplemented is returned when the boot time is not implemented for the
// current platform.
var ErrNotImplemented = errors.New("not implemented")

// BootTime returns the approximate boot time of the system. The idea
// is that this timestamp can be used with readings of CLOCK_MONOTONIC
// to get a wall clock time.
func BootTime() (time.Time, error) {
	return bootTime()
}
