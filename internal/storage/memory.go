package storage

import (
	"context"
	"sync"

	"github.com/dmitri-lerko/honeypot-ingest/internal/model"
)

// MemoryWriter is an in-memory writer for testing
type MemoryWriter struct {
	mu          sync.Mutex
	fingerprints []model.Fingerprint
}

// NewMemoryWriter creates a new in-memory writer
func NewMemoryWriter() *MemoryWriter {
	return &MemoryWriter{
		fingerprints: make([]model.Fingerprint, 0),
	}
}

// Write stores a fingerprint in memory
func (w *MemoryWriter) Write(ctx context.Context, fp model.Fingerprint) error {
	w.mu.Lock()
	defer w.mu.Unlock()
	w.fingerprints = append(w.fingerprints, fp)
	return nil
}

// Flush is a no-op for memory writer
func (w *MemoryWriter) Flush(ctx context.Context) error {
	return nil
}

// Close is a no-op for memory writer
func (w *MemoryWriter) Close() error {
	return nil
}

// GetAll returns all stored fingerprints
func (w *MemoryWriter) GetAll() []model.Fingerprint {
	w.mu.Lock()
	defer w.mu.Unlock()
	result := make([]model.Fingerprint, len(w.fingerprints))
	copy(result, w.fingerprints)
	return result
}

// Clear removes all stored fingerprints
func (w *MemoryWriter) Clear() {
	w.mu.Lock()
	defer w.mu.Unlock()
	w.fingerprints = w.fingerprints[:0]
}

// Ensure MemoryWriter implements Writer
var _ Writer = (*MemoryWriter)(nil)
