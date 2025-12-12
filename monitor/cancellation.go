package monitor

import (
	"context"
	"sync"
)

// CancellationRegistry manages cancel functions for active requests
type CancellationRegistry struct {
	mu      sync.RWMutex
	cancels map[string]context.CancelFunc // requestID -> cancelFunc
}

var globalRegistry *CancellationRegistry

func init() {
	globalRegistry = &CancellationRegistry{
		cancels: make(map[string]context.CancelFunc),
	}
}

// RegisterCancel stores a cancel function for a request
func (r *CancellationRegistry) RegisterCancel(requestID string, cancel context.CancelFunc) {
	if requestID == "" || cancel == nil {
		return
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	r.cancels[requestID] = cancel
}

// CancelRequest triggers cancellation for a request and removes it from registry
// Returns true if cancel function was found and called
func (r *CancellationRegistry) CancelRequest(requestID string) bool {
	r.mu.Lock()
	defer r.mu.Unlock()
	cancel, exists := r.cancels[requestID]
	if !exists {
		return false
	}

	// Call the cancel function
	cancel()
	// Remove from registry
	delete(r.cancels, requestID)

	return true
}

// UnregisterCancel removes a cancel function from registry (called when request completes)
func (r *CancellationRegistry) UnregisterCancel(requestID string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	delete(r.cancels, requestID)
}

// GetRegistry returns the global registry
func GetRegistry() *CancellationRegistry {
	return globalRegistry
}
