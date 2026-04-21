// Package procstats samples CPU and resident-memory usage for a process
// tree. The status bar calls Sample on the selected project's root PID every
// status tick; CPU% is a delta vs the previous call so the first sample of
// a given PID returns 0.0.
package procstats

import (
	"fmt"
	"sync"

	"github.com/shirou/gopsutil/v3/process"
)

// Stats is the summed tree-wide snapshot for one call to Sampler.Sample.
// CPU is a percentage that can exceed 100 on multi-core processes (matches
// htop convention). RSS is in bytes.
type Stats struct {
	CPU float64
	RSS uint64
}

// Sampler caches gopsutil process handles keyed by PID so successive
// CPUPercent() calls return a delta since the last call rather than
// cumulative-since-start. Safe for concurrent use.
type Sampler struct {
	mu      sync.Mutex
	handles map[int32]*process.Process
}

// New returns an empty Sampler ready to use.
func New() *Sampler {
	return &Sampler{handles: map[int32]*process.Process{}}
}

// Sample walks the process tree rooted at pid and returns summed CPU% + RSS
// across root and all descendants. Processes that exit mid-walk are skipped
// silently. Returns an error only when the root PID cannot be opened —
// caller should treat that as "no stats yet" rather than a UI error.
func (s *Sampler) Sample(pid int) (Stats, error) {
	if pid <= 0 {
		return Stats{}, fmt.Errorf("procstats: invalid pid %d", pid)
	}
	root, err := s.handleFor(int32(pid))
	if err != nil {
		return Stats{}, err
	}
	var total Stats
	s.accumulate(root, &total)
	// Recursive children — missing children between the call and the walk are
	// possible on a busy system; gopsutil returns them as non-nil with errors
	// on their method calls, which accumulate() drops via the zero-value path.
	kids, err := root.Children()
	if err == nil {
		for _, k := range kids {
			s.accumulateTree(k, &total)
		}
	}
	return total, nil
}

// Forget drops any cached handle for pid. Call this when a child restarts
// (new PID) so the next Sample builds a fresh CPU% baseline instead of
// comparing against the pre-restart process.
func (s *Sampler) Forget(pid int) {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.handles, int32(pid))
}

func (s *Sampler) handleFor(pid int32) (*process.Process, error) {
	s.mu.Lock()
	if p, ok := s.handles[pid]; ok {
		s.mu.Unlock()
		return p, nil
	}
	s.mu.Unlock()

	p, err := process.NewProcess(pid)
	if err != nil {
		return nil, err
	}
	s.mu.Lock()
	s.handles[pid] = p
	s.mu.Unlock()
	return p, nil
}

// accumulate adds p's CPU% + RSS to out. Errors from either call are
// swallowed — a single missing process shouldn't zero out the whole tree.
func (s *Sampler) accumulate(p *process.Process, out *Stats) {
	if cpu, err := p.CPUPercent(); err == nil {
		out.CPU += cpu
	}
	if mem, err := p.MemoryInfo(); err == nil && mem != nil {
		out.RSS += mem.RSS
	}
}

// accumulateTree walks p and its own children recursively. We hand-roll the
// recursion instead of using process.Children(recursive bool) because the
// gopsutil v3 API only exposes non-recursive Children(); our tree walks
// depth-first and reuses handleFor so each node's CPU% delta is measured
// against the last sample.
func (s *Sampler) accumulateTree(p *process.Process, out *Stats) {
	// Re-acquire handle via cache so CPUPercent deltas are stable.
	handle, err := s.handleFor(p.Pid)
	if err != nil {
		return
	}
	s.accumulate(handle, out)
	kids, err := handle.Children()
	if err != nil {
		return
	}
	for _, k := range kids {
		s.accumulateTree(k, out)
	}
}
