package allgs

import (
	"fmt"
	"sync/atomic"
	"unsafe"

	"github.com/DataExMachina-dev/side-eye-go/internal/snapshotpb"
)

// Goroutine is a wrapper around *runtime.g with a pointer to the runtime config
// derived from DWARF.
type Goroutine struct {
	gPtr   unsafe.Pointer
	config *snapshotpb.RuntimeConfig
}

// GoroutineIterator iterates over all goroutines.
type GoroutineIterator struct {
	allGs unsafe.Pointer
	cfg   *snapshotpb.RuntimeConfig
}

// Iterate calls f for each goroutine.
func (it GoroutineIterator) Iterate(f func(Goroutine)) {
	allGs := *(*[]uintptr)(it.allGs)
	for _, gPtr := range allGs {
		f(Goroutine{gPtr: unsafe.Pointer(gPtr), config: it.cfg})
	}
}

// NewGoroutineIterator creates a new GoroutineIterator given the actual address
// of the bss section.
func NewGoroutineIterator(cfg *snapshotpb.RuntimeConfig, bssAddrShift uint64) (GoroutineIterator, error) {
	if cfg.VariableRuntimeDotAllgs == 0 || cfg.GoRuntimeBssAddress == 0 ||
		cfg.VariableRuntimeDotAllgs < cfg.GoRuntimeBssAddress {
		return GoroutineIterator{}, fmt.Errorf("invalid runtime config: missing allgs or bss section address")
	}
	allGs := unsafe.Pointer(uintptr(cfg.VariableRuntimeDotAllgs + bssAddrShift))
	return GoroutineIterator{allGs: allGs, cfg: cfg}, nil
}

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
func (g Goroutine) BP() unsafe.Pointer {
	return *(*unsafe.Pointer)(unsafe.Pointer(uintptr(g.gPtr) + uintptr(g.config.GGoBufBpOffset)))
}

// SyscallPC returns the program counter of the syscall.
func (g Goroutine) SyscallPC() uintptr {
	return *(*uintptr)(unsafe.Pointer(uintptr(g.gPtr) + uintptr(g.config.GSyscallPcOffset)))
}

// Goid returns the ID of the goroutine.
func (g Goroutine) Goid() uint64 {
	return *(*uint64)(unsafe.Pointer(uintptr(g.gPtr) + uintptr(g.config.GGoidOffset)))
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
