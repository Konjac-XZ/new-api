package monitor

import (
	"runtime"
	"sync"
	"sync/atomic"
	"time"
)

// StoreEventType represents the type of store event
type StoreEventType string

const (
	EventTypeNew     StoreEventType = "new"
	EventTypeUpdate  StoreEventType = "update"
	EventTypeChannel StoreEventType = "channel"
)

// StoreEvent represents an event emitted by the Store
type StoreEvent struct {
	Type    StoreEventType
	Payload interface{}
}

// Store is a ring buffer storage for request records.
// Uses a global index lock plus per-slot locks to reduce contention.
type Store struct {
	records  []*recordSlot
	index    map[string]int // ID -> position mapping
	mu       sync.RWMutex
	head     int
	count    int
	events   chan StoreEvent
	eventsMu sync.RWMutex

	// realtimeEnabled gates websocket event work (snapshot/summary/clone + emit).
	// When false, mutations still update storage, but skip realtime event overhead.
	realtimeEnabled atomic.Bool
}

type recordSlot struct {
	mu     sync.RWMutex
	record *RequestRecord
}

// NewStore creates a new Store instance
func NewStore() *Store {
	records := make([]*recordSlot, MaxRecords)
	for i := range records {
		records[i] = &recordSlot{}
	}
	s := &Store{
		records: records,
		index:   make(map[string]int),
		events:  make(chan StoreEvent, 100),
	}
	// Zero-impact default: do not build/emit websocket payloads until a
	// monitor websocket client is connected.
	s.realtimeEnabled.Store(false)
	return s
}

// SetRealtimeEnabled toggles realtime websocket event work.
func (s *Store) SetRealtimeEnabled(enabled bool) {
	s.realtimeEnabled.Store(enabled)
}

func (s *Store) shouldEmitRealtime() bool {
	return s.realtimeEnabled.Load()
}

// Events returns the event channel for subscribing to store events
func (s *Store) Events() <-chan StoreEvent {
	s.eventsMu.RLock()
	defer s.eventsMu.RUnlock()
	return s.events
}

// emitEvent sends an event to subscribers
func (s *Store) emitEvent(eventType StoreEventType, payload interface{}) {
	s.eventsMu.RLock()
	defer s.eventsMu.RUnlock()

	if s.events != nil {
		select {
		case s.events <- StoreEvent{Type: eventType, Payload: payload}:
		default:
			// Drop event if channel is full to avoid blocking
		}
	}
}

// Add adds a new record to the store
func (s *Store) Add(record *RequestRecord) {
	emitRealtime := s.shouldEmitRealtime()

	s.mu.Lock()
	slot := s.records[s.head]
	slot.mu.Lock()

	// If we're overwriting an existing record, remove it from the index
	if slot.record != nil {
		delete(s.index, slot.record.ID)
	}

	// Store the new record
	slot.record = record
	s.index[record.ID] = s.head

	// Move head forward
	s.head = (s.head + 1) % MaxRecords
	if s.count < MaxRecords {
		s.count++
	}

	var snap recordSnapshot
	if emitRealtime {
		// Snapshot under slot lock (flat copy, no heap allocs)
		snap = snapshotRecord(record)
	}
	slot.mu.Unlock()
	s.mu.Unlock()

	if emitRealtime {
		// Build summary outside lock
		s.emitEvent(EventTypeNew, snap.toSummary())
	}
}

// Update updates an existing record by ID
func (s *Store) Update(id string, updater func(*RequestRecord)) {
	emitRealtime := s.shouldEmitRealtime()

	s.mu.RLock()
	pos, exists := s.index[id]
	if !exists {
		s.mu.RUnlock()
		return
	}
	slot := s.records[pos]
	s.mu.RUnlock()

	slot.mu.Lock()
	record := slot.record
	if record == nil || record.ID != id {
		slot.mu.Unlock()
		return
	}

	updater(record)

	var snap recordSnapshot
	if emitRealtime {
		// Snapshot under slot lock (flat copy, no heap allocs)
		snap = snapshotRecord(record)
	}
	slot.mu.Unlock()

	if emitRealtime {
		// Build summary outside lock
		s.emitEvent(EventTypeUpdate, snap.toSummary())
	}
}

// UpdateIfChanged updates an existing record and only emits an event if changes were made.
func (s *Store) UpdateIfChanged(id string, updater func(*RequestRecord) bool) {
	emitRealtime := s.shouldEmitRealtime()

	s.mu.RLock()
	pos, exists := s.index[id]
	if !exists {
		s.mu.RUnlock()
		return
	}
	slot := s.records[pos]
	s.mu.RUnlock()

	slot.mu.Lock()
	record := slot.record
	if record == nil || record.ID != id {
		slot.mu.Unlock()
		return
	}

	if !updater(record) {
		slot.mu.Unlock()
		return
	}

	var snap recordSnapshot
	if emitRealtime {
		snap = snapshotRecord(record)
	}
	slot.mu.Unlock()

	if emitRealtime {
		s.emitEvent(EventTypeUpdate, snap.toSummary())
	}
}

// UpdateAndBroadcastChannel combines Update + BroadcastChannelUpdate into a
// single write lock acquisition. Under one lock: run updater, capture snapshot,
// capture channel update fields. After unlock: emit both events.
func (s *Store) UpdateAndBroadcastChannel(id string, updater func(*RequestRecord)) {
	emitRealtime := s.shouldEmitRealtime()

	s.mu.RLock()
	pos, exists := s.index[id]
	if !exists {
		s.mu.RUnlock()
		return
	}
	slot := s.records[pos]
	s.mu.RUnlock()

	slot.mu.Lock()
	record := slot.record
	if record == nil || record.ID != id {
		slot.mu.Unlock()
		return
	}

	updater(record)

	var snap recordSnapshot
	var chUpdate *ChannelUpdate
	if emitRealtime {
		// Capture snapshot and channel update under the same lock
		snap = snapshotRecord(record)
		chUpdate = &ChannelUpdate{
			RequestID:       record.ID,
			CurrentPhase:    record.CurrentPhase,
			CurrentChannel:  cloneCurrentChannel(record.CurrentChannel),
			ChannelAttempts: cloneChannelAttemptsForUpdate(record.ChannelAttempts),
		}
	}
	slot.mu.Unlock()

	if emitRealtime {
		// Emit both events outside the lock
		s.emitEvent(EventTypeUpdate, snap.toSummary())
		s.emitEvent(EventTypeChannel, chUpdate)
	}
}

// UpdateAndBroadcastChannelIfChanged combines Update + BroadcastChannelUpdate and
// only emits events if the updater reports changes.
func (s *Store) UpdateAndBroadcastChannelIfChanged(id string, updater func(*RequestRecord) bool) {
	emitRealtime := s.shouldEmitRealtime()

	s.mu.RLock()
	pos, exists := s.index[id]
	if !exists {
		s.mu.RUnlock()
		return
	}
	slot := s.records[pos]
	s.mu.RUnlock()

	slot.mu.Lock()
	record := slot.record
	if record == nil || record.ID != id {
		slot.mu.Unlock()
		return
	}

	if !updater(record) {
		slot.mu.Unlock()
		return
	}

	var snap recordSnapshot
	var chUpdate *ChannelUpdate
	if emitRealtime {
		snap = snapshotRecord(record)
		chUpdate = &ChannelUpdate{
			RequestID:       record.ID,
			CurrentPhase:    record.CurrentPhase,
			CurrentChannel:  cloneCurrentChannel(record.CurrentChannel),
			ChannelAttempts: cloneChannelAttemptsForUpdate(record.ChannelAttempts),
		}
	}
	slot.mu.Unlock()

	if emitRealtime {
		s.emitEvent(EventTypeUpdate, snap.toSummary())
		s.emitEvent(EventTypeChannel, chUpdate)
	}
}

// BatchUpdate applies multiple updaters under a single write lock, captures
// snapshot and optional channel update, then emits events after unlock.
func (s *Store) BatchUpdate(id string, broadcastChannel bool, updaters ...func(*RequestRecord)) {
	emitRealtime := s.shouldEmitRealtime()

	s.mu.RLock()
	pos, exists := s.index[id]
	if !exists {
		s.mu.RUnlock()
		return
	}
	slot := s.records[pos]
	s.mu.RUnlock()

	slot.mu.Lock()
	record := slot.record
	if record == nil || record.ID != id {
		slot.mu.Unlock()
		return
	}

	for _, updater := range updaters {
		updater(record)
	}

	var snap recordSnapshot
	var chUpdate *ChannelUpdate
	if emitRealtime {
		snap = snapshotRecord(record)
	}
	if emitRealtime && broadcastChannel {
		chUpdate = &ChannelUpdate{
			RequestID:       record.ID,
			CurrentPhase:    record.CurrentPhase,
			CurrentChannel:  cloneCurrentChannel(record.CurrentChannel),
			ChannelAttempts: cloneChannelAttemptsForUpdate(record.ChannelAttempts),
		}
	}
	slot.mu.Unlock()

	if emitRealtime {
		s.emitEvent(EventTypeUpdate, snap.toSummary())
		if chUpdate != nil {
			s.emitEvent(EventTypeChannel, chUpdate)
		}
	}
}

// Get retrieves a record by ID
func (s *Store) Get(id string) *RequestRecord {
	s.mu.RLock()
	pos, exists := s.index[id]
	if !exists {
		s.mu.RUnlock()
		return nil
	}
	slot := s.records[pos]
	s.mu.RUnlock()

	slot.mu.RLock()
	record := slot.record
	if record == nil || record.ID != id {
		slot.mu.RUnlock()
		return nil
	}
	cloned := cloneRecord(record)
	slot.mu.RUnlock()
	return cloned
}

// GetChannelUpdate returns a lightweight ChannelUpdate for the given record,
// cloning only CurrentChannel and ChannelAttempts (no bodies/headers).
func (s *Store) GetChannelUpdate(id string) *ChannelUpdate {
	s.mu.RLock()
	pos, exists := s.index[id]
	if !exists {
		s.mu.RUnlock()
		return nil
	}
	slot := s.records[pos]
	s.mu.RUnlock()

	slot.mu.RLock()
	r := slot.record
	if r == nil || r.ID != id {
		slot.mu.RUnlock()
		return nil
	}
	update := &ChannelUpdate{
		RequestID:       r.ID,
		CurrentPhase:    r.CurrentPhase,
		CurrentChannel:  cloneCurrentChannel(r.CurrentChannel),
		ChannelAttempts: cloneChannelAttemptsForUpdate(r.ChannelAttempts),
	}
	slot.mu.RUnlock()
	return update
}

// BroadcastChannelUpdate sends a channel update message for the given record ID.
// Uses GetChannelUpdate to avoid a full deep clone.
func (s *Store) BroadcastChannelUpdate(id string) {
	if !s.shouldEmitRealtime() {
		return
	}

	update := s.GetChannelUpdate(id)
	if update == nil {
		return
	}
	s.emitEvent(EventTypeChannel, update)
}

// GetAll returns all stored records in chronological order (oldest first)
func (s *Store) GetAll() []*RequestRecord {
	s.mu.RLock()
	count := s.count
	head := s.head
	s.mu.RUnlock()

	result := make([]*RequestRecord, 0, count)

	appendIfPresent := func(slot *recordSlot) {
		slot.mu.RLock()
		record := slot.record
		if record != nil {
			result = append(result, cloneRecord(record))
		}
		slot.mu.RUnlock()
	}

	if count < MaxRecords {
		// Buffer not full yet, start from 0
		for i := 0; i < count; i++ {
			appendIfPresent(s.records[i])
		}
	} else {
		// Buffer is full, start from head (oldest)
		for i := 0; i < MaxRecords; i++ {
			pos := (head + i) % MaxRecords
			appendIfPresent(s.records[pos])
		}
	}

	return result
}

// GetAllSummaries returns lightweight summaries of all stored records in chronological order
func (s *Store) GetAllSummaries() []*RequestSummary {
	s.mu.RLock()
	count := s.count
	head := s.head
	s.mu.RUnlock()

	result := make([]*RequestSummary, 0, count)

	appendIfPresent := func(slot *recordSlot) {
		slot.mu.RLock()
		record := slot.record
		if record != nil {
			result = append(result, record.ToSummary())
		}
		slot.mu.RUnlock()
	}

	if count < MaxRecords {
		// Buffer not full yet, start from 0
		for i := 0; i < count; i++ {
			appendIfPresent(s.records[i])
		}
	} else {
		// Buffer is full, start from head (oldest)
		for i := 0; i < MaxRecords; i++ {
			pos := (head + i) % MaxRecords
			appendIfPresent(s.records[pos])
		}
	}

	return result
}

// GetActive returns all records with status "processing"
func (s *Store) GetActive() []*RequestRecord {
	result := make([]*RequestRecord, 0)
	for _, slot := range s.records {
		slot.mu.RLock()
		record := slot.record
		if record != nil && record.Status == StatusProcessing {
			result = append(result, cloneRecord(record))
		}
		slot.mu.RUnlock()
	}

	return result
}

// GetStats returns monitoring statistics.
// ReadMemStats is called outside the lock to avoid holding the read lock
// during a stop-the-world pause.
func (s *Store) GetStats() MonitorStats {
	stats := MonitorStats{}

	for _, slot := range s.records {
		slot.mu.RLock()
		record := slot.record
		if record != nil {
			stats.TotalRequests++
			switch record.Status {
			case StatusProcessing:
				stats.ActiveRequests++
			case StatusCompleted:
				stats.Completed++
			case StatusError:
				stats.Errors++
			}
		}
		slot.mu.RUnlock()
	}

	// ReadMemStats triggers a STW pause — do it with no lock held.
	var mem runtime.MemStats
	runtime.ReadMemStats(&mem)
	stats.MemoryBytes = int64(mem.Alloc)

	return stats
}

// MarkComplete marks a record as completed with response info
func (s *Store) MarkComplete(id string, response *ResponseInfo) {
	s.Update(id, func(r *RequestRecord) {
		now := time.Now()
		r.EndTime = &now
		r.Duration = now.Sub(r.StartTime).Milliseconds()
		r.Response = response
		if response != nil && response.Error != nil {
			r.Status = StatusError
		} else {
			r.Status = StatusCompleted
		}
	})
}

func cloneRecord(record *RequestRecord) *RequestRecord {
	if record == nil {
		return nil
	}

	cloned := *record
	cloned.Downstream = record.Downstream
	cloned.Downstream.Headers = cloneStringMap(record.Downstream.Headers)

	if record.Upstream != nil {
		upstream := *record.Upstream
		upstream.Headers = cloneStringMap(record.Upstream.Headers)
		cloned.Upstream = &upstream
	}

	if record.Response != nil {
		response := *record.Response
		response.Headers = cloneStringMap(record.Response.Headers)
		if record.Response.Error != nil {
			errInfo := *record.Response.Error
			response.Error = &errInfo
		}
		cloned.Response = &response
	}

	cloned.CurrentChannel = cloneCurrentChannel(record.CurrentChannel)
	cloned.ChannelAttempts = cloneChannelAttempts(record.ChannelAttempts)

	return &cloned
}

func cloneStringMap(input map[string]string) map[string]string {
	if len(input) == 0 {
		return nil
	}
	cloned := make(map[string]string, len(input))
	for key, value := range input {
		cloned[key] = value
	}
	return cloned
}
