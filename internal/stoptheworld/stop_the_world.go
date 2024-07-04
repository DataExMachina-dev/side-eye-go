// Package stoptheworld contains logic to stop the world.
package stoptheworld

import (
	"runtime"
	"sync"
	"unsafe"

	"github.com/DataExMachina-dev/side-eye-go/internal/snapshotpb"
)

var dereference = Dereference
var dereferenceAddr = **(**uintptr)(unsafe.Pointer(&dereference))

// StopTheWorld is a magical function that calls f with the world stopped.
//
// The function must not panic or perform any IO or blocking operations,
// and may not defer any functions.
//
// Additionally, if the function wants to read unsafe memory, it must use
// the Dereference function. This function interacts with signal handlers
// that are set up by this call.
//
//go:noinline
func StopTheWorld(config *snapshotpb.RuntimeConfig, f func()) (ok bool) {
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()

	base := ComputeTextSectionBaseOffset(config)

	state.mu.Lock()
	defer state.mu.Unlock()

	state.setConfig(config, base)
	defer state.clearConfig()

	setHandler()
	defer resetHandler()

	ws := stopTheWorld(stwStartTrace)
	defer startTheWorld(ws)

	defer clearRecoveryState()
	return stopTheWorldWrapper(f)
}

// This wrapper function is used by the signal handler as a sort of longjmp
// target. The setRecoveryState call sets up the state for the signal handler
// to recover to the return of this function.
//
//go:noinline
func stopTheWorldWrapper(f func()) bool {
	setRecoveryState()
	f()
	return true
}

// Used in assembly.
//
//lint:ignore U1000 this is used in assembly.
type config = snapshotpb.RuntimeConfig

// ComputeTextSectionBaseOffset computes the base offset of the text section
// from its base address in the object file. This can be non-zero when aslr
// is enabled.
func ComputeTextSectionBaseOffset(config *snapshotpb.RuntimeConfig) uintptr {
	return dereferenceAddr - uintptr(config.DereferenceStartPc)
}

var state = signalState{}

type signalState struct {
	mu                               sync.Mutex
	dereferenceStart, dereferenceEnd uintptr

	//lint:ignore U1000 this is used in assembly.
	prevAction sigaction
	//lint:ignore U1000 this is used in assembly.
	snapshotTid uint32
	//lint:ignore U1000 this is read from assembly.
	gPtr unsafe.Pointer

	// Offset from the stack top to the base of the recovery frame.
	recoveryFrameBaseOffset uintptr

	config *snapshotpb.RuntimeConfig
}

func (s *signalState) setConfig(
	config *snapshotpb.RuntimeConfig,
	base uintptr,
) {
	s.config = config
	s.dereferenceStart = uintptr(config.DereferenceStartPc) + base
	s.dereferenceEnd = uintptr(config.DereferenceEndPc) + base
}

func (s *signalState) clearConfig() {
	s.config = nil
	s.dereferenceStart = 0
	s.dereferenceEnd = 0
}

// Set the signal handler to one that can gracefully recover from a segfault
// in the Dereference function. Save the old signal handler to restore later.
//
// Must be called with state.mu locked.
func setHandler()

// Restore the old signal handler.
//
// Must be called with state.mu locked.
func resetHandler()

//lint:ignore U1000 this is used in assembly.
func sigreturn__sigaction()

//lint:ignore U1000 this is used in assembly.
func sigsegvHandler()

func setRecoveryState()

func clearRecoveryState() {
	state.recoveryFrameBaseOffset = 0
	state.gPtr = nil
}

// Dereference can be used to read the memory at an address into the
// provided address. The dst ptr must be of sufficient length.
//
//go:noinline
func Dereference(dst unsafe.Pointer, src unsafe.Pointer, byteLen int) (ok bool) {
	// NB: We need to call dereferenceInner in order to reliably
	// detect Dereference because memmove is a "frameless" function
	// meaning there'd be a portion of the body of Dereference which does
	// not lie in its own pc range. We'd need to detect this framelessness
	// using either some ad-hoc approach to notice the memmove call and
	// use its stack pointer as the frame pointer, or we'd need to
	// implement a more general table-based unwinding mechanism.
	//
	// For now, the dominant cost of dereferencing pointers are the cache
	// misses and not the instructions to call dereferenceInner.
	return dereferenceInner(dst, src, byteLen)
}

//go:linkname memmove runtime.memmove
func memmove(to, from unsafe.Pointer, n uintptr)

//go:noinline
//go:nosplit
func dereferenceInner(dst, src unsafe.Pointer, byteLen int) (ok bool) {
	memmove(dst, src, uintptr(byteLen))
	return true
}
