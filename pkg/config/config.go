package config

import (
	"os"
	"strconv"
	"time"
)

// Config holds application configuration.
type Config struct {
	// NATS configuration
	NATSUrl string

	// Server configuration
	MetricsPort     int
	DashboardPort   int
	ShutdownTimeout time.Duration

	// Workflow configuration
	StoreBucket            string
	WorkflowStream         string
	WorkflowSubjectPrefix  string
	ProcessorStream        string
	ProcessorSubjectPrefix string
	MaxRetries             int
	AckTimeout             time.Duration
	StreamMaxBytes         int64
}

// Load loads configuration from environment variables with defaults.
func Load() *Config {
	return &Config{
		NATSUrl:                getEnv("NATS_URL", "nats://localhost:4222"),
		MetricsPort:            getEnvInt("METRICS_PORT", 2112),
		DashboardPort:          getEnvInt("DASHBOARD_PORT", 8085),
		ShutdownTimeout:        getEnvDuration("SHUTDOWN_TIMEOUT", 30*time.Second),
		StoreBucket:            getEnv("STORE_BUCKET", "workflow-state"),
		WorkflowStream:         getEnv("WORKFLOW_STREAM", "WORKFLOW"),
		WorkflowSubjectPrefix:  getEnv("WORKFLOW_SUBJECT_PREFIX", "workflow"),
		ProcessorStream:        getEnv("PROCESSOR_STREAM", "WLOW_PROCESSOR"),
		ProcessorSubjectPrefix: getEnv("PROCESSOR_SUBJECT_PREFIX", "wlow.processor"),
		MaxRetries:             getEnvInt("MAX_RETRIES", 3),
		AckTimeout:             getEnvDuration("ACK_TIMEOUT", 5*time.Minute),
		StreamMaxBytes:         getEnvInt64("STREAM_MAX_BYTES", 1024*1024*1024),
	}
}

func getEnv(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

func getEnvInt(key string, fallback int) int {
	if v := os.Getenv(key); v != "" {
		if i, err := strconv.Atoi(v); err == nil {
			return i
		}
	}
	return fallback
}

func getEnvInt64(key string, fallback int64) int64 {
	if v := os.Getenv(key); v != "" {
		if i, err := strconv.ParseInt(v, 10, 64); err == nil {
			return i
		}
	}
	return fallback
}

func getEnvDuration(key string, fallback time.Duration) time.Duration {
	if v := os.Getenv(key); v != "" {
		if d, err := time.ParseDuration(v); err == nil {
			return d
		}
	}
	return fallback
}
