package handler

import (
	"bytes"
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"

	"github.com/dmitri-lerko/honeypot-ingest/internal/model"
)

// mockWriter implements storage.Writer for testing
type mockWriter struct {
	written []model.Fingerprint
	err     error
}

func (m *mockWriter) Write(ctx context.Context, fp model.Fingerprint) error {
	if m.err != nil {
		return m.err
	}
	m.written = append(m.written, fp)
	return nil
}

func (m *mockWriter) Flush(ctx context.Context) error {
	return nil
}

func (m *mockWriter) Close() error {
	return nil
}

func TestHandler_Ingest_Success(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(os.Stdout, nil))
	mock := &mockWriter{}
	h := New(mock, logger)

	reqBody := model.FingerprintRequest{
		Fingerprint: "abc123def456",
		PageURL:     "https://example.com/page",
		Referrer:    "https://google.com",
	}
	body, _ := json.Marshal(reqBody)

	req := httptest.NewRequest(http.MethodPost, "/ingest", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("User-Agent", "TestAgent/1.0")
	req.Header.Set("CF-Connecting-IP", "1.2.3.4")
	req.Header.Set("CF-IPCountry", "GB")

	rr := httptest.NewRecorder()
	h.Ingest(rr, req)

	if rr.Code != http.StatusAccepted {
		t.Errorf("expected status %d, got %d", http.StatusAccepted, rr.Code)
	}

	if len(mock.written) != 1 {
		t.Fatalf("expected 1 fingerprint written, got %d", len(mock.written))
	}

	fp := mock.written[0]
	if fp.Fingerprint != "abc123def456" {
		t.Errorf("expected fingerprint 'abc123def456', got '%s'", fp.Fingerprint)
	}
	if fp.PageURL != "https://example.com/page" {
		t.Errorf("expected page_url 'https://example.com/page', got '%s'", fp.PageURL)
	}
	if fp.Referrer != "https://google.com" {
		t.Errorf("expected referrer 'https://google.com', got '%s'", fp.Referrer)
	}
	if fp.UserAgent != "TestAgent/1.0" {
		t.Errorf("expected user_agent 'TestAgent/1.0', got '%s'", fp.UserAgent)
	}
	if fp.ClientIP != "1.2.3.4" {
		t.Errorf("expected client_ip '1.2.3.4', got '%s'", fp.ClientIP)
	}
	if fp.Country != "GB" {
		t.Errorf("expected country 'GB', got '%s'", fp.Country)
	}
	if fp.EventID == "" {
		t.Error("expected event_id to be set")
	}
}

func TestHandler_Ingest_MissingFingerprint(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(os.Stdout, nil))
	mock := &mockWriter{}
	h := New(mock, logger)

	reqBody := model.FingerprintRequest{
		PageURL: "https://example.com/page",
	}
	body, _ := json.Marshal(reqBody)

	req := httptest.NewRequest(http.MethodPost, "/ingest", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")

	rr := httptest.NewRecorder()
	h.Ingest(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Errorf("expected status %d, got %d", http.StatusBadRequest, rr.Code)
	}

	if len(mock.written) != 0 {
		t.Errorf("expected no fingerprints written, got %d", len(mock.written))
	}
}

func TestHandler_Ingest_InvalidJSON(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(os.Stdout, nil))
	mock := &mockWriter{}
	h := New(mock, logger)

	req := httptest.NewRequest(http.MethodPost, "/ingest", bytes.NewReader([]byte("not json")))
	req.Header.Set("Content-Type", "application/json")

	rr := httptest.NewRecorder()
	h.Ingest(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Errorf("expected status %d, got %d", http.StatusBadRequest, rr.Code)
	}
}

func TestHandler_Ingest_MethodNotAllowed(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(os.Stdout, nil))
	mock := &mockWriter{}
	h := New(mock, logger)

	req := httptest.NewRequest(http.MethodGet, "/ingest", nil)

	rr := httptest.NewRecorder()
	h.Ingest(rr, req)

	if rr.Code != http.StatusMethodNotAllowed {
		t.Errorf("expected status %d, got %d", http.StatusMethodNotAllowed, rr.Code)
	}
}

func TestHandler_Health(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(os.Stdout, nil))
	mock := &mockWriter{}
	h := New(mock, logger)

	req := httptest.NewRequest(http.MethodGet, "/health", nil)
	rr := httptest.NewRecorder()
	h.Health(rr, req)

	if rr.Code != http.StatusOK {
		t.Errorf("expected status %d, got %d", http.StatusOK, rr.Code)
	}

	var resp map[string]string
	if err := json.NewDecoder(rr.Body).Decode(&resp); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}

	if resp["status"] != "healthy" {
		t.Errorf("expected status 'healthy', got '%s'", resp["status"])
	}
}

func TestCORS_Preflight(t *testing.T) {
	middleware := CORS([]string{"https://example.com"})
	handler := middleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest(http.MethodOptions, "/ingest", nil)
	req.Header.Set("Origin", "https://example.com")

	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusNoContent {
		t.Errorf("expected status %d, got %d", http.StatusNoContent, rr.Code)
	}

	if rr.Header().Get("Access-Control-Allow-Origin") != "https://example.com" {
		t.Errorf("expected CORS origin header, got '%s'", rr.Header().Get("Access-Control-Allow-Origin"))
	}
}

func TestCORS_DisallowedOrigin(t *testing.T) {
	middleware := CORS([]string{"https://example.com"})
	handler := middleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest(http.MethodPost, "/ingest", nil)
	req.Header.Set("Origin", "https://evil.com")

	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	if rr.Header().Get("Access-Control-Allow-Origin") != "" {
		t.Errorf("expected no CORS header for disallowed origin, got '%s'", rr.Header().Get("Access-Control-Allow-Origin"))
	}
}

func TestExtractClientIP(t *testing.T) {
	tests := []struct {
		name     string
		headers  map[string]string
		expected string
	}{
		{
			name:     "CF-Connecting-IP",
			headers:  map[string]string{"CF-Connecting-IP": "1.2.3.4"},
			expected: "1.2.3.4",
		},
		{
			name:     "X-Forwarded-For single",
			headers:  map[string]string{"X-Forwarded-For": "5.6.7.8"},
			expected: "5.6.7.8",
		},
		{
			name:     "X-Forwarded-For multiple",
			headers:  map[string]string{"X-Forwarded-For": "1.1.1.1, 2.2.2.2, 3.3.3.3"},
			expected: "1.1.1.1",
		},
		{
			name:     "X-Real-IP",
			headers:  map[string]string{"X-Real-IP": "9.10.11.12"},
			expected: "9.10.11.12",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodPost, "/", nil)
			for k, v := range tt.headers {
				req.Header.Set(k, v)
			}

			got := extractClientIP(req)
			if got != tt.expected {
				t.Errorf("expected '%s', got '%s'", tt.expected, got)
			}
		})
	}
}
