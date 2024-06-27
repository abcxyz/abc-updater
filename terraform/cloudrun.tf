# Copyright 2024 The Authors (see AUTHORS file)
#
# Licensed under the Apache License, Version 2.0 (the "License");
# you may not use this file except in compliance with the License.
# You may obtain a copy of the License at
#
#     http://www.apache.org/licenses/LICENSE-2.0
#
# Unless required by applicable law or agreed to in writing, software
# distributed under the License is distributed on an "AS IS" BASIS,
# WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
# See the License for the specific language governing permissions and
# limitations under the License.


locals {
  default_run_envvars = {}
}

resource "google_cloud_run_v2_service" "metrics" {
  project = var.project_id

  name         = var.metrics_service_name
  location     = var.compute_region
  ingress      = "INGRESS_TRAFFIC_INTERNAL_LOAD_BALANCER"
  template {
    containers {
      image = "gcr.io/cloudrun/placeholder"
      resources {
        limits = {
          cpu    = "1"
          memory = "512Mi"
        }
      }
      dynamic "env" {
        for_each = merge(local.default_run_envvars, var.metrics_envvars)

        content {
          name  = env.key
          value = env.value
        }
      }
    }
    service_account = google_service_account.cloud_run_service_account.email
  }

  depends_on = [
    google_project_service.services["run.googleapis.com"],
  ]

  lifecycle {
    ignore_changes = [
      client,
      client_version,
      template[0].containers[0].image,
    ]
  }
}

resource "google_cloud_run_v2_service_iam_member" "public_metrics_access" {
  project = var.project_id

  location = google_cloud_run_v2_service.metrics.location
  name     = google_cloud_run_v2_service.metrics.name
  role     = "roles/run.invoker"
  members = [
    "allUsers"
  ]
}

// We want more narrow permissions than the default cloud run service account.
resource "google_service_account" "cloud_run_service_account" {
  project = var.project_id

  account_id   = "abc-m-sa"
  display_name = "ABC Metrics Server Cloud Run Service service account"
}

// External SA needs both run_as for the cloud run service account, as well as
// roles/run.developer.
resource "google_service_account_iam_member" "cloud_run_sa_user" {
  service_account_id = google_service_account.cloud_run_service_account.name
  role               = "roles/iam.serviceAccountUser"
  member             = "serviceAccount:${var.ci_service_account_email}"
}

resource "google_project_service_identity" "run_agent" {
  provider = google-beta
  project = var.project_id

  service = "run.googleapis.com"

  depends_on = [
    google_project_service.services["run.googleapis.com"],
  ]
}

// External SA needs both run_as for the cloud run service account, as well as
// roles/run.developer.
resource "google_cloud_run_v2_service_iam_member" "developers" {
  for_each = toset([
    "serviceAccount:${var.ci_service_account_email}",
  ])

  project = var.project_id

  location = google_cloud_run_v2_service.metrics.location
  name     = google_cloud_run_v2_service.metrics.name
  role     = "roles/run.developer"
  member   = each.value
}

