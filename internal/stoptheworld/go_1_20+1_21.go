// Contains logic for stopping the world in Go 1.20 and 1.21.

//go:build (go1.20 && !go1.21) || (go1.21 && !go1.22)

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
	stopTheWorldImpl(reason)
	return worldStop{}
}

func startTheWorld(ws worldStop) {
	fPtr := (*uintptr)(unsafe.Pointer(&startTheWorldImpl))
	*fPtr = (uintptr)(unsafe.Pointer(&state.startTheWorldAddr))
	startTheWorldImpl()
}

type worldStop struct{}

type stwReason int8

type stopTheWorldFunc func(reason stwReason)
type startTheWorldFunc func()

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
