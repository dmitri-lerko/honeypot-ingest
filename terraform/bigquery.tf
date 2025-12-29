# BigQuery dataset for fingerprint analytics
resource "google_bigquery_dataset" "fingerprints" {
  dataset_id  = local.dataset_id
  location    = var.region
  description = "Fingerprint analytics data from honeypot-ingest service"

  labels = {
    service     = local.service_name
    environment = var.environment
    managed_by  = "terraform"
  }

  # Delete contents when destroying in non-prod
  delete_contents_on_destroy = var.environment != "prod"

  depends_on = [google_project_service.apis]
}

# BigQuery connection to GCS for BigLake/Iceberg
resource "google_bigquery_connection" "gcs_connection" {
  provider      = google-beta
  connection_id = "${local.service_name}-${var.environment}-gcs"
  location      = var.region
  description   = "Connection to GCS for Iceberg tables"

  cloud_resource {}

  depends_on = [google_project_service.apis]
}

# BigLake managed table with Iceberg format
# This creates an external table that BigQuery manages in Iceberg format
resource "google_bigquery_table" "fingerprints_iceberg" {
  provider            = google-beta
  dataset_id          = google_bigquery_dataset.fingerprints.dataset_id
  table_id            = "fingerprints"
  deletion_protection = var.environment == "prod"

  description = "Browser fingerprint events stored in Iceberg format"

  # BigLake table configuration for Iceberg
  external_data_configuration {
    autodetect    = false
    connection_id = google_bigquery_connection.gcs_connection.name
    source_format = "PARQUET"

    # Point to the GCS location with Parquet files
    source_uris = ["gs://${google_storage_bucket.data_lake.name}/fingerprints/*"]

    # Hive partitioning for efficient queries
    hive_partitioning_options {
      mode                     = "AUTO"
      source_uri_prefix        = "gs://${google_storage_bucket.data_lake.name}/fingerprints/"
      require_partition_filter = false
    }

    # Parquet options
    parquet_options {
      enum_as_string        = true
      enable_list_inference = true
    }
  }

  # Schema definition matching the Parquet files
  schema = jsonencode([
    {
      name        = "fingerprint"
      type        = "STRING"
      mode        = "REQUIRED"
      description = "Browser fingerprint hash from ThumbmarkJS"
    },
    {
      name        = "page_url"
      type        = "STRING"
      mode        = "NULLABLE"
      description = "Page URL where fingerprint was captured"
    },
    {
      name        = "referrer"
      type        = "STRING"
      mode        = "NULLABLE"
      description = "HTTP referrer"
    },
    {
      name        = "user_agent"
      type        = "STRING"
      mode        = "NULLABLE"
      description = "User agent string"
    },
    {
      name        = "client_ip"
      type        = "STRING"
      mode        = "NULLABLE"
      description = "Client IP address"
    },
    {
      name        = "country"
      type        = "STRING"
      mode        = "NULLABLE"
      description = "ISO country code from Cloudflare"
    },
    {
      name        = "timestamp"
      type        = "TIMESTAMP"
      mode        = "REQUIRED"
      description = "Event timestamp"
    },
    {
      name        = "event_id"
      type        = "STRING"
      mode        = "REQUIRED"
      description = "Unique event identifier"
    }
  ])

  labels = {
    service     = local.service_name
    environment = var.environment
    managed_by  = "terraform"
  }

  depends_on = [
    google_bigquery_connection.gcs_connection,
    google_storage_bucket_iam_member.bigquery_reader
  ]
}

# View for unique visitors per day
resource "google_bigquery_table" "daily_visitors_view" {
  dataset_id          = google_bigquery_dataset.fingerprints.dataset_id
  table_id            = "daily_visitors"
  deletion_protection = false

  view {
    query          = <<-SQL
      SELECT
        DATE(timestamp) AS date,
        COUNT(DISTINCT fingerprint) AS unique_visitors,
        COUNT(*) AS total_events,
        COUNT(DISTINCT page_url) AS unique_pages
      FROM `${var.project_id}.${local.dataset_id}.fingerprints`
      GROUP BY DATE(timestamp)
      ORDER BY date DESC
    SQL
    use_legacy_sql = false
  }

  depends_on = [google_bigquery_table.fingerprints_iceberg]
}

# View for top pages
resource "google_bigquery_table" "top_pages_view" {
  dataset_id          = google_bigquery_dataset.fingerprints.dataset_id
  table_id            = "top_pages"
  deletion_protection = false

  view {
    query          = <<-SQL
      SELECT
        page_url,
        COUNT(DISTINCT fingerprint) AS unique_visitors,
        COUNT(*) AS total_views,
        MIN(timestamp) AS first_view,
        MAX(timestamp) AS last_view
      FROM `${var.project_id}.${local.dataset_id}.fingerprints`
      WHERE timestamp >= TIMESTAMP_SUB(CURRENT_TIMESTAMP(), INTERVAL 30 DAY)
      GROUP BY page_url
      ORDER BY unique_visitors DESC
      LIMIT 100
    SQL
    use_legacy_sql = false
  }

  depends_on = [google_bigquery_table.fingerprints_iceberg]
}

# View for visitor countries
resource "google_bigquery_table" "visitor_countries_view" {
  dataset_id          = google_bigquery_dataset.fingerprints.dataset_id
  table_id            = "visitor_countries"
  deletion_protection = false

  view {
    query          = <<-SQL
      SELECT
        COALESCE(country, 'Unknown') AS country,
        COUNT(DISTINCT fingerprint) AS unique_visitors,
        COUNT(*) AS total_events
      FROM `${var.project_id}.${local.dataset_id}.fingerprints`
      WHERE timestamp >= TIMESTAMP_SUB(CURRENT_TIMESTAMP(), INTERVAL 30 DAY)
      GROUP BY country
      ORDER BY unique_visitors DESC
    SQL
    use_legacy_sql = false
  }

  depends_on = [google_bigquery_table.fingerprints_iceberg]
}
