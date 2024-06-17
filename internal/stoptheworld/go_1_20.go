//go:build go1.20 && !go1.21

package stoptheworld

import (
	_ "unsafe" // for go:linkname
)

type worldStop struct{}

func startTheWorld(worldStop) {
	startTheWorldInner()
}

func stopTheWorld(reason stwReason) worldStop {
	reasonStr := stwReasonStrings[0]
	if reason < stwReason(len(stwReasonStrings)) {
		reasonStr = stwReasonStrings[reason]
	}
	stopTheWorldInner(reasonStr)
	return worldStop{}
}

//go:linkname stopTheWorldInner runtime.stopTheWorld
func stopTheWorldInner(reason string)

//go:linkname startTheWorldInner runtime.startTheWorld
func startTheWorldInner()

type stwReason int8

const (
	stwUnknown                     stwReason = iota // "unknown"
	stwGCMarkTerm                                   // "GC mark termination"
	stwGCSweepTerm                                  // "GC sweep termination"
	stwWriteHeapDump                                // "write heap dump"
	stwGoroutineProfile                             // "goroutine profile"
	stwGoroutineProfileCleanup                      // "goroutine profile cleanup"
	stwAllGoroutinesStack                           // "all goroutines stack trace"
	stwReadMemStats                                 // "read mem stats"
	stwAllThreadsSyscall                            // "AllThreadsSyscall"
	stwGOMAXPROCS                                   // "GOMAXPROCS"
	stwStartTrace                                   // "start trace"
	stwStopTrace                                    // "stop trace"
	stwForTestCountPagesInUse                       // "CountPagesInUse (test)"
	stwForTestReadMetricsSlow                       // "ReadMetricsSlow (test)"
	stwForTestReadMemStatsSlow                      // "ReadMemStatsSlow (test)"
	stwForTestPageCachePagesLeaked                  // "PageCachePagesLeaked (test)"
	stwForTestResetDebugLog                         // "ResetDebugLog (test)"
)

// If you add to this list, also add it to src/internal/trace/parser.go.
// If you change the values of any of the stw* constants, bump the trace
// version number and make a copy of this.
var stwReasonStrings = [...]string{
	stwUnknown:                     "unknown",
	stwGCMarkTerm:                  "GC mark termination",
	stwGCSweepTerm:                 "GC sweep termination",
	stwWriteHeapDump:               "write heap dump",
	stwGoroutineProfile:            "goroutine profile",
	stwGoroutineProfileCleanup:     "goroutine profile cleanup",
	stwAllGoroutinesStack:          "all goroutines stack trace",
	stwReadMemStats:                "read mem stats",
	stwAllThreadsSyscall:           "AllThreadsSyscall",
	stwGOMAXPROCS:                  "GOMAXPROCS",
	stwStartTrace:                  "start trace",
	stwStopTrace:                   "stop trace",
	stwForTestCountPagesInUse:      "CountPagesInUse (test)",
	stwForTestReadMetricsSlow:      "ReadMetricsSlow (test)",
	stwForTestReadMemStatsSlow:     "ReadMemStatsSlow (test)",
	stwForTestPageCachePagesLeaked: "PageCachePagesLeaked (test)",
	stwForTestResetDebugLog:        "ResetDebugLog (test)",
}
