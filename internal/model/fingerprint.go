package model

import "time"

// Fingerprint represents a browser fingerprint event from ThumbmarkJS
type Fingerprint struct {
	// Fingerprint hash from ThumbmarkJS
	Fingerprint string `json:"fingerprint"`

	// Page URL where the fingerprint was captured
	PageURL string `json:"page_url"`

	// Referrer URL
	Referrer string `json:"referrer"`

	// User agent string
	UserAgent string `json:"user_agent"`

	// Client IP address (captured server-side)
	ClientIP string `json:"client_ip"`

	// ISO 3166-1 alpha-2 country code (from Cloudflare header)
	Country string `json:"country"`

	// Timestamp when the event was received
	Timestamp time.Time `json:"timestamp"`

	// Unique event ID
	EventID string `json:"event_id"`
}

// FingerprintRequest represents the incoming request from the browser
type FingerprintRequest struct {
	Fingerprint string `json:"fingerprint"`
	PageURL     string `json:"page_url"`
	Referrer    string `json:"referrer"`
}
