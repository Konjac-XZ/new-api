package monitor

import (
	"runtime"
	"sync"
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

// Store is a ring buffer storage for request records
// Not concurrency-safe for iteration; uses internal mutexes for callers.
type Store struct {
	records  []*RequestRecord
	index    map[string]int // ID -> position mapping
	mu       sync.RWMutex
	head     int
	count    int
	events   chan StoreEvent
	eventsMu sync.RWMutex
}

// NewStore creates a new Store instance
func NewStore() *Store {
	return &Store{
		records: make([]*RequestRecord, MaxRecords),
		index:   make(map[string]int),
		events:  make(chan StoreEvent, 100),
	}
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
	var summary *RequestSummary
	s.mu.Lock()

	// If we're overwriting an existing record, remove it from the index
	if s.records[s.head] != nil {
		delete(s.index, s.records[s.head].ID)
	}

	// Store the new record
	s.records[s.head] = record
	s.index[record.ID] = s.head

	// Move head forward
	s.head = (s.head + 1) % MaxRecords
	if s.count < MaxRecords {
		s.count++
	}

	summary = record.ToSummary()
	s.mu.Unlock()

	// Emit event for new record
	if summary != nil {
		s.emitEvent(EventTypeNew, summary)
	}
}

// Update updates an existing record by ID
func (s *Store) Update(id string, updater func(*RequestRecord)) {
	var summary *RequestSummary
	s.mu.Lock()

	pos, exists := s.index[id]
	if !exists {
		s.mu.Unlock()
		return
	}

	record := s.records[pos]
	if record == nil {
		s.mu.Unlock()
		return
	}

	updater(record)

	summary = record.ToSummary()
	s.mu.Unlock()

	// Emit event for record update
	if summary != nil {
		s.emitEvent(EventTypeUpdate, summary)
	}
}

// Get retrieves a record by ID
func (s *Store) Get(id string) *RequestRecord {
	s.mu.RLock()
	defer s.mu.RUnlock()

	pos, exists := s.index[id]
	if !exists {
		return nil
	}

	return cloneRecord(s.records[pos])
}

// BroadcastChannelUpdate sends a channel update message for the given record ID
func (s *Store) BroadcastChannelUpdate(id string) {
	record := s.Get(id)
	if record == nil {
		return
	}

	update := record.ToChannelUpdate()
	if update == nil {
		return
	}

	// Emit event for channel update
	s.emitEvent(EventTypeChannel, update)
}

// GetAll returns all stored records in chronological order (oldest first)
func (s *Store) GetAll() []*RequestRecord {
	s.mu.RLock()
	defer s.mu.RUnlock()

	result := make([]*RequestRecord, 0, s.count)

	if s.count < MaxRecords {
		// Buffer not full yet, start from 0
		for i := 0; i < s.count; i++ {
			if s.records[i] != nil {
				result = append(result, cloneRecord(s.records[i]))
			}
		}
	} else {
		// Buffer is full, start from head (oldest)
		for i := 0; i < MaxRecords; i++ {
			pos := (s.head + i) % MaxRecords
			if s.records[pos] != nil {
				result = append(result, cloneRecord(s.records[pos]))
			}
		}
	}

	return result
}

// GetAllSummaries returns lightweight summaries of all stored records in chronological order
func (s *Store) GetAllSummaries() []*RequestSummary {
	s.mu.RLock()
	defer s.mu.RUnlock()

	result := make([]*RequestSummary, 0, s.count)

	if s.count < MaxRecords {
		// Buffer not full yet, start from 0
		for i := 0; i < s.count; i++ {
			if s.records[i] != nil {
				result = append(result, s.records[i].ToSummary())
			}
		}
	} else {
		// Buffer is full, start from head (oldest)
		for i := 0; i < MaxRecords; i++ {
			pos := (s.head + i) % MaxRecords
			if s.records[pos] != nil {
				result = append(result, s.records[pos].ToSummary())
			}
		}
	}

	return result
}

// GetActive returns all records with status "processing"
func (s *Store) GetActive() []*RequestRecord {
	s.mu.RLock()
	defer s.mu.RUnlock()

	result := make([]*RequestRecord, 0)

	for _, record := range s.records {
		if record != nil && record.Status == StatusProcessing {
			result = append(result, cloneRecord(record))
		}
	}

	return result
}

// GetStats returns monitoring statistics
func (s *Store) GetStats() MonitorStats {
	s.mu.RLock()
	defer s.mu.RUnlock()

	stats := MonitorStats{}
	var mem runtime.MemStats
	runtime.ReadMemStats(&mem)
	stats.MemoryBytes = int64(mem.Alloc)

	for _, record := range s.records {
		if record == nil {
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
		}
	}

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

// TruncateBody truncates a body string to MaxBodySize
func TruncateBody(body string) string {
	if len(body) <= MaxBodySize {
		return body
	}
	return body[:MaxBodySize] + "... [truncated]"
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
