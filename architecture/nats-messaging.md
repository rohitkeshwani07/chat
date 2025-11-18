# NATS Messaging Architecture

## Overview

NATS serves as the central message broker for asynchronous communication between backend pods and workflow services. This document details the messaging patterns, subject design, and message schemas.

## NATS Configuration

### JetStream
Enable JetStream for:
- Message persistence
- At-least-once delivery guarantees
- Message replay capabilities
- Consumer acknowledgments

### Connection Settings
```go
// Example configuration
nats.Connect(
    natsURL,
    nats.MaxReconnects(-1),
    nats.ReconnectWait(2*time.Second),
    nats.DisconnectErrHandler(handleDisconnect),
    nats.ReconnectHandler(handleReconnect),
)
```

## Subject Naming Convention

```
{domain}.{entity}.{action}.{identifier}
```

Examples:
- `chat.workflow.execute.sess_123`
- `chat.pod.pod-1.response`
- `chat.session.sess_123.broadcast`

## Message Flow Patterns

### 1. Workflow Execution Pattern

**Direction**: Backend Pod → Workflow Service

**Subject**: `chat.workflow.execute.{sessionId}`

**Publisher**: Backend Pods (any pod receiving chat message)

**Subscriber**: Workflow Service (queue group for load balancing)

**Message Schema**:
```json
{
  "message_id": "msg_abc123",
  "session_id": "sess_456",
  "user_id": "user_789",
  "message": "What is the weather today?",
  "context": {
    "ai_provider": "openai",
    "model": "gpt-4",
    "temperature": 0.7,
    "max_tokens": 2000
  },
  "timestamp": "2025-11-17T20:30:00Z",
  "correlation_id": "corr_xyz"
}
```

**NATS Code (Publisher)**:
```go
func PublishWorkflowRequest(nc *nats.Conn, sessionID string, req WorkflowRequest) error {
    subject := fmt.Sprintf("chat.workflow.execute.%s", sessionID)
    data, err := json.Marshal(req)
    if err != nil {
        return err
    }
    return nc.Publish(subject, data)
}
```

**NATS Code (Subscriber)**:
```go
// Queue group for load balancing across workflow service instances
nc.QueueSubscribe("chat.workflow.execute.*", "workflow-workers", func(m *nats.Msg) {
    var req WorkflowRequest
    json.Unmarshal(m.Data, &req)
    processWorkflow(req)
})
```

### 2. Pod-Specific Response Pattern

**Direction**: Workflow Service → Specific Backend Pod(s)

**Subject**: `chat.pod.{podId}.response`

**Publisher**: Workflow Service

**Subscriber**: Specific Backend Pod

**Message Schema**:
```json
{
  "session_id": "sess_456",
  "message_id": "msg_abc123",
  "chunk_id": 42,
  "chunk": " weather",
  "chunk_type": "content", // "content", "metadata", "error"
  "is_final": false,
  "metadata": {
    "model": "gpt-4",
    "tokens_used": 15
  },
  "timestamp": "2025-11-17T20:30:05.123Z",
  "correlation_id": "corr_xyz"
}
```

**NATS Code (Publisher - Workflow Service)**:
```go
func PublishResponseChunk(nc *nats.Conn, podIDs []string, chunk ResponseChunk) error {
    data, err := json.Marshal(chunk)
    if err != nil {
        return err
    }

    // Publish to each active pod
    for _, podID := range podIDs {
        subject := fmt.Sprintf("chat.pod.%s.response", podID)
        if err := nc.Publish(subject, data); err != nil {
            log.Printf("Failed to publish to pod %s: %v", podID, err)
            // Continue publishing to other pods
        }
    }
    return nil
}
```

**NATS Code (Subscriber - Backend Pod)**:
```go
func (p *Pod) SubscribeToResponses(nc *nats.Conn) error {
    subject := fmt.Sprintf("chat.pod.%s.response", p.ID)
    _, err := nc.Subscribe(subject, func(m *nats.Msg) {
        var chunk ResponseChunk
        json.Unmarshal(m.Data, &chunk)

        // Route to appropriate SSE connection
        p.routeToSSE(chunk)
    })
    return err
}
```

### 3. Session Broadcast Pattern (Fallback)

**Direction**: Workflow Service → All Backend Pods

**Subject**: `chat.session.{sessionId}.broadcast`

**Publisher**: Workflow Service (when active pods unknown)

**Subscriber**: All Backend Pods (filter internally)

**Message Schema**: Same as Pod-Specific Response

**Use Case**:
- Database/Redis unavailable
- No active connections found in registry
- Emergency fallback mechanism

**NATS Code (Subscriber - All Pods)**:
```go
nc.Subscribe("chat.session.*.broadcast", func(m *nats.Msg) {
    var chunk ResponseChunk
    json.Unmarshal(m.Data, &chunk)

    // Check if this pod has active connection for this session
    if p.hasActiveSession(chunk.SessionID) {
        p.routeToSSE(chunk)
    }
    // Otherwise ignore
})
```

## Advanced Patterns

### 4. Request-Reply Pattern (Health Check)

**Subject**: `chat.pod.{podId}.health`

**Pattern**: Synchronous request-reply

```go
// Backend Pod - Responder
nc.Subscribe(fmt.Sprintf("chat.pod.%s.health", podID), func(m *nats.Msg) {
    response := HealthResponse{
        PodID: podID,
        Status: "healthy",
        ActiveConnections: p.connectionCount(),
        Timestamp: time.Now(),
    }
    data, _ := json.Marshal(response)
    m.Respond(data)
})

// Monitoring Service - Requester
msg, err := nc.Request("chat.pod.pod-1.health", nil, 2*time.Second)
if err != nil {
    log.Printf("Pod unhealthy: %v", err)
}
```

### 5. Stream Processing with JetStream

**Use Case**: Reliable message processing with acknowledgments

```go
// Create stream for workflow requests
js, _ := nc.JetStream()

// Add stream
js.AddStream(&nats.StreamConfig{
    Name:     "WORKFLOW_REQUESTS",
    Subjects: []string{"chat.workflow.execute.*"},
    Retention: nats.WorkQueuePolicy,
    MaxAge: 24 * time.Hour,
})

// Consumer for durable processing
js.Subscribe("chat.workflow.execute.*", func(m *nats.Msg) {
    // Process message
    processWorkflow(m.Data)

    // Acknowledge
    m.Ack()
}, nats.Durable("workflow-processor"))
```

## Message Schemas

### WorkflowRequest
```json
{
  "message_id": "string (unique)",
  "session_id": "string",
  "user_id": "string",
  "message": "string (user message)",
  "context": {
    "ai_provider": "string (openai|anthropic|custom)",
    "model": "string",
    "temperature": "float",
    "max_tokens": "int",
    "system_prompt": "string (optional)",
    "conversation_history": ["array of previous messages"]
  },
  "timestamp": "ISO 8601 string",
  "correlation_id": "string (for tracing)"
}
```

### ResponseChunk
```json
{
  "session_id": "string",
  "message_id": "string",
  "chunk_id": "int (sequential)",
  "chunk": "string (text content or JSON)",
  "chunk_type": "string (content|metadata|error|system)",
  "is_final": "boolean",
  "metadata": {
    "model": "string",
    "tokens_used": "int",
    "finish_reason": "string (stop|length|error)"
  },
  "error": {
    "code": "string",
    "message": "string"
  },
  "timestamp": "ISO 8601 string",
  "correlation_id": "string"
}
```

### HeartbeatMessage
```json
{
  "pod_id": "string",
  "timestamp": "ISO 8601 string",
  "active_connections": "int",
  "memory_usage_mb": "float",
  "cpu_usage_percent": "float"
}
```

## Error Handling

### Publish Failures

```go
func PublishWithRetry(nc *nats.Conn, subject string, data []byte, maxRetries int) error {
    var err error
    for i := 0; i < maxRetries; i++ {
        err = nc.Publish(subject, data)
        if err == nil {
            return nil
        }

        log.Printf("Publish failed (attempt %d/%d): %v", i+1, maxRetries, err)
        time.Sleep(time.Duration(i+1) * 100 * time.Millisecond)
    }
    return fmt.Errorf("publish failed after %d retries: %w", maxRetries, err)
}
```

### Subscription Failures

```go
nc.SetDisconnectErrHandler(func(nc *nats.Conn, err error) {
    log.Printf("NATS disconnected: %v", err)
    // Trigger pod health check failure
    // Load balancer will stop routing to this pod
})

nc.SetReconnectHandler(func(nc *nats.Conn) {
    log.Printf("NATS reconnected")
    // Resubscribe to subjects if needed
    resubscribe()
})
```

## Monitoring and Observability

### Key Metrics

1. **Publish Rate**: Messages/second per subject
2. **Subscription Lag**: Time between publish and delivery
3. **Message Size**: Average and P99 message sizes
4. **Error Rate**: Failed publishes/subscribes per minute
5. **Active Subscriptions**: Number per pod

### Logging

```go
// Structured logging for NATS operations
log.WithFields(log.Fields{
    "subject": subject,
    "message_id": msgID,
    "session_id": sessionID,
    "pod_id": podID,
    "latency_ms": latency,
}).Info("Message published")
```

### Distributed Tracing

```go
// Inject trace context in messages
type TracedMessage struct {
    TraceID string `json:"trace_id"`
    SpanID  string `json:"span_id"`
    Data    interface{} `json:"data"`
}

// Extract and propagate in subscribers
func handleMessage(m *nats.Msg) {
    var msg TracedMessage
    json.Unmarshal(m.Data, &msg)

    ctx := tracing.ExtractContext(msg.TraceID, msg.SpanID)
    span := tracing.StartSpan(ctx, "process_message")
    defer span.Finish()

    // Process with tracing context
}
```

## Security

### Authentication
```go
nc, err := nats.Connect(
    natsURL,
    nats.UserInfo("username", "password"),
    // Or use token
    nats.Token("secure-token"),
    // Or use TLS certificates
    nats.ClientCert("cert.pem", "key.pem"),
    nats.RootCAs("ca.pem"),
)
```

### Authorization
- Use NATS authorization to restrict publish/subscribe permissions per service
- Backend pods: Can publish to `chat.workflow.*`, subscribe to `chat.pod.{their-id}.*`
- Workflow service: Can subscribe to `chat.workflow.*`, publish to `chat.pod.*`

### Encryption
- Enable TLS for all NATS connections
- Encrypt sensitive message payloads before publishing

## Performance Optimization

### Batching (for high-throughput scenarios)

```go
// Instead of publishing each chunk individually
// Batch multiple chunks
type ChunkBatch struct {
    SessionID string
    Chunks    []ResponseChunk
}

// Publish batches every 50ms or 100 chunks, whichever comes first
```

### Connection Pooling

```go
// Reuse NATS connections across goroutines
var globalNC *nats.Conn

func init() {
    var err error
    globalNC, err = nats.Connect(natsURL)
    if err != nil {
        log.Fatal(err)
    }
}
```

### Subject Wildcards (use sparingly)

```go
// Subscribe to all workflow requests
nc.Subscribe("chat.workflow.execute.*", handler)

// Subscribe to all pod responses
nc.Subscribe("chat.pod.*.response", handler)
```

## Testing

### Mock NATS Server

```go
import "github.com/nats-io/nats-server/v2/server"

func startTestNATS() *server.Server {
    opts := &server.Options{
        Host: "127.0.0.1",
        Port: -1, // Random port
    }
    return server.New(opts)
}
```

### Integration Tests

```go
func TestWorkflowExecution(t *testing.T) {
    // Start test NATS
    s := startTestNATS()
    defer s.Shutdown()

    nc, _ := nats.Connect(s.ClientURL())
    defer nc.Close()

    // Test publish and subscribe
    done := make(chan bool)
    nc.Subscribe("chat.workflow.execute.test", func(m *nats.Msg) {
        done <- true
    })

    nc.Publish("chat.workflow.execute.test", []byte("test"))

    select {
    case <-done:
        // Success
    case <-time.After(1 * time.Second):
        t.Fatal("Message not received")
    }
}
```
