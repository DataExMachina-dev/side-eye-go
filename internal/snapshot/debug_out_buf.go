package snapshot

import (
	"bytes"
	"fmt"
	"unsafe"

	"github.com/DataExMachina-dev/side-eye-go/internal/framing"
)

type debugOutBuf struct {
	b      outBuf
	logBuf bytes.Buffer
}

var _ outBufI = (*debugOutBuf)(nil)

func makeDebugOutBuf(size uint32) debugOutBuf {
	return debugOutBuf{
		b: makeOutBuf(size),
	}
}

// full implements outBufI.
func (d *debugOutBuf) full() bool {
	return d.b.full()
}

// truncate implements outBufI.
func (d *debugOutBuf) truncate(offset uint32) {
	d.b.truncate(offset)
	fmt.Fprintf(&d.logBuf, "truncate(%d)\n", offset)
}

// writeGoroutineHeader implements outBufI.
func (d *debugOutBuf) writeGoroutineHeader() (*framing.GoroutineHeader, bool) {
	h, ok := d.b.writeGoroutineHeader()
	fmt.Fprintf(&d.logBuf, "writeGoroutineHeader() = %p, %v\n", h, ok)
	return h, ok
}

// writeQueueEntry implements outBufI.
func (d *debugOutBuf) writeQueueEntry(entry framing.QueueEntry) (dataOffset uint32, ok bool) {
	dataOffset, ok = d.b.writeQueueEntry(entry)
	fmt.Fprintf(&d.logBuf, "writeQueueEntry(%#v) = %d, %v %v\n", entry, dataOffset, ok, d.full())
	return dataOffset, ok
}

// writeSnapshotHeader implements outBufI.
func (d *debugOutBuf) writeSnapshotHeader() (*framing.SnapshotHeader, bool) {
	return d.b.writeSnapshotHeader()
}

// Dereference implements stackmachine.OutBuf.
func (d *debugOutBuf) Dereference(offset uint32, ptr uintptr, len uint32) bool {
	ret := d.b.Dereference(offset, ptr, len)
	fmt.Fprintf(&d.logBuf, "Dereference(%d, %x, %d) = %v\n", offset, ptr, len, ret)
	return ret
}

// GetEntryLen implements stackmachine.OutBuf.
func (d *debugOutBuf) GetEntryLen(entryOffset uint32) uint32 {
	entryLen := d.b.GetEntryLen(entryOffset)
	fmt.Fprintf(&d.logBuf, "GetEntryLen(%d) = %d\n", entryOffset, entryLen)
	return entryLen
}

// Len implements stackmachine.OutBuf.
func (d *debugOutBuf) Len() uint32 {
	return d.b.Len()
}

// PrepareFrameData implements stackmachine.OutBuf.
func (d *debugOutBuf) PrepareFrameData(typeID uint32, progID uint32, dataLen uint32, depth uint32) (offset uint32, ok bool) {
	offset, ok = d.b.PrepareFrameData(typeID, progID, dataLen, depth)
	fmt.Fprintf(&d.logBuf, "PrepareFrameData(%d, %d, %d, %d) = %d, %v %v\n", typeID, progID, dataLen, depth, offset, ok, d.full())
	return offset, ok
}

// Ptr implements stackmachine.OutBuf.
func (d *debugOutBuf) Ptr(offset uint32) unsafe.Pointer {
	return d.b.Ptr(offset)
}

// Zero implements stackmachine.OutBuf.
func (d *debugOutBuf) Zero(offset uint32, len uint32) {
	d.b.Zero(offset, len)
}

func (d *debugOutBuf) writeStack(stack []uintptr) (uint32, bool) {
	return d.b.writeStack(stack)
}

func (d *debugOutBuf) data() []byte {
	return d.b.data()
}

func (d *debugOutBuf) String() string {
	return d.logBuf.String()
}
