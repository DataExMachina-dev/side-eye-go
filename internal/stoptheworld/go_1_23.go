//go:build go1.23

package stoptheworld

import (
	_ "unsafe" // for go:linkname
)

//go:linkname stopTheWorld runtime.stopTheWorld
func stopTheWorld(reason stwReason) worldStop

//go:linkname startTheWorld runtime.startTheWorld
func startTheWorld(ws worldStop)

type stwReason int8

//go:linkname stwReasonString (runtime.stwReason).String
func stwReasonString(r stwReason)

// https://github.com/golang/go/blob/3a842931/src/runtime/proc.go#L1310-L1315
type worldStop struct {
	reason stwReason
	start  int64
}

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
