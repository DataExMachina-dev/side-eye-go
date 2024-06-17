package allgs

import (
	"fmt"
	"sync/atomic"
	"unsafe"
	_ "unsafe" // for go:linkname

	"github.com/DataExMachina-dev/side-eye-go/internal/snapshotpb"
)

type Goroutine struct {
	gPtr   unsafe.Pointer
	config *snapshotpb.RuntimeConfig
}

//go:linkname forEachG runtime.forEachG
func forEachG(func(pointer unsafe.Pointer))

type Status uint32

func (s Status) String() string {
	return gStatusStrings[s]
}

func (g Goroutine) PC() uintptr {
	return *(*uintptr)(unsafe.Pointer(uintptr(g.gPtr) + uintptr(g.config.GGoBufPcOffset)))
}

//go:inline
func (g Goroutine) BP() uintptr {
	return *(*uintptr)(unsafe.Pointer(uintptr(g.gPtr) + uintptr(g.config.GGoBufBpOffset)))
}

func (g Goroutine) SyscallPC() uintptr {
	return *(*uintptr)(unsafe.Pointer(uintptr(g.gPtr) + uintptr(g.config.GSyscallPcOffset)))
}

func (g Goroutine) Goid() int64 {
	return *(*int64)(unsafe.Pointer(uintptr(g.gPtr) + uintptr(g.config.GGoidOffset)))
}

func (g Goroutine) Status() Status {
	status := (*uint32)(unsafe.Pointer(uintptr(g.gPtr) + uintptr(g.config.GAtomicstatusOffset)))
	return Status(atomic.LoadUint32(status))
}

func (g Goroutine) String() string {
	return fmt.Sprintf("{Goid: %d, Status: %s}", g.Goid(), g.Status())
}

func ForEach(cfg *snapshotpb.RuntimeConfig, f func(Goroutine)) {
	forEachG(func(gPtr unsafe.Pointer) {
		f(Goroutine{gPtr: gPtr, config: cfg})
	})
}
