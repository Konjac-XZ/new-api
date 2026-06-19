package monitor

import "sync"

// LoadState tracks monitor pressure independently from the 100-item display
// buffer. It intentionally stores only request IDs so high concurrency does
// not add body/detail memory pressure.
type LoadState struct {
	mu                    sync.Mutex
	active                map[string]struct{}
	degraded              bool
	degradationGeneration uint64
}

type LoadSnapshot struct {
	ActiveRequests        int    `json:"active_requests"`
	Capacity              int    `json:"capacity"`
	Degraded              bool   `json:"degraded"`
	DegradationGeneration uint64 `json:"degradation_generation"`
}

func NewLoadState() *LoadState {
	return &LoadState{
		active: make(map[string]struct{}),
	}
}

func (s *LoadState) Start(id string) LoadSnapshot {
	if s == nil || id == "" {
		return LoadSnapshot{Capacity: MaxRecords}
	}

	s.mu.Lock()
	s.active[id] = struct{}{}
	s.updateDegradedLocked()
	snapshot := s.snapshotLocked()
	s.mu.Unlock()

	return snapshot
}

func (s *LoadState) Finish(id string) LoadSnapshot {
	if s == nil || id == "" {
		return LoadSnapshot{Capacity: MaxRecords}
	}

	s.mu.Lock()
	delete(s.active, id)
	s.updateDegradedLocked()
	snapshot := s.snapshotLocked()
	s.mu.Unlock()

	return snapshot
}

func (s *LoadState) Snapshot() LoadSnapshot {
	if s == nil {
		return LoadSnapshot{Capacity: MaxRecords}
	}

	s.mu.Lock()
	s.updateDegradedLocked()
	snapshot := s.snapshotLocked()
	s.mu.Unlock()

	return snapshot
}

func (s *LoadState) IsDegraded() bool {
	return s.Snapshot().Degraded
}

func (s *LoadState) DegradationGeneration() uint64 {
	return s.Snapshot().DegradationGeneration
}

func (s *LoadState) updateDegradedLocked() {
	activeCount := len(s.active)
	if s.degraded {
		if activeCount <= MonitorRecoverActiveLimit {
			s.degraded = false
		}
		return
	}
	if activeCount > MonitorDegradeActiveLimit {
		s.degraded = true
		s.degradationGeneration++
	}
}

func (s *LoadState) snapshotLocked() LoadSnapshot {
	return LoadSnapshot{
		ActiveRequests:        len(s.active),
		Capacity:              MonitorDegradeActiveLimit,
		Degraded:              s.degraded,
		DegradationGeneration: s.degradationGeneration,
	}
}
