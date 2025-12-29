# GCS bucket for storing Parquet files (Iceberg data lake)
resource "google_storage_bucket" "data_lake" {
  name                        = local.bucket_name
  location                    = var.region
  uniform_bucket_level_access = true
  force_destroy               = var.environment != "prod"

  # Enable versioning for data protection
  versioning {
    enabled = var.environment == "prod"
  }

  # Lifecycle rules to manage storage costs
  lifecycle_rule {
    condition {
      age = 90
    }
    action {
      type          = "SetStorageClass"
      storage_class = "NEARLINE"
    }
  }

  lifecycle_rule {
    condition {
      age = 365
    }
    action {
      type          = "SetStorageClass"
      storage_class = "COLDLINE"
    }
  }

  # Soft delete for recovery
  soft_delete_policy {
    retention_duration_seconds = var.environment == "prod" ? 604800 : 86400 # 7 days prod, 1 day dev
  }

  labels = {
    service     = local.service_name
    environment = var.environment
    managed_by  = "terraform"
  }

  depends_on = [google_project_service.apis]
}

# IAM: Cloud Run service account can write to bucket
resource "google_storage_bucket_iam_member" "cloud_run_writer" {
  bucket = google_storage_bucket.data_lake.name
  role   = "roles/storage.objectUser"
  member = "serviceAccount:${google_service_account.cloud_run.email}"
}

# IAM: BigQuery connection can read from bucket
resource "google_storage_bucket_iam_member" "bigquery_reader" {
  bucket = google_storage_bucket.data_lake.name
  role   = "roles/storage.objectViewer"
  member = "serviceAccount:${google_bigquery_connection.gcs_connection.cloud_resource[0].service_account_id}"

  depends_on = [google_bigquery_connection.gcs_connection]
}
