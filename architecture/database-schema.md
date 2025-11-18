# Database Schema

## Overview

The database stores chat sessions, active SSE connections, conversation history, and user information. Designed for high availability with read replicas and caching.

## Schema Design

### Users Table

```sql
CREATE TABLE users (
    user_id VARCHAR(50) PRIMARY KEY,
    username VARCHAR(100) NOT NULL,
    email VARCHAR(255) UNIQUE NOT NULL,
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
    is_active BOOLEAN DEFAULT true,

    INDEX idx_email (email),
    INDEX idx_created_at (created_at)
);
```

### Chat Sessions Table

```sql
CREATE TABLE chat_sessions (
    session_id VARCHAR(50) PRIMARY KEY,
    user_id VARCHAR(50) NOT NULL,
    title VARCHAR(255),
    ai_provider VARCHAR(50), -- 'openai', 'anthropic', 'custom', etc.
    model_name VARCHAR(100),
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
    last_message_at TIMESTAMP,
    is_active BOOLEAN DEFAULT true,

    FOREIGN KEY (user_id) REFERENCES users(user_id) ON DELETE CASCADE,
    INDEX idx_user_id (user_id),
    INDEX idx_created_at (created_at),
    INDEX idx_last_message_at (last_message_at),
    INDEX idx_active (is_active, user_id)
);
```

### Messages Table

```sql
CREATE TABLE messages (
    message_id VARCHAR(50) PRIMARY KEY,
    session_id VARCHAR(50) NOT NULL,
    role ENUM('user', 'assistant', 'system') NOT NULL,
    content TEXT NOT NULL,
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    token_count INT,
    metadata JSON, -- Store additional metadata like model params, latency, etc.

    FOREIGN KEY (session_id) REFERENCES chat_sessions(session_id) ON DELETE CASCADE,
    INDEX idx_session_id (session_id),
    INDEX idx_created_at (created_at),
    INDEX idx_session_created (session_id, created_at)
);
```

### Active Connections Table (Session Registry)

```sql
CREATE TABLE active_connections (
    connection_id VARCHAR(50) PRIMARY KEY,
    session_id VARCHAR(50) NOT NULL,
    pod_id VARCHAR(50) NOT NULL,
    user_id VARCHAR(50) NOT NULL,
    connected_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    last_heartbeat TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    client_ip VARCHAR(45),
    user_agent TEXT,

    FOREIGN KEY (session_id) REFERENCES chat_sessions(session_id) ON DELETE CASCADE,
    FOREIGN KEY (user_id) REFERENCES users(user_id) ON DELETE CASCADE,
    INDEX idx_session_id (session_id),
    INDEX idx_pod_id (pod_id),
    INDEX idx_heartbeat (last_heartbeat),
    INDEX idx_session_heartbeat (session_id, last_heartbeat)
);
```

### Workflow Jobs Table (Optional - for tracking)

```sql
CREATE TABLE workflow_jobs (
    job_id VARCHAR(50) PRIMARY KEY,
    session_id VARCHAR(50) NOT NULL,
    message_id VARCHAR(50) NOT NULL,
    status ENUM('pending', 'processing', 'completed', 'failed') DEFAULT 'pending',
    ai_provider VARCHAR(50),
    model_name VARCHAR(100),
    started_at TIMESTAMP,
    completed_at TIMESTAMP,
    error_message TEXT,
    token_usage JSON, -- Store prompt_tokens, completion_tokens, total_tokens

    FOREIGN KEY (session_id) REFERENCES chat_sessions(session_id) ON DELETE CASCADE,
    FOREIGN KEY (message_id) REFERENCES messages(message_id) ON DELETE CASCADE,
    INDEX idx_session_id (session_id),
    INDEX idx_status (status),
    INDEX idx_created (started_at)
);
```

## Redis Cache Schema

For high-performance session lookups and connection tracking:

### Active Connections (Redis)

```
Key Pattern: session:connections:{session_id}
Type: SET
Value: ["pod-1:conn-abc", "pod-2:conn-xyz"]
TTL: 300 seconds (5 minutes)

Purpose: Quick lookup of which pods have active connections for a session
```

```
Key Pattern: pod:connections:{pod_id}
Type: HASH
Fields:
  - conn-abc -> session_id_1
  - conn-xyz -> session_id_2
TTL: 300 seconds (5 minutes)

Purpose: Pod-level tracking of all its active connections
```

### Heartbeat Tracking (Redis)

```
Key Pattern: heartbeat:{pod_id}:{connection_id}
Type: STRING
Value: timestamp
TTL: 30 seconds

Purpose: Track connection liveness, auto-expire stale connections
```

### Session Metadata Cache (Redis)

```
Key Pattern: session:meta:{session_id}
Type: HASH
Fields:
  - user_id
  - ai_provider
  - model_name
  - last_message_at
TTL: 3600 seconds (1 hour)

Purpose: Reduce database reads for session metadata
```

## Database Queries

### Register New SSE Connection

```sql
INSERT INTO active_connections
(connection_id, session_id, pod_id, user_id, connected_at, last_heartbeat, client_ip, user_agent)
VALUES (?, ?, ?, ?, NOW(), NOW(), ?, ?);
```

**Redis Command:**
```
SADD session:connections:{session_id} {pod_id}:{connection_id}
HSET pod:connections:{pod_id} {connection_id} {session_id}
SET heartbeat:{pod_id}:{connection_id} {timestamp} EX 30
```

### Update Heartbeat

```sql
UPDATE active_connections
SET last_heartbeat = NOW()
WHERE connection_id = ?;
```

**Redis Command:**
```
SET heartbeat:{pod_id}:{connection_id} {timestamp} EX 30
```

### Get Active Pods for Session

```sql
SELECT DISTINCT pod_id
FROM active_connections
WHERE session_id = ?
  AND last_heartbeat > NOW() - INTERVAL 30 SECOND;
```

**Redis Command (preferred):**
```
SMEMBERS session:connections:{session_id}
-- Returns: ["pod-1:conn-abc", "pod-2:conn-xyz"]
-- Parse to extract pod IDs
```

### Remove Stale Connections (Cleanup Job)

```sql
DELETE FROM active_connections
WHERE last_heartbeat < NOW() - INTERVAL 30 SECOND;
```

**Redis:**
- Handled automatically via TTL expiration

### Get User's Active Sessions

```sql
SELECT cs.session_id, cs.title, cs.last_message_at,
       COUNT(DISTINCT ac.connection_id) as active_connections
FROM chat_sessions cs
LEFT JOIN active_connections ac ON cs.session_id = ac.session_id
  AND ac.last_heartbeat > NOW() - INTERVAL 30 SECOND
WHERE cs.user_id = ?
  AND cs.is_active = true
GROUP BY cs.session_id
ORDER BY cs.last_message_at DESC;
```

### Get Session Message History

```sql
SELECT message_id, role, content, created_at, token_count
FROM messages
WHERE session_id = ?
ORDER BY created_at ASC
LIMIT 100;
```

## Indexing Strategy

### Active Connections Table
- **Primary focus**: Fast lookups by `session_id` and `pod_id`
- **Composite index**: `(session_id, last_heartbeat)` for active connection queries
- **Single index**: `last_heartbeat` for cleanup jobs

### Messages Table
- **Primary focus**: Fast retrieval of conversation history
- **Composite index**: `(session_id, created_at)` for chronological message retrieval
- **Consider**: Partitioning by date for very large datasets

### Chat Sessions Table
- **Composite index**: `(is_active, user_id)` for active session lookups per user
- **Single index**: `last_message_at` for sorting recent conversations

## Caching Strategy

### Cache Hierarchy

1. **L1 - In-Memory (Pod-level)**
   - Active SSE connection objects
   - Connection ID to session mapping
   - TTL: Connection lifetime

2. **L2 - Redis (Shared)**
   - Session to pod mappings
   - Session metadata
   - TTL: 5-60 minutes depending on data

3. **L3 - Database (Source of Truth)**
   - Persistent storage
   - Fallback when cache misses

### Cache Invalidation

- **Connection Established**: Write to Redis + Database
- **Connection Closed**: Remove from Redis + Database
- **Heartbeat**: Update Redis only (periodic sync to DB)
- **Session Updated**: Invalidate Redis cache, update DB

## Scaling Considerations

### Database
- **Read Replicas**: Route connection lookups to read replicas
- **Write Master**: All connection registrations go to master
- **Partitioning**: Consider sharding by `user_id` or `session_id`

### Redis
- **Redis Cluster**: Shard by session_id for horizontal scaling
- **Replication**: Redis replica for read-heavy operations
- **Persistence**: Enable AOF for crash recovery

### Connection Pool
- Configure appropriate connection pool sizes per pod
- Monitor connection pool exhaustion
- Implement circuit breakers for database failures

## Backup and Recovery

- **Database Backups**: Daily full backups, hourly incrementals
- **Point-in-Time Recovery**: Enable for critical data
- **Redis Snapshots**: Periodic RDB snapshots for cache warm-up
- **Message Archive**: Consider archiving old messages to cold storage
