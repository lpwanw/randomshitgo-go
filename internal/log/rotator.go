package log

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sync"
)

// Rotator is an io.Writer with size-based rotation.
// On every Write, appends to <path>, and when the file grows past MaxBytes,
// performs: drop .log.N -> rename .log.(N-1) -> .log.N -> … -> .log -> .log.1.
// Single writer per Rotator. Safe for concurrent Write calls (mutex).
type Rotator struct {
	path       string
	maxBytes   int64
	maxBackups int

	mu      sync.Mutex
	f       *os.File
	written int64
}

func NewRotator(path string, maxSizeMB, maxBackups int) (*Rotator, error) {
	if maxSizeMB < 1 {
		maxSizeMB = 1
	}
	if maxBackups < 1 {
		maxBackups = 1
	}
	r := &Rotator{
		path:       path,
		maxBytes:   int64(maxSizeMB) * 1024 * 1024,
		maxBackups: maxBackups,
	}
	if err := r.open(); err != nil {
		return nil, err
	}
	return r, nil
}

func (r *Rotator) open() error {
	if err := os.MkdirAll(filepath.Dir(r.path), 0o755); err != nil {
		return fmt.Errorf("mkdir log dir: %w", err)
	}
	f, err := os.OpenFile(r.path, os.O_WRONLY|os.O_CREATE|os.O_APPEND, 0o644)
	if err != nil {
		return fmt.Errorf("open log: %w", err)
	}
	st, err := f.Stat()
	if err != nil {
		_ = f.Close()
		return fmt.Errorf("stat log: %w", err)
	}
	r.f = f
	r.written = st.Size()
	return nil
}

func (r *Rotator) Write(b []byte) (int, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.f == nil {
		if err := r.open(); err != nil {
			return 0, err
		}
	}
	n, err := r.f.Write(b)
	r.written += int64(n)
	if err != nil {
		return n, err
	}
	if r.written >= r.maxBytes {
		if rerr := r.rotateLocked(); rerr != nil {
			fmt.Fprintf(os.Stderr, "procs: rotation failed for %s: %v\n", r.path, rerr)
		}
	}
	return n, nil
}

func (r *Rotator) Close() error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.f == nil {
		return nil
	}
	err := r.f.Close()
	r.f = nil
	return err
}

func (r *Rotator) rotateLocked() error {
	if err := r.f.Close(); err != nil {
		return fmt.Errorf("close current: %w", err)
	}
	r.f = nil

	drop := fmt.Sprintf("%s.%d", r.path, r.maxBackups)
	if _, err := os.Stat(drop); err == nil {
		if err := os.Remove(drop); err != nil {
			return fmt.Errorf("drop %s: %w", drop, err)
		}
	}

	for i := r.maxBackups - 1; i >= 1; i-- {
		src := fmt.Sprintf("%s.%d", r.path, i)
		dst := fmt.Sprintf("%s.%d", r.path, i+1)
		if _, err := os.Stat(src); err != nil {
			continue
		}
		if err := os.Rename(src, dst); err != nil {
			return fmt.Errorf("shift %s->%s: %w", src, dst, err)
		}
	}

	if _, err := os.Stat(r.path); err == nil {
		if err := os.Rename(r.path, r.path+".1"); err != nil {
			return fmt.Errorf("rotate main %s: %w", r.path, err)
		}
	}
	return r.open()
}

var _ io.WriteCloser = (*Rotator)(nil)
