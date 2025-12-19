package monitor

import (
	"sync"
	"time"
)

const (
	MaxRecords        = 100
	BodySizeThreshold = 1024 * 1024 // Threshold for marking body as "large" (1MB, ~256K tokens)
)

// Store is a ring buffer storage for request records
type Store struct {
	records      []*RequestRecord
	index        map[string]int // ID -> position mapping
	mu           sync.RWMutex
	head         int
	count        int
	hub          *Hub
	evictionSink EvictionSink
}

// EvictionSink receives records that are evicted due to the ring buffer being full.
// Implementations must be non-blocking and concurrency-safe.
type EvictionSink interface {
	OnEvicted(record *RequestRecord)
}

func cloneStringMap(in map[string]string) map[string]string {
	if in == nil {
		return nil
	}
	out := make(map[string]string, len(in))
	for k, v := range in {
		out[k] = v
	}
	return out
}

func cloneRequestRecordForAPI(in *RequestRecord) *RequestRecord {
	if in == nil {
		return nil
	}

	out := *in

	// Downstream
	out.Downstream = in.Downstream
	out.Downstream.Headers = cloneStringMap(in.Downstream.Headers)

	// Upstream
	if in.Upstream != nil {
		up := *in.Upstream
		up.Headers = cloneStringMap(in.Upstream.Headers)
		out.Upstream = &up
	}

	// Response
	if in.Response != nil {
		resp := *in.Response
		resp.Headers = cloneStringMap(in.Response.Headers)
		if in.Response.Error != nil {
			errInfo := *in.Response.Error
			resp.Error = &errInfo
		}
		out.Response = &resp
	}

	// CurrentChannel
	if in.CurrentChannel != nil {
		cc := *in.CurrentChannel
		out.CurrentChannel = &cc
	}

	// ChannelAttempts
	if in.ChannelAttempts != nil {
		out.ChannelAttempts = append([]ChannelAttempt(nil), in.ChannelAttempts...)
	}

	return &out
}

// isActiveStatus reports whether a request status should be treated as active/processing
func isActiveStatus(status string) bool {
	switch status {
	case StatusProcessing, StatusWaitingUpstream, StatusStreaming:
		return true
	default:
		return false
	}
}

// NewStore creates a new Store instance
func NewStore(hub *Hub) *Store {
	return &Store{
		records: make([]*RequestRecord, MaxRecords),
		index:   make(map[string]int),
		hub:     hub,
	}
}

func (s *Store) SetEvictionSink(sink EvictionSink) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.evictionSink = sink
}

// Count returns the number of records currently held in the ring buffer.
func (s *Store) Count() int {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.count
}

// GetSnapshot retrieves a deep-copied record by ID suitable for JSON serialization
// without racing with concurrent updates.
func (s *Store) GetSnapshot(id string) *RequestRecord {
	s.mu.RLock()
	defer s.mu.RUnlock()

	pos, exists := s.index[id]
	if !exists {
		return nil
	}

	return cloneRequestRecordForAPI(s.records[pos])
}

// Add adds a new record to the store
func (s *Store) Add(record *RequestRecord) {
	var evicted *RequestRecord
	var sink EvictionSink
	var summary *RequestSummary

	s.mu.Lock()

	// If we're overwriting an existing record, remove it from the index
	if s.records[s.head] != nil {
		evicted = s.records[s.head]
		delete(s.index, evicted.ID)
	}

	// Store the new record
	s.records[s.head] = record
	s.index[record.ID] = s.head
	summary = record.ToSummary()
	sink = s.evictionSink

	// Move head forward
	s.head = (s.head + 1) % MaxRecords
	if s.count < MaxRecords {
		s.count++
	}
	s.mu.Unlock()

	if evicted != nil && sink != nil {
		sink.OnEvicted(evicted)
	}

	// Broadcast new record summary to WebSocket clients
	if s.hub != nil {
		s.hub.Broadcast(&WSMessage{
			Type:    WSMessageTypeNew,
			Payload: summary,
		})
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

	// Broadcast update summary to WebSocket clients
	if s.hub != nil {
		s.hub.Broadcast(&WSMessage{
			Type:    WSMessageTypeUpdate,
			Payload: summary,
		})
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

	return s.records[pos]
}

// BroadcastChannelUpdate sends a channel update message for the given record ID
func (s *Store) BroadcastChannelUpdate(id string) {
	if s.hub == nil {
		return
	}

	// Copy only the fields required for channel update under lock, so we don't
	// race with concurrent updates or copy large bodies unnecessarily.
	var update *ChannelUpdate
	s.mu.RLock()
	pos, exists := s.index[id]
	if exists {
		record := s.records[pos]
		if record != nil {
			var currentChannel *CurrentChannel
			if record.CurrentChannel != nil {
				cc := *record.CurrentChannel
				currentChannel = &cc
			}
			attempts := append([]ChannelAttempt(nil), record.ChannelAttempts...)
			update = &ChannelUpdate{
				RequestID:       record.ID,
				CurrentPhase:    record.CurrentPhase,
				CurrentChannel:  currentChannel,
				ChannelAttempts: attempts,
			}
		}
	}
	s.mu.RUnlock()
	if update == nil {
		return
	}

	s.hub.Broadcast(&WSMessage{
		Type:    WSMessageTypeChannel,
		Payload: update,
	})
}

// GetAllSnapshot returns deep-copied records in chronological order, suitable for
// JSON serialization without racing with concurrent updates.
func (s *Store) GetAllSnapshot() []*RequestRecord {
	s.mu.RLock()
	defer s.mu.RUnlock()

	result := make([]*RequestRecord, 0, s.count)

	if s.count < MaxRecords {
		for i := 0; i < s.count; i++ {
			if s.records[i] != nil {
				result = append(result, cloneRequestRecordForAPI(s.records[i]))
			}
		}
	} else {
		for i := 0; i < MaxRecords; i++ {
			pos := (s.head + i) % MaxRecords
			if s.records[pos] != nil {
				result = append(result, cloneRequestRecordForAPI(s.records[pos]))
			}
		}
	}

	return result
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
				result = append(result, s.records[i])
			}
		}
	} else {
		// Buffer is full, start from head (oldest)
		for i := 0; i < MaxRecords; i++ {
			pos := (s.head + i) % MaxRecords
			if s.records[pos] != nil {
				result = append(result, s.records[pos])
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
		if record != nil && isActiveStatus(record.Status) {
			result = append(result, record)
		}
	}

	return result
}

// GetActiveSnapshot returns deep-copied active records without racing with updates.
func (s *Store) GetActiveSnapshot() []*RequestRecord {
	s.mu.RLock()
	defer s.mu.RUnlock()

	result := make([]*RequestRecord, 0)

	for _, record := range s.records {
		if record != nil && isActiveStatus(record.Status) {
			result = append(result, cloneRequestRecordForAPI(record))
		}
	}

	return result
}

// GetStats returns monitoring statistics
func (s *Store) GetStats() MonitorStats {
	s.mu.RLock()
	defer s.mu.RUnlock()

	stats := MonitorStats{}
	stats.TotalRequests = s.count
	var memoryBytes int64

	for _, record := range s.records {
		if record == nil {
			continue
		}
		switch record.Status {
		case StatusProcessing, StatusWaitingUpstream, StatusStreaming:
			stats.ActiveRequests++
		case StatusCompleted:
			stats.Completed++
		case StatusError:
			stats.Errors++
		}

		// Calculate memory for this record
		memoryBytes += record.EstimateSize()
	}

	// Add index map overhead (approximate)
	// Each map entry: key (string) + value (int) + overhead
	memoryBytes += int64(len(s.index) * (32 + 8 + 16))

	stats.MemoryBytes = memoryBytes

	return stats
}

// GetMemoryUsage calculates the approximate memory used by the monitor store
func (s *Store) GetMemoryUsage() int64 {
	s.mu.RLock()
	defer s.mu.RUnlock()

	var totalBytes int64

	// Calculate memory for each RequestRecord
	for _, record := range s.records {
		if record != nil {
			totalBytes += record.EstimateSize()
		}
	}

	// Add index map overhead (approximate)
	// Each map entry: key (string) + value (int) + overhead
	totalBytes += int64(len(s.index) * (32 + 8 + 16))

	return totalBytes
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

// CheckBodySize checks if body exceeds the size threshold
// Returns the full body and a boolean indicating whether it exceeds the threshold
func CheckBodySize(body string) (string, bool) {
	return body, len(body) > BodySizeThreshold
}
