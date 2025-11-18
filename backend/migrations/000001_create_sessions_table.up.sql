-- Create sessions table
CREATE TABLE IF NOT EXISTS sessions (
    id UUID PRIMARY KEY,
    pod_id VARCHAR(255) NOT NULL,
    created_at TIMESTAMP NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMP NOT NULL DEFAULT NOW(),
    last_activity TIMESTAMP NOT NULL DEFAULT NOW(),
    status VARCHAR(50) NOT NULL DEFAULT 'active',
    metadata JSONB
);

-- Create index on pod_id for faster lookups
CREATE INDEX idx_sessions_pod_id ON sessions(pod_id);

-- Create index on status for filtering
CREATE INDEX idx_sessions_status ON sessions(status);

-- Create index on last_activity for cleanup queries
CREATE INDEX idx_sessions_last_activity ON sessions(last_activity);
