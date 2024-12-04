// Package framing contains type definitions for the framing protocol in the message
// buffer. It is a binary protocol copied from framing.h.
package framing

type SnapshotHeader struct {
	DataByteLen       uint32
	GoroutinesByteLen uint32
	Statistics        Statistics
	KTimeNS           uint64
}

type Statistics struct {
	StacksDurationNs  uint64
	PointerDurationNs uint64
	TotalDurationNs   uint64
	NumGoroutines     uint32
	NonLiveGoroutines uint32
}

type GoroutineHeader struct {
	Goid           int64
	StackHash      uint64
	Status         uint32
	WaitReason     uint8
	WaitSinceNanos int64
	StackBytes     uint32
	DataByteLen    uint32
}

type QueueEntry struct {
	Type uint32
	Len  uint32
	Addr uint64
}

type FrameHeader struct {
	DataByteLen uint32
	_           uint32
}
