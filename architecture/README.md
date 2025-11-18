# Architecture Documentation

This directory contains comprehensive architecture documentation for the Universal Chat Interface system.

## Overview

The Universal Chat Interface is a highly available, distributed system designed to handle chat messages and stream AI-generated responses to clients in real-time. The architecture supports multiple backend pods behind an API gateway, with NATS-based messaging for asynchronous workflow execution and intelligent response routing.

## Key Design Principles

1. **High Availability**: Multiple backend pods ensure no single point of failure
2. **Horizontal Scalability**: Stateless pods allow easy scaling based on demand
3. **Real-time Streaming**: Server-Sent Events (SSE) for low-latency response delivery
4. **Asynchronous Processing**: NATS message broker decouples request handling from AI processing
5. **Session Awareness**: Smart routing ensures responses reach the correct client connections

## Architecture Documents

### [Backend Architecture](./backend-architecture.md)
Complete overview of the backend system architecture including:
- High-level system components and their interactions
- Message flow for chat submission and response streaming
- Session management and connection tracking
- NATS subject design and message routing
- Failure scenarios and recovery mechanisms
- Scaling considerations

**Start here** for a comprehensive understanding of how the system works end-to-end.

### [Database Schema](./database-schema.md)
Detailed database design including:
- SQL schema for users, sessions, messages, and active connections
- Redis cache schema for high-performance lookups
- Indexing strategy for optimal query performance
- Common database queries and their patterns
- Caching hierarchy (L1/L2/L3)
- Scaling and backup strategies

**Use this** when implementing data layer or optimizing database performance.

### [NATS Messaging](./nats-messaging.md)
In-depth guide to NATS messaging patterns:
- Subject naming conventions
- Message schemas for all communication types
- Workflow execution pattern (Backend → Workflow Service)
- Pod-specific response routing (Workflow Service → Backend)
- Error handling and retry logic
- Monitoring, security, and performance optimization

**Reference this** when implementing NATS publishers/subscribers or debugging message flow.

### [Chunk Buffering and Ordering](./chunk-buffering.md)
Detailed implementation guide for handling streaming response chunks:
- Memory-first buffering strategy (no DB writes until completion)
- Out-of-order chunk handling with reordering logic
- In-memory buffer data structures and algorithms
- Atomic database persistence only after final chunk
- Timeout handling, cleanup, and memory management
- Complete code examples and test cases

**Critical for** implementing response streaming and ensuring correct message delivery despite NATS ordering issues.

## System Architecture Diagram

```
┌──────────┐
│  Client  │
│ (Browser)│
└────┬─────┘
     │
     │ HTTP/HTTPS
     │ (1) POST /chat - Submit message
     │ (2) GET /sse - Open response stream
     │
     ▼
┌─────────────────┐
│  API Gateway    │
│ (Load Balancer) │
│  - Nginx/HAProxy│
└────────┬────────┘
         │
         │ Load balancing (Round-robin/Least-connection)
         │
    ┌────┴────┬─────────┬─────────┐
    │         │         │         │
┌───▼───┐ ┌──▼────┐ ┌──▼────┐ ┌──▼────┐
│ Pod-1 │ │ Pod-2 │ │ Pod-3 │ │ Pod-N │
│       │ │       │ │       │ │       │
│ - REST│ │ - REST│ │ - REST│ │ - REST│
│ - SSE │ │ - SSE │ │ - SSE │ │ - SSE │
└───┬───┘ └───┬───┘ └───┬───┘ └───┬───┘
    │         │         │         │
    └─────────┴─────────┴─────────┘
              │                   │
              │                   │
              ▼                   ▼
        ┌───────────┐      ┌─────────────┐
        │   NATS    │      │   Redis     │
        │ (Message  │      │   (Cache)   │
        │  Broker)  │      │             │
        └─────┬─────┘      └──────┬──────┘
              │                   │
              ▼                   ▼
    ┌──────────────────┐   ┌────────────┐
    │ Workflow Service │   │  Database  │
    │  (AI Processing) │   │ (Postgres) │
    │                  │   │            │
    │ - OpenAI API     │   └────────────┘
    │ - Anthropic API  │
    │ - Custom Models  │
    └──────────────────┘
```

## Component Responsibilities

### API Gateway
- Entry point for all client requests
- SSL/TLS termination
- Load balancing across backend pods
- Rate limiting (optional)
- Request routing based on path

### Backend Pods (Golang)
- Handle incoming chat messages
- Maintain active SSE connections
- Publish workflow requests to NATS
- Subscribe to pod-specific response subjects
- Route response chunks to appropriate SSE connections
- Register/deregister connections in Session Registry

### NATS Message Broker
- Central message bus for asynchronous communication
- Routes workflow execution requests to Workflow Service
- Routes AI response chunks back to specific pods
- Provides pub/sub and request/reply patterns
- JetStream for message persistence and reliability

### Workflow Service
- Subscribes to workflow execution requests
- Integrates with various AI providers (OpenAI, Anthropic, etc.)
- Streams response tokens/chunks as they're generated
- Queries Session Registry for active pod assignments
- Publishes response chunks to appropriate pod subjects

### Session Registry (Redis + Database)
- Tracks active SSE connections per session
- Maps sessions to pod IDs with active connections
- Provides fast lookups for response routing
- Handles connection heartbeats and cleanup

### Database (PostgreSQL)
- Persistent storage for users, sessions, and messages
- Conversation history
- Workflow job tracking
- Backup for Session Registry

## Message Flow Overview

### 1. Chat Message Submission
```
Client → API Gateway → Pod-X → NATS → Workflow Service
```

### 2. SSE Connection
```
Client → API Gateway → Pod-Y → Session Registry (register)
```

### 3. Response Streaming
```
Workflow Service → Query Session Registry → Publish to NATS (pod.Pod-Y.response)
                                         → Pod-Y receives → Route to SSE → Client
```

## Next Steps

1. **Implementation**: Start with backend pod structure (see `/backend`)
2. **Database Setup**: Use schemas from `database-schema.md`
3. **NATS Integration**: Follow patterns in `nats-messaging.md`
4. **Testing**: Build integration tests for message flows
5. **Frontend**: Connect React UI to backend APIs (see `/frontend`)

## Additional Resources

- [Golang Documentation](https://go.dev/doc/)
- [NATS Documentation](https://docs.nats.io/)
- [Redis Documentation](https://redis.io/docs/)
- [PostgreSQL Documentation](https://www.postgresql.org/docs/)
- [Server-Sent Events Spec](https://html.spec.whatwg.org/multipage/server-sent-events.html)
