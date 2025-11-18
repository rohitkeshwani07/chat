package registry

import (
	"context"
	"fmt"
	"time"

	"github.com/redis/go-redis/v9"
	"github.com/rohitkeshwani07/chat/backend/internal/models"
)

// SessionRegistry manages active SSE connections using Redis
type SessionRegistry struct {
	client *redis.Client
	ctx    context.Context
}

// New creates a new session registry
func New(addr, password string, db int) (*SessionRegistry, error) {
	client := redis.NewClient(&redis.Options{
		Addr:     addr,
		Password: password,
		DB:       db,
	})

	ctx := context.Background()

	// Test connection
	if err := client.Ping(ctx).Err(); err != nil {
		return nil, fmt.Errorf("failed to connect to Redis: %w", err)
	}

	return &SessionRegistry{
		client: client,
		ctx:    ctx,
	}, nil
}

// RegisterConnection registers a new SSE connection
func (r *SessionRegistry) RegisterConnection(conn *models.ActiveConnection) error {
	// Key pattern: session:connections:{session_id}
	sessionKey := fmt.Sprintf("session:connections:%s", conn.SessionID)

	// Value: pod_id:connection_id
	value := fmt.Sprintf("%s:%s", conn.PodID, conn.ConnectionID)

	// Add to set with TTL of 5 minutes
	if err := r.client.SAdd(r.ctx, sessionKey, value).Err(); err != nil {
		return fmt.Errorf("failed to register connection: %w", err)
	}

	// Set expiration on the key (renewed on heartbeat)
	if err := r.client.Expire(r.ctx, sessionKey, 5*time.Minute).Err(); err != nil {
		return fmt.Errorf("failed to set expiration: %w", err)
	}

	// Also store in pod:connections:{pod_id} hash
	podKey := fmt.Sprintf("pod:connections:%s", conn.PodID)
	if err := r.client.HSet(r.ctx, podKey, conn.ConnectionID, conn.SessionID).Err(); err != nil {
		return fmt.Errorf("failed to register in pod hash: %w", err)
	}
	if err := r.client.Expire(r.ctx, podKey, 5*time.Minute).Err(); err != nil {
		return fmt.Errorf("failed to set pod hash expiration: %w", err)
	}

	// Store heartbeat
	heartbeatKey := fmt.Sprintf("heartbeat:%s:%s", conn.PodID, conn.ConnectionID)
	if err := r.client.Set(r.ctx, heartbeatKey, time.Now().Unix(), 30*time.Second).Err(); err != nil {
		return fmt.Errorf("failed to set heartbeat: %w", err)
	}

	return nil
}

// DeregisterConnection removes a connection from the registry
func (r *SessionRegistry) DeregisterConnection(sessionID, podID, connectionID string) error {
	// Remove from session:connections:{session_id}
	sessionKey := fmt.Sprintf("session:connections:%s", sessionID)
	value := fmt.Sprintf("%s:%s", podID, connectionID)
	if err := r.client.SRem(r.ctx, sessionKey, value).Err(); err != nil {
		return fmt.Errorf("failed to deregister from session: %w", err)
	}

	// Remove from pod:connections:{pod_id}
	podKey := fmt.Sprintf("pod:connections:%s", podID)
	if err := r.client.HDel(r.ctx, podKey, connectionID).Err(); err != nil {
		return fmt.Errorf("failed to deregister from pod hash: %w", err)
	}

	// Remove heartbeat
	heartbeatKey := fmt.Sprintf("heartbeat:%s:%s", podID, connectionID)
	if err := r.client.Del(r.ctx, heartbeatKey).Err(); err != nil {
		return fmt.Errorf("failed to delete heartbeat: %w", err)
	}

	return nil
}

// UpdateHeartbeat updates the heartbeat timestamp for a connection
func (r *SessionRegistry) UpdateHeartbeat(podID, connectionID string) error {
	heartbeatKey := fmt.Sprintf("heartbeat:%s:%s", podID, connectionID)
	if err := r.client.Set(r.ctx, heartbeatKey, time.Now().Unix(), 30*time.Second).Err(); err != nil {
		return fmt.Errorf("failed to update heartbeat: %w", err)
	}
	return nil
}

// GetActivePods returns a list of pod IDs that have active connections for a session
func (r *SessionRegistry) GetActivePods(sessionID string) ([]string, error) {
	sessionKey := fmt.Sprintf("session:connections:%s", sessionID)

	// Get all members of the set
	members, err := r.client.SMembers(r.ctx, sessionKey).Result()
	if err != nil {
		return nil, fmt.Errorf("failed to get active pods: %w", err)
	}

	// Extract unique pod IDs
	podMap := make(map[string]bool)
	for _, member := range members {
		// member format: pod_id:connection_id
		// Extract pod_id (everything before last colon)
		podID := member[:len(member)-len(member[len(member)-36:])-1] // Assuming UUID connection ID
		if len(podID) > 0 {
			podMap[podID] = true
		}
	}

	pods := make([]string, 0, len(podMap))
	for podID := range podMap {
		pods = append(pods, podID)
	}

	return pods, nil
}

// GetPodConnections returns all connection IDs for a specific pod
func (r *SessionRegistry) GetPodConnections(podID string) (map[string]string, error) {
	podKey := fmt.Sprintf("pod:connections:%s", podID)

	// Get all connections for this pod
	connections, err := r.client.HGetAll(r.ctx, podKey).Result()
	if err != nil {
		return nil, fmt.Errorf("failed to get pod connections: %w", err)
	}

	return connections, nil
}

// CacheSessionMetadata stores session metadata in Redis
func (r *SessionRegistry) CacheSessionMetadata(session *models.ChatSession) error {
	key := fmt.Sprintf("session:meta:%s", session.SessionID)

	data := map[string]interface{}{
		"user_id":     session.UserID,
		"ai_provider": session.AIProvider,
		"model_name":  session.ModelName,
	}

	if err := r.client.HSet(r.ctx, key, data).Err(); err != nil {
		return fmt.Errorf("failed to cache session metadata: %w", err)
	}

	// Set TTL of 1 hour
	if err := r.client.Expire(r.ctx, key, 1*time.Hour).Err(); err != nil {
		return fmt.Errorf("failed to set metadata expiration: %w", err)
	}

	return nil
}

// GetSessionMetadata retrieves cached session metadata
func (r *SessionRegistry) GetSessionMetadata(sessionID string) (map[string]string, error) {
	key := fmt.Sprintf("session:meta:%s", sessionID)

	data, err := r.client.HGetAll(r.ctx, key).Result()
	if err != nil {
		return nil, fmt.Errorf("failed to get session metadata: %w", err)
	}

	return data, nil
}

// Close closes the Redis connection
func (r *SessionRegistry) Close() error {
	return r.client.Close()
}
