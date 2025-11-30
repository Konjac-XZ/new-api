package monitor

import (
	"log"
	"sync"
	"time"
)

const (
	MaxRecords  = 100
	MaxBodySize = 10 * 1024 // 10KB max body storage per request
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

	log.Printf("[Monitor Store] Add: record.ID=%s, head=%d, count=%d", record.ID, s.head, s.count)

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

	log.Printf("[Monitor Store] Add complete: new head=%d, new count=%d, hub=%p", s.head, s.count, s.hub)

	// Broadcast new record summary to WebSocket clients
	if s.hub != nil {
		log.Printf("[Monitor Store] Broadcasting new record summary to WebSocket clients")
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
		log.Printf("[Monitor Store] Update skipped: id=%s not found", id)
		return
	}

	record := s.records[pos]
	if record == nil {
		log.Printf("[Monitor Store] Update skipped: record nil for id=%s", id)
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

// GetAll returns all stored records in chronological order (oldest first)
func (s *Store) GetAll() []*RequestRecord {
	s.mu.RLock()
	defer s.mu.RUnlock()

	log.Printf("[Monitor Store] GetAll: count=%d, head=%d", s.count, s.head)

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

	log.Printf("[Monitor Store] GetAll: returning %d records", len(result))
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
	log.Printf("[Monitor Store] MarkComplete: id=%s, hasResponse=%t", id, response != nil)
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
