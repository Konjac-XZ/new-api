package monitor

import (
	"sync"
	"sync/atomic"
)

// Manager manages the monitor system with thread-safe operations
type Manager struct {
	store   *Store
	hub     *Hub
	enabled atomic.Bool
	mu      sync.RWMutex
}

var (
	defaultManager *Manager
	managerOnce    sync.Once
)

// GetManager returns the singleton Manager instance
func GetManager() *Manager {
	managerOnce.Do(func() {
		defaultManager = &Manager{}
		defaultManager.enabled.Store(true)
	})
	return defaultManager
}

// Init initializes the monitor system
func (m *Manager) Init() {
	m.mu.Lock()
	defer m.mu.Unlock()

	if m.hub == nil {
		m.hub = NewHub()
		m.store = NewStore()
		go m.hub.Run()
		go m.wireStoreEvents()
	}
}

// wireStoreEvents subscribes to Store events and forwards them to Hub
func (m *Manager) wireStoreEvents() {
	for event := range m.store.Events() {
		if m.hub == nil {
			continue
		}

		var wsMessageType string
		switch event.Type {
		case EventTypeNew:
			wsMessageType = WSMessageTypeNew
		case EventTypeUpdate:
			wsMessageType = WSMessageTypeUpdate
		case EventTypeChannel:
			wsMessageType = WSMessageTypeChannel
		default:
			continue
		}

		m.hub.Broadcast(&WSMessage{
			Type:    wsMessageType,
			Payload: event.Payload,
		})
	}
}

// GetStore returns the Store instance
func (m *Manager) GetStore() *Store {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.store
}

// GetHub returns the Hub instance
func (m *Manager) GetHub() *Hub {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.hub
}

// IsEnabled returns whether monitoring is enabled
func (m *Manager) IsEnabled() bool {
	return m.enabled.Load()
}

// SetEnabled enables or disables monitoring
func (m *Manager) SetEnabled(enable bool) {
	m.enabled.Store(enable)
}
