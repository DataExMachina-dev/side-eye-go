//go:build linux
// +build linux

package boottime

import (
	"syscall"
	"time"
	"unsafe"
)

func bootTime() (time.Time, error) {
	// https://github.com/torvalds/linux/blob/ffd294d3/include/uapi/linux/time.h#L49-L50
	const CLOCK_REALTIME = 0
	const CLOCK_MONOTONIC = 1

	var monotonic syscall.Timespec
	var wallTime syscall.Timespec
	syscall.Syscall(syscall.SYS_CLOCK_GETTIME, CLOCK_MONOTONIC, uintptr(unsafe.Pointer(&monotonic)), 0)
	syscall.Syscall(syscall.SYS_CLOCK_GETTIME, CLOCK_REALTIME, uintptr(unsafe.Pointer(&wallTime)), 0)
	nanos := wallTime.Nsec - monotonic.Nsec
	secs := wallTime.Sec - monotonic.Sec
	return time.Unix(secs, nanos), nil
}
