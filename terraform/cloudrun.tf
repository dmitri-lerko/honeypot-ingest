# Service account for Cloud Run
resource "google_service_account" "cloud_run" {
  account_id   = "${local.service_name}-${var.environment}"
  display_name = "Honeypot Ingest Cloud Run Service Account"
  description  = "Service account for honeypot-ingest Cloud Run service"
}

# Artifact Registry repository for container images
resource "google_artifact_registry_repository" "repo" {
  repository_id = local.service_name
  location      = var.region
  format        = "DOCKER"
  description   = "Container images for honeypot-ingest service"

  labels = {
    service     = local.service_name
    environment = var.environment
    managed_by  = "terraform"
  }

  depends_on = [google_project_service.apis]
}

# Cloud Run service
resource "google_cloud_run_v2_service" "ingest" {
  name     = "${local.service_name}-${var.environment}"
  location = var.region

  template {
    service_account = google_service_account.cloud_run.email

    scaling {
      min_instance_count = var.cloud_run_min_instances
      max_instance_count = var.cloud_run_max_instances
    }

    containers {
      image = "${var.region}-docker.pkg.dev/${var.project_id}/${google_artifact_registry_repository.repo.repository_id}/${local.service_name}:latest"

      resources {
        limits = {
          cpu    = "1000m"
          memory = "512Mi"
        }
        cpu_idle = true
      }

      env {
        name  = "GCS_BUCKET"
        value = google_storage_bucket.data_lake.name
      }

      env {
        name  = "GCS_PATH_PREFIX"
        value = "fingerprints"
      }

      env {
        name  = "BUFFER_SIZE"
        value = tostring(var.buffer_size)
      }

      env {
        name  = "FLUSH_INTERVAL"
        value = var.flush_interval
      }

      env {
        name  = "ALLOWED_ORIGINS"
        value = join(",", var.allowed_origins)
      }

      ports {
        container_port = 8080
      }

      startup_probe {
        http_get {
          path = "/health"
          port = 8080
        }
        initial_delay_seconds = 0
        period_seconds        = 10
        failure_threshold     = 3
      }

      liveness_probe {
        http_get {
          path = "/health"
          port = 8080
        }
        period_seconds    = 30
        failure_threshold = 3
      }
    }
  }

  traffic {
    type    = "TRAFFIC_TARGET_ALLOCATION_TYPE_LATEST"
    percent = 100
  }

  labels = {
    service     = local.service_name
    environment = var.environment
    managed_by  = "terraform"
  }

  depends_on = [
    google_artifact_registry_repository.repo,
    google_storage_bucket_iam_member.cloud_run_writer,
  ]
}

# Allow unauthenticated access to Cloud Run (public API)
resource "google_cloud_run_v2_service_iam_member" "public_access" {
  name     = google_cloud_run_v2_service.ingest.name
  location = var.region
  role     = "roles/run.invoker"
  member   = "allUsers"
}
