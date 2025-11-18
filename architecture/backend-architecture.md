# Backend Architecture

## Overview

The backend is designed as a highly available, distributed system that handles chat messages and streams responses back to clients via Server-Sent Events (SSE). The architecture supports multiple backend pods behind an API gateway, with NATS-based messaging for asynchronous workflow execution and response routing.

## High-Level Architecture

```
┌─────────────┐
│   Client    │
└──────┬──────┘
       │
       │ (1) POST /chat (message)
       │ (2) GET /sse (stream)
       ▼
┌─────────────────┐
│  API Gateway    │
│ (Load Balancer) │
└────────┬────────┘
         │
         │ Round-robin / Least-connection
         ▼
    ┌────┴────┬─────────┬─────────┐
    │         │         │         │
┌───▼───┐ ┌──▼────┐ ┌──▼────┐ ┌──▼────┐
│ Pod-1 │ │ Pod-2 │ │ Pod-3 │ │ Pod-N │
└───┬───┘ └───┬───┘ └───┬───┘ └───┬───┘
    │         │         │         │
    └─────────┴─────────┴─────────┘
              │
              │ Subscribe/Publish
              ▼
        ┌───────────┐
        │   NATS    │
        │ (Message  │
        │   Broker) │
        └─────┬─────┘
              │
              ▼
    ┌──────────────────┐
    │ Workflow Service │
    │  (AI Processing) │
    └──────────────────┘
              │
              ▼
        ┌──────────┐
        │ Database │
        └──────────┘
```

## Key Components

### 1. API Gateway
- Load balances incoming requests across backend pods
- Routes chat message POST requests
- Routes SSE connection GET requests
- Provides single entry point for clients

### 2. Backend Pods
- Stateless application servers (Golang)
- Handle chat message ingestion
- Maintain active SSE connections
- Subscribe to pod-specific NATS subjects for response routing
- Route response chunks to appropriate SSE connections

### 3. NATS Message Broker
- Facilitates asynchronous communication
- Publishes workflow execution requests
- Routes AI response chunks back to appropriate pods
- Subjects:
  - `workflow.execute.{chatSessionId}` - Trigger workflow execution
  - `pod.{podId}.responses` - Pod-specific response routing
  - `session.{chatSessionId}.response` - Session broadcast (for discovery)

### 4. Workflow Service
- Processes chat messages
- Integrates with AI models
- Streams response tokens/chunks back via NATS
- Publishes to appropriate pod subjects

### 5. Session Registry (Database/Cache)
- Tracks active chat sessions
- Maps sessions to active pods with SSE connections
- Schema:
  ```
  chat_sessions:
    - session_id (PK)
    - user_id
    - created_at
    - updated_at

  active_connections:
    - session_id (FK)
    - pod_id
    - connection_id
    - connected_at
    - last_heartbeat
  ```

## Message Flow

### A. Chat Message Submission

1. Client sends POST request to `/api/chat` with message payload
   ```json
   {
     "session_id": "sess_123",
     "message": "Hello, AI!",
     "user_id": "user_456"
   }
   ```

2. API Gateway routes request to Pod-X (any available pod)

3. Pod-X validates request and publishes to NATS:
   ```
   Subject: workflow.execute.sess_123
   Payload: {
     "session_id": "sess_123",
     "message": "Hello, AI!",
     "user_id": "user_456",
     "timestamp": "2025-11-17T20:00:00Z"
   }
   ```

4. Pod-X responds to client: `202 Accepted`

5. Workflow Service picks up message from NATS and begins processing

### B. SSE Connection Establishment

1. Client opens SSE connection: `GET /api/sse?session_id=sess_123`

2. API Gateway routes to Pod-Y (could be different from Pod-X)

3. Pod-Y:
   - Establishes SSE connection
   - Generates unique `connection_id`
   - Registers in Session Registry:
     ```sql
     INSERT INTO active_connections
     (session_id, pod_id, connection_id, connected_at, last_heartbeat)
     VALUES ('sess_123', 'pod-y', 'conn_789', NOW(), NOW())
     ```
   - Ensures pod is subscribed to `pod.pod-y.responses`

4. SSE connection remains open, waiting for messages

### C. Response Streaming

1. Workflow Service generates response tokens/chunks

2. For each chunk, Workflow Service:
   - Queries Session Registry for active connections:
     ```sql
     SELECT pod_id FROM active_connections
     WHERE session_id = 'sess_123' AND
           last_heartbeat > NOW() - INTERVAL '30 seconds'
     ```
   - Publishes to each active pod's subject:
     ```
     Subject: pod.pod-y.responses
     Payload: {
       "session_id": "sess_123",
       "chunk": "Hello",
       "chunk_id": 1,
       "is_final": false
     }
     ```

3. Pod-Y receives message on `pod.pod-y.responses`:
   - **Buffers chunk in memory** (chunks stored by `chunk_id`)
   - **Checks for ordering**: If `chunk_id` is next expected, stream to client
   - **Handles out-of-order**: If gap detected, waits for missing chunks
   - Streams available consecutive chunks to SSE connection(s)
   ```
   data: {"chunk": "Hello", "chunk_id": 1, "is_final": false}
   ```

4. Process repeats for each chunk until completion

5. Final chunk sent with `is_final: true`

6. **Database Persistence** (only after final chunk):
   - Pod assembles complete message from all buffered chunks
   - Verifies all chunks received (0 to `final_chunk_id`)
   - **Single atomic write** to database with complete message
   - Clears chunk buffer from memory
   - Until final chunk, all data remains in memory only

## Chunk Buffering and Ordering

### Memory-First Strategy

**Key Principle**: Chunks are buffered in memory until the complete message is received, then persisted atomically to the database.

**Benefits**:
- Reduces database write operations (1 write per message vs N writes per chunk)
- Enables handling of out-of-order chunk delivery
- Prevents partial messages in database
- Faster streaming to clients (no database I/O per chunk)

### Out-of-Order Handling

NATS does not guarantee message ordering, especially when:
- Multiple workflow service instances publish chunks
- Network conditions vary between publishers and subscribers
- Load balancing causes routing differences

**Solution**: Each pod maintains an in-memory chunk buffer per active message:

```go
type ChunkBuffer struct {
    Chunks       map[int]*ResponseChunk  // chunk_id -> chunk
    NextToSend   int                     // next sequential chunk to stream
    IsFinal      bool                    // final chunk received?
    FinalChunkID int                     // chunk_id of final chunk
}
```

**Example Flow** (chunks arrive: 3, 1, 4, 0, 5-final, 2):
1. Chunk 3 arrives → Buffer {3}, wait (missing 0-2)
2. Chunk 1 arrives → Buffer {1,3}, wait (missing 0)
3. Chunk 4 arrives → Buffer {1,3,4}, wait (missing 0)
4. Chunk 0 arrives → Buffer {0,1,3,4}, **stream 0→1 to client** (next: 2)
5. Chunk 5 arrives (final) → Buffer {0,1,3,4,5}, wait (missing 2)
6. Chunk 2 arrives → Buffer complete, **stream 2→3→4→5 to client**
7. **Persist complete message to database**, clear buffer

### Buffer Lifecycle

1. **Creation**: When first chunk for a message arrives
2. **Accumulation**: Chunks stored by `chunk_id`, handle duplicates gracefully
3. **Streaming**: Send consecutive chunks starting from `NextToSend`
4. **Completion**: When `is_final` chunk received AND all gaps filled
5. **Persistence**: Assemble full message, write to DB atomically
6. **Cleanup**: Remove buffer from memory

### Timeout and Error Handling

- **Missing Chunk Timeout**: 30 seconds max wait for missing chunks
- **Buffer Age Limit**: Auto-cleanup buffers older than 5 minutes
- **Memory Limits**: Maximum 10,000 concurrent buffers per pod
- **Gap Detection**: Log warnings when chunks arrive out of order

**See [Chunk Buffering Documentation](./chunk-buffering.md) for complete implementation details.**

## Session Management Strategy

### Registration
- When SSE connection established, register in `active_connections` table
- Store: `session_id`, `pod_id`, `connection_id`, timestamp

### Heartbeat
- Pods send periodic heartbeat updates (every 10s) to Session Registry
- Update `last_heartbeat` timestamp
- Allows detection of stale/dead connections

### Cleanup
- Background job removes connections with stale heartbeats (>30s old)
- On graceful pod shutdown, pod removes its connections
- On SSE disconnect, immediately remove from registry

### Session Discovery
- Workflow Service queries active connections before publishing each chunk
- Caches results briefly (1-2s) to reduce database load for rapid chunks
- Falls back to broadcast subject if no active connections found

## NATS Subject Design

### Workflow Execution
- **Subject**: `workflow.execute.{sessionId}`
- **Publishers**: Backend Pods
- **Subscribers**: Workflow Service
- **Purpose**: Trigger AI processing workflow

### Pod-Specific Responses
- **Subject**: `pod.{podId}.responses`
- **Publishers**: Workflow Service
- **Subscribers**: Specific Backend Pod
- **Purpose**: Route response chunks to pods with active SSE connections

### Session Broadcast (Optional/Fallback)
- **Subject**: `session.{sessionId}.response`
- **Publishers**: Workflow Service
- **Subscribers**: All Backend Pods
- **Purpose**: Broadcast when active pods unknown (pods filter by session)

## Scaling Considerations

### Horizontal Scaling
- Backend pods are stateless and can scale independently
- Session Registry must handle high read/write throughput
- Consider Redis for Session Registry (fast, supports TTL)

### Connection Distribution
- Clients should reconnect SSE if connection drops
- Load balancer should support sticky sessions (optional)
- Multiple SSE connections per session supported (multi-device)

### NATS Performance
- Use NATS JetStream for message persistence if needed
- Monitor subject subscription counts
- Consider subject sharding for very high throughput

## Failure Scenarios

### Pod Failure
- Session Registry heartbeat mechanism detects dead connections
- Client SSE connection breaks, client should reconnect
- New connection may land on different pod
- Workflow Service queries fresh pod assignments

### NATS Failure
- Backend pods cannot publish new workflow requests
- Return 503 Service Unavailable to clients
- Implement retry logic with exponential backoff

### Database Failure
- Cannot register new SSE connections
- Cannot query active pods for response routing
- Fallback: Use broadcast subject (less efficient)
- Implement in-memory cache for recent session mappings

### Workflow Service Failure
- Messages queue in NATS (if using JetStream)
- Client SSE connections remain open, waiting
- Consider timeout and retry mechanisms

## Future Enhancements

1. **Rate Limiting**: Per-user, per-session rate limits
2. **Message Persistence**: Store full conversation history
3. **Multi-Region**: Deploy across regions for lower latency
4. **Observability**: Distributed tracing, metrics, logging
5. **Security**: Authentication, authorization, message encryption
