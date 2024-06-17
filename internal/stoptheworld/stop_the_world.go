// Package stoptheworld contains logic to stop the world.
package stoptheworld

import (
	"runtime"
	"sync"
	"unsafe"

	"github.com/DataExMachina-dev/side-eye-go/internal/snapshotpb"
)

// StopTheWorld is a magical function that calls f with the world stopped.
//
// The function must not panic or perform any IO or blocking operations.
//
// Additionally, if the function wants to read unsafe memory, it must use
// the Dereference function. This function interacts with signal handlers
// that are set up by this call.
func StopTheWorld(config *snapshotpb.RuntimeConfig, f func()) {

	// TODO: Add another layer of protection to ensure that this function can
	// recover from unexpected segfaults, including in the signal handler.
	//
	// One approach to do this would be to create another call frame and inside
	// of that call frame, work out the offset to its frame pointer from the
	// stack root and stash that somewhere along with the g pointer itself.
	// Then, if we get a segfault, we can use that offset and the stack base in
	// the g to work out the corrected frame pointer and simulate returning from
	// that function.
	//
	// We'd want to take care to ensure that no defer underneath that function
	// needs to run.

	runtime.LockOSThread()
	defer runtime.UnlockOSThread()

	state.mu.Lock()
	defer state.mu.Unlock()

	state.dereferenceStart = uintptr(config.DereferenceStartPc)
	state.dereferenceEnd = uintptr(config.DereferenceEndPc)

	setHandler()
	defer resetHandler()

	ws := stopTheWorld(stwStartTrace)
	defer startTheWorld(ws)

	f()
}

var state = signalState{}

type signalState struct {
	mu                               sync.Mutex
	dereferenceStart, dereferenceEnd uintptr
	prevAction                       sigactiont
	snapshotTid                      uint32
}

// Used in assembly.
//
// These are the flags to set on out sigactiont struct in setHandler.
const saFlags = _SA_ONSTACK | _SA_SIGINFO | _SA_NODEFER | _SA_RESTORER

// Set the signal handler to one that can gracefully recover from a segfault
// in the Dereference function. Save the old signal handler to restore later.
//
// Must be called with state.mu locked.
func setHandler()

// Restore the old signal handler.
//
// Must be called with state.mu locked.
func resetHandler()

func sigreturn__sigaction()

func sigsegvHandler()

// Dereference can be used to read the memory at an address into the
// provided byte slice. The dst slice must be of sufficient length.
//
//go:noinline
func Dereference(dst []byte, ptr uintptr, byteLen int) (ok bool) {
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
	return dereferenceInner(dst, ptr, byteLen)
}

//go:noinline
//go:nosplit
func dereferenceInner(dst []byte, ptr uintptr, byteLen int) (ok bool) {
	// unsafe.Slice overflow checking can lead to a static panic if ptr is close to overflowing.
	if uintptr(ptr)+uintptr(byteLen) < uintptr(ptr) {
		return false
	}
	// Defeat the copy bounds checks. !!!
	if len(dst) < byteLen {
		return false
	}
	copy(dst, unsafe.Slice((*byte)(unsafe.Pointer(ptr)), byteLen))
	return true
}
