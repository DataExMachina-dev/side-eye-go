package snapshot

import (
	"unsafe"

	"github.com/DataExMachina-dev/side-eye-go/internal/stoptheworld"
)

type unwinder struct {
	base      uintptr
	numFrames int
	pcBuf     [maxStackFrames]uintptr
	fpBuf     [maxStackFrames]uintptr

	// Used during unwinding to avoid allocations.
	frameBuf callFrame
}

type callFrame struct {
	fp unsafe.Pointer
	pc uintptr
}

func newUnwinder(base uintptr) *unwinder {
	uw := new(unwinder)
	uw.base = base
	return uw
}

func (b *unwinder) walkStack(pc uintptr, fp unsafe.Pointer, stackTopSP uintptr) (pcs []uintptr, cfa []uintptr) {
	b.pcBuf[0] = pc
	b.fpBuf[0] = uintptr(fp)
	b.numFrames = 1
	if fp == nil {
		return b.pcBuf[:b.numFrames], b.fpBuf[:b.numFrames]
	}
	nextCallFrame := fp
	for ; b.numFrames < maxStackFrames; b.numFrames++ {
		if nextCallFrame == nil {
			break
		}
		if !stoptheworld.Dereference(
			unsafe.Pointer(&b.frameBuf),
			nextCallFrame,
			int(unsafe.Sizeof(callFrame{})),
		) {
			// Make sure we don't have an invalid unsafe.Pointer sitting in the
			// frameBuf.
			b.frameBuf.fp = nil
			break
		}

		b.fpBuf[b.numFrames] = uintptr(b.frameBuf.fp)
		b.pcBuf[b.numFrames] = b.frameBuf.pc
		nextCallFrame = b.frameBuf.fp
	}
	for i := 0; i < b.numFrames; i++ {
		b.pcBuf[i] -= b.base
	}
	return b.pcBuf[:b.numFrames], adjustCFA(b.fpBuf[:b.numFrames], stackTopSP)
}
