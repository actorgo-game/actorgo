package cqueue

import (
	"fmt"
	"math/rand"
	"sync"
	"testing"
	"time"
)

func TestQueue(t *testing.T) {
	num := 3

	q := NewQueue()
	for i := range num {
		q.Push(i)
	}

	for range num {
		fmt.Println(q.Pop())
	}

}

func BenchmarkNewFIFOQueue(b *testing.B) {
	q := NewQueue()
	for i := range 1000000 {
		q.Push(i)
	}

	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		q.Push(i)
		q.Pop()
	}
}

func TestQueuePop(t *testing.T) {
	num := 10

	q := NewQueue()
	for i := range num {
		q.Push(i)
	}

	go func() {
		for {
			time.Sleep(10 * time.Millisecond)
			q.Push(rand.Int31n(10000))
		}
	}()

	go func() {
		postTicker := time.NewTicker(5 * time.Millisecond)
		postNum := 100

		for {
			<-postTicker.C

			for range postNum {
				v := q.Pop()
				if v == nil {
					break
				}
				fmt.Println(v)
			}
		}
	}()

	wg := &sync.WaitGroup{}
	wg.Add(1)
	wg.Wait()
}
