output "cloud_run_url" {
  description = "URL of the Cloud Run service"
  value       = google_cloud_run_v2_service.ingest.uri
}

output "gcs_bucket" {
  description = "GCS bucket for Parquet/Iceberg data"
  value       = google_storage_bucket.data_lake.name
}

output "bigquery_dataset" {
  description = "BigQuery dataset ID"
  value       = google_bigquery_dataset.fingerprints.dataset_id
}

output "bigquery_table" {
  description = "BigQuery Iceberg table full path"
  value       = "${var.project_id}.${google_bigquery_dataset.fingerprints.dataset_id}.${google_bigquery_table.fingerprints_iceberg.table_id}"
}

output "artifact_registry" {
  description = "Artifact Registry repository for container images"
  value       = "${var.region}-docker.pkg.dev/${var.project_id}/${google_artifact_registry_repository.repo.repository_id}"
}

output "service_account_email" {
  description = "Cloud Run service account email"
  value       = google_service_account.cloud_run.email
}

output "ingest_endpoint" {
  description = "Full URL for the ingest endpoint"
  value       = "${google_cloud_run_v2_service.ingest.uri}/ingest"
}
