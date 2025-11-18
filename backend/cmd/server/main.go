package main

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/rohitkeshwani07/chat/backend/internal/buffer"
	"github.com/rohitkeshwani07/chat/backend/internal/config"
	"github.com/rohitkeshwani07/chat/backend/internal/handlers"
	"github.com/rohitkeshwani07/chat/backend/internal/models"
	natsClient "github.com/rohitkeshwani07/chat/backend/internal/nats"
	"github.com/rohitkeshwani07/chat/backend/internal/registry"
	"github.com/rohitkeshwani07/chat/backend/internal/sse"
)

func main() {
	logger := log.New(os.Stdout, "[CHAT-SERVER] ", log.LstdFlags|log.Lshortfile)
	logger.Println("Starting Chat Server...")

	// Load configuration
	cfg, err := config.Load()
	if err != nil {
		logger.Fatalf("Failed to load configuration: %v", err)
	}

	logger.Printf("Configuration loaded. Pod ID: %s", cfg.Server.PodID)

	// Initialize session registry (Redis)
	logger.Println("Connecting to Redis...")
	sessionRegistry, err := registry.New(
		cfg.Redis.GetRedisAddr(),
		cfg.Redis.Password,
		cfg.Redis.DB,
	)
	if err != nil {
		logger.Fatalf("Failed to connect to Redis: %v", err)
	}
	defer sessionRegistry.Close()
	logger.Println("Connected to Redis")

	// Initialize NATS client
	logger.Println("Connecting to NATS...")
	nats, err := natsClient.New(
		cfg.NATS.URL,
		cfg.Server.PodID,
		cfg.NATS.MaxReconnects,
		cfg.NATS.ReconnectWait,
		logger,
	)
	if err != nil {
		logger.Fatalf("Failed to connect to NATS: %v", err)
	}
	defer nats.Close()
	logger.Println("Connected to NATS")

	// Initialize SSE manager
	sseManager := sse.NewManager(logger)
	logger.Println("SSE Manager initialized")

	// Start SSE heartbeat (every 30 seconds)
	sseManager.StartHeartbeat(30 * time.Second)

	// Initialize buffer manager
	bufferManager := buffer.NewManager(
		cfg.Buffer.MaxBuffersPerPod,
		cfg.Buffer.MaxChunksPerBuffer,
		cfg.Buffer.MaxBufferAge,
		cfg.Buffer.CleanupInterval,
		cfg.Buffer.MissingChunkTimeout,
	)
	logger.Println("Buffer Manager initialized")

	// Start buffer cleanup
	bufferManager.StartCleanup()
	defer bufferManager.StopCleanup()

	// Initialize HTTP handlers
	handler := handlers.New(
		cfg.Server.PodID,
		nats,
		sessionRegistry,
		sseManager,
		bufferManager,
		logger,
	)

	// Subscribe to NATS response chunks
	logger.Println("Subscribing to NATS response subjects...")
	if err := nats.SubscribeToResponses(handler.HandleResponseChunk); err != nil {
		logger.Fatalf("Failed to subscribe to responses: %v", err)
	}

	// Also subscribe to broadcast (fallback)
	if err := nats.SubscribeToBroadcast(func(chunk *models.ResponseChunk) error {
		// Only handle if this pod has active connections for the session
		if sseManager.HasActiveConnections(chunk.SessionID) {
			return handler.HandleResponseChunk(chunk)
		}
		return nil
	}); err != nil {
		logger.Fatalf("Failed to subscribe to broadcast: %v", err)
	}
	logger.Println("Subscribed to NATS subjects")

	// Setup HTTP routes
	mux := http.NewServeMux()
	mux.HandleFunc("/api/chat", handler.HandleChat)
	mux.HandleFunc("/api/sse", handler.HandleSSE)
	mux.HandleFunc("/health", handler.HandleHealth)
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/" {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprintf(w, `{"service":"chat-backend","version":"1.0.0","pod_id":"%s"}`, cfg.Server.PodID)
	})

	// Add CORS middleware
	corsHandler := enableCORS(mux)

	// Create HTTP server
	addr := fmt.Sprintf("%s:%d", cfg.Server.Host, cfg.Server.Port)
	server := &http.Server{
		Addr:         addr,
		Handler:      corsHandler,
		ReadTimeout:  cfg.Server.ReadTimeout,
		WriteTimeout: cfg.Server.WriteTimeout,
	}

	// Start server in goroutine
	go func() {
		logger.Printf("Server listening on %s", addr)
		if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			logger.Fatalf("Server failed: %v", err)
		}
	}()

	// Wait for interrupt signal
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit

	logger.Println("Shutting down server...")

	// Graceful shutdown
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	// Drain NATS connections
	if err := nats.Drain(); err != nil {
		logger.Printf("Failed to drain NATS: %v", err)
	}

	// Shutdown HTTP server
	if err := server.Shutdown(ctx); err != nil {
		logger.Printf("Server forced to shutdown: %v", err)
	}

	logger.Println("Server stopped")
}

// enableCORS adds CORS headers to all responses
func enableCORS(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, DELETE, OPTIONS")
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization")

		if r.Method == "OPTIONS" {
			w.WriteHeader(http.StatusOK)
			return
		}

		next.ServeHTTP(w, r)
	})
}
