//go:build e2e

package e2e

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"

	"cloud.google.com/go/storage"
	"github.com/dmitri-lerko/honeypot-ingest/internal/handler"
	"github.com/dmitri-lerko/honeypot-ingest/internal/model"
	gcsStorage "github.com/dmitri-lerko/honeypot-ingest/internal/storage"
	"google.golang.org/api/iterator"
)

var (
	testBucket     = os.Getenv("E2E_GCS_BUCKET")
	testPathPrefix = fmt.Sprintf("e2e-test-%d", time.Now().Unix())
)

func TestMain(m *testing.M) {
	if testBucket == "" {
		fmt.Println("E2E_GCS_BUCKET not set, skipping e2e tests")
		os.Exit(0)
	}
	code := m.Run()
	cleanup()
	os.Exit(code)
}

func cleanup() {
	ctx := context.Background()
	client, err := storage.NewClient(ctx)
	if err != nil {
		fmt.Printf("Failed to create GCS client for cleanup: %v\n", err)
		return
	}
	defer client.Close()

	bucket := client.Bucket(testBucket)
	it := bucket.Objects(ctx, &storage.Query{Prefix: testPathPrefix})
	for {
		attrs, err := it.Next()
		if err == iterator.Done {
			break
		}
		if err != nil {
			fmt.Printf("Failed to list objects: %v\n", err)
			break
		}
		if err := bucket.Object(attrs.Name).Delete(ctx); err != nil {
			fmt.Printf("Failed to delete object %s: %v\n", attrs.Name, err)
		} else {
			fmt.Printf("Cleaned up: %s\n", attrs.Name)
		}
	}
}

func TestE2E_IngestAndWriteToGCS(t *testing.T) {
	ctx := context.Background()
	logger := slog.New(slog.NewTextHandler(os.Stdout, nil))

	// Create GCS writer with small buffer for testing
	config := gcsStorage.GCSConfig{
		Bucket:        testBucket,
		PathPrefix:    testPathPrefix,
		BufferSize:    2, // Small buffer to force writes
		FlushInterval: 10 * time.Second,
	}

	writer, err := gcsStorage.NewGCSWriter(ctx, config, logger)
	if err != nil {
		t.Fatalf("Failed to create GCS writer: %v", err)
	}
	defer writer.Close()

	// Create handler
	h := handler.New(writer, logger)

	// Create test server
	mux := http.NewServeMux()
	mux.HandleFunc("/ingest", h.Ingest)
	mux.HandleFunc("/health", h.Health)
	server := httptest.NewServer(mux)
	defer server.Close()

	// Test health endpoint
	t.Run("health check", func(t *testing.T) {
		resp, err := http.Get(server.URL + "/health")
		if err != nil {
			t.Fatalf("Health check failed: %v", err)
		}
		defer resp.Body.Close()

		if resp.StatusCode != http.StatusOK {
			t.Errorf("Expected status 200, got %d", resp.StatusCode)
		}
	})

	// Send fingerprint requests
	t.Run("ingest fingerprints", func(t *testing.T) {
		fingerprints := []model.FingerprintRequest{
			{Fingerprint: "e2e-test-fp-001", PageURL: "/page1", Referrer: "https://google.com"},
			{Fingerprint: "e2e-test-fp-002", PageURL: "/page2", Referrer: "https://bing.com"},
			{Fingerprint: "e2e-test-fp-003", PageURL: "/page3", Referrer: ""},
		}

		for i, fp := range fingerprints {
			body, _ := json.Marshal(fp)
			resp, err := http.Post(server.URL+"/ingest", "application/json", bytes.NewReader(body))
			if err != nil {
				t.Fatalf("Request %d failed: %v", i, err)
			}
			defer resp.Body.Close()

			if resp.StatusCode != http.StatusAccepted {
				bodyBytes, _ := io.ReadAll(resp.Body)
				t.Errorf("Request %d: expected status 202, got %d: %s", i, resp.StatusCode, string(bodyBytes))
			}

			var result map[string]string
			json.NewDecoder(resp.Body).Decode(&result)
			if result["event_id"] == "" {
				t.Errorf("Request %d: expected event_id in response", i)
			}
		}
	})

	// Force flush and verify files in GCS
	t.Run("verify GCS files", func(t *testing.T) {
		// Flush any remaining data
		if err := writer.Flush(ctx); err != nil {
			t.Fatalf("Flush failed: %v", err)
		}

		// Wait a bit for writes to complete
		time.Sleep(2 * time.Second)

		// List objects in GCS
		client, err := storage.NewClient(ctx)
		if err != nil {
			t.Fatalf("Failed to create GCS client: %v", err)
		}
		defer client.Close()

		bucket := client.Bucket(testBucket)
		it := bucket.Objects(ctx, &storage.Query{Prefix: testPathPrefix})

		var files []string
		for {
			attrs, err := it.Next()
			if err == iterator.Done {
				break
			}
			if err != nil {
				t.Fatalf("Failed to list objects: %v", err)
			}
			files = append(files, attrs.Name)
			t.Logf("Found file: %s (size: %d bytes)", attrs.Name, attrs.Size)
		}

		if len(files) == 0 {
			t.Error("No Parquet files found in GCS")
		}

		// Verify files are Parquet format
		for _, file := range files {
			if !strings.HasSuffix(file, ".parquet") {
				t.Errorf("Expected .parquet extension, got: %s", file)
			}
		}
	})
}

func TestE2E_CORSHeaders(t *testing.T) {
	ctx := context.Background()
	logger := slog.New(slog.NewTextHandler(os.Stdout, nil))

	config := gcsStorage.GCSConfig{
		Bucket:     testBucket,
		PathPrefix: testPathPrefix + "/cors",
		BufferSize: 100,
	}

	writer, err := gcsStorage.NewGCSWriter(ctx, config, logger)
	if err != nil {
		t.Fatalf("Failed to create GCS writer: %v", err)
	}
	defer writer.Close()

	h := handler.New(writer, logger)
	corsMiddleware := handler.CORS([]string{"https://deploy.live", "https://example.com"})

	mux := http.NewServeMux()
	mux.HandleFunc("/ingest", h.Ingest)
	server := httptest.NewServer(corsMiddleware(mux))
	defer server.Close()

	t.Run("preflight request", func(t *testing.T) {
		req, _ := http.NewRequest(http.MethodOptions, server.URL+"/ingest", nil)
		req.Header.Set("Origin", "https://deploy.live")

		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatalf("Request failed: %v", err)
		}
		defer resp.Body.Close()

		if resp.StatusCode != http.StatusNoContent {
			t.Errorf("Expected status 204, got %d", resp.StatusCode)
		}

		if resp.Header.Get("Access-Control-Allow-Origin") != "https://deploy.live" {
			t.Errorf("Expected CORS origin header")
		}
	})

	t.Run("actual request with CORS", func(t *testing.T) {
		body, _ := json.Marshal(model.FingerprintRequest{
			Fingerprint: "cors-test-fp",
			PageURL:     "/cors-test",
		})

		req, _ := http.NewRequest(http.MethodPost, server.URL+"/ingest", bytes.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Origin", "https://deploy.live")

		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatalf("Request failed: %v", err)
		}
		defer resp.Body.Close()

		if resp.StatusCode != http.StatusAccepted {
			t.Errorf("Expected status 202, got %d", resp.StatusCode)
		}

		if resp.Header.Get("Access-Control-Allow-Origin") != "https://deploy.live" {
			t.Errorf("Expected CORS origin header in response")
		}
	})
}

func TestE2E_InvalidRequests(t *testing.T) {
	ctx := context.Background()
	logger := slog.New(slog.NewTextHandler(os.Stdout, nil))

	config := gcsStorage.GCSConfig{
		Bucket:     testBucket,
		PathPrefix: testPathPrefix + "/invalid",
		BufferSize: 100,
	}

	writer, err := gcsStorage.NewGCSWriter(ctx, config, logger)
	if err != nil {
		t.Fatalf("Failed to create GCS writer: %v", err)
	}
	defer writer.Close()

	h := handler.New(writer, logger)
	mux := http.NewServeMux()
	mux.HandleFunc("/ingest", h.Ingest)
	server := httptest.NewServer(mux)
	defer server.Close()

	tests := []struct {
		name       string
		method     string
		body       string
		wantStatus int
	}{
		{"GET request", http.MethodGet, "", http.StatusMethodNotAllowed},
		{"empty body", http.MethodPost, "", http.StatusBadRequest},
		{"invalid JSON", http.MethodPost, "not-json", http.StatusBadRequest},
		{"missing fingerprint", http.MethodPost, `{"page_url":"/test"}`, http.StatusBadRequest},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var body io.Reader
			if tt.body != "" {
				body = strings.NewReader(tt.body)
			}

			req, _ := http.NewRequest(tt.method, server.URL+"/ingest", body)
			req.Header.Set("Content-Type", "application/json")

			resp, err := http.DefaultClient.Do(req)
			if err != nil {
				t.Fatalf("Request failed: %v", err)
			}
			defer resp.Body.Close()

			if resp.StatusCode != tt.wantStatus {
				t.Errorf("Expected status %d, got %d", tt.wantStatus, resp.StatusCode)
			}
		})
	}
}
