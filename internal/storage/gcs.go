package storage

import (
	"bytes"
	"context"
	"fmt"
	"log/slog"
	"sync"
	"time"

	"cloud.google.com/go/storage"
	"github.com/dmitri-lerko/honeypot-ingest/internal/model"
	"github.com/google/uuid"
	"github.com/parquet-go/parquet-go"
)

// GCSConfig holds configuration for GCS storage
type GCSConfig struct {
	// Bucket name for storing Parquet files
	Bucket string

	// PathPrefix is the base path within the bucket (e.g., "data/fingerprints")
	PathPrefix string

	// BufferSize is the number of events to buffer before writing a Parquet file
	BufferSize int

	// FlushInterval is the maximum time to wait before flushing buffered events
	FlushInterval time.Duration
}

// ParquetFingerprint is the Parquet schema representation
type ParquetFingerprint struct {
	Fingerprint string `parquet:"fingerprint"`
	PageURL     string `parquet:"page_url"`
	Referrer    string `parquet:"referrer"`
	UserAgent   string `parquet:"user_agent"`
	ClientIP    string `parquet:"client_ip"`
	Country     string `parquet:"country"`
	Timestamp   int64  `parquet:"timestamp,timestamp(millisecond)"`
	EventID     string `parquet:"event_id"`
}

// GCSWriter writes fingerprint data to GCS in Parquet format
// compatible with BigQuery managed Iceberg tables
type GCSWriter struct {
	client *storage.Client
	config GCSConfig
	logger *slog.Logger
	buffer []model.Fingerprint
	mu     sync.Mutex
	stopCh chan struct{}
	doneCh chan struct{}
}

// NewGCSWriter creates a new GCS Parquet writer
func NewGCSWriter(ctx context.Context, config GCSConfig, logger *slog.Logger) (*GCSWriter, error) {
	client, err := storage.NewClient(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to create GCS client: %w", err)
	}

	// Set defaults
	if config.BufferSize == 0 {
		config.BufferSize = 100
	}
	if config.FlushInterval == 0 {
		config.FlushInterval = 1 * time.Minute
	}

	w := &GCSWriter{
		client: client,
		config: config,
		logger: logger,
		buffer: make([]model.Fingerprint, 0, config.BufferSize),
		stopCh: make(chan struct{}),
		doneCh: make(chan struct{}),
	}

	// Start background flusher
	go w.backgroundFlusher()

	return w, nil
}

// Write buffers a fingerprint event for writing
func (w *GCSWriter) Write(ctx context.Context, fp model.Fingerprint) error {
	w.mu.Lock()
	defer w.mu.Unlock()

	w.buffer = append(w.buffer, fp)

	// Flush if buffer is full
	if len(w.buffer) >= w.config.BufferSize {
		return w.flushLocked(ctx)
	}

	return nil
}

// Flush forces buffered data to be written
func (w *GCSWriter) Flush(ctx context.Context) error {
	w.mu.Lock()
	defer w.mu.Unlock()
	return w.flushLocked(ctx)
}

// flushLocked writes buffered data to GCS (must be called with lock held)
func (w *GCSWriter) flushLocked(ctx context.Context) error {
	if len(w.buffer) == 0 {
		return nil
	}

	// Generate Iceberg-compatible path with partitioning
	// Format: data/fingerprints/dt=YYYY-MM-DD/HH/<uuid>.parquet
	now := time.Now().UTC()
	path := fmt.Sprintf("%s/dt=%s/%02d/%s.parquet",
		w.config.PathPrefix,
		now.Format("2006-01-02"),
		now.Hour(),
		uuid.New().String(),
	)

	w.logger.Info("flushing buffer to GCS",
		"path", path,
		"events", len(w.buffer),
	)

	// Write Parquet file to GCS
	if err := w.writeParquet(ctx, path, w.buffer); err != nil {
		return fmt.Errorf("failed to write parquet: %w", err)
	}

	// Clear buffer
	w.buffer = w.buffer[:0]

	return nil
}

// writeParquet writes fingerprints as a Parquet file to GCS
func (w *GCSWriter) writeParquet(ctx context.Context, path string, fingerprints []model.Fingerprint) error {
	// Convert to Parquet structs
	records := make([]ParquetFingerprint, len(fingerprints))
	for i, fp := range fingerprints {
		records[i] = ParquetFingerprint{
			Fingerprint: fp.Fingerprint,
			PageURL:     fp.PageURL,
			Referrer:    fp.Referrer,
			UserAgent:   fp.UserAgent,
			ClientIP:    fp.ClientIP,
			Country:     fp.Country,
			Timestamp:   fp.Timestamp.UnixMilli(),
			EventID:     fp.EventID,
		}
	}

	// Write to buffer first
	var buf bytes.Buffer
	if err := parquet.Write(&buf, records); err != nil {
		return fmt.Errorf("failed to write parquet data: %w", err)
	}

	// Upload to GCS
	obj := w.client.Bucket(w.config.Bucket).Object(path)
	gcsWriter := obj.NewWriter(ctx)
	gcsWriter.ContentType = "application/vnd.apache.parquet"

	if _, err := gcsWriter.Write(buf.Bytes()); err != nil {
		gcsWriter.Close()
		return fmt.Errorf("failed to write to GCS: %w", err)
	}

	if err := gcsWriter.Close(); err != nil {
		return fmt.Errorf("failed to close GCS writer: %w", err)
	}

	return nil
}

// backgroundFlusher periodically flushes the buffer
func (w *GCSWriter) backgroundFlusher() {
	defer close(w.doneCh)

	ticker := time.NewTicker(w.config.FlushInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			if err := w.Flush(context.Background()); err != nil {
				w.logger.Error("background flush failed", "error", err)
			}
		case <-w.stopCh:
			// Final flush on shutdown
			if err := w.Flush(context.Background()); err != nil {
				w.logger.Error("final flush failed", "error", err)
			}
			return
		}
	}
}

// Close stops the background flusher and closes the GCS client
func (w *GCSWriter) Close() error {
	close(w.stopCh)
	<-w.doneCh
	return w.client.Close()
}
