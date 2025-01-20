//go:build !linux
// +build !linux

package boottime

import (
	"time"
)

func bootTime() (time.Time, error) {
	return time.Time{}, ErrNotImplemented
}
