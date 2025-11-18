package sse

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/rohitkeshwani07/chat/backend/internal/models"
)

// Connection represents an active SSE connection
type Connection struct {
	ID         string
	SessionID  string
	UserID     string
	Writer     http.ResponseWriter
	Flusher    http.Flusher
	Done       chan struct{}
	CreatedAt  time.Time
	LastSent   time.Time
	ClientIP   string
	UserAgent  string
}

// Manager manages all active SSE connections
type Manager struct {
	connections map[string]*Connection // connection_id -> connection
	sessions    map[string][]*Connection // session_id -> connections
	mu          sync.RWMutex
	logger      *log.Logger
}

// NewManager creates a new SSE connection manager
func NewManager(logger *log.Logger) *Manager {
	return &Manager{
		connections: make(map[string]*Connection),
		sessions:    make(map[string][]*Connection),
		logger:      logger,
	}
}

// AddConnection registers a new SSE connection
func (m *Manager) AddConnection(sessionID, userID string, w http.ResponseWriter, r *http.Request) (*Connection, error) {
	flusher, ok := w.(http.Flusher)
	if !ok {
		return nil, fmt.Errorf("streaming not supported")
	}

	// Set SSE headers
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.Header().Set("X-Accel-Buffering", "no") // Disable nginx buffering

	conn := &Connection{
		ID:        uuid.New().String(),
		SessionID: sessionID,
		UserID:    userID,
		Writer:    w,
		Flusher:   flusher,
		Done:      make(chan struct{}),
		CreatedAt: time.Now(),
		LastSent:  time.Now(),
		ClientIP:  r.RemoteAddr,
		UserAgent: r.UserAgent(),
	}

	m.mu.Lock()
	m.connections[conn.ID] = conn
	m.sessions[sessionID] = append(m.sessions[sessionID], conn)
	m.mu.Unlock()

	if m.logger != nil {
		m.logger.Printf("Added SSE connection: %s for session %s", conn.ID, sessionID)
	}

	// Send initial connection event
	m.SendEvent(conn.ID, &models.SSEEvent{
		Event: "connected",
		Data: map[string]string{
			"connection_id": conn.ID,
			"session_id":    sessionID,
		},
	})

	return conn, nil
}

// RemoveConnection removes a connection
func (m *Manager) RemoveConnection(connectionID string) {
	m.mu.Lock()
	defer m.mu.Unlock()

	conn, exists := m.connections[connectionID]
	if !exists {
		return
	}

	// Remove from connections map
	delete(m.connections, connectionID)

	// Remove from sessions map
	if conns, ok := m.sessions[conn.SessionID]; ok {
		for i, c := range conns {
			if c.ID == connectionID {
				m.sessions[conn.SessionID] = append(conns[:i], conns[i+1:]...)
				break
			}
		}

		// Clean up empty session entry
		if len(m.sessions[conn.SessionID]) == 0 {
			delete(m.sessions, conn.SessionID)
		}
	}

	// Close the done channel
	close(conn.Done)

	if m.logger != nil {
		m.logger.Printf("Removed SSE connection: %s", connectionID)
	}
}

// SendEvent sends an SSE event to a specific connection
func (m *Manager) SendEvent(connectionID string, event *models.SSEEvent) error {
	m.mu.RLock()
	conn, exists := m.connections[connectionID]
	m.mu.RUnlock()

	if !exists {
		return fmt.Errorf("connection not found: %s", connectionID)
	}

	return m.writeEvent(conn, event)
}

// SendToSession sends an event to all connections for a session
func (m *Manager) SendToSession(sessionID string, event *models.SSEEvent) error {
	m.mu.RLock()
	conns := m.sessions[sessionID]
	m.mu.RUnlock()

	if len(conns) == 0 {
		return fmt.Errorf("no connections for session: %s", sessionID)
	}

	var lastErr error
	for _, conn := range conns {
		if err := m.writeEvent(conn, event); err != nil {
			lastErr = err
			if m.logger != nil {
				m.logger.Printf("Failed to send to connection %s: %v", conn.ID, err)
			}
		}
	}

	return lastErr
}

// SendChunk sends a response chunk to all connections for a session
func (m *Manager) SendChunk(sessionID string, chunk *models.ResponseChunk) error {
	event := &models.SSEEvent{
		Event: "chunk",
		Data:  chunk,
	}

	return m.SendToSession(sessionID, event)
}

// writeEvent writes an SSE event to a connection
func (m *Manager) writeEvent(conn *Connection, event *models.SSEEvent) error {
	// Format SSE message
	var message string

	if event.ID != "" {
		message += fmt.Sprintf("id: %s\n", event.ID)
	}

	if event.Event != "" {
		message += fmt.Sprintf("event: %s\n", event.Event)
	}

	// Marshal data to JSON
	dataJSON, err := json.Marshal(event.Data)
	if err != nil {
		return fmt.Errorf("failed to marshal event data: %w", err)
	}

	message += fmt.Sprintf("data: %s\n\n", string(dataJSON))

	// Write to connection
	if _, err := fmt.Fprint(conn.Writer, message); err != nil {
		return fmt.Errorf("failed to write event: %w", err)
	}

	// Flush immediately
	conn.Flusher.Flush()

	// Update last sent time
	conn.LastSent = time.Now()

	return nil
}

// GetConnection retrieves a connection by ID
func (m *Manager) GetConnection(connectionID string) (*Connection, bool) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	conn, exists := m.connections[connectionID]
	return conn, exists
}

// GetSessionConnections returns all connections for a session
func (m *Manager) GetSessionConnections(sessionID string) []*Connection {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.sessions[sessionID]
}

// HasActiveConnections checks if a session has any active connections
func (m *Manager) HasActiveConnections(sessionID string) bool {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return len(m.sessions[sessionID]) > 0
}

// GetConnectionCount returns the total number of active connections
func (m *Manager) GetConnectionCount() int {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return len(m.connections)
}

// GetSessionCount returns the number of sessions with active connections
func (m *Manager) GetSessionCount() int {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return len(m.sessions)
}

// SendHeartbeat sends a heartbeat/ping to all connections
func (m *Manager) SendHeartbeat() {
	m.mu.RLock()
	conns := make([]*Connection, 0, len(m.connections))
	for _, conn := range m.connections {
		conns = append(conns, conn)
	}
	m.mu.RUnlock()

	event := &models.SSEEvent{
		Event: "ping",
		Data: map[string]interface{}{
			"timestamp": time.Now().Unix(),
		},
	}

	for _, conn := range conns {
		if err := m.writeEvent(conn, event); err != nil {
			if m.logger != nil {
				m.logger.Printf("Heartbeat failed for connection %s: %v", conn.ID, err)
			}
			// Connection likely dead, will be cleaned up
		}
	}
}

// StartHeartbeat starts sending periodic heartbeats
func (m *Manager) StartHeartbeat(interval time.Duration) {
	ticker := time.NewTicker(interval)
	go func() {
		for range ticker.C {
			m.SendHeartbeat()
		}
	}()
}
