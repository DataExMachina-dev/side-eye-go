// Copyright 2024 The Cockroach Authors.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or
// implied. See the License for the specific language governing
// permissions and limitations under the License.

// Taken from https://github.com/cockroachdb/fifo/blob/0bbfbd93/queue.go

package fifo

// Queue implements an allocation efficient FIFO queue. It is not safe for
// concurrent access.
//
// Note that the queue provides pointer access to the internal storage (via
// PeekFront and PushBack) so it must be used with care. These pointers must not
// be used once the respective element is popped out of the queue.
//
// -- Implementation --
//
// The queue is implemented as a linked list of nodes, where each node is a
// small ring buffer. The nodes are allocated using a sync.Pool (a single pool
// is created for any given type and is used for all queues of that type).
type Queue[T any] struct {
	len        int
	head, tail *queueNode[T]
	freeList[T]
}

type freeList[T any] struct {
	head *queueNode[T]
}

func (f *freeList[T]) get() *queueNode[T] {
	if f.head == nil {
		return new(queueNode[T])
	}
	qn := f.head
	qn.next = nil
	f.head = qn.next
	return qn
}

func (f *freeList[T]) put(qn *queueNode[T]) {
	qn.head = 0
	qn.len = 0
	qn.next = f.head
	f.head = qn
}

// MakeQueue constructs a new Queue.
//
// The pool should be a singleton object initialized with MakeQueueBackingPool.
// A single pool can and should be used by all queues of that type.
func MakeQueue[T any]() Queue[T] {
	return Queue[T]{}
}

// Len returns the current length of the queue.
func (q *Queue[T]) Len() int {
	return q.len
}

// PushBack adds t to the end of the queue.
//
// The returned pointer can be used to modify the element while it is in the
// queue; it is valid until the element is removed from the queue.
func (q *Queue[T]) PushBack(t T) *T {
	if q.head == nil {
		q.head = q.freeList.get()
		q.tail = q.head
	} else if q.tail.IsFull() {
		newTail := q.freeList.get()
		q.tail.next = newTail
		q.tail = newTail
	}
	q.len++
	return q.tail.PushBack(t)
}

// PeekFront returns the current head of the queue, or nil if the queue is
// empty.
//
// The result is only valid until the next call to PopFront.
func (q *Queue[T]) PeekFront() *T {
	if q.len == 0 {
		return nil
	}
	return q.head.PeekFront()
}

// PopFront removes the current head of the queue.
//
// It is illegal to call PopFront on an empty queue.
func (q *Queue[T]) PopFront() {
	q.head.PopFront()
	if q.head.len == 0 {
		oldHead := q.head
		q.head = oldHead.next
		q.freeList.put(oldHead)
	}
	q.len--
}

// We batch the allocation of this many queue objects. The value was chosen
// without experimentation - it provides a reasonable amount of amortization
// without a very large increase in memory overhead if T is large.
const queueNodeSize = 128

type queueNode[T any] struct {
	buf       [queueNodeSize]T
	head, len int32
	next      *queueNode[T]
}

func (qn *queueNode[T]) IsFull() bool {
	return qn.len == queueNodeSize
}

func (qn *queueNode[T]) PushBack(t T) *T {
	i := (qn.head + qn.len) % queueNodeSize
	qn.buf[i] = t
	qn.len++
	return &qn.buf[i]
}

func (qn *queueNode[T]) PeekFront() *T {
	return &qn.buf[qn.head]
}

func (qn *queueNode[T]) PopFront() T {
	t := qn.buf[qn.head]
	var zero T
	qn.buf[qn.head] = zero
	qn.head = (qn.head + 1) % queueNodeSize
	qn.len--
	return t
}
