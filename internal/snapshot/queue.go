package snapshot

import (
	"github.com/DataExMachina-dev/side-eye-go/internal/fifo"
	"github.com/DataExMachina-dev/side-eye-go/internal/framing"
)

type queueEntryKey struct {
	addr uintptr
	t    uint32
}

type queue struct {
	seen map[queueEntryKey]struct{}
	q    fifo.Queue[framing.QueueEntry]
}

func makeQueue() queue {
	return queue{
		seen: make(map[queueEntryKey]struct{}, 16<<10),
		q:    fifo.MakeQueue[framing.QueueEntry](),
	}
}

func (q *queue) Len() int {
	return q.q.Len()
}

func (q *queue) Pop() (r framing.QueueEntry, ok bool) {
	if q.Len() == 0 {
		return framing.QueueEntry{}, false
	}
	r = *q.q.PeekFront()
	q.q.PopFront()
	return r, true
}

func (q *queue) ShouldRecord(addr uintptr, t uint32) bool {
	if addr == 0 {
		return false
	}
	if _, ok := q.seen[queueEntryKey{addr: addr, t: t}]; ok {
		return false
	}
	q.seen[queueEntryKey{addr: addr, t: t}] = struct{}{}
	return true
}

// TODO: rethink this boolean return
func (q *queue) Push(addr uintptr, t uint32, dataLen uint32) bool {
	if q.ShouldRecord(addr, t) {
		q.q.PushBack(framing.QueueEntry{
			Addr: uint64(addr),
			Type: t,
			Len:  dataLen,
		})
	}
	return true
}
