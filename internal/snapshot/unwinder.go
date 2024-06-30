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
	frameBuf      callFrame
	frameBufSlice []byte
}

type callFrame struct {
	fp uintptr
	pc uintptr
}

func newUnwinder(base uintptr) *unwinder {
	uw := new(unwinder)
	uw.base = base
	uw.frameBufSlice = unsafe.Slice(
		(*byte)(unsafe.Pointer(&uw.frameBuf)),
		unsafe.Sizeof(callFrame{}),
	)
	return uw
}

func (b *unwinder) walkStack(pc uintptr, fp uintptr, stackTopSP uintptr) (pcs []uintptr, cfa []uintptr) {
	b.pcBuf[0] = pc
	b.fpBuf[0] = fp
	b.numFrames = 1
	if fp == 0 {
		return b.pcBuf[:b.numFrames], b.fpBuf[:b.numFrames]
	}
	for ; b.numFrames < maxStackFrames; b.numFrames++ {
		if b.fpBuf[b.numFrames-1] == 0 {
			break
		}
		nextCallFrame := b.fpBuf[b.numFrames-1]
		if !stoptheworld.Dereference(
			b.frameBufSlice,
			nextCallFrame,
			int(unsafe.Sizeof(callFrame{})),
		) {
			break
		}

		b.fpBuf[b.numFrames] = b.frameBuf.fp
		b.pcBuf[b.numFrames] = b.frameBuf.pc
	}
	for i := 0; i < b.numFrames; i++ {
		b.pcBuf[i] -= b.base
	}
	return b.pcBuf[:b.numFrames], adjustCFA(b.fpBuf[:b.numFrames], stackTopSP)
}
