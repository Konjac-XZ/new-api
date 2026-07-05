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

// Store keeps recent request records in chronological order.
// Retention is time-window based with a minimum record floor.
type Store struct {
	records  []*recordEntry
	index    map[string]*recordEntry
	mu       sync.RWMutex
	events   chan StoreEvent
	eventsMu sync.RWMutex

	// realtimeEnabled gates websocket event work (snapshot/summary/clone + emit).
	// When false, mutations still update storage, but skip realtime event overhead.
	realtimeEnabled atomic.Bool
}

type recordEntry struct {
	mu     sync.RWMutex
	record *RequestRecord
}

// NewStore creates a new Store instance
func NewStore() *Store {
	s := &Store{
		records: make([]*recordEntry, 0, MonitorMinRecords),
		index:   make(map[string]*recordEntry),
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

func (s *Store) pruneLocked(now time.Time) {
	if len(s.records) <= MonitorMinRecords {
		return
	}

	cutoff := now.Add(-MonitorRetentionWindow)
	removeCount := 0
	for _, entry := range s.records {
		if len(s.records)-removeCount <= MonitorMinRecords {
			break
		}
		entry.mu.RLock()
		record := entry.record
		if record == nil {
			entry.mu.RUnlock()
			removeCount++
			continue
		}
		if !record.StartTime.Before(cutoff) {
			entry.mu.RUnlock()
			break
		}
		delete(s.index, record.ID)
		entry.mu.RUnlock()
		removeCount++
	}
	if removeCount > 0 {
		copy(s.records, s.records[removeCount:])
		clear(s.records[len(s.records)-removeCount:])
		s.records = s.records[:len(s.records)-removeCount]
	}
}

func shouldEmitSummaryWhenDegraded(summary *RequestSummary) bool {
	if summary == nil {
		return false
	}
	return summary.Status == StatusCompleted ||
		summary.Status == StatusError ||
		summary.Status == StatusAbandoned
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
	entry := &recordEntry{record: record}

	s.mu.Lock()
	if existing := s.index[record.ID]; existing != nil {
		for i, item := range s.records {
			if item == existing {
				s.records = append(s.records[:i], s.records[i+1:]...)
				break
			}
		}
	}
	s.records = append(s.records, entry)
	s.index[record.ID] = entry
	pruneAt := record.StartTime
	if pruneAt.IsZero() {
		pruneAt = time.Now()
	}
	s.pruneLocked(pruneAt)
	s.mu.Unlock()

	var snap recordSnapshot
	if emitRealtime {
		snap = snapshotRecord(record)
	}

	if emitRealtime {
		// Build summary outside lock
		if !IsDegraded() {
			s.emitEvent(EventTypeNew, snap.toSummary())
		}
	}
}

// Update updates an existing record by ID
func (s *Store) Update(id string, updater func(*RequestRecord)) {
	emitRealtime := s.shouldEmitRealtime()

	s.mu.RLock()
	entry := s.index[id]
	if entry == nil {
		s.mu.RUnlock()
		return
	}
	s.mu.RUnlock()

	entry.mu.Lock()
	record := entry.record
	if record == nil || record.ID != id {
		entry.mu.Unlock()
		return
	}
	updater(record)

	var snap recordSnapshot
	if emitRealtime {
		snap = snapshotRecord(record)
	}
	entry.mu.Unlock()

	if emitRealtime {
		// Build summary outside lock
		summary := snap.toSummary()
		if !IsDegraded() || shouldEmitSummaryWhenDegraded(summary) {
			s.emitEvent(EventTypeUpdate, summary)
		}
	}
}

// UpdateIfChanged updates an existing record and only emits an event if changes were made.
func (s *Store) UpdateIfChanged(id string, updater func(*RequestRecord) bool) {
	emitRealtime := s.shouldEmitRealtime()

	s.mu.RLock()
	entry := s.index[id]
	if entry == nil {
		s.mu.RUnlock()
		return
	}
	s.mu.RUnlock()

	entry.mu.Lock()
	record := entry.record
	if record == nil || record.ID != id {
		entry.mu.Unlock()
		return
	}
	if !updater(record) {
		entry.mu.Unlock()
		return
	}

	var snap recordSnapshot
	if emitRealtime {
		snap = snapshotRecord(record)
	}
	entry.mu.Unlock()

	if emitRealtime {
		summary := snap.toSummary()
		if !IsDegraded() || shouldEmitSummaryWhenDegraded(summary) {
			s.emitEvent(EventTypeUpdate, summary)
		}
	}
}

// UpdateAndBroadcastChannel combines Update + BroadcastChannelUpdate into a
// single write lock acquisition. Under one lock: run updater, capture snapshot,
// capture channel update fields. After unlock: emit both events.
func (s *Store) UpdateAndBroadcastChannel(id string, updater func(*RequestRecord)) {
	emitRealtime := s.shouldEmitRealtime()

	s.mu.RLock()
	entry := s.index[id]
	if entry == nil {
		s.mu.RUnlock()
		return
	}
	s.mu.RUnlock()

	entry.mu.Lock()
	record := entry.record
	if record == nil || record.ID != id {
		entry.mu.Unlock()
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
			ServerNowMs:     time.Now().UnixMilli(),
			CurrentPhase:    record.CurrentPhase,
			CurrentChannel:  cloneCurrentChannel(record.CurrentChannel),
			ChannelAttempts: cloneChannelAttemptsForUpdate(record.ChannelAttempts),
		}
	}
	entry.mu.Unlock()

	if emitRealtime {
		// Emit both events outside the lock
		summary := snap.toSummary()
		if !IsDegraded() || shouldEmitSummaryWhenDegraded(summary) {
			s.emitEvent(EventTypeUpdate, summary)
		}
		if !IsDegraded() {
			s.emitEvent(EventTypeChannel, chUpdate)
		}
	}
}

// UpdateAndBroadcastChannelIfChanged combines Update + BroadcastChannelUpdate and
// only emits events if the updater reports changes.
func (s *Store) UpdateAndBroadcastChannelIfChanged(id string, updater func(*RequestRecord) bool) {
	emitRealtime := s.shouldEmitRealtime()

	s.mu.RLock()
	entry := s.index[id]
	if entry == nil {
		s.mu.RUnlock()
		return
	}
	s.mu.RUnlock()

	entry.mu.Lock()
	record := entry.record
	if record == nil || record.ID != id {
		entry.mu.Unlock()
		return
	}
	if !updater(record) {
		entry.mu.Unlock()
		return
	}

	var snap recordSnapshot
	var chUpdate *ChannelUpdate
	if emitRealtime {
		snap = snapshotRecord(record)
		chUpdate = &ChannelUpdate{
			RequestID:       record.ID,
			ServerNowMs:     time.Now().UnixMilli(),
			CurrentPhase:    record.CurrentPhase,
			CurrentChannel:  cloneCurrentChannel(record.CurrentChannel),
			ChannelAttempts: cloneChannelAttemptsForUpdate(record.ChannelAttempts),
		}
	}
	entry.mu.Unlock()

	if emitRealtime {
		summary := snap.toSummary()
		if !IsDegraded() || shouldEmitSummaryWhenDegraded(summary) {
			s.emitEvent(EventTypeUpdate, summary)
		}
		if !IsDegraded() {
			s.emitEvent(EventTypeChannel, chUpdate)
		}
	}
}

// BatchUpdate applies multiple updaters under a single write lock, captures
// snapshot and optional channel update, then emits events after unlock.
func (s *Store) BatchUpdate(id string, broadcastChannel bool, updaters ...func(*RequestRecord)) {
	emitRealtime := s.shouldEmitRealtime()

	s.mu.RLock()
	entry := s.index[id]
	if entry == nil {
		s.mu.RUnlock()
		return
	}
	s.mu.RUnlock()

	entry.mu.Lock()
	record := entry.record
	if record == nil || record.ID != id {
		entry.mu.Unlock()
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
			ServerNowMs:     time.Now().UnixMilli(),
			CurrentPhase:    record.CurrentPhase,
			CurrentChannel:  cloneCurrentChannel(record.CurrentChannel),
			ChannelAttempts: cloneChannelAttemptsForUpdate(record.ChannelAttempts),
		}
	}
	entry.mu.Unlock()

	if emitRealtime {
		summary := snap.toSummary()
		if !IsDegraded() || shouldEmitSummaryWhenDegraded(summary) {
			s.emitEvent(EventTypeUpdate, summary)
		}
		if chUpdate != nil && !IsDegraded() {
			s.emitEvent(EventTypeChannel, chUpdate)
		}
	}
}

// Get retrieves a record by ID
func (s *Store) Get(id string) *RequestRecord {
	s.mu.RLock()
	entry := s.index[id]
	if entry == nil {
		s.mu.RUnlock()
		return nil
	}
	s.mu.RUnlock()

	entry.mu.RLock()
	record := entry.record
	if record == nil || record.ID != id {
		entry.mu.RUnlock()
		return nil
	}
	cloned := cloneRecord(record)
	entry.mu.RUnlock()
	return cloned
}

// GetChannelUpdate returns a lightweight ChannelUpdate for the given record,
// cloning only CurrentChannel and ChannelAttempts (no bodies/headers).
func (s *Store) GetChannelUpdate(id string) *ChannelUpdate {
	s.mu.RLock()
	entry := s.index[id]
	if entry == nil {
		s.mu.RUnlock()
		return nil
	}
	s.mu.RUnlock()

	entry.mu.RLock()
	r := entry.record
	if r == nil || r.ID != id {
		entry.mu.RUnlock()
		return nil
	}
	update := &ChannelUpdate{
		RequestID:       r.ID,
		ServerNowMs:     time.Now().UnixMilli(),
		CurrentPhase:    r.CurrentPhase,
		CurrentChannel:  cloneCurrentChannel(r.CurrentChannel),
		ChannelAttempts: cloneChannelAttemptsForUpdate(r.ChannelAttempts),
	}
	entry.mu.RUnlock()
	return update
}

// BroadcastChannelUpdate sends a channel update message for the given record ID.
// Uses GetChannelUpdate to avoid a full deep clone.
func (s *Store) BroadcastChannelUpdate(id string) {
	if !s.shouldEmitRealtime() || IsDegraded() {
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
	entries := append([]*recordEntry(nil), s.records...)
	s.mu.RUnlock()

	result := make([]*RequestRecord, 0, len(entries))
	for _, entry := range entries {
		entry.mu.RLock()
		if entry.record != nil {
			result = append(result, cloneRecord(entry.record))
		}
		entry.mu.RUnlock()
	}
	return result
}

// GetAllSummaries returns lightweight summaries of all stored records in chronological order
func (s *Store) GetAllSummaries() []*RequestSummary {
	s.mu.RLock()
	entries := append([]*recordEntry(nil), s.records...)
	s.mu.RUnlock()

	result := make([]*RequestSummary, 0, len(entries))
	for _, entry := range entries {
		entry.mu.RLock()
		if entry.record != nil {
			result = append(result, entry.record.ToSummary())
		}
		entry.mu.RUnlock()
	}
	return result
}

// GetActive returns all records with status "processing"
func (s *Store) GetActive() []*RequestRecord {
	s.mu.RLock()
	entries := append([]*recordEntry(nil), s.records...)
	s.mu.RUnlock()

	result := make([]*RequestRecord, 0)
	for _, entry := range entries {
		entry.mu.RLock()
		if entry.record != nil && entry.record.Status == StatusProcessing {
			result = append(result, cloneRecord(entry.record))
		}
		entry.mu.RUnlock()
	}
	return result
}

// GetStats returns monitoring statistics.
// ReadMemStats is called outside the lock to avoid holding the read lock
// during a stop-the-world pause.
func (s *Store) GetStats() MonitorStats {
	stats := MonitorStats{}

	s.mu.RLock()
	entries := append([]*recordEntry(nil), s.records...)
	s.mu.RUnlock()

	for _, entry := range entries {
		entry.mu.RLock()
		record := entry.record
		if record == nil {
			entry.mu.RUnlock()
			continue
		}
		stats.TotalRequests++
		switch record.Status {
		case StatusProcessing:
			stats.ActiveRequests++
		case StatusCompleted:
			stats.Completed++
		case StatusError:
			stats.Errors++
		case StatusAbandoned:
			stats.Abandoned++
		}
		entry.mu.RUnlock()
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
		r.EndTimeMs = timeToUnixMilli(now)
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
	cloned.Downstream.Body = cloneMonitorBody(record.Downstream.Body)

	if record.Upstream != nil {
		upstream := *record.Upstream
		upstream.Headers = cloneStringMap(record.Upstream.Headers)
		upstream.Body = cloneMonitorBody(record.Upstream.Body)
		cloned.Upstream = &upstream
	}

	if record.Response != nil {
		response := *record.Response
		response.Headers = cloneStringMap(record.Response.Headers)
		response.Body = cloneMonitorBody(record.Response.Body)
		if record.Response.Error != nil {
			errInfo := *record.Response.Error
			response.Error = &errInfo
		}
		cloned.Response = &response
	}

	cloned.CurrentChannel = cloneCurrentChannel(record.CurrentChannel)
	cloned.ChannelAttempts = cloneChannelAttempts(record.ChannelAttempts)
	cloned.ServerNowMs = time.Now().UnixMilli()

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

func cloneMonitorBody(input MonitorBody) MonitorBody {
	if len(input) == 0 {
		return nil
	}
	cloned := make([]byte, len(input))
	copy(cloned, input)
	return cloned
}
