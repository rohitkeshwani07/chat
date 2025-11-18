# Backend Service

Golang backend service for the Universal Chat Interface. Implements a highly available, distributed system for handling chat messages and streaming AI-generated responses.

## Architecture

This backend implements the architecture detailed in `/architecture/backend-architecture.md`:

- **HTTP API**: REST endpoints for chat submission + SSE for response streaming
- **NATS Messaging**: Asynchronous workflow execution and response routing
- **Redis Session Registry**: Tracks active SSE connections across pods
- **In-Memory Chunk Buffering**: Handles out-of-order chunks, atomic DB persistence
- **Horizontal Scalability**: Stateless pods behind load balancer

## Project Structure

```
backend/
├── cmd/server/          # Main application entry point
├── internal/
│   ├── config/          # Configuration management
│   ├── models/          # Data models and DTOs
│   ├── buffer/          # Chunk buffer manager (out-of-order handling)
│   ├── registry/        # Session registry (Redis)
│   ├── nats/            # NATS client and publishers/subscribers
│   ├── sse/             # SSE connection manager
│   └── handlers/        # HTTP request handlers
├── go.mod
└── README.md
```

## Key Features

### 1. Chunk Buffering & Ordering
- Buffers response chunks in memory until complete message received
- Handles out-of-order NATS message delivery
- Single atomic database write per message (not per chunk)
- See `/architecture/chunk-buffering.md`

### 2. Session Registry
- Redis-based tracking of active SSE connections
- Maps sessions to pod IDs for response routing
- Heartbeat mechanism for stale connection detection

### 3. SSE Streaming
- Maintains long-lived connections to clients
- Streams response chunks in real-time
- Supports multiple connections per session (multi-device)

### 4. NATS Integration
- Publishes workflow requests: `chat.workflow.execute.{sessionId}`
- Subscribes to pod-specific responses: `chat.pod.{podId}.response`
- Broadcast fallback: `chat.session.*.broadcast`

## Configuration

Environment variables:

```bash
# Server
SERVER_HOST=0.0.0.0
SERVER_PORT=8080
SERVER_READ_TIMEOUT=30s
SERVER_WRITE_TIMEOUT=30s
POD_ID=pod-1  # Auto-generated from hostname if not set

# Database (PostgreSQL) - Not yet implemented
DB_HOST=localhost
DB_PORT=5432
DB_USER=postgres
DB_PASSWORD=postgres
DB_NAME=chat
DB_SSLMODE=disable

# Redis
REDIS_HOST=localhost
REDIS_PORT=6379
REDIS_PASSWORD=
REDIS_DB=0

# NATS
NATS_URL=nats://localhost:4222
NATS_MAX_RECONNECTS=-1  # -1 = infinite
NATS_RECONNECT_WAIT=2s

# Buffer
BUFFER_MAX_BUFFERS=10000
BUFFER_MAX_CHUNKS=10000
BUFFER_MAX_AGE=5m
BUFFER_CLEANUP_INTERVAL=30s
BUFFER_MISSING_CHUNK_TIMEOUT=30s
```

## Getting Started

### Prerequisites

- Go 1.21+
- Redis running on localhost:6379
- NATS server running on localhost:4222

### Install Dependencies

```bash
cd backend
go mod download
```

### Run Locally

```bash
# Start server
go run cmd/server/main.go

# Or build and run
go build -o bin/server cmd/server/main.go
./bin/server
```

### Docker

Build and run with Docker:

```bash
# Build image
docker build -t chat-backend .

# Run with environment variables
docker run -p 8080:8080 \
  -e REDIS_HOST=redis \
  -e NATS_URL=nats://nats:4222 \
  -e DB_HOST=postgres \
  chat-backend
```

### Docker Compose (Recommended)

Run the entire system with all dependencies:

```bash
# From project root
docker-compose up -d

# View backend logs
docker-compose logs -f backend-1 backend-2 backend-3

# Scale backend pods
docker-compose up -d --scale backend-1=5
```

See root `README.md` for complete Docker Compose instructions.

## API Endpoints

### POST /api/chat
Submit a chat message for processing.

**Request:**
```json
{
  "session_id": "sess_123",
  "user_id": "user_456",
  "message": "Hello, AI!",
  "ai_provider": "openai",
  "model": "gpt-4"
}
```

**Response:** `202 Accepted`
```json
{
  "message_id": "msg_abc",
  "session_id": "sess_123",
  "status": "accepted",
  "timestamp": "2025-11-18T12:00:00Z",
  "correlation_id": "corr_xyz"
}
```

### GET /api/sse
Establish SSE connection to receive streaming responses.

**Query Parameters:**
- `session_id` (required): Chat session ID
- `user_id` (required): User ID

**Response:** Server-Sent Events stream
```
event: connected
data: {"connection_id":"conn_123","session_id":"sess_456"}

event: chunk
data: {"session_id":"sess_456","message_id":"msg_abc","chunk_id":0,"chunk":"Hello","is_final":false}

event: chunk
data: {"session_id":"sess_456","message_id":"msg_abc","chunk_id":1,"chunk":" world","is_final":false}

event: message_complete
data: {"message_id":"msg_abc","token_count":42}
```

### GET /health
Health check endpoint.

**Response:** `200 OK`
```json
{
  "status": "healthy",
  "pod_id": "pod-1",
  "timestamp": 1700000000,
  "active_connections": 15,
  "active_sessions": 10,
  "active_buffers": 5,
  "nats_connected": true
}
```

## Development

### Run Tests (Coming Soon)

```bash
go test ./...
```

### Linting

```bash
golangci-lint run
```

## Deployment

### Kubernetes

See `/k8s` directory for Kubernetes manifests (coming soon).

Key considerations:
- Deploy multiple pods (3+ for HA)
- Configure horizontal pod autoscaling
- Use sticky sessions on load balancer (optional)
- Ensure Redis and NATS are highly available

### Environment Variables in Production

Use secrets management for sensitive values:
- `DB_PASSWORD`
- `REDIS_PASSWORD`
- `NATS_TOKEN` (if using authentication)

## Monitoring

Key metrics to monitor:
- Active SSE connections per pod
- Active chunk buffers per pod
- NATS connection status
- Redis connection pool usage
- HTTP request latency (P50, P95, P99)
- Buffer cleanup rate

## Troubleshooting

### Chunks arriving out of order
Check NATS logs and network conditions. The buffer manager handles this automatically with a 30s timeout.

### SSE connections dropping
- Check firewall/load balancer timeout settings
- Verify heartbeat interval (default: 10s)
- Review client reconnection logic

### High memory usage
- Check `BUFFER_MAX_BUFFERS` and `BUFFER_MAX_CHUNKS` settings
- Review buffer cleanup interval
- Monitor for stuck buffers (timeout not triggering)

## Next Steps

- [ ] Implement database layer (PostgreSQL)
- [ ] Add authentication & authorization
- [ ] Implement message persistence
- [ ] Add distributed tracing (OpenTelemetry)
- [ ] Add metrics (Prometheus)
- [ ] Implement rate limiting
- [ ] Add comprehensive tests
- [ ] Create Dockerfile and K8s manifests
