package nats

import (
	"encoding/json"
	"fmt"
	"log"
	"time"

	"github.com/nats-io/nats.go"
	"github.com/rohitkeshwani07/chat/backend/internal/models"
)

// Client wraps the NATS connection
type Client struct {
	conn   *nats.Conn
	podID  string
	logger *log.Logger
}

// ResponseHandler is a function that handles incoming response chunks
type ResponseHandler func(*models.ResponseChunk) error

// New creates a new NATS client
func New(url string, podID string, maxReconnects int, reconnectWait time.Duration, logger *log.Logger) (*Client, error) {
	opts := []nats.Option{
		nats.MaxReconnects(maxReconnects),
		nats.ReconnectWait(reconnectWait),
		nats.DisconnectErrHandler(func(nc *nats.Conn, err error) {
			if logger != nil {
				logger.Printf("NATS disconnected: %v", err)
			}
		}),
		nats.ReconnectHandler(func(nc *nats.Conn) {
			if logger != nil {
				logger.Printf("NATS reconnected to %s", nc.ConnectedUrl())
			}
		}),
	}

	nc, err := nats.Connect(url, opts...)
	if err != nil {
		return nil, fmt.Errorf("failed to connect to NATS: %w", err)
	}

	return &Client{
		conn:   nc,
		podID:  podID,
		logger: logger,
	}, nil
}

// PublishWorkflowRequest publishes a workflow execution request
func (c *Client) PublishWorkflowRequest(req *models.WorkflowRequest) error {
	subject := fmt.Sprintf("chat.workflow.execute.%s", req.SessionID)

	data, err := json.Marshal(req)
	if err != nil {
		return fmt.Errorf("failed to marshal workflow request: %w", err)
	}

	if err := c.conn.Publish(subject, data); err != nil {
		return fmt.Errorf("failed to publish workflow request: %w", err)
	}

	if c.logger != nil {
		c.logger.Printf("Published workflow request to %s (message_id=%s)", subject, req.MessageID)
	}

	return nil
}

// SubscribeToResponses subscribes to response chunks for this pod
func (c *Client) SubscribeToResponses(handler ResponseHandler) error {
	subject := fmt.Sprintf("chat.pod.%s.response", c.podID)

	_, err := c.conn.Subscribe(subject, func(msg *nats.Msg) {
		var chunk models.ResponseChunk
		if err := json.Unmarshal(msg.Data, &chunk); err != nil {
			if c.logger != nil {
				c.logger.Printf("Failed to unmarshal response chunk: %v", err)
			}
			return
		}

		if err := handler(&chunk); err != nil {
			if c.logger != nil {
				c.logger.Printf("Failed to handle response chunk: %v", err)
			}
		}
	})

	if err != nil {
		return fmt.Errorf("failed to subscribe to responses: %w", err)
	}

	if c.logger != nil {
		c.logger.Printf("Subscribed to %s", subject)
	}

	return nil
}

// SubscribeToBroadcast subscribes to broadcast messages for all sessions
func (c *Client) SubscribeToBroadcast(handler ResponseHandler) error {
	subject := "chat.session.*.broadcast"

	_, err := c.conn.Subscribe(subject, func(msg *nats.Msg) {
		var chunk models.ResponseChunk
		if err := json.Unmarshal(msg.Data, &chunk); err != nil {
			if c.logger != nil {
				c.logger.Printf("Failed to unmarshal broadcast chunk: %v", err)
			}
			return
		}

		// Handler should check if this pod has active connections for the session
		if err := handler(&chunk); err != nil {
			if c.logger != nil {
				c.logger.Printf("Failed to handle broadcast chunk: %v", err)
			}
		}
	})

	if err != nil {
		return fmt.Errorf("failed to subscribe to broadcast: %w", err)
	}

	if c.logger != nil {
		c.logger.Printf("Subscribed to %s", subject)
	}

	return nil
}

// Close closes the NATS connection
func (c *Client) Close() {
	if c.conn != nil {
		c.conn.Close()
	}
}

// IsConnected returns whether the client is currently connected
func (c *Client) IsConnected() bool {
	return c.conn != nil && c.conn.IsConnected()
}

// Drain gracefully drains the connection
func (c *Client) Drain() error {
	if c.conn != nil {
		return c.conn.Drain()
	}
	return nil
}
