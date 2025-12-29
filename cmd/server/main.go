package main

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/dmitri-lerko/honeypot-ingest/internal/handler"
	"github.com/dmitri-lerko/honeypot-ingest/internal/storage"
)

func main() {
	// Setup structured logging
	logger := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{
		Level: slog.LevelInfo,
	}))
	slog.SetDefault(logger)

	// Load configuration from environment
	config := loadConfig()

	logger.Info("starting honeypot-ingest",
		"port", config.Port,
		"bucket", config.GCSBucket,
	)

	// Create GCS writer
	ctx := context.Background()
	gcsConfig := storage.GCSConfig{
		Bucket:        config.GCSBucket,
		PathPrefix:    config.GCSPathPrefix,
		BufferSize:    config.BufferSize,
		FlushInterval: config.FlushInterval,
	}

	writer, err := storage.NewGCSWriter(ctx, gcsConfig, logger)
	if err != nil {
		logger.Error("failed to create GCS writer", "error", err)
		os.Exit(1)
	}
	defer writer.Close()

	// Create handler
	h := handler.New(writer, logger)

	// Setup routes
	mux := http.NewServeMux()
	mux.HandleFunc("/ingest", h.Ingest)
	mux.HandleFunc("/health", h.Health)
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/" {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"service":"honeypot-ingest","version":"1.0.0"}`))
	})

	// Apply CORS middleware
	corsMiddleware := handler.CORS(config.AllowedOrigins)
	server := &http.Server{
		Addr:         fmt.Sprintf(":%s", config.Port),
		Handler:      corsMiddleware(mux),
		ReadTimeout:  10 * time.Second,
		WriteTimeout: 10 * time.Second,
		IdleTimeout:  60 * time.Second,
	}

	// Start server in goroutine
	go func() {
		logger.Info("server listening", "addr", server.Addr)
		if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			logger.Error("server error", "error", err)
			os.Exit(1)
		}
	}()

	// Wait for shutdown signal
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit

	logger.Info("shutting down server...")

	// Graceful shutdown with timeout
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	// Flush remaining data
	if err := writer.Flush(ctx); err != nil {
		logger.Error("failed to flush on shutdown", "error", err)
	}

	if err := server.Shutdown(ctx); err != nil {
		logger.Error("server shutdown error", "error", err)
		os.Exit(1)
	}

	logger.Info("server stopped")
}

type config struct {
	Port           string
	GCSBucket      string
	GCSPathPrefix  string
	BufferSize     int
	FlushInterval  time.Duration
	AllowedOrigins []string
}

func loadConfig() config {
	c := config{
		Port:           getEnv("PORT", "8080"),
		GCSBucket:      getEnv("GCS_BUCKET", ""),
		GCSPathPrefix:  getEnv("GCS_PATH_PREFIX", "fingerprints"),
		BufferSize:     getEnvInt("BUFFER_SIZE", 100),
		FlushInterval:  getEnvDuration("FLUSH_INTERVAL", 1*time.Minute),
		AllowedOrigins: strings.Split(getEnv("ALLOWED_ORIGINS", "*"), ","),
	}

	if c.GCSBucket == "" {
		slog.Error("GCS_BUCKET environment variable is required")
		os.Exit(1)
	}

	return c
}

func getEnv(key, defaultValue string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return defaultValue
}

func getEnvInt(key string, defaultValue int) int {
	if value := os.Getenv(key); value != "" {
		var i int
		fmt.Sscanf(value, "%d", &i)
		return i
	}
	return defaultValue
}

func getEnvDuration(key string, defaultValue time.Duration) time.Duration {
	if value := os.Getenv(key); value != "" {
		if d, err := time.ParseDuration(value); err == nil {
			return d
		}
	}
	return defaultValue
}
