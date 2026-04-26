//go:build !windows

package process

import (
	"bytes"
	"context"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/lpwanw/randomshitgo-go/internal/config"
	"github.com/lpwanw/randomshitgo-go/internal/log"
)

// safeBuf is a thread-safe bytes.Buffer for tee tests.
type safeBuf struct {
	mu sync.Mutex
	bytes.Buffer
}

func (s *safeBuf) Write(p []byte) (int, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.Buffer.Write(p)
}

func (s *safeBuf) String() string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.Buffer.String()
}

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

func TestChild_SubscribeTeeReceivesPTYBytes(t *testing.T) {
	events := make(chan Event, 64)
	reg := newCaptureRegistry()

	proj := config.Project{
		Path:    t.TempDir(),
		Cmd:     "echo hello-tee",
		Restart: config.RestartNever,
	}
	child := newChild("subtest", proj, makeTestSettings(), reg, events, &noopWG{})

	if err := child.Start(context.Background()); err != nil {
		t.Fatalf("Start: %v", err)
	}

	var subBuf safeBuf
	unsub := child.Subscribe(&subBuf)
	defer unsub()

	// Drain events until ExitedEvent.
	deadline := time.After(5 * time.Second)
loop:
	for {
		select {
		case ev := <-events:
			if _, ok := ev.(ExitedEvent); ok {
				break loop
			}
		case <-deadline:
			t.Fatal("timed out")
		}
	}

	got := subBuf.String()
	if !strings.Contains(got, "hello-tee") {
		t.Fatalf("subscriber did not receive PTY bytes; got %q", got)
	}
	// Log ring should also have received the same bytes.
	if reg.ring.Len() == 0 {
		t.Fatal("log ring missed bytes — fanout broke ring path")
	}
}

func TestChild_SubscribeUnsubscribeStopsDelivery(t *testing.T) {
	events := make(chan Event, 64)
	reg := newCaptureRegistry()

	proj := config.Project{
		Path:    t.TempDir(),
		Cmd:     "for i in 1 2 3 4 5; do echo l$i; sleep 0.01; done",
		Restart: config.RestartNever,
	}
	child := newChild("unsub", proj, makeTestSettings(), reg, events, &noopWG{})

	if err := child.Start(context.Background()); err != nil {
		t.Fatalf("Start: %v", err)
	}

	var subBuf safeBuf
	unsub := child.Subscribe(&subBuf)

	// Wait for at least one log event to know data is flowing.
	for {
		select {
		case ev := <-events:
			if _, ok := ev.(LogLineEvent); ok {
				goto unsubscribe
			}
		case <-time.After(2 * time.Second):
			t.Fatal("no log event before unsub")
		}
	}
unsubscribe:
	unsub()
	beforeLen := len(subBuf.String())

	// Drain rest of process.
	deadline := time.After(3 * time.Second)
done:
	for {
		select {
		case ev := <-events:
			if _, ok := ev.(ExitedEvent); ok {
				break done
			}
		case <-deadline:
			t.Fatal("timed out")
		}
	}

	afterLen := len(subBuf.String())
	if afterLen != beforeLen {
		t.Fatalf("unsub did not stop deliveries: before=%d after=%d", beforeLen, afterLen)
	}
}
