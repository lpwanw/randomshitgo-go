package log

import "sync"

// RingBuffer is a fixed-capacity circular buffer. O(1) push, O(1) eviction.
// Generation counter lets subscribers detect changes even when Len is stable.
type RingBuffer[T any] struct {
	mu       sync.RWMutex
	buf      []T
	cap      int
	head     int
	count    int
	gen      int64
}

func NewRingBuffer[T any](cap int) *RingBuffer[T] {
	if cap < 1 {
		cap = 1
	}
	return &RingBuffer[T]{buf: make([]T, cap), cap: cap}
}

func (r *RingBuffer[T]) Push(v T) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.pushLocked(v)
}

func (r *RingBuffer[T]) PushMany(vs []T) {
	if len(vs) == 0 {
		return
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if len(vs) >= r.cap {
		offset := len(vs) - r.cap
		for i := 0; i < r.cap; i++ {
			r.buf[i] = vs[offset+i]
		}
		r.head = 0
		r.count = r.cap
		r.gen += int64(len(vs))
		return
	}
	for _, v := range vs {
		r.pushLocked(v)
	}
}

func (r *RingBuffer[T]) pushLocked(v T) {
	w := (r.head + r.count) % r.cap
	r.buf[w] = v
	if r.count < r.cap {
		r.count++
	} else {
		r.head = (r.head + 1) % r.cap
	}
	r.gen++
}

func (r *RingBuffer[T]) Len() int {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return r.count
}

func (r *RingBuffer[T]) Generation() int64 {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return r.gen
}

// Snapshot returns all items in order, oldest first. Returns a fresh slice.
func (r *RingBuffer[T]) Snapshot() []T {
	r.mu.RLock()
	defer r.mu.RUnlock()
	out := make([]T, r.count)
	for i := 0; i < r.count; i++ {
		out[i] = r.buf[(r.head+i)%r.cap]
	}
	return out
}

// Tail returns the last n items (or all if fewer).
func (r *RingBuffer[T]) Tail(n int) []T {
	r.mu.RLock()
	defer r.mu.RUnlock()
	if n > r.count {
		n = r.count
	}
	start := r.count - n
	out := make([]T, n)
	for i := 0; i < n; i++ {
		out[i] = r.buf[(r.head+start+i)%r.cap]
	}
	return out
}

func (r *RingBuffer[T]) Clear() {
	r.mu.Lock()
	defer r.mu.Unlock()
	var zero T
	for i := range r.buf {
		r.buf[i] = zero
	}
	r.head = 0
	r.count = 0
}
