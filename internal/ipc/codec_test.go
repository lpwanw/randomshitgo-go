package ipc

import (
	"bytes"
	"encoding/binary"
	"errors"
	"io"
	"testing"
	"time"

	"github.com/lpwanw/randomshitgo-go/internal/event"
	"github.com/lpwanw/randomshitgo-go/internal/log"
)

// roundTrip encodes envs through an Encoder and decodes them back.
func roundTrip(t *testing.T, envs []*Envelope) []*Envelope {
	t.Helper()
	var buf bytes.Buffer
	enc := NewEncoder(&buf)
	for _, e := range envs {
		if err := enc.Write(e); err != nil {
			t.Fatalf("encode: %v", err)
		}
	}
	dec := NewDecoder(&buf)
	out := make([]*Envelope, 0, len(envs))
	for range envs {
		got, err := dec.Read()
		if err != nil {
			t.Fatalf("decode: %v", err)
		}
		out = append(out, got)
	}
	return out
}

func TestCodec_CommandRoundTrip(t *testing.T) {
	in := &Envelope{Kind: KindCommand, Command: &Command{Op: OpStartGroup, Group: "fullstack"}}
	got := roundTrip(t, []*Envelope{in})[0]
	if got.Kind != KindCommand || got.Command == nil {
		t.Fatalf("kind/command lost: %+v", got)
	}
	if got.Command.Op != OpStartGroup || got.Command.Group != "fullstack" {
		t.Fatalf("command mismatch: %+v", got.Command)
	}
}

func TestCodec_LogLineRawANSIPreserved(t *testing.T) {
	raw := []byte("\x1b[31mERROR\x1b[0m boom\twith\ttabs")
	in := &Envelope{Kind: KindEvent, Event: &EventMsg{
		Kind:    EvLogLine,
		LogLine: &event.LogLineEvent{ID: "web", Bytes: raw, IsPartial: true, At: time.Unix(1700000000, 0)},
	}}
	got := roundTrip(t, []*Envelope{in})[0]
	if got.Event == nil || got.Event.LogLine == nil {
		t.Fatalf("logline lost: %+v", got)
	}
	if !bytes.Equal(got.Event.LogLine.Bytes, raw) {
		t.Fatalf("raw ANSI bytes mangled:\n want %q\n got  %q", raw, got.Event.LogLine.Bytes)
	}
	if !got.Event.LogLine.IsPartial {
		t.Fatalf("IsPartial lost")
	}
}

func TestCodec_SnapshotMultiLine(t *testing.T) {
	lines := []log.Line{
		{Bytes: []byte("first"), Rendered: "first", IsPartial: false, Timestamp: time.Unix(1, 0)},
		{Bytes: []byte("\x1b[32msecond\x1b[0m"), Rendered: "second", IsPartial: false, Timestamp: time.Unix(2, 0)},
		{Bytes: []byte("partial-no-newline"), Rendered: "partial-no-newline", IsPartial: true, Timestamp: time.Unix(3, 0)},
	}
	in := &Envelope{Kind: KindSnapshot, Snapshot: &Snapshot{Projects: []ProjectSnap{
		{ID: "web", State: "running", PID: 4321, Lines: lines},
	}}}
	got := roundTrip(t, []*Envelope{in})[0]
	if got.Snapshot == nil || len(got.Snapshot.Projects) != 1 {
		t.Fatalf("snapshot lost: %+v", got)
	}
	p := got.Snapshot.Projects[0]
	if p.ID != "web" || p.State != "running" || p.PID != 4321 {
		t.Fatalf("project meta mismatch: %+v", p)
	}
	if len(p.Lines) != 3 {
		t.Fatalf("want 3 lines, got %d", len(p.Lines))
	}
	if !bytes.Equal(p.Lines[1].Bytes, lines[1].Bytes) || p.Lines[1].Rendered != "second" {
		t.Fatalf("line 2 mismatch: %+v", p.Lines[1])
	}
	if !p.Lines[2].IsPartial {
		t.Fatalf("partial flag lost on line 3")
	}
}

func TestCodec_MultipleFramesSharedTypeState(t *testing.T) {
	// Several frames over one encoder: type descriptors are sent once; all
	// frames must still decode in order.
	envs := []*Envelope{
		{Kind: KindEvent, Event: &EventMsg{Kind: EvStarted, Started: &event.StartedEvent{ID: "a", PID: 1}}},
		{Kind: KindEvent, Event: &EventMsg{Kind: EvStarted, Started: &event.StartedEvent{ID: "b", PID: 2}}},
		{Kind: KindEvent, Event: &EventMsg{Kind: EvExited, Exited: &event.ExitedEvent{ID: "a", Code: 0}}},
	}
	got := roundTrip(t, envs)
	if got[0].Event.Started.ID != "a" || got[1].Event.Started.ID != "b" {
		t.Fatalf("started ids out of order: %q %q", got[0].Event.Started.ID, got[1].Event.Started.ID)
	}
	if got[2].Event.Exited == nil || got[2].Event.Exited.ID != "a" {
		t.Fatalf("exited frame mismatch: %+v", got[2].Event)
	}
}

// oneByteReader serves at most one byte per Read, stressing the framedReader's
// boundary logic against the worst-case chunking a real socket can produce.
type oneByteReader struct{ r io.Reader }

func (o oneByteReader) Read(p []byte) (int, error) {
	if len(p) == 0 {
		return 0, nil
	}
	return o.r.Read(p[:1])
}

func TestCodec_ManyFramesByteChunked(t *testing.T) {
	const n = 200
	var buf bytes.Buffer
	enc := NewEncoder(&buf)
	for i := 0; i < n; i++ {
		env := &Envelope{Kind: KindEvent, Event: &EventMsg{
			Kind:    EvLogLine,
			LogLine: &event.LogLineEvent{ID: "p", Bytes: []byte{byte(i), '\x1b', '[', '0', 'm'}, At: time.Unix(int64(i), 0)},
		}}
		if err := enc.Write(env); err != nil {
			t.Fatalf("encode %d: %v", i, err)
		}
	}
	dec := NewDecoder(oneByteReader{r: &buf})
	for i := 0; i < n; i++ {
		got, err := dec.Read()
		if err != nil {
			t.Fatalf("decode %d: %v", i, err)
		}
		if got.Event == nil || got.Event.LogLine == nil || got.Event.LogLine.Bytes[0] != byte(i) {
			t.Fatalf("frame %d desynced: %+v", i, got.Event)
		}
	}
	if _, err := dec.Read(); err == nil {
		t.Fatalf("expected EOF after %d frames", n)
	}
}

func TestCodec_OversizeFrameRejectedBeforeAlloc(t *testing.T) {
	// Forge a 4-byte length prefix larger than the cap, followed by no body.
	// The decoder must reject on the header without trying to allocate.
	var buf bytes.Buffer
	var hdr [4]byte
	binary.BigEndian.PutUint32(hdr[:], MaxFrameSize+1)
	buf.Write(hdr[:])

	dec := NewDecoder(&buf)
	_, err := dec.Read()
	if !errors.Is(err, ErrFrameTooLarge) {
		t.Fatalf("want ErrFrameTooLarge, got %v", err)
	}
}

func TestCodec_TruncatedStreamReturnsEOF(t *testing.T) {
	var buf bytes.Buffer
	enc := NewEncoder(&buf)
	_ = enc.Write(&Envelope{Kind: KindCommand, Command: &Command{Op: OpStopAll}})
	// Drop the trailing byte to simulate a truncated frame.
	trunc := buf.Bytes()[:buf.Len()-1]
	dec := NewDecoder(bytes.NewReader(trunc))
	if _, err := dec.Read(); err == nil || errors.Is(err, ErrFrameTooLarge) {
		t.Fatalf("want a read/EOF error on truncation, got %v", err)
	}
	_ = io.EOF
}
