//go:build darwin
// +build darwin

package boottime

import (
	"time"
	_ "unsafe" // required for go:linkname
)

// runtimeNano returns the current value of the runtime clock in nanoseconds.
//
//go:linkname runtimeNano runtime.nanotime
func runtimeNano() int64

func bootTime() (time.Time, error) {
	now := time.Now()
	mono := runtimeNano()
	return now.Add(-time.Duration(mono)), nil
}
