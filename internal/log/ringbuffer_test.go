package log

import (
	"sync"
	"testing"
)

func TestRingBufferPushAndSnapshot(t *testing.T) {
	r := NewRingBuffer[int](3)
	for i := 1; i <= 5; i++ {
		r.Push(i)
	}
	snap := r.Snapshot()
	if len(snap) != 3 || snap[0] != 3 || snap[1] != 4 || snap[2] != 5 {
		t.Fatalf("wrap broken: %v", snap)
	}
	if r.Len() != 3 {
		t.Fatalf("len: %d", r.Len())
	}
	if r.Generation() != 5 {
		t.Fatalf("gen: %d", r.Generation())
	}
}

func TestRingBufferTail(t *testing.T) {
	r := NewRingBuffer[int](5)
	for i := 1; i <= 5; i++ {
		r.Push(i)
	}
	if got := r.Tail(3); !equalInts(got, []int{3, 4, 5}) {
		t.Fatalf("tail: %v", got)
	}
	if got := r.Tail(10); !equalInts(got, []int{1, 2, 3, 4, 5}) {
		t.Fatalf("tail>len: %v", got)
	}
}

func TestRingBufferPushMany(t *testing.T) {
	r := NewRingBuffer[int](3)
	r.PushMany([]int{1, 2, 3, 4, 5, 6, 7})
	if got := r.Snapshot(); !equalInts(got, []int{5, 6, 7}) {
		t.Fatalf("pushmany overflow: %v", got)
	}
	if r.Generation() != 7 {
		t.Fatalf("gen: %d", r.Generation())
	}
}

func TestRingBufferConcurrent(t *testing.T) {
	r := NewRingBuffer[int](100)
	var wg sync.WaitGroup
	for w := 0; w < 8; w++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for i := 0; i < 1000; i++ {
				r.Push(i)
			}
		}()
	}
	for s := 0; s < 8; s++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for i := 0; i < 1000; i++ {
				_ = r.Snapshot()
			}
		}()
	}
	wg.Wait()
	if r.Len() != 100 {
		t.Fatalf("len after churn: %d", r.Len())
	}
}

func TestRingBufferClear(t *testing.T) {
	r := NewRingBuffer[int](3)
	r.Push(1)
	r.Push(2)
	r.Clear()
	if r.Len() != 0 || len(r.Snapshot()) != 0 {
		t.Fatal("clear failed")
	}
}

func equalInts(a, b []int) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}
