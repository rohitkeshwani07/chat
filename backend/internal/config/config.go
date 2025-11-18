package config

import (
	"fmt"
	"os"
	"strconv"
	"time"
)

// Config holds all configuration for the application
type Config struct {
	Server   ServerConfig
	Database DatabaseConfig
	Redis    RedisConfig
	NATS     NATSConfig
	Buffer   BufferConfig
}

// ServerConfig holds HTTP server configuration
type ServerConfig struct {
	Host         string
	Port         int
	ReadTimeout  time.Duration
	WriteTimeout time.Duration
	PodID        string // Unique identifier for this pod instance
}

// DatabaseConfig holds database connection settings
type DatabaseConfig struct {
	Host     string
	Port     int
	User     string
	Password string
	Database string
	SSLMode  string
}

// RedisConfig holds Redis connection settings
type RedisConfig struct {
	Host     string
	Port     int
	Password string
	DB       int
}

// NATSConfig holds NATS connection settings
type NATSConfig struct {
	URL              string
	MaxReconnects    int
	ReconnectWait    time.Duration
	WorkflowSubject  string // Pattern for workflow execution subjects
	ResponseSubject  string // Pattern for pod response subjects
}

// BufferConfig holds chunk buffer configuration
type BufferConfig struct {
	MaxBuffersPerPod   int
	MaxChunksPerBuffer int
	MaxBufferAge       time.Duration
	CleanupInterval    time.Duration
	MissingChunkTimeout time.Duration
}

// Load reads configuration from environment variables
func Load() (*Config, error) {
	config := &Config{
		Server: ServerConfig{
			Host:         getEnv("SERVER_HOST", "0.0.0.0"),
			Port:         getEnvAsInt("SERVER_PORT", 8080),
			ReadTimeout:  getEnvAsDuration("SERVER_READ_TIMEOUT", 30*time.Second),
			WriteTimeout: getEnvAsDuration("SERVER_WRITE_TIMEOUT", 30*time.Second),
			PodID:        getEnv("POD_ID", generatePodID()),
		},
		Database: DatabaseConfig{
			Host:     getEnv("DB_HOST", "localhost"),
			Port:     getEnvAsInt("DB_PORT", 5432),
			User:     getEnv("DB_USER", "postgres"),
			Password: getEnv("DB_PASSWORD", "postgres"),
			Database: getEnv("DB_NAME", "chat"),
			SSLMode:  getEnv("DB_SSLMODE", "disable"),
		},
		Redis: RedisConfig{
			Host:     getEnv("REDIS_HOST", "localhost"),
			Port:     getEnvAsInt("REDIS_PORT", 6379),
			Password: getEnv("REDIS_PASSWORD", ""),
			DB:       getEnvAsInt("REDIS_DB", 0),
		},
		NATS: NATSConfig{
			URL:              getEnv("NATS_URL", "nats://localhost:4222"),
			MaxReconnects:    getEnvAsInt("NATS_MAX_RECONNECTS", -1), // -1 = infinite
			ReconnectWait:    getEnvAsDuration("NATS_RECONNECT_WAIT", 2*time.Second),
			WorkflowSubject:  "chat.workflow.execute.*",
			ResponseSubject:  "chat.pod.%s.response", // %s will be replaced with pod ID
		},
		Buffer: BufferConfig{
			MaxBuffersPerPod:    getEnvAsInt("BUFFER_MAX_BUFFERS", 10000),
			MaxChunksPerBuffer:  getEnvAsInt("BUFFER_MAX_CHUNKS", 10000),
			MaxBufferAge:        getEnvAsDuration("BUFFER_MAX_AGE", 5*time.Minute),
			CleanupInterval:     getEnvAsDuration("BUFFER_CLEANUP_INTERVAL", 30*time.Second),
			MissingChunkTimeout: getEnvAsDuration("BUFFER_MISSING_CHUNK_TIMEOUT", 30*time.Second),
		},
	}

	return config, nil
}

// Helper functions to read environment variables

func getEnv(key, defaultValue string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return defaultValue
}

func getEnvAsInt(key string, defaultValue int) int {
	valueStr := os.Getenv(key)
	if valueStr == "" {
		return defaultValue
	}
	value, err := strconv.Atoi(valueStr)
	if err != nil {
		return defaultValue
	}
	return value
}

func getEnvAsDuration(key string, defaultValue time.Duration) time.Duration {
	valueStr := os.Getenv(key)
	if valueStr == "" {
		return defaultValue
	}
	value, err := time.ParseDuration(valueStr)
	if err != nil {
		return defaultValue
	}
	return value
}

func generatePodID() string {
	hostname, err := os.Hostname()
	if err != nil {
		hostname = "unknown"
	}
	return fmt.Sprintf("pod-%s", hostname)
}

// GetDSN returns the PostgreSQL connection string
func (c *DatabaseConfig) GetDSN() string {
	return fmt.Sprintf(
		"host=%s port=%d user=%s password=%s dbname=%s sslmode=%s",
		c.Host, c.Port, c.User, c.Password, c.Database, c.SSLMode,
	)
}

// GetRedisAddr returns the Redis address
func (c *RedisConfig) GetRedisAddr() string {
	return fmt.Sprintf("%s:%d", c.Host, c.Port)
}

// GetPodResponseSubject returns the NATS subject for this pod's responses
func (c *NATSConfig) GetPodResponseSubject(podID string) string {
	return fmt.Sprintf(c.ResponseSubject, podID)
}
