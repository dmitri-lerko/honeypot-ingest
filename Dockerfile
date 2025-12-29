# Build stage
FROM golang:1.22-alpine AS builder

WORKDIR /app

# Install build dependencies
RUN apk add --no-cache git ca-certificates

# Copy go mod files
COPY go.mod go.sum ./
RUN go mod download

# Copy source code
COPY . .

# Build binary
RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build \
    -ldflags="-w -s" \
    -o /honeypot-ingest \
    ./cmd/server

# Runtime stage
FROM gcr.io/distroless/static-debian12:nonroot

WORKDIR /

# Copy binary from builder
COPY --from=builder /honeypot-ingest /honeypot-ingest

# Expose port
EXPOSE 8080

# Run as non-root user
USER nonroot:nonroot

ENTRYPOINT ["/honeypot-ingest"]
