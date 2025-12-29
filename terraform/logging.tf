# Cloud Run access logs → BigQuery (bypassing default Cloud Logging sink)

# Log sink to route Cloud Run request logs directly to BigQuery
resource "google_logging_project_sink" "cloud_run_to_bq" {
  name        = "${local.service_name}-${var.environment}-access-logs"
  description = "Routes Cloud Run access logs to BigQuery for analytics"

  destination = "bigquery.googleapis.com/projects/${var.project_id}/datasets/${google_bigquery_dataset.fingerprints.dataset_id}"

  filter = <<-EOT
    resource.type="cloud_run_revision"
    resource.labels.service_name="${google_cloud_run_v2_service.ingest.name}"
  EOT

  # Use partitioned tables for cost efficiency
  bigquery_options {
    use_partitioned_tables = true
  }

  # Ensure this sink is created before the exclusion
  depends_on = [google_bigquery_dataset.fingerprints]
}

# Grant the log sink permission to write to BigQuery
resource "google_bigquery_dataset_iam_member" "log_writer" {
  dataset_id = google_bigquery_dataset.fingerprints.dataset_id
  role       = "roles/bigquery.dataEditor"
  member     = google_logging_project_sink.cloud_run_to_bq.writer_identity
}

# Exclude Cloud Run logs from the _Default sink to avoid duplicate storage costs
resource "google_logging_project_exclusion" "cloud_run_default_exclusion" {
  name        = "${local.service_name}-${var.environment}-exclude-default"
  description = "Exclude Cloud Run access logs from _Default sink (routed to BigQuery instead)"

  filter = <<-EOT
    resource.type="cloud_run_revision"
    resource.labels.service_name="${google_cloud_run_v2_service.ingest.name}"
  EOT
}

# View for access logs analytics
resource "google_bigquery_table" "access_logs_view" {
  dataset_id          = google_bigquery_dataset.fingerprints.dataset_id
  table_id            = "access_logs"
  deletion_protection = false

  view {
    query = <<-SQL
      SELECT
        timestamp,
        httpRequest.requestUrl AS request_url,
        httpRequest.requestMethod AS method,
        httpRequest.status AS status_code,
        httpRequest.userAgent AS user_agent,
        httpRequest.remoteIp AS client_ip,
        httpRequest.latency AS latency,
        httpRequest.requestSize AS request_size,
        httpRequest.responseSize AS response_size,
        resource.labels.service_name AS service,
        resource.labels.revision_name AS revision
      FROM `${var.project_id}.${google_bigquery_dataset.fingerprints.dataset_id}.run_googleapis_com_requests`
      WHERE timestamp >= TIMESTAMP_SUB(CURRENT_TIMESTAMP(), INTERVAL 30 DAY)
      ORDER BY timestamp DESC
    SQL
    use_legacy_sql = false
  }

  depends_on = [google_logging_project_sink.cloud_run_to_bq]
}

# View for daily access stats (consent-free analytics)
resource "google_bigquery_table" "daily_access_stats_view" {
  dataset_id          = google_bigquery_dataset.fingerprints.dataset_id
  table_id            = "daily_access_stats"
  deletion_protection = false

  view {
    query = <<-SQL
      SELECT
        DATE(timestamp) AS date,
        COUNT(*) AS total_requests,
        COUNT(DISTINCT httpRequest.remoteIp) AS unique_ips,
        COUNTIF(httpRequest.status >= 200 AND httpRequest.status < 300) AS successful_requests,
        COUNTIF(httpRequest.status >= 400) AS error_requests,
        AVG(CAST(REGEXP_EXTRACT(httpRequest.latency, r'([0-9.]+)') AS FLOAT64)) AS avg_latency_seconds
      FROM `${var.project_id}.${google_bigquery_dataset.fingerprints.dataset_id}.run_googleapis_com_requests`
      GROUP BY DATE(timestamp)
      ORDER BY date DESC
    SQL
    use_legacy_sql = false
  }

  depends_on = [google_logging_project_sink.cloud_run_to_bq]
}

# View for top pages from access logs
resource "google_bigquery_table" "access_logs_top_pages_view" {
  dataset_id          = google_bigquery_dataset.fingerprints.dataset_id
  table_id            = "access_logs_top_pages"
  deletion_protection = false

  view {
    query = <<-SQL
      SELECT
        REGEXP_EXTRACT(httpRequest.requestUrl, r'^https?://[^/]+(/[^?]*)') AS path,
        COUNT(*) AS hits,
        COUNT(DISTINCT httpRequest.remoteIp) AS unique_ips,
        AVG(CAST(REGEXP_EXTRACT(httpRequest.latency, r'([0-9.]+)') AS FLOAT64)) AS avg_latency_seconds
      FROM `${var.project_id}.${google_bigquery_dataset.fingerprints.dataset_id}.run_googleapis_com_requests`
      WHERE timestamp >= TIMESTAMP_SUB(CURRENT_TIMESTAMP(), INTERVAL 30 DAY)
        AND httpRequest.status >= 200 AND httpRequest.status < 300
      GROUP BY path
      ORDER BY hits DESC
      LIMIT 100
    SQL
    use_legacy_sql = false
  }

  depends_on = [google_logging_project_sink.cloud_run_to_bq]
}
