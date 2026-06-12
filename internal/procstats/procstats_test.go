package procstats

import (
	"os"
	"testing"
	"time"
)

func TestSampler_SelfSampleNonZeroRSS(t *testing.T) {
	s := New()
	// First call primes CPUPercent baseline — CPU may legitimately be 0.
	stats, err := s.Sample(os.Getpid())
	if err != nil {
		t.Fatalf("first sample: %v", err)
	}
	if stats.RSS == 0 {
		t.Errorf("self RSS should be > 0, got 0")
	}
	// Burn a little CPU then resample; the delta should be non-negative.
	end := time.Now().Add(50 * time.Millisecond)
	for time.Now().Before(end) {
		_ = time.Now().UnixNano()
	}
	stats2, err := s.Sample(os.Getpid())
	if err != nil {
		t.Fatalf("second sample: %v", err)
	}
	if stats2.CPU < 0 {
		t.Errorf("CPU delta should be non-negative, got %.2f", stats2.CPU)
	}
}

func TestSampler_BogusPIDReturnsError(t *testing.T) {
	s := New()
	if _, err := s.Sample(-1); err == nil {
		t.Error("negative pid should error")
	}
	if _, err := s.Sample(0); err == nil {
		t.Error("pid 0 should error")
	}
}

func TestSampler_SamplePrunesUnseenHandles(t *testing.T) {
	s := New()
	// Simulate a stale descendant handle left over from an earlier walk.
	s.mu.Lock()
	s.handles[999999] = nil
	s.mu.Unlock()
	if _, err := s.Sample(os.Getpid()); err != nil {
		t.Fatalf("sample: %v", err)
	}
	s.mu.Lock()
	_, stale := s.handles[999999]
	_, self := s.handles[int32(os.Getpid())]
	s.mu.Unlock()
	if stale {
		t.Error("handle for PID not seen in walk should be pruned")
	}
	if !self {
		t.Error("sampled root handle should survive pruning")
	}
}

func TestSampler_ForgetDropsCache(t *testing.T) {
	s := New()
	if _, err := s.Sample(os.Getpid()); err != nil {
		t.Fatalf("sample: %v", err)
	}
	if _, ok := s.handles[int32(os.Getpid())]; !ok {
		t.Fatal("handle should be cached after Sample")
	}
	s.Forget(os.Getpid())
	if _, ok := s.handles[int32(os.Getpid())]; ok {
		t.Error("handle should be cleared after Forget")
	}
}
