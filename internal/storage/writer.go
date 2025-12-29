package storage

import (
	"context"

	"github.com/dmitri-lerko/honeypot-ingest/internal/model"
)

// Writer defines the interface for writing fingerprint data
type Writer interface {
	// Write writes a single fingerprint event
	Write(ctx context.Context, fp model.Fingerprint) error

	// Flush forces any buffered data to be written
	Flush(ctx context.Context) error

	// Close closes the writer and releases resources
	Close() error
}
