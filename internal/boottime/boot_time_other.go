//go:build !linux && !darwin
// +build !linux,!darwin

package boottime

import (
	"time"
)

func bootTime() (time.Time, error) {
	return time.Time{}, ErrNotImplemented
}
