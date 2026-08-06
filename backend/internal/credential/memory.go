// Package credential provides ephemeral credential handling for live platform
// sessions. Passwords are never serialized, logged, or written to storage.
package credential

import "sync"

// Value is a username/password pair held only for the backend process lifetime.
type Value struct {
	Username string
	Password string
}

// Memory is a thread-safe, process-local credential store keyed by connection ID.
type Memory struct {
	mu     sync.RWMutex
	values map[string]Value
}

func NewMemory() *Memory {
	return &Memory{values: make(map[string]Value)}
}

func (m *Memory) Put(connectionID string, value Value) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.values[connectionID] = value
}

func (m *Memory) Get(connectionID string) (Value, bool) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	v, ok := m.values[connectionID]
	return v, ok
}

func (m *Memory) Delete(connectionID string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	delete(m.values, connectionID)
}
