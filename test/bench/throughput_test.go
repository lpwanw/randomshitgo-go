// Package bench contains throughput benchmarks for the procs log pipeline.
package bench

import (
	"fmt"
	"testing"

	"github.com/lpwanw/randomshitgo-go/internal/log"
)

// BenchmarkRingBufferPush measures raw push throughput for RingBuffer.
func BenchmarkRingBufferPush(b *testing.B) {
	rb := log.NewRingBuffer[log.Line](1000)
	line := log.Line{Bytes: []byte("benchmark line content")}
	b.ResetTimer()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		rb.Push(line)
	}
}

// BenchmarkLogFanin simulates 5 children each pushing lines at full speed,
// measuring the ring buffer's concurrency under write load.
func BenchmarkLogFanin(b *testing.B) {
	const numChildren = 5
	rb := log.NewRingBuffer[log.Line](10000)

	// Pre-build lines to avoid allocation in the measured path.
	lines := make([]log.Line, numChildren)
	for i := range lines {
		lines[i] = log.Line{Bytes: []byte(fmt.Sprintf("child-%d: log line content for throughput test", i))}
	}

	b.ResetTimer()
	b.ReportAllocs()
	b.SetParallelism(numChildren)
	b.RunParallel(func(pb *testing.PB) {
		// Each parallel goroutine represents one child.
		i := 0
		for pb.Next() {
			rb.Push(lines[i%numChildren])
			i++
		}
	})
}

// BenchmarkLineBufferFeed measures how fast raw bytes are split into lines.
func BenchmarkLineBufferFeed(b *testing.B) {
	lb := log.NewLineBuffer(0)
	// Simulate a chunk of PTY output with multiple lines.
	chunk := []byte("line one\nline two\nline three\nline four\nline five\n")
	b.ResetTimer()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		lb.Feed(chunk)
	}
}
