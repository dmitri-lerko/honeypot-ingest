package handler

import (
	"encoding/json"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/dmitri-lerko/honeypot-ingest/internal/model"
	"github.com/dmitri-lerko/honeypot-ingest/internal/storage"
	"github.com/google/uuid"
)

// Handler handles fingerprint ingestion requests
type Handler struct {
	writer storage.Writer
	logger *slog.Logger
}

// New creates a new Handler
func New(writer storage.Writer, logger *slog.Logger) *Handler {
	return &Handler{
		writer: writer,
		logger: logger,
	}
}

// Ingest handles POST requests to ingest fingerprint data
func (h *Handler) Ingest(w http.ResponseWriter, r *http.Request) {
	// Only accept POST
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// Parse request body
	var req model.FingerprintRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		h.logger.Error("failed to decode request", "error", err)
		http.Error(w, "Invalid request body", http.StatusBadRequest)
		return
	}

	// Validate required fields
	if req.Fingerprint == "" {
		http.Error(w, "fingerprint is required", http.StatusBadRequest)
		return
	}

	// Build fingerprint model
	fp := model.Fingerprint{
		Fingerprint: req.Fingerprint,
		PageURL:     req.PageURL,
		Referrer:    req.Referrer,
		UserAgent:   r.Header.Get("User-Agent"),
		ClientIP:    extractClientIP(r),
		Country:     r.Header.Get("CF-IPCountry"), // Cloudflare header
		Timestamp:   time.Now().UTC(),
		EventID:     uuid.New().String(),
	}

	// Write to storage
	if err := h.writer.Write(r.Context(), fp); err != nil {
		h.logger.Error("failed to write fingerprint", "error", err, "event_id", fp.EventID)
		http.Error(w, "Internal server error", http.StatusInternalServerError)
		return
	}

	h.logger.Info("fingerprint ingested",
		"event_id", fp.EventID,
		"fingerprint", fp.Fingerprint[:min(8, len(fp.Fingerprint))]+"...",
		"page_url", fp.PageURL,
	)

	// Return success with CORS headers
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusAccepted)
	json.NewEncoder(w).Encode(map[string]string{
		"status":   "accepted",
		"event_id": fp.EventID,
	})
}

// Health handles health check requests
func (h *Handler) Health(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(map[string]string{"status": "healthy"})
}

// CORS wraps a handler with CORS support
func CORS(allowedOrigins []string) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			origin := r.Header.Get("Origin")

			// Check if origin is allowed
			allowed := false
			for _, o := range allowedOrigins {
				if o == "*" || o == origin {
					allowed = true
					break
				}
			}

			if allowed {
				w.Header().Set("Access-Control-Allow-Origin", origin)
				w.Header().Set("Access-Control-Allow-Methods", "POST, OPTIONS")
				w.Header().Set("Access-Control-Allow-Headers", "Content-Type")
				w.Header().Set("Access-Control-Max-Age", "86400")
			}

			// Handle preflight
			if r.Method == http.MethodOptions {
				w.WriteHeader(http.StatusNoContent)
				return
			}

			next.ServeHTTP(w, r)
		})
	}
}

// extractClientIP extracts the client IP from request headers
func extractClientIP(r *http.Request) string {
	// Cloudflare
	if ip := r.Header.Get("CF-Connecting-IP"); ip != "" {
		return ip
	}
	// Standard proxy header
	if ip := r.Header.Get("X-Forwarded-For"); ip != "" {
		// Take first IP in chain
		if idx := strings.Index(ip, ","); idx != -1 {
			return strings.TrimSpace(ip[:idx])
		}
		return ip
	}
	// X-Real-IP
	if ip := r.Header.Get("X-Real-IP"); ip != "" {
		return ip
	}
	// Fall back to RemoteAddr
	if idx := strings.LastIndex(r.RemoteAddr, ":"); idx != -1 {
		return r.RemoteAddr[:idx]
	}
	return r.RemoteAddr
}
