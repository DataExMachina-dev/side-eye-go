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

// NOTE: On Darwin, the monotonic clock stops when the system is suspended, so
// bootTime() does not actually return the boot time. It does return, however,
// what Side-Eye needs: a base time to add to subsequent readings of the
// monotonic clock to get the correct wall clock times.
func bootTime() (time.Time, error) {
	now := time.Now()
	mono := runtimeNano()
	return now.Add(-time.Duration(mono)), nil
}
