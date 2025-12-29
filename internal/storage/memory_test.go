package storage

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/dmitri-lerko/honeypot-ingest/internal/model"
)

func TestMemoryWriter_Write(t *testing.T) {
	w := NewMemoryWriter()
	ctx := context.Background()

	fp := model.Fingerprint{
		Fingerprint: "test123",
		PageURL:     "https://example.com",
		Timestamp:   time.Now(),
		EventID:     "event-1",
	}

	if err := w.Write(ctx, fp); err != nil {
		t.Fatalf("Write failed: %v", err)
	}

	all := w.GetAll()
	if len(all) != 1 {
		t.Fatalf("expected 1 fingerprint, got %d", len(all))
	}

	if all[0].Fingerprint != "test123" {
		t.Errorf("expected fingerprint 'test123', got '%s'", all[0].Fingerprint)
	}
}

func TestMemoryWriter_Clear(t *testing.T) {
	w := NewMemoryWriter()
	ctx := context.Background()

	w.Write(ctx, model.Fingerprint{Fingerprint: "fp1"})
	w.Write(ctx, model.Fingerprint{Fingerprint: "fp2"})

	if len(w.GetAll()) != 2 {
		t.Fatalf("expected 2 fingerprints before clear")
	}

	w.Clear()

	if len(w.GetAll()) != 0 {
		t.Errorf("expected 0 fingerprints after clear, got %d", len(w.GetAll()))
	}
}

func TestMemoryWriter_Concurrent(t *testing.T) {
	w := NewMemoryWriter()
	ctx := context.Background()

	var wg sync.WaitGroup
	numWriters := 100

	for i := 0; i < numWriters; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			fp := model.Fingerprint{
				Fingerprint: "concurrent-test",
				EventID:     string(rune(id)),
			}
			if err := w.Write(ctx, fp); err != nil {
				t.Errorf("concurrent write failed: %v", err)
			}
		}(i)
	}

	wg.Wait()

	all := w.GetAll()
	if len(all) != numWriters {
		t.Errorf("expected %d fingerprints, got %d", numWriters, len(all))
	}
}
