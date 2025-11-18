# Chunk Buffering and Message Ordering

## Overview

The Chat Service implements an in-memory buffering strategy for response chunks received from NATS. This ensures:
1. **Ordered delivery** to clients even when chunks arrive out of order
2. **Memory efficiency** by avoiding database writes for every chunk
3. **Atomic persistence** by committing complete messages only after final chunk

## Design Principles

### 1. Memory-First Buffering
- All chunks for an in-progress message are stored in memory
- Only the complete, final message is written to the database
- Reduces database write load significantly
- Enables fast chunk streaming to clients

### 2. Out-of-Order Handling
- NATS does not guarantee message ordering across different publishers
- Workflow Service may publish chunks that arrive out of order at pods
- Chat Service must buffer and reorder chunks before streaming to client

### 3. Atomic Persistence
- Database write happens only once per message
- Either complete message is saved or nothing (on failure)
- Prevents partial messages in database

## In-Memory Chunk Buffer

### Data Structure

```go
// ChunkBuffer holds chunks for a single message being streamed
type ChunkBuffer struct {
    SessionID     string
    MessageID     string
    Chunks        map[int]*ResponseChunk  // chunk_id -> chunk
    MaxChunkID    int                      // highest chunk_id seen
    NextToSend    int                      // next expected chunk_id to send to client
    IsFinal       bool                     // has final chunk been received?
    FinalChunkID  int                      // chunk_id of final chunk
    CreatedAt     time.Time
    UpdatedAt     time.Time
    mu            sync.RWMutex             // protects concurrent access
}

// ResponseChunk represents a single chunk from NATS
type ResponseChunk struct {
    SessionID     string    `json:"session_id"`
    MessageID     string    `json:"message_id"`
    ChunkID       int       `json:"chunk_id"`
    Chunk         string    `json:"chunk"`
    ChunkType     string    `json:"chunk_type"`  // content, metadata, error
    IsFinal       bool      `json:"is_final"`
    Metadata      map[string]interface{} `json:"metadata"`
    Timestamp     time.Time `json:"timestamp"`
    CorrelationID string    `json:"correlation_id"`
}

// BufferManager manages all active chunk buffers for a pod
type BufferManager struct {
    buffers map[string]*ChunkBuffer  // messageID -> buffer
    mu      sync.RWMutex

    // Configuration
    maxBufferAge time.Duration  // how long to keep incomplete buffers
    cleanupInterval time.Duration
}
```

### Buffer Operations

#### 1. Add Chunk (Incoming from NATS)

```go
func (bm *BufferManager) AddChunk(chunk *ResponseChunk) error {
    bm.mu.Lock()
    defer bm.mu.Unlock()

    // Get or create buffer for this message
    buffer, exists := bm.buffers[chunk.MessageID]
    if !exists {
        buffer = &ChunkBuffer{
            SessionID:  chunk.SessionID,
            MessageID:  chunk.MessageID,
            Chunks:     make(map[int]*ResponseChunk),
            NextToSend: 0,
            CreatedAt:  time.Now(),
        }
        bm.buffers[chunk.MessageID] = buffer
    }

    buffer.mu.Lock()
    defer buffer.mu.Unlock()

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
```

#### 2. Get Next Ordered Chunks (For SSE Streaming)

```go
func (bm *BufferManager) GetNextChunks(messageID string) ([]*ResponseChunk, bool, error) {
    bm.mu.RLock()
    buffer, exists := bm.buffers[messageID]
    bm.mu.RUnlock()

    if !exists {
        return nil, false, fmt.Errorf("buffer not found for message %s", messageID)
    }

    buffer.mu.Lock()
    defer buffer.mu.Unlock()

    var chunksToSend []*ResponseChunk

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
```

#### 3. Finalize and Persist

```go
func (bm *BufferManager) FinalizeMessage(messageID string) (*Message, error) {
    bm.mu.Lock()
    buffer, exists := bm.buffers[messageID]
    if exists {
        delete(bm.buffers, messageID) // Remove from active buffers
    }
    bm.mu.Unlock()

    if !exists {
        return nil, fmt.Errorf("buffer not found for message %s", messageID)
    }

    buffer.mu.Lock()
    defer buffer.mu.Unlock()

    // Verify all chunks received
    if !buffer.IsFinal {
        return nil, fmt.Errorf("message %s not yet finalized", messageID)
    }

    // Reconstruct full message content from chunks
    var fullContent strings.Builder
    var totalTokens int
    var metadata map[string]interface{}

    for i := 0; i <= buffer.FinalChunkID; i++ {
        chunk, exists := buffer.Chunks[i]
        if !exists {
            return nil, fmt.Errorf("missing chunk %d for message %s", i, messageID)
        }

        if chunk.ChunkType == "content" {
            fullContent.WriteString(chunk.Chunk)
        }

        // Extract metadata from final chunk
        if chunk.IsFinal && chunk.Metadata != nil {
            metadata = chunk.Metadata
            if tokens, ok := chunk.Metadata["tokens_used"].(int); ok {
                totalTokens = tokens
            }
        }
    }

    // Create complete message for database persistence
    message := &Message{
        MessageID:  messageID,
        SessionID:  buffer.SessionID,
        Role:       "assistant",
        Content:    fullContent.String(),
        TokenCount: totalTokens,
        Metadata:   metadata,
        CreatedAt:  time.Now(),
    }

    // Persist to database
    if err := db.SaveMessage(message); err != nil {
        return nil, fmt.Errorf("failed to save message: %w", err)
    }

    return message, nil
}
```

## Handling Out-of-Order Chunks

### Example Scenario

Chunks arrive in order: `3, 1, 4, 0, 5(final), 2`

```
Time  | Arrive | Buffer State        | Send to Client | Notes
------|--------|---------------------|----------------|------------------------
T1    | 3      | {3}                 | -              | Gap detected (0-2 missing)
T2    | 1      | {1, 3}              | -              | Gap detected (0 missing)
T3    | 4      | {1, 3, 4}           | -              | Gap detected (0 missing)
T4    | 0      | {0, 1, 3, 4}        | 0, 1           | Send consecutive: 0, 1
T5    | 5      | {0, 1, 3, 4, 5}     | -              | Final received, gap at 2
T6    | 2      | {0, 1, 2, 3, 4, 5}  | 2, 3, 4, 5     | Complete! Send rest
```

### Buffer State Tracking

```go
type BufferState struct {
    TotalExpected int  // FinalChunkID + 1 (if known)
    TotalReceived int  // len(Chunks)
    TotalSent     int  // NextToSend
    MissingChunks []int // chunk IDs still missing
    IsComplete    bool
}

func (b *ChunkBuffer) GetState() BufferState {
    b.mu.RLock()
    defer b.mu.RUnlock()

    state := BufferState{
        TotalReceived: len(b.Chunks),
        TotalSent:     b.NextToSend,
    }

    if b.IsFinal {
        state.TotalExpected = b.FinalChunkID + 1
    }

    // Find missing chunks
    if b.IsFinal {
        for i := 0; i <= b.FinalChunkID; i++ {
            if _, exists := b.Chunks[i]; !exists {
                state.MissingChunks = append(state.MissingChunks, i)
            }
        }
        state.IsComplete = len(state.MissingChunks) == 0
    }

    return state
}
```

## SSE Streaming with Buffering

### Integration with SSE Handler

```go
func (h *SSEHandler) handleResponseChunk(chunk *ResponseChunk) {
    // 1. Add chunk to buffer
    if err := h.bufferManager.AddChunk(chunk); err != nil {
        log.Errorf("Failed to buffer chunk: %v", err)
        return
    }

    // 2. Try to get next ordered chunks
    chunksToSend, isComplete, err := h.bufferManager.GetNextChunks(chunk.MessageID)
    if err != nil {
        log.Errorf("Failed to get next chunks: %v", err)
        return
    }

    // 3. Stream available chunks to client
    for _, c := range chunksToSend {
        if err := h.sendSSEEvent(chunk.SessionID, c); err != nil {
            log.Errorf("Failed to send SSE event: %v", err)
            return
        }
    }

    // 4. If message complete, persist to database
    if isComplete {
        message, err := h.bufferManager.FinalizeMessage(chunk.MessageID)
        if err != nil {
            log.Errorf("Failed to finalize message: %v", err)
            return
        }

        log.Infof("Message %s persisted to database", message.MessageID)

        // Send completion event to client
        h.sendSSEEvent(chunk.SessionID, &ResponseChunk{
            ChunkType: "system",
            Chunk:     "message_complete",
            MessageID: chunk.MessageID,
        })
    }
}

func (h *SSEHandler) sendSSEEvent(sessionID string, chunk *ResponseChunk) error {
    // Find SSE connections for this session
    connections := h.getConnectionsForSession(sessionID)

    // Format SSE message
    data, _ := json.Marshal(chunk)
    sseMessage := fmt.Sprintf("data: %s\n\n", string(data))

    // Send to all active connections
    for _, conn := range connections {
        if _, err := conn.Write([]byte(sseMessage)); err != nil {
            log.Errorf("Failed to write to SSE connection: %v", err)
            // Connection likely dead, will be cleaned up by heartbeat
        }
    }

    return nil
}
```

## Buffer Cleanup and Memory Management

### Automatic Cleanup

```go
func (bm *BufferManager) StartCleanup(ctx context.Context) {
    ticker := time.NewTicker(bm.cleanupInterval)
    defer ticker.Stop()

    for {
        select {
        case <-ctx.Done():
            return
        case <-ticker.C:
            bm.cleanup()
        }
    }
}

func (bm *BufferManager) cleanup() {
    bm.mu.Lock()
    defer bm.mu.Unlock()

    now := time.Now()
    staleThreshold := now.Add(-bm.maxBufferAge)

    var staleBuffers []string

    for messageID, buffer := range bm.buffers {
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
        buffer := bm.buffers[messageID]

        // Log warning for incomplete buffers
        if !buffer.IsFinal {
            state := buffer.GetState()
            log.Warnf("Cleaning up incomplete buffer for message %s: %+v",
                messageID, state)
        }

        delete(bm.buffers, messageID)
    }

    if len(staleBuffers) > 0 {
        log.Infof("Cleaned up %d stale chunk buffers", len(staleBuffers))
    }
}
```

### Memory Limits

```go
// Configuration
const (
    MaxBuffersPerPod    = 10000        // Maximum concurrent buffers
    MaxChunksPerBuffer  = 10000        // Maximum chunks per message
    MaxBufferAge        = 5 * time.Minute  // How long to keep incomplete buffers
    CleanupInterval     = 30 * time.Second
)

func (bm *BufferManager) AddChunk(chunk *ResponseChunk) error {
    bm.mu.Lock()
    defer bm.mu.Unlock()

    // Check global buffer limit
    if len(bm.buffers) >= MaxBuffersPerPod {
        return fmt.Errorf("buffer limit reached: %d buffers", MaxBuffersPerPod)
    }

    buffer, exists := bm.buffers[chunk.MessageID]
    if !exists {
        buffer = newChunkBuffer(chunk.SessionID, chunk.MessageID)
        bm.buffers[chunk.MessageID] = buffer
    }

    buffer.mu.Lock()
    defer buffer.mu.Unlock()

    // Check per-buffer chunk limit
    if len(buffer.Chunks) >= MaxChunksPerBuffer {
        return fmt.Errorf("chunk limit reached for message %s: %d chunks",
            chunk.MessageID, MaxChunksPerBuffer)
    }

    buffer.Chunks[chunk.ChunkID] = chunk
    buffer.UpdatedAt = time.Now()

    if chunk.ChunkID > buffer.MaxChunkID {
        buffer.MaxChunkID = chunk.ChunkID
    }

    if chunk.IsFinal {
        buffer.IsFinal = true
        buffer.FinalChunkID = chunk.ChunkID
    }

    return nil
}
```

## Error Handling

### Missing Chunks Timeout

```go
func (bm *BufferManager) checkMissingChunks(messageID string) error {
    buffer := bm.buffers[messageID]
    if buffer == nil {
        return fmt.Errorf("buffer not found")
    }

    buffer.mu.RLock()
    defer buffer.mu.RUnlock()

    if !buffer.IsFinal {
        return nil // Can't check until we know total chunks
    }

    state := buffer.GetState()
    if len(state.MissingChunks) > 0 {
        // Missing chunks detected
        waitTime := time.Since(buffer.UpdatedAt)

        if waitTime > 30*time.Second {
            // Timeout - chunks likely lost
            return fmt.Errorf("timeout waiting for chunks %v (waited %v)",
                state.MissingChunks, waitTime)
        }

        // Still within timeout, keep waiting
        log.Debugf("Waiting for missing chunks %v for message %s",
            state.MissingChunks, messageID)
    }

    return nil
}
```

### Duplicate Chunk Handling

```go
func (b *ChunkBuffer) AddChunk(chunk *ResponseChunk) error {
    b.mu.Lock()
    defer b.mu.Unlock()

    // Check for duplicate
    if existing, exists := b.Chunks[chunk.ChunkID]; exists {
        // Verify content matches
        if existing.Chunk != chunk.Chunk {
            log.Warnf("Duplicate chunk %d with different content for message %s",
                chunk.ChunkID, chunk.MessageID)
        }
        // Ignore duplicate (already have this chunk)
        return nil
    }

    // Store new chunk
    b.Chunks[chunk.ChunkID] = chunk
    // ... rest of logic

    return nil
}
```

## Monitoring and Metrics

### Key Metrics

```go
type BufferMetrics struct {
    ActiveBuffers       int           // Current number of buffers
    TotalChunksBuffered int           // Total chunks in memory
    AvgChunksPerBuffer  float64       // Average chunks per buffer
    OldestBufferAge     time.Duration // Age of oldest buffer
    OutOfOrderCount     int64         // Count of out-of-order chunks received
    TimeoutCount        int64         // Count of buffers that timed out
}

func (bm *BufferManager) GetMetrics() BufferMetrics {
    bm.mu.RLock()
    defer bm.mu.RUnlock()

    metrics := BufferMetrics{
        ActiveBuffers: len(bm.buffers),
    }

    totalChunks := 0
    var oldestTime time.Time

    for _, buffer := range bm.buffers {
        buffer.mu.RLock()
        totalChunks += len(buffer.Chunks)
        if oldestTime.IsZero() || buffer.CreatedAt.Before(oldestTime) {
            oldestTime = buffer.CreatedAt
        }
        buffer.mu.RUnlock()
    }

    metrics.TotalChunksBuffered = totalChunks
    if metrics.ActiveBuffers > 0 {
        metrics.AvgChunksPerBuffer = float64(totalChunks) / float64(metrics.ActiveBuffers)
    }
    if !oldestTime.IsZero() {
        metrics.OldestBufferAge = time.Since(oldestTime)
    }

    return metrics
}
```

### Logging

```go
// Log when out-of-order chunk received
if chunk.ChunkID < buffer.NextToSend {
    log.Debugf("Out-of-order chunk received: expected %d, got %d (message %s)",
        buffer.NextToSend, chunk.ChunkID, chunk.MessageID)
    metrics.IncrementOutOfOrderCount()
}

// Log when chunks sent
if len(chunksToSend) > 0 {
    log.Infof("Streaming %d chunks for message %s (session %s)",
        len(chunksToSend), chunk.MessageID, chunk.SessionID)
}

// Log when message finalized
log.Infof("Message %s finalized: %d chunks, %d tokens, %d bytes",
    message.MessageID,
    len(buffer.Chunks),
    message.TokenCount,
    len(message.Content))
```

## Testing

### Unit Tests

```go
func TestOutOfOrderChunks(t *testing.T) {
    bm := NewBufferManager()
    messageID := "test-msg"
    sessionID := "test-session"

    // Send chunks out of order: 2, 0, 1, 3(final)
    chunks := []*ResponseChunk{
        {MessageID: messageID, SessionID: sessionID, ChunkID: 2, Chunk: "world"},
        {MessageID: messageID, SessionID: sessionID, ChunkID: 0, Chunk: "hello"},
        {MessageID: messageID, SessionID: sessionID, ChunkID: 1, Chunk: " "},
        {MessageID: messageID, SessionID: sessionID, ChunkID: 3, Chunk: "!", IsFinal: true, FinalChunkID: 3},
    }

    for _, chunk := range chunks {
        err := bm.AddChunk(chunk)
        assert.NoError(t, err)
    }

    // Should get all chunks in order
    ordered, isComplete, err := bm.GetNextChunks(messageID)
    assert.NoError(t, err)
    assert.True(t, isComplete)
    assert.Len(t, ordered, 4)

    // Verify order
    assert.Equal(t, 0, ordered[0].ChunkID)
    assert.Equal(t, 1, ordered[1].ChunkID)
    assert.Equal(t, 2, ordered[2].ChunkID)
    assert.Equal(t, 3, ordered[3].ChunkID)

    // Reconstruct message
    var content strings.Builder
    for _, c := range ordered {
        content.WriteString(c.Chunk)
    }
    assert.Equal(t, "hello world!", content.String())
}
```

## Best Practices

1. **Set Appropriate Timeouts**: Balance between waiting for late chunks and failing fast
2. **Monitor Buffer Size**: Alert on unusually large buffers or long-lived buffers
3. **Log Out-of-Order Events**: Track frequency to identify NATS or network issues
4. **Implement Graceful Degradation**: If buffer limits hit, consider dropping oldest incomplete buffers
5. **Use Correlation IDs**: Track message flow end-to-end for debugging
6. **Test Edge Cases**: Missing chunks, duplicates, very large messages, concurrent access
