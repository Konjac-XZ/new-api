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
	records []*RequestRecord
	index   map[string]int // ID -> position mapping
	mu      sync.RWMutex
	head    int
	count   int
	hub     *Hub
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

// Add adds a new record to the store
func (s *Store) Add(record *RequestRecord) {
	s.mu.Lock()
	defer s.mu.Unlock()

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

	// Broadcast new record summary to WebSocket clients
	if s.hub != nil {
		s.hub.Broadcast(&WSMessage{
			Type:    WSMessageTypeNew,
			Payload: record.ToSummary(),
		})
	}
}

// Update updates an existing record by ID
func (s *Store) Update(id string, updater func(*RequestRecord)) {
	s.mu.Lock()
	defer s.mu.Unlock()

	pos, exists := s.index[id]
	if !exists {
		return
	}

	record := s.records[pos]
	if record == nil {
		return
	}

	updater(record)

	// Broadcast update summary to WebSocket clients
	if s.hub != nil {
		s.hub.Broadcast(&WSMessage{
			Type:    WSMessageTypeUpdate,
			Payload: record.ToSummary(),
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

	record := s.Get(id)
	if record == nil {
		return
	}

	update := record.ToChannelUpdate()
	if update == nil {
		return
	}

	s.hub.Broadcast(&WSMessage{
		Type:    WSMessageTypeChannel,
		Payload: update,
	})
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

// GetStats returns monitoring statistics
func (s *Store) GetStats() MonitorStats {
	s.mu.RLock()
	defer s.mu.RUnlock()

	stats := MonitorStats{}
	var memoryBytes int64

	for _, record := range s.records {
		if record == nil {
			continue
		}
		stats.TotalRequests++
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
