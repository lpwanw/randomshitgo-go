package ipc

import (
	"bytes"
	"encoding/binary"
	"encoding/gob"
	"errors"
	"fmt"
	"io"
	"sync"
)

// MaxFrameSize caps a single encoded Envelope. Frames larger than this are
// rejected before any payload allocation, so a hostile/buggy peer cannot force
// an out-of-memory allocation via a forged length prefix.
const MaxFrameSize = 16 << 20 // 16 MiB

// ErrFrameTooLarge is returned when a frame's declared/encoded size exceeds
// MaxFrameSize.
var ErrFrameTooLarge = errors.New("ipc: frame exceeds max size")

// Encoder writes length-prefixed gob frames to a writer. A single persistent
// gob.Encoder is reused so type descriptors are sent only once across frames;
// each frame's bytes are length-prefixed for the reader's size guard.
// Safe for concurrent use.
type Encoder struct {
	mu  sync.Mutex
	w   io.Writer
	buf *bytes.Buffer
	enc *gob.Encoder
}

// NewEncoder returns an Encoder writing framed Envelopes to w.
func NewEncoder(w io.Writer) *Encoder {
	buf := new(bytes.Buffer)
	return &Encoder{w: w, buf: buf, enc: gob.NewEncoder(buf)}
}

// Write encodes env and writes one framed message. The encoder accumulates gob
// output for this frame in an internal buffer, then emits a 4-byte big-endian
// length prefix followed by the payload.
func (e *Encoder) Write(env *Envelope) error {
	e.mu.Lock()
	defer e.mu.Unlock()

	e.buf.Reset()
	if err := e.enc.Encode(env); err != nil {
		return fmt.Errorf("ipc: encode: %w", err)
	}
	payload := e.buf.Bytes()
	if len(payload) > MaxFrameSize {
		return ErrFrameTooLarge
	}

	var hdr [4]byte
	binary.BigEndian.PutUint32(hdr[:], uint32(len(payload)))
	if _, err := e.w.Write(hdr[:]); err != nil {
		return fmt.Errorf("ipc: write header: %w", err)
	}
	if _, err := e.w.Write(payload); err != nil {
		return fmt.Errorf("ipc: write payload: %w", err)
	}
	return nil
}

// Decoder reads length-prefixed gob frames. A persistent gob.Decoder reads from
// framedReader, which reconstructs the continuous gob stream frame-by-frame
// while enforcing the per-frame size cap before any bytes are consumed.
type Decoder struct {
	fr  *framedReader
	dec *gob.Decoder
}

// NewDecoder returns a Decoder reading framed Envelopes from r.
func NewDecoder(r io.Reader) *Decoder {
	fr := &framedReader{r: r, max: MaxFrameSize}
	return &Decoder{fr: fr, dec: gob.NewDecoder(fr)}
}

// Read decodes the next Envelope. Returns io.EOF when the stream ends, or
// ErrFrameTooLarge if a frame's length prefix exceeds the cap.
func (d *Decoder) Read() (*Envelope, error) {
	var env Envelope
	if err := d.dec.Decode(&env); err != nil {
		return nil, err
	}
	return &env, nil
}

// framedReader presents a sequence of length-prefixed frames as a continuous
// byte stream to a gob.Decoder, refusing any frame whose declared size exceeds
// max. One decoded Envelope consumes exactly one frame because each Encode.Write
// emits exactly one frame.
type framedReader struct {
	r         io.Reader
	max       uint32
	remaining uint32 // bytes left in the current frame
}

func (fr *framedReader) Read(p []byte) (int, error) {
	if fr.remaining == 0 {
		var hdr [4]byte
		if _, err := io.ReadFull(fr.r, hdr[:]); err != nil {
			return 0, err
		}
		n := binary.BigEndian.Uint32(hdr[:])
		if n == 0 {
			return 0, errors.New("ipc: zero-length frame")
		}
		if n > fr.max {
			return 0, ErrFrameTooLarge
		}
		fr.remaining = n
	}
	if uint32(len(p)) > fr.remaining {
		p = p[:fr.remaining]
	}
	n, err := fr.r.Read(p)
	fr.remaining -= uint32(n)
	return n, err
}
