# Honeypot Ingest

A lightweight fingerprint ingestion service that collects browser fingerprints from [ThumbmarkJS](https://github.com/AltMechanic/ThumbmarkJS) and stores them in GCS using Parquet format for analysis via BigQuery managed Iceberg tables.

## Architecture

```
Browser (ThumbmarkJS) → Cloud Run → GCS (Parquet) → BigQuery (Iceberg)
```

### Data Flow

1. **Browser**: ThumbmarkJS generates a browser fingerprint
2. **Cloud Run**: Receives POST requests with fingerprint data
3. **GCS**: Stores data as Parquet files with date/hour partitioning
4. **BigQuery**: Queries data via BigLake managed Iceberg tables

## Quick Start

### Prerequisites

- Go 1.22+
- Docker
- Terraform 1.5+
- Google Cloud SDK (`gcloud`)
- Access to a GCP project

### Local Development

```bash
# Clone the repository
git clone https://github.com/dmitri-lerko/honeypot-ingest
cd honeypot-ingest

# Install dependencies
go mod tidy

# Run tests
make test

# Run locally (requires GCS_BUCKET env var)
export GCS_BUCKET=your-test-bucket
make run
```

### Deploy to GCP

```bash
# 1. Configure Terraform
cd terraform
cp terraform.tfvars.example terraform.tfvars
# Edit terraform.tfvars with your values

# 2. Initialize and apply Terraform
terraform init
terraform apply

# 3. Build and push container image
cd ..
make docker-push GCP_PROJECT=your-project GCP_REGION=europe-west2

# 4. Get the Cloud Run URL
terraform output cloud_run_url
```

## API Endpoints

### POST /ingest

Ingest a fingerprint event.

**Request:**
```json
{
  "fingerprint": "abc123def456...",
  "page_url": "/blog/my-post/",
  "referrer": "https://google.com"
}
```

**Response:**
```json
{
  "status": "accepted",
  "event_id": "550e8400-e29b-41d4-a716-446655440000"
}
```

### GET /health

Health check endpoint.

**Response:**
```json
{
  "status": "healthy"
}
```

## Configuration

| Environment Variable | Description | Default |
|---------------------|-------------|---------|
| `PORT` | HTTP server port | `8080` |
| `GCS_BUCKET` | GCS bucket for Parquet files | (required) |
| `GCS_PATH_PREFIX` | Path prefix within bucket | `fingerprints` |
| `BUFFER_SIZE` | Events to buffer before write | `100` |
| `FLUSH_INTERVAL` | Max time before flush | `1m` |
| `ALLOWED_ORIGINS` | CORS allowed origins (comma-separated) | `*` |

## BigQuery Tables

After deployment, the following BigQuery resources are created:

- `fingerprints` - Main Iceberg table with raw events
- `daily_visitors` - View: unique visitors per day
- `top_pages` - View: most visited pages (last 30 days)
- `visitor_countries` - View: visitors by country (last 30 days)

### Example Queries

```sql
-- Unique visitors today
SELECT COUNT(DISTINCT fingerprint) as unique_visitors
FROM `project.dataset.fingerprints`
WHERE DATE(timestamp) = CURRENT_DATE();

-- Top pages this week
SELECT page_url, COUNT(DISTINCT fingerprint) as visitors
FROM `project.dataset.fingerprints`
WHERE timestamp >= TIMESTAMP_SUB(CURRENT_TIMESTAMP(), INTERVAL 7 DAY)
GROUP BY page_url
ORDER BY visitors DESC
LIMIT 10;

-- Returning visitors (seen more than once)
SELECT fingerprint, COUNT(*) as visits
FROM `project.dataset.fingerprints`
GROUP BY fingerprint
HAVING visits > 1
ORDER BY visits DESC;
```

## Client Integration

Add this to your website after user consent:

```javascript
ThumbmarkJS.getFingerprint().then(function(fp) {
  fetch('https://your-cloud-run-url/ingest', {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({
      fingerprint: fp,
      page_url: window.location.pathname,
      referrer: document.referrer
    })
  });
});
```

## Testing

```bash
# Unit tests
make test-unit

# E2E tests (requires GCS bucket)
export E2E_GCS_BUCKET=your-test-bucket
make test-e2e

# All tests
make test-all
```

## Project Structure

```
.
├── cmd/server/          # Main application entry point
├── internal/
│   ├── handler/         # HTTP handlers
│   ├── model/           # Data models
│   └── storage/         # Storage implementations (GCS, memory)
├── e2e/                 # End-to-end tests
├── terraform/           # Infrastructure as code
├── Dockerfile           # Container definition
└── Makefile             # Build and test commands
```

## License

MIT
