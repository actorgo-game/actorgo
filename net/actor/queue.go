package cactor

import (
	"sync"
	"sync/atomic"
	"unsafe"
)

var queueNodePool = sync.Pool{
	New: func() any {
		return &queueNode{}
	},
}

type (
	queue struct {
		head, tail *queueNode
		C          chan int32
		count      int32
	}

	queueNode struct {
		next *queueNode
		val  any
	}
)

func newQueue() queue {
	stub := &queueNode{}
	q := queue{
		head:  stub,
		tail:  stub,
		C:     make(chan int32, 1),
		count: 0,
	}
	return q
}

func (p *queue) Push(v any) {
	if v == nil {
		return
	}

	n := queueNodePool.Get().(*queueNode)
	n.val = v
	n.next = nil
	// current producer acquires head node
	prev := (*queueNode)(atomic.SwapPointer((*unsafe.Pointer)(unsafe.Pointer(&p.head)), unsafe.Pointer(n)))

	// release node to consumer
	atomic.StorePointer((*unsafe.Pointer)(unsafe.Pointer(&prev.next)), unsafe.Pointer(n))

	p._setCount(1)
}

func (p *queue) Pop() any {
	tail := p.tail
	next := (*queueNode)(atomic.LoadPointer((*unsafe.Pointer)(unsafe.Pointer(&tail.next)))) // acquire
	if next != nil {
		p.tail = next
		v := next.val
		next.val = nil
		// recycle the consumed stub node (tail), not next
		tail.next = nil
		queueNodePool.Put(tail)
		p._setCount(-1)
		return v
	}

	return nil
}

func (p *queue) Empty() bool {
	tail := p.tail
	next := (*queueNode)(atomic.LoadPointer((*unsafe.Pointer)(unsafe.Pointer(&tail.next))))
	return next == nil
}

func (p *queue) Count() int32 {
	return atomic.LoadInt32(&p.count)
}

func (p *queue) _setCount(delta int32) {
	count := atomic.AddInt32(&p.count, delta)
	if count > 0 {
		select {
		case p.C <- count:
		default:
		}
	}
}

func (p *queue) Destroy() {
	close(p.C)
	p.head = nil
	p.tail = nil
	p.count = 0
}
