package allgs

import (
	"fmt"
	"sync/atomic"
	"unsafe"
	_ "unsafe"

	"github.com/DataExMachina-dev/side-eye-go/internal/snapshotpb"
)

// Goroutine is a wrapper around *runtime.g with a pointer to the runtime config
// derived from DWARF.
type Goroutine struct {
	gPtr   unsafe.Pointer
	config *snapshotpb.RuntimeConfig
}

// ForEach calls f for each goroutine.
//
// Note that the current implementation uses runtime.forEachG which acquires a runtime
// lock. This means that the function f must not panic.
func ForEach(cfg *snapshotpb.RuntimeConfig, f func(Goroutine)) {
	forEachG(func(gPtr unsafe.Pointer) {
		f(Goroutine{gPtr: gPtr, config: cfg})
	})
}

//go:linkname forEachG runtime.forEachG
func forEachG(func(pointer unsafe.Pointer))

// Status is the status of a goroutine.
type Status uint32

func (s Status) String() string {
	return gStatusStrings[s]
}

// PC returns the program counter of the goroutine.
func (g Goroutine) PC() uintptr {
	return *(*uintptr)(unsafe.Pointer(uintptr(g.gPtr) + uintptr(g.config.GGoBufPcOffset)))
}

// BP returns the program counter of the goroutine.
//
// TODO(https://github.com/DataExMachina-dev/side-eye/issues/756): Note that
// this is currently buggy when the goroutine is in a syscall. In later go versions
// (go1.23+), callers should be able to use syscallbp.
func (g Goroutine) BP() uintptr {
	return *(*uintptr)(unsafe.Pointer(uintptr(g.gPtr) + uintptr(g.config.GGoBufBpOffset)))
}

// SyscallPC returns the program counter of the syscall.
func (g Goroutine) SyscallPC() uintptr {
	return *(*uintptr)(unsafe.Pointer(uintptr(g.gPtr) + uintptr(g.config.GSyscallPcOffset)))
}

// Goid returns the ID of the goroutine.
func (g Goroutine) Goid() int64 {
	return *(*int64)(unsafe.Pointer(uintptr(g.gPtr) + uintptr(g.config.GGoidOffset)))
}

// Status returns the status of the goroutine.
func (g Goroutine) Status() Status {
	status := (*uint32)(unsafe.Pointer(uintptr(g.gPtr) + uintptr(g.config.GAtomicstatusOffset)))
	return Status(atomic.LoadUint32(status))
}

func (g Goroutine) String() string {
	return fmt.Sprintf("{Goid: %d, Status: %s}", g.Goid(), g.Status())
}

// Stktopsp returns the top of the stack of the goroutine.
func (g Goroutine) Stktopsp() uintptr {
	return *(*uintptr)(unsafe.Pointer(uintptr(g.gPtr) + uintptr(g.config.GStktopspOffset)))
}
