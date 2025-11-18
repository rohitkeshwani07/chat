package handlers

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"time"

	"github.com/google/uuid"
	"github.com/rohitkeshwani07/chat/backend/internal/buffer"
	"github.com/rohitkeshwani07/chat/backend/internal/models"
	natsClient "github.com/rohitkeshwani07/chat/backend/internal/nats"
	"github.com/rohitkeshwani07/chat/backend/internal/registry"
	"github.com/rohitkeshwani07/chat/backend/internal/sse"
)

// Handler holds dependencies for HTTP handlers
type Handler struct {
	podID          string
	nats           *natsClient.Client
	registry       *registry.SessionRegistry
	sseManager     *sse.Manager
	bufferManager  *buffer.Manager
	logger         *log.Logger
}

// New creates a new handler
func New(
	podID string,
	natsClient *natsClient.Client,
	registry *registry.SessionRegistry,
	sseManager *sse.Manager,
	bufferManager *buffer.Manager,
	logger *log.Logger,
) *Handler {
	return &Handler{
		podID:         podID,
		nats:          natsClient,
		registry:      registry,
		sseManager:    sseManager,
		bufferManager: bufferManager,
		logger:        logger,
	}
}

// HandleChat handles POST /api/chat requests
func (h *Handler) HandleChat(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var req models.ChatRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid request body", http.StatusBadRequest)
		return
	}

	// Validate request
	if req.SessionID == "" || req.Message == "" || req.UserID == "" {
		http.Error(w, "Missing required fields", http.StatusBadRequest)
		return
	}

	// Generate message ID and correlation ID
	messageID := uuid.New().String()
	correlationID := uuid.New().String()

	// Create workflow request
	workflowReq := &models.WorkflowRequest{
		MessageID:     messageID,
		SessionID:     req.SessionID,
		UserID:        req.UserID,
		Message:       req.Message,
		Context:       req.Context,
		Timestamp:     time.Now(),
		CorrelationID: correlationID,
	}

	// Set AI provider and model if specified
	if req.AIProvider != "" {
		if workflowReq.Context == nil {
			workflowReq.Context = make(map[string]interface{})
		}
		workflowReq.Context["ai_provider"] = req.AIProvider
		workflowReq.Context["model"] = req.Model
	}

	// Publish to NATS
	if err := h.nats.PublishWorkflowRequest(workflowReq); err != nil {
		h.logger.Printf("Failed to publish workflow request: %v", err)
		http.Error(w, "Failed to process request", http.StatusInternalServerError)
		return
	}

	// Return response
	response := &models.ChatResponse{
		MessageID:     messageID,
		SessionID:     req.SessionID,
		Status:        "accepted",
		Timestamp:     time.Now(),
		CorrelationID: correlationID,
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusAccepted)
	json.NewEncoder(w).Encode(response)

	h.logger.Printf("Chat message accepted: session=%s, message=%s", req.SessionID, messageID)
}

// HandleSSE handles GET /api/sse requests
func (h *Handler) HandleSSE(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// Get session ID from query parameters
	sessionID := r.URL.Query().Get("session_id")
	if sessionID == "" {
		http.Error(w, "Missing session_id parameter", http.StatusBadRequest)
		return
	}

	// Get user ID from query parameters or auth header
	userID := r.URL.Query().Get("user_id")
	if userID == "" {
		http.Error(w, "Missing user_id parameter", http.StatusBadRequest)
		return
	}

	// Create SSE connection
	conn, err := h.sseManager.AddConnection(sessionID, userID, w, r)
	if err != nil {
		h.logger.Printf("Failed to create SSE connection: %v", err)
		http.Error(w, "Failed to establish SSE connection", http.StatusInternalServerError)
		return
	}

	// Register connection in session registry
	activeConn := &models.ActiveConnection{
		ConnectionID:  conn.ID,
		SessionID:     sessionID,
		PodID:         h.podID,
		UserID:        userID,
		ConnectedAt:   conn.CreatedAt,
		LastHeartbeat: time.Now(),
		ClientIP:      conn.ClientIP,
		UserAgent:     conn.UserAgent,
	}

	if err := h.registry.RegisterConnection(activeConn); err != nil {
		h.logger.Printf("Failed to register connection in registry: %v", err)
		// Continue anyway - connection is still usable
	}

	h.logger.Printf("SSE connection established: %s for session %s", conn.ID, sessionID)

	// Start heartbeat for this connection
	heartbeatTicker := time.NewTicker(10 * time.Second)
	defer heartbeatTicker.Stop()

	// Keep connection alive
	for {
		select {
		case <-conn.Done:
			// Connection closed by client
			h.cleanup(conn)
			return

		case <-r.Context().Done():
			// Request context cancelled
			h.cleanup(conn)
			return

		case <-heartbeatTicker.C:
			// Send heartbeat and update registry
			if err := h.registry.UpdateHeartbeat(h.podID, conn.ID); err != nil {
				h.logger.Printf("Failed to update heartbeat: %v", err)
			}
		}
	}
}

// cleanup handles connection cleanup
func (h *Handler) cleanup(conn *sse.Connection) {
	h.logger.Printf("Cleaning up SSE connection: %s", conn.ID)

	// Remove from SSE manager
	h.sseManager.RemoveConnection(conn.ID)

	// Deregister from session registry
	if err := h.registry.DeregisterConnection(conn.SessionID, h.podID, conn.ID); err != nil {
		h.logger.Printf("Failed to deregister connection: %v", err)
	}
}

// HandleResponseChunk handles incoming response chunks from NATS
func (h *Handler) HandleResponseChunk(chunk *models.ResponseChunk) error {
	// Add chunk to buffer
	if err := h.bufferManager.AddChunk(chunk); err != nil {
		return fmt.Errorf("failed to buffer chunk: %w", err)
	}

	// Get next available chunks to send
	chunksToSend, isComplete, err := h.bufferManager.GetNextChunks(chunk.MessageID)
	if err != nil {
		return fmt.Errorf("failed to get next chunks: %w", err)
	}

	// Send available chunks to client via SSE
	for _, c := range chunksToSend {
		if err := h.sseManager.SendChunk(chunk.SessionID, c); err != nil {
			h.logger.Printf("Failed to send chunk to SSE: %v", err)
			// Continue sending other chunks
		}
	}

	// If message is complete, finalize and persist
	if isComplete {
		message, err := h.bufferManager.FinalizeMessage(chunk.MessageID)
		if err != nil {
			return fmt.Errorf("failed to finalize message: %w", err)
		}

		// TODO: Persist message to database
		h.logger.Printf("Message complete: %s (%d bytes, %d tokens)",
			message.MessageID, len(message.Content), message.TokenCount)

		// Send completion event
		h.sseManager.SendToSession(chunk.SessionID, &models.SSEEvent{
			Event: "message_complete",
			Data: map[string]interface{}{
				"message_id":  message.MessageID,
				"token_count": message.TokenCount,
			},
		})
	}

	return nil
}

// HandleHealth handles GET /health requests
func (h *Handler) HandleHealth(w http.ResponseWriter, r *http.Request) {
	health := map[string]interface{}{
		"status":              "healthy",
		"pod_id":              h.podID,
		"timestamp":           time.Now().Unix(),
		"active_connections":  h.sseManager.GetConnectionCount(),
		"active_sessions":     h.sseManager.GetSessionCount(),
		"active_buffers":      h.bufferManager.GetBufferCount(),
		"nats_connected":      h.nats.IsConnected(),
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(health)
}
