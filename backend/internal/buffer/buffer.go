package buffer

import (
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/rohitkeshwani07/chat/backend/internal/models"
)

// ChunkBuffer holds chunks for a single message being streamed
type ChunkBuffer struct {
	SessionID    string
	MessageID    string
	Chunks       map[int]*models.ResponseChunk
	MaxChunkID   int
	NextToSend   int
	IsFinal      bool
	FinalChunkID int
	CreatedAt    time.Time
	UpdatedAt    time.Time
	mu           sync.RWMutex
}

// Manager manages all active chunk buffers for a pod
type Manager struct {
	buffers             map[string]*ChunkBuffer
	mu                  sync.RWMutex
	maxBuffersPerPod    int
	maxChunksPerBuffer  int
	maxBufferAge        time.Duration
	cleanupInterval     time.Duration
	missingChunkTimeout time.Duration
	stopCleanup         chan struct{}
}

// NewManager creates a new buffer manager
func NewManager(maxBuffers, maxChunks int, maxAge, cleanupInterval, missingChunkTimeout time.Duration) *Manager {
	return &Manager{
		buffers:             make(map[string]*ChunkBuffer),
		maxBuffersPerPod:    maxBuffers,
		maxChunksPerBuffer:  maxChunks,
		maxBufferAge:        maxAge,
		cleanupInterval:     cleanupInterval,
		missingChunkTimeout: missingChunkTimeout,
		stopCleanup:         make(chan struct{}),
	}
}

// AddChunk adds a chunk to the appropriate buffer
func (m *Manager) AddChunk(chunk *models.ResponseChunk) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	// Check global buffer limit
	if len(m.buffers) >= m.maxBuffersPerPod {
		return fmt.Errorf("buffer limit reached: %d buffers", m.maxBuffersPerPod)
	}

	// Get or create buffer for this message
	buffer, exists := m.buffers[chunk.MessageID]
	if !exists {
		buffer = &ChunkBuffer{
			SessionID:  chunk.SessionID,
			MessageID:  chunk.MessageID,
			Chunks:     make(map[int]*models.ResponseChunk),
			NextToSend: 0,
			CreatedAt:  time.Now(),
		}
		m.buffers[chunk.MessageID] = buffer
	}

	buffer.mu.Lock()
	defer buffer.mu.Unlock()

	// Check per-buffer chunk limit
	if len(buffer.Chunks) >= m.maxChunksPerBuffer {
		return fmt.Errorf("chunk limit reached for message %s: %d chunks",
			chunk.MessageID, m.maxChunksPerBuffer)
	}

	// Check for duplicate
	if existing, exists := buffer.Chunks[chunk.ChunkID]; exists {
		// Verify content matches
		if existing.Chunk != chunk.Chunk {
			// Different content - this is concerning but we'll keep the first one
			return nil
		}
		// Duplicate with same content - ignore
		return nil
	}

	// Store chunk
	buffer.Chunks[chunk.ChunkID] = chunk
	buffer.UpdatedAt = time.Now()

	// Track max chunk ID
	if chunk.ChunkID > buffer.MaxChunkID {
		buffer.MaxChunkID = chunk.ChunkID
	}

	// Mark if final chunk received
	if chunk.IsFinal {
		buffer.IsFinal = true
		buffer.FinalChunkID = chunk.ChunkID
	}

	return nil
}

// GetNextChunks retrieves all consecutive chunks starting from NextToSend
func (m *Manager) GetNextChunks(messageID string) ([]*models.ResponseChunk, bool, error) {
	m.mu.RLock()
	buffer, exists := m.buffers[messageID]
	m.mu.RUnlock()

	if !exists {
		return nil, false, fmt.Errorf("buffer not found for message %s", messageID)
	}

	buffer.mu.Lock()
	defer buffer.mu.Unlock()

	var chunksToSend []*models.ResponseChunk

	// Collect all consecutive chunks starting from NextToSend
	for {
		chunk, exists := buffer.Chunks[buffer.NextToSend]
		if !exists {
			// Gap detected - missing chunk(s)
			break
		}

		chunksToSend = append(chunksToSend, chunk)
		buffer.NextToSend++

		// Check if this was the final chunk
		if chunk.IsFinal {
			break
		}
	}

	// Determine if message is complete
	isComplete := buffer.IsFinal && buffer.NextToSend > buffer.FinalChunkID

	return chunksToSend, isComplete, nil
}

// FinalizeMessage assembles the complete message and removes it from buffers
func (m *Manager) FinalizeMessage(messageID string) (*models.Message, error) {
	m.mu.Lock()
	buffer, exists := m.buffers[messageID]
	if exists {
		delete(m.buffers, messageID) // Remove from active buffers
	}
	m.mu.Unlock()

	if !exists {
		return nil, fmt.Errorf("buffer not found for message %s", messageID)
	}

	buffer.mu.Lock()
	defer buffer.mu.Unlock()

	// Verify all chunks received
	if !buffer.IsFinal {
		return nil, fmt.Errorf("message %s not yet finalized", messageID)
	}

	// Verify no missing chunks
	for i := 0; i <= buffer.FinalChunkID; i++ {
		if _, exists := buffer.Chunks[i]; !exists {
			return nil, fmt.Errorf("missing chunk %d for message %s", i, messageID)
		}
	}

	// Reconstruct full message content from chunks
	var fullContent strings.Builder
	var totalTokens int
	var metadata map[string]interface{}

	for i := 0; i <= buffer.FinalChunkID; i++ {
		chunk := buffer.Chunks[i]

		if chunk.ChunkType == "content" {
			fullContent.WriteString(chunk.Chunk)
		}

		// Extract metadata from final chunk
		if chunk.IsFinal && chunk.Metadata != nil {
			metadata = chunk.Metadata
			if tokens, ok := chunk.Metadata["tokens_used"].(float64); ok {
				totalTokens = int(tokens)
			}
		}
	}

	// Create complete message for database persistence
	message := &models.Message{
		MessageID:  messageID,
		SessionID:  buffer.SessionID,
		Role:       "assistant",
		Content:    fullContent.String(),
		TokenCount: totalTokens,
		Metadata:   metadata,
		CreatedAt:  time.Now(),
	}

	return message, nil
}

// GetState returns the current state of a buffer
func (m *Manager) GetState(messageID string) (*models.BufferState, error) {
	m.mu.RLock()
	buffer, exists := m.buffers[messageID]
	m.mu.RUnlock()

	if !exists {
		return nil, fmt.Errorf("buffer not found for message %s", messageID)
	}

	buffer.mu.RLock()
	defer buffer.mu.RUnlock()

	state := &models.BufferState{
		TotalReceived: len(buffer.Chunks),
		TotalSent:     buffer.NextToSend,
	}

	if buffer.IsFinal {
		state.TotalExpected = buffer.FinalChunkID + 1

		// Find missing chunks
		for i := 0; i <= buffer.FinalChunkID; i++ {
			if _, exists := buffer.Chunks[i]; !exists {
				state.MissingChunks = append(state.MissingChunks, i)
			}
		}
		state.IsComplete = len(state.MissingChunks) == 0
	}

	return state, nil
}

// StartCleanup starts the background cleanup goroutine
func (m *Manager) StartCleanup() {
	ticker := time.NewTicker(m.cleanupInterval)
	go func() {
		defer ticker.Stop()
		for {
			select {
			case <-m.stopCleanup:
				return
			case <-ticker.C:
				m.cleanup()
			}
		}
	}()
}

// StopCleanup stops the background cleanup goroutine
func (m *Manager) StopCleanup() {
	close(m.stopCleanup)
}

// cleanup removes stale buffers
func (m *Manager) cleanup() {
	m.mu.Lock()
	defer m.mu.Unlock()

	now := time.Now()
	staleThreshold := now.Add(-m.maxBufferAge)

	var staleBuffers []string

	for messageID, buffer := range m.buffers {
		buffer.mu.RLock()
		isStale := buffer.UpdatedAt.Before(staleThreshold)
		isFinal := buffer.IsFinal
		buffer.mu.RUnlock()

		// Remove if:
		// 1. No updates in maxBufferAge AND not finalized (likely stuck)
		// 2. Finalized but still in memory (should have been cleaned up)
		if isStale || isFinal {
			staleBuffers = append(staleBuffers, messageID)
		}
	}

	// Remove stale buffers
	for _, messageID := range staleBuffers {
		delete(m.buffers, messageID)
	}
}

// GetBufferCount returns the number of active buffers
func (m *Manager) GetBufferCount() int {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return len(m.buffers)
}
