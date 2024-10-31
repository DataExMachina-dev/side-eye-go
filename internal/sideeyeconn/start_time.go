package sideeyeconn

import (
	"time"
)

// This serves as an approximate start time for the process. It is used as part
// of the process fingerprint.
var startTime = time.Now()

func getStartTime() time.Time {
	return startTime
}
