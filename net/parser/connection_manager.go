package parser

import (
	"fmt"
	"sync"
)

// ConnectionManager indexes live connections by both connection ID and bound UID.
type ConnectionManager struct {
	mu          sync.RWMutex
	connections map[string]*Connection
	byUID       map[int64]string
}

// NewConnectionManager creates an empty connection index.
func NewConnectionManager() *ConnectionManager {
	return &ConnectionManager{connections: make(map[string]*Connection), byUID: make(map[int64]string)}
}

// Add indexes a newly accepted connection.
func (m *ConnectionManager) Add(connection *Connection) {
	if connection != nil {
		m.mu.Lock()
		m.connections[connection.ID()] = connection
		m.mu.Unlock()
	}
}

// Remove deletes both the connection ID and any UID binding.
func (m *ConnectionManager) Remove(id string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	connection := m.connections[id]
	delete(m.connections, id)
	if connection != nil {
		if uid := connection.UID(); uid > 0 && m.byUID[uid] == id {
			delete(m.byUID, uid)
		}
	}
}

// Get looks up a connection by its server-assigned ID.
func (m *ConnectionManager) Get(id string) (*Connection, bool) {
	m.mu.RLock()
	connection, ok := m.connections[id]
	m.mu.RUnlock()
	return connection, ok
}

// GetUID looks up the connection currently bound to uid.
func (m *ConnectionManager) GetUID(uid int64) (*Connection, bool) {
	m.mu.RLock()
	id, ok := m.byUID[uid]
	connection := m.connections[id]
	m.mu.RUnlock()
	return connection, ok && connection != nil
}

// Bind enforces a one-to-one UID-to-connection mapping and replaces the
// server-owned session data exposed to subsequent Actor requests.
func (m *ConnectionManager) Bind(id string, uid int64, data map[string]string) error {
	if uid <= 0 {
		return fmt.Errorf("actorgo agp: uid must be positive")
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	connection := m.connections[id]
	if connection == nil {
		return fmt.Errorf("actorgo agp: connection %q not found", id)
	}
	if existing := m.byUID[uid]; existing != "" && existing != id {
		return fmt.Errorf("actorgo agp: uid %d is already bound", uid)
	}
	if oldUID := connection.UID(); oldUID > 0 && m.byUID[oldUID] == id {
		delete(m.byUID, oldUID)
	}
	connection.bind(uid, data)
	m.byUID[uid] = id
	return nil
}

// Count returns the number of active indexed connections.
func (m *ConnectionManager) Count() int {
	m.mu.RLock()
	count := len(m.connections)
	m.mu.RUnlock()
	return count
}

// Snapshot returns the current connections without holding the manager lock.
func (m *ConnectionManager) Snapshot() []*Connection {
	m.mu.RLock()
	connections := make([]*Connection, 0, len(m.connections))
	for _, connection := range m.connections {
		connections = append(connections, connection)
	}
	m.mu.RUnlock()
	return connections
}

// Range visits a stable snapshot and stops when fn returns false.
func (m *ConnectionManager) Range(fn func(*Connection) bool) {
	// Snapshot under the lock, then invoke user code without holding it.
	for _, connection := range m.Snapshot() {
		if !fn(connection) {
			return
		}
	}
}

// CloseAll idempotently closes every indexed connection.
func (m *ConnectionManager) CloseAll() {
	m.Range(func(connection *Connection) bool { connection.Close(); return true })
}
