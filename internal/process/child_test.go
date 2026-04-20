//go:build !windows

package process

import (
	"context"
	"testing"
	"time"

	"github.com/taynguyen/procs/internal/config"
	"github.com/taynguyen/procs/internal/log"
)

// noopRegistry discards all writes.
type noopRegistry struct{}

func (n *noopRegistry) WriteRaw(_ string, _ []byte) {}

// noopWG is a no-op WaitGroup for tests that create children directly.
type noopWG struct{}

func (n *noopWG) Add(_ int) {}
func (n *noopWG) Done()     {}

// captureRegistry captures writes for assertion.
type captureRegistry struct {
	ring *log.RingBuffer[log.Line]
	line *log.LineBuffer
}

func newCaptureRegistry() *captureRegistry {
	return &captureRegistry{
		ring: log.NewRingBuffer[log.Line](100),
		line: log.NewLineBuffer(0),
	}
}

func (c *captureRegistry) WriteRaw(_ string, b []byte) {
	lines := c.line.Feed(b)
	c.ring.PushMany(lines)
}

func makeTestSettings() config.Settings {
	s := config.DefaultSettings
	s.PtyCols = 80
	s.PtyRows = 24
	s.ShutdownGraceMs = 500
	s.RestartBackoffMs = []int{50, 100}
	s.RestartMaxAttempts = 3
	s.RestartResetAfterMs = 60000
	return s
}

func TestChild_LogLinesAndExitEvent(t *testing.T) {
	events := make(chan Event, 64)
	reg := newCaptureRegistry()

	proj := config.Project{
		Path:    t.TempDir(),
		Cmd:     "for i in 1 2 3; do echo line $i; sleep 0.01; done",
		Restart: config.RestartNever,
	}
	child := newChild("test", proj, makeTestSettings(), reg, events, &noopWG{})

	ctx := context.Background()
	if err := child.Start(ctx); err != nil {
		t.Fatalf("Start: %v", err)
	}

	var logLines []LogLineEvent
	var exited *ExitedEvent

	deadline := time.After(5 * time.Second)
loop:
	for {
		select {
		case ev, ok := <-events:
			if !ok {
				break loop
			}
			switch e := ev.(type) {
			case LogLineEvent:
				if !e.IsPartial {
					logLines = append(logLines, e)
				}
			case ExitedEvent:
				exited = &e
				break loop
			}
		case <-deadline:
			t.Fatal("timed out waiting for child to exit")
		}
	}

	if len(logLines) < 3 {
		t.Fatalf("expected at least 3 log lines, got %d", len(logLines))
	}

	if exited == nil {
		t.Fatal("no ExitedEvent received")
	}
	if exited.Code != 0 {
		t.Fatalf("expected exit code 0, got %d", exited.Code)
	}

	// Registry ring should also have entries.
	if reg.ring.Len() < 3 {
		t.Fatalf("ring buffer should have ≥3 entries, got %d", reg.ring.Len())
	}
}
