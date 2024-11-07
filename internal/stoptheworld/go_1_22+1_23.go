//go:build (go1.22 && !go1.23) || (go1.23 && !go1.24)

package stoptheworld

import (
	"unsafe"
)

func GoVersionSupported() bool {
	return true
}

var (
	stopTheWorldImpl  stopTheWorldFunc
	startTheWorldImpl startTheWorldFunc
)

func stopTheWorld(reason stwReason) worldStop {
	fPtr := (*uintptr)(unsafe.Pointer(&stopTheWorldImpl))
	*fPtr = (uintptr)(unsafe.Pointer(&state.stopTheWorldAddr))
	return stopTheWorldImpl(reason)
}

func startTheWorld(ws worldStop) {
	fPtr := (*uintptr)(unsafe.Pointer(&startTheWorldImpl))
	*fPtr = (uintptr)(unsafe.Pointer(&state.startTheWorldAddr))
	startTheWorldImpl(ws)
}

type stwReason int8

// https://github.com/golang/go/blob/3a842931/src/runtime/proc.go#L1310-L1315
type worldStop struct {
	reason stwReason
	start  int64
}

type stopTheWorldFunc func(reason stwReason) worldStop
type startTheWorldFunc func(ws worldStop)

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
