package snapshot

import (
	"unsafe"

	"github.com/DataExMachina-dev/side-eye-go/internal/framing"
	"github.com/DataExMachina-dev/side-eye-go/internal/stoptheworld"
)

type outBuf struct {
	out    []byte
	isFull bool
}

func makeOutBuf(size uint32) outBuf {
	return outBuf{
		out:    make([]byte, 0, size),
		isFull: false,
	}
}

// GetEntryLen implements stackmachine.OutBuf.
func (o *outBuf) GetEntryLen(entryOffset uint32) uint32 {
	entry := (*framing.QueueEntry)(o.Ptr(entryOffset - uint32(unsafe.Sizeof(framing.QueueEntry{}))))
	return entry.Len
}

// PrepareFrameData implements stackmachine.OutBuf.
func (o *outBuf) PrepareFrameData(
	typeID uint32,
	progID uint32,
	dataLen uint32,
	depth uint32,
) (*framing.FrameHeader, uint32, bool) {
	paddedLen := dataLen
	rem := paddedLen % 8
	if rem != 0 {
		paddedLen += 8 - rem
	}
	frameHeaderOffset := o.Len()
	newLen := len(o.out) +
		int(unsafe.Sizeof(framing.FrameHeader{})) +
		int(unsafe.Sizeof(framing.QueueEntry{})) +
		int(paddedLen) +
		8
	if newLen > cap(o.out) {
		o.isFull = true
		return nil, 0, false
	}
	o.out = o.out[:newLen]
	queueEntryOffset := frameHeaderOffset + uint32(unsafe.Sizeof(framing.FrameHeader{}))
	var frameHeader *framing.FrameHeader = (*framing.FrameHeader)(o.Ptr(frameHeaderOffset))
	*frameHeader = framing.FrameHeader{
		// Actual length will be computed once frame is processed.
		DataByteLen: queueEntryOffset,
	}
	*(*framing.QueueEntry)(o.Ptr(queueEntryOffset)) = framing.QueueEntry{
		Type: typeID,
		Len:  dataLen + 8,
		Addr: 0,
	}
	offset := queueEntryOffset + uint32(unsafe.Sizeof(framing.QueueEntry{}))
	*(*uint32)(o.Ptr(offset + dataLen)) = progID
	*(*uint32)(o.Ptr(offset + dataLen + 4)) = depth
	return frameHeader, offset, true
}

func (o *outBuf) ConcludeFrameData(frameHeader *framing.FrameHeader) {
	frameHeader.DataByteLen = o.Len() - frameHeader.DataByteLen
}

// Ptr implements stackmachine.OutBuf.
//
// Note that it is assumed that the offset is valid.
func (o *outBuf) Ptr(offset uint32) unsafe.Pointer {
	return unsafe.Pointer(
		uintptr(unsafe.Pointer(unsafe.SliceData(o.out))) +
			uintptr(offset))
}

// Zero will zero the memory at the given offset for the given length.
//
// Note that it is assumed that the length fits in the buf.
func (o *outBuf) Zero(offset uint32, zeroLen uint32) {
	if offset+zeroLen > uint32(cap(o.out)) {
		return
	}
	toZero := o.out[offset : offset+zeroLen]
	for i := range toZero {
		toZero[i] = 0
	}
}

// Copy buffer data from one offset to the other. Ranges should not overlap.
//
// Note that it is assumed that the length fits in the buf.
func (o *outBuf) Copy(src uint32, dst uint32, len uint32) {
	if src+len > uint32(cap(o.out)) {
		return
	}
	if dst+len > uint32(cap(o.out)) {
		return
	}
	for i := uint32(0); i < len; i++ {
		o.out[dst+i] = o.out[src+i]
	}
}

// Dereference the pointer at the given offset for the given length.
// If the dereference operation fails, the memory in the buf is zeroed.
//
// Note that it is assumed that the length fits in the buf.
func (o *outBuf) Dereference(offset uint32, ptr uintptr, dereferenceLen uint32) (ok bool) {
	if offset+dereferenceLen > uint32(cap(o.out)) {
		return false
	}
	if !stoptheworld.Dereference(
		o.Ptr(offset),
		unsafe.Pointer(ptr),
		int(dereferenceLen),
	) {
		o.Zero(offset, dereferenceLen)
		return false
	}
	return true
}

// Len return the length of the outBuf in bytes.
func (o *outBuf) Len() uint32 {
	return uint32(len(o.out))
}

func (o *outBuf) EnsureLen(minLen uint32) (ok bool) {
	if minLen < o.Len() {
		return true
	}
	if minLen > uint32(cap(o.out)) {
		o.isFull = true
		return false
	}
	o.out = o.out[:minLen]
	return true
}

func (o *outBuf) data() []byte {
	return o.out
}

func (o *outBuf) writeSnapshotHeader() (*framing.SnapshotHeader, bool) {
	offset := o.Len()
	newLen := len(o.out) + int(unsafe.Sizeof(framing.SnapshotHeader{}))
	if newLen > cap(o.out) {
		o.isFull = true
		return nil, false
	}
	o.out = o.out[:newLen]
	return (*framing.SnapshotHeader)(o.Ptr(offset)), true
}

// writeGoroutineHeader extends the outBuf to include a new goroutine header.
// If there is not enough room, false is returned.
func (o *outBuf) writeGoroutineHeader() (*framing.GoroutineHeader, bool) {
	offset := o.Len()
	newLen := offset + uint32(unsafe.Sizeof(framing.GoroutineHeader{}))
	if !o.EnsureLen(newLen) {
		return nil, false
	}
	return (*framing.GoroutineHeader)(o.Ptr(offset)), true
}

func (o *outBuf) full() bool {
	return o.isFull
}

func (o *outBuf) truncate(offset uint32) {
	o.out = o.out[:offset]
}

// writeQueueEntry extends the outBuf to include a new queue entry header, and
// dereferences its data. If either the outBuf is full or the dereference fails,
// the buffer is unmodified.
func (o *outBuf) writeQueueEntry(entry framing.QueueEntry) (dataOffset uint32, ok bool) {
	origLen := o.Len()
	headerOffset := origLen
	paddedLen := entry.Len
	rem := paddedLen % 8
	if rem != 0 {
		paddedLen += 8 - rem
	}
	newLen := headerOffset + paddedLen + uint32(unsafe.Sizeof(framing.QueueEntry{}))
	if !o.EnsureLen(newLen) {
		return 0, false
	}
	*(*framing.QueueEntry)(o.Ptr(headerOffset)) = entry
	dataOffset = headerOffset + uint32(unsafe.Sizeof(framing.QueueEntry{}))
	if !o.Dereference(dataOffset, uintptr(entry.Addr), entry.Len) {
		(*framing.QueueEntry)(o.Ptr(headerOffset)).Type |= (1 << 31)
		return 0, false
	}
	return dataOffset, true
}

func (o *outBuf) writeStack(stack []uintptr) (uint32, bool) {
	offset := o.Len()
	byteLen := uint32(len(stack)) * uint32(unsafe.Sizeof(uintptr(0)))
	newLen := offset + byteLen
	if !o.EnsureLen(newLen) {
		return 0, false
	}
	copy(o.out[offset:], unsafe.Slice((*byte)(unsafe.Pointer(&stack[0])), byteLen))
	return byteLen, true
}
