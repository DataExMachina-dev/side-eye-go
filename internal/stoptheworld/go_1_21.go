//go:build go1.21 && !go1.22

package stoptheworld

import (
	_ "unsafe" // for go:linkname
)

type worldStop struct{}

func startTheWorld(worldStop) {
	startTheWorldInner()
}

func stopTheWorld(reason stwReason) worldStop {
	stopTheWorldInner(reason)
	return worldStop{}
}

//go:linkname stopTheWorldInner runtime.stopTheWorld
func stopTheWorldInner(reason stwReason)

//go:linkname startTheWorldInner runtime.startTheWorld
func startTheWorldInner()

type stwReason int8

//go:linkname stwReasonString (runtime.stwReason).String
func stwReasonString(r stwReason)

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
