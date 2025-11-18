package models

import (
	"time"
)

// User represents a user in the system
type User struct {
	UserID    string    `json:"user_id" db:"user_id"`
	Username  string    `json:"username" db:"username"`
	Email     string    `json:"email" db:"email"`
	CreatedAt time.Time `json:"created_at" db:"created_at"`
	UpdatedAt time.Time `json:"updated_at" db:"updated_at"`
	IsActive  bool      `json:"is_active" db:"is_active"`
}

// ChatSession represents a chat session
type ChatSession struct {
	SessionID     string     `json:"session_id" db:"session_id"`
	UserID        string     `json:"user_id" db:"user_id"`
	Title         string     `json:"title" db:"title"`
	AIProvider    string     `json:"ai_provider" db:"ai_provider"`
	ModelName     string     `json:"model_name" db:"model_name"`
	CreatedAt     time.Time  `json:"created_at" db:"created_at"`
	UpdatedAt     time.Time  `json:"updated_at" db:"updated_at"`
	LastMessageAt *time.Time `json:"last_message_at" db:"last_message_at"`
	IsActive      bool       `json:"is_active" db:"is_active"`
}

// Message represents a chat message
type Message struct {
	MessageID  string                 `json:"message_id" db:"message_id"`
	SessionID  string                 `json:"session_id" db:"session_id"`
	Role       string                 `json:"role" db:"role"` // user, assistant, system
	Content    string                 `json:"content" db:"content"`
	CreatedAt  time.Time              `json:"created_at" db:"created_at"`
	TokenCount int                    `json:"token_count" db:"token_count"`
	Metadata   map[string]interface{} `json:"metadata" db:"metadata"`
}

// ActiveConnection represents an active SSE connection
type ActiveConnection struct {
	ConnectionID  string    `json:"connection_id" db:"connection_id"`
	SessionID     string    `json:"session_id" db:"session_id"`
	PodID         string    `json:"pod_id" db:"pod_id"`
	UserID        string    `json:"user_id" db:"user_id"`
	ConnectedAt   time.Time `json:"connected_at" db:"connected_at"`
	LastHeartbeat time.Time `json:"last_heartbeat" db:"last_heartbeat"`
	ClientIP      string    `json:"client_ip" db:"client_ip"`
	UserAgent     string    `json:"user_agent" db:"user_agent"`
}

// WorkflowRequest represents a request to execute a workflow
type WorkflowRequest struct {
	MessageID     string                 `json:"message_id"`
	SessionID     string                 `json:"session_id"`
	UserID        string                 `json:"user_id"`
	Message       string                 `json:"message"`
	Context       map[string]interface{} `json:"context"`
	Timestamp     time.Time              `json:"timestamp"`
	CorrelationID string                 `json:"correlation_id"`
}

// ResponseChunk represents a chunk of response from the workflow service
type ResponseChunk struct {
	SessionID     string                 `json:"session_id"`
	MessageID     string                 `json:"message_id"`
	ChunkID       int                    `json:"chunk_id"`
	Chunk         string                 `json:"chunk"`
	ChunkType     string                 `json:"chunk_type"` // content, metadata, error, system
	IsFinal       bool                   `json:"is_final"`
	Metadata      map[string]interface{} `json:"metadata,omitempty"`
	Error         *ErrorInfo             `json:"error,omitempty"`
	Timestamp     time.Time              `json:"timestamp"`
	CorrelationID string                 `json:"correlation_id"`
}

// ErrorInfo represents error information in a response chunk
type ErrorInfo struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

// ChatRequest represents an incoming chat message request
type ChatRequest struct {
	SessionID  string                 `json:"session_id"`
	Message    string                 `json:"message"`
	UserID     string                 `json:"user_id"`
	AIProvider string                 `json:"ai_provider,omitempty"`
	Model      string                 `json:"model,omitempty"`
	Context    map[string]interface{} `json:"context,omitempty"`
}

// ChatResponse represents the response to a chat request
type ChatResponse struct {
	MessageID     string    `json:"message_id"`
	SessionID     string    `json:"session_id"`
	Status        string    `json:"status"`
	Timestamp     time.Time `json:"timestamp"`
	CorrelationID string    `json:"correlation_id"`
}

// SSEEvent represents a Server-Sent Event
type SSEEvent struct {
	Event string      `json:"-"`
	Data  interface{} `json:"data"`
	ID    string      `json:"-"`
}

// BufferState represents the current state of a chunk buffer
type BufferState struct {
	TotalExpected int   `json:"total_expected"`
	TotalReceived int   `json:"total_received"`
	TotalSent     int   `json:"total_sent"`
	MissingChunks []int `json:"missing_chunks"`
	IsComplete    bool  `json:"is_complete"`
}
